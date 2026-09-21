// Effect dispatch: the contract, and the array it is looked up in.

package engine

import (
	"errors"
	"fmt"
)

// Effect resolves one ability.
//
// Implementations are stateless shared values, which is what Java measured
// rather than what this port assumes: ApiType's isStateLess parameter defaults
// to true and the count of constants passing false is zero. Everything an
// effect needs comes from the game and the ability it is handed, so there is
// no per-resolution allocation and no instance per card.
//
// Resolve takes the resolving player's controller too, threaded from
// ResolveStack's own parameter of the same name -- discardEffect's own
// Mode$ TgtChoose (discardeffect.go) is the first implementation that needs
// to ask a player anything mid-resolution rather than reading the game state
// outright; every effect before it ignores the parameter.
type Effect interface {
	Resolve(g *Game, a *Ability, controller PlayerController) error
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

// Resolve asks first, if a.Optional says to (CR 603.3d's own "may" trigger,
// WrappedAbility.resolve()'s own `if (decider != null) { if
// (!decider.getController().confirmTrigger(this)) return; }`, checked before
// anything else there too), then dispatches to its effect, then chains its
// own SubAbility$ if it names one (resolveSubAbility, subability.go) --
// AbilityUtils.resolveApiAbility's own "sa.resolve(); resolveSubAbilities(sa,
// game)" pairing, recursive through this same method for a chain more than
// one deep. A decline skips both -- the whole ability, chain included, never
// ran -- the identical early return WrappedAbility.resolve() gives before
// ever reaching its own playSpellAbilityNoStack call.
//
// A missing effect is an error and not a panic: it is a gap in the port, which
// the corpus coverage gate tracks, not an invariant breach (GO-7, ADR-0011).
// One unimplemented API must fail its game and no more -- including one
// reached only by chaining into a SubAbility$ this port has not implemented
// yet, the identical error a card naming it as its own top-level ability
// would already get. Checked after the optional confirm, not before: a
// declined "may" is real Magic's own outcome regardless of whether this port
// can run the ability behind it, so asking first and failing loudly only once
// something would actually try to run matches CR 603.3d more closely than
// erroring out a card a controller would have declined anyway.
func (r *Registry) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if a.Optional && !controller.ConfirmOptionalTrigger(g, a.Controller, a.Source) {
		return nil
	}
	if int(a.API) >= numAPITypes {
		return fmt.Errorf("%w: %s", ErrUnimplemented, a.API)
	}
	e := r[a.API]
	if e == nil {
		return fmt.Errorf("%w: %s", ErrUnimplemented, a.API)
	}
	if err := e.Resolve(g, a, controller); err != nil {
		return err
	}
	return r.resolveSubAbility(g, a, controller)
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
