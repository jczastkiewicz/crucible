package engine

//enginelint:allow card game ability control

import (
	"fmt"
	"strings"
)

// regenerationEffect is RegenerationEffect.java: CR 701.16's own
// regeneration action (heal damage, tap, remove from combat) -- distinct
// from RegenerateEffect (regenerateeffect.go), which only grants the shield
// a later destruction spends.
//
// Every real corpus line reaches this through a permanent's own always-on
// "if this would be destroyed, regenerate it" replacement, never through an
// ordinary AB$/DB$ chain a card script could name on its own (0 real
// AB$/SP$ Regeneration lines exist): destroyReplacedByRegeneration
// (regeneration.go) builds the *Ability this Resolve reads directly from
// the ReplaceWith$ SVar the moment a real destruction would otherwise
// apply, calling this value directly rather than through
// Registry.Resolve -- the call sites that decide a destruction run outside
// any Registry.Resolve call, so Game.registry cannot be relied on to be set
// yet (destroyReplacedByRegeneration's own doc comment has the full
// reasoning). This is still the code NewRegistry registers for
// ApiType.Regeneration, so the registered API and the one the corpus's own
// 3 real lines run are the same code, not two copies that could drift.
//
// Ported from
// forge-game/src/main/java/forge/game/ability/effects/RegenerationEffect.java's
// resolve.
type regenerationEffect struct{}

func (regenerationEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	defined := "Self"
	if a.Params != nil {
		if v, ok := a.Params.Param("Defined"); ok {
			defined = v
		}
		// Every real corpus line is a bare `DB$ Regeneration | Defined$
		// ReplacedCard` -- any other param (RememberObjects$, SubAbility$, ...)
		// is refused rather than silently dropped (GO-7): the leading `DB`
		// key every compiled Ability carries is the one exception, not a
		// param a card script actually wrote.
		for _, p := range a.Params.Params {
			switch strings.ToLower(p.Key) {
			case "db", "defined":
			default:
				return fmt.Errorf("engine: Regeneration: %s$ not resolvable yet", p.Key)
			}
		}
	}
	// ReplacedCard -- Java's own "the object the replacement is actually
	// about" (100% of the corpus's 3 real lines) -- and Self both resolve to
	// a.Source here: every real line's own replacement carries ValidCard$
	// Card.Self, so the two are the same card. A future replacement whose
	// ValidCard$ names a DIFFERENT card would need a general "replacing
	// object" context this port does not carry (drawReplaced's/
	// gainLifeReplaced's own doc comments, replacement.go, note the same gap
	// for their own Event$s) -- any other Defined$ value is refused rather
	// than guessed at (GO-7).
	if !strings.EqualFold(defined, "Self") && !strings.EqualFold(defined, "ReplacedCard") {
		return fmt.Errorf("engine: Regeneration: Defined$ %s not resolvable yet", defined)
	}
	g.regenerateBody(controller, a.Source)
	return nil
}
