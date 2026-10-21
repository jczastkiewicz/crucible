package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// DestinationAlternative$ (ChangeZoneEffect.handleAltDest): the alternative
// zone replaces the destination when DestAltSVar$ allows it and the decider
// declines the primary one (the confirm is inverted: false picks the
// alternative).
func TestChangeZoneDestinationAlternative(t *testing.T) {
	t.Parallel()

	const base = "DB$ ChangeZone | Origin$ Library | Destination$ Graveyard | ChangeType$ Card | Mandatory$ True | DestinationAlternative$ Exile"
	for _, tc := range []struct {
		name    string
		extra   string
		confirm bool
		asks    bool
		want    engine.ZoneType
		wantErr bool
	}{
		{"declining the primary picks the alternative", "", false, true, engine.Exile, false},
		{"accepting the primary keeps it", "", true, true, engine.Graveyard, false},
		{"a mandatory SVar skips the question", " | DestAltSVar$ MANDATORY 1", false, false, engine.Exile, false},
		{"an SVar that fails its comparison keeps the primary", " | DestAltSVar$ 0", false, false, engine.Graveyard, false},
		{"an SVar that passes its comparison asks", " | DestAltSVar$ 2 | DestAltSVarCompare$ GE2", false, true, engine.Exile, false},
		{"an unresolvable SVar", " | DestAltSVar$ Bogus", false, false, engine.Library, true},
		{"a malformed comparison", " | DestAltSVar$ 1 | DestAltSVarCompare$ GE", false, false, engine.Library, true},
		{"an unresolvable comparison operand", " | DestAltSVar$ 1 | DestAltSVarCompare$ GEBogus", false, false, engine.Library, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGame(t)
			card := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
			c := engine.NewScriptedController()
			c.QueueCardChoice([]engine.CardID{card})
			if tc.asks {
				c.QueueConfirmEffect(tc.confirm)
			}
			_, err := castETBChain(t, g, p, etbChainDef(t, "Test Alt Dest", base+tc.extra), c)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if z := g.Card(card).Zone; z != tc.want {
				t.Errorf("card zone = %v, want %v", z, tc.want)
			}
		})
	}

	t.Run("a name that is no zone", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		card := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{card})
		line := "DB$ ChangeZone | Origin$ Library | Destination$ Graveyard | ChangeType$ Card | Mandatory$ True | DestinationAlternative$ Nowhere"
		if _, err := castETBChain(t, g, p, etbChainDef(t, "Test Alt Dest Bad", line), c); err == nil {
			t.Error("DestinationAlternative$ Nowhere resolved")
		}
	})
}
