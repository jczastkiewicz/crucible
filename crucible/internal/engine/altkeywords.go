// Surge (CR 702.117) and Freerunning (CR 702.173): "you may cast this spell
// for <cost> if <history gate>" -- one more cast option of the card's own
// spell, whose cost replaces its mana cost. Dash (CR 702.109) and Blitz (CR
// 702.152) are such casts with no gate; the permanent leaves at the next end
// step (permanentEffect) and has haste (and Blitz's "when this dies, draw a
// card") while it carries that cast, through the keyword's static
// (keyword.Expand: Card.Self+dashed / +blitzed). Java builds a copy of the first
// spell ability with the keyword's cost and marks it AlternativeCost.Surge /
// Freerunning (CardFactoryUtil.java:3038-3065 Freerunning, :3535-3546 Surge);
// SpellAbilityRestriction.java:388-392, :413-417 gate it on
// Player.hasSurge / Player.hasFreerunning. The cast records the way
// (optSurge, optFreerunning), which Condition$ Surge and
// Count$Freerunning.<paid>.<unpaid> read (SpellAbilityCondition.java surge
// branch, AbilityUtils.java:1717-1718).

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// altKeywordCost is the mana cost of c's keyword (Surge or Freerunning). ok is
// false without the keyword or for a cost that is not plain mana: such a cast
// is not offered, never made for a guessed cost (GO-7).
func altKeywordCost(c *Card, name string) (mana.Cost, bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != name {
			continue
		}
		if k.Details == "" || strings.Contains(k.Details, ":") {
			return mana.Cost{}, false
		}
		mc, err := mana.Parse(k.Details)
		if err != nil {
			return mana.Cost{}, false
		}
		return mc, true
	}
	return mana.Cost{}, false
}

// hasSurge is Player.hasSurge: pid has cast a spell this turn. This port has
// no teams, so a team is the player alone.
func (g *Game) hasSurge(pid PlayerID) bool {
	for _, rec := range g.castThisTurn {
		if rec.controller == pid {
			return true
		}
	}
	return false
}

// hasFreerunning is Player.hasFreerunning: a card pid controlled (as it was
// when it dealt the damage) that is an Assassin or a commander dealt combat
// damage to a player this turn; the commander is the designation of ADR-0051.
func (g *Game) hasFreerunning(pid PlayerID) bool {
	for _, d := range g.playerDamageThisTurn {
		if d.combat && d.source.Controller() == pid && (d.source.Type().HasSubtype("Assassin") || d.source.isCommander()) {
			return true
		}
	}
	return false
}

// hasProwl is Player.hasProwl (Player.java:2170-2172): a creature pid
// controlled (as it was when it dealt the damage) that shares a creature type
// with c dealt combat damage to a player this turn.
func (g *Game) hasProwl(pid PlayerID, c *Card) bool {
	for i := range g.playerDamageThisTurn {
		d := &g.playerDamageThisTurn[i]
		if d.combat && d.source.Controller() == pid && g.sharesCreatureType(&d.source, c) {
			return true
		}
	}
	return false
}

// opponentLostLifeThisTurn is Player.getOpponentLostLifeThisTurn > 0
// (Player.java:2162-2168), Spectacle's gate: an opponent of pid, one who left
// the game included, lost life this turn.
func (g *Game) opponentLostLifeThisTurn(pid PlayerID) bool {
	for _, o := range g.Players() {
		if o != pid && g.Player(o).LifeLostThisTurn > 0 {
			return true
		}
	}
	return false
}

// altKeywordOptions are the Surge, Freerunning, Dash, Blitz and Warp ways pid
// may cast c from the hand (Warp's spell is restricted to it,
// CardFactoryUtil.java:3672): those whose keyword c has and, for the first two,
// whose history gate holds.
func (g *Game) altKeywordOptions(pid PlayerID, c *Card) []castOption {
	var out []castOption
	if c.Type().Has(cardtype.Land) {
		return nil
	}
	if mc, ok := altKeywordCost(c, "Surge"); ok && g.hasSurge(pid) {
		out = append(out, castOption{hasAlt: true, alt: mc, altKeyword: "Surge"})
	}
	if mc, ok := altKeywordCost(c, "Freerunning"); ok && g.hasFreerunning(pid) {
		out = append(out, castOption{hasAlt: true, alt: mc, altKeyword: "Freerunning"})
	}
	// Prowl (CardFactoryUtil.java:3354-3374) and Spectacle (:3524-3535) are
	// such casts too, gated by SpellAbilityRestriction.java:393-397, :408-412.
	if mc, ok := altKeywordCost(c, "Prowl"); ok && g.hasProwl(pid, c) {
		out = append(out, castOption{hasAlt: true, alt: mc, altKeyword: "Prowl"})
	}
	if mc, ok := altKeywordCost(c, "Spectacle"); ok && g.opponentLostLifeThisTurn(pid) {
		out = append(out, castOption{hasAlt: true, alt: mc, altKeyword: "Spectacle"})
	}
	if o, ok := awakenOption(c); ok {
		out = append(out, o)
	}
	if o, ok := prototypeOption(c); ok {
		out = append(out, o)
	}
	for _, name := range [...]string{"Dash", "Blitz", "Warp", "Bestow", "Web-slinging"} {
		if name == "Bestow" && !c.Type().Has(cardtype.Creature) {
			continue
		}
		if o, ok := g.keywordCastOption(pid, c, name); ok {
			out = append(out, o)
		}
	}
	return out
}

