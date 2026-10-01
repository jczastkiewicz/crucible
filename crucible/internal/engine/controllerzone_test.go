package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// stolenCreatureGame is a two-player game where p's Aura (Control Magic's
// real shape) takes other's creature, and one more creature of other's stays
// put. The first state-based-action pass applies the steal.
func stolenCreatureGame(t *testing.T) (g *engine.Game, p, other engine.PlayerID, aura, stolen, bystander engine.CardID) {
	t.Helper()
	g, p, other = layer456Game(t)
	aura = g.NewCard(layer456Def(t, "Test Control Magic", "Enchantment Aura", "", nil,
		"Mode$ Continuous | AffectedDefined$ Enchanted | GainControl$ You"), p, engine.Battlefield)
	stolen = g.NewCard(layer456Creature(t, "G"), other, engine.Battlefield)
	bystander = g.NewCard(layer456Creature(t, "G"), other, engine.Battlefield)
	g.Attach(aura, stolen)
	return g, p, other, aura, stolen, bystander
}

// TestStolenPermanentMovesToItsControllersBattlefield proves ADR-0037: after a
// Layer 2 steal the permanent is in the thief's battlefield list (at the
// end), not its owner's, keeps its timestamp, and moves back when the Aura
// leaves.
func TestStolenPermanentMovesToItsControllersBattlefield(t *testing.T) {
	t.Parallel()

	g, p, other, aura, stolen, bystander := stolenCreatureGame(t)
	stamp := g.Card(stolen).Timestamp

	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Zone(engine.Battlefield, p).Cards(); !slices.Equal(got, []engine.CardID{aura, stolen}) {
		t.Errorf("thief's battlefield = %v, want the Aura then the stolen creature", got)
	}
	if got := g.Zone(engine.Battlefield, other).Cards(); !slices.Equal(got, []engine.CardID{bystander}) {
		t.Errorf("owner's battlefield = %v, want only the bystander", got)
	}
	if got := g.Card(stolen).ZoneOwner; got != p {
		t.Errorf("stolen ZoneOwner = %v, want the thief %v", got, p)
	}
	if got := g.Card(stolen).Owner; got != other {
		t.Errorf("stolen Owner = %v, want %v: ownership never changes", got, other)
	}
	if got := g.Card(stolen).Timestamp; got != stamp {
		t.Errorf("stolen Timestamp = %d, want %d: control change is not a zone change (CR 400.7)", got, stamp)
	}
	if !g.Card(stolen).SummonSick {
		t.Error("stolen creature is not summoning sick under its new controller")
	}

	g.Move(aura, engine.Graveyard, p)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())

	if got := g.Zone(engine.Battlefield, other).Cards(); !slices.Equal(got, []engine.CardID{bystander, stolen}) {
		t.Errorf("owner's battlefield after the Aura left = %v, want the bystander then the returned creature", got)
	}
	if got := g.Zone(engine.Battlefield, p).Cards(); len(got) != 0 {
		t.Errorf("thief's battlefield after the Aura left = %v, want empty", got)
	}
}

// TestStolenCreatureAttacksForItsController proves the attack declaration
// reads the thief's battlefield: the stolen creature is offered to the thief
// and not to its owner.
func TestStolenCreatureAttacksForItsController(t *testing.T) {
	t.Parallel()

	g, p, _, _, stolen, _ := stolenCreatureGame(t)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	g.Card(stolen).SummonSick = false // controlled since the turn began
	g.SetTurnState(1, p, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{stolen})

	got, err := g.DeclareCombatAttackers(c)
	if err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if !slices.Equal(got, []engine.CardID{stolen}) {
		t.Errorf("attackers = %v, want the stolen creature", got)
	}
}

// TestStolenCreatureUntapsInItsControllersUntapStep proves the untap step
// reads the thief's battlefield: it untaps and loses its sickness for the
// thief, and the owner's untap step leaves it alone.
func TestStolenCreatureUntapsInItsControllersUntapStep(t *testing.T) {
	t.Parallel()

	g, p, other, _, stolen, _ := stolenCreatureGame(t)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	g.Card(stolen).Tapped = true

	g.StartTurn(other, engine.NewScriptedController())
	if !g.Card(stolen).Tapped {
		t.Error("stolen creature untapped in its owner's untap step")
	}
	g.StartTurn(p, engine.NewScriptedController())
	if g.Card(stolen).Tapped {
		t.Error("stolen creature stayed tapped through its controller's untap step")
	}
	if g.Card(stolen).SummonSick {
		t.Error("stolen creature is still summoning sick after its controller's untap step")
	}
}

// TestStolenCreatureBlocksForItsController proves the block declaration reads
// the thief's battlefield: the owner's attacker is blocked by the creature the
// owner lost, which is offered to its controller.
func TestStolenCreatureBlocksForItsController(t *testing.T) {
	t.Parallel()

	g, p, other, _, stolen, bystander := stolenCreatureGame(t)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	g.Card(bystander).SummonSick = false
	g.SetTurnState(1, other, engine.DeclareAttackers)
	c := engine.NewScriptedController()
	c.QueueAttackers([]engine.CardID{bystander})
	if _, err := g.DeclareCombatAttackers(c); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	want := []engine.Block{{Blocker: stolen, Attacker: bystander}}
	c.QueueBlocks(want)

	got, err := g.DeclareCombatBlockers(c)
	if err != nil {
		t.Fatalf("DeclareCombatBlockers: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("blocks = %v, want %v: the thief %v blocks with the stolen creature", got, want, p)
	}
}
