# Port: L2 Scenario Runner

- **Java source:** none ported. The runner drives upstream code: `GameState` (`parse`, `applyToGame`, `initFromGame`, `toString`), `PhaseHandler.setupFirstTurn`/`mainLoopStep`, and the test kit in `forge-gui-desktop/src/test/java/forge/gamesimulationtests/util/` (`LobbyPlayerForTests`, `PlayerControllerForTests`)
- **Go target:** `crucible/tools/l2diff` (emit, normalise, report); Java side `crucible/oracle-java/src/main/java/crucible/oracle/ScenarioRunner.java`, built and run by `crucible/oracle-java/l2-run.sh`
- **Status:** ADR-0060 phase 1 (report) runs locally on all 868 scenarios. One verb is mapped (`startturn`), so most scenarios are not runnable yet; no CI job and no `l2-baseline.txt` yet (phase 2)

## What it does

Each `testdata/scenarios` fixture runs on both engines and the two post-states are compared, so a wrong `expect.state` stops passing silently (ADR-0060).

```bash
cd crucible
go run ./tools/l2diff emit -out /tmp/l2                 # go/<name>.state, manifest.tsv, random.txt, go-failed.txt
oracle-java/l2-run.sh /tmp/l2/manifest.tsv /tmp/l2/java # java/<name>.state|.unmapped|.error; L2_NO_BUILD=1 reuses the build
go run ./tools/l2diff diff -dir /tmp/l2                 # report.txt, diffs/<name>.diff
```

`emit` plays every scenario the way `TestScenarios` does (seed 1) and records Go's final turn and phase, the point Java must stop at, and whether the run drew from the game's generator after setup (`random.txt`). One JVM runs the whole manifest: the card database takes about 10 s to load and a scenario a few milliseconds. `diff` sorts each scenario into `same`, `differs`, `random` (a difference in a scenario that drew randomness, outside the gate) or `not runnable` (an unmapped line, a Java exception, or no Java output).

## Setup rules

How the runner turns a fixture into a Java game. Each is a place the Go harness and Java's own tooling disagree on what a fixture means.

| Rule | What | Reason |
| --- | --- | --- |
| S1 | Seats: `human` is player 0, `ai` player 1, `p<i>` player i; two seats unless the setup names a higher `p<i>` | `GameState.getPlayerState` and `fixture.playerSlot` index seats the same way |
| S2 | A setup with no `activeplayer=` gets the player of the first `startturn` | `GameState.applyGameOnThread` indexes the active player's state and throws without one; `fixture.Load` needs none |
| S3 | Both seats are `LobbyPlayerForTests`, never `LobbyPlayerAi` | An AI controller would act on its own priority |
| S4 | `MyRandom.setRandom(new Random(1))` after `applyToGame` | The seed `TestScenarios` gives `javarand` |
| S5 | The whole run executes on Forge's game thread, the caller waits | `GameAction.invoke` hands state changes to `ThreadUtil`'s game thread; from another thread `applyToGame` returns before the setup exists |
| S6 | The JVM starts in `forge-gui-desktop/`, with `-Djava.awt.headless=true` | Forge finds `res/` relative to the working directory, as upstream's own tests run it; `GuiDesktop` needs no display |
| S7 | The reactor's `target/classes` precede the `~/.m2` jars on the classpath (`mvn -pl forge-gui-desktop -am test-compile dependency:build-classpath`) | A SNAPSHOT jar can predate the checked-out source; the oracle would quietly be old Java |
| S8 | Java stops at Go's final turn and phase with an empty stack, stepping `PhaseHandler.mainLoopStep` | The checkpoint model of ADR-0060: only the end state is compared |
| S9 | The runner writes `over=`, `<player>lost=`, `<player>won=` (only once the game is over), `monarch=` and `initiative=` from Java's model, by registered seat | ADR-0060: Crucible-only keys are compared, not dropped. `Player.hasWon` is true for any player with an outcome and no loss, which only means a win at game end |
| S10 | Seats in Java's text go back to the registered seat (`Game.getRegisteredPlayers`) | `GameState.initFromGame` numbers by `Game.getPlayers`, which drops a player who lost, so the survivors' `p<i>` shift |

## Verbs

An `actions.log` line is mapped to a Java answer, a checkpoint, or nothing. A scenario with any unmapped line is reported `not runnable` and never compared (ADR-0060). Counts are scenarios using the verb.

