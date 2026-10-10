package movement

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/agent-wow/agent-wow/modules/classic/internal/fakesession"
	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
	movementv1 "github.com/agent-wow/agent-wow/modules/classic/movement/api"
	worldv1 "github.com/agent-wow/agent-wow/modules/classic/world/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"google.golang.org/protobuf/proto"
)

func newService(start wire.Position) (*Service, *fakesession.Session) {
	f := &fakesession.Session{Answer: func(module, method string, _, resp proto.Message) error {
		if module == "world" && method == "self" {
			proto.Merge(resp, &worldv1.SelfInfo{Guid: 7, RunSpeed: 7, Position: &worldv1.Position{X: start.X, Y: start.Y, Z: start.Z}})
		}
		return nil
	}}
	s := New(f, 7)
	clock := time.Unix(0, 0)
	s.now = func() time.Time { return clock }
	s.sleep = func(_ context.Context, d time.Duration) error { clock = clock.Add(d); return nil }
	return s, f
}

func decode(t *testing.T, p fakesession.Packet) wire.MovementInfo {
	t.Helper()
	r := wire.NewReader(p.Body)
	m := wire.ReadMovementInfo(r)
	if r.Err() != nil || r.Len() != 0 {
		t.Fatalf("%s: %v, %d trailing", classic.WorldName(p.Op), r.Err(), r.Len())
	}
	return m
}

func TestMoveTo(t *testing.T) {
	s, f := newService(wire.Position{X: 0, Y: 0, Z: 10})
	resp, err := s.MoveTo(context.Background(), &movementv1.MoveToRequest{X: 30, Y: 40, StopDistance: 5})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(resp.X-27)) > 0.01 || math.Abs(float64(resp.Y-36)) > 0.01 || resp.Z != 10 {
		t.Fatalf("stopped at %+v, want (27, 36, 10)", resp)
	}
	sent := f.Sent()
	if len(sent) < 2 || sent[0].Op != classic.MSGMoveStartForward || sent[len(sent)-1].Op != classic.MSGMoveStop {
		t.Fatalf("packet sequence %v", sent)
	}
	var last uint32
	for i, p := range sent {
		m := decode(t, p)
		if m.Time == 0 || m.Time <= last {
			t.Fatalf("packet %d time %d after %d", i, m.Time, last)
		}
		last = m.Time
		forward := m.Flags&wire.MoveForward != 0
		if forward == (p.Op == classic.MSGMoveStop) || math.Abs(float64(m.Pos.O)-math.Atan2(40, 30)) > 1e-5 {
			t.Fatalf("packet %d %s flags %#x facing %f", i, classic.WorldName(p.Op), m.Flags, m.Pos.O)
		}
	}
	// 45 yards at 7 yards/s is 6.4 s: a start, 12 heartbeats and a stop.
	if len(sent) != 14 {
		t.Fatalf("sent %d packets, want 14", len(sent))
	}
	if updates := len(f.Calls); updates < len(sent) {
		t.Fatalf("world.set_position called %d times for %d packets", updates, len(sent))
	}
	if _, err := s.MoveTo(context.Background(), &movementv1.MoveToRequest{X: 200}); err == nil {
		t.Fatal("accepted a 200-yard move")
	}
}

func TestAcks(t *testing.T) {
	s, f := newService(wire.Position{})
	s.pos = wire.Position{X: 1, Y: 2, Z: 3}
	speed := wire.AppendPackedGUID(nil, 7)
	speed = wire.AppendF32(wire.AppendU32(speed, 4), 9.5)
	if _, err := s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: classic.SMSGForceRunSpeedChange, Payload: speed}); err != nil {
		t.Fatal(err)
	}
	teleport := wire.AppendPackedGUID(nil, 7)
	teleport = wire.AppendMovementInfo(wire.AppendU32(teleport, 5), 0, 1, wire.Position{X: 50, Y: 60, Z: 70})
	s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: classic.MSGMoveTeleportAck, Payload: teleport})
	s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: classic.SMSGNewWorld, Payload: make([]byte, 20)})
	sent := f.Sent()
	if len(sent) != 3 {
		t.Fatalf("sent %d packets", len(sent))
	}
	// CMSG_FORCE_RUN_SPEED_CHANGE_ACK: GUID, counter, MovementInfo, speed.
	ack := sent[0]
	if ack.Op != classic.CMSGForceRunSpeedChangeAck || len(ack.Body) != 8+4+28+4 ||
		binary.LittleEndian.Uint64(ack.Body) != 7 || binary.LittleEndian.Uint32(ack.Body[8:]) != 4 ||
		math.Float32frombits(binary.LittleEndian.Uint32(ack.Body[40:])) != 9.5 || s.speed != 9.5 {
		t.Fatalf("speed ack %x", ack.Body)
	}
	// MSG_MOVE_TELEPORT_ACK: GUID, counter, time.
	if tp := sent[1]; tp.Op != classic.MSGMoveTeleportAck || len(tp.Body) != 16 || binary.LittleEndian.Uint32(tp.Body[8:]) != 5 || s.pos.X != 50 {
		t.Fatalf("teleport ack %x, position %+v", tp.Body, s.pos)
	}
	if sent[2].Op != classic.MSGMoveWorldportAck || len(sent[2].Body) != 0 {
		t.Fatalf("worldport ack %+v", sent[2])
	}
}

func TestForcedSpeedSurvivesRefresh(t *testing.T) {
	s, f := newService(wire.Position{})
	slow := wire.AppendF32(wire.AppendU32(wire.AppendPackedGUID(nil, 7), 1), 3.5) // a snare
	s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: classic.SMSGForceRunSpeedChange, Payload: slow})
	s.flags = wire.MoveForward
	fast := wire.AppendF32(wire.AppendU32(wire.AppendPackedGUID(nil, 7), 2), 3.5)
	s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: classic.SMSGForceRunSpeedChange, Payload: fast})
	if ack := f.Sent()[1]; binary.LittleEndian.Uint32(ack.Body[12:]) != wire.MoveForward {
		t.Fatalf("mid-move ack flags %#x", binary.LittleEndian.Uint32(ack.Body[12:]))
	}
	// The world module still reports 7; the move must use the acknowledged 3.5.
	resp, err := s.MoveTo(context.Background(), &movementv1.MoveToRequest{X: 35})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ElapsedMs != 10000 {
		t.Fatalf("35 yards took %d ms, want 10000 at 3.5 yards/s", resp.ElapsedMs)
	}
}
