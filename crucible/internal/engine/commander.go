// The Commander designation (ADR-0051): Player.addCommander / removeCommander
// (Player.java:2838-2854) and Card.setCommander, as AlterAttribute's Commander
// attribute calls them. addCommander also makes the owner's Commander Effect
// card once (Player.createCommanderEffect, ADR-0053): its 903.9b Moved
// replacement sends a commander bound for the hand or library to the Command
// zone, its MayPlay$ static lets the owner cast it from there, and the cast
// is counted (castrecord.go) and taxed (commanderTax).
//
// The Commander variant (ADR-0055) is GameRules.appliedVariants: a bit set
// on the game, with its starting life, commander damage and the 21-damage
// loss (Player.java:690-698, 2077-2083) and the CR 903.9a return
// (GameAction.stateBasedAction_Commander). Brawl, Oathbreaker, TinyLeaders
// and Planeswalker are not ported.

package engine

//enginelint:allow id card player game zone control

import (
	"slices"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
)

// variantLifeBonus is RegisteredPlayer.forVariants's Commander step
// (RegisteredPlayer.java:155-157): twenty over the default twenty.
const variantLifeBonus = 20

// commanderDamageLoss is Player.java:2079's hard-coded 21 (CR 903.10a).
const commanderDamageLoss = 21

// HasVariant is GameRules.hasAppliedVariant.
func (g *Game) HasVariant(v Variant) bool { return g.variants&v == v }

// applyVariant is GameRules.addAppliedVariant: AlterAttribute's Commander
// applies the variant mid-game (AlterAttributeEffect.java:119-124), with no
// change to anyone's life.
func (g *Game) applyVariant(v Variant) { g.variants |= v }

// SetupVariant is the match setup for v, before the opening hands: the
// variant applies and every seat starts at its starting life
// (RegisteredPlayer.forVariants, Game.java:375-378).
func (g *Game) SetupVariant(v Variant) {
	g.applyVariant(v)
	life := defaultStartingLife
	if v&VariantCommander != 0 {
		life += variantLifeBonus
	}
	for _, pid := range g.Players() {
		p := g.Player(pid)
		p.StartingLife, p.Life = life, life
	}
}

// SeatCommander is Player.initVariantsZones's commander step
// (Player.java:3016-3025): a card made from def in owner's Command zone,
// then designated, which also makes the Commander Effect once.
func (g *Game) SeatCommander(owner PlayerID, def *compile.Card) CardID {
	id := g.NewCard(def, owner, Command)
	g.addCommander(owner, id)
	return id
}

// realCommander is Card.getRealCommander (Card.java:7466-7476): the card
// itself when it is a commander, else its meld partner when that is one,
// else the first merged component that is one; NoCard when none is.
func (g *Game) realCommander(id CardID) CardID {
	c := g.Card(id)
	switch {
	case c.commander:
		return id
	case c.MeldedWith != NoCard && g.Card(c.MeldedWith).commander:
		return c.MeldedWith
	case c.Zone == Battlefield:
		for _, m := range c.merged {
			if g.Card(m).commander {
				return m
			}
		}
	}
	return NoCard
}

// noteCommanderDamage is the Player.java:690-698 step of
// addDamageAfterPrevention: combat damage a commander deals to target is
// added under its real commander. No variant gates it here: the three that
// exclude it (Oathbreaker, TinyLeaders, Brawl) are not ported.
func (g *Game) noteCommanderDamage(source CardID, target PlayerID, amount int) {
	cmd := g.realCommander(source)
	if cmd == NoCard {
		return
	}
	p := g.Player(target)
	for i := range p.commanderDamage {
		if p.commanderDamage[i].commander == cmd {
			p.commanderDamage[i].n += amount
			return
		}
	}
	p.commanderDamage = append(p.commanderDamage, commanderCastCount{commander: cmd, n: amount})
}

// CommanderDamage is Player.getCommanderDamage(commander): the combat
// damage pid was dealt by commander this game.
func (g *Game) CommanderDamage(pid PlayerID, commander CardID) int {
	for _, e := range g.Player(pid).commanderDamage {
		if e.commander == commander {
			return e.n
		}
	}
	return 0
}

// lostToCommanderDamage is Player.java:2077-2083's test: under the
// Commander variant, one commander has dealt pid 21 or more.
func (g *Game) lostToCommanderDamage(pid PlayerID) bool {
	if !g.HasVariant(VariantCommander) {
		return false
	}
	for _, e := range g.Player(pid).commanderDamage {
		if e.n >= commanderDamageLoss {
			return true
		}
	}
	return false
}

// returnCommandersToCommandZone is CR 903.9a
// (GameAction.stateBasedAction_Commander, GameAction.java:1853-1866, run at
// :1561-1570): under the variant, each player's graveyard then exile is
// walked for a commander moved since the last check (toCommandZone, set by
// Game.Move); the mark is cleared and the owner asked whether it goes to the
// Command zone. True when one moved.
func (g *Game) returnCommandersToCommandZone(controller PlayerController) bool {
	if !g.HasVariant(VariantCommander) {
		return false
	}
	moved := false
	for _, pid := range g.Players() {
		for _, z := range []ZoneType{Graveyard, Exile} {
			// threadSafeIterable: a snapshot, since each return removes a
			// card from the zone being walked.
			for _, id := range slices.Clone(g.Zone(z, pid).Cards()) {
				c := g.Card(id)
				if c.Zone != z || !c.commander || !c.toCommandZone {
					continue
				}
				c.toCommandZone = false
				if controller.ConfirmEffect(g, c.Owner, id) {
					// moveTo with the SBA's params: the move's replacements
					// and triggers run as for any effect move.
					g.moveByEffect(controller, id, Command, 0, NoPlayer, false)
					moved = true
				}
			}
		}
	}
	return moved
}

// addCommander makes id one of owner's commanders, once.
func (g *Game) addCommander(owner PlayerID, id CardID) {
	p := g.Player(owner)
	if !slices.Contains(p.commanders, id) {
		p.commanders = append(p.commanders, id)
	}
	if p.commanderEffect == NoCard {
		// Permanent like the monarch's card: a zero lifetime would end it at
		// the first cleanup.
		p.commanderEffect = g.putDesignationCard(NoCard, owner, g.db.CommanderEffect)
	}
	g.Card(id).commander = true
}

// IsCommander is Card.isCommander for id (ADR-0051).
func (g *Game) IsCommander(id CardID) bool { return g.Card(id).isCommander() }

// AddCommander makes id, a card owner owns, one of owner's commanders: the
// Commander setup a test or fixture gives a game (ADR-0053).
func (g *Game) AddCommander(owner PlayerID, id CardID) { g.addCommander(owner, id) }

// commanderTax is the CostAdjustment.java:56-61 step: two generic mana for
// each time pid cast card, a commander cast from the Command zone, before.
func (g *Game) commanderTax(pid PlayerID, c *Card) int {
	if !c.isCommander() || c.Zone != Command {
		return 0
	}
	return 2 * g.commanderCastsOf(pid, c.ID)
}

// removeCommander is Player.removeCommander: id is no longer a commander.
func (g *Game) removeCommander(owner PlayerID, id CardID) {
	p := g.Player(owner)
	p.commanders = slices.DeleteFunc(p.commanders, func(c CardID) bool { return c == id })
	g.Card(id).commander = false
}

// Commanders is pid's commanders, in the order they were designated.
func (g *Game) Commanders(pid PlayerID) []CardID {
	return slices.Clone(g.Player(pid).commanders)
}
