// Charm ChoiceRestriction$: a mode already chosen this turn, this game or
// during the controller's last combat cannot be chosen again.
//
// Ported from forge-game/src/main/java/forge/game/card/Card.java
// (getChosenModes, addChosenModes, resetChosenModeTurn, onEndOfCombat),
// SpellAbilityStackInstance.java:77-80 and CharmEffect.makePossibleOptions.

package engine

//enginelint:allow id game zone ability card

// charmRestricted is Card.getChosenModes: the modes the restriction forbids.
// A restriction other than the three Java knows is no restriction.
func (g *Game) charmRestricted(a *Ability) []string {
	restriction, ok := a.Params.Param("ChoiceRestriction")
	if !ok {
		return nil
	}
	lists := g.charmModes[charmModeKey{a.Source, a.Params}]
	switch restriction {
	case "ThisTurn":
		return lists.turn
	case "ThisGame":
		return lists.game
	case "YourLastCombat":
		return lists.lastCombat
	}
	return nil
}

// recordCharmMode is the SpellAbilityStackInstance constructor's
// addChosenModes call, which names only the first chained mode
// (sa.getSubAbility()): a Charm with CharmNum$ 2 restricts one mode.
func (g *Game) recordCharmMode(a *Ability, mode string) {
	if _, ok := a.Params.Param("ChoiceRestriction"); !ok {
		return
	}
	if g.charmModes == nil {
		g.charmModes = map[charmModeKey]charmModeLists{}
	}
	key := charmModeKey{a.Source, a.Params}
	lists := g.charmModes[key]
	lists.turn = append(lists.turn, mode)
	lists.game = append(lists.game, mode)
	if g.activePhase.IsCombat() {
		lists.combat = append(lists.combat, mode)
	}
	g.charmModes[key] = lists
}

// resetCharmModesTurn is Card.resetChosenModeTurn, run as a new turn starts.
func (g *Game) resetCharmModesTurn() {
	for k, l := range g.charmModes {
		l.turn = nil
		g.charmModes[k] = l
	}
}

// charmModesEndOfCombat is Card.onEndOfCombat for every battlefield card its
// active player controls: this combat's modes become the last combat's.
func (g *Game) charmModesEndOfCombat() {
	for k, l := range g.charmModes {
		c := g.Card(k.Host)
		if c.Zone != Battlefield || c.Controller() != g.activePlayer {
			continue
		}
		l.lastCombat, l.combat = l.combat, nil
		g.charmModes[k] = l
	}
}
