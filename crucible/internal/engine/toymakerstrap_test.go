package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// The Toymaker's Trap: the controller secretly picks a number from 1 to 5 not
// chosen before, an opponent guesses, and a wrong guess costs the guesser that
// many life (the guess is the chosen number while GuessWrong$ runs) and draws
// the controller a card; a right one sacrifices the Trap.
func TestToymakersTrapPunishesAWrongGuessAndRemembersTheNumber(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		guess        int
		wantLife     int
		wantTrapZone engine.ZoneType
	}{
		{"wrong guess", 2, 18, engine.Battlefield},
		{"right guess", 3, 20, engine.Graveyard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			libraryCards(t, g, p, 8)
			libraryCards(t, g, other, 8)
			trap := g.NewCard(corpusCard(t, "The Toymaker's Trap"), p, engine.Battlefield)
			g.SetTurnState(2, p, engine.Untap)
			c := engine.NewScriptedController()
			c.QueueNumberChoice(3)
			c.QueueNumberChoice(tc.guess)
			handBefore := len(g.Zone(engine.Hand, p).Cards())
			toStep(t, g, c, p, engine.Upkeep, nil)
			if got := g.Player(other).Life; got != tc.wantLife {
				t.Errorf("opponent life = %d, want %d", got, tc.wantLife)
			}
			if got := g.Card(trap).Zone; got != tc.wantTrapZone {
				t.Errorf("the Trap is in %v, want %v", got, tc.wantTrapZone)
			}
			if tc.guess != 3 && len(g.Zone(engine.Hand, p).Cards()) != handBefore+1 {
				t.Errorf("no card was drawn for the wrong guess")
			}
		})
	}
}
