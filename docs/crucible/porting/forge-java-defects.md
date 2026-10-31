# Forge Java Defects

- **Status:** Active

Bugs in upstream Forge's Java that the port found: a method that cannot do what its name says, an unguarded index, a
loop that never ends. PORT-8: reported and fixed upstream, never compensated in Go. Reason: a workaround makes Crucible
disagree with the oracle for a reason no diff can explain. Card-script bugs go in
[card-script-defects.md](card-script-defects.md) instead.

A row lands in the same commit as the port that found it. What Crucible does meanwhile is fail closed — an `error` or a
rejected param — never a silent fix. "Upstream" tracks the PR filed against
[Card-Forge/forge](https://github.com/Card-Forge/forge); update it in place once one exists (DOC-16).

Rows start with the ChooseSource/Empower batch. Bugs noted before it are only in their own port-log sections.

The "Upstream" cell says `open` or `merged` for each pull request as of upstream/master `9748136a531`. A `Not filed` row's site was re-read there: it still holds, only its line moved.

## Status

| Site                                                                   | Defect                                                                                                                                                                                                                                                                                                                                                                             | Crucible meanwhile                                                                                             | Upstream                                                                                                                                                |
| ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `SpellAbilityEffect.java:797`                                          | `addToCombat`'s blocking half: `getDefenderPlayerByAttacker` returns null for an attacker no longer in combat (killed in response to Brimaz's trigger) and `.equals` throws                                                                                                                                                                                                        | `addToCombatBlocking` skips the block (CR 506.4)                                                               | [#12376](https://github.com/Card-Forge/forge/pull/12376) open                                                                                           |
| `CombatUtil.java:70`                                                   | `getAllPossibleDefenders`: `battle.getProtectingPlayer().isOpponentOf` throws for a battle with no protector                                                                                                                                                                                                                                                                       | Such a battle is not offered as an attack target                                                               | Not pursued: latent (left out of #12376)                                                                                                                |
| `Player.java:3435`                                                     | `getMonarchSet` ternary condition inverted                                                                                                                                                                                                                                                                                                                                         | No counterpart: no set codes in Crucible                                                                       | Fixed upstream (`8cd226407b1`)                                                                                                                          |
| `AlterAttributeEffect.java:87`                                         | Unpreparing is only a leaves-play command: a creature prepared as it enters whose entry is then replaced (exiled by Containment Priest) never left the battlefield, so its prepared copy stays castable by the card's owner                                                                                                                                                        | Reproduced (oracle parity, `replacemententry.go`)                                                              | Not filed                                                                                                                                               |
| `CardUtil.java:345`                                                    | Recursive frame resolves `Valid$` against the reflecting host                                                                                                                                                                                                                                                                                                                      | None: `ManaReflected` deferred                                                                                 | Not filed                                                                                                                                               |
| `FlipOntoBattlefieldEffect.java:109`                                   | Neighbor filter re-tests the landing spot instead of the candidate; "always true" only for a non-Aura-enchantment spot                                                                                                                                                                                                                                                             | `flipCandidates` rejects that one shape with an `error`                                                        | Not filed                                                                                                                                               |
| `PlayEffect.java:312`, `:389`                                          | `continue` without `amount--` under `AllowRepeats$` re-offers the same unplayable card forever                                                                                                                                                                                                                                                                                     | `playRepeatLoop` returns an `error`                                                                            | Not pursued: [#12373](https://github.com/Card-Forge/forge/pull/12373) closed, latent and AI-only                                                        |
| `TargetRestrictions.java:526-528`                                      | `hasCandidates` returns true for a stack zone without testing `ValidTgts$` or `canTarget`: an Animate or Airbend that targets only a spell is offered with none to choose                                                                                                                                                                                                          | `targeting.go` refuses a `TgtZone$` that names the stack                                                       | not filed                                                                                                                                               |
| `StaticAbilityCantAttackBlock.java:269`                                | `cantBlockBy(attacker, null)` always false, so `CombatUtil.canBeBlocked`'s unblockable check (`CombatUtil.java:533`) never fires                                                                                                                                                                                                                                                   | Check not ported; no validator outcome depends on it                                                           | Not filed                                                                                                                                               |
| `Player.java:2642`                                                     | `planeswalk`'s `getZone(PlanarDeck).get(0)` unguarded; throws on an empty planar deck, after every plane has already left                                                                                                                                                                                                                                                          | `error` before anything moves                                                                                  | [#12376](https://github.com/Card-Forge/forge/pull/12376) open                                                                                           |
| `TriggerChaosEnsues.java:43-48`                                        | Suspected: `Affected` iterable must be all host, so chaos ensuing for two planes fires neither                                                                                                                                                                                                                                                                                     | `Defined$` naming two planes returns an `error`                                                                | Not filed                                                                                                                                               |
| `RunChaosEffect.java:25-31`                                            | `setOptionalTrigger(true)` hits the RunChaos ability, not the copy; decider never null, so every copy asks `confirmTrigger`                                                                                                                                                                                                                                                        | `OptionalDecider$`/`Cost$` chaos trigger: `error`                                                              | Not filed                                                                                                                                               |
| `GameAction.java:650-653`                                              | `unmeldPosition` computed, never used: the melded card goes to the primary's own `position`, so the owner never orders the two (CR 401.4)                                                                                                                                                                                                                                          | Reproduced (oracle parity): secondary over primary on library top                                              | Not filed                                                                                                                                               |
| `AbilityUtils.java:2186`                                               | `Count$Presence_<Type>` counts controlled permanents of the type as the amount is read (`Count$LastStateBattlefield`), not "as you cast this spell"                                                                                                                                                                                                                                | Reproduced (oracle parity): `presenceCount` reads the battlefield then                                         | Not filed                                                                                                                                               |
| `PlaySpellAbility.java:766-782`                                        | Suspected: `announceValuesLikeX` reads the raw `getPayCosts()`, so an X named only by a `Mode$ RaiseCost` part (Crashing Wave's `Waterbend<X>` with no `Announce$`) is never asked and reads as 0                                                                                                                                                                                  | Reproduced (oracle parity) once R25h lands; not ported                                                         | Not filed                                                                                                                                               |
| `ChangeZoneEffect.java:961-963`, `:1014-1016`                          | `OriginAlternative$` with the library declined leaves `origin = [Graveyard]`; with no `DefinedPlayer$` the pool becomes every player's graveyard (`game.getCardsIn(origin)`), so a `ChangeType$` without `YouOwn` takes an opponent's card: Captain Kirk, Boldly Going (`Land.Basic`), Kassandra, Eagle Bearer, Shaun & Rebecca, Agents, Auditore Ambush, against "your graveyard" | Reproduced (oracle parity)                                                                                     | Not filed                                                                                                                                               |
| `ChangeZoneEffect.java:532`                                            | Known-origin `Chooser$`: `getDefinedPlayers(...).get(0)` unguarded, throws on an empty list. The corpus's known-origin lines name `You` (30), `Targeted` (21), `ChosenPlayer` (5), `Player.IsRemembered` (3), `Opponent` (3) and a few more; the non-`You` ones can resolve to an empty list if that player left the game or nothing was remembered                                | `error` before anything moves                                                                                  | [#12376](https://github.com/Card-Forge/forge/pull/12376) open                                                                                           |
| `PlayerControllerHuman.java:374-381`                                   | Suspected: the non-mana X cap (`getMaxForNonManaX`) reads the raw cost too, so Aether Tide's `Discard<X/Creature>` from `RaiseCost` is not capped and the discard fails at payment after targets are chosen                                                                                                                                                                        | Reproduced (oracle parity) once R25h lands; not ported                                                         | Not filed                                                                                                                                               |
| `SpellAbility.java:2153-2161`                                          | `TargetingPlayer$` naming nobody: `chooseSingleEntityForEffect` returns null for the empty list (`PlayerControllerHuman.java:585-590`) and `targetingPlayer.getController()` throws a NullPointerException                                                                                                                                                                         | An `error`                                                                                                     | [#12377](https://github.com/Card-Forge/forge/pull/12377) open                                                                                           |
| `GameAction.java:2400`                                                 | `numCardsInHandStartedThisTurnWith` is set after the game loop, not at the turn's start                                                                                                                                                                                                                                                                                            | See `port-log/game-state/m6-r9d-cast-and-turn-records.md`                                                      | Not filed                                                                                                                                               |
| `DamageDealEffect.java:248`, `DamagePreventEffect.java:105`            | A loop variable leaks into the per-target amount of a divided damage or prevention                                                                                                                                                                                                                                                                                                 | See `port-log/game-state/m6-r9-dealdamage-divided.md`                                                          | Not filed                                                                                                                                               |
| `AbilityUtils.java:1129-1130`                                          | `Defined$ DefendingPlayer` reads `getCombat()` unguarded, so it throws outside combat                                                                                                                                                                                                                                                                                              | An `error`; no corpus line reaches it                                                                          | Not pursued: latent (left out of #12376)                                                                                                                |
| `SpellAbilityCondition.java:420-424`                                   | A `ConditionLifeAmount` SVar other than `X` throws                                                                                                                                                                                                                                                                                                                                 | An `error`; no corpus line reaches it                                                                          | Issue first: what a missing or non-literal param means is the authors' call                                                                             |
| `mana_clash.txt:11`, `sword_of_dungeons_dragons.txt:12`                | `SVar:RepeatCheck:Number$ 0` / `Number$ 1`: the space makes xCount's `Integer.parseInt` (`AbilityUtils.java:1585-1587`) throw if read. Latent: `RepeatEffect.java:39-52` reads it only after the repeated chain's `StoreSVar` rewrote it                                                                                                                                           | Read after the first repetition as Java; the script value is never read                                        | Not filed                                                                                                                                               |
| `TextBoxExchangeEffect.java:74-76`                                     | Removes the exchanged traits when an `AsLongAsInPlay` exchange ends, but leaves the two `addEmpty` clear rows `Card.java:4895-4896` wrote, so word changes made before the exchange stay wiped after it (the source comment is a question)                                                                                                                                         | The ending exchange drops its own rows, so earlier word changes return: differs from Java on this one sequence | Not filed                                                                                                                                               |
| `AbilityUtils.java:3142`                                               | Text-change regex built from the unescaped word with no right boundary: `Red` also rewrites the front of `Reduce`                                                                                                                                                                                                                                                                  | Reproduced (PORT-7), parity depends on it                                                                      | Not filed                                                                                                                                               |
| `SacrificeAllEffect.java:66`                                           | `MayFlashSac` sacrifices by card id (`GameEntity.equals` is id only), so a countered and reanimated spell's new object is sacrificed too                                                                                                                                                                                                                                           | Reproduced, not worked around                                                                                  | Not filed                                                                                                                                               |
| `AbilityUtils.java:1584`                                               | `xCount` `Number$` with a non-integer body throws `NumberFormatException`. Latent                                                                                                                                                                                                                                                                                                  | Refused                                                                                                        | Not filed                                                                                                                                               |
| `TriggerDamageDoneOnce.java:67-70`                                     | `AttackingPlayer` is the first source's controller of a `HashMap`: undetermined when one entity was hit by several controllers' sources in one damage table                                                                                                                                                                                                                        | Refused, not reproduced                                                                                        | Not filed                                                                                                                                               |
| `TriggerRolledDie.java:57-60`                                          | `Number$` branch unboxes a missing `AbilityKey.Number` and throws on a planar roll (`PlanarDice.java:81,87` runs `RolledDie` with `Result` 0, no `Number`). Latent: 0 corpus lines                                                                                                                                                                                                 | Planar die roll refused before the roll                                                                        | Not filed                                                                                                                                               |
| `ChangeTargetsEffect.java:52`                                          | `Chooser$`: `.get(0)` on an empty player list throws, even with no spell to retarget. Latent                                                                                                                                                                                                                                                                                       | `Chooser$` refused                                                                                             | [#12377](https://github.com/Card-Forge/forge/pull/12377) open                                                                                           |
| `ChangeTargetsEffect.java:71-106`, `:152-163`                          | `TargetRestriction$` is read only on the default branch; `ChangeSingleTarget$` and `DefinedMagnet$` ignore it. Latent: 6 lines carry it, none on those branches                                                                                                                                                                                                                    | Go ignores it on the same branches, as Java                                                                    | Not filed                                                                                                                                               |
| `CopySpellAbilityEffect.java:87`, `:176`, `:195`                       | `copies` is declared per controller and shared across the spell loop, so a CopySpell replacement (Twinning Staff) counts earlier spells' copies in its Amount and extras                                                                                                                                                                                                           | `CopyForEachCanTarget$` refused                                                                                | [#12379](https://github.com/Card-Forge/forge/pull/12379) open                                                                                           |
| `TargetRestrictions.java:53`, `:586-590`                               | `getAllCandidates` scans only the target zone (Battlefield by default) and never adds spells, so Glimmervoid Basin's `CopyForEachCanTarget$ Spell,Permanent,Card,Player` cannot offer a spell or an off-battlefield card                                                                                                                                                           | `CopyForEachCanTarget$` refused                                                                                | Not filed                                                                                                                                               |
| `AiCostDecision.java:55-61`, `:523-564`                                | `visit(CostBehold)` pays one card whatever the cost's amount (Celestial Reunion `Behold<2>`, Kindle the Inner Flame `Behold<3>`); `visit(CostReveal)` falls through for `ChooseCard` (revealFrom All) to a hand-only filter and pays nothing (Close Encounter)                                                                                                                     | Not read: the AI cost decision is not ported                                                                   | Not filed                                                                                                                                               |
| `StaticAbilityDisableTriggers.java:109`                                | `getParam("ValidCause").split(",")` throws for a ChangesZoneAll static without `ValidCause$`. Latent: all 9 corpus lines carry it                                                                                                                                                                                                                                                  | ChangesZoneAll path does not read the static                                                                   | Issue first: what a missing or non-literal param means is the authors' call                                                                             |
| `EncodeEffect.java:57-61`                                              | Cipher exiles the spell card, fires the zone-change triggers, then asks for the creature, where the rule is one action (CR 702.99): exile triggers run before the creature is chosen. Low severity; the pre-checked non-empty choice list makes a null pick unreachable in practice                                                                                                | `Encode` unregistered: Cipher cards refuse                                                                     | Not filed                                                                                                                                               |
| `PlayerControllerAi.java:246-274`                                      | `announceRequirements` returns null for any API but ChooseNumber/BidLife; `PlaySpellAbility.java:746-763` reads null as cancel, so the AI can never cast the March of Reckless Joy cycle (Dig announce)                                                                                                                                                                            | Non-mana `RaiseCost$` refuses the cast                                                                         | Not filed                                                                                                                                               |
| `SetStateEffect.java:31`                                               | `sa.getParam("Mode").equals("Specialize")` throws when Mode is absent (Skittish Valesk's `NewState$` shape); `Card.java:676` then skips the TurnFaceDown guard (`:762-767`)                                                                                                                                                                                                        | `NewState$` and Specialize refused                                                                             | [#12377](https://github.com/Card-Forge/forge/pull/12377) open (`:31` only; `Card:676` is an issue: `NewState$ FaceDown` vs the TurnFaceDown guard)      |
| `Player.java:903-912`                                                  | Player counter removal never fires `CounterRemoved` (the trigger block is commented out under a TODO), so energy loss and rad removal cannot trigger                                                                                                                                                                                                                               | `CounterRemoved` mode not ported                                                                               | Not filed                                                                                                                                               |
| `TriggerManaExpend.java:47`                                            | `Integer.parseInt(mapParams.get("Amount"))` throws on a missing `Amount$`. Latent: all 13 corpus lines have it                                                                                                                                                                                                                                                                     | `ManaExpend` mode not ported                                                                                   | Issue first: what a missing or non-literal param means is the authors' call                                                                             |
| `Card.java:6641`                                                       | `setSaddled` returns true whatever it did, so `AlterAttributeEffect.java:108` runs `BecomesSaddled` for `Activate$ False` and for a repeat saddle. Latent: no corpus line deactivates                                                                                                                                                                                              | Crucible reproduces it (quirks.md)                                                                             | not reported                                                                                                                                            |
| `Game.java:1522`                                                       | `setDayTime` compares two `Boolean`s with `!=` (reference comparison). Latent: current callers autobox                                                                                                                                                                                                                                                                             | `DayTimeChanges` compares values                                                                               | Not filed                                                                                                                                               |
| `Player.java:584-592`, `Card.java:4787-4795`                           | `PayLife` and `Tap` run the replacement but never read back an `Updated` `Amount` or `IsCombat`: the local value is used. Latent: no corpus line sets Updated there                                                                                                                                                                                                                | PayLife and Tap events not hooked                                                                              | Not filed                                                                                                                                               |
| `ConniveEffect.java:73`, `ExploreEffect.java:65`                       | Connive, Explore and Learn (`Player.java:3928`) treat `!= NotReplaced` (which includes `Updated`) as a replacement and skip the whole effect. Latent: no corpus line sets Updated there                                                                                                                                                                                            | The three effects refuse a live replacement for their event                                                    | Not filed                                                                                                                                               |
| `PumpEffect.java:387`                                                  | `NoRepetition$` takes `get(0)` of the targets and throws with none                                                                                                                                                                                                                                                                                                                 | `RandomKeyword$ NoRepetition$` errors with no target                                                           | [#12377](https://github.com/Card-Forge/forge/pull/12377) open                                                                                           |
| `PlayerControllerHuman.java:1864-1871`                                 | `MayEffectFromOpeningHand`: `break` after the first matching SA of a host drops a second one for humans, while the AI keeps both. Latent: one per card                                                                                                                                                                                                                             | Opening-hand effects not ported                                                                                | Not filed                                                                                                                                               |
| `CountersNoteEffect.java:35-47`, `:27-33`                              | `loadCounters` reads every `NoteCounters*` SVar on the Effect card, not only those noted from the one card, and a second noted card overwrites the first's count of a kind; `:44` never clears the SVars, `p` is unused and the table's controller is the activator, not the chooser                                                                                               | `NoteCounterDefined$` and `WithNotedCounters$` refused (Tawnos's Coffin)                                       | Not filed                                                                                                                                               |
| `CardFactoryUtil.java:1134-1150`                                       | The permanent gift trigger's description is built at load from `getPromisedGift()`, so it reads "Gift a a card to null" (display only; `GiveGift` does fire for a permanent, the trigger hands the Gift keyword to its ability)                                                                                                                                                    | Not modelled (display text)                                                                                    | Not filing: display only                                                                                                                                |
| `GameActionUtil.java:633-638`                                          | Casualty X: `Aggregates.max` of no creatures is `Integer.MIN_VALUE`, handed to `chooseNumber` as the maximum                                                                                                                                                                                                                                                                       | Casualty X refused (Ob Nixilis, the Adversary)                                                                 | Not filed                                                                                                                                               |
| `PlayerControllerHuman.java:378-381`                                   | X of a spell is bounded by `getMaxForNonManaX` over the raw cost, before RaiseCost: Aether Tide's `Discard<X/Creature>` and Tectonic Split's `Sac<X/Land>` announce an X no hand or board can pay. `Cost.java:985-986`: `Amount$ 2` over a part with X appends two X parts (2X)                                                                                                    | RaiseCost with a part past mana refused                                                                        | Not filed                                                                                                                                               |
| `AbilityUtils.java:3758-3760`, `StaticAbilityTapPowerValue.java:47-52` | `TapPowerValue` returns without `doXMath` (a math suffix is dropped), and its `Value$` is `parseInt`ed unguarded; `StaticAbilityNumLoyaltyAct` tests `Twice` by presence, so `Twice$ False` enables it. No corpus line hits any of the three                                                                                                                                       | None needed                                                                                                    | Not filed                                                                                                                                               |
| `PlayEffect.java:493-494`                                              | `amount--` and `totalCMCLimit -= tgtCMC` run after `playSaFromPlayEffect`, so a declined or failed play still spends the Amount$ and the WithTotalCMC$ budget; the other failure paths `continue` before the decrement                                                                                                                                                             | `WithTotalCMC$` refused (4 cards)                                                                              | Not filed: unverified; [#12373](https://github.com/Card-Forge/forge/pull/12373) (closed) was the opposite case                                          |
| `CardTraitBase.java:472` / `AbilityUtils.java:1013`                    | A trigger's `CheckSVar$` passes the `Trigger` as context, but the `Triggered*` branch of `getDefinedPlayers` needs a `SpellAbility`: `TriggeredAttackedTargetAndYou` falls through to every player (Undercellar Sweep)                                                                                                                                                             | `CheckSVar$` refused                                                                                           | Not filed: unverified, a three-player Java test did not fire the trigger at all                                                                         |
| `AttachEffect.java:84-86`                                              | `PlayerChoices$` removes from `choices` while iterating it in a for-each; `FCollection.iterator()` is the backing list's, so it throws when a listed player cannot be enchanted (3 corpus lines; derived from the Java, not run)                                                                                                                                                   | `PlayerChoices$` with `Object$` refused                                                                        | Not a defect on upstream/master: `FCollection` iterates a snapshot                                                                                      |
| `ManaPool.java:153-158`                                                | A unit that is both persistent and combat mana is added to the kept list twice and re-added twice after the removal (no corpus card has both flags)                                                                                                                                                                                                                                | Combat mana kept once                                                                                          | Not filed                                                                                                                                               |
| `RollDiceEffect.java:492`, `:443`                                      | `Collections.max(rolls)` throws on an empty list (`IgnoreLower$` >= `Amount$` with `UseDifferenceBetweenRolls$`); `naturalRolls.get(i)` throws when more dice are ignored than rolled (no corpus line)                                                                                                                                                                             | Not reached                                                                                                    | Not filed                                                                                                                                               |
| `ProtectEffect.java:111`                                               | `Choser$` is read only when it equals `Controller`; any other value silently leaves the activator choosing (no corpus line)                                                                                                                                                                                                                                                        | `Choser$ Controller` resolved, others choose the activator                                                     | Not filed                                                                                                                                               |
| `AddTurnEffect.java:56`                                                | `SkipUntap$` is tested with `hasParam`, so `SkipUntap$ False` still skips the untap step (no corpus line)                                                                                                                                                                                                                                                                          | Resolved as Java                                                                                               | Not filed                                                                                                                                               |
| `StoreSVarEffect.java:22-24`, `:50`                                    | `SVar$ EachPlayer` takes `.get(0)` of an empty Remembered list (Plague of Vermin); a `CountSVar` expression with `/` and no `.` indexes out of bounds (all corpus forms have both)                                                                                                                                                                                                 | `SVar$ EachPlayer` refused                                                                                     | Not filing: latent, Plague of Vermin always has a Remembered player (RepeatEach) and every corpus `CountSVar` form has both `/` and `.`                 |
| `ControlSpellEffect.java:99`                                           | `si.setActivatingPlayer` dereferences the stack instance that is null once the named spell left the stack (Sudden Substitution)                                                                                                                                                                                                                                                    | Refused with an error                                                                                          | Not filing: latent, Sudden Substitution has split second, so the spell cannot leave the stack first                                                     |
| `CardProperty.java:355-357`, `Card.java:5891`                          | A `+Other` after `sharesNameWith Valid <list>` splits at the outer level and becomes a property of the tested card, so the host's own copy counts (Pattern Matcher)                                                                                                                                                                                                                | Reproduced (PORT-7)                                                                                            | Issue first: needs a syntax that nests `+` inside `Valid`; a Java test reproduces it (Pattern Matcher finds its own copy)                               |
| `Aggregates.java:119-192`                                              | `Aggregates.random` draws from the process-global `MyRandom`, not the game's RNG, so a `Random$` pick is not per-game deterministic (Tyrant of Discord)                                                                                                                                                                                                                            | Sacrifice Random$ refused                                                                                      | Not filed                                                                                                                                               |
| `ChangeZoneEffect.java:831-846`                                        | `ImprintLast$` is read only inside the `hasMergedCard()` branch, so it does nothing for an ordinary card; its comment names Duplicant, which works only because `Unimprint$` clears the list                                                                                                                                                                                       | Reproduced (PORT-7): ignored for an ordinary card, a merged permanent refused                                  | Not filing: latent, Duplicant is the only line and works                                                                                                |
| `AbilityUtils.java:3461-3468`, `:1013`, `:1191-1193`                   | `playerXCount` `Condition` computes the per-player amount once from the first player and compares everyone to it (Game Over); a trigger's `CheckSVar$` reaching a `Triggered*` defined player gets every player because the branch needs a `SpellAbility`                                                                                                                          | Reproduced (PORT-7): same single value, same widened player list                                               | Not filing: latent for Game Over, every player has the same starting life; the `CheckSVar$` half is the `CardTraitBase.java:472` row                    |
| `____________trespasser.txt:7` (`KW$ HIDDEN Unblockable`)              | No Java code reads a hidden keyword `Unblockable` (only a comment in `CombatUtil.java:533`), so the Pump's "can't be blocked this turn" never happens: a param that never reaches its effect                                                                                                                                                                                       | The card plays                                                                                                 | [#12457](https://github.com/Card-Forge/forge/pull/12457), merged                                                                                        |
| `ChangeZoneEffect.java:541`, `SpellAbility.java:2638`                  | `RandomOrder$` is read only by `changeKnownOriginResolve`, but `Origin$ Library` makes a line hidden even with `Defined$`, so Hurkyl, Master Wizard and Green Sun's Twilight (Library to Library) never reach it: a param that never reaches its effect                                                                                                                            | `RandomOrder$` ignored on a hidden path                                                                        | Not filed: unverified, a 12-card Java test saw the order change on the hidden path (the controller may order the cards)                                 |
| `hurkyls_final_meditation.txt:4`, `CostAdjustment.java`                | `Affected$ Card.Self` on a `Mode$ RaiseCost` static is read by no cost-modification code; the line applies through its host alone (matters only with a second copy on the stack)                                                                                                                                                                                                   | `Affected$` accepted and ignored                                                                               | Not filed                                                                                                                                               |
| `GameAction.java:207`, `CardCopyService.java:313`                      | A Permanent color from Animate on a spell on the stack looks to survive the spell's zone change: the stack card is copied on every zone change, `copyFrom` carries `changedCardColors` and `setColor` bakes it into the new base (CR 400.7 ends it); unverified by running                                                                                                         | `TgtZone$ Stack` on Animate refused                                                                            | Not filed                                                                                                                                               |
| `StaticAbilityMustAttack.java:22-27`, `cogwork_tracker.txt:4-5`        | `entitiesMustAttack` never reads `ValidPlayer$` (only `mustAttackSpecific` does, :55): both Tracker lines add the attacker unconditionally, so the creature must attack anything twice; the intended restriction to one player is dead (PlayerProperty.java:262-265 compares a draft note with the player name and could never match)                                              | `Mode$ MustAttack` with `ValidPlayer$` refused                                                                 | Not filed                                                                                                                                               |

### `Player.java:3435` — `getMonarchSet` ternary is inverted

```java
public String getMonarchSet() {
    return monarchEffect == null ? monarchEffect.getSetCode() : null;
}
```

Compare the sibling three lines down, which is correct:

```java
public String getInitiativeSet() {
    return initiativeEffect != null ? initiativeEffect.getSetCode() : null;
}
```

`getMonarchSet` has the condition and the branches both backwards from `getInitiativeSet`'s pattern:

- When `monarchEffect == null` (no one is the monarch, or `createMonarchEffect` hasn't run), the "true" branch calls
  `monarchEffect.getSetCode()` on a null reference — `NullPointerException`.
- When `monarchEffect != null` (there is a monarch and its set code is known), the "false" branch returns `null` instead
  of the set code — the caller (the monarch UI badge's set icon) silently gets nothing instead of the set that should
  never NPE.

Either path is wrong: it either throws whenever anyone would call it while there is no monarch, or (if some caller
happens to only invoke it after already checking `isMonarch()`/similar) returns `null` when a real value exists. There
is no code path where this method returns a useful non-null result.

**Proposed fix:** match `getInitiativeSet`'s pattern exactly:

```java
public String getMonarchSet() {
    return monarchEffect != null ? monarchEffect.getSetCode() : null;
}
```

The one game-rules caller is `Game.java:992/994`, passing the monarchy on when the monarch loses: `monarchEffect` is set
there, so it passes `null` rather than throwing.

**Upstream:** fixed as proposed, `monarchEffect != null`, in `8cd226407b1` (2026-09-25).

**Crucible meanwhile:** no counterpart. `BecomeMonarch` is ported (`becomemonarcheffect.go`), but this port carries no
set codes and `becomeMonarch` takes none, so there is nothing for the defect to reach.

### `GameAction.java:2568-2573` — `takeInitiative` gives the initiative to a player who has lost

```java
if (!p.equals(previous)) {
    if (previous != null) {
        previous.removeInitiativeEffect();
    }

    if (p.hasLost()) { // the person who should take initiative is gone, it goes to next player
        takeInitiative(game.getNextPlayerAfter(p), set);
    }

    game.setHasInitiative(p);
    p.createInitiativeEffect(set);
}
```

The `hasLost` branch passes the take to the next player, as its comment says, and then falls through: `p` -- the player
who has lost -- becomes the holder and gets a new "The Initiative" card, while the next player's card, created by the
recursive call, is never removed. Both players then hold a card; only the lost one is `game.getHasInitiative()`.

Reached from `Game.onPlayerLost` (`Game.java:998-1006`) when the holder and the active player lose in the same
`checkGameOverCondition` pass: the holder's initiative passes to the active player, who has lost too.
`TakeInitiativeEffect` never reaches it (it skips players not in the game).

**Proposed fix:** return after the recursive call.

```java
if (p.hasLost()) {
    takeInitiative(game.getNextPlayerAfter(p), set);
    return;
}
```

**Crucible meanwhile:** reproduced. `takeinitiativeeffect.go`'s `takeInitiative` runs the same code, so the lost active
player ends holding the initiative (`TestInitiativeToALostActivePlayerReproducesJava`). A state bug, not a crash or a
hang, so matching the oracle wins over correcting it in Go alone (PORT-8).

### `CardUtil.java:345` — nested `getReflectableManaColors` frame reads the wrong host

```java
// CardUtil.java:237-243
private static Set<String> getReflectableManaColors(final SpellAbility abMana, final SpellAbility sa,
        Set<String> colors, final CardCollection parents) {
    ...
    final Card card = abMana.getHostCard();

// CardUtil.java:264-271
    if (validCard.startsWith("Defined.")) {
        cards = AbilityUtils.getDefinedCards(card, TextUtil.fastReplace(validCard, "Defined.", ""), abMana);
    } else {
        ...
        cards = CardLists.getValidCards(activator.getGame().getCardsIn(ZoneType.Battlefield), validCard, activator, card, sa);
    }

// CardUtil.java:345, inside the ReflectProperty$ Produce branch
    colors = CardUtil.getReflectableManaColors(sa, ab, colors, parents);
```

The recursion reads `ab`'s own `Valid$`, `ColorOrType$` and `ReflectProperty$` through `sa`, but passes the outer `sa`
as `abMana`. So `card` in the nested frame is the reflecting card's host, not `ab.getHostCard()`, and:

- `Valid$ Defined.*` resolves against the reflecting card, and `getDefinedCards`' player is `abMana`'s activator
  (`AbilityUtils.java:71-77`).
- A non-`Defined` `Valid$` uses the reflecting card as its valid-string source.

Corpus cases, each wrong under CR 106.7 ("the types of mana the reflected ability could produce"):

| Reflecting                              | Reflected                                                       | Java reads                                                |
| --------------------------------------- | --------------------------------------------------------------- | --------------------------------------------------------- |
| Reflecting Pool (`Valid$ Land.YouCtrl`) | Pit of Offerings (`Valid$ Defined.ExiledWith`)                  | Cards exiled with Reflecting Pool: none, so no colors     |
| Exotic Orchard (`Valid$ Land.OppCtrl`)  | Opponent's The Grey Havens (`Defined.ValidGraveyard ...YouOwn`) | Legendary creatures in the Orchard controller's graveyard |

Depth 2 is inconsistent again: its `abMana` is depth 1's `ab`, so a chain of three reflecting lands reads a different
wrong host at each level.

**Proposed fix:** pass the reflected ability as both arguments, so `card` and the defined-player are `ab`'s own:

```java
colors = CardUtil.getReflectableManaColors(ab, ab, colors, parents);
```

`abMana.getApi()` (`:245`) still reads `ManaReflected`, since only such abilities reach `reflectAbilities`. The
`Produced` branch's `abMana.getRootAbility()` (`:297`) is never reached from the recursion: `Produced` abilities are not
added to `reflectAbilities` (`:333`).

**Crucible meanwhile:** no counterpart. `ManaReflected` is deferred
([`effects-manareflected.md`](port-log/game-state/effects-manareflected.md)); whoever ports the `Produce` walk decides
between reproducing it (oracle parity) and carrying the fix upstream first.

### `FlipOntoBattlefieldEffect.java:109` — neighbor filter always matches

```java
} else if (c.isPlaneswalker() || c.isArtifact() || (c.isEnchantment() && !c.isAura())) {
    return card.isPlaneswalker() || card.isArtifact() || (c.isEnchantment() && !c.isAura());
```

`getNeighboringCard`'s own filter decides whether `card` is a candidate neighbor for the landing spot `c`. The third
clause of the return re-tests `c` (the landing spot) instead of `card` (the candidate under test) —
`c.isEnchantment() && !c.isAura()` instead of `card.isEnchantment() && !card.isAura()`. Entering the branch at all needs
only one of the three OR'd conditions on `c` (`java:108`'s own `else if`). A planeswalker or artifact spot that is not
also a non-Aura enchantment reaches the return with its own third clause `false`, so it degenerates to the correct
`card.isPlaneswalker() || card.isArtifact()` — no bug there. Only when `c` is itself a non-Aura enchantment is that
third clause `true` unconditionally, and the whole return degenerates to "true" regardless of `card`: every permanent on
the landing spot's controller's battlefield becomes a valid neighbor.

**Proposed fix:** test the candidate, matching every other clause in the same return:

```java
return card.isPlaneswalker() || card.isArtifact() || (card.isEnchantment() && !card.isAura());
```

**Crucible meanwhile:** `flipCandidates` (`flipontobattlefieldeffect.go`) rejects a non-Aura-enchantment landing spot
outright with an `error` rather than sweeping the whole battlefield the way the bug does. A planeswalker or artifact
landing spot — including Chaos Orb choosing itself, a real reachable shape — does not trigger the bug and is not
rejected; it resolves through the correct two-clause filter above.

### `PlayEffect.java:312`, `:389` — `AllowRepeats$` re-offers an unplayable card forever

```java
if (!sa.hasParam("AllowRepeats")) {
    tgtCards.remove(tgtCard);
}
// ...
if (sas.isEmpty()) {
    continue;                       // :312
}
// ...
} else if (tgtSA.getPayCosts().hasManaCost() && tgtSA.getPayCosts().getCostMana().getMana().isNoCost()) {
    // unpayable
    continue;                       // :389
}
```

The loop runs `while (!tgtCards.isEmpty() && amount > 0 ...)`. Both `continue`s skip `amount--` (`:484`). Without
`AllowRepeats$` the card already left `tgtCards` (`:267-269`), so the loop still shrinks. With it, the card stays, the
next pass offers it again, and a non-optional or single-candidate pass picks it every time: a hang. The `continue`s at
`:327-333` (a cancelled `getAbilityToPlay`) and `:365-367` (`XMin$` under an alternative cost) have the same shape.

Latent: the two real `AllowRepeats$` lines (Mnemonic Deluge, Chandra, Pyromaster) carry `ValidSA$ Spell` and
`WithoutManaCost$`, so their `ValidSA$` pre-filter (`:198`) removes a card with no spell and neither reaches `:389`.

**Proposed fix:** spend the pick on every `continue` that does not play, or remove the card from `tgtCards` there:

```java
if (sas.isEmpty()) {
    tgtCards.remove(tgtCard);
    continue;
}
```

**Crucible meanwhile:** `playRepeatLoop` (`playeffect.go`) returns an `error` when either branch is reached under
`AllowRepeats$`, rather than looping or quietly dropping the card.

### `StaticAbilityCantAttackBlock.java:269` — `cantBlockBy(attacker, null)` can never be true

`CombatUtil.canBeBlocked(attacker, combat, defendingPlayer)` ends with an "Unblockable check" (`CombatUtil.java:533`):
`StaticAbilityCantAttackBlock.cantBlockBy(attacker, null)`. Every static reaches `applyCantBlockByAbility`, whose
`if (blocker == null || !stAb.matchesValidParam("ValidDefender", blocker.getController())) return false;` (`:269`)
returns false for a null blocker before any restriction is applied. So an attacker that "can't be blocked" still reads
as blockable to `canBeBlocked`, and every caller relying on it (the AI's block planning, `mustBlockAnAttacker`) has to
be saved by a separate per-pair `canBlock(attacker, blocker)`.

**Proposed fix:** give `applyCantBlockByAbility` a blocker-less mode that applies a static with no `ValidBlocker$`,
`ValidBlockerRelative$` or `ValidDefender$` (an unconditional "can't be blocked"), or drop the dead check.

**Crucible meanwhile:** `canBeBlockedInCombat` (`blockvalidation.go`) leaves the check out. The block validator's
outcomes do not change: each of its requirement tests also asks `CanBlock(attacker, blocker)`, which applies the static.

### `Player.java:2642` — `planeswalk` reads an empty planar deck's top card

`Player.planeswalk` is `planeswalkTo(sa, new CardCollection(getZone(ZoneType.PlanarDeck).get(0)))` with no size check.
`PlaneswalkEffect.resolve` calls it after every player's `leaveCurrentPlane` (`PlaneswalkEffect.java:39-48`), so an
activator with an empty planar deck throws `IndexOutOfBoundsException` with every plane already moved to a planar deck.
Reachable only when the activator's deck is empty and the active plane is not theirs: their own leaving plane refills it
first.

**Proposed fix:** return from `planeswalk` when the planar deck is empty.

**Crucible meanwhile:** `planeswalkEffect` (`planeswalkeffect.go`) returns
`engine: Planeswalk: activator's planar deck is empty` before anything moves (`TestPlaneswalkRejectsUnportedShapes`).

### `TriggerChaosEnsues.java:43-48` — chaos ensuing for two planes fires neither

Suspected, not confirmed: no real card reaches it. `performTest`'s `Iterable` branch over `AbilityKey.Affected` returns
false as soon as one affected card is not the trigger's host, so a trigger passes only when every affected card is its
own host. `ChaosEnsuesEffect.resolve` (`ChaosEnsuesEffect.java:41-57`) fills `Affected` with every `Defined$` card
carrying a chaos ability; two distinct planes there fire no chaos ability at all. CR 311.7 speaks of chaos ensuing "for
a particular object", so the multi-card intent is undefined; "any affected card is the host" (`contains`) reads as the
likely intent. The one real `Defined$` line (The Fertile Lands of Saulvinia) remembers a single plane.

**Proposed fix:** test `Iterables.contains(affected, getHostCard())` instead of every element, or confirm the all-host
reading is deliberate.

**Crucible meanwhile:** `chaosEnsuesAffected` (`chaosensueseffect.go`) returns
`engine: ChaosEnsues: Defined$ <spec> naming more than one plane not resolvable yet` before anything fires
(`TestChaosEnsuesRejectsUnportedDefinedShapes`).

### `RunChaosEffect.java:25-31` — chaos copies' optionality history-dependent, always confirmed

`RunChaosEffect.resolve` builds one `WrappedAbility` per chaos trigger of each card. For `OptionalDecider$` (`:26-28`)
and `Cost$` (`:29-30`) it calls `sa.setOptionalTrigger(true)`, where `sa` is the RunChaos ability itself, not
`triggerSA`, the copy the wrapper delegates `isOptionalTrigger` to (`WrappedAbility.java:363-365`). The copy is optional
only if the stored ability `Trigger.ensureAbility` returns (`Trigger.java:620-631`) was already flagged by an earlier
ordinary run of the same trigger: `TriggerHandler.java:473-475` stores that ability as the overriding one and `:507-508`
flags it, and `SpellAbility.copy`'s `clone()` (`SpellAbility.java:1253-1256`) keeps the flag. `decider` starts as the
activator (`:25`), never `null`, unlike `TriggerHandler.java:505-516`, which leaves it `null` for a mandatory trigger.
So `WrappedAbility.resolve` (`WrappedAbility.java:431-437`) asks `confirmTrigger` for every copy:
`PlayerControllerHuman.confirmTrigger` prompts even for a mandatory chaos ability, while `PlayerControllerAi.java:411`
reads `isMandatory()` as true for an unflagged copy and never declines it, so an `OptionalDecider$ You` chaos ability is
optional for the AI only if its plane's chaos trigger already fired the ordinary way. Pools of Becoming reaches it
whenever it reveals one of the 6 planes whose chaos ability names `OptionalDecider$ You`.

**Proposed fix:** set the flag on `triggerSA` and start `decider` at `null`, as `TriggerHandler.registerActiveTrigger`
does.

**Crucible meanwhile:** `runChaosEffect` (`runchaoseffect.go`) returns
`engine: RunChaos: <card>: chaos trigger OptionalDecider$ not resolvable yet` (or `Cost$`) before any copy is pushed
(`TestRunChaosRejectsUnportedChaosTriggers`). A mandatory chaos ability resolves without confirmation.

### `SwitchBlockEffect.java:22-25` — re-block `Blocks` trigger never sees its attacker

`runTriggers` builds one run-param map with `AbilityKey.Attacker` and `AbilityKey.Blocker` and runs both
`AttackerBlockedByCreature` and `Blocks` with it. `TriggerBlocks.performTest` reads `AbilityKey.Attackers`
(`TriggerBlocks.java:61`, and `:71` for triggering objects), as `BlockEffect.java` and `PhaseHandler` supply it. Under
`RemoveFromCombat$` (Sorrow's Path) a `Mode$ Blocks` trigger naming `ValidBlocked$` tests `null` and never fires; 8
corpus `Blocks` triggers name it.

**Proposed fix:** put `AbilityKey.Attackers` in the map for the `Blocks` run.

**Crucible meanwhile:** `checkReblockTriggerKeys` (`switchblockeffect.go`) returns an `error` before any block moves
when a re-block would test such a trigger (`TestSwitchBlockSorrowsPathValidBlockedTriggerFailsClosed`). A `Blocks`
trigger without `ValidBlocked$` fires normally.

### `SwitchBlockEffect.java:97,103,151,157` — switched blocks leave no blocked-by record

`BlockEffect.java:60-61` and `PhaseHandler.java:804-805` record `addBlockedThisTurn`/`addBlockedByThisTurn` for every
block they add; `SwitchBlockEffect`'s four `combat.addBlocker` calls do not. Sorrow's Path finds its attackers through
that record (`DefinedAttacker$ Valid Creature.blockedByValidThisTurn Targeted`, `CardProperty.java:1618`). After one
switch, a second activation targeting a switched blocker misses the attacker it now blocks: `removeFromCombat` strips
the blocker from it and the other blocker is never added, contrary to "each one then blocks all creatures the other was
blocking".

**Proposed fix:** record both lists in `SwitchBlockEffect` after each `addBlocker`, as `BlockEffect` does.

**Crucible meanwhile:** `checkStrayBlocks` (`switchblockeffect.go`) returns an `error` before any block moves when a
targeted blocker blocks a live attacker the scan missed (`TestSwitchBlockSorrowsPathSecondSwitchFailsClosed`). Blocks
the effect adds are not recorded either, matching Java's record.

### `SwitchBlockEffect.java:88` — General Jarkeld drops a blocker's other blocks

General Jarkeld: "each creature that's blocking exactly one of those attacking creatures stops blocking it and is
blocking the other attacking creature." `combat.removeFromCombat(blocker)` (`:88`) removes every block the creature has.
A creature blocking one of the two targets and a third attacker (a multi-block, e.g. from a `Block` effect) loses the
third block and only gets the switched one back.

**Proposed fix:** `combat.removeBlockAssignment(attacker, blocker)` for the two targeted attackers only.

**Crucible meanwhile:** `checkStrayBlocks` returns an `error` before any block moves when a switching blocker also
blocks a live attacker outside the two targets (`TestSwitchBlockJarkeldStrayBlockFailsClosed`).

### `RestartGameEffect.java:47-51` — a pending begin-of-combat command survives a restart

```java
game.getUntap().clearCommands();
game.getUpkeep().clearCommands();
game.getEndOfCombat().clearCommands();
game.getEndOfTurn().clearCommands();
game.getCleanup().clearCommands();
```

Five phase-command lists, cleared as part of Karn Liberated's `-14` reset. `game.getBeginOfCombat()` is not among them,
nor does `PhaseHandler.restart()` (`:826-829`, only `extraPhases`/`extraTurns`/`turn`) or the `RestartedByKarn` exit
branch (`:1149-1154`, only phase and an event) touch it either. A `ControlPlayerEffect`
(`ControlPlayerEffect.java:34,44`) that scheduled its `Combat$` grant onto `getBeginOfCombat()` and has not yet fired it
keeps that command through the restart: the grant still activates at the target's own next combat, in the new game, with
no card on the battlefield or anywhere else recording that it is coming.

**Proposed fix:** add `game.getBeginOfCombat().clearCommands();` alongside the other five.

**Crucible meanwhile:** reproduced. `resetForRestart` (`restartgameeffect.go`) filters `g.scheduled` down to entries at
`boundaryBeginCombat` only, dropping `boundaryCleanup`/`boundaryEndCombat` ones exactly as the five real
`clearCommands()` calls do, and leaving a pending begin-of-combat grant in place
(`TestRestartGamePreservesAPendingBeginCombatGrant`).

### `CamouflageEffect.java:78-80` — pool filter throws on a creature that can't block

```java
CardCollection pool = new CardCollection(defender.getCreaturesInPlay());
for (final Card blocker : pool) {
    if (!CombatUtil.canBlock(blocker)) {
        pool.remove(blocker);
    }
}
```

`FCollection.iterator()` is its backing `ArrayList`'s (`FCollection.java:231-233`); `remove` changes that list
(`:288-292`). The next `next()` throws `ConcurrentModificationException`. Only a removed creature sitting second-to-last
escapes: `hasNext()` is then false, so the loop ends early and the last creature goes unchecked. `getCreaturesInPlay`
includes tapped creatures, so any defender with a tapped, "can't block", detained or suspected creature reaches it. This
is the human branch, the one every non-AI declarer takes.

**Proposed fix:** `pool.removeIf(blocker -> !CombatUtil.canBlock(blocker));`, or `CardLists.filter`.

**Crucible meanwhile:** `camouflagePiles` (`camouflageeffect.go`) returns an `error` naming the creature before the
declarer is offered any pile, when `canBlockAtAll` rejects one of the defender's creatures
(`TestCamouflageFailsClosedOnACreatureThatCannotBlock`). That includes Java's second-to-last case, which does not crash
but leaves the last creature unchecked. Filtering silently would play a game state Java never reaches; ADR-0035 has the
reasoning.

### `PlaySpellAbility.java:766-782`, `PlayerControllerHuman.java:374-381` — X named only by a RaiseCost part is neither asked nor capped

Suspected, not run: found while reading the announce path for open plan R25g.

`announceValuesLikeX` looks at `ability.getPayCosts()`, the raw cost (`:766-767`). A
`S:Mode$ RaiseCost | ValidCard$ Card.Self` additional cost is not in it: `CostAdjustment.adjust` returns a new `Cost`
and never stores it on the spell ability (`CostAdjustment.java:37-40`, `:82-94`; run later, `CostPayment.java:137`,
`PlaySpellAbility.java:472`). Two consequences:

- **No announce.** Crashing Wave (`ManaCost:U U`, RaiseCost `Waterbend<X>`, `TargetMax$ X`, no `Announce$`) has no X in
  its raw cost, so `:780` sets `xManaCostPaid` to null and `Count$xPaid` reads 0. The three cards with `Announce$ X`
  (Foggy Swamp Visions, Waterbender's Restoration, Soul Immolation) are not affected: `:745-763` asks regardless.
- **No cap.** `getMaxForNonManaX` runs on the same raw cost (`PlayerControllerHuman.java:376-381`), so Aether Tide's
  `Discard<X/Creature>` is not limited to the creature cards in hand. X can exceed them, and the discard then fails in
  `HumanCostDecision` (`:168-172`) after the targets are chosen.

**Proposed fix:** announce and cap against the adjusted cost (`CostAdjustment.adjust` applied before
`announceValuesLikeX`), or set `Announce$ X` on the three affected scripts. Confirm by running Crashing Wave and Aether
Tide in the Java game first.

**Crucible meanwhile:** `spellCost` (`costmod.go`) refuses the cast of any RaiseCost with a non-mana `Cost$` (open plan
R25h), so nothing is played. When R25h lands, the announce follows Java (PORT-7) until upstream agrees.

## Attack costs not refunded when the attack turns illegal (`PhaseHandler.java:577`)

When dropping an unpaid attacker makes the declared attack illegal, costs already paid for other attackers are not
refunded (Java's own TODO at `PhaseHandler.java:577`). Reproduce: two creatures under Propaganda, pay for one, decline
the other where a requirement forces both to attack together.

**Crucible meanwhile:** the port keeps the payments too (PORT-7), so parity holds until upstream fixes it.

### `ChooseGenericEffect.java:69-80`, `:134-136` — an unpaid, non-switched choice with no fallback does nothing

Found while porting the `UnlessCost$` filter (open plan R25g). Not filed.

Choices whose `UnlessCost$` the chooser cannot pay are dropped (`:73-77`). When none is left and the script has no
`FallbackAbility$`, Java prints a stderr warning and resolves nothing (`:134-136`). Zoyowa's Lava-Tongue, Osseous
Sticktwister, Forbidden Ritual and Fear My Authority are non-switched ("X unless you pay") with no fallback, so a
chooser with no permanents and no cards in hand escapes the effect. Script-side fix: add a `FallbackAbility$` doing the
effect. `CostPartMana.java:118-120` (`canPay` always true, "for now") makes the 13 mana choices unfiltered, and `:75`
builds the cost from the raw string, skipping `AbilityUtils.calculateUnlessCost` (`AbilityUtils.java:1447`). Crucible
reproduces both (`quirks.md`).

### `CountersRemoveEffect.java:75`, `:137`, `TapEffect.java:44` — Chooser$ ignored

Found while porting the `Choices$` pick (open plan R16e). Not filed. No corpus line combines the two.

`CountersPutEffect` (`:196-202`), `ControlGainEffect` (`:108-109`) and `MustBlockEffect` (`:42-44`) ask the player
`Chooser$` names; `CountersRemoveEffect` and `TapEffect` ask the activator and never read it.
`ChoiceAmount$`/`ChoiceTitle$` are also unread by RemoveCounter (it reads `ChoiceNum$`), and `ChoiceOptional$` by Tap.
`Mandatory$` is read by none of the five. `ControlGainEffect.java:109` and `MustBlockEffect.java:44` call `.get(0)` on
`Chooser$` with no empty check (an empty list throws; `CountersPutEffect:198` returns). Crucible reproduces the ignored
parameters and returns an error for the empty list.

## `AtEOTTrig$ You_Sacrifice` never restricts to your end step

`SpellAbilityEffect.addSelfTrigger` (`SpellAbilityEffect.java:498-526`) splits `You_Sacrifice` and appends
`| Player$ You` to the Phase trigger. `TriggerPhase.performTest` reads `ValidPlayer$` only (`TriggerPhase.java:50-55`),
so the "your next end step" text of Gut, Fanatical Priestess is not enforced: the token is sacrificed at the next end
step of any player. In practice the tokens appear in the controller's own turn, so the first end step is theirs.
Crucible reproduces it (`quirks.md`). Fix upstream: `ValidPlayer$ You` in place of `Player$ You`.

## `FirstUpkeep$` and `FirstUpkeepThisGame$` never fire

`PhaseHandler.onPhaseBegin` increments `nUpkeepsThisTurn` and `nUpkeepsThisGame` as the upkeep begins
(`PhaseHandler.java:257-258`), before the phase triggers are checked, while `isFirstUpkeep` and `isFirstUpkeepThisGame`
test for the counter being 0 (`:980-986`). A Phase trigger carrying `FirstUpkeep$` (one card) or `FirstUpkeepThisGame$`
(two Command-zone cards) therefore never fires: "at the beginning of the first upkeep" does nothing. Crucible reproduces
it (`triggerPhasesCheck` answers false). Fix upstream: compare against 1, or read the counters before the increment.

## `NoPrevention$` is lost when `DamageMap$` defers the damage

`ReplacementHandler.java:407` reads `NoPrevention$` from the `cause` that finalizes the damage. `DamageDealEffect`
defers the deal under `DamageMap$` (`:210-214`), so the cause is the later `DamageResolve`
(`DamageResolveEffect.java:28`) or `RepeatEach` (`RepeatEachEffect.java:199-201`), neither of which carried the param
in Volcano Hellion's script. Its "can't be prevented" stayed preventable. The script now puts `NoPrevention$ True` on
`DmgResolve` (PR #12483); Crucible reads the param off `DamageResolve`
and ignores it on the `DealDamage` lines under `DamageMap$`, as Java does. A Java-side fix (copy the flag onto the
damage map or read it per source SA) is not needed for any corpus line.

## `Transformed$` on a known-origin `ChangeZone` still moves a card that cannot transform

`ChangeZoneEffect.java:598-607` (`changeKnownOriginResolve`) transforms the card when `isTransformable()`, and its
`else` branch holds only the comment "If it can't Transform, don't change zones", so the card moves untransformed. The
hidden path (`:1339-1349`) does `continue`. Crucible follows each path as written (`quirks.md`). Fix upstream:
`continue` in the known-origin `else`.

## `SacEachValid$` with `Destroy$` asks for a sacrifice and filters by sacrifice rules

`SacrificeEffect.java:124` calls `choosePermanentsToSacrifice` for every `SacEachValid$` alternative, then `:154`
destroys what was chosen, and `:114` applies `canBeSacrificedBy` even when `Destroy$` is set (the single-choice branch
skips it at `:132-135`). No corpus line combines the two, so Crucible refuses the combination. Fix upstream: pick the
controller method and the filter from `Destroy$` in both branches.

## `MinTotalCMC$` makes `Valid$`, `Amount$` and the found destinations dead on `DigUntil`

`DigUntilEffect.java:181` is an `else if`: with `MinTotalCMC$` set nothing is ever "found", so `Valid$`, the `Amount$`
threshold (`:192`), `FoundDestination$` outside the sequential shape, `RememberFound$` and `ImprintFound$` never act.
The three corpus lines all use equal destinations, so only the sequential shape works; Crucible refuses the rest.

`SetStateEffect.java:177-178` passes `FaceDownPower$`, `FaceDownToughness$` and `FaceDownSetType$` to
`CardFactoryUtil.setFaceDownState` but not `FaceDownKeyword$`, which `CardFactoryUtil.java:4147` handles: the param
never reaches its effect. No corpus line carries it, so Crucible ignores it the same way.

`ChangeZoneEffect.java:1450` (hidden origin, Destination Exile) reads `ExileFaceDown$` only; `FaceDown$` is read for a
battlefield destination (`:1417`) and on the known path (`:757`). Tezzeret's Reckoning spelt it `FaceDown$` with Origin
Library and exiled its three cards face up; the script now says `ExileFaceDown$` (PR #12483). Crucible still refuses `FaceDown$` on a hidden origin.

`CharmEffect.java:43-44` judges a mode's `getNumCandidates` before `setChoosingPlayer` (`:273`), so a mode whose
`ValidTgts$` names `ChoosingPlayer` with `TargetMin > 0` is filtered with no chooser. `AbilityUtils.java:1131-1132` adds
the root's chooser unguarded: `Defined$ ChoosingPlayer` outside a Charm adds null. No corpus line hits either (Fatal
Lore's mode has `TargetMin$ 0`; all four `ChoosingPlayer` lines sit in Charm modes).

### `TargetRestrictions.java:526-528` — `hasCandidates` ignores `ValidTgts$` on the stack

`hasCandidates` short-circuits to true as soon as the target zone list holds the stack, before the `isValid` loop the
other zones go through. Its one caller is `ChooseGenericEffect.java:94` (`AtRandom$ Urza`), so a targeting choice with no
spell on the stack to hit counts as having candidates there. `getNumCandidates` (:556-563) does test
`canTargetSpellAbility`, so the two disagree. Airbend's own prompt goes through `getAllCandidates` (:571-590), which
tests `canTarget` per card, so Aang, Swift Savior is offered only legal targets; Crucible still refuses a stack
`TgtZone$` on Airbend until the stack path is ported.

### Found porting the hidden-keyword, attach and choose gaps

`CostAdjustment.java:120-121` counts `ForEachShard$` shards in the host's printed mana cost even when the spell is cast
for an alternative cost, and `:123` has a TODO for the same on abilities. `applyReduceCostAbility` (`:459-511`) never
reads `ForEachShard$`. Only Drought uses it, on RaiseCost, so Crucible follows the printed-cost rule.

`StaticAbilityContinuous.java:358` reads `AddAbility$` only; `AddAbilities$` is read for Clone and CopyPermanent
(`CardFactory.java:649`) and never on a continuous static. Supper for Spiders' script spelt it `AddAbilities$`, so its Food
sacrifice ability was never granted. The script now says `AddAbility$` (PR
[#12474](https://github.com/Card-Forge/forge/pull/12474)).

`AbilityUtils.java:162` recognises `ParentTarget` in `getDefinedCards` but not `ParentTargeted`, which is a script
mistake, not a Java one: Captain's Maneuver spelt it `ParentTargeted`, so `ChooseCard` chose from an empty list. The
script now says `ParentTarget` (PR [#12457](https://github.com/Card-Forge/forge/pull/12457)).

`CombatUtil.java:533-535` tests only `StaticAbilityCantAttackBlock.cantBlockBy`; nothing in `forge-game` reads an
`Unblockable` keyword, so `KW$ HIDDEN Unblockable` (the `_____ _____ _____ Trespasser`) grants nothing. The script now
uses a `CantBlockBy` Effect (PR [#12457](https://github.com/Card-Forge/forge/pull/12457)).

`AttachEffect.java:83-89` removes from the `FCollection` it iterates (`FCollection.java:231-233`, `:288-298`), so a
`PlayerChoices$` entry that cannot be attached throws `ConcurrentModificationException` unless it is the second to last.
It throws only when a listed player cannot be enchanted by the attachment; Lynde, Cheerful Tormentor with a Curse
attached to an opponent never hits it. Crucible resolves `PlayerChoices$` with `Object$` and refuses the throwing shape
by name (`attachObjectToPlayerChoice`).

`CountersPutOrRemoveEffect.java:93-95` adds the counter at once when the card holds no counter of any kind and never reads
`RemoveConditionSVar$`, so Shape of the Wiitigo's creature gains a +1/+1 counter at the upkeep after it ran out of
counters although it did not attack or block. Crucible resolves the normal case (a counter present) and refuses the
zero-counter case by name. Not filed.

`PlaySpellAbility.announceValuesLikeX` (`:766-767`) reads the printed cost, not the adjusted one, so Crashing Wave's
static-merged `Waterbend<X>` is never announced; `TargetMax$ X` reads 0 and the tapping cap is 0. Foggy Swamp Visions
and Waterbender's Restoration say `Announce$ X` and work. Crucible keeps refusing the three.

`AbilityUtils.java:426-438` reads `SVar:Y:4-X` (Stroke of Luck) as a non-numeric single token and returns 0, so
`LoseLife | LifeAmount$ Y` loses nothing. The script now says `SVar$X/NMinus.4` (PR
[#12457](https://github.com/Card-Forge/forge/pull/12457); [card-script-defects.md](card-script-defects.md)).

## RollDice RerollResults: equal stored values lose rerolls

`RollDiceEffect.rerollDice` (`RollDiceEffect.java:578-594`) keys its `replaceMap` by the old stored value, and
`Card.replaceStoredRoll` (`Card.java:1604-1610`) removes one copy per key. With k equal stored values all rerolled, k
dice are rolled (their triggers fire) but one copy is replaced and k-1 rerolls vanish. Centaur of Attention is the only
card. Crucible reproduces the map (PORT-8); fix upstream by keying by index.

`BoosterGenerator.java:443` and `:457` call `contains` on `tryGetStaticSheet(...)` without a null check, so a `SheetReplaceCardFromSheet` naming a missing sheet throws a `NullPointerException`; `replaceCardFromExtraSheet` (`:591-593`) checks. `:487` reads `boosterSlots.get(slotType)` for a slot name with no `[Slot]` section and `:528` dereferences it. The `try`/`catch` of `:509-511` is dead for an unknown sheet: `makeSheet` returns an empty sheet or throws a different exception. `PrintSheet.fetchRoulette` (`:95-110`) is documented as a weighted unique draw but skips and walks on, which is not weighted among the cards left. Not filed. Crucible reproduces the draws (PORT-7) and returns an error where Java throws.

`PlayerProperty.java:262-265` (`NotedDefender`, Cogwork Tracker only) reads `player.getDraftNotes()`, the tested player's own notes, and compares them with that same player; `NotedColor` and the other noted-name properties read `sourceController.getDraftNotes()` (`CardProperty.java:2003`). The `sourceController.getDraftNotes()` change (branch `upstream-pr/cogwork-tracker-noted-defender`, not opened, not carried in the fork) is necessary but not sufficient: the note holds the passer's draft seat (`LimitedPlayer.java:334`, `String.valueOf(fromPlayer.order)`), while `String.valueOf(player)` is the player's name (`GameEntity.java:406`), so the comparison never matches, and a game carries no seat-to-player mapping to translate with. A complete fix needs that mapping in `forge-game`, an owner decision. A game here has no draft notes, so Crucible answers false for the property.
