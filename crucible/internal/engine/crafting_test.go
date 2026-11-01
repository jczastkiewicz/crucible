package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// craftBraidedNet seats Braided Net (Craft with an artifact {1}{U}) for p in
// p's main phase, and an artifact to craft with when material is true.
func craftBraidedNet(t *testing.T, material bool) (*engine.Game, engine.PlayerID, engine.PlayerID, engine.CardID, engine.CardID) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	net := g.NewCard(corpusCard(t, "Braided Net"), p, engine.Battlefield)
	ring := engine.NoCard
	if material {
		ring = g.NewCard(corpusCard(t, "Sol Ring"), p, engine.Battlefield)
	}
	return g, p, other, net, ring
}

// craftIndex is the Craft ability's index among the card's activated
// abilities: the last one, after the printed lines.
func craftIndex(g *engine.Game, card engine.CardID) int {
	return len(g.Card(card).Def.Faces[0].Abilities) - 1
}

// Craft (CR 702.167, CardFactoryUtil.java:2734-2743): exile the permanent and
// the material as the cost, then return it transformed under its owner's
// control, at sorcery speed.
func TestCraftExilesTheMaterialAndReturnsTransformed(t *testing.T) {
	t.Parallel()
	g, p, _, net, ring := craftBraidedNet(t, true)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c.QueuePayGeneric(mana.ShardU)
	c.QueueCardChoice([]engine.CardID{ring})
	if !g.ActivateAbility(p, net, craftIndex(g, net), c) {
		t.Fatalf("Craft: %v", g.TakePendingError())
	}
	if z := g.Card(net).Zone; z != engine.Exile {
		t.Errorf("crafted card in %v while the ability waits, want Exile", z)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	n := g.Card(net)
	if n.Zone != engine.Battlefield || !n.InTransformedState() || n.Name() != "Braided Quipu" || n.Controller() != p {
		t.Errorf("crafted card %q in %v transformed %v, want Braided Quipu on p's battlefield", n.Name(), n.Zone, n.InTransformedState())
	}
	if z := g.Card(ring).Zone; z != engine.Exile {
		t.Errorf("material in %v, want Exile", z)
	}
}

// Craft is sorcery speed and needs its material: not on the opponent's turn,
// and not with no other artifact to exile.
func TestCraftNeedsSorcerySpeedAndMaterial(t *testing.T) {
	t.Parallel()
	g, p, other, net, _ := craftBraidedNet(t, true)
	g.SetTurnState(1, other, engine.Main1)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	if g.ActivateAbility(p, net, craftIndex(g, net), engine.NewScriptedController()) {
		t.Error("Craft activated on the opponent's turn")
	}

	g, p, _, net, _ = craftBraidedNet(t, false)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	if g.ActivateAbility(p, net, craftIndex(g, net), engine.NewScriptedController()) {
		t.Error("Craft activated with no artifact to exile")
	}
	if z := g.Card(net).Zone; z != engine.Battlefield {
		t.Errorf("Braided Net in %v after a refused Craft, want Battlefield", z)
	}
}

// Paleontologist's Pick-Axe crafts with X creatures (XMin1) into Dinosaur
// Headdress. The returned permanent keeps the exiled cards as its own
// (GameAction.java:485-486), so when its ETB attaches it, the Attached
// replacement chooses among them and the Static$ True Attached trigger makes
// the equipped creature a copy of the chosen card.
func TestDinosaurHeaddressCopiesACreatureUsedToCraftIt(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	axe := g.NewCard(corpusCard(t, "Paleontologist's Pick-Axe"), p, engine.Battlefield)
	angel := g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Graveyard)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Red, 6)
	for range 6 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueuePayX(1)
	c.QueueCardChoice([]engine.CardID{angel})
	if !g.ActivateAbility(p, axe, craftIndex(g, axe), c) {
		t.Fatalf("Craft: %v", g.TakePendingError())
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bears)})
	c.QueueCardChoice([]engine.CardID{angel})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	sba(g)
	sba(g)
	if name := g.Card(axe).Name(); name != "Dinosaur Headdress" {
		t.Fatalf("crafted card is %q, want Dinosaur Headdress", name)
	}
	if at, _ := g.Card(axe).AttachedTo(); at != bears {
		t.Errorf("Headdress attached to %v, want the Bears", at)
	}
	if name := g.Card(bears).Name(); name != "Serra Angel" {
		t.Errorf("equipped Bears is %q, want a copy of Serra Angel", name)
	}
	if pw, _ := g.Card(bears).Power(); pw != 4 {
		t.Errorf("equipped Bears power = %d, want 4", pw)
	}
}

// Throne of the Grim Captain crafts with four ExileCtrlOrGrave parts, one card
// of each type, paid part by part; without a Pirate it cannot be crafted.
func TestThroneOfTheGrimCaptainCraftsWithOneOfEachType(t *testing.T) {
	t.Parallel()
	for _, withPirate := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		throne := g.NewCard(corpusCard(t, "Throne of the Grim Captain"), p, engine.Battlefield)
		var materials []engine.CardID
		for _, name := range []string{"Colossal Dreadmaw", "Merfolk of the Pearl Trident", "Fathom Fleet Cutthroat", "Vampire Interloper"} {
			if name == "Fathom Fleet Cutthroat" && !withPirate {
				continue
			}
			materials = append(materials, g.NewCard(corpusCard(t, name), p, engine.Graveyard))
		}
		c := engine.NewScriptedController()
		g.Player(p).ManaPool.Add(mana.Black, 4)
		for range 4 {
			c.QueuePayGeneric(mana.ShardB)
		}
		for _, m := range materials {
			c.QueueCardChoice([]engine.CardID{m})
		}
		ok := g.ActivateAbility(p, throne, craftIndex(g, throne), c)
		if ok != withPirate {
			t.Fatalf("with a Pirate %v: Craft activated %v (%v)", withPirate, ok, g.TakePendingError())
		}
		if !ok {
			continue
		}
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatal(err)
		}
		if name := g.Card(throne).Name(); name != "The Grim Captain" || g.Card(throne).Zone != engine.Battlefield {
			t.Errorf("crafted card %q in %v, want The Grim Captain on the battlefield", name, g.Card(throne).Zone)
		}
		for _, m := range materials {
			if z := g.Card(m).Zone; z != engine.Exile {
				t.Errorf("material %q in %v, want Exile", g.Card(m).Name(), z)
			}
		}
	}
}

// The XMin of a Craft cost is a floor on the announced X
// (AbilityUtils.getAnnouncementBounds): Paleontologist's Pick-Axe will not
// craft with X = 0, which would exile no creature.
func TestCraftRefusesAnXBelowItsMinimum(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	axe := g.NewCard(corpusCard(t, "Paleontologist's Pick-Axe"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Red, 6)
	for range 6 {
		c.QueuePayGeneric(mana.ShardR)
	}
	c.QueuePayX(0)
	if g.ActivateAbility(p, axe, craftIndex(g, axe), c) {
		t.Fatal("Craft activated with X = 0 under XMin1")
	}
	_ = g.TakePendingError()
	if z := g.Card(axe).Zone; z != engine.Battlefield {
		t.Errorf("Pick-Axe in %v after the refusal, want Battlefield", z)
	}
}
