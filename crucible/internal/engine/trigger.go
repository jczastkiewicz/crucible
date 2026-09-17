// Trigger firing: CR 603, trimmed to the corpus's three most frequent modes
// -- a permanent entering the battlefield (Mode$ ChangesZone, Destination$
// Battlefield), one leaving it to a graveyard (Mode$ ChangesZone, Origin$
// Battlefield, Destination$ Graveyard, CR 700.4's "dies"), and a creature
// attacking (Mode$ Attacks, CR 508.3).

package engine

import (
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// checkETBTriggers is CR 603.2's "look back in time" for a card that just
// entered the battlefield, called from every real "moves onto the
// battlefield" site this port has (permanentEffect.Resolve, attachEffect.Resolve
// -- castspell.go; Game.PlayLand -- land.go), not from Game.Move itself:
// Matches (valid.go) depends on game.go, so game.go cannot depend back on
// anything that calls it without a cycle enginelint is built to catch
// (ability.go's own doc comment gives the identical reason Ability moved out
// of effect.go).
//
// This is CR 603's own trigger vocabulary at its narrowest: only
// Mode$ ChangesZone with Destination$ Battlefield fires (an ETB trigger),
// keyed off compile.Face.Triggers -- M3's own trigger-line compilation,
// already typed the same way an ability line is (compile.Ability), just
// never read by the engine before now. Every other trigger mode but Attacks
// and Dies (Tapped, a spell being cast, ...) is a gap this does not close
// (game-state.md's "Not ported yet"). CR 603.3b's own simultaneous-trigger
// ordering (a controller's own multiple triggers, in an order they choose;
// APNAP order between different controllers') does apply now that
// checkOtherETBTriggers exists -- one entering card can push both its own
// trigger and another permanent's -- but is not implemented: both are pushed
// in a fixed order (entered's own trigger, then every other battlefield
// permanent's own in Players()/zone order), not a chosen or APNAP one
// (game-state.md's "Not ported yet" has the same gap for addSimultaneousStackEntry).
//
// Checked against two sets of Triggers: entered's own ("when CARDNAME
// enters", ValidCard$ Card.Self, the overwhelming corpus-frequency shape --
// the loop directly below), and, since checkOtherETBTriggers below, every
// OTHER permanent already on the battlefield watching for one to enter
// ("Whenever another creature enters the battlefield under your control...").
// Both read the identical Mode$ ChangesZone/Destination$ Battlefield shape;
// they differ only in whose Triggers list is walked and what ValidCard is
// matched against.
//
// A matching trigger's own Execute$ sub-ability names the API it would run
// and carries that sub-ability's own params onto the stack as Ability.Params
// (triggerEffectAPI, below, ability.go) -- pushed the same way CastSpell
// pushes a cast spell, CR 603.3's own "triggered ability becomes an object on
// the stack." Resolving it is a different question: the 203 corpus-frequency
// effects a real trigger's own sub-ability can need are M6's job, one at a
// time as each lands in NewRegistry (draweffect.go's Draw is the first) --
// ResolveStack reports ErrUnimplemented for every API that has not yet
// (Registry.Resolve's own contract, effect.go) -- detecting and queuing a
// trigger correctly, regardless of whether its own Execute$ API happens to
// be implemented yet, is this port's whole job here.
func (g *Game) checkETBTriggers(entered CardID) {
	c := g.Card(entered)
	if c.Def != nil {
		for _, face := range c.Def.Faces {
			for _, t := range face.Triggers {
				if !isETBTrigger(t) {
					continue
				}
				validCard, ok := t.Param("ValidCard")
				if !ok {
					continue
				}
				if !Matches(g, c, valid.Parse(validCard), c.Controller, entered) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(t); ok {
					g.PushAbility(Ability{API: api, Source: entered, Controller: c.Controller, Params: sub})
				}
			}
		}
	}
	g.checkOtherETBTriggers(entered)
}

// checkOtherETBTriggers is checkETBTriggers's wider half: every permanent
// already on the battlefield, other than entered itself, gets its own
// Triggers walked against entered -- CR 603.2's "look back in time" applied
// from the watcher's side rather than the entering card's own. entered is
// skipped because its own Card.Self-shaped triggers are already handled by
// the loop above; running both loops over every card would fire that trigger
// twice.
//
// A watcher's ValidCard is matched with the watcher as source and the
// watcher's own controller, not entered's -- Matches's own doc comment
// ("from sourceController's perspective... source as the card the spec is
// written on"). "Other" and "YouCtrl" in a corpus ValidCard string
// (Creature.Other, Creature.nonSpirit+YouCtrl+Other) resolve against that
// pairing: Other compares entered's id to the watcher's, YouCtrl compares
// entered's controller to the watcher's controller, exactly the fields this
// passes.
//
// TriggerZones$ Battlefield -- present on most corpus lines shaped this way
// -- needs no separate check: only cards this loop already found on the
// battlefield are walked, so a watcher not there is never considered in the
// first place.
func (g *Game) checkOtherETBTriggers(entered CardID) {
	for _, pid := range g.Players() {
		for _, watcher := range g.Zone(Battlefield, pid).Cards() {
			if watcher == entered {
				continue
			}
			w := g.Card(watcher)
			if w.Def == nil {
				continue
			}
			for _, face := range w.Def.Faces {
				for _, t := range face.Triggers {
					if !isETBTrigger(t) {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(entered), valid.Parse(validCard), w.Controller, watcher) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						g.PushAbility(Ability{API: api, Source: watcher, Controller: w.Controller, Params: sub})
					}
				}
			}
		}
	}
}

