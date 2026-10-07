package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// TestDiscardOptionalFilteredRemembersWhatWasDiscarded is Mox Diamond's
// "you may discard a land card" (DiscardEffect.java:255-264): the pick comes
// from the hand cards matching DiscardValid$, may be empty with Optional$,
// must come from the offered cards, and RememberDiscarded$ records the card.
func TestDiscardOptionalFilteredRemembersWhatWasDiscarded(t *testing.T) {
	t.Parallel()

	const line = "DB$ Discard | Mode$ TgtChoose | DiscardValid$ Land | Optional$ True | RememberDiscarded$ True"

	g, p, _ := newTwoPlayerGame(t)
	land := g.NewCard(landDef(t, "Test Land", "Land"), p, engine.Hand)
	bear := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{land})
	host, err := resolveNow(t, g, p, c, nil, line)
	if err != nil {
		t.Fatal(err)
	}
	if g.Card(land).Zone != engine.Graveyard || g.Card(bear).Zone != engine.Hand {
		t.Errorf("land in %v, bear in %v, want graveyard and hand", g.Card(land).Zone, g.Card(bear).Zone)
	}
	if r := g.Card(host).Memory.Remembered(); len(r) != 1 || r[0] != engine.CardEntity(land) {
		t.Errorf("host remembers %v, want the discarded land", r)
	}

	g2, p2, _ := newTwoPlayerGame(t)
	land2 := g2.NewCard(landDef(t, "Test Land", "Land"), p2, engine.Hand)
	c2 := engine.NewScriptedController()
	c2.QueueCardChoice(nil)
	if _, err := resolveNow(t, g2, p2, c2, nil, line); err != nil {
		t.Fatal(err)
	}
	if g2.Card(land2).Zone != engine.Hand {
		t.Error("an empty optional pick discarded the land anyway")
	}

	g3, p3, _ := newTwoPlayerGame(t)
	g3.NewCard(landDef(t, "Test Land", "Land"), p3, engine.Hand)
	bear3 := g3.NewCard(creatureDefPT(t, "1", "1"), p3, engine.Hand)
	c3 := engine.NewScriptedController()
	c3.QueueCardChoice([]engine.CardID{bear3})
	if _, err := resolveNow(t, g3, p3, c3, nil, line); err == nil {
		t.Error("discarding a card DiscardValid$ does not offer succeeded, want an error")
	}

	g4, p4, _ := newTwoPlayerGame(t)
	g4.NewCard(creatureDefPT(t, "1", "1"), p4, engine.Hand)
	if _, err := resolveNow(t, g4, p4, engine.NewScriptedController(), nil, line); err != nil {
		t.Errorf("no land to discard: %v, want nothing to do", err)
	}
	g5, p5, _ := newTwoPlayerGame(t)
	if _, err := resolveNow(t, g5, p5, engine.NewScriptedController(), nil, line); err != nil {
		t.Errorf("empty hand: %v, want nothing to do", err)
	}
}

// TestSacrificeStrictAmountSacrificesNothingWhenShort is Lotus Vale's "sacrifice
// two untapped lands": StrictAmount$ (SacrificeEffect.java:137-144) sacrifices
// nothing, and asks nobody, when fewer than Amount$ cards qualify.
func TestSacrificeStrictAmountSacrificesNothingWhenShort(t *testing.T) {
	t.Parallel()

	const line = "DB$ Sacrifice | SacValid$ Creature.Other | Amount$ 2 | StrictAmount$ True | Defined$ You"

	g, p, _ := newTwoPlayerGame(t)
	one := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, line); err != nil {
		t.Fatal(err)
	}
	if g.Card(one).Zone != engine.Battlefield {
		t.Error("a strict sacrifice of two took the only creature")
	}

	g2, p2, _ := newTwoPlayerGame(t)
	a := g2.NewCard(creatureDefPT(t, "1", "1"), p2, engine.Battlefield)
	b := g2.NewCard(creatureDefPT(t, "1", "1"), p2, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueSacrificeChoice([]engine.CardID{a, b})
	if _, err := resolveNow(t, g2, p2, c, nil, line); err != nil {
		t.Fatal(err)
	}
	if g2.Card(a).Zone == engine.Battlefield || g2.Card(b).Zone == engine.Battlefield {
		t.Error("two creatures available, a strict sacrifice of two left one")
	}
}

// TestLoseLifeWithoutDefinedIsTheActivator is Lich's "you lose life equal to
// your life total" (a LoseLife with no Defined$ and no target).
func TestLoseLifeWithoutDefinedIsTheActivator(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ LoseLife | LifeAmount$ 3")
	if got := g.Player(p).Life; got != 17 {
		t.Errorf("life = %d, want 17", got)
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("opponent life = %d, want 20", got)
	}
}

// TestCastSaAndWasCastNeedACastCard proves the valid properties Primeval
// Spawn and Containment Priest read: a card that was never cast is not
// wasCast, fails every CastSa spec, and a malformed spec matches nothing.
func TestCastSaAndWasCastNeedACastCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	card := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	match := func(spec string) bool {
		return engine.Matches(g, g.Card(card), valid.Parse(spec), p, card)
	}
	for spec, want := range map[string]bool{
		"Card.wasCast":                        false,
		"Card.!wasCast":                       true,
		"Card.CastSa Spell.ManaSpent EQ0":     false,
		"Card.CastSa Spell.ManaSpent GE1":     false,
		"Card.CastSa Spell.ManaSpent EQx":     false,
		"Card.CastSa Spell.ManaSpent E":       false,
		"Card.CastSa Spell.Other EQ0":         false,
		"Card.!wasCast,Card.CastSa Spell.Foo": true,
	} {
		if got := match(spec); got != want {
			t.Errorf("%q matches = %v, want %v", spec, got, want)
		}
	}
}

