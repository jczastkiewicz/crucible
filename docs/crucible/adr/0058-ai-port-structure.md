# ADR-0058 — The AI Port: Per-Game Controller, Generated Per-API Registry, Typed Profiles, No Clocks

- **Status:** Accepted
- **Date:** 2026-11-05
- **Deciders:** Crucible session (M7 preparation), accepted by `jc@archlab.pl`

## Context

M7 ports Forge's AI (plan items 33-35, rows R22a-R22g). `forge-ai` is 214 files and 72,124 LOC, of which `simulation/`
is 14,148 LOC. ADR-0003 places it in `internal/ai`, importing the engine. Five properties of the Java code cannot be
translated as they stand.

| Java                                                                                                                                                  | Where                                                                                                | Conflict                                                    |
| ----------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| `SpellApiToAi.Converter`: an enum singleton whose `EnumMap` maps 195 `ApiType`s to 148 classes, instantiated reflectively and cached in a mutable map | `SpellApiToAi.java:13-16`, `:227-239`; `ReflectionUtil.java:34`                                      | GO-4 (no reflection), GO-2 (no package-level mutable state) |
| `AiCache`, a static multimap "shared between games"; static profile map; static evaluator singletons                                                  | `AiCache.java:16-23`; `AiProfileUtil.java:42-66`; `ComputerUtilCard.java:769-770`                    | GO-2, ADR-0005 (goroutine per game)                         |
| Profiles as `KEY=value` text read through string lookups (`getIntProperty` parses on each read)                                                       | 6 `.ai` files under `forge-gui/res/ai`; `AiProps.java:26-191`, 150 keys; `AiProfileUtil.java:96-152` | GO-8 (no stringly-typed params), PORT-2 (parse once)        |
| The global `MyRandom` that the game also uses, about 87 AI uses, swapped wholesale by the simulators                                                  | `SpellAbilityPicker.java:371-391`; `FlatMonteCarloPicker.java:1287-1329`                             | ADR-0006 (partitioned streams)                              |
| Wall-clock budgets: an evaluation thread with a 5 s deadline; Monte Carlo budgets of 450 ms per decision and 2,000 ms per turn                        | `AiController.java:2677-2682`; `Game.java:102`; `FlatMonteCarloPicker.java:125`, `:496-581`          | ADR-0006: output must not depend on timing                  |

`PlayerControllerAi` (1,931 LOC) overrides 119 `PlayerController` methods. About 25 go to `AiController`, 10 to the
per-API `SpellAbilityAi` (`chooseSingleEntityForEffect`, `chooseBinary`, `chooseNumber` and others). The rest are
answered inline or through `ComputerUtil*` statics.

## Decision Drivers

- GO-2, GO-4, GO-8, ADR-0005: the same rules the engine follows.
- ADR-0006: a game re-run from its seed is byte-identical, AI included.
- ADR-0010 L4: parity with Java's AI is statistical, so the port reproduces policy, not Java's random draws or timing.
- ADR-0017: the engine already dispatches 199 effect APIs through a generated registry, and its CI completeness check
  can be reused.

## Considered Options

### Per-API dispatch

1. **One interface plus an embedded default struct per class, registered in a generated array indexed by `engine.API`.**
   Chosen.
2. **One function switching on the API.** Rejected: 148 classes and the 30,177 LOC of `ability/` in one switch cannot be
   reviewed, and per-API tests lose their file mapping (TEST-3).
3. **A `map[engine.API]SpellAbilityAI` filled in `init`.** Rejected: package-level mutable state (GO-2), and nothing
   checks that it is complete.

### Time budgets

1. **Keep wall-clock budgets.** Rejected: results depend on machine load, so ADR-0006 reruns fail.
2. **Fixed work budgets, counted in candidates, rollouts and plays.** Chosen. The profile's existing
   `FLAT_MC_MAX_CANDIDATES`, `ROLLOUTS` and `MAX_PLAYOUT_PLAYS` bound the work.

## Decision

**Controller.** `ai.Controller` implements `engine.PlayerController`. There is one per player per game, created by
`internal/sim`. Per-game memory (`AiCardMemory`, the `AiCache` contents) lives on it and dies with the game. Nothing in
`internal/ai` is a package-level variable except immutable tables.

**Dispatch.** `SpellAbilityAI` is an interface over Java's hooks: `CanPlay`/`CheckAPILogic`, `DoTriggerNoCost`,
`ChkDrawback`, `Confirm`, `ChooseSingleEntity`, `ChooseNumber`, `ChooseBinary` and the rest. Each class embeds a
`baseAI` that gives `SpellAbilityAi.java`'s defaults (Go embedding stands in for Java inheritance). `genregistry` emits
an array indexed by `engine.API` (the 195 Java mappings). An API Java does not map answers with `cannotPlayAI`, as Java
does (`SpellApiToAi.java:230-234`). CI fails when an engine API has neither an entry nor an explicit `cannotPlayAI` row.

**Profiles.** The 6 `.ai` files compile at load into an immutable `ai.Profile` struct. It has one typed field per
`AiProps` constant, with Java's default (`AiProps.getDefault`). The struct is generated from `AiProps.java` and shared
read-only across games. An unknown key or a malformed number is a load error, as Java throws (`AiProfileUtil.java:108`,
`:127-129`).

**Randomness.** Every AI draw uses the AI stream of ADR-0006 through `*Game`. Java's shared `MyRandom` and the
simulators' generator swaps are not reproduced. L4 parity does not need them, and L3 replays recorded answers instead of
AI draws.

**No clocks, no threads.** Every Java deadline becomes the profile's work count. An AI decision runs in the game's
goroutine and starts no goroutine of its own. Java's evaluation thread and per-attack executor
(`AiAttackController.java:895`) become plain calls.

**Lookahead.** Flat Monte Carlo and `SpellAbilityPicker` are ported last (R22f-R22g) and are off by default. Java's
`AI_FLAT_MONTE_CARLO` game flag defaults to false (`Game.java:106`), so the default Java AI-vs-AI game does not use
them, and L4 is measured against that default.

## Consequences

**Good:** The AI keeps the engine's guarantees: games run in parallel, re-runs are byte-identical, nothing is
reflective, and a missing per-API class is a CI failure, not a stderr line. Profile typos fail at load.

**Bad:** Results can differ from Java's even where the policy matches. Java's budgets cut a search short on a slow
machine, and the work-count budget does not, so L4 must allow for that. The generated profile struct must be regenerated
when upstream adds an `AiProps` key; the upstream-sync runbook gains that step.

**Neutral:** `genregistry` gains a second output beside the engine's effect registry; `genparams` gains the profile
struct.

## Related

ADR-0003, ADR-0005, ADR-0006, ADR-0010, ADR-0012, ADR-0017
