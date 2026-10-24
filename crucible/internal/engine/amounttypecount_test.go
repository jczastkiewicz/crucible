package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Korvold, Gleeful Glutton: Count$Valid Creature.YouCtrl$CreatureType counts
// the distinct creature types among the matches (AbilityUtils.java:3809), and
// the same measure reads planeswalker and land types.
func TestCountValidDistinctSubtypeKinds(t *testing.T) {
	t.Parallel()

	setup := func(g *engine.Game, p engine.PlayerID) {
		g.NewCard(fillerDef(t, "A", "Creature Elf Warrior", "1", "1"), p, engine.Battlefield)
		g.NewCard(fillerDef(t, "B", "Creature Elf Shaman", "1", "1"), p, engine.Battlefield)
		g.NewCard(fillerDef(t, "C", "Land Forest", "", ""), p, engine.Battlefield)
		g.NewCard(fillerDef(t, "D", "Land Forest", "", ""), p, engine.Battlefield)
		g.NewCard(fillerDef(t, "E", "Planeswalker Jace", "", ""), p, engine.Hand)
	}
	for _, tc := range []struct {
		x    string
		want int
	}{
		{"Count$Valid Creature.YouCtrl$CreatureType", 3},
		{"Count$Valid Land.YouCtrl$LandType", 1},
		{"Count$ValidHand Planeswalker.YouOwn$PlaneswalkerType", 1},
		{"Count$Valid Creature.YouCtrl$CreatureType/LimitMax.2", 2},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		setup(g, p)
		before := g.Player(p).Life
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ GainLife | Defined$ You | LifeAmount$ X", "X", tc.x)
		if got := g.Player(p).Life - before; got != tc.want {
			t.Errorf("%s = %d, want %d", tc.x, got, tc.want)
		}
	}
}

// Without the database's subtype vocabulary the measure is not guessed.
func TestCountValidSubtypeKindsNeedsTheVocabulary(t *testing.T) {
	t.Parallel()

	setup := func(g *engine.Game, p, _ engine.PlayerID) {
		g.NewCard(fillerDef(t, "A", "Creature Elf", "1", "1"), p, engine.Battlefield)
	}
	if err := gainErr(t, setup, "Count$Valid Creature.YouCtrl$CreatureType"); err == nil {
		t.Error("resolved without a type vocabulary")
	}
}

// Korvold's cost reduction: the permanent card types among what the player
// sacrificed this turn.
func TestPlayerCountSacrificedPermanentTypesThisTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(fillerDef(t, "A", "Artifact Creature Golem", "1", "1"), p, engine.Battlefield)
	g.NewCard(fillerDef(t, "B", "Land", "", ""), p, engine.Battlefield)
	keep := engine.NewScriptedController()
	keep.KeepCardOrder()
	if _, err := resolveNow(t, g, p, keep, nil, "DB$ SacrificeAll | ValidCards$ Permanent.YouCtrl"); err != nil {
		t.Fatal(err)
	}
	before := g.Player(p).Life
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ GainLife | Defined$ You | LifeAmount$ X",
		"X", "PlayerCountPropertyYou$SacrificedPermanentTypesThisTurn")
	if got := g.Player(p).Life - before; got != 3 {
		t.Errorf("types = %d, want 3 (artifact, creature, land)", got)
	}
}
