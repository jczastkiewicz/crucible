package engine

// regenerate is RegenerationEffect.java, the replacement a regeneration
// shield applies to one destruction (CR 701.15b): if id has a shield, one is
// used up and, instead of being destroyed, the permanent has all damage
// removed, is tapped (firing Taps triggers when it was untapped) and is
// removed from combat. It reports whether the destruction was replaced.
func (g *Game) regenerate(controller PlayerController, id CardID) bool {
	c := g.Card(id)
	if c.RegenShields <= 0 {
		return false
	}
	c.RegenShields--
	c.Damage.Clear()
	if !c.Tapped {
		c.Tapped = true
		g.checkTapsTriggers(controller, id, c.Controller(), false)
	}
	g.removeFromCombat(id)
	return true
}
