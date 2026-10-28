package engine

//enginelint:allow id card game player ability defined condition control amount zone effecthelpers

import "fmt"

// rearrangeTopOfLibraryUnresolvedParams: none; RearrangePlayer$ names who
// orders the cards (RearrangeTopOfLibraryEffect.java:96-100).
var rearrangeTopOfLibraryUnresolvedParams = [...]string{}

// rearrangeTopOfLibraryEffect is RearrangeTopOfLibraryEffect.java: for each
// target player (default You) the activator (or the RearrangePlayer$ player)
// orders the top NumCards$ cards of that library (OrderCardsForZone) and each is put back on top in that
// order, so the last one ends up on top -- Java's own moveToLibrary(next, 0)
// loop. MayShuffle$ then offers a shuffle (ConfirmEffect).
type rearrangeTopOfLibraryEffect struct{}

func (rearrangeTopOfLibraryEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if err := rejectUnreadCondition(a, a.Params.Name); err != nil {
		return err
	}
	for _, key := range rearrangeTopOfLibraryUnresolvedParams {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: RearrangeTopOfLibrary: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	raw, _ := a.Params.Param("NumCards")
	num, ok := resolveNamedAmount(g, a.Amounts, source, raw)
	if !ok {
		return fmt.Errorf("engine: RearrangeTopOfLibrary: NumCards$ %q not resolvable yet", raw)
	}
	players, err := targetedOrDefinedPlayers(g, a.Controller, a.Source, a.Params, a.refs())
	if err != nil {
		return fmt.Errorf("engine: RearrangeTopOfLibrary: %w", err)
	}
	_, mayShuffle := a.Params.Param("MayShuffle")
	// RearrangeTopOfLibraryEffect.java:96-100: the first RearrangePlayer$
	// player, else the activator, orders the cards and answers MayShuffle$
	// (Tahngarth's Glare: the targeted opponent orders the caster's library);
	// none resolving returns silently.
	orderer := a.Controller
	if raw, ok := a.Params.Param("RearrangePlayer"); ok {
		named, err := definedPlayers(g, a.Controller, a.Source, raw, a.refs())
		if err != nil {
			return fmt.Errorf("engine: RearrangeTopOfLibrary: RearrangePlayer$: %w", err)
		}
		if len(named) == 0 {
			return nil
		}
		orderer = named[0]
	}
	for _, p := range players {
		if g.Player(p).Lost {
			continue
		}
		library := g.Zone(Library, p).Cards()
		n := num
		if n > len(library) {
			n = len(library)
		}
		top := append([]CardID(nil), library[:n]...)
		if len(top) > 0 {
			ordered := controller.OrderCardsForZone(g, orderer, top, Library)
			if err := checkChoice(ordered, top, len(top), len(top)); err != nil {
				return fmt.Errorf("engine: RearrangeTopOfLibrary: %w", err)
			}
			for _, id := range ordered {
				g.MoveToLibraryTop(id, p)
			}
		}
		if mayShuffle && controller.ConfirmEffect(g, orderer, a.Source) {
			g.shuffleLibrary(controller, p, a)
		}
	}
	return nil
}
