package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// ChooseColor's ExcludeColorsFrom$ takes the defined cards' colors out of the
// choices (ChooseColorEffect.java:58-68): Opulent Clomper's upkeep trigger
// picks a random color it is not, so the green Clomper ends up green and one
// other color, whatever the seed.
func TestExcludeColorsFromPicksAColorTheCardIsNot(t *testing.T) {
	t.Parallel()

	db := scenarioDB(t)
	for seed := int64(1); seed <= 20; seed++ {
		g := engine.NewGame(db, javarand.New(seed), []string{"a", "b"})
		p, other := g.Players()[0], g.Players()[1]
		g.SetTurnState(1, p, engine.Main1)
		clomper := g.NewCard(corpusCard(t, "Opulent Clomper"), p, engine.Battlefield)
		libraryCards(t, g, p, 5)
		libraryCards(t, g, other, 5)
		toStep(t, g, engine.NewScriptedController(), p, engine.Upkeep, nil)

		colors := g.Card(clomper).Colors()
		if !colors.Has(mana.Green) || colors.Count() != 2 {
			t.Errorf("seed %d: Opulent Clomper's colors = %v, want green and one other", seed, colors)
		}
	}
}
