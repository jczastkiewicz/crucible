# ADR-0065 — A Step's Triggers Wait for the State-Based Actions

- **Status:** Accepted
- **Date:** 2026-11-13
- **Deciders:** Crucible session (R21c), accepted by the owner

## Context

ADR-0064 holds a resolution's triggers until the state-based actions after it, and puts both sets on the stack in one
APNAP pass. It leaves a step's turn-based actions out: their triggers go on the stack as each action ends.

Java treats the two cases the same way. `onPhaseBegin` runs the step's turn-based actions (combat damage, declarations,
the draw). The triggers they fire go into `simultaneousStackEntryList`. Before a player gets priority,
`checkStateBasedEffects` runs `checkStateEffects` and then `addAllTriggeredAbilitiesToStack`, in a loop
(`PhaseHandler.java:1308-1318`). If the state-based actions end the game, it returns before the push, so no waiting
trigger ever reaches the stack.

Go opens one trigger batch around the combat damage step (`dealCombatDamageStep`). It closes that batch before
`beginStep` checks the state-based actions. Two things differ from Java:

- **Lethal damage.** Go pushes the damage triggers, then the state-based actions end the game. Java ends the game with
  the stack empty. The R21c replay of rakdos seed 3 diverges here: Go pushes Ragavan's combat damage trigger after
  Java's game has already ended.
- **Deaths from combat damage.** A creature the damage kills triggers in a second APNAP pass, on top of the damage
  triggers. Java orders all of them in one pass, and each player orders their own together.

## Decision Drivers

- L3 replay parity (ADR-0010): Java's event order and Java's trigger order
- CR 603.3 (triggers wait until a player would receive priority) and CR 704.3 (state-based actions come first)
- One mechanism for every point where triggers wait, the batch ADR-0064 already uses

## Considered Options

1. Keep the per-action batches; let the replay pin the divergence
2. Drop a closing batch's triggers when the game is over; keep the per-action batches
3. One trigger batch per step: open it when `beginStep` starts, close it after the step's state-based actions, and push
   nothing once the game is over

## Decision

Option 3.

**Where the batch opens and closes.** `beginStep` opens a trigger batch first, before the mana pools empty. A deferred
close runs on every return: after the state-based actions in a step that grants priority, after the turn-based actions
in one that does not (Untap, a damage step with no damage), and after the cleanup step's own check. The batches of the
step's actions (declarations, combat damage) and of its state-based actions nest inside it.

**A game that is over pushes nothing.** `endTriggerBatch` discards the batch when the game is over. This applies to
every outermost batch, the resolution batch of ADR-0064 included, because Java's `checkStateBasedEffects` returns before
`addAllTriggeredAbilitiesToStack` whatever the triggers came from.

**Measured on the working change:**

| Check        | Result                                                                                               |
| ------------ | ---------------------------------------------------------------------------------------------------- |
| Engine tests | Green with no other change                                                                           |
| L2           | 1,215 same, 4 differs, the same differs as the baseline                                              |
| Replay       | Rakdos seed 3 replays the same to its end; all seven recorded games now replay the same to their end |

Option 1 hides what L3 exists to find.

Option 2 fixes rakdos seed 3 but leaves the deaths from combat damage in a second pass. That order is a CR 603.3b
difference a later recording would find.

## Consequences

**Good:**

- The triggers of a step's turn-based actions and of the state-based actions after them reach the stack in one APNAP
  pass, as CR 603.3b and Java order them.
- A game that ends in its state-based actions leaves no trigger on the stack, for every source of triggers.

**Bad:**

- A step that grants no priority still pushes its triggers when the step ends. Java holds them until the next priority
  (an Untap step's triggers wait for Upkeep's). No recorded game shows this yet.
- A trigger's match is still decided when its event happens (ADR-0064, option 3, still open).

**Neutral:**

- Casting and activating are unchanged: a cost's triggers wait for the spell (`holdCastTriggers`).

## Related

ADR-0010, ADR-0026, ADR-0063, ADR-0064
