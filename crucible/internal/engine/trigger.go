// Trigger firing: CR 603, trimmed to the corpus's ten most frequent modes --
// a permanent entering the battlefield (Mode$ ChangesZone, Destination$
// Battlefield), one leaving it to a graveyard (Mode$ ChangesZone, Origin$
// Battlefield, Destination$ Graveyard, CR 700.4's "dies"), a creature
// attacking (Mode$ Attacks, CR 508.3), blocking (Mode$ Blocks, CR 509.2),
// dealing damage (Mode$ DamageDone), a card being discarded (Mode$
// Discarded), a permanent becoming tapped (Mode$ Taps) or tapping for mana
// (Mode$ TapsForMana), a player casting a spell (Mode$ SpellCast), and the
// beginning of a step or phase (Mode$ Phase, CR 500) -- plus, for every one
// of those ten, CR 603.3b's own APNAP ordering when more than one triggers
// off a single event (pushTriggeredAbilities, below). game-state.md's own
// trigger-firing section has every mode's own corpus-frequency count and
// unresolved params.

package engine

import (
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/expr"
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
func (g *Game) checkETBTriggers(controller PlayerController, entered CardID, origin ZoneType) {
	var matches []Ability
	c := g.Card(entered)
	if c.Def != nil {
		for _, face := range c.Def.Faces {
			for _, t := range face.Triggers {
				if !isETBTrigger(t, origin) {
					continue
				}
				validCard, ok := t.Param("ValidCard")
				if !ok {
					continue
				}
				if !Matches(g, c, valid.Parse(validCard), c.Controller(), entered) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(g, c, face.Amounts, t); ok {
					matches = append(matches, Ability{API: api, Source: entered, Controller: c.Controller(), Params: sub, Amounts: face.Amounts})
				}
			}
		}
	}
	matches = append(matches, g.otherETBTriggerMatches(entered, origin)...)
	g.pushTriggeredAbilities(controller, matches)
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
func (g *Game) otherETBTriggerMatches(entered CardID, origin ZoneType) []Ability {
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
					if !isETBTrigger(t, origin) {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(entered), valid.Parse(validCard), w.Controller(), watcher) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, w, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller(), Params: sub, Amounts: face.Amounts})
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
// sits in, and c.Controller() (game.go's own Move does not clear it on
// leaving) still reads the last real controller -- neither needs a lookup.
// A ValidCard$ testing power, toughness, type, color, a keyword or a counter
// does: Move's own battlefield-leaving branch clears exactly those before
// this function ever runs, so reading g.Card(left) directly would test the
// dying card's printed-only state, not the state it died in (Reyhan, Last of
// the Abzan's own "whenever a creature you control with a +1/+1 counter on
// it dies" among 116 real corpus lines this would silently under-fire for).
// g.LKI (game.go) is exactly the frozen copy CR 603.6d asks for, taken the
// instant before Move reset any of it.
func (g *Game) checkDiesTriggers(controller PlayerController, left CardID) {
	var matches []Ability
	c := g.Card(left)
	if snap := g.LKI(left); snap != nil {
		c = snap
	}
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
				if !Matches(g, c, valid.Parse(validCard), c.Controller(), left) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(g, c, face.Amounts, t); ok {
					matches = append(matches, Ability{API: api, Source: left, Controller: c.Controller(), Params: sub, Amounts: face.Amounts})
				}
			}
		}
	}
	matches = append(matches, g.otherDiesTriggerMatches(left)...)
	g.pushTriggeredAbilities(controller, matches)
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
// blocks this the way an earlier version of this comment assumed. left's own
// state is the one side that does need the lookback -- g.LKI, exactly as
// checkDiesTriggers's own doc comment above now explains.
//
// Returns matches rather than pushing them directly, checkDiesTriggers's own
// reason (otherETBTriggerMatches's own doc comment).
func (g *Game) otherDiesTriggerMatches(left CardID) []Ability {
	var matches []Ability
	dying := g.Card(left)
	if snap := g.LKI(left); snap != nil {
		dying = snap
	}
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
					if !Matches(g, dying, valid.Parse(validCard), w.Controller(), watcher) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, w, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller(), Params: sub, Amounts: face.Amounts})
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
// Attacked$ (47 real lines) and FirstAttack$ (4) are resolved now too.
// Attacked$ matches AbilityKey.Attacked, a single GameEntity (a player,
// planeswalker or Battle) rather than a *Card -- attackedTargetMatches
// (below, built for AttackersDeclared's own AttackedTarget$, which faces the
// identical player-shaped/card-shaped mixed-token problem for a whole
// collection of attacked entities) already handles one entity as the
// trivial one-element case, no new dispatch needed. FirstAttack$ reads a new
// Card.AttacksThisTurn (card.go), CardDamageHistory.getCreatureAttacksThisTurn's
// own per-card counter, incremented for each declared attacker right before
// this function runs (DeclareCombatAttackers, attack.go) and reset every
// cleanup (cleanupStep, turn.go) -- true exactly when the just-incremented
// count is 1, "this is the first time this creature has attacked this
// turn," ported directly from Java's own `> 1` skip rather than a `== 1`
// require, since a trigger carrying neither param is unaffected either way.
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
func (g *Game) checkAttacksTriggers(controller PlayerController, attacker CardID) {
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
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if !Matches(g, g.Card(attacker), valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if attacked, ok := t.Param("Attacked"); ok {
						if !attackedTargetMatches(g, h, []EntityID{g.combat.AttackTargets[attacker]}, attacked) {
							continue
						}
					}
					if _, ok := t.Param("FirstAttack"); ok {
						if g.Card(attacker).AttacksThisTurn > 1 {
							continue
						}
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
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
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
func (g *Game) checkSpellCastTriggers(controller PlayerController, cast CardID, activator PlayerID) {
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
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if !matchesActivatingPlayer(g, t, activator, h.Controller()) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
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
func (g *Game) checkBlocksTriggers(controller PlayerController, blk Block) {
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
					if !Matches(g, g.Card(blk.Blocker), valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if validBlocked, ok := t.Param("ValidBlocked"); ok &&
						!Matches(g, g.Card(blk.Attacker), valid.Parse(validBlocked), h.Controller(), host) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// isBlocksTrigger reports whether t is CR 509.2's "blocks" shape: Mode$
// Blocks.
func isBlocksTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Blocks")
}

// checkAttackerBlockedTriggers is CR 509.2's own "becomes blocked" shape --
// TriggerAttackerBlocked.performTest, ported from PlayerController's own
// declareBlockers, fired once per attacker that ended up with at least one
// legal blocker (not once per blocker the way checkBlocksTriggers/
// checkAttackerBlockedByCreatureTriggers, below, both are), with the whole
// blocker group gathered before this runs -- ValidBlocker$/
// ValidBlockerAmount$ (74 of 127 real lines carry neither, an unqualified
// "becomes blocked") counts how many of them a spec matches, the identical
// GE1-defaulted compare-against-an-amount shape validAttackersCountMatches
// (below) already has for AttackersDeclared's own ValidAttackers$/
// ValidAttackersAmount$, generalized to any []CardID rather than
// g.combat.Attackers specifically (validCardsCountMatches, below) since the
// group here is one attacker's own blockers, not the whole combat's
// attackers.
//
// Called from DeclareCombatBlockers (block.go) once per distinct attacker,
// after every Block for it has been filtered by CanBlock/menaceLegal and
// checkBlocksTriggers/checkAttackerBlockedByCreatureTriggers have both
// already run for each -- CR 509.2 groups every "blocks"/"becomes blocked"
// trigger as one simultaneous event, but this port fires each shape as its
// own separate APNAP pass the same way checkAttacksTriggers and
// checkAttackersDeclaredTrigger already are for the declare-attackers step,
// rather than collecting every shape at once. No own/other split is
// needed: TriggerAttackerBlocked itself never special-cases the attacker's
// own trigger, the identical reason checkBlocksTriggers needs none.
//
// Not resolved: ValidCard$ LessPowerThanBlocker (1 real line) -- a hardcoded
// power comparison against the blocker group rather than a valid-string,
// the same shape Skulk's own hardcoded X already is (skulkBlocks,
// staticability.go) but for a different pairing; explicitly refused rather
// than left to a bare-word valid-string parse that would silently match no
// card and never fire, a wrong reason not the right one to never fire for.
func (g *Game) checkAttackerBlockedTriggers(controller PlayerController, attacker CardID, blockers []CardID) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !strings.EqualFold(t.Name, "AttackerBlocked") {
						continue
					}
					validCard, ok := t.Param("ValidCard")
					if !ok {
						continue
					}
					if strings.EqualFold(validCard, "LessPowerThanBlocker") {
						continue
					}
					if !Matches(g, g.Card(attacker), valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if validBlocker, ok := t.Param("ValidBlocker"); ok {
						amount, ok := t.Param("ValidBlockerAmount")
						if !ok {
							amount = "GE1"
						}
						if !validCardsCountMatches(g, h, blockers, validBlocker, amount) {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// checkAttackerBlockedByCreatureTriggers is CR 509.2's own per-pair
// "becomes blocked by a creature" shape -- TriggerAttackerBlockedByCreature.
// performTest, checkBlocksTriggers' own exact mirror image: ValidCard$
// matched against blk.Attacker (blocks' own ValidBlocked$), ValidBlocker$
// matched against blk.Blocker (blocks' own ValidCard$), both single-card
// matches rather than a counted group -- this mode has no ValidBlockerAmount$
// of its own, one blocker at a time being the whole point of "by a
// creature." Called once per declared Block, the identical per-pair
// granularity checkBlocksTriggers already has, and for the identical
// reason: DeclareCombatBlockers (block.go) has no wider grouping at the
// point either already runs.
//
// Not resolved: ValidCard$/ValidBlocker$ LessPowerThanBlocker/
// LessPowerThanAttacker (1 real line each) -- checkAttackerBlockedTriggers'
// own doc comment has the identical reason this refuses rather than lets a
// bare-word valid-string parse silently never match.
func (g *Game) checkAttackerBlockedByCreatureTriggers(controller PlayerController, blk Block) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, host := range g.Zone(Battlefield, pid).Cards() {
			h := g.Card(host)
			if h.Def == nil {
				continue
			}
			for _, face := range h.Def.Faces {
				for _, t := range face.Triggers {
					if !strings.EqualFold(t.Name, "AttackerBlockedByCreature") {
						continue
					}
					if validCard, ok := t.Param("ValidCard"); ok {
						if strings.EqualFold(validCard, "LessPowerThanBlocker") {
							continue
						}
						if !Matches(g, g.Card(blk.Attacker), valid.Parse(validCard), h.Controller(), host) {
							continue
						}
					}
					if validBlocker, ok := t.Param("ValidBlocker"); ok {
						if strings.EqualFold(validBlocker, "LessPowerThanAttacker") {
							continue
						}
						if !Matches(g, g.Card(blk.Blocker), valid.Parse(validBlocker), h.Controller(), host) {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// validCardsCountMatches is validAttackersCountMatches' own generalization:
// how many of cards spec matches, compared against amount (a "GE1"-shaped
// operator+operand, Expressions.compare's own vocabulary via compareOp) --
// factored out once checkAttackerBlockedTriggers needed the identical count
// over one attacker's own blocker group rather than g.combat.Attackers.
func validCardsCountMatches(g *Game, host *Card, cards []CardID, spec, amount string) bool {
	parsed := valid.Parse(spec)
	n := 0
	for _, id := range cards {
		if Matches(g, g.Card(id), parsed, host.Controller(), host.ID) {
			n++
		}
	}
	if len(amount) < 3 {
		return false
	}
	operand, err := strconv.Atoi(amount[2:])
	if err != nil {
		return false
	}
	return compareOp(n, amount[:2], operand)
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
func (g *Game) checkDamageDoneTriggersToCard(controller PlayerController, source, target CardID, amount int, isCombat bool) {
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
						!Matches(g, g.Card(target), valid.Parse(validTarget), h.Controller(), host) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

func (g *Game) checkDamageDoneTriggersToPlayer(controller PlayerController, source CardID, target PlayerID, amount int, isCombat bool) {
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
						matched, recognized := matchesPlayerSpec(g, target, h.Controller(), validTarget)
						if !recognized || !matched {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
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
	if validSource, ok := t.Param("ValidSource"); ok && !Matches(g, g.Card(source), valid.Parse(validSource), h.Controller(), host) {
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
func (g *Game) checkDiscardedTriggers(controller PlayerController, card CardID, player PlayerID) {
	var matches []Ability
	c := g.Card(card)
	if c.Def != nil {
		for _, face := range c.Def.Faces {
			for _, t := range face.Triggers {
				if !discardedTriggerMatches(g, t, c, c.Controller(), card, player) {
					continue
				}
				if sub, api, ok := triggerEffectAPI(g, c, face.Amounts, t); ok {
					matches = append(matches, Ability{API: api, Source: card, Controller: c.Controller(), Params: sub, Amounts: face.Amounts})
				}
			}
		}
	}
	matches = append(matches, g.otherDiscardedTriggerMatches(card, player)...)
	g.pushTriggeredAbilities(controller, matches)
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
					if !discardedTriggerMatches(g, t, c, h.Controller(), host, player) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
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
func (g *Game) checkTapsTriggers(controller PlayerController, card CardID, player PlayerID, isAttacker bool) {
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
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if validPlayer, ok := t.Param("ValidPlayer"); ok {
						matched, recognized := matchesPlayerBase(player, h.Controller(), validPlayer)
						if !recognized || !matched {
							continue
						}
					}
					if attacker, ok := t.Param("Attacker"); ok {
						if strings.EqualFold(attacker, "True") != isAttacker {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
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
func (g *Game) checkTapsForManaTriggers(controller PlayerController, card CardID, player PlayerID) {
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
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if activator, ok := t.Param("Activator"); ok {
						matched, recognized := matchesPlayerSpec(g, player, h.Controller(), activator)
						if !recognized || !matched {
							continue
						}
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// checkUntapsTriggers is CR 502.3/603's own "whenever ~ becomes untapped"
// mode, Mode$ Untaps, ported from TriggerUntaps.performTest -- Taps's own
// mirror image at the opposite end of the identical event (a card's own
// Tapped field flipping), and structurally its exact twin: TriggerUntaps
// never special-cases its own host's trigger either, so one battlefield walk
// covers both "whenever CARDNAME becomes untapped" (Inspired, the corpus's
// own dominant real shape) and "whenever a permanent becomes untapped"
// (mesmeric_orb.txt's own real Card-bare form) alike, the identical
// checkTapsTriggers' own reasoning for its own mirror event.
//
// Called from untapStep (turn.go) once per card that actually untaps this
// step -- Card.untap()'s own early "if (!tapped) return false" before the
// trigger even fires, ported as untapStep's own wasTapped check rather than
// duplicated here: a card already untapped generates no event to check
// triggers against at all, the identical "nothing happened" skip every real
// zone-change/tap call site already gives a no-op.
//
// 30 real T: Mode$ Untaps lines corpus-wide (vocabscan). Not resolved:
// OptionalDecider$ (3) -- an interactive "may" confirm this port's own
// PlayerController has no hook for, the identical gap Discard's own
// Optional$/BecomesTarget's own OptionalDecider$ already document.
func (g *Game) checkUntapsTriggers(controller PlayerController, card CardID) {
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
					if !isUntapsTrigger(t) {
						continue
					}
					if hasAnyParam(t, "OptionalDecider") {
						continue
					}
					if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, c, valid.Parse(validCard), h.Controller(), host) {
						continue
					}
					if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
						matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// isUntapsTrigger reports whether t is CR 603's "becomes untapped" shape:
// Mode$ Untaps.
func isUntapsTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Untaps")
}

// isTapsForManaTrigger reports whether t is CR 603's "taps for mana" shape:
// Mode$ TapsForMana.
func isTapsForManaTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "TapsForMana")
}

// checkPhaseTriggers is CR 500's own "at the beginning of a step or phase"
// mode, Mode$ Phase, ported from TriggerPhase.performTest plus the base
// Trigger class's own phasesCheck (Trigger.java) -- the corpus's SECOND most
// frequent trigger mode after ChangesZone (2,362 real lines, ahead of
// Attacks/SpellCast/DamageDone), left unbuilt until now even though
// phase.go's own PhaseByName/PhaseType.String were built with this in mind
// from the start (their own doc comments: "the name a script writes").
//
// Unlike every other mode this port checks, TriggerPhase.performTest itself
// has no ValidCard at all -- there is no "object" a phase change happens
// TO, only a step or phase happening, so this is a single condition-only
// walk: is the trigger's own host in one of the zones TriggerZones$ names
// (phaseTriggerZoneMatches, below), is the CURRENT phase one Phase$ names
// (phaseTriggerMatches, below), and does ValidPlayer$ match the active
// player (matchesPlayerSpec, valid.go -- the same one checkSpellCastTriggers'
// own matchesActivatingPlayer and checkDamageDoneTriggersToPlayer/
// checkTapsForManaTriggers already use, since ValidPlayer here is checked
// against AbilityKey.Player, which PhaseHandler.onPhaseBegin sets to
// getPlayerTurn() -- the active player, not the trigger's own host
// controller). 2,001 of 2,065 real ValidPlayer$ lines resolve this way (You,
// 1,832; Player, 113; Opponent, 47; the qualified Player.Opponent, 7, and
// Player.Other, 2, through matchesPlayerSpec's own dotted-property layer);
// missing entirely is a pass, the same absent-is-a-pass contract every
// other mode's optional param already has.
//
// IsPresent$/PresentCompare$/PresentZone$/PresentPlayer$ (272, 104 real
// Mode$ Phase lines) and CheckSVar$/SVarCompare$ (310) are resolved now too
// -- triggerCommonRequirementsMet (below) already evaluates both generically
// for every trigger mode's own Execute$ gate, through triggerEffectAPI, the
// identical choke point this function already calls; the hasAnyParam
// pre-filter used to name both here anyway, a leftover from before that
// general mechanism existed that silently kept them skipped even after it
// landed -- 686 real lines combined never fired regardless of whether their
// own condition actually held, not a hypothetical gap.
//
// FirstUpkeep$/FirstUpkeepThisGame$/FirstCombat$/TurnCount$ resolve now too
// (or skip correctly rather than firing unconditionally) through
// triggerPhasesCheck (below, Trigger.phasesCheck's own port) -- reached
// through triggerEffectAPI the identical way IsPresent$/CheckSVar$ already
// are, so the pre-filter naming all four here is gone the same way the
// pre-filter naming IsPresent$/CheckSVar$ already was (this function's own
// doc comment, above): a leftover from before that general mechanism
// existed would otherwise keep them silently skipped even after it landed.
//
// Not resolved, skipped via hasAnyParam: Condition$ (65, an arbitrary
// SVar-shaped boolean condition distinct from CheckSVar$/SVarCompare$'s own
// resolved shape -- SpellAbilityCondition's own separate switch, not
// CardTraitBase's), and APlayerHasMoreLifeThanEachOther$/
// APlayerHasMostCardsInHand$ (a whole-table comparison no other trigger mode
// needs) -- 3 real lines or fewer past Condition$'s own 65. A trigger
// carrying either of these is skipped entirely, not fired unconditionally
// (GO-7). The qualified
// ValidPlayer$ forms matchesPlayerSpec cannot resolve
// (Player.EnchantedController, 34; Player.EnchantedBy, 14; You.descended,
// 10; Player.Chosen, 3; Opponent.EnchantedBy, 2; Player.isMonarch, 1) stay
// unresolved for the identical reason SpellCast's own
// Player.EnchantedBy/Player.Chosen do (matchesPlayerSpec's own doc comment).
func (g *Game) checkPhaseTriggers(controller PlayerController) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, z := range phaseTriggerZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, t := range face.Triggers {
						if !isPhaseTrigger(t) {
							continue
						}
						if hasAnyParam(t, "Condition",
							"APlayerHasMoreLifeThanEachOther", "APlayerHasMostCardsInHand") {
							continue
						}
						if !phaseTriggerZoneMatches(t, z) {
							continue
						}
						if !phaseTriggerMatches(t, g.activePhase) {
							continue
						}
						if validPlayer, ok := t.Param("ValidPlayer"); ok {
							matched, recognized := matchesPlayerSpec(g, g.activePlayer, h.Controller(), validPlayer)
							if !recognized || !matched {
								continue
							}
						}
						if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
							matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
						}
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// phaseTriggerZones is every zone this port walks looking for a Mode$ Phase
// trigger's host -- the four the real corpus's own TriggerZones$ actually
// names (Battlefield, 2,219 of 2,336 real lines that carry one; Command, 84;
// Graveyard, 28; Exile, 3). Hand/Library/Stack/Sideboard/every other
// ZoneType carry zero real Mode$ Phase lines, so walking them would find
// nothing a real card needs; a future card that puts one there is a
// coverage gap this port would need to widen this list for, not a wrong
// answer today.
var phaseTriggerZones = []ZoneType{Battlefield, Command, Graveyard, Exile}

// phaseTriggerZoneMatches is TriggerReplacementBase.zonesCheck's own
// contract applied to Mode$ Phase: TriggerZones$ absent or empty passes
// regardless of zone (26 of 2,362 real lines carry none), a comma-list
// (`Command,Battlefield`, 1 real line) matches if zone is any one of them.
func phaseTriggerZoneMatches(t *compile.Ability, zone ZoneType) bool {
	v, ok := t.Param("TriggerZones")
	if !ok {
		return true
	}
	for _, name := range strings.Split(v, ",") {
		if z, ok := ZoneByName(name); ok && z == zone {
			return true
		}
	}
	return false
}

// phaseTriggerMatches is Phase$ itself: does the trigger fire during
// current, the phase that was just entered. `Main` (29 real lines, every one
// paired with `PhaseCount$ 2` -- "your second main phase," Survival's own
// cards among them) is the one token PhaseByName deliberately does not
// resolve (TestPhaseNamesRoundTrip's own assertion, event_test.go) --
// PhaseType.parseRange's own special case for it (PhaseType.java) expands to
// both Main1 and Main2 when no PhaseCount$ narrows it to the second one
// alone. A PhaseCount$ value other than "2" (0 real lines) has no known
// meaning here and is refused rather than guessed at (GO-7). Every other
// token resolves through phaseNameFold, below -- a single comma-list entry
// (`Main1,Main2`, 1 real line) or, far more often, one bare name.
func phaseTriggerMatches(t *compile.Ability, current PhaseType) bool {
	spec, ok := t.Param("Phase")
	if !ok {
		return false
	}
	for _, token := range strings.Split(spec, ",") {
		if strings.EqualFold(token, "Main") {
			if count, hasCount := t.Param("PhaseCount"); hasCount {
				if count != "2" {
					return false
				}
				if current == Main2 {
					return true
				}
				continue
			}
			if current == Main1 || current == Main2 {
				return true
			}
			continue
		}
		if p, ok := phaseNameFold(token); ok && p == current {
			return true
		}
	}
	return false
}

// phaseNameFold is PhaseByName's own case-insensitive twin, needed only
// here: the corpus itself is inconsistent about one phase's own case
// (`Phase$ End of Turn`, 677 real lines; `Phase$ End Of Turn`, 3 more, a
// capital `O`) the way ZoneByName's own doc comment says zone names never
// are, so an exact match would silently drop those 3 real lines rather than
// fire their trigger. PhaseByName itself stays exact -- fixture.go's own
// GameState text format is this port's own, not the corpus's, and has no
// such inconsistency to tolerate.
func phaseNameFold(name string) (PhaseType, bool) {
	for i, n := range phaseNames {
		if strings.EqualFold(n, name) {
			return PhaseType(i), true
		}
	}
	return Untap, false
}

// isPhaseTrigger reports whether t is CR 500's "beginning of a step or
// phase" shape: Mode$ Phase.
func isPhaseTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Phase")
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
//
// checkBecomesTargetTriggers (below) runs right after each PushAbility,
// matching MagicStack.add's own order for the identical event (cast/copy
// trigger checks, then "Run BecomesTarget triggers") -- a triggered
// ability's own chosen targets are exactly as real a "becomes the target of
// a spell or ability" event as a spell's are, CR 115 draws no distinction.
func (g *Game) pushTriggeredAbilities(controller PlayerController, matches []Ability) {
	for _, pid := range g.playersInAPNAPOrder() {
		for i := range matches {
			if matches[i].Controller != pid {
				continue
			}
			if !g.resolveTargets(controller, &matches[i]) {
				continue
			}
			g.PushAbility(matches[i])
			g.checkBecomesTargetTriggers(controller, matches[i].Targets)
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
// shape: Mode$ ChangesZone with Destination$ naming Battlefield (or
// unrestricted) and Origin$ permitting origin (TriggerChangesZone.java's own
// performTest, ported exactly: Origin$ absent or "Any" is not itself a
// restriction, present-and-specific narrows to that zone list -- 5,846 of
// 5,858 real ETB-shaped lines carry Origin$ Any or no Origin$ at all,
// matching regardless of origin the way this predicate's own zero-arg
// history already assumed; the other 21 name Origin$ Graveyard/Hand/Stack/
// Exile/AttractionDeck, restrictions this predicate silently ignored until
// origin became a real parameter here -- a reanimation-flavored "enters from
// a graveyard" trigger firing on an ordinary cast from hand was a real
// over-firing bug this port had, not a hypothetical one).
func isETBTrigger(t *compile.Ability, origin ZoneType) bool {
	return strings.EqualFold(t.Name, "ChangesZone") &&
		hasZoneOrAny(t, "Destination", Battlefield) &&
		hasZoneOrAny(t, "Origin", origin) &&
		changesZoneResolvable(t)
}

// isDiesTrigger reports whether t is CR 603.6d's "leaves the battlefield"
// shape restricted to the one destination this port's own checkDiesTriggers
// call sites ever reach, a graveyard (CR 700.4's "dies") --
// TriggerChangesZone.java's own performTest again: Origin$ must permit
// Battlefield (absent/"Any"/a list containing it) and Destination$ must
// permit Graveyard the identical way. A bare Destination$ Graveyard with no
// Origin$ restriction at all (31 real lines, "put into a graveyard from
// anywhere") now matches too, since an unrestricted origin already covers
// the battlefield-origin instance this predicate is only ever asked about;
// Destination$ Any or no Destination$ at all (253+11 real lines, CR
// 603.6c's own unqualified "leaves the battlefield") now matches for the
// identical reason on the other side -- both previously required an exact
// literal match this predicate never had a wildcard for, so these 295 real
// lines never fired even on an ordinary death.
func isDiesTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "ChangesZone") &&
		hasZoneOrAny(t, "Origin", Battlefield) &&
		hasZoneOrAny(t, "Destination", Graveyard) &&
		changesZoneResolvable(t)
}

// hasZone reports whether t's param key names zone among its comma-separated
// list of zones -- Destination$ Battlefield,Command among them, a real shape
// the corpus writes (a trigger that fires entering either zone). An absent
// param never matches: hasZoneOrAny, below, is the wildcard-aware sibling
// isETBTrigger/isDiesTrigger actually need.
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

// hasZoneOrAny reports whether t's key param permits zone:
// TriggerChangesZone.performTest's own two ways of saying "no restriction"
// -- key absent entirely, or present naming the literal value "Any" (not one
// token among several; Java's own check is a full-string
// getParam(key).equals("Any")) -- match unconditionally; otherwise zone's own
// name must appear in the param's comma-separated zone list (hasZone,
// above).
func hasZoneOrAny(t *compile.Ability, key string, zone ZoneType) bool {
	v, ok := t.Param(key)
	if !ok || v == "Any" {
		return true
	}
	return hasZone(t, key, zone.String())
}

// changesZoneUnresolvedParams names Mode$ ChangesZone's own performTest
// params past Origin$/Destination$/ValidCard$ this port does not evaluate.
// Each is vanishingly rare in the real corpus (12 of 7,609 real
// T:Mode$ ChangesZone lines combined) but a line naming one skips rather
// than firing unconditionally and guessing wrong (PORT-8/GO-7):
// ValidCause$/NotThisAbility$/ConditionYouCastThisTurn$ (1 each --
// AbilityKey.Cause tracking, a "did I cause my own trigger" self-reference,
// and a per-turn cast-count condition, none of which this port tracks
// anywhere a trigger could read); CheckOnTriggeredCard$ (6 -- a second
// AbilityUtils.calculateAmount comparison against the moved card itself,
// needing a reference vocabulary this predicate does not have);
// ExcludedOrigins$/ExcludedDestinations$ (2, 1 -- 0 real lines combine one
// with an Origin$/Destination$ this predicate already resolves, so skipping
// the line outright costs nothing today). Fizzle$ carries 0 real lines,
// dormant.
var changesZoneUnresolvedParams = [...]string{
	"ValidCause", "NotThisAbility", "ConditionYouCastThisTurn",
	"CheckOnTriggeredCard", "ExcludedOrigins", "ExcludedDestinations", "Fizzle",
}

func changesZoneResolvable(t *compile.Ability) bool {
	for _, key := range changesZoneUnresolvedParams {
		if _, ok := t.Param(key); ok {
			return false
		}
	}
	return true
}

// triggerCommonRequirementsMet is CardTraitBase.meetsCommonRequirements'
// own general gate, checked before ANY trigger mode's own performTest runs
// in Java -- every check*Triggers function in this file calls it, through
// triggerEffectAPI below, the same way every one of them already shares
// that one choke point for turning a match into a pushed Ability.
//
// Resolved (1,148 of the corpus's own ~1,271 real T: lines carrying at
// least one of these params, tallied directly against T: lines specifically
// since S:/A: lines carry some of the identical param names for a different
// switch -- StaticAbility.checkConditions' own Condition$, SpellAbilityCondition's
// own Condition$/ConditionPresent$, neither this function's concern):
//   - IsPresent$/PresentCompare$/PresentZone$/PresentPlayer$ and the
//     identical IsPresent2$/PresentCompare2$/PresentZone2$/PresentPlayer2$
//     pair (624) -- isPresentMatches, below, ports the zone-scan branch of
//     CardTraitBase's own block (a PresentDefined$ line, 40 of 624, skips:
//     no Defined$-to-cards resolver for an arbitrary reference exists in
//     this port yet, drawDefinedPlayers' own narrow You/Opponent form being
//     the only Defined$ evaluator built so far, and it resolves players, not
//     cards).
//   - CheckSVar$/SVarCompare$ (474) -- checkSVarMatches, below, resolving
//     both sides through resolveAmount the identical way ptParam already
//     does for a continuous effect's own numeric params; a line also naming
//     CheckSecondSVar$ (0 real T: lines today) skips, since Java ORs a
//     second check against the first and this file has no need to guess at
//     that shape blind.
//   - Metalcraft$/Delirium$/Threshold$/Hellbent$/FatefulHour$ as a
//     True/False flag (38) -- boolFlagMatches, below, reusing
//     continuousConditionMet's own underlying predicates (item 27's own
//     Condition$ paragraph): the identical player-state question, asked as
//     a flag here rather than as the whole condition.
//   - LifeTotal$ You/ActivePlayer with LifeAmount$ (12) -- lifeTotalMatches,
//     below; OpponentSmallest/OpponentGreatest (0 real T: lines) skip, no
//     multi-opponent life-total query built for triggers.
//
// Not resolved, each skipped whole rather than treated as met (GO-7):
// Revolt$ (25, Game.leftBattlefieldThisTurn tracks nothing for triggers to
// read); WerewolfTransformCondition$/WerewolfUntransformCondition$ (65,
// Innistrad's own day/night mechanic, a "spells cast last turn" list this
// port tracks nowhere); CheckDefinedPlayer$ (20, every real line qualifies
// it with isMonarch/hasInitiative/withMostLife/withMostType -- a mechanic
// this port has no state for, not a shape a general Defined$-to-players
// resolver could close on its own); ManaSpent$/ManaNotSpent$ (8, no
// paying-colors-by-cast tracked); Adamant$ (1); Bloodthirst$/Monarch$/
// EnduringStory$/DayTime$/ClassLevel$ (0 real T: lines each, dormant).
func triggerCommonRequirementsMet(g *Game, host *Card, amounts map[string]expr.Amount, t *compile.Ability) bool {
	for _, key := range [...]string{
		"Revolt", "WerewolfTransformCondition", "WerewolfUntransformCondition",
		"CheckDefinedPlayer", "ManaSpent", "ManaNotSpent", "Adamant",
		"Bloodthirst", "Monarch", "EnduringStory", "DayTime", "ClassLevel",
	} {
		if _, ok := t.Param(key); ok {
			return false
		}
	}
	if !isPresentMatches(g, host, amounts, t, "IsPresent", "PresentCompare", "PresentDefined", "PresentZone", "PresentPlayer") {
		return false
	}
	if !isPresentMatches(g, host, amounts, t, "IsPresent2", "PresentCompare2", "PresentDefined2", "PresentZone2", "PresentPlayer2") {
		return false
	}
	if !checkSVarMatches(g, host, amounts, t, "CheckSVar", "SVarCompare", "CheckSecondSVar") {
		return false
	}
	if !boolFlagMatches(t, "Metalcraft", func() bool { return battlefieldArtifactCount(g, host.Controller()) >= 3 }) {
		return false
	}
	if !boolFlagMatches(t, "Delirium", func() bool { return graveyardCoreTypeCount(g, host.Controller()) >= 4 }) {
		return false
	}
	if !boolFlagMatches(t, "Threshold", func() bool { return len(g.Zone(Graveyard, host.Controller()).Cards()) >= 7 }) {
		return false
	}
	if !boolFlagMatches(t, "Hellbent", func() bool { return len(g.Zone(Hand, host.Controller()).Cards()) == 0 }) {
		return false
	}
	if !boolFlagMatches(t, "FatefulHour", func() bool { return g.Player(host.Controller()).Life <= 5 }) {
		return false
	}
	if !lifeTotalMatches(g, host, amounts, t) {
		return false
	}
	return true
}

// isPresentMatches is CardTraitBase's own IsPresent$/PresentCompare$/
// PresentDefined$/PresentZone$/PresentPlayer$ block (isKey absent from t
// entirely is a pass, the identical "no restriction" contract every other
// optional gate in this port already has). PresentZone$ defaults to
// Battlefield, a comma list otherwise (ZoneByName per entry, an
// unrecognized name skips rather than guesses); PresentPlayer$ is "You"
// (host's own controller only), anything else -- "Any", the corpus's own
// overwhelming default, or absent -- every player, Java's own three
// additive You/Opponent/Allies blocks collapsed to the one partition they
// produce for a single-valued param. PresentDefined$ skips the whole line:
// no Defined$-to-cards resolver exists for an arbitrary reference yet.
func isPresentMatches(g *Game, host *Card, amounts map[string]expr.Amount, t *compile.Ability, isKey, compareKey, definedKey, zoneKey, playerKey string) bool {
	spec, ok := t.Param(isKey)
	if !ok {
		return true
	}
	if _, ok := t.Param(definedKey); ok {
		return false
	}
	var zones []ZoneType
	if zoneList, ok := t.Param(zoneKey); ok {
		for _, name := range strings.Split(zoneList, ",") {
			z, ok := ZoneByName(name)
			if !ok {
				return false
			}
			zones = append(zones, z)
		}
	} else {
		zones = []ZoneType{Battlefield}
	}
	onlyYou := false
	if player, ok := t.Param(playerKey); ok {
		onlyYou = strings.EqualFold(player, "You")
	}
	var candidates []CardID
	for _, pid := range g.Players() {
		if onlyYou && pid != host.Controller() {
			continue
		}
		for _, z := range zones {
			candidates = append(candidates, g.Zone(z, pid).Cards()...)
		}
	}
	parsed := valid.Parse(spec)
	n := 0
	for _, id := range candidates {
		if Matches(g, g.Card(id), parsed, host.Controller(), host.ID) {
			n++
		}
	}
	compare, ok := t.Param(compareKey)
	if !ok {
		compare = "GE1"
	}
	if len(compare) < 3 {
		return false
	}
	right, ok := resolveNamedAmount(g, amounts, host, compare[2:])
	if !ok {
		return false
	}
	return compareOp(n, compare[:2], right)
}

// checkSVarMatches is CardTraitBase's own CheckSVar$/SVarCompare$ block,
// both sides resolved through resolveNamedAmount (ptParam's own shape,
// continuous.go, factored out once this needed the identical
// literal-or-named-SVar resolution against a *Card rather than a
// *compile.Ability's own param). checkKey/compareKey/secondKey are the three
// param names this exact shape uses under two different names in the real
// corpus -- CardTraitBase's own CheckSVar$/SVarCompare$/CheckSecondSVar$
// (triggerCommonRequirementsMet, below) and SpellAbilityCondition's own
// ConditionCheckSVar$/ConditionSVarCompare$/OrOtherConditionSVarCompare$
// (subAbilityConditionMet, condition.go) -- generalized once the second
// caller needed the identical logic under its own param names.
func checkSVarMatches(g *Game, host *Card, amounts map[string]expr.Amount, t *compile.Ability, checkKey, compareKey, secondKey string) bool {
	checkSVar, ok := t.Param(checkKey)
	if !ok {
		return true
	}
	if _, ok := t.Param(secondKey); ok {
		return false
	}
	left, ok := resolveNamedAmount(g, amounts, host, checkSVar)
	if !ok {
		return false
	}
	compare, ok := t.Param(compareKey)
	if !ok {
		compare = "GE1"
	}
	if len(compare) < 3 {
		return false
	}
	right, ok := resolveNamedAmount(g, amounts, host, compare[2:])
	if !ok {
		return false
	}
	return compareOp(left, compare[:2], right)
}

// boolFlagMatches is CardTraitBase's own repeated
// `"True".equalsIgnoreCase(params.get(key)) != predicate()` shape: key
// absent is a pass, key present compares its True/False value against
// has(), Threshold$ False meaning "must NOT have threshold" as much a real
// line as Threshold$ True.
func boolFlagMatches(t *compile.Ability, key string, has func() bool) bool {
	v, ok := t.Param(key)
	if !ok {
		return true
	}
	return strings.EqualFold(v, "True") == has()
}

// lifeTotalMatches is CardTraitBase's own LifeTotal$/LifeAmount$ block, the
// two real corpus values on a T: line: "You" (host's own controller) and
// "ActivePlayer" (Game.ActivePlayer()) -- OpponentSmallest/OpponentGreatest
// carry no real T: line and are not resolved.
func lifeTotalMatches(g *Game, host *Card, amounts map[string]expr.Amount, t *compile.Ability) bool {
	player, ok := t.Param("LifeTotal")
	if !ok {
		return true
	}
	var life int
	switch player {
	case "You":
		life = g.Player(host.Controller()).Life
	case "ActivePlayer":
		life = g.Player(g.ActivePlayer()).Life
	default:
		return false
	}
	compare, ok := t.Param("LifeAmount")
	if !ok {
		compare = "GE1"
	}
	if len(compare) < 3 {
		return false
	}
	right, ok := resolveNamedAmount(g, amounts, host, compare[2:])
	if !ok {
		return false
	}
	return compareOp(life, compare[:2], right)
}

// triggerPhasesCheck ports Trigger.phasesCheck (Trigger.java) -- a general
// gate every trigger mode carries regardless of what it fires on, checked by
// TriggerHandler.isTriggerActive BEFORE a trigger's own mode-specific
// performTest ever runs at all. Kept as its own function rather than folded
// into triggerCommonRequirementsMet (below): the two port genuinely
// different Java methods on different classes (Trigger itself, vs.
// CardTraitBase.meetsCommonRequirements, called from inside performTest),
// not two overlapping views of the same one.
//
// Phase$ (19 real T: lines outside Mode$ Phase's own dispatch) restricts a
// trigger of ANY mode to firing only during the named step(s)/phase(s) --
// reusing phaseTriggerMatches outright, the identical "does Phase$ name the
// current phase" question Mode$ Phase's own dispatch (checkPhaseTriggers,
// above) already answers with it, just asked generically here for every
// OTHER mode too. Confusingly, this is the same literal param key Mode$
// Phase reads for a wholly different reason (TriggerPhase.performTest itself
// checks only ValidPlayer$; Phase$ there is exactly this same general gate
// applied to that one mode, not a separate mode-specific dispatch --
// phaseTriggerMatches's own doc comment). A Mode$ Phase line reaching this
// function too (through triggerEffectAPI, below) re-asks the identical
// question against the identical inputs and gets the identical answer --
// redundant with checkPhaseTriggers' own explicit call, but harmless.
//
// PlayerTurn$ (61) / NotPlayerTurn$ (0 real lines, ported anyway for
// symmetry with Java's own hasParam-not-value-checked contract) restrict to
// the trigger's own host controller's turn, or explicitly not it --
// Trigger.java's own isPlayerTurn(hostController) check, ported directly.
// sentinel_tower.txt's own real "Whenever an instant or sorcery spell is
// cast during your turn, CARDNAME deals 1 damage to each opponent" is
// PlayerTurn$ True on a Mode$ SpellCast line -- one of 43 real lines across
// six already-built modes (SpellCast 12, ChangesZone 9, LifeGained 5, Taps
// 2, Discarded 1, Drawn 1, plus Phase$'s own ChangesZone 11/SpellCast 2)
// that were firing UNCONDITIONALLY until this landed, a wrong answer rather
// than a coverage gap (PORT-8/GO-7) -- this port had never checked either
// key at all before now. OpponentTurn$ (23, SpellCast/Drawn) is
// Player.isOpponentOf's own question, which collapses to the identical
// check NotPlayerTurn$ already makes in this port's own no-team model
// (matchesPlayerBase's own doc comment: with no teams, "not my turn" and
// "my opponent's turn" are the same fact).
//
// FirstCombat$ (6, Attacks/AttackersDeclared, both already built) is
// PhaseHandler.isFirstCombat's own nCombatsThisTurn==1 -- always true here:
// this port has no extra-combat mechanism (an AddCombat effect is not
// built, turn.go's own doc comment: "extra turns/phases... not here"), so
// no real game this port can play ever reaches a second combat phase in the
// same turn, making a hardcoded true the CORRECT answer today rather than a
// guess -- the identical reasoning combatdamage.go's own CombatDamage$
// check already uses for the identical "no mechanism makes this false yet"
// shape. 0 real lines write FirstCombat$ False, so the reverse case needs
// no answer here.
//
// Not resolved, each skipping the whole line rather than guessing (GO-7):
// FirstUpkeep$ (1) / FirstUpkeepThisGame$ (2, both Mode$ Phase only) --
// PhaseHandler.isFirstUpkeep/isFirstUpkeepThisGame's own per-turn/per-game
// upkeep-step counters, unlike FirstCombat$ genuinely reachable as false on
// any turn after the first even without a new mechanism, and this port
// tracks neither; TurnCount$ (0 real lines, dormant).
func triggerPhasesCheck(g *Game, host *Card, t *compile.Ability) bool {
	for _, key := range [...]string{"FirstUpkeep", "FirstUpkeepThisGame", "TurnCount"} {
		if _, ok := t.Param(key); ok {
			return false
		}
	}
	if _, ok := t.Param("Phase"); ok {
		if !phaseTriggerMatches(t, g.ActivePhase()) {
			return false
		}
	}
	if _, ok := t.Param("PlayerTurn"); ok {
		if g.ActivePlayer() != host.Controller() {
			return false
		}
	}
	if _, ok := t.Param("NotPlayerTurn"); ok {
		if g.ActivePlayer() == host.Controller() {
			return false
		}
	}
	if _, ok := t.Param("OpponentTurn"); ok {
		if g.ActivePlayer() == host.Controller() {
			return false
		}
	}
	if v, ok := t.Param("FirstCombat"); ok {
		if !strings.EqualFold(v, "True") {
			return false
		}
	}
	return true
}

// triggerEffectAPI is a trigger's own Execute$ sub-ability -- the "DB$ <API>"
// record its SVar compiled into -- and the APIType that record's own Name
// names (compile.Ability's own Name field, the API for a Spell/DB record).
// The returned *compile.Ability is what Ability.Params carries onto the
// stack: an Effect's own Resolve reads Defined$/NumCards$/whatever else it
// needs straight off it (drawEffect, draweffect.go, is the first). Also
// checks triggerPhasesCheck (above, Trigger.phasesCheck's own port) and
// triggerCommonRequirementsMet (below, CardTraitBase.meetsCommonRequirements's
// own port) -- the two gates every trigger mode shares, Java's own checks
// before any mode-specific performTest runs at all, folded in here rather
// than duplicated at all eighteen call sites.
// Reports false for a trigger with no Execute key at all, or one naming an
// API string ApiType.java does not have (APIByName's own exact-match
// contract) -- neither is reachable against the real corpus today, but a
// card cannot be trusted not to be the first (PORT-8).
func triggerEffectAPI(g *Game, host *Card, amounts map[string]expr.Amount, t *compile.Ability) (*compile.Ability, APIType, bool) {
	if !triggerPhasesCheck(g, host, t) {
		return nil, 0, false
	}
	if !triggerCommonRequirementsMet(g, host, amounts, t) {
		return nil, 0, false
	}
	for _, sub := range t.Subs {
		if !strings.EqualFold(sub.Key, "Execute") {
			continue
		}
		api, ok := APIByName(sub.Ability.Name)
		return sub.Ability, api, ok
	}
	return nil, 0, false
}

// checkAttackersDeclaredTrigger is CR 508.1's own "whenever a player
// attacks" trigger -- TriggerAttackersDeclared.performTest, ported from
// PhaseHandler.java's own declareAttackersStep, the exact call site that
// fires it: once per combat, right after every attacker is tapped and
// assigned a target, and only "if (!combat.getAttackers().isEmpty())" --
// unlike checkAttacksTriggers (above), which fires once per declared
// attacker, this fires at most once per combat regardless of how many
// creatures attacked, so a combat with zero attackers never reaches this at
// all (DeclareCombatAttackers, attack.go, guards the call the identical way).
//
// 286 real S:T:Mode$ AttackersDeclared lines corpus-wide (vocabscan). Walks
// the same four zones Mode$ Phase does (phaseTriggerZones, above) rather
// than Battlefield alone: 273 real lines carry TriggerZones$ Battlefield,
// but 7 carry Command and 5 carry Graveyard (grep against a corpus dump of
// every real line), the identical minority-but-real split that made Phase's
// own zone walk necessary rather than a battlefield-only shortcut.
//
// IsPresent$/PresentCompare$ (14, 4) and CheckSVar$ (13) are resolved now
// too, the identical fix checkPhaseTriggers' own doc comment describes:
// triggerCommonRequirementsMet (below) already evaluates both generically
// through triggerEffectAPI, and this function's own hasAnyParam pre-filter
// used to name them anyway, silently keeping them skipped even after that
// general mechanism landed.
//
// Skipped via hasAnyParam, the same "whole line, not a guess" contract every
// other trigger mode's own skip-list already has: Condition$ (1) --
// StaticAbility.java's own runtime gate, no equivalent for any trigger mode
// yet.
//
// Resolved: AttackingPlayer$ (matchesPlayerSpec, valid.go, against
// g.activePlayer -- Combat.getAttackingPlayer() is always the active player
// in this port's own combat model, DeclareCombatAttackers' own doc comment)
// -- 175 of 286 real lines; AttackedTarget$ (attackedTargetMatches, below) --
// 63 of 286; ValidAttackers$/ValidAttackersAmount$ (validAttackersCountMatches,
// below) -- 123 of 286.
func (g *Game) checkAttackersDeclaredTrigger(controller PlayerController) {
	if len(g.combat.Attackers) == 0 {
		return
	}
	targets := attackedTargetsOf(g)
	var matches []Ability
	for _, pid := range g.Players() {
		for _, z := range phaseTriggerZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, t := range face.Triggers {
						if !isAttackersDeclaredTrigger(t) {
							continue
						}
						if !phaseTriggerZoneMatches(t, z) {
							continue
						}
						if hasAnyParam(t, "Condition") {
							continue
						}
						if attackingPlayer, ok := t.Param("AttackingPlayer"); ok {
							matched, recognized := matchesPlayerSpec(g, g.activePlayer, h.Controller(), attackingPlayer)
							if !recognized || !matched {
								continue
							}
						}
						if attackedTarget, ok := t.Param("AttackedTarget"); ok {
							if !attackedTargetMatches(g, h, targets, attackedTarget) {
								continue
							}
						}
						if validAttackers, ok := t.Param("ValidAttackers"); ok {
							if !validAttackersCountMatches(g, h, t, validAttackers) {
								continue
							}
						}
						if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
							matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
						}
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// attackedTargetsOf is every distinct entity actually attacked this combat
// -- PhaseHandler.java's own "for (GameEntity ge : combat.getDefenders()) if
// (!combat.getAttackersOf(ge).isEmpty()) attackedTarget.add(ge)", only the
// defenders someone is actually attacking, not every legal defender in the
// game. Order follows Combat.Attackers' own declaration order (first
// attacker's target first), a plain membership map used only to skip a
// repeat, never to iterate -- GO-12's own ordering concern does not reach a
// lookup that never walks the map itself.
func attackedTargetsOf(g *Game) []EntityID {
	seen := map[EntityID]bool{}
	var out []EntityID
	for _, attacker := range g.combat.Attackers {
		target := g.combat.AttackTargets[attacker]
		if seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	return out
}

// attackedTargetMatches is TriggerAttackersDeclared's own AttackedTarget$
// check: CardTraitBase.matchesValid's own Iterable branch tries every
// attacked entity against the whole comma-split spec and reports true the
// instant any one of them matches any one token (matchesValid, CardTraitBase
// .java) -- ported here as two nested loops over the identical "any target,
// any token" combination, rather than one loop that assumes which type a
// given token means: real corpus specs mix player-shaped tokens ("You",
// "Player,Planeswalker") and card-shaped tokens ("Planeswalker.YouCtrl") in
// the same comma list (You,Planeswalker.YouCtrl, 4 real lines), and Java's
// own dispatch is by the CANDIDATE's type (Player.isValid vs Card.isValid),
// not the token's syntax, so both matchers are tried against both kinds of
// target and the wrong-kind attempt just reports "not recognized" and moves
// on (matchesPlayerSpec's own ok=false for an unrecognized base, valid.go;
// valid.Parse building a Spec no real card or player ever has the base type
// of, for the reverse mismatch).
//
// A qualified player token matchesPlayerSpec cannot resolve (Player
// .EnchantedBy, 9 of 63 real AttackedTarget$ lines; Player.hasInitiative,
// Player.IsPoisoned, Opponent.lifeGTX, 1 each) never matches through either
// branch, so a trigger naming one of these never fires -- GO-7's usual
// "skip rather than guess" outcome, reached here by simply never matching
// rather than a separate hasAnyParam skip, since the two are observably
// identical (this trigger firing) and Java's own dispatch has no
// "unresolvable, abort" case of its own to mirror.
func attackedTargetMatches(g *Game, host *Card, targets []EntityID, spec string) bool {
	for _, token := range strings.Split(spec, ",") {
		for _, target := range targets {
			if pid, ok := target.AsPlayer(); ok {
				if matched, recognized := matchesPlayerSpec(g, pid, host.Controller(), token); recognized && matched {
					return true
				}
				continue
			}
			if cid, ok := target.AsCard(); ok {
				if Matches(g, g.Card(cid), valid.Parse(token), host.Controller(), host.ID) {
					return true
				}
			}
		}
	}
	return false
}

// validAttackersCountMatches is TriggerAttackersDeclared's own
// ValidAttackers$/ValidAttackersAmount$ pair: how many of this combat's
// declared attackers ValidAttackers$ matches, compared against
// ValidAttackersAmount$ (default "GE1", Java's own
// `getParamOrDefault("ValidAttackersAmount", "GE1")` -- "one or more,"
// matching TriggerDescription's own real corpus phrasing, "whenever one or
// more Knights you control attack"). Every real ValidAttackersAmount$ value
// is a plain two-letter-operator-plus-digit shape (GE2, GE3, EQ1, ...,
// vocabscan), never an SVar or "X" needing `AbilityUtils.calculateAmount`
// the way Java's own code path technically allows for, so this reads the
// digits directly rather than resolving an amount.
func validAttackersCountMatches(g *Game, host *Card, t *compile.Ability, spec string) bool {
	parsed := valid.Parse(spec)
	n := 0
	for _, id := range g.combat.Attackers {
		if Matches(g, g.Card(id), parsed, host.Controller(), host.ID) {
			n++
		}
	}
	amount, ok := t.Param("ValidAttackersAmount")
	if !ok {
		amount = "GE1"
	}
	if len(amount) < 3 {
		return false
	}
	operand, err := strconv.Atoi(amount[2:])
	if err != nil {
		return false
	}
	return compareOp(n, amount[:2], operand)
}

func isAttackersDeclaredTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "AttackersDeclared")
}

// checkDrawnTriggers is CR 120.3's own "whenever you draw a card" mode,
// `Mode$ Drawn` -- TriggerDrawn.performTest, called from DrawCards' own
// per-card loop (turn.go) the identical way checkTapsTriggers is called from
// its own two real tap sites: one call per card actually drawn, not once per
// DrawCards invocation, so "draw two cards" checks this twice with a
// different `number` each time -- the reason DrawCards' own doc comment
// (turn.go) already draws one card at a time rather than moving n at once.
//
// 161 real S:T:Mode$ Drawn lines corpus-wide (vocabscan). Walks
// phaseTriggerZones (above) rather than Battlefield alone: 150 real lines
// carry TriggerZones$ Battlefield, but 6 carry Command and 3 carry Graveyard,
// the identical minority-but-real split Phase's own zone walk exists for.
//
// Resolved: ValidCard$ (156 of 161) against drawn -- every real value
// (Card.YouCtrl, Card.OppOwn, Card.YouOwn, Card.OwnedBy, bare Card, ...) is
// an ordinary valid-string Matches (valid.go) already evaluates, needing
// nothing new; ValidPlayer$ (13) through the existing matchesPlayerSpec,
// against the player who drew rather than the trigger's own host controller
// (TriggerDrawn.performTest's own AbilityKey.Player, set to the drawing
// player in Player.java's own drawCard, not a qualified restriction on the
// host); Number$ (79) against CardsDrawnThisTurn (player.go), Java's own
// numDrawnThisTurn incremented once per card BEFORE the trigger check runs
// (Player.java's own drawCard: "numDrawnThisTurn++ ... runParams.put
// (AbilityKey.Number, numDrawnThisTurn)"), so this port's own increment
// (DrawCards, turn.go) happens in the identical order.
//
// Not resolved, skipped via hasAnyParam: FirstCardInDrawStep$ (5) --
// Java's own numDrawnThisDrawStep, a second, narrower per-draw-step counter
// this port tracks nothing for (only the per-TURN counter, CardsDrawnThisTurn,
// exists); ForReveal$ (5) -- Java's own AbilityKey.CanReveal, a
// reveal-while-drawing flag (Sensei's Divining Top-adjacent shapes) this
// port's own DrawCards has no equivalent state for.
func (g *Game) checkDrawnTriggers(controller PlayerController, drawer PlayerID, drawn CardID, number int) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, z := range phaseTriggerZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, t := range face.Triggers {
						if !isDrawnTrigger(t) {
							continue
						}
						if !phaseTriggerZoneMatches(t, z) {
							continue
						}
						if hasAnyParam(t, "FirstCardInDrawStep", "ForReveal") {
							continue
						}
						if validCard, ok := t.Param("ValidCard"); ok && !Matches(g, g.Card(drawn), valid.Parse(validCard), h.Controller(), host) {
							continue
						}
						if validPlayer, ok := t.Param("ValidPlayer"); ok {
							matched, recognized := matchesPlayerSpec(g, drawer, h.Controller(), validPlayer)
							if !recognized || !matched {
								continue
							}
						}
						if numberParam, ok := t.Param("Number"); ok {
							n, err := strconv.Atoi(numberParam)
							if err != nil || n != number {
								continue
							}
						}
						if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
							matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
						}
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

func isDrawnTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "Drawn")
}

// checkLifeGainedTriggers is CR 119.1's own "whenever you gain life" mode,
// Mode$ LifeGained, ported from TriggerLifeGained.performTest -- the
// identical shape checkDrawnTriggers is (above), no ValidCard at all (there
// is no card the event happens to, only a player gaining life), keyed
// against gainer through ValidPlayer$'s own matchesPlayerSpec dispatch, and
// walking the identical four zones phaseTriggerZones already covers: 95 of
// 98 real Mode$ LifeGained lines name TriggerZones$ Battlefield, 2 Graveyard,
// 1 Command -- the identical minority-but-real split every earlier mode that
// reuses phaseTriggerZones already has. ValidPlayer$ is present on every
// real line (You, 95; Opponent, 2; a qualified Player.Opponent, 1), so
// absence is treated as no match rather than unrestricted, the same
// contract checkAttacksTriggers' own primary-dispatch ValidCard$ has.
//
// Not resolved, skipped via hasAnyParam: OptionalDecider$ (7) -- a "you may"
// choice needing a PlayerController hook this port does not have;
// FirstTime$ (6) -- Java's own AbilityKey.FirstTime, a per-turn
// "first life gain this turn" flag distinct from any per-card counter this
// port tracks (Card.AttacksThisTurn's own shape does not apply to a
// player-keyed event); ValidSource$/Spell$/ResolvedLimit$ (1 each, no shape
// worth guessing at from a single real line).
func (g *Game) checkLifeGainedTriggers(controller PlayerController, gainer PlayerID) {
	var matches []Ability
	for _, pid := range g.Players() {
		for _, z := range phaseTriggerZones {
			for _, host := range g.Zone(z, pid).Cards() {
				h := g.Card(host)
				if h.Def == nil {
					continue
				}
				for _, face := range h.Def.Faces {
					for _, t := range face.Triggers {
						if !isLifeGainedTrigger(t) {
							continue
						}
						if hasAnyParam(t, "OptionalDecider", "FirstTime", "ValidSource", "Spell", "ResolvedLimit") {
							continue
						}
						if !phaseTriggerZoneMatches(t, z) {
							continue
						}
						validPlayer, ok := t.Param("ValidPlayer")
						if !ok {
							continue
						}
						matched, recognized := matchesPlayerSpec(g, gainer, h.Controller(), validPlayer)
						if !recognized || !matched {
							continue
						}
						if sub, api, ok := triggerEffectAPI(g, h, face.Amounts, t); ok {
							matches = append(matches, Ability{API: api, Source: host, Controller: h.Controller(), Params: sub, Amounts: face.Amounts})
						}
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

func isLifeGainedTrigger(t *compile.Ability) bool {
	return strings.EqualFold(t.Name, "LifeGained")
}

// checkBecomesTargetTriggers is CR 115/603.3's own "whenever ~ becomes the
// target of a spell or ability" -- Mode$ BecomesTarget, ported from
// TriggerBecomesTarget.performTest at the shape this port can reach:
// targets carries every distinct object a.Targets (targeting.go) just chose
// for one ability, deduplicated the identical way MagicStack.add's own
// distinctObjects set is ("Track distinct objects so Becomes targets don't
// trigger for things like Seeds of Strength"). Called from
// pushTriggeredAbilities (below) right after PushAbility, matching
// MagicStack.add's own order (SpellCastOrCopy/cast-trigger checks, then
// "Run BecomesTarget triggers"), and from castAura (castspell.go) for an
// Aura's own single cast-time target -- the two places this port ever
// finishes choosing a target for something. A targeted Instant/Sorcery would
// be a third (not built -- CastSpell only casts a permanent or an Aura
// today, castspell.go's own doc comment), so this only sees a
// triggered-ability's own targets or an Aura's own attach target, not yet a
// removal spell's, the identical "mechanism now, content later" gap
// targeting.go's own doc comment already names for a future cast path.
//
// ValidTarget$ is matched with attackedTargetMatches (below,
// AttackersDeclared's own AttackedTarget$ dispatch): the identical
// one-entity-of-either-kind problem, since a real ValidTarget$ can be a
// player spec, a card spec, or (rarely) a comma list mixing both.
// Card.AttachedBy/EnchantedBy ("enchanted creature becomes the target of a
// spell or ability," Ice Cage's own real shape) needs no new code at all:
// Matches (valid.go) already reads its own source argument as "the object
// c is checked for being attached to," and host.ID -- the watcher itself --
// is exactly that for a trigger living on the Aura in question.
//
// FirstTime$ (Glyph Keeper's own "for the first time each turn, counter
// it") reads a new Card.BecameTargetThisTurn (card.go): Java's own
// hasBecomeTargetThisTurn()/addTargetFromThisTurn is a per-target Player
// set, but every real FirstTime$ line just asks whether the set was empty,
// never which players are in it, so a bool suffices. Computed once per
// distinct target (not once per watcher) the same "check, then mark"
// order Java's own MagicStack.add loop has, so two watchers checking the
// same targeting event see identical first-time-ness.
//
// 40 of the corpus's own 118 real Mode$ BecomesTarget lines resolve --
// Illusionary Servant's own real "When CARDNAME becomes the target of a
// spell or ability, sacrifice it" among them (Sacrifice itself is still
// ErrUnimplemented, M6's own remaining scope; TestDestroyLethalToughnessFiresDiesTrigger's
// own "prove the trigger reached the stack, not that its effect ran"
// precedent applies here identically). Not resolved, each failing loudly by
// name rather than firing unconditionally (PORT-8/GO-7): ValidSource$ (77)
// -- matched against the triggering ability itself (AbilityKey.SourceSA in
// Java, a SpellAbility, not a Card), needing a Spell/Activated/Triggered
// ability-kind classifier this port's own Ability struct does not carry;
// OptionalDecider$ (12) -- an interactive "may" confirm this port's own
// PlayerController has no hook for, the identical gap Discard's own
// Optional$ already documents; Valiant$ (10) -- Card.isValiant's own
// per-activator "have you not targeted this before" set, a separate
// mechanic FirstTime$'s own plain bool cannot answer; ActivationLimit$ (3)
// and Static$ (1) -- each its own further mechanic.
func (g *Game) checkBecomesTargetTriggers(controller PlayerController, targets []EntityID) {
	var matches []Ability
	seen := make(map[EntityID]bool, len(targets))
	for _, tgt := range targets {
		if seen[tgt] {
			continue
		}
		seen[tgt] = true

		firstTime := false
		if cid, ok := tgt.AsCard(); ok {
			c := g.Card(cid)
			firstTime = !c.BecameTargetThisTurn
			c.BecameTargetThisTurn = true
		}

		for _, pid := range g.Players() {
			for _, watcher := range g.Zone(Battlefield, pid).Cards() {
				w := g.Card(watcher)
				if w.Def == nil {
					continue
				}
				for _, face := range w.Def.Faces {
					for _, t := range face.Triggers {
						if !isBecomesTargetTrigger(t) {
							continue
						}
						validTarget, ok := t.Param("ValidTarget")
						if !ok {
							continue
						}
						if !attackedTargetMatches(g, w, []EntityID{tgt}, validTarget) {
							continue
						}
						if _, ok := t.Param("FirstTime"); ok && !firstTime {
							continue
						}
						if sub, api, ok := triggerEffectAPI(g, w, face.Amounts, t); ok {
							matches = append(matches, Ability{API: api, Source: watcher, Controller: w.Controller(), Params: sub, Amounts: face.Amounts})
						}
					}
				}
			}
		}
	}
	g.pushTriggeredAbilities(controller, matches)
}

// isBecomesTargetTrigger reports whether t is a Mode$ BecomesTarget line
// this port resolves -- ValidTarget$ present, and none of
// checkBecomesTargetTriggers' own doc-commented unresolved params named.
func isBecomesTargetTrigger(t *compile.Ability) bool {
	if !strings.EqualFold(t.Name, "BecomesTarget") {
		return false
	}
	if _, ok := t.Param("ValidTarget"); !ok {
		return false
	}
	return !hasAnyParam(t, "ValidSource", "OptionalDecider", "Valiant", "ActivationLimit", "Static")
}
