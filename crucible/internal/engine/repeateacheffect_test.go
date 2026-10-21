package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestRepeatEachEffectEachPlayer proves RepeatPlayers$: the sub-ability
// resolves once per player with that player as the Remembered one.
func TestRepeatEachEffectEachPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test RepeatEach Players", "DB$ RepeatEach | RepeatPlayers$ Player | RepeatSubAbility$ DBLose",
		"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 2")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 18 || g.Player(other).Life != 18 {
		t.Errorf("life = %d/%d, want 18/18", g.Player(p).Life, g.Player(other).Life)
	}
	if n := len(g.Card(host).Memory.Remembered()); n != 0 {
		t.Errorf("remembered after = %d, want 0", n)
	}
}

// TestRepeatEachEffectEachCard proves RepeatCards$: once per matching card,
// that card Remembered while its sub-ability resolves.
func TestRepeatEachEffectEachCard(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test RepeatEach Cards", "DB$ RepeatEach | RepeatCards$ Creature.OppCtrl | RepeatSubAbility$ DBTap",
		"DBTap", "DB$ Tap | Defined$ Remembered")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(a).Tapped || !g.Card(b).Tapped {
		t.Error("opponent creatures not both tapped")
	}
}

// TestRepeatEachEffectUseImprintedAndOptional proves UseImprinted$ swaps
// the Imprinted list instead, and RepeatOptionalForEachPlayer$ lets a
// player decline.
func TestRepeatEachEffectUseImprintedAndOptional(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	c.QueueConfirmEffect(true)
	def := etbChainDef(t, "Test RepeatEach Imprint",
		"DB$ RepeatEach | DefinedCards$ Targeted | ValidTgts$ Creature.OppCtrl | UseImprinted$ True | RepeatSubAbility$ DBTap | SubAbility$ DBPlayers",
		"DBTap", "DB$ Tap | Defined$ Imprinted",
		"DBPlayers", "DB$ RepeatEach | RepeatPlayers$ Player | RepeatOptionalForEachPlayer$ True | RepeatSubAbility$ DBLose",
		"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 1")
	c.QueueTargets([]engine.EntityID{engine.CardEntity(a)})
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !g.Card(a).Tapped {
		t.Error("imprinted creature not tapped")
	}
	if n := len(g.Card(host).Memory.Imprinted()); n != 0 {
		t.Errorf("imprinted after = %d, want 0", n)
	}
	if g.Player(p).Life != 20 || g.Player(other).Life != 19 {
		t.Errorf("life = %d/%d, want 20/19 (first player declined)", g.Player(p).Life, g.Player(other).Life)
	}
}

// TestRepeatEachEffectStartingWithOrdersPlayers proves StartingWith$: the
// first player asked is the one it names (the first answer declines, the
// second accepts, so the life loss lands on whoever was asked second).
func TestRepeatEachEffectStartingWithOrdersPlayers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		start        string
		pLife, oLife int
	}{{"You", 20, 19}, {"Opponent", 19, 20}} {
		g, p, other := newTwoPlayerGame(t)
		c := engine.NewScriptedController()
		c.QueueConfirmEffect(false)
		c.QueueConfirmEffect(true)
		def := etbChainDef(t, "Test RepeatEach Start", "DB$ RepeatEach | RepeatPlayers$ Player | StartingWith$ "+tc.start+" | RepeatOptionalForEachPlayer$ True | RepeatSubAbility$ DBLose",
			"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 1")
		if _, err := castETBChain(t, g, p, def, c); err != nil {
			t.Fatalf("StartingWith$ %s: %v", tc.start, err)
		}
		if g.Player(p).Life != tc.pLife || g.Player(other).Life != tc.oLife {
			t.Errorf("StartingWith$ %s: life = %d/%d, want %d/%d", tc.start, g.Player(p).Life, g.Player(other).Life, tc.pLife, tc.oLife)
		}
	}
}

// TestRepeatEachEffectRejectsUnresolvedParam proves ChooseOrder$ fails closed.
func TestRepeatEachEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test RepeatEach Bad", "DB$ RepeatEach | RepeatPlayers$ Player | ChooseOrder$ True | RepeatSubAbility$ DBLose",
		"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 1")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved ChooseOrder$")
	}
}
