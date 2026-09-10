// Counters on a card.

package engine

import "github.com/jczastkiewicz/crucible/pkg/collect"

// CounterType names a kind of counter.
//
// Java's CounterEnumType has 233 constants, and CounterType wraps it to allow
// keyword counters the enum does not list. A named string covers both without
// a generated enum that would still need the escape hatch, and card scripts
// write these names directly.
type CounterType string

// The counters the rules name most often. The type is open: any script-written
// name is a valid counter, which is what Java's keyword-counter escape hatch
// exists for.
const (
	P1P1    CounterType = "P1P1"
	M1M1    CounterType = "M1M1"
	Loyalty CounterType = "LOYALTY"
	Charge  CounterType = "CHARGE"
	Stun    CounterType = "STUN"
	Shield  CounterType = "SHIELD"
)

// Counters is what a card has on it.
//
// Insertion-ordered, so the same game produces the same report and the same
// event stream every run (GO-12). A count is never stored at zero: the rules
// say a permanent with no counters of a kind has none, not that it has zero,
// and the difference shows in "counters of any kind" checks.
type Counters struct {
	byType *collect.OrderedMap[CounterType, int]
}

// Count is how many counters of a kind the card has.
func (c *Counters) Count(t CounterType) int {
	if c.byType == nil {
		return 0
	}
	n, _ := c.byType.Get(t)
	return n
}

// Add puts counters on, and returns the new count. A negative delta removes,
// and the total never goes below zero -- Java clamps the same way, because
// "remove two counters" from a card with one removes one.
func (c *Counters) Add(t CounterType, delta int) int {
	if c.byType == nil {
		c.byType = collect.NewOrderedMap[CounterType, int](2)
	}
	n, _ := c.byType.Get(t)
	n += delta
	if n <= 0 {
		c.byType.Delete(t)
		return 0
	}
	c.byType.Set(t, n)
	return n
}

// Kinds returns every counter type present, in the order they were first put
// on the card.
func (c *Counters) Kinds() []CounterType {
	if c.byType == nil {
		return nil
	}
	return c.byType.Keys()
}

// Total is how many counters of all kinds the card has. Several cards count
// this rather than one kind.
func (c *Counters) Total() int {
	if c.byType == nil {
		return 0
	}
	sum := 0
	for _, n := range c.byType.Values() {
		sum += n
	}
	return sum
}

// Any reports whether the card has a counter of any kind.
func (c *Counters) Any() bool { return c.byType != nil && c.byType.Len() > 0 }

// clone returns an independent copy. A card with no counters -- which is most
// of them -- clones to another with none, allocating nothing.
func (c Counters) clone() Counters {
	if c.byType == nil {
		return Counters{}
	}
	return Counters{byType: c.byType.Clone()}
}
