// Package wire decodes and encodes the WoW 1.12.1 (build 5875) gameplay
// packets the classic modules use, as cMaNGOS Classic writes them at revision
// 8ec338a1 (src/game/Entities/Object.cpp, Unit.cpp, UpdateData.cpp and
// Movement/packet_builder.cpp).
package wire

import (
	"encoding/binary"
	"errors"
	"math"
)

// ErrShort reports a packet that ends before a field it declares.
var ErrShort = errors.New("packet is truncated")

// Reader consumes little-endian fields and remembers the first error.
type Reader struct {
	b   []byte
	err error
}

func NewReader(b []byte) *Reader { return &Reader{b: b} }

// Err returns the first decoding error.
func (r *Reader) Err() error { return r.err }

// Len returns the unread byte count.
func (r *Reader) Len() int { return len(r.b) }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || len(r.b) < n {
		r.err = ErrShort
		r.b = nil
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *Reader) U8() uint8 {
	if v := r.take(1); v != nil {
		return v[0]
	}
	return 0
}

func (r *Reader) U16() uint16 {
	if v := r.take(2); v != nil {
		return binary.LittleEndian.Uint16(v)
	}
	return 0
}

func (r *Reader) U32() uint32 {
	if v := r.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}
	return 0
}

func (r *Reader) U64() uint64 {
	if v := r.take(8); v != nil {
		return binary.LittleEndian.Uint64(v)
	}
	return 0
}

func (r *Reader) F32() float32 { return math.Float32frombits(r.U32()) }

// Skip discards n bytes.
func (r *Reader) Skip(n int) { r.take(n) }

// PackedGUID reads a mask byte followed by the GUID's non-zero bytes.
func (r *Reader) PackedGUID() uint64 {
	mask := r.U8()
	var guid uint64
	for i := 0; i < 8; i++ {
		if mask&(1<<i) != 0 {
			guid |= uint64(r.U8()) << (8 * i)
		}
	}
	return guid
}

// AppendPackedGUID appends guid in packed form.
func AppendPackedGUID(b []byte, guid uint64) []byte {
	maskAt := len(b)
	b = append(b, 0)
	for i := 0; i < 8; i++ {
		if v := byte(guid >> (8 * i)); v != 0 {
			b[maskAt] |= 1 << i
			b = append(b, v)
		}
	}
	return b
}

func AppendU32(b []byte, v uint32) []byte { return binary.LittleEndian.AppendUint32(b, v) }

func AppendF32(b []byte, v float32) []byte {
	return binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
}

// HighGUID returns the high 16 bits that classify an object GUID.
func HighGUID(guid uint64) uint16 { return uint16(guid >> 48) }

// Type IDs (TypeID in mangos-classic ObjectGuid.h).
const (
	TypeObject        = 0
	TypeItem          = 1
	TypeContainer     = 2
	TypeUnit          = 3
	TypePlayer        = 4
	TypeGameObject    = 5
	TypeDynamicObject = 6
	TypeCorpse        = 7
)

// Update field indices (mangos-classic UpdateFields.h).
const (
	FieldEntry           = 0x03
	FieldUnitTarget      = 0x10 // 2 fields
	FieldHealth          = 0x16
	FieldMaxHealth       = 0x1c
	FieldLevel           = 0x22
	FieldFactionTemplate = 0x23
	FieldUnitFlags       = 0x2e
	FieldDynamicFlags    = 0x8f
	FieldPlayerXP        = 0x2cc
	FieldPlayerNextXP    = 0x2cd
)

// Unit flags and dynamic flags used by the modules (mangos-classic UnitDefines.h).
const (
	UnitFlagNotAttackable = 0x00000002
	UnitFlagInCombat      = 0x00080000
	DynFlagLootable       = 0x0001
	DynFlagDead           = 0x0020
)
