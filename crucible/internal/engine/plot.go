// Plot (CR 702.170): a special action that, any time its owner could cast a
// sorcery, pays the plot cost and exiles the card from their hand face up,
// plotted; on a later turn they may cast it from exile as a sorcery without
// paying its mana cost. Ported from the Plot branch of CardFactoryUtil.java
// (:3278-3326, an AbilityStatic with sorcery timing and Hand as its zone, or
// the card's zone under Mode$ PlotZone) and GameActionUtil.java:241-247 (the
// cast: from Exile, the owner, sorcery timing, not entered exile this turn, no
// mana cost).

package engine

//enginelint:allow id card game player ability control zone zonemove manapay manarestrict priority trigger triggerpack staticability valid parts

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// plotCostOf is c's K:Plot:<cost> as a mana cost: CardManaCost (Fblthp, Lost
// on the Range's grant) is the card's own mana cost. A cost that is not plain
// mana is not offered (GO-7).
func plotCostOf(c *Card) (mana.Cost, bool) {
	for _, line := range c.KeywordLines() {
		k := keyword.Parse(line)
		if k.Name != "Plot" || k.Details == "" {
			continue
		}
		if k.Details == "CardManaCost" {
			return c.manaCost(), true
		}
		if parsed := cost.Parse(k.Details); !parsed.IsPureMana() {
			return mana.Cost{}, false
		}
		mc, err := mana.Parse(k.Details)
		if err != nil || mc.CountX() > 0 {
			return mana.Cost{}, false
		}
		return mc, true
	}
	return mana.Cost{}, false
}

// Plot is the special action (CR 116.2k, 702.170a): pid, at sorcery speed,
// pays card's plot cost and exiles it from their hand (or, under a Mode$
// PlotZone line admitting it, from where it is), plotted. False when the card
// is not pid's, has no plain-mana Plot, the timing does not allow it, or the
// cost is not paid.
func (g *Game) Plot(pid PlayerID, card CardID, controller PlayerController) bool {
	c := g.Card(card)
	if c.Def == nil || c.Owner != pid || !g.canActSorcerySpeed(pid) {
		return false
	}
	if c.Zone != Hand && !g.plotZone(c) {
		return false
	}
	mc, ok := plotCostOf(c)
	if !ok {
		return false
	}
	mc = g.plottingCost(pid, c, mc)
	from := c.Zone
	restore := g.payingFor(&payContext{item: &Ability{Source: card, Controller: pid}, cost: mc, special: "Plotting"})
	_, paid := g.payManaCostX(pid, mc, controller)
	restore()
	if !paid {
		return false
	}
	g.moveByEffect(controller, card, Exile, 0, NoPlayer, false)
	g.checkChangesZoneAllTriggers(controller, []CardID{card}, from, Exile)
	c = g.Card(card)
	if c.Zone != Exile {
		// A replacement sent it elsewhere: Java's setPlotted marks the card
		// GameAction.exile returned, wherever that is; a plotted card outside
		// exile is never cast from it.
		return true
	}
	c.Plotted = true
	g.checkBecomesPlottedTriggers(controller, card)
	return true
}

// plottedCastAllowed is GameActionUtil.java:241: pid may cast c from exile
// without paying its mana cost as it is plotted, theirs, there since an
// earlier turn, and they could cast a sorcery now.
func (g *Game) plottedCastAllowed(pid PlayerID, c *Card) bool {
	return c.Zone == Exile && c.Plotted && c.Owner == pid && c.Def != nil &&
		c.zoneEntryTurn < g.turn && g.canActSorcerySpeed(pid)
}

// plotZoneParams are the params a Mode$ PlotZone line may carry.
var plotZoneParams = map[string]bool{"validcard": true}

// plotZone is StaticAbilityPlotZone.plotZone: a live Mode$ PlotZone line's
// ValidCard$ admits c, so it may be plotted from the zone it is in (Fblthp:
// the top card of its owner's library).
func (g *Game) plotZone(c *Card) bool {
	return g.anyStatic("PlotZone", plotZoneParams, func(h *Card, s *compile.Ability) bool {
		spec, ok := s.Param("ValidCard")
		return ok && Matches(g, c, valid.Parse(spec), h.Controller(), h.ID)
	})
}

// plottingParams are the params a ReduceCost line naming ValidSpell$
// Static.Plotting may carry that plottingCost reads.
var plottingParams = map[string]bool{
	"validspell": true, "affectedzone": true, "activator": true, "amount": true, "type": true, "validcard": true,
}

// plottingCost is CostAdjustment.adjust over the plot action: each live
// Mode$ ReduceCost line whose ValidSpell$ names Static.Plotting (Doc Aurlock,
// Grizzled Genius) takes its numeric Amount$ off the generic mana, when its
// Activator$, AffectedZone$ and ValidCard$ (each absent matching all) admit
// the plotting.
func (g *Game) plottingCost(pid PlayerID, c *Card, mc mana.Cost) mana.Cost {
	reduce := 0
	g.anyStatic("ReduceCost", plottingParams, func(h *Card, s *compile.Ability) bool {
		spec, _ := s.Param("ValidSpell")
		if !strings.Contains(spec, "Static.Plotting") {
			return false
		}
		if v, ok := s.Param("Activator"); ok {
			if matched, known := matchesPlayerSpec(g, pid, h.Controller(), h.ID, v); !known || !matched {
				return false
			}
		}
		if v, ok := s.Param("AffectedZone"); ok {
			zones, err := parseZoneList(v)
			if err != nil || !zoneIn(c.Zone, zones) {
				return false
			}
		}
		if v, ok := s.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(v), h.Controller(), h.ID) {
			return false
		}
		raw, _ := s.Param("Amount")
		if n, err := strconv.Atoi(raw); err == nil {
			reduce += n
		}
		return false
	})
	return mana.FromShards(mc.Shards(), max(mc.Generic()-reduce, 0))
}
