// Trigger firing: CR 603, trimmed to the corpus's nine most frequent modes
// -- a permanent entering the battlefield (Mode$ ChangesZone, Destination$
// Battlefield), one leaving it to a graveyard (Mode$ ChangesZone, Origin$
// Battlefield, Destination$ Graveyard, CR 700.4's "dies"), a creature
// attacking (Mode$ Attacks, CR 508.3), blocking (Mode$ Blocks, CR 509.2),
// dealing damage (Mode$ DamageDone), a card being discarded (Mode$
// Discarded), a permanent becoming tapped (Mode$ Taps) or tapping for mana
// (Mode$ TapsForMana), and a player casting a spell (Mode$ SpellCast) --
// plus, for every one of those nine, CR 603.3b's own APNAP ordering when
// more than one triggers off a single event (pushTriggeredAbilities, below).
// game-state.md's own trigger-firing section has every mode's own
// corpus-frequency count and unresolved params.

package engine

import (
	"strconv"
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
// ordering applies now that checkOtherETBTriggers exists -- one entering
// card can push both its own trigger and another permanent's -- and is
// implemented via pushTriggeredAbilities (below): every match this function
// finds, its own and checkOtherETBTriggers', is collected first and pushed
// together in one APNAP pass, rather than each pushed the moment it is
// found.
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
	var matches []Ability
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
					matches = append(matches, Ability{API: api, Source: entered, Controller: c.Controller, Params: sub})
				}
			}
		}
	}
	matches = append(matches, g.otherETBTriggerMatches(entered)...)
	g.pushTriggeredAbilities(matches)
}

// otherETBTriggerMatches is checkETBTriggers's wider half: every permanent
// already on the battlefield, other than entered itself, gets its own
// Triggers walked against entered -- CR 603.2's "look back in time" applied
// from the watcher's side rather than the entering card's own. entered is
// skipped because its own Card.Self-shaped triggers are already handled by
// checkETBTriggers directly; running both loops over every card would fire
// that trigger twice.
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
//
// Returns matches rather than pushing them directly -- checkETBTriggers
// pushes its own plus these together, in one CR 603.3b APNAP pass
// (pushTriggeredAbilities, below).
func (g *Game) otherETBTriggerMatches(entered CardID) []Ability {
	var matches []Ability
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
						matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller, Params: sub})
					}
				}
			}
		}
	}
	return matches
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
	var matches []Ability
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
					matches = append(matches, Ability{API: api, Source: left, Controller: c.Controller, Params: sub})
				}
			}
		}
	}
	matches = append(matches, g.otherDiesTriggerMatches(left)...)
	g.pushTriggeredAbilities(matches)
}

