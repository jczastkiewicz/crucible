// The castability probe (MET-5, MET-6): once per turn, at the end of the
// controller's precombat main phase with an empty stack, whether each card in
// hand could be cast and, if not, why. It reads the board and changes nothing.
//
// A probe answers "could the deck have cast this", not "would the AI have": a
// card blocked on mana indicts the mana base, a card the AI left in hand
// while castable indicts the card (MET-4).

package engine

//enginelint:allow phase ability card game targeting castspell activateability priority cantbecast player zone id

import (
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// CastBlock is a bitmask of the reasons a card could not be cast (MET-5).
type CastBlock uint8

// The block reasons. A card may be blocked several ways at once.
const (
	BlockInsufficientMana CastBlock = 1 << iota
	BlockColorUnavailable
	BlockNoLegalTarget
	BlockTimingRestriction
	BlockZoneOrStateRestriction
	BlockOpponentLock
	BlockLandDropUsed
)

var blockNames = [...]string{"InsufficientMana", "ColorUnavailable", "NoLegalTarget", "TimingRestriction", "ZoneOrStateRestriction", "OpponentLock", "LandDropUsed"}

// Names lists the reasons set in b, in declaration order.
func (b CastBlock) Names() []string {
	var out []string
	for i, n := range blockNames {
		if b&(1<<uint(i)) != 0 {
			out = append(out, n)
		}
	}
	return out
}

// CastProbe is one card's answer.
type CastProbe struct {
	Card CardID
	// Castable is true when nothing blocks the card; Blocked is then zero.
	Castable bool
	Blocked  CastBlock
	// MissingColors are, when Blocked holds BlockColorUnavailable, the
	// colours whose shards in the cost outnumber the untapped sources that
	// make them (MET-12: colour screw per missing colour).
	MissingColors mana.Colors
}

// CastabilityProbe asks, for every card in pid's hand, whether pid could cast
// or play it as the game stands: their mana sources (MET-6: solved against the
// real sources, then against the same number of sources making any colour),
// legal targets, timing, and statics that forbid it. It is meant for the end
// of pid's precombat main phase with an empty stack.
func (g *Game) CastabilityProbe(pid PlayerID) []CastProbe {
	// The recorder asks as the combat phase begins, which is the end of the
	// precombat main phase: answer as that main phase would, so a sorcery-speed
	// card is not blocked by the step the question happens to be asked in.
	if phase := g.activePhase; pid == g.activePlayer && phase == CombatBegin {
		g.activePhase = Main1
		defer func() { g.activePhase = phase }()
	}
	var out []CastProbe
	for _, id := range g.Zone(Hand, pid).Cards() {
		c := g.Card(id)
		p := CastProbe{Card: id}
		if c.Def != nil {
			p.Blocked, p.MissingColors = g.probeBlocks(pid, c)
		}
		p.Castable = p.Blocked == 0 && c.Def != nil
		out = append(out, p)
	}
	return out
}

func (g *Game) probeBlocks(pid PlayerID, c *Card) (CastBlock, mana.Colors) {
	var b CastBlock
	var missing mana.Colors
	if c.Type().Has(cardtype.Land) {
		if !g.hasLandDrop(pid) {
			b |= BlockLandDropUsed
		}
		if g.cantPlayLand(pid, c.ID) {
			b |= BlockOpponentLock
		}
		if pid != g.activePlayer || (g.activePhase != Main1 && g.activePhase != Main2) || len(g.stack) != 0 {
			b |= BlockTimingRestriction
		}
		return b, missing
	}
	if g.cantBeCast(pid, c.ID) {
		b |= BlockOpponentLock
	}
	if !c.Type().Has(cardtype.Instant) && !g.canActSorcerySpeed(pid) && !c.HasKeyword("Flash") {
		b |= BlockTimingRestriction
	}
	sp := firstSpellAbility(c)
	if sp != nil && !g.timingRestrictionsMet(pid, sp) {
		b |= BlockTimingRestriction
	}
	total, ok := g.castCost(pid, c, castOpts{}, false, nil)
	switch {
	case !ok:
		b |= BlockZoneOrStateRestriction
	case total.IsNoCost():
		b |= BlockZoneOrStateRestriction
	default:
		if _, paid := g.PlanMana(pid, total); !paid {
			if _, hypo := g.PlanManaAnyColor(pid, total); hypo {
				b |= BlockColorUnavailable
				missing = g.missingCostColors(pid, total)
			} else {
				b |= BlockInsufficientMana
			}
		}
	}
	if sp != nil {
		if api, known := APIByName(sp.Name); known {
			a := Ability{API: api, Source: c.ID, Controller: pid, Params: sp, Amounts: c.Def.Faces[0].Amounts, spell: true}
			if _, _, okTargets := g.targetChoiceFor(&a); !okTargets {
				b |= BlockNoLegalTarget
			}
		}
	}
	return b, missing
}

// shardOfColor is the one-colour shard of each colour, in WUBRG order.
var shardOfColor = [...]struct {
	color mana.Colors
	shard mana.Shard
}{{mana.White, mana.ShardW}, {mana.Blue, mana.ShardU}, {mana.Black, mana.ShardB}, {mana.Red, mana.ShardR}, {mana.Green, mana.ShardG}}

// missingCostColors are the colours of cost whose one-colour shards
// outnumber pid's untapped sources able to make that colour: a second black
// source short of {B}{B} names black. A cost blocked only through its hybrid
// or Phyrexian shards names the cost's colours no source makes.
func (g *Game) missingCostColors(pid PlayerID, cost mana.Cost) mana.Colors {
	sources := g.ManaSources(pid)
	var missing, avail mana.Colors
	for _, s := range sources {
		avail |= s.Colors
	}
	for _, sc := range shardOfColor {
		need := cost.Count(sc.shard)
		if need == 0 {
			continue
		}
		have := 0
		for _, s := range sources {
			if s.Colors&sc.color != 0 {
				have++
			}
		}
		if have < need {
			missing |= sc.color
		}
	}
	if missing == 0 {
		missing = cost.Colors() &^ avail
	}
	return missing
}
