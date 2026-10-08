package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover a PutCounter with ETB$ inside a Copy-layer replacement's
// chain (Altered Ego, Undercover Operative, Dominion Saboteur, The
// Mimeoplasm): counters placed as part of the entry, which wait on the card
// until it is on the battlefield and then go in with the entry's other
// counters.

func counterCount(g *engine.Game, id engine.CardID, ct engine.CounterType) int {
	return g.Card(id).Counters.Count(ct)
}

// TestEntersAsCopyWithAdditionalCounters proves Altered Ego's shape: the
// copy enters with the counters its chain put into the entry, under the
// copied values; declining the copy declines the counters with it; and an
// AddCounter replacement (Hardened Scales) changes the whole pile once.
func TestEntersAsCopyWithAdditionalCounters(t *testing.T) {
	t.Parallel()

	ego := func(t *testing.T) *compile.Card {
		t.Helper()
		return testCloneDef(t, "Test Ego", "Creature.Other | SubAbility$ DBAddCounter",
			"SVar:DBAddCounter:DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | ETB$ True | CounterNum$ 2")
	}

	t.Run("a copy enters with the counters", func(t *testing.T) {
		t.Parallel()
		g, p, other := newTwoPlayerGame(t)
		giant := g.NewCard(giantDef(t), other, engine.Battlefield)
		card := g.NewCard(ego(t), p, engine.Hand)
		sc := engine.NewScriptedController()
		sc.QueueConfirmEffect(true)
		sc.QueueCardChoice([]engine.CardID{giant})
		mustCastAndResolve(t, g, p, sc, card)
		if got := counterCount(g, card, engine.P1P1); got != 2 {
			t.Errorf("+1/+1 counters = %d, want 2", got)
		}
		wantPT(t, g, card, 6, 7)
	})

	t.Run("declining the copy declines the counters", func(t *testing.T) {
		t.Parallel()
		g, p, other := newTwoPlayerGame(t)
		g.NewCard(giantDef(t), other, engine.Battlefield)
		card := g.NewCard(ego(t), p, engine.Hand)
		sc := engine.NewScriptedController()
		sc.QueueConfirmEffect(false)
		mustCastAndResolve(t, g, p, sc, card)
		if got := counterCount(g, card, engine.P1P1); got != 0 {
			t.Errorf("+1/+1 counters = %d, want 0", got)
		}
	})

	t.Run("an AddCounter replacement sees the pile once", func(t *testing.T) {
		t.Parallel()
		g, p, other := newTwoPlayerGame(t)
		giant := g.NewCard(giantDef(t), other, engine.Battlefield)
		g.NewCard(copyTestDef(t, "Test Scales", "Creature Elf", "1", "1",
			"R:Event$ AddCounter | ActiveZones$ Battlefield | ValidCard$ Creature.YouCtrl+inZoneBattlefield | ValidCounterType$ P1P1 | ReplaceWith$ AddOneMoreCounters | Description$ x",
			"SVar:AddOneMoreCounters:DB$ ReplaceCounter | ValidCounterType$ P1P1 | ChooseCounter$ True | Amount$ X",
			"SVar:X:ReplaceCount$CounterNum/Plus.1"), p, engine.Battlefield)
		card := g.NewCard(ego(t), p, engine.Hand)
		sc := engine.NewScriptedController()
		sc.QueueConfirmEffect(true)
		sc.QueueCardChoice([]engine.CardID{giant})
		mustCastAndResolve(t, g, p, sc, card)
		if got := counterCount(g, card, engine.P1P1); got != 3 {
			t.Errorf("+1/+1 counters = %d, want 3 (two, plus one)", got)
		}
	})

	t.Run("counters stay off a card whose entry was replaced", func(t *testing.T) {
		t.Parallel()
		g, p, other := newTwoPlayerGame(t)
		g.NewCard(priestDef(t), p, engine.Battlefield)
		giant := g.NewCard(giantDef(t), other, engine.Battlefield)
		host := reanimator(t, g, p)
		card := g.NewCard(ego(t), p, engine.Graveyard)
		sc := engine.NewScriptedController()
		sc.QueueConfirmEffect(true)
		sc.QueueCardChoice([]engine.CardID{giant})
		mustActivate(t, g, p, sc, host, card)
		if z := g.Card(card).Zone; z != engine.Exile {
			t.Fatalf("card is in %v, want Exile", z)
		}
		if got := counterCount(g, card, engine.P1P1); got != 0 {
			t.Errorf("exiled card holds %d +1/+1 counters, want 0", got)
		}
	})
}

