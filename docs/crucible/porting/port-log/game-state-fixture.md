# Port: GameState fixture format

- **Java source:** `forge-game/src/main/java/forge/game/GameState.java` (1,432 LOC) — `parse`/`parseLine`,
  `processCardsForZone`, `applyGameOnThread`, `toString`, and `PhaseType.smartValueOf`
- **Go target:** `crucible/internal/fixture`
- **Status:** Text parse, `Load` and `Dump` done for a core slice — M4. The per-card annotation long tail, tokens, and
  every `Player` field with no `engine.Player` home yet stay text-only; see "Not ported yet"

## What it does

Reads a fixture's `key=value` lines into a `State` (`Parse`), builds an `*engine.Game` from one (`Load`), and writes a
`*Game` back out (`Dump` to a `State`, `Write` to text). TEST-5 adopts this format verbatim so the same fixture runs
against the Java oracle — the whole reason to use Forge's own reader rather than inventing one.

The split mirrors `internal/carddb`'s own two stages (PORT-2): `Parse` keeps zone contents as raw
`Name|Set:X|Tapped:True;...` text, because turning that into `Card`s needs `compile.DB` and a `*Game` to allocate into.
`Load` is that second stage. `Dump` and `Write` are its inverse, and together the four make the P3 exit gate an actual
test: `TestDumpRoundTripsThroughParse` runs a fixture through `Parse → Load → Dump → Write → Parse → Load` and checks
every field the second `Loaded` produces against the first.

## `Loaded` no longer carries turn state

`Turn`, `ActivePlayer` and `ActivePhase` used to live on `Loaded` rather than `engine.Game`, because M5's turn/phase
loop had not landed and guessing its shape early was exactly the kind of speculative design CLAUDE.md rules out.
`turn.go`'s `StartTurn`/`AdvancePhase` gave them a real home (`porting/port-log/game-state.md`'s "turn structure"
section), so `Load` now calls `Game.SetTurnState` and `Dump` reads `Game.Turn`/`ActivePlayer`/`ActivePhase` back —
`SetTurnState` is `Load`'s tool for this the same way `devAdvanceToPhase` is Java's: a state injection, not a step
`AdvancePhase` would actually run.

`Loaded` keeps only what still has no `Game` equivalent: `ActivePhaseAdvance`/`PhaseAdvanced`, a fixture-only "advance
once more after setup" directive that is parsed but not yet applied, and `Unapplied`.

One consequence of the move: `Game` has no "phase is unset" state of its own — a real game always has some active phase
once turn state is initialised. `Dump` only writes `activephase=` when `ActivePlayer` is also set, so a fixture that
names no active player does not gain a phantom `activephase=Untap` line it never wrote; one that does gain an explicit
phase on its first dump, stable from then on.

## Load builds the game itself

Java's `GameState.applyToGame` runs against a `Game` a `Match` already built, with its players already seated —
`GameState` only fills in state on players that exist. Crucible has no `Match` yet, so `Load` seats the players itself,
one per `Named` slot, in slot order (human, ai, p2..p9) — the same order every fixture already writes in. This is a
structural difference from Java (PORT-1), not a data one: the seating order a fixture implies is unchanged.

## Two-pass resolution

`AttachedTo:`, `RememberedCards:` and `Imprinting:` can name a card declared later in the same fixture — Rancor
attaching to a creature written after it in the file has to work. `Load`'s `loader` type collects these as it creates
cards and resolves them once every card in the fixture exists (`resolveRefs`), the same two-pass shape
`GameState.applyGameOnThread` uses (`idToCard`, `cardToAttachId`, ...).

The three lists are ordered slices, not maps keyed by card, because two auras naming the same host must attach in the
order the fixture wrote them: that order is what breaks a tie between their continuous effects when both share a
timestamp (GO-12). `RememberedCards:`/`Imprinting:` have no such cross-card ordering concern — each card's `Memory` is
independent — but the slices are ordered anyway, once discipline was already needed for attachments.

## `Id:` is the card's own handle

Java hands out arbitrary per-fixture integers and only writes one when something else references that card
(`cardsReferencedByID`). Go always writes `Id:<CardID>` and always uses the handle the card already has: there is
nothing to gain from a fixture-chosen numbering scheme `Load` would have to remember separately, and
`Parse(Write(Dump(x)))` does not need to reproduce `x`'s own numbers to prove the round trip works — only that the
references resolve to the same cards again, which a fresh handle does just as well as an old one.

## Compiled cards needed a name back

