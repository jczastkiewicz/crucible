# Port Log — Game State: M6 Effects: Play and CopySpellAbility land

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine)
- **Research record:** [`effects-play-copyspellability.md`](effects-play-copyspellability.md) — corpus shapes, the
  deferral this file closes
- **Design:** [ADR-0018](../../../adr/0018-instant-sorcery-spell-object.md) — built on master's stack-item identity
  (`Ability.ID`, `PushAbility`) and `castInstantOrSorcery`, not a second version of either

## Stack and casting pieces both APIs share

Each is the smallest change to an existing piece, not a parallel mechanism. Reason for each in its row.

| Piece                                          | Where                                                             | Why                                                                                                                                                                                                       | Java                                                  |
| ---------------------------------------------- | ----------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `castSpell(controller, pid, card, castOpts)`   | `castspell.go`; `CastSpell` = timing/hand gates + this            | One cast path from any zone for all three spell shapes; Play casts through it, `castWithoutPaying` (Discover) now calls it                                                                                | `PlaySpellAbility.playSpellAbility`                   |
| `castOpts.withoutManaCost`, `payCastCost`      | `castspell.go`                                                    | `WithoutManaCost$`: nothing paid, X is 0, `ChoosePayX` never asked                                                                                                                                        | `SpellAbility.copyWithNoManaCost`                     |
| `putSpellOnStack`                              | `castspell.go`                                                    | Caster becomes the spell's controller (CR 110.2). `Move` never set it, so an opponent's card cast by Play resolved under the opponent                                                                     | `MagicStack.add`'s `source.setController(activator)`  |
| `Move`/`MoveToLibraryTop` controller reset     | `game.go`                                                         | Spell leaving the stack for anywhere but the battlefield returns to its owner's control (CR 108.4a)                                                                                                       | new `Card` object per zone                            |
| `Ability.spell`                                | `ability.go`; set by every cast path and `copySpell`              | Card in the Stack zone does not identify its spell entry: a "when you cast this spell" trigger's `Source` is the same card, pushed above it                                                               | `SpellAbility.isSpell`                                |
| `spellItemOf`, `stackItem`                     | `stack.go`                                                        | Card → its spell's `StackItemID`; ID → the live stack item                                                                                                                                                | `MagicStack.getInstanceMatchingSpellAbilityID`        |
| `triggeredObjects.spellAbility`                | `ability.go`; set by `checkSpellCastTriggers` (`trigger.go`)      | `Defined$ TriggeredSpellAbility`. Read from the top of the stack before any trigger is pushed; no signature change. `resolveSubAbility` already carries `triggered` down the chain                        | `AbilityKey.SpellAbility`                             |
| `Card.IsCopiedSpell`, `ceaseCopiedSpell`       | `card.go`, `game.go` (called first by `Move`, `MoveToLibraryTop`) | Copy moving anywhere is removed silently: no timestamp, no `ZoneChanged`, no trigger. Parked in owner's `None` zone like a ceased token. Both move primitives call it: `MoveToLibraryTop` bypasses `Move` | `GameAction.changeZone` (`GameAction.java:100-105`)   |
| `copyBecomesToken`                             | `castspell.go`, first line of `permanentEffect`/`attachEffect`    | Copy of a permanent spell resolves as a token (CR 111.11), so its `Move` onto the battlefield is not a cease                                                                                              | `GameAction.java:96-98`                               |
| `targetChoiceFor`                              | `targeting.go`; `resolveTargets` = this + `ChooseTargets`         | One candidates-and-bounds scan shared by casting and a copy's new targets                                                                                                                                 | `TargetRestrictions.getAllCandidates`                 |
| `APICopySpellAbility` → `stackSpellCandidates` | `targeting.go`                                                    | `ValidTgts$` without `TargetType$` names spells (Mischievous Quanar's `Instant,Sorcery`); the battlefield scan found none                                                                                 | `CopySpellAbilityEffect.buildSpellAbility` (`:28-33`) |
| `playLandNow`, `hasLandDrop`                   | `land.go`; `PlayLand` = gates + these                             | Play's land option: any zone, caster's turn, a drop left (CR 305.3)                                                                                                                                       | `Player.playLandNoCheck`, `canPlayLand(…, true, …)`   |
| `Game.Clone` copies `nextStackItemID`          | `game.go`                                                         | Crucible bug: a clone restarted at 0 and reused IDs already on its stack. Regression `TestCloneContinuesStackItemIDs` (`clone_test.go`)                                                                   | —                                                     |
| `queue confirmeffect <bool>`                   | `internal/fixture/actions.go`                                     | Scenario verb for `ConfirmEffect`; Play's single option and CopySpellAbility's `Optional$`/`MayChooseTarget$` ask it                                                                                      | —                                                     |

No new `PlayerController` method. Reason: every decision both APIs ask maps onto an existing one (tables below).

Inherited, not changed: `castInstantOrSorcery` passes `isSpellSource` false to `checkBecomesTargetTriggers` (only an
Aura is a spell source there, since `becomesTargetSourceMatches` reads `.Aura` as always true under one). CastSpell
offers no alternative cost (flashback, evoke, kicker); a Play that pays casts the basic spell only, like `CastSpell`.

## Play lands

`playeffect.go`, `PlayEffect.java:84-493`, `AbilityUtils.getSpellsFromPlayEffect` (`AbilityUtils.java:2910-2979`). Casts
during resolution, timing ignored (CR 608.2g), through `castSpell`: an instant or sorcery Play casts sits above the
resolving Play and resolves next. Corpus: 330 lines.

| Param                                                                  | Resolved as                                                                                                                                   |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `Valid$` + `ValidZone$` (default Hand)                                 | zone by zone, each across every player (`Game.getCardsIn(Iterable)`), activator as "You" (`filterListByType`)                                 |
| else `Defined$` (default Self) / `ValidTgts$`                          | `targetedOrDefinedCards`                                                                                                                      |
| `ValidSA$`                                                             | `validSAMatches`: heads `Spell`, `SpellAbility`, `LandAbility`/`Ability`/`Static` (the land play), `Instant`/`Sorcery`; `!`; properties below |
| `Controller$`                                                          | first `definedPlayers` answer; none is an error (Java's `.get(0)`)                                                                            |
| `Amount$` (`All` = every candidate)                                    | `optionalAmount`                                                                                                                              |
| `Optional$`                                                            | `ChooseCardsForEffect` lo 0; one candidate with `Amount$` 1 asks `ConfirmEffect` instead (`singleOption`)                                     |
| `WithoutManaCost$`                                                     | `castOpts.withoutManaCost`; without it the cost is paid and a "no cost" card skipped (`:387-389`)                                             |
| `CopyCard$`                                                            | token copy (`IsToken`) of the chosen card in its zone, cast instead; CR 704.5d removes it once off the stack                                  |
| `AllowRepeats$`                                                        | the pick stays a candidate                                                                                                                    |
| `RememberPlayed$`/`ImprintPlayed$`/`ForgetPlayed$`/`ForgetRemembered$` | host memory after a successful play; `ForgetPlayed$` forgets the chosen original (`tgtCard`, `:473`)                                          |
| `ShowCardToActivator$`                                                 | no-op: `revealTo` is display-only                                                                                                             |
| land chosen                                                            | `playLandNow` if the caster's turn and a drop is left, then `ChangesZoneAll`                                                                  |
| spell cast                                                             | `castSpell`, then `ChangesZoneAll` origin → Stack (`triggerList.triggerChangesZoneAll`, `:476-481`)                                           |

`ValidSA$` properties (`SpellAbilityProperty.java:245-320`): `YouCtrl` true (caster is "You"), `OppCtrl` false,
`cmc<op><X>` against the card's mana value with the operand through the host's SVars (`resolveNamedAmount`), anything
else the card's own property through `Matches`.

Decisions: `ChooseCardsForEffect` (pick), `ConfirmEffect` (single option), then the cast's own (`ChooseTargets`,
`ChooseEnchantTarget`, Charm modes, mana payment).

Rejected before acting (`playUnresolvedParams`):

| Param                                                                                   | Reason                                                                                                  |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| `ReplaceGraveyard$`/`ReplaceGraveyardValid$` (35 lines)                                 | effect-card replacement on the spell's own Stack → Graveyard move, which has no hook (ADR-0018 point 3) |
| `TgtZone$` (33)                                                                         | `resolveTargets` scans the battlefield only, so a pushed target came from the wrong zone                |
| `ConditionDefined$` (20), `Condition$` (1)                                              | `subAbilityConditionMet` reads either as never met, silently                                            |
| `CopyFromChosenName$`, `AnySupportedCard$`, `RandomCopied$`, `RandomNum$`, `ChoiceNum$` | card built from outside the game                                                                        |
| `CastFaceDown$`, `CastTransformed$`, `ReplaceIlluMask$`                                 | alternate states                                                                                        |
| `PlayCost$`, `PlayReduceCost$`, `PlayRaiseCost$`, `ManaConversion$`                     | alternative or modified costs                                                                           |
| `ControlledByPlayer$`, `WithTotalCMC$`, `ShowCards$`, `ZoneRegardless$`                 | each its own mechanic                                                                                   |

Also an error, never a silent empty pool or a guess (GO-7):

| Case                                                                                                                                                            | Where                                 |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| `Valid$` property `Matches` has no case for: `ExiledWith…` (23), `TargetedPlayerCtrl` (6), `OwnedBy`/`ControlledBy` (8), `shares…`, `named…`, non-literal `cmc` | `playSpecGap`                         |
| `ValidSA$` cmc operand not resolvable, or card property with no case                                                                                            | `validSAPropertyMatches`              |
| chosen split/adventure/omen/modal/prepare card (a choice of spells, `getAbilityToPlay`, no decision here)                                                       | `playCastGap`                         |
| chosen instant/sorcery with no `A:SP$` line, two of them, or `Cost$` on it (an additional cost `castInstantOrSorcery` does not pay)                             | `playCastGap`                         |
| a card with nothing to play, or no mana cost to pay, under `AllowRepeats$`                                                                                      | `playRepeatLoop`; Forge defect, below |

The `ValidSA$` pre-filter keeps a card `playCastGap` names (Java would offer it); only choosing it fails.

Not ported, no corpus line affected: `equalsWithGameTimestamp` on targeted cards (a `CardID` is stable across zones),
`XMin$` on the cast spell under `WithoutManaCost$` (`:365`), `getAbilityToPlay`'s cancel (`:327-333`).

`TestSubAbilityChainUnimplementedAPIErrors` chains into `Phases` as its unbuilt API, since `Play` resolves.

| Test                                                  | Proves                                                                                                                                                                                                                |
| ----------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| scenario `play-casts-a-milled-instant-without-paying` | Jace's Mindseeker mills Lightning Bolt, casts it from the opponent's graveyard free under its controller; Bolt resolves, returns to its owner's graveyard                                                             |
| `playcasting_test.go`                                 | stack object and cast trigger, decline, optional stop, `ValidSA$` cmc filter and offer order, zone-major scan, `CopyCard$`, land drop and turn, `Amount$ All`, control, targets and Aura, rejections, `AllowRepeats$` |

**Forge bug (PORT-8).** `PlayEffect.java:312`/`:389` `AllowRepeats$` re-offer loop, latent
([`forge-java-defects.md`](../../forge-java-defects.md)).
