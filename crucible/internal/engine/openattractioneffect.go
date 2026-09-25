package engine

//enginelint:allow ability card condition control defined effecthelpers game id memory parts player zone zonemove

// openAttractionEffect is OpenAttractionEffect.java (Unfinity's "open an
// Attraction"): each named player puts the top Amount$ (default 1) cards of
// their own Attraction deck onto the battlefield.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/
// OpenAttractionEffect.java's resolve. Not resolved: no real corpus line
// carries a param this port would need to reject -- every one of the 20
// real OpenAttraction$ lines is either bare or Amount$ 2 -- so nothing is
// rejected here; a future line naming a param neither this file nor its
// test exercises still resolves the same way Java's own bare-bones resolve
// does, since Java itself reads only Amount$ and Remember$.
type openAttractionEffect struct{}

func (openAttractionEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return err
	}
	amount, err := optionalAmount(g, a, "OpenAttraction", "Amount", 1)
	if err != nil {
		return err
	}
	remember := hasParam(a, "Remember")

	var opened []CardID
	for _, pid := range players {
		if g.Player(pid).Lost {
			continue
		}
		deck := g.Zone(AttractionDeck, pid)
		for i := 0; i < amount; i++ {
			cards := deck.Cards()
			if len(cards) == 0 {
				break
			}
			id := cards[0]
			g.moveByEffect(controller, id, Battlefield, 0, pid, false)
			opened = append(opened, id)
			if remember {
				source.Memory.Remember(CardEntity(id))
			}
		}
	}
	g.checkChangesZoneAllTriggers(controller, opened, AttractionDeck, Battlefield)
	return nil
}
