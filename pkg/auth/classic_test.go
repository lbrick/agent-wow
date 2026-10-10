package auth

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/agent-wow/agent-wow/pkg/protocol"
)

// classicServerTrim mirrors BigNumber::AsByteArray(): the server hashes
// little-endian integers without their high zero bytes.
func classicServerTrim(b []byte) []byte {
	return bytes.TrimRight(b, "\x00")
}

// classicServerKey mirrors mangos-classic SRP6::HashSessionKey: it interleaves
// SHA-1 digests of the even and odd bytes of all 32 bytes of S.
func classicServerKey(s [32]byte) [40]byte {
	var even, odd [16]byte
	for i := range even {
		even[i], odd[i] = s[2*i], s[2*i+1]
	}
	e, o := digest(even[:]), digest(odd[:])
	var key [40]byte
	for i := range e {
		key[2*i], key[2*i+1] = e[i], o[i]
	}
	return key
}

func TestClassicLogin(t *testing.T) {
	address, serverKey := serveClassicLogon(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := NewClient(address, protocol.Classic)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Login(ctx, "player", "password")
	if err != nil {
		t.Fatal(err)
	}
	if session.Key != *serverKey || session.AccountFlags != 0 || session.Username != "PLAYER" {
		t.Fatalf("session = %+v, want the server's key and no account flags", session)
	}
}

func TestClassicLoginRealms(t *testing.T) {
	address, serverKey := serveClassicLogon(t, func(conn net.Conn) error {
		var request [5]byte
		if _, err := io.ReadFull(conn, request[:]); err != nil {
			return err
		}
		if request != [5]byte{0x10} {
			return fmt.Errorf("invalid realm list request %x", request)
		}
		_, err := conn.Write(classicRealmList(classicRealmRecord(1, 0, "MaNGOS", "127.0.0.1:8085", 0, 1, 1)))
		return err
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := NewClient(address, protocol.Classic)
	if err != nil {
		t.Fatal(err)
	}
	session, realms, err := client.LoginRealms(ctx, "player", "password")
	if err != nil {
		t.Fatal(err)
	}
	if session.Key != *serverKey || len(realms) != 1 || realms[0].Name != "MaNGOS" || realms[0].ClientBuild != protocol.BuildClassic {
		t.Fatalf("LoginRealms = %+v, %+v", session, realms)
	}
}

// serveClassicLogon runs a cMaNGOS Classic logon for PLAYER:PASSWORD, then
// next on the same connection. It returns the address and the server's key,
// which is set once the client's proof has been checked.
func serveClassicLogon(t *testing.T, next func(net.Conn) error) (string, *[40]byte) {
	t.Helper()
	// A salt with a high zero byte exercises the trimmed hashing.
	var c challenge
	for i := range c.salt {
		c.salt[i] = byte(i + 1)
	}
	c.salt[31] = 0
	identity := digest([]byte("PLAYER:PASSWORD"))
	x := digest(classicServerTrim(c.salt[:]), identity[:])
	v := new(big.Int).Exp(big.NewInt(7), fromLittleEndian(x[:]), modulus)
	var private [32]byte
	for i := range private {
		private[i] = byte(64 + i)
	}
	b := fromLittleEndian(private[:])
	public := new(big.Int).Add(new(big.Int).Mul(big.NewInt(3), v), new(big.Int).Exp(big.NewInt(7), b, modulus))
	c.public = toLittleEndian(public.Mod(public, modulus))

	serverKey := new([40]byte)
	address := serve(t, func(conn net.Conn) error {
		var request [40]byte
		if _, err := io.ReadFull(conn, request[:]); err != nil {
			return err
		}
		if request[0] != 0 || request[1] != 3 || !bytes.Equal(request[8:11], []byte{1, 12, 1}) ||
			binary.LittleEndian.Uint16(request[11:13]) != 5875 || string(request[34:]) != "PLAYER" {
			return fmt.Errorf("invalid 1.12.1 logon challenge: %x", request)
		}
		if _, err := conn.Write(challengePacket(c)); err != nil {
			return err
		}
		var proof [75]byte
		if _, err := io.ReadFull(conn, proof[:]); err != nil {
			return err
		}
		a := proof[1:33]
		u := digest(classicServerTrim(a), classicServerTrim(c.public[:]))
		base := new(big.Int).Mul(fromLittleEndian(a), new(big.Int).Exp(v, fromLittleEndian(u[:]), modulus))
		*serverKey = classicServerKey(toLittleEndian(new(big.Int).Exp(base, b, modulus)))
		n := toLittleEndian(modulus)
		group, gHash := digest(n[:]), digest([]byte{7})
		for i := range group {
			group[i] ^= gHash[i]
		}
		name := digest([]byte("PLAYER"))
		m1 := digest(classicServerTrim(group[:]), name[:], classicServerTrim(c.salt[:]), classicServerTrim(a),
			classicServerTrim(c.public[:]), classicServerTrim(serverKey[:]))
		if !bytes.Equal(proof[33:53], m1[:]) {
			_, err := conn.Write([]byte{1, 4})
			return errors.Join(errors.New("client proof does not match the cMaNGOS equation"), err)
		}
		m2 := digest(classicServerTrim(a), classicServerTrim(m1[:]), classicServerTrim(serverKey[:]))
		// sAuthLogonProof_S_BUILD_6005: cmd, error, M2, LoginFlags.
		response := append([]byte{1, 0}, m2[:]...)
		if _, err := conn.Write(binary.LittleEndian.AppendUint32(response, 0)); err != nil || next == nil {
			return err
		}
		return next(conn)
	})
	return address, serverKey
}

func TestClassicReconnectProofLayout(t *testing.T) {
	session := &Session{Username: "PLAYER"}
	for i := range session.Key {
		session.Key[i] = byte(i + 1)
	}
	session.Key[39] = 0
	var nonce [16]byte
	for i := range nonce {
		nonce[i] = byte(0xa0 + i)
	}
	nonce[15] = 0 // the server hashes it trimmed
	address := serve(t, func(conn net.Conn) error {
		var header [4]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return err
		}
		if header[0] != 2 || header[1] != 3 {
			return fmt.Errorf("invalid reconnect challenge header %x", header)
		}
		if _, err := io.ReadFull(conn, make([]byte, binary.LittleEndian.Uint16(header[2:]))); err != nil {
			return err
		}
		if _, err := conn.Write(append(append([]byte{2, 0}, nonce[:]...), make([]byte, 16)...)); err != nil {
			return err
		}
		// mangos-classic: the dispatcher reads the command byte, then
		// _HandleReconnectProof reads sizeof(sAuthReconnectProof_C), which
		// starts with its own cmd field.
		var cmd [1]byte
		var proof [58]byte
		if _, err := io.ReadFull(conn, cmd[:]); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, proof[:]); err != nil {
			return err
		}
		r1 := proof[1:17]
		want := digest([]byte("PLAYER"), classicServerTrim(r1), classicServerTrim(nonce[:]), classicServerTrim(session.Key[:]))
		if cmd[0] != 3 || !bytes.Equal(proof[17:37], want[:]) {
			return fmt.Errorf("reconnect proof misaligned or wrong: cmd %d", cmd[0])
		}
		_, err := conn.Write([]byte{3, 0, 0, 0})
		return err
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := NewClient(address, protocol.Classic)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckSession(ctx, session); err != nil {
		t.Fatal(err)
	}
}

func classicRealmRecord(realmType uint32, flags byte, name, address string, population float32, characters, category byte) []byte {
	record := binary.LittleEndian.AppendUint32(nil, realmType)
	record = append(record, flags)
	record = append(record, name...)
	record = append(record, 0)
	record = append(record, address...)
	record = append(record, 0)
	record = binary.LittleEndian.AppendUint32(record, math.Float32bits(population))
	return append(record, characters, category, 0)
}

func classicRealmList(records ...[]byte) []byte {
	body := binary.LittleEndian.AppendUint32(nil, 0)
	body = append(body, byte(len(records)))
	for _, r := range records {
		body = append(body, r...)
	}
	body = append(body, 2, 0)
	return append([]byte{0x10, byte(len(body)), byte(len(body) >> 8)}, body...)
}

func TestReadClassicRealms(t *testing.T) {
	packet := classicRealmList(
		classicRealmRecord(1, RealmFlagSpecifyBuild, "MaNGOS (1,12,1)", "172.18.0.23:8085", 0.5, 2, 1),
		classicRealmRecord(4, RealmFlagOffline, "Test", "127.0.0.1:8086", 0, 0, 1),
	)
	realms, err := readClassicRealms(bytes.NewReader(packet))
	if err != nil {
		t.Fatal(err)
	}
	want := []Realm{
		{ID: 1, Name: "MaNGOS (1,12,1)", Address: "172.18.0.23:8085", Type: 1, Population: 0.5, Characters: 2, Timezone: 1},
		{ID: 2, Name: "Test", Address: "127.0.0.1:8086", Type: 4, Flags: RealmFlagOffline, Timezone: 1},
	}
	if len(realms) != len(want) {
		t.Fatalf("got %d realms", len(realms))
	}
	for i := range want {
		if realms[i] != want[i] {
			t.Errorf("realm %d = %+v, want %+v", i, realms[i], want[i])
		}
	}
	realms[0].ClientBuild = protocol.BuildClassic
	if !realms[0].Selectable() || realms[1].Selectable() {
		t.Error("unexpected realm availability")
	}
	for name, bad := range map[string][]byte{
		"truncated":   packet[:len(packet)-3],
		"bad trailer": append(append([]byte(nil), packet[:len(packet)-2]...), 0x10, 0),
		"bad count":   classicRealmList()[:3],
		"wrong type":  classicRealmList(classicRealmRecord(1<<9, 0, "X", "h:1", 0, 0, 0)),
		"bad address": classicRealmList(classicRealmRecord(1, 0, "X", "nohost", 0, 0, 0)),
	} {
		if _, err := readClassicRealms(bytes.NewReader(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestClassicTrim(t *testing.T) {
	if got := classicSRP.trim([]byte{1, 2, 0, 0}); !bytes.Equal(got, []byte{1, 2}) {
		t.Errorf("trim = %x", got)
	}
	if got := azerothSRP.trim([]byte{1, 0}); len(got) != 2 {
		t.Errorf("12340 trimmed: %x", got)
	}
}
