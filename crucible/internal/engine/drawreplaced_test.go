package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// replacementEnchantmentDefWithSVar is replacementEnchantmentDef's own
// sibling, carrying the SVar a ReplaceWith$ line references -- thought_reflection.txt's
// own real shape needs both the R: line and the SVar it points at, unlike
// every other replacementEnchantmentDef caller so far.
func replacementEnchantmentDefWithSVar(t *testing.T, name, replacement, svarName, svarBody string) *compile.Card {
	t.Helper()

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: name}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = name
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Replacements = []string{replacement}
	raw.Faces[0].SVars.Set(svarName, svarBody)

	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestDrawReplacedByBareReplacement proves drawReplaced (replacement.go) is
// wired into DrawCards (turn.go): thought_reflection.txt's own real bare "if
// you would draw a card, draw two cards instead" (ValidPlayer$ You,
// ReplaceWith$ naming a plain DB$ Draw | Defined$ You | NumCards$ 2) draws
// two cards for one requested draw.
func TestDrawReplacedByBareReplacement(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Draw Doubler",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ DrawTwo | Description$ Draw two instead.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("hand has %d cards, want 2 -- ReplaceWith$ DrawTwo must draw two cards for the one requested draw", got)
	}
}

// TestDrawReplacedSetsDrewFromEmptyLibraryWhenItRunsOut proves the
// replacement's own draws run through the identical drawOneCard primitive a
// normal draw does: with only one card in the library, drawing "two instead"
// draws that one card and then genuinely attempts to draw from an empty
// library, which must still be recorded -- unlike Prevent$, a substituted
// draw is a real draw attempt, not a skipped one.
func TestDrawReplacedSetsDrewFromEmptyLibraryWhenItRunsOut(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Draw Doubler",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ DrawTwo | Description$ Draw two instead.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1 -- the library only had one card to give", got)
	}
	if !g.Player(p).DrewFromEmptyLibrary {
		t.Error("DrewFromEmptyLibrary = false, want true -- the replacement's own second draw genuinely ran out of library")
	}
}

// TestDrawReplacedWhenHellbentConditionMet proves the general-gate fold-in
// (replacementRequirementsCheck) on Draw's own ReplaceWith$ family:
// phial_of_galadriel.txt's own real "if you would draw a card while you have
// no cards in hand, draw two cards instead" (Hellbent$ True) fires once the
// hand is empty.
func TestDrawReplacedWhenHellbentConditionMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Hellbent Draw Doubler",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | Hellbent$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead while hellbent.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("hand has %d cards, want 2 -- Hellbent$ True is met with an empty hand, so the replacement must fire", got)
	}
}

// TestDrawNotReplacedWhenHellbentConditionNotMet is the same lock's own
// mirror: with a card already in hand, Hellbent$ is not met, so the draw
// proceeds normally.
func TestDrawNotReplacedWhenHellbentConditionNotMet(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Hand)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Hellbent Draw Doubler",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | Hellbent$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead while hellbent.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("hand has %d cards, want 2 (1 already there + 1 normal draw) -- Hellbent$ is not met, so the draw must not be replaced", got)
	}
}

// TestDrawReplacedWithPutCounterInstead proves a ReplaceWith$ target naming
// DB$ PutCounter, not DB$ Draw, also dispatches: ormos_archive_keeper.txt's
// own real "if you would draw a card while your library has no cards in it,
// instead put five +1/+1 counters on CARDNAME" (IsPresent$ Card.YouOwn |
// PresentZone$ Library | PresentCompare$ EQ0, resolved through
// replacementRequirementsCheck's own triggerCommonRequirementsMet fold-in).
func TestDrawReplacedWithPutCounterInstead(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	host := g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Empty-Library Counters",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | IsPresent$ Card.YouOwn | PresentZone$ Library | PresentCompare$ EQ0 | ReplaceWith$ AddCounters | Description$ Put counters instead.",
		"AddCounters", "DB$ PutCounter | CounterType$ P1P1 | CounterNum$ 5 | Defined$ Self"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("hand has %d cards, want 0 -- the draw was replaced, not performed", got)
	}
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 5 {
		t.Errorf("P1P1 counters = %d, want 5 -- ReplaceWith$ AddCounters must put them on the host", got)
	}
	if g.Player(p).DrewFromEmptyLibrary {
		t.Error("DrewFromEmptyLibrary = true, want false -- the draw was replaced before the empty-library check ever ran")
	}
}

// TestDrawNotReplacedByTargetAbilityWithSubAbility proves
// blood_scrivener.txt's own real ReplaceWith$ target -- DB$ Draw carrying
// its own SubAbility$ DBLoseLife -- is refused outright rather than run with
// the chained half silently dropped (GO-7): even with Hellbent$ True met
// (an empty hand), the draw proceeds normally rather than doubling.
func TestDrawNotReplacedByTargetAbilityWithSubAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)

	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "Test Chained Draw Doubler"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Test Chained Draw Doubler"
	raw.Faces[0].Type = cardtype.Parse(reg, "Enchantment")
	raw.Faces[0].Replacements = []string{
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | Hellbent$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead while hellbent.",
	}
	raw.Faces[0].SVars.Set("DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2 | SubAbility$ DBLoseLife")
	raw.Faces[0].SVars.Set("DBLoseLife", "DB$ LoseLife | Defined$ You | LifeAmount$ 1")
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g.NewCard(def, p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1 -- a chained ReplaceWith$ target must not dispatch, so the draw stays a plain single draw", got)
	}
}

// TestDrawNotReplacedByUnrecognizedTargetAbility proves a ReplaceWith$
// target this port has no direct-dispatch case for -- magus_of_the_chains.txt's
// own real DB$ Discard target -- skips the whole line rather than guessing,
// and the draw proceeds normally.
func TestDrawNotReplacedByUnrecognizedTargetAbility(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Discard Instead",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ Player | ReplaceWith$ DiscardOne | Description$ Discard instead.",
		"DiscardOne", "DB$ Discard | Defined$ ReplacedPlayer | Mandatory$ True | NumCards$ 1 | Mode$ TgtChoose"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1 -- an unrecognized ReplaceWith$ target must skip the whole line", got)
	}
}
