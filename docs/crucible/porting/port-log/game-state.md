# Port Log — Game State

- **Java counterpart:** `forge-game/src/main/java/forge/game/Game.java`, `GameAction.java` (2,897 LOC), `card/Card.java`
  (8,105 LOC), `player/Player.java`, `player/PlayerController.java`, `zone/Zone.java`, `zone/ZoneType.java`,
  `phase/PhaseHandler.java` (1,324 LOC), `mulligan/*` (338 LOC across 7 files)
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

| Part                                 | Clone treatment | Reason                                                                                                         |
| ------------------------------------ | --------------- | -------------------------------------------------------------------------------------------------------------- |
| `compile.DB`                         | shared          | Immutable, one per process; copying it would be the costliest thing                                            |
| `javarand.Rand`                      | copied by value | The clone continues the stream; sharing it would let the AI's exploration change what the real game rolls      |
| Zones, counters, memory, attachments | deep            | Otherwise the lookahead mutates the real game                                                                  |
| Stack (`[]Ability`)                  | deep            | `Ability` has no pointer fields, but a shared backing array would still let a push on one alias the other      |
| `Card.PT`                            | deep            | Same reasoning as the stack: `PTEffect` has no pointer fields, but the slice still needs its own backing array |

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

## Controller

`PlayerController` is where the game asks a player to decide something. Ported from
`forge-game/src/main/java/forge/game/player/PlayerController.java`, which has 110 abstract methods — most of them take
`SpellAbility`, `Combat`, `ReplacementEffect` or other types that do not exist until the stack, combat and layer system
land in M5. Porting the full interface now would mean inventing those types speculatively, ahead of the milestone that
actually designs them, so only the four methods answerable with today's engine are here: `ChooseStartingPlayer`,
`ChooseStartingHand`, `MulliganKeepHand`, `TuckCardsViaMulligan`. Each gets added when its own caller does, same as
these four — mulligans and the starting-player choice have real callers in `GameAction` and `mulligan/`, even though
`mulligan/` is not ported yet.

Forge instantiates one controller per player. Go's methods take the deciding player as an explicit `PlayerID` instead of
binding an instance to one seat, so `ScriptedController` — the fixture-driven implementation TEST-5 runs scenarios
against — answers for every player in a game from one value, with no per-player state to wire up (GO-2, PORT-1).

A `ScriptedController` queue running dry mid-scenario panics rather than returning a zero value: it is a
fixture-authoring mistake, not a rules question a card script could cause, so it has to fail loud (GO-7).

## State-based actions

