package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Heroes in a Half Shell / Vulture: Defined$ TriggeredSources is the damage
// sources a DamageDoneOnce trigger's ValidSource$ keeps, so only the big
// attacker of the two gets the counter.
func TestDamageDoneOnceTriggeredSourcesAreTheValidSourcesOnly(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	a, b := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, a, engine.Main1)
	g.Player(a).Life, g.Player(b).Life = 20, 20
	small := g.NewCard(creatureDefPT(t, "3", "3"), a, engine.Battlefield)
	big := g.NewCard(creatureDefPT(t, "4", "4"), a, engine.Battlefield)

	raw := &carddb.Card{Filename: "sources-watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Sources Watcher"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Triggers = []string{
		"Mode$ DamageDoneOnce | CombatDamage$ True | ValidSource$ Creature.powerGE4+YouCtrl | ValidTarget$ Player | Execute$ TrigCounter",
	}
	raw.Faces[0].SVars.Set("TrigCounter", "DB$ PutCounter | Defined$ TriggeredSources | CounterType$ P1P1 | CounterNum$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g.NewCard(def, a, engine.Battlefield)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{small, big})
	declareAttackers(t, g, ac)
	bc := engine.NewScriptedController()
	bc.QueueBlocks(nil)
	declareBlockers(t, g, bc)
	dc := engine.NewScriptedController()
	g.DealCombatDamage(dc)
	if err := g.ResolveStack(engine.NewRegistry(), dc); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	if got := g.Card(big).Counters.Count("P1P1"); got != 1 {
		t.Errorf("counters on the 4/4 = %d, want 1", got)
	}
	if got := g.Card(small).Counters.Count("P1P1"); got != 0 {
		t.Errorf("counters on the 3/3 = %d, want 0", got)
	}
}
