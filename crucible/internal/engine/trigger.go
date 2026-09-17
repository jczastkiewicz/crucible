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

// checkSpellCastTriggers is CR 603's own "whenever a player casts a spell"
// trigger, ported from TriggerSpellAbilityCastOrCopy.performTest -- fired
// once a spell is cast (put on the stack, cost paid), not on resolution,
// since that is what "cast" means in Java's own cast-time
// checkTriggerEffects call (castspell.go's two call sites, right where the
// SpellCast event itself already fires). Like checkAttacksTriggers, a single
// walk covers both a card's own "whenever you cast a creature spell" and
// another permanent's "whenever a player casts an instant" -- Java's own
// performTest never special-cases the caster's own trigger, just ValidCard
// and ValidActivatingPlayer both being ordinary matchesValidParam checks.
//
// ValidCard is optional in Java (matchesValidParam returns true when the
// param is absent, CardTraitBase.matchesValidParam) -- 100 of 1,435 real
// corpus lines have none at all ("whenever you cast a spell", no
// restriction on which one) -- so a missing ValidCard is a pass, not a
// skip, unlike checkETBTriggers/checkDiesTriggers/checkAttacksTriggers,
// where the corpus shape this port covers always carries one.
//
// ValidActivatingPlayer is the corpus's dominant param here (1,216 of 1,435
// lines -- more common than ValidCard itself), matched by
// matchesActivatingPlayer below against three bare values covering 1,191 of
// those 1,216 (You/Opponent/Player); a qualified form (Player.Opponent,
// Player.EnchantedBy, Player.NonActive, Player.Active, Player.Other,
// Player.Chosen -- 25 lines) has no player-valid evaluator this port
// builds, so it is skipped the same way hasAnyParam skips a trigger with an
// unresolved card-valid param: never fires, rather than fires unconditionally
// (GO-7).
//
// Not resolved, skipped via hasAnyParam below: ValidSA/ValidSAonCard (a
// SpellAbility, not a Card, Matches (valid.go) only evaluates one of
// those), TargetsValid/CanTargetOtherCondition (this port's targeting has
// no per-trigger target-inspection hook), HasXManaCost/NoColoredMana/
// SnowSpentForCardsColor (no mana-payment-detail tracking past whether the
// cost was paid), IsSingleTarget (no generic target-count reader),
// TriggersWhenSpent (a mana-ability-specific remembered-list this port has
// no mana-ability ChosenPlayer/Remembered plumbing for) and
// ActivatorThisTurnCast/ActivatorThisTurnCastEach (a per-turn cast-history
// count this port tracks nothing for). 1,163 of 1,435 real lines carry none
// of these.
func (g *Game) checkSpellCastTriggers(cast CardID, activator PlayerID) {
	c := g.Card(cast)
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !isSpellCastTrigger(t) {
						continue
					}
					if hasAnyParam(t, "ValidSA", "ValidSAonCard", "TargetsValid", "CanTargetOtherCondition",
						"HasXManaCost", "IsSingleTarget", "NoColoredMana", "SnowSpentForCardsColor",
						"TriggersWhenSpent", "ActivatorThisTurnCast", "ActivatorThisTurnCastEach") {
						continue
					}
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller, host) {
						continue
					}
					if !matchesActivatingPlayer(t, activator, h.Controller) {
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

// matchesActivatingPlayer is ValidActivatingPlayer's own check, ported from
// matchesValidParam("ValidActivatingPlayer", activator) -- matchesPlayerBase
// (valid.go)'s own three bare values. Missing entirely is a pass, the same
// CardTraitBase.matchesValidParam contract ValidCard's own absence gets
// above. Any qualified form (a dot in the value) is unrecognized and never
// matches -- checkSpellCastTriggers' own doc comment names the six real
// ones this cannot evaluate.
func matchesActivatingPlayer(t *compile.Ability, activator, hostController PlayerID) bool {
	v, ok := t.Param("ValidActivatingPlayer")
	if !ok {
		return true
	}
	matched, recognized := matchesPlayerBase(activator, hostController, v)
	return recognized && matched
}

// isSpellCastTrigger reports whether t is CR 603's "a player casts a spell"
// shape: Mode$ SpellCast.
func isSpellCastTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "SpellCast")
}

