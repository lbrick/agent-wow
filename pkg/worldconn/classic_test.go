package worldconn

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/agent-wow/agent-wow/pkg/auth"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"github.com/agent-wow/agent-wow/pkg/protocol"
)

// classicServer is a fake build-5875 worldserver. Its header crypt is the
// server side of mangos-classic AuthCrypt.cpp, written independently of
// classicCipher.
type classicServer struct {
	conn                       net.Conn
	key                        []byte
	sendI, sendJ, recvI, recvJ byte
	crypt                      bool
}

func (s *classicServer) write(op uint16, body []byte) error {
	header := binary.BigEndian.AppendUint16(nil, uint16(len(body)+2))
	header = binary.LittleEndian.AppendUint16(header, op)
	if s.crypt {
		for t := range header { // AuthCrypt::EncryptSend
			s.sendI %= byte(len(s.key))
			x := (header[t] ^ s.key[s.sendI]) + s.sendJ
			s.sendI++
			header[t], s.sendJ = x, x
		}
	}
	_, err := s.conn.Write(append(header, body...))
	return err
}

func (s *classicServer) read() (uint32, []byte, error) {
	var header [6]byte
	if _, err := io.ReadFull(s.conn, header[:]); err != nil {
		return 0, nil, err
	}
	if s.crypt {
		for t := range header { // AuthCrypt::DecryptRecv
			s.recvI %= byte(len(s.key))
			x := (header[t] - s.recvJ) ^ s.key[s.recvI]
			s.recvI++
			s.recvJ = header[t]
			header[t] = x
		}
	}
	body := make([]byte, int(binary.BigEndian.Uint16(header[:2]))-4)
	if _, err := io.ReadFull(s.conn, body); err != nil {
		return 0, nil, err
	}
	return binary.LittleEndian.Uint32(header[2:]), body, nil
}

func classicSession() *auth.Session {
	s := &auth.Session{Username: "PLAYER"}
	for i := range s.Key {
		s.Key[i] = byte(3*i + 1)
	}
	s.Key[39] = 0 // the server hashes K without its high zero byte
	return s
}

// serveClassic runs handler as a fake realm and returns a selectable realm.
func serveClassic(t *testing.T, handler func(*classicServer) error) auth.Realm {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		done <- handler(&classicServer{conn: conn, key: classicSession().Key[:]})
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		if err := <-done; err != nil {
			t.Errorf("fake worldserver: %v", err)
		}
	})
	return auth.Realm{ID: 1, Name: "Classic", Address: listener.Addr().String(), ClientBuild: protocol.BuildClassic}
}

// acceptClassicAuth checks CMSG_AUTH_SESSION against WorldSocket::HandleAuthSession.
func acceptClassicAuth(s *classicServer, seed []byte) error {
	if err := s.write(classic.SMSGAuthChallenge, seed); err != nil {
		return err
	}
	op, body, err := s.read()
	if err != nil {
		return err
	}
	r := bytes.NewBuffer(body)
	var head [8]byte
	if _, err := io.ReadFull(r, head[:]); err != nil || op != classic.CMSGAuthSession || binary.LittleEndian.Uint32(head[:4]) != 5875 {
		return fmt.Errorf("invalid auth session opcode %#x or build", op)
	}
	account, err := r.ReadString(0)
	if err != nil || account != "PLAYER\x00" {
		return fmt.Errorf("invalid account %q", account)
	}
	var clientSeed [4]byte
	var digest [20]byte
	if _, err := io.ReadFull(r, clientSeed[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, digest[:]); err != nil {
		return err
	}
	h := sha1.New()
	h.Write([]byte("PLAYER"))
	h.Write([]byte{0, 0, 0, 0})
	h.Write(clientSeed[:])
	h.Write(seed)
	h.Write(bytes.TrimRight(s.key, "\x00"))
	if !bytes.Equal(h.Sum(nil), digest[:]) {
		return errors.New("auth session digest does not match")
	}
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return err
	}
	z, err := zlib.NewReader(r)
	if err != nil {
		return fmt.Errorf("addon block: %w", err)
	}
	addons, err := io.ReadAll(z)
	if err != nil || len(addons) == 0 || len(addons) != int(binary.LittleEndian.Uint32(size[:])) {
		return fmt.Errorf("addon block: %d bytes, %v", len(addons), err)
	}
	s.crypt = true
	// SendAuthOk: AUTH_OK and the three billing fields.
	return s.write(classic.SMSGAuthResponse, []byte{0x0c, 0, 0, 0, 0, 0, 0, 0, 0, 0})
}

