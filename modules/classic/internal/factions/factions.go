// Package factions classifies WoW 1.12.1 units by faction template, using the
// rules of mangos-classic FactionTemplateEntry at revision 8ec338a1.
package factions

// Template is one FactionTemplate.dbc row.
type Template struct {
	Faction                       uint32
	Group, Friends, Enemies       uint32 // FactionGroupMask bits
	EnemyFactions, FriendFactions [4]uint32
}

// Reaction is how a unit regards another.
type Reaction string

const (
	Hostile  Reaction = "hostile"
	Neutral  Reaction = "neutral"
	Friendly Reaction = "friendly"
	Unknown  Reaction = "unknown"
)

// Lookup returns a template by ID.
func Lookup(id uint32) (Template, bool) {
	t, ok := templates[id]
	return t, ok
}

// React follows mangos-classic Relations.cpp GetFactionReaction(this, other):
// how a unit with template t regards one with template other.
func (t Template) React(other Template) Reaction {
	if other.Group&t.Enemies != 0 {
		return Hostile
	}
	if t.EnemyFactions[0] != 0 && other.Faction != 0 {
		for _, f := range t.EnemyFactions {
			if f == other.Faction {
				return Hostile
			}
		}
	}
	if other.Group&t.Friends != 0 {
		return Friendly
	}
	if t.FriendFactions[0] != 0 && other.Faction != 0 {
		for _, f := range t.FriendFactions {
			if f == other.Faction {
				return Friendly
			}
		}
	}
	if t.Group&other.Friends != 0 {
		return Friendly
	}
	if other.FriendFactions[0] != 0 && t.Faction != 0 {
		for _, f := range other.FriendFactions {
			if f == t.Faction {
				return Friendly
			}
		}
	}
	return Neutral
}

// ReactionTo reports how a unit with template unit regards the player with
// template self, which decides whether it attacks on sight. Factions the
// server resolves through the player's reputation (Faction.dbc entries with a
// reputation index) use their template default here, so a faction the player
// is at war with through reputation reads as its template reaction. Other
// players are classified by template too; the server's duel, group and
// free-for-all rules for players are not applied.
func ReactionTo(unit, self uint32) Reaction {
	u, ok1 := Lookup(unit)
	s, ok2 := Lookup(self)
	if !ok1 || !ok2 {
		return Unknown
	}
	return u.React(s)
}
