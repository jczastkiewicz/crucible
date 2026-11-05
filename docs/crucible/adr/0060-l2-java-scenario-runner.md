# ADR-0060 — L2 Scenario Runner on the Java Engine

- **Status:** Proposed
- **Date:** 2026-11-05
- **Deciders:** Crucible session (open plan R19), awaiting `jc@archlab.pl`

## Context

ADR-0010 L2 runs each `testdata/scenarios/` fixture on both engines with scripted decisions and says it gates merges.
Only the Go half exists: `TestScenarios` loads `setup.state`, runs `actions.log` through `fixture.RunActions` and
compares the result with `expect.state` (`compareGames`, `scenario_test.go`). The hand-written `expect.state` is the
only oracle, so a rule the author misread passes on both sides of the comparison. ADR-0010 does not say how a Java run
reads `actions.log`, what it is compared with, or how the 112-method `PlayerController` is answered.

| Fact                                                                                                                                                                                                                                                                                                             | Where                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| 850 scenarios. Their `actions.log` verbs: `startturn`, `advance`, `step`, `passpriority`, `resolvestack`, `castspell`, `playland`, `tapformana`, `paymanacost`, `declareattackers`, `declareblockers`, `firststrikedamage`, `combatdamage`, special actions, opening-hand verbs, `queue <kind>` (about 50 kinds) | `game-state-fixture.md`, "Scenarios: actions.log and the harness"                           |
| The Go verbs call engine steps directly: `combatdamage` is `Game.DealCombatDamage`, separate from `advance`                                                                                                                                                                                                      | `crucible/internal/fixture/actions.go`                                                      |
| Java deals damage, declares combat and puts triggers on the stack inside its own phase loop; `PhaseHandler.mainLoopStep()` is the public single step, used by upstream's `AITest`                                                                                                                                | `PhaseHandler.java:1047`; `forge-gui-desktop/src/test/java/forge/ai/AITest.java:92`, `:204` |
| Upstream ships a scripted-test kit: `PlayerControllerForTests` overrides all 112 methods, `GameWrapper` and `PlayerActions` feed it actions with preconditions                                                                                                                                                   | `forge-gui-desktop/src/test/java/forge/gamesimulationtests/util/`                           |
| `GameState.applyToGame` loads the fixture format; its `Id:` handles go into a private `idToCard` map; Java cards get their own ids                                                                                                                                                                               | `GameState.java:76`, `:611`, `:1382-1384`                                                   |
| `GameState.toString` writes effect cards into the command zone by name but skips `DetachedCardEffect`; nothing can load an effect card back                                                                                                                                                                      | `GameState.java:225-233`                                                                    |
| `crucible/oracle-java/pom.xml` has no dependencies; `${revision}` in the installed upstream poms keeps Maven from resolving them outside the reactor, so `dump-boosters.sh` compiles one class against `dependency:build-classpath`                                                                              | `crucible/oracle-java/pom.xml`, `dump-boosters.sh`                                          |
| 10 of 850 scenarios draw from the game's generator after setup (mulligans, opening hands, a shuffle trigger, a die roll, a subgame, Karn's restart, Camouflage); Java's `MyRandom` is unseeded unless `setRandom` replaces it                                                                                    | measured 2026-11-05 with a probe in `scenario_test.go`; `MyRandom.java:12`, `:46`           |
| 17 scenarios use Crucible-only keys (`over=`, `lost=`, `won=`, `monarch=`, `initiative=`, `Protector:`); 7 use `expecterror`, `run`, `resumerestart` or `concede`; about one queues an id the game created                                                                                                       | grep over `testdata/scenarios`                                                              |

## Decision Drivers

- ADR-0010: with no AI in the loop, any difference is a rules bug. The runner must not add a third source of divergence.
- REV-1: no edit outside `crucible/` and `docs/crucible/`. The runner lives in `crucible/oracle-java`.
- One fixture vocabulary (ADR-0010 rejects a second format). `actions.log` stays the only script.
- A verb or queue kind with no Java mapping fails loudly on the Java side, never runs as a no-op.
- The normaliser is ADR-0010's main silent-failure risk: each of its rules needs a test.

## Considered Options

**Driving model:**

1. Mirror each Go verb with a Java call: `combatdamage` → the damage step's body. Java keeps those bodies private inside
   `PhaseHandler`; reaching them needs reflection on upstream internals or an upstream edit.
2. Convert `actions.log` into a decision script for Java's own loop, `GameWrapper`/`PlayerActions` style. Phase verbs
   become a stop point. Game actions (`castspell`, `playland`, `tapformana`, a special action) become the controller's
   answer at the next priority the actor gets. `queue` lines become the answers to the matching `PlayerController`
   methods. Go-only step verbs (`declareattackers`, `firststrikedamage`, `combatdamage`, `resolvestack`) become
   checkpoints that Java reaches by itself.

**What is compared:**

1. Java's post-state with `expect.state`. Java cannot load the Crucible-only keys, and both sides are checked against
   the same hand-written guess.
