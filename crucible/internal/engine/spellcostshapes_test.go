package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// costShapeGame is a main-phase game whose active player has 10 of each
// color floating and a library to draw from.
func costShapeGame(t *testing.T) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	libraryCards(t, g, p, 5)
	for _, col := range []mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green} {
		g.Player(p).ManaPool.Add(col, 10)
	}
	return g, p, other
}

func queueGeneric(c *engine.ScriptedController, shard mana.Shard, n int) {
	for range n {
		c.QueuePayGeneric(shard)
	}
}

// Cost$ XMin1 X B (Welcome the Darkness, CostPartMana.getXMin): X can't be 0.
func TestXMinRefusesAZeroX(t *testing.T) {
	t.Parallel()
	for _, x := range []int{0, 2} {
		g, p, _ := costShapeGame(t)
		spell := g.NewCard(corpusCard(t, "Welcome the Darkness"), p, engine.Hand)
		c := engine.NewScriptedController()
		c.QueuePayX(x)
		queueGeneric(c, mana.ShardB, x)
		cast := g.CastSpell(p, spell, c)
		err := g.TakePendingError()
		if x == 0 && (cast || err == nil || !strings.Contains(err.Error(), "XMin")) {
			t.Errorf("X=0: cast %v err %v, want a refusal naming XMin", cast, err)
		}
		if x == 2 && !cast {
			t.Errorf("X=2: cast failed: %v", err)
		}
	}
}

// Torgaar, Famine Incarnate (Sac<X/Creature>, ReduceCost Y = X*2): X is
// announced before the cost is read, two creatures are sacrificed, and the
// spell costs {4} less.
func TestAnnouncedXSacrificeReducesTheCost(t *testing.T) {
	t.Parallel()
	g, p, _ := costShapeGame(t)
	bears := []engine.CardID{
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield),
		g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield),
	}
	torgaar := g.NewCard(corpusCard(t, "Torgaar, Famine Incarnate"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueuePayX(2)
	c.QueueSacrificeChoice(bears)
	queueGeneric(c, mana.ShardW, 2)
	if !g.CastSpell(p, torgaar, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	for _, id := range bears {
		if g.Card(id).Zone != engine.Graveyard {
			t.Errorf("a Bear is in %v, want sacrificed", g.Card(id).Zone)
		}
	}
	if got := 50 - g.Player(p).ManaPool.Total(); got != 4 {
		t.Errorf("paid %d mana, want 4 ({6}{B}{B} less {4})", got)
	}
}

// Soulblast (Sac<All/Creature>): every creature its caster controls is
// sacrificed with no choice, and X is their total power.
func TestSacrificeAllPaysWithEveryCreature(t *testing.T) {
	t.Parallel()
	g, p, other := costShapeGame(t)
	g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	blast := g.NewCard(corpusCard(t, "Soulblast"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	queueGeneric(c, mana.ShardR, 3)
	if !g.CastSpell(p, blast, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if got := len(g.Zone(engine.Battlefield, p).Cards()); got != 0 {
		t.Errorf("%d permanents left, want every creature sacrificed", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Player(other).Life; got != 15 {
		t.Errorf("opponent's life %d, want 15 (2 + 3 power)", got)
	}
}

// Phyrexian Purge (PayLife<X>, X = 3 per target): the X past the mana is read
// from the targets just chosen.
func TestTargetDerivedXIsReadAfterTargets(t *testing.T) {
	t.Parallel()
	g, p, other := costShapeGame(t)
	a := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	b := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	purge := g.NewCard(corpusCard(t, "Phyrexian Purge"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a), engine.CardEntity(b)})
	queueGeneric(c, mana.ShardB, 2)
	if !g.CastSpell(p, purge, c) {
		t.Fatalf("cast failed: %v", g.TakePendingError())
	}
	if got := g.Player(p).Life; got != 14 {
		t.Errorf("life %d, want 14 (3 for each of two targets)", got)
	}
}

// Ertai's Meddling (RememberSpell$, CopySpellAbility UseOriginalHost$): the
// targeted spell is exiled with X delay counters; at its controller's upkeep a
// counter comes off, and with none left a copy of the original spell goes on
// the stack, hosted by the exiled card, which stays in exile.
func TestErtaisMeddlingDelaysASpell(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, other, engine.Main1)
	c := engine.NewScriptedController()
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), other, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Red, 1)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
	if !g.CastSpell(other, bolt, c) {
		t.Fatalf("Bolt: %v", g.TakePendingError())
	}
	meddling := g.NewCard(corpusCard(t, "Ertai's Meddling"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c.QueuePayX(1)
	c.QueueTargets([]engine.EntityID{engine.CardEntity(bolt)})
	c.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, meddling, c) {
		t.Fatalf("Ertai's Meddling: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if b := g.Card(bolt); b.Zone != engine.Exile || b.Counters.Count("DELAY") != 1 {
		t.Fatalf("Bolt in %v with %d delay counters, want exiled with 1", b.Zone, b.Counters.Count("DELAY"))
	}
	if got := g.Player(p).Life; got != 20 {
		t.Fatalf("life %d before the delay ends, want 20", got)
	}

	g.SetTurnState(3, other, engine.Untap)
	g.AdvancePhase(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Player(p).Life; got != 17 {
		t.Errorf("life %d after the delayed copy resolved, want 17", got)
	}
	if got := g.Card(bolt).Zone; got != engine.Exile {
		t.Errorf("the original Bolt is in %v, want still exiled", got)
	}
	g.SetTurnState(5, other, engine.Untap)
	g.AdvancePhase(c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	// Forge's script keeps the card exiled (UseOriginalHost$), so the copy
	// comes back at every later upkeep: reproduced as Java does it, logged in
	// card-script-defects.md (PORT-8).
	if got := g.Player(p).Life; got != 14 {
		t.Errorf("life %d after a later upkeep, want 14 (the reproduced repeat)", got)
	}
}
