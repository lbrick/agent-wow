package wire

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

// Update block types (mangos-classic UpdateData.h).
const (
	UpdateValues        = 0
	UpdateMovement      = 1
	UpdateCreateObject  = 2
	UpdateCreateObject2 = 3
	UpdateOutOfRange    = 4
	UpdateNearObjects   = 5
)

// Update flags of the movement section.
const (
	updateFlagTransport       = 0x02
	updateFlagAttackingTarget = 0x04
	updateFlagHighGUID        = 0x08
	updateFlagAll             = 0x10
	updateFlagLiving          = 0x20
	updateFlagHasPosition     = 0x40
)

// Block is one decoded SMSG_UPDATE_OBJECT block.
type Block struct {
	Type   uint8
	GUID   uint64
	TypeID uint8
	// Position is set when the block carries one (create blocks of units and
	// positioned objects).
	Position *Position
	// RunSpeed is the unit's run speed from a living movement section.
	RunSpeed float32
	// Destination is the end of a spline the unit is following.
	Destination *Position
	Values      map[uint16]uint32
	// GUIDs lists objects for out-of-range and near-objects blocks.
	GUIDs []uint64
}

// Inflate returns the body of SMSG_COMPRESSED_UPDATE_OBJECT: a uint32
// uncompressed size followed by zlib data.
func Inflate(body []byte) ([]byte, error) {
	if len(body) < 4 {
		return nil, ErrShort
	}
	size := int(NewReader(body[:4]).U32())
	if size > 1<<22 {
		return nil, fmt.Errorf("compressed update of %d bytes is too large", size)
	}
	z, err := zlib.NewReader(bytes.NewReader(body[4:]))
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(io.LimitReader(z, int64(size)+1))
	if err != nil {
		return nil, err
	}
	if len(out) != size {
		return nil, fmt.Errorf("compressed update inflated to %d bytes, expected %d", len(out), size)
	}
	return out, nil
}

// ParseUpdate decodes an uncompressed SMSG_UPDATE_OBJECT body.
func ParseUpdate(body []byte) ([]Block, error) {
	r := NewReader(body)
	count := r.U32()
	r.U8() // has transport
	if count > 10000 {
		return nil, fmt.Errorf("update claims %d blocks", count)
	}
	blocks := make([]Block, 0, count)
	for i := uint32(0); i < count && r.Err() == nil; i++ {
		var b Block
		b.Type = r.U8()
		switch b.Type {
		case UpdateValues:
			b.GUID = r.PackedGUID()
			b.Values = readValues(r)
		case UpdateMovement:
			b.GUID = r.PackedGUID()
			readMovement(r, &b)
		case UpdateCreateObject, UpdateCreateObject2:
			b.GUID = r.PackedGUID()
			b.TypeID = r.U8()
			readMovement(r, &b)
			b.Values = readValues(r)
		case UpdateOutOfRange, UpdateNearObjects:
			n := r.U32()
			if n > 10000 {
				return blocks, fmt.Errorf("update block lists %d objects", n)
			}
			for j := uint32(0); j < n && r.Err() == nil; j++ {
				b.GUIDs = append(b.GUIDs, r.PackedGUID())
			}
		default:
			return blocks, fmt.Errorf("unknown update block type %d", b.Type)
		}
		if r.Err() != nil {
			break
		}
		blocks = append(blocks, b)
	}
	if r.Err() != nil {
		return blocks, r.Err()
	}
	if r.Len() != 0 {
		return blocks, errors.New("trailing data after update blocks")
	}
	return blocks, nil
}

// readMovement follows Object::BuildMovementUpdate.
func readMovement(r *Reader, b *Block) {
	flags := r.U8()
	switch {
	case flags&updateFlagLiving != 0:
		info := ReadMovementInfo(r)
		pos := info.Pos
		b.Position = &pos
		var speeds [6]float32 // walk, run, run back, swim, swim back, turn rate
		for i := range speeds {
			speeds[i] = r.F32()
		}
		b.RunSpeed = speeds[1]
		if info.Flags&MoveSplineEnabled != 0 {
			b.Destination = readSplineCreate(r)
		}
	case flags&updateFlagHasPosition != 0:
		p := Position{r.F32(), r.F32(), r.F32(), r.F32()}
		b.Position = &p
	}
	if flags&updateFlagHighGUID != 0 {
		r.U32()
	}
	if flags&updateFlagAll != 0 {
		r.U32()
	}
	if flags&updateFlagAttackingTarget != 0 {
		r.PackedGUID()
	}
	if flags&updateFlagTransport != 0 {
		r.U32()
	}
}

// Spline flags (mangos-classic MoveSplineFlag.h).
const (
	splineFinalPoint  = 0x00010000
	splineFinalTarget = 0x00020000
	splineFinalAngle  = 0x00040000
)

// readSplineCreate follows PacketBuilder::WriteCreate and returns the final
// destination.
func readSplineCreate(r *Reader) *Position {
	flags := r.U32()
	switch {
	case flags&splineFinalPoint != 0:
		r.Skip(12)
	case flags&splineFinalTarget != 0:
		r.Skip(8)
	case flags&splineFinalAngle != 0:
		r.Skip(4)
	}
	r.U32() // time passed
	r.U32() // duration
	r.U32() // id
	nodes := r.U32()
	if nodes > 1000 {
		r.err = fmt.Errorf("spline with %d nodes", nodes)
		return nil
	}
	r.Skip(int(nodes) * 12)
	dest := Position{X: r.F32(), Y: r.F32(), Z: r.F32()}
	return &dest
}

// readValues reads a block count, the update mask and one uint32 per set bit.
func readValues(r *Reader) map[uint16]uint32 {
	blocks := int(r.U8())
	mask := make([]uint32, blocks)
	for i := range mask {
		mask[i] = r.U32()
	}
	values := make(map[uint16]uint32)
	for i, word := range mask {
		for bit := 0; bit < 32; bit++ {
			if word&(1<<bit) != 0 {
				values[uint16(i*32+bit)] = r.U32()
			}
		}
	}
	return values
}

// MonsterMove is the decoded part of SMSG_MONSTER_MOVE the modules need.
type MonsterMove struct {
	GUID        uint64
	Start       Position
	Stop        bool
	Destination Position
	DurationMs  uint32
}

// splineFlying selects the Catmull-Rom path encoding.
const splineFlying = 0x00000200

// ParseMonsterMove follows MoveSplineInit and PacketBuilder::WriteMonsterMove.
func ParseMonsterMove(body []byte) (MonsterMove, error) {
	r := NewReader(body)
	var m MonsterMove
	m.GUID = r.PackedGUID()
	m.Start = Position{X: r.F32(), Y: r.F32(), Z: r.F32()}
	r.U32() // spline id
	switch r.U8() {
	case 1: // MonsterMoveStop
		m.Stop = true
		m.Destination = m.Start
		return m, r.Err()
	case 2: // facing spot
		r.Skip(12)
	case 3: // facing target
		r.Skip(8)
	case 4: // facing angle
		r.Skip(4)
	}
	flags := r.U32()
	m.DurationMs = r.U32()
	count := r.U32()
	if count == 0 || count > 1000 {
		return m, fmt.Errorf("monster move with %d points", count)
	}
	if flags&splineFlying != 0 {
		r.Skip(int(count-1) * 12)
		m.Destination = Position{X: r.F32(), Y: r.F32(), Z: r.F32()}
	} else {
		// Linear: destination, then packed offsets for the intermediate points.
		m.Destination = Position{X: r.F32(), Y: r.F32(), Z: r.F32()}
		r.Skip(int(count-1) * 4)
	}
	return m, r.Err()
}
