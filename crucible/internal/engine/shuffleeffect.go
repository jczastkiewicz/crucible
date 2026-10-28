// Shuffle: CR 701.20, M6's own next script-driven effect -- 54 real
// (AB|DB)$ Shuffle lines, 42 naming Defined$ and 3 naming ValidTgts$
// instead -- SpellAbilityEffect.getTargetPlayers(sa)'s own either/or
// contract, targetedOrDefinedPlayers (defined.go), millEffect's own first
// caller (milleffect.go) reused wholesale here.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/ShuffleEffect.java's
// resolve. Game.Shuffle (game.go) already exists -- mulligan.go's own real
// caller for London's own "shuffle the cards put back" step -- so this file
// is the thinnest M6 effect yet: read the player list, call it once per
// player.

package engine

//enginelint:allow id card game ability defined condition control zone

import "fmt"

// shuffleUnresolvedParams names ShuffleEffect's own params this port does
// not evaluate. Every one fails the whole line loudly (PORT-8/GO-7):
// none now (Optional$ asks the activator once per player, below).

// shuffleEffect resolves Mode$/DB$/AB$ Shuffle. ConditionPresent$/
// ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$ are resolved
// through subAbilityConditionMet (condition.go), the identical way every
// other M6 effect's own does.
type shuffleEffect struct{}

func (shuffleEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Shuffle: %w", err)
	}
	_, optional := a.Params.Param("Optional")
	for _, pid := range players {
		// ShuffleEffect.java:21: the activating player answers for each
		// player, not the player who shuffles.
		if optional && !controller.ConfirmEffect(g, a.Controller, a.Source) {
			continue
		}
		g.shuffleLibrary(controller, pid, a)
	}
	return nil
}

// shuffleLibrary is Player.shuffle(sa) (Player.java:1630-1645): the shuffle,
// then Mode$ Shuffled, always -- cause is the spell or ability that shuffles
// (nil for the rules' own shuffles, which a ShuffleFromEffect$ line skips).
func (g *Game) shuffleLibrary(controller PlayerController, owner PlayerID, cause *Ability) {
	g.Shuffle(Library, owner)
	g.checkShuffledTriggers(controller, owner, cause)
}
