package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// usableManaReader records the pool a payment may spend when it is asked
// which mana pays a generic unit.
type usableManaReader struct {
	*engine.ScriptedController
	asked bool
	seen  [6]int
}

func (r *usableManaReader) ChoosePayGeneric(g *engine.Game, decider engine.PlayerID) mana.Shard {
	r.asked, r.seen = true, g.UsableMana(decider)
	return r.ScriptedController.ChoosePayGeneric(g, decider)
}

// A controller choosing generic mana sees only the units the payment may
// spend: Eldrazi Temple's {C}{C} pays for colorless Eldrazi alone, so it is
// absent while Sol Ring is paid for, though the pool still holds it.
func TestUsableManaLeavesOutRestrictedMana(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	r := &usableManaReader{ScriptedController: engine.NewScriptedController()}
	temple := g.NewCard(corpusCard(t, "Eldrazi Temple"), p, engine.Battlefield)
	forest := g.NewCard(scriptDef(t, "Test Forest", "Land", "A:AB$ Mana | Cost$ T | Produced$ G"), p, engine.Battlefield)
	island := g.NewCard(scriptDef(t, "Test Island", "Land", "A:AB$ Mana | Cost$ T | Produced$ U"), p, engine.Battlefield)
	// The Temple's second ability is its restricted {C}{C}.
	for _, src := range []struct {
		id    engine.CardID
		index int
	}{{temple, 1}, {forest, 0}, {island, 0}} {
		if !g.ActivateManaAbility(p, src.id, src.index, r) {
			t.Fatalf("%s not tapped: %v", g.Card(src.id).Name(), g.TakePendingError())
		}
	}
	ring := g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Hand)
	r.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, ring, r) {
		t.Fatalf("Sol Ring not cast: %v", g.TakePendingError())
	}
	if want := [6]int{0, 1, 0, 0, 1, 0}; !r.asked || r.seen != want {
		t.Errorf("asked %v, usable %v; want %v, the Temple's {C}{C} left out", r.asked, r.seen, want)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got[5] != 2 {
		t.Errorf("pool %v, want the Temple's {C}{C} still there", got)
	}
}
