package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestUntapStepLetsTheControllerLeaveAPermanentTapped proves the "You may
// choose not to untap CARDNAME during your untap step." keyword
// (Untap.optionalUntap, Untap.java:174-196): the controller answers
// UntapOrLeaveTapped for it, and a permanent without the keyword untaps
// without a question.
func TestUntapStepLetsTheControllerLeaveAPermanentTapped(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		untap      bool
		wantTapped bool
	}{
		{"Untaps", true, false},
		{"StaysTapped", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newGame(t, "a", "b")
			p := g.Players()[0]
			def := creatureDefPT(t, "2", "2")
			def.Faces[0].Keywords = append(def.Faces[0].Keywords, "You may choose not to untap CARDNAME during your untap step.")
			choosy := g.NewCard(def, p, engine.Battlefield)
			plain := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
			g.Card(choosy).Tapped, g.Card(plain).Tapped = true, true

			c := engine.NewScriptedController()
			c.QueueBinary(tc.untap)
			g.StartTurn(p, c)

			if got := g.Card(choosy).Tapped; got != tc.wantTapped {
				t.Errorf("optional permanent tapped = %v, want %v", got, tc.wantTapped)
			}
			if g.Card(plain).Tapped {
				t.Error("plain permanent stayed tapped, want it untapped")
			}
		})
	}
}
