// Surge (CR 702.117) and Freerunning (CR 702.173): "you may cast this spell
// for <cost> if <history gate>" -- one more cast option of the card's own
// spell, whose cost replaces its mana cost. Java builds a copy of the first
// spell ability with the keyword's cost and marks it AlternativeCost.Surge /
// Freerunning (CardFactoryUtil.java:3038-3065 Freerunning, :3535-3546 Surge);
// SpellAbilityRestriction.java:388-392, :413-417 gate it on
// Player.hasSurge / Player.hasFreerunning. The cast records the way
// (optSurge, optFreerunning), which Condition$ Surge and
// Count$Freerunning.<paid>.<unpaid> read (SpellAbilityCondition.java surge
// branch, AbilityUtils.java:1717-1718).

package engine

import (
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

// altKeywordOptions are the Surge and Freerunning ways pid may cast c from
// the hand: those whose keyword c has and whose history gate holds.
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
	return out
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
