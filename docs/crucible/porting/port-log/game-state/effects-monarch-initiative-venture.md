# Effects: BecomeMonarch, TakeInitiative, Venture

Designations and dungeons: Command-zone cards whose own triggers run the mechanic. Batch file for the three APIs
(DOC-12, `port-effect` step 5).

---

## BecomeMonarch lands

`BecomeMonarch` (64 corpus lines) resolves: each targeted or `Defined$` player (default the activator) still in the game
becomes the monarch (CR 724), unless a `Mode$ CantBecomeMonarch` static names them. `becomemonarcheffect.go`, ported
from `BecomeMonarchEffect.java`, `GameAction.becomeMonarch` (`GameAction.java:2536-2555`) and
`Player.createMonarchEffect`/`removeMonarchEffect` (`Player.java:3438-3481`).

**The Monarch is an effect card built in Go, not a parsed script.** Java builds it in `createMonarchEffect` from two
trigger strings; this port builds the same `compile.Ability` trees directly (`monarchEffectDef`), the way
`earthbendReturnTrigger` already builds Earthbend's delayed triggers. Nothing is parsed at runtime (PORT-2), and the
card needs no database entry, so every `NewGame(nil, ...)` test game can hold one.

| Trigger                                                                                                        | Effect                                                    |
| -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `Mode$ Phase \| Phase$ End of Turn \| TriggerZones$ Command \| ValidPlayer$ You`                               | `DB$ Draw \| Defined$ You`                                |
| `Mode$ DamageDone \| ValidSource$ Creature \| ValidTarget$ You \| CombatDamage$ True \| TriggerZones$ Command` | `DB$ BecomeMonarch \| Defined$ TriggeredSourceController` |

The card is an `IsEffect` Command-zone card, so `traitHosts` and `checkPhaseTriggers` already run its triggers there
(the `Effect` port's machinery). Its lifetime is `effectPermanent`: the designation, not a `Duration$`, removes it.

**One card per player, reused.** `Player.monarchEffect` holds the player's card for the whole game, as Java's
`Player.monarchEffect` field does. Losing the monarchy parks it in `None` (`exileEffect`); becoming the monarch again
puts the same card back. Reason: making a new card per swap grows the `CardID` arena (never freed, ADR-0009) with every
monarch change.

**`becomeMonarch` keeps Java's order.** The previous monarch's card leaves first; then, if a `CantBecomeMonarch` static
stops the new player, nothing else happens and `Game.monarch` still names the previous monarch
(`GameAction.java:2542-2548`). Otherwise the card enters, `Game.monarch` is set, and `Mode$ BecomeMonarch` triggers run.

### New engine state

| State                                                                      | Why                                                                                                                                         |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `Game.monarch`, `Game.monarchBeginTurn`                                    | `Game.monarch`/`monarchBeginTurn`; the second is set as each turn begins (`PhaseHandler.java:848`) for `Mode$ BecomeMonarch`'s `BeginTurn$` |
| `Player.monarchEffect`                                                     | The one reused card, above                                                                                                                  |
| `Player.lossHandled`                                                       | `Game.onPlayerLost` runs once per loser; without the flag a monarch the successor cannot replace would be re-passed on every later SBA pass |
| `Ability.triggered` (`triggeredObjects`: source, sourceController, player) | Java's triggering-objects map, the keys ported modes set. Carried to every chained sub-ability, as `TriggerRemembered` is                   |

`Game.Clone` copies both `Game` fields; the `Player` and `Ability` fields are values copied with their structs.

**`Defined$ Triggered*` is new and fails closed.** `definedPlayers` reads `TriggeredSource`, `TriggeredSourceController`
and `TriggeredPlayer` off `Ability.triggered`. A trigger mode that does not record the key leaves it zero, and the
reference is an `error` (`the trigger recorded no source`), never an empty player list. Set today:

| Mode                          | Key recorded                                                                                                                        |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `DamageDone` (card or player) | Source = the damage source; sourceController read at damage time. Java stores an LKI copy (`TriggerDamageDone.java`'s `getLKICopy`) |
| `BecomeMonarch`               | Player = the new monarch (`TriggerBecomeMonarch.setTriggeringObjects`)                                                              |

A card source named as players (`Defined$ TriggeredSource` on a `DamageDone` trigger) is rejected:
`AbilityUtils.addPlayer` over a card is a shape no ported line needs.

**`Mode$ BecomeMonarch`** (5 corpus lines) runs through `playerActionTriggerMatches`, `checkPlayerActionTriggers`' walk
split out with a per-mode gate: `ValidPlayer$` against the new monarch, `BeginTurn$` against `Game.monarchBeginTurn`
(nobody matches nothing, as `matchesValid` on `null`). `Static$ True` lines (1, Palace Jailer's "exiled until an
opponent becomes the monarch") are skipped: no mode here resolves a trigger off the stack.

**`Mode$ CantBecomeMonarch`** (1 line, Jared Carthalion, reached through `DB$ Effect | StaticAbilities$`):
`ValidPlayer$` through `matchesPlayerSpec`, absent naming every player. A static carrying any other param (a
`checkConditions` shape) or a `ValidPlayer$` `matchesPlayerSpec` cannot read is an `error` (`noPlayerStatic`).

**CR 724.4, the monarch leaving the game** (`Game.java:988-996`): `onPlayersLost`, called from `CheckStateBasedActions`
after CR 704.5a-c, passes the monarchy once per loser -- to the active player, or to the next player in turn order when
the loser is the active player. It runs before the game-over check, as `checkGameOverCondition` does, so a two-player
game's winner ends as the monarch.

**Engine fix: `playersInAPNAPOrder` with a lost active player.** It walked `nextPlayerAfter` round to the active player,
which `nextPlayerAfter` never returns once that player has lost -- an infinite loop the first time a trigger fired in
the SBA pass that eliminated the active player. It now starts at the next player still in the game, as
`MagicStack.java:835-838` does. `TestMonarchPassesWhenTheMonarchLoses` is the regression test.

**Rejected** (`error` before acting): `ConditionDefined$` (1 line) -- `subAbilityConditionMet` reads it as unmet, which
would skip the effect silently; `Defined$ TriggeredTarget` (1) -- `DamageDone`'s Target key is not recorded.

**Fixture key `monarch=`** (Crucible-only, `lost=`/`won=`/`over=`'s pattern, `game-state-fixture.md`). Java's
`GameState` dumps the monarch's effect card by name into the command zone and cannot load it back; `monarch=<player>`
writes the designation, `Load` recreates the card through `Game.SetMonarch`, and `Dump` leaves designation cards out of
the command zone. `compareGames` compares the monarch by name. Scenario:
`testdata/scenarios/monarch-draws-at-end-step-and-passes-by-combat-damage`.

**Follow-ups, not in this batch.** `Player.isMonarch`/`Count$...Monarch` valid properties stay unresolved: turning them
on changes statements in `layers.md`, `triggers.md`, `trigger-modes.md` and `replacement.md`, and nothing here needs
them.

**Forge `getMonarchSet` (PORT-8, `forge-java-defects.md`).** No Crucible counterpart: this port carries no set codes,
and `becomeMonarch` takes none.
