// Defined$ resolution shared across M6's own script-driven effects --
// AbilityUtils.getDefinedPlayers's/getDefinedCards's own real corpus shapes
// this port can resolve without its full ability-context reference
// vocabulary (Targeted, Remembered, TriggeredPlayer, TriggeredController,
// ... -- game-state.md's "Not ported yet"). No single effect owns this
// outright, the identical "shared, so neither" reason amount.go's own
// resolveAmount lives apart from its first two callers.

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

// definedCards resolves Defined$ to the cards it names, relative to the
// ability's own host card rather than its controller (pumpEffect's first
// caller): "Self" (the host itself, 1,094 of pumpEffect's own 1,147 real
// resolvable lines) and "Enchanted"/"Equipped" (what the host -- an Aura or
// an Equipment -- is currently attached to, Card.AttachedTo, empty rather
// than an error when nothing is, matching Java's own
// AbilityUtils.getDefinedCards returning an empty list for an unattached
// Aura/Equipment rather than failing the ability).
func definedCards(host *Card, defined string) ([]CardID, error) {
	switch defined {
	case "Self":
		return []CardID{host.ID}, nil
	case "Enchanted", "Equipped":
		if id, ok := host.AttachedTo(); ok {
			return []CardID{id}, nil
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("engine: Defined$ %q not resolvable yet", defined)
	}
}
