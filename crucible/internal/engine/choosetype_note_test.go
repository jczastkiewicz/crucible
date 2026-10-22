package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestChooseTypeNoteRecordsWithoutChoosingAndSkipsNotedTypes proves Note$
// (ChooseTypeEffect.java:117-121, :138-142): the pick is appended to the
// host's noted types, not written as its chosen type, and a later ChooseType
// on the same card no longer offers it.
func TestChooseTypeNoteRecordsWithoutChoosingAndSkipsNotedTypes(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Noter", "DB$ ChooseType | Defined$ You | Type$ Creature | ValidTypes$ Elf,Goblin,Human | Note$ True")
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	rec := &optionRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueOption(0)
	rec.QueueOption(0)
	for range 2 {
		g.PushAbility(engine.Ability{API: engine.APIChooseType, Source: host, Controller: p, Params: sub})
		if err := g.ResolveStack(engine.NewRegistry(), rec); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
	}
	if got := g.Card(host).Memory.NotedTypes(); !slices.Equal(got, []string{"Elf", "Goblin"}) {
		t.Errorf("noted = %v, want [Elf Goblin]", got)
	}
	if got := g.Card(host).Memory.ChosenType(false); got != "" {
		t.Errorf("chosen type = %q, want none", got)
	}
	if len(rec.offers) != 2 || !slices.Equal(rec.offers[1], []string{"Goblin", "Human"}) {
		t.Errorf("offers = %v, want the second without Elf", rec.offers)
	}
}

// TestChooseTypeMostPrevalentInDefinedZone proves MostPrevalentInDefinedZone$
// (Favored Enemy): the creature types held by the most cards of the named
// player's zone are offered, and the NotedType valid property then matches a
// creature of the noted type (CardStateProperty.java:107-115).
func TestChooseTypeMostPrevalentInDefinedZone(t *testing.T) {
	t.Parallel()

	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.NewCard(etbChainDef(t, "Elf One", "DB$ BlankLine"), opp, engine.Library)
	g.NewCard(etbChainDef(t, "Elf Two", "DB$ BlankLine"), opp, engine.Library)
	rec := &optionRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueOption(0)
	host, err := resolveNow(t, g, p, rec.ScriptedController, nil, "DB$ ChooseType | Defined$ You | Type$ Creature | MostPrevalentInDefinedZone$ Player.Opponent_Library | Note$ True")
	if err != nil {
		t.Fatalf("ChooseType: %v", err)
	}
	if got := g.Card(host).Memory.NotedTypes(); !slices.Equal(got, []string{"Elf"}) {
		t.Errorf("noted = %v, want [Elf]", got)
	}
}
