// What a card remembers.

package engine

import (
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/collect"
)

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

	// chosenPlayer, chosenColors and chosenNumber are Java Card's own
	// setChosenPlayer/setChosenColors/setChosenNumber: single values, each
	// overwritten by the next ChoosePlayer/ChooseColor/ChooseNumber and
	// cleared by Cleanup's ClearChosenPlayer$/ClearChosenColor$.
	chosenPlayer    PlayerID
	chosenColors    mana.Colors
	chosenNumber    int
	hasChosenNumber bool

	// chosenEvenOdd, chosenDirection, chosenType/chosenType2 and namedCards
	// are Card's setChosenEvenOdd, setChosenDirection, setChosenType/
	// setChosenType2 and addNamedCard: what ChooseEvenOdd, ChooseDirection,
	// ChooseType and NameCard record for later abilities to read.
	chosenEvenOdd   string
	chosenDirection string
	chosenType      string
	chosenType2     string
	namedCards      []string
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
// Forget removes e from the remembered list -- Java's removeRemembered, what
// ChooseCard's ForgetChosen$ and Cleanup's ForgetDefined$ call.
func (m *Memory) Forget(e EntityID) bool {
	if m.remembered == nil {
		return false
	}
	return m.remembered.Remove(e)
}

// forgetImprinted removes id from the imprinted list -- Java's
// removeImprintedCard, RepeatEach's UseImprinted$ undo.
func (m *Memory) forgetImprinted(id CardID) {
	if m.imprinted != nil {
		m.imprinted.Remove(id)
	}
}

// SetChosenPlayer records the player a ChoosePlayer picked; NoPlayer clears it.
func (m *Memory) SetChosenPlayer(p PlayerID) { m.chosenPlayer = p }

// ChosenPlayer is the player last recorded by SetChosenPlayer, NoPlayer when none.
func (m *Memory) ChosenPlayer() PlayerID { return m.chosenPlayer }

// SetChosenColors records the colors a ChooseColor picked; zero clears them.
func (m *Memory) SetChosenColors(c mana.Colors) { m.chosenColors = c }

// ChosenColors is the colors last recorded by SetChosenColors.
func (m *Memory) ChosenColors() mana.Colors { return m.chosenColors }

// SetChosenNumber records the number a ChooseNumber picked.
func (m *Memory) SetChosenNumber(n int) { m.chosenNumber, m.hasChosenNumber = n, true }

// ChosenNumber is the number last recorded by SetChosenNumber; ok is false
// when none was, since zero is a legal choice.
func (m *Memory) ChosenNumber() (n int, ok bool) { return m.chosenNumber, m.hasChosenNumber }

// SetChosenEvenOdd records ChooseEvenOdd's pick, "Odd" or "Even".
func (m *Memory) SetChosenEvenOdd(v string) { m.chosenEvenOdd = v }

// ChosenEvenOdd is ChooseEvenOdd's pick, "" when none was made.
func (m *Memory) ChosenEvenOdd() string { return m.chosenEvenOdd }

// SetChosenDirection records ChooseDirection's pick, "Left" or "Right".
func (m *Memory) SetChosenDirection(v string) { m.chosenDirection = v }

// ChosenDirection is ChooseDirection's pick, "" when none was made.
func (m *Memory) ChosenDirection() string { return m.chosenDirection }

// SetChosenType records ChooseType's pick; second is Java's chosenType2.
func (m *Memory) SetChosenType(v string, second bool) {
	if second {
		m.chosenType2 = v
		return
	}
	m.chosenType = v
}

// ChosenType is ChooseType's pick (second: chosenType2), "" when none.
func (m *Memory) ChosenType(second bool) string {
	if second {
		return m.chosenType2
	}
	return m.chosenType
}

// AddNamedCard records one NameCard pick; NamedCards lists them in order.
func (m *Memory) AddNamedCard(name string) { m.namedCards = append(m.namedCards, name) }

// NamedCards lists every NameCard pick, oldest first.
func (m *Memory) NamedCards() []string { return m.namedCards }

// ClearNamedCards is Cleanup's ClearNamedCard$.
func (m *Memory) ClearNamedCards() { m.namedCards = nil }

func (m Memory) clone() Memory {
	out := Memory{
		chosenPlayer:    m.chosenPlayer,
		chosenColors:    m.chosenColors,
		chosenNumber:    m.chosenNumber,
		hasChosenNumber: m.hasChosenNumber,
		chosenEvenOdd:   m.chosenEvenOdd,
		chosenDirection: m.chosenDirection,
		chosenType:      m.chosenType,
		chosenType2:     m.chosenType2,
		namedCards:      append([]string(nil), m.namedCards...),
	}
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
