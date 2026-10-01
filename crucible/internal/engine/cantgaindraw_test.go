package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// gainThree activates a "{T}: you gain 3 life" creature of p's and resolves it.
func gainThree(t *testing.T, g *engine.Game, p engine.PlayerID) {
	t.Helper()
	def := scriptDef(t, "Test Healer", "Creature Elf", "A:AB$ GainLife | Cost$ T | Defined$ You | LifeAmount$ 3")
	healer := g.NewCard(def, p, engine.Battlefield)
	g.Card(healer).SummonSick = false
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, healer, 0, c) {
		t.Fatal("ActivateAbility failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// Mode$ CantGainLife with ValidPlayer$ names which players gain no life
// (Erebos's Titan-style "your opponents can't gain life", Sulfuric Vortex's
// "players can't gain life").
func TestCantGainLifeStopsOnlyTheNamedPlayers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		line       string
		gainerLife int
	}{
		{"everyone", "S:Mode$ CantGainLife | ValidPlayer$ Player", 20},
		{"opponents of the static's controller", "S:Mode$ CantGainLife | ValidPlayer$ Player.Opponent", 23},
		{"no ValidPlayer names every player", "S:Mode$ CantGainLife", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			g.NewCard(scriptDef(t, "Test Vortex", "Enchantment", tc.line), p, engine.Battlefield)
			// other is the opponent of the static's controller p; p is the gainer.
			gainThree(t, g, p)
			if got := g.Player(p).Life; got != tc.gainerLife {
				t.Errorf("the static controller's life = %d, want %d", got, tc.gainerLife)
			}
			gainThree(t, g, other)
			if tc.line == "S:Mode$ CantGainLife | ValidPlayer$ Player.Opponent" {
				if got := g.Player(other).Life; got != 20 {
					t.Errorf("the opponent's life = %d, want 20", got)
				}
			}
		})
	}
}

// A static whose Condition$ fails does not apply.
func TestCantGainLifeHonoursItsCondition(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Conditional", "Enchantment",
		"S:Mode$ CantGainLife | ValidPlayer$ You | Condition$ NotPlayerTurn"), p, engine.Battlefield)
	gainThree(t, g, p) // p's own turn: NotPlayerTurn fails
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 -- the static is off on its controller's turn", got)
	}
}

// Mode$ CantDraw: no DrawLimit$ means no draws; DrawLimit$ N allows N a turn
// (Spirit of the Labyrinth's "can't draw more than one card each turn").
func TestCantDrawAndDrawLimit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		line  string
		drawn int
	}{
		{"no limit", "S:Mode$ CantDraw | ValidPlayer$ Player", 0},
		{"limit one", "S:Mode$ CantDraw | ValidPlayer$ Player | DrawLimit$ 1", 1},
		{"the static controller's opponents", "S:Mode$ CantDraw | ValidPlayer$ Opponent", 0},
		{"only its controller", "S:Mode$ CantDraw | ValidPlayer$ You", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(scriptDef(t, "Test Labyrinth", "Creature Elf", tc.line), g.Players()[1], engine.Battlefield)
			for range 5 {
				g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
			}
			g.DrawCards(p, 3, engine.NewScriptedController())
			if got := len(g.Zone(engine.Hand, p).Cards()); got != tc.drawn {
				t.Errorf("cards drawn = %d, want %d", got, tc.drawn)
			}
		})
	}
}
