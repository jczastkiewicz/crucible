# Port: GameState fixture format

- **Java source:** `forge-game/src/main/java/forge/game/GameState.java` (1,432 LOC) — `parse`/`parseLine`,
  `processCardsForZone`, `applyGameOnThread`, `toString`, and `PhaseType.smartValueOf`
- **Go target:** `crucible/internal/fixture`
- **Status:** Text parse, `Load` and `Dump` done for a core slice (M4); `RunActions` and the scenario harness (M5) run
  against the real corpus, including `lost=`/`won=`/`over=` so a scenario can assert a game actually ended. The per-card
  annotation long tail, tokens, and most remaining `Player` fields stay text-only; see "Not ported yet"

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

Nothing had actually called `LoadDB` against the real corpus before the scenario harness below did, and it turned out
not to work: it compiled each script inside the same pass that parsed it, which fails any `CopyFaceFrom:` card (Bind //
Liberate among them) since that placeholder only resolves once the whole corpus has been read. Fixed in `LoadDB` itself
— `porting/port-log/ability-factory.md`'s own section on it, since the bug was there, not here.

## Scenarios: `actions.log` and the harness

`internal/engine`'s `TestScenarios` (`scenario_test.go`) is TEST-5's directory walk: `setup.state` and `expect.state`
are this package's `Parse`/`Load`, unchanged. What is new is `RunActions` (`actions.go`) — the "ordered, explicit
decisions" Plan Section 3.3 names but does not itself define a format for, because Java's own differential tooling
drives a real `PlayerController` from Java code and never needed a text vocabulary for it. This one is Crucible's own,
line-oriented the same way `setup.state` is:

```text
startturn <player>            Game.StartTurn(player, controller)
advance [n]                   Game.AdvancePhase(controller), n times (default 1)
mulligan <firstplayer>        PerformMulligans(game, controller, firstplayer)
declareattackers              Game.DeclareCombatAttackers(controller)
declareblockers               Game.DeclareCombatBlockers(controller)
firststrikedamage             Game.DealFirstStrikeDamage(controller)
combatdamage                  Game.DealCombatDamage(controller)
queue keephand <bool>         ScriptedController.QueueKeepHand
queue tuck <id>[,<id>...]     ScriptedController.QueueTuck, ids from Loaded.CardByFixtureID
queue startingplayer <p>      ScriptedController.QueueStartingPlayer
queue startinghand <n>        ScriptedController.QueueStartingHand
queue legendarykeep <id>      ScriptedController.QueueLegendaryToKeep, id from Loaded.CardByFixtureID
queue attackers [<id>,...]    ScriptedController.QueueAttackers, ids from Loaded.CardByFixtureID (no ids declines)
queue attacktarget <p>|<id>   ScriptedController.QueueAttackTarget, a player name or a planeswalker/battle's Loaded.CardByFixtureID
queue blocks [<b>=<a>,...]    ScriptedController.QueueBlocks, blocker=attacker pairs from Loaded.CardByFixtureID (no pairs declines)
queue damage <b>=<n>[,...]    ScriptedController.QueueDamageAssignment, blocker=amount pairs from Loaded.CardByFixtureID
queue discard <id>[,...]      ScriptedController.QueueDiscard, ids from Loaded.CardByFixtureID
queue battleprotector <p>     ScriptedController.QueueBattleProtector, a seated player's name
paymanacost <player> <cost>   Game.PayManaCost(player, cost, controller), cost is mana.Parse's own text
tapformana <player> <id> <color> Game.TapLandForMana(player, id, color), id from Loaded.CardByFixtureID
queue paygeneric <shard>      ScriptedController.QueuePayGeneric, a bare shard symbol ("W", "C", ...)
queue hybridmanacolor <color> ScriptedController.QueueHybridManaColor, a bare color letter
queue paymonocoloredhybrid <bool>     ScriptedController.QueuePayMonocoloredHybrid
queue paycolorlesshybrid <bool>       ScriptedController.QueuePayColorlessHybrid
queue payphyrexian <bool>             ScriptedController.QueuePayPhyrexian
queue payhybridphyrexian <color|life> ScriptedController.QueuePayHybridPhyrexian, "life" for the zero mana.Colors answer
```