| Verb | Scenarios | Java | State |
| --- | --- | --- | --- |
| `startturn <player>` | 823 | `devModeSet(null, p, false, 0)`, then `PhaseHandler.setupFirstTurn(p, null)`: Java's first turn counts up from 0, as Go's `Game.StartTurn` always begins turn 1 at Untap | mapped |
| `advance [n]` | 767 | — | unmapped |
| `queue <kind> ...` | 753 | — (`ScenarioController`'s per-kind answers) | unmapped |
| `declareattackers`, `declareblockers`, `firststrikedamage`, `combatdamage` | 375, 359, 28, 353 | — (checkpoints Java reaches in its own loop) | unmapped |
| `tapformana`, `paymanacost` | 343, 28 | — | unmapped |
| `resolvestack`, `passpriority`, `step [n]` | 316, 103, 12 | — | unmapped |
| `castspell`, `playland` | 250, 24 | — (an answer at the actor's next priority) | unmapped |
| `expecterror` | 18 | — | unmapped |
| `mulligan`, `dealopeninghands` | 5, 3 | — | unmapped |
| `foretell`, `plot`, `suspend`, `turnfaceup` | 2, 1, 1, 1 | — | unmapped |
| `run`, `resumerestart`, `concede` | 2, 1, 1 | — | unmapped |

Queue kinds by scenarios: attackers 380, blocks 356, paygeneric 235, targets 224, action 108, cardchoice 44, cardorder 30, confirmeffect 24, confirmpaycost 22, optionaltrigger 14, sacrificechoice 13, replacement 11, attacktarget 11, payx 8, scry 7, keephand 7, confirmreplacement 7, returnchoice 6, enchanttarget 6, divide 6, and 34 kinds below 6.

## Normaliser

`crucible/tools/l2diff/normalize.go`; each rule has a case in `TestNormalizeRules`. Nothing else is normalised (ADR-0060).

| Rule | What | Reason |
| --- | --- | --- |
| N1 | Java's `p0`/`p1` become `human`/`ai`: key prefixes, the values of `activeplayer`, `monarch` and `initiative`, and the `Owner`, `EnchantingPlayer`, `Protector` tokens (`P<i>`, case folded) | `GameState.toString` writes seats by index, `fixture.Write` by name; `GameState.parsePlayerString` reads both |
| N2 | `activephase` is the `PhaseType` constant (`MAIN1`, `END_OF_TURN`) | Java writes the constant, Go the script spelling; `PhaseType.smartValueOf` reads both |
| N3 | Java's `Set:` and `Art:` tokens are dropped | The printing; Go's fixture has none |
| N4 | A key with an empty value, and `landsplayed`, `landsplayedlastturn`, `numringtemptedyou`, `speed` at 0, are dropped | Java writes every key, Go only the set ones |
| N5 | `Id:` is kept only on a card an `AttachedTo`, `ExiledWith`, `RememberedCards`, `Imprinting` or `ChosenCards` token names, renumbered in dump order, and every reference rewritten | Java writes `Id:` only for a referenced card (`cardsReferencedByID`), with its own card ids; Go writes every card's `CardID` |
| N6 | The Monarch, The Initiative and The Ring leave Java's command zone | `Game.IsDesignationCard`; Go writes `monarch=`, `initiative=` and `numringtemptedyou=` instead (ADR-0060) |
| N7 | A card's tokens after its name are sorted | Token order is the writer's; `GameState` parses them in any order |
| N8 | Lines are sorted by key | Key order is the writer's |
| N9 | A player who lost keeps only `lost=` (and `won=`) | `GameState.initFromGame` writes the players still in the game (`Game.getPlayers`) |

## Findings

From the first full run: 32 same, 15 differs, 0 random, 821 not runnable (818 with an unmapped line, 3 Java errors). Each `differs` scenario needs a Go fix, a `quirks.md` row or a `forge-java-defects.md` row before phase 2's baseline (ADR-0060). None is classified yet.

| Finding | Scenarios | What the diff shows |
| --- | --- | --- |
| F1 | 6 `replacement-*` (Rest in Peace, Leyline of the Void, Kalitas, Gravebane Zombie, Nissa's Chosen, Progenitus), `world-rule-keeps-newest`, `trigger-ongoing-scheme-static-anthem-applies-from-command` | Java's `applyGameOnThread` runs `checkStateEffects(true)` before `copyLastState`, so a setup's lethal damage or rule breach is settled at load with no Moved replacement and no layer-applied static in effect; Go settles it in `StartTurn`'s Untap. A runner question first: Go checks state-based actions on entering Untap (`beginPhase`), Java only when a player would receive priority (CR 704.3; `mainLoopStep` with `givePriorityToPlayer`, false in Untap) |
| F2 | `life-loss-at-zero`, `poison-loss`, `sba-poison-eleven-loses`, `cant-lose-phyrexian-unlife-does-not-stop-a-poison-loss`, `life-loss-eliminates-one-player-game-continues` | After the active player loses, Java's `activeplayer` is the next seat, Go keeps the loser |
| F3 | `life-loss-eliminates-one-player-game-continues` | Java's `applyGameOnThread` sets a setup's life of 0 or less after the load-time state-based actions by `Game.getPlayers` index (`GameState.java:697-701`); with the loser already out, index 0 is the next seat, whose life becomes 0. Unverified tooling candidate (PORT-8: a Java test first) |
| F4 | `life-loss-below-zero-also-loses` | F1's Untap point: Go's check ends the game, Java reaches Untap with no check and the player at -5 |
| F5 | `sba-start-your-engines-sets-speed-to-one` | Java writes `speed=1`; `fixture.Dump` writes no `speed=` |
| F6 | `planeswalker-zero-loyalty-destroyed`, `sba-dream-counters-past-the-cap-are-trimmed`, `sba-saga-at-its-final-chapter-with-none-on-the-stack-is-sacrificed` | Java error: `PlayerControllerForTests.playSpellAbilityNoStack` throws for any card its list does not name; `ScenarioController` overrides it |

## Not ported yet

- `ScenarioController` (ADR-0060): the per-kind queued answers, Go's defaults and panics. Until it exists, `startturn` is the only verb and the controller is upstream's `PlayerControllerForTests`
- Every verb but `startturn`. By the first unmapped line per scenario, `advance` unlocks the most (734), then `queue` (63)
- A created id (ADR-0059, open plan R21a-c)
- Phase 2: the `crucible-oracle` CI job and `crucible/testdata/l2-baseline.txt`
- `Protector:` from Java's model (a battle's protector; `GameState.toString` writes none)
