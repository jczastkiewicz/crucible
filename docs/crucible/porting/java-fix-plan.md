# Java Fix Plan

- **Status:** Active

How the rows of [forge-java-defects.md](forge-java-defects.md) become pull requests against
[Card-Forge/forge](https://github.com/Card-Forge/forge). PORT-8: a Forge bug is fixed upstream, never compensated in Go.
Sizes are estimates from the row text (XS up to 5 lines, S up to 30, M more or a design decision); each site is re-read
on `upstream/master` before work, since line numbers move.

## Packages

One package is one pull request: one theme, small enough to review in one sitting.

| #   | Package                                             | Sites                                                                                                                                            | Size | Notes                                                   |
| --- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ---- | ------------------------------------------------------- |
| 1   | Null and empty-list guards: combat and zones        | `SpellAbilityEffect:797`, `Player:2642` (planeswalk), `ChangeZoneEffect:532`                                                                     | S    | One line each                                           |
| 2   | Null guards: effect params                          | `ChangeTargetsEffect:52`, `PumpEffect:387`, `SpellAbility:2153`, `SetStateEffect:31`                                                             | S    | Four of the seven rows; the rest are issues below       |
| 6   | ChangeTargets and CopySpell                         | `ChangeTargetsEffect:71-106/152`, `CopySpellAbilityEffect:87/176/195`                                                                            | S    | `copies` is shared across spells (Twinning Staff)       |

Order: 1, 2, then 6.

## Issues first, not pull requests

The fix depends on what the authors meant, so open an issue and wait: `StaticAbilityDisableTriggers:109` (missing
`ValidCause$`), `TriggerManaExpend:47` (missing `Amount$`), `SpellAbilityCondition:420` (`ConditionLifeAmount` naming an
SVar other than `X`), `Card:676` (should `NewState$ FaceDown` pass the TurnFaceDown guard), `CostMill:67` (`<` vs `<=`),
Cipher order (`EncodeEffect:57`), `unmeldPosition` (`GameAction:635`), X from `RaiseCost` (`PlaySpellAbility:766`,
`PlayerControllerHuman:374`), `Count$Presence_<Type>` (`AbilityUtils:2186`), `numCardsInHandStartedThisTurnWith`
(`GameAction:2398`), `TextBoxExchangeEffect:74`, `AbilityUtils:3090` (regex without a right boundary),
`SacrificeAllEffect:66`, `TriggerDamageDoneOnce:67`, `TargetRestrictions:53` (CopyForEachCanTarget), `CombatUtil:532`
(hidden Unblockable), `ChangeZoneEffect:961` (OriginAlternative takes an opponent's graveyard), `TriggerChaosEnsues`,
`RunChaosEffect`, `AlterAttributeEffect:87`, `CardUtil:345`, `DamageDealEffect:248` and `DamagePreventEffect:105` (what a radiance copy receives from a divided amount), `ChangeTargetsEffect:71-106` (`TargetRestriction$` on `ChangeSingleTarget$`/`DefinedMagnet$`), `StaticAbilityCantAttackBlock:269`, `FlipOntoBattlefieldEffect:146` (random visual layout, no way to test), `ChangeZoneEffect:1090` (is the `Searched$` reset intended).

## Left out

| Item                                                                                                       | Reason                                                                                                                                                                                                                                      |
| ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AiCostDecision`, `PlayerControllerAi:246`, `MayEffectFromOpeningHand` in `PlayerControllerHuman`          | AI or UI code, nothing Crucible reads                                                                                                                                                                                                       |
| `mana_clash.txt`, `sword_of_dungeons_dragons.txt`                                                          | Card scripts: `card-script-defects.md`                                                                                                                                                                                                      |
| `Player:3435`                                                                                              | Fixed upstream (`8cd226407b1`)                                                                                                                                                                                                              |
| `Untap:151-155` (untap during another player's step)                                                       | Not a defect: bypassing `UntapAdjust`, `OnlyUntapChosen`, the optional prompt and `doesn't untap` is what the rulings of Seedborn Muse and Prophet of Kruphix say                                                                           |
| `Untap:132` (`UntapAdjust` asks again on a null choice)                                                    | Not a defect: null only comes from a paused netplay GUI or a cancelled reply when AI takes over; `Untap` reads `getController()` on every pass, so the loop ends on reconnect or takeover (`FServerManager.resumeAndResync`, `convertToAI`) |
| `PlayEffect:312/389` (`AllowRepeats$` re-offers an unplayable card)                                        | Latent (both real lines filter it out) and only an AI could keep picking it; #12373 closed as low priority. Crucible refuses the shape                                                                                                      |
| `Player:904-913` (`CounterRemoved` for player counters)                                                    | No card has a `CounterRemoved` trigger for a player counter, and the trigger's `Player` param means the remover for cards but the owner for players: a design call for the authors, once a card needs it                                    |
| `Player:584-592`, `Card:4787`, `ConniveEffect:73`, `ExploreEffect:65`, `Player:3928` (`Updated` read back) | No replacement in the corpus returns `Updated` for PayLife, Tap, Connive, Explore or Learn, and the events have no param an `Updated` result could change                                                                                   |

## Procedure per package

1. Branch `upstream-pr/<slug>` off `upstream/master`, holding only tests and fix, never a change under `crucible/` or
   `docs/`.
2. Per site: a test that reproduces the defect first (seen failing), then the fix (seen passing);
   `mvn -pl forge-game -am compile` after each change; only the new tests run, never the whole suite.
3. English PR body: the site, one sentence on the defect, one on the fix.
4. Push and open the PR only after the user approves that package.
5. On `master`, cherry-pick the package's fix commits (not the tests) in one commit with one row per file in **Pending
   upstream fixes** of `upstream-patches.md`, and set the defect row's "Upstream" cell to the PR number. When a sync
   brings the fix back, make Go match upstream's fix, remove its refusal and delete the row (DOC-16).
