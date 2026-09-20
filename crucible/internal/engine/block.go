// Declaring blockers: CR 509.1, the step after declaring attackers.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// Blocks returns the current combat's blocking assignments, if any.
func (g *Game) Blocks() []Block { return g.combat.Blocks }

// DeclareCombatBlockers is CR 509.1: the defending player chooses which of their
// untapped creatures block which attacker. Unlike declaring attackers,
// blocking does not tap the blocker -- CR 508.1f only taps attackers, CR
// 509 has no equivalent.
//
// The offered eligible list is still only "untapped creature the defending
// player controls" -- CR 509.1a's own "eligible" is a property of the
// creature (untapped, yours), not of a specific attacker/blocker pairing.
// CantBlockBy (CR 509.1b: flying/reach, Fear, Horsemanship, and every real
// corpus S: line in that shape, cantBlockBy in staticability.go) is a
// property of the pairing -- the same creature can be eligible in general
// and still illegal against one particular attacker -- so it is checked
// after the controller answers, via CanBlock, not folded into eligible.
// This is the one place a controller's own answer IS re-checked, unlike
// every other Choose*/Declare* method (control.go's own doc comment):
// "eligible" was never meant to encode a per-attacker answer, so nothing
// else here would catch a pairing this port now knows is illegal. An
// illegal pairing is dropped silently, the same as a controller declining
// to use part of what it was offered.
//
// Menace (CR 702.111b) is checked too, but not via CanBlock: it is a
// minimum-blocker-_count_ rule over the whole group assigned to one
// attacker, not a per-pair question, so it is a second filter (menaceLegal,
// below) applied after CanBlock's, on whatever CanBlock already accepted.
// Forge itself does not run Menace through the CantBlockBy static-ability
// engine either (cantBlockByKeywords' own doc comment, staticability.go) --
// getMinMaxBlocker hardcodes attacker.hasKeyword(Keyword.MENACE) directly,
// reproduced here the same way. Still not checked: Skulk, blocked on its
// own specific missing dependency (same doc comment).
//
// "Who is defending" is each attacker's own defender (defenderOf,
// attack.go) -- the controller of whatever it's attacking, a player,
// planeswalker or battle. A two-player game, or a multiplayer game where
// the active player sent every attacker at one opponent, has exactly one:
// that defender is asked once, for every attacker, the same as before CR
// 506.4's multiplayer case existed. When attackers are split across more
// than one defending player at once, each defender is asked in turn, only
// about the attackers actually attacking them, offering only their own
// eligible creatures -- CR 509.1's "the defending player" read per
// defender rather than assumed singular. Defenders are asked in the order
// their first attacker appears in attackers, so the sequence is
// deterministic across a run (GO-12).
//
// A defender with no eligible creature is skipped, not asked with an
// empty list -- the same reasoning DeclareCombatAttackers uses for an
// active player with nothing to attack with. The combined result is nil,
// not an empty non-nil slice, when every defender is skipped this way.
//
// checkBlocksTriggers/checkAttackerBlockedByCreatureTriggers (trigger.go)
// each run once per final Block, after every filter above -- CR 509.2's own
// "whenever ~ blocks"/"whenever ~ becomes blocked by a creature" fire only
// for a legally declared block, not one CanBlock or menaceLegal already
// dropped. checkAttackerBlockedTriggers (CR 509.2's own "becomes blocked,"
// the whole blocker group rather than one at a time) runs once per distinct
// attacker afterward, once every Block naming it is known.
func (g *Game) DeclareCombatBlockers(controller PlayerController) []Block {
	attackers := g.combat.Attackers
	if len(attackers) == 0 {
		return nil
	}

	byDefender := map[PlayerID][]CardID{}
	var defenders []PlayerID
	for _, id := range attackers {
		d := g.defenderOf(id)
		if _, ok := byDefender[d]; !ok {
			defenders = append(defenders, d)
		}
		byDefender[d] = append(byDefender[d], id)
	}

	var blocks []Block
	for _, defender := range defenders {
		var eligible []CardID
		for _, id := range g.Zone(Battlefield, defender).Cards() {
			c := g.Card(id)
			if c.Type().Has(cardtype.Creature) && !c.Tapped {
				eligible = append(eligible, id)
			}
		}
		if len(eligible) == 0 {
			continue
		}
		var accepted []Block
		for _, blk := range controller.DeclareCombatBlockers(g, defender, byDefender[defender], eligible) {
			if g.CanBlock(blk.Attacker, blk.Blocker) {
				accepted = append(accepted, blk)
			}
		}
		blocks = append(blocks, menaceLegal(g, accepted)...)
	}
	g.combat.Blocks = blocks
	blockersByAttacker := map[CardID][]CardID{}
	var blockedAttackers []CardID
	for _, blk := range blocks {
		g.checkBlocksTriggers(controller, blk)
		g.checkAttackerBlockedByCreatureTriggers(controller, blk)
		if _, ok := blockersByAttacker[blk.Attacker]; !ok {
			blockedAttackers = append(blockedAttackers, blk.Attacker)
		}
		blockersByAttacker[blk.Attacker] = append(blockersByAttacker[blk.Attacker], blk.Blocker)
	}
	for _, attacker := range blockedAttackers {
		g.checkAttackerBlockedTriggers(controller, attacker, blockersByAttacker[attacker])
	}
	return blocks
}

// menaceLegal drops every Block naming an attacker with Menace that ends up
// with fewer than two distinct blockers -- CR 702.111b, checked as a group
// once CanBlock has already filtered blocks down to individually legal
// pairs (DeclareCombatBlockers's own doc comment has the reason Menace is a
// second, group-cardinality filter rather than another CanBlock check). Both
// blockers of a legal two-plus group stay; every Block naming a Menace
// attacker that got only one is dropped entirely, not reduced to a
// single-blocker assignment -- CR 702.111b makes the whole attempt illegal
// to declare, not partially legal.
func menaceLegal(g *Game, blocks []Block) []Block {
	blockers := map[CardID]map[CardID]bool{}
	for _, blk := range blocks {
		if blockers[blk.Attacker] == nil {
			blockers[blk.Attacker] = map[CardID]bool{}
		}
		blockers[blk.Attacker][blk.Blocker] = true
	}
	var legal []Block
	for _, blk := range blocks {
		if g.Card(blk.Attacker).HasKeyword("Menace") && len(blockers[blk.Attacker]) < 2 {
			continue
		}
		legal = append(legal, blk)
	}
	return legal
}

// CanBlock reports whether blocker may legally block attacker (CR 509.1):
// an untapped creature the defender controls, the same base rule
// DeclareCombatBlockers's own eligible filter already applies, and not
// excluded by any Mode$ CantBlockBy static ability in play (cantBlockBy,
// staticability.go).
func (g *Game) CanBlock(attacker, blocker CardID) bool {
	b := g.Card(blocker)
	return b.Type().Has(cardtype.Creature) && !b.Tapped && !cantBlockBy(g, attacker, blocker)
}
