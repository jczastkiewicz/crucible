// Draw: M6's own first script-driven effect, CR 120.3.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/DrawEffect.java's
// resolve, trimmed to the two params this port can resolve today.

package engine

import (
	"fmt"
	"strconv"
)

// drawEffect resolves Mode$/DB$ Draw. Two of Java's params are handled:
// NumCards$ (default 1, Java's own `sa.hasParam("NumCards") ? ... : 1`), only
// when it is a plain base-10 integer -- a "*"-shaped or SVar-driven amount
// needs AbilityUtils.calculateAmount, which needs an ability-context
// evaluator internal/expr does not have yet (valid.go's compareMatches doc
// comment already carries the identical gap for a valid-string's own numeric
// compare) -- and Defined$ (drawDefinedPlayers, below).
//
// Not ported: Upto (a player chooses how many, 0 to NumCards$), the optional
// draw's own confirmation prompt (OptionalDecider$), Reveal, and
// RememberDrawn -- none of PlayerController's methods this port has yet
// covers a numeric or reveal choice (control.go's own "90 of 110 methods"
// gap), so a script needing one of these fails loudly (below) rather than
// drawing the wrong number silently.
type drawEffect struct{}

func (drawEffect) Resolve(g *Game, a *Ability) error {
	n := 1
	if v, ok := a.Params.Param("NumCards"); ok {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("engine: Draw: NumCards$ %q is not a plain integer", v)
		}
		n = parsed
	}
	for _, key := range []string{"Upto", "OptionalDecider", "Reveal", "RememberDrawn"} {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Draw: %s$ not resolvable yet", key)
		}
	}
	defined, _ := a.Params.Param("Defined")
	players, err := drawDefinedPlayers(g, a.Controller, defined)
	if err != nil {
		return err
	}
	for _, pid := range players {
		g.DrawCards(pid, n)
	}
	return nil
}

// drawDefinedPlayers resolves Defined$ to the players it names -- the two
// corpus-frequent shapes this port can resolve without Java's full
// AbilityUtils.getDefinedPlayers reference vocabulary (Targeted, Remembered,
// TriggeredPlayer, TriggeredController, ...; game-state.md's "Not ported
// yet"): "You" (the ability's own controller) and "Opponent"/"Player.Opponent"
// (every opponent). A player no longer in the game is skipped, matching
// Java's own `if (!p.isInGame()) continue`.
func drawDefinedPlayers(g *Game, controller PlayerID, defined string) ([]PlayerID, error) {
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
		return nil, fmt.Errorf("engine: Draw: Defined$ %q not resolvable yet", defined)
	}
	var players []PlayerID
	for _, pid := range candidates {
		if !g.Player(pid).Lost {
			players = append(players, pid)
		}
	}
	return players, nil
}
