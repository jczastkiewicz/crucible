// Restricted mana (ADR-0046): what a unit of mana made under RestrictValid$
// may pay for, and the payment context that asks.
//
// Ported from forge-game/src/main/java/forge/game/spellability/
// AbilityManaPart.java's meetsManaRestrictions (:365-453) and
// ManaPool.tryPayCostWithColor's use of it (ManaPool.java:228-250).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// payContext is what a mana payment is for: the spell being cast, the ability
// being activated, or the effect an unless-cost is paid for. item carries the
// kind (spell, activated, triggered), the host card and the controller;
// cost is the mana cost being paid, for CostContainsX.
type payContext struct {
	item *Ability
	cost mana.Cost
	// resolving is a payment made while item resolves (an unless-cost):
	// AbilityManaPart.java:443-445 meets no ordinary restriction then.
	resolving bool
	// special names the special action a payment is for: "Foretelling" or
	// "Unlock" (RestrictValid$ Static.Foretelling, Static.Unlock); "" for a
	// spell, ability or effect.
	special string
	// cumulative is a payment for a cumulative upkeep cost
	// (SpellAbility.isCumulativeUpkeep).
	cumulative bool
}

// payingFor sets what the payments that follow are for and returns the call
// that restores the previous setting. A payment under no context may use no
// restricted unit.
func (g *Game) payingFor(ctx *payContext) func() {
	prev := g.payFor
	g.payFor = ctx
	return func() { g.payFor = prev }
}

// manaRestrictionSpecials are the RestrictValid$ alternatives that are not a
// SpellAbility.isValid string and that restrictionMet answers itself.
var manaRestrictionSpecials = map[string]bool{
	"nonSpell": true, "CostContainsX": true, "CumulativeUpkeep": true,
	"CantCastNonArtifactSpells": true, "CantCastSpellFromHand": true,
}

// staticSpecials are the special actions a Static.<name> alternative can
// name. The turn-face-up ones (isTurnFaceUp, MorphUp, ManifestUp) are judged
// but never paid for: this port has no turn-face-up special action, so mana
// restricted to it pays only its other alternatives.
var staticSpecials = map[string]bool{
	"Foretelling": true, "Unlock": true, "isTurnFaceUp": true, "MorphUp": true, "ManifestUp": true,
}

// manaRestrictionSupported reports whether every alternative of restrict is
// one restrictionMet can judge: a special above, or a SpellAbility kind with
// "+"-joined properties stackItemHasProperty reads. Anything else makes the
// mana ability refuse rather than make mana that pays wrongly (GO-7).
func (g *Game) manaRestrictionSupported(restrict string) bool {
	for _, alt := range strings.Split(restrict, ",") {
		if manaRestrictionSpecials[alt] {
			continue
		}
		head, rest, hasRest := strings.Cut(alt, ".")
		switch head {
		case "Spell", "SpellAbility", "Ability", "Activated", "Triggered", "Instant", "Sorcery":
		case "Static":
			for _, prop := range strings.Split(rest, "+") {
				if !staticSpecials[prop] && !g.hostCardPropertyOrType(prop) {
					return false
				}
			}
			continue
		default:
			return false
		}
		if !hasRest {
			continue
		}
		for _, prop := range strings.Split(rest, "+") {
			switch prop {
			case "YouCtrl", "OppCtrl", "Other":
			case "Kicked", "isCastFaceDown":
			default:
				if !g.hostCardPropertyOrType(prop) {
					return false
				}
			}
		}
	}
	return true
}

// restrictionMet is Mana.meetsManaRestrictions for one unit against the
// payment context: the alternatives in order, the first that answers wins.
func (g *Game) restrictionMet(u poolUnit, ctx *payContext) bool {
	if u.restrict == "" {
		return true
	}
	if ctx == nil || ctx.item == nil || u.src.card == NoCard {
		return false
	}
	item := ctx.item
	host := g.Card(item.Source)
	for _, alt := range strings.Split(u.restrict, ",") {
		switch alt {
		case "nonSpell":
			return !item.spell
		case "CumulativeUpkeep":
			if ctx.cumulative {
				return true
			}
			continue
		case "CostContainsX":
			if ctx.cost.CountX() > 0 {
				return true
			}
			continue
		case "CantCastNonArtifactSpells":
			return !item.spell || host.Type().Has(cardtype.Artifact)
		case "CantCastSpellFromHand":
			if !item.spell {
				return true
			}
			if host.castFrom != Hand {
				return true
			}
		}
		if ctx.resolving {
			return false
		}
		if head, rest, _ := strings.Cut(alt, "."); head == "Static" {
			if g.staticRestrictionMet(ctx, host, rest) {
				return true
			}
			continue
		}
		src := g.manaSourceCard(u.src)
		self := &Ability{Source: u.src.card, Controller: src.Controller()}
		if ok, err := g.stackItemMatches(item, alt, self); err == nil && ok {
			return true
		}
	}
	return false
}

// usableMana is the units the current payment may spend (ADR-0046).
func (g *Game) usableMana() func(poolUnit) bool {
	ctx := g.payFor
	return func(u poolUnit) bool { return g.restrictionMet(u, ctx) && !g.spellManaRestricted(u, ctx) }
}

// manaRestrictionParams are the params a Mode$ ManaRestriction line may carry
// that spellManaRestricted reads.
var manaRestrictionParams = map[string]bool{"validsource": true, "invertvalidsource": true}

// spellManaRestricted is StaticAbilityManaRestriction.manaRestriction
// (SpellAbility.allowsPayingWithShard, SpellAbility.java:447): a spell whose
// own card carries a live Mode$ ManaRestriction line may not be paid with
// mana from a source that line's ValidSource$ rejects (Imperiosaur, Myr
// Superion), or, with InvertValidSource$, accepts (Hogaak, Arisen
// Necropolis). Mana with no source card matches no ValidSource$, as Java's
// matchesValid(null) does.
func (g *Game) spellManaRestricted(u poolUnit, ctx *payContext) bool {
	if ctx == nil || ctx.item == nil || !ctx.item.spell {
		return false
	}
	host := g.Card(ctx.item.Source)
	if host.Def == nil {
		return false
	}
	src := g.manaSourceCard(u.src)
	return g.hostStatic(host, "ManaRestriction", manaRestrictionParams, func(h *Card, s *compile.Ability) bool {
		_, invert := s.Param("InvertValidSource")
		spec, ok := s.Param("ValidSource")
		matched := !ok || src != nil && Matches(g, src, valid.Parse(spec), h.Controller(), h.ID)
		return matched == invert
	})
}

// staticRestrictionMet is Static.<special>[+<host property>...] against a
// payment for a special action: the first property names the action, the
// rest are asked of its host card.
func (g *Game) staticRestrictionMet(ctx *payContext, host *Card, props string) bool {
	for _, prop := range strings.Split(props, "+") {
		switch {
		case prop == ctx.special && ctx.special != "":
		case staticSpecials[prop]:
			return false
		case !Matches(g, host, valid.Parse("Card."+prop), host.Controller(), host.ID):
			return false
		}
	}
	return ctx.special != ""
}
