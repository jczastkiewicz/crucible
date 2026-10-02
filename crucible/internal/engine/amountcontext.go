// Ability-context amounts: `Targeted$CardPower`, `TriggeredCard$CardToughness`,
// `Remembered$Valid Creature`, `Sacrificed$CardManaCost` and the rest of
// AbilityUtils.calculateAmount's object-list branches (AbilityUtils.java:
// 640-715). The head names a list of cards the resolving ability carries --
// its targets, what it was triggered by, what its host remembers or imprinted,
// what its cost used up -- and the body measures them (handlePaid).
//
// The ability is Game.resolving, set by Registry.resolve: amounts are read
// while an ability resolves, and a sub-ability reads its root's lists because
// the chain copies them.

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// contextHeads are the amount heads that name a list of cards.
var contextHeads = map[string]bool{
	"Targeted": true, "TriggeredCard": true, "TriggeredAttacker": true, "TriggeredBlocker": true,
	"Remembered": true, "Imprinted": true, "Sacrificed": true, "Exiled": true, "Discarded": true,
}

// contextCards is the cards head names for the ability resolving now. A card
// that left the battlefield (a sacrificed creature) is read from its
// last-known information.
func (g *Game) contextCards(head string) ([]*Card, bool) {
	a := g.resolving
	if a == nil {
		return nil, false
	}
	var ids []CardID
	switch head {
	case "Targeted":
		for _, e := range a.Targets {
			if id, ok := e.AsCard(); ok {
				ids = append(ids, id)
			}
		}
	case "TriggeredCard":
		ids = nonNone(a.triggered.card)
	case "TriggeredAttacker":
		ids = nonNone(a.triggered.attacker)
	case "TriggeredBlocker":
		ids = nonNone(a.triggered.blocker)
	case "Remembered":
		if a.Source == NoCard {
			return nil, false
		}
		for _, e := range g.Card(a.Source).Memory.Remembered() {
			if id, ok := e.AsCard(); ok {
				ids = append(ids, id)
			}
		}
	case "Imprinted":
		if a.Source == NoCard {
			return nil, false
		}
		ids = g.Card(a.Source).Memory.Imprinted()
	case "Sacrificed":
		ids = a.paid.sacrificed
	case "Exiled":
		ids = a.paid.exiled
	case "Discarded":
		ids = a.paid.discarded
	default:
		return nil, false
	}
	cards := make([]*Card, 0, len(ids))
	for _, id := range ids {
		c := g.Card(id)
		if c.Zone != Battlefield {
			if snap := g.LKI(id); snap != nil && (head == "Sacrificed" || head == "TriggeredCard") {
				c = snap
			}
		}
		cards = append(cards, c)
	}
	return cards, true
}

func nonNone(id CardID) []CardID {
	if id == NoCard {
		return nil
	}
	return []CardID{id}
}

// contextValue is handlePaid over the cards head names: how many (`Amount`), how
// many match a valid string (`Valid <spec>`), or a per-card measure summed
// (`CardPower`), or its greatest, least or distinct values. false for a
// measure this port does not read, never a guess (GO-7).
func (g *Game) contextValue(head, body string) (int, bool) {
	cards, ok := g.contextCards(head)
	if !ok {
		return 0, false
	}
	switch {
	case strings.HasPrefix(body, "Amount"):
		return len(cards), true
	case strings.HasPrefix(body, "Valid "):
		var source *Card
		controller := NoPlayer
		if g.resolving != nil && g.resolving.Source != NoCard {
			source = g.Card(g.resolving.Source)
			controller = g.resolving.Controller
		}
		spec := valid.Parse(strings.TrimPrefix(body, "Valid "))
		n := 0
		for _, c := range cards {
			if source != nil && Matches(g, c, spec, controller, source.ID) {
				n++
			}
		}
		return n, source != nil
	}
	fold, perCard := sumFold, body
	switch {
	case strings.HasPrefix(body, "Least"):
		fold, perCard = minFold, strings.TrimPrefix(body, "Least")
	case strings.HasPrefix(body, "Greatest"):
		fold, perCard = maxFold, strings.TrimPrefix(body, "Greatest")
	case strings.HasPrefix(body, "Different"):
		fold, perCard = distinctFold, strings.TrimPrefix(body, "Different")
	}
	measure, ok := liveCardMeasure(perCard)
	if !ok {
		return 0, false
	}
	if len(cards) == 0 {
		return 0, true // handlePaid's own first line
	}
	values := make([]int, len(cards))
	for i, c := range cards {
		v, ok := measure(c)
		if !ok {
			return 0, false
		}
		values[i] = v
	}
	return fold(values), true
}

// liveCardMeasure is the per-card xCount of a card read while an ability
// resolves: power and toughness too (perCardMeasure refuses them, for it runs
// while Layer 7 is being rebuilt), mana value, a counter count, the number of
// colors.
func liveCardMeasure(head string) (func(*Card) (int, bool), bool) {
	switch head {
	case "CardPower":
		return func(c *Card) (int, bool) { return c.Power() }, true
	case "CardToughness":
		return func(c *Card) (int, bool) { return c.Toughness() }, true
	case "CardNumColors":
		return func(c *Card) (int, bool) { return c.Colors().Count(), true }, true
	}
	if m, ok := perCardMeasure(head); ok {
		return func(c *Card) (int, bool) { return m(c), true }, true
	}
	return nil, false
}
