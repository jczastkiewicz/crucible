package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Belt of Giant Strength's Equip {10} costs {X} less, X the power of the
// creature it targets (ReduceCost$ X, SVar X Targeted$CardPower): the target
// is chosen before the cost is adjusted (CR 601.2c before 601.2f), so equipping
// a 4/4 costs {6}.
func TestEquipReduceCostReadsTheTarget(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	belt := g.NewCard(corpusCard(t, "Belt of Giant Strength"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Green, 6)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	for range 6 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.ActivateAbility(p, belt, 0, c) {
		t.Fatalf("equip failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got, ok := g.Card(belt).AttachedTo(); !ok || got != bear {
		t.Errorf("belt attached to %v, want the 4/4", got)
	}
	if pw, _ := g.Card(bear).Power(); pw != 10 {
		t.Errorf("equipped power %d, want 10", pw)
	}
}

// Bloodthorn Flail's Equip {3} may be paid by discarding a card instead
// (AlternateCost$ Discard<1/Card>): the activation offers both costs
// (GameActionUtil.getAdditionalCostSpell) and the second costs no mana.
func TestEquipAlternateCostIsOfferedBesideThePrintedOne(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	flail := g.NewCard(corpusCard(t, "Bloodthorn Flail"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	card := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueOption(1) // Discard<1/Card> instead of {3}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	c.QueueDiscardChoice([]engine.CardID{card})
	if !g.ActivateAbility(p, flail, 0, c) {
		t.Fatalf("equip for the alternate cost failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(card).Zone; got != engine.Graveyard {
		t.Errorf("discarded card in %v, want the graveyard", got)
	}
	if got, ok := g.Card(flail).AttachedTo(); !ok || got != bear {
		t.Errorf("flail attached to %v, want the 2/2", got)
	}
}

// Razorfield Ripper's Reconfigure {2} may be paid with three energy instead
// (Reconfigure:2:PayEnergy<3>, CardFactoryUtil.java:3381).
func TestReconfigureAlternateCostPaysEnergy(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	ripper := g.NewCard(corpusCard(t, "Razorfield Ripper"), p, engine.Battlefield)
	bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.Player(p).Counters.Add(engine.Energy, 3)
	c := engine.NewScriptedController()
	c.QueueOption(1) // PayEnergy<3> instead of {2}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bear)})
	if !g.ActivateAbility(p, ripper, 0, c) {
		t.Fatalf("reconfigure for energy failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Player(p).Counters.Count(engine.Energy); got != 0 {
		t.Errorf("%d energy left, want 0", got)
	}
	if got, ok := g.Card(ripper).AttachedTo(); !ok || got != bear {
		t.Errorf("ripper attached to %v, want the 2/2", got)
	}
}

// Visions of Ruin's Flashback {8}{R}{R} costs {X} less, X the greatest mana
// value of a commander its owner has on the battlefield (the keyword's
// ReduceCost$ extra, read as the flashback spell's own): with a mana value 5
// commander out it is cast from the graveyard for {3}{R}{R}.
func TestFlashbackReduceCostReadsTheCardsSVar(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	visions := g.NewCard(corpusCard(t, "Visions of Ruin"), p, engine.Graveyard)
	commander := g.NewCard(corpusCard(t, "Shivan Dragon"), p, engine.Battlefield)
	g.AddCommander(p, commander)
	art := g.NewCard(corpusCard(t, "Ornithopter"), other, engine.Battlefield)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c := engine.NewScriptedController()
	for range 3 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueueSacrificeChoice([]engine.CardID{art})
	if !g.CastSpell(p, visions, c) {
		t.Fatalf("flashback cast failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(visions).Zone; got != engine.Exile {
		t.Errorf("Visions of Ruin in %v, want exiled by flashback", got)
	}
	if got := g.Card(art).Zone; got != engine.Graveyard {
		t.Errorf("opponent's artifact in %v, want sacrificed", got)
	}
}

// Without a commander the same flashback costs the full {8}{R}{R} (X is 0).
func TestFlashbackReduceCostWithoutACommanderIsTheFullCost(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	visions := g.NewCard(corpusCard(t, "Visions of Ruin"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Red, 11)
	c := engine.NewScriptedController()
	for range 8 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.CastSpell(p, visions, c) {
		t.Fatalf("flashback cast failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 1 {
		t.Errorf("%d mana left, want 1 of 11 after {8}{R}{R}", got)
	}
}

// Warbreak Trumpeter's Morph {X}{X}{R}: X is announced as the special action
// is paid, both X symbols stand for it, and the turned-face-up trigger creates
// X Goblins (Count$xPaid of the cause, AbilityUtils.java:1680-1685).
func TestMorphXCostAnnouncesXForTheTrigger(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	trumpeter := g.NewCard(corpusCard(t, "Warbreak Trumpeter"), p, engine.Hand)
	castFaceDown(t, g, p, trumpeter)
	g.Player(p).ManaPool.Add(mana.Red, 5)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	for range 4 {
		c.QueuePayGeneric(mana.ShardR)
	}
	if !g.TurnFaceUp(p, trumpeter, 0, c) {
		t.Fatalf("turn face up failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want all five spent on {2}{2}{R}", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	goblins := 0
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken {
			goblins++
		}
	}
	if goblins != 2 {
		t.Errorf("%d Goblin tokens, want X = 2", goblins)
	}
}

// The other Equip ReduceCost$ shapes, each with its reduction in force:
// Dragonfire Blade's colors of the target, Warrior's Blades' +1/+1 counters on
// it, Plate Armor's other Equipment, Arm-Mounted Anchor's near-empty hand and
// Crown of Gondor's monarch. Each equip spends exactly the reduced cost.
func TestEquipReduceCostShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		equipment string
		pay       int
		setup     func(g *engine.Game, p engine.PlayerID, target engine.CardID)
		target    string // a corpus creature, "" for a vanilla 2/2
	}{
		{"Dragonfire Blade", 2, nil, "Watchwolf"},
		{"Warrior's Blades", 1, func(g *engine.Game, _ engine.PlayerID, target engine.CardID) {
			g.Card(target).Counters.Add(engine.P1P1, 2)
		}, ""},
		{"Plate Armor", 2, func(g *engine.Game, p engine.PlayerID, _ engine.CardID) {
			g.NewCard(corpusCard(t, "Bloodthorn Flail"), p, engine.Battlefield)
		}, ""},
		{"Arm-Mounted Anchor", 0, nil, ""},
		{"Crown of Gondor", 1, func(g *engine.Game, p engine.PlayerID, _ engine.CardID) {
			g.SetMonarch(p)
		}, ""},
	} {
		t.Run(tc.equipment, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			eq := g.NewCard(corpusCard(t, tc.equipment), p, engine.Battlefield)
			def := creatureDefPT(t, "2", "2")
			if tc.target != "" {
				def = corpusCard(t, tc.target)
			}
			target := g.NewCard(def, p, engine.Battlefield)
			if tc.setup != nil {
				tc.setup(g, p, target)
			}
			g.Player(p).ManaPool.Add(mana.Green, tc.pay)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.CardEntity(target)})
			for range tc.pay {
				c.QueuePayGeneric(mana.ShardG)
			}
			if !g.ActivateAbility(p, eq, 0, c) {
				t.Fatalf("equip failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			if got, ok := g.Card(eq).AttachedTo(); !ok || got != target {
				t.Errorf("%s attached to %v, want the target", tc.equipment, got)
			}
		})
	}
}

// Aurelia's Vindicator's Disguise {X}{3}{W}: the turned-face-up trigger's
// "up to X other target creatures" reads the X the special action paid as
// its targets are chosen, so X = 2 exiles two creatures.
func TestDisguiseXCostBoundsTheTriggersTargets(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	angel := g.NewCard(corpusCard(t, "Aurelia's Vindicator"), p, engine.Hand)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	castFaceDown(t, g, p, angel)
	g.Player(p).ManaPool.Add(mana.White, 6)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	for range 5 {
		c.QueuePayGeneric(mana.ShardW)
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a), engine.CardEntity(b)})
	if !g.TurnFaceUp(p, angel, 0, c) {
		t.Fatalf("turn face up failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	for _, id := range []engine.CardID{a, b} {
		if got := g.Card(id).Zone; got != engine.Exile {
			t.Errorf("creature %v in %v, want exiled", id, got)
		}
	}
}

// Twinned Vision's Flashback {1}{U/R}{U/R} with Discard<1/Card>: the cost's
// non-mana part is paid with the mana, so the card is cast from the
// graveyard and the discarded card reaches the graveyard first.
func TestFlashbackPaysItsNonManaCostParts(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	vision := g.NewCard(corpusCard(t, "Twinned Vision"), p, engine.Graveyard)
	fodder := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 3)

	c := engine.NewScriptedController()
	c.QueueHybridManaColor(mana.Blue)
	c.QueueHybridManaColor(mana.Blue)
	c.QueuePayGeneric(mana.ShardU)
	c.QueueDiscardChoice([]engine.CardID{fodder})
	if !g.CastSpell(p, vision, c) {
		t.Fatal("CastSpell by Flashback from the graveyard failed")
	}
	if got := g.Card(fodder).Zone; got != engine.Graveyard {
		t.Errorf("discarded card zone = %v, want Graveyard", got)
	}
	if got := g.Card(vision).Zone; got != engine.Stack {
		t.Errorf("Twinned Vision zone = %v, want Stack", got)
	}
}