// checkDiesTriggers is CR 603.6d's "look back in time" for a card that just
// left the battlefield to a graveyard -- checkETBTriggers's own narrowness,
// just for Mode$ ChangesZone's other corpus-frequent shape (Origin$
// Battlefield, Destination$ Graveyard, CR 700.4's "dies") instead of
// entering. Checked against the dying card's own Card.Self triggers here
// ("when CARDNAME dies"); checkOtherDiesTriggers, below, is the wider half.
//
// Card.Def is fixed at compile time and unaffected by the zone a card now
// sits in, so nothing here actually needs to look anything up as it "was":
// Def.Faces[i].Triggers reads the same list whether left is still on the
// battlefield or not, and c.Controller (game.go's own Move does not clear
// it on leaving) still reads the last real controller, exactly the
// last-known-information Java's own layer system gives a leaving card.
func (g *Game) checkDiesTriggers(left CardID) {
	c := g.Card(left)
	if c.Def != nil {
		for _, face := range c.Def.Faces {
			for _, t := range face.Triggers {
				if !isDiesTrigger(t) {
					continue
				}
				validCard, ok := t.Param("ValidCard")
				if !ok {
					continue
				}
				if !Matches(g, c, valid.Parse(validCard), c.Controller, left) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(t); ok {
					g.PushAbility(Ability{API: api, Source: left, Controller: c.Controller, Params: sub})
				}
			}
		}
	}
	g.checkOtherDiesTriggers(left)
}

// checkOtherDiesTriggers is checkDiesTriggers's wider half, the identical
// shape checkOtherETBTriggers is for entering: every permanent still on the
// battlefield gets its own Triggers walked against left, the card that just
// died ("Whenever a creature you control dies...", "Whenever another Cleric
// dies..."). No entered == left skip is needed the way checkOtherETBTriggers
// has one: left is already in the graveyard by the time this runs (every
// real call site moves it there first, action.go), so it never appears in
// the Battlefield walk to begin with -- unlike checkOtherETBTriggers, where
// the entered card is already ON the battlefield being walked.
//
// This closes the gap checkDiesTriggers's own doc comment used to name as
// not-yet-done: a watcher's own dies-shaped trigger needs left's state as a
// dying object, not the watcher's own zone -- the watcher itself is
// unaffected by left leaving and is exactly as reachable by a battlefield
// walk as any ETB watcher is, so nothing about "look back in time" actually
// blocks this the way an earlier version of this comment assumed.
func (g *Game) checkOtherDiesTriggers(left CardID) {
	for _, pid := range g.Players() {
		for _, watcher := range g.Zone(Battlefield, pid).Cards() {
			w := g.Card(watcher)
			if w.Def == nil {
				continue
			}
			for _, face := range w.Def.Faces {
				for _, t := range face.Triggers {
					if !isDiesTrigger(t) {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(left), valid.Parse(validCard), w.Controller, watcher) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						g.PushAbility(Ability{API: api, Source: watcher, Controller: w.Controller, Params: sub})
					}
				}
			}
		}
	}
}

