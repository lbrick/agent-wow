package char

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/agent-wow/agent-wow/pkg/protocol"
	"github.com/agent-wow/agent-wow/pkg/worldconn"
)

// classicRecord builds one mangos-classic Player::BuildEnumData record.
func classicRecord(guid uint64, name string, race, class, level byte, zone, mapID uint32) []byte {
	r := binary.LittleEndian.AppendUint64(nil, guid)
	r = append(r, name...)
	r = append(r, 0, race, class, 1, 2, 3, 4, 5, 6, level)
	r = binary.LittleEndian.AppendUint32(r, zone)
	r = binary.LittleEndian.AppendUint32(r, mapID)
	for _, f := range []float32{-8949.95, -132.493, 83.5312} {
		r = binary.LittleEndian.AppendUint32(r, math.Float32bits(f))
	}
	r = binary.LittleEndian.AppendUint32(r, 0) // guild
	r = binary.LittleEndian.AppendUint32(r, 0) // flags
	r = append(r, 1)                           // first login
	r = append(r, make([]byte, 12)...)         // pet display, level, family
	for slot := 0; slot < 20; slot++ {         // display ID and inventory type
		r = binary.LittleEndian.AppendUint32(r, uint32(slot))
		r = append(r, byte(slot))
	}
	return r
}

func TestDecodeClassicCharacters(t *testing.T) {
	body := append([]byte{2}, classicRecord(9, "Arlen", 1, 1, 12, 12, 0)...)
	body = append(body, classicRecord(10, "Mira", 8, 3, 1, 14, 1)...)
	characters, err := decodeClassicCharacters(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(characters) != 2 || characters[0].Name != "Arlen" || characters[0].Level != 12 || characters[0].ZoneID != 12 ||
		characters[1].Race != RaceTroll || characters[1].Class != ClassHunter || characters[1].MapID != 1 ||
		characters[0].Appearance != (Appearance{Skin: 2, Face: 3, HairStyle: 4, HairColor: 5, FacialHair: 6}) {
		t.Fatalf("characters = %+v", characters)
	}
	for name, bad := range map[string][]byte{
		"truncated": body[:len(body)-1],
		"trailing":  append(append([]byte(nil), body...), 0),
		"count":     {3, 0},
	} {
		if _, err := decodeClassicCharacters(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// The 3.3.5a decoder must reject a 1.12 list rather than misread it.
	if _, err := decodeCharacters(body); err == nil {
		t.Error("12340 decoder accepted a 1.12 character list")
	}
}

func TestClassicCreationCatalogue(t *testing.T) {
	c := &Client{wire: worldconn.New(nil, protocol.Classic)}
	seen := map[[2]uint8]bool{}
	for _, pair := range classicCatalogue.RaceClasses {
		race, class := Race(pair[0]), Class(pair[1])
		result, err := c.resolve(CreateOptions{Race: &race, Class: &class}, nil)
		if err != nil {
			t.Fatalf("%s %s: %v", race, class, err)
		}
		if result.Race != race || result.Class != class {
			t.Fatalf("resolved %+v", result)
		}
		seen[pair] = true
	}
	if len(seen) != 40 {
		t.Fatalf("1.12 catalogue has %d race/class pairs, want 40", len(seen))
	}
	for _, race := range []Race{RaceBloodElf, RaceDraenei} {
		if _, err := c.resolve(CreateOptions{Race: &race}, nil); err == nil {
			t.Errorf("1.12 created a %s", race)
		}
	}
	dk := ClassDeathKnight
	if _, err := c.resolve(CreateOptions{Class: &dk}, nil); err == nil {
		t.Error("1.12 created a Death Knight")
	}
}

func TestClassicResultCodes(t *testing.T) {
	codes := codesFor(protocol.Classic)
	if codes.createSuccess != 0x2e || codes.deleteSuccess != 0x39 || !codes.interim(0x2d) || !codes.retryableName(0x31) || !codes.retryableName(0x46) || codes.retryableName(0x2e) {
		t.Fatalf("unexpected 1.12 codes %+v", codes)
	}
	if err := (&ServerError{Operation: "create", Code: 0x31, messages: codes.messages}); err.Error() != "character create: name already in use (0x31)" {
		t.Fatal(err)
	}
}
