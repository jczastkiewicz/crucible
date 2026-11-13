# ADR-0064 — Triggers Wait for the Resolution and the State-Based Actions

- **Status:** Accepted
- **Date:** 2026-11-13
- **Deciders:** Crucible session (R21c), accepted by the owner

## Context

CR 603.3: a triggered ability waits until a player would next receive priority, then goes on the stack with every other
waiting trigger, in APNAP order, each player ordering their own. CR 117.5: state-based actions are checked first, and
the triggers they fire wait with the rest.

Java does this with a frozen stack. While an ability resolves, a trigger that fires waits (`TriggerHandler`
`waitingTriggers`). After the resolution Java fires `GameEventSpellResolved`, ends a control grant, and unfreezes the
stack (`MagicStack.java:690-700`, `finishResolving`, `unfreezeStack` `:163-170`). The unfreeze runs the waiting triggers
into `simultaneousStackEntryList`. `checkStateEffects` adds the triggers its own actions fire to the same list, and
`addAllTriggeredAbilitiesToStack` puts the list on the stack in one APNAP pass with one `orderAndPlaySimultaneousSa` per
player. A trigger in that list counts as on the stack: `hasSourceOnStack` (`MagicStack.java:1016-1035`, CR 704.5v's
battle at zero defense, a chapter, a dungeon's last room) and `hasStateTrigger` (`:956-968`, a state trigger already
triggered) read it.

Go collects a trigger batch while one event runs several modes, and while one effect resolves (`beginTriggerBatch`,
`resolveEffectBatched`, `trigger.go:2609`). The resolution's batch closes inside `Registry.Resolve`, so its triggers go
on the stack before `AbilityResolved`, before the spell leaves the stack and before the state-based actions. A creature
that died in those actions puts its trigger on the stack on top of them, in a second APNAP pass.

The R21c replay (`crucible replay`) finds this first in four of the seven recorded games: Seasoned Pyromancer's,
Commercial District's, Haliya's and Arboreal Grazer's enters triggers are pushed before the spell resolves.

## Decision Drivers

- L3 replay parity (ADR-0010): Java's event order and Java's trigger order
- CR 603.3 and CR 117.5
- The change stays inside the batch mechanism Go already has, with no new kind of state

## Considered Options

1. Keep the inline push; reorder the events in the replay's normaliser
2. One trigger batch per resolution: open before the ability resolves, close after the state-based actions
3. Java's waiting queue: record a trigger's event, decide its match when the stack unfreezes

## Decision

Option 2.

**Where the batch opens and closes.** `resolveTop` opens a trigger batch once it has popped the ability. It closes the
batch after four things, in order: the resolution, `AbilityResolved`, the spell's move off the stack, and the
state-based actions. The close is deferred, so it runs after the control grant ends, as Java's unfreeze does. The
resolution's own batches, and the batches the state-based actions open, nest inside it. Every trigger of the resolution
and its state-based actions therefore reaches the stack in one APNAP pass.

**A restart keeps the batch.** A card returned by Karn's restart keeps its enters trigger. That trigger resolves in the
restarted game (ADR-0034), as in Java.

**A waiting trigger counts as on the stack.** `hasSourceOnStack`, `hasChapterOnStack` and `alwaysOnStack` read the batch
as well as the stack, as Java's two readers read `simultaneousStackEntryList`.

**Measured on the working change:**

| Check        | Result                                                                                                                                                          |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Engine tests | Green once the three readers were changed. Without them a battle at zero defense died with its trigger waiting, and Varis's Wolf and The Atropal swapped places |
| L2           | 1,215 same, 4 differs, the same differs as the baseline                                                                                                         |
| Replay       | All four games get past the push they diverged at                                                                                                               |

Option 1 hides what L3 exists to find. The order is not cosmetic: triggers fired by the state-based actions would still
be ordered apart from the resolution's. A reorder in the normaliser also has to guess where a fizzled ability's resolve
would have been, because Go emits none.

Option 3 also moves when a trigger's condition is read. That is the open `collectTriggerForWaiting` row in
`game-state.md`. It touches every trigger-check site, and no recorded game shows it yet.

## Consequences

**Good:**

- A resolution's triggers and its state-based actions' triggers are ordered together, once per player, as CR 603.3b and
  Java order them.
- The replay's push and resolve anchors agree.

**Bad:**

- A trigger's match is still decided when its event happens, not when the stack unfreezes (option 3, still open).
- Every new reader of "a trigger of this card is on the stack" has to read the batch too.

**Neutral:**

- Casting and activating are unchanged: a cost's triggers wait for the spell (`holdCastTriggers`). A step's turn-based
  actions wait the same way, for the state-based actions after them (ADR-0065).

## Related

ADR-0010, ADR-0019, ADR-0034, ADR-0063, ADR-0065, `porting/port-log/game-state.md` ("Not ported yet":
`collectTriggerForWaiting`)
