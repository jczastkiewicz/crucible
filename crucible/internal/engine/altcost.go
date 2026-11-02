// Mode$ AlternativeCost statics for spells (CR 118.9): "you may pay <cost>
// rather than pay this spell's mana cost" -- the Bringers, Fireblast, Daze,
// Force of Will on the spell itself; Fist of Suns, Jodah, Leyline of
// Mutation, Rooftop Storm, Runeforge Champion, Kentaro and Dream Halls for
// other spells. Java builds one more SpellAbility per static whose cost
// replaces the mana cost (StaticAbilityAlternativeCost.alternativeCosts,
// copyWithManaCostReplaced, Cost.copyWithNoMana: every mana part goes, X
// included, the other parts stay), and marks it OptionalCost.AltCost only
// when the static's host is the spell itself (:84-89), which Count$AltCost
// reads (AbilityUtils.java:2045-2046).
//
// Hosts are the spell itself and the static-ability source zones
// (StaticAbilityAlternativeCost.java:22-23, staticHostsWith), each line live
// only in its EffectZone$ (StaticAbility.java:338-355, staticHostZoneOK).
// ValidSA$ is Spell, Spell.Self or Spell.<card property> (the property read
// on the spell's card, SpellAbilityProperty.java:324); ValidCard$ is matched
// on the spell, ValidPlayer$ on the caster, both against the host's
// controller (StaticAbilityAlternativeCost.java:108); absent means any.
// Conditions are read as any static's are (staticConditionsMet). The cost is
// one parseUnlessCost reads: its mana in place of the mana cost, its other
// parts paid with it, as Escape's are. Kentaro's XAlternative$
// Count$CardManaCost fixes X at the spell's mana value with no announce
// (PlaySpellAbility.java:457-465). A spell whose X the static's cost drops
// casts with X 0 (Fist of Suns on an X spell: xPaid is never set).
//
// A line from the hand is offered with the hand cast (castOptions); a
// spell's own EffectZone$ Graveyard line (Squee, Worldheart Phoenix) is its
// cast from the graveyard (graveyardAltCost; Java restricts that ability to
// the static's zone, StaticAbilityAlternativeCost.java:60-65), never from
// the hand. A line naming a param this port does not read (Announce$,
// ManaRestriction$, Named$, CostDesc$, an XAlternative$ other than
// Count$CardManaCost, a ConvertedManaCost cost) is not offered, never cast
// at the wrong price (GO-7). An Activated.* line with Cost$ 0 (the equip,
// power-up and unearth freebies) is freeActivationCost's. Not ported: the
// cycling and crew alternative costs and other-hosted lines on casts that are
// not from the hand (game-state.md).

package engine

//enginelint:allow id zone card game ability control staticability unlesscost castoptions valid

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// altCostParams are the params a Mode$ AlternativeCost line may carry here:
// its shape, its cost and the conditions staticConditionsMet reads.
var altCostParams = map[string]bool{
	"mode": true, "validsa": true, "validcard": true, "validplayer": true, "effectzone": true, "cost": true,
	"xalternative": true, "description": true, "stackdescription": true, "secondary": true, "condition": true,
	"ispresent": true, "presentcompare": true, "presentzone": true, "presentplayer": true, "presentdefined": true,
	"ispresent2": true, "presentcompare2": true, "presentzone2": true, "presentplayer2": true, "presentdefined2": true,
	"checksvar": true, "svarcompare": true,
}

// altCostAppliesTo reports whether s, a static of host, is an
// AlternativeCost line for pid casting c as a spell (ValidSA$, ValidCard$,
// ValidPlayer$; zone and conditions are the caller's). An Activated.* line
// (Heart of Kiran's crew, the equip freebies) is never one.
func (g *Game) altCostAppliesTo(pid PlayerID, c, host *Card, s *compile.Ability) bool {
	if !strings.EqualFold(s.Name, "AlternativeCost") {
		return false
	}
	sa, _ := s.Param("ValidSA")
	switch {
	case sa == "Spell":
	case sa == "Spell.Self":
		if host.ID != c.ID {
			return false
		}
	case strings.HasPrefix(sa, "Spell."):
		if !Matches(g, c, valid.Parse("Card."+strings.TrimPrefix(sa, "Spell.")), host.Controller(), host.ID) {
			return false
		}
	default:
		return false
	}
	if card, ok := s.Param("ValidCard"); ok {
		if card == "Card.Self" {
			if host.ID != c.ID {
				return false
			}
		} else if !Matches(g, c, valid.Parse(card), host.Controller(), host.ID) {
			return false
		}
	}
	if player, ok := s.Param("ValidPlayer"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, player)
		if !recognized || !matched {
			return false
		}
	}
	return true
}

