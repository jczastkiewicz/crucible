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

## Status

| Site                                                        | Defect                                                                                                                                                                                                                                                                                                                                                                             | Crucible meanwhile                                                                                             | Upstream                                                                 |
| ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `CostMill.java:67`                                          | `canPay` is `amount < library size`: milling 4 with exactly 4 cards in the library is refused, where the comprehensive rules' Mill rule only forbids milling more cards than the library holds (Ashcoat of the Shadow Swarm's `Cost$ Mill<4>`)                                                                                                                                     | `Mill<N>` cost part not read: the trigger fails with an error                                                  | Not filed                                                                |
| `SpellAbilityEffect.java:797`                               | `addToCombat`'s blocking half: `getDefenderPlayerByAttacker` returns null for an attacker no longer in combat (killed in response to Brimaz's trigger) and `.equals` throws                                                                                                                                                                                                        | `addToCombatBlocking` skips the block (CR 506.4)                                                               | Not filed                                                                |
| `CardProperty.java:1522`                                    | `attackingBattle` reads `combat.getDefenderByAttacker(source)`, not the card's: Rampaging Geoderm's "if it's attacking a battle" asks whether the Geoderm is                                                                                                                                                                                                                       | `attackingBattle` refused with a pending error                                                                 | Not filed                                                                |
| `CombatUtil.java:70`                                        | `getAllPossibleDefenders`: `battle.getProtectingPlayer().isOpponentOf` throws for a battle with no protector                                                                                                                                                                                                                                                                       | Such a battle is not offered as an attack target                                                               | Not filed                                                                |
| `Player.java:3435`                                          | `getMonarchSet` ternary condition inverted                                                                                                                                                                                                                                                                                                                                         | No counterpart: no set codes in Crucible                                                                       | Fixed upstream (`8cd226407b1`)                                           |
| `GameAction.java:2568-2573`                                 | `takeInitiative` has no `return` after passing a lost player's take on                                                                                                                                                                                                                                                                                                             | Reproduced (oracle parity)                                                                                     | Not filed                                                                |
| `AlterAttributeEffect.java:87`                              | Unpreparing is only a leaves-play command: a creature prepared as it enters whose entry is then replaced (exiled by Containment Priest) never left the battlefield, so its prepared copy stays castable by the card's owner                                                                                                                                                        | Reproduced (oracle parity, `replacemententry.go`)                                                              | Not filed                                                                |
| `ChooseColorEffect.java:44`, `:55`, `:84`                   | `Choices$` builds `Arrays.asList`, a fixed-size list; `Exclude$`'s `remove` (:55) and `Random$`'s `remove` (:84) throw `UnsupportedOperationException` beside it. No corpus card combines them yet                                                                                                                                                                                 | `Choices$` with `Random$` or `Exclude$` refused (`choosecoloreffect.go`)                                       | [#12370](https://github.com/Card-Forge/forge/pull/12370)                 |
| `CardUtil.java:345`                                         | Recursive frame resolves `Valid$` against the reflecting host                                                                                                                                                                                                                                                                                                                      | None: `ManaReflected` deferred                                                                                 | Not filed                                                                |
| `FlipOntoBattlefieldEffect.java:109`                        | Neighbor filter re-tests the landing spot instead of the candidate; "always true" only for a non-Aura-enchantment spot                                                                                                                                                                                                                                                             | `flipCandidates` rejects that one shape with an `error`                                                        | Not filed                                                                |
| `PlayEffect.java:312`, `:389`                               | `continue` without `amount--` under `AllowRepeats$` re-offers the same unplayable card forever                                                                                                                                                                                                                                                                                     | `playRepeatLoop` returns an `error`                                                                            | [#12373](https://github.com/Card-Forge/forge/pull/12373)                 |
| `StaticAbilityCantAttackBlock.java:269`                     | `cantBlockBy(attacker, null)` always false, so `CombatUtil.canBeBlocked`'s unblockable check (`CombatUtil.java:533`) never fires                                                                                                                                                                                                                                                   | Check not ported; no validator outcome depends on it                                                           | Not filed                                                                |
| `Player.java:2642`                                          | `planeswalk`'s `getZone(PlanarDeck).get(0)` unguarded; throws on an empty planar deck, after every plane has already left                                                                                                                                                                                                                                                          | `error` before anything moves                                                                                  | Not filed                                                                |
| `TriggerChaosEnsues.java:43-48`                             | Suspected: `Affected` iterable must be all host, so chaos ensuing for two planes fires neither                                                                                                                                                                                                                                                                                     | `Defined$` naming two planes returns an `error`                                                                | Not filed                                                                |
| `RunChaosEffect.java:25-31`                                 | `setOptionalTrigger(true)` hits the RunChaos ability, not the copy; decider never null, so every copy asks `confirmTrigger`                                                                                                                                                                                                                                                        | `OptionalDecider$`/`Cost$` chaos trigger: `error`                                                              | Not filed                                                                |
| `GameAction.java:635-640`                                   | `unmeldPosition` computed, never used: the melded card goes to the primary's own `position`, so the owner never orders the two (CR 401.4)                                                                                                                                                                                                                                          | Reproduced (oracle parity): secondary over primary on library top                                              | Not filed                                                                |
| `SwitchBlockEffect.java:22-25`                              | `Blocks` trigger run with `AbilityKey.Attacker`; `TriggerBlocks.java:61` reads `Attackers`, so a re-block never matches `ValidBlocked$`                                                                                                                                                                                                                                            | That re-block: `error` before anything moves                                                                   | `upstream-pr/switch-block`                                               |
| `SwitchBlockEffect.java:97,103,151,157`                     | `addBlocker` never records `addBlockedByThisTurn`; Sorrow's Path's history-based `DefinedAttacker$` then misses a switched block                                                                                                                                                                                                                                                   | That switch: `error` before anything moves                                                                     | `upstream-pr/switch-block` (:151, :157; :97, :103 left, rules unsettled) |
| `SwitchBlockEffect.java:88`                                 | Jarkeld's `removeFromCombat(blocker)` drops the blocker's block on a third attacker too; card text moves only the switched block                                                                                                                                                                                                                                                   | That switch: `error` before anything moves                                                                     | `upstream-pr/switch-block`                                               |
| `RestartGameEffect.java:47-51`                              | Clears untap/upkeep/end-of-combat/end-of-turn/cleanup command lists, never `getBeginOfCombat()`'s; a pending `Combat$` `ControlPlayer` grant survives a restart                                                                                                                                                                                                                    | Reproduced (oracle parity): the boundary-begin-combat entry is kept                                            | Not filed                                                                |
| `CamouflageEffect.java:78-80`                               | Pool filter removes from the list it iterates: `ConcurrentModificationException` once the defender has a creature that can't block                                                                                                                                                                                                                                                 | `error` before any pile is chosen (ADR-0035)                                                                   | [#12370](https://github.com/Card-Forge/forge/pull/12370)                 |
| `ChooseCardEffect.java:262-269`                             | `QuasiLibrarySearch$`: a declined or empty pick `return`s before `removeController(controlTimestamp)`, so the search-control grant (Opposition Agent) never ends                                                                                                                                                                                                                   | `QuasiLibrarySearch$` under a search-control redirect: `error`                                                 | Not filed                                                                |
| `ChooseCardEffect.java:243-260`                             | `QuasiLibrarySearch$` checks neither `canSearchLibraryWith` (Leonin Arbiter) nor `LimitSearchLibrary` for the choices (Aven Mindcensor shortens only the reveal)                                                                                                                                                                                                                   | Reproduced (oracle parity): no check                                                                           | Not filed                                                                |
| `ChangeZoneEffect.java:1090-1092`                           | Suspected: `Searched$` resets `searchedLibrary`, so `FoundSearchingLibrary` (`:1308`) is false and Opposition Agent never exiles what Doubling Chant finds                                                                                                                                                                                                                         | Reproduced (oracle parity)                                                                                     | Not filed                                                                |
| `AbilityUtils.java:2186`                                    | `Count$Presence_<Type>` counts controlled permanents of the type as the amount is read (`Count$LastStateBattlefield`), not "as you cast this spell"                                                                                                                                                                                                                                | Reproduced (oracle parity): `presenceCount` reads the battlefield then                                         | Not filed                                                                |
| `PlaySpellAbility.java:766-782`                             | Suspected: `announceValuesLikeX` reads the raw `getPayCosts()`, so an X named only by a `Mode$ RaiseCost` part (Crashing Wave's `Waterbend<X>` with no `Announce$`) is never asked and reads as 0                                                                                                                                                                                  | Reproduced (oracle parity) once R25h lands; not ported                                                         | Not filed                                                                |
| `ChangeZoneEffect.java:961-963`, `:1014-1016`               | `OriginAlternative$` with the library declined leaves `origin = [Graveyard]`; with no `DefinedPlayer$` the pool becomes every player's graveyard (`game.getCardsIn(origin)`), so a `ChangeType$` without `YouOwn` takes an opponent's card: Captain Kirk, Boldly Going (`Land.Basic`), Kassandra, Eagle Bearer, Shaun & Rebecca, Agents, Auditore Ambush, against "your graveyard" | Reproduced (oracle parity)                                                                                     | Not filed                                                                |
| `ChangeZoneEffect.java:532`                                 | Known-origin `Chooser$`: `getDefinedPlayers(...).get(0)` unguarded, throws on an empty list. Unreached by the corpus: its 5 lines name `You` or a single target's owner/controller, whose loss fizzles the ability first                                                                                                                                                           | `error` before anything moves                                                                                  | Not filed                                                                |
| `PlayerControllerHuman.java:374-381`                        | Suspected: the non-mana X cap (`getMaxForNonManaX`) reads the raw cost too, so Aether Tide's `Discard<X/Creature>` from `RaiseCost` is not capped and the discard fails at payment after targets are chosen                                                                                                                                                                        | Reproduced (oracle parity) once R25h lands; not ported                                                         | Not filed                                                                |
| `SpellAbility.java:2153-2161`                               | `TargetingPlayer$` naming nobody: `chooseSingleEntityForEffect` returns null for the empty list (`PlayerControllerHuman.java:585-590`) and `targetingPlayer.getController()` throws a NullPointerException                                                                                                                                                                         | An `error`                                                                                                     | Not filed                                                                |
| `GameAction.java:2398-2401`                                 | `numCardsInHandStartedThisTurnWith` is set after the game loop, not at the turn's start                                                                                                                                                                                                                                                                                            | See `port-log/game-state/m6-r9d-cast-and-turn-records.md`                                                      | Not filed                                                                |
| `DamageDealEffect.java:248`, `DamagePreventEffect.java:105` | A loop variable leaks into the per-target amount of a divided damage or prevention                                                                                                                                                                                                                                                                                                 | See `port-log/game-state/m6-r9-dealdamage-divided.md`                                                          | Not filed                                                                |
| `AbilityUtils.java:1129-1130`                               | `Defined$ DefendingPlayer` reads `getCombat()` unguarded, so it throws outside combat                                                                                                                                                                                                                                                                                              | An `error`; no corpus line reaches it                                                                          | Not filed                                                                |
| `SpellAbilityCondition.java:420-424`                        | A `ConditionLifeAmount` SVar other than `X` throws                                                                                                                                                                                                                                                                                                                                 | An `error`; no corpus line reaches it                                                                          | Not filed                                                                |
| `mana_clash.txt:11`, `sword_of_dungeons_dragons.txt:12`     | `SVar:RepeatCheck:Number$ 0` / `Number$ 1`: the space makes xCount's `Integer.parseInt` (`AbilityUtils.java:1585-1587`) throw if read. Latent: `RepeatEffect.java:39-52` reads it only after the repeated chain's `StoreSVar` rewrote it                                                                                                                                           | Read after the first repetition as Java; the script value is never read                                        | Not filed                                                                |
| `TextBoxExchangeEffect.java:74-76`                          | Removes the exchanged traits when an `AsLongAsInPlay` exchange ends, but leaves the two `addEmpty` clear rows `Card.java:4895-4896` wrote, so word changes made before the exchange stay wiped after it (the source comment is a question)                                                                                                                                         | The ending exchange drops its own rows, so earlier word changes return: differs from Java on this one sequence | Not filed                                                                |
| `AbilityUtils.java:3090`                                    | Text-change regex built from the unescaped word with no right boundary: `Red` also rewrites the front of `Reduce`                                                                                                                                                                                                                                                                  | Reproduced (PORT-7), parity depends on it                                                                      | Not filed                                                                |
| `VoteEffect.java:93-94`                                     | Removes `realVoter` (the `ControlVote$` player) from a `VotePlayer$ Other` ballot, not the voting player: under a controlled vote each player may vote for themselves and nobody for the controller                                                                                                                                                                                | That combination returns an error citing the line                                                              | Not filed                                                                |
| `SacrificeAllEffect.java:66`                                | `MayFlashSac` sacrifices by card id (`GameEntity.equals` is id only), so a countered and reanimated spell's new object is sacrificed too                                                                                                                                                                                                                                           | Reproduced, not worked around                                                                                  | Not filed                                                                |
| `AbilityUtils.java:3310,3321`                               | `playerXCount` Highest/Lowest pass the whole `s`, `/op` included, to `playerXProperty`: the operator applies twice. Latent, no corpus line                                                                                                                                                                                                                                         | Refused, not reproduced                                                                                        | [#12371](https://github.com/Card-Forge/forge/pull/12371)                 |
| `AbilityUtils.java:3257`                                    | `doXMath` `Mod` with a zero operand throws `ArithmeticException`. Latent                                                                                                                                                                                                                                                                                                           | Refused                                                                                                        | [#12371](https://github.com/Card-Forge/forge/pull/12371)                 |
| `AbilityUtils.java:1584`                                    | `xCount` `Number$` with a non-integer body throws `NumberFormatException`. Latent                                                                                                                                                                                                                                                                                                  | Refused                                                                                                        | Not filed                                                                |
| `TriggerDamageDoneOnce.java:67-70`                          | `AttackingPlayer` is the first source's controller of a `HashMap`: undetermined when one entity was hit by several controllers' sources in one damage table                                                                                                                                                                                                                        | Refused, not reproduced                                                                                        | Not filed                                                                |
| `TriggerRolledDie.java:57-60`                               | `Number$` branch unboxes a missing `AbilityKey.Number` and throws on a planar roll (`PlanarDice.java:81,87` runs `RolledDie` with `Result` 0, no `Number`). Latent: 0 corpus lines                                                                                                                                                                                                 | Planar die roll refused before the roll                                                                        | Not filed                                                                |
| `SpellAbilityCondition.java:450`                            | Loop variable reset: `abSub = sa.getSubAbility()` where `abSub.getSubAbility()` is meant, so a `ConditionTargetValidTargeting` ChangeTargets with a SubAbility and no matching target never ends. Latent: 0 of 3 corpus lines have a SubAbility                                                                                                                                    | `ConditionTargetValidTargeting$` stays refused                                                                 | Not filed                                                                |
| `SpellAbilityCondition.java:465-471`                        | `ConditionTargetsSingleTarget` counts `sa.getAllTargetChoices()`, the ChangeTargets SA's own targets, never the targeted spell's: always true for a one-target SA, so Meddle and Quicksilver Dragon retarget multi-target spells                                                                                                                                                   | `ConditionTargetsSingleTarget$` stays refused                                                                  | Not filed                                                                |
| `ChangeTargetsEffect.java:52`                               | `Chooser$`: `.get(0)` on an empty player list throws, even with no spell to retarget. Latent                                                                                                                                                                                                                                                                                       | `Chooser$` refused                                                                                             | Not filed                                                                |
| `ChangeTargetsEffect.java:142-143`                          | Random branch reads `getTotalDividedValue()` after `resetTargets()` replaced the targets, so a divided allocation becomes 0 (`:155` reads it before). Latent: no `RandomTarget$` with a divided spell                                                                                                                                                                              | `RandomTarget$` refused                                                                                        | Not filed                                                                |
| `ChangeTargetsEffect.java:71-106`, `:152-163`               | `TargetRestriction$` is read only on the default branch; `ChangeSingleTarget$` and `DefinedMagnet$` ignore it. Latent: 6 lines carry it, none on those branches                                                                                                                                                                                                                    | Go ignores it on the same branches, as Java                                                                    | Not filed                                                                |
| `CopySpellAbilityEffect.java:87`, `:176`, `:195`            | `copies` is declared per controller and shared across the spell loop, so a CopySpell replacement (Twinning Staff) counts earlier spells' copies in its Amount and extras                                                                                                                                                                                                           | `CopyForEachCanTarget$` refused                                                                                | Not filed                                                                |
| `TargetRestrictions.java:53`, `:586-590`                    | `getAllCandidates` scans only the target zone (Battlefield by default) and never adds spells, so Glimmervoid Basin's `CopyForEachCanTarget$ Spell,Permanent,Card,Player` cannot offer a spell or an off-battlefield card                                                                                                                                                           | `CopyForEachCanTarget$` refused                                                                                | Not filed                                                                |
| `AiCostDecision.java:55-61`, `:523-564`                     | `visit(CostBehold)` pays one card whatever the cost's amount (Celestial Reunion `Behold<2>`, Kindle the Inner Flame `Behold<3>`); `visit(CostReveal)` falls through for `ChooseCard` (revealFrom All) to a hand-only filter and pays nothing (Close Encounter)                                                                                                                     | Not read: the AI cost decision is not ported                                                                   | Not filed                                                                |
| `StaticAbilityDisableTriggers.java:109`                     | `getParam("ValidCause").split(",")` throws for a ChangesZoneAll static without `ValidCause$`. Latent: all 9 corpus lines carry it                                                                                                                                                                                                                                                  | ChangesZoneAll path does not read the static                                                                   | Not filed                                                                |
| `EncodeEffect.java:57-61`                                   | Cipher exiles the spell card, fires the zone-change triggers, then asks for the creature, where the rule is one action (CR 702.99): exile triggers run before the creature is chosen. Low severity; the pre-checked non-empty choice list makes a null pick unreachable in practice                                                                                                | `Encode` unregistered: Cipher cards refuse                                                                     | Not filed                                                                |
| `Untap.java:132-142`                                        | `while (!restrictList.isEmpty())` spins forever if `chooseSingleEntityForEffect` returns null (non-optional call). Latent                                                                                                                                                                                                                                                          | `UntapAdjust` not ported                                                                                       | [#12373](https://github.com/Card-Forge/forge/pull/12373)                 |
| `Untap.java:151-155`                                        | The other-player untap loop bypasses `canUntap`, the optional-untap prompt, `UntapAdjust` and `OnlyUntapChosen`, and the `doesn't untap` keyword                                                                                                                                                                                                                                   | `UntapOtherPlayer` not ported                                                                                  | Not filed                                                                |
| `PlayerControllerAi.java:246-274`                           | `announceRequirements` returns null for any API but ChooseNumber/BidLife; `PlaySpellAbility.java:746-763` reads null as cancel, so the AI can never cast the March of Reckless Joy cycle (Dig announce)                                                                                                                                                                            | Non-mana `RaiseCost$` refuses the cast                                                                         | Not filed                                                                |
| `SetStateEffect.java:31`                                    | `sa.getParam("Mode").equals("Specialize")` throws when Mode is absent (Skittish Valesk's `NewState$` shape); `Card.java:676` then skips the TurnFaceDown guard (`:762-767`)                                                                                                                                                                                                        | `NewState$` and Specialize refused                                                                             | Not filed                                                                |
| `CountersPutEffect.java:250-259`                            | `DividedRandomly` mutates the target list while iterating it: `ConcurrentModificationException`                                                                                                                                                                                                                                                                                    | `DividedRandomly` refused                                                                                      | [#12370](https://github.com/Card-Forge/forge/pull/12370)                 |
| `Player.java:904-913`                                       | Player counter removal never fires `CounterRemoved` (the trigger block is commented out under a TODO), so energy loss and rad removal cannot trigger                                                                                                                                                                                                                               | `CounterRemoved` mode not ported                                                                               | Not filed                                                                |
| `TriggerManaExpend.java:47`                                 | `Integer.parseInt(mapParams.get("Amount"))` throws on a missing `Amount$`. Latent: all 13 corpus lines have it                                                                                                                                                                                                                                                                     | `ManaExpend` mode not ported                                                                                   | Not filed                                                                |
| `Game.java:1430`                                            | `setDayTime` compares two `Boolean`s with `!=` (reference comparison). Latent: current callers autobox                                                                                                                                                                                                                                                                             | `DayTimeChanges` compares values                                                                               | Not filed                                                                |
| `Player.java:584-592`, `Card.java:4787-4795`                | `PayLife` and `Tap` run the replacement but never read back an `Updated` `Amount` or `IsCombat`: the local value is used. Latent: no corpus line sets Updated there                                                                                                                                                                                                                | PayLife and Tap events not hooked                                                                              | Not filed                                                                |
| `ConniveEffect.java:73`, `ExploreEffect.java:65`            | Connive, Explore and Learn (`Player.java:3928`) treat `!= NotReplaced` (which includes `Updated`) as a replacement and skip the whole effect. Latent: no corpus line sets Updated there                                                                                                                                                                                            | The three effects refuse a live replacement for their event                                                    | Not filed                                                                |
| `CombatUtil.java:532`                                       | Pump `KW$ HIDDEN Unblockable` is inert: the combat check never reads the hidden keyword                                                                                                                                                                                                                                                                                            | Not modelled                                                                                                   | Not filed                                                                |
| `PumpEffect.java:387`                                       | `NoRepetition$` takes `get(0)` of the targets and throws with none                                                                                                                                                                                                                                                                                                                 | `RandomKeyword$ NoRepetition$` errors with no target                                                           | Not filed                                                                |
| `PlayerControllerHuman.java:1864-1871`                      | `MayEffectFromOpeningHand`: `break` after the first matching SA of a host drops a second one for humans, while the AI keeps both. Latent: one per card                                                                                                                                                                                                                             | Opening-hand effects not ported                                                                                | Not filed                                                                |

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
(`DamageResolveEffect.java:29`) or `RepeatEach` (`RepeatEachEffect.java:199-201`), neither of which carries the param.
Volcano Hellion's damage ("can't be prevented") stays preventable. Crucible keeps the line refused
(`DealDamage: NoPrevention$ under DamageMap$`) rather than reproduce it. Fix upstream: copy the flag onto the damage map
or read it per source SA.

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

`ChangeZoneEffect.java:1416-1431` (hidden origin, Destination Exile) reads `ExileFaceDown$` only; `FaceDown$` is read
for a battlefield destination (`:1396`) and on the known path (`:757`). Tezzeret's Reckoning (`FaceDown$ True`, Origin
Library) exiles its three cards face up. Crucible refuses `FaceDown$` on ChangeZone until this is fixed upstream.

`CharmEffect.java:43-44` judges a mode's `getNumCandidates` before `setChoosingPlayer` (`:273`), so a mode whose
`ValidTgts$` names `ChoosingPlayer` with `TargetMin > 0` is filtered with no chooser. `AbilityUtils.java:1131-1132` adds
the root's chooser unguarded: `Defined$ ChoosingPlayer` outside a Charm adds null. No corpus line hits either (Fatal
Lore's mode has `TargetMin$ 0`; all four `ChoosingPlayer` lines sit in Charm modes).
