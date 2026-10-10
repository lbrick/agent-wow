package wire

// Movement flags (mangos-classic Object.h MovementFlags).
const (
	MoveForward        = 0x00000001
	MoveJumping        = 0x00002000
	MoveSwimming       = 0x00200000
	MoveSplineEnabled  = 0x00400000
	MoveOnTransport    = 0x02000000
	MoveSplineElevated = 0x04000000
)

// Position is a world coordinate with facing in radians.
type Position struct {
	X, Y, Z, O float32
}

// MovementInfo is mangos-classic MovementInfo as MovementInfo::Write emits
// and the client sends it.
type MovementInfo struct {
	Flags    uint32
	Time     uint32
	Pos      Position
	Fall     uint32
	pitch    float32
	jump     [4]float32
	splineEl float32
}

// ReadMovementInfo reads one MovementInfo.
func ReadMovementInfo(r *Reader) MovementInfo {
	var m MovementInfo
	m.Flags = r.U32()
	m.Time = r.U32()
	m.Pos = Position{r.F32(), r.F32(), r.F32(), r.F32()}
	if m.Flags&MoveOnTransport != 0 {
		r.Skip(8 + 16) // transport guid and its local position
	}
	if m.Flags&MoveSwimming != 0 {
		m.pitch = r.F32()
	}
	m.Fall = r.U32()
	if m.Flags&MoveJumping != 0 {
		for i := range m.jump {
			m.jump[i] = r.F32()
		}
	}
	if m.Flags&MoveSplineElevated != 0 {
		m.splineEl = r.F32()
	}
	return m
}

// AppendMovementInfo appends a client MovementInfo for ground movement: no
// transport, swimming, jumping or spline elevation.
func AppendMovementInfo(b []byte, flags, time uint32, p Position) []byte {
	b = AppendU32(b, flags)
	b = AppendU32(b, time)
	for _, f := range []float32{p.X, p.Y, p.Z, p.O} {
		b = AppendF32(b, f)
	}
	return AppendU32(b, 0) // fall time
}
