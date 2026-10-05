# Port Log — Game State: M5 batch C, replacement effects

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine) — `replacementchoice.go`, `replacement.go`,
  `control.go`

Siblings: [`replacement.md`](replacement.md) (the first replacement shapes), this file (CR 616 ordering, the next
`Event$` values, `Moved` destinations).

## CR 616: the affected player chooses

Java: `ReplacementHandler.run` (`ReplacementHandler.java:169-290`). Per `ReplacementLayer`: collect the candidates
(`getReplacementList`), let the decider pick one (`chooseSingleReplacementEffect`), mark it `hasRun`, apply it, and on
an `Updated` result run the event again without it; `Replaced` ends the event; `NotReplaced` (declined, nothing to do)
tries the others. The decider is the affected player, or the affected permanent's controller.

`runReplacements` (`replacementchoice.go`) is that loop. A dispatch hands it a `collect` closure that lists the
candidates in play order (players, zones, cards, faces, lines: `eachReplacementRule`) and, per candidate, an `apply`
closure that edits the event the dispatch owns and returns Replaced, Updated or NotReplaced. `collect` runs again after
every applied effect, so a line gated on the amount (`DamageAmount$`) sees the updated one.

| Dispatch                                     | Event$       | Decider                       |
| -------------------------------------------- | ------------ | ----------------------------- |
| `drawReplaced`                               | `Draw`       | the drawing player            |
| `gainLifeReplaced`                           | `GainLife`   | the player gaining            |
| `damageReplaced` (`damageReplacement`)       | `DamageDone` | the damaged card's controller |
| `damageReplacedPlayer` (`damageReplacement`) | `DamageDone` | the damaged player            |

`PlayerController.ChooseReplacementEffect(g, decider, options []ReplacementOption) int` is the hook, with its own caller
here and nowhere else. `ScriptedController` answers from `QueueReplacementEffect(i)`; the fixture verb is
`queue replacement <index>`, the index into the candidates in play order. Like Java's human controller it does not ask
for a lone candidate, nor for several that read the same (one permanent, one `Description$`; two copies of a card are
two permanents and are asked). A controller answering out of range records a pending error and gets the first candidate
(GO-7).

Two Furnaces of Rath compound (2, 4, 8). Rhox Faithmender (twice) and Angel of Vitality (plus 1) turn a gain of 6 into
13 or 14 depending on the order.

| Scenario                                           | Proves                                            |
| -------------------------------------------------- | ------------------------------------------------- |
| `replacement-616-gain-life-doubling-then-plus-one` | index 0 applies Faithmender first: 6 gives 13     |
| `replacement-616-gain-life-plus-one-then-doubling` | index 1 applies Angel first: 6 gives 14           |
| `replacement-616-damage-doubling-then-plus-one`    | Furnace then Thor: 2 gives 5                      |
| `replacement-616-damage-plus-one-then-doubling`    | Thor then Furnace: 2 gives 6                      |
| `replacement-616-two-furnaces-quadruple-damage`    | two identical shapes compound, one question asked |

Not ported: Java's `ReplacementLayer` order (CantHappen, Control, Copy, Transform, Other) as one loop per layer. The
Copy layer (`applyCopyReplacements`) and the `Prevent$ True` / `Layer$ CantHappen` shapes run in their own functions
before these four, which is Java's order; a `Draw`/`GainLife`/`DamageDone` line with another `Layer$` is not resolved. A
`ReplaceWith$` the dispatch cannot resolve is a candidate whose `apply` does nothing, so with two or more live
candidates the player can be asked about one that then declines. `Draw`'s `ReplaceWith$ DB$ Draw` draws directly rather
than raising a new `Draw` event, so a second draw replacement does not see the replacement's own cards (Java does, with
the first marked `hasRun`).
