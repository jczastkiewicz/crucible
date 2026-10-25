// Spell cost modification (CR 601.2f): Mode$ RaiseCost and Mode$ ReduceCost
// statics, CostAdjustment.getSpellCostChange (raise) and CostAdjustment.adjust
// (reduce) for Type$ Spell lines.

//enginelint:allow id zone card game valid player staticability amount parts zonemove cantbecast unlesscost

package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// costModParams are the params a RaiseCost/ReduceCost line may carry that this
// port evaluates (Relative$ on the spell's own line only, ValidTarget$ and
// UnlessValidTarget$ on a spell, costModStatus); any
// other (Color$, UpTo$,
// OnlyFirstSpell$, ForEachShard$, a keyword Amount$ ...) makes the
// line unresolvable and it is not applied (GO-7). Affected$ is listed because
// no cost-modification code in Java reads it (Hurkyl's Final Meditation's
// `Affected$ Card.Self` never reaches its effect, forge-java-defects.md); the
// line is evaluated as Java does, without it.
var costModParams = map[string]bool{
	"mode": true, "validcard": true, "activator": true, "type": true, "affectedzone": true,
	"amount": true, "cost": true, "minmana": true, "effectzone": true,
	"condition": true, "phases": true, "playerturn": true, "relative": true,
	"validspell": true, "validtarget": true, "unlessvalidtarget": true, "affected": true, "description": true, "secondary": true, "spelldescription": true, "stackdescription": true,
}

// spellCost is base, the printed (or alternative) mana cost of casting card as
// pid, after every RaiseCost static (their Cost$, default {1}, added Amount$
// times) and then every ReduceCost static (generic mana only, down to
// MinMana$ -- CostAdjustment.applyReduceCostAbility's cap on total reduction
// against the converted cost). Hosts are the battlefield and Command-zone
// statics and the card itself (EffectZone$ All). An X in the cost is
// untouched: only the generic part shrinks.
//
// paying is the cost actually paid (payCastCost), after the X is announced and
// the targets chosen, which a Relative$ amount may read; a ReduceCost Amount$
// that does not resolve then refuses the cast. The probe before the announce
// (announceXFor, which only asks whether an X is left) skips such a line: a
// reduction never touches the X.
func (g *Game) spellCost(pid PlayerID, card CardID, base mana.Cost, paying, extras bool) (mana.Cost, bool) {
	return g.costStatics(pid, card, nil, base, paying, extras)
}

