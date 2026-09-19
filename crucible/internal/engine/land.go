// Playing a land: CR 305. Not casting a spell -- no cost, no stack.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// maxLandPlays is CR 305.2's default one land per turn, the base
// LandPlayLimit (player.go) folds Layer 8's own AdjustLandPlays$ continuous
// effects on top of.
const maxLandPlays = 1

// PlayLand is CR 305: playing a land is not casting a spell, so it has no
// cost to pay and never touches the stack (CR 305.1) -- the card moves
// straight from hand to the battlefield.
//
// Timing is CR 305.3's own gate, collapsed to what this port can check
// without an interactive priority system: pid's own turn, one of pid's main
// phases, and nothing already on the stack. CR 305.3 itself phrases the last
// two as "any time they could cast a sorcery" (CR 307.5's own definition);
// this port has no PlayerController method that lets anyone respond to
// anything on the stack yet (stack.go's own doc comment), so "stack empty"
// is never false today -- checked anyway, since land.go should not have to
// change again once casting exists to make it meaningful.
//
// Reports whether the land was played. false covers every legal-but-declined
// case: not pid's turn, not a main phase, something on the stack, the card
// is not in pid's hand, the card is not a land, or the per-turn limit
// (LandPlayLimit, player.go) is already spent -- the same "declined by the
// rules, not a bug" contract PayManaCost and TapLandForMana already carry.
func (g *Game) PlayLand(pid PlayerID, card CardID) bool {
	if pid != g.activePlayer {
		return false
	}
	if g.activePhase != Main1 && g.activePhase != Main2 {
		return false
	}
	if len(g.stack) != 0 {
		return false
	}
	c := g.Card(card)
	if c.Controller() != pid || c.Zone != Hand {
		return false
	}
	if !c.Type().Has(cardtype.Land) {
		return false
	}
	if limit, unlimited := g.Player(pid).LandPlayLimit(maxLandPlays); !unlimited && g.Player(pid).LandsPlayed >= limit {
		return false
	}
	origin := c.Zone
	g.Move(card, Battlefield, pid)
	g.Player(pid).LandsPlayed++
	g.checkMovedReplacement(card, origin)
	g.checkETBTriggers(card)
	return true
}