`queue battleprotector` is a state-based action's own question, not tied to any combat verb:
`Game.CheckStateBasedActions` asks it whenever a Battle has no protector (or its protector has left the game) and
nothing is currently attacking it (`game-state.md`'s "Combat" section, `assignBattleProtector`). A fixture with a Battle
in `setup.state` needs one queued before the first `startturn`/`advance`/`combatdamage`-family action that would run a
state-based-action check, since that first check is what asks. The result is nameable in `expect.state` too —
`Protector:<player>`, Crucible-only the same way `lost=`/`won=`/`over=` are (no Java `GameState` key exists to diverge
from), compared by name the same way everything else two independently loaded games disagree on numerically is.
`testdata/scenarios/battle-protector-assigned-to-opponent` is the example.

`queue discard` is needed only when `advance` reaches `Cleanup` with the active player's hand over `MaxHandSize` (7,
`game-state.md`'s "Turn structure") — a hand already at or under that never asks. There is no `none` shortcut, the same
reasoning `queue damage` has none: `Game.cleanupStep` only asks when there is a nonzero, known count to discard
(`hand.Len() - MaxHandSize`), so declining entirely was never a legal answer to make room for.

`queue attacktarget` is needed only when a declared attacker has more than one eligible target -- a planeswalker or
battle present on the opponent's side, or (multiplayer) more than one living opponent -- one call per such attacker, in
the order `declareattackers` declared them. A lone eligible target (any two-player game with nothing else to attack, the
ordinary case) is assigned automatically without consuming a queue entry; an entry queued for a question that was never
asked is simply left unread, the same as any other over-queued answer (`ScriptedController` has no "everything was
consumed" check of its own).

A scenario with a first striker needs both `firststrikedamage` and `combatdamage`, with an `advance` between them: a
first-strike kill has to actually happen (`CheckStateBasedActions` runs on every phase entry, `game-state.md`'s "Turn
structure") before the regular step asks whether the dead creature still deals or receives anything, and `advance`ing
from the `FirstStrikeDamage` phase into `CombatDamage` is what runs that check — no separate verb exists just for it. A
scenario with nothing carrying "First Strike"/"Double Strike" can skip `firststrikedamage` entirely; calling it anyway
is a safe no-op.

`queue blocks`' pairs are `blocker=attacker`, both `Id:` numbers — `1=2` means the card with `Id:1` blocks the card with
`Id:2`; `1=3,2=3` is a gang block, two blockers on one attacker. `queue damage`'s pairs are `blocker=amount` and, unlike
`queue blocks`, keep the order written: that order is the order `AssignCombatDamage` divides a gang-blocked attacker's
damage in (CR 510.1c), so reordering the pairs would answer a different question. Any of an attacker's power left
unassigned across the pairs tramples over to the defending player if the attacker has trample (CR 702.19c), or is wasted
if not — both are computed after `queue damage`'s answer is applied, not part of what it names. There is no `none`
shortcut for `queue damage` — `Game.DealCombatDamage` only ever asks when an attacker has more than one blocker, so an
empty answer is never itself the legal one the way declining to attack or block is.

`Game.StartTurn`/`AdvancePhase` take `controller` because `CheckStateBasedActions` does now too — the legend rule needs
one (`game-state.md`'s "The legend rule needed `CheckStateBasedActions` to take a controller"), and every path that
reaches a state-based-action check had to gain the same parameter.

`paymanacost` is not tied to any turn/phase/combat verb either, and needs none run first: `Game.PayManaCost` reads and
spends `manapool=`'s own pool directly (`Player.ManaPool`), the same self-contained-rules-chapter reasoning
`manapay.go`'s own doc comment gives for building it before anything casts a spell. `queue paygeneric` is needed once
per unit of the cost's own generic amount, regardless of whether the payment is going to succeed — `PayManaCost` asks
before it ever checks the pool, so a cost with `{2}` asks twice even when the pool cannot cover either answer,
`testdata/scenarios/mana-payment-fails-atomically`'s own shape. Every hybrid and Phyrexian shape now has its own verb
too: `queue hybridmanacolor`, `queue paymonocoloredhybrid`, `queue paycolorlesshybrid` and `queue payphyrexian` mirror
their own `ScriptedController` method's argument shape (a bare color letter or a bool); `queue payhybridphyrexian` takes
either a color letter or the literal word `life` for the zero `mana.Colors` answer (CR 118.4's own three-way choice,
`ChoosePayHybridPhyrexian`'s own doc comment) — `resolveManaColor` (`actions.go`) is the one parser both
`hybridmanacolor` and `payhybridphyrexian` share, since a bare color letter is exactly `mana.ParseShard`'s own pure
shard, taken down to its `Colors()` half.

`tapformana` is the one action verb, not a `queue` kind: unlike every `ScriptedController` answer, there is no decision
here for a controller to make ahead of time — the player, the card and the color are all named directly in the line, the
same "self-contained rules chapter, no turn/phase/combat verb needed first" position `paymanacost` is already in. It
reuses `resolveManaColor` for its own `<color>` argument and `resolveCardIDs` (requiring exactly one id, the same check
`queue legendarykeep` already makes) for `<id>`. `Game.TapLandForMana`'s `bool` return is not asserted, the same
"declined by the rules, not a fixture error" convention `paymanacost` already established —
`testdata/scenarios/mana-payment-tap-land-for-mana` is the one fixture so far, and the first mana-payment fixture where
the paid mana comes from a real card (a corpus `Plains`) rather than `manapool=`.

`Loaded.CardByFixtureID` is the other piece `RunActions` needed: the same `Id:` map `AttachedTo:`/`RememberedCards:`
resolution already builds internally, kept around after `Load` returns instead of discarded. A scenario naming a
specific card to tuck needs a handle that survives a mulligan's shuffle, and `Id:` — assigned once, at load time, never
touched again — is exactly that; the `CardID` a shuffle produces is not something a fixture author could predict.

The comparison itself does not go through `Dump`. `Dump`'s `Id:` is the card's own `CardID` (see above), and
`setup.state` (run through actions.log) and `expect.state` are two independently loaded games whose `CardID`s were never
going to agree by number. `compareGames` (`scenario_test.go`) compares the two `*engine.Game`s directly instead — zone
contents by name and position, `Tapped`/`SummonSick`/`Damage`/`Counters`/attachment/protector per card — which sidesteps
the numbering question entirely and reaches fields `Dump` cannot write down at all (see below). Protector is compared by
name too, the same reasoning `CardID` gets: two independently loaded games were never going to agree on raw `PlayerID`s
either, only on who they name.

**`Lost`, `Won` and `Over` have their own keys: `lost=`, `won=`, `over=`.** `GameState.java`'s own format has none of
these — a Java fixture is always a still-being-played snapshot, never one that asserts the game already ended — so this
is Crucible-only, the same category as `actions.log` itself. Without them an `expect.state` loaded fresh always reported
`Lost`/`Won`/`Over` `false` regardless of what a scenario intended, which is why `compareGames` used to skip comparing
them: comparing would have failed every scenario that legitimately ends the game. `<player>lost=true` and
`<player>won=true` sit next to the other per-player keys (`PlayerState.Lost`/`Won`); `over=true` is top-level
(`State.Over`), since a game ending is not itself a per-player fact even though CR 104.2a's loss/win bookkeeping is.
`testdata/scenarios/poison-loss` is the example: ten poison counters going in via `setup.state`, `startturn human`
running `CheckStateBasedActions` in `actions.log`, and `expect.state` writing `humanlost=true`, `aiwon=true`,
`over=true` down as the assertion.

**`Protector:` is `lost=`/`won=`/`over=`'s own pattern applied to a single card field.** `Card.ProtectingPlayer` (CR
704.5w, `game-state.md`'s "Combat") is Crucible state with no Java `GameState` key to diverge from at all — grep finds
nothing resembling one in `GameState.java`. `Load`'s case is `Owner:`'s own shape exactly (`playerSlot`, then
`ld.slotToID[slot]`), gated to Battlefield-only in `Dump` the same way `Owner:`/`Tapped`/`Damage` are, since `Move`
clears it on the same "left the battlefield" transition. Written only when set (`ProtectingPlayer != NoPlayer`), the
same "say nothing when there's nothing to say" discipline `Owner:` already follows for when owner equals controller.
`testdata/scenarios/battle-protector-assigned-to-opponent` exercises the whole path: `queue battleprotector`, a
`Counters:DEFENSE=` high enough to survive `destroyZeroDefense` until the state-based action that assigns a protector
runs (CR 704.5v's ETB gap means a Battle placed directly on the battlefield starts at zero defense otherwise), and
`expect.state` naming the result with `Protector:`.

`TestScenarios` loads the real corpus once per test binary run (`sync.Once`), not once per scenario — synthetic cards
would defeat the point of a format meant to run against the Java oracle too, and 33,697 cards is too much to pay for per
case. That first load costs real time (order a minute, cold); TEST-13 already prices L3 at "every commit," same as L1,
so this is the cost that entry was always going to have once scenarios existed to pay it.

**Combat's own fixtures cover what its Go unit tests already prove, at the whole-engine level CLAUDE.md's testing table
asks for.** Every combat mechanic — declaring attackers/blockers, first strike, trample, gang blocking, attacking a
planeswalker, the legend rule — landed with full `package engine_test` coverage, but none of it had a
`testdata/scenarios/*` directory until this pass added seven: `combat-attacker-unblocked`,
`combat-single-block-kills-attacker`, `combat-first-strike-prevents-return-damage`, `combat-trample-excess-to-player`,
`combat-gang-block-damage-assignment`, `combat-attack-a-planeswalker`, `legend-rule-keeps-one`. Real corpus cards
throughout — Silvercoat Lion; Silver Knight; Craw Giant; Craw Wurm; Narset, Parter of Veils; Isamaru, Hound of Konda —
not synthetic defs, since `TestScenarios` runs against the real corpus and a scenario naming a card the corpus doesn't
have is a scenario with a typo. Each card's non-combat text (Rampage on Craw Giant, an activated ability on Narset) is
inert here on purpose: nothing this port has built fires a trigger, evaluates a static ability or activates anything, so
a real card's full script is exactly as safe a source of "just the keyword/type/P-T this scenario needs" as a synthetic
one — safer, since it also proves the scenario would keep meaning what it says once those systems exist and start
reading the rest of that same script.

Getting a first-strike or gang-block scenario right needs the phase walk to be real, not shortcut: `declareattackers`
and `declareblockers` each run while `AdvancePhase` has actually put the game in the matching phase
(`Declare Attackers`, `Declare Blockers`), one `advance` apart, because nothing in either method reads `ActivePhase` to
enforce that itself (game-state.md's "Combat" section) — a scenario that called them back-to-back without advancing
would still "work" mechanically but would end up asserting a phase that never happened. The state-based-action check
between the first-strike and regular damage steps is the sharper version of the same discipline:
`combat-first-strike-prevents-return-damage` only gets the right answer because `advance`ing from `First Strike Damage`
into `Combat Damage` is what actually kills the lethally-struck blocker before `combatdamage` runs
(`CheckStateBasedActions` runs on every phase entry, `game-state.md`'s "Turn structure") — skipping that `advance` would
leave the blocker alive to hit back, a different (wrong) scenario the fixture format makes easy to write by accident if
the phase walk isn't respected.

**`combat-mixed-first-strike-gang-block`** covers a combination the original seven didn't: one gang-blocked attacker
with blockers on both sides of the first-strike line, not one fixture per keyword. Durkwood Boars (4/4, no keywords) is
blocked by Elvish Archers (2/1, First Strike) and Devoted Hero (1/2, no keywords) — `firststrikedamage` only lets
Archers act, and `combatdamage`'s own `AssignCombatDamage` call still has to see both blockers as live, since neither
has taken any damage yet at that point (Archers dealt damage in the earlier step; nothing has dealt any to it). Getting
`dealsInStep`'s per-creature check wrong in either direction — Archers firing twice, or `AssignCombatDamage` only being
offered the blocker without first strike — is exactly the class of bug a first-strike fixture and a gang-block fixture,
each exercised alone, cannot catch.

**`combat-deathtouch-kills-regardless-of-toughness`** is the first fixture to touch Deathtouch at all. Typhoid Rats
(1/1, Deathtouch) attacks; Durkwood Boars (4/4, no keywords) blocks. A single point of damage is lethal to the Boars
despite its 4 toughness — `destroyDamagedCreatures` reading the deathtouch flag `dealPermanentDamage` set on the mark
(`game-state.md`'s "Lethal and deathtouch damage" section), not the raw amount against toughness a Go unit test
isolating that one function already proves correctly in isolation but which no scenario had exercised end to end through
declare-attackers/declare-blockers/combat-damage/state-based-actions together.

**`combat-split-across-two-defending-players`** is the first fixture to seat three players. Grizzly Bears attacks ai,
Silvercoat Lion attacks p2 — one combat, two defending players (CR 506.4) — and each is blocked by only that defender's
own creature (Hill Giant, Durkwood Boars), proving `DeclareCombatBlockers`' per-defender grouping asks the right player
about the right attacker rather than assuming one shared defender the way it did before this fixture existed. Human's
library needed a real card, not an empty one: CR 103.8a's "the first player skips their first draw step" is
two-player-only, so unlike every other fixture here (which are all two-player and rely on that skip), this one's active
player draws for real on turn 1 — the fixture's own `humanhand=Hill Giant` is that draw, not a card placed directly in
hand.

**`cleanup-discards-to-hand-size`** is the same discipline applied to CR 514.1 rather than combat: nine real cards
(Mountain) in hand, twelve `advance`s from `Untap` to land exactly on `Cleanup` (`Untap` is phase 0, `Cleanup` is 12),
`queue discard` naming the two that should leave. The count matters here more than in most scenarios — one `advance`
short lands on `End of Turn` instead, where `cleanupStep` never runs at all and the queued discard is simply never read.

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
| Player-level `PersistentMana:`, `LandsPlayed[LastTurn]:`, `NumRingTemptedYou:`, `Speed:` — `engine.Player` has none of these fields yet. `Counters:` is applied (`Player.Counters`, since M5's SBA work), and so is `ManaPool:` (`Player.ManaPool`, `applyManaPool`, since M5's mana-payment work)                                                                                                                                                                                                                                                                                        | M5-M6, as each field lands on `Player` |
| `ability<key>=` string values are stored verbatim in `AbilityStrings`; nothing parses or resolves them (puzzle-mode precast targeting)                                                                                                                                                                                                                                                                                                                                                                                                                                                    | Puzzle mode, if ever                   |
| `[metadata]` section (puzzle-mode name/description)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Puzzle mode, if ever                   |
| `actions.log` verbs for casting or targeting — nothing downstream of `ScriptedController` can answer those decisions yet either. Combat, mana payment (`paymanacost` and every `queue` kind `PayManaCost` can ask) and the one mana ability this port has (`tapformana`) have verbs                                                                                                                                                                                                                                                                                                       | M5-M6                                  |
| `expect.events` — the Plan's own fixture shape names it (Section 3.5) alongside `setup.state`/`actions.log`/`expect.state`, but `TestScenarios` (`internal/engine/scenario_test.go`) never reads a fourth file: `runScenario` loads only `setup.state` and `expect.state` and calls `compareGames`, which does not touch `Game`'s event sink at all. A fixture proving `LifeChanged`/`CounterChanged` actually fired (not just that life or a counter ended up at the right number) has nowhere to assert that yet                                                                        | M5-M6                                  |
