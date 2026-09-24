// Tap: CR 701.21, M6's own next script-driven effect after Destroy -- 577
// of the corpus's real (AB|DB)$ Tap lines once the corpus's own dominant
// shape (818 of 1,395, naming ETB$) is set aside: that shape is CR 614's own
// "enters the battlefield tapped" replacement effect, already resolved
// end to end by replacement.go's own tapAbilityResolvesTap/
// replacementTapsOnMove, never reaching Registry.Resolve or this file at
// all (a ReplaceWith$ target's own sub-ability runs by hand, the identical
// "not back through Resolve itself" reasoning drawReplaced's own doc
// comment gives, replacement.go). Of the 577 remaining, 413 name ValidTgts$
// and 151 name Defined$ instead -- SpellAbilityEffect.getTargetCards(sa)'s
// own either/or contract, targetedOrDefinedCards (defined.go).
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/TapEffect.java's
// resolve. Not ported: TriggerType.TapAll (2 real corpus T: lines, not
// worth building -- taptype.go's own identical reasoning for the same
// trigger, reached from a cost-paid tap rather than a script-driven one
// there).

package engine

import "fmt"

// tapUnresolvedParams names TapEffect's own params this port does not
// evaluate. Every one fails the whole line loudly (PORT-8/GO-7):
// CardChoices$/ChoiceAmount$/ChoicePrompt$/AnyNumber$ (10 combined) -- an
// interactive "choose N battlefield permanents matching a spec" this port's
// own PlayerController has no hook for past ChoosePermanentsToTap's own
// fixed-candidate-list shape (taptype.go, a cost, not an effect); Tapper$
// (2) -- a tap attributed to a player other than the activator, CR 701.21's
// own "tapped by" distinction nothing downstream reads yet; Condition$/
// ConditionDefined$ (0/7) -- condition.go's own subAbilityConditionMet
// would otherwise silently no-op a card naming either, dealDamageEffect's
// own identical reasoning; ETB$ (0 reaching here -- the file doc comment
// above has the reason) is listed anyway, a defensive reject in case a real
// line somehow combines it with a shape that reaches this path some other
// way, rather than silently tapping-without-a-trigger the wrong permanent.
var tapUnresolvedParams = [...]string{
	"CardChoices", "ChoiceAmount", "ChoicePrompt", "AnyNumber", "Tapper",
	"Condition", "ConditionDefined", "ETB",
}

// tapEffect resolves Mode$/DB$/AB$ Tap. ConditionPresent$/ConditionCompare$/
// ConditionCheckSVar$/ConditionSVarCompare$ are resolved through
// subAbilityConditionMet (condition.go), the identical way every other M6
// effect's own does.
type tapEffect struct{}

func (tapEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	for _, key := range tapUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: Tap: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	cards, err := targetedOrDefinedCards(source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: Tap: %w", err)
	}
	_, remTapped := a.Params.Param("RememberTapped")
	_, alwaysRem := a.Params.Param("AlwaysRemember")
	for _, id := range cards {
		c := g.Card(id)
		if c.Zone != Battlefield {
			continue
		}
		wasUntapped := !c.Tapped
		if remTapped && wasUntapped || alwaysRem {
			g.Card(a.Source).Memory.Remember(CardEntity(id))
		}
		if wasUntapped {
			c.Tapped = true
			g.checkTapsTriggers(controller, id, c.Controller(), false)
		}
	}
	return nil
}
