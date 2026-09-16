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

| Part                                 | Clone treatment | Reason                                                                                                                                   |
| ------------------------------------ | --------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `compile.DB`                         | shared          | Immutable, one per process; copying it would be the costliest thing                                                                      |
| `javarand.Rand`                      | copied by value | The clone continues the stream; sharing it would let the AI's exploration change what the real game rolls                                |
| Zones, counters, memory, attachments | deep            | Otherwise the lookahead mutates the real game                                                                                            |
| Stack (`[]Ability`)                  | deep            | `Ability` has no pointer fields, but a shared backing array would still let a push on one alias the other                                |
| `Card.PT`                            | deep            | Same reasoning as the stack: `PTEffect` has no pointer fields, but the slice still needs its own backing array                           |
| `Combat`                             | deep            | Same reasoning again: `Attackers []CardID`, `AttackTargets map[CardID]EntityID` and `Blocks []Block` each need their own backing storage |

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

`CheckStateBasedActions` is `GameAction.checkGameOverCondition`, `Player.checkLoseCondition`, `stateBasedAction704_5q`,
`handlePlaneswalkerRule`, `stateBasedAction_Battle` and `handleLegendRule`, plus `destroyLethalToughness`,
`destroyDamagedCreatures` and `cleanupDanglingAttachments` for a slice of what `changeZone` folds in elsewhere in Java
(`## Move carries what Java gets for free`, below) — the rules answerable without the full layer system: CR 704.5a (a
player at zero or less life loses), CR 704.5c (ten or more poison counters loses), CR 704.5q (a permanent carrying both
+1/+1 and -1/-1 counters loses the smaller pile from each, in equal number — five +1/+1 and two -1/-1 leaves three +1/+1
and none — `stateBasedAction704_5q`'s own name is the source for this letter), CR 704.5f (a creature at zero or less
toughness dies, Layer 7 and counters folded in — `GameAction.java`'s own comment on this check, not 704.5g), CR 704.5g
and 704.5h together (a creature dealt lethal damage, or any deathtouch damage at all, dies — indestructible creatures
excepted, `## Lethal and deathtouch damage`, below), a partial CR 704.5v (a Battle at zero or less defense dies, its own
trigger-on-the-stack exception checked and always false today — `## Loyalty is not a layer`'s Battle paragraph, below),
CR 704.5w/704.5x (a Battle's protector — `assignBattleProtector`, `## Combat`'s own paragraph on it, below), and three
rules Java's own comments do not number: a planeswalker at zero or less loyalty dies (`handlePlaneswalkerRule`), the
legend rule (`handleLegendRule` — `## The legend rule needed CheckStateBasedActions to take a controller`, below), and a
partial "cleanup aura" rule (Java's own comment for it, `GameAction.java:1511` — an Aura not attached to a permanent on
the battlefield goes to its owner's graveyard; an Equipment or Fortification in the same state just becomes unattached
alongside it). Citing these against Java's own comments rather than the rulebook from memory is deliberate:
`GameAction.java` labels the toughness check 704.5f, not 704.5g, and disagrees with itself about the attachment rule
(one comment calls it 704.5q, the same letter `stateBasedAction704_5q`'s own name already claims for counter
annihilation) — a wrong citation is worse than none, so the attachment, loyalty and legend rules are not asserted a
specific sub-letter here. Every other SBA in Java's loop — lethal damage to a planeswalker or a Battle via its
loyalty/defense rather than a creature's toughness, the rest of 704.5f/704.5g's own toughness (`*` with no
characteristic-defining effect to replace it, or a `Count$` reference — `internal/expr` has no evaluator yet), the rest
of the attachment rules' own legality (an Aura's `Enchant` restriction violated by something other than its host
leaving, protection, hexproof), and the legend rule's own two corner cases (`ignoreLegendRule`,
Partner-with-non-legendary-creature-names) — reads a characteristic the rest of the continuous-effect layer system
computes, or needs a restriction a `valid`-string evaluator would check (`internal/valid`'s own doc comment), and none
of that is M5 work this has fully reached yet. Damage dealt to a planeswalker or a Battle, which CR 120.3c/121.5 removes
as loyalty/defense counters rather than marking `Damage`, is wired too (`dealPermanentDamage`, `## Combat`, below) —
combat can attack one directly, so `destroyZeroLoyalty`/`destroyZeroDefense` are exercised by real play as well as by
tests that remove counters directly. Only _non-combat_ damage to a planeswalker or Battle is still a gap: nothing that
deals damage outside combat exists yet (no `SpellAbility`, no activated ability), so a burn spell or an ability aimed at
a planeswalker's loyalty has nowhere to come from regardless of whether the target-side plumbing is ready. A rule this
port has not implemented simply never fires, the same as a real game with no permanent that rule ever applies to — it is
a coverage gap (ADR-0011), not a wrong answer.

CR 704.5q's own guard — some cards grant "counters can't be removed from CARDNAME" — is a static ability, so it is not
checked either: nothing this port can grant that effect yet, so its absence changes no card's behaviour today.

Java's own loop runs up to nine times, because one SBA firing can make another one true. `destroyLethalToughness`,
`destroyDamagedCreatures`, `destroyZeroLoyalty`, `assignBattleProtector`, `destroyZeroDefense` and `resolveLegendRule`
all run before `cleanupDanglingAttachments`, not after, for exactly that reason: a creature, planeswalker, Battle or
legendary permanent this pass destroys can leave an Aura dangling that the very same `CheckStateBasedActions` call has
to catch, the one real cascade among the ten rules here. `assignBattleProtector` itself sits before `destroyZeroDefense`
for a narrower version of the same reason — matching `stateBasedAction_Battle`'s own combined-function order, a Battle
destroyed for having no eligible protector (`## Combat`'s own paragraph has the reachability of that case) never reaches
the defense check at all, rather than because assigning a protector changes any card's Defense. Nothing else cascades a
second time — destroying a permanent cannot itself change another one's printed toughness, damage total, loyalty/defense
count or name, and nothing yet grants an effect that could — so one ordered pass is complete. A game that already ended
skips every check below entirely, the same as Java: `checkStateEffects` returns before its creature loop runs once
`checkGameOverCondition` finds the game over. The loop returns once a rule that can cascade twice lands — a card script
writing to `Player.Life` or a permanent's counters mid-check does not exist yet either.

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
`baseOK` up front. `destroyLethalToughness` (CR 704.5f, `## State-based actions`) now reads `Toughness()` instead of
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
`destroyZeroLoyalty` (CR 704.5, `## State-based actions` — Java's own comments do not number this one) reads
`Card.Counters.Count(Loyalty)` directly, not a computed accessor that would just be that same call one level removed.

`Move` grants a planeswalker its printed starting loyalty as counters the moment it enters the battlefield (CR 121.5) —
the same "Java gets this for free by building a new Card object" gap `SummonSick` already closed for combat, closed here
for loyalty. `NewCard` still does not: fixture loading seats a battlefield permanent with exactly the counters the
fixture names, on purpose (`Move`'s own doc comment), so a scenario or a unit test isolating a state-based action still
sets `Loyalty` counters directly (`humancounters=LOYALTY=5`, or `Counters.Add` in a Go test) rather than relying on an
ETB grant that only fires for a card genuinely transitioning zones. `destroyZeroLoyalty` reads
`Card.Counters.Count(Loyalty)` either way, real and correct against whatever count is there, however it got there — the
same as `destroyLethalToughness` was real before `Power`/`Toughness` folded in Layer 7.

**A Battle's defense is the same shape, granted the same way.** `Card.BaseDefense`/`compile.Face.Defense`/
`carddb.Face.Defense` mirror `BaseLoyalty` exactly, `Move` grants it on entry the same way, and `destroyZeroDefense` (CR
704.5v, `GameAction.java`'s own comment) reads `Card.Counters.Count(Defense)` directly, the same story. One extra piece
of 704.5v is here too: Java's own version does not destroy a Battle at zero defense if it is the source of a trigger
that has fired but not yet left the stack, `hasSourceOnStack` in `GameAction.java`. That exception is checked, not
skipped — `destroyZeroDefense`'s own doc comment explains why it always reads false today (nothing puts a trigger on the
stack yet) rather than being silently dropped. CR 704.5w/704.5x, a Battle's protector assignment, is a separate
state-based action now too (`assignBattleProtector`, `## Combat`'s own paragraph on it, below) — never a prerequisite
for 704.5v's own defense check to be correct on its own terms, which is why the two landed in different sessions without
either one blocking on the other.

## Lethal and deathtouch damage, and the one keyword this port checks

`destroyDamagedCreatures` is CR 704.5g and 704.5h, Java's own comments on a single `else if` in `GameAction.java`'s
loop: a creature dealt damage at least equal to its current `Toughness()` dies, and a creature dealt any amount of
deathtouch damage dies regardless of the amount — `Card.Damage`'s own `Marked`/`Deathtouch` fields already existed for
exactly this (`## The card's mutable parts`) but had no reader until now.

Java's own check has an earlier branch first: indestructible creatures skip both halves. This is the one keyword this
port reads anywhere — not because keywords in general are in scope, but because getting this one wrong would not be a
coverage gap, it would be an actively wrong answer: a creature this port destroys that a real game would not.
`Card.HasKeyword` (card.go) is the accessor, `compile.Face.Keywords` (carried through from `carddb.Face.Keywords`
unchanged, the same "carry the printed text" split `Type`/`Power`/`Toughness`/`Loyalty` already use) is what it reads,
and `internal/keyword`'s own `Parse(line).Name` does the actual matching — the head as written, so `"Ward:2"` is still
found by `"Ward"`. Expanding a keyword into what it actually grants (the triggers, statics and abilities behind it) is a
different, much larger job `keyword.go`'s own doc comment already says this package does not do; `HasKeyword` only ever
answers "is the bare word present."

`cleanupStep` (`turn.go`) is a partial CR 514.2, landing alongside this because a damage-based SBA that never clears the
damage it is checking would be testing a state that cannot occur in a real game past one turn. Clearing damage on every
permanent in the game (not just the active player's, unlike `untapStep`) is the only piece built: discarding to hand
size (CR 514.1) needs a `PlayerController` decision this port cannot ask yet, and ending "until end of turn" effects
(514.2's other half) needs duration tracking this port does not have.

## The legend rule needed `CheckStateBasedActions` to take a controller

`resolveLegendRule` is `handleLegendRule`: a player controlling two or more legendary permanents sharing a name keeps
one and puts the rest into their owners' graveyards. It is the first state-based action this port has that asks a player
a question rather than just reading game state, and that changed a signature every other SBA in this file already used
without one — `CheckStateBasedActions(g *Game)` became `CheckStateBasedActions(g *Game, controller PlayerController)`,
which rippled to every path that reaches it: `Game.StartTurn`, `Game.AdvancePhase`, `Game.beginPhase` (turn.go) and
`Game.ResolveStack` (stack.go) all gained the same parameter, and every call site — production and test alike — had to
pass one through. `ChooseLegendaryToKeep` (control.go) is the fifth `PlayerController` method, alongside a
`ScriptedController.QueueLegendaryToKeep` and a `queue legendarykeep <id>` `actions.log` verb
(`internal/fixture/actions.go`) for scenarios that need to script it.

This is the real fix, not a workaround: CR 704.3 says state-based actions are checked automatically, so a state-based
action that needs a decision has to have somewhere to get one from, and bolting a separate
`ResolveLegendRule(g, controller)` on as something a caller has to remember to invoke would not actually be "automatic"
the way the rule requires. Threading the controller through instead keeps every state-based action, decision-needing or
not, reachable the same way.

Grouping is per player, not across the whole battlefield: two different players may each legally control their own copy
of one legendary permanent, so only a player's own duplicates trigger the rule. Within a player, names are grouped in
the order their permanents first appear on the battlefield (GO-12) — the same determinism `Multimaps.index`'s
insertion-ordered keys give Java. Two of Java's own corner cases are not here: a legendary permanent that opts out via
`ignoreLegendRule` (nothing this port can grant that effect yet), and Partner-with-a-non-legendary-creature-name pairs
(Spy Kit and similar) sharing a "true name" even though their printed names differ — a rule specific to a handful of
cards, not the general case.

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
its creature dies keeps pointing at a `CardID` no longer on the battlefield). That is CR 704.5's "cleanup aura" rule's
job, not `Move`'s — it is an SBA, checked continuously, not something a zone change fires inline — and
`## State-based actions` above is where it landed (`cleanupDanglingAttachments`). `Game.NewCard` stays untouched by any
of this: it is the arena-allocation primitive fixture loading uses to seat a board mid-game, where a battlefield card's
starting `Tapped`/`SummonSick` is exactly what the fixture says, not a rule this port applies at construction time.

## Turn structure

`turn.go` is `PhaseHandler.java` (1,324 LOC), reduced to what does not need the stack, triggers or `SpellAbility`:
`Turn`, `ActivePlayer` and `ActivePhase` live on `Game` now (`StartTurn`, `AdvancePhase`, `SetTurnState`), and three
steps — Untap, Draw and Cleanup — have real bodies (below). Every other step (`onPhaseBegin`'s Upkeep, Main, five combat
steps, End of Turn) still just changes `ActivePhase` and nothing else, because casting, blocking and firing a trigger
all need machinery this port has not reached. `AdvancePhase` walks through them as bookkeeping only, until each one's
turn comes.

**Cleanup discards to hand size (CR 514.1) before clearing damage (CR 514.2).** `cleanupStep(controller)` gained the
`controller` parameter and a new `DiscardToHandSize` decision (`PlayerController`, control.go) once the active player's
own hand could actually need asking about: `Game.Zone(Hand, g.activePlayer).Len() > MaxHandSize` (a new constant, 7 — CR
103.4's default, used unconditionally since nothing this port can grant a modified or unlimited hand size yet, the same
continuous-effect gap the rest of the layer system has) is what decides whether to ask at all, the same "nothing
meaningful to decide" reasoning every other decision point in this port uses for an empty or already-satisfied set.
Unlike the damage clear right after it, CR 514.1 is scoped to the active player only — Forge's own `CLEANUP` case reads
`playerTurn`'s hand, not every player's, which is why `cleanupStep` checks just the one zone before its existing
every-player damage loop runs. The discarded cards go to their owner's graveyard the same way `resolveLegendRule` and
every "destroy" state-based action already move things — `g.Move(id, Graveyard, g.Card(id).Owner)`, not a new pattern.

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

**`DealOpeningHands` (`mulligan.go`) is the flow that was missing: `GameAction.startGame`'s pre-mulligan half,** with
every `Match`-level part trimmed. It decides who plays first (CR 103.2's coin flip — `g.rand`'s own
`Int32n(len(players))`, `Aggregates.random`'s algorithm for a `List` source, the exact call Java's own
`Aggregates.random(game.getPlayers())` makes), shuffles every library, and deals each player `startingHandSize` cards.
It does not call `PerformMulligans` itself, and does not start the first turn either — both stay separate, explicit
calls a caller makes with the returned first player, the same way `DeclareCombatAttackers` and `DealCombatDamage` stayed
separate functions rather than one that "plays a combat": `PerformMulligans` already has its own test suite built
against hands dealt directly (no RNG in the loop to predict), and coupling the two would break that.

**This port has no `Match`, so `isFirstGame` is always `true`.** Java's `determineFirstTurnPlayer` only reaches the coin
flip when there is no previous game's loser to name (`lastGameOutcome == null`); every other branch — Puzzle, Archenemy,
Power Play — is a format variant nothing in the corpus this port targets uses. A future `Match` that plays more than one
game and needs "the loser of the last one goes first" is new work for `DealOpeningHands` to grow into, not a shortcut
taken here.

## Combat: declaring attackers, declaring blockers, and dealing damage

`combat.go`/`attack.go`/`block.go`/`combatdamage.go` are CR 506-510, cut down to CR 508.1's declare-attackers step (now
including 508.1d's attack-target choice), CR 509.1's declare-blockers step, and CR 510.1-510.4's combat damage — both
the first-strike sub-step and the regular one — plus CR 702.19's trample. This is the whole of Combat this port has
reached; not every keyword or restriction is (below). `Combat` (combat.go) is the game's own combat state, currently
`Attackers []CardID`, `AttackTargets map[CardID]EntityID` and `Blocks []Block`; it is a new `Game` field
(`combat Combat`), cloned and cleared the same way `Card.PT`/the stack already are, and split into its own file for the
same reason `ability.go`/`layer.go`/`pt.go` are: `Game` needs the `Combat` type for its own field, and
`Game.DeclareCombatAttackers` (attack.go)/`Game.DeclareCombatBlockers` (block.go)/`Game.DealCombatDamage`/
`Game.DealFirstStrikeDamage` (combatdamage.go) need `*Game` — one of them has to sit below the other in the dependency
graph, or `enginelint` catches the cycle the same way it already has three times this milestone. `control.go` needed to
move into the `combat` group's own allow-list too, once `PlayerController.DeclareCombatBlockers` had to name `Block` in
its signature; `combatdamage`'s own group needed `parts` added to its allow-list, the first combat file to touch
`Card.Damage`; `block`/`combatdamage` both needed `attack` added once they started calling `defenderOf` (below).

`Game.DeclareCombatAttackers` computes eligibility itself — untapped, and either no summoning sickness or haste (CR
302.6) — rather than trusting the caller, the same "the game decides what is legal, the controller only decides among
what is offered" split `PerformMulligans` already uses for `MulliganKeepHand`. A declared attacker taps unless it has
vigilance (CR 508.1f) — `Card.HasKeyword`'s second real caller, after `destroyDamagedCreatures`'s `Indestructible`
check.

**Named `DeclareCombatAttackers`/`DeclareCombatBlockers`, not `DeclareAttackers`/`DeclareBlockers`.** `PhaseType`
already has constants with both of those exact names (phase.go) — Go allows a method and a package-level constant to
share a name, since methods live under their receiver's own namespace, but `enginelint`'s plain-identifier matching does
not tell the two apart, and neither would a reader skimming for one and finding the other. `enginelint` caught this
twice, once per method: `attack.go`'s `DeclareAttackers` first, and `block.go`'s `DeclareBlockers` the same way when it
landed. The method, the `PlayerController` interface method and the `ScriptedController` implementation are all renamed
in both cases; the phase constants, the actual CR 508/509 steps these methods are one piece of, keep their own names
unchanged.

**Blocking does not tap the blocker.** CR 508.1f taps an attacker; CR 509 has no equivalent step for a blocker, so
`Game.DeclareCombatBlockers` never touches `Card.Tapped`. Gang blocking (CR 509.1c) is unrestricted on the attacker side
— `Block` is a flat `[]Block` of `{Blocker, Attacker}` pairs, and more than one pair naming the same `Attacker` is
ordinary, not a case the code has to special-case.

**Flying/reach, menace, protection and every other block restriction are not checked.** `Game.DeclareCombatBlockers`'s
own eligibility computation is only "untapped creature the defending player controls." This is a deliberate,
architecture-driven gap, not an oversight parallel to the Indestructible/Vigilance precedent: in Forge, CR 509.1b's
restrictions — Flying included — all run through the general `CantBlockBy` static-ability engine
(`StaticAbilityCantAttackBlock.java`, `ValidBlocker`-matched against arbitrary strings), the same generic mechanism
Menace's minimum-blocker-count and every "can't be blocked except by"/"must be blocked by" card use. Indestructible
(`GameAction.java`) and Vigilance (`Card.attackVigilance()`) are different in kind — Forge hardcodes those two directly
in engine code, which is exactly why `Card.HasKeyword` special-cases them here too. Hardcoding Flying the same way would
invent a mechanism specific to one keyword that Forge itself does not use for it, and would need re-deciding once the
real static-ability engine lands and the two disagree. The honest gap is "wait for that engine" (M5/M6), not a
Flying-shaped patch now.

**Not wired into `AdvancePhase`'s automatic walk through the phases.** `PerformMulligans` is the standing precedent for
a real M5 mechanic a scenario calls explicitly (`actions.log`'s own `declareattackers`/`declareblockers` verbs) rather
than one the turn structure invokes on every entry to that phase — the same "stub standing in for a decision no one can
make yet" reasoning `turn.go`'s own comment already gives for keeping `ResolveStack` out of `beginPhase`. Most games
reaching the DeclareAttackers phase attack with nothing at all; auto-wiring would mean every such phase entry pays the
cost of asking a question with an empty answer set almost every time.

**No eligible creature means the controller is never asked.** The same reasoning applies on both sides:
`Game.DeclareCombatAttackers` skips the question when the active player has nothing eligible, and
`Game.DeclareCombatBlockers` skips it both when there are no attackers at all and when the defending player has nothing
untapped to block with — there is nothing meaningful to decide, so nothing is queued for it. This is also what lets
every existing scenario and test that walks through combat without ever creating a creature keep working without queuing
an attackers or blocks answer it was never going to need.

**Attack targets: CR 508.1d generalized "who's defending" into "what's being attacked."** `assignAttackTargets`
(attack.go), called from inside `DeclareCombatAttackers` right after attackers are chosen and tapped, gives every
declared attacker an `EntityID` target — a player, or a planeswalker/battle that player controls
(`eligibleAttackTargets`). CR 508.1d makes "which creatures attack" and "what each attacks" one combined announcement,
not two sequential decisions, which is why this lives inside `DeclareCombatAttackers` rather than as its own
`actions.log` verb: `Game.DeclareCombatAttackers`'s own public signature and return value (`[]CardID`, which creatures
attacked) don't change at all, only a new side effect and, sometimes, a new controller call get added.

A lone eligible target — any two-player game with no planeswalker or battle on the other side, the case every existing
scenario before this one was — is assigned automatically, without ever calling `ChooseAttackTarget`. This is the load-
bearing backward-compatibility property: every fixture and test written before attack targets existed keeps passing
unmodified, because none of them gives an opponent a second thing to be attacked, so the ask branch never fires and
`ScriptedController.attackTargets` never has to hold anything. More than one eligible target — a planeswalker/battle
present, or (multiplayer) more than one living opponent — asks `ChooseAttackTarget` once per attacker, trusted the same
way `ChooseLegendaryToKeep`'s answer is.

**`defenderOf` (attack.go) is what `DeclareCombatBlockers` and `DealCombatDamage` now ask instead of
`nextPlayerAfter`.** It resolves an attacker's own `AttackTarget` to the player who can legally block it (CR 802.4a):
itself, if the target is a player; the target's controller, if it's a planeswalker or battle. Both callers ask it only
once, for `g.combat.Attackers[0]`, and assume every attacker in the current combat shares that one defender — true of
any two-player game (with or without a planeswalker/battle target) and of a multiplayer game where the active player
sends every attacker at one opponent, but not of a single combat split across several different defending players at
once. That narrower case — several defending players each declaring their own blocks against their own share of the
attackers, in some order — needs per-defender block declaration passes, a bigger redesign than assigning targets is;
this slice closes "which opponent" and "attack a planeswalker/battle" without needing that redesign, because both stay
within the one-shared-defender assumption.

**Attacking a planeswalker or battle changes how combat damage lands, not who deals it.** `dealAttackTargetDamage`
(combatdamage.go) is the dispatcher every "damage past the last blocker" call site (unblocked, trample overflow) now
goes through, in place of always calling `dealPlayerDamage`: a player target still reduces `Life`, but a
planeswalker/battle target goes to `dealPermanentDamage` instead. `dealPermanentDamage` (renamed from
`dealCreatureDamage`, which it still does everything of) removes loyalty or defense counters for a planeswalker or
battle target (CR 120.3c, 121.5) in addition to — not instead of — marking `Card.Damage` if the target is also a
creature, the same independent-checks shape Forge's own `Card.addDamageAfterPrevention` uses for a card that's more than
one type at once. No new state-based-action work was needed: `destroyZeroLoyalty`/`destroyZeroDefense`
(`## State-based actions`) already existed and already read `Counters.Count(Loyalty/Defense)`, so removing counters via
combat damage is all it took to make them fire for real instead of only in tests that added counters by hand.

**Combat damage is the first thing that actually deals damage.** Every earlier state-based action reading
`Card.Damage.Marked`/`Deathtouch` (`destroyDamagedCreatures`) only ever saw what a test had marked directly —
"`Damage.Mark`'s only callers are tests" (below, "Not ported yet") stops being true here. `dealCombatDamageStep`
(combatdamage.go), the shared body behind both `DealFirstStrikeDamage` and `DealCombatDamage`, computes every attacker's
exchange one at a time rather than computing all amounts first and applying them together: CR 510.2 makes a single
step's damage simultaneous, but nothing this port has built triggers off damage being dealt or reads a life total
mid-step, so the two orders are indistinguishable to anything that can currently observe them. An unblocked attacker
deals its power to whatever it's attacking (`dealAttackTargetDamage`, below). A single blocker exchanges full power for
full power automatically; a gang-blocked attacker (more than one live `Block` naming it) asks its controller to divide
its power via `AssignCombatDamage` (CR 510.1c) — trusted the same way `ChooseLegendaryToKeep`'s answer is, including the
"lethal before moving on" ordering constraint CR 510.1c itself imposes. `DamageDealt` and `LifeChanged` both wire here
for the first time (below, "Events, wired"), each attributed to `Source` (the dealing card) and flagged `FlagCombat`,
plus `FlagDeathtouch` when the source has that keyword.

**First strike (CR 510.4) is one function asked twice, not two functions.** `dealsInStep(c, firstStrike)` is the whole
of it: a creature with "First Strike" acts only when `firstStrike` is true, "Double Strike" acts either way, everything
else only when it's false. `DealFirstStrikeDamage` and `DealCombatDamage` are that same body called with `true` and
`false` — a fixture with no first striker at all can still call `DealFirstStrikeDamage` and get a real no-op back
(nobody's `dealsInStep` returns true), the same "the game decides what is legal, ask anyway" reasoning
`DeclareCombatAttackers` already applies to an empty eligible list. The two steps are separate `actions.log` verbs
(`firststrikedamage`, `combatdamage`, `game-state-fixture.md`), not one call that internally loops twice, because a real
state-based-action check has to happen between them — a first-strike kill has to be dead before the regular step asks
whether it still deals or receives anything — and that check already happens for free: `beginPhase` runs
`CheckStateBasedActions` on every phase entry (`## Turn structure`, `turn.go:107`), so a scenario that `advance`s from
`FirstStrikeDamage` into `CombatDamage` between the two verbs gets the kill applied without a new verb invented just for
it.

**A creature that left the battlefield between the two steps deals nothing and receives nothing.** This was a
documented, genuinely unreachable gap before first strike existed — nothing could kill a creature between
`DeclareCombatBlockers` and combat damage. First strike makes it reachable: `alive` (`Card.Zone == Battlefield`) gates
every attacker at the top of `dealCombatDamageStep`, and a dead attacker's blockers are filtered out before either side
of its exchange runs, so a creature killed by a first-strike blow neither swings again in the regular step nor gets hit
by something that's no longer there to hit it. `wasUnblocked` (`dealAttackerDamage`'s own parameter) tracks whether an
attacker was ever blocked at all, separately from whether it currently has zero live blockers — CR 510.1c treats "never
blocked" (hits the player) and "blocked, but every blocker has since died" (hits nobody, no trample) as different
outcomes that happen to look the same by the time only `len(liveBlockers) == 0` is left to check.

**Trample (CR 702.19) changes only how an attacker's own power splits, not who decides.** Against a single live blocker,
`lethalDamage` computes the minimum this port can assign it — the game deciding, since no decision was being asked in
that case anyway — and the rest goes to the player; against a gang-blocked attacker, whatever the controller's
`AssignCombatDamage` answer leaves unassigned across all its named blockers goes to the player instead of being wasted
(a non-trampler's own unassigned remainder is still wasted, unchanged from before trample existed). An unresolvable
toughness (`Toughness`'s own `*`/`Count$` gap) makes `lethalDamage` unable to compute lethal at all; its caller treats
that as "not trampling this blocker" — full power assigned to it, nothing guessed at — the same conservative default
`Toughness`'s own `ok`-false already gets everywhere else in this port, not a new one invented for trample. CR 702.19e's
"every blocker gone by the time damage is assigned" case — reachable the same way the paragraph above is, a trampler's
blocker dying to first strike — sends the attacker's full power to the player.

**A Battle's protector (CR 704.5w/704.5x) is a state-based action, not part of declaring attackers, even though it gates
whether one can be attacked without asking.** `assignBattleProtector` (action.go) runs in `CheckStateBasedActions`, not
here, because CR 704.5w fires independent of combat entirely — a freshly-played Battle needs a protector chosen before
anyone ever attacks it. `attackersOf` (attack.go) is what lets it ask CR 704.5w's own question ("is anyone currently
attacking this Battle") without duplicating `Combat.AttackTargets`' own bookkeeping: a reverse lookup by `EntityID`
rather than a new field, general enough that a player target works with it too even though only a Battle needs to ask
today. Only the Siege shape is implemented (every Battle in the compiled corpus prints that subtype; action.go's own doc
comment has the reasoning), and CR 704.5w's "no eligible opponent, destroy the Battle instead" fallback is real code
with no test behind it — provably unreachable as long as `CheckStateBasedActions`'s own win-condition check keeps
running first (its own doc comment works through why), kept anyway because Forge keeps it too, for the same "not
reachable given today's engine, but a fully specified rule" reason (Forge's own comment: "unless range of influence gets
implemented").

Not here yet: a single combat split across more than one defending player at once (`defenderOf`'s own doc comment has
the reason). Block legality beyond "untapped creature the defending player controls" — Flying/reach, menace, protection,
"must be blocked by" — waits on the general static-ability engine, above.

## Mana pool and payment

`mana.go`'s `Pool` (CR 106.4, one per `Player`) and its `Pay` method (CR 601.2h/601.2i) are M5 item 28's mana-payment
slice — the plan's own "budget the most time here" warning is about the full version, and this is deliberately not that:
`Pay` handles a cost's `Generic` amount plus its six "pure" shards (`ShardW`/`U`/`B`/`R`/`G`/`C`) and nothing else, the
same "plain-integer operand" discipline `valid.go`'s `compareMatches` already applies to numeric comparisons, for the
same reason — the harder cases are each a real decision (which color a hybrid symbol takes, mana or life for a Phyrexian
one) this port has no `PlayerController` method to ask yet.

**Nothing casts a spell yet, and `Pay` does not need one to be worth building.** `turn.go`'s own doc comment already
says why the priority loop isn't wired in: no `PlayerController` method can cast or activate anything, so `Pay` has no
real caller today beyond its own tests — the same position `DeclareCombatAttackers`/`AssignCombatDamage` were in before
anything glued a full combat together, and CR 106/601.2h is exactly as self-contained a rules chapter as CR 508-510 was.
What is not deferrable is CR 500.4: mana already empties between every phase and step regardless of whether anything is
being cast, so `emptyManaPools` is a real, unconditional consumer of `Pool` from the moment `beginPhase` exists — not a
hypothetical one waiting on a future effect, the gap every other "build state ahead of its writer" call this port has
made (`Memory`, before anything in `effect.go`'s empty `Registry` could write to it) had to weigh instead.

**`emptyManaPools` runs at the top of `beginPhase`, not a separate `onPhaseEnd`.** Java's `PhaseHandler.onPhaseEnd`
clears every player's pool once per transition, right before the next phase's `onPhaseBegin` runs; this port's phase
walk has no separate "ending" hook (`beginPhase`'s own comment: `AdvancePhase` "walks through them as bookkeeping
only... until each one's turn comes"), so the one hook that already fires on every transition is where CR 500.4 lands —
same cadence, same effect, just attached to whichever half of the transition this port actually implemented. Mana burn
(losing life for mana left unspent) is not reproduced: it left the rules in 2010, before anything this port's corpus
targets, so there is no parity to keep with a rule no card in scope was ever printed under.

**Generic is paid from whatever is left over, in a fixed order, not a real choice.** CR 601.2h gives the paying player
free choice of which floating mana covers a generic cost; `Pay` spends colorless first, then white/blue/black
/red/green, a deterministic tie-break rather than a decision — nothing reads what is left in the pool after a payment
yet, so the order cannot be observably wrong today, only arbitrary. It becomes a real `PlayerController` question once
something does.

## Events, wired

ADR-0013's schema (`event.go`) landed with the turn structure it names but with nothing behind it: no `Game` field held
a `Sink`, and nothing called `Emit`. Every mechanism this port has built now does:

| Call site                                                                       | Kind(s)                                             |
| ------------------------------------------------------------------------------- | --------------------------------------------------- |
| `Move`                                                                          | `ZoneChanged`                                       |
| `StartTurn`, `AdvancePhase` (on wrap)                                           | `TurnBegan`                                         |
| `beginPhase` (every step)                                                       | `PhaseBegan`                                        |
| `drawStep`                                                                      | `CardDrawn`, alongside `Move`'s own `ZoneChanged`   |
| `CheckStateBasedActions` (once, on end)                                         | `GameEnded`                                         |
| `PushAbility`                                                                   | `AbilityActivated`                                  |
| `ResolveStack` (per item)                                                       | `AbilityResolved`                                   |
| `dealCombatDamageStep` (per exchange, both damage steps)                        | `DamageDealt`, plus `LifeChanged` for player damage |
| `annihilateCounters`, `dealPermanentDamage`, `Move`'s ETB loyalty/defense grant | `CounterChanged`                                    |

`Game.sink` defaults to `DiscardSink{}`, set in `NewGame`, so no existing caller — every test, `fixture.Load` — had to
start constructing one. `SetSink` is the opt-in a recorder (M8) uses. `Game.Clone` always gives the clone a fresh
`DiscardSink` regardless of what the original holds, which is the reason `DiscardSink`'s own doc comment already gave
before anything called it: the AI's lookahead explores lines that never happened, and a clone holding the real sink
would record imagined casts as real.

`PhaseBegan` fires before that phase's own actions, not after — a recorder reading the stream in order sees "entered
Draw" before "drew a card," matching when a real player would notice the step change. `TurnBegan` fires before
`ActivePhase` changes to `Untap`, for the same reason.

`CounterChanged` fires from every counter change this port can cause today — `annihilateCounters`'s CR 704.5q pile
shrink, `dealPermanentDamage`'s loyalty/defense removal, `Move`'s ETB loyalty/defense grant — with `Amount` as the
signed delta (negative for a loss, matching `LifeChanged`'s own convention) and `Detail` as a `CounterDetail`
(`event.go`) encoding which `CounterType` changed. `CounterDetail` is closed over the eight named `CounterType`
constants (`counters.go`) — `P1P1`, `M1M1`, `Loyalty`, `Defense`, `Charge`, `Stun`, `Shield`, `Poison` — not the open
string `CounterType` itself allows, because nothing yet creates a counter from a script-written name; that needs a
`SpellAbility` to run one, M6's problem. `counterDetail` (unexported, `event.go`) returns `ok == false` for anything
outside that set, and the caller drops the event rather than emit one with a lying `Detail` — unreachable today, since
every existing caller passes a named constant, but the seam exists for when M6's script-driven counters make it
reachable. Extending the switch, or replacing it with a per-`Game` interning table (ADR-0009's arena pattern, the same
shape as `CardID`) if the corpus turns out to need more than a closed set, is whichever shape M6 needs when a real
caller forces the choice — not a decision worth making before one exists.

Not wired, and each is a real gap rather than an oversight: `SpellCast` (nothing that would fire it exists yet —
casting, unlike combat damage, counters changing, or stack resolution, is still unbuilt), and anything from
`PerformMulligans` itself — a mulligan is fully visible as the `ZoneChanged` cascade `Move` already produces, and no
`MulliganTaken`-shaped kind exists in the schema to add without also bumping `SchemaVersion`, a more deliberate act than
this pass earned. `AbilityActivated`/`AbilityResolved` are wired now (`## Stack`), even though nothing yet calls
`PushAbility` or `ResolveStack` outside a test — the same "mechanism now, content later" the effect registry already
established. `DamageDealt`/`LifeChanged` are wired for real content now too (`## Combat`, above) — every combat exchange
fires `DamageDealt`, and player damage also fires `LifeChanged` with `Amount` as the signed change (negative for the
ordinary case, a loss), a sign convention this port chose freely since nothing wired either kind before combat damage
did.

## The scenario harness lives partly here

`TestScenarios` and `compareGames` (`scenario_test.go`) are this package's half of TEST-5's Layer 2 harness; the other
half — `Parse`/`Load`/`Dump`/`RunActions` — is `internal/fixture`'s, documented in full there
(`porting/port-log/game-state-fixture.md`'s "Scenarios" section). Worth repeating here: `compareGames` reads two
`*engine.Game`s directly rather than through `Dump`, because `Dump`'s `Id:` is a `CardID` and the two games being
compared were never going to agree on those by number.

## Not ported yet

| Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Lands |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `CardState` — face/characteristics data for transform, flip and meld                                                                                                                                                                                                                                                                                                                                                                                                                                                         | M5    |
| 99 of `PlayerController`'s 110 methods — everything needing `SpellAbility`, targeting or cost payment, and the rest of Combat past dealing damage                                                                                                                                                                                                                                                                                                                                                                            | M5-M6 |
| `AIController`, the real (non-scripted) implementation                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | M7    |
| Non-combat damage to a planeswalker or a Battle (a burn spell, an activated ability) — combat damage already removes loyalty/defense counters (CR 120.3c, 121.5); nothing outside combat deals damage at all yet                                                                                                                                                                                                                                                                                                             | M5-M6 |
| The rest of CR 704.5f/704.5g's toughness — `*`, `1+*`, a `Count$` reference, or toughness a continuous effect or a counter has changed — needs `internal/expr` and the layer system, not just `strconv.Atoi`                                                                                                                                                                                                                                                                                                                 | M5-M6 |
| The rest of the "cleanup aura" rule's legality — an Aura's own `Enchant` restriction, protection, hexproof — needs a `valid`-string evaluator, not just "is the host still on the battlefield"                                                                                                                                                                                                                                                                                                                               | M5-M6 |
| The legend rule's own two corner cases — `ignoreLegendRule` (nothing grants that effect yet) and Partner-with-non-legendary-creature-name pairs sharing a "true name"                                                                                                                                                                                                                                                                                                                                                        | M5-M6 |
| CR 613.6-613.8's dependency reordering within a layer — `foldPT` only sorts by timestamp, correct until two effects on one card can actually disagree about order                                                                                                                                                                                                                                                                                                                                                            | M5-M6 |
| Layers 1-6 and 8 (copy, control, text, type, color, ability, rules effects) — only 7a/7b/7c (power/toughness) have anything to apply yet                                                                                                                                                                                                                                                                                                                                                                                     | M5-M6 |
| `changeZone`'s replacement effects, triggers, last-known-information and token/copy-vanishing rules                                                                                                                                                                                                                                                                                                                                                                                                                          | M5-M6 |
| `PhaseHandler`'s Upkeep, Main and End of Turn step bodies, and `CombatEnd` — need triggers, `SpellAbility` or the rest of Combat                                                                                                                                                                                                                                                                                                                                                                                             | M5-M6 |
| `Match` — a series spanning more than one game, "the loser of the last game goes first" (`DealOpeningHands` always takes CR 103.2's coin flip), Puzzle/Archenemy/Power Play's own starting-player rules                                                                                                                                                                                                                                                                                                                      | M5-M6 |
| The rest of CR 514.2: "until end of turn"/"this turn" effects ending — needs duration tracking this port does not have, `PT`'s own effects included                                                                                                                                                                                                                                                                                                                                                                          | M5-M6 |
| A modified or unlimited maximum hand size (CR 514.1's `isUnlimitedHandSize`/a continuous effect changing it) — `MaxHandSize` is used unconditionally since layers 1-6/8 aren't built                                                                                                                                                                                                                                                                                                                                         | M5-M6 |
| Interactive priority (`mainLoopStep`'s real APNAP pass), extra turns/phases, topsy-turvy phase order, "doesn't untap" effects — `ResolveStack` plays out only the degenerate case, nobody able to respond                                                                                                                                                                                                                                                                                                                    | M5-M6 |
| Original, Paris, Vancouver and Houston mulligan rules — out of scope, not deferred (PORT-6)                                                                                                                                                                                                                                                                                                                                                                                                                                  | never |
| `SpellCast` — nothing yet causes it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | M5-M6 |
| `CounterChanged` for a script-written (non-named-constant) `CounterType` — `counterDetail` (`event.go`) is closed over the eight named constants; a `SpellAbility` creating an arbitrary keyword counter needs the encoding extended or replaced first (`## Events, wired`)                                                                                                                                                                                                                                                  | M5-M6 |
| Basic land mana abilities — `Plains.txt` carries no ability line at all (`Name:Plains / ManaCost:no cost / Types:Basic Land Plains / Oracle:({T}: Add {W}.)`); the Java mechanism that synthesizes a basic land's tap-for-mana ability from its subtype was not found after checking `Card.java`, `CardFactory.java`, `MagicColor.java`, `CardRules.java`, `AbilityManaPart.java` and `Aggregates.java`. Blocks `Pool.Add`'s first real (non-test) caller (PORT-8: find the mechanism or leave the gap open, don't guess it) | M5-M6 |
| `MagicStack`'s freeze/unfreeze, `addSimultaneousStackEntry`, `undoStack` — need a second ability arriving while one is still resolving, which nothing can cause yet                                                                                                                                                                                                                                                                                                                                                          | M5-M6 |
| Trigger firing (CR 603) — needs a `valid`-grammar evaluator against `Game`/`Card` and a `TriggerType` port, neither built                                                                                                                                                                                                                                                                                                                                                                                                    | M5-M6 |
| Replacement effects (CR 616, `ReplacementHandler.java`) — same evaluator dependency as triggers                                                                                                                                                                                                                                                                                                                                                                                                                              | M5-M6 |
| A single combat split across more than one defending player at once (multiplayer, attackers sent at different opponents) — `defenderOf` assumes one shared defender; needs per-defender block declaration passes                                                                                                                                                                                                                                                                                                             | M5-M6 |
| Block legality beyond "untapped creature the defending player controls" — flying/reach, menace, protection, "must be blocked by" — needs the general `CantBlockBy` static-ability engine                                                                                                                                                                                                                                                                                                                                     | M5-M6 |
| `Pool.Pay` for hybrid, Phyrexian, `{X}` and snow shards — each is a real decision (which color, mana or life) with no `PlayerController` method to ask it; a real choice of which floating mana pays a generic cost                                                                                                                                                                                                                                                                                                          | M5-M6 |
| Mana abilities themselves — nothing taps a land or activates anything to put mana in a `Pool` yet; `Pool.Add`/`AddColorless` exist for `Pay`'s own tests today                                                                                                                                                                                                                                                                                                                                                               | M5-M6 |
