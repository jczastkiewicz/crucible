// The ways a player may cast one card from where it is: casting it normally
// from their hand, and each MayPlay$ grant and exile permission that lets
// them cast it (for free, for an alternative cost, spending any type of
// mana, with flash). Java offers every grant as its own SpellAbility for the
// player to pick between (GameActionUtil.getAdditionalCostAbilities and the
// CardPlayOption list); here one castOption each, and when more than one
// differs the controller picks through ChooseOption.
//
// A grant MayPlayDontGrantZonePermissions$ marks changes how a card is cast,
// not whether its zone allows casting: it is an option only when the card is
// castable from where it is anyway, from the hand or through another grant
// (SpellAbilityRestriction.java:231-255).

package engine

import (
	"fmt"

	"github.com/jczastkiewicz/crucible/internal/mana"
)

// castOption is one way to cast a card.
type castOption struct {
	label       string
	withoutMana bool
	hasAlt      bool
	alt         mana.Cost
	anyType     bool
	flash       bool
	// limits are the MayPlayLimit$ counters a cast by this option uses up.
	limits []mayPlayLimitKey
}

// sameWay reports whether two options differ only in flash and limits, which
// merge: a grant that only adds flash leaves nothing to pick.
func (o castOption) sameWay(p castOption) bool {
	return o.withoutMana == p.withoutMana && o.hasAlt == p.hasAlt && o.alt.Equal(p.alt) && o.anyType == p.anyType
}

// castOptions is every way pid may cast card now from its zone: the normal
// hand cast (fromHand) and each live grant whose limit is not spent. An empty
// result means the card is not castable from here by any of them.
func (g *Game) castOptions(pid PlayerID, card CardID, fromHand bool) []castOption {
	c := g.Card(card)
	var grants []castOption
	zonePermission := fromHand
	for _, gr := range g.mayPlay {
		if gr.CardID != card || gr.Grantee != pid || gr.Timestamp != c.Timestamp {
			continue
		}
		if gr.Limit > 0 && g.mayPlayUsedThisTurn(gr.LimitKey) >= gr.Limit {
			continue
		}
		zonePermission = zonePermission || gr.ZonePermission
		o := castOption{withoutMana: gr.WithoutManaCost, hasAlt: gr.HasAltCost, alt: gr.AltCost, anyType: gr.AnyType, flash: gr.WithFlash}
		if gr.Limit > 0 {
			o.limits = []mayPlayLimitKey{gr.LimitKey}
		}
		grants = append(grants, o)
	}
	if grant, ok := g.MayPlayFromExile(pid, card); ok {
		zonePermission = true
		o := castOption{anyType: grant.AnyManaType}
		if grant.HasAltCost {
			o.hasAlt, o.alt = true, mana.MustParse(fmt.Sprint(grant.AltGeneric))
		}
		grants = append(grants, o)
	}
	if !zonePermission {
		return nil
	}
	var out []castOption
	if fromHand {
		out = append(out, castOption{})
	}
	for _, o := range grants {
		merged := false
		for i := range out {
			// A limited grant stays its own option: a cast uses up only the
			// static it went through (two of them allow two casts a turn).
			if out[i].sameWay(o) && len(out[i].limits) == 0 && len(o.limits) == 0 {
				out[i].flash = out[i].flash || o.flash
				out[i].limits = append(out[i].limits, o.limits...)
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, o)
		}
	}
	for i := range out {
		out[i].label = out[i].describe(g.Card(card).Def.Name)
	}
	return out
}

// describe is the option's label, what ChooseOption shows.
func (o castOption) describe(name string) string {
	switch {
	case o.withoutMana:
		return "Cast " + name + " without paying its mana cost"
	case o.hasAlt:
		return "Cast " + name + " for " + o.alt.String()
	case o.anyType:
		return "Cast " + name + " spending mana of any type"
	}
	return "Cast " + name
}

// chooseCastOption is the option pid casts by: the only one, or the one the
// controller picks. ok is false when there are none or the controller's
// answer is out of range (a pending error, GO-7).
func (g *Game) chooseCastOption(controller PlayerController, pid PlayerID, card CardID, options []castOption) (castOption, bool) {
	switch len(options) {
	case 0:
		return castOption{}, false
	case 1:
		return options[0], true
	}
	labels := make([]string, len(options))
	for i, o := range options {
		labels[i] = o.label
	}
	pick := controller.ChooseOption(g, pid, card, labels)
	if pick < 0 || pick >= len(options) {
		g.recordPendingError(fmt.Errorf("engine: card %d: cast option %d is not one of %d", card, pick, len(options)))
		return castOption{}, false
	}
	return options[pick], true
}

// mayPlayUsedThisTurn is how many casts this turn went through the limited
// grant key.
func (g *Game) mayPlayUsedThisTurn(key mayPlayLimitKey) int {
	if use, ok := g.mayPlayUses[key]; ok && use.Turn == g.turn {
		return use.N
	}
	return 0
}

// noteMayPlayUse counts one cast through each of o's limited grants.
func (g *Game) noteMayPlayUse(o castOption) {
	for _, key := range o.limits {
		if g.mayPlayUses == nil {
			g.mayPlayUses = map[mayPlayLimitKey]mayPlayUse{}
		}
		use := g.mayPlayUses[key]
		if use.Turn != g.turn {
			use = mayPlayUse{Turn: g.turn}
		}
		use.N++
		g.mayPlayUses[key] = use
	}
}

// anyTypeCost is cost with every single-mana requirement (a colored, colorless
// or hybrid shard) turned into generic: mana of any type may be spent, so any
// mana pays it.
func anyTypeCost(total mana.Cost) mana.Cost {
	generic := total.Generic()
	var kept []mana.Shard
	for _, s := range total.Shards() {
		if s.CMC() == 1 && !s.IsX() && !s.IsSnow() {
			generic++
			continue
		}
		kept = append(kept, s)
	}
	return mana.FromShards(kept, generic)
}
