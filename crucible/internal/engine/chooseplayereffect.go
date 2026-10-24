package engine

//enginelint:allow id card game ability defined condition control parts effecthelpers

import "fmt"

// choosePlayerUnresolvedParams are ChoosePlayerEffect.java's params this
// port cannot honour yet: Protect$ (battle protector reassignment).
// ChooseSubAbility$/CantChooseSubAbility$ run after a player is, or is not,
// chosen (ChoosePlayerEffect.java:72-88). Random$ picks
// with Aggregates.random over Choices$ (ChoosePlayerEffect.java:47-48);
// Secretly$ writes the same chosen-player slot, only without updating the
// view (Card.setSecretChosenPlayer, Card.java:2127-2129).
var choosePlayerUnresolvedParams = [...]string{
	"Protect",
}

// choosePlayerEffect is ChoosePlayerEffect.java: each chooser
// (getTargetPlayers -- Defined$/ValidTgts$, default You) picks one player
// out of Choices$ (a Defined$-style player list, default every player) and
// the host records it (Memory.SetChosenPlayer, read back by Defined$
// ChosenPlayer). An empty choice list records nothing. 127 real lines.
type choosePlayerEffect struct{}

func (choosePlayerEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range choosePlayerUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: ChoosePlayer: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	choosers, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChoosePlayer: %w", err)
	}
	choicesParam, ok := a.Params.Param("Choices")
	if !ok {
		choicesParam = "Player"
	}
	choices, err := definedPlayers(g, a.Controller, a.Source, choicesParam, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ChoosePlayer: Choices$: %w", err)
	}
	_, random := a.Params.Param("Random")
	for _, pid := range choosers {
		if g.Player(pid).Lost {
			continue
		}
		chosen := NoPlayer
		switch {
		case len(choices) == 0:
		case random:
			chosen = choices[g.randomIndex(len(choices))]
		default:
			chosen = controller.ChoosePlayerForEffect(g, pid, a.Source, choices)
			if chosen == NoPlayer && hasParam(a, "Optional") {
				// ChoosePlayerEffect.java:50-59: Optional$ lets the chooser decline
				// (presence only), and a decline sets nothing: an earlier chosen
				// player stays.
				break
			}
			if err := checkChoice([]PlayerID{chosen}, choices, 1, 1); err != nil {
				return fmt.Errorf("engine: ChoosePlayer: %w", err)
			}
		}
		if chosen == NoPlayer {
			// The sub-ability that only fires when no player is chosen.
			if err := g.resolveAdditionalKey(a, controller, "CantChooseSubAbility"); err != nil {
				return err
			}
			continue
		}
		source.Memory.SetChosenPlayer(chosen)
		if _, ok := a.Params.Param("ForgetOtherRemembered"); ok {
			source.Memory.ClearRemembered()
		}
		if _, ok := a.Params.Param("RememberChosen"); ok {
			source.Memory.Remember(PlayerEntity(chosen))
		}
		// The sub-ability that only fires when a player is chosen.
		if err := g.resolveAdditionalKey(a, controller, "ChooseSubAbility"); err != nil {
			return err
		}
	}
	return nil
}
