package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

func convokeSorcery(t *testing.T, cost string) *compile.Card {
	t.Helper()

	raw := &carddb.Card{Filename: "convoke-spell"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Convoke Spell"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Sorcery")
	raw.Faces[0].ManaCost = mana.MustParse(cost)
	raw.Faces[0].Keywords = []string{"Convoke"}
	raw.Faces[0].Abilities = []string{"SP$ GainLife | Defined$ You | LifeAmount$ 1"}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return def
}

// ManaCostBeingPaid.payManaViaConvoke (ManaCostBeingPaidTest#testPayManaViaConvoke):
// a convoking creature pays a colored shard it shares a color with, else {1}.
// With no mana at all, the creatures pay the whole cost in each of the
// Java test's orders.
func TestConvokeCreatureColorsPayColoredShardsFirstElseGeneric(t *testing.T) {
	t.Parallel()

	const (
		white = "Savannah Lions"
		green = "Grizzly Bears"
		red   = "Mons's Goblin Raiders"
		none  = "Ornithopter"
	)
	for _, tc := range []struct {
		name    string
		cost    string
		pickers []string
		ok      bool
	}{
		{"white, colorless, white on 1WW", "1 W W", []string{white, none, white}, true},
		{"colorless, white, white on 1WW", "1 W W", []string{none, white, white}, true},
		{"green, white, white on 1WW", "1 W W", []string{green, white, white}, true},
		{"green, red, white on 1WG", "1 W G", []string{green, red, white}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			spell := g.NewCard(convokeSorcery(t, tc.cost), p, engine.Hand)
			var picks []engine.CardID
			for _, name := range tc.pickers {
				picks = append(picks, g.NewCard(corpusCard(t, name), p, engine.Battlefield))
			}
			c := engine.NewScriptedController()
			c.QueueCardChoice(picks)
			if got := g.CastSpell(p, spell, c); got != tc.ok {
				t.Fatalf("CastSpell = %v, want %v", got, tc.ok)
			}
			for _, id := range picks {
				if g.Card(id).Tapped != tc.ok {
					t.Errorf("a convoking creature's tapped state = %v, want %v", g.Card(id).Tapped, tc.ok)
				}
			}
		})
	}
}