`Dump` writing a card's name needs to ask `Def.Name`, and `compile.Card` did not carry one: it holds `Filename` (a
snake_case file stem, not a printed name) and the compiled `Faces`, because nothing before this needed to go from a
compiled card back to the string that named it. Added `Name string`, set from the primary face during `Compile` — a
one-line, backward-looking fix rather than new scope, and it does not touch `WriteCanonical`/`Fingerprint`, so the
golden AST diff (M3's P2 gate) is unaffected.

`compile.NewDB` is the other small addition this needed: `LoadDB` is the only prior constructor and always walks a real
corpus directory, which is wrong for a test that wants three known cards and nothing else. `NewDB` builds a `*DB` from
an already-compiled `map[string]*Card`.

## Deviations from Java

| Java                                                                                                         | Go                                                                                                                                                                                                                                                                                             |
| ------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `splitLine` throws on a blank line (`line.charAt(0)`)                                                        | A blank line is skipped. Reproducing a crash would make blank lines unwritable in Crucible's own fixtures for no benefit                                                                                                                                                                       |
| Unrecognised category prints to stderr and is dropped                                                        | Returned in `Unknown` (top-level keys) or `Loaded.Unapplied` (card annotations and player fields with no engine home), so a fixture silently testing nothing is a test failure instead of a log line                                                                                           |
| A malformed number (`humanlife=twenty`) throws and dies                                                      | Returned as an `error` naming the line, not a panic — a card script cannot cause this, but a typo in a hand-written fixture is exactly the kind of caller mistake `error` is for (GO-7)                                                                                                        |
| `PhaseType.smartValueOf` matches the script name or the enum constant name, case-insensitively, off one list | `phaseByName` tries `engine.PhaseByName` (script name, case-sensitive) first, then a second table of Java's enum constant names, because `engine.PhaseByName`'s contract is specifically the script vocabulary and `GameState.toString`'s own dump (bare `Enum#toString`) writes the other one |
| `RemoveSummoningSickness` is state `GameState` remembers                                                     | It is a one-time load directive. By the time `Load` returns, every card it applied to already has `SummonSick` false, and `Dump` writes that per card; re-emitting the directive would assert something `Dump` cannot actually know                                                            |
| `Id:` is arbitrary, chosen by whoever wrote the fixture                                                      | Always the card's own `CardID` (see above)                                                                                                                                                                                                                                                     |

## Pinned quirks (PORT-7)

- **`p10life` addresses player 1, not player 10.** `getPlayerState(key)` does
  `Integer.parseInt(String.valueOf(key.charAt(1)))` — one digit, always. A fixture that means player 10 cannot be
  written in this format, in either engine.
- **The first `=` splits, the rest of the value does not.** `humancounters=POISON=3` parses correctly by accident of
  this rule, not because counters get special handling.
- **Keys fold, values do not.** Both engines lowercase the key before matching and leave the value exactly as written.
- **`Tapped` and `SummonSick` match on prefix, no colon required.** `info.startsWith("Tapped")` accepts `Tapped`,
  `Tapped:True`, or in principle `TappedFoo` — reproduced because a fixture written either way has to load the same in
  both engines.

## Not ported yet

| Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Lands                                  |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| Token cards (`t:`/`T:` entries) — need `TokenInfo`/`AbilityFactory`, neither built                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | M5-M6                                  |
| The rest of the per-card annotation grammar: `Renowned`, `Solved`, `Saddled`, `Suspected`, `Monstrous`, `PhasedOut`, `FaceDown`, `Transformed`/`Modal`/`Flipped`/`Meld`, `OnAdventure`, `IsCommander`, `IsRingBearer`, `EnchantingPlayer:`, `Ability:`, `ChosenColor:`/`ChosenType:`/`ChosenType2:`, `ChosenCards:`, `MergedCards:`, `NamedCard:`, `ExecuteScript:`, `ExiledWith:`, `Attacking`, `NoETBTrigs`, `Foretold`/`ForetoldThisTurn`, `IsToken`, `ClassLevel:`, `UnlockedRoom:` — each needs a mechanic or a type (`CardState`, `SpellAbility`, combat) this port has not reached | M5-M6, mechanic by mechanic            |
| Player-level `Counters:`, `ManaPool:`, `PersistentMana:`, `LandsPlayed[LastTurn]:`, `NumRingTemptedYou:`, `Speed:` — `engine.Player` has none of these fields yet                                                                                                                                                                                                                                                                                                                                                                                                                         | M4-M5, as each field lands on `Player` |
| `ability<key>=` string values are stored verbatim in `AbilityStrings`; nothing parses or resolves them (puzzle-mode precast targeting)                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Puzzle mode, if ever                   |
| `[metadata]` section (puzzle-mode name/description)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Puzzle mode, if ever                   |
