package worldconn

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/agent-wow/agent-wow/pkg/auth"
)

// ErrWardenUnsupported reports a Warden request on a build whose client does
// not implement Warden.
var ErrWardenUnsupported = errors.New("the server sent Warden data, but this client build does not implement Warden; set Warden.Enable = 0 in the realm's anticheat.conf")

// headerCipher encrypts or decrypts packet headers in place, keeping its
// stream state across calls. *rc4.Cipher satisfies it for build 12340.
type headerCipher interface {
	XORKeyStream(dst, src []byte)
}

// classicCipher is the build-5875 header cipher, mangos-classic AuthCrypt.cpp
// at 8ec338a1, viewed from the client: it encrypts like the server's
// EncryptSend and decrypts like the server's DecryptRecv.
type classicCipher struct {
	key     [40]byte
	i, j    byte
	encrypt bool
}

func (c *classicCipher) XORKeyStream(dst, src []byte) {
	for n, b := range src {
		k := c.key[int(c.i)%len(c.key)]
		c.i = byte((int(c.i) + 1) % len(c.key))
		if c.encrypt {
			x := (b ^ k) + c.j
			c.j = x
			dst[n] = x
		} else {
			dst[n] = (b - c.j) ^ k
			c.j = b
		}
	}
}

// minimalLE trims the high zero bytes of a little-endian number, matching the
// server's BigNumber::AsByteArray() in the hashes it computes.
func minimalLE(b []byte) []byte {
	n := len(b)
	for n > 0 && b[n-1] == 0 {
		n--
	}
	return b[:n]
}

// classicAddons is the stock 1.12.1 client's Blizzard addon list. The server
// requires a non-empty, zlib-compressed block (AddonHandler ReadAddonInfo).
var classicAddons = []string{
	"Blizzard_AuctionUI", "Blizzard_BattlefieldMinimap", "Blizzard_BindingUI", "Blizzard_CombatText",
	"Blizzard_CraftUI", "Blizzard_GMSurveyUI", "Blizzard_InspectUI", "Blizzard_MacroUI",
	"Blizzard_RaidUI", "Blizzard_TalentUI", "Blizzard_TradeSkillUI", "Blizzard_TrainerUI",
}

func classicAddonBlock() ([]byte, error) {
	var plain bytes.Buffer
	for _, name := range classicAddons {
		plain.WriteString(name)
		plain.WriteByte(0)
		plain.WriteByte(1)                                             // enabled
		plain.Write(binary.LittleEndian.AppendUint32(nil, 0x4C1C776D)) // stock modulus CRC
		plain.Write(binary.LittleEndian.AppendUint32(nil, 0))          // URL CRC
	}
	var packed bytes.Buffer
	w := zlib.NewWriter(&packed)
	if _, err := w.Write(plain.Bytes()); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return append(binary.LittleEndian.AppendUint32(nil, uint32(plain.Len())), packed.Bytes()...), nil
}

// authenticateClassic answers SMSG_AUTH_CHALLENGE for build 5875
// (mangos-classic WorldSocket::HandleAuthSession at 8ec338a1).
func (c *Conn) authenticateClassic(session *auth.Session, challenge []byte) error {
	if len(challenge) != 4 {
		return errors.New("invalid realm authentication challenge")
	}
	var seed [4]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return err
	}
	username := session.Username
	h := sha1.New()
	h.Write([]byte(username))
	h.Write([]byte{0, 0, 0, 0})
	h.Write(seed[:])
	h.Write(challenge)
	h.Write(minimalLE(session.Key[:]))
	addons, err := classicAddonBlock()
	if err != nil {
		return fmt.Errorf("build addon info: %w", err)
	}
	body := binary.LittleEndian.AppendUint32(nil, uint32(c.Profile().Build))
	body = binary.LittleEndian.AppendUint32(body, 0) // unused server ID
	body = append(body, []byte(username)...)
	body = append(body, 0)
	body = append(body, seed[:]...)
	body = append(body, h.Sum(nil)...)
	body = append(body, addons...)
	if _, err := c.WritePacket(uint32(c.Profile().Op.CMSGAuthSession), body); err != nil {
		return err
	}
	c.send = &classicCipher{key: session.Key, encrypt: true}
	c.recv = &classicCipher{key: session.Key}
	for range 4096 {
		body, err := c.waitPacket(c.Profile().Op.SMSGAuthResponse)
		if err != nil {
			return fmt.Errorf("read authentication response (saved session may be stale): %w", err)
		}
		if len(body) == 0 {
			return errors.New("empty authentication response")
		}
		switch body[0] {
		case 0x0c: // AUTH_OK: billing fields, or the one-byte form after a queue.
			if len(body) != 1 && len(body) != 10 {
				return errors.New("invalid authentication success response")
			}
			return nil
		case 0x1b: // AUTH_WAIT_QUEUE: billing fields and position, then position updates.
			if len(body) != 5 && len(body) != 14 {
				return errors.New("invalid authentication queue response")
			}
		default:
			return &AuthError{Code: body[0]}
		}
	}
	return errors.New("too many authentication queue updates")
}
