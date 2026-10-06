// Match: the series of games a match is, and who starts each one.

package engine

import "fmt"

// Match is the series state of forge.game.Match (Match.java:125-160): the
// outcome of each game played so far, from which the score, whether the
// match is over and who lost the last game follow. A seat is a PlayerID (1 to seats, 0 is NoPlayer), the
// same across the match's games: every game of a match seats the same
// players in the same order. A Match holds no Game, so each game of a batch
// stays its own goroutine's (GO-2).
type Match struct {
	seats      int
	gamesToWin int
	outcomes   []GameOutcome
}

// GameOutcome is one finished game: who won it. A drawn game has no winner
// (GameOutcome.getWinningPlayer is null).
type GameOutcome struct {
	Winner    PlayerID
	HasWinner bool
}

// NewMatch is a match of seats players, first to gamesToWin game wins
// (GameRules.getGamesToWinMatch). gamesToWin below one is an error: a match
// that is over before it starts is a configuration bug (GO-7).
func NewMatch(seats, gamesToWin int) (*Match, error) {
	if seats < 2 || gamesToWin < 1 {
		return nil, fmt.Errorf("engine: match needs at least 2 seats and 1 game to win, got %d and %d", seats, gamesToWin)
	}
	return &Match{seats: seats, gamesToWin: gamesToWin}, nil
}

// Record adds a finished game's outcome. Recording into a match that is
// already over is an error: Java's loop stops asking for games then
// (Match.isMatchOver, :171).
func (m *Match) Record(o GameOutcome) error {
	if m.IsOver() {
		return fmt.Errorf("engine: match is already over")
	}
	if o.HasWinner && (o.Winner < 1 || int(o.Winner) > m.seats) {
		return fmt.Errorf("engine: winner %d is not one of %d seats", o.Winner, m.seats)
	}
	m.outcomes = append(m.outcomes, o)
	return nil
}

// GamesPlayed is how many outcomes are recorded.
func (m *Match) GamesPlayed() int { return len(m.outcomes) }

// GamesWon is Match.getGamesWonBy for a seat.
func (m *Match) GamesWon(seat PlayerID) int {
	n := 0
	for _, o := range m.outcomes {
		if o.HasWinner && o.Winner == seat {
			n++
		}
	}
	return n
}

// IsOver is Match.isMatchOver: some seat has won gamesToWin games. A match is
// first to X wins, not first to X wins or Y games, so draws never end it.
func (m *Match) IsOver() bool {
	for s := 1; s <= m.seats; s++ {
		if m.GamesWon(PlayerID(s)) >= m.gamesToWin {
			return true
		}
	}
	return false
}

// Winner is Match.getWinner: the seat that took the match, once it is over.
func (m *Match) Winner() (PlayerID, bool) {
	if !m.IsOver() {
		return NoPlayer, false
	}
	return m.outcomes[len(m.outcomes)-1].Winner, true
}

// LastLoser is the player who decides who starts the next game
// (determineFirstTurnPlayer, GameAction.java:2434-2441): the first seat that
// did not win the last game. After a draw nobody won, so that is the first seat. ok
// is false before any game was played.
func (m *Match) LastLoser() (PlayerID, bool) {
	if len(m.outcomes) == 0 {
		return NoPlayer, false
	}
	last := m.outcomes[len(m.outcomes)-1]
	for s := 1; s <= m.seats; s++ {
		if !last.HasWinner || last.Winner != PlayerID(s) {
			return PlayerID(s), true
		}
	}
	return NoPlayer, false
}

// powerPlayName is the card whose owner starts the game (CR 103.2 via the
// Vanguard-era Power Play, GameAction.java:2403-2416).
const powerPlayName = "Power Play"

// DetermineFirstTurnPlayer is GameAction.determineFirstTurnPlayer
// (GameAction.java:2385-2447), in Java's order:
//
//  1. Puzzle: the first seated player, no question asked.
//  2. Archenemy (CR 904.6): the player with a scheme deck, no question asked.
//  3. Power Play: a player owning one in the Command zone. With several, Java
//     copies them from a HashSet and Collections.shuffle's them; that set has
//     no order, so seat order is shuffled here, with Collections.shuffle's
//     own algorithm on g's stream.
//  4. Otherwise the decider is chosen (game one: the coin flip, g.rand's
//     Int32n; later games: m.LastLoser) and PlayerController.ChooseStartingPlayer
//     gives the answer, isFirstGame true only for game one.
//
// m is nil, or has no games played, for game one. puzzle is the Puzzle game
// type, which a Game does not record.
func DetermineFirstTurnPlayer(g *Game, controller PlayerController, m *Match, puzzle bool) PlayerID {
	players := g.Players()
	if puzzle {
		return players[0]
	}
	for _, pid := range players {
		if g.Zone(SchemeDeck, pid).Len() > 0 {
			return pid
		}
	}
	var power []PlayerID
	for _, pid := range players {
		for _, id := range g.Zone(Command, pid).Cards() {
			c := g.Card(id)
			if c.Def.Name == powerPlayName && c.Owner == pid {
				power = append(power, pid)
				break
			}
		}
	}
	if len(power) > 0 {
		for i := len(power); i > 1; i-- {
			j := int(g.rand.Int32n(int32(i)))
			power[i-1], power[j] = power[j], power[i-1]
		}
		return power[0]
	}
	if loser, ok := lastLoser(m); ok {
		return controller.ChooseStartingPlayer(g, loser, false)
	}
	decider := players[g.rand.Int32n(int32(len(players)))]
	return controller.ChooseStartingPlayer(g, decider, true)
}

func lastLoser(m *Match) (PlayerID, bool) {
	if m == nil {
		return NoPlayer, false
	}
	return m.LastLoser()
}
