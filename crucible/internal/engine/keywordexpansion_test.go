package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// ADR-0038: a printed Equip, Cycling, Prowess or Exalted line compiles into the
// traits CardFactoryUtil builds for it, and the engine plays them.

// Equip is sorcery speed (CR 702.6a): on the opponent's turn the ability cannot
// be activated; on its own turn it attaches, and re-equipping moves it.
func TestEquipAttachesAtSorcerySpeedAndMoves(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	sword := g.NewCard(corpusCard(t, "Bonesplitter"), p, engine.Battlefield)
	first := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	second := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)

	equip := func(target engine.CardID) bool {
		g.Player(p).ManaPool.Add(mana.Red, 1)
		c := engine.NewScriptedController()
		c.QueuePayGeneric(mana.ShardR)
		c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
		if !g.ActivateAbility(p, sword, 0, c) {
			return false
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("ResolveStack: %v", err)
		}
		return true
	}

	g.SetTurnState(1, other, engine.Main1)
	if equip(first) {
		t.Fatal("Equip activated on the opponent's turn")
	}
	g.Player(p).ManaPool = engine.Pool{}
	g.SetTurnState(2, p, engine.Main1)
	if !equip(first) {
		t.Fatal("Equip could not be activated at sorcery speed")
	}
	if got, _ := g.Card(sword).AttachedTo(); got != first {
		t.Fatalf("Bonesplitter attached to %v, want the first Bears", got)
	}
	if !equip(second) {
		t.Fatal("re-equip failed")
	}
	if got, _ := g.Card(sword).AttachedTo(); got != second {
		t.Errorf("Bonesplitter attached to %v after re-equip, want the second Bears", got)
	}
	sba(g)
	if power, _ := g.Card(second).Power(); power != 4 {
		t.Errorf("equipped Bears power = %d, want 4", power)
	}
}

// Cycling (CR 702.29a): pay the cost, discard the card from hand, draw a card.
func TestCyclingDiscardsAndDraws(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	cycler := g.NewCard(corpusCard(t, "Barkhide Mauler"), p, engine.Hand)
	top := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardG)
	c.QueuePayGeneric(mana.ShardG)
	if !g.ActivateAbility(p, cycler, 0, c) {
		t.Fatal("cycling could not be activated from hand")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Card(cycler).Zone != engine.Graveyard || g.Card(top).Zone != engine.Hand {
		t.Errorf("cycler in %v, drawn card in %v; want Graveyard and Hand", g.Card(cycler).Zone, g.Card(top).Zone)
	}
}

// Prowess (CR 702.108a): a noncreature spell cast by the controller gives +1/+1
// until end of turn.
func TestProwessPumpsOnANoncreatureSpell(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	monk := g.NewCard(corpusCard(t, "Monastery Swiftspear"), p, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(g.Players()[1])})
	if !g.CastSpell(p, bolt, c) {
		t.Fatal("cast failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if power, _ := g.Card(monk).Power(); power != 2 {
		t.Errorf("Swiftspear power = %d, want 2 after one prowess trigger", power)
	}
}

// Exalted (CR 702.83a): a creature attacking alone gets +1/+1 for each
// instance among permanents its controller controls.
func TestExaltedPumpsALoneAttacker(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	squire := g.NewCard(corpusCard(t, "Akrasan Squire"), p, engine.Battlefield)
	g.Card(squire).SummonSick = false
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{squire})
	if _, err := g.DeclareCombatAttackers(ac); err != nil {
		t.Fatalf("DeclareCombatAttackers: %v", err)
	}
	if err := g.ResolveStack(engine.NewRegistry(), ac); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if power, _ := g.Card(squire).Power(); power != 2 {
		t.Errorf("Akrasan Squire power = %d, want 2", power)
	}
}