// keywordCastOption is the cast for c's keyword name (Dash, Blitz, Warp or
// Bestow, whose spell is the Aura spell animateBestow makes, bestow.go):
// its cost in place of the mana cost, Java's copyWithDefinedCost for Dash
// (CardFactoryUtil.java:2744-2764) and Warp (:3648-3675) and
// copyWithManaCostReplaced for Blitz
// (:2690-2713), alike for a spell with no Cost$ of its own. A part past the
// mana (Blitz's PayLife<2>, Discard<1/Card>, Timeline Culler's warp
// PayLife<2>) is paid as an alternative
// cost's other parts are. ok is false without the keyword, for an X, for a
// cost part this port cannot pay, and for a Blitz with a third field (its
// ValidAfterStack$, Henzie's and The Caldaia's grants, whose cost is
// CardManaCost too): not offered, never cast for a guessed cost (GO-7).
func (g *Game) keywordCastOption(pid PlayerID, c *Card, name string) (castOption, bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != name {
			continue
		}
		details := k.Details
		if name == "Bestow" {
			// Bestow's second field is its AILogic$ (CardFactoryUtil.java:2661).
			details, _, _ = strings.Cut(details, ":")
		}
		if details == "" || strings.Contains(details, ":") {
			return castOption{}, false
		}
		if name == "Web-slinging" {
			// CardFactoryUtil.java:3677-3701: the keyword's mana and the
			// return of a tapped creature, in place of the mana cost.
			details += " Return<1/Creature.tapped>"
		}
		uc, ok := parseUnlessCost(details)
		if !ok || (uc.hasMana && uc.mana.CountX() > 0) {
			return castOption{}, false
		}
		o := castOption{hasAlt: true, alt: mana.MustParse("0"), altKeyword: name}
		if uc.hasMana {
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
	return castOption{}, false
}

// altKeywordKind is the optionalKinds bit a cast by the keyword records.
func altKeywordKind(name string) optionalKinds {
	switch name {
	case "Surge":
		return optSurge
	case "Freerunning":
		return optFreerunning
	}
	return 0
}

// prototypeWay is c's "Prototype:<cost>:<P>:<T>". ok is false without the
// keyword, for a malformed line (CardFactoryUtil.java:3332-3335 skips one
// with fewer than four fields) and for an X in the cost.
func prototypeWay(c *Card) (prototypeChars, bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Prototype" {
			continue
		}
		args := k.Args()
		if len(args) < 3 || strings.Contains(args[0], "X") {
			return prototypeChars{}, false
		}
		cost, err := mana.Parse(args[0])
		if err != nil {
			return prototypeChars{}, false
		}
		p, errP := strconv.Atoi(args[1])
		t, errT := strconv.Atoi(args[2])
		if errP != nil || errT != nil {
			return prototypeChars{}, false
		}
		return prototypeChars{cost: cost, power: p, toughness: t}, true
	}
	return prototypeChars{}, false
}

// prototypeOption is the Prototype cast of c: its prototype cost in place of
// its mana cost.
func prototypeOption(c *Card) (castOption, bool) {
	p, ok := prototypeWay(c)
	if !ok {
		return castOption{}, false
	}
	return castOption{hasAlt: true, alt: p.cost, altKeyword: "Prototype"}, true
}

// awakenOption is the Awaken cast of c (CR 702.113, CardFactoryUtil.java:
// 2636-2652): "Awaken:<N>:<cost>", its spell the compiled
// Face.AwakenSpell for the awaken cost. ok is false without the keyword or
// that spell, and for a cost that is not plain mana (none in the corpus).
func awakenOption(c *Card) (castOption, bool) {
	if c.Def == nil || c.Def.Faces[0].AwakenSpell == nil {
		return castOption{}, false
	}
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Awaken" {
			continue
		}
		args := k.Args()
		if len(args) < 2 || strings.Contains(args[1], "X") {
			return castOption{}, false
		}
		mc, err := mana.Parse(args[1])
		if err != nil {
			return castOption{}, false
		}
		return castOption{hasAlt: true, alt: mc, altKeyword: "Awaken"}, true
	}
	return castOption{}, false
}
