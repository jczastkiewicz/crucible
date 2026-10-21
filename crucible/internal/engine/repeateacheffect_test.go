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

// TestRepeatEachEffectRejectsUnresolvedParam proves LoseLifeMap$ fails closed.
func TestRepeatEachEffectRejectsUnresolvedParam(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test RepeatEach Bad", "DB$ RepeatEach | RepeatPlayers$ Player | LoseLifeMap$ True | RepeatSubAbility$ DBLose",
		"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 1")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err == nil {
		t.Fatal("ResolveStack succeeded, want an error for unresolved LoseLifeMap$")
	}
}

// TestRepeatEachEffectDamageMapDealsOnceAfterTheLoop proves DamageMap$: each
// iteration's DealDamage records into the loop's table and the RepeatEach
// itself deals the whole batch afterwards (RepeatEachEffect.java:199-201).
func TestRepeatEachEffectDamageMapDealsOnceAfterTheLoop(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	def := etbChainDef(t, "Test RepeatEach Map", "DB$ RepeatEach | RepeatPlayers$ Player | DamageMap$ True | RepeatSubAbility$ DBDmg",
		"DBDmg", "DB$ DealDamage | Defined$ Remembered | NumDmg$ 2")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if g.Player(p).Life != 18 || g.Player(other).Life != 18 {
		t.Errorf("life = %d/%d, want 18/18", g.Player(p).Life, g.Player(other).Life)
	}
}

// TestRepeatEachEffectChooseOrderLetsTheChooserOrderTheCards proves ChooseOrder$
// True (RepeatEachEffect.java:92-96): the activator's order, not the zone's, is
// the order the sub-ability sees the cards in (the last card sent to the
// library ends on top); an answer that is not a permutation of the cards is an
// error.
func TestRepeatEachEffectChooseOrderLetsTheChooserOrderTheCards(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		answer  func(a, b engine.CardID) []engine.CardID
		top     func(a, b engine.CardID) engine.CardID
		wantErr bool
	}{
		{"reversed", func(a, b engine.CardID) []engine.CardID { return []engine.CardID{b, a} }, func(a, _ engine.CardID) engine.CardID { return a }, false},
		{"as listed", func(a, b engine.CardID) []engine.CardID { return []engine.CardID{a, b} }, func(_, b engine.CardID) engine.CardID { return b }, false},
		{"not a permutation", func(a, _ engine.CardID) []engine.CardID { return []engine.CardID{a, a} }, nil, true},
	} {
		g, p, _ := newTwoPlayerGame(t)
		first := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
		second := g.NewCard(creatureDefPT(t, "1", "2"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueCardOrder(tc.answer(first, second))
		def := etbChainDef(t, "Test RepeatEach Order",
			"DB$ RepeatEach | RepeatCards$ Creature.Other+YouCtrl | ChooseOrder$ True | RepeatSubAbility$ DBBack",
			"DBBack", "DB$ ChangeZone | Defined$ Remembered | Origin$ Battlefield | Destination$ Library")
		_, err := castETBChain(t, g, p, def, c)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: ResolveStack error = %v, want error %v", tc.name, err, tc.wantErr)
			continue
		}
		if tc.wantErr {
			continue
		}
		if lib := g.Zone(engine.Library, p).Cards(); len(lib) != 2 || lib[0] != tc.top(first, second) {
			t.Errorf("%s: library = %v, want %d on top", tc.name, lib, tc.top(first, second))
		}
	}
}
