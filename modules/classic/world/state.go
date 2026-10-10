package world

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/agent-wow/agent-wow/modules/classic/internal/factions"
	"github.com/agent-wow/agent-wow/modules/classic/internal/wire"
)

// object is one unit or player the server has created for this client.
type object struct {
	typeID uint8
	values map[uint16]uint32
	pos    wire.Position
	// A spline in progress: the unit moves from pos to dest between
	// moveStart and moveStart+moveDur.
	dest      *wire.Position
	moveStart time.Time
	moveDur   time.Duration
	runSpeed  float32
}

// position interpolates a moving unit's location at now.
func (o *object) position(now time.Time) wire.Position {
	if o.dest == nil || o.moveDur <= 0 {
		return o.pos
	}
	t := float32(now.Sub(o.moveStart)) / float32(o.moveDur)
	if t >= 1 {
		return wire.Position{X: o.dest.X, Y: o.dest.Y, Z: o.dest.Z, O: o.pos.O}
	}
	if t < 0 {
		t = 0
	}
	return wire.Position{
		X: o.pos.X + (o.dest.X-o.pos.X)*t,
		Y: o.pos.Y + (o.dest.Y-o.pos.Y)*t,
		Z: o.pos.Z + (o.dest.Z-o.pos.Z)*t,
		O: o.pos.O,
	}
}

// State is the world as the client knows it. It is safe for concurrent use.
type State struct {
	mu      sync.Mutex
	self    uint64
	mapID   uint32
	objects map[uint64]*object
	now     func() time.Time
}

func NewState(self uint64) *State {
	return &State{self: self, objects: map[uint64]*object{}, now: time.Now}
}

func (s *State) obj(guid uint64) *object {
	o := s.objects[guid]
	if o == nil {
		o = &object{values: map[uint16]uint32{}}
		s.objects[guid] = o
	}
	return o
}

// Apply merges decoded update blocks.
func (s *State) Apply(blocks []wire.Block) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range blocks {
		switch b.Type {
		case wire.UpdateOutOfRange:
			for _, g := range b.GUIDs {
				if g != s.self {
					delete(s.objects, g)
				}
			}
		case wire.UpdateCreateObject, wire.UpdateCreateObject2, wire.UpdateValues, wire.UpdateMovement:
			if b.Type != wire.UpdateValues && b.TypeID != wire.TypeUnit && b.TypeID != wire.TypePlayer && b.Type != wire.UpdateMovement {
				continue // items, game objects and corpses are not tracked
			}
			if b.Type == wire.UpdateValues && s.objects[b.GUID] == nil {
				continue
			}
			o := s.obj(b.GUID)
			if b.TypeID != 0 {
				o.typeID = b.TypeID
			}
			if b.Type == wire.UpdateCreateObject || b.Type == wire.UpdateCreateObject2 {
				o.values = map[uint16]uint32{}
			}
			for k, v := range b.Values {
				o.values[k] = v
			}
			if b.Position != nil {
				o.pos, o.dest = *b.Position, nil
				if b.Destination != nil {
					// The create block does not say how far along the spline the
					// unit is; assume it arrives at run speed from here.
					o.startMove(*b.Destination, b.RunSpeed, s.now())
				}
			}
			if b.RunSpeed > 0 {
				o.runSpeed = b.RunSpeed
			}
		}
	}
}

func (o *object) startMove(dest wire.Position, speed float32, now time.Time) {
	if speed <= 0 {
		speed = 7
	}
	d := distance2D(o.pos, dest)
	o.dest = &dest
	o.moveStart = now
	o.moveDur = time.Duration(float64(d/speed) * float64(time.Second))
}

// MonsterMove records a unit's new spline.
func (s *State) MonsterMove(m wire.MonsterMove) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.objects[m.GUID]
	if o == nil {
		return
	}
	now := s.now()
	o.pos = wire.Position{X: m.Start.X, Y: m.Start.Y, Z: m.Start.Z, O: o.pos.O}
	if m.Stop {
		o.dest = nil
		return
	}
	dest := m.Destination
	o.dest, o.moveStart, o.moveDur = &dest, now, time.Duration(m.DurationMs)*time.Millisecond
}

// Moved records another player's MSG_MOVE_* position.
func (s *State) Moved(guid uint64, p wire.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o := s.objects[guid]; o != nil {
		o.pos, o.dest = p, nil
	}
}

// Destroy forgets an object (SMSG_DESTROY_OBJECT).
func (s *State) Destroy(guid uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if guid != s.self {
		delete(s.objects, guid)
	}
}

// SetSelfPosition records the player's own position; the server does not echo
// the client's movement.
func (s *State) SetSelfPosition(mapID *uint32, p wire.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.obj(s.self)
	o.pos, o.dest = p, nil
	if o.typeID == 0 {
		o.typeID = wire.TypePlayer
	}
	if mapID != nil {
		if *mapID != s.mapID {
			// A new map: every other object is gone.
			for g := range s.objects {
				if g != s.self {
					delete(s.objects, g)
				}
			}
		}
		s.mapID = *mapID
	}
}