// TestEntryReplacementChainErrorIsRecorded proves a pre-entry Moved line
// whose ReplaceWith$ chain cannot run does not enter the card as if nothing
// replaced it (GO-7), and that the same line with a runnable chain exiles it.
func TestEntryReplacementChainErrorIsRecorded(t *testing.T) {
	t.Parallel()

	const (
		moved = "Event$ Moved | ActiveZones$ Battlefield | Destination$ Battlefield | ValidCard$ Creature | ReplaceWith$ Exile"
		pull  = "DB$ ChangeZone | Origin$ Hand | Destination$ Battlefield | ChangeType$ Creature | ChangeNum$ 1 | Hidden$ True | Mandatory$ True"
	)
	for _, tc := range []struct {
		name, chain string
		wantErr     bool
		wantZone    engine.ZoneType
	}{
		{"runnable chain exiles it", "DB$ ChangeZone | Hidden$ True | Origin$ All | Destination$ Exile | Defined$ ReplacedCard", false, engine.Exile},
		{"unrunnable chain is an error", "DB$ ChangeZone | Hidden$ True | Origin$ All | Destination$ Exile | Defined$ ReplacedCard | Duration$ Bad", true, engine.Hand},
	} {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Watcher", moved, "Exile", tc.chain), p, engine.Battlefield)
		bear := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{bear})
		if _, err := resolveNow(t, g, p, c, nil, pull); (err != nil) != tc.wantErr {
			t.Errorf("%s: resolve error = %v, want error %v", tc.name, err, tc.wantErr)
		}
		if got := g.Card(bear).Zone; got != tc.wantZone {
			t.Errorf("%s: bear in %v, want %v", tc.name, got, tc.wantZone)
		}
	}
}

// TestGainLifeReplacementRefusesAnUnknownValidSource proves a ValidSource$
// other than SpellAbility is a recorded error, not a silent skip.
func TestGainLifeReplacementRefusesAnUnknownValidSource(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Gain Watcher",
		"Event$ GainLife | ActiveZones$ Battlefield | ValidSource$ Card | ReplaceWith$ R",
		"R", "DB$ LoseLife | LifeAmount$ 1 | Defined$ ReplacedPlayer"), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ GainLife | LifeAmount$ 2"); err == nil {
		t.Error("a GainLife replacement with ValidSource$ Card did not record an error")
	}
}

// TestDrawAndDamageReplacementFallThroughToTheRegistry proves a ReplaceWith$
// ability the hand-run shapes do not cover runs through the Registry, and an
// error in it is recorded.
func TestDrawAndDamageReplacementFallThroughToTheRegistry(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Draw Gain",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ R",
		"R", "DB$ GainLife | Defined$ ReplacedPlayer | LifeAmount$ 4"), p, engine.Battlefield)
	g.DrawCards(p, 1, engine.NewScriptedController())
	if got := g.Player(p).Life; got != 24 {
		t.Errorf("life = %d, want 24 -- the draw became a 4-life gain", got)
	}
	if got := g.Zone(engine.Hand, p).Len(); got != 0 {
		t.Errorf("hand = %d, want 0", got)
	}

	g2, p2, _ := newTwoPlayerGame(t)
	g2.NewCard(nil, p2, engine.Library)
	g2.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Draw",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ R",
		"R", "DB$ ChangeZone | Defined$ Self | Destination$ Exile | Duration$ Bad"), p2, engine.Battlefield)
	g2.DrawCards(p2, 1, engine.NewScriptedController())
	if g2.TakePendingError() == nil {
		t.Error("a Draw replacement whose ability errors recorded nothing")
	}

	g3, p3, _ := newTwoPlayerGame(t)
	g3.NewCard(replacementEnchantmentDefWithSVar(t, "Test Damage Gain",
		"Event$ DamageDone | ActiveZones$ Battlefield | ReplaceWith$ R",
		"R", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p3, engine.Battlefield)
	resolveLine(t, g3, p3, engine.NewScriptedController(), "DB$ DealDamage | Defined$ You | NumDmg$ 2")
	if got := g3.Player(p3).Life; got != 21 {
		t.Errorf("life = %d, want 21 -- the damage was replaced by a 1-life gain", got)
	}

	g4, p4, _ := newTwoPlayerGame(t)
	g4.NewCard(replacementEnchantmentDefWithSVar(t, "Test Bad Damage",
		"Event$ DamageDone | ActiveZones$ Battlefield | ReplaceWith$ R",
		"R", "DB$ ChangeZone | Defined$ Self | Destination$ Exile | Duration$ Bad"), p4, engine.Battlefield)
	if _, err := resolveNow(t, g4, p4, engine.NewScriptedController(), nil, "DB$ DealDamage | Defined$ You | NumDmg$ 2"); err == nil {
		t.Error("a DamageDone replacement whose ability errors recorded nothing")
	}
}
