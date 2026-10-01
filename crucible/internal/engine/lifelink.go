// Lifelink (CR 702.15): damage dealt by a source with lifelink also makes its
// controller gain that much life.

//enginelint:allow id card game player gainlifeeffect control trigger

package engine

// applyLifelink is the lifelink half of GameAction.dealDamage
// (GameAction.java:2733-2736, CR 702.15e): for each damage source in the batch
// (first-seen order, GO-12), the damage it actually dealt -- already past
// prevention and replacement, since table records only that -- is summed, and
// when the sum is positive and the source has lifelink its controller gains it
// in one event. One gain per source per batch: a lifelinker dealing damage to
// two blockers triggers "whenever you gain life" once, not twice, and
// checkDamageTableTriggers calls this before any damage trigger is checked, as
// Java runs the gain inside the loop that deals the damage.
//
// The source's keywords are read as they stand: every caller runs this before
// the state-based actions that could remove the source, which is what Java's
// last-known-information copy of it preserves.
func (g *Game) applyLifelink(controller PlayerController, table damageTable) {
	var order []CardID
	dealt := map[CardID]int{}
	for _, e := range table {
		if e.Amount <= 0 {
			continue
		}
		if _, ok := dealt[e.Source]; !ok {
			order = append(order, e.Source)
		}
		dealt[e.Source] += e.Amount
	}
	for _, src := range order {
		c := g.Card(src)
		if c.HasKeyword("Lifelink") {
			g.gainLife(controller, c.Controller(), dealt[src], src)
		}
	}
}
