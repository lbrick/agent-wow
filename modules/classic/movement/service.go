// Package movement is the classic movement module: straight-line ground
// movement with MSG_MOVE_* packets, and the acknowledgements the server
// expects for forced speed changes and teleports.
package movement

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
	movementv1 "github.com/agent-wow/agent-wow/modules/classic/movement/api"
	worldv1 "github.com/agent-wow/agent-wow/modules/classic/world/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// MaxDistance bounds one MoveTo so it finishes inside agent-wow's 30 s call timeout.
const MaxDistance = 150

// heartbeat is the interval between MSG_MOVE_HEARTBEAT packets, as a client sends them.
const heartbeat = 500 * time.Millisecond

// Session is the part of the agent-wow session API the module uses.
type Session interface {
	SendPacket(ctx context.Context, op uint32, body []byte) error
	Clock(ctx context.Context) (uint32, error)
	Invoke(ctx context.Context, module, method string, req, resp proto.Message) error
}

// Service implements movementv1.MovementServer.
type Service struct {
	movementv1.UnimplementedMovementServer
	session Session
	self    uint64
	sleep   func(context.Context, time.Duration) error
	now     func() time.Time

	moving sync.Mutex // one MoveTo at a time
	mu     sync.Mutex
	pos    wire.Position
	known  bool
	speed  float32
	// forced marks a run speed set by SMSG_FORCE_RUN_SPEED_CHANGE; the world
	// module only learns speeds from create blocks, so it would be stale.
	forced bool
	flags  uint32 // movement flags of the last packet sent
}

func New(session Session, self uint64) *Service {
	return &Service{session: session, self: self, sleep: sleepContext, now: time.Now}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// refresh reads the player's position and run speed from the world module.
func (s *Service) refresh(ctx context.Context) error {
	var me worldv1.SelfInfo
	if err := s.session.Invoke(ctx, "world", "self", &emptypb.Empty{}, &me); err != nil {
		return err
	}
	if me.GetPosition() == nil {
		return status.Error(codes.FailedPrecondition, "the player's position is not known yet")
	}
	p := me.GetPosition()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pos, s.known = wire.Position{X: p.X, Y: p.Y, Z: p.Z, O: p.O}, true
	if me.GetRunSpeed() > 0 && !s.forced {
		s.speed = me.GetRunSpeed()
	}
	return nil
}

func (s *Service) send(ctx context.Context, op uint16, flags uint32, p wire.Position) error {
	now, err := s.session.Clock(ctx)
	if err != nil {
		return err
	}
	if now == 0 {
		now = 1 // the anticheat flags a zero client time
	}
	if err := s.session.SendPacket(ctx, uint32(op), wire.AppendMovementInfo(nil, flags, now, p)); err != nil {
		return err
	}
	s.mu.Lock()
	s.pos, s.flags = p, flags
	s.mu.Unlock()
	return s.session.Invoke(ctx, "world", "set_position", &worldv1.SetPositionRequest{Position: &worldv1.Position{X: p.X, Y: p.Y, Z: p.Z, O: p.O}}, &emptypb.Empty{})
}

func facing(from wire.Position, x, y float32) float32 {
	o := math.Atan2(float64(y-from.Y), float64(x-from.X))
	if o < 0 {
		o += 2 * math.Pi
	}
	return float32(o)
}

func (s *Service) MoveTo(ctx context.Context, req *movementv1.MoveToRequest) (*movementv1.MoveToResponse, error) {
	s.moving.Lock()
	defer s.moving.Unlock()
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	start, speed := s.pos, s.speed
	s.mu.Unlock()
	if speed <= 0 {
		speed = 7
	}
	dx, dy := req.GetX()-start.X, req.GetY()-start.Y
	full := float32(math.Hypot(float64(dx), float64(dy)))
	if full > MaxDistance {
		return nil, status.Errorf(codes.InvalidArgument, "point is %.0f yards away; move at most %d yards per call", full, MaxDistance)
	}
	targetZ := start.Z
	if req.Z != nil {
		targetZ = req.GetZ()
	}
	o := facing(start, req.GetX(), req.GetY())
	dist := full - req.GetStopDistance()
	if dist <= 0.05 {
		// Already there: just face the point.
		p := start
		p.O = o
		if err := s.send(ctx, classic.MSGMoveSetFacing, 0, p); err != nil {
			return nil, err
		}
		return &movementv1.MoveToResponse{X: p.X, Y: p.Y, Z: p.Z}, nil
	}
	at := func(d float32) wire.Position {
		f := d / full
		return wire.Position{X: start.X + dx*f, Y: start.Y + dy*f, Z: start.Z + (targetZ-start.Z)*f, O: o}
	}
	begin := s.now()
	total := time.Duration(float64(dist/speed) * float64(time.Second))
	if err := s.send(ctx, classic.MSGMoveStartForward, wire.MoveForward, at(0)); err != nil {
		s.stop(at(0))
		return nil, err
	}
	// Once moving, every exit sends MSG_MOVE_STOP so the server never keeps
	// MOVEFLAG_FORWARD for a client that stopped sending heartbeats.
	elapsed := func() wire.Position {
		return at(min(dist, speed*float32(s.now().Sub(begin).Seconds())))
	}
	for {
		left := total - s.now().Sub(begin)
		if left <= 0 {
			break
		}
		if err := s.sleep(ctx, min(heartbeat, left)); err != nil {
			s.stop(elapsed())
			return nil, err
		}
		if s.now().Sub(begin) >= total {
			break
		}
		if err := s.send(ctx, classic.MSGMoveHeartbeat, wire.MoveForward, elapsed()); err != nil {
			s.stop(elapsed())
			return nil, err
		}
	}
	end := at(dist)
	if err := s.send(ctx, classic.MSGMoveStop, 0, end); err != nil {
		s.stop(end)
		return nil, err
	}
	return &movementv1.MoveToResponse{X: end.X, Y: end.Y, Z: end.Z, ElapsedMs: uint32(s.now().Sub(begin).Milliseconds())}, nil
}

// stop ends a canceled move where the character is, on a fresh context.
func (s *Service) stop(p wire.Position) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.send(ctx, classic.MSGMoveStop, 0, p); err != nil {
		slog.Warn("stop after canceled move failed", "error", err)
	}
}

