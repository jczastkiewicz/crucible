# ADR-0054 — Mana Abilities Are Activated Inside a Mana Payment Through One Controller Decision

- **Status:** Proposed
- **Date:** 2026-11-02
- **Deciders:** Crucible session (plan B4, R11a)

## Context

CR 605.3a lets a player activate mana abilities whenever they are asked to pay a mana cost, priority or not. Go offers
mana abilities only as priority answers (`ActionTapForMana`, `ActionManaAbility`, ADR-0026), and every mana payment
(`payManaCostX`, `manapay.go`) spends the pool as it stands. A cost paid outside priority therefore pays from a pool
that was emptied at the last step boundary: an attack tax (`payAttackCost`, `attackcost.go`), an `UnlessCost$` at
resolution, a triggered ability's mana cost. Ghostly Prison and Propaganda can never be paid in a real game, and every
"counter unless its controller pays {N}" counters.

Java has one controller call per mana payment:
`PlayerController.payManaCost(toPay, costPartMana, sa, prompt, matrix, effect)` (`PlayerController.java:370`), reached
from `CostPartMana.payAsDecided` (`CostPartMana.java:169`) for every cost with a mana part, casts included, and from
`PlaySpellAbility.java:416` for effect payments. The human controller opens `InputPayMana`, where the player spends
floating mana and activates mana abilities in any order until the cost is paid or cancelled
(`PlayerControllerHuman.java:2726-2737`, `PlaySpellAbility.java:490-505`). The AI hands the whole payment to
`ComputerUtilMana.payManaCost` (`PlayerControllerAi.java:1395-1397`), which spends the pool first and then taps sources.
Both controllers mutate the game inside the call.

## Decision Drivers

- R18: Go's controller decisions follow Java's `PlayerController` methods, one Go decision per Java call.
- R21a/R21c: the recorder logs one decision per Java `PlayerController` call, and the replay feeds the answers back. A
  per-activation loop would record N decisions where Java records one.
- ADR-0019 / ADR-0026: a controller answers, the engine applies. No controller mutates `*Game` inside a callback, or a
  replayed answer cannot be checked.
- `ScriptedController` panics on a missing answer, with one documented exception: an empty `TakeAction` queue answers
  pass (ADR-0019). Every test that preloads the pool must keep passing unchanged.
- M7 (R22a) ports `ComputerUtilMana`; the method shape chosen here is the one it answers.

## Considered Options

1. One decision per payment, returning an ordered list of mana activations the engine applies before paying from the
   pool.
2. A loop: the engine asks for one mana `Action` at a time until the controller passes. Rejected: N decisions per Java
   call, which breaks R21a's one-to-one log, and an extra round trip per land.
3. The engine taps sources itself (an auto-payer in the engine). Rejected: choosing which sources to tap is the AI's
   decision (`ComputerUtilMana`), not a rule; the engine would carry a strategy.
4. Leave payments outside priority to the pool. Rejected: attack taxes and `UnlessCost$` stay unpayable, a known
   divergence on every Propaganda or Mana Leak game.

## Decision

Option 1. `PlayerController` gains `PayManaCost(g *Game, payer PlayerID, toPay mana.Cost, paidFor CardID) []Action`,
Java's `payManaCost`. The answer is an ordered list of `ActionTapForMana` and `ActionManaAbility` actions (ADR-0026's
mana kinds; any other kind is ignored). `payManaCostX` applies each through `TapLandForMana` or `ActivateManaAbility`,
in order, skipping one that is illegal now, then pays `toPay` from the pool exactly as today.

The decision is asked on every mana payment, as Java asks `payManaCost` on every cost with a mana part, so a recorded
Java game and its Go replay ask it the same number of times. `ScriptedController` answers an empty list when its
`QueuePayManaCost` queue is empty, the second exception to its panic policy beside `TakeAction`'s pass (ADR-0019), and
for the same reason: "activate nothing more" is the answer a test that preloads the pool means. A pool that covers the
cost is spent first in both: Java's AI pays from the pool before tapping (`ComputerUtilMana`), and Go pays from the pool
after the listed activations.

A mana ability activated from the list pays its own mana cost through `PayManaCost` too, so a nested decision may open.
`g.payingFor` already stacks the `payContext` and restores it, so the outer payment's matrix and item are read again
after the nested one. When the payment then fails, the mana produced stays in the pool and the tapped sources stay
tapped. Java's human cancel refunds the mana paid and undoes undoable abilities (`InputPayMana`); Go has no undo, a
deviation logged in `quirks.md`.

The smoke probe pre-fills pools and its controller answers no actions, so `card-smoke` counts do not move. The sim's
default-answer controller answers no actions until R22a's `ComputerUtilMana` port answers this decision.

## Consequences

**Good:** attack taxes, `UnlessCost$` at resolution and triggered mana costs can be paid with untapped lands, as CR
605.3a allows; the decision has Java's shape, so the recorder and replay match it one to one, and M7 ports
`ComputerUtilMana` against it. **Bad:** every `PlayerController` implementation gains a method, asked on every mana
payment; mana produced for a failed payment is not undone. **Neutral:** casts during priority may still pre-tap through
priority answers; both paths fill the same pool.

## Related

ADR-0019, ADR-0026, `porting/port-log/game-state.md`, `porting/port-log/quirks.md`, plan B4 (R11a), R18, R21a, R22a