// checkBlocksTriggers is CR 509.2's own "whenever ~ blocks" trigger, ported
// from TriggerBlocks.performTest at the one param this port can resolve:
// ValidCard matched against the declared blocker. Like checkAttacksTriggers,
// one walk covers both a creature's own "when this blocks" and another
// permanent's "whenever a creature you control blocks" -- TriggerBlocks
// itself never special-cases the blocker's own trigger either.
//
// Not resolved: ValidBlocked$ (8 of 127 real lines) -- TriggerBlocks.performTest
// matches it against the full collection of attackers this blocker blocks
// (AbilityKey.Attackers), which this port's Block (combat.go) never groups
// back into a per-blocker set of attackers, so a trigger carrying it is
// skipped entirely rather than checked against only the one attacker in blk.
// 119 of 127 real lines carry none of it.
//
// Called once per declared Block, after CanBlock and menaceLegal have both
// already filtered the pairing down to a legal one (DeclareCombatBlockers,
// block.go) -- a blocker declared against more than one attacker at once (a
// real corpus rarity this port's own combat model does not otherwise
// restrict) fires once per Block entry rather than once with every attacker
// gathered, the same per-pair granularity every other Block-consuming caller
// already uses.
func (g *Game) checkBlocksTriggers(blk Block) {
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !isBlocksTrigger(t) {
						continue
					}
					if hasAnyParam(t, "ValidBlocked") {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(blk.Blocker), valid.Parse(validCard), h.Controller, host) {
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

// isBlocksTrigger reports whether t is CR 509.2's "blocks" shape: Mode$
// Blocks.
func isBlocksTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Blocks")
}

// checkDamageDoneTriggersToCard and checkDamageDoneTriggersToPlayer are CR
// 603's own "whenever ~ deals damage" mode, Mode$ DamageDone, ported from
// TriggerDamageDone.performTest -- split in two because the actual damaged
// object is either a *Card (a creature, planeswalker or battle) or a
// *Player, and ValidTarget needs a different evaluator for each: Matches
// (valid.go) for the first, matchesPlayerBase (valid.go, the same one
// matchesActivatingPlayer/matchesValidDefender already use) for the second.
// damageDoneMatches (below) is everything the two calls share -- one walk
// over the battlefield, ValidSource, and CombatDamage$ -- everything but
// that one different check.
//
// isCombat is always true at both real call sites (dealPermanentDamage/
// dealPlayerDamage, combatdamage.go): nothing outside combat deals damage
// in this port yet (game-state.md's "Not ported yet"), so a
// CombatDamage$ False line (a rare "whenever ~ deals noncombat damage"
// shape) never fires and a CombatDamage$ True line or one carrying neither
// always passes that part of the check.
func (g *Game) checkDamageDoneTriggersToCard(source, target CardID, isCombat bool) {
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !damageDoneMatches(g, t, source, h, host, isCombat) {
						continue
					}
					if validTarget, ok := t.Param("ValidTarget"); ok &&
						!Matches(g, g.Card(target), valid.Parse(validTarget), h.Controller, host) {
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

func (g *Game) checkDamageDoneTriggersToPlayer(source CardID, target PlayerID, isCombat bool) {
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !damageDoneMatches(g, t, source, h, host, isCombat) {
						continue
					}
					if validTarget, ok := t.Param("ValidTarget"); ok {
						matched, recognized := matchesPlayerBase(target, h.Controller, validTarget)
						if !recognized || !matched {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						g.PushAbility(Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
}

// damageDoneMatches is checkDamageDoneTriggersToCard/ToPlayer's own shared
// half: is t a Mode$ DamageDone trigger this port can evaluate at all
// (isDamageDoneTrigger, no unresolved param), does ValidSource match (absent
// is a pass, matchesValidParam's own contract, the same as ValidCard's own
// absence in checkSpellCastTriggers), and does CombatDamage$ agree with
// isCombat -- everything but ValidTarget, which the two callers each check
// their own way.
//
// Not resolved, skipped via hasAnyParam: DamageAmount$ (8 of 1,080 real
// lines) -- a plain integer or TargetToughness both need
// AbilityUtils.calculateAmount, the same gap ptParam (continuous.go) and
// Draw's own NumCards$ already have; ValidCause$ (1) -- a SpellAbility, not
// a Card, Matches cannot evaluate one; TargetRelativeToCause$/
// TargetRelativeToSource$ (0 real lines alongside the shapes above) -- a
// GameEntity-vs-GameEntity relative match this port has no evaluator for. A
// trigger carrying any of these is skipped entirely, not fired
// unconditionally (GO-7). 1,071 of 1,080 real lines carry none of them.
func damageDoneMatches(g *Game, t *compile.Ability, source CardID, h *Card, host CardID, isCombat bool) bool {
	if !isDamageDoneTrigger(t) {
		return false
	}
	if hasAnyParam(t, "DamageAmount", "ValidCause", "TargetRelativeToCause", "TargetRelativeToSource") {
		return false
	}
	if validSource, ok := t.Param("ValidSource"); ok && !Matches(g, g.Card(source), valid.Parse(validSource), h.Controller, host) {
		return false
	}
	if combatDamage, ok := t.Param("CombatDamage"); ok {
		if strings.EqualFold(combatDamage, "True") != isCombat {
			return false
		}
	}
	return true
}

// isDamageDoneTrigger reports whether t is CR 603's "deals damage" shape:
// Mode$ DamageDone.
func isDamageDoneTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "DamageDone")
}

// checkDiscardedTriggers is CR 603's own "whenever ~ is discarded" mode,
// Mode$ Discarded, ported from TriggerDiscarded.performTest -- but unlike
// every other mode this port checks, a "Card.Self" shaped Discarded trigger
// (14 of 105 real lines, the Madness-adjacent "when this card is discarded,
// you may cast it" shape) lives on a card that is never on the battlefield
// at the moment it fires: it is discarded FROM HAND. Java's own
// TriggerReplacementBase.zonesCheck is unrestricted by default
// (validHostZones == null passes regardless of the host's current zone) --
// a Discarded line with no explicit TriggerZones$ (the corpus norm) is
// checked wherever its host card currently sits, not only on the
// battlefield the way an ETB/Dies/Attacks/Blocks/DamageDone/SpellCast
// trigger's own CardFactoryUtil-synthesized TriggerZones$ Battlefield/Stack
// always is. So this needs its own explicit "own" half, checked against the
// discarded card directly (card's own Move-preserved Controller as source,
// checkDiesTriggers' own precedent for reading a card no longer on the
// battlefield), on top of checkOtherDiscardedTriggers' battlefield walk for
// a watcher.
func (g *Game) checkDiscardedTriggers(card CardID, player PlayerID) {
	c := g.Card(card)
	if c.Def != nil {
		for _, face := range c.Def.Faces {
			for _, t := range face.Triggers {
				if !discardedTriggerMatches(g, t, c, c.Controller, card, player) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(t); ok {
					g.PushAbility(Ability{API: api, Source: card, Controller: c.Controller, Params: sub})
				}
			}
		}
	}
	g.checkOtherDiscardedTriggers(card, player)
}

// checkOtherDiscardedTriggers is checkDiscardedTriggers' wider half: every
// permanent on the battlefield gets its own Triggers walked against the
// discarded card and the player who discarded it ("Whenever you discard a
// card, ..."), checkOtherETBTriggers'/checkOtherDiesTriggers' own shape.
func (g *Game) checkOtherDiscardedTriggers(card CardID, player PlayerID) {
	c := g.Card(card)
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !discardedTriggerMatches(g, t, c, h.Controller, host, player) {
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

// discardedTriggerMatches is checkDiscardedTriggers'/checkOtherDiscardedTriggers'
// own shared check: ValidCard against the discarded card, matched with
// sourceController/source the caller's own pairing (the discarded card
// itself for the "own" half, the watching host for the "other" half) --
// Matches's own "source is the card the spec is written on" contract
// (valid.go). ValidPlayer -- a Player, not a Card, matchesPlayerBase's own
// job -- matches player, who discarded it.
//
// Not resolved: ValidCause$ (11 of 105 real lines) -- a SpellAbility, not a
// Card, Matches cannot evaluate one. A trigger carrying it is skipped
// entirely, not fired unconditionally (GO-7). 94 of 105 real lines carry
// none of it.
func discardedTriggerMatches(g *Game, t *compile.Ability, discarded *Card, sourceController PlayerID, source CardID, player PlayerID) bool {
	if !isDiscardedTrigger(t) {
		return false
	}
	if hasAnyParam(t, "ValidCause") {
		return false
	}
	if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, discarded, valid.Parse(validCard), sourceController, source) {
		return false
	}
	if validPlayer, ok := t.Param("ValidPlayer"); ok {
		matched, recognized := matchesPlayerBase(player, sourceController, validPlayer)
		if !recognized || !matched {
			return false
		}
	}
	return true
}

// isDiscardedTrigger reports whether t is CR 603's "is discarded" shape:
// Mode$ Discarded.
func isDiscardedTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Discarded")
}

// checkTapsTriggers is CR 603's own "whenever ~ becomes tapped" mode, Mode$
// Taps, ported from TriggerTaps.performTest -- the identical single-walk
// shape checkAttacksTriggers/checkBlocksTriggers/checkDamageDoneTriggersToCard
// already established, since TriggerTaps never special-cases the tapped
// card's own trigger either. player is the tapped card's own controller at
// both real call sites this port has (DeclareCombatAttackers, attack.go;
// TapLandForMana, manaability.go) -- neither models anyone else's action
// tapping a permanent yet, so ValidPlayer's own dominant real value ("You,"
// 4 of 4 real lines that carry it) is exactly this.
//
// Not resolved: FirstTime$ (1 of 177 real lines) -- "the first time a
// permanent taps this turn," per-card-per-turn state this port tracks
// nothing for; Teamwork$ (1) -- CostTeamwork, a cost-type this port has no
// concept of; ValidCause$ (0) -- a SpellAbility, not a Card, Matches
// (valid.go) cannot evaluate one. A trigger carrying any of these three is
// skipped entirely, not fired unconditionally (GO-7). Attacker$ (2) IS
// resolved: isAttacker reports whether this tap was caused by attacking or
// something else (a mana ability, the only other real tap site) -- the same
// boolean TriggerTaps.performTest itself compares against
// AbilityKey.Attacker. 173 of 177 real lines carry none of the three
// skipped params.
func (g *Game) checkTapsTriggers(card CardID, player PlayerID, isAttacker bool) {
	c := g.Card(card)
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !isTapsTrigger(t) {
						continue
					}
					if hasAnyParam(t, "FirstTime", "Teamwork", "ValidCause") {
						continue
					}
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller, host) {
						continue
					}
					if validPlayer, ok := t.Param("ValidPlayer"); ok {
						matched, recognized := matchesPlayerBase(player, h.Controller, validPlayer)
						if !recognized || !matched {
							continue
						}
					}
					if attacker, ok := t.Param("Attacker"); ok {
						if strings.EqualFold(attacker, "True") != isAttacker {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						g.PushAbility(Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
}

// isTapsTrigger reports whether t is CR 603's "becomes tapped" shape: Mode$
// Taps.
func isTapsTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Taps")
}

// checkTapsForManaTriggers is CR 603's own "whenever ~ taps for mana" mode,
// Mode$ TapsForMana, ported from TriggerTapsForMana.performTest -- narrower
// than checkTapsTriggers (a mana ability specifically, not any tap), so it
// is its own check rather than a param on the general one, matching Java's
// own separate Trigger subclass. Activator -- a Player, not a Card,
// matchesPlayerBase's own job -- is player, the same "the tapped card's own
// controller" simplification checkTapsTriggers already makes, since
// TapLandForMana (manaability.go) is the only real mana-ability call site
// this port has and nothing there models anyone but the land's own
// controller activating it.
//
// Not resolved: Produced$ (3 of 65 real lines) -- "C" (2) can never match
// anyway, since TapLandForMana only ever produces one of the five colors,
// never colorless, and "ChosenColor" (1) needs a runtime value this port
// has no evaluator for; skipped together rather than trying to resolve one
// and not the other. 62 of 65 real lines carry none of it.
func (g *Game) checkTapsForManaTriggers(card CardID, player PlayerID) {
	c := g.Card(card)
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !isTapsForManaTrigger(t) {
						continue
					}
					if hasAnyParam(t, "Produced") {
						continue
					}
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller, host) {
						continue
					}
					if activator, ok := t.Param("Activator"); ok {
						matched, recognized := matchesPlayerBase(player, h.Controller, activator)
						if !recognized || !matched {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						g.PushAbility(Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
}

// isTapsForManaTrigger reports whether t is CR 603's "taps for mana" shape:
// Mode$ TapsForMana.
func isTapsForManaTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "TapsForMana")
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
