// Effect dispatch: the contract, and the array it is looked up in.

package engine

//enginelint:allow id zone parts card player game ability control subability defined manapay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/mana"
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

//go:generate go run ../../tools/genregistry

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
//
// An ability naming UnlessCost$ takes resolveUnlessCost's own alternative
// route instead of the plain Resolve/resolveSubAbility pairing below --
// AbilityUtils.resolveApiAbility's own if/else between `sa.resolve()` and
// `handleUnlessCost(sa, game)`, the same branch point.
//
// A static trigger the effect set off at a site with no error return (an
// effect tapping a land for mana, activateabilityeffect.go) leaves its error
// pending on the Game; it is returned here, once the effect has run, as this
// ability's own failure (ADR-0020 decision 4).
func (r *Registry) Resolve(g *Game, a *Ability, controller PlayerController) error {
	err := r.resolve(g, a, controller)
	if g != nil {
		if pending := g.TakePendingError(); pending != nil {
			if err == nil {
				return pending
			}
			return errors.Join(err, pending)
		}
	}
	return err
}

// resolve is Resolve without the pending static-trigger error check.
func (r *Registry) resolve(g *Game, a *Ability, controller PlayerController) error {
	if g != nil {
		g.registry = r
	}
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
	if a.Params != nil {
		if unlessCost, ok := a.Params.Param("UnlessCost"); ok {
			return r.resolveUnlessCost(g, a, controller, e, unlessCost)
		}
	}
	if err := e.Resolve(g, a, controller); err != nil {
		return err
	}
	// A static trigger the effect set off failed: stop here, before the
	// sub-ability chain changes anything more (GO-7, ADR-0020 decision 4).
	if g != nil && g.pendingErr != nil {
		return nil
	}
	return r.resolveSubAbility(g, a, controller)
}

// resolveUnlessCost is CR's own "unless a cost is paid" gate --
// AbilityUtils.handleUnlessCost, ported apart from the ordinary
// Resolve/resolveSubAbility pairing above because Java's own version
// decides both whether the ability's body runs AND whether/when its own
// SubAbility$ chains, in one place, rather than falling through to the
// identical unconditional trailing call every other ability gets.
//
// Each of UnlessPayer$'s own players (definedPlayers, reused; the value is
// required explicitly -- an absent UnlessPayer$ defaults to
// "TargetedController" in Java, not resolved here, below) is asked
// ConfirmPayCost in turn and, on a yes, actually charged via PayManaCost
// (manapay.go) -- payCostToPreventEffect's own "decide, then pay" pairing,
// split the identical way every other mana decision on PlayerController
// already is. The ability's own body runs when paying did NOT happen
// (handleUnlessCost's own `alreadyPaid == isSwitched`, isSwitched false by
// default -- UnlessSwitched$'s own presence flips it, "pay to make it
// happen instead"). UnlessResolveSubs$ decides whether the chained
// SubAbility$ still runs regardless (absent, Java's own "Always") or only
// on one particular outcome ("WhenPaid"/"WhenNotPaid").
//
// Trimmed to the corpus's own one resolvable shape, and further to what is
// actually reachable at all: a pure-mana UnlessCost$ (cost.Parse's own Mana
// tokens alone -- no Sac<.../Discard<.../PayLife<.../... cost Part, no
// Tap/Untap/Mandatory/XMin token, and no X shard once parsed, each its own
// further mechanic with nowhere to route a mid-resolution "decide, then
// pay" question through) and an explicit UnlessPayer$ naming
// You/Player/Opponent/Player.Opponent (definedPlayers, reused). 56 of the
// corpus's 727 real UnlessCost$ lines resolve past this gate and are
// actually reachable by this port at all -- an activated ability's own
// Cost$-gated UnlessCost$ line composes with ActivateAbility
// (activateability.go) too, now that general activated-ability casting is
// built, an instant/sorcery's own top-level UnlessCost$ line still does not
// (CastSpell's own doc comment: "an instant or sorcery resolves into a
// script effect this port does not build"), and a line reached only through
// an unbuilt API's own SubAbility$/RepeatSubAbility$/... chain link (DB$
// Effect, DB$ Repeat, DB$ GenericChoice, DB$ DelayedTrigger, S:...
// AddTrigger$'s own dynamically granted trigger, none of them built) all
// fail loudly one hop up the call chain rather than here, PORT-8/GO-7's
// "skip the whole line" applied at whichever link in the chain the actual
// gap sits.
func (r *Registry) resolveUnlessCost(g *Game, a *Ability, controller PlayerController, e Effect, unlessCostText string) error {
	parsed := cost.Parse(unlessCostText)
	if !parsed.IsPureMana() {
		return fmt.Errorf("engine: UnlessCost$ %q not resolvable yet", unlessCostText)
	}
	manaCost, err := mana.Parse(strings.Join(parsed.Mana, " "))
	if err != nil || manaCost.CountX() > 0 {
		return fmt.Errorf("engine: UnlessCost$ %q not resolvable yet", unlessCostText)
	}

	payerSpec, ok := a.Params.Param("UnlessPayer")
	if !ok {
		return fmt.Errorf("engine: UnlessPayer$ default (TargetedController) not resolvable yet")
	}
	payers, err := definedPlayers(g, a.Controller, a.Source, payerSpec, a.refs())
	if err != nil {
		return fmt.Errorf("engine: UnlessPayer$: %w", err)
	}

	resolveSubs, hasResolveSubs := a.Params.Param("UnlessResolveSubs")
	execWhenPaid := !hasResolveSubs || resolveSubs == "WhenPaid"
	execWhenNotPaid := !hasResolveSubs || resolveSubs == "WhenNotPaid"
	_, switched := a.Params.Param("UnlessSwitched")

	paid := false
	for _, pid := range payers {
		if controller.ConfirmPayCost(g, pid, manaCost, a.Source) && g.PayManaCost(pid, manaCost, controller) {
			paid = true
		}
	}

	if paid == switched {
		if err := e.Resolve(g, a, controller); err != nil {
			return err
		}
	}
	if paid && execWhenPaid || !paid && execWhenNotPaid {
		return r.resolveSubAbility(g, a, controller)
	}
	return nil
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
