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
	if c.Controller() != pid || c.Zone != Hand {
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
	g.Player(pid).SpellsCastThisTurn++
	g.checkSpellCastTriggers(controller, card, pid)
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
//
// checkBecomesTargetTriggers (trigger.go) runs after the push too: an Aura's
// own attach target is CR 115's "becomes the target of a spell" exactly as
// much as a triggered ability's chosen target is (pushTriggeredAbilities's
// own identical call, trigger.go) -- Illusionary Servant's real "when
// CARDNAME becomes the target of a spell or ability, sacrifice it" fires
// off an opponent Auraing it exactly as much as off a triggered ability
// targeting it, and this is the only cast-time path this port has today
// that could ever reach it (a targeted Instant/Sorcery is not built yet,
// checkBecomesTargetTriggers' own doc comment).
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
	g.Player(pid).SpellsCastThisTurn++
	g.checkSpellCastTriggers(controller, card, pid)
	g.checkBecomesTargetTriggers(controller, []EntityID{CardEntity(target)}, true, pid)
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
// the card leaves the stack and becomes a permanent, CR 614.1's own "enters
// tapped" replacement runs against it before anything else sees the result
// (checkMovedReplacement, replacement.go), and then CR 603.2's own ETB
// trigger check runs (checkETBTriggers, trigger.go) the same way Java's
// table.triggerChangesZoneAll does after every zone change. Java
// splits the two APIs only for getStackDescription's own display text
// (PermanentCreatureEffect overrides it to show P/T); this port has no
// stack-description system, so one stateless value answers for both.
type permanentEffect struct{}

func (permanentEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	origin := g.Card(a.Source).Zone
	g.Move(a.Source, Battlefield, a.Controller)
	g.checkMovedReplacement(a.Source, origin)
	g.checkETBTriggers(controller, a.Source, origin)
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

func (attachEffect) Resolve(g *Game, a *Ability, controller PlayerController) error {
	origin := g.Card(a.Source).Zone
	g.Move(a.Source, Battlefield, a.Controller)
	g.Attach(a.Source, a.Target)
	g.checkMovedReplacement(a.Source, origin)
	g.checkETBTriggers(controller, a.Source, origin)
	return nil
}

// NewRegistry builds a Registry carrying every Effect this port has.
// One hundred thirty-seven entries today: APIPermanentCreature and APIPermanentNoncreature share
// permanentEffect, CastSpell's own first (and so far only) real caller of
// PushAbility outside stack.go's tests; APIAttach is attachEffect, castAura's
// own; APIDraw is drawEffect (draweffect.go), M6's own first script-driven
// effect and trigger.go's first real Execute$ sub-ability to actually
// resolve rather than report ErrUnimplemented; APIDealDamage is
// dealDamageEffect (dealdamageeffect.go), M6's second, the first to reuse
// combat's own damage machinery (dealPermanentDamage/dealPlayerDamage,
// combatdamage.go) for a non-combat source; APIGainLife is gainLifeEffect
// (gainlifeeffect.go), M6's third and the corpus's single largest resolvable
// slice after DealDamage; APIPump is pumpEffect (pumpeffect.go), M6's fourth
// and the first whose own contribution outlives its Resolve call, closing
// the "duration tracking this port does not have" gap continuous.go's own
// applyContinuousPT used to name; APIPumpAll is pumpAllEffect
// (pumpalleffect.go), M6's fifth and pumpEffect's own blanket sibling --
// a ValidCards$-matched set rather than a single Defined$ card, sharing its
// duration tracking outright; APILoseLife is loseLifeEffect
// (loselifeeffect.go), M6's sixth and gainLifeEffect's own mirror image,
// unlike which it calls no trigger check at all -- 0 real
// T:Mode$ LifeLost/LifeLostAll lines corpus-wide; APIPutCounter is
// putCounterEffect (putcountereffect.go), M6's seventh and the corpus's own
// second-largest resolvable slice after Pump, the first to write
// Card.Counters/Player.Counters from a script rather than combat's own
// hardcoded loyalty-on-entry path; APIDiscard is discardEffect
// (discardeffect.go), M6's eighth and the first Effect implementation that
// asks the player anything mid-resolution -- Effect.Resolve gained a
// PlayerController parameter for it (effect.go's own doc comment); APIScry
// is scryEffect (scryeffect.go), M6's ninth and the first effect to ask the
// player to reorder cards rather than choose a subset of them --
// PlayerController's own second new decision, ArrangeForScry, and a new
// Game.MoveToLibraryTop (game.go) for the half of it Move's own
// append-at-the-end could not reach; APISurveil is surveilEffect
// (surveileffect.go), M6's tenth and ArrangeForScry's own sibling decision
// reused wholesale -- the only difference from Scry is that the cards not
// kept on top go to the graveyard rather than the bottom of the library;
// APISacrifice is sacrificeEffect (sacrificeeffect.go), M6's eleventh and
// the first effect to read SacValid$'s own battlefield scan rather than a
// Defined$/ValidTgts$ target; APISacrificeAll is sacrificeAllEffect
// (sacrificealleffect.go), M6's twelfth and sacrificeEffect's own blanket
// sibling; APIDestroy is destroyEffect (destroyeffect.go), M6's thirteenth
// and the first to read a's own Targets field directly for a ValidTgts$-
// bearing line rather than only through Defined$ (targetedOrDefinedCards,
// defined.go); APITap is tapEffect (tapeffect.go), M6's fourteenth and the
// first whose own dominant real corpus shape (ETB$'s "enters tapped")
// resolves entirely outside Registry.Resolve, through replacement.go's own
// tapAbilityResolvesTap; APIUntap is untapEffect (untapeffect.go), M6's
// fifteenth and Tap's own mirror image, firing checkUntapsTriggers
// (trigger.go) rather than checkTapsTriggers; APIFight is fightEffect
// (fighteffect.go), M6's sixteenth and the first to combine a Defined$ card
// with a ValidTgts$ target into two separate fighters rather than reading
// either alone; APIMill is millEffect (milleffect.go), M6's seventeenth and
// the first to read a PLAYER target directly (targetedOrDefinedPlayers,
// defined.go) rather than a card one, reusing scryEffect's own top-of-
// library defensive-copy idiom for a plain move instead of a reorder.
// APIRemoveCounter is removeCounterEffect (removecountereffect.go), M6's
// eighteenth and putCounterEffect's own mirror image, reusing
// definedCounterTargets outright; APIDamageAll is damageAllEffect
// (damagealleffect.go), M6's nineteenth and DealDamage's own "to every
// matching creature and/or player at once" sibling, sharing one damageTable
// across the whole sweep the identical way dealDamageEffect's own multi-
// player loop already does; APISetLife is setLifeEffect (setlifeeffect.go),
// M6's twentieth and the first to re-derive Player.setLife's own CR 119.5
// gain-or-loss dispatch (setPlayerLife) rather than mutating Player.Life
// directly the way GainLife/LoseLife each do for their own single
// direction; APIShuffle is shuffleEffect (shuffleeffect.go), M6's twenty-
// first and the thinnest effect yet -- Game.Shuffle (game.go) already
// existed, mulligan.go's own real caller, so this file is a target-
// resolution loop around one existing call; APIExchangeLife is
// exchangeLifeEffect (exchangelifeeffect.go), M6's twenty-second and
// setPlayerLife's own second caller, CR 119.10's own exchange reduced to
// "the higher total loses the difference, the lower gains it." APITapAll is
// tapAllEffect (tapalleffect.go), M6's twenty-third and tapEffect's own
// battlefield-scan sibling, ValidCards$ read through valid.Parse/Matches
// (valid.go) rather than a's own Targets/Defined$; APIUntapAll is
// untapAllEffect (untapalleffect.go), M6's twenty-fourth and TapAll's own
// mirror image; APIPutCounterAll is putCounterAllEffect
// (putcounteralleffect.go), M6's twenty-fifth and putCounterEffect's own
// battlefield-scan sibling; APIRemoveCounterAll is removeCounterAllEffect
// (removecounteralleffect.go), M6's twenty-sixth, reusing removeCounters
// (removecountereffect.go) outright for its own per-card body the identical
// way removeCounterEffect's own does; APIMultiplyCounter is
// multiplyCounterEffect (multiplycountereffect.go), M6's twenty-seventh and
// the first M6 effect to read a's own Targets field through
// targetedOrDefinedCards for a MultiplyCounter-shaped line rather than a
// Destroy/Tap/Untap-shaped one. APIMana is manaEffect (manaeffect.go), M6's
// twenty-eighth and the first to reuse activatemanaability.go's own
// producedManaColor/parseComboColors/ChooseManaColor for a RESOLVING mana
// burst rather than an activated ability; APIMoveCounter is
// moveCounterEffect (movecountereffect.go), M6's twenty-ninth and
// PutCounter's/RemoveCounter's own card-to-card sibling; APIPoison is
// poisonEffect (poisoneffect.go), M6's thirtieth and Radiation's own
// mirror-shaped sibling below; APIUnattach is unattachEffect
// (unattacheffect.go), M6's thirty-first, a target-resolution loop around
// the already-built Game.Unattach; APIRevealHand is revealHandEffect
// (revealhandeffect.go), M6's thirty-second and the first effect whose only
// real state change is Memory.Remember, since this port's own engine has no
// hidden information to actually reveal; APILosesGame is losesGameEffect
// (losesgameeffect.go), M6's thirty-third, a target-resolution loop around
// the already-checked Player.Lost; APIWinsGame is winsGameEffect
// (winsgameeffect.go), M6's thirty-fourth and Player.Won's own first writer
// past CheckStateBasedActions' own CR 104.2a elimination count, which now
// checks it first (action.go); APIRadiation is radiationEffect
// (radiationeffect.go), M6's thirty-fifth and Poison's own mirror-shaped
// sibling above; APIRemoveFromCombat is removeFromCombatEffect
// (removefromcombateffect.go), M6's thirty-sixth, a target-resolution loop
// around the new Game.removeFromCombat (combat.go); APIConnive is
// conniveEffect (conniveeffect.go), M6's thirty-seventh and the first M6
// effect built entirely by composing three already-built primitives
// (Game.DrawCards, ChooseCardsToDiscard/discardCards, Counters.Add) rather
// than adding a new one of its own. The next ten -- Cleanup, DestroyAll,
// ChooseCard, ChoosePlayer, ChooseColor, ChooseNumber, Reveal,
// PeekAndReveal, TapOrUntap, Proliferate -- land together: Cleanup and the
// four Choose effects write the host's Memory (memory.go) that definedCards'
// Remembered/Imprinted/ChosenCard and definedPlayers' ChosenPlayer/Remembered
// cases (defined.go) read back. The next twenty -- ChangeZone,
// ChangeZoneAll, Dig, DigUntil, RearrangeTopOfLibrary, Explore, LookAt,
// Branch, GenericChoice, Repeat, RepeatEach, Regenerate, Fog, AddTurn,
// SkipTurn, GainControl, ExchangeControl, HealDamage, EachDamage, DrainMana
// -- share three new pieces of engine: moveByEffect/orderCardsByTheirOwners
// (zonemove.go), AdditionalAbility dispatch (additional.go), and
// regeneration shields, extra/skipped turns and one-shot control changes
// (regeneration.go, turn.go, gaincontroleffect.go). The next twenty --
// Token, Investigate, Amass, Incubate, Animate, AnimateAll, Debuff,
// Protection, ProtectionAll, DelayedTrigger, ImmediateTrigger, Charm,
// FlipCoin, RollDice, Clash, Seek, StoreSVar, Balance, AddPhase, SkipPhase
// -- add token creation (token.go), Animate-shaped one-shot layer records
// (animate.go), delayed triggers (delayedtrigger.go), push-time Charm modes
// (charmeffect.go) and extra/skipped phases (addphaseeffect.go,
// skipphaseeffect.go). The next fifty -- BlankLine through DamageResolve,
// registered below in that order -- add ChooseBinary/ChooseOption
// decisions, detain and goad legality, forced blocks, prevention shields,
// damage maps, face-down and transformed cards (facedown.go,
// setstateeffect.go), stack-spell targeting for Counter, and day/night.
//
// Explicit construction here, not an
// init() populating a package-level Registry, is ADR-0003's own "explicit
// wiring... so the direction stays visible and test binaries can register a
// subset" -- a caller that wants fewer registered APIs builds its own
// Registry by hand instead of calling this.
func NewRegistry() *Registry {
	var r Registry
	r[APIPermanentCreature] = permanentEffect{}
	r[APIPermanentNoncreature] = permanentEffect{}
	r[APIAttach] = attachEffect{}
	r[APIDraw] = drawEffect{}
	r[APIDealDamage] = dealDamageEffect{}
	r[APIGainLife] = gainLifeEffect{}
	r[APIPump] = pumpEffect{}
	r[APIPumpAll] = pumpAllEffect{}
	r[APILoseLife] = loseLifeEffect{}
	r[APIPutCounter] = putCounterEffect{}
	r[APIDiscard] = discardEffect{}
	r[APIScry] = scryEffect{}
	r[APISurveil] = surveilEffect{}
	r[APISacrifice] = sacrificeEffect{}
	r[APISacrificeAll] = sacrificeAllEffect{}
	r[APIDestroy] = destroyEffect{}
	r[APITap] = tapEffect{}
	r[APIUntap] = untapEffect{}
	r[APIFight] = fightEffect{}
	r[APIMill] = millEffect{}
	r[APIRemoveCounter] = removeCounterEffect{}
	r[APIDamageAll] = damageAllEffect{}
	r[APISetLife] = setLifeEffect{}
	r[APIShuffle] = shuffleEffect{}
	r[APIExchangeLife] = exchangeLifeEffect{}
	r[APITapAll] = tapAllEffect{}
	r[APIUntapAll] = untapAllEffect{}
	r[APIPutCounterAll] = putCounterAllEffect{}
	r[APIRemoveCounterAll] = removeCounterAllEffect{}
	r[APIMultiplyCounter] = multiplyCounterEffect{}
	r[APIMana] = manaEffect{}
	r[APIMoveCounter] = moveCounterEffect{}
	r[APIPoison] = poisonEffect{}
	r[APIUnattach] = unattachEffect{}
	r[APIRevealHand] = revealHandEffect{}
	r[APILosesGame] = losesGameEffect{}
	r[APIWinsGame] = winsGameEffect{}
	r[APIRadiation] = radiationEffect{}
	r[APIRemoveFromCombat] = removeFromCombatEffect{}
	r[APIConnive] = conniveEffect{}
	r[APICleanup] = cleanupEffect{}
	r[APIDestroyAll] = destroyAllEffect{}
	r[APIChooseCard] = chooseCardEffect{}
	r[APIChoosePlayer] = choosePlayerEffect{}
	r[APIChooseColor] = chooseColorEffect{}
	r[APIChooseNumber] = chooseNumberEffect{}
	r[APIReveal] = revealEffect{}
	r[APIPeekAndReveal] = peekAndRevealEffect{}
	r[APITapOrUntap] = tapOrUntapEffect{}
	r[APIProliferate] = proliferateEffect{}
	r[APIChangeZone] = changeZoneEffect{}
	r[APIChangeZoneAll] = changeZoneAllEffect{}
	r[APIDig] = digEffect{}
	r[APIDigUntil] = digUntilEffect{}
	r[APIRearrangeTopOfLibrary] = rearrangeTopOfLibraryEffect{}
	r[APIExplore] = exploreEffect{}
	r[APILookAt] = lookAtEffect{}
	r[APIBranch] = branchEffect{}
	r[APIGenericChoice] = genericChoiceEffect{}
	r[APIRepeat] = repeatEffect{}
	r[APIRepeatEach] = repeatEachEffect{}
	r[APIRegenerate] = regenerateEffect{}
	r[APIFog] = fogEffect{}
	r[APIAddTurn] = addTurnEffect{}
	r[APISkipTurn] = skipTurnEffect{}
	r[APIGainControl] = gainControlEffect{}
	r[APIExchangeControl] = exchangeControlEffect{}
	r[APIHealDamage] = healDamageEffect{}
	r[APIEachDamage] = eachDamageEffect{}
	r[APIDrainMana] = drainManaEffect{}
	r[APIToken] = tokenEffect{}
	r[APIInvestigate] = investigateEffect{}
	r[APIAmass] = amassEffect{}
	r[APIIncubate] = incubateEffect{}
	r[APIAnimate] = animateEffect{}
	r[APIAnimateAll] = animateAllEffect{}
	r[APIDebuff] = debuffEffect{}
	r[APIProtection] = protectionEffect{}
	r[APIProtectionAll] = protectionAllEffect{}
	r[APIDelayedTrigger] = delayedTriggerEffect{}
	r[APIImmediateTrigger] = immediateTriggerEffect{}
	r[APICharm] = charmEffect{}
	r[APIFlipCoin] = flipCoinEffect{}
	r[APIRollDice] = rollDiceEffect{}
	r[APIClash] = clashEffect{}
	r[APISeek] = seekEffect{}
	r[APIStoreSVar] = storeSVarEffect{}
	r[APIBalance] = balanceEffect{}
	r[APIAddPhase] = addPhaseEffect{}
	r[APISkipPhase] = skipPhaseEffect{}
	r[APIBlankLine] = blankLineEffect{}
	r[APIGameDrawn] = gameDrawnEffect{}
	r[APIRemoveFromGame] = removeFromGameEffect{}
	r[APIReverseTurnOrder] = reverseTurnOrderEffect{}
	r[APIChangeSpeed] = changeSpeedEffect{}
	r[APIGainOwnership] = gainOwnershipEffect{}
	r[APIReorderZone] = reorderZoneEffect{}
	r[APIEndTurn] = endTurnEffect{}
	r[APIEndCombatPhase] = endCombatPhaseEffect{}
	r[APIChooseEvenOdd] = chooseEvenOddEffect{}
	r[APIChooseDirection] = chooseDirectionEffect{}
	r[APIExchangeLifeVariant] = exchangeLifeVariantEffect{}
	r[APIExchangePower] = exchangePowerEffect{}
	r[APITapOrUntapAll] = tapOrUntapAllEffect{}
	r[APIAddOrRemoveCounter] = addOrRemoveCounterEffect{}
	r[APIBecomesBlocked] = becomesBlockedEffect{}
	r[APIBlock] = blockEffect{}
	r[APIChangeCombatants] = changeCombatantsEffect{}
	r[APIGainControlVariant] = gainControlVariantEffect{}
	r[APIDetain] = detainEffect{}
	r[APIIntensify] = intensifyEffect{}
	r[APIBlight] = blightEffect{}
	r[APITimeTravel] = timeTravelEffect{}
	r[APIEndure] = endureEffect{}
	r[APIAssignGroup] = assignGroupEffect{}
	r[APIVillainousChoice] = villainousChoiceEffect{}
	r[APITwoPiles] = twoPilesEffect{}
	r[APIChooseType] = chooseTypeEffect{}
	r[APINameCard] = nameCardEffect{}
	r[APIPreventDamage] = preventDamageEffect{}
	r[APIDigMultiple] = digMultipleEffect{}
	r[APIRecruit] = recruitEffect{}
	r[APIBidLife] = bidLifeEffect{}
	r[APIExchangeControlVariant] = exchangeControlVariantEffect{}
	r[APIDayTime] = dayTimeEffect{}
	r[APIAlterAttribute] = alterAttributeEffect{}
	r[APIVote] = voteEffect{}
	r[APIMakeCard] = makeCardEffect{}
	r[APILearn] = learnEffect{}
	r[APICopyPermanent] = copyPermanentEffect{}
	r[APICounter] = counterEffect{}
	r[APIManifest] = manifestEffect{api: "Manifest", remember: "RememberManifested"}
	r[APICloak] = manifestEffect{api: "Cloak", cloak: true, remember: "RememberCloaked"}
	r[APIManifestDread] = manifestDreadEffect{}
	r[APISetState] = setStateEffect{}
	r[APIGoad] = goadEffect{}
	r[APIRemoveFromMatch] = removeFromMatchEffect{}
	r[APIActivateAbility] = activateAbilityEffect{}
	r[APIMultiplePiles] = multiplePilesEffect{}
	r[APIDamageResolve] = damageResolveEffect{}
	return &r
}
