package wire

import (
	"bytes"
	"compress/zlib"
	"testing"
)

// The builders below follow the mangos-classic writers independently of the
// parser: Object::BuildCreateUpdateBlockForPlayer, BuildMovementUpdate,
// BuildValuesUpdate and UpdateData::BuildPacket.

func values(fields map[uint16]uint32, count int) []byte {
	blocks := (count + 31) / 32
	mask := make([]uint32, blocks)
	for i := range fields {
		mask[i/32] |= 1 << (i % 32)
	}
	b := []byte{byte(blocks)}
	for _, w := range mask {
		b = AppendU32(b, w)
	}
	for i := 0; i < count; i++ {
		if v, ok := fields[uint16(i)]; ok {
			b = AppendU32(b, v)
		}
	}
	return b
}

func livingCreate(guid uint64, typeID byte, flags uint32, p Position, spline []byte, fields map[uint16]uint32) []byte {
	b := []byte{UpdateCreateObject2}
	b = AppendPackedGUID(b, guid)
	b = append(b, typeID, updateFlagLiving|updateFlagAll|updateFlagHasPosition)
	b = AppendU32(b, flags)
	b = AppendU32(b, 1234)
	for _, f := range []float32{p.X, p.Y, p.Z, p.O} {
		b = AppendF32(b, f)
	}
	b = AppendU32(b, 0) // fall time
	for _, s := range []float32{2.5, 7, 4.5, 4.722222, 2.5, 3.141594} {
		b = AppendF32(b, s)
	}
	b = append(b, spline...)
	b = AppendU32(b, 1) // UPDATEFLAG_ALL
	return append(b, values(fields, 0x2d0)...)
}

func packet(blocks ...[]byte) []byte {
	b := AppendU32(nil, uint32(len(blocks)))
	b = append(b, 0)
	for _, blk := range blocks {
		b = append(b, blk...)
	}
	return b
}

func TestParseUpdate(t *testing.T) {
	spline := AppendU32(nil, splineFinalTarget)
	spline = append(spline, 1, 2, 3, 4, 5, 6, 7, 8)                // facing target
	spline = AppendU32(AppendU32(AppendU32(spline, 100), 2000), 7) // passed, duration, id
	spline = AppendU32(spline, 2)
	for _, f := range []float32{1, 2, 3, 4, 5, 6, -8940, -130, 82} { // 2 nodes, destination
		spline = AppendF32(spline, f)
	}
	creature := uint64(0xF130000C9E000102)
	player := uint64(0x70c)
	outOfRange := []byte{UpdateOutOfRange}
	outOfRange = AppendU32(outOfRange, 2)
	outOfRange = AppendPackedGUID(AppendPackedGUID(outOfRange, 0xF1300000000000AA), 0x55)
	valuesBlock := AppendPackedGUID([]byte{UpdateValues}, creature)
	valuesBlock = append(valuesBlock, values(map[uint16]uint32{FieldHealth: 0, FieldDynamicFlags: DynFlagDead}, 0x90)...)
	body := packet(
		outOfRange,
		livingCreate(player, TypePlayer, 0, Position{-8950, -132.5, 83.5, 1.5}, nil,
			map[uint16]uint32{FieldHealth: 60, FieldMaxHealth: 60, FieldLevel: 1, FieldFactionTemplate: 1, FieldPlayerXP: 40, FieldPlayerNextXP: 400}),
		livingCreate(creature, TypeUnit, MoveForward|MoveSplineEnabled, Position{-8945, -131, 82, 0}, spline,
			map[uint16]uint32{FieldEntry: 299, FieldHealth: 45, FieldMaxHealth: 45, FieldLevel: 2, FieldFactionTemplate: 7}),
		valuesBlock,
	)
	blocks, err := ParseUpdate(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("got %d blocks", len(blocks))
	}
	if blocks[0].Type != UpdateOutOfRange || len(blocks[0].GUIDs) != 2 || blocks[0].GUIDs[0] != 0xF1300000000000AA || blocks[0].GUIDs[1] != 0x55 {
		t.Errorf("out of range block = %+v", blocks[0])
	}
	p := blocks[1]
	if p.GUID != player || p.TypeID != TypePlayer || p.Position == nil || p.Position.X != -8950 || p.RunSpeed != 7 ||
		p.Values[FieldPlayerXP] != 40 || p.Values[FieldPlayerNextXP] != 400 || p.Values[FieldHealth] != 60 {
		t.Errorf("player block = %+v", p)
	}
	c := blocks[2]
	if c.GUID != creature || c.Values[FieldEntry] != 299 || c.Values[FieldLevel] != 2 || c.Destination == nil || c.Destination.X != -8940 {
		t.Errorf("creature block = %+v dest %+v", c, c.Destination)
	}
	if v := blocks[3]; v.GUID != creature || v.Values[FieldHealth] != 0 || v.Values[FieldDynamicFlags] != DynFlagDead {
		t.Errorf("values block = %+v", v)
	}
	for i := 1; i < len(body); i++ {
		if _, err := ParseUpdate(body[:i]); err == nil {
			t.Fatalf("accepted a packet truncated to %d bytes", i)
		}
	}

	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write(body)
	w.Close()
	inflated, err := Inflate(append(AppendU32(nil, uint32(len(body))), z.Bytes()...))
	if err != nil || !bytes.Equal(inflated, body) {
		t.Fatalf("Inflate = %d bytes, %v", len(inflated), err)
	}
	if _, err := Inflate(append(AppendU32(nil, uint32(len(body)+1)), z.Bytes()...)); err == nil {
		t.Fatal("accepted a size mismatch")
	}
}

