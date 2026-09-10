// What a card remembers.

package engine

import "github.com/jczastkiewicz/crucible/pkg/collect"

// Memory is the lists a card carries between the parts of one effect.
//
// `RememberChanged$`, `ImprintCards$` and `ChooseCard` all write here, and a
// later link in the same chain reads it back as `Remembered`, `Imprinted` or
// `ChosenCard`. The list outlives the spell, which is why almost every chain
// that writes one ends in `DB$ Cleanup | ClearRemembered$ True` -- a missing
// cleanup is a real defect, and five of them have been fixed upstream
// (docs/crucible/porting/card-script-defects.md).
//
// Remembered holds entities, not cards: `RememberObjects$ ChosenCard & Player.IsRemembered`
// puts a player in the same list as a card.
type Memory struct {
	remembered *collect.OrderedSet[EntityID]
	imprinted  *collect.OrderedSet[CardID]
	chosen     *collect.OrderedSet[CardID]
}

// Remember adds an entity, and reports whether it was new. Order is the order
// things were remembered, which is what a `RepeatEach$` over the list walks.
func (m *Memory) Remember(e EntityID) bool {
	if m.remembered == nil {
		m.remembered = collect.NewOrderedSet[EntityID](2)
	}
	return m.remembered.Add(e)
}

// Remembered returns what the card remembers, in order.
func (m *Memory) Remembered() []EntityID {
	if m.remembered == nil {
		return nil
	}
	return m.remembered.All()
}

// ClearRemembered empties the remembered list, which is what
// `ClearRemembered$ True` does.
func (m *Memory) ClearRemembered() { m.remembered = nil }

// Imprint adds a card to the imprinted list.
func (m *Memory) Imprint(id CardID) bool {
	if m.imprinted == nil {
		m.imprinted = collect.NewOrderedSet[CardID](2)
	}
	return m.imprinted.Add(id)
}

// Imprinted returns the imprinted cards, in order.
func (m *Memory) Imprinted() []CardID {
	if m.imprinted == nil {
		return nil
	}
	return m.imprinted.All()
}

// ClearImprinted empties the imprinted list.
func (m *Memory) ClearImprinted() { m.imprinted = nil }

// Choose adds a card to the chosen list.
func (m *Memory) Choose(id CardID) bool {
	if m.chosen == nil {
		m.chosen = collect.NewOrderedSet[CardID](2)
	}
	return m.chosen.Add(id)
}

// Chosen returns the chosen cards, in order.
func (m *Memory) Chosen() []CardID {
	if m.chosen == nil {
		return nil
	}
	return m.chosen.All()
}

// ClearChosen empties the chosen list.
func (m *Memory) ClearChosen() { m.chosen = nil }

// clone returns an independent copy. Each list is copied only when it exists,
// because the overwhelming majority of cards remember nothing.
func (m Memory) clone() Memory {
	var out Memory
	if m.remembered != nil {
		out.remembered = m.remembered.Clone()
	}
	if m.imprinted != nil {
		out.imprinted = m.imprinted.Clone()
	}
	if m.chosen != nil {
		out.chosen = m.chosen.Clone()
	}
	return out
}
