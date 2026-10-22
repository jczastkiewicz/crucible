// ExchangeLife: CR 119.10, M6's own next script-driven effect -- 6 real
// (AB|DB)$ ExchangeLife lines, every one naming ValidTgts$ (a two-target
// "two target players exchange life totals" shape, or a one-target
// "exchange life totals with target opponent/player" shape against the
// ability's own activator) -- SpellAbilityEffect.getTargetPlayers(sa)'s own
// contract, targetedOrDefinedPlayers (defined.go), millEffect's own first
// caller (milleffect.go) reused wholesale here.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/LifeExchangeEffect.java's
// resolve: whichever of the two players has MORE life loses the
// difference, the other gains it -- CR 119.10's own net effect of an
// exchange, reusing setLifeEffect's own gain/loss halves directly (Player.
// setLife's own CR 119.5 dispatch, setlifeeffect.go's own doc comment has
// the reason a life change this port ports at all always splits into
// exactly those two shapes) rather than a third, redundant copy of the
// identical gainLifePrevented/gainLifeReplaced/checkLifeGainedTriggers
// sequence.

package engine

//enginelint:allow id card game player ability defined condition control setlifeeffect effecthelpers

import "fmt"

// RememberOwnLoss$ and RememberDifference$ remember a number (Memory.
// RememberNumber): the life the activator lost, and p1's life less p2's after
// the exchange (LifeExchangeEffect.java:73-80), which reads negative -- the
// player who had more life now has less -- so the corpus reads it through
// Abs (Profane Transfusion).
//
// Condition$/ConditionDefined$ (0/0) stay refused through rejectUnreadCondition.

// exchangeLifeEffect resolves Mode$/DB$/AB$ ExchangeLife. ConditionPresent$/
// ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$ are resolved
// through subAbilityConditionMet (condition.go), the identical way every
// other M6 effect's own does.
type exchangeLifeEffect struct{}

func (exchangeLifeEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: ExchangeLife: %w", err)
	}

	var p1, p2 PlayerID
	switch len(players) {
	case 0:
		return nil
	case 1:
		p1, p2 = a.Controller, players[0]
	default:
		p1, p2 = players[0], players[1]
	}
	loser, gainer := p1, p2
	diff := g.Player(p1).Life - g.Player(p2).Life
	if diff < 0 {
		diff = -diff
		loser, gainer = p2, p1
	}
	if p1 != p2 && diff != 0 {
		before := g.Player(loser).Life
		setPlayerLife(g, controller, a.Source, loser, before-diff)
		setPlayerLife(g, controller, a.Source, gainer, g.Player(gainer).Life+diff)
		if lost := before - g.Player(loser).Life; lost > 0 && hasParam(a, "RememberOwnLoss") && loser == a.Controller {
			source.Memory.RememberNumber(lost)
		}
	}
	if hasParam(a, "RememberDifference") {
		source.Memory.RememberNumber(g.Player(loser).Life - g.Player(gainer).Life)
	}
	return nil
}