// checkAttacksTriggers is CR 508.3's own "whenever ~ attacks" trigger,
// ported from TriggerAttacks.performTest at the one param this port can
// resolve: ValidCard matched against the declared attacker. Unlike
// checkETBTriggers/checkDiesTriggers, this needs no separate "own" and
// "other" loop: TriggerAttacks itself never special-cases the attacker's own
// trigger, so ValidCard$ Card.Self (1,282 of 1,606 real corpus lines) and
// ValidCard$ Creature.YouCtrl (an anthem-shaped "whenever a creature you
// control attacks") both fall out of the identical check below, just with
// different ValidCard strings and a different host.
//
// Not resolved: Attacked$ (47 real lines) -- TriggerAttacks.performTest
// matches it against a GameEntity (a player, planeswalker or Battle), and
// Matches (valid.go) only evaluates a *Card; Alone$ (57), FirstAttack$ (4),
// DefendingPlayerPoisoned$ (1) and AttackDifferentPlayers$ (1) -- each its
// own runtime condition (how many other attackers, a creature's own
// attack-count history, a player's poison count, attacking more than one
// player at once) this port tracks nothing for. A trigger carrying any of
// these five is skipped entirely, not fired unconditionally -- GO-7: better
// to miss a real trigger than fire one whose own restriction this port
// silently ignored. 1,496 of 1,606 real lines carry none of them.
func (g *Game) checkAttacksTriggers(attacker CardID) {
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !isAttacksTrigger(t) {
						continue
					}
					if hasAnyParam(t, "Attacked", "Alone", "FirstAttack", "DefendingPlayerPoisoned", "AttackDifferentPlayers") {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(attacker), valid.Parse(validCard), h.Controller, host) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						g.PushAbility(Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
}

// hasAnyParam reports whether t carries any of keys, regardless of value --
// checkAttacksTriggers' own way of skipping a trigger this port cannot fully
// evaluate rather than firing it as if the extra condition were not there.
func hasAnyParam(t *compile.Ability, keys ...string) bool {
	for _, k := range keys {
		if _, ok := t.Param(k); ok {
			return true
		}
	}
	return false
}

// isAttacksTrigger reports whether t is CR 508.3's "attacks" shape: Mode$
// Attacks.
func isAttacksTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Attacks")
}

// isETBTrigger reports whether t is CR 603.2's "enters the battlefield"
// shape: Mode$ ChangesZone with Destination$ Battlefield. Origin is
// unchecked -- entering from hand, library, graveyard or anywhere else all
// count, the corpus's own broad "enters" wording.
func isETBTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "ChangesZone") && hasZone(t, "Destination", "Battlefield")
}

// isDiesTrigger reports whether t is CR 700.4's "dies" shape: Mode$
// ChangesZone with Origin$ Battlefield and Destination$ Graveyard, both
// required -- unlike isETBTrigger, a bare Destination$ Graveyard alone would
// also match a discard or a mill, neither of which is a death.
func isDiesTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "ChangesZone") && hasZone(t, "Origin", "Battlefield") && hasZone(t, "Destination", "Graveyard")
}

// hasZone reports whether t's param key names zone among its comma-separated
// list of zones -- Destination$ Battlefield,Command among them, a real shape
// the corpus writes (a trigger that fires entering either zone).
func hasZone(t *compile.Ability, key, zone string) bool {
	v, ok := t.Param(key)
	if !ok {
		return false
	}
	for _, z := range strings.Split(v, ",") {
		if z == zone {
			return true
		}
	}
	return false
}

// triggerEffectAPI is a trigger's own Execute$ sub-ability -- the "DB$ <API>"
// record its SVar compiled into -- and the APIType that record's own Name
// names (compile.Ability's own Name field, the API for a Spell/DB record).
// The returned *compile.Ability is what Ability.Params carries onto the
// stack: an Effect's own Resolve reads Defined$/NumCards$/whatever else it
// needs straight off it (drawEffect, draweffect.go, is the first). Reports
// false for a trigger with no Execute key at all, or one naming an API
// string ApiType.java does not have (APIByName's own exact-match contract)
// -- neither is reachable against the real corpus today, but a card cannot
// be trusted not to be the first (PORT-8).
func triggerEffectAPI(t *compile.Ability) (*compile.Ability, APIType, bool) {
	for _, sub := range t.Subs {
		if !strings.EqualFold(sub.Key, "Execute") {
			continue
		}
		api, ok := APIByName(sub.Ability.Name)
		return sub.Ability, api, ok
	}
	return nil, 0, false
}
