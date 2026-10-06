package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestCounterReplacementLinesThatDoNotApplyLetTheCounterHappen proves each way
// counterReplaced skips an Event$ Counter ReplaceWith$ line -- a ValidCard$ the
// spell does not match, a ValidSA$ it does not match or the port cannot read,
// failing requirements, a param it does not read -- leaves the spell countered.
func TestCounterReplacementLinesThatDoNotApplyLetTheCounterHappen(t *testing.T) {
	t.Parallel()

	for name, extra := range map[string]string{
		"ValidCard mismatch":  "ValidCard$ Card.Token",
		"ValidSA mismatch":    "ValidSA$ Activated",
		"ValidSA unreadable":  "ValidSA$ Bogus",
		"requirements fail":   "IsPresent$ Card.Nonexistent | PresentCompare$ GE1",
		"Spell with property": "ValidSA$ Spell.Foo",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGame(t)
			g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Guile Skip",
				"Event$ Counter | ActiveZones$ Battlefield | "+extra+" | ReplaceWith$ Rm",
				"Rm", "DB$ ChangeZone | Defined$ ReplacedCard | Origin$ Stack | Destination$ Exile"), p, engine.Battlefield)
			spell := g.NewCard(creatureDefCost(t, "Spell", "G"), other, engine.Hand)
			g.SetTurnState(1, other, engine.Main1)
			g.Player(other).ManaPool.Add(mana.Green, 1)
			if !g.CastSpell(other, spell, engine.NewScriptedController()) {
				t.Fatal("cast failed")
			}
			def := etbChainDef(t, "Counterer", "DB$ Counter | TargetType$ Spell | ValidTgts$ Card")
			host := g.NewCard(def, p, engine.Battlefield)
			sub := def.Faces[0].Triggers[0].Subs[0].Ability
			g.PushAbility(engine.Ability{API: engine.APICounter, Source: host, Controller: p, Params: sub,
				Targets: []engine.EntityID{engine.CardEntity(spell)}})
			// Any pending error is the line's refusal; the counter itself must happen.
			_ = g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController())
			if z := g.Card(spell).Zone; z != engine.Graveyard {
				t.Errorf("spell zone = %v, want Graveyard", z)
			}
		})
	}
}

// TestDestroyReplacementLinesThatDoNotApplyLetTheDestroyHappen proves the
// ValidCard$ and requirements skips of destroyInstead.
func TestDestroyReplacementLinesThatDoNotApplyLetTheDestroyHappen(t *testing.T) {
	t.Parallel()

	for name, extra := range map[string]string{
		"ValidCard mismatch": "ValidCard$ Card.Token",
		"requirements fail":  "IsPresent$ Card.Nonexistent | PresentCompare$ GE1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Egress Skip",
				"Event$ Destroy | ActiveZones$ Battlefield | "+extra+" | ReplaceWith$ Rm", "Rm", "DB$ Sacrifice"), p, engine.Battlefield)
			victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			def := etbChainDef(t, "Destroyer", "DB$ Destroy | Defined$ Targeted | ValidTgts$ Creature")
			host := g.NewCard(def, p, engine.Battlefield)
			sub := def.Faces[0].Triggers[0].Subs[0].Ability
			g.PushAbility(engine.Ability{API: engine.APIDestroy, Source: host, Controller: p, Params: sub,
				Targets: []engine.EntityID{engine.CardEntity(victim)}})
			if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err != nil {
				t.Fatal(err)
			}
			if g.Card(victim).Zone != engine.Graveyard {
				t.Errorf("victim zone = %v, want Graveyard", g.Card(victim).Zone)
			}
		})
	}
}

// TestBeginPhaseReplacementLinesTheDispatchCannotRun proves a BeginPhase line
// naming a param the dispatch does not read records a pending error and the
// phase begins, and a line with neither Skip$ True nor ReplaceWith$ is the
// same.
func TestBeginPhaseReplacementLinesTheDispatchCannotRun(t *testing.T) {
	t.Parallel()

	for name, line := range map[string]string{
		"unreadable param": "Event$ BeginPhase | ActiveZones$ Battlefield | Phase$ Draw | Skip$ True | Weird$ True",
		"neither":          "Event$ BeginPhase | ActiveZones$ Battlefield | Phase$ Draw",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			g.SetTurnState(1, p, engine.Upkeep)
			g.NewCard(replacementEnchantmentDef(t, "Test Odd Phase", line), p, engine.Battlefield)
			g.AdvancePhase(engine.NewScriptedController())
			if g.ActivePhase() != engine.Draw {
				t.Errorf("phase = %v, want Draw: the line is not runnable", g.ActivePhase())
			}
			if g.TakePendingError() == nil {
				t.Error("no pending error recorded")
			}
		})
	}
}
