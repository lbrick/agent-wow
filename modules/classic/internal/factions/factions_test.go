package factions

import "testing"

func TestReactions(t *testing.T) {
	const human = 1 // PLAYER, Human: group PLAYER|ALLIANCE
	for _, tc := range []struct {
		unit uint32
		want Reaction
	}{
		{14, Hostile},  // Monster: enemy of the PLAYER group
		{2, Hostile},   // PLAYER, Orc: enemy of ALLIANCE
		{12, Friendly}, // Stormwind: friend of ALLIANCE
		{32, Neutral},  // faction 29 (entry 299, Young Wolf): no enemy masks
		{31, Neutral},  // Prey
		{human, Friendly},
		{99999, Unknown},
	} {
		if got := ReactionTo(tc.unit, human); got != tc.want {
			t.Errorf("ReactionTo(%d, human) = %s, want %s", tc.unit, got, tc.want)
		}
	}
}
