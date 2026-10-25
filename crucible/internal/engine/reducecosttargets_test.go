package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A spell that costs {3} less if it targets a creature whose controller has
// eight or more cards in their graveyard, Lullmage's Domination's own ReduceCost
// line (Amount$ XGrave = Count$Compare CheckTgt GE8.3.0, CheckTgt =
// TargetedController$CardsInGraveyard) on a {3}{U} body: the amount reads the
// spell's chosen target (CostAdjustment.java:466-479 passes the spell's own
// ability). The real card's {X}{U}{U}{U} has no generic part at X = 0 to reduce.
func TestReduceCostAmountReadsTheSpellsTarget(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		graveyard int
		cost      int // mana the cast needs
	}{
		{"seven cards: full price", 7, 4},
		{"eight cards: three less", 8, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, extra := range []int{-1, 0} {
				g, p, other := newTwoPlayerGame(t)
				g.SetTurnState(1, p, engine.Main1)
				victim := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
				for range tc.graveyard {
					g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Graveyard)
				}
				def := scriptDef(t, "Test Domination", "Sorcery",
					"A:SP$ GainControl | ValidTgts$ Creature",
					"S:Mode$ ReduceCost | ValidCard$ Card.Self | Type$ Spell | Amount$ XGrave | Relative$ True | EffectZone$ All",
					"SVar:XGrave:Count$Compare CheckTgt GE8.3.0",
					"SVar:CheckTgt:TargetedController$CardsInGraveyard")
				def.Faces[0].ManaCost = mana.MustParse("3 U")
				spell := g.NewCard(def, p, engine.Hand)
				have := tc.cost + extra
				g.Player(p).ManaPool.Add(mana.Blue, have)
				c := engine.NewScriptedController()
				c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
				for range have {
					c.QueuePayGeneric(mana.ShardU)
				}
				if got := g.CastSpell(p, spell, c); got != (extra == 0) {
					t.Errorf("graveyard %d, %d mana: cast = %v, want %v", tc.graveyard, have, got, extra == 0)
				}
			}
		})
	}
}
