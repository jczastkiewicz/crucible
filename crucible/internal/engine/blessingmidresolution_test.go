package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Ascend's blessing is given between the links of a resolving ability, as
// Java's checkStaticAbilities runs the Always triggers before each
// sub-ability (AbilityUtils.java:1369, GameAction.java:1245): Ocelot Pride's
// Cat token is the tenth permanent, so its next link copies each token that
// entered this turn, that Cat included.
func TestBlessingGivenMidResolutionCountsForTheNextLink(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		lands, cats int
	}{{7, 2}, {6, 1}} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		libraryCards(t, g, p, 4)
		libraryCards(t, g, other, 4)
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, "Ocelot Pride"), p, engine.Battlefield)
		for range tc.lands {
			g.NewCard(corpusCard(t, "Plains"), p, engine.Battlefield)
		}
		c := engine.NewScriptedController()
		if _, err := resolveNow(t, g, p, c, nil, "DB$ GainLife | Defined$ You | LifeAmount$ 1"); err != nil {
			t.Fatal(err)
		}
		toStep(t, g, c, p, engine.EndOfTurn, nil)
		cats := 0
		for _, id := range g.Zone(engine.Battlefield, p).Cards() {
			if card := g.Card(id); card.IsToken && card.Name() == "Cat Token" {
				cats++
			}
		}
		if cats != tc.cats || g.Player(p).Blessing != (tc.cats == 2) {
			t.Errorf("%d lands: %d Cat tokens, blessing %v; want %d", tc.lands, cats, g.Player(p).Blessing, tc.cats)
		}
	}
}
