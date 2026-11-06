package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Warp (CR 702.185): Germinating Wurm cast for {1}{G} enters, makes the turn a
// Void turn (Game.isVoid), is exiled at the beginning of the next end step,
// and may be cast from exile on a later turn for its mana cost.
func TestWarpedCreatureIsExiledAndCastLater(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 6)
	libraryCards(t, g, other, 6)
	g.SetTurnState(1, p, engine.Main1)
	wurm := g.NewCard(corpusCard(t, "Germinating Wurm"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueuePayGeneric(mana.ShardG)
	life := g.Player(p).Life
	castThenResolve(t, g, p, wurm, c)
	if got := g.Card(wurm).Zone; got != engine.Battlefield {
		t.Fatalf("Germinating Wurm in %v, want the battlefield", got)
	}
	if got := g.Player(p).Life - life; got != 2 {
		t.Errorf("gained %d life, want 2 from the Wurm entering", got)
	}

	// Count$Void.5.0 is 5 with nothing having left the battlefield: the warp
	// cast alone makes the turn a Void turn.
	life = g.Player(p).Life
	resolveLine(t, g, p, c, "DB$ GainLife | Defined$ You | LifeAmount$ X", "X", "Count$Void.5.0")
	if got := g.Player(p).Life - life; got != 5 {
		t.Errorf("Void gain %d, want 5 after a warp cast", got)
	}

	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if got := g.Card(wurm).Zone; got != engine.Exile {
		t.Fatalf("after the end step the Wurm is in %v, want exile", got)
	}

	toStep(t, g, c, p, engine.Main1, nil)
	g.Player(p).ManaPool.Add(mana.Green, 5)
	for range 4 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, wurm, c) {
		t.Fatalf("Germinating Wurm not cast from exile on a later turn: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(wurm).Zone; got != engine.Battlefield {
		t.Errorf("Germinating Wurm in %v, want the battlefield again", got)
	}
}

// Cantor of the Refrain's "When you cast a spell for its warp cost" reads
// ValidSA$ Spell.Warp: it returns from the graveyard; a normal cast leaves it.
func TestCantorReturnsOnAWarpCast(t *testing.T) {
	t.Parallel()
	for _, warp := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		cantor := g.NewCard(corpusCard(t, "Cantor of the Refrain"), p, engine.Graveyard)
		wurm := g.NewCard(corpusCard(t, "Germinating Wurm"), p, engine.Hand)
		c := engine.NewScriptedController()
		if warp {
			g.Player(p).ManaPool.Add(mana.Green, 2)
			c.QueueOption(1)
			c.QueuePayGeneric(mana.ShardG)
		} else {
			g.Player(p).ManaPool.Add(mana.Green, 5)
			c.QueueOption(0)
			for range 4 {
				c.QueuePayGeneric(mana.ShardG)
			}
		}
		castThenResolve(t, g, p, wurm, c)
		want := engine.Graveyard
		if warp {
			want = engine.Battlefield
		}
		if got := g.Card(cantor).Zone; got != want {
			t.Errorf("warp=%v: Cantor of the Refrain in %v, want %v", warp, got, want)
		}
	}
}

// A SpellCast trigger fires from the graveyard when its TriggerZones$ names
// it: Vengevine returns as its controller casts a second creature spell in a
// turn, not on the first.
func TestVengevineReturnsOnTheSecondCreatureSpell(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	vengevine := g.NewCard(corpusCard(t, "Vengevine"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	castThenResolve(t, g, p, g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Hand), c)
	if got := g.Card(vengevine).Zone; got != engine.Graveyard {
		t.Fatalf("after one creature spell Vengevine is in %v, want the graveyard", got)
	}
	c.QueueConfirmOptionalTrigger(true)
	castThenResolve(t, g, p, g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Hand), c)
	if got := g.Card(vengevine).Zone; got != engine.Battlefield {
		t.Errorf("after the second creature spell Vengevine is in %v, want the battlefield", got)
	}
}

// Timeline Culler's own ValidSA$ Spell.Warp grant casts it from the graveyard
// for its warp cost, {B} and 2 life (GameActionUtil.java:352: the grant's
// host is the spell's).
func TestTimelineCullerIsWarpedFromTheGraveyard(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	culler := g.NewCard(corpusCard(t, "Timeline Culler"), p, engine.Graveyard)
	g.Player(p).ManaPool.Add(mana.Black, 1)
	c := engine.NewScriptedController()
	life := g.Player(p).Life
	engine.CheckStateBasedActions(g, c)
	castThenResolve(t, g, p, culler, c)
	if got := g.Card(culler).Zone; got != engine.Battlefield {
		t.Fatalf("Timeline Culler in %v, want the battlefield", got)
	}
	if got := life - g.Player(p).Life; got != 2 {
		t.Errorf("paid %d life, want 2", got)
	}
}
