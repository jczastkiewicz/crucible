// Defined$ resolution shared across M6's own script-driven effects --
// AbilityUtils.getDefinedPlayers's own two real corpus shapes this port can
// resolve without its full ability-context reference vocabulary (Targeted,
// Remembered, TriggeredPlayer, TriggeredController, ... -- game-state.md's
// "Not ported yet"). Neither drawEffect (draweffect.go) nor dealDamageEffect
// (dealdamageeffect.go) owns this outright, the identical "shared, so
// neither" reason amount.go's own resolveAmount lives apart from its first
// two callers.

package engine

import "fmt"

// definedPlayers resolves Defined$ to the players it names: "You" (the
// ability's own controller) and "Opponent"/"Player.Opponent" (every
// opponent). A player no longer in the game is skipped, matching Java's own
// `if (!p.isInGame()) continue`.
func definedPlayers(g *Game, controller PlayerID, defined string) ([]PlayerID, error) {
	var candidates []PlayerID
	switch defined {
	case "You":
		candidates = []PlayerID{controller}
	case "Opponent", "Player.Opponent":
		for _, pid := range g.Players() {
			if pid != controller {
				candidates = append(candidates, pid)
			}
		}
	default:
		return nil, fmt.Errorf("engine: Defined$ %q not resolvable yet", defined)
	}
	var players []PlayerID
	for _, pid := range candidates {
		if !g.Player(pid).Lost {
			players = append(players, pid)
		}
	}
	return players, nil
}