// Self is the player's state.
type Self struct {
	GUID                     uint64
	Map                      uint32
	Pos                      wire.Position
	Level, Health, MaxHealth uint32
	XP, NextXP, Faction      uint32
	Target                   uint64
	RunSpeed                 float32
	InCombat, Dead, Known    bool
}

func (s *State) Self() Self {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.objects[s.self]
	if o == nil {
		return Self{}
	}
	v := o.values
	speed := o.runSpeed
	if speed == 0 {
		speed = 7
	}
	_, created := v[wire.FieldMaxHealth]
	return Self{
		GUID: s.self, Map: s.mapID, Pos: o.position(s.now()),
		Level: v[wire.FieldLevel], Health: v[wire.FieldHealth], MaxHealth: v[wire.FieldMaxHealth],
		XP: v[wire.FieldPlayerXP], NextXP: v[wire.FieldPlayerNextXP], Faction: v[wire.FieldFactionTemplate],
		Target:   uint64(v[wire.FieldUnitTarget]) | uint64(v[wire.FieldUnitTarget+1])<<32,
		RunSpeed: speed, InCombat: v[wire.FieldUnitFlags]&wire.UnitFlagInCombat != 0,
		Dead: created && v[wire.FieldHealth] == 0, Known: created,
	}
}

// Unit is another unit as the player sees it.
type Unit struct {
	GUID                       uint64
	Entry                      uint32
	Player                     bool
	Pos                        wire.Position
	Dest                       *wire.Position
	Distance                   float32
	Level, Health, MaxHealth   uint32
	Faction                    uint32
	Reaction                   factions.Reaction
	Attackable, Dead, Lootable bool
	InCombat                   bool
	Target                     uint64
}

func (s *State) unit(guid uint64, o *object, self *object, now time.Time) Unit {
	v := o.values
	u := Unit{
		GUID: guid, Entry: v[wire.FieldEntry], Player: o.typeID == wire.TypePlayer, Pos: o.position(now),
		Level: v[wire.FieldLevel], Health: v[wire.FieldHealth], MaxHealth: v[wire.FieldMaxHealth],
		Faction: v[wire.FieldFactionTemplate], Target: uint64(v[wire.FieldUnitTarget]) | uint64(v[wire.FieldUnitTarget+1])<<32,
		InCombat: v[wire.FieldUnitFlags]&wire.UnitFlagInCombat != 0,
		Lootable: v[wire.FieldDynamicFlags]&wire.DynFlagLootable != 0,
	}
	if u.Player {
		u.Entry = 0
	}
	if o.dest != nil && now.Before(o.moveStart.Add(o.moveDur)) {
		d := *o.dest
		u.Dest = &d
	}
	u.Dead = u.MaxHealth > 0 && u.Health == 0 || v[wire.FieldDynamicFlags]&wire.DynFlagDead != 0
	selfFaction := uint32(0)
	if self != nil {
		selfFaction = self.values[wire.FieldFactionTemplate]
		u.Distance = distance3D(self.position(now), u.Pos)
	}
	u.Reaction = factions.ReactionTo(u.Faction, selfFaction)
	u.Attackable = !u.Dead && u.Reaction != factions.Friendly && u.Reaction != factions.Unknown &&
		v[wire.FieldUnitFlags]&wire.UnitFlagNotAttackable == 0
	return u
}

// Unit returns one unit by GUID.
func (s *State) Unit(guid uint64) (Unit, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.objects[guid]
	if o == nil || guid == s.self {
		return Unit{}, false
	}
	return s.unit(guid, o, s.objects[s.self], s.now()), true
}

// Units lists other units within radius yards, nearest first.
func (s *State) Units(radius float32, reaction string, aliveOnly bool, entry uint32) []Unit {
	s.mu.Lock()
	defer s.mu.Unlock()
	if radius <= 0 {
		radius = 100
	}
	now, self := s.now(), s.objects[s.self]
	var units []Unit
	for g, o := range s.objects {
		if g == s.self || (o.typeID != wire.TypeUnit && o.typeID != wire.TypePlayer) {
			continue
		}
		u := s.unit(g, o, self, now)
		if self != nil && u.Distance > radius ||
			reaction != "" && string(u.Reaction) != reaction ||
			aliveOnly && u.Dead ||
			entry != 0 && u.Entry != entry {
			continue
		}
		units = append(units, u)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Distance < units[j].Distance })
	return units
}

func distance2D(a, b wire.Position) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

func distance3D(a, b wire.Position) float32 {
	dx, dy, dz := float64(a.X-b.X), float64(a.Y-b.Y), float64(a.Z-b.Z)
	return float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
}
