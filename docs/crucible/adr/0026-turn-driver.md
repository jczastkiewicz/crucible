# ADR-0026 — Turn Driver: Priority Wired Into the Turn Structure

- **Status:** Accepted
- **Date:** 2026-09-26
- **Deciders:** `mc@archlab.pl`
- **Supersedes:** ADR-0019 Decision point 2's "not wired into the turn structure", and its out-of-scope exclusion of
  mana abilities as a priority answer. The rest of ADR-0019 stands.

## Context

`PassPriority` (ADR-0019) runs one CR 117 round, but nothing in the turn structure calls it. `AdvancePhase`
(`turn.go:63`) is bookkeeping: it moves the phase, runs Untap/Draw/CombatEnd/Cleanup bodies, pushes phase triggers and
stops. Combat declarations and damage are separate calls (`attack.go:54`, `block.go:66`, `combatdamage.go:16-30`) that a
fixture makes itself. Result: no game can be played end to end. M8's runner (P7 gate: 100k games, zero hangs, turn cap)
needs a loop that does.

`AdvancePhase` cannot become that loop:

| Constraint                                                                                               | Why it blocks                                                                                        |
| -------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `endcombatphaseeffect.go:26` calls `AdvancePhase` during resolution                                      | Priority inside `AdvancePhase` would open a round inside `resolveTop` — ADR-0019's "no nested loops" |
| 356 scenarios drive `advance` then `declareattackers`/`resolvestack` explicitly                          | Combat or priority inside `AdvancePhase` double-declares and auto-resolves; every `expect.*` changes |
| `givesPriority(Cleanup)` re-runs state-based actions after `beginPhase` already ran them (`priority.go`) | The second run usually finds nothing; CR 514.3a's repeat would never fire                            |
| `Action` has cast and activate only; every step empties mana pools (CR 500.4, `beginPhase`)              | Inside a driven step nobody can put mana in a pool, so nobody can pay for anything or play a land    |

Java's shape (`PhaseHandler.java`): `mainGameLoop` (`:1032-1037`) calls `mainLoopStep` until the game is over.
`onPhaseBegin` (`:240-451`) runs the step's turn-based action, then `TriggerType.Phase` triggers, and sets
`givePriorityToPlayer` per step: false for Untap (`:251`) and Cleanup (`:422`); `inCombat()` for declare attackers
(`:311`); false for a damage step where `Combat.assignCombatDamage` assigns nothing (`:328`, `:340`,
`Combat.java:919-925`). `isSkippingPhase` (`:219-238`) skips declare blockers and both damage steps when no creature
attacked (`skipDamageSteps`, `:229`). Cleanup repeats (`bRepeatCleanup`, `:156-158`) when its own state-based check did
something (`:425-426`) or the stack is non-empty after triggers (`:447-449`). Lands and mana abilities go through the
same `chooseSpellAbilityToPlay` ask as spells (`:1056`, `:1079`). No turn cap; only an AI-only 999-action guard
(`:1102-1105`).

## Decision Drivers

- TEST-1: all existing scenarios and tests stay byte-identical. `AdvancePhase` keeps its exact behavior.
- GO-2: driver state lives on `*Game` or in locals, never package-level.
- GO-7: a misbehaving controller or a failed declaration fails its own game with an error, never a panic.
- P7 gate: a turn cap, enforced by the loop itself.
- Parity: step order, skip rules and priority grants follow `onPhaseBegin`, cited per branch.

## Considered Options

1. **Priority inside `AdvancePhase`/`beginPhase`.** Rejected: every row of the table above.
2. **A driver that calls public `AdvancePhase`, then the combat calls, then `PassPriority`.** Rejected: `beginPhase`
   pushes "at the beginning of declare attackers" triggers before the driver could declare attackers — Java runs the
   turn-based action first (`:305-312`, then `:438-440`). Cleanup's priority would come from `givesPriority`'s second
   state-based check.
3. **One internal `beginPhase(driven)` both paths share; a new public driver on top.** `AdvancePhase` passes
   `driven=false` and behaves exactly as now. The driver passes `driven=true`: combat turn-based actions run inside the
   step body, before triggers, and the step reports its own priority grant the way `onPhaseBegin` sets
   `givePriorityToPlayer`.

Option 3 chosen.

## Decision

1. **Public entries**, in a new file `driver.go`:
   - `Game.Step(reg *Registry, controller PlayerController) error` — leave the current step, begin the next one driven
     (Decision 2), run a priority round if the step grants one (Decision 3), repeat Cleanup while it grants one (CR
     514.3a). One call is one step. The current step's own priority window is assumed played already: `StartTurn` begins
     Untap, which grants none, so `StartTurn` then `Step`/`Run` covers a whole game.
   - `Game.Run(reg *Registry, controller PlayerController, maxTurns int) error` — `Step` until the game is over or turn
     `maxTurns`'s Cleanup has finished. A capped game returns nil with `Over()` false; the caller (M8) records it as
     unfinished. Checked at the Cleanup→Untap boundary so a capped game always stops after a whole turn.
