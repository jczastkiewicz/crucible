# ADR-0063 — Replay Events: Anchors in Order, Everything Between as Net Changes

- **Status:** Proposed
- **Date:** 2026-11-12
- **Deciders:** Crucible session (R21b), owner

## Context

ADR-0010 L3 diffs the full-game event stream of a Java game and its Go replay. ADR-0059 and ADR-0062 fix the decision
log; neither says which events are compared or how. Java fires Forge `GameEvent`s on a Guava `EventBus`
(`Game.java:1143-1145`, synchronous, nested posts delivered FIFO after the current handler). Go emits `engine.Event`s to
a `Sink` (ADR-0013). One recorded game (Boros Energy against Azorius Control, seed 1) and the fire sites give these
asymmetries:

| What                        | Java                                                                                                                                                                                                                     | Go                                                                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| Turn and step               | `GameEventTurnBegan` (`PhaseHandler.java:207`, fired before the turn number increments), `GameEventTurnPhase` (`:255`)                                                                                                   | `TurnBegan`, `PhaseBegan` (`turn.go:68, 362`)                                                       |
| Something goes on the stack | One `GameEventSpellAbilityCast` (`MagicStack.java:347`) for a spell, an activated ability and a trigger                                                                                                                  | `AbilityActivated` from `PushAbility` (`stack.go:41`) for all three; a spell also emits `SpellCast` |
| It resolves                 | `GameEventSpellResolved` (`MagicStack.java:698`)                                                                                                                                                                         | `AbilityResolved` (`stack.go:232`)                                                                  |
| A card moves                | `GameEventCardChangeZone` per card (`GameAction.java:597`), including a cast to the stack and a token's creation (`[null] -> [Battlefield]`, with the token's id); none for a token leaving the battlefield (`:124-127`) | `ZoneChanged` per card (`game.go:1415`), a draw included                                            |
| A draw                      | The zone change alone                                                                                                                                                                                                    | `ZoneChanged` and `CardDrawn` (`turn.go:717`)                                                       |
| Damage                      | `GameEventCardDamaged` for a creature (`Card.java:6399`), `GameEventPlayerDamaged` (`Player.java:741`); planeswalker and battle damage is a counter change                                                               | `DamageDealt` for every target (`combatdamage.go:440, 526`)                                         |
| Life                        | `GameEventPlayerLivesChanged` with old and new totals; damage's life loss is batched per damage step (`Player.java:841-842`)                                                                                             | `LifeChanged` with the delta, per loss or gain                                                      |
| Counters                    | `GameEventCardCounters` old and new totals per add or remove (`Card.java:1799, 1927`)                                                                                                                                    | `CounterChanged` with the delta (`game.go:1032`)                                                    |
| Mana lost at a step's end   | `GameEventManaPool` `Cleared`, no amount (`ManaPool.java:173`)                                                                                                                                                           | `ManaLost` with the amount                                                                          |

Within one step, the order of events that are not stack or step boundaries is not rules evidence: mass moves loop over
hash-ordered lists, and a nested post is delivered after the handler that caused it.

## Decision Drivers

- A difference the diff reports must be a rules difference, not an artifact of either engine's event plumbing.
- The first difference must say where it is: turn, step, and the stack item around it.
- ADR-0013: a new `engine.Event` kind or field bumps the telemetry schema version and is a separate decision.

## Considered Options

1. **Compare both streams line by line.** Rejected: the asymmetries above make every game differ in its first turn.
2. **Cut the streams into windows at Java's controller calls.** Rejected: Go asks different questions at different
   points from Java's calls (ADR-0062), so the windows have no Go counterpart.
3. **Compare a few anchor events in order, and the events between two anchors as net changes.** Chosen.

## Decision

**Normalised events.** Java's recorder writes normalised events into the replay log; Go maps its `engine.Event`s to the
same typed shape, card ids translated through the replay's Java-id map (ADR-0059). Both sides drop what the other cannot
see.

| Normalised | Java source                                                    | Go source                                             | Kind of event |
| ---------- | -------------------------------------------------------------- | ----------------------------------------------------- | ------------- |
| `turn`     | `TurnBegan`: turn number from the event, active seat           | `TurnBegan`                                           | Anchor        |
| `step`     | `TurnPhase`                                                    | `PhaseBegan`                                          | Anchor        |
| `push`     | `SpellAbilityCast`: host card, ability identity                | `AbilityActivated`                                    | Anchor        |
| `resolve`  | `SpellResolved`: host card, fizzled                            | `AbilityResolved`                                     | Anchor        |
| `end`      | `GameOutcome`: winning seat or draw                            | `GameEnded`                                           | Anchor        |
| `move`     | `CardChangeZone`: card, from, to                               | `ZoneChanged`, except a token leaving the battlefield | Between       |
| `damage`   | `CardDamaged`, `PlayerDamaged`: source, target, amount, combat | `DamageDealt` to a creature or a player               | Between       |
| `life`     | `PlayerLivesChanged`: new minus old                            | `LifeChanged`                                         | Between, net  |
| `counter`  | `CardCounters`, `PlayerCounters`: new minus old                | `CounterChanged`                                      | Between, net  |

Dropped: Go's `SpellCast` (its `push` is the `AbilityActivated` beside it), `CardDrawn` (the zone change carries it),
`ManaLost` (Java's event has no amount), `Phased` (until both sides are checked against a game that phases); Java's UI,
log, sound, priority and statistics events.

**Comparison.** Anchors are compared as a sequence. Between two consecutive anchors, `move` and `damage` events are
compared as a sorted multiset, and `life` and `counter` as one net delta per (entity, counter type). The first anchor
that differs, or the first interval whose contents differ, is the divergence; it is reported with its turn, step and the
anchors on both sides.

**Log format.** Event records make the replay log version 2. The Go reader refuses version 1.

**Not compared yet:** tapping and untapping, and characteristic changes (`GameEventCardTapped`,
`GameEventCardStatsChanged`) have no Go kind, and adding one is an ADR-0013 change. Declared attackers and blockers are
already in the decision log as the answers of `declareAttackers` and `declareBlockers`.

## Consequences

**Good:** Plumbing differences (batched life loss, hash-ordered mass moves, the event a spell fires twice in Go) cannot
reach the report. Every divergence has a turn, a step and a stack item. A token's creation carries its Java id, so the
same records build ADR-0059's id map.

**Bad:** Two errors inside one interval that cancel out (a card moved twice, back where it was; life lost and gained by
the same amount) are invisible. A missing tap or a wrong power is invisible until it changes a later move, damage or
life total.

**Neutral:** The normaliser fixes on Java's step names and zone names; Go's are mapped onto them.

## Related

ADR-0010, ADR-0013, ADR-0059, ADR-0062
