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
