package world

import (
	"testing"
	"time"

	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
)

func TestStateUnitsAndSelf(t *testing.T) {
	const self, wolf, marshal = 0x73d, 0xF130000C9E000102, 0xF130000337000103
	now := time.Unix(1000, 0)
	s := NewState(self)
	s.now = func() time.Time { return now }
	s.SetSelfPosition(new(uint32), wire.Position{X: 0, Y: 0, Z: 0})
	s.Apply([]wire.Block{
		{Type: wire.UpdateCreateObject2, GUID: self, TypeID: wire.TypePlayer, Position: &wire.Position{}, RunSpeed: 7,
			Values: map[uint16]uint32{wire.FieldHealth: 60, wire.FieldMaxHealth: 60, wire.FieldLevel: 1, wire.FieldFactionTemplate: 1, wire.FieldPlayerXP: 10}},
		{Type: wire.UpdateCreateObject2, GUID: wolf, TypeID: wire.TypeUnit, Position: &wire.Position{X: 30, Y: 40},
			Values: map[uint16]uint32{wire.FieldEntry: 299, wire.FieldHealth: 100, wire.FieldMaxHealth: 100, wire.FieldLevel: 1, wire.FieldFactionTemplate: 32}},
		{Type: wire.UpdateCreateObject2, GUID: marshal, TypeID: wire.TypeUnit, Position: &wire.Position{X: 3, Y: 4},
			Values: map[uint16]uint32{wire.FieldEntry: 823, wire.FieldHealth: 100, wire.FieldMaxHealth: 100, wire.FieldFactionTemplate: 12}},
		{Type: wire.UpdateCreateObject2, GUID: 0x4000000000000001, TypeID: wire.TypeItem}, // ignored
	})
	me := s.Self()
	if !me.Known || me.Health != 60 || me.XP != 10 || me.RunSpeed != 7 || me.Dead {
		t.Fatalf("self = %+v", me)
	}
	units := s.Units(0, "", false, 0)
	if len(units) != 2 || units[0].GUID != marshal || units[0].Distance != 5 || units[1].Distance != 50 {
		t.Fatalf("units = %+v", units)
	}
	if units[0].Reaction != "friendly" || units[0].Attackable || units[1].Reaction != "neutral" || !units[1].Attackable {
		t.Fatalf("reactions = %s/%t, %s/%t", units[0].Reaction, units[0].Attackable, units[1].Reaction, units[1].Attackable)
	}
	if got := s.Units(10, "", false, 0); len(got) != 1 {
		t.Fatalf("radius filter kept %d", len(got))
	}
	if got := s.Units(100, "neutral", false, 299); len(got) != 1 || got[0].GUID != wolf {
		t.Fatalf("reaction/entry filter = %+v", got)
	}

	// A spline moves the wolf halfway after half its duration.
	s.MonsterMove(wire.MonsterMove{GUID: wolf, Start: wire.Position{X: 30, Y: 40}, Destination: wire.Position{X: 30, Y: 60}, DurationMs: 2000})
	now = now.Add(time.Second)
	if u, _ := s.Unit(wolf); u.Pos.Y != 50 || u.Dest == nil {
		t.Fatalf("interpolated wolf = %+v", u)
	}
	now = now.Add(5 * time.Second)
	if u, _ := s.Unit(wolf); u.Pos.Y != 60 || u.Dest != nil {
		t.Fatalf("arrived wolf = %+v", u)
	}

	// Death arrives as a values block.
	s.Apply([]wire.Block{{Type: wire.UpdateValues, GUID: wolf, Values: map[uint16]uint32{wire.FieldHealth: 0, wire.FieldDynamicFlags: wire.DynFlagLootable}}})
	if u, _ := s.Unit(wolf); !u.Dead || !u.Lootable || u.Attackable {
		t.Fatalf("dead wolf = %+v", u)
	}
	if got := s.Units(100, "", true, 0); len(got) != 1 {
		t.Fatalf("alive filter kept %d", len(got))
	}

	s.Destroy(marshal)
	s.Apply([]wire.Block{{Type: wire.UpdateOutOfRange, GUIDs: []uint64{wolf, self}}})
	if got := s.Units(0, "", false, 0); len(got) != 0 || !s.Self().Known {
		t.Fatalf("after removal: units %d, self known %t", len(got), s.Self().Known)
	}

	// A new map forgets everything but the player.
	s.Apply([]wire.Block{{Type: wire.UpdateCreateObject, GUID: wolf, TypeID: wire.TypeUnit, Position: &wire.Position{}}})
	mapID := uint32(1)
	s.SetSelfPosition(&mapID, wire.Position{X: 9})
	if got := s.Units(0, "", false, 0); len(got) != 0 || s.Self().Map != 1 || s.Self().Pos.X != 9 {
		t.Fatalf("after map change: %d units, self %+v", len(got), s.Self())
	}
}
