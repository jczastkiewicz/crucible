// GainsAbilitiesOf$ / GainsAbilitiesOfDefined$: a Continuous static copies the
// activated abilities of other cards onto the cards it affects (Quicksilver
// Elemental, the Planeswalker-copying Grell Philosopher, Havengul Lich).
//
// Ported from forge-game/src/main/java/forge/game/staticability/
// StaticAbilityContinuous.java (:829-852, cardsGainedFrom :1027-1044).

//enginelint:allow ability amount card game id valid defined zone
package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// gainedAbilityGrants is the layer 6 half of cardsGainedFrom plus the copy loop:
// one trait grant per source card holding its printed activated abilities that
// GainsValidAbilities$ passes (an absent key passes all, CardTraitBase.java:
// 259-265), resolving their SVars against that card. ok is false for a spec
// this port cannot resolve and for GainsAbilitiesLimitPerTurn$, whose limit
// Java keeps on the source ability's shared restriction (GO-7: the static is
// then not applied rather than applied wrongly).
func (g *Game) gainedAbilityGrants(host *Card, s *compile.Ability) (out []traitGrant, ok bool) {
	var sources []CardID
	if _, has := s.Param("GainsAbilitiesOfDefined"); has {
		sources, ok = g.cardsGainedFrom(host, s, "GainsAbilitiesOfDefined")
	} else if _, has := s.Param("GainsAbilitiesOf"); has {
		sources, ok = g.cardsGainedFrom(host, s, "GainsAbilitiesOf")
	} else {
		return nil, true
	}
	if !ok {
		return nil, false
	}
	if _, has := s.Param("GainsAbilitiesLimitPerTurn"); has {
		return nil, false
	}
	validAbilities, hasValid := s.Param("GainsValidAbilities")
	for _, id := range sources {
		src := g.Card(id)
		if src.Def == nil {
			continue
		}
		face := src.Def.Faces[0]
		var kept []*compile.Ability
		for _, ab := range face.Abilities {
			if ab.Record != compile.Activated {
				continue
			}
			if hasValid {
				sa := &Ability{Source: id, Controller: src.Controller(), Params: ab, activated: true}
				matched, known := g.spellAbilityMatches(sa, validAbilities, host, host.Controller(), host.abilityAmounts(s))
				if !known {
					return nil, false
				}
				if !matched {
					continue
				}
			}
			kept = append(kept, ab)
		}
		if len(kept) > 0 {
			out = append(out, traitGrant{abilities: kept, amounts: face.Amounts})
		}
	}
	return out, true
}

// cardsGainedFrom is StaticAbilityContinuous.cardsGainedFrom
// (StaticAbilityContinuous.java:1027-1044): a *Defined param names cards
// through Defined$, any other is a valid string matched in
// GainsAbilitiesOfZones$ (default the battlefield). false is a spec or zone
// this port cannot resolve.
func (g *Game) cardsGainedFrom(host *Card, s *compile.Ability, param string) ([]CardID, bool) {
	raw, _ := s.Param(param)
	if strings.Contains(param, "Defined") {
		cards, err := definedCards(host, raw, abilityRefs{})
		return cards, err == nil
	}
	zones := []ZoneType{Battlefield}
	if list, has := s.Param("GainsAbilitiesOfZones"); has {
		zones = nil
		for _, name := range strings.Split(list, ",") {
			z, known := ZoneByName(strings.TrimSpace(name))
			if !known {
				return nil, false
			}
			zones = append(zones, z)
		}
	}
	parsed := valid.Parse(raw)
	var sources []CardID
	for _, z := range zones {
		for _, pid := range g.Players() {
			for _, id := range g.Zone(z, pid).Cards() {
				if Matches(g, g.Card(id), parsed, host.Controller(), host.ID) {
					sources = append(sources, id)
				}
			}
		}
	}
	return sources, true
}

// gainedTriggerGrants is GainsTriggerAbsOf$ (StaticAbilityContinuous.java:
// 867-878): every trigger of each source card, as Koh, the Face Stealer and
// Idris, Soul of the TARDIS gain those of the card they exiled, each read
// against its own card's SVars. Java filters them by nothing.
func (g *Game) gainedTriggerGrants(host *Card, s *compile.Ability) ([]traitGrant, bool) {
	if _, has := s.Param("GainsTriggerAbsOf"); !has {
		return nil, true
	}
	sources, ok := g.cardsGainedFrom(host, s, "GainsTriggerAbsOf")
	if !ok {
		return nil, false
	}
	var out []traitGrant
	for _, id := range sources {
		src := g.Card(id)
		if src.Def == nil || len(src.Def.Faces[0].Triggers) == 0 {
			continue
		}
		face := src.Def.Faces[0]
		out = append(out, traitGrant{triggers: face.Triggers, amounts: face.Amounts})
	}
	return out, true
}
