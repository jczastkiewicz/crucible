// Reconfigure (CR 702.151): an Equipment creature with reconfigure may be
// attached to a creature (GameEntity.java:244 exempts it from "a creature
// can't be attached"), and while attached it is not a creature. The keyword's
// Attach and Unattach abilities are keyword.Expand's; Card.attachToEntity
// (Card.java:3973-3985) makes the type change an effect under a fresh
// timestamp each time a card with reconfigure becomes attached, ended by its
// unattach or leaving play.

package engine

//enginelint:allow id card game animate typemod parts

import (
	"github.com/jczastkiewicz/crucible/internal/cardtype"
)

// startReconfigure is Card.attachToEntity's Reconfigure Effect: id, just
// attached, stops being a creature. Nothing happens to a card without the
// keyword.
func (g *Game) startReconfigure(id CardID) {
	c := g.Card(id)
	if !c.HasKeyword("Reconfigure") {
		return
	}
	g.dropReconfigure(id)
	g.timestamp++
	c.reconfigureTS = g.timestamp
	g.addAnimate(animateRecord{
		Card: id, Timestamp: c.reconfigureTS, Permanent: true,
		Types: TypeEffect{RemoveTypes: cardtype.ParseToken("Creature")},
	})
}

// dropReconfigure ends id's Reconfigure Effect, the unattach command and the
// leaves-play command Java registers with it. It reports whether there was
// one.
func (g *Game) dropReconfigure(id CardID) bool {
	c := g.Card(id)
	if c.reconfigureTS == 0 {
		return false
	}
	kept := g.animates[:0]
	for _, r := range g.animates {
		if r.Card != id || r.Timestamp != c.reconfigureTS {
			kept = append(kept, r)
		}
	}
	g.animates = kept
	c.reconfigureTS = 0
	return true
}
