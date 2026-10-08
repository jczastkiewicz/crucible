package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// destroyReplacementDef is a Destroy replacement card carrying the Pay SVar
// and X = Count$xPaid, the pair the pay-life lines have.
func destroyReplacementDef(t *testing.T, name, replacement, payBody string) *compile.Card {
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
	raw.Faces[0].SVars.Set("Pay", payBody)
	raw.Faces[0].SVars.Set("X", "Count$xPaid")
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return c
}

// TestReplacementPayLifeXIsPaidInsideTheReplacement is the Cost$ Mandatory
// PayLife<X> of a ReplaceWith$ ability (Phyrexian Processor, Minion of the
// Wastes, Nameless Race) reached through a destruction replacement, which runs
// the same runReplacementChain as the entry replacements the scenarios cover:
// the chosen X comes out of the life total before the ability resolves, a
// number outside 0..life or past XMax$ is an error that takes no life, and a
// Cost$ of another shape is not paid.
func TestReplacementPayLifeXIsPaidInsideTheReplacement(t *testing.T) {
	t.Parallel()

	const payLife = "AB$ StoreSVar | Cost$ Mandatory PayLife<X> | %s SVar$ Paid | Type$ Calculate | Expression$ X"
	cases := []struct {
		name     string
		ability  string
		choice   int
		wantLife int
		wantErr  string
	}{
		{"pays the chosen amount", strings.Replace(payLife, "%s ", "", 1), 3, 17, ""},
		{"pays nothing for zero", strings.Replace(payLife, "%s ", "", 1), 0, 20, ""},
		{"stays within XMax$", strings.Replace(payLife, "%s", "XMax$ 5 |", 1), 5, 15, ""},
		{"refuses more than XMax$", strings.Replace(payLife, "%s", "XMax$ 2 |", 1), 3, 20, "outside 0..2"},
		{"refuses more than the life total", strings.Replace(payLife, "%s ", "", 1), 21, 20, "outside 0..20"},
		{"refuses a negative amount", strings.Replace(payLife, "%s ", "", 1), -1, 20, "outside 0..20"},
		{"refuses an XMax$ it cannot read", strings.Replace(payLife, "%s", "XMax$ Missing |", 1), 1, 20, "XMax"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			g.NewCard(destroyReplacementDef(t, "Test Pay Life",
				"Event$ Destroy | ActiveZones$ Battlefield | ValidCard$ Creature | ReplaceWith$ Pay | Description$ Pay life instead.",
				tc.ability), p, engine.Battlefield)
			victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			c.QueueNumberChoice(tc.choice)

			_, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(victim)}, "DB$ Destroy | Defined$ Targeted")

			if tc.wantErr == "" && err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("resolve error = %v, want one containing %q", err, tc.wantErr)
			}
			if got := g.Player(p).Life; got != tc.wantLife {
				t.Errorf("life = %d, want %d", got, tc.wantLife)
			}
		})
	}
}

// TestReplacementCostOfAnotherShapeIsNotPaid pins that only the one corpus
// shape asks the controller for a number: any other Cost$ on a ReplaceWith$
// ability is left to the ability, as before.
func TestReplacementCostOfAnotherShapeIsNotPaid(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(destroyReplacementDef(t, "Test Free Cost",
		"Event$ Destroy | ActiveZones$ Battlefield | ValidCard$ Creature | ReplaceWith$ Pay | Description$ Store instead.",
		"AB$ StoreSVar | Cost$ 0 | SVar$ Paid | Type$ Number | Expression$ 4"), p, engine.Battlefield)
	victim := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)

	// No number is queued: asking for one would panic.
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(victim)}, "DB$ Destroy | Defined$ Targeted"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("life = %d, want 20", got)
	}
}
