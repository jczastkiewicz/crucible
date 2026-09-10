// Effect dispatch: the contract, and the array it is looked up in.

package engine

import (
	"errors"
	"fmt"
)

//go:generate go run ../../tools/genapitype -apitype ../../../forge-game/src/main/java/forge/game/ability/ApiType.java

// APIType names an ability API. The constants are generated from Forge's
// ApiType enum, so the set cannot drift from upstream without a build failure.
type APIType uint16

// String returns the API as a card script spells it.
func (a APIType) String() string {
	if int(a) >= numAPITypes {
		return fmt.Sprintf("APIType(%d)", uint16(a))
	}
	return apiNames[a]
}

// APIByName looks an API up by the name a script writes, and reports whether
// it is one. Exact match: an API that silently resolves to something else is
// a card doing the wrong thing rather than nothing.
func APIByName(name string) (APIType, bool) {
	for i, n := range apiNames {
		if n == name {
			return APIType(i), true
		}
	}
	return 0, false
}

// Effect resolves one ability.
//
// Implementations are stateless shared values, which is what Java measured
// rather than what this port assumes: ApiType's isStateLess parameter defaults
// to true and the count of constants passing false is zero. Everything an
// effect needs comes from the game and the ability it is handed, so there is
// no per-resolution allocation and no instance per card.
type Effect interface {
	Resolve(g *Game, a *Ability) error
}

// Ability is one resolvable ability on the stack. The fields land with the
// stack itself; for now it carries the API so dispatch has something to
// dispatch on.
type Ability struct {
	// API decides which Effect resolves this.
	API APIType
	// Source is the card the ability came from.
	Source CardID
	// Controller is who is resolving it, which is not always the source's
	// controller once control-changing effects are involved.
	Controller PlayerID
}

// Registry maps an API to the code that resolves it.
//
// An array rather than a map: dispatch sits under the stack resolution loop,
// so it is an index into an interface value with no allocation and no hashing
// (ADR-0008).
type Registry [numAPITypes]Effect

// ErrUnimplemented is what an unregistered API resolves to. During the port
// most of the array is empty, and a gap has to be a clear diagnostic naming
// the API rather than a nil dereference.
var ErrUnimplemented = errors.New("engine: no effect registered for API")

// Resolve dispatches an ability to its effect.
//
// A missing effect is an error and not a panic: it is a gap in the port, which
// the corpus coverage gate tracks, not an invariant breach (GO-7, ADR-0011).
// One unimplemented API must fail its game and no more.
func (r *Registry) Resolve(g *Game, a *Ability) error {
	if int(a.API) >= numAPITypes {
		return fmt.Errorf("%w: %s", ErrUnimplemented, a.API)
	}
	e := r[a.API]
	if e == nil {
		return fmt.Errorf("%w: %s", ErrUnimplemented, a.API)
	}
	return e.Resolve(g, a)
}

// Implemented is how many APIs have an effect. The corpus coverage report
// reads it, so progress through M6 is measured rather than estimated.
func (r *Registry) Implemented() int {
	n := 0
	for _, e := range r {
		if e != nil {
			n++
		}
	}
	return n
}

// NumAPIs is how many ability APIs Forge declares.
func NumAPIs() int { return numAPITypes }