// altCostOptions are the ways AlternativeCost statics let pid cast c from
// the hand: c's own lines first, then each static-ability host's in
// traitHosts order (GO-12), each line whose zone and conditions hold and
// whose cost pid can pay past its mana.
func (g *Game) altCostOptions(pid PlayerID, c *Card) []castOption {
	if c.Def == nil || c.Controller() != pid {
		return nil
	}
	var out []castOption
	for _, hid := range g.staticHostsWith(c.ID) {
		host := g.Card(hid)
		if host.Def == nil {
			continue
		}
		for _, face := range host.traitFaces() {
			for _, s := range face.Statics {
				if o, ok := g.altCostOption(pid, c, host, s); ok {
					out = append(out, o)
				}
			}
		}
	}
	return out
}

// graveyardAltCost is the cast option of c's own EffectZone$ Graveyard
// AlternativeCost line while pid owns c and it is in the graveyard.
func (g *Game) graveyardAltCost(pid PlayerID, c *Card) (castOption, bool) {
	if c.Zone != Graveyard || c.Owner != pid || c.Def == nil {
		return castOption{}, false
	}
	for _, face := range c.traitFaces() {
		for _, s := range face.Statics {
			if o, ok := g.altCostOption(pid, c, c, s); ok {
				return o, true
			}
		}
	}
	return castOption{}, false
}

// altCostOption is the cast option s, a static of host, gives pid for c, if
// it applies now and its cost reads and pays.
func (g *Game) altCostOption(pid PlayerID, c, host *Card, s *compile.Ability) (castOption, bool) {
	if !g.altCostAppliesTo(pid, c, host, s) || !staticParamsKnown(s, altCostParams) ||
		!staticHostZoneOK(host, s) || !g.staticConditionsMet(host, s) {
		return castOption{}, false
	}
	text, _ := s.Param("Cost")
	if strings.Contains(text, "ConvertedManaCost") {
		return castOption{}, false
	}
	o := castOption{hasAlt: true, altStatic: true, altOwn: host.ID == c.ID, altText: text}
	if xAlt, ok := s.Param("XAlternative"); ok {
		// Kentaro: Cost$ X, X the spell's mana value, never announced.
		if xAlt != "Count$CardManaCost" || strings.TrimSpace(text) != "X" {
			return castOption{}, false
		}
		o.alt = mana.MustParse(strconv.Itoa(c.CMC()))
		return o, true
	}
	uc, ok := parseUnlessCost(text)
	if !ok {
		return castOption{}, false
	}
	o.alt = mana.MustParse("0")
	if uc.hasMana {
		if uc.mana.CountX() > 0 {
			return castOption{}, false
		}
		o.alt = uc.mana
	}
	uc.hasMana, uc.mana, uc.manaTokens = false, mana.Cost{}, nil
	if hasNonManaParts(uc) {
		if !g.unlessPayable(pid, c.ID, uc) {
			return castOption{}, false
		}
		o.altExtra, o.hasAltExtra = uc, true
	}
	return o, true
}

// hasNonManaParts reports whether uc, its mana taken out, still asks for
// something.
func hasNonManaParts(uc unlessCost) bool {
	return len(uc.parsed.Parts) > 0
}

// altActivationParams are the params a Mode$ AlternativeCost line for an
// activated ability may carry: altCostParams plus the zones Highway Reaver's
// unearth freebie is written with.
var altActivationParams = func() map[string]bool {
	m := map[string]bool{"affectedzone": true}
	for k := range altCostParams {
		m[k] = true
	}
	return m
}()

// freeActivationCost reports whether a live AlternativeCost static lets pid
// pay {0} rather than the mana cost of activated (Forge Anew, Bruenor
// Battlehammer, Kili, Advancing the Spirit, Highway Reaver:
// "Activated.Equip", "Activated.PowerUp", "Activated.Unearth" lines with
// Cost$ 0 and a CheckSVar$ for the first activation of the turn). Java builds
// the alternative as the ability copied with its mana cost replaced
// (StaticAbilityAlternativeCost.alternativeCosts, Cost.copyWithNoMana), so
// the ability's other cost parts stay.
func (g *Game) freeActivationCost(pid PlayerID, card CardID, activated *Ability) bool {
	for _, hid := range g.staticHostsWith(card) {
		host := g.Card(hid)
		if host.Def == nil {
			continue
		}
		for _, face := range host.traitFaces() {
			for _, s := range face.Statics {
				if !strings.EqualFold(s.Name, "AlternativeCost") || !staticParamsKnown(s, altActivationParams) ||
					!staticHostZoneOK(host, s) || !g.staticConditionsMet(host, s) {
					continue
				}
				if text, _ := s.Param("Cost"); strings.TrimSpace(text) != "0" {
					continue
				}
				sa, ok := s.Param("ValidSA")
				if !ok || !strings.HasPrefix(sa, "Activated") {
					continue
				}
				if matched, recognized := g.spellAbilityMatches(activated, sa, host, host.Controller(), host.abilityAmounts(s)); !recognized || !matched {
					continue
				}
				if player, ok := s.Param("ValidPlayer"); ok {
					if m, rec := matchesPlayerSpec(g, pid, host.Controller(), host.ID, player); !rec || !m {
						continue
					}
				}
				return true
			}
		}
	}
	return false
}
