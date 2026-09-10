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

## Not ported yet

| Missing                                                       | Lands |
| ------------------------------------------------------------- | ----- |
| `CardState`, counters, damage, attachments as their own types | M4    |
| The ~110-method `PlayerController` interface                  | M4    |
| `GameState` fixture load and dump                             | M4    |
| Stack, combat, phases, priority                               | M5    |
