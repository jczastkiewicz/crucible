// Prototype (CR 702.160): "Prototype <cost> -- <P>/<T>" is one more cast of
// the card for <cost>; the spell, and the permanent it becomes, has that mana
// cost, the colors of that cost and that power and toughness instead of the
// printed ones (CR 702.160b), until it leaves the battlefield (CR 702.160c).
//
// Ported from CardFactoryUtil.java:3330-3352, a copy of the first spell with
// the prototype cost carrying SetManaCost$, SetColorByManaCost$, SetPower$
// and SetToughness$, which Spell.java:206-212 turns into a clone state as the
// card goes on the stack (CardFactory.java:509-514, :576-577, :604-611) and
// Card.java:7492-7496 keeps on the battlefield. The new object a zone change
// makes has none of it (GameAction.java:207). Here the cast stores the three
// characteristics on the card (Card.prototype), which a move other than stack
// to battlefield clears.

package engine

//enginelint:allow id card game zone castoptions

// enterPrototyped gives card, put on the stack by a Prototype cast, its
// prototype characteristics (Spell.java:206-212).
func (g *Game) enterPrototyped(card CardID) {
	c := g.Card(card)
	if p, ok := prototypeWay(c); ok {
		c.prototype = &p
	}
}

// Prototyped reports whether c has its prototype characteristics.
func (c *Card) Prototyped() bool { return c.prototype != nil }
