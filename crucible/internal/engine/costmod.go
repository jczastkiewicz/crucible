// Spell cost modification (CR 601.2f): Mode$ RaiseCost and Mode$ ReduceCost
// statics, CostAdjustment.getSpellCostChange (raise) and CostAdjustment.adjust
// (reduce) for Type$ Spell lines.

//enginelint:allow id zone card game valid player staticability amount parts zonemove cantbecast unlesscost ability castspell control

package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/expr"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// costModParams are the params a RaiseCost/ReduceCost line may carry that this
// port evaluates (Relative$ on the spell's own line only, ValidTarget$,
// UnlessValidTarget$ and ForEachShard$ on a spell, costModStatus); any
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
	"validspell": true, "foreachshard": true, "validtarget": true, "unlessvalidtarget": true, "affected": true, "description": true, "secondary": true, "spelldescription": true, "stackdescription": true,
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
			n, ok := g.costModAmountFor(c, pid, act, r.face, r.host, raw)
			if !ok {
				return base, false
			}
			count = n
		}
		text := "1"
		if raw, ok := r.s.Param("Cost"); ok {
			text = raiseXFromSVar(g, r.host, c, r.face, raw)
		}
		if n, ok := raiseWaterbend(text); ok && act == nil && extras && !raiseHasAmount(r.s) {
			// Waterbend<N> merges into the spell's mana as N generic the caster
			// may tap artifacts and creatures to pay (staticRaiseWaterbend).
			generic += n * count
			continue
		}
		add, err := mana.Parse(text)
		if err != nil && act == nil && extras && !raiseHasAmount(r.s) && (raiseExtraShape(raiseXFromSVar(g, r.host, c, r.face, text)) || raiseXAnnounceLater(r.host, c, text)) {
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
	// A cost with X can be reduced past its printed generic part: the announced
	// X is generic mana too once it is paid (payManaCostX takes the leftover).
	if paying && act == nil && base.CountX() > 0 {
		cmc += 1000
	}
	reduced := 0
	for _, r := range reduces {
		raw, ok := r.s.Param("Amount")
		if !ok {
			// calculateAmount of no Amount$ is 0 (CostAdjustment.java:466-479).
			continue
		}
		value, ok := g.costModAmountFor(c, pid, act, r.face, r.host, raw)
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
	if paying && act == nil {
		g.xReduce = 0
		if base.CountX() > 0 && reduced > generic {
			g.xReduce = reduced - generic
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
	// ForEachShard$ repeats the raise once per shard of that color in the
	// card's printed mana cost (CostAdjustment.java:118-131); none is no raise.
	// An activation's count is read off a cost this port does not look into.
	if v, ok := s.Param("ForEachShard"); ok {
		n, known := raiseShardCount(c, act, v)
		switch {
		case !known:
			return costModUnresolvable
		case n == 0:
			return costModSkips
		case act != nil:
			// An activation owing such a raise would need its extra cost
			// paid with it; only the zero count is answered (GO-7).
			return costModUnresolvable
		}
	}
	if !paramsResolvable(s, costModParams) {
		return costModUnresolvable
	}
	return costModApplies
}

// raiseShardCount is the number of shards of the named color (ForEachShard$
// Black) in the mana cost of the spell c (its printed cost, CostAdjustment.java:
// 120-121) or of the activation act (its Cost$ mana part, :122-124): any shard
// whose color mask holds it counts, hybrid, Phyrexian and {2/B} alike, an X
// shard does not. known is false for a name that is not one of the five colors
// (ManaAtom.fromName) or a mana part that does not parse.
func raiseShardCount(c *Card, act *compile.Ability, name string) (n int, known bool) {
	var color mana.Colors
	switch strings.ToLower(name) {
	case "white":
		color = mana.White
	case "blue":
		color = mana.Blue
	case "black":
		color = mana.Black
	case "red":
		color = mana.Red
	case "green":
		color = mana.Green
	default:
		return 0, false
	}
	mc := c.manaCost()
	if act != nil {
		mc = mana.Cost{}
		if text, ok := act.Param("Cost"); ok {
			if m := cost.Parse(text).Mana; len(m) > 0 {
				parsed, err := mana.Parse(strings.Join(m, " "))
				if err != nil {
					return 0, false
				}
				mc = parsed
			}
		}
	}
	for _, sh := range mc.Shards() {
		if sh.Colors()&color != 0 {
			n++
		}
	}
	return n, true
}

// raiseScaledByShards repeats a RaiseCost Cost$ once per ForEachShard$ shard:
// Cost.mergeTo adds the part n times and same-type parts sum (Cost.java:982-994,
// :1021-1025), so Sac<1/Swamp> becomes Sac<n/Swamp>.
func raiseScaledByShards(c *Card, s *compile.Ability, text string) string {
	name, ok := s.Param("ForEachShard")
	if !ok {
		return text
	}
	n, _ := raiseShardCount(c, nil, name)
	var out strings.Builder
	for i := 0; i < len(text); {
		out.WriteByte(text[i])
		i++
		if text[i-1] != '<' {
			continue
		}
		j := i
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		if j == i || j >= len(text) || (text[j] != '/' && text[j] != '>') {
			continue
		}
		k, _ := strconv.Atoi(text[i:j])
		out.WriteString(strconv.Itoa(k * n))
		i = j
	}
	return out.String()
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

// costModAmountFor is costModAmount read against the spell being cast, whose
// chosen targets a Count$Compare / TargetedController amount reads (Lullmage's
// Domination's XGrave, CostAdjustment.java:466-479 passes the spell's sa):
// g.resolving is a spell Ability carrying them while the amount is worked out.
func (g *Game) costModAmountFor(c *Card, pid PlayerID, act *compile.Ability, face *compile.Face, host *Card, raw string) (int, bool) {
	if act == nil && g.castTargetsFor == c.ID {
		saved := g.resolving
		g.resolving = &Ability{Source: c.ID, Controller: pid, Targets: g.castTargets, spell: true}
		defer func() { g.resolving = saved }()
	}
	return costModAmount(g, face, host, raw)
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

// raiseXFromSVar writes the value of the host's SVar X into a RaiseCost Cost$
// whose part amounts name X (Tectonic Split's Sac<X/Land/land(s)>). Such an X is
// not announced: PlaySpellAbility.announceValuesLikeX asks only when the SVar
// is Count$xPaid or absent (:766-771), and CostPart.getAbilityAmount reads the
// SVar as the part is paid (CostPart.java:154-156). An announced X, or an SVar
// this port cannot evaluate, leaves the text as it was, so the line stays
// refused.
func raiseXFromSVar(g *Game, host *Card, c *Card, face *compile.Face, text string) string {
	text = raiseNamedFromSVars(g, host, face, text)
	if !strings.Contains(text, "<X/") && !strings.Contains(text, "<X>") {
		return text
	}
	var n int
	if amt, ok := face.Amounts["x"]; !ok || (amt.Kind == expr.Expression && amt.Head == "Count" && strings.EqualFold(amt.Body, "xPaid")) {
		// An announced X: the X the spell being cast announced before its
		// targets (announceXFor), for the card's own static (Soul Immolation's
		// Blight<X>, Foggy Swamp Visions' Waterbend<X>).
		if host.ID != c.ID || !g.hasPreX {
			return text
		}
		n = g.preX
	} else {
		var ok bool
		if n, ok = resolveNamedAmount(g, face.Amounts, host, "X"); !ok || n < 0 {
			return text
		}
	}
	v := strconv.Itoa(n)
	return strings.ReplaceAll(strings.ReplaceAll(text, "<X/", "<"+v+"/"), "<X>", "<"+v+">")
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
				if n, ok := raiseWaterbend(raiseXFromSVar(g, h, c, &live[i], text)); ok {
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
				text = raiseXFromSVar(g, h, c, &live[i], text)
				if _, ok := raiseWaterbend(text); ok && !raiseHasAmount(s) {
					continue
				}
				text = raiseScaledByShards(c, s, text)
				if has || raiseHasAmount(s) || (!raiseExtraShape(text) && !raiseXAnnounceLater(h, c, text)) {
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

// raiseAnnouncedXText is the Cost$ text of a RaiseCost static that applies to
// casting c and names an announced X in a part past mana (Soul Immolation's
// Blight<X>, Foggy Swamp Visions' Waterbend<X>), or "" for none: the X is asked
// before the targets like any other announced X (announceXFor).
func (g *Game) raiseAnnouncedXText(pid PlayerID, c *Card) string {
	if !xIsPaid(c) {
		return ""
	}
	for _, host := range g.staticHostsWith(c.ID) {
		h := g.Card(host)
		if h.Def == nil || h.ID != c.ID {
			continue
		}
		live := h.liveTraitFaces()
		for i := range live {
			for _, s := range live[i].Statics {
				if !strings.EqualFold(s.Name, "RaiseCost") || raiseHasAmount(s) {
					continue
				}
				text, ok := s.Param("Cost")
				if !ok || !costNamesX(text) || g.costModStatus(pid, c, h, s, nil) != costModApplies {
					continue
				}
				return text
			}
		}
	}
	return ""
}

// raiseXAnnounceLater reports a RaiseCost text naming an X that has not been
// announced yet: the card's own static (host is c) with X unresolved. The cost
// is worked out once before the announcement (announceXFor) and once after, and
// the first pass must not refuse it.
func raiseXAnnounceLater(host, c *Card, text string) bool {
	return host.ID == c.ID && costNamesX(text) && xIsPaid(c)
}

// raiseNamedAmount matches a part amount that is an SVar name other than X:
// "<Y/" or "<Y>".
var raiseNamedAmount = regexp.MustCompile(`<([A-WYZ][A-Za-z]*)([/>])`)

// raiseNamedFromSVars writes the value of each SVar a RaiseCost Cost$ part
// amount names (March of Reckless Joy's ExileFromHand<Y/...>, Y = SVar$Exiled,
// the count announced with Announce$) into the text, as CostPart.
// getAbilityAmount reads the SVar when the part is paid. A name that does not
// resolve stays, so the line is refused.
func raiseNamedFromSVars(g *Game, host *Card, face *compile.Face, text string) string {
	return raiseNamedAmount.ReplaceAllStringFunc(text, func(m string) string {
		sub := raiseNamedAmount.FindStringSubmatch(m)
		n, ok := resolveNamedAmount(g, face.Amounts, host, sub[1])
		if !ok || n < 0 {
			return m
		}
		return "<" + strconv.Itoa(n) + sub[2]
	})
}

// announceSVarsFor is PlaySpellAbility.announceValuesLikeX for the spell line's
// Announce$ <SVar> (March of Reckless Joy's "how many red cards to exile"):
// the caster names a number from 0 to what the card's own RaiseCost part, with
// that count in its amount, can still pay, and it is stored as the SVar on the
// card (Card.setSVar) for the cost, the cost reduction and the effect to read.
// An X announcement is announceXFor's. false when the caster declines.
func (g *Game) announceSVarsFor(pid PlayerID, c *Card, controller PlayerController, line *compile.Ability) bool {
	raw, ok := line.Param("Announce")
	if !ok {
		return true
	}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" || name == "X" || name == "NumTimes" {
			continue
		}
		key := strings.ToLower(name)
		text := ""
		for _, host := range g.staticHostsWith(c.ID) {
			h := g.Card(host)
			if h.ID != c.ID || h.Def == nil {
				continue
			}
			for _, s := range h.liveTraitFaces()[0].Statics {
				if t, has := s.Param("Cost"); has && strings.EqualFold(s.Name, "RaiseCost") && raiseNamedAmount.MatchString(t) {
					text = t
				}
			}
		}
		if text == "" {
			return true
		}
		face := &c.Def.Faces[0]
		best := 0
		for n := 1; n <= 200; n++ {
			if c.svars == nil {
				c.svars = map[string]int{}
			}
			c.svars[key] = n
			uc, ok := parseUnlessCost(raiseNamedFromSVars(g, c, face, text))
			if !ok || !g.unlessPayable(pid, c.ID, uc) {
				break
			}
			best = n
		}
		if c.svars == nil {
			c.svars = map[string]int{}
		}
		n := controller.ChooseNumber(g, pid, c.ID, 0, best)
		if n < 0 || n > best {
			return false
		}
		c.svars[key] = n
	}
	return true
}
