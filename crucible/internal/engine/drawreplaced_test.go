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

// TestDrawNotReplacedOnFirstDrawOfOwnDrawStep proves
// teferis_ageless_insight.txt's/bard_king_of_dale.txt's own real
// NotFirstCardInDrawStep$ True gate: with the game genuinely in p's own Draw
// step and p having drawn nothing yet this step (DrawnThisDrawStep == 0),
// the lock's own ReplaceWith$ DrawTwo must not fire for this, the exempted,
// draw.
func TestDrawNotReplacedOnFirstDrawOfOwnDrawStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.Draw)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Ageless Insight",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | NotFirstCardInDrawStep$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead, except the first each draw step.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1 -- the first draw of p's own draw step is exempt", got)
	}
}

// TestDrawReplacedOnSecondDrawOfOwnDrawStep is the same lock's own mirror:
// p having already drawn once this Draw step (DrawnThisDrawStep == 1), a
// second draw during the identical step is not exempt and doubles.
func TestDrawReplacedOnSecondDrawOfOwnDrawStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.Draw)
	g.Player(p).DrawnThisDrawStep = 1
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Ageless Insight",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | NotFirstCardInDrawStep$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead, except the first each draw step.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("hand has %d cards, want 2 -- a second draw in the same draw step is not exempt, so ReplaceWith$ DrawTwo must fire", got)
	}
}

// TestDrawnThisDrawStepAdvancesAcrossConsecutiveDraws proves
// DrawnThisDrawStep (player.go) actually increments in drawOneCard (turn.go)
// rather than only being poke-able by a test: two separate DrawCards calls
// in the identical Draw step -- the first exempt, the second not -- without
// ever setting the counter by hand.
func TestDrawnThisDrawStepAdvancesAcrossConsecutiveDraws(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.Draw)
	for i := 0; i < 3; i++ {
		g.NewCard(nil, p, engine.Library)
	}
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Ageless Insight",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | NotFirstCardInDrawStep$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead, except the first each draw step.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Fatalf("after 1st draw: hand has %d cards, want 1 -- exempt", got)
	}

	g.DrawCards(p, 1, engine.NewScriptedController())
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 3 {
		t.Errorf("after 2nd draw: hand has %d cards, want 3 (1 + doubled 2) -- drawOneCard's own increment must make the 2nd draw non-exempt", got)
	}
}

// TestDrawnThisDrawStepResetsAtEachDrawStep proves drawStep's own reset
// (turn.go, PhaseHandler.java:268-271's own per-player loop): without it,
// DrawnThisDrawStep would keep accumulating turn over turn and every turn
// past the first would wrongly see its own first draw as "not the first."
func TestDrawnThisDrawStepResetsAtEachDrawStep(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.Upkeep)
	for i := 0; i < 2; i++ {
		g.NewCard(nil, p, engine.Library)
	}

	g.AdvancePhase(engine.NewScriptedController()) // -> Draw, turn 2
	if got := g.Player(p).DrawnThisDrawStep; got != 1 {
		t.Fatalf("after turn 2's draw: DrawnThisDrawStep = %d, want 1", got)
	}

	for i := 0; i < 20 && (g.Turn() != 3 || g.ActivePhase() != engine.Draw); i++ {
		g.AdvancePhase(engine.NewScriptedController())
	}
	if g.Turn() != 3 || g.ActivePhase() != engine.Draw {
		t.Fatalf("did not reach turn 3's Draw step within 20 AdvancePhase calls (turn=%d, phase=%v)", g.Turn(), g.ActivePhase())
	}
	if got := g.Player(p).DrawnThisDrawStep; got != 1 {
		t.Errorf("after turn 3's draw: DrawnThisDrawStep = %d, want 1 -- the counter must reset at the start of each Draw step, not accumulate", got)
	}
}

// TestDrawReplacedOutsideOwnDrawStepEvenOnFirstDraw proves NotFirstCardInDrawStep$'s
// own "ownDraw" half: even with DrawnThisDrawStep still 0, a draw outside
// p's own Draw step (a spell drawing a card during Main1) is never the
// exempted "first card of the draw step" and must still be replaced.
func TestDrawReplacedOutsideOwnDrawStepEvenOnFirstDraw(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a")
	p := g.Players()[0]
	g.SetTurnState(2, p, engine.Main1)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Ageless Insight",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ You | NotFirstCardInDrawStep$ True | ReplaceWith$ DrawTwo | Description$ Draw two instead, except the first each draw step.",
		"DrawTwo", "DB$ Draw | Defined$ You | NumCards$ 2"), p, engine.Battlefield)

	g.DrawCards(p, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, p).Cards()); got != 2 {
		t.Errorf("hand has %d cards, want 2 -- outside the Draw step, NotFirstCardInDrawStep$'s own exemption never applies", got)
	}
}

// TestDrawReplacedByNotionThiefShapeDivertsToHostController proves
// notion_thief.txt's own real "if an opponent would draw a card except the
// first one they draw in each of their draw steps, instead you draw a card"
// (ValidPlayer$ Opponent, ReplaceWith$ naming Defined$ You): other, p's
// opponent, draws a card outside other's own draw step (not exempt), and it
// must be p -- Notion Thief's own controller -- who draws instead, not
// other.
func TestDrawReplacedByNotionThiefShapeDivertsToHostController(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, other, engine.Main1)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, other, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Notion Thief",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ Opponent | NotFirstCardInDrawStep$ True | ReplaceWith$ RepYouDraw | Description$ Instead you draw a card.",
		"RepYouDraw", "DB$ Draw | Defined$ You | NumCards$ 1"), p, engine.Battlefield)

	g.DrawCards(other, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, other).Cards()); got != 0 {
		t.Errorf("other's hand has %d cards, want 0 -- other's own draw must be replaced entirely", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("p's hand has %d cards, want 1 -- Defined$ You must resolve to the lock's own controller, not other", got)
	}
}

// TestDrawNotReplacedByNotionThiefShapeOnOpponentsOwnFirstDraw is the same
// lock's own regression proof: other drawing the very first card of other's
// own Draw step is exempt, so other draws normally and p's hand stays empty.
func TestDrawNotReplacedByNotionThiefShapeOnOpponentsOwnFirstDraw(t *testing.T) {
	t.Parallel()

	g := newGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, other, engine.Draw)
	g.NewCard(nil, p, engine.Library)
	g.NewCard(nil, other, engine.Library)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Notion Thief",
		"Event$ Draw | ActiveZones$ Battlefield | ValidPlayer$ Opponent | NotFirstCardInDrawStep$ True | ReplaceWith$ RepYouDraw | Description$ Instead you draw a card.",
		"RepYouDraw", "DB$ Draw | Defined$ You | NumCards$ 1"), p, engine.Battlefield)

	g.DrawCards(other, 1, engine.NewScriptedController())

	if got := len(g.Zone(engine.Hand, other).Cards()); got != 1 {
		t.Errorf("other's hand has %d cards, want 1 -- the first draw of other's own draw step is exempt", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("p's hand has %d cards, want 0 -- the replacement must not have fired", got)
	}
}
