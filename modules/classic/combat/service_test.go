package combat

import (
	"context"
	"encoding/binary"
	"testing"

	combatv1 "github.com/agent-wow/agent-wow/modules/classic/combat/api"
	"github.com/agent-wow/agent-wow/modules/classic/internal/fakesession"
	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
	worldv1 "github.com/agent-wow/agent-wow/modules/classic/world/api"
	modv1 "github.com/agent-wow/agent-wow/pkg/modules/v1"
	"github.com/agent-wow/agent-wow/pkg/opcode/classic"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const self, wolf = 0x73d, 0xF130000C9E000102

func TestAttackAndCast(t *testing.T) {
	f := &fakesession.Session{Answer: func(module, method string, _, resp proto.Message) error {
		if method == "unit" {
			proto.Merge(resp, &worldv1.UnitInfo{Guid: wolf, Attackable: true, Position: &worldv1.Position{X: 5, Y: 6}})
		}
		return nil
	}}
	s := New(f, self)
	if _, err := s.Attack(context.Background(), &combatv1.AttackRequest{Guid: wolf}); err != nil {
		t.Fatal(err)
	}
	if len(f.Calls) != 2 || f.Calls[1].Module != "movement" || f.Calls[1].Method != "face" {
		t.Fatalf("calls %+v", f.Calls)
	}
	sent := f.Sent()
	if len(sent) != 2 || sent[0].Op != classic.CMSGSetSelection || sent[1].Op != classic.CMSGAttackswing ||
		binary.LittleEndian.Uint64(sent[1].Body) != wolf {
		t.Fatalf("attack packets %+v", sent)
	}
	s.Cast(context.Background(), &combatv1.CastRequest{SpellId: 78, Target: wolf})
	s.Cast(context.Background(), &combatv1.CastRequest{SpellId: 2457})
	sent = f.Sent()
	unit := wire.AppendPackedGUID(binary.LittleEndian.AppendUint16(wire.AppendU32(nil, 78), 2), wolf)
	if string(sent[2].Body) != string(unit) || string(sent[3].Body) != string(binary.LittleEndian.AppendUint16(wire.AppendU32(nil, 2457), 0)) {
		t.Fatalf("cast bodies %x, %x", sent[2].Body, sent[3].Body)
	}

	notAttackable := &fakesession.Session{Answer: func(_, _ string, _, resp proto.Message) error {
		proto.Merge(resp, &worldv1.UnitInfo{Guid: wolf, Reaction: "friendly"})
		return nil
	}}
	if _, err := New(notAttackable, self).Attack(context.Background(), &combatv1.AttackRequest{Guid: wolf}); err == nil {
		t.Fatal("attacked a friendly unit")
	}
}

func TestCombatLog(t *testing.T) {
	f := &fakesession.Session{}
	s := New(f, self)
	packet := func(op uint16, body []byte) {
		t.Helper()
		if _, err := s.OnPacket(context.Background(), &modv1.WorldPacket{Opcode: uint32(op), Payload: body}); err != nil {
			t.Fatal(err)
		}
	}
	start := binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint64(nil, self), wolf)
	packet(classic.SMSGAttackstart, start)
	// A fight between two other units is not logged.
	packet(classic.SMSGAttackstart, binary.LittleEndian.AppendUint64(binary.LittleEndian.AppendUint64(nil, 0x99), wolf))
	hit := wire.AppendPackedGUID(wire.AppendPackedGUID(wire.AppendU32(nil, 2), self), wolf)
	packet(classic.SMSGAttackerstateupdate, append(wire.AppendU32(hit, 7), 1))
	packet(classic.SMSGAttackswingNotinrange, nil)
	xp := wire.AppendU32(binary.LittleEndian.AppendUint64(nil, wolf), 50)
	packet(classic.SMSGLogXpgain, append(append(xp, 0), wire.AppendF32(wire.AppendU32(nil, 50), 1)...))
	packet(classic.SMSGAttackstop, wire.AppendU32(wire.AppendPackedGUID(wire.AppendPackedGUID(nil, self), wolf), 1))
	packet(classic.SMSGCastResult, []byte{78, 0, 0, 0, 2, 11})
	st, _ := s.Status(context.Background(), &emptypb.Empty{})
	if st.Attacking != 0 || st.XpGained != 50 || len(st.Kills) != 1 || st.Kills[0] != wolf {
		t.Fatalf("status %+v", st)
	}
	var kinds []string
	for _, e := range st.Events {
		kinds = append(kinds, e.Kind)
	}
	want := []string{"attack_start", "hit", "not_in_range", "xp", "attack_stop", "cast_failed"}
	if len(kinds) != len(want) {
		t.Fatalf("events %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("events %v, want %v", kinds, want)
		}
	}
	if st.Events[1].Amount != 7 {
		t.Fatalf("hit amount %d", st.Events[1].Amount)
	}
}