func TestPackedGUID(t *testing.T) {
	for _, g := range []uint64{0, 1, 0x70c, 0xF130000C9E000102, ^uint64(0)} {
		b := AppendPackedGUID(nil, g)
		if got := NewReader(b).PackedGUID(); got != g {
			t.Errorf("round trip %#x = %#x", g, got)
		}
	}
	if b := AppendPackedGUID(nil, 0x0100000000000002); !bytes.Equal(b, []byte{0x81, 0x02, 0x01}) {
		t.Errorf("packed = %x", b)
	}
}

func TestMovementInfoRoundTrip(t *testing.T) {
	b := AppendMovementInfo(nil, MoveForward, 5000, Position{1, 2, 3, 0.5})
	if len(b) != 28 {
		t.Fatalf("client MovementInfo is %d bytes, want 28", len(b))
	}
	r := NewReader(b)
	m := ReadMovementInfo(r)
	if r.Err() != nil || r.Len() != 0 || m.Flags != MoveForward || m.Time != 5000 || m.Pos != (Position{1, 2, 3, 0.5}) {
		t.Fatalf("read %+v err %v", m, r.Err())
	}
}

func TestParseMonsterMove(t *testing.T) {
	head := AppendPackedGUID(nil, 0xF130000C9E000102)
	for _, f := range []float32{-8945, -131, 82} {
		head = AppendF32(head, f)
	}
	head = AppendU32(head, 9) // spline id

	linear := append(append([]byte(nil), head...), 0) // normal
	linear = AppendU32(linear, 0x100)                 // run mode
	linear = AppendU32(linear, 1500)
	linear = AppendU32(linear, 3)
	for _, f := range []float32{-8930, -120, 81} {
		linear = AppendF32(linear, f)
	}
	linear = append(linear, 1, 2, 3, 4, 5, 6, 7, 8) // two packed offsets
	m, err := ParseMonsterMove(linear)
	if err != nil || m.Stop || m.Destination.X != -8930 || m.DurationMs != 1500 {
		t.Fatalf("linear = %+v, %v", m, err)
	}

	stop := append(append([]byte(nil), head...), 1)
	m, err = ParseMonsterMove(stop)
	if err != nil || !m.Stop || m.Destination.X != -8945 {
		t.Fatalf("stop = %+v, %v", m, err)
	}

	flying := append(append([]byte(nil), head...), 4) // facing angle
	flying = AppendF32(flying, 1)
	flying = AppendU32(flying, splineFlying)
	flying = AppendU32(flying, 900)
	flying = AppendU32(flying, 2)
	for _, f := range []float32{1, 1, 1, -8900, -100, 90} {
		flying = AppendF32(flying, f)
	}
	m, err = ParseMonsterMove(flying)
	if err != nil || m.Destination.X != -8900 || m.Destination.Z != 90 {
		t.Fatalf("flying = %+v, %v", m, err)
	}
}