`CheckStateBasedActions` is `GameAction.checkGameOverCondition`, `Player.checkLoseCondition`, `stateBasedAction704_5q`
and `handlePlaneswalkerRule`, plus `destroyLethalToughness` and `cleanupDanglingAttachments` for a slice of what
`changeZone` folds in elsewhere in Java (`## Move carries what Java gets for free`, below) — the rules answerable
without the full layer system: CR 704.5a (a player at zero or less life loses), CR 704.5c (ten or more poison counters
loses), CR 704.5q (a permanent carrying both +1/+1 and -1/-1 counters loses the smaller pile from each, in equal number
— five +1/+1 and two -1/-1 leaves three +1/+1 and none), CR 704.5g (a creature at zero or less toughness dies, Layer 7
and counters folded in), CR 704.5h (a planeswalker at zero or less loyalty dies), and a partial CR 704.5f/704.5m (an
Aura not attached to a permanent on the battlefield goes to its owner's graveyard; an Equipment or Fortification in the
same state just becomes unattached). Every other SBA in Java's loop — lethal damage, the rest of 704.5g's own toughness
(`*` with no characteristic-defining effect to replace it, or a `Count$` reference — `internal/expr` has no evaluator
yet), and the rest of 704.5f/704.5m's own legality (an Aura's `Enchant` restriction violated by something other than its
host leaving, protection, hexproof) — reads a characteristic the rest of the continuous-effect layer system computes, or
a restriction a `valid`-string evaluator would check (`internal/valid`'s own doc comment), and neither is M5 work this
has fully reached yet. A rule this port has not implemented simply never fires, the same as a real game with no
permanent that rule ever applies to — it is a coverage gap (ADR-0011), not a wrong answer.

CR 704.5q's own guard — some cards grant "counters can't be removed from CARDNAME" — is a static ability, so it is not
checked either: nothing this port can grant that effect yet, so its absence changes no card's behaviour today.

Java's own loop runs up to nine times, because one SBA firing can make another one true. `destroyLethalToughness` and
`destroyZeroLoyalty` both run before `cleanupDanglingAttachments`, not after, for exactly that reason: a creature or
planeswalker this pass destroys can leave an Aura dangling that the very same `CheckStateBasedActions` call has to
catch, the one real cascade among the six rules here. Nothing else cascades a second time — destroying a permanent
cannot itself change another one's printed toughness or loyalty count, and nothing yet grants an effect that could — so
one ordered pass is complete. A game that already ended skips every check below entirely, the same as Java:
`checkStateEffects` returns before its creature loop runs once `checkGameOverCondition` finds the game over. The loop
returns once a rule that can cascade twice lands — a card script writing to `Player.Life` or a permanent's counters
mid-check does not exist yet either.

`cleanupDanglingAttachments` needed `Card.Type()` to exist at all: `compile.Card` carried no printed characteristics
before this, only compiled ability lines, because nothing before this needed to go from a compiled card back to "what
type is it" (`## Compiled cards needed a name back` in `game-state-fixture.md` is the same shape of gap, for `Name`
instead of `Type`). `carddb.Face` already parses one (`Type cardtype.Line`, from the corpus's own `Type:` line);
`compile.Face` now carries it through unchanged, since it is a printed value `Compile` does not interpret, the same way
`Name` is copied rather than recomputed. `Card.Type()` returns the primary face's line and, for a `nil` `Def` (every
synthetic test card in this package), the zero `Line` — which matches no subtype, so a test card is never mistaken for
an Aura.

## Layer 0: printed power and toughness

