// Restricted mana (ADR-0046): what a unit of mana made under RestrictValid$
// may pay for, and the payment context that asks.
//
// Ported from forge-game/src/main/java/forge/game/spellability/
// AbilityManaPart.java's meetsManaRestrictions (:365-453) and
// ManaPool.tryPayCostWithColor's use of it (ManaPool.java:228-250).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
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
	"nonSpell": true, "CostContainsX": true,
	"CantCastNonArtifactSpells": true, "CantCastSpellFromHand": true,
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
		default:
			return false
		}
		if !hasRest {
			continue
		}
		for _, prop := range strings.Split(rest, "+") {
			switch prop {
			case "YouCtrl", "OppCtrl", "Other":
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
	return func(u poolUnit) bool { return g.restrictionMet(u, ctx) }
}
