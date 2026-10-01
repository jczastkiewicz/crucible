// Mode$ CantBeCast (StaticAbilityCantBeCast.cantBeCastAbility): a static that
// stops a player casting a card, for every card and caster its ValidCard$ and
// Caster$ name.

//enginelint:allow id zone card game valid player continuous staticability zonemove

package engine

import (
	"slices"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// cantBeCastParams are the params a CantBeCast line may carry that this port
// evaluates; any other (cmcGT$, CheckSVar$, a ValidCard$-filtered
// NumLimitEachTurn$, ...) makes the line unresolvable, and it is not applied
// (GO-7). The cosmetic ones (Description$, Secondary$, ...) change nothing.
var cantBeCastParams = map[string]bool{
	"mode": true, "validcard": true, "caster": true, "onlysorceryspeed": true, "origin": true,
	"numlimiteachturn": true, "effectzone": true, "condition": true, "phases": true, "playerturn": true,
	"description": true, "secondary": true, "spelldescription": true, "stackdescription": true,
}

// cantBeCast reports whether some Mode$ CantBeCast static stops pid casting
// card from the zone it is in now. Hosts are the battlefield and Command-zone
// statics plus the card's own (EffectZone$ All lines on the card itself).
// Params (applyCantBeCastAbility): ValidCard$, Caster$, OnlySorcerySpeed$ (the
// line applies only when the caster could not cast a sorcery), Origin$ (the
// zones it was cast from), and NumLimitEachTurn$ (applies once the caster has
// cast that many spells this turn; only resolved for a ValidCard$ of "Card" or
// none, since this port counts spells cast, not which).
func (g *Game) cantBeCast(pid PlayerID, card CardID) bool {
	c := g.Card(card)
	for _, host := range g.staticHostsWith(card) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		for _, face := range h.Def.Faces {
			for _, s := range face.Statics {
				if strings.EqualFold(s.Name, "CantBeCast") && g.cantBeCastApplies(pid, c, h, s) {
					return true
				}
			}
		}
	}
	return false
}

func (g *Game) cantBeCastApplies(pid PlayerID, c, host *Card, s *compile.Ability) bool {
	for _, p := range s.Params {
		if !cantBeCastParams[strings.ToLower(p.Key)] {
			return false
		}
	}
	if !g.staticConditionsMet(host, s) {
		return false
	}
	validCard, hasValid := s.Param("ValidCard")
	if hasValid && !Matches(g, c, valid.Parse(validCard), host.Controller(), host.ID) {
		return false
	}
	if caster, ok := s.Param("Caster"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, caster)
		if !recognized || !matched {
			return false
		}
	}
	if _, ok := s.Param("OnlySorcerySpeed"); ok && g.canActSorcerySpeed(pid) {
		return false
	}
	if origin, ok := s.Param("Origin"); ok {
		zones, err := parseZoneList(origin)
		if err != nil || !slices.Contains(zones, c.Zone) {
			return false
		}
	}
	if raw, ok := s.Param("NumLimitEachTurn"); ok {
		limit, err := strconv.Atoi(raw)
		if err != nil || (hasValid && validCard != "Card") || g.Player(pid).SpellsCastThisTurn < limit {
			return false
		}
	}
	return true
}

// staticHostsWith is every card whose static abilities apply to card: the
// ones in a static-ability source zone (traitHosts) and card itself, where its
// own EffectZone$ All lines live (StaticAbilityCantBeCast's own `allp.add(card)`).
func (g *Game) staticHostsWith(card CardID) []CardID {
	hosts := []CardID{card}
	for _, p := range g.Players() {
		for _, h := range g.traitHosts(p) {
			if h != card {
				hosts = append(hosts, h)
			}
		}
	}
	return hosts
}
