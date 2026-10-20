package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// A delayed Mode$ SpellCast trigger fires once, on the next spell its
// ValidCard$ and ValidActivatingPlayer$ accept, and goes above that spell on
// the stack (TriggerHandler.runWaitingTrigger runs delayed triggers after the
// collected card triggers, then removes them).
func TestDelayedSpellCastFiresOnceOnTheNextMatchingSpell(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ SpellCast | ValidCard$ Instant | ValidActivatingPlayer$ You | ThisTurn$ True | Execute$ DBGain"
	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, watch,
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
		t.Fatal(err)
	}
	cast := func(typeLine string) {
		t.Helper()
		spell := g.NewCard(spellDefWith(t, "Test "+typeLine, typeLine, "G", "SP$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if !g.CastSpell(p, spell, engine.NewScriptedController()) {
			t.Fatalf("could not cast the %s", typeLine)
		}
	}

	cast("Sorcery")
	if got := g.StackLen(); got != 1 {
		t.Fatalf("stack holds %d items after a sorcery, want the spell alone", got)
	}
	cast("Instant")
	if got := g.StackLen(); got != 3 {
		t.Fatalf("stack holds %d items after the instant, want both spells and the delayed trigger", got)
	}
	top, _ := g.StackTop()
	if top.Source == 0 || g.Card(top.Source).Def.Name == "Test Instant" {
		t.Errorf("top of the stack is %q, want the delayed trigger above the instant", g.Card(top.Source).Def.Name)
	}
	cast("Instant")
	if got := g.StackLen(); got != 4 {
		t.Errorf("stack holds %d items after a second instant, want one more spell and no trigger: the watch fires once", got)
	}
}

// ValidActivatingPlayer$ You reads the delayed trigger's controller: the
// opponent's spell does not spend it.
func TestDelayedSpellCastIgnoresTheOpponentsSpell(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ SpellCast | ValidActivatingPlayer$ You | ThisTurn$ True | Execute$ DBGain"
	g, p, other := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, watch,
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
		t.Fatal(err)
	}
	spell := g.NewCard(spellDefWith(t, "Test Theirs", "Instant", "G", "SP$ GainLife | Defined$ You | LifeAmount$ 1"), other, engine.Hand)
	g.Player(other).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(other, spell, engine.NewScriptedController()) {
		t.Fatal("could not cast the opponent's spell")
	}
	if got := g.StackLen(); got != 1 {
		t.Errorf("stack holds %d items, want the opponent's spell alone", got)
	}
}

// Params the SpellCast check cannot resolve are refused at registration, not
// silently ignored.
func TestDelayedSpellCastRefusesUnresolvedParams(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ DelayedTrigger | Mode$ SpellCast | IsSingleTarget$ True | Execute$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3")
	if err == nil {
		t.Fatal("IsSingleTarget$ registered, want an error")
	}
}

// ValidSA$ is read against the spell on the stack: Spell.Instant accepts the
// instant, and Activated never accepts a cast.
func TestDelayedSpellCastReadsValidSA(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		spec string
		want int
	}{{"Spell.Instant", 2}, {"Activated", 1}} {
		g, p, _ := newTwoPlayerGame(t)
		watch := "DB$ DelayedTrigger | Mode$ SpellCast | ValidSA$ " + tc.spec + " | ValidActivatingPlayer$ You | ThisTurn$ True | Execute$ DBGain"
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, watch,
			"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
			t.Fatalf("%s: %v", tc.spec, err)
		}
		spell := g.NewCard(spellDefWith(t, "Test Instant", "Instant", "G", "SP$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1)
		if !g.CastSpell(p, spell, engine.NewScriptedController()) {
			t.Fatalf("%s: could not cast the instant", tc.spec)
		}
		if got := g.StackLen(); got != tc.want {
			t.Errorf("ValidSA$ %s: stack holds %d items, want %d", tc.spec, got, tc.want)
		}
	}
}

// HasXManaCost$ is resolved only for a card's own trigger, so a delayed one
// naming it is refused.
func TestDelayedSpellCastRefusesHasXManaCost(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ DelayedTrigger | Mode$ SpellCast | HasXManaCost$ True | Execute$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err == nil {
		t.Fatal("HasXManaCost$ registered, want an error")
	}
}

