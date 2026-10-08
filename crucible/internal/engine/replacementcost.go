// The cost of a ReplaceWith$ ability. ReplacementHandler plays the ability with
// playSpellAbilityNoStack, which pays its Cost$ like any other ability; this
// port pays the one shape the corpus uses, "As CARDNAME enters, pay any amount
// of life" (Phyrexian Processor, Minion of the Wastes, Nameless Race):
// `AB$ StoreSVar | Cost$ Mandatory PayLife<X>`.

package engine

import (
	"fmt"
)

// replacementPayLifeX is the only Cost$ a ReplaceWith$ ability carries.
const replacementPayLifeX = "Mandatory PayLife<X>"

// payReplacementLifeX announces and pays X for a `Cost$ Mandatory PayLife<X>`
// replacement ability: the controller chooses any amount from 0 up to their
// life total, or XMax$ when the line names one (Nameless Race's Limit), none
// at all while a static forbids paying life (CR 119.4). The life is paid
// before the ability resolves and X is the ability's xManaCostPaid, which the
// line's Count$xPaid reads. A Cost$ of any other shape is left unpaid, as
// before.
func (g *Game) payReplacementLifeX(controller PlayerController, h *Card, a *Ability) error {
	if cost, ok := a.Params.Param("Cost"); !ok || cost != replacementPayLifeX {
		return nil
	}
	pid := a.Controller
	hi := g.Player(pid).Life
	if raw, ok := a.Params.Param("XMax"); ok {
		limit, ok := resolveNamedAmount(g, a.Amounts, h, raw)
		if !ok {
			return fmt.Errorf("engine: %q: XMax$ %q is not resolvable", h.Def.Name, raw)
		}
		hi = min(hi, limit)
	}
	hi = max(hi, 0)
	if hi > 0 && g.cantPayLife(pid, false, causeActivated) {
		hi = 0
	}
	n := controller.ChooseNumber(g, pid, h.ID, 0, hi)
	if n < 0 || n > hi {
		return fmt.Errorf("engine: %q: life paid %d is outside 0..%d", h.Def.Name, n, hi)
	}
	if n > 0 {
		g.Player(pid).Life -= n
		g.sink.Emit(Event{Kind: LifeChanged, Source: h.ID, Target: PlayerEntity(pid), Amount: -int32(n)})
		g.noteLifeLost(controller, pid, n)
	}
	xAnnounced{value: n, announced: true}.setOn(a)
	// Registry.Resolve pays a triggered AB$'s Cost$ unless told it is paid.
	a.costPaid = true
	return nil
}
