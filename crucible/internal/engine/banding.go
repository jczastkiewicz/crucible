// Banding (CR 702.22, ADR-0057): attacking creatures declared as a band are
// blocked together (CR 702.22h), and the side with banding divides the other
// side's combat damage (CR 702.22j-k).
//
// Ported from AttackingBand.java:29-65 (isValidBand), Combat.java:267-292 (a
// band per attacker, a singleton by default), :402-404 (a block is on the
// band) and the two assigning-player switches, :764-781 for a blocker's damage
// and :836-841 for a blocked attacker's. Java's bands ride on
// PlayerController.declareAttackers; here they are their own decision,
// DeclareBands, asked only when an attacker can band. Combat.bands holds the
// declared bands of two or more; every other attacker is a band of one.

package engine

//enginelint:allow id card game combat control keyword valid blockeffect

import (
	"github.com/jczastkiewicz/crucible/internal/keyword"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// canBand reports whether c has Banding or a "Bands with other" keyword.
func canBand(c *Card) bool {
	return c.HasKeyword("Banding") || c.HasKeyword("Bands with other")
}

// validBand is AttackingBand.isValidBand: a band to form (shareDamage false)
// may have one member without banding, a band that shares damage needs one
// member with it; either is valid when a member's "Bands with other:<valid>"
// matches every member.
func (g *Game) validBand(band []CardID, shareDamage bool) bool {
	if len(band) == 0 {
		return false
	}
	banding := 0
	for _, id := range band {
		if g.Card(id).HasKeyword("Banding") {
			banding++
		}
	}
	need := len(band) - 1
	if shareDamage {
		need = 1
	}
	if need <= banding {
		return true
	}
	for _, id := range band {
		c := g.Card(id)
		for _, line := range c.KeywordLines() {
			k := keyword.Parse(line)
			if k.Name != "Bands with other" || len(k.Args()) == 0 || k.Args()[0] == "" {
				continue
			}
			spec := valid.Parse(k.Args()[0])
			all := true
			for _, other := range band {
				all = all && Matches(g, g.Card(other), spec, c.Controller(), id)
			}
			if all {
				return true
			}
		}
	}
	return false
}

// declareBands asks the active player for the bands among attackers, when one
// of them can band, and checks the answer (ADR-0024): every member a declared
// attacker in one band only, all attacking the same target, and each band of
// two or more valid. It returns the bands of two or more.
func (g *Game) declareBands(controller PlayerController, attackers []CardID, targets map[CardID]EntityID) ([][]CardID, error) {
	can := false
	for _, id := range attackers {
		can = can || canBand(g.Card(id))
	}
	if !can {
		return nil, nil
	}
	var bands [][]CardID
	var seen []CardID
	for _, band := range controller.DeclareBands(g, g.activePlayer, attackers) {
		for _, id := range band {
			if !containsCard(attackers, id) {
				return nil, illegal("CR 702.22c", "a band member is not a declared attacker", id)
			}
			if containsCard(seen, id) {
				return nil, illegal("CR 702.22c", "an attacker is in two bands", id)
			}
			if targets[id] != targets[band[0]] {
				return nil, illegal("CR 702.22c", "a band's members attack different targets", band...)
			}
			seen = append(seen, id)
		}
		if len(band) < 2 {
			continue
		}
		if !g.validBand(band, false) {
			return nil, illegal("CR 702.22c", "not a valid band: more than one member without banding", band...)
		}
		bands = append(bands, append([]CardID(nil), band...))
	}
	return bands, nil
}

// bandOf is the band attacker attacks in: its declared band, or itself alone.
func (c *Combat) bandOf(attacker CardID) []CardID {
	for _, band := range c.bands {
		if containsCard(band, attacker) {
			return band
		}
	}
	return []CardID{attacker}
}

// keepBands drops from every band the attackers no longer in kept, and a band
// left with fewer than two members (Java leaves an emptied band in its map,
// Combat.java:280, :640, read by nothing here).
func (c *Combat) keepBands(kept []CardID) {
	var bands [][]CardID
	for _, band := range c.bands {
		var left []CardID
		for _, id := range band {
			if containsCard(kept, id) {
				left = append(left, id)
			}
		}
		if len(left) >= 2 {
			bands = append(bands, left)
		}
	}
	c.bands = bands
}

// expandBandBlocks is CR 702.22h (Combat.java:402-404): a creature blocking
// one member of a band blocks every member. The added blocks follow the
// declared one, in band order.
func (c *Combat) expandBandBlocks(blocks []Block) []Block {
	if len(c.bands) == 0 {
		return blocks
	}
	var out []Block
	for _, b := range blocks {
		if !containsBlock(out, b) {
			out = append(out, b)
		}
		for _, member := range c.bandOf(b.Attacker) {
			if blk := (Block{Blocker: b.Blocker, Attacker: member}); !containsBlock(out, blk) {
				out = append(out, blk)
			}
		}
	}
	return out
}