// costStatics is spellCost for a spell (act nil) or for the activation of
// ability act on card (CostAdjustment.adjust over an activated ability, called
// from CostPayment.payCost): Type$ Ability and an absent Type$ reach it, and
// ValidSpell$ Activated[.props] is read against act. A cost with no mana
// part still takes a raise, so its base is never "no cost" there.
func (g *Game) costStatics(pid PlayerID, card CardID, act *compile.Ability, base mana.Cost, paying, extras bool) (mana.Cost, bool) {
	if act == nil && base.IsNoCost() {
		return base, true
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
		// The host's live faces only: a modal or transforming card's other face
		// is not its state, and its statics do not apply (Extus, Oriq
		// Overlord's back-face reduction while the creature is cast).
		live := h.liveTraitFaces()
		for i := range live {
			face := &live[i]
			for _, s := range face.Statics {
				switch {
				case strings.EqualFold(s.Name, "RaiseCost"):
					switch g.costModStatus(pid, c, h, s, act) {
					case costModApplies:
						raises = append(raises, line{h, s, face})
					case costModUnresolvable:
						// A raise this port cannot evaluate: casting for less
						// than Java would charge is wrong, so the cast is
						// refused (GO-7), and why is kept once the cost is being paid.
						if paying {
							g.recordPendingError(fmt.Errorf("engine: RaiseCost on %s: line not resolvable yet", h.Def.Name))
						}
						return base, false
					}
				case strings.EqualFold(s.Name, "ReduceCost") && g.costModStatus(pid, c, h, s, act) == costModApplies:
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
				return base, false
			}
			count = n
		}
		text := "1"
		if raw, ok := r.s.Param("Cost"); ok {
			text = raw
		}
		if n, ok := raiseWaterbend(text); ok && act == nil && extras && !raiseHasAmount(r.s) {
			// Waterbend<N> merges into the spell's mana as N generic the caster
			// may tap artifacts and creatures to pay (staticRaiseWaterbend).
			generic += n * count
			continue
		}
		add, err := mana.Parse(text)
		if err != nil && act == nil && extras && !raiseHasAmount(r.s) && raiseExtraShape(text) {
			// A non-mana raise on a spell is the spell's additional cost,
			// paid with the rest (castExtraCost, staticRaiseExtra).
			continue
		}
		if err != nil || add.CountX() > 0 {
			// A non-mana or X-bearing raise (Cost$ PayLife<1>, Discard<X/...>) is
			// not charged here: the cast is refused, and why is kept (GO-7).
			if paying {
				g.recordPendingError(fmt.Errorf("engine: RaiseCost: Cost$ %q is not a plain mana cost, not resolvable yet", text))
			}
			return base, false
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
			// calculateAmount of no Amount$ is 0 (CostAdjustment.java:466-479).
			continue
		}
		value, ok := costModAmount(g, r.face, r.host, raw)
		if !ok && !paying {
			continue
		}
		if !ok {
			// Casting for more than Java would charge is as wrong as for less:
			// the cast is refused, and why is kept (GO-7).
			g.recordPendingError(fmt.Errorf("engine: ReduceCost: Amount$ %q is not resolvable", raw))
			return base, false
		}
		minMana := 0
		if raw, ok := r.s.Param("MinMana"); ok {
			minMana, _ = strconv.Atoi(raw)
		}
		if maxReduction := cmc - minMana - reduced; maxReduction > 0 {
			reduced += min(value, maxReduction)
		}
	}
	return mana.FromShards(shards, max(generic-reduced, 0)), true
}

// costModResult is how a RaiseCost/ReduceCost line stands against a cast.
type costModResult int

const (
	costModSkips costModResult = iota
	costModApplies
	// costModUnresolvable: the line names this card, caster and a spell, but
	// carries a param or condition this port does not evaluate.
	costModUnresolvable
)

// costModStatus is CostAdjustment.checkRequirement for a spell: Type$ is Spell,
// ValidCard$ matches the card, Activator$ the caster and AffectedZone$ the
// zone the card is in -- else the line skips. A line that passes those but has a
// param outside costModParams, or whose conditions do not hold, is applied only
// when everything is evaluable; with an unlisted param it is
// costModUnresolvable.
func (g *Game) costModStatus(pid PlayerID, c, host *Card, s *compile.Ability, act *compile.Ability) costModResult {
	// An absent Type$ gates nothing (CostAdjustment.java:535-576): the line
	// reaches spells and activations alike. ValidSpell$ Activated[.props] never
	// holds for a spell (SpellAbility.java:2232).
	// checkRequirement tests the static's conditions first
	// (CostAdjustment.java:514-516), so a line whose condition fails skips
	// before any param this port cannot evaluate refuses the cast.
	if !g.staticConditionsMet(host, s) {
		return costModSkips
	}
	wantType := "Spell"
	if act != nil {
		wantType = "Ability"
	}
	if t, ok := s.Param("Type"); ok && t != wantType {
		return costModSkips
	}
	if v, ok := s.Param("ValidSpell"); ok {
		switch {
		case act == nil:
			matched, recognized := validSpellForSpell(v)
			if !recognized {
				return costModUnresolvable
			}
			if !matched {
				return costModSkips
			}
		case act != nil:
			_, loyalty := act.Param("Planeswalker")
			matched, recognized := validSpecForActivation(v, act.Name == "Mana", loyalty)
			if !recognized {
				return costModUnresolvable
			}
			if !matched {
				return costModSkips
			}
		}
	}
	if v, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(v), host.Controller(), host.ID) {
		return costModSkips
	}
	if v, ok := s.Param("Activator"); ok {
		matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), host.ID, v)
		if !recognized || !matched {
			return costModSkips
		}
	}
	if v, ok := s.Param("AffectedZone"); ok {
		zones, err := parseZoneList(v)
		if err != nil || !zoneIn(c.Zone, zones) {
			return costModSkips
		}
	}
	// Relative$ reads Amount$ against the spell being cast rather than the
	// static (CostAdjustment.java:155-157). On the spell's own line the host
	// is that spell, which is what the amount is read against here (Bite Down
	// on Crime's Count$OptionalGenericCostPaid); another host's is not read.
	if _, relative := s.Param("Relative"); relative && host.ID != c.ID {
		return costModUnresolvable
	}
	// ValidTarget$ holds when some target of the spell being cast is valid
	// (CostAdjustment.java:590-612); UnlessValidTarget$ flips it. Before the
	// targets are chosen (the affordability probe) there are none, so the line
	// does not raise, as Java's does not.
	if v, ok := s.Param("ValidTarget"); ok {
		if act != nil {
			return costModUnresolvable
		}
		targetValid := false
		if g.castTargetsFor == c.ID {
			parsed := valid.Parse(v)
			for _, e := range g.castTargets {
				if g.entityMatches(e, v, parsed, host.Controller(), host.ID) {
					targetValid = true
					break
				}
			}
		}
		if _, unless := s.Param("UnlessValidTarget"); targetValid == unless {
			return costModSkips
		}
	}
	if !paramsResolvable(s, costModParams) {
		return costModUnresolvable
	}
	return costModApplies
}

// validSpellForSpell evaluates a ValidSpell$ spec against a spell being cast
// (SpellAbility.isValid, SpellAbility.java:2200-2250): comma-separated
// alternatives, of which "Spell" matches and "Activated", "Static" and
// "Triggered" never do. A property on Spell (Flashback, ...) or any other base
// is not evaluated here: recognized is false and the line stays unresolvable.
func validSpellForSpell(spec string) (matched, recognized bool) {
	for _, alt := range strings.Split(spec, ",") {
		base, props, _ := strings.Cut(alt, ".")
		switch base {
		case "Spell":
			if props != "" {
				return false, false
			}
			matched = true
		case "Activated", "Static", "Triggered":
		default:
			return false, false
		}
	}
	return matched, true
}

