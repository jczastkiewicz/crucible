# Java Fix Plan

- **Status:** Active

How the rows of [forge-java-defects.md](forge-java-defects.md) become pull requests against
[Card-Forge/forge](https://github.com/Card-Forge/forge). PORT-8: a Forge bug is fixed upstream, never compensated in Go. Sizes are estimates from the row text (XS up to 5 lines, S up to 30, M more or a design decision); each site is re-read on `upstream/master` before work, since line numbers move.

## Packages

One package is one pull request: one theme, small enough to review in one sitting.

| #  | Package                                             | Sites                                                                                                                                                                                         | Size | Notes                                                   |
| -- | --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---- | ------------------------------------------------------- |
| 1  | Null and empty-list guards: combat and zones        | `SpellAbilityEffect:797`, `CombatUtil:70`, `Player:2642` (planeswalk), `ChangeZoneEffect:532`, `AbilityUtils:1129` (DefendingPlayer)                                                          | S    | One line each; some latent                              |
| 2  | Null guards: effect params                          | `ChangeTargetsEffect:52`, `PumpEffect:387`, `StaticAbilityDisableTriggers:109`, `TriggerManaExpend:47`, `SpellAbility:2153`, `SpellAbilityCondition:420`, `SetStateEffect:31` with `Card:676` | S    | Several latent: no corpus card reaches them yet         |
| 3  | Collection mutated while iterating, fixed-size list | `CamouflageEffect:78`, `CountersPutEffect:250` (DividedRandomly), `ChooseColorEffect:44/55/84`                                                                                                | XS   | Plain crashes                                           |
| 4  | Infinite loops                                      | `Untap:132`, `PlayEffect:312/389` (AllowRepeats)                                                                                                                                              | S    | `PlayEffect` needs `amount--` on the `continue` branch  |
| 5  | Loop-variable and condition slips                   | `SpellAbilityCondition:450` and `:465`, `DamageDealEffect:248`, `DamagePreventEffect:105`, `ChangeTargetsEffect:142`                                                                          | S    | Copy-paste bugs; `:465` changes the condition's meaning |
| 6  | ChangeTargets and CopySpell                         | `ChangeTargetsEffect:71-106/152`, `CopySpellAbilityEffect:87/176/195`                                                                                                                         | S    | `copies` is shared across spells (Twinning Staff)       |
| 7  | SwitchBlockEffect                                   | `:22-25`, `:97/103/151/157`, `:88`                                                                                                                                                            | S    | One class, three defects                                |
| 8  | Small rules fixes                                   | `VoteEffect:93`, `GameAction:2568` (takeInitiative), `RestartGameEffect:47`, `FlipOntoBattlefieldEffect:109`, `StaticAbilityCantAttackBlock:269`                                              | S    | Independent, 1-5 lines each                             |
| 9  | xCount/doXMath hardening                            | `AbilityUtils:3257`, `:1584`, `:3310/3321`, `Game:1430`                                                                                                                                       | XS   | All latent                                              |
| 10 | Replacement result read back                        | `Player:584-592` and `Card:4787` (PayLife, Tap `Updated`), `ConniveEffect:73`, `ExploreEffect:65`, `Player:3928` (Learn)                                                                      | S    | Changes when an effect is skipped                       |
| 11 | CounterRemoved trigger                              | `Player:904-913` (commented out under a TODO)                                                                                                                                                 | M    | Energy and rad counters                                 |
| 12 | QuasiLibrarySearch and Searched                     | `ChooseCardEffect:262-269` and `:243-260`, `ChangeZoneEffect:1090`                                                                                                                            | S    | The last is only suspected                              |
| 13 | Untap of another player's permanents                | `Untap:151-155` (skips `canUntap`, UntapAdjust, OnlyUntapChosen)                                                                                                                              | M    | 17 corpus lines; largest fixable item                   |

Order: 3, 4, 1, 2, 9 (plain crashes, easiest to accept), then 5-8, then 10-13.

## Issues first, not pull requests

The fix depends on what the authors meant, so open an issue and wait: `CostMill:67` (`<` vs `<=`), Cipher order (`EncodeEffect:57`), `unmeldPosition` (`GameAction:635`), X from `RaiseCost` (`PlaySpellAbility:766`, `PlayerControllerHuman:374`), `Count$Presence_<Type>` (`AbilityUtils:2186`), `numCardsInHandStartedThisTurnWith` (`GameAction:2398`), `TextBoxExchangeEffect:74`, `AbilityUtils:3090` (regex without a right boundary), `SacrificeAllEffect:66`, `TriggerDamageDoneOnce:67`, `TargetRestrictions:53` (CopyForEachCanTarget), `CombatUtil:532` (hidden Unblockable), `ChangeZoneEffect:961` (OriginAlternative takes an opponent's graveyard), `TriggerChaosEnsues`, `RunChaosEffect`, `AlterAttributeEffect:87`, `CardUtil:345`.

## Left out

| Item | Reason |
| --- | --- |
| `AiCostDecision`, `PlayerControllerAi:246`, `MayEffectFromOpeningHand` in `PlayerControllerHuman` | AI or UI code, nothing Crucible reads |
| `CardProperty:1522` (`attackingBattle`) | The Java may do what its name says; the defect row is re-checked before any fix |
| `mana_clash.txt`, `sword_of_dungeons_dragons.txt` | Card scripts: `card-script-defects.md` |
| `Player:3435` | Fixed upstream (`8cd226407b1`) |

## Procedure per package

1. Branch `upstream-pr/<slug>` off `upstream/master`, holding only tests and fix, never a change under `crucible/` or `docs/`.
2. Per site: a test that reproduces the defect first (seen failing), then the fix (seen passing); `mvn -pl forge-game -am compile` after each change; only the new tests run, never the whole suite.
3. English PR body: the site, one sentence on the defect, one on the fix.
4. Push and open the PR only after the user approves that package.
5. On `master`, set the row's "Upstream" cell to the PR number. When a sync brings the fix back, make Go match upstream's fix, remove its refusal and delete the row (DOC-16).
