# ADR-0063 — Replay Events: Anchors in Order, Everything Between as Net Changes

- **Status:** Accepted
- **Date:** 2026-11-12
- **Deciders:** Crucible session (R21b), accepted by the owner

## Context

ADR-0010 L3 diffs the full-game event stream of a Java game and its Go replay. ADR-0059 and ADR-0062 fix the decision
log; neither says which events are compared or how. Java fires Forge `GameEvent`s on a Guava `EventBus`
(`Game.java:1143-1145`, synchronous, nested posts delivered FIFO after the current handler). Go emits `engine.Event`s to
a `Sink` (ADR-0013). One recorded game (Boros Energy against Azorius Control, seed 1) and the fire sites give these
asymmetries:

| What                        | Java                                                                                                                                                                                                                                                                     | Go                                                                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| Turn and step               | `GameEventTurnBegan` (`PhaseHandler.java:207`, fired after the turn number increments), `GameEventTurnPhase` (`:255`)                                                                                                                                                    | `TurnBegan`, `PhaseBegan` (`turn.go:68, 362`)                                                       |
| Something goes on the stack | One `GameEventSpellAbilityCast` (`MagicStack.java:347`) for a spell, an activated ability and a trigger                                                                                                                                                                  | `AbilityActivated` from `PushAbility` (`stack.go:41`) for all three; a spell also emits `SpellCast` |
| It resolves                 | `GameEventSpellResolved` (`MagicStack.java:698`), fizzled or not                                                                                                                                                                                                         | `AbilityResolved` (`stack.go:232`), not for a fizzled ability                                       |
| A card moves                | `GameEventCardChangeZone` per card (`GameAction.java:597`), including a cast to the stack and a token's creation (`[null] -> [Battlefield]`, with the token's id); a token leaving the battlefield moves like any card, and no event once it is out of play (`:124-127`) | `ZoneChanged` per card (`game.go:1415`), a draw included                                            |
| A draw                      | The zone change alone                                                                                                                                                                                                                                                    | `ZoneChanged` and `CardDrawn` (`turn.go:717`)                                                       |
| Damage                      | `GameEventCardDamaged` for a creature (`Card.java:6399`), `GameEventPlayerDamaged` (`Player.java:741`); planeswalker and battle damage is a counter change                                                                                                               | `DamageDealt` for every target (`combatdamage.go:440, 526`)                                         |
| Life                        | `GameEventPlayerLivesChanged` with old and new totals; damage's life loss is batched per damage step (`Player.java:841-842`)                                                                                                                                             | `LifeChanged` with the delta, per loss or gain                                                      |
| Counters                    | `GameEventCardCounters` old and new totals per add or remove (`Card.java:1799, 1927`)                                                                                                                                                                                    | `CounterChanged` with the delta (`game.go:1032`)                                                    |
| Mana lost at a step's end   | `GameEventManaPool` `Cleared`, no amount (`ManaPool.java:173`)                                                                                                                                                                                                           | `ManaLost` with the amount                                                                          |

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

| Normalised | Java source                                                                  | Go source                               | Kind of event |
| ---------- | ---------------------------------------------------------------------------- | --------------------------------------- | ------------- |
| `turn`     | `TurnBegan`: turn number from the event, active seat                         | `TurnBegan`                             | Anchor        |
| `step`     | `TurnPhase`                                                                  | `PhaseBegan`                            | Anchor        |
| `push`     | `SpellAbilityCast`: host card, controller                                    | `AbilityActivated`                      | Anchor        |
| `resolve`  | `SpellResolved` that did not fizzle: host card                               | `AbilityResolved`                       | Anchor        |
| `end`      | `GameOutcome`: winning seat or draw                                          | `GameEnded`                             | Anchor        |
| `move`     | `CardChangeZone`: card, from, to, except an effect card's                    | `ZoneChanged`, except an effect card's  | Between       |
| `damage`   | `CardDamaged`, `PlayerDamaged`: source, target, amount                       | `DamageDealt` to a creature or a player | Between       |
| `life`     | `PlayerLivesChanged`: new minus old                                          | `LifeChanged`                           | Between, net  |
| `counter`  | `CardCounters` new minus old; a player's counters against the last ones seen | `CounterChanged`                        | Between, net  |

Go's events carry the host card alone, so `push` compares host and controller; the Java record adds the ability's
ADR-0059 identity for the report. A fizzled ability has a `push` and no `resolve` on both sides, because Go emits no
`AbilityResolved` for it. `damage` has no combat flag: `CardDamaged` carries none, and a combat damage step's own damage
is the interval after its `step` anchor, before any `push`. `PlayerCounters` fires with no type when a player's counters
are cleared or replaced (`Player.java:952, 989`), so the recorder compares each player's counters with the last ones it
saw. Turn and step of a between event come from the anchors before it, not from the event. An effect card (Forge's
carrier of a duration effect, `GamePieceType.EFFECT`) is not a rules object: its moves and counters are compared on
neither side, and as the host of a `push` or `resolve` every effect card compares equal, because Java names it "<host>
(<id>)'s Effect" with an id Go never pairs.

Dropped: Go's `SpellCast` (its `push` is the `AbilityActivated` beside it), `CardDrawn` (the zone change carries it),
`ManaLost` (Java's event has no amount), `Phased` (until both sides are checked against a game that phases); Java's UI,
log, sound, priority and statistics events.

**Comparison.** Anchors are compared as a sequence. Between two consecutive anchors, `move` and `damage` events are
compared as a sorted multiset, and `life` and `counter` as one net delta per (entity, counter type). The first anchor
that differs, or the first interval whose contents differ, is the divergence; it is reported with its turn, step and the
anchors on both sides. A `step` followed directly by another `step`, a `turn` or the `end` is dropped on both sides
first: Java fires `TurnPhase` for every step and only then checks whether it skips it (`PhaseHandler.java:255`, then
`isSkippingPhase` in `onPhaseBegin`), while Go begins no skipped step (`turn.go` `drivenSkips`). A step in which nothing
happened is no rules evidence.

**Log format.** Event records make the replay log version 2. The Go reader refuses version 1. Forge's `EventBus` logs a
subscriber's exception and carries on, so the recorder catches its own, counts them in the header and exits 3, and the
Go reader refuses a log with any: a missing event would read as a divergence.

**Not compared yet:** tapping and untapping, and characteristic changes (`GameEventCardTapped`,
`GameEventCardStatsChanged`) have no Go kind, and adding one is an ADR-0013 change. Declared attackers and blockers are
already in the decision log as the answers of `declareAttackers` and `declareBlockers`.

## Consequences

**Good:** Plumbing differences (batched life loss, hash-ordered mass moves, the event a spell fires twice in Go) cannot
reach the report. Every divergence has a turn, a step and a stack item. A token's creation carries its Java id, so the
same records build ADR-0059's id map.

**Bad:** A step that one engine enters and the other skips is invisible when nothing happens in it. Two errors inside
one interval that cancel out (a card moved twice, back where it was; life lost and gained by the same amount) are
invisible. A missing tap or a wrong power is invisible until it changes a later move, damage or life total.

**Neutral:** The normaliser fixes on Java's step names and zone names; Go's are mapped onto them.

## Related

ADR-0010, ADR-0013, ADR-0059, ADR-0062
