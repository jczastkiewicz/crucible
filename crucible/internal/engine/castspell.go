// Casting a spell: CR 601, trimmed to the two shapes with nothing left to
// decide once a target (an Aura) or nothing (every other permanent) is
// chosen -- an instant or sorcery still resolves into a script effect this
// port does not build.

package engine

import (
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// castableAsPermanent reports whether c is a non-Aura permanent spell
// CastSpell's own no-target branch can cast: a creature, artifact,
// enchantment, planeswalker or Battle. Ported from CardState.java's
// getBasicSpells, which routes a permanent, non-Aura card to SpellPermanent
// -- an Aura routes to getAuraSpell() instead (castAura, below) since it
// needs a target chosen at cast time (CR 601.2c) that this shape has none
// of; an instant or sorcery resolves into a script effect this port does not
// build. A land is never a spell at all (CR 305.1) and is correctly excluded
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
// kind since M4, with nothing to emit it until now), then checks CR 603's
// own "whenever a player casts a spell" trigger (checkSpellCastTriggers,
// trigger.go) -- fired at cast time, not on resolution, the same place
// Java's own checkTriggerEffects call sits.
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

	if c.Type().HasSubtype("Aura") {
		return g.castAura(pid, card, c, controller)
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
	g.checkSpellCastTriggers(card, pid)
	return true
}

// castAura is CastSpell's own Aura branch (CardState.java's getAuraSpell,
// the "SP$ Attach" ability it builds, and AttachEffect.java's own resolve).
// CR 601.2c puts choosing a target before paying the cost, the one thing
// that makes an Aura's cast different from castableAsPermanent's own
// no-decision case: a target is picked here, carried on the pushed Ability
// (Target, ability.go) to wherever attachEffect (below) reads it back at
// resolution.
//
// Reports false for every legal-but-declined case castableAsPermanent's own
// CastSpell branch already has, plus two more: enchantSpec finds nothing
// checkable (an "Enchant Player"/"Enchant Opponent" Aura, its own doc
// comment's gap -- this port cannot tell a legal host from an illegal one
// without a card-type spec to check), or the battlefield has no legal host
// at all (CR 601.2c: a spell requiring a target that has none is illegal to
// cast, not one cast with nothing to point at).
func (g *Game) castAura(pid PlayerID, card CardID, c *Card, controller PlayerController) bool {
	spec, ok := enchantSpec(c)
	if !ok {
		return false
	}
	eligible := g.enchantTargets(spec, pid, card)
	if len(eligible) == 0 {
		return false
	}
	target := eligible[0]
	if len(eligible) > 1 {
		target = controller.ChooseEnchantTarget(g, pid, card, eligible)
	}
	if !g.PayManaCost(pid, c.Def.Faces[0].ManaCost, controller) {
		return false
	}
	g.Move(card, Stack, pid)
	g.PushAbility(Ability{API: APIAttach, Source: card, Controller: pid, Target: target})
	g.sink.Emit(Event{Kind: SpellCast, Phase: g.activePhase, Active: g.activePlayer, Actor: pid, Turn: uint16(g.turn), Source: card})
	g.checkSpellCastTriggers(card, pid)
	return true
}

// enchantTargets is every battlefield permanent, across every player, that
// spec (self's own Enchant restriction, enchantSpec) matches, and that does
// not refuse self outright (hostRefusesEnchant, staticability.go -- CR
// 702.16e/702.11h's own Protection/Hexproof gate, a separate question from
// the card-type restriction spec itself checks) -- CR 601.2c's legal-target
// set for casting self as an Aura. Matches' own source parameter is self,
// the same "the enchantment's own id, not the host's" convention
// cleanupDanglingAttachments (action.go) already uses when re-checking an
// attached Aura's own restriction after the fact.
func (g *Game) enchantTargets(spec valid.Spec, controller PlayerID, self CardID) []CardID {
	var eligible []CardID
	for _, pid := range g.Players() {
		for _, id := range g.Zone(Battlefield, pid).Cards() {
			if Matches(g, g.Card(id), spec, controller, self) && !hostRefusesEnchant(g, g.Card(self), id) {
				eligible = append(eligible, id)
			}
		}
	}
	return eligible
}

// permanentEffect is CR 608.2m/608.3g's own resolution for a permanent
// spell, trimmed to what this port can support: no Dash/Blitz/Warp/Sneak
// alternate-cast-mode handling (PermanentEffect.java's own resolve checks
// host.wasCast()/isDash()/isBlitz()/isWarp()/isSneak(), none of which this
// port can grant a spell). What is left, ported directly, is the whole of
// what both APIPermanentCreature and APIPermanentNoncreature actually do:
// the card leaves the stack and becomes a permanent, and CR 603.2's own ETB
// trigger check runs against it (checkETBTriggers, trigger.go) the same way
// Java's table.triggerChangesZoneAll does after every zone change. Java
// splits the two APIs only for getStackDescription's own display text
// (PermanentCreatureEffect overrides it to show P/T); this port has no
// stack-description system, so one stateless value answers for both.
type permanentEffect struct{}

func (permanentEffect) Resolve(g *Game, a *Ability) error {
	g.Move(a.Source, Battlefield, a.Controller)
	g.checkETBTriggers(a.Source)
	return nil
}

// attachEffect is CR 601.2c/608.2c's own resolution for an Aura: move it to
// the battlefield, then attach it to the target castAura chose at cast time
// (Ability.Target) -- Java's own AttachEffect.resolve does the two in the
// same order (moveToPlay before attachToEntity), though nothing here would
// change if they ran the other way since Move and Attach touch disjoint
// fields.
//
// Not ported: CR 608.2b's fizzle check, re-validating the target is still
// legal right before this runs. This port has no way to make a chosen
// target illegal between casting and resolving yet -- no responses exist,
// so nothing can happen to the target in between -- and if that ever stops
// being true, the existing cleanupDanglingAttachments state-based action
// (action.go) already catches an Aura attached to an illegal host, however
// it got there, on the very next check.
type attachEffect struct{}

func (attachEffect) Resolve(g *Game, a *Ability) error {
	g.Move(a.Source, Battlefield, a.Controller)
	g.Attach(a.Source, a.Target)
	g.checkETBTriggers(a.Source)
	return nil
}

// NewRegistry builds a Registry carrying every Effect this port has. Four
// entries today: APIPermanentCreature and APIPermanentNoncreature share
// permanentEffect, CastSpell's own first (and so far only) real caller of
// PushAbility outside stack.go's tests; APIAttach is attachEffect, castAura's
// own; APIDraw is drawEffect (draweffect.go), M6's own first script-driven
// effect and trigger.go's first real Execute$ sub-ability to actually
// resolve rather than report ErrUnimplemented. Explicit construction here,
// not an init() populating a package-level Registry, is ADR-0003's own
// "explicit wiring... so the direction stays visible and test binaries can
// register a subset" -- a caller that wants fewer registered APIs builds its
// own Registry by hand instead of calling this.
func NewRegistry() *Registry {
	var r Registry
	r[APIPermanentCreature] = permanentEffect{}
	r[APIPermanentNoncreature] = permanentEffect{}
	r[APIAttach] = attachEffect{}
	r[APIDraw] = drawEffect{}
	return &r
}