2. Java's post-state with Go's post-state from the same run (`fixture.Dump`), both as `GameState` text, through a
   normaliser. `expect.state` stays the Go assertion; L2 asserts Go equals Java.

**Controller:**

1. A new 112-method controller in `oracle-java`.
2. Extend upstream's `PlayerControllerForTests`, compiled against `forge-gui-desktop`'s test classes on the
   `dependency:build-classpath` path, with no pom change upstream.

## Decision

**Driving: option 2.** `ScenarioRunner` (`crucible/oracle-java`, test tooling, ADR-0002) bootstraps the model the way
`AITest.initializeModel` does. It applies `setup.state` with `GameState.applyToGame` and seeds `MyRandom.setRandom` with
`new Random(1)`, the seed `TestScenarios` gives `javarand`. It then steps `PhaseHandler.mainLoopStep()` until the Go
run's final turn, phase and empty-stack point. Every `actions.log` line maps through one table, `l2-verbs.md` in
`docs/crucible/porting/port-log/`, written with the runner: a Java answer, a checkpoint, or `unmapped`. A scenario with
an `unmapped` line is reported as not runnable, never compared. Reason: Java's step bodies are not callable one by one
without reflection into `PhaseHandler`, and a checkpoint that does not match Java's own position in the loop is itself a
finding.

**Controller: option 2.** `ScenarioController extends PlayerControllerForTests` and answers each method from the queued
answers of its kind, in order. It applies Go's own fallback: when nothing is queued, it panics where
`ScriptedController` panics and returns Go's default where Go defaults (`ChooseAssistant` → nobody). A method Go never
asks is a finding. Reason: upstream already wrote and maintains the 112 overrides, and the build recipe
`dump-boosters.sh` uses avoids both REV-1 and `${revision}`.

**Identity: fixture handles.** `queue` and verb ids are `setup.state`'s `Id:` values, resolved through `GameState`'s
`idToCard` read by reflection. Reflection is allowed in Java test tooling only; a renamed field fails the runner loudly.
An id the game created during the run (one scenario today) uses ADR-0059's structural identity. Reason: no third
identity scheme.

**Comparison: option 2.** Both sides write `GameState` text: Java `GameState.toString`, Go `fixture.Dump`. The
normaliser (`crucible/tools/l2diff`, Go, with its own tests) applies these rules:

- It renumbers `Id:` in dump order and rewrites every reference through the renumbering: `AttachedTo`, `ExiledWith`,
  `RememberedCards`.
- Java's run writes Crucible-only keys from Java's own model: `over=`, `lost=` and `won=` from `Game.isGameOver` and the
  outcome, `monarch=`, `initiative=`, `Protector:`. They are compared, not dropped.
- Effect cards stay: both dumps write them by name.

Nothing else is normalised without a row in `l2-verbs.md` and a test. Reason: a scenario whose post-state holds an
effect card cannot be asserted with `expect.state`, but it can be compared engine to engine. A diff against a
hand-written file would only check the author twice.

**Random scenarios.** The 10 scenarios that draw after setup run with the shared seed. A difference in one of them is
reported as `random`, outside the gate. When ADR-0059's per-site outcome log exists (open plan R21b), the runner replays
Go's outcomes into Java the same way L3 does. Reason: ADR-0006 lets draw counts differ.

**Gating, in two phases.**

1. Report. A CI job (`crucible-oracle`, needs the JVM) runs every runnable scenario and publishes three lists:
   `differs`, `not runnable` and `random`. Each `differs` line gets a Go fix, a `quirks.md` row or a
   `forge-java-defects.md` row (PORT-8: a Java test first).
2. Gate. The job fails on any `differs` scenario missing from `crucible/testdata/l2-baseline.txt`. Each baseline line
   cites its `quirks.md` or `forge-java-defects.md` row. ADR-0010's "L2 gates merges" holds from this phase on.

**Out of scope:** `expect.events`. Go has no reader for it (`game-state-fixture.md`), so L2 compares post-states only
until one exists.

## Consequences

**Good:** every scenario gets a second, independent judge at the cost of one table and one controller subclass. A wrong
`expect.state` stops passing silently. Effect-card post-states become checkable. `l2-verbs.md` writes down where the Go
harness's step granularity departs from Java's loop.

**Bad:**

- The runner depends on upstream test-scope classes (`PlayerControllerForTests`, `AITest`'s bootstrap) and on a private
  field. An upstream refactor breaks it, loudly, on the next sync (`runbooks/upstream-sync.md` gets a step).
- The checkpoint model cannot compare mid-scenario states, only the end.
- The JVM job adds minutes to CI. It runs beside the Go jobs, not in the commit hook.

**Neutral:** the 7 scenarios with Go-only verbs and the scenario that queues a created id are `not runnable` until their
mapping lands. The count is in the report, not hidden.

## Related

- ADR-0010: the layers, L2's role
- ADR-0002: the oracle in-tree as test tooling
- ADR-0006: why draw counts may differ
- ADR-0059: structural identity, per-site random outcomes
- [game-state-fixture.md](../porting/port-log/game-state-fixture.md): the fixture format and the Go harness