func (s *Service) Face(ctx context.Context, req *movementv1.FaceRequest) (*emptypb.Empty, error) {
	s.moving.Lock()
	defer s.moving.Unlock()
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	p := s.pos
	s.mu.Unlock()
	p.O = facing(p, req.GetX(), req.GetY())
	if err := s.send(ctx, classic.MSGMoveSetFacing, 0, p); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// speedAcks maps forced speed changes to their acknowledgements.
var speedAcks = map[uint16]uint16{
	classic.SMSGForceRunSpeedChange:      classic.CMSGForceRunSpeedChangeAck,
	classic.SMSGForceRunBackSpeedChange:  classic.CMSGForceRunBackSpeedChangeAck,
	classic.SMSGForceSwimSpeedChange:     classic.CMSGForceSwimSpeedChangeAck,
	classic.SMSGForceWalkSpeedChange:     classic.CMSGForceWalkSpeedChangeAck,
	classic.SMSGForceSwimBackSpeedChange: classic.CMSGForceSwimBackSpeedChangeAck,
	classic.SMSGForceTurnRateChange:      classic.CMSGForceTurnRateChangeAck,
}

func (s *Service) OnPacket(ctx context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if err := s.handle(ctx, uint16(p.GetOpcode()), p.GetPayload()); err != nil {
		slog.Warn("packet not acknowledged", "opcode", classic.WorldName(p.GetOpcode()), "error", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) handle(ctx context.Context, op uint16, body []byte) error {
	now, err := s.session.Clock(ctx)
	if err != nil {
		return err
	}
	if ack, ok := speedAcks[op]; ok {
		// Server: packed GUID, counter, speed. Client (HandleForceSpeedChangeAckOpcodes):
		// GUID, counter, MovementInfo, speed.
		r := wire.NewReader(body)
		guid := r.PackedGUID()
		counter := r.U32()
		speed := r.F32()
		if r.Err() != nil || guid != s.self {
			return r.Err()
		}
		s.mu.Lock()
		p, flags := s.pos, s.flags
		if op == classic.SMSGForceRunSpeedChange {
			s.speed, s.forced = speed, true
		}
		s.mu.Unlock()
		b := binary.LittleEndian.AppendUint64(nil, guid)
		b = wire.AppendU32(b, counter)
		// The ack carries the current movement state, so a mid-move ack does not
		// read as a stop.
		b = wire.AppendMovementInfo(b, flags, now, p)
		return s.session.SendPacket(ctx, uint32(ack), wire.AppendF32(b, speed))
	}
	switch op {
	case classic.MSGMoveTeleportAck:
		// Server: packed GUID, counter, MovementInfo. Client
		// (HandleMoveTeleportAckOpcode): GUID, counter, time.
		r := wire.NewReader(body)
		guid := r.PackedGUID()
		counter := r.U32()
		info := wire.ReadMovementInfo(r)
		if r.Err() != nil || guid != s.self {
			return r.Err()
		}
		s.mu.Lock()
		s.pos, s.known = info.Pos, true
		s.mu.Unlock()
		b := binary.LittleEndian.AppendUint64(nil, guid)
		b = wire.AppendU32(b, counter)
		return s.session.SendPacket(ctx, classic.MSGMoveTeleportAck, wire.AppendU32(b, now))
	case classic.SMSGNewWorld:
		return s.session.SendPacket(ctx, classic.MSGMoveWorldportAck, nil)
	}
	return errors.New("unhandled packet")
}
