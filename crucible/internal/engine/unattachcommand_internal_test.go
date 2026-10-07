package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): LoseControl$ UntilSourceUnattached reads the attachment a
// Mode$ Attached trigger recorded as its triggering Source, and no trigger mode
// in this port records one yet, so only a hand-built Ability can carry it.
// This file pins the unattach command list itself (ControlGainEffect.java:
// 196-201): control ends when the attachment is detached or leaves play.

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/carddb/vocab"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

func TestGainControlUntilSourceUnattachedEndsWithTheAttachment(t *testing.T) {
	t.Parallel()

	for _, how := range []string{"unattached", "leaves play"} {
		g := NewGame(nil, javarand.New(1), []string{"a", "b"})
		a, b := g.Players()[0], g.Players()[1]
		g.SetTurnState(1, a, Main1)
		def := targetCandidatesTestCreature(t)
		host := g.NewCard(def, a, Battlefield)
		victim := g.NewCard(def, b, Battlefield)
		aura := g.NewCard(def, a, Battlefield)
		holder := g.NewCard(def, b, Battlefield)
		g.Attach(aura, holder)

		ab := Ability{API: APIGainControl, Source: host, Controller: a,
			Targets:   []EntityID{CardEntity(victim)},
			triggered: triggeredObjects{source: CardEntity(aura)},
			Params: &compile.Ability{Name: "GainControl", Params: []vocab.Param{
				{Key: "ValidTgts", Value: "Creature"}, {Key: "LoseControl", Value: "UntilSourceUnattached"},
			}}}
		if err := NewRegistry().Resolve(g, &ab, NewScriptedController()); err != nil {
			t.Fatalf("%s: %v", how, err)
		}
		if got := g.Card(victim).Controller(); got != a {
			t.Fatalf("%s: controller = %v, want the thief %v", how, got, a)
		}
		if how == "unattached" {
			g.Unattach(aura)
		} else {
			g.Move(aura, Graveyard, a)
		}
		if got := g.Card(victim).Controller(); got != b {
			t.Errorf("%s: controller = %v, want the owner %v", how, got, b)
		}
	}
}
