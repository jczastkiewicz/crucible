// Trigger firing: CR 603, trimmed to the one mode this port can detect at
// all -- a permanent's own "when this enters the battlefield" trigger
// (Mode$ ChangesZone, Destination$ Battlefield).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// checkETBTriggers is CR 603.2's "look back in time" for a card that just
// entered the battlefield, called from every real "moves onto the
// battlefield" site this port has (permanentEffect.Resolve, attachEffect.Resolve
// -- castspell.go; Game.PlayLand -- land.go), not from Game.Move itself:
// Matches (valid.go) depends on game.go, so game.go cannot depend back on
// anything that calls it without a cycle enginelint is built to catch
// (ability.go's own doc comment gives the identical reason Ability moved out
// of effect.go).
//
// This is CR 603's own trigger vocabulary at its narrowest: only
// Mode$ ChangesZone with Destination$ Battlefield fires (an ETB trigger),
// keyed off compile.Face.Triggers -- M3's own trigger-line compilation,
// already typed the same way an ability line is (compile.Ability), just
// never read by the engine before now. Every other trigger mode (Attacks,
// Dies, Tapped, a spell being cast, ...) is a gap this does not close
// (game-state.md's "Not ported yet"); CR 603.3b's own simultaneous-trigger
// ordering does not apply either, since nothing this port can cause yet
// puts more than one ETB trigger on the stack from the same event.
//
// Only checked against entered's own Triggers -- "when CARDNAME enters"
// (ValidCard$ Card.Self), the overwhelming corpus-frequency shape. A
// permanent watching some OTHER permanent enter ("Whenever another creature
// enters the battlefield under your control...") is not checked at all:
// that needs walking every OTHER permanent's own triggers against this
// event, not this card's, a wider search this narrowest slice does not do.
//
// A matching trigger's own Execute$ sub-ability names the API it would run
// (triggerEffectAPI, below) -- pushed onto the stack the same way CastSpell
// pushes a cast spell, CR 603.3's own "triggered ability becomes an object on
// the stack." Resolving it is a different question: the 203 corpus-frequency
// effects a real trigger's own sub-ability needs are M6's job, so
// ResolveStack reports ErrUnimplemented for every one of them today
// (Registry.Resolve's own contract, effect.go) -- detecting and queuing a
// trigger correctly is this port's whole job here, the same "mechanism now,
// content later" shape permanentEffect/attachEffect's own Registry already
// established for casting.
func (g *Game) checkETBTriggers(entered CardID) {
	c := g.Card(entered)
	if c.Def == nil {
		return
	}
	for _, face := range c.Def.Faces {
		for _, t := range face.Triggers {
			if !strings.EqualFold(t.Name, "ChangesZone") {
				continue
			}
			if !hasZone(t, "Destination", "Battlefield") {
				continue
			}
			validCard, ok := t.Param("ValidCard")
			if !ok {
				continue
			}
			if !Matches(g, c, valid.Parse(validCard), c.Controller, entered) {
				continue
			}
			if api, ok := triggerEffectAPI(t); ok {
				g.PushAbility(Ability{API: api, Source: entered, Controller: c.Controller})
			}
		}
	}
}

// hasZone reports whether t's param key names zone among its comma-separated
// list of zones -- Destination$ Battlefield,Command among them, a real shape
// the corpus writes (a trigger that fires entering either zone).
func hasZone(t *compile.Ability, key, zone string) bool {
	v, ok := t.Param(key)
	if !ok {
		return false
	}
	for _, z := range strings.Split(v, ",") {
		if z == zone {
			return true
		}
	}
	return false
}

// triggerEffectAPI is the APIType a trigger's own Execute$ sub-ability would
// run -- the "DB$ <API>" record its SVar compiled into (compile.Ability's own
// Name field, the API for a Spell/DB record). Reports false for a trigger
// with no Execute key at all, or one naming an API string ApiType.java does
// not have (APIByName's own exact-match contract) -- neither is reachable
// against the real corpus today, but a card cannot be trusted not to be the
// first (PORT-8).
func triggerEffectAPI(t *compile.Ability) (APIType, bool) {
	for _, sub := range t.Subs {
		if !strings.EqualFold(sub.Key, "Execute") {
			continue
		}
		return APIByName(sub.Ability.Name)
	}
	return 0, false
}
