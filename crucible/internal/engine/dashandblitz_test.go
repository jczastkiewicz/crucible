package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Dash (CR 702.109): Zurgo Bellstriker cast for {1}{R} has haste and returns
// to its owner's hand at the beginning of the next end step; cast for its mana
// cost it has neither.
func TestDashedCreatureHasHasteAndReturnsAtEndStep(t *testing.T) {
	t.Parallel()
	for _, dash := range []bool{true, false} {
		g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
		libraryCards(t, g, p, 4)
		libraryCards(t, g, other, 4)
		g.SetTurnState(1, p, engine.Main1)
		zurgo := g.NewCard(corpusCard(t, "Zurgo Bellstriker"), p, engine.Hand)
		c := engine.NewScriptedController()
		if dash {
			g.Player(p).ManaPool.Add(mana.Red, 2)
			c.QueueOption(1)
			c.QueuePayGeneric(mana.ShardR)
		} else {
			g.Player(p).ManaPool.Add(mana.Red, 1)
			c.QueueOption(0)
		}
		castThenResolve(t, g, p, zurgo, c)
		engine.CheckStateBasedActions(g, c)
		if got := g.Card(zurgo).HasKeyword("Haste"); got != dash {
			t.Errorf("dash=%v: haste %v", dash, got)
		}
		toStep(t, g, c, p, engine.EndOfTurn, nil)
		want := engine.Battlefield
		if dash {
			want = engine.Hand
		}
		if got := g.Card(zurgo).Zone; got != want {
			t.Errorf("dash=%v: after the end step Zurgo is in %v, want %v", dash, got, want)
		}
	}
}

// Blitz (CR 702.152): Mayhem Patrol cast for {1}{R} has haste, is sacrificed
// at the beginning of the next end step, and its blitz trigger draws a card as
// it dies.
func TestBlitzedCreatureIsSacrificedAndDraws(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Main1)
	patrol := g.NewCard(corpusCard(t, "Mayhem Patrol"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueuePayGeneric(mana.ShardR)
	castThenResolve(t, g, p, patrol, c)
	engine.CheckStateBasedActions(g, c)
	if !g.Card(patrol).HasKeyword("Haste") {
		t.Error("the blitzed Mayhem Patrol has no haste")
	}
	hand := len(g.Zone(engine.Hand, p).Cards())
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if got := g.Card(patrol).Zone; got != engine.Graveyard {
		t.Fatalf("after the end step Mayhem Patrol is in %v, want the graveyard", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != hand+1 {
		t.Errorf("hand %d, want %d: the blitz trigger draws a card", got, hand+1)
	}
}

// Warbringer's "Dash costs you pay cost {2} less" reads ValidSpell$ Spell.Dash:
// Zurgo's dash {1}{R} costs {R}.
func TestDashCostReductionReadsTheDashCast(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Warbringer"), p, engine.Battlefield)
	zurgo := g.NewCard(corpusCard(t, "Zurgo Bellstriker"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	if !g.CastSpell(p, zurgo, c) {
		t.Fatalf("Zurgo not cast for its reduced dash cost: %v", g.TakePendingError())
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("%d mana left, want 0", got)
	}
}

// The dash cast stays with the permanent it became, not with a new object:
// Zurgo dashed, exiled and returned has no haste and no end-step return.
func TestFlickeredDashCreatureIsANewObject(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Main1)
	zurgo := g.NewCard(corpusCard(t, "Zurgo Bellstriker"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueuePayGeneric(mana.ShardR)
	castThenResolve(t, g, p, zurgo, c)
	g.Move(zurgo, engine.Exile, p)
	g.Move(zurgo, engine.Battlefield, p)
	engine.CheckStateBasedActions(g, c)
	if g.Card(zurgo).HasKeyword("Haste") {
		t.Error("the returned Zurgo has haste")
	}
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if got := g.Card(zurgo).Zone; got != engine.Battlefield {
		t.Errorf("after the end step the returned Zurgo is in %v, want the battlefield", got)
	}
}

// A MayPlay$ grant with ValidSA$ Spell.Blitz allows only the blitz cast:
// Tenacious Underdog is cast from its graveyard for {2}{B}{B} and 2 life, has
// haste, and is sacrificed at the end step. No choice is asked: the plain cast
// is not offered from there.
func TestTenaciousUnderdogIsBlitzedFromTheGraveyard(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 4)
	libraryCards(t, g, other, 4)
	g.SetTurnState(1, p, engine.Main1)
	dog := g.NewCard(corpusCard(t, "Tenacious Underdog"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	engine.CheckStateBasedActions(g, c)
	g.Player(p).ManaPool.Add(mana.Black, 4)
	c.QueuePayGeneric(mana.ShardB)
	c.QueuePayGeneric(mana.ShardB)
	life := g.Player(p).Life
	castThenResolve(t, g, p, dog, c)
	if got := g.Card(dog).Zone; got != engine.Battlefield {
		t.Fatalf("Tenacious Underdog in %v, want the battlefield", got)
	}
	if got := life - g.Player(p).Life; got != 2 {
		t.Errorf("paid %d life, want 2", got)
	}
	engine.CheckStateBasedActions(g, c)
	if !g.Card(dog).HasKeyword("Haste") {
		t.Error("the blitzed Tenacious Underdog has no haste")
	}
	toStep(t, g, c, p, engine.EndOfTurn, nil)
	if got := g.Card(dog).Zone; got != engine.Graveyard {
		t.Errorf("after the end step Tenacious Underdog is in %v, want the graveyard", got)
	}
}