// A DelayedTrigger that is itself a targeted spell (ValidTgts$ with
// RememberObjects$ Targeted) remembers the target it was cast with: Reincarnation
// watches that creature alone, and returns a creature card of its owner's
// to the battlefield when it dies this turn.
func TestTargetedDelayedTriggerWatchesItsTarget(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	watched := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	bystander := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	r := &stackOfferRecorder{ScriptedController: engine.NewScriptedController()}
	if err := castCounter(t, g, p, "Graceful Reprieve", r, []engine.EntityID{engine.CardEntity(watched)}, mana.White, mana.White); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	kill := func(id engine.CardID) {
		t.Helper()
		if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(id)},
			"DB$ Destroy | ValidTgts$ Creature"); err != nil {
			t.Fatal(err)
		}
	}
	kill(bystander)
	if got := g.Card(bystander).Zone; got != engine.Graveyard {
		t.Fatalf("an unwatched creature that died is in %v, want the graveyard: the watch fired for it", got)
	}
	kill(watched)
	if got := g.Card(watched).Zone; got != engine.Battlefield {
		t.Errorf("the watched creature is in %v after it died, want the battlefield: Graceful Reprieve returned it", got)
	}
}

// Static$ on a delayed trigger resolves it at the trigger site, off the stack
// (TriggerHandler.runSingleTrigger -> playTrigger): Overmaster's "can't be
// countered" effect is in place before the spell can be responded to.
func TestStaticDelayedSpellCastResolvesWithoutTheStack(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ SpellCast | ValidActivatingPlayer$ You | ThisTurn$ True | Static$ True | Execute$ DBGain"
	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, watch,
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
		t.Fatal(err)
	}
	spell := g.NewCard(spellDefWith(t, "Test Static", "Instant", "G", "SP$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(p, spell, engine.NewScriptedController()) {
		t.Fatal("could not cast the instant")
	}
	if got := g.StackLen(); got != 1 {
		t.Errorf("stack holds %d items, want the spell alone: a static trigger never goes on the stack", got)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23: the static trigger resolved as the spell was cast", got)
	}
}

// A static delayed Mode$ Phase trigger resolves when its step begins, off the
// stack, and only once.
func TestStaticDelayedPhaseTriggerResolvesOnce(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ Phase | Phase$ Upkeep | ValidPlayer$ You | Static$ True | Execute$ DBGain"
	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, watch,
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
		t.Fatal(err)
	}
	g.SetTurnState(2, p, engine.Untap)
	for range 3 {
		g.AdvancePhase(engine.NewScriptedController())
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d after the upkeep, want 23", got)
	}
	if got := g.StackLen(); got != 0 {
		t.Errorf("stack holds %d items, want none", got)
	}
}

// A delayed Mode$ SpellAbilityCast trigger fires on the next activated ability
// its ValidSA$ accepts, and goes above that ability (Jaya's Phoenix); once.
func TestDelayedSpellAbilityCastFiresOnTheNextActivatedAbility(t *testing.T) {
	t.Parallel()

	const watch = "DB$ DelayedTrigger | Mode$ SpellAbilityCast | ValidSA$ Activated | ValidActivatingPlayer$ You | ThisTurn$ True | Execute$ DBGain"
	g, p, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, watch,
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ 3"); err != nil {
		t.Fatal(err)
	}
	elf := g.NewCard(creatureDefWithAbility(t, "Test Elf", "AB$ GainLife | Cost$ 0 | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	activate := func() {
		t.Helper()
		if !g.ActivateAbility(p, elf, 0, engine.NewScriptedController()) {
			t.Fatal("could not activate the ability")
		}
	}
	activate()
	if got := g.StackLen(); got != 2 {
		t.Fatalf("stack holds %d items after the first activation, want the ability and the delayed trigger", got)
	}
	activate()
	if got := g.StackLen(); got != 3 {
		t.Errorf("stack holds %d items after the second activation, want one more ability and no trigger: the watch fires once", got)
	}
}
