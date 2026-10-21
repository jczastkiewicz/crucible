package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): causeMatches and controlsMatches are pure readers of a valid
// string; a table over their spellings is the whole contract.

import "testing"

func TestCauseMatchesKindsAndProperties(t *testing.T) {
	t.Parallel()

	const host, opp = PlayerID(1), PlayerID(2)
	for _, tc := range []struct {
		spec, kind string
		cause      PlayerID
		want       bool
	}{
		{"Spell", causeSpell, host, true},
		{"Spell", causeActivated, host, false},
		{"Activated", causeManaAbil, host, true},
		{"Activated", causeSpell, host, false},
		{"Triggered", causeTriggered, host, true},
		{"Triggered", causeActivated, host, false},
		{"SpellAbility", causeTriggered, host, true},
		{"Bogus", causeSpell, host, false},
		{"Activated.ManaAbility", causeManaAbil, host, true},
		{"Activated.ManaAbility", causeActivated, host, false},
		{"Activated.!ManaAbility", causeActivated, host, true},
		{"Activated.!ManaAbility", causeManaAbil, host, false},
		{"Spell.YouCtrl", causeSpell, host, true},
		{"Spell.YouCtrl", causeSpell, opp, false},
		{"Spell.YouCtrl", causeSpell, NoPlayer, false},
		{"Spell.OppCtrl", causeSpell, opp, true},
		{"Spell.OppCtrl", causeSpell, host, false},
		{"Spell.Bogus", causeSpell, host, false},
		{"Triggered,Spell.YouCtrl", causeSpell, host, true},
	} {
		if got := causeMatches(tc.spec, tc.kind, tc.cause, host); got != tc.want {
			t.Errorf("causeMatches(%q, %q, %v) = %v, want %v", tc.spec, tc.kind, tc.cause, got, tc.want)
		}
	}
}

func TestControlsMatchesCountsTheCandidatesPermanents(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	def := targetCandidatesTestCreature(t)
	g.NewCard(def, other, Battlefield)
	g.NewCard(def, other, Battlefield)
	src := g.NewCard(def, p, Battlefield)

	for _, tc := range []struct {
		property       string
		matched, known bool
	}{
		{"Creature", true, true},
		{"Land", false, true},
		{"Creature_GE2", true, true},
		{"Creature_GE3", false, true},
		{"Creature_EQ2", true, true},
		{"Creature_GE", false, false},
		{"Creature_GEx", false, false},
		{`Creature\_x`, false, true},
	} {
		matched, known := controlsMatches(g, other, p, src, tc.property)
		if matched != tc.matched || known != tc.known {
			t.Errorf("controlsMatches(%q) = (%v, %v), want (%v, %v)", tc.property, matched, known, tc.matched, tc.known)
		}
	}
}