`destroyLethalToughness` needed the same kind of characteristic `Card.Type()` already carries, for power and toughness
instead of the type line: `compile.Face` now also copies `Power`/`Toughness` through from `carddb.Face` unchanged,
printed text, not a number — either can be `*`, `1+*` or a `Count$` reference (`carddb.Face`'s own doc comment), which
is exactly why they stay text at this layer too. `Card.BasePower`/`BaseToughness` resolve that text to an `int` only
when it is a plain integer (`strconv.Atoi`), reporting `false` otherwise rather than a wrong number or a panic — the
same "coverage gap, not a wrong answer" contract `Type()` already keeps.

"Base" is Java's own word (`getBasePower`/`getBaseToughness`) for the printed value, CR 613's Layer 0 — before a
characteristic-defining ability (Layer 7a), a setting effect (7b), a modifying effect (7c) or a counter (CR 613.4, after
Layer 7) has applied.

## Layer 7: `PT` and `Card.Power`/`Toughness`

`layer.go` is `forge.game.staticability.StaticAbilityLayer`: the ten-value enum, Forge's own order, 7a/7b/7c split and
Layer 8 (Forge's own rule-changing bookkeeping, no CR number) included, even though only 7a/7b/7c have anything to apply
yet. `pt.go`'s `PT` is a card's set of `PTEffect`s — one continuous effect's power/toughness contribution, a layer, a
timestamp, nothing else — the same "mechanism now, content later" shape `effect.go`'s `Registry` and `stack.go`'s stack
already landed in: zero production writers, proven by tests that add a `PTEffect` directly the way `stack_test.go`
pushes a stub `Ability`.

`Card.Power`/`Toughness` (`card.go`) is what actually applies CR 613.4's ordering: sort `PT`'s effects by layer then
timestamp, then fold — `LayerCharacteristic` and `LayerSetPT` each replace the running value, `LayerModifyPT` adds to
it, and +1/+1/-1/-1 counters (`Card.Counters`, already built) apply last, after every layer. A `LayerCharacteristic`
effect can turn an unresolvable base (`*`, `BasePower`'s own `ok=false`) into a resolvable one — a
characteristic-defining ability's entire purpose — so `foldPT` starts from `(base, baseOK)` rather than requiring
`baseOK` up front. `destroyLethalToughness` (CR 704.5g, `## State-based actions`) now reads `Toughness()` instead of
`BaseToughness()`, so a creature a `LayerModifyPT` pump or an annihilated -1/-1 pile actually reduces to zero dies here
too, not only one whose printed toughness always read zero.

**Not here: CR 613.6-613.8's dependency reordering.** Java sorts effects within a layer by timestamp and then
re-evaluates whether an unapplied effect has become dependent on or independent of another as each one resolves
(`GameAction.checkStaticAbilities`'s `findStaticAbilityToApply`, 1,099-line `StaticAbilityContinuous.java`). Nothing
this port can build yet puts more than one continuous effect on the same card that could disagree about order — no
static ability content exists to generate a `PTEffect` in production at all — so `foldPT`'s plain timestamp sort is a
real port of CR 613.7's tiebreak, not a stand-in for 613.8's harder case; that case is only decidable once the first two
effects that could actually depend on each other exist to prove it against.

`PT.Clear()` runs from `Move` the moment a card leaves the battlefield, the same list `Counters`, `Damage` and `Tapped`
already clear there: a continuous effect that only applied on the battlefield does not survive the trip, and this port
has no duration tracking ("until end of turn" wearing off on its own) to model the alternative anyway.

`Player.Counters` is new here, the same type `Card.Counters` already uses: poison is the only player-level counter any
rule reads today, but nothing about "a count that is never stored at zero" is specific to what holds it. `Game.Clone`
deep-copies it for the same reason it already deep-copies a card's — sharing the underlying map would let the AI's
lookahead poison the real game.

## Loyalty is not a layer

`Card.BaseLoyalty` mirrors `BasePower`/`BaseToughness` — `compile.Face.Loyalty` carried through from `carddb.Face`'s
`InitialLoyalty`, resolved to an `int` only when it is a plain printed integer — but a planeswalker's loyalty does not
have a `Card.Loyalty()` counterpart the way power and toughness have `Power()`/`Toughness()`, because CR 121.5 does not
put loyalty through Layer 7 at all: a planeswalker's loyalty _is_ its `Loyalty` counter count (`counters.go`) from the
moment it enters the battlefield, full stop. `BaseLoyalty` only ever answers "how many counters would it enter with" —
`destroyZeroLoyalty` (CR 704.5h, `## State-based actions`) reads `Card.Counters.Count(Loyalty)` directly, not a computed
accessor that would just be that same call one level removed.

Nothing yet puts a planeswalker's starting loyalty counters on it when it enters the battlefield (CR 121.5): `Move` has
no ETB hook for any permanent's starting counters, the same gap `changeZone`'s un-ported replacement effects and
triggers already are (below). A fixture or a test sets `Loyalty` counters directly (`humancounters=LOYALTY=5`) until
that lands — `destroyZeroLoyalty` is real and correct against whatever count is there, however it got there, the same as
`destroyLethalToughness` was real before `Power`/`Toughness` folded in Layer 7.

## Move carries what Java gets for free

`GameAction.changeZone` (2,897 LOC, most of it replacement effects, triggers and last-known-information bookkeeping this
port has not reached) is not ported. One piece of it is: the part that exists only because Go's cards do not work the
way Java's do.

Java rebuilds a `Card` as a new object on every zone change (`CardCopyService.copyCard`), so a field the new object does
not carry — tapped, damage, counters, summoning sickness — is simply gone, free of charge. ADR-0009 chose the opposite:
a `CardID` is stable for the card's whole life in the game, so the same struct that was tapped on the battlefield is
still tapped after `Move` if nothing clears it. `Move` now does that clearing explicitly: leaving the battlefield clears
`Counters`, `Damage`, `PT`, `Tapped` and the card's own attachment; entering it sets `SummonSick`, since a
freshly-arrived permanent has not been under its controller's control since their last turn began (CR 302.6).

What it deliberately does not do: unattach whatever was attached _to_ the leaving card (an Equipment left behind when
its creature dies keeps pointing at a `CardID` no longer on the battlefield). That is CR 704.5f/704.5m's job, not
`Move`'s — it is an SBA, checked continuously, not something a zone change fires inline — and `## State-based actions`
above is where it landed (`cleanupDanglingAttachments`). `Game.NewCard` stays untouched by any of this: it is the
arena-allocation primitive fixture loading uses to seat a board mid-game, where a battlefield card's starting
`Tapped`/`SummonSick` is exactly what the fixture says, not a rule this port applies at construction time.

## Turn structure

`turn.go` is `PhaseHandler.java` (1,324 LOC), reduced to what does not need the stack, triggers or `SpellAbility`:
`Turn`, `ActivePlayer` and `ActivePhase` live on `Game` now (`StartTurn`, `AdvancePhase`, `SetTurnState`), and two steps
— Untap and Draw — have real bodies. Every other step (`onPhaseBegin`'s Upkeep, Main, five combat steps, End of Turn,
Cleanup cases) still just changes `ActivePhase` and nothing else, because casting, blocking, discarding to hand size and
firing a trigger all need machinery this port has not reached. `AdvancePhase` walks through them as bookkeeping only,
until each one's turn comes.

Priority (`mainLoopStep`) is not here either, on purpose. With no stack and no `PlayerController` method that can cast
anything, asking a player "do you have a legal action" always answers no — building that loop today would be a stub
standing in for a decision no one can make yet, not a real one deferred. It lands with the stack.

Two rules came along because Draw needed them to mean something real rather than silently doing nothing:

- **CR 103.7a** — the first player skips the draw step of their own first turn in a two-player game.
  `turn == 1 && len(Players()) == 2`, the same condition Java's `isSkippingPhase` uses.
- **CR 704.5b** — an attempted draw with nothing to draw loses the game. `Player.DrewFromEmptyLibrary` records the
  attempt (a one-shot flag, cleared the moment `CheckStateBasedActions` reads it, matching Java's
  `triedToDrawFromEmptyLibrary`), checked first among the loss conditions per Java's own order — its comment cites
  Lich's Mirror, a card not ported, so today the order changes nothing observable.

`CheckStateBasedActions` runs after every phase entry, not just when a card script asks: `beginPhase` calls it right
after the step's own action, the same pairing `onPhaseBegin`/`checkStateBasedEffects` make at the top of `mainLoopStep`
(CR 704.3, "whenever a player would get priority").

Turn order skips a player who has lost (`nextPlayerAfter`), which is CR 800-something's "a player who has left the game
is skipped when play passes to them" — needed the moment a 3+ player game outlives its first loser, which
`CheckStateBasedActions` already supports.

**The top of the library is index 0** of the zone's order — a design decision, not a Java fact reproduced: nothing
established a convention before this, so `drawStep` set one. A fixture author writing `humanlibrary=Top;Next;...` names
it left to right, top to bottom, and `Load` already builds cards in that order, so drawing `Cards()[0]` and returning a
mulligan's tuck to the zone's end (bottom) both fall out of the existing `Zone`/`Move` behaviour with no new API.

Not modeled, and each is a real rule some card will eventually need: Java's extra-turn and extra-phase stacks
(`AddTurnEffect`, `SkipPhaseEffect` — nothing can push onto either yet, since neither ability is implemented),
topsy-turvy phase order (a handful of effects reverse it), and CR 502.3's "this permanent doesn't untap" effects.
Skipping them today is not a gap a card can expose, because nothing that would trigger them exists yet.

## Stack

`stack.go` is `forge-game/src/main/java/forge/game/zone/MagicStack.java` (1,025 LOC), cut down to CR 405's container and
CR 405.5/608's resolve loop. `PushAbility`, `StackLen`, `StackTop` and `ResolveStack` are new methods on `Game`, not a
new type: `Ability` already said "one resolvable ability on the stack" when `effect.go` landed it, so the stack itself
is `[]Ability`, last element on top. `ResolveStack` pops the top, dispatches it through the `Registry` the caller
supplies, emits `AbilityResolved`, then runs `CheckStateBasedActions` before popping what is now on top -- the same
pairing `beginPhase` already runs after a turn-based action.

`Ability` (and `APIType` with it) moved out of `effect.go` into a new `ability.go` to make `Game.stack []Ability` legal.
`Effect.Resolve(g *Game, a *Ability) error` already made the `effect` group depend on `game`; adding an `Ability` field
to `Game` itself would have made `game` depend on `effect` right back — a cycle `enginelint` is built to catch, not a
hole it missed. Splitting the vocabulary (`APIType`, `Ability`) from the dispatch machinery that resolves one against a
`*Game` (`Effect`, `Registry`) put the former below both `game` and `effect`, the same position `id.go` already holds
relative to everything else.

Most of `MagicStack.java` is not here: `freezeStack`/`unfreezeStack` (holding new pushes while one ability is still
resolving), `addSimultaneousStackEntry` (CR 603.3b's "your own simultaneous triggers, in an order you choose"), and
`undoStack` all exist to serve a second ability arriving on top of one still resolving -- and nothing can make that
happen yet. Casting has no cost payment or targeting to drive it, and triggered abilities have no firing pipeline
(matching a trigger's `ValidCard`-shaped conditions against an event needs a card/game evaluator for the `valid` grammar
that does not exist -- `internal/valid` parses the grammar today, nothing evaluates it). This is the same "mechanism
now, content later" shape `effect.go`'s `Registry` already landed in: zero production callers, proven by tests that push
a stub `Ability` the way `effect_test.go` registers a stub `Effect` (`stack_test.go`).

`ResolveStack` is also CR 117's priority algorithm, for the one case this port can play out today. Priority's real job
-- offering every player, in APNAP order, a chance to respond to what is on top before it resolves -- needs a
`PlayerController` method that can activate or cast something, which does not exist (`## Controller`). With nobody able
to respond, every priority pass is a pass in succession, so the top item always resolves next; `ResolveStack` encodes
exactly that degenerate case rather than a full pass-tracking loop nothing could yet exercise.

`turn.go`'s `beginPhase` does not call `ResolveStack`. Doing so today would be a no-op on every call -- nothing pushes
an ability in production yet -- and a call site that can never do anything is exactly the "stub standing in for a
decision no one can make yet" the turn-structure port already ruled out once for priority itself. It is wired in once a
real production pusher exists; triggered abilities are the more likely first one, since they need no cast cost or target
to fire.

## Mulligans

`PerformMulligans` (`mulligan.go`) is `MulliganService` plus `LondonMulligan` — one rule, not the five-class strategy
hierarchy Java has. London is the only one modern paper Magic has used since 2019 and the only one the constructed
corpus Crucible targets exercises; Original, Paris and Vancouver are the rules it replaced, and Houston is a
Forge-specific casual variant. Porting a hierarchy for four rules nothing in scope calls is exactly the speculative work
CLAUDE.md rules out (PORT-6) — a straight function reads better than an interface with one real implementation.

It calls the two `PlayerController` methods M4 built and never used: `MulliganKeepHand` and `TuckCardsViaMulligan`.
Building the controller interface ahead of its callers, the way `ChooseStartingPlayer` was already sitting there unused,
is exactly what paid off here.

Two real rules came with it:

- **CR 103.4** — a game with more than two players gives every player one free mulligan. Heads-up London gives none: the
  very first mulligan already costs a card.
- **The last-offered mulligan can cost more than a fresh hand holds.** `LondonMulligan.canMulligan`'s bound
  (`tuckCardsDuringMulligan() <= maxHandSize`) reads the mulligan count from _before_ the mulligan it is gating, one
  step behind what that mulligan will actually cost once taken — Java's own code, not a port artifact. The practical
  effect is the last offered mulligan can ask a seven-card hand to tuck eight. `mulligan()` clamps the tuck count to the
  hand's actual size before asking `TuckCardsViaMulligan` for it — a defensive floor, not a rules change: tucking
  everything and tucking "everything, and then some" both leave an empty hand.

`Player.shuffle` needed `OrderedSet` to support reordering at all, which it could not: `Add` only appends, and nothing
before this needed to put a zone's cards in anything but insertion order. `OrderedSet.Swap(i, j)` is the addition —
exchanges two positions and keeps the lookup index in step — and `Game.Shuffle` drives it with `javarand.Rand.Shuffle`,
the same `Collections.shuffle(list, MyRandom.getRandom())` call Java's `Player.shuffle` makes, so a shuffled library
replays identically from the same seed (pkg/javarand's own P0 gate covers the algorithm; this is the first caller that
exercises it against a real zone).

Opening hands are not dealt here. Java's `MulliganService` assumes `Game` already dealt one per player, and so does
`PerformMulligans` — dealing one needs a `Match`/`StartGame` flow this port has not built, so a caller populates each
hand (a fixture, today; a real game-start procedure, eventually) before calling this.

## Events, wired

ADR-0013's schema (`event.go`) landed with the turn structure it names but with nothing behind it: no `Game` field held
a `Sink`, and nothing called `Emit`. Every mechanism this port has built now does:

| Call site                               | Kind(s)                                           |
| --------------------------------------- | ------------------------------------------------- |
| `Move`                                  | `ZoneChanged`                                     |
| `StartTurn`, `AdvancePhase` (on wrap)   | `TurnBegan`                                       |
| `beginPhase` (every step)               | `PhaseBegan`                                      |
| `drawStep`                              | `CardDrawn`, alongside `Move`'s own `ZoneChanged` |
| `CheckStateBasedActions` (once, on end) | `GameEnded`                                       |
| `PushAbility`                           | `AbilityActivated`                                |
| `ResolveStack` (per item)               | `AbilityResolved`                                 |

`Game.sink` defaults to `DiscardSink{}`, set in `NewGame`, so no existing caller — every test, `fixture.Load` — had to
start constructing one. `SetSink` is the opt-in a recorder (M8) uses. `Game.Clone` always gives the clone a fresh
`DiscardSink` regardless of what the original holds, which is the reason `DiscardSink`'s own doc comment already gave
before anything called it: the AI's lookahead explores lines that never happened, and a clone holding the real sink
would record imagined casts as real.

`PhaseBegan` fires before that phase's own actions, not after — a recorder reading the stream in order sees "entered
Draw" before "drew a card," matching when a real player would notice the step change. `TurnBegan` fires before
`ActivePhase` changes to `Untap`, for the same reason.

Not wired, and each is a real gap rather than an oversight: `CounterChanged` (annihilating counters is the only thing
that would fire it, and `Event.Detail` is a numeric payload that has nowhere to put an open string like `CounterType`
without inventing an encoding first), `SpellCast`/`DamageDealt` (nothing that would fire them exists yet — casting and
damage dealing, unlike stack resolution itself, are still unbuilt), and anything from `PerformMulligans` itself — a
mulligan is fully visible as the `ZoneChanged` cascade `Move` already produces, and no `MulliganTaken`-shaped kind
exists in the schema to add without also bumping `SchemaVersion`, a more deliberate act than this pass earned.
`AbilityActivated`/`AbilityResolved` are wired now (`## Stack`), even though nothing yet calls `PushAbility` or
`ResolveStack` outside a test — the same "mechanism now, content later" the effect registry already established.

## The scenario harness lives partly here

`TestScenarios` and `compareGames` (`scenario_test.go`) are this package's half of TEST-5's Layer 2 harness; the other
half — `Parse`/`Load`/`Dump`/`RunActions` — is `internal/fixture`'s, documented in full there
(`porting/port-log/game-state-fixture.md`'s "Scenarios" section). Worth repeating here: `compareGames` reads two
`*engine.Game`s directly rather than through `Dump`, because `Dump`'s `Id:` is a `CardID` and the two games being
compared were never going to agree on those by number.

## Not ported yet

| Missing                                                                                                                                                                                                   | Lands |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `CardState` — face/characteristics data for transform, flip and meld                                                                                                                                      | M5    |
| 106 of `PlayerController`'s 110 methods — everything needing `SpellAbility`, `Combat`, targeting or cost payment                                                                                          | M5-M6 |
| `AIController`, the real (non-scripted) implementation                                                                                                                                                    | M7    |
| Every other CR 704.5 state-based action — lethal damage, a Battle at zero defense — needs the full layer system or a permanent type not modeled                                                           | M5-M6 |
| CR 121.5: a planeswalker entering the battlefield with its printed starting loyalty as counters — `Move` has no ETB hook for any permanent's starting counters yet                                        | M5-M6 |
| The rest of CR 704.5g's toughness — `*`, `1+*`, a `Count$` reference, or toughness a continuous effect or a counter has changed — needs `internal/expr` and the layer system, not just `strconv.Atoi`     | M5-M6 |
| The rest of CR 704.5f/704.5m's legality — an Aura's own `Enchant` restriction, protection, hexproof — needs a `valid`-string evaluator, not just "is the host still on the battlefield"                   | M5-M6 |
| CR 613.6-613.8's dependency reordering within a layer — `foldPT` only sorts by timestamp, correct until two effects on one card can actually disagree about order                                         | M5-M6 |
| Layers 1-6 and 8 (copy, control, text, type, color, ability, rules effects) — only 7a/7b/7c (power/toughness) have anything to apply yet                                                                  | M5-M6 |
| `changeZone`'s replacement effects, triggers, last-known-information and token/copy-vanishing rules                                                                                                       | M5-M6 |
| `PhaseHandler`'s Upkeep, Main, combat, End of Turn and Cleanup step bodies — need triggers, `SpellAbility` or Combat                                                                                      | M5-M6 |
| Interactive priority (`mainLoopStep`'s real APNAP pass), extra turns/phases, topsy-turvy phase order, "doesn't untap" effects — `ResolveStack` plays out only the degenerate case, nobody able to respond | M5-M6 |
| Original, Paris, Vancouver and Houston mulligan rules — out of scope, not deferred (PORT-6)                                                                                                               | never |
| Dealing opening hands — no `Match`/`StartGame` flow exists to call `PerformMulligans` from yet                                                                                                            | M5    |
| `CounterChanged`, `SpellCast`, `DamageDealt` — nothing yet causes them                                                                                                                                    | M5-M6 |
| `MagicStack`'s freeze/unfreeze, `addSimultaneousStackEntry`, `undoStack` — need a second ability arriving while one is still resolving, which nothing can cause yet                                       | M5-M6 |
| Trigger firing (CR 603) — needs a `valid`-grammar evaluator against `Game`/`Card` and a `TriggerType` port, neither built                                                                                 | M5-M6 |
| Replacement effects (CR 616, `ReplacementHandler.java`) — same evaluator dependency as triggers                                                                                                           | M5-M6 |
| Combat                                                                                                                                                                                                    | M5    |
