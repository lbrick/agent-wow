// Package combat is the classic combat module: target selection, melee,
// spell casts, and a log of the server's combat results.
package combat

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"

	combatv1 "github.com/agent-wow/agent-wow/modules/classic/combat/api"
	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
	movementv1 "github.com/agent-wow/agent-wow/modules/classic/movement/api"
	worldv1 "github.com/agent-wow/agent-wow/modules/classic/world/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Session is the part of the agent-wow session API the module uses.
type Session interface {
	SendPacket(ctx context.Context, op uint32, body []byte) error
	Clock(ctx context.Context) (uint32, error)
	Invoke(ctx context.Context, module, method string, req, resp proto.Message) error
}

const maxEvents = 50

// Service implements combatv1.CombatServer. The server broadcasts every
// nearby fight; the log keeps only events involving the player.
type Service struct {
	combatv1.UnimplementedCombatServer
	session Session
	self    uint64

	mu        sync.Mutex
	attacking uint64
	kills     []uint64
	xp        uint32
	events    []*combatv1.Event
}

func New(session Session, self uint64) *Service { return &Service{session: session, self: self} }

func (s *Service) Attack(ctx context.Context, req *combatv1.AttackRequest) (*emptypb.Empty, error) {
	var u worldv1.UnitInfo
	if err := s.session.Invoke(ctx, "world", "unit", &worldv1.UnitRequest{Guid: req.GetGuid()}, &u); err != nil {
		return nil, err
	}
	if !u.GetAttackable() {
		return nil, fmt.Errorf("unit %d is not attackable (reaction %s, dead %t)", u.GetGuid(), u.GetReaction(), u.GetDead())
	}
	// Melee needs the player to face the target (SMSG_ATTACKSWING_BADFACING).
	if p := u.GetPosition(); p != nil {
		if err := s.session.Invoke(ctx, "movement", "face", &movementv1.FaceRequest{X: p.X, Y: p.Y}, &emptypb.Empty{}); err != nil {
			return nil, err
		}
	}
	guid := binary.LittleEndian.AppendUint64(nil, req.GetGuid())
	if err := s.session.SendPacket(ctx, classic.CMSGSetSelection, guid); err != nil {
		return nil, err
	}
	if err := s.session.SendPacket(ctx, classic.CMSGAttackswing, guid); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *Service) StopAttack(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.session.SendPacket(ctx, classic.CMSGAttackstop, nil)
}

// Cast sends CMSG_CAST_SPELL (HandleCastSpellOpcode): spell ID, then
// SpellCastTargets with a uint16 mask and, for a unit, its packed GUID.
func (s *Service) Cast(ctx context.Context, req *combatv1.CastRequest) (*emptypb.Empty, error) {
	b := wire.AppendU32(nil, req.GetSpellId())
	if req.GetTarget() == 0 {
		b = binary.LittleEndian.AppendUint16(b, 0x0000) // TARGET_FLAG_SELF
	} else {
		b = binary.LittleEndian.AppendUint16(b, 0x0002) // TARGET_FLAG_UNIT
		b = wire.AppendPackedGUID(b, req.GetTarget())
	}
	return &emptypb.Empty{}, s.session.SendPacket(ctx, classic.CMSGCastSpell, b)
}

func (s *Service) Status(context.Context, *emptypb.Empty) (*combatv1.StatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &combatv1.StatusResponse{
		Attacking: s.attacking, Kills: append([]uint64(nil), s.kills...), XpGained: s.xp,
		Events: append([]*combatv1.Event(nil), s.events...),
	}, nil
}

func (s *Service) record(ctx context.Context, e *combatv1.Event) {
	if now, err := s.session.Clock(ctx); err == nil {
		e.TimeMs = now
	}
	s.events = append(s.events, e)
	if len(s.events) > maxEvents {
		s.events = s.events[len(s.events)-maxEvents:]
	}
}

func (s *Service) OnPacket(ctx context.Context, p *modv1.WorldPacket) (*emptypb.Empty, error) {
	if err := s.handle(ctx, uint16(p.GetOpcode()), p.GetPayload()); err != nil {
		slog.Warn("packet skipped", "opcode", classic.WorldName(p.GetOpcode()), "error", err)
	}
	return &emptypb.Empty{}, nil
}

// Melee swing errors carry no body.
var swingErrors = map[uint16]string{
	classic.SMSGAttackswingNotinrange:  "not_in_range",
	classic.SMSGAttackswingBadfacing:   "bad_facing",
	classic.SMSGAttackswingNotstanding: "not_standing",
	classic.SMSGAttackswingDeadtarget:  "dead_target",
	classic.SMSGAttackswingCantAttack:  "cannot_attack",
}

func (s *Service) handle(ctx context.Context, op uint16, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := wire.NewReader(body)
	if kind, ok := swingErrors[op]; ok {
		s.record(ctx, &combatv1.Event{Kind: kind, Source: s.self, Target: s.attacking})
		return nil
	}
	switch op {
	case classic.SMSGAttackstart: // attacker, victim (full GUIDs)
		attacker, victim := r.U64(), r.U64()
		if r.Err() != nil {
			return r.Err()
		}
		if attacker == s.self {
			s.attacking = victim
		}
		if attacker == s.self || victim == s.self {
			s.record(ctx, &combatv1.Event{Kind: "attack_start", Source: attacker, Target: victim})
		}
	case classic.SMSGAttackstop: // packed attacker, packed victim, uint32 dead
		attacker, victim := r.PackedGUID(), r.PackedGUID()
		if r.Err() != nil {
			return r.Err()
		}
		if attacker == s.self {
			s.attacking = 0
		}
		if attacker == s.self || victim == s.self {
			s.record(ctx, &combatv1.Event{Kind: "attack_stop", Source: attacker, Target: victim})
		}
	case classic.SMSGAttackerstateupdate: // hit info, packed attacker, packed target, total damage, ...
		r.U32()
		attacker, target, damage := r.PackedGUID(), r.PackedGUID(), r.U32()
		if r.Err() != nil {
			return r.Err()
		}
		if attacker == s.self || target == s.self {
			s.record(ctx, &combatv1.Event{Kind: "hit", Source: attacker, Target: target, Amount: damage})
		}
	case classic.SMSGLogXpgain: // victim GUID, xp, type (0 kill)
		victim, xp, kind := r.U64(), r.U32(), r.U8()
		if r.Err() != nil {
			return r.Err()
		}
		s.xp += xp
		if kind == 0 && victim != 0 {
			s.kills = append(s.kills, victim)
		}
		s.record(ctx, &combatv1.Event{Kind: "xp", Target: victim, Amount: xp})
	case classic.SMSGCastResult: // spell, status (2 = failed), reason
		spell, result := r.U32(), r.U8()
		if r.Err() != nil {
			return r.Err()
		}
		if result == 2 {
			reason := r.U8()
			s.record(ctx, &combatv1.Event{Kind: "cast_failed", Source: s.self, Amount: spell, Detail: fmt.Sprintf("reason %d", reason)})
		}
	}
	return nil
}
