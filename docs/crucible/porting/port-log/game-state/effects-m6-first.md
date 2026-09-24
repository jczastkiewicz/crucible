# Port Log — Game State: M6 Effects: Draw to LoseLife

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine)

M6's first script-driven effects, one section each.

## M6's first effect: Draw

Every trigger this port could detect
([`## Trigger firing`](triggers.md#trigger-firing-entering-dying-attacking-blocking-dealing-damage-being-discarded-becoming-tapped-tapping-for-mana-casting-a-spell-the-beginning-of-a-step-or-phase-a-player-attacking-drawing-a-card-and-watching-another-permanent))
reported `ErrUnimplemented` the moment `ResolveStack` reached it — correct as far as it went, but nothing had ever
actually resolved a script-driven effect. `drawEffect` (`draweffect.go`) is the first: `Mode$`/`DB$ Draw`, CR 120.3,
ported from `DrawEffect.java`'s own `resolve`.

Reaching it needed a real gap closed first: `Ability` (`ability.go`) carried only `API`/`Source`/`Controller`/`Target`
onto the stack, nothing of the sub-ability's own params — `triggerEffectAPI` extracted just the `APIType` from a
trigger's `Execute$` sub-ability and threw the rest away, which was invisible as long as nothing on the stack ever
needed to read `Defined$`/`NumCards$`/anything else back. A new `Ability.Params *compile.Ability` field carries that
sub-ability along now; `triggerEffectAPI` returns it alongside the `APIType`, and every `PushAbility` call in
`trigger.go` sets it.

Two of Java's params are handled, the corpus-frequent shapes among a bigger vocabulary
(`AbilityUtils.calculateAmount`/`getDefinedPlayers`, both far larger than what this slice needed): `NumCards$` (absent
means 1, Java's own default), through `resolveNamedAmount` (amount.go) — a plain integer or a named SVar this face
defines, upgraded from a bare `strconv.Atoi` once `dealDamageEffect`'s own `NumDmg$` needed the identical resolution and
`Ability` grew an `Amounts` field to carry it ([`## M6's second effect: DealDamage`](#m6s-second-effect-dealdamage),
below); a `*`-shaped amount or one outside `resolveAmount`'s own Valid family still errors by name rather than guessing
(the identical gap `valid.go`'s `compareMatches` already documents for a valid-string's own numeric compare); and
`Defined$ You` (896 of 2,576 real `DB$ Draw` lines) or `Defined$ Opponent`/`Player.Opponent` (11) — every opponent still
in the game, `p.isInGame()`'s own check reproduced as `!g.Player(pid).Lost`, through a new `definedPlayers`
(`defined.go`) once `dealDamageEffect` needed the identical resolution too (below) — this file's own version,
`drawDefinedPlayers`, relocated and renamed since neither effect owns it outright. `Remembered`/`ChosenPlayer` read the
host's `Memory` (`defined.go`); the rest of what Java's `getTargetPlayersWithDuplicates` can resolve (`TriggeredPlayer`,
`TriggeredController`, plain spell targeting when `Defined$` is absent entirely) is not: each is a named
reference-resolution vocabulary this port has no representation for yet, so `drawEffect` errors by name instead of
drawing for the wrong player. `Upto`, `OptionalDecider`, `Reveal` and `RememberDrawn` are the same kind of gap — none of
`PlayerController`'s methods this port has yet covers a numeric or reveal choice — checked and rejected explicitly
rather than silently ignored.

The actual draw mechanism was already built and correct: `drawStep` (`turn.go`) already drew one card for the active
player, library-empty case included. `DrawCards(pid, n)` is that same body, generalized to n cards for any player and
exported so `drawEffect` can call it — `drawStep` becomes a one-line `g.DrawCards(g.activePlayer, 1)`, not a duplicate.

`NewRegistry` (`castspell.go`) registers `APIDraw`; a new `draweffect` `enginelint` group sits above `turn` (for
`DrawCards`), `player`, `game` and `ability`, and `castspell` gained it as a dependency to register into.
`TestCastSpellFiresETBTrigger` changed from checking `ResolveStack` reports `ErrUnimplemented` naming `Draw` to checking
a real library card actually reaches hand — the same fixture, testing what is now really there instead of the gap that
used to be. Six new tests (`draweffect_test.go`) drive every resolvable and every rejected shape through the real
cast-and-resolve pipeline, `drawEffect` itself being unexported (TEST-1).

`drawEffect` never named `SubAbility$` among its own unresolved params, so once `resolveSubAbility` ("SubAbility
chaining itself lands," further below) landed, chaining started working for `Draw` for free: 170 of the corpus's own 747
real SVar-defined `Draw` lines naming `SubAbility$` chain to an already-built leaf ability and resolve end to end --
Rousing Read's own real "draw two cards, then discard a card" (`DB$ Draw`, chaining into `DB$ Discard`) among them.

---

## M6's second effect: DealDamage

`DealDamage` (CR 119/120.1, `DamageDealEffect.java`) is the corpus's single most frequent `AB$`/`DB$` API after
`ChangeZone` -- 2,219 real `(AB|DB)$ DealDamage` lines. Java's own `resolve` batches every target's damage into a
`CardDamageTable` and applies the whole table at once through `GameAction.dealDamage`; this port's own damage machinery,
`dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go), already applies one target's damage immediately -- marking,
the `DamageDealt` event, CR 614's own prevention and CR 603's own trigger, all in one call -- the identical
simplification `dealCombatDamageStep`'s own doc comment already gives for combat's own multi-target exchanges: nothing
between two sequential applications can observe or react differently yet (no interactive priority pass exists), so
sequential produces the identical final state "simultaneous" would.

Scoped to the corpus's single largest resolvable slice: a plain-or-named-SVar `NumDmg$` dealt to a `Defined$` player
(`You`/`Opponent`/`Player.Opponent`) or the ability's own host (`Self`), sourced from that same host -- 62 of the 822
real lines naming any `Defined$` value at all (2,219 total). `DamageSource$` (17 of 822) names a source other than the
host and is not resolved, no reference vocabulary for it existing yet; every other real line either carries no
`Defined$` at all (`ValidTgts$`-driven targeting, this port's own "90 of `PlayerController`'s 110 methods" gap,
[`## Not ported yet`](../game-state.md#not-ported-yet) below) or one of the ~15 other `Defined$` shapes real corpus
lines use (`TriggeredPlayer`, `Remembered`, `Targeted`, ...), none of which this port has a reference-resolution
vocabulary for yet -- the identical gap `drawEffect`'s own doc comment already names for `Draw`.

`dealDamageEffect` (`dealdamageeffect.go`, new) reads `NumDmg$` through `resolveNamedAmount` (amount.go), and `Defined$`
two ways: `"Self"` resolves directly to `a.Source` (the ability's own host, `dealPermanentDamage`); anything else goes
through `definedPlayers` (`defined.go`, new -- `drawEffect`'s own `drawDefinedPlayers` renamed and relocated, below).
`HasKeyword("Deathtouch")` read off the source card supplies `dealPermanentDamage`'s own deathtouch flag the identical
way combat's own attacker/blocker already do -- CR 702.2b's own "any nonzero deathtouch damage is lethal" then applies
through the ordinary lethal-damage state-based action, no special case needed for a script source versus a creature.

Reusing `dealPermanentDamage`/`dealPlayerDamage` directly (rather than a parallel script-only damage path) needed one
real generalization: both were combat-only until now, each hardcoding `true` for `isCombat` at their own
`damagePrevented`/`checkDamageDoneTriggersToCard`-style calls and their own emitted event's `FlagCombat`. Both gained an
explicit `isCombat bool` parameter -- every existing call site in `combatdamage.go` now passes `true` literally,
`dealDamageEffect` the first to pass `false` -- and `FlagCombat`'s own doc comment ("marks damage dealt in combat rather
than by an effect") finally has a second case to distinguish,
`var flags EventFlags; if isCombat { flags |= FlagCombat }` in place of the bare literal each function used to emit
unconditionally.

`resolveNamedAmount` needed a way to reach a card that isn't a trigger's own host: `Ability` (`ability.go`) gained an
`Amounts map[string]expr.Amount` field, alongside `Params`, so a script-driven effect's own `Resolve` can read the
compiling face's SVar table the identical way `triggerCommonRequirementsMet`/`ptParam` already do. Every one of the
eighteen `Ability{API: api, Source: ..., Controller: ..., Params: sub}` constructions across `trigger.go`'s own
check-triggers functions gained `Amounts: face.Amounts` -- `face` already in scope at each, `triggerEffectAPI` itself
already taking it as a parameter
([`## CardTraitBase.meetsCommonRequirements`](triggers.md#cardtraitbasemeetscommonrequirements-the-one-gate-every-trigger-mode-shares))
-- confirmed mechanical by a one-line `sed` substitution matched against the exact count of eighteen. `drawEffect`'s own
`NumCards$` was upgraded to `resolveNamedAmount` too once the field existed, closing a named-SVar shape it used to
reject outright for free (above).

`definedPlayers` is `drawDefinedPlayers` (draweffect.go) verbatim, moved to a new `defined.go` and renamed once
`dealDamageEffect` needed the identical `You`/`Opponent`/`Player.Opponent` resolution its own `Defined$` already has --
neither effect owns it outright, the same "shared, so neither" reason `amount.go`'s own `resolveAmount` sits apart from
its callers.

Not resolved, each skipped whole via an allow-list of the params real corpus lines pair with this shape rather than a
reject-list of the ones found (`tapAbilityResolvesTap`'s own identical style, replacement.go) -- a param neither list
has seen skips by construction instead of silently applying (PORT-8/GO-7): `Planeswalker$`/
`ValidTgts$`/`TriggeredSpellAbility$`/`CounterNum$`/`Optional$`/`TgtPrompt$` (each its own further mechanic, no real
line among the 822 combining more than one); `NoPrevention$` (1) -- this port's own
`damagePrevented`/`damagePreventedPlayer` would otherwise wrongly apply where Java's own `AbilityKey.NoPreventDamage`
says the damage cannot be prevented at all. `SubAbility$` (80 of 822) no longer blocks -- removed from
`dealDamageUnresolvedParams` once `resolveSubAbility`
(["SubAbility chaining itself lands"](targeting-and-chaining.md#subability-chaining-itself-lands),) landed: 9 of the
corpus's own 316 real SVar-defined `DealDamage` lines naming `SubAbility$` chain to an already-built leaf ability and
resolve end to end. `UnlessPayer$`/`UnlessCost$`/`UnlessResolveSubs$` no longer block either, removed once
`resolveUnlessCost`
(["`Registry.Resolve`'s own `UnlessCost$` gate"](activation.md#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),)
landed: 3 of the corpus's own 31 real `DealDamage` lines naming `UnlessCost$` resolve past that gate. `DamageMap$`
records the damage in the ability's damage map instead of dealing it; a later `DamageResolve` in the chain deals the
whole batch
(["Fifty more"](effects-batches.md#fifty-more-combat-changes-choices-prevention-copies-face-down-cards-transform-counterspells)).

`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` (5 of the 822, once
`SubAbility$`/`DamageSource$`/every other still-unresolved param above is excluded) are resolved now too, the exact same
fix `LandTapped`'s own checkland shape needed
([`## Replacement effects`](replacement.md#replacement-effects-entering-the-battlefield-tapped)):
`subAbilityConditionMet` (`condition.go`, new) is `SpellAbilityCondition.areMet`'s own gate, shared because both a
replacement's `DB$ Tap` and a script effect's own top-level ability need the identical two shapes. A met condition lets
`dealDamageEffect.Resolve` run as normal; an unmet one returns `nil` rather than an error --
`SpellAbilityCondition.areMet`'s own contract is "the ability does nothing," the identical "declined by the rules"
outcome `PlayLand`/`PayManaCost` already report as a plain `false`/`nil`. Two params stay in
`dealDamageUnresolvedParams`, failing loudly by name the identical way `DamageSource$`/`SubAbility$` already do:
`Condition$` itself (SpellAbilityCondition's own separate Threshold/Metalcraft/... flag switch) and `ConditionDefined$`
(an arbitrary reference this port has no Defined$-to-objects resolver for). `condition.go`'s own generic
unresolved-param guard would otherwise silently no-op these two specifically, which this file's own established contract
(every unresolvable param fails loudly, never silently) does not allow.

`NewRegistry` (`castspell.go`) registers `APIDealDamage`; new `enginelint` groups `defined` (above `game`/`player`),
`dealdamageeffect` (above `card`/`game`/`player`/`ability`/`combatdamage`/`defined`/`amount`/`condition`) and
`condition` (above `id`/`card`/`game`/`ability`/`trigger`, shared with `replacement`), `castspell` gaining
`dealdamageeffect` as a dependency to register into, `draweffect` gaining `card`/`defined`/`amount` for its own upgraded
`NumCards$` and shared `definedPlayers`. `TestCastSpellFiresOtherPermanentsWatchingTrigger` (Impact Tremors, item 26's
own "checking the mechanism, not the content" trigger fixture) changed from checking `ResolveStack` reports
`ErrUnimplemented` naming `DealDamage` to checking both opponents' life actually drops -- the same fixture, testing what
is now really there. Thirteen tests (`dealdamageeffect_test.go`) drive every resolvable and every rejected shape through
the real cast-and-resolve pipeline, `dealDamageEffect` itself being unexported (TEST-1); one of them (a Deathtouch
source damaging itself) is split into two -- a Deathtouch-free case proving `Damage.Marked`/`Deathtouch` directly, and a
separate Deathtouch case proving the creature destroys itself instead, since any nonzero deathtouch damage is lethal (CR
702.2b) and a dead creature no longer meaningfully has the fields the first case checks. A genuinely single-player
`newGame` combined with a multi-ability `ResolveStack` pass surfaced a real, separate, pre-existing engine property
while debugging this: `CheckStateBasedActions`' own CR 104.2a check ("one player left standing wins") ends the game the
moment exactly one player remains un-lost, which is _every_ single-player game from its very first check onward -- fine
for every earlier test, which either never needed a second `ResolveStack` loop iteration or completed its own
assertion-relevant work before that first check ran, but fatal to a test relying on `ResolveStack` to loop back around
for a trigger a first resolution pushes. Not a bug to fix, since a real game is never single-player; the fix was the
test's own player count, not the engine.

Three of those thirteen are the Condition-family additions: a met/unmet pair,
`TestDealDamageEffectFiresWhenConditionCheckSVarIsMet`/`TestDealDamageEffectNoOpsWhenConditionCheckSVarIsNotMet`, and
`TestDealDamageEffectRejectsConditionItself` (proving `Condition$` itself still fails loudly, distinct from the resolved
`ConditionCheckSVar$`/`ConditionPresent$` pair).

---

## M6's fourth effect: GainLife, and `Mode$ LifeGained`

`GainLife` (CR 119.1, `LifeGainEffect.java`) is the corpus's single largest resolvable slice past `DealDamage` -- 1,700
real `(AB|DB)$ GainLife` lines, and once scoped to `Defined$ You`/`Player.Opponent` with no other unresolved param, 857
of them resolve, more than `DealDamage`'s own 62. `gainLifeEffect` (`gainlifeeffect.go`, new) is `dealDamageEffect`'s
own shape almost verbatim: `LifeAmount$` through `resolveNamedAmount` (amount.go, `NumDmg$`'s own mechanism), `Defined$`
through `definedPlayers` (defined.go, shared with `DealDamage`/`Draw`), and `subAbilityConditionMet` (condition.go)
gating resolution the identical way it now gates `DealDamage`'s and a checkland's `DB$ Tap` --
`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` resolve, `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionOptionalPaid$` still fail loudly by name. Unlike `DealDamage`, `GainLife`
has no `Self` shape at all (a player gains life, never a card). CR 119's own "life gain replacement" family
(`Event$ GainLife`, 21 real replacement lines) has its one directly resolvable real shape now: `gainLifePrevented`
(replacement.go,
["`ReplacementEffect.requirementsCheck` lands"](replacement.md#replacementeffectrequirementscheck-lands),) resolves
sulfuric_vortex.txt's own bare `Prevent$ True` -- the only one of the 21 naming `Prevent$` at all -- checked per player
before `Player.Life` is touched; the other 20 name `ReplaceWith$` instead, a real substitution needing a runtime value
this port cannot read back, not built. `Player.Life` gains directly; `LifeChanged` (`dealPlayerDamage`'s own event for a
life LOSS, combatdamage.go) is emitted with a positive `Amount` for the gain, reused rather than duplicated.
`SubAbility$` no longer blocks this effect's own resolution either -- removed from its own unresolved-param list once
`resolveSubAbility` ("SubAbility chaining itself lands," further below) landed: 18 of the corpus's own 253 real
SVar-defined `GainLife` lines naming `SubAbility$` chain to an already-built leaf ability and resolve end to end.

**`Mode$ LifeGained` (CR 119.1's own "whenever you gain life" trigger, `TriggerLifeGained.performTest`) is real now
too** -- `checkLifeGainedTriggers` (trigger.go, new), `gainLifeEffect`'s own real (non-test) caller, once per player who
actually gained life. No `ValidCard$` at all, the identical no-object-the-event-happens-to shape `Mode$ Phase`/
`Mode$ Drawn` already have, so it reuses `phaseTriggerZones`'s own four-zone walk outright (95 of 98 real lines name
`TriggerZones$ Battlefield`, 2 `Graveyard`, 1 `Command` -- the identical minority-but-real split every earlier reuser
already justified) and `matchesPlayerSpec` for `ValidPlayer$` (present on every real line -- `You`, 95; `Opponent`, 2;
one qualified `Player.Opponent+Active+controlsArtifact.named<card>` this port cannot resolve), matched against the
gaining player rather than the host's own controller, the identical "compare two `PlayerID`s, whichever they represent"
indifference `Phase`'s own `ValidPlayer$` already relies on. Since `ValidPlayer$` is the mode's only real dispatch key
(there is no `ValidCard$` to fall back to), its absence is treated as no match rather than unrestricted --
`checkAttacksTriggers`'s own contract for `ValidCard$`, applied here to `ValidPlayer$` instead, since 0 real lines omit
it. `FirstTime$` (6) resolves too -- Java's own `AbilityKey.FirstTime` (`lifeGainedTimesThisTurn == 0`, computed in
`Player.gainLife` BEFORE the counter itself increments), a new pre-increment read of a new
`Player.LifeGainedTimesThisTurn` (player.go) -- the identical pre-increment-count contract `checkLandPlayedTriggers`'s
own `NotFirstLand$` already has ([`## Mode$ LandPlayed lands`](trigger-modes.md#mode-landplayed-lands)), a count of GAIN
EVENTS rather than the amount gained (Java's own separate `lifeGainedThisTurn` field, not tracked here since no real
corpus line needs it), incremented once per `gainLifeEffect` resolution -- the only life-gain call site this port has,
so no other site needs to touch it. `ActivationLimit$` (4) now skips the whole line too -- a real correctness fix
(PORT-8/GO-7), not a new resolution: this port never checked the key at all before, so those 4 real lines were firing
every single time they could rather than up to their own per-turn/per-game cap, a wrong answer this port had, not a
coverage gap it was honest about -- the identical unbuilt per-trigger-activation counter `checkBecomesTargetTriggers`'s
own doc comment already names for `ActivationLimit$` there. 93 of the corpus's own 98 real lines resolve now
(`OptionalDecider$`, 7, every real line "You", resolves too through `triggerEffectAPI`'s own `triggerIsOptional`,
"`CR 603.3d's own "may" triggered ability`," below). Not resolved, skipped via `hasAnyParam`: `ValidSource$`/`Spell$` (1
line, both named together) -- matched against the triggering `SpellAbility` itself, the identical ability-kind
classifier `becomesTargetSourceMatches`
([`## Mode$ BecomesTarget lands`](trigger-modes.md#mode-becomestarget-lands-and-targeting-gets-a-second-real-event)) has
for a different mode, not built here since this one real line stays blocked by `Spell$` regardless of whether
`ValidSource$` resolves; `ResolvedLimit$` (1) -- `Trigger.getResolvedThisTurn`'s own separate per-trigger resolution
counter, a different mechanic from `ActivationLimit$`'s own per-trigger activation counter.

14 tests (`gainlifeeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `gainLifeEffect` itself being unexported (TEST-1) -- the identical set `dealdamageeffect_test.go` has minus
the Deathtouch/`FlagCombat` pair (neither applies to a player-only effect), plus
`TestGainLifeEffectFiresLifeGainedTrigger` (a separate watcher's own `Mode$ LifeGained` fires and its `Execute$ Draw`
resolves) and `TestGainLifeEffectSkipsLifeGainedTriggerForOpponent` (the negative control -- `ValidPlayer$ You` never
matches an opponent's own gain), plus three more proving the new work:
`TestGainLifeEffectFiresLifeGainedTriggerFirstTimeOnFirstGain`/
`TestGainLifeEffectSkipsLifeGainedTriggerFirstTimeOnSecondGain` prove `FirstTime$` across two separate gains the same
turn, and `TestGainLifeEffectSkipsLifeGainedTriggerWithActivationLimit` proves the `ActivationLimit$` correctness fix.
Both new gates were regression-checked by temporarily disabling them and confirming the corresponding test failed with
the expected wrong library-card zone before restoring them. `NewRegistry` (`castspell.go`) registers `APIGainLife`; new
`enginelint` group `gainlifeeffect` (above
`card`/`game`/`player`/`ability`/`event`/`trigger`/`defined`/`amount`/`condition` -- `gainlifeeffect` is
`dealdamageeffect`'s own first sibling to call `trigger.go` directly rather than through `combatdamage.go`'s own
intermediary, since no combat-style damage-marking layer exists for life to route through), `castspell` gaining it as a
dependency to register into.

---

## M6's fifth effect: Pump, and duration tracking

`Pump` (CR 611, `PumpEffect.java`) is the corpus's own single largest script-driven effect by real line count after
`ChangeZone`/`Draw` -- 4,103 real `(AB|DB)$ Pump` lines. Java's own `resolve` reads two dozen params across a targeted
grant, a `Defined$` grant, `Radiance$`'s own fan-out, `SharedKeywordsZone$`'s own zone scan, `DefinedKW$`'s own
placeholder substitution and more; this port scopes to `Defined$ Self`/`Enchanted`/`Equipped` -- no real target, 1,335
of the 4,103 real lines -- and within that, 1,147 resolve. A new `definedCards` (defined.go, `definedPlayers`'s own
sibling) resolves `Self` to the ability's own host card directly and `Enchanted`/`Equipped` to what the host is
currently attached to (`Card.AttachedTo`, an Aura or Equipment's own reference to its host) -- `pumpEffect`'s own first
caller, matching Java's `AbilityUtils.getDefinedCards` for exactly these two shapes and no others. `NumAtt$`/`NumDef$`
resolve through `resolveNamedAmount` (`NumDmg$`'s own mechanism), `KW$` through `keywordTokens` (continuous.go,
`AddKeyword$`'s own " & "-separated token split and dynamic-marker rejection, reused rather than duplicated -- a `KW$`
naming a marker like `ChosenType` with no real corpus line combining that with a resolved shape here anyway), and
`PumpZone$` through a new `pumpZoneMatches` (absent means Battlefield alone, `ZoneType.listValueOf`'s own Java default;
present, a comma list checked via `hasZone`, trigger.go's own `TriggerZones$` mechanism reused for a resolving ability
instead of a trigger). `subAbilityConditionMet` (condition.go) gates resolution the identical way it gates
`DealDamage`'s/`GainLife`'s.

**This is the first script-driven effect whose own contribution outlives its `Resolve` call.** Every prior effect
(`Draw`, `DealDamage`, `GainLife`) changes game state once and is done; a `Pump` with no `Duration$` (1,282 of the 1,335
real `Defined$ Self`/`Enchanted`/`Equipped` lines) has to keep applying until end of turn, and this port had never
needed a duration-scoped continuous effect before -- `applyContinuousPT`'s own doc comment used to name this as the one
thing keeping its own blanket per-pass rebuild correct only by accident: nothing yet ever added a `PTEffect` outside a
`Mode$ Continuous` static line. A new `Game.pumps` ledger (`pumpRecord`, game.go -- `Card`, a `Timestamp`,
`Power`/`Toughness`, `Keywords`, `Permanent`) records each resolved `Pump`'s own contribution there instead.
`applyPumpEffects` (continuous.go, new) re-adds every record into its target's own `PT`/`KeywordMod` every
`CheckStateBasedActions` pass, called right after `applyContinuousPT`/`applyContinuousKeyword` so their own per-pass
`Clear()` has already emptied every battlefield card's effects for this pass -- the identical "recompute fresh every
pass" contract a `Mode$ Continuous` static already has, just fed from `Game.pumps` instead of a card's own script.
`cleanupStep` (turn.go) drops every non-`Permanent` record at end of turn -- CR 514.2's own "until end of turn" effects
wearing off, the half of 514.2 its own doc comment used to name as unbuilt. `Duration$ Permanent` (19 of 1,335) marks a
record that survives cleanup; every other real value (`Perpetual`, 12; `UntilEndOfCombat`, 4; `UntilYourNextUpkeep`, 1)
is not resolved, each its own further expiry hook this port has no equivalent of.

A record also has to stop applying the moment its own target leaves the battlefield, before cleanup ever runs --
`Game.Move`'s existing `PT.Clear()`/`KeywordMod.Clear()` branch (the same one `Counters`/`Damage`/`Tapped` already reset
there) now calls a new `clearPumps` (game.go) too, dropping every record naming that card. Without it, `CardID` being
stable across zone changes here (ADR-0009) would let a `Permanent` record -- or even a same-turn `Duration$`-less one --
silently survive a trip to the graveyard and reapply the moment a Raise Dead-style effect returned the same `CardID` to
the battlefield: Java's own `applyPump` guards against exactly this with a per-instance game-timestamp check
(`applyTo.equalsWithGameTimestamp(gameCard)`) this port has no equivalent of, so dropping the record on exit gets the
same real-world answer without one.

`SubAbility$` (119 of 1,335) no longer blocks -- removed from `pumpUnresolvedParams` once `resolveSubAbility`
(["SubAbility chaining itself lands"](targeting-and-chaining.md#subability-chaining-itself-lands),) landed: 17 of the
corpus's own 571 real SVar-defined `Pump` lines naming `SubAbility$` chain to an already-built leaf ability and resolve
end to end. `UnlessCost$`/`UnlessPayer$`/ `UnlessSwitched$` no longer block either, removed once `resolveUnlessCost`
("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed: 2 of the corpus's own 15 real `Pump` lines naming
`UnlessCost$` resolve past that gate and are actually reachable at all -- spitting_slug.txt's own real "gains first
strike... unless you pay {1}{G}," chaining `UnlessResolveSubs$ WhenNotPaid` into `PumpAll` when the cost goes unpaid,
and, once `ActivateAbility` landed too
([`## Activating an ability lands`](activation.md#activating-an-ability-lands-cr-6022)), nakaya_shade.txt's own real
activated `{B}:` ability itself gated by its own nested "unless any player pays {2}" -- the two gates compose with no
special-casing needed, since `ActivateAbility` threads the ability's own compiled `Params` (`UnlessCost$` included)
through unchanged and `Registry.Resolve` reads `UnlessCost$` off whatever pushed the ability, activation included. The
other 13 name a non-pure-mana cost, an unresolvable `UnlessPayer$`, or a top-level spell on an Instant (`CastSpell` does
not cast an instant or sorcery at all), pushing the `Defined$`-shape count from 1,147 to 1,148 of 1,335
(nakaya_shade.txt's own line already counted in that 1,148 once its own outer activation and inner `UnlessCost$` both
resolve).

Not resolved, each failing loudly by name rather than guessing (PORT-8/GO-7): `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$`/`ConditionActivationLimit$` (0/19/0/4) --
`SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does not cover, the identical
`DealDamage`/`GainLife`-shaped gap; `PlayerTurn$` (2) -- unclear semantics on a `Pump` line, not worth guessing at from
two real lines; `NumAtt$`/`NumDef$` naming the literal `Double`/`Triple` (1 combined) -- the target's own power or
toughness doubled or tripled, a hardcoded special case rather than a named SVar `resolveNamedAmount` could resolve; a
`KW$` token starting with `HIDDEN` (22) -- a hidden-keyword phrase (`gameCard.addHiddenExtrinsicKeywords`), its own
separate mechanic; `CanBlockAmount$`/`CanBlockAny$` (4/0) -- an additional-blocker grant this port's own block-legality
gate (staticability.go) has nowhere to consult a one-shot record from; `DefinedKW$`/`KWChoice$`/`RandomKeyword$` (3/3/1)
-- a placeholder substitution, an interactive choice and a random draw, none of which this port's own `KW$` handling
does; `SharedKeywordsZone$`/`SharedRestrictions$` (2/2) -- `CardFactoryUtil.sharedKeywords`'s own zone scan;
`ValidTgts$` (1) -- a real target past the `Defined$` card this effect already resolves; `AtEOT$` (9) --
`registerDelayedTrigger`, a new trigger this effect would silently fail to create; `ImprintCards$` (1) and
`DefinedLandwalk$`/`ForgetObjects$`/`RememberObjects$`/`RememberPumped$`/`LeaveBattlefield$`/`ForgetImprinted$`/
`NoteCards$`/`NoteCardsFor$`/`ClearNotedCardsFor$`/`NoteNumber$` (0 each in this scope) -- each its own further tracking
mechanic; `IsPresent$` (6) -- unclear semantics on a resolving (not triggering) `Pump` line, skipped rather than assumed
harmless; `Optional$`/`OptionQuestion$` (0/0) -- a "may" confirmation this port's own `PlayerController` has no hook
for; `Radiance$` (0) -- `CardUtil.getRadiance`'s own "and everything else that shares a color" fan-out.

11 new tests (`pumpeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `pumpEffect` itself being unexported (TEST-1): `Defined$ Self` granting power/toughness and a keyword,
`Duration$`'s default wearing off at cleanup and `Permanent` surviving it, `SubAbility$`/`Double`/a `HIDDEN` keyword
each rejected loudly, `ConditionCheckSVar$`'s own met/unmet pair, `PumpZone$` rejecting a target not in the zone it
names, and an Aura's own `Defined$ Enchanted` shape end to end -- cast, attach, and the enchanted creature (not the Aura
itself) gets the boost. `NewRegistry` (`castspell.go`) registers `APIPump`; new `enginelint` group `pumpeffect` (above
`id`/`card`/`game`/`ability`/`defined`/`amount`/`condition`/`trigger`/`continuous`/`zone` -- the first effect group
needing `continuous` directly, to call `keywordTokens`, and `zone`, for `Battlefield`/`ZoneType` in `pumpZoneMatches`),
`castspell` gaining it as a dependency to register into.

---

## M6's sixth effect: PumpAll, the blanket sibling

`PumpAll` (CR 611, `PumpAllEffect.java`) is `Pump`'s own blanket counterpart: a `ValidCards$`-matched set rather than a
single `Defined$`/targeted card. 833 real `(AB|DB)$ PumpAll` lines, and unlike `Pump` itself, the dominant real shape
carries no `Defined$` and no target at all -- 818 of the 833 -- Java's own
`!sa.usesTargeting() && !sa.hasParam("Defined")` branch, `game.getCardsIn(affectedZones)` filtered by `ValidCards$` with
nothing narrowing it to specific players first. That is this port's entire real scope, since neither targeting nor most
`Defined$` shapes exist; within it, plus the 3 real `Defined$ You`/`Player.Opponent` lines `definedPlayers` already
covers, 642 of 833 resolve -- a much higher hit rate than `Pump`'s own 1,147 of 4,103, since a blanket `ValidCards$`
match needs no card-reference resolver at all, only the same `Matches` evaluator `applyOneContinuousPT`'s own
`Affected$` already reuses for the identical "every battlefield permanent this valid string matches" shape
(`## Continuous effects: Layer 7's own PT`, above).

`pumpAllEffect` (`pumpalleffect.go`, new) reuses `Pump`'s own machinery outright rather than duplicating it:
`pumpAmount`/`pumpKeywords` (pumpeffect.go) both gained an `effect string` parameter once `PumpAll` became their second
caller, naming the effect (`Pump`/`PumpAll`) in their own error text instead of hardcoding it; `Game.pumps`/
`applyPumpEffects`/`cleanupStep`'s own duration tracking
([`## M6's fifth effect: Pump, and duration tracking`](#m6s-fifth-effect-pump-and-duration-tracking), above) needs no
changes at all, since a `pumpRecord` never itself distinguishes which effect created it -- `PumpAll`'s own resolved
lines just append more of them. `PumpZone$`'s own meaning shifts from a single-target zone check (`pumpZoneMatches`,
`Pump`'s own reader) to the list of zones actually scanned: a new `pumpAllZones` parses the same comma list
(`ZoneByName`, zone.go) into `[]ZoneType`, defaulting to `Battlefield` alone -- `ZoneType.listValueOf`'s own Java
default -- rather than checking one card's membership in it. `Defined$ You`/`Player.Opponent` (3 real lines) resolve
through `definedPlayers` (defined.go) to narrow the player set scanned before `ValidCards$` filters each of their own
zones, rather than scanning every player unconditionally.

`SubAbility$` (90 of 833) no longer blocks -- removed from `pumpAllUnresolvedParams` once `resolveSubAbility`
(["SubAbility chaining itself lands"](targeting-and-chaining.md#subability-chaining-itself-lands),) landed: 6 of the
corpus's own 75 real SVar-defined `PumpAll` lines naming `SubAbility$` chain to an already-built leaf ability and
resolve end to end. `UnlessCost$`/`UnlessPayer$` no longer block either, removed once `resolveUnlessCost`
(["`Registry.Resolve`'s own `UnlessCost$` gate"](activation.md#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),)
landed: 0 of the corpus's own 3 real `PumpAll` lines naming `UnlessCost$` resolve, though -- rhystic_shield.txt's own
real "get +0/+2... unless any player pays {2}" is the only one clearing that gate's own pure-mana-cost/resolvable-payer
filter, and it is a top-level `A:SP$ PumpAll` line on an Instant, which `CastSpell` cannot cast at all; the other 2 name
a controller- derived `UnlessPayer$` and a non-mana `UnlessCost$` neither resolvable here regardless.

Not resolved, each failing loudly by name rather than guessing (PORT-8/GO-7): `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$`/`ConditionManaSpent$`/`ConditionManaNotSpent$` (4/5/3/1/4/0)
-- `SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does not cover, the identical `Pump`-shaped gap;
`ValidTgts$` (12) -- a real target past the blanket `ValidCards$` match; targeting itself now exists ("Targeting itself
lands," below), `PumpAll` just has not been extended to read `Targeted` back yet; `Planeswalker$`/`Ultimate$` (26/13) --
unclear semantics on a `PumpAll` line, not worth guessing at from either; `RememberPumped$` (8) -- `Card.Memory` has no
writer wired to a blanket multi-card grant; `SharedKeywordsZone$`/`SharedRestrictions$` (4/4) --
`CardFactoryUtil.sharedKeywords`'s own zone scan, a further mechanic; `ModeCost$`/`Exhaust$` (3/4) -- each its own
further activation mechanic.

11 new tests (`pumpalleffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `pumpAllEffect` itself being unexported (TEST-1): a blanket `ValidCards$ Creature.YouCtrl` match pumping every
one of the caster's own creatures including the entering one itself, an opponent's creature correctly excluded, `KW$`
granting a keyword, `Duration$`'s default wearing off at cleanup and `Permanent` surviving it,
`SubAbility$`/`ValidTgts$` each rejected loudly, `Defined$ You` narrowing the scan away from a broader
`ValidCards$ Creature` match that would otherwise also catch the opponent's own creature, `ConditionCheckSVar$`'s own
met/unmet pair, and the default zone scan (Battlefield alone) never reaching a matching card sitting in Hand.
`NewRegistry` (`castspell.go`) registers `APIPumpAll`; `enginelint` group `pumpeffect` gains `pumpalleffect.go`
alongside `pumpeffect.go` (sharing every private helper freely within the one group, `pumpAmount`/`pumpKeywords`
included) and gains `valid` in its own allow-list, for `Matches`/`valid.Parse` -- `pumpAllEffect`'s first real need for
the general valid-string evaluator, `Pump` itself never having needed one since every real `Defined$` shape it resolves
already names an exact card.

---

## M6's seventh effect: LoseLife, GainLife's own mirror image

`LoseLife` (CR 119.3, `LifeLoseEffect.java`) is `GainLife`'s own mirror image -- a plain-or-named-SVar `LifeAmount$`
subtracted from a `Defined$` player rather than added. 445 real `(AB|DB)$ LoseLife` lines name
`Defined$ You`/`Opponent`/`Player.Opponent`, and 226 of those carry no other unresolved param and resolve (a later
chunk, ["Targeting itself lands"](targeting-and-chaining.md#targeting-itself-lands),, extends this to 300 by also
reading a targeted player directly). `loseLifeEffect` (`loselifeeffect.go`, new) reuses `gainLifeEffect`'s own machinery
outright: `resolveNamedAmount` for `LifeAmount$`, `definedPlayers` for `Defined$`, `subAbilityConditionMet` gating
resolution the identical way -- the two files are close enough to be near-mirrors of each other, `-=` and a negated
`Amount` the only real difference in the resolve path itself.

Unlike `GainLife`, this calls no trigger check at all. Java's own `Player.loseLife` fires `TriggerType.LifeLost`
directly, and `LifeLoseEffect.resolve` separately fires `TriggerType.LifeLostAll` once more over every player who
actually lost life -- two trigger points where `GainLife` has one (`TriggerLifeGained`, `checkLifeGainedTriggers`'s own
real caller). But `T:Mode$ LifeLost` and `T:Mode$ LifeLostAll` both carry 0 real lines corpus-wide (a `grep` against the
whole cardsfolder for either, case-exact), against `Mode$ LifeGained`'s own 98 -- there is nothing for a
`checkLifeLostTriggers` to ever match, so none is built. The identical `LifeChanged` event `dealPlayerDamage`/
`gainLifeEffect` already emit is reused here too, with a negative `Amount` for the loss -- the third real emitter after
those two, still the one event both directions of a life total change ever fire.

Two gaps `GainLife`'s own paragraph (above) already names apply here unchanged, not newly discovered: Java's own
`Player.gainLife`/`loseLife` are both gated by a `canGainLife`/`canLoseLife` check (`StaticAbilityCantGainLosePayLife`,
an S: line this port's own static-ability engine has no reader for), and CR 119's own life-total replacement family
exists for a loss too (`ReplacementType.LifeReduced`, `GainLife`'s own `ReplacementType.GainLife` counterpart) --
neither built, the identical real-gap-not-a-wrong-answer every other unbuilt replacement remainder already is.

Not resolved, each failing loudly by name rather than draining the wrong amount from the wrong player (PORT-8/GO-7):
`Condition$` itself and `ConditionDefined$`/`ConditionZone$` (0/14/1) -- `SpellAbilityCondition`'s own shapes
`subAbilityConditionMet` does not cover, the identical `GainLife`-shaped gap; `Planeswalker$` (6, counted across the
wider 823-line real `Defined$` set) -- each its own further mechanic;
`Ultimate$`/`IsPresent$`/`PresentCompare$`/`NumCards$`/`ModeCost$` (1/2/2/2/1) -- unclear semantics on a `LoseLife`
line, not worth guessing at from a handful of real lines. `ValidTgts$` (163 of the 823) no longer blocks -- "Targeting
itself lands," below, is why. `SubAbility$` (210 of 445) no longer blocks either -- "SubAbility chaining itself lands,"
further below, removed it from this file's own unresolved-param list: Sphinx Sovereign's own real "gain 3 life if
untapped, otherwise each opponent loses 3" (one `DB$ LoseLife` with a `SubAbility$ DB$ GainLife`, the negated condition
split across the two) is exactly why that chain has to run regardless of whether `subAbilityConditionMet` let this
effect's own body run. 144 of the corpus's own 382 real SVar-defined `LoseLife` lines naming `SubAbility$` chain to an
already-built leaf ability and resolve end to end. `UnlessPayer$`/`UnlessCost$`/`UnlessSwitched$` no longer block
either, removed once `resolveUnlessCost`
(["`Registry.Resolve`'s own `UnlessCost$` gate"](activation.md#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),)
landed: 0 of the corpus's own 42 real `LoseLife` lines naming `UnlessCost$` resolve, though -- delaying_shield.txt's own
real pure-mana "{1}{W}" is the only one clearing that gate's own pure-mana-cost/resolvable-payer filter, and it is
reached only through `DB$ Repeat`'s own `RepeatSubAbility$` (not the plain `SubAbility$` chaining `resolveSubAbility`
reads), a general repeat-N-times mechanic this port does not build; every other real line names a
`Sac<.../Discard<.../PayLife<.../...` cost part or a controller-derived `UnlessPayer$` this port cannot resolve.

9 new tests (`loselifeeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `loseLifeEffect` itself being unexported (TEST-1) -- `gainLifeEffect_test.go`'s own set minus the two
`Mode$ LifeGained`-firing tests neither this effect nor any trigger mode here has an equivalent of. `NewRegistry`
(`castspell.go`) registers `APILoseLife`; new `enginelint` group `loselifeeffect` (`id`/`card`/`game`/
`player`/`ability`/`event`/`defined`/`amount`/`condition` -- no `trigger`, the one dependency `gainlifeeffect`'s own
allow-list has that this group does not, since nothing here ever calls one), `castspell` gaining it as a dependency to
register into.
