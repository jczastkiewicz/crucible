package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

func tappedPaidCard(t *testing.T, name, ability string, svars map[string]string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].Abilities = []string{ability}
	for k, v := range svars {
		raw.Faces[0].SVars.Set(k, v)
	}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return def
}

// Keldon Battlewagon: the activation's tapXType list is its Tapped$ list, so
// +X/+0 is the power of the creature tapped to pay.
func TestTappedAmountIsThePowerOfTheCreatureTappedForTheCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	wagon := g.NewCard(tappedPaidCard(t, "Test Wagon", "AB$ Pump | Cost$ tapXType<1/Creature> | NumAtt$ +X",
		map[string]string{"X": "Tapped$CardPower"}), p, engine.Battlefield)
	g.Card(wagon).SummonSick = false
	big := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTapChoice([]engine.CardID{big})
	if !g.ActivateAbility(p, wagon, 0, c) {
		t.Fatal("could not activate")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if power, _ := g.Card(wagon).Power(); power != 5 {
		t.Errorf("power = %d, want 1+4", power)
	}
}

// Defined$ Tapped reads the same list (Swallow Whole's counter).
func TestDefinedTappedIsTheCreatureTappedForTheCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	src := g.NewCard(tappedPaidCard(t, "Test Tapper",
		"AB$ PutCounter | Cost$ tapXType<1/Creature> | Defined$ Tapped | CounterType$ P1P1 | CounterNum$ 1", nil), p, engine.Battlefield)
	g.Card(src).SummonSick = false
	other := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTapChoice([]engine.CardID{other})
	if !g.ActivateAbility(p, src, 0, c) {
		t.Fatal("could not activate")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(other).Counters.Count("P1P1"); got != 1 {
		t.Errorf("counters on the tapped creature = %d, want 1", got)
	}
	if got := g.Card(src).Counters.Count("P1P1"); got != 0 {
		t.Errorf("counters on the source = %d, want 0", got)
	}
}
