// RevealHand: CR 701.15 -- 50 real (AB|DB)$ RevealHand lines. Revealing
// itself has nothing to change: this port's own engine is already
// omniscient (no hidden-information model for any zone, GameState's own
// full-visibility fixture format), so Java's own game.getAction().reveal
// call -- an information-only UI/log event -- has no state-changing
// counterpart to port. What DOES change state is RememberRevealed$ (16
// real lines) and RememberRevealedPlayer$ (3), both plain Memory.Remember
// (memory.go) the identical way sacrificeAllEffect's/pumpAllEffect's/
// millEffect's own already are -- the reason this file exists at all rather
// than reducing to a silent no-op. Look$ (23, who does the looking) changes
// nothing this port tracks either way and is not read.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/RevealHandEffect.java's
// resolve.

package engine

//enginelint:allow id card game ability defined condition control valid zone parts

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// revealHandEffect resolves Mode$/DB$/AB$ RevealHand. ConditionPresent$/
// ConditionCompare$/ConditionCheckSVar$/ConditionSVarCompare$ are resolved
// through subAbilityConditionMet (condition.go), the identical way every
// other M6 effect's own does. Optional$ asks each revealing player's own
// controller (RevealHandEffect.java:42); there is no reveal event to fire,
// so only the Remember/Imprint bookkeeping acts.
type revealHandEffect struct{}

func (revealHandEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}

	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: RevealHand: %w", err)
	}

	_, remCards := a.Params.Param("RememberRevealed")
	_, imprint := a.Params.Param("ImprintRevealed")
	_, remPlayer := a.Params.Param("RememberRevealedPlayer")
	_, optional := a.Params.Param("Optional")
	if !remCards && !imprint && !remPlayer {
		return nil
	}

	var spec valid.Spec
	hasType := false
	if revealType, ok := a.Params.Param("RevealType"); ok {
		spec = valid.Parse(revealType)
		hasType = true
	}

	for _, pid := range players {
		if optional && !controller.ConfirmEffect(g, pid, a.Source) {
			continue
		}
		for _, cid := range g.Zone(Hand, pid).Cards() {
			if hasType && !Matches(g, g.Card(cid), spec, a.Controller, a.Source) {
				continue
			}
			if remCards {
				source.Memory.Remember(CardEntity(cid))
			}
			if imprint {
				source.Memory.Imprint(cid)
			}
		}
		if remPlayer {
			source.Memory.Remember(PlayerEntity(pid))
		}
	}
	return nil
}
