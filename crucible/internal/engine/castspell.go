// Casting a spell: CR 601, trimmed to the one shape with nothing else to
// decide at cast time -- a permanent spell that is not an Aura.

package engine

import "github.com/jczastkiewicz/crucible/internal/cardtype"

// castableAsPermanent reports whether c is a permanent spell CastSpell can
// cast: a creature, artifact, enchantment, planeswalker or Battle, and not
// an Aura. Ported from CardState.java's getBasicSpells, which routes a
// permanent, non-Aura card to SpellPermanent and everything else (Aura,
// instant, sorcery) to a different SpellAbility this port does not build
// yet -- an Aura needs a target to attach to (CR 601.2c) and an
// instant/sorcery resolves into a script effect, neither of which this port
// has. A land is never a spell at all (CR 305.1) and is correctly excluded
// by not appearing in this list rather than by a special case.
func castableAsPermanent(c *Card) bool {
	t := c.Type()
	if t.HasSubtype("Aura") {
		return false
	}
	return t.Has(cardtype.Creature) || t.Has(cardtype.Artifact) || t.Has(cardtype.Enchantment) ||
		t.Has(cardtype.Planeswalker) || t.Has(cardtype.Battle)
}

// CastSpell is CR 601: pay the cost, then the spell becomes an object on the
// stack (CR 405.2, 601.2i) -- resolving is a separate step, ResolveStack.
// Timing is CR 601.3a's own default (sorcery speed, no flash this port can
// grant), collapsed the same way PlayLand's own CR 305.3 check is: active
// player, a main phase, an empty stack.
//
// Reports whether the spell was cast. false covers every legal-but-declined
// case: wrong timing, the card is not in pid's hand, castableAsPermanent
// says no, or the cost could not be paid -- the same "declined by the
// rules, not a bug" contract PayManaCost and PlayLand already carry. A
// failed cost payment leaves the pool exactly as PayManaCost already
// guarantees, and the card never leaves hand.
//
// A successful cast fires SpellCast (ADR-0013's own schema has named this
// kind since M4, with nothing to emit it until now).
func (g *Game) CastSpell(pid PlayerID, card CardID, controller PlayerController) bool {
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
	if c.Controller != pid || c.Zone != Hand {
		return false
	}
	if !castableAsPermanent(c) {
		return false
	}
	if !g.PayManaCost(pid, c.Def.Faces[0].ManaCost, controller) {
		return false
	}
	g.Move(card, Stack, pid)
	api := APIPermanentNoncreature
	if c.Type().Has(cardtype.Creature) {
		api = APIPermanentCreature
	}
	g.PushAbility(Ability{API: api, Source: card, Controller: pid})
	g.sink.Emit(Event{Kind: SpellCast, Phase: g.activePhase, Active: g.activePlayer, Actor: pid, Turn: uint16(g.turn), Source: card})
	return true
}

// permanentEffect is CR 608.2m/608.3g's own resolution for a permanent
// spell, trimmed to what this port can support: no Dash/Blitz/Warp/Sneak
// alternate-cast-mode handling (PermanentEffect.java's own resolve checks
// host.wasCast()/isDash()/isBlitz()/isWarp()/isSneak(), none of which this
// port can grant a spell), no triggers firing off the resulting zone change
// (table.triggerChangesZoneAll -- CR 603 firing is not built). What is left,
// ported directly, is the whole of what both APIPermanentCreature and
// APIPermanentNoncreature actually do: the card leaves the stack and
// becomes a permanent. Java splits the two only for getStackDescription's
// own display text (PermanentCreatureEffect overrides it to show P/T); this
// port has no stack-description system, so one stateless value answers for
// both.
type permanentEffect struct{}

func (permanentEffect) Resolve(g *Game, a *Ability) error {
	g.Move(a.Source, Battlefield, a.Controller)
	return nil
}

// NewRegistry builds a Registry carrying every Effect this port has. Two
// entries today: APIPermanentCreature and APIPermanentNoncreature share
// permanentEffect, CastSpell's own first (and so far only) real caller of
// PushAbility outside stack.go's tests. Explicit construction here, not an
// init() populating a package-level Registry, is ADR-0003's own "explicit
// wiring... so the direction stays visible and test binaries can register a
// subset" -- a caller that wants fewer registered APIs builds its own
// Registry by hand instead of calling this.
func NewRegistry() *Registry {
	var r Registry
	r[APIPermanentCreature] = permanentEffect{}
	r[APIPermanentNoncreature] = permanentEffect{}
	return &r
}
