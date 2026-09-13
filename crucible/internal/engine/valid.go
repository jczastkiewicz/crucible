// Valid-string matching: deciding whether a card satisfies a parsed
// internal/valid.Spec. internal/valid only parses the grammar (it must not
// import the engine, ADR-0003); this is the evaluation half its own doc
// comment says lands here.
//
// Ported from forge-game/src/main/java/forge/game/card/Card.java's
// isValid/hasProperty and the small slice of CardProperty.java's 2,135-line
// cardHasProperty this covers. CardProperty, CardStateProperty,
// PlayerProperty and SpellAbilityProperty together answer 928 residual
// property names (docs/crucible/porting/port-log/valid-strings.md); this is
// three of them (YouCtrl, OppCtrl, Self) plus the one fallthrough every
// chain shares -- a bare type, supertype or subtype word is a property in
// the same sense a keyword name is. The rest is M5-M6, corpus-frequency
// order, the same shape effect.go's Registry was always going to grow in
// (ADR-0011).
package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Matches decides whether c satisfies spec, from sourceController's
// perspective, with source as the card the spec is written on -- a card's
// own `Enchant`/`ValidCard`, an ability's `ValidTgts`. Both parameters are
// exactly `Ability.Controller`/`Ability.Source` where the spec comes from a
// resolving ability, but Matches does not require one: `cleanupDanglingAttachments`
// (action.go)'s eventual `Enchant`-restriction check would call this with
// the Aura's own controller and the Aura itself, no `Ability` in sight.
//
// An alternative matches when its base and every property do (`Spec`'s own
// doc comment: alternatives are OR, properties within one are AND). A `!`
// on the base negates that whole AND, not just the base -- Java's
// `testFailed` short-circuit, reproduced exactly in altMatches, because
// getting this backwards silently inverts every negated valid string in the
// corpus. A `!` on a property negates only that property, which is the
// simple case (`Card.hasProperty`'s own wrapper).
func Matches(c *Card, spec valid.Spec, sourceController PlayerID, source CardID) bool {
	for _, alt := range spec.Alternatives {
		if altMatches(c, alt, sourceController, source) {
			return true
		}
	}
	return false
}

func altMatches(c *Card, alt valid.Alternative, sourceController PlayerID, source CardID) bool {
	if !baseMatches(c, alt.Base.Name) {
		return alt.Base.Negated
	}
	for _, p := range alt.Properties {
		ok := propertyMatches(c, p.Name, sourceController, source)
		if p.Negated {
			ok = !ok
		}
		if !ok {
			return alt.Base.Negated
		}
	}
	return !alt.Base.Negated
}

// baseMatches is Card.isValid's own switch on the token before the first
// `.`. The named cases are Java's special ones; everything else falls
// through to a type/supertype/subtype check, the same fallthrough
// `getType().hasStringType(incR[0])` is in Java.
//
// Spell, Effect, Emblem and Boon never match: this port has nothing on the
// stack, no continuous-effect objects, no emblems and no boons yet, so a
// restriction naming one is a coverage gap, not a wrong answer -- the same
// as any SBA this port has not reached.
func baseMatches(c *Card, name string) bool {
	switch name {
	case "Permanent":
		return c.Type().IsPermanent()
	case "card", "Card":
		// Java excludes isImmutable() objects (Effect, Emblem, Boon) here.
		// Nothing this port creates is ever one, so every real card matches.
		return true
	case "Any":
		return c.Type().Has(cardtype.Creature) || c.Type().Has(cardtype.Planeswalker) || c.Type().Has(cardtype.Battle)
	case "Spell", "Effect", "Emblem", "Boon":
		return false
	default:
		return c.Type().HasStringType(name)
	}
}

// propertyMatches is the slice of CardProperty.cardHasProperty this port
// answers. Three branches are ported by name, `strings.HasPrefix` rather
// than `==` because Java's own chain tests with `startsWith` (a property
// can carry a suffix argument on other branches this port does not reach,
// and reproducing the match style is what keeps a future addition from
// silently behaving differently on the bare token) -- and a fourth
// fallthrough covers a bare type/supertype/subtype word used as a
// property, the same fallthrough CardState.hasProperty eventually reaches
// for one (`internal/cardtype.CoreTypeNames`'s own doc comment).
//
// Java's `YouCtrl`/`OppCtrl` compare against the controller
// `game.getChangeZoneLKIInfo` resolves, not `card.getController()`
// directly -- last-known-information for a card whose own zone change is
// mid-resolution. This port has no LKI tracking (game-state.md's "Not
// ported yet"), so this reads `c.Controller` as of now, which agrees with
// Java's LKI everywhere except the one moment a card's own leaving is what
// a property is trying to describe.
//
// `OppCtrl` is `controller.getOpponents().contains(sourceController)` in
// Java, which is team-aware. This port has no team system, so it reads as
// "controlled by anyone other than sourceController" -- correct for every
// game this port can play today (two players, or free-for-all with no
// teams), wrong only once a team variant exists to disagree with it.
func propertyMatches(c *Card, name string, sourceController PlayerID, source CardID) bool {
	switch {
	case strings.HasPrefix(name, "YouCtrl"):
		return c.Controller == sourceController
	case strings.HasPrefix(name, "OppCtrl"):
		return c.Controller != sourceController
	case strings.HasPrefix(name, "Self"):
		return c.ID == source
	default:
		return c.Type().HasStringType(name)
	}
}
