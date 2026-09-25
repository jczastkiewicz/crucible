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
| `DungeonCompleted`            | Player = who completed it (`TriggerCompletedDungeon.setTriggeringObjects`)                                                          |

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

---

## Venture lands

`Venture` (46 corpus lines) resolves: each targeted or `Defined$` player (default the activator) still in the game
ventures into the dungeon (CR 701.49) -- into `Dungeon$`'s dungeon type when named (the initiative's Undercity).
`ventureeffect.go`, ported from `VentureEffect.java` (`getDungeonCard`, `chooseNextRoom`, `ventureIntoDungeon`),
`GameAction.completeDungeon` (`GameAction.java:2783-2790`) and `stateBasedAction_Dungeon` (`:1736-1743`).

**Rooms compile from `K:Dungeon:` (PORT-2).** A dungeon's room SVars are named only by its keyword, so nothing compiled
them. `compile.dungeonRooms` expands the keyword the way `CardFactoryUtil.java:1955-1989` does at card creation: one
`Mode$ RoomEntered | TriggerZones$ Command | ValidCard$ Card.Self | ValidRoom$ <RoomName>` trigger per room, in keyword
order, the room's ability as its `Execute$`, plus `NextRoomName$` (the `RoomName$` values of its `NextRoom$` SVars) on
each room ability. Only the four `res/tokenscripts` dungeons carry the keyword; no `cardsfolder` card does, so the
golden AST is unchanged. A room without `RoomName$`, or leading to an SVar its keyword does not list, is
`compile.ErrBadRoom` -- Java dereferences `null` on both.

| Step                | Port                                                                                                                                                                                                                                                                                                                                                                                  |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Which dungeon       | The player's dungeon in the Command zone, unless its marker is on its last room -- that one is completed first. Otherwise a new one from the token table (`DB.TokenScripts`, new, sorted), `Dungeon$`'s subtype or every enterable dungeon, offered sorted by name through `ChooseOption` (Java's `chooseSingleCardFace` over a `TreeMap`) -- asked even for one option, as Java asks |
| Enterable           | `CardRules.isEnterableDungeon` reads oracle text for "You can't enter this dungeon unless"; `compile.Face` has none, so the matching `K:` line (Undercity's only) stands in                                                                                                                                                                                                           |
| Which room          | Empty marker: the first room. Otherwise the one `NextRoomName$` room, or the player's pick of several through `ChooseAbilitiesForEffect` over room names (Java's `chooseSingleSpellForEffect`)                                                                                                                                                                                        |
| Enter               | `Card.CurrentRoom` set; `Mode$ RoomEntered` triggers run (`playerActionTriggerMatches`, gated on `ValidCard$` and an exact `ValidRoom$` match); `Player.VenturedThisTurn` counts up                                                                                                                                                                                                   |
| Complete (CR 309.7) | SBA `completeFinishedDungeons`, next to `removeTokensOffBattlefield` (Java's same 704.5d loop): marker on the last room and nothing from the dungeon on the stack. The dungeon joins `Player.completedDungeons`, ceases to exist (`None`, as `exileEffect` parks an effect card), `Mode$ DungeonCompleted` triggers run                                                               |

**New engine state:** `Card.CurrentRoom` (`Card.currentRoom`), `Player.VenturedThisTurn` (reset at cleanup, as
`Player.onCleanupPhase` does), `Player.completedDungeons` (deep-copied by `Game.Clone`; nil until the first completion,
so `TestCloneAllocationsStayBounded` holds). `Game.CompletedDungeons` exposes the list.

**Statics and properties.** `Mode$ CantVenture` (Keen-Eared Sentry) shares `noPlayerStatic` with `CantBecomeMonarch`;
its `ValidPlayer$ Opponent.VenturedThisTurn` needed the `VenturedThisTurn` player property (`matchesPlayerProperty`,
`PlayerProperty.java:482`). `Mode$ DungeonCompleted` (4 corpus lines) records `TriggeredPlayer`.

**Rejected:** `ConditionDefined$` (0 corpus lines; rejected for the reason `BecomeMonarch` rejects it). Unported and not
reached by the dungeons: `Count$DungeonsCompleted` and the other dungeon counts, and the Panharmonicon-style
`ValidMode$ RoomEntered` statics.

**No scenario fixture.** A dungeon in the Command zone cannot be written down: `GameState.java` has no current-room key,
and this port's `Load` does not load `T:` token entries. Module tests (`dungeon_test.go`) walk the real Lost Mine of
Phandelver to completion instead.