// otherDiesTriggerMatches is checkDiesTriggers's wider half, the identical
// shape otherETBTriggerMatches is for entering: every permanent still on the
// battlefield gets its own Triggers walked against left, the card that just
// died ("Whenever a creature you control dies...", "Whenever another Cleric
// dies..."). No entered == left skip is needed the way otherETBTriggerMatches
// has one: left is already in the graveyard by the time this runs (every
// real call site moves it there first, action.go), so it never appears in
// the Battlefield walk to begin with -- unlike otherETBTriggerMatches, where
// the entered card is already ON the battlefield being walked.
//
// This closes the gap checkDiesTriggers's own doc comment used to name as
// not-yet-done: a watcher's own dies-shaped trigger needs left's state as a
// dying object, not the watcher's own zone -- the watcher itself is
// unaffected by left leaving and is exactly as reachable by a battlefield
// walk as any ETB watcher is, so nothing about "look back in time" actually
// blocks this the way an earlier version of this comment assumed.
//
// Returns matches rather than pushing them directly, checkDiesTriggers's own
// reason (otherETBTriggerMatches's own doc comment).
func (g *Game) otherDiesTriggerMatches(left CardID) []Ability {
	var matches []Ability
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
						matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller, Params: sub})
					}
				}
			}
		}
	}
	return matches
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
// Matches (valid.go) only evaluates a *Card; FirstAttack$ (4) -- a
// creature's own per-turn attack-count history (Java's own
// CardDamageHistory.getCreatureAttacksThisTurn), which this port tracks
// nothing for. A trigger carrying either is skipped entirely, not fired
// unconditionally -- GO-7: better to miss a real trigger than fire one whose
// own restriction this port silently ignored. 1,555 of 1,606 real lines
// carry neither.
//
// Resolved: Alone$ (57) -- Combat.Attackers minus attacker itself is
// TriggerAttacks.performTest's own AbilityKey.OtherAttackers (CombatUtil's
// own checkDeclaredAttacker, "otherAttackers = combat.getAttackers();
// otherAttackers.remove(c)"), so len(g.combat.Attackers)-1 stands in
// directly -- attacksOtherCount, below; DefendingPlayerPoisoned$ (1) --
// defenderOf(attacker) (attack.go) is AbilityKey.DefendingPlayer, and
// Counters.Count(Poison) is Player.getPoisonCounters(); AttackDifferentPlayers$
// (1) -- attacksMultiplePlayers, below, walks g.combat.Attackers the same
// way TriggerAttacks.performTest walks AbilityKey.Defenders, since this port
// has no separate "defenders actually attacked" list of its own to build
// (attackersOf, attack.go, already answers the same "did anyone attack this
// one" question the corresponding Java field precomputes).
func (g *Game) checkAttacksTriggers(attacker CardID) {
	var matches []Ability
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
					if hasAnyParam(t, "Attacked", "FirstAttack") {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(attacker), valid.Parse(validCard), h.Controller, host) {
						continue
					}
					if alone, ok := t.Param("Alone"); ok {
						if strings.EqualFold(alone, "True") != (attacksOtherCount(g, attacker) == 0) {
							continue
						}
					}
					if _, ok := t.Param("DefendingPlayerPoisoned"); ok {
						if g.Player(g.defenderOf(attacker)).Counters.Count(Poison) == 0 {
							continue
						}
					}
					if _, ok := t.Param("AttackDifferentPlayers"); ok {
						if !attacksMultiplePlayers(g, g.combat.AttackTargets[attacker]) {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
}

// attacksOtherCount is how many creatures other than attacker are also
// declared attacking this combat -- CombatUtil.checkDeclaredAttacker's own
// AbilityKey.OtherAttackers, "all of combat's own declared attackers, minus
// this one." Combat.Attackers holds every declared attacker for the whole
// combat, not just the ones sharing attacker's own defender, matching
// TriggerAttacks' own Alone$'s corpus meaning ("attacks alone" means no
// other creature attacks at all this combat, not just no other creature
// attacking the same thing).
func attacksOtherCount(g *Game, attacker CardID) int {
	n := 0
	for _, id := range g.combat.Attackers {
		if id != attacker {
			n++
		}
	}
	return n
}

// attacksMultiplePlayers reports whether attacked is a player and at least
// one other declared attacker this combat is attacking a different player --
// TriggerAttacks.performTest's own AttackDifferentPlayers$ branch, which
// only ever fires true when the entity attacked is itself a Player (a
// planeswalker/Battle attacker never satisfies it, matching Java's own
// "attacked instanceof Player" guard).
func attacksMultiplePlayers(g *Game, attacked EntityID) bool {
	pid, ok := attacked.AsPlayer()
	if !ok {
		return false
	}
	for _, other := range g.combat.Attackers {
		if otherPid, ok := g.combat.AttackTargets[other].AsPlayer(); ok && otherPid != pid {
			return true
		}
	}
	return false
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
// those 1,216 (You/Opponent/Player), plus matchesPlayerSpec's own
// Active/NonActive/Other dotted property, covering Player.Opponent (12),
// Player.NonActive (4), Player.Active (2), Player.Other (1) and
// Opponent.NonActive (1) -- 19 more of the 25 qualified lines. The
// remaining 6 (Player.EnchantedBy, 5; Player.Chosen, 1) stay unresolved
// (matchesPlayerSpec's own doc comment, valid.go): a trigger carrying one
// never fires, rather than firing unconditionally (GO-7).
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
	var matches []Ability
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
					if !matchesActivatingPlayer(g, t, activator, h.Controller) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
}

// matchesActivatingPlayer is ValidActivatingPlayer's own check, ported from
// matchesValidParam("ValidActivatingPlayer", activator) -- matchesPlayerSpec
// (valid.go)'s own bare values plus its Active/NonActive/Other property
// layer, together covering 19 of the 25 real qualified lines
// checkSpellCastTriggers' own doc comment counts (Player.Opponent,
// Player.NonActive, Player.Active, Player.Other, Opponent.NonActive).
// Missing entirely is a pass, the same CardTraitBase.matchesValidParam
// contract ValidCard's own absence gets above. Player.EnchantedBy (5) and
// Player.Chosen (1) stay unrecognized -- matchesPlayerSpec's own doc comment
// has the reason.
func matchesActivatingPlayer(g *Game, t *compile.Ability, activator, hostController PlayerID) bool {
	v, ok := t.Param("ValidActivatingPlayer")
	if !ok {
		return true
	}
	matched, recognized := matchesPlayerSpec(g, activator, hostController, v)
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
// ValidBlocked$ (8 of 127 real lines, every one an "or blocks/becomes
// blocked by one or more X creatures" description) is resolved against
// blk.Attacker directly: TriggerBlocks.performTest itself matches it
// against the full collection of attackers this blocker blocks
// (AbilityKey.Attackers), ANY of which satisfying it fires the trigger
// once -- but this port's own checkBlocksTriggers is already called once
// per declared Block (a per-pair granularity, this doc comment's own next
// paragraph), never once per blocker with every attacker gathered, so
// checking the one attacker each call already has stands in for "any
// member of the collection" correctly for the overwhelming single-attacker
// case and no worse than the existing per-pair granularity for the rare
// double-block one (a blocker legally blocking two attackers at once fires
// once per matching attacker here, where Java fires once total -- an
// existing divergence, not a new one this param introduces).
//
// Called once per declared Block, after CanBlock and menaceLegal have both
// already filtered the pairing down to a legal one (DeclareCombatBlockers,
// block.go) -- a blocker declared against more than one attacker at once (a
// real corpus rarity this port's own combat model does not otherwise
// restrict) fires once per Block entry rather than once with every attacker
// gathered, the same per-pair granularity every other Block-consuming caller
// already uses.
func (g *Game) checkBlocksTriggers(blk Block) {
	var matches []Ability
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
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(blk.Blocker), valid.Parse(validCard), h.Controller, host) {
						continue
					}
					if validBlocked, ok := t.Param("ValidBlocked"); ok &&
						!Matches(g, g.Card(blk.Attacker), valid.Parse(validBlocked), h.Controller, host) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
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
// (valid.go) for the first, matchesPlayerSpec (valid.go, the same one
// matchesActivatingPlayer uses) for the second -- 4 of the 5 real qualified
// ValidTarget$ Player.* lines resolve this way (Player.Opponent x3,
// Player.Other x1); the fifth, Player.EnchantedBy, does not
// (matchesPlayerSpec's own doc comment). damageDoneMatches (below) is
// everything the two calls share -- one walk over the battlefield,
// ValidSource, and CombatDamage$ -- everything but that one different check.
//
// isCombat is always true at both real call sites (dealPermanentDamage/
// dealPlayerDamage, combatdamage.go): nothing outside combat deals damage
// in this port yet (game-state.md's "Not ported yet"), so a
// CombatDamage$ False line (a rare "whenever ~ deals noncombat damage"
// shape) never fires and a CombatDamage$ True line or one carrying neither
// always passes that part of the check.
func (g *Game) checkDamageDoneTriggersToCard(source, target CardID, amount int, isCombat bool) {
	var matches []Ability
	toughness, hasToughness := g.Card(target).Toughness()
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !damageDoneMatches(g, t, source, h, host, isCombat, amount, toughness, hasToughness) {
						continue
					}
					if validTarget, ok := t.Param("ValidTarget"); ok &&
						!Matches(g, g.Card(target), valid.Parse(validTarget), h.Controller, host) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
}

func (g *Game) checkDamageDoneTriggersToPlayer(source CardID, target PlayerID, amount int, isCombat bool) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !damageDoneMatches(g, t, source, h, host, isCombat, amount, 0, false) {
						continue
					}
					if validTarget, ok := t.Param("ValidTarget"); ok {
						matched, recognized := matchesPlayerSpec(g, target, h.Controller, validTarget)
						if !recognized || !matched {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
}

// damageDoneMatches is checkDamageDoneTriggersToCard/ToPlayer's own shared
// half: is t a Mode$ DamageDone trigger this port can evaluate at all
// (isDamageDoneTrigger, no unresolved param), does ValidSource match (absent
// is a pass, matchesValidParam's own contract, the same as ValidCard's own
// absence in checkSpellCastTriggers), and does CombatDamage$ agree with
// isCombat -- everything but ValidTarget, which the two callers each check
// their own way.
//
// Resolved: DamageAmount$ (8 of 1,080 real lines) -- damageAmountMatches,
// below, ports TriggerDamageDone.performTest's own inline parse directly
// (its own hand-rolled substring(0,2)/substring(2) split, not
// AbilityUtils.calculateAmount -- every real line is a plain integer or the
// literal "TargetToughness", never an SVar reference).
//
// Not resolved, skipped via hasAnyParam: ValidCause$ (1) -- a SpellAbility,
// not a Card, Matches cannot evaluate one; TargetRelativeToCause$/
// TargetRelativeToSource$ (0 real lines alongside the shapes above) -- a
// GameEntity-vs-GameEntity relative match this port has no evaluator for. A
// trigger carrying either is skipped entirely, not fired unconditionally
// (GO-7). 1,079 of 1,080 real lines carry neither.
func damageDoneMatches(g *Game, t *compile.Ability, source CardID, h *Card, host CardID, isCombat bool, amount, toughness int, hasToughness bool) bool {
	if !isDamageDoneTrigger(t) {
		return false
	}
	if hasAnyParam(t, "ValidCause", "TargetRelativeToCause", "TargetRelativeToSource") {
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
	if da, ok := t.Param("DamageAmount"); ok && !damageAmountMatches(da, amount, toughness, hasToughness) {
		return false
	}
	return true
}

// damageAmountMatches ports TriggerDamageDone.performTest's own DamageAmount$
// branch: the first two characters are the operator (Expressions.compare's
// own vocabulary -- compareOp, valid.go, already has it), the rest is either
// a base-10 operand or the literal "TargetToughness", the damaged card's own
// net toughness at the moment of damage (Card.getNetToughness, read before
// this port's own lethal-damage SBA can move it, so folded Toughness() still
// answers correctly). hasToughness is false for player damage (there is no
// target card to measure) -- a line naming TargetToughness there is a
// shape that cannot arise for real (Java would itself throw
// ClassCastException casting the player to a Card), so it is skipped rather
// than guessed at (GO-7), not a real corpus case either way.
func damageAmountMatches(param string, amount, toughness int, hasToughness bool) bool {
	if len(param) < 3 {
		return false
	}
	operator := param[:2]
	rest := param[2:]
	operand := 0
	if rest == "TargetToughness" {
		if !hasToughness {
			return false
		}
		operand = toughness
	} else {
		n, err := strconv.Atoi(rest)
		if err != nil {
			return false
		}
		operand = n
	}
	return compareOp(amount, operator, operand)
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
	var matches []Ability
	c := g.Card(card)
	if c.Def != nil {
		for _, face := range c.Def.Faces {
			for _, t := range face.Triggers {
				if !discardedTriggerMatches(g, t, c, c.Controller, card, player) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(t); ok {
					matches = append(matches, Ability{API: api, Source: card, Controller: c.Controller, Params: sub})
				}
			}
		}
	}
	matches = append(matches, g.otherDiscardedTriggerMatches(card, player)...)
	g.pushTriggeredAbilities(matches)
}

// otherDiscardedTriggerMatches is checkDiscardedTriggers' wider half: every
// permanent on the battlefield gets its own Triggers walked against the
// discarded card and the player who discarded it ("Whenever you discard a
// card, ..."), otherETBTriggerMatches'/otherDiesTriggerMatches' own shape,
// including the "return matches, don't push them" contract
// (otherETBTriggerMatches' own doc comment has the reason).
func (g *Game) otherDiscardedTriggerMatches(card CardID, player PlayerID) []Ability {
	var matches []Ability
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
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	return matches
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
	var matches []Ability
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
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
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
// matchesPlayerSpec's own job -- is player, the same "the tapped card's own
// controller" simplification checkTapsTriggers already makes, since
// TapLandForMana (manaability.go) is the only real mana-ability call site
// this port has and nothing there models anyone but the land's own
// controller activating it. The one real qualified value, Activator$
// Player.NonActive, resolves through matchesPlayerSpec's own Active/
// NonActive property.
//
// Not resolved: Produced$ (3 of 65 real lines) -- "C" (2) can never match
// anyway, since TapLandForMana only ever produces one of the five colors,
// never colorless, and "ChosenColor" (1) needs a runtime value this port
// has no evaluator for; skipped together rather than trying to resolve one
// and not the other. 62 of 65 real lines carry none of it.
func (g *Game) checkTapsForManaTriggers(card CardID, player PlayerID) {
	var matches []Ability
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
						matched, recognized := matchesPlayerSpec(g, player, h.Controller, activator)
						if !recognized || !matched {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller, Params: sub})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(matches)
}

// isTapsForManaTrigger reports whether t is CR 603's "taps for mana" shape:
// Mode$ TapsForMana.
func isTapsForManaTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "TapsForMana")
}

// pushTriggeredAbilities is CR 603.3b: when more than one ability triggers
// off a single event, each player -- in APNAP order (playersInAPNAPOrder,
// below) -- puts the abilities they control on the stack, choosing their own
// order among more than one (Player.getController().orderAndPlaySimultaneousSa,
// MagicStack.addAllTriggeredAbilitiesToStack). This port has no
// PlayerController hook for that choice yet (control.go's own "90 of 110
// methods" gap, game-state.md), so a single player's own multiple matches
// stay in the deterministic order the caller found them (GO-12) -- the same
// simplification every other real-choice gap already makes when the
// corresponding PlayerController method does not exist.
//
// MagicStack's own addAllTriggeredAbilitiesToStack pushes the active
// player's own group onto the stack FIRST, then each following player's
// group in turn order, one player at a time -- so, since the stack is LIFO
// (PushAbility's own doc comment, "last on, first off"), the LAST player in
// APNAP order ends up on top, resolving FIRST; the active player's own group
// resolves LAST. Before this, every trigger-check function pushed each match
// the instant it found one, in whatever order Players()/the battlefield walk
// happened to visit -- correct only when a single card's own trigger fires
// alone, wrong the instant checkOtherETBTriggers (or any of its siblings)
// makes more than one permanent trigger off the same event, which is a real
// case now (game-state.md's own account of this gap, "Not ported yet").
//
// Every check function collects its own matches into a []Ability first (its
// own "own" and "other" halves both feed the same slice when it has both)
// and calls this once at the end, instead of pushing inline.
func (g *Game) pushTriggeredAbilities(matches []Ability) {
	for _, pid := range g.playersInAPNAPOrder() {
		for _, a := range matches {
			if a.Controller == pid {
				g.PushAbility(a)
			}
		}
	}
}

// playersInAPNAPOrder is CR 603.3b's own "APNAP order": Game.ActivePlayer()
// first, then turn order after it (nextPlayerAfter, turn.go -- the same
// seating-order/skip-a-lost-player rule turn advancement itself already
// uses) until every seated player has appeared once. Before StartTurn,
// ActivePlayer() is NoPlayer (game.go's own doc comment) -- Players()' own
// seating order stands in, since there is no active player yet to order
// around; no real call site reaches pushTriggeredAbilities before StartTurn
// today, but a helper that panicked on it would be a worse failure mode than
// falling back to a deterministic order that still is one (GO-7).
func (g *Game) playersInAPNAPOrder() []PlayerID {
	active := g.ActivePlayer()
	if active == NoPlayer {
		return g.Players()
	}
	order := []PlayerID{active}
	for p := g.nextPlayerAfter(active); p != active; p = g.nextPlayerAfter(p) {
		order = append(order, p)
	}
	return order
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
