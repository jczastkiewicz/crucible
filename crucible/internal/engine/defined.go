// Defined$ resolution shared across M6's own script-driven effects --
// AbilityUtils.getDefinedPlayers's/getDefinedCards's own real corpus shapes
// this port can resolve without its full ability-context reference
// vocabulary (Targeted, Remembered, TriggeredPlayer, TriggeredController,
// ... -- game-state.md's "Not ported yet"). No single effect owns this
// outright, the identical "shared, so neither" reason amount.go's own
// resolveAmount lives apart from its first two callers.

package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// definedPlayers resolves Defined$ to the players it names: "You" (the
// ability's own controller), "Opponent"/"Player.Opponent" (every opponent),
// "Player" (every player in the game, unfiltered -- AbilityUtils.
// getDefinedPlayers's own fallthrough `else` branch, `game.
// getPlayersInTurnOrder()`, reached because a bare "Player" matches none of
// its named cases; "Player.Opponent" does not fall into this branch at all,
// since it is Java's dotted-suffix filter applied to that same fallthrough
// set -- the identical opponents-only result "Opponent" gets directly,
// which is why both are one case here) and "TargetedPlayer"/"Targeted"
// (every PlayerEntity in targets -- resolveTargets's own answer,
// targeting.go -- filtered from a mixed EntityID slice even though no real
// ValidTgts$ line this port evaluates ever actually mixes cards and players
// in one target set, since nothing about Defined$'s own reading enforces
// that). A player no longer in the game is skipped, matching Java's own
// `if (!p.isInGame()) continue`.
func definedPlayers(g *Game, controller PlayerID, defined string, targets []EntityID) ([]PlayerID, error) {
	var candidates []PlayerID
	switch defined {
	case "You":
		candidates = []PlayerID{controller}
	case "Player":
		candidates = g.Players()
	case "Opponent", "Player.Opponent":
		for _, pid := range g.Players() {
			if pid != controller {
				candidates = append(candidates, pid)
			}
		}
	case "TargetedPlayer", "Targeted":
		for _, e := range targets {
			if pid, ok := e.AsPlayer(); ok {
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
// resolvable lines), "Enchanted"/"Equipped" (what the host -- an Aura or
// an Equipment -- is currently attached to, Card.AttachedTo, empty rather
// than an error when nothing is, matching Java's own
// AbilityUtils.getDefinedCards returning an empty list for an unattached
// Aura/Equipment rather than failing the ability), and "Targeted"/
// "ThisTargetedCard" (every CardEntity in targets -- resolveTargets's own
// answer, targeting.go).
func definedCards(host *Card, defined string, targets []EntityID) ([]CardID, error) {
	switch defined {
	case "Self":
		return []CardID{host.ID}, nil
	case "Enchanted", "Equipped":
		if id, ok := host.AttachedTo(); ok {
			return []CardID{id}, nil
		}
		return nil, nil
	case "Targeted", "ThisTargetedCard":
		var cards []CardID
		for _, e := range targets {
			if id, ok := e.AsCard(); ok {
				cards = append(cards, id)
			}
		}
		return cards, nil
	default:
		return nil, fmt.Errorf("engine: Defined$ %q not resolvable yet", defined)
	}
}

// targetedOrDefinedCards is SpellAbilityEffect.getTargetCards(sa)'s own
// either/or contract (getCards(false, "Defined", sa), forge-game's own
// SpellAbilityEffect.java): an ability that carries ValidTgts$ itself uses
// its own chosen targets -- a's own Targets field, resolveTargets's own
// answer (targeting.go) against these same Params, already populated before
// Resolve is ever called (pushTriggeredAbilities, trigger.go) -- and Defined$
// is not consulted at all, even if also present on the same line (destroy_
// evil.txt-shaped SubAbility$ chains sometimes carry both, the outer half
// dead). No ValidTgts$ at all falls back to Defined$, defaulting to "Self"
// the same way Java's own getParamOrDefault(definedParam, "Self") does --
// destroyEffect's/tapEffect's/untapEffect's own first caller, DestroyEffect.
// java/TapEffect.java/UntapEffect.java each calling the identical
// getTargetCards(sa) with no definedParam override.
//
// DestroyEffect.java's own dominant real shape (782 of 986 non-DestroyAll
// (AB|DB)$ Destroy lines) names ValidTgts$ alone; TapEffect.java's (413 of
// 577 non-ETB (AB|DB)$ Tap lines) does too -- the first two effects in this
// port to read a's own Targets field directly rather than only through
// definedCards's own "Targeted"/"ThisTargetedCard" case, Ability.Targets's
// own doc comment updated to match (ability.go).
// targetedOrDefinedPlayers is getTargetPlayers(sa)'s own mirror-image
// contract (getPlayers(false, "Defined", sa)), the player-shaped twin of
// targetedOrDefinedCards, above -- millEffect's own first caller
// (millEffect.java calling the identical getTargetPlayers(sa) with no
// definedParam override). Defaults to "You" rather than "Self" when neither
// ValidTgts$ nor Defined$ is present, Java's own
// getParamOrDefault(definedParam, "You") for a player list.
//
// Not ported: getPlayers' own trailing APNAP sort (StartingWith$/the active
// player). This port's own definedPlayers already returns "Player"'s/
// "Opponent"'s own candidates in Players()' own fixed seat order rather than
// turn order -- an established simplification every other effect naming
// Defined$ Player/Opponent already carries (scryEffect's/discardEffect's
// own doc comments), not a new gap Mill introduces.
func targetedOrDefinedPlayers(g *Game, controller PlayerID, a *compile.Ability, targets []EntityID) ([]PlayerID, error) {
	if _, ok := a.Param("ValidTgts"); ok {
		var players []PlayerID
		for _, e := range targets {
			if pid, ok := e.AsPlayer(); ok {
				players = append(players, pid)
			}
		}
		return players, nil
	}
	defined, ok := a.Param("Defined")
	if !ok {
		defined = "You"
	}
	return definedPlayers(g, controller, defined, targets)
}

func targetedOrDefinedCards(host *Card, a *compile.Ability, targets []EntityID) ([]CardID, error) {
	if _, ok := a.Param("ValidTgts"); ok {
		var cards []CardID
		for _, e := range targets {
			if id, ok := e.AsCard(); ok {
				cards = append(cards, id)
			}
		}
		return cards, nil
	}
	defined, ok := a.Param("Defined")
	if !ok {
		defined = "Self"
	}
	return definedCards(host, defined, targets)
}
