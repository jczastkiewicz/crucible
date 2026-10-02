// Spell cost modification (CR 601.2f): Mode$ RaiseCost and Mode$ ReduceCost
// statics, CostAdjustment.getSpellCostChange (raise) and CostAdjustment.adjust
// (reduce) for Type$ Spell lines.

//enginelint:allow id zone card game valid player staticability amount parts zonemove

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// costModParams are the params a RaiseCost/ReduceCost line may carry that this
// port evaluates; any other (ValidTarget$, Color$, Relative$, UpTo$,
// OnlyFirstSpell$, ValidSpell$, ForEachShard$, a keyword Amount$ ...) makes the
// line unresolvable and it is not applied (GO-7).
var costModParams = map[string]bool{
	"mode": true, "validcard": true, "activator": true, "type": true, "affectedzone": true,
	"amount": true, "cost": true, "minmana": true, "effectzone": true,
	"condition": true, "phases": true, "playerturn": true,
	"description": true, "secondary": true, "spelldescription": true, "stackdescription": true,
}

// spellCost is base, the printed (or alternative) mana cost of casting card as
// pid, after every RaiseCost static (their Cost$, default {1}, added Amount$
// times) and then every ReduceCost static (generic mana only, down to
// MinMana$ -- CostAdjustment.applyReduceCostAbility's cap on total reduction
// against the converted cost). Hosts are the battlefield and Command-zone
// statics and the card itself (EffectZone$ All). An X in the cost is
// untouched: only the generic part shrinks.
func (g *Game) spellCost(pid PlayerID, card CardID, base mana.Cost) mana.Cost {
	if base.IsNoCost() {
		return base
	}
	c := g.Card(card)
	generic := base.Generic()
	shards := append([]mana.Shard(nil), base.Shards()...)

	type line struct {
		host *Card
		s    *compile.Ability
		face *compile.Face
	}
	var raises, reduces []line
	for _, host := range g.staticHostsWith(card) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		for i := range h.Def.Faces {
			face := &h.Def.Faces[i]
			for _, s := range face.Statics {
				switch {
				case strings.EqualFold(s.Name, "RaiseCost") && g.costModApplies(pid, c, h, s):
					raises = append(raises, line{h, s, face})
				case strings.EqualFold(s.Name, "ReduceCost") && g.costModApplies(pid, c, h, s):
					reduces = append(reduces, line{h, s, face})
				}
			}
		}
	}
	for _, r := range raises {
		count := 1
		if raw, ok := r.s.Param("Amount"); ok {
			n, ok := costModAmount(g, r.face, r.host, raw)
			if !ok {
				continue
			}
			count = n
		}
		text := "1"
		if raw, ok := r.s.Param("Cost"); ok {
			text = raw
		}
		add, err := mana.Parse(text)
		if err != nil || add.CountX() > 0 {
			continue
		}
		for range max(count, 0) {
			generic += add.Generic()
			shards = append(shards, add.Shards()...)
		}
	}
	cmc := mana.FromShards(shards, generic).CMC()
	reduced := 0
	for _, r := range reduces {
		raw, ok := r.s.Param("Amount")
		if !ok {
			continue
		}
		value, ok := costModAmount(g, r.face, r.host, raw)
		if !ok {
			continue
		}
		minMana := 0
		if raw, ok := r.s.Param("MinMana"); ok {
			minMana, _ = strconv.Atoi(raw)
		}
		if maxReduction := cmc - minMana - reduced; maxReduction > 0 {
			reduced += min(value, maxReduction)
		}
	}
	return mana.FromShards(shards, max(generic-reduced, 0))
}

// costModApplies is CostAdjustment.checkRequirement for a spell: the static's
// conditions hold, Type$ is Spell, ValidCard$ matches the card, Activator$ the
// caster, AffectedZone$ the zone the card is in, and every param is one this
// port reads.
func (g *Game) costModApplies(pid PlayerID, c, host *Card, s *compile.Ability) bool {
	if !paramsResolvable(s, costModParams) || !g.staticConditionsMet(host, s) {
		return false
	}
	if t, ok := s.Param("Type"); !ok || t != "Spell" {
		return false
	}
	if v, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(v), host.Controller(), host.ID) {
		return false
	}
	if v, ok := s.Param("Activator"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, v)
		if !recognized || !matched {
			return false
		}
	}
	if v, ok := s.Param("AffectedZone"); ok {
		zones, err := parseZoneList(v)
		if err != nil || !zoneIn(c.Zone, zones) {
			return false
		}
	}
	return true
}

// costModAmount is a line's Amount$: a plain number, or an SVar of the host's
// face resolveNamedAmount evaluates; false when it cannot.
func costModAmount(g *Game, face *compile.Face, host *Card, raw string) (int, bool) {
	if n, err := strconv.Atoi(raw); err == nil {
		return n, true
	}
	return resolveNamedAmount(g, face.Amounts, host, raw)
}
