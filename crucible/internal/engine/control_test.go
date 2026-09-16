package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

func TestScriptedControllerAnswersInOrder(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p0, p1 := g.Players()[0], g.Players()[1]

	c := engine.NewScriptedController()
	c.QueueStartingPlayer(p1)
	c.QueueStartingPlayer(p0)
	c.QueueKeepHand(true)
	c.QueueKeepHand(false)

	if got := c.ChooseStartingPlayer(g, p0, true); got != p1 {
		t.Errorf("first starting player %v, want %v", got, p1)
	}
	if got := c.ChooseStartingPlayer(g, p0, false); got != p0 {
		t.Errorf("second starting player %v, want %v", got, p0)
	}
	if got := c.MulliganKeepHand(g, p0, p1, 0); got != true {
		t.Error("first keep-hand answer was false, want true")
	}
	if got := c.MulliganKeepHand(g, p1, p1, 1); got != false {
		t.Error("second keep-hand answer was true, want false")
	}
}

func TestScriptedControllerTuckAndStartingHand(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p0 := g.Players()[0]
	hand := []engine.CardID{
		g.NewCard(nil, p0, engine.Hand),
		g.NewCard(nil, p0, engine.Hand),
		g.NewCard(nil, p0, engine.Hand),
	}

	c := engine.NewScriptedController()
	tucked := hand[:1]
	c.QueueTuck(tucked)
	c.QueueStartingHand(1)

	got := c.TuckCardsViaMulligan(g, p0, hand, 1)
	if len(got) != 1 || got[0] != hand[0] {
		t.Errorf("tucked %v, want %v", got, tucked)
	}

	candidates := [][]engine.CardID{hand[:2], hand[1:]}
	if got := c.ChooseStartingHand(g, p0, candidates); got != 1 {
		t.Errorf("starting hand index %d, want 1", got)
	}
}

// A queue running dry mid-scenario is a fixture-authoring mistake, not a
// rules question a card script could cause -- it has to fail loud rather than
// silently answer "keep" or "player 0" and pass the fixture for the wrong
// reason (GO-7).
func TestScriptedControllerPanicsWhenExhausted(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p0 := g.Players()[0]
	c := engine.NewScriptedController()

	defer func() {
		if recover() == nil {
			t.Error("an empty queue did not panic")
		}
	}()
	c.ChooseStartingPlayer(g, p0, true)
}

// Every decision kind panics on its own exhausted queue, not just the first
// one tested above.
func TestScriptedControllerEachQueuePanicsWhenExhausted(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p0 := g.Players()[0]

	calls := map[string]func(*engine.ScriptedController){
		"starting hand":  func(c *engine.ScriptedController) { c.ChooseStartingHand(g, p0, nil) },
		"keep hand":      func(c *engine.ScriptedController) { c.MulliganKeepHand(g, p0, p0, 0) },
		"tuck":           func(c *engine.ScriptedController) { c.TuckCardsViaMulligan(g, p0, nil, 0) },
		"legendary keep": func(c *engine.ScriptedController) { c.ChooseLegendaryToKeep(g, p0, nil) },
		"attackers":      func(c *engine.ScriptedController) { c.DeclareCombatAttackers(g, p0, nil) },
		"attack target":  func(c *engine.ScriptedController) { c.ChooseAttackTarget(g, p0, 0, nil) },
		"blocks":         func(c *engine.ScriptedController) { c.DeclareCombatBlockers(g, p0, nil, nil) },
		"damage":         func(c *engine.ScriptedController) { c.AssignCombatDamage(g, p0, 0, nil) },
		"discard":        func(c *engine.ScriptedController) { c.DiscardToHandSize(g, p0, nil, 0) },
		"battle protector": func(c *engine.ScriptedController) {
			c.ChooseBattleProtector(g, p0, 0, nil)
		},
		"hybrid mana color": func(c *engine.ScriptedController) {
			c.ChooseHybridManaColor(g, p0, 0)
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: an empty queue did not panic", name)
				}
			}()
			call(engine.NewScriptedController())
		})
	}
}

// Each decision kind has its own queue: exhausting one must not let a call to
// a different method read from it.
func TestScriptedControllerQueuesAreIndependent(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p0 := g.Players()[0]
	c := engine.NewScriptedController()
	c.QueueKeepHand(true)

	if got := c.MulliganKeepHand(g, p0, p0, 0); !got {
		t.Fatal("keep-hand queue did not answer")
	}

	defer func() {
		if recover() == nil {
			t.Error("ChooseStartingPlayer answered from an unrelated queue")
		}
	}()
	c.ChooseStartingPlayer(g, p0, true)
}

// Both interface implementations line up at compile time -- the point of the
// interface existing before AI or scripted play consume it.
var _ engine.PlayerController = (*engine.ScriptedController)(nil)
