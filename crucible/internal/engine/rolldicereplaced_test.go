package engine_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// rollReplacementDef is an enchantment carrying one replacement line and the
// SVars it chains through, in pairs of name and body.
func rollReplacementDef(t *testing.T, name, replacement string, svars ...string) *compile.Card {
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
	for i := 0; i+1 < len(svars); i += 2 {
		raw.Faces[0].SVars.Set(svars[i], svars[i+1])
	}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

const pixieLine = "Event$ RollDice | ActiveZones$ Battlefield | ValidPlayer$ You | ReplaceWith$ PlusRoll | Description$ One more die, ignore the lowest."

// rollTwoDice resolves "roll 2d6, gain life equal to the total" for p.
func rollTwoDice(t *testing.T, g *engine.Game, p engine.PlayerID) {
	t.Helper()
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ RollDice | Amount$ 2 | Sides$ 6 | ResultSVar$ Result | SubAbility$ DBGain",
		"DBGain", "DB$ GainLife | Defined$ You | LifeAmount$ Result")
}

// TestRollDiceReplacementRollsOneMoreAndIgnoresTheLowest is Pixie Guide's
// "roll that many dice plus one and ignore the lowest roll" (ReplaceRollDice):
// two dice become three and the lowest is set aside, so the total is the
// two highest of the first three rolls on the game's stream.
func TestRollDiceReplacementRollsOneMoreAndIgnoresTheLowest(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(rollReplacementDef(t, "Test Pixie", pixieLine,
		"PlusRoll", "DB$ ReplaceEffect | VarName$ Number | VarValue$ ReplaceCount$Number/Plus.1 | SubAbility$ IgnoreLowest",
		"IgnoreLowest", "DB$ ReplaceEffect | VarName$ Ignore | VarValue$ ReplaceCount$Ignore/Plus.1"), p, engine.Battlefield)

	rollTwoDice(t, g, p)

	r := javarand.New(1)
	rolls := []int{int(r.Int32n(6)) + 1, int(r.Int32n(6)) + 1, int(r.Int32n(6)) + 1}
	sort.Ints(rolls)
	if got, want := g.Player(p).Life, 20+rolls[1]+rolls[2]; got != want {
		t.Errorf("life = %d, want %d (rolls %v, lowest ignored)", got, want, rolls)
	}
}

// TestRollDiceReplacementHonoursValidSidesAndPlayer proves ReplaceRollDice's
// own two gates: ValidSides$ 4 does not apply to a six-sided die, and
// ValidPlayer$ Opponent does not apply to the host controller's roll.
func TestRollDiceReplacementHonoursValidSidesAndPlayer(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		strings.Replace(pixieLine, "ValidPlayer$ You", "ValidPlayer$ You | ValidSides$ 4", 1),
		strings.Replace(pixieLine, "ValidPlayer$ You", "ValidPlayer$ Opponent", 1),
	} {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(rollReplacementDef(t, "Test Gated Pixie", line,
			"PlusRoll", "DB$ ReplaceEffect | VarName$ Number | VarValue$ ReplaceCount$Number/Plus.1"), p, engine.Battlefield)

		rollTwoDice(t, g, p)

		r := javarand.New(1)
		want := 20 + int(r.Int32n(6)) + 1 + int(r.Int32n(6)) + 1
		if got := g.Player(p).Life; got != want {
			t.Errorf("%q: life = %d, want %d (an unmodified 2d6)", line, got, want)
		}
	}
}

// TestRollDiceReplacementWithAnExchangeStillFailsClosed is Vedalken
// Squirrel-Whacker's DicePTExchanges (VarType$ CardSet): a post-roll exchange
// this port does not model, so the roll errors rather than landing unmodified.
func TestRollDiceReplacementWithAnExchangeStillFailsClosed(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(rollReplacementDef(t, "Test Swapper",
		"Event$ RollDice | ActiveZones$ Battlefield | ValidPlayer$ You | ValidSides$ 6 | ReplaceWith$ SwapRoll | Description$ Exchange.",
		"SwapRoll", "DB$ ReplaceEffect | VarName$ DicePTExchanges | VarType$ CardSet | VarValue$ Self"), p, engine.Battlefield)

	if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Die", "DB$ RollDice | Sides$ 6"), engine.NewScriptedController()); err == nil {
		t.Error("roll with an exchange replacement in play succeeded, want an error")
	}
}
