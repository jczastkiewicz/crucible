// The opening-hand actions: a card whose keyword is MayEffectFromOpeningHand
// (Leylines, the Chancellors, Gemstone Caverns) may run its ability from the
// opening hand, after the mulligans and before the first turn
// (GameAction.runOpeningHandActions, GameAction.java:2507-2551).

//enginelint:allow card game ability control zone effecthelpers effect id

package engine

import (
	"fmt"
	"slices"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// openingHandAction is one opening-hand ability on a hand card.
type openingHandAction struct {
	card CardID
	face *compile.Face
	act  compile.OpeningHand
}

// RunOpeningHandActions is GameAction.runOpeningHandActions: starting with
// first and going round the table, each player is offered the hand cards that
// carry a MayEffectFromOpeningHand keyword (a card marked !PlayFirst not being
// offered to first), chooses which to run and in what order
// (ChooseCardsForEffect with no source and the whole offer as options; Java's
// chooseSaToActivateFromOpeningHand is one call per player, the ordered
// subset), and each chosen card's ability resolves with no stack
// (playSpellAbilityNoStack, so no priority and no state-based action check
// between them). It returns who takes the first turn: first, unless a chosen
// ability names BecomeStartingPlayer$, the last such player in turn order
// winning (GameAction.java:2543-2545). The !PlayFirst gate reads the player
// who was first before the pass, as Java does.
func RunOpeningHandActions(g *Game, reg *Registry, controller PlayerController, first PlayerID) (PlayerID, error) {
	newFirst := first
	players := g.Players()
	start := slices.Index(players, first)
	if start < 0 {
		return first, fmt.Errorf("engine: opening hand actions: %v is not seated", first)
	}
	for i := range players {
		pid := players[(start+i)%len(players)]
		var offer []openingHandAction
		var cards []CardID
		for _, id := range g.Zone(Hand, pid).Cards() {
			listed := false
			for _, face := range g.Card(id).traitFaces() {
				for _, act := range face.OpeningHand {
					if act.NotFirst && pid == first {
						continue
					}
					offer = append(offer, openingHandAction{card: id, face: &face, act: act})
					listed = true
				}
			}
			if listed {
				cards = append(cards, id)
			}
		}
		if len(cards) == 0 {
			continue
		}
		chosen := controller.ChooseCardsForEffect(g, pid, NoCard, cards, 0, len(cards))
		seen := map[CardID]bool{}
		for _, id := range chosen {
			if !slices.Contains(cards, id) || seen[id] {
				return first, fmt.Errorf("engine: opening hand actions: %v chose %v, not an offered card", pid, id)
			}
			seen[id] = true
		}
		for _, id := range chosen {
			for _, o := range offer {
				if o.card != id || g.Card(id).Zone != Hand {
					continue
				}
				api, ok := APIByName(o.act.Ability.Name)
				if !ok {
					return first, fmt.Errorf("engine: opening hand actions: unknown API %q", o.act.Ability.Name)
				}
				a := Ability{API: api, Source: id, Controller: pid, Params: o.act.Ability, Amounts: o.face.Amounts}
				if err := reg.Resolve(g, &a, controller); err != nil {
					return first, fmt.Errorf("engine: opening hand actions: %w", err)
				}
				if _, ok := o.act.Ability.Param("BecomeStartingPlayer"); ok {
					newFirst = pid
				}
			}
		}
	}
	return newFirst, nil
}
