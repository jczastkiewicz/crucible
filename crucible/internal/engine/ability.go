// The API vocabulary and the value that names one resolvable ability,
// kept apart from effect.go's dispatch machinery on purpose: dispatch
// (Effect.Resolve, Registry) needs *Game, and Game.stack (stack.go) needs
// to name Ability, so Ability and the APIType it carries have to sit below
// both in the dependency graph or the two would depend on each other
// (enginelint).

package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
)

//go:generate go run ../../tools/genapitype -apitype ../../../forge-game/src/main/java/forge/game/ability/ApiType.java

// APIType names an ability API. The constants are generated from Forge's
// ApiType enum, so the set cannot drift from upstream without a build failure.
type APIType uint16

// String returns the API as a card script spells it.
func (a APIType) String() string {
	if int(a) >= numAPITypes {
		return fmt.Sprintf("APIType(%d)", uint16(a))
	}
	return apiNames[a]
}

// APIByName looks an API up by the name a script writes, and reports whether
// it is one. Exact match: an API that silently resolves to something else is
// a card doing the wrong thing rather than nothing.
func APIByName(name string) (APIType, bool) {
	for i, n := range apiNames {
		if n == name {
			return APIType(i), true
		}
	}
	return 0, false
}

// Ability is one resolvable ability on the stack. The cost already paid and
// everything else CR 601-609 tracks per stack object beyond API/Source/
// Controller/Target land here once casting grows enough to fill them; today
// it carries just enough for dispatch, for the stack to know whose it is,
// and for the one target CastSpell's own Aura branch chooses at cast time
// (CR 601.2c).
type Ability struct {
	// API decides which Effect resolves this.
	API APIType
	// Source is the card the ability came from.
	Source CardID
	// Controller is who is resolving it, which is not always the source's
	// controller once control-changing effects are involved.
	Controller PlayerID
	// Target is the single card this ability was announced against at cast
	// time (CR 601.2c) -- NoCard when the ability has none. An Aura's own
	// APIAttach entry is the only thing that sets it today; this port has no
	// representation for a target that is a player (enchantSpec's own doc
	// comment) or for more than one target (no ability needing that shape
	// exists yet), so a single CardID is enough rather than a slice or an
	// EntityID.
	Target CardID
	// Targets is the general CR 601.2c/603.3b "choose targets" answer --
	// resolveTargets (targeting.go) fills it from a ValidTgts$/TargetMin$/
	// TargetMax$ triple right before this ability is pushed onto the stack
	// (pushTriggeredAbilities, trigger.go, the only pusher this port has),
	// asking the controller once via ChooseTargets. Nil for an ability
	// naming no ValidTgts$ at all -- the overwhelming majority -- and for
	// Target's own Aura shape above, which predates this field and is not
	// migrated onto it: two callers, two shapes, no single caller needing
	// both. A card or a player target is carried the same way here
	// (EntityID, unlike Target's own CardID-only shape), matching
	// AbilityUtils.getTargetCards/getTargetPlayers' own split -- an effect
	// naming Defined$ reads either half back through definedCards's/
	// definedPlayers's own "Targeted" case (defined.go); one naming
	// ValidTgts$ on its own line instead reads this field directly
	// (targetedOrDefinedCards, defined.go, SpellAbilityEffect.
	// getTargetCards(sa)'s own contract -- destroyEffect's/tapEffect's/
	// untapEffect's own first callers), since it IS that ability's own
	// resolveTargets answer, already populated against these same Params
	// before Resolve is ever called.
	Targets []EntityID
	// Params is the compiled sub-ability record backing this API call --
	// Defined$, NumCards$, and every other key an Effect's own Resolve reads
	// (compile.Ability.Param). Nil for a cast ability
	// (APIPermanentCreature/APIPermanentNoncreature/APIAttach): nothing
	// reads a param off casting itself, only off a card script's own
	// sub-ability -- a trigger's Execute$ (triggerEffectAPI, trigger.go)
	// today, an activated ability's own cost-paid effect once that exists.
	Params *compile.Ability
	// Amounts is the compile.Face's own SVar-defined amounts (compile.go)
	// Params' own host face carries -- resolveNamedAmount (amount.go) needs
	// it to resolve a named-SVar param value (dealDamageEffect's own
	// NumDmg$, dealdamageeffect.go, the first Effect to need one) the
	// identical way a trigger's own numeric params already do
	// (triggerCommonRequirementsMet, trigger.go). Nil wherever Params is,
	// for the identical reason.
	Amounts map[string]expr.Amount
	// Optional is CR 603.3d's own "may" triggered ability -- a trigger's own
	// OptionalDecider$ (WrappedAbility.java's own decider field), true only
	// for the "You" case (triggerEffectAPI's own doc comment, trigger.go,
	// has the corpus accounting). Registry.Resolve (effect.go) asks
	// PlayerController.ConfirmOptionalTrigger before running this ability's
	// body OR chaining its own SubAbility$ at all -- WrappedAbility
	// .resolve()'s own early return on a declined confirmTrigger, ported
	// directly. False for every ability this port pushes any other way (a
	// cast spell, a chained SubAbility$'s own child, resolveSubAbility,
	// subability.go) -- neither is ever optional on its own, a trigger's own
	// OptionalDecider$ never carrying onto what it chains into.
	Optional bool
	// TriggerRemembered is what a delayed or reflexive trigger remembered when
	// it was created (RememberObjects$, Java's Trigger.addRemembered),
	// carried onto the ability it runs and every sub-ability that ability
	// chains -- Java reads it off the root ability. Defined$
	// DelayTriggerRemembered[LKI] reads it (defined.go).
	TriggerRemembered []EntityID
	// Modes is a Charm's chosen modes, each with its own targets, picked as
	// the Charm was put on the stack (chooseCharmModes, charmeffect.go).
	Modes []Ability
	// modesErr is why chooseCharmModes could not pick modes for a Charm it
	// was asked about; the Charm is still pushed and fails with this error
	// when it resolves, since pushing has no error path of its own (GO-7).
	modesErr error
}

// abilityRefs is what Defined$ can name beyond the host card: the
// ability's own chosen targets and, for a delayed or reflexive trigger's
// ability, what that trigger remembered.
type abilityRefs struct {
	targets           []EntityID
	triggerRemembered []EntityID
}

// refs is a's own abilityRefs.
func (a *Ability) refs() abilityRefs {
	return abilityRefs{targets: a.Targets, triggerRemembered: a.TriggerRemembered}
}
