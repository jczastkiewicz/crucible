package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Wild Growth's own trigger and Execute$ lines, verbatim from
// forge-gui/res/cardsfolder/w/wild_growth.txt: a Static$ True TapsForMana
// trigger, CR 605.1b's triggered mana ability (ADR-0020).
const (
	wildGrowthTrigger = "Mode$ TapsForMana | ValidCard$ Card.AttachedBy | Execute$ TrigMana | Static$ True | " +
		"TriggerDescription$ Whenever enchanted land is tapped for mana, its controller adds an additional {G}."
	wildGrowthTrigMana = "DB$ Mana | Produced$ G | Amount$ 1 | Defined$ TriggeredCardController"
)

// auraDefWithTrigger compiles an Aura carrying one T: line and its Execute$
// SVar, the way a card script writes them, so the test drives the real
// compiled trigger rather than a hand-built compile.Ability (TEST-1).
func auraDefWithTrigger(t *testing.T, name, enchant, trigger, svar, svarText string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment Aura")
	raw.Faces[0].Keywords = []string{"Enchant:" + enchant}
	raw.Faces[0].Triggers = []string{trigger}
	raw.Faces[0].SVars.Set(svar, svarText)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

func wildGrowthDef(t *testing.T) *compile.Card {
	t.Helper()
	return auraDefWithTrigger(t, "Wild Growth", "Land", wildGrowthTrigger, "TrigMana", wildGrowthTrigMana)
}

// greenPips is the green entry of ManaPool.Breakdown (W U B R G C).
func greenPips(g *engine.Game, p engine.PlayerID) int {
	return g.Player(p).ManaPool.Breakdown()[4]
}

// Tapping a Forest enchanted by Wild Growth adds both greens in the same
// call: the static trigger resolves at its trigger site, never reaches the
// stack, and the extra mana pays a {G}{G} cost with no ResolveStack between
// the tap and the payment (CR 605.1b, TriggerHandler.java:522-527).
func TestStaticManaTriggerAddsManaBeforeThePayment(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	aura := g.NewCard(wildGrowthDef(t), p, engine.Battlefield)
	g.Attach(aura, forest)
	c := engine.NewScriptedController()

	if !g.TapLandForMana(p, forest, mana.Green, c) {
		t.Fatal("TapLandForMana failed tapping a Forest for green")
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("static trigger failed: %v", err)
	}
	if got := g.StackLen(); got != 0 {
		t.Errorf("StackLen() = %d, want 0: a triggered mana ability never uses the stack", got)
	}
	if got := greenPips(g, p); got != 2 {
		t.Fatalf("green in pool = %d, want 2 (the Forest's own and Wild Growth's)", got)
	}
	if !g.PayManaCost(p, mana.MustParse("G G"), c) {
		t.Fatal("PayManaCost {G}{G} failed right after the tap")
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("pool total after paying = %d, want 0", got)
	}
}

// A static trigger is not an activated or stacked ability, so it emits
// neither AbilityActivated nor AbilityResolved (ADR-0020 decision 2).
func TestStaticManaTriggerEmitsNoStackEvents(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	aura := g.NewCard(wildGrowthDef(t), p, engine.Battlefield)
	g.Attach(aura, forest)
	var sink recordingSink
	g.SetSink(&sink)

	if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	for _, e := range sink.events {
		if e.Kind == engine.AbilityActivated || e.Kind == engine.AbilityResolved {
			t.Errorf("event %v emitted for a static trigger", e.Kind)
		}
	}
}

// Defined$ TriggeredCardController names the tapped land's controller, not
// the trigger host's: an opponent's Wild Growth on my Forest adds my extra
// green (AbilityUtils.java:1017-1027).
func TestStaticManaTriggerPaysTheTappedCardsController(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	me, opp := g.Players()[0], g.Players()[1]
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), me, engine.Battlefield)
	aura := g.NewCard(wildGrowthDef(t), opp, engine.Battlefield)
	g.Attach(aura, forest)

	if !g.TapLandForMana(me, forest, mana.Green, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana failed")
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("static trigger failed: %v", err)
	}
	if got := greenPips(g, me); got != 2 {
		t.Errorf("my green = %d, want 2", got)
	}
	if got := g.Player(opp).ManaPool.Total(); got != 0 {
		t.Errorf("opponent's pool total = %d, want 0", got)
	}
}

