package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// tokenNames counts p's battlefield tokens by name.
func tokenNames(g *engine.Game, p engine.PlayerID) map[string]int {
	out := map[string]int{}
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if c := g.Card(id); c.IsToken {
			out[c.Def.Name]++
		}
	}
	return out
}

// ReplaceToken Type$ AddToken (ReplaceTokenEffect.java:60-87): Chatterfang
// adds that many Squirrels to the tokens you create; Academy Manufactor
// (Type$ ReplaceToken) turns one Clue, Food or Treasure into one of each.
func TestReplaceTokenAddsAndReplacesTokens(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card, script string
		amount       int
		want         map[string]int
	}{
		{"Chatterfang, Squirrel General", "c_a_treasure_sac", 2, map[string]int{"Treasure Token": 2, "Squirrel Token": 2}},
		{"Academy Manufactor", "c_a_clue_draw", 1, map[string]int{"Clue Token": 1, "Food Token": 1, "Treasure Token": 1}},
		{"Divine Visitation", "w_1_1_soldier", 2, map[string]int{"Angel Token": 2}},
		{"Divine Visitation", "c_a_treasure_sac", 1, map[string]int{"Treasure Token": 1}},
	} {
		t.Run(tc.card+" "+tc.script, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
			resolveLine(t, g, p, engine.NewScriptedController(),
				"DB$ Token | TokenScript$ "+tc.script+" | TokenAmount$ "+string(rune('0'+tc.amount)))
			got := tokenNames(g, p)
			for name, n := range tc.want {
				if got[name] != n {
					t.Errorf("tokens = %v, want %v", got, tc.want)
					break
				}
			}
			if len(got) != len(tc.want) {
				t.Errorf("tokens = %v, want exactly %v", got, tc.want)
			}
		})
	}
}

// Type$ ReplaceController in a Layer$ Control replacement (Crafty Cutpurse's
// shape): an opponent's token enters under your control, still theirs. The
// control layer applies before Divine Visitation's (Layer$ Other), which then
// sees a creature token you control and makes it an Angel (CR 616.1b).
func TestReplaceTokenControlLayerAppliesFirst(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(scriptDef(t, "Test Cutpurse", "Enchantment",
		"R:Event$ CreateToken | ActiveZones$ Battlefield | ValidToken$ Card.OppCtrl | ReplaceWith$ Mine | Layer$ Control | Description$ Mine.",
		"SVar:Mine:DB$ ReplaceToken | Type$ ReplaceController | NewController$ You"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Divine Visitation"), p, engine.Battlefield)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Token | TokenOwner$ Opponent | TokenScript$ w_1_1_soldier")
	if got := tokenNames(g, p); got["Angel Token"] != 1 || len(got) != 1 {
		t.Fatalf("p's tokens = %v, want one Angel", got)
	}
	angel := g.Zone(engine.Battlefield, p).Cards()
	for _, id := range angel {
		if c := g.Card(id); c.IsToken && c.Owner != other {
			t.Errorf("Angel owner = %v, want the opponent who created it", c.Owner)
		}
	}
}

// Type$ ReplaceToken with ValidChoices$ and TokenScript$ Chosen (Esix,
// Fractal Bloom, an Optional$ Layer$ Copy line): the tokens become copies of
// a creature the controller picks; declined, they are made as they were.
func TestReplaceTokenChosenCopyIsOptional(t *testing.T) {
	t.Parallel()

	for _, accept := range []bool{true, false} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, "Esix, Fractal Bloom"), p, engine.Battlefield)
		bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
		c := engine.NewScriptedController()
		c.QueueConfirmReplacementEffect(accept)
		if accept {
			c.QueueCardChoice([]engine.CardID{bears})
		}
		resolveLine(t, g, p, c, "DB$ Token | TokenScript$ c_a_treasure_sac | TokenAmount$ 2")
		got := tokenNames(g, p)
		want := map[string]int{"Treasure Token": 2}
		if accept {
			want = map[string]int{"Grizzly Bears": 2}
		}
		if len(got) != len(want) || got["Treasure Token"] != want["Treasure Token"] || got["Grizzly Bears"] != want["Grizzly Bears"] {
			t.Errorf("accept %v: tokens = %v, want %v", accept, got, want)
		}
	}
}

// Investigate makes its Clue through the token table too (InvestigateEffect
// extends TokenEffectBase): Academy Manufactor turns it into a Clue, a Food
// and a Treasure, and Doubling Season doubles it.
func TestInvestigateRunsCreateTokenReplacements(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card string
		want map[string]int
	}{
		{"Academy Manufactor", map[string]int{"Clue Token": 1, "Food Token": 1, "Treasure Token": 1}},
		{"Doubling Season", map[string]int{"Clue Token": 2}},
	} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		g.NewCard(corpusCard(t, tc.card), p, engine.Battlefield)
		resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Investigate")
		got := tokenNames(g, p)
		if len(got) != len(tc.want) {
			t.Errorf("%s: tokens = %v, want %v", tc.card, got, tc.want)
			continue
		}
		for name, n := range tc.want {
			if got[name] != n {
				t.Errorf("%s: tokens = %v, want %v", tc.card, got, tc.want)
				break
			}
		}
	}
}
