package replay

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
)

// IDMap pairs a replayed Go game's cards with the recorded Java game's
// (ADR-0059): deck cards by seat and deck position when the game is set up,
// every later card at its creation, in order. It is the [Cards] a
// [Normalizer] reads.
type IDMap struct {
	g      *engine.Game
	toGo   map[int]engine.CardID
	toJava map[engine.CardID]int
	// created is the Java cards created during the game (a move from no
	// zone), in Java's order, with their names; paired is how many of each
	// name Go has claimed.
	created []Ref
	claimed map[int]bool
	// seated is the last Go id of a deck card: anything after it was created
	// during the game.
	seated engine.CardID
}

func newIDMap(g *engine.Game, log *Log) *IDMap {
	m := &IDMap{g: g, toGo: map[int]engine.CardID{}, toJava: map[engine.CardID]int{}, claimed: map[int]bool{}}
	seen := map[int]bool{}
	for _, s := range log.Header.Seats {
		for _, c := range s.Cards {
			seen[c.Card] = true
		}
	}
	for _, e := range log.Events() {
		if e.Type == KindMove && e.From == "None" && !seen[e.Card.Card] {
			seen[e.Card.Card] = true
			m.created = append(m.created, e.Card)
		}
	}
	return m
}

func (m *IDMap) pair(java int, id engine.CardID) {
	m.toGo[java] = id
	m.toJava[id] = java
	if id > m.seated {
		m.seated = id
	}
}

// GoCard is the Go card paired with the Java card java.
func (m *IDMap) GoCard(java int) (engine.CardID, bool) {
	id, ok := m.toGo[java]
	return id, ok
}

// JavaID is card's Java id. A card created during the game is paired at its
// first lookup -- its creation event, the first thing that names it -- with
// the first Java card created with the same name that no Go card has
// claimed.
func (m *IDMap) JavaID(card engine.CardID) (int, bool) {
	if j, ok := m.toJava[card]; ok {
		return j, true
	}
	if card <= m.seated {
		return 0, false
	}
	name := m.g.Card(card).Name()
	for _, c := range m.created {
		if !m.claimed[c.Card] && c.Name == name {
			m.claimed[c.Card] = true
			m.toGo[c.Card] = card
			m.toJava[card] = c.Card
			return c.Card, true
		}
	}
	return 0, false
}

// Name is card's name.
func (m *IDMap) Name(card engine.CardID) string { return m.g.Card(card).Name() }

// IsEffect reports whether card is an effect card.
func (m *IDMap) IsEffect(card engine.CardID) bool { return m.g.Card(card).IsEffect }

// IsCreature reports whether card is a creature now.
func (m *IDMap) IsCreature(card engine.CardID) bool {
	return m.g.Card(card).Type().Has(cardtype.Creature)
}

// CounterName is the counter type detail names in the game, spelled as
// Java's CounterType.toString: a keyword counter prints its keyword
// ("First Strike", CounterKeywordType.java:16-18,37-39), not the upper-case
// name Go keys it by.
func (m *IDMap) CounterName(detail engine.CounterDetail) string {
	t, _ := m.g.CounterTypeOf(detail)
	if k, ok := javaKeywordCounters[string(t)]; ok {
		return k
	}
	return string(t)
}

// javaKeywordCounters maps Go's counter key to Java's spelling for the
// keyword counters of rule 122.1b (CounterKeywordType.keywordCounter).
var javaKeywordCounters = func() map[string]string {
	out := map[string]string{}
	for _, k := range []string{"Flying", "First Strike", "Double Strike", "Deathtouch", "Decayed", "Exalted",
		"Haste", "Hexproof", "Indestructible", "Lifelink", "Menace", "Reach", "Shadow", "Trample", "Vigilance"} {
		out[strings.ToUpper(k)] = k
		out[strings.ToUpper(strings.ReplaceAll(k, " ", ""))] = k
		out[strings.ToUpper(strings.ReplaceAll(k, " ", "_"))] = k
	}
	return out
}()