// A printed "T: Add G" mana ability (ActivateManaAbility) is a mana ability
// with a tap cost too, so it fires the same static trigger.
func TestStaticManaTriggerFiresFromActivatedManaAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	g.SetTurnState(1, p, engine.Main1)
	dork := g.NewCard(creatureDefWithAbility(t, "Test Mana Dork", "AB$ Mana | Cost$ T | Produced$ G"), p, engine.Battlefield)
	aura := g.NewCard(wildGrowthDef(t), p, engine.Battlefield)
	g.Attach(aura, dork)

	if !g.ActivateManaAbility(p, dork, 0, engine.NewScriptedController()) {
		t.Fatal("ActivateManaAbility failed")
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("static trigger failed: %v", err)
	}
	if got, want := g.StackLen(), 0; got != want {
		t.Errorf("StackLen() = %d, want %d", got, want)
	}
	if got := greenPips(g, p); got != 2 {
		t.Errorf("green in pool = %d, want 2", got)
	}
}

// The same event's non-static match still goes on the stack; only the
// Static$ True one resolves on the spot (TriggerHandler.java:300-309).
func TestStaticManaTriggerLeavesNonStaticMatchStacked(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	static := g.NewCard(wildGrowthDef(t), p, engine.Battlefield)
	g.Attach(static, forest)
	stacked := g.NewCard(auraDefWithTrigger(t, "Test Stacked Growth", "Land",
		"Mode$ TapsForMana | ValidCard$ Card.AttachedBy | Execute$ TrigMana",
		"TrigMana", wildGrowthTrigMana), p, engine.Battlefield)
	g.Attach(stacked, forest)
	c := engine.NewScriptedController()

	if !g.TapLandForMana(p, forest, mana.Green, c) {
		t.Fatal("TapLandForMana failed")
	}
	if got := g.StackLen(); got != 1 {
		t.Fatalf("StackLen() = %d, want 1 (the non-static trigger only)", got)
	}
	if got := greenPips(g, p); got != 2 {
		t.Errorf("green before ResolveStack = %d, want 2", got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := greenPips(g, p); got != 3 {
		t.Errorf("green after ResolveStack = %d, want 3: the stacked trigger reads TriggeredCardController too", got)
	}
}

// A static trigger whose effect this port refuses has no error path at the
// tap itself (TapLandForMana returns bool), so the error waits on the Game
// and the next boundary returns it exactly once (ADR-0020 decision 4, GO-7).
func TestStaticManaTriggerErrorReachesTheNextBoundary(t *testing.T) {
	t.Parallel()

	refused := "DB$ Mana | Produced$ G | RestrictValid$ Creature | Defined$ TriggeredCardController"
	build := func(t *testing.T) (*engine.Game, engine.PlayerID, engine.CardID) {
		t.Helper()
		g := newGame(t, "a", "b")
		p := g.Players()[0]
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		aura := g.NewCard(auraDefWithTrigger(t, "Test Restricted Growth", "Land", wildGrowthTrigger, "TrigMana", refused), p, engine.Battlefield)
		g.Attach(aura, forest)
		if !g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
			t.Fatal("TapLandForMana failed")
		}
		return g, p, forest
	}

	t.Run("TakePendingError", func(t *testing.T) {
		t.Parallel()
		g, p, _ := build(t)
		err := g.TakePendingError()
		if err == nil || !strings.Contains(err.Error(), "RestrictValid") {
			t.Fatalf("TakePendingError() = %v, want the RestrictValid$ refusal", err)
		}
		if again := g.TakePendingError(); again != nil {
			t.Errorf("second TakePendingError() = %v, want nil: the error is returned once", again)
		}
		if got := greenPips(g, p); got != 1 {
			t.Errorf("green in pool = %d, want 1 (the land's own only)", got)
		}
	})
	t.Run("ResolveStack", func(t *testing.T) {
		t.Parallel()
		g, _, _ := build(t)
		if err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController()); err == nil {
			t.Fatal("ResolveStack() = nil on an empty stack, want the pending static-trigger error")
		}
	})
	t.Run("Clone", func(t *testing.T) {
		t.Parallel()
		g, _, _ := build(t)
		if err := g.Clone().TakePendingError(); err == nil {
			t.Error("clone has no pending error, want the original's")
		}
		if err := g.TakePendingError(); err == nil {
			t.Error("taking the clone's error cleared the original's")
		}
	})
}

// A clone owns the same immutable Registry, so a static trigger fired inside
// the AI's lookahead resolves there too rather than finding none.
func TestStaticManaTriggerResolvesOnAClone(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p := g.Players()[0]
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	aura := g.NewCard(wildGrowthDef(t), p, engine.Battlefield)
	g.Attach(aura, forest)

	clone := g.Clone()
	if !clone.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController()) {
		t.Fatal("TapLandForMana on the clone failed")
	}
	if err := clone.TakePendingError(); err != nil {
		t.Fatalf("static trigger on the clone failed: %v", err)
	}
	if got := greenPips(clone, p); got != 2 {
		t.Errorf("clone green = %d, want 2", got)
	}
	if got := g.Player(p).ManaPool.Total(); got != 0 {
		t.Errorf("original pool total = %d, want 0", got)
	}
}
