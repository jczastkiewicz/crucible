package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestControlMagicShapeGainsControlThroughAffectedDefined proves Layer 2's
// GainControl$ You reaches the enchanted creature through AffectedDefined$
// Enchanted, the shape 35 of the corpus's 42 real Mode$ Continuous
// GainControl$ lines write (Control Magic), and nothing else: a bystander
// stays with its owner, and control reverts when the Aura leaves.
func TestControlMagicShapeGainsControlThroughAffectedDefined(t *testing.T) {
	t.Parallel()

	g, p, other := layer456Game(t)
	aura := g.NewCard(layer456Def(t, "Test Control Magic", "Enchantment Aura", "", nil,
		"Mode$ Continuous | AffectedDefined$ Enchanted | GainControl$ You"), p, engine.Battlefield)
	stolen := g.NewCard(layer456Creature(t, "G"), other, engine.Battlefield)
	bystander := g.NewCard(layer456Creature(t, "G"), other, engine.Battlefield)
	g.Attach(aura, stolen)

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Card(stolen).Controller(); got != p {
		t.Errorf("enchanted creature controller = %v, want the Aura's controller %v", got, p)
	}
	if got := g.Card(bystander).Controller(); got != other {
		t.Errorf("bystander controller = %v, want its owner %v", got, other)
	}

	g.Move(aura, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	if got := g.Card(stolen).Controller(); got != other {
		t.Errorf("controller after the Aura left = %v, want control back with the owner %v", got, other)
	}
}
