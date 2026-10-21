package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// newVocabularyGame is newTwoPlayerGame over a DB that knows the subtype
// vocabulary, which RemoveCreatureTypes$ needs.
func newVocabularyGame(t *testing.T) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	db := compile.NewDB(nil).WithTypes(attachmentTypeRegistry(t))
	g := engine.NewGame(db, javarand.New(1), []string{"a", "b"})
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	return g, p, other
}

// assimilated reports whether id is a Borg artifact creature that lost its
// old creature types (AssimilateEffect.ANIMATE).
func assimilated(g *engine.Game, id engine.CardID) bool {
	typ := g.Card(id).Type()
	return typ.Has(cardtype.Artifact) && typ.Has(cardtype.Creature) && typ.HasSubtype("Borg") && !typ.HasSubtype("Elf")
}

// TestAssimilateEffectEntersFromGraveyard proves a card in another zone
// enters under the activator's control with a +1/+1 counter and the Borg
// types (Borg Queen's shape).
func TestAssimilateEffectEntersFromGraveyard(t *testing.T) {
	t.Parallel()

	g, p, other := newVocabularyGame(t)
	buried := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Graveyard)

	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(buried)},
		"DB$ Assimilate | TgtZone$ Graveyard | ValidTgts$ Creature.OppOwn"); err != nil {
		t.Fatalf("Assimilate: %v", err)
	}
	c := g.Card(buried)
	if c.Zone != engine.Battlefield || c.Controller() != p {
		t.Errorf("zone %v controller %v, want the battlefield under %v", c.Zone, c.Controller(), p)
	}
	if got := c.Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("+1/+1 counters = %d, want 1", got)
	}
	if !assimilated(g, buried) {
		t.Error("card is not a Borg artifact creature without its old types")
	}
}

// TestAssimilateEffectTakesPermanentInPlace proves a permanent only changes
// hands and gets the counter and types where it stands.
func TestAssimilateEffectTakesPermanentInPlace(t *testing.T) {
	t.Parallel()

	g, p, other := newVocabularyGame(t)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)

	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(theirs)},
		"DB$ Assimilate | ValidTgts$ Creature"); err != nil {
		t.Fatalf("Assimilate: %v", err)
	}
	c := g.Card(theirs)
	if c.Controller() != p {
		t.Errorf("controller = %v, want %v", c.Controller(), p)
	}
	if got := c.Counters.Count(engine.P1P1); got != 1 {
		t.Errorf("+1/+1 counters = %d, want 1", got)
	}
	if !assimilated(g, theirs) {
		t.Error("permanent is not a Borg artifact creature without its old types")
	}
}

// TestAssimilateEffectNeedsSubtypeVocabulary proves the creature-type
// removal fails closed on a DB without the vocabulary.
func TestAssimilateEffectNeedsSubtypeVocabulary(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(theirs)},
		"DB$ Assimilate | ValidTgts$ Creature")
	if err == nil || !strings.Contains(err.Error(), "vocabulary") {
		t.Errorf("error = %v, want a vocabulary error", err)
	}
}