2. **`beginPhase(controller, driven bool) (priority bool, err error)`**, `advancePhase` likewise. `driven` is passed
   through `consumeSkip`'s recursion, so a skipped phase never drops the driver back to bookkeeping mode. Driven-only
   branches:

   | Step              | Turn-based action (before phase triggers)          | Priority                                                                                 |
   | ----------------- | -------------------------------------------------- | ---------------------------------------------------------------------------------------- |
   | Untap             | unchanged                                          | never (`:251`)                                                                           |
   | DeclareAttackers  | `DeclareCombatAttackers`                           | always, zero attackers included (`inCombat()`, `:311`)                                   |
   | DeclareBlockers   | `DeclareCombatBlockers`                            | always                                                                                   |
   | FirstStrikeDamage | `DealFirstStrikeDamage`, if any damage is assigned | only if damage is assigned (`:328`)                                                      |
   | CombatDamage      | `DealCombatDamage`, if any damage is assigned      | only if damage is assigned (`:340`)                                                      |
   | Cleanup           | unchanged                                          | only if the step's state-based check did something or the stack is non-empty (CR 514.3a) |
   | every other step  | unchanged                                          | always                                                                                   |

   Skip rule: entering DeclareBlockers driven sets `Game.skipDamageSteps` to "no attackers declared" (`:229`);
   DeclareBlockers, FirstStrikeDamage and CombatDamage are then skipped the way `consumeSkip` skips them — not begun, no
   `PhaseBegan`, no triggers — matching Java's `skipped` path, which runs neither turn-based actions nor
   `TriggerType.Phase` (`:244-246`, `:436-441`). "Damage is assigned" mirrors `assignCombatDamage`'s return: some live
   attacker dealing damage in the step has power above zero, or some live blocker dealing damage in the step still
   blocks a live attacker.

   Declaration errors (ADR-0024) and a static trigger's pending error (ADR-0020) are returned from the driven step
   before any priority round.

3. **Priority round.** `PassPriority`'s loop body moves into `priorityRound`; `PassPriority` keeps its `givesPriority`
   gate and calls it. The driver calls `priorityRound` directly, gated by Decision 2's per-step grant, never by
   `givesPriority`. The `passpriority` fixture verb and every existing test see no change.
4. **Three new `Action` kinds.** Pools empty on every step, so paying for anything in a driven step needs mana abilities
   as a priority answer (CR 117.1d, 605.3a):

   | Kind                | Applies                                   | Keeps priority  |
   | ------------------- | ----------------------------------------- | --------------- |
   | `ActionPlayLand`    | `PlayLand` (CR 305.1, special action)     | yes (CR 116.3)  |
   | `ActionTapForMana`  | `TapLandForMana` with `Action.Color`      | yes (CR 605.3a) |
   | `ActionManaAbility` | `ActivateManaAbility` with `AbilityIndex` | yes (CR 605.3a) |

   A declined one is an error, the same as a declined cast (ADR-0019 Decision point 5).

5. **Mid-round phase jumps.** `EndTurn` and `EndCombatPhase` (`endturneffect.go`, `endcombatphaseeffect.go`) move the
   phase non-driven during resolution. The round continues in the new phase, as Java's does; the next `Step` advances
   from wherever the phase landed. A Cleanup begun this way does not repeat (its state-based result is discarded): named
   as a gap, not silently right.

**Explicitly out of scope:** a cap on actions per priority round — `ScriptedController` is finite, and a looping AI is
M7's problem, the same place Java put its guard (`:1102-1105`); the per-combatant "dealt first-strike damage" set
(`Combat.java:906-917`) — `dealsInStep` reads keywords held now, so a creature gaining or losing first strike between
the two damage steps diverges; mulligans and opening hands, which the caller runs before `StartTurn`.

## Consequences

**Good:** a whole game runs from `StartTurn` to a result, the prerequisite for M7's controller and M8's runner. Phase
triggers now resolve through real priority rounds in a driven game. Every existing scenario and test is untouched:
`driven=false` is today's code path.

**Bad:** two ways to walk the turn — `advance` (bookkeeping, fixtures) and `step` (driven). A fixture mixing them must
know `advance` skips combat's turn-based actions. `Game` gains one field (`skipDamageSteps`), copied by `Clone`.

**Neutral:** `Action` grows to five kinds and one field (`Color`). No new event (ADR-0019 Decision point 6's volume
reasoning holds for a step, too).

## Related

ADR-0019 (interactive priority — this wires it in), ADR-0020 (pending static-trigger errors), ADR-0024 (declaration
errors), ADR-0013 (no new event), [04-adr-process](../guidelines/04-adr-process.md)