// TestEntersAsCopyConditionalShieldCounter proves Undercover Operative:
// RememberCloneOrigin$ and a ConditionDefined$ Remembered PutCounter give the
// copy a shield counter only if you control the copied creature.
func TestEntersAsCopyConditionalShieldCounter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mine bool
		want int
	}{
		{"your creature", true, 1},
		{"an opponent's creature", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			owner := other
			if tc.mine {
				owner = p
			}
			giant := g.NewCard(giantDef(t), owner, engine.Battlefield)
			card := g.NewCard(testCloneDef(t, "Test Operative", "Creature.Other | SubAbility$ DBAddCounter | RememberCloneOrigin$ True",
				"SVar:DBAddCounter:DB$ PutCounter | Defined$ Self | CounterType$ SHIELD | ETB$ True | ConditionDefined$ Remembered | ConditionPresent$ Creature.YouCtrl | SubAbility$ DBCleanup",
				"SVar:DBCleanup:DB$ Cleanup | ClearRemembered$ True"), p, engine.Hand)
			sc := engine.NewScriptedController()
			sc.QueueConfirmEffect(true)
			sc.QueueCardChoice([]engine.CardID{giant})
			mustCastAndResolve(t, g, p, sc, card)
			if got := counterCount(g, card, engine.Shield); got != tc.want {
				t.Errorf("shield counters = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestEntersAsCopyWithTheSameCountersAsTheCopied proves Dominion Saboteur's
// EachFromSource$: one pile per kind the copied permanent has, at its count,
// and CounterNum$ overrides the count.
func TestEntersAsCopyWithTheSameCountersAsTheCopied(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		extra string
		p1p1  int
		shld  int
	}{
		{"every counter", "", 2, 1},
		{"CounterNum$ overrides the count", " | CounterNum$ 1", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			giant := g.NewCard(giantDef(t), other, engine.Battlefield)
			g.Card(giant).Counters.Add(engine.P1P1, 2)
			g.Card(giant).Counters.Add(engine.Shield, 1)
			card := g.NewCard(testCloneDef(t, "Test Saboteur", "Creature.Other | SubAbility$ DBAddCounter | RememberCloneOrigin$ True",
				"SVar:DBAddCounter:DB$ PutCounter | Defined$ Self | ETB$ True | CounterType$ EachFromSource | EachFromSource$ Remembered"+tc.extra+" | SubAbility$ DBCleanup",
				"SVar:DBCleanup:DB$ Cleanup | ClearRemembered$ True"), p, engine.Hand)
			sc := engine.NewScriptedController()
			sc.QueueConfirmEffect(true)
			sc.QueueCardChoice([]engine.CardID{giant})
			mustCastAndResolve(t, g, p, sc, card)
			if a, b := counterCount(g, card, engine.P1P1), counterCount(g, card, engine.Shield); a != tc.p1p1 || b != tc.shld {
				t.Errorf("counters P1P1 %d shield %d, want %d and %d", a, b, tc.p1p1, tc.shld)
			}
		})
	}
}

// TestEntersAsCopyETBCounterFailsClosed proves the ETB$ PutCounter shapes the
// entry cannot place are errors before the copy applies, not counters on
// another card or silently none.
func TestEntersAsCopyETBCounterFailsClosed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{"a Defined$ that is not the entering card", "Defined$ Targeted | CounterType$ P1P1 | ETB$ True", `Defined$ "Targeted" not resolvable yet`},
		{"a CounterNum$ that does not resolve", "Defined$ Self | CounterType$ P1P1 | ETB$ True | CounterNum$ Nope", "CounterNum$"},
		{"a counter kind list", "Defined$ Self | CounterType$ P1P1,M1M1 | ETB$ True", "CounterType$"},
		{"an EachFromSource$ Defined$ that does not resolve", "Defined$ Self | CounterType$ P1P1 | ETB$ True | EachFromSource$ Nope", "Defined$"},
		{"an unported param", "Defined$ Self | CounterType$ P1P1 | ETB$ True | Optional$ True", "Optional$ not resolvable yet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGame(t)
			giant := g.NewCard(giantDef(t), other, engine.Battlefield)
			card := g.NewCard(testCloneDef(t, "Test Ego", "Creature.Other | SubAbility$ DBAddCounter",
				"SVar:DBAddCounter:DB$ PutCounter | "+tc.line), p, engine.Hand)
			sc := engine.NewScriptedController()
			sc.QueueConfirmEffect(true)
			sc.QueueCardChoice([]engine.CardID{giant})
			err := castAndResolve(t, g, p, sc, card)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestEntersAsCopyPutCounterETBOutsideAnEntryStillRefuses proves the ETB$
// counter table is only what an entry's chain has: any other PutCounter
// naming it is the unported counter-table shape.
func TestEntersAsCopyPutCounterETBOutsideAnEntryStillRefuses(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host := g.NewCard(copyTestDef(t, "Test Putter", "Creature Elf", "1", "1",
		"A:AB$ PutCounter | Cost$ 0 | Defined$ Self | CounterType$ P1P1 | ETB$ True"), p, engine.Battlefield)
	err := activate(g, p, engine.NewScriptedController(), host)
	if err == nil || !strings.Contains(err.Error(), "ETB$ not resolvable yet") {
		t.Fatalf("err = %v, want an ETB$ refusal", err)
	}
}