func TestClassicDialAndFraming(t *testing.T) {
	seed := []byte{0x11, 0x22, 0x33, 0x44}
	realm := serveClassic(t, func(s *classicServer) error {
		if err := acceptClassicAuth(s, seed); err != nil {
			return err
		}
		op, body, err := s.read()
		if err != nil {
			return err
		}
		if op != classic.CMSGCharEnum || len(body) != 0 {
			return fmt.Errorf("decrypted %#x with %d bytes", op, len(body))
		}
		// A size with the high bit set must still be a four-byte header in 1.12.
		big := make([]byte, 0x8000)
		return s.write(classic.SMSGCharEnum, big)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := Dial(ctx, realm, classicSession())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Profile() != protocol.Classic || c.Expansion() != 0 {
		t.Fatalf("profile %v expansion %d", c.Profile(), c.Expansion())
	}
	if _, err := c.WritePacket(classic.CMSGCharEnum, nil); err != nil {
		t.Fatal(err)
	}
	op, body, err := c.ReadPacket()
	if err != nil || op != classic.SMSGCharEnum || len(body) != 0x8000 {
		t.Fatalf("ReadPacket = %#x, %d bytes, %v", op, len(body), err)
	}
}

func TestClassicWardenUnsupported(t *testing.T) {
	realm := serveClassic(t, func(s *classicServer) error {
		if err := s.write(classic.SMSGAuthChallenge, []byte{1, 2, 3, 4}); err != nil {
			return err
		}
		if _, _, err := s.read(); err != nil {
			return err
		}
		s.crypt = true
		// Warden starts before the auth response when Warden.Enable = 1.
		if err := s.write(classic.SMSGWardenData, []byte{0, 1, 2}); err != nil {
			return err
		}
		_, _, _ = s.read() // the client hangs up instead of answering
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Dial(ctx, realm, classicSession())
	if !errors.Is(err, ErrWardenUnsupported) {
		t.Fatalf("Dial error = %v, want ErrWardenUnsupported", err)
	}
}

func TestClassicAuthQueueAndRejection(t *testing.T) {
	for name, tc := range map[string]struct {
		responses [][]byte
		wantCode  uint8
	}{
		"queued then admitted": {responses: [][]byte{{0x1b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0}, {0x1b, 1, 0, 0, 0}, {0x0c}}},
		"unknown account":      {responses: [][]byte{{0x15}}, wantCode: 0x15},
	} {
		t.Run(name, func(t *testing.T) {
			realm := serveClassic(t, func(s *classicServer) error {
				if err := s.write(classic.SMSGAuthChallenge, []byte{9, 9, 9, 9}); err != nil {
					return err
				}
				if _, _, err := s.read(); err != nil {
					return err
				}
				s.crypt = true
				for _, r := range tc.responses {
					if err := s.write(classic.SMSGAuthResponse, r); err != nil {
						return err
					}
				}
				return nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			c, err := Dial(ctx, realm, classicSession())
			var rejected *AuthError
			if tc.wantCode != 0 {
				if !errors.As(err, &rejected) || rejected.Code != tc.wantCode {
					t.Fatalf("Dial error = %v, want code %#x", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			c.Close()
		})
	}
}