// costModAmount is a line's Amount$: a plain number, or an SVar of the host's
// face resolveNamedAmount evaluates; false when it cannot.
func costModAmount(g *Game, face *compile.Face, host *Card, raw string) (int, bool) {
	if n, err := strconv.Atoi(raw); err == nil {
		return n, true
	}
	return resolveNamedAmount(g, face.Amounts, host, raw)
}

// validSpecForActivation evaluates a ValidSpell$ spec against an activated
// ability: comma-separated alternatives (SpellAbility.isValid), of which the
// Activated ones go through validActivatedSA and "Spell", "Static" and
// "Triggered" (with any properties) never match an activation. An Activated
// alternative validActivatedSA does not know leaves the line unresolvable.
func validSpecForActivation(spec string, isMana, isLoyalty bool) (matched, recognized bool) {
	for _, alt := range strings.Split(spec, ",") {
		base, _, _ := strings.Cut(alt, ".")
		switch base {
		case "Spell", "Static", "Triggered":
			continue
		}
		m, ok := validActivatedSA(alt, isMana, isLoyalty)
		if !ok {
			return false, false
		}
		matched = matched || m
	}
	return matched, true
}

// raiseHasAmount reports whether a RaiseCost line carries Amount$.
func raiseHasAmount(s *compile.Ability) bool {
	_, ok := s.Param("Amount")
	return ok
}

// raiseExtraShape reports whether text is a non-mana RaiseCost Cost$ this port
// pays as a spell's additional cost: every part's amount a plain number and
// the whole thing a shape parseUnlessCost reads without any mana. X-bearing
// amounts (announced after the legality check in Java), Waterbend (a mana-side
// merge) and mixed mana-plus-part raises stay refused.
func raiseExtraShape(text string) bool {
	parsed := cost.Parse(text)
	if len(parsed.Parts) == 0 {
		return false
	}
	for _, p := range parsed.Parts {
		if _, err := strconv.Atoi(p.Field(0)); err != nil || p.Name == "Waterbend" {
			return false
		}
	}
	uc, ok := parseUnlessCost(text)
	return ok && !uc.hasMana
}

// raiseWaterbend reads a RaiseCost Cost$ that is exactly Waterbend<N> with a
// plain N (CostWaterbend, a mana part: Cost.mergeTo folds it into the mana).
func raiseWaterbend(text string) (int, bool) {
	parsed := cost.Parse(text)
	if len(parsed.Parts) != 1 || parsed.Parts[0].Name != "Waterbend" {
		return 0, false
	}
	n, err := strconv.Atoi(parsed.Parts[0].Field(0))
	return n, err == nil && n > 0
}

// staticRaiseWaterbend is the total Waterbend amount the RaiseCost statics put
// on casting c: the caster may tap that many artifacts and creatures for the
// generic mana (waterbendCast).
func (g *Game) staticRaiseWaterbend(pid PlayerID, c *Card) int {
	total := 0
	for _, host := range g.staticHostsWith(c.ID) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		live := h.liveTraitFaces()
		for i := range live {
			for _, s := range live[i].Statics {
				if !strings.EqualFold(s.Name, "RaiseCost") || raiseHasAmount(s) {
					continue
				}
				text, hasCost := s.Param("Cost")
				if !hasCost || g.costModStatus(pid, c, h, s, nil) != costModApplies {
					continue
				}
				if n, ok := raiseWaterbend(text); ok {
					total += n
				}
			}
		}
	}
	return total
}

// staticRaiseExtra is the additional cost the RaiseCost statics with a
// non-mana Cost$ put on casting c (CostAdjustment.applyRaiseCostAbility,
// Cost.mergeTo: a separate cost part). ok is false for a shape this port does
// not pay or when two such lines apply (one extra cost is modelled), so the
// spell is not cast cheaper than Java charges it (GO-7).
func (g *Game) staticRaiseExtra(pid PlayerID, c *Card) (uc unlessCost, has, ok bool) {
	for _, host := range g.staticHostsWith(c.ID) {
		h := g.Card(host)
		if h.Def == nil {
			continue
		}
		live := h.liveTraitFaces()
		for i := range live {
			for _, s := range live[i].Statics {
				if !strings.EqualFold(s.Name, "RaiseCost") {
					continue
				}
				text, hasCost := s.Param("Cost")
				if !hasCost {
					continue
				}
				if _, err := mana.Parse(text); err == nil || g.costModStatus(pid, c, h, s, nil) != costModApplies {
					continue
				}
				if _, ok := raiseWaterbend(text); ok && !raiseHasAmount(s) {
					continue
				}
				if has || raiseHasAmount(s) || !raiseExtraShape(text) {
					g.recordPendingError(fmt.Errorf("engine: RaiseCost: Cost$ %q is not a plain mana cost, not resolvable yet", text))
					return unlessCost{}, false, false
				}
				uc, _ = parseUnlessCost(text)
				has = true
			}
		}
	}
	return uc, has, true
}
