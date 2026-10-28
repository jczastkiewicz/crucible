package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castWithMask activates Illusionary Mask with GG for X=2 on a game whose hand
// holds Grizzly Bears ({1}{G}, castable from the mana) and Serra Angel ({3}{W}{W},
// not), and resolves the Mask and the face-down creature spell. It returns the
// game, the controller, the Bears and the controller object.
func castWithMask(t *testing.T) (*engine.Game, engine.PlayerID, engine.CardID, *engine.ScriptedController) {
	t.Helper()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	mask := g.NewCard(corpusCard(t, "Illusionary Mask"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Hand)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c.QueuePayX(2)
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	c.QueueConfirmEffect(true)
	if !g.ActivateAbility(p, mask, 0, c) {
		t.Fatal("ActivateAbility failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatal(err)
	}
	return g, p, bears, c
}

// TestIllusionaryMaskCastsTheCreatureFaceDown proves Mask's Play: only the
// creature whose mana cost the X mana can pay is offered (the Angel is not,
// so the choice is a single confirmation), the chosen card is cast without its
// cost as a face-down 2/2 spell and enters the battlefield face down
// (CR 708.3, 708.4), with none of its own text.
func TestIllusionaryMaskCastsTheCreatureFaceDown(t *testing.T) {
	t.Parallel()
	g, _, bears, _ := castWithMask(t)
	c := g.Card(bears)
	if c.Zone != engine.Battlefield || !c.IsFaceDown() {
		t.Fatalf("Bears zone = %v, face down = %v, want a face-down permanent", c.Zone, c.IsFaceDown())
	}
	if c.Def.Name != "" {
		t.Errorf("face-down name = %q, want none (CR 708.2a)", c.Def.Name)
	}
}

// TestIllusionaryMaskTurnsTheCreatureFaceUpWhenItIsDamaged proves the
// Event$ DealtDamage replacement: damage that would be dealt to the face-down
// creature turns it face up first, and it is dealt the damage as a Bears.
func TestIllusionaryMaskTurnsTheCreatureFaceUpWhenItIsDamaged(t *testing.T) {
	t.Parallel()
	g, p, bears, c := castWithMask(t)
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)}, "DB$ DealDamage | ValidTgts$ Creature | NumDmg$ 1"); err != nil {
		t.Fatal(err)
	}
	got := g.Card(bears)
	if got.IsFaceDown() || got.Def.Name != "Grizzly Bears" {
		t.Errorf("after damage: face down = %v, name = %q, want Grizzly Bears face up", got.IsFaceDown(), got.Def.Name)
	}
	if got.Damage.Marked != 1 {
		t.Errorf("damage marked = %d, want 1", got.Damage.Marked)
	}
}
