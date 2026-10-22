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
	if defined, has := s.Param("GainsAbilitiesOfDefined"); has {
		cards, err := definedCards(host, defined, abilityRefs{})
		if err != nil {
			return nil, false
		}
		sources = cards
	} else if spec, has := s.Param("GainsAbilitiesOf"); has {
		zones := []ZoneType{Battlefield}
		if raw, has := s.Param("GainsAbilitiesOfZones"); has {
			zones = nil
			for _, name := range strings.Split(raw, ",") {
				z, known := ZoneByName(strings.TrimSpace(name))
				if !known {
					return nil, false
				}
				zones = append(zones, z)
			}
		}
		parsed := valid.Parse(spec)
		for _, z := range zones {
			for _, pid := range g.Players() {
				for _, id := range g.Zone(z, pid).Cards() {
					if Matches(g, g.Card(id), parsed, host.Controller(), host.ID) {
						sources = append(sources, id)
					}
				}
			}
		}
	} else {
		return nil, true
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
