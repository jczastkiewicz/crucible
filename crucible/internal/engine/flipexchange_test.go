package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestFlipCoinForEachPlayerResolvesTheSideWithThatPlayersRemembered proves
// FlipCoin ForEachPlayer$ (Mutalith Vortex Beast): one flip per named player,
// the winning side drawing, the losing side hurting the player lost for.
func TestFlipCoinForEachPlayerResolvesTheSideWithThatPlayersRemembered(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	libraryCards(t, g, p, 3)
	c := engine.NewScriptedController()
	c.QueueCoinCall(true)
	_ = resolveLine(t, g, p, c,
		"DB$ FlipCoin | ForEachPlayer$ Opponent | WinSubAbility$ DBDraw | LoseSubAbility$ DBDmg",
		"DBDraw", "DB$ Draw | NumCards$ Wins",
		"DBDmg", "DB$ DealDamage | NumDmg$ 3 | Defined$ Remembered")
	drew := g.Zone(engine.Hand, p).Len() == 1
	hurt := g.Player(other).Life == 17
	if drew == hurt {
		t.Errorf("drew = %v, hurt = %v, want exactly one side to resolve", drew, hurt)
	}
}

// TestExchangePowerBasePowerIgnoresCountersAndSurvivesPerpetually proves
// BasePower$ reads getCurrentPower (no counters) and Duration$ Perpetual keeps
// the swap (High Fae Prankster).
func TestExchangePowerBasePowerIgnoresCountersAndSurvivesPerpetually(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Battlefield)
	g.Card(a).Counters.Add(engine.P1P1, 1)
	pushPlayLine(t, g, p, []engine.EntityID{engine.EntityID(a), engine.EntityID(b)},
		"DB$ ExchangePower | ValidTgts$ Creature | TargetMin$ 2 | TargetMax$ 2 | BasePower$ True | Duration$ Perpetual")
	if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if pw, _ := g.Card(a).Power(); pw != 6 {
		t.Errorf("a power = %d, want 6 (the other's 5 plus its own counter)", pw)
	}
	if pw, _ := g.Card(b).Power(); pw != 2 {
		t.Errorf("b power = %d, want 2 (a's base, not its counter)", pw)
	}
}
