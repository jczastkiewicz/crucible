# Port Log — Game State

- **Java counterpart:** `forge-game/src/main/java/forge/game/Game.java`, `card/Card.java` (8,105 LOC),
  `player/Player.java`, `zone/Zone.java`, `zone/ZoneType.java`
- **Go:** [`internal/engine`](../../../crucible/internal/engine)

The arena, and the handles that address it. Everything else in the engine indexes into this.

## What it does

`Game` owns every entity in one game. Cards and players live in slices; a `CardID` is an index. Zones are keyed by
`(type, owner)`, because a dozen callers have that pair and not the player.

## Handles, not pointers

A `Card` holds no `*Game` and no `*Player` — a `PlayerID` for its controller, a `CardID` for what it is attached to.
That is what makes a game copy a slice copy with nothing to remap, and it is why `valid` and `expr` are separate
packages rather than core members (ADR-0003, ADR-0009).

| Rule                                | Reason                                                                                                     |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Slot 0 is `NoCard` / `NoPlayer`     | The zero value of a handle field means "none". Without it, an uninitialised field aliases the first entity |
| Handles are never reused            | LKI snapshots, `Remembered` lists and delayed triggers hold handles to objects that are gone               |
| A bad handle panics                 | An out-of-range handle is an invariant breach, not something a card script can cause (GO-7)                |
| Every zone change takes a timestamp | Continuous effects order by it, so a card that leaves and returns is a new object to the layer system      |

`EntityID` is a card or a player in one 4-byte value, tagged by the high bit. It is stored in the thousands — target
lists, remembered lists, LKI — so it is a handle that compares with `==` rather than a struct with a kind field. The
accessors are `AsCard` and `AsPlayer`, not `Card` and `Player`, because a method named `Card` in the leaf file reads as
a reference to the `Card` type and `enginelint` scores it as one.

## Zone order is the contract

A `Zone` is an ordered set, not a set. A library is a stack, a graveyard's order decides `CardsInGraveyard` and
delirium, and the battlefield's order breaks ties between otherwise simultaneous triggers (GO-12).

A card's zone is recorded twice — in the zone's list and as a reverse index on the card — so `Move` is the only thing
allowed to write either. Two representations of one fact is how a card ends up in two zones at once.

## `compile.DB`, not `carddb.DB`

ADR-0009 names the shared database `*carddb.DB`, but it cannot live in `carddb`: `compile` imports `carddb`, so a
`carddb` type holding `*compile.Card` would cycle. It is `compile.DB` — the compiled corpus, built once at startup,
never written again, shared by pointer across every game goroutine (ADR-0005).

## The card's mutable parts

`Card` carries identity and location; the rest is split by concern, because 8,105 lines of Java's `Card` has to land
somewhere and these are the pieces with their own invariants (ADR-0009).

| Type       | Invariant it exists to hold                                                                                 |
| ---------- | ----------------------------------------------------------------------------------------------------------- |
| `Counters` | A count is never stored at zero — "a counter of any kind" must say no for a card that gained and lost one   |
| `Damage`   | A deathtouch source sets a flag later ordinary damage cannot clear; the SBA checks the flag, not the amount |
| `Memory`   | Three independent lists, because scripts clear them independently                                           |
| Attachment | One fact stored twice, so only `Game.Attach` and `Game.Unattach` write either side                          |

`CounterType` is a named string rather than a generated enum. Java's `CounterEnumType` has 233 constants and
`CounterType` wraps it anyway to allow keyword counters the enum does not list, so a generated enum would still need the
escape hatch and scripts write these names directly.

`Memory.Remembered` holds `EntityID`, not `CardID`: `RememberObjects$ ChosenCard & Player.IsRemembered` puts a player
and a card in one list.

## `collect.OrderedMap`

Counters need a key-to-count map with a stable order, and CLAUDE.md already named `collect.OrderedMap` as the tool for
where Java used an ordered collection. It did not exist; it does now. Insertion order rather than sorted order, because
that is what Java's `LinkedHashMap` gives and parity is measured against Java. Updating a value keeps its position — a
counter changing count must not jump to the end of every report — and deleting shifts rather than swapping the last
entry into the hole, which costs O(n) and is the whole point of the type.

