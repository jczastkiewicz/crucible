package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// putCounterAbility activates a "{T}: put two counters on Defined$" creature of
// p's and resolves it.
func putCounterAbility(t *testing.T, g *engine.Game, p engine.PlayerID, counterType, defined string) {
	t.Helper()
	def := scriptDef(t, "Test Gardener", "Creature Elf",
		"A:AB$ PutCounter | Cost$ T | Defined$ "+defined+" | CounterType$ "+counterType+" | CounterNum$ 2")
	gardener := g.NewCard(def, p, engine.Battlefield)
	g.Card(gardener).SummonSick = false
	c := engine.NewScriptedController()
	if !g.ActivateAbility(p, gardener, 0, c) {
		t.Fatal("ActivateAbility failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
}

// Mode$ CantPutCounter: ValidCard$ and CounterType$ name which counters a card
// cannot receive (Solemnity-style "counters can't be put on creatures").
func TestCantPutCounterOnCards(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		line string
		want int
	}{
		{"no static", "", 2},
		{"every counter on creatures", "S:Mode$ CantPutCounter | ValidCard$ Creature", 0},
		{"only that counter type", "S:Mode$ CantPutCounter | ValidCard$ Creature | CounterType$ P1P1", 0},
		{"another counter type", "S:Mode$ CantPutCounter | ValidCard$ Creature | CounterType$ M1M1", 2},
		{"another card type", "S:Mode$ CantPutCounter | ValidCard$ Artifact", 2},
		{"the player half does not name cards", "S:Mode$ CantPutCounter | ValidPlayer$ Player", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			if tc.line != "" {
				g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", tc.line), p, engine.Battlefield)
			}
			putCounterAbility(t, g, p, "P1P1", "Self")
			var got int
			for _, id := range g.Zone(engine.Battlefield, p).Cards() {
				got += g.Card(id).Counters.Count(engine.P1P1)
			}
			if got != tc.want {
				t.Errorf("+1/+1 counters on the battlefield = %d, want %d", got, tc.want)
			}
		})
	}
}

// ValidPlayer$ names players: "you can't get poison counters" (Melira) stops
// infect damage from poisoning its controller.
func TestCantPutCounterOnPlayers(t *testing.T) {
	t.Parallel()

	g, a, b := combatGame(t)
	g.NewCard(scriptDef(t, "Test Melira", "Creature Elf",
		"S:Mode$ CantPutCounter | ValidPlayer$ You | CounterType$ POISON"), b, engine.Battlefield)
	attacker := g.NewCard(creatureDefPTKeywords(t, "3", "3", "Infect"), a, engine.Battlefield)
	unblockedHit(t, g, attacker)

	if got := g.Player(b).Counters.Count(engine.Poison); got != 0 {
		t.Errorf("poison counters = %d, want 0 -- the static's controller can't get them", got)
	}
}
