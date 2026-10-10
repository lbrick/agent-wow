// Package world is the classic world-state module: it decodes the server's
// object updates for the player and nearby units.
package world

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"

	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
	worldv1 "github.com/agent-wow/agent-wow/modules/classic/world/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Service implements worldv1.WorldServer.
type Service struct {
	worldv1.UnimplementedWorldServer
	state *State
}

func New(self uint64) *Service { return &Service{state: NewState(self)} }

func pos(p wire.Position) *worldv1.Position {
	return &worldv1.Position{X: p.X, Y: p.Y, Z: p.Z, O: p.O}
}

func (s *Service) Self(context.Context, *emptypb.Empty) (*worldv1.SelfInfo, error) {
	me := s.state.Self()
	if !me.Known {
		return &worldv1.SelfInfo{}, nil
	}
	return &worldv1.SelfInfo{
		Guid: me.GUID, Map: me.Map, Position: pos(me.Pos), Level: me.Level, Health: me.Health, MaxHealth: me.MaxHealth,
		Xp: me.XP, NextLevelXp: me.NextXP, FactionTemplate: me.Faction, Target: me.Target, RunSpeed: me.RunSpeed,
		InCombat: me.InCombat, Dead: me.Dead,
	}, nil
}

func unitInfo(u Unit) *worldv1.UnitInfo {
	info := &worldv1.UnitInfo{
		Guid: u.GUID, Entry: u.Entry, Player: u.Player, Position: pos(u.Pos), Distance: u.Distance,
		Level: u.Level, Health: u.Health, MaxHealth: u.MaxHealth, FactionTemplate: u.Faction,
		Reaction: string(u.Reaction), Attackable: u.Attackable, Dead: u.Dead, Lootable: u.Lootable,
		InCombat: u.InCombat, Target: u.Target,
	}
	if u.Dest != nil {
		info.Destination = pos(*u.Dest)
	}
	return info
}

func (s *Service) Units(_ context.Context, req *worldv1.UnitsRequest) (*worldv1.UnitsResponse, error) {
	var out worldv1.UnitsResponse
	for _, u := range s.state.Units(req.GetRadius(), req.GetReaction(), req.GetAliveOnly(), req.GetEntry()) {
		out.Units = append(out.Units, unitInfo(u))
	}
	return &out, nil
}

func (s *Service) Unit(_ context.Context, req *worldv1.UnitRequest) (*worldv1.UnitInfo, error) {
	u, ok := s.state.Unit(req.GetGuid())
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unit %d is not in range", req.GetGuid())
	}
	return unitInfo(u), nil
}

func (s *Service) SetPosition(_ context.Context, req *worldv1.SetPositionRequest) (*emptypb.Empty, error) {
	p := req.GetPosition()
	if p == nil {
		return nil, status.Error(codes.InvalidArgument, "position is required")
	}
	s.state.SetSelfPosition(nil, wire.Position{X: p.X, Y: p.Y, Z: p.Z, O: p.O})
	return &emptypb.Empty{}, nil
}

// OnPacket decodes the server packets the manifest subscribes to. Decoding
// errors are logged and the packet skipped, so a format gap never ends the
// session.
func (s *Service) OnPacket(_ context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if err := s.handle(uint16(p.GetOpcode()), p.GetPayload()); err != nil {
		slog.Warn("packet skipped", "opcode", classic.WorldName(p.GetOpcode()), "error", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) handle(op uint16, body []byte) error {
	switch op {
	case classic.SMSGCompressedUpdateObject:
		inflated, err := wire.Inflate(body)
		if err != nil {
			return err
		}
		body = inflated
		fallthrough
	case classic.SMSGUpdateObject:
		blocks, err := wire.ParseUpdate(body)
		s.state.Apply(blocks) // apply what decoded even if a later block failed
		return err
	case classic.SMSGMonsterMove:
		m, err := wire.ParseMonsterMove(body)
		if err != nil {
			return err
		}
		s.state.MonsterMove(m)
	case classic.SMSGDestroyObject:
		if len(body) != 8 {
			return fmt.Errorf("destroy object of %d bytes", len(body))
		}
		s.state.Destroy(binary.LittleEndian.Uint64(body))
	case classic.SMSGLoginVerifyWorld, classic.SMSGNewWorld:
		r := wire.NewReader(body)
		m := r.U32()
		p := wire.Position{X: r.F32(), Y: r.F32(), Z: r.F32(), O: r.F32()}
		if r.Err() != nil {
			return r.Err()
		}
		s.state.SetSelfPosition(&m, p)
	case classic.MSGMoveTeleportAck:
		// Server form: packed mover GUID, counter, MovementInfo.
		r := wire.NewReader(body)
		guid := r.PackedGUID()
		r.U32()
		info := wire.ReadMovementInfo(r)
		if r.Err() != nil {
			return r.Err()
		}
		if guid == s.state.self {
			s.state.SetSelfPosition(nil, info.Pos)
		}
	default:
		// Other players' movement: packed GUID, MovementInfo.
		r := wire.NewReader(body)
		guid := r.PackedGUID()
		info := wire.ReadMovementInfo(r)
		if r.Err() != nil {
			return r.Err()
		}
		s.state.Moved(guid, info.Pos)
	}
	return nil
}
