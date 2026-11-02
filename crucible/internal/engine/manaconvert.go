// Mode$ ManaConvert (StaticAbilityManaConvert.manaConvert) for casting a
// spell: a static that lets its player spend mana as though it were mana of
// any color ("AnyType->AnyColor") or of any type ("AnyType->AnyType") on the
// spells its ValidCard$ and ValidSA$ name. Java builds a ManaConversionMatrix
// the payment reads; here the same effect rewrites the cost the way a
// MayPlayIgnoreColor$/MayPlayIgnoreType$ grant does (anyColorCost,
// anyTypeCost), so the payment code stays one path.
//
// Activated-ability costs read the same lines (manaConvertForActivation).
// Not read: the per-color conversions (White->Red, C->AnyColor, ...), which
// need a conversion matrix in the payment rather than a cost rewrite; a line
// carrying one is skipped and the card pays as printed (game-state.md, "Not
// ported yet"). Optional$ (North Star) is asked as the spell's cost is paid
// (StaticAbilityManaConvert.java:41-47).

//enginelint:allow ability id zone card game valid player continuous staticability zonemove cantbecast saspec castspell castoptions control

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// manaConvertParams are the params a ManaConvert line may carry that this
// port evaluates; AffectedZone$ and the rest make the line unread.
var manaConvertParams = map[string]bool{
	"mode": true, "validplayer": true, "validcard": true, "validsa": true, "manaconversion": true, "optional": true,
	"effectzone": true, "condition": true, "description": true, "secondary": true,
}

// manaConversionKind is what a ManaConversion$ value does to a spell's cost:
// anyColor and anyType are the two shapes read, none anything else.
type manaConversionKind uint8

const (
	conversionNone manaConversionKind = iota
	conversionAnyColor
	conversionAnyType
)

// readManaConversion is the cost rewrite s's ManaConversion$ asks for.
func readManaConversion(s *compile.Ability) manaConversionKind {
	v, _ := s.Param("ManaConversion")
	switch strings.TrimSpace(v) {
	case "AnyType->AnyColor":
		return conversionAnyColor
	case "AnyType->AnyType":
		return conversionAnyType
	}
	return conversionNone
}

// manaConvertSupported reports whether a static line is a ManaConvert this
// port reads for a spell's cost: only evaluated params and a conversion
// readManaConversion knows. animate.go grants only such lines, so a grant is
// never inert.
func manaConvertSupported(s *compile.Ability) bool {
	if readManaConversion(s) == conversionNone {
		return false
	}
	for _, p := range s.Params {
		if !manaConvertParams[strings.ToLower(p.Key)] {
			return false
		}
	}
	return true
}

// manaConvertForSpell is the ManaConvert statics' verdict on pid casting card:
// mana of any type pays the whole cost (anyType), or a colored part
// (anyColor). AnyType wins when both apply.
//
// controller is nil while the cost is only measured; when the cost is paid it
// answers an Optional$ line, which applies only on a yes.
func (g *Game) manaConvertForSpell(pid PlayerID, card CardID, controller PlayerController) (anyType, anyColor bool) {
	return g.manaConvertFor(pid, card, &Ability{Source: card, Controller: pid, spell: true}, controller)
}

// manaConvertForActivation is cost rewritten by the same verdict for an
// activated ability of card: Java applies ManaConvert to every cost paid through
// PlaySpellAbility.payManaCost (:644-658), so ValidSA$ Activated lines
// (Manascape Refractor, Agatha's Soul Cauldron) and lines naming no ValidSA$
// (Chromatic Orrery) reach it.
func (g *Game) manaConvertForActivation(pid PlayerID, card CardID, a *Ability, cost mana.Cost) mana.Cost {
	switch anyType, anyColor := g.manaConvertFor(pid, card, a, nil); {
	case anyType:
		return anyTypeCost(cost)
	case anyColor:
		return anyColorCost(cost)
	}
	return cost
}

// manaConvertFor is the shared walk: every live ManaConvert line that
// applies to sa, the spell or activated ability of card being paid.
func (g *Game) manaConvertFor(pid PlayerID, card CardID, spell *Ability, controller PlayerController) (anyType, anyColor bool) {
	c := g.Card(card)
	// Prismatic (CardFactoryUtil.java:3953-3955) is a ManaConvert static on the
	// card itself: EffectZone$ Stack, ValidPlayer$ You, ValidCard$ Card.Self,
	// ValidSA$ Spell, AnyType->AnyColor. The cards gain it perpetually through
	// Animate Keywords$, which grants no keyword statics here, so the keyword
	// is read directly.
	if spell.spell && c.HasKeyword("Prismatic") {
		anyColor = true
	}
	for _, host := range g.staticHostsWith(card) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		for _, face := range h.traitFaces() {
			for _, s := range face.Statics {
				if !strings.EqualFold(s.Name, "ManaConvert") || !manaConvertSupported(s) ||
					!g.manaConvertApplies(pid, c, h, s, spell) {
					continue
				}
				if _, optional := s.Param("Optional"); optional && controller != nil {
					// StaticAbilityManaConvert.java:41-47: the host forgets,
					// asks, and on a yes remembers the spell (North Star's
					// ForgetOnCast$ Card.IsRemembered then ends its effect).
					h.Memory.ClearRemembered()
					if !controller.ConfirmEffect(g, pid, host) {
						continue
					}
					h.Memory.Remember(CardEntity(card))
				}
				if readManaConversion(s) == conversionAnyType {
					anyType = true
				} else {
					anyColor = true
				}
			}
		}
	}
	return anyType, anyColor
}

// manaConvertApplies is StaticAbilityManaConvert.checkManaConvert for a spell:
// ValidPlayer$, ValidCard$ and ValidSA$ each match when absent, and the line's
// Condition$ holds. A spec this port cannot recognize leaves the line unread.
func (g *Game) manaConvertApplies(pid PlayerID, c, host *Card, s *compile.Ability, spell *Ability) bool {
	// The card being cast is on the stack when its mana is paid, though Go-side
	// it is still in hand until the spell is put there; its own statics are read
	// from the Stack, every other host from where it is.
	// An activated ability's own host stays where it is.
	zone := host.Zone
	if host.ID == c.ID && spell.spell {
		zone = Stack
	}
	if !staticZoneOK(zone, s) || !g.staticOtherConditionsMet(host, s) {
		return false
	}
	if spec, ok := s.Param("ValidPlayer"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, spec)
		if !recognized || !matched {
			return false
		}
	}
	if spec, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(spec), host.Controller(), host.ID) {
		return false
	}
	if spec, ok := s.Param("ValidSA"); ok {
		matched, recognized := g.spellAbilityMatches(spell, spec, host, host.Controller(), host.abilityAmounts(s))
		return recognized && matched
	}
	return true
}