## Effect dispatch

`Registry` is an array indexed by `APIType`, not a map: dispatch sits under the stack resolution loop, so it is an index
into an interface value with no hashing and no allocation (ADR-0008). The constants are generated from Forge's `ApiType`
enum by `tools/genapitype` — 202 of them — so a new API upstream is a regenerated file and a build failure rather than a
gap found on card 12,004.

An unregistered API returns `ErrUnimplemented` naming the API, and does not panic. Most of the array is empty for the
whole port; a gap is a tracked hole in coverage (ADR-0011), not an invariant breach, and one unimplemented API must fail
its own game and no more (GO-7).

`Effect` implementations are stateless shared values. That is Java measured rather than assumed: `ApiType`'s
`isStateLess` parameter defaults to true and the count of constants passing `false` is zero.

## Cloning

`Game.Clone` is what the AI's lookahead runs on, so its cost decides search depth. Handles are indices, so nothing is
remapped: there is no equivalent of Java's `CopiedGameObjectMap`, which is the point of addressing entities by handle.

"A slice copy" is the shape but not the whole job. A `Card` owns collections behind pointers — counters, three memory
lists, attachments — and copying the slice alone would leave clone and original writing to the same ones. Each is copied
when it exists and left nil when it does not, which is most cards most of the time.

| Part                                 | Clone treatment | Reason                                                                                                    |
| ------------------------------------ | --------------- | --------------------------------------------------------------------------------------------------------- |
| `compile.DB`                         | shared          | Immutable, one per process; copying it would be the costliest thing                                       |
| `javarand.Rand`                      | copied by value | The clone continues the stream; sharing it would let the AI's exploration change what the real game rolls |
| Zones, counters, memory, attachments | deep            | Otherwise the lookahead mutates the real game                                                             |

Measured on a mid-game board — 118 cards, two players, counters on twelve permanents:

```console
BenchmarkGameClone-14    13851 ns/op    26688 B/op    193 allocs/op
```

The allocation count is gated, not just recorded (GO-16). Allocations rather than nanoseconds, because allocation counts
are deterministic across machines and CI runners are not: a time bound would be flaky or so loose it catches nothing.
The budget has headroom; it exists to catch a change in shape, such as something shared becoming copied per card.

## Events

`Event` is a flat fixed-size record, and `Sink` takes it by value. At 10^5 games the stream is the largest thing the
runner produces, so a pointer in the record would be an allocation per event and a pointer chase per read; the
kind-specific payload is a `Detail` number rather than a field per kind.

The recorder folds synchronously inside the game's goroutine. A game owns its state exclusively, so its recorder can
too, and folding in place means there is no queue to fill, no drop policy to get wrong and no scheduling input to the
output (ADR-0005, ADR-0013).

`DiscardSink` is what a cloned game gets. The AI explores lines that never happened, and a clone holding the real sink
would record imagined casts as real.

`SchemaVersion` is stamped separately from the metrics version: the schema is what was emitted, the metrics version is
how it was interpreted, and they move independently.

Two names differ from ADR-0013's sketch. The event's card field is `Source`, not `Card`, and `EntityID`'s accessors are
`AsCard` and `AsPlayer` — a field or method named `Card` in a package that declares a `Card` type reads as a reference
to it, and `enginelint` scores it as one.

`PhaseType` carries the names card scripts write, which are Java's second constructor argument rather than the enum
constant: `Phase$ End of Turn` has spaces in it, so these are not identifiers. Combat is a contiguous range, which is
what lets a trigger restricted to combat test a bound instead of listing six steps.

## Not ported yet

| Missing                                                       | Lands |
| ------------------------------------------------------------- | ----- |
| `CardState`, counters, damage, attachments as their own types | M4    |
| The ~110-method `PlayerController` interface                  | M4    |
| `GameState` fixture load and dump                             | M4    |
| Stack, combat, phases, priority                               | M5    |
