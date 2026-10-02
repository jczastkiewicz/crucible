// The cast history of the turn: every spell cast, by whom and from where.
// Count$ThisTurnCast_<valid> (about 100 corpus uses: storm-like counts, Weftwalking's
// "first spell each turn") counts the entries whose card matches <valid> as
// it was when cast (MagicStack.getSpellsCastThisTurn, an LKI list).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/valid"
)

// recordSpellCast counts card as cast by pid and adds it to the turn's
// history. Called with the card already on the stack: from is where the cast
// started, kept by the cast path in Card.castFrom.
func (g *Game) recordSpellCast(pid PlayerID, card CardID) {
	g.Player(pid).SpellsCastThisTurn++
	g.castThisTurn = append(g.castThisTurn, castRecord{card: card, controller: pid, from: g.Card(card).castFrom})
}

// thisTurnCastCount is Count$ThisTurnCast_<spec>: the spells cast so far this
// turn whose card, read as its caster controlled it, matches spec from
// source's controller's point of view.
func (g *Game) thisTurnCastCount(source CardID, spec string) (int, bool) {
	if source == NoCard {
		return 0, false
	}
	parsed := valid.Parse(strings.TrimSpace(spec))
	host := g.Card(source)
	n := 0
	for _, rec := range g.castThisTurn {
		view := *g.Card(rec.card)
		view.controller = rec.controller
		if Matches(g, &view, parsed, host.Controller(), source) {
			n++
		}
	}
	return n, true
}
