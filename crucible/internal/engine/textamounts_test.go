package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestChangeTextRewritesWhatAnAmountCounts proves AbilityUtils.java:440: the
// text of an SVar amount ("Valid Creature.Elf" of Count$Valid Creature.Elf)
// goes through the card's word changes when it is read, so a card counting Elves
// counts Giants after an Elf -> Giant change, and the printed definition keeps
// its own amount (CR 707.2). An amount with no word in it, and a Number$ one,
// are left alone.
func TestChangeTextRewritesWhatAnAmountCounts(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	tally := g.NewCard(copyTestDef(t, "Tally", "Creature Elf", "*", "3",
		"S:Mode$ Continuous | EffectZone$ All | Affected$ Card.Self | CharacteristicDefining$ True | SetPower$ X",
		"SVar:X:Count$Valid Creature.Elf",
		"SVar:Y:Number$4",
		"SVar:Z:Count$Valid Creature"), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Elf Friend", "Creature Elf", "1", "1"), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Giant Friend", "Creature Giant", "1", "1"), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Giant Friend", "Creature Giant", "1", "1"), p, engine.Battlefield)
	g.NewCard(copyTestDef(t, "Giant Friend", "Creature Giant", "1", "1"), p, engine.Battlefield)
	sba(g)
	wantPT(t, g, tally, 2, 3) // itself and the other Elf

	changeText(t, g, p, engine.NewScriptedController(),
		"DB$ ChangeText | Defined$ Targeted | ChangeTypeWord$ Elf Giant | Duration$ Permanent", tally)
	// Tally is now a Giant counting Giants: itself and the three others.
	wantPT(t, g, tally, 4, 3)

	faces := g.Card(tally).Def.Faces[0]
	if got := faces.Amounts["x"].String(); got != "Count$Valid Creature.Giant" {
		t.Errorf("X = %q, want Count$Valid Creature.Giant", got)
	}
	if faces.Amounts["y"].String() != "Number$4" || faces.Amounts["z"].String() != "Count$Valid Creature" {
		t.Errorf("Y = %q, Z = %q, want both unchanged", faces.Amounts["y"].String(), faces.Amounts["z"].String())
	}
	if got := g.Card(tally).UncopiedDef().Faces[0].Amounts["x"].String(); got != "Count$Valid Creature.Elf" {
		t.Errorf("printed X = %q, want the printed Count$Valid Creature.Elf", got)
	}
}
