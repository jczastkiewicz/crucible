// Returning a permanent to its owner's hand as an activation cost:
// Return<1/CARDNAME|NICKNAME> (the self-reference shape, SelfSac/SelfExile's
// own sibling) and Return<N/Type> (the chosen-N-of-a-type shape, tapXType's
// own sibling for CR 602's "return N permanents of a type you control to
// their owner's hand" rather than "tap N"). ActivateAbility/
// ActivateManaAbility's own ActivationShape.SelfReturn/ReturnTypeN/
// ReturnTypeSpec (internal/cost) carry which shape a line names.
//
// Ported from
// forge-game/src/main/java/forge/game/cost/CostReturn.java: canPay/
// getMaxAmountX never exclude the ability's own host from the type-list
// branch's own candidates the way CostTapType.java's canTapSource does for
// tapXType -- a permanent already tapped by an earlier T component of the
// SAME cost is still a legal Return<N/Type> candidate, since nothing about
// being tapped stops it from also being returned to hand. This file's own
// returnTypeCandidates therefore takes no excludeSelf parameter at all,
// unlike tapTypeCandidates (taptype.go).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// returnCards actually returns each of ids to its own owner's hand --
// GameAction.moveToHand's own per-card use inside CostReturn.doPayment,
// mirroring exileCards's (exile.go) own shape at the one real difference:
// the destination zone. Takes no *Ability parameter for the identical
// reason exileCards's own doc comment gives -- 0 real Return<...> cost
// lines carry a Remember-shaped param this dispatch would need to read.
func returnCards(g *Game, controller PlayerController, ids []CardID) {
	var returned []CardID
	for _, id := range ids {
		c := g.Card(id)
		if c.Zone != Battlefield {
			continue
		}
		g.Move(id, Hand, c.Owner)
		g.checkReturnedTriggers(controller, id)
		returned = append(returned, id)
	}
	g.checkChangesZoneAllTriggers(controller, returned, Battlefield, Hand)
}

// returnTypeCandidates finds every one of controller's own battlefield
// permanents rawSpec matches -- CostReturn.java's own canPay/getMaxAmountX,
// ported: the identical semicolon-to-comma OR translation tapTypeCandidates
// (taptype.go) already needs, for the identical Cost-syntax reason. No
// CAN_TAP-shaped filter exists here at all -- CostReturn never restricts by
// tapped state, unlike CostTapType -- and no excludeSelf parameter either,
// this file's own doc comment has the reason.
func returnTypeCandidates(g *Game, pid PlayerID, source CardID, rawSpec string) []CardID {
	spec := valid.Parse(strings.ReplaceAll(rawSpec, ";", ","))
	var candidates []CardID
	for _, cid := range g.Zone(Battlefield, pid).Cards() {
		if !Matches(g, g.Card(cid), spec, pid, source) {
			continue
		}
		candidates = append(candidates, cid)
	}
	return candidates
}

// checkReturnedTriggers fires the leaves-the-battlefield triggers of a
// permanent that just went to its owner's hand (checkLeftToTriggers, leftto.go,
// which also holds the Hand-destination trigger shape and its other-watcher
// walk).
func (g *Game) checkReturnedTriggers(controller PlayerController, left CardID) {
	g.checkLeftToTriggers(controller, left, Hand)
}
