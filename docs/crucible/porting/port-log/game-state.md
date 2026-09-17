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
actually designs them, so only the twenty methods answerable with today's engine are here — mulligans and the
starting-player choice, the legend rule's own `ChooseLegendaryToKeep`, every combat decision point, mana payment's own
eight harder shapes, and `ChooseEnchantTarget` (`CastSpell`'s own Aura branch, CR 601.2c) — each added when its own
caller was (control.go's own header comment names all twenty against their callers). Mulligans and the starting-player
choice have real callers in `GameAction` and `mulligan/`, even though `mulligan/` is not ported yet.

Forge instantiates one controller per player. Go's methods take the deciding player as an explicit `PlayerID` instead of
binding an instance to one seat, so `ScriptedController` — the fixture-driven implementation TEST-5 runs scenarios
against — answers for every player in a game from one value, with no per-player state to wire up (GO-2, PORT-1).

A `ScriptedController` queue running dry mid-scenario panics rather than returning a zero value: it is a
fixture-authoring mistake, not a rules question a card script could cause, so it has to fail loud (GO-7).

## State-based actions

`CheckStateBasedActions` is `GameAction.checkGameOverCondition`, `Player.checkLoseCondition`, `stateBasedAction704_5q`,
`handlePlaneswalkerRule`, `stateBasedAction_Battle`, `handleLegendRule` and `handleWorldRule`, plus
`destroyLethalToughness`, `destroyDamagedCreatures` and `cleanupDanglingAttachments` for a slice of what `changeZone`
folds in elsewhere in Java (`## Move carries what Java gets for free`, below) — the rules answerable without the full
layer system: CR 704.5a (a player at zero or less life loses), CR 704.5c (ten or more poison counters loses), CR 704.5q
(a permanent carrying both +1/+1 and -1/-1 counters loses the smaller pile from each, in equal number — five +1/+1 and
two -1/-1 leaves three +1/+1 and none — `stateBasedAction704_5q`'s own name is the source for this letter), CR 704.5f (a
creature at zero or less toughness dies, Layer 7 and counters folded in — `GameAction.java`'s own comment on this check,
not 704.5g), CR 704.5g and 704.5h together (a creature dealt lethal damage, or any deathtouch damage at all, dies —
indestructible creatures excepted, `## Lethal and deathtouch damage`, below), a partial CR 704.5v (a Battle at zero or
less defense dies, its own trigger-on-the-stack exception checked and always false today — `## Loyalty is not a layer`'s
Battle paragraph, below), CR 704.5w/704.5x (a Battle's protector — `assignBattleProtector`, `## Combat`'s own paragraph
on it, below), CR 704.5m (more than one permanent with the World supertype on the battlefield at once, across every
player, destroys every one but the newest by `Card.Timestamp` — `resolveWorldRule`, the same field
`## Handles, not pointers`'s own table above already stamps on every zone change), and three rules Java's own comments
do not number: a planeswalker at zero or less loyalty dies (`handlePlaneswalkerRule`), the legend rule
(`handleLegendRule` — `## The legend rule needed CheckStateBasedActions to take a controller`, below), and a "cleanup
aura" rule (Java's own comment for it, `GameAction.java:1511` — an Aura not attached to a permanent on the battlefield,
or attached to one that no longer matches the Aura's own `Enchant` restriction (CR 303.4a, `enchantSpec`, below), goes
to its owner's graveyard; an Equipment or Fortification in the same state just becomes unattached alongside it). Citing
these against Java's own comments rather than the rulebook from memory is deliberate: `GameAction.java` labels the
toughness check 704.5f, not 704.5g, and disagrees with itself about the attachment rule (one comment calls it 704.5q,
the same letter `stateBasedAction704_5q`'s own name already claims for counter annihilation) — a wrong citation is worse
than none, so the attachment, loyalty and legend rules are not asserted a specific sub-letter here. Every other SBA in
Java's loop — lethal damage to a planeswalker or a Battle via its loyalty/defense rather than a creature's toughness,
the rest of 704.5f/704.5g's own toughness (`*` with no characteristic-defining effect to replace it, or a `Count$`
reference — `internal/expr` has no evaluator yet), protection and hexproof preventing an attachment in the first place
(CR 702.11h/702.16e, a quality-matching static-ability question, not the `Enchant` restriction itself), and the legend
rule's own two corner cases (`ignoreLegendRule`, Partner-with-non-legendary-creature-names) — reads a characteristic the
rest of the continuous-effect layer system computes, or needs a static-ability engine this port does not have, and none
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

**`cleanupDanglingAttachments` now checks an Aura's own `Enchant` restriction, not just its host's presence.**
`enchantSpec` (`action.go`) reads the Aura's `K:Enchant:...` keyword (`internal/keyword`'s `Type`-kind parsing) and
turns the valid-string half of it into an `internal/valid.Spec` — `KeywordWithType.java`'s own
`"<validString>:<display text>"` split, reproduced with `strings.Cut` on the first remaining `:` in
`keyword.Keyword.Details` since Go's own `keyword.Parse` only cuts the head off once. `Matches` (`valid.go`) then checks
the host against that spec from the Aura's own controller and the Aura itself — exactly the call `Matches`'s own doc
comment already named as this rule's eventual caller, with no `Ability` in sight, before this landed. `"Player"` and
`"Opponent"` (`K:Enchant:Player`, `K:Enchant:Opponent` — Tenuous Truce, Archenemy, Overencumbered, Psychic Possession)
are Java's own literal forms for an Aura that enchants a player rather than a permanent; `enchantSpec` reports no
checkable spec for either rather than reading the bare word as a card-type restriction no permanent's type line could
ever contain, which would silently destroy every such Aura on the very next `CheckStateBasedActions` call. This port's
`AttachedTo` (`CardID`-only) has no representation for "attached to a player" at all, so a player-target Aura is a gap
this specific check does not close — a distinct, larger one from the type-restriction check it does close.

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

## Layer 7, Layer 4 and Layer 5: `PT`/`TypeMod`/`ColorMod`, `Card.Power`/`Toughness`/`Type`/`Colors`, and the first real Continuous callers

`layer.go` is `forge.game.staticability.StaticAbilityLayer`: the ten-value enum, Forge's own order, 7a/7b/7c split and
Layer 8 (Forge's own rule-changing bookkeeping, no CR number) included, even though only 7b/7c have a real caller today.
`pt.go`'s `PT` is a card's set of `PTEffect`s — one continuous effect's power/toughness contribution, a layer, a
timestamp, and (`HasPower`/`HasToughness`, below) which dimension it actually sets.

`Card.Power`/`Toughness` (`card.go`) is what actually applies CR 613.4's ordering: sort `PT`'s effects by layer then
timestamp, then fold — `LayerCharacteristic` and `LayerSetPT` each replace the running value, `LayerModifyPT` adds to
it, and +1/+1/-1/-1 counters (`Card.Counters`, already built) apply last, after every layer. A `LayerCharacteristic`
effect can turn an unresolvable base (`*`, `BasePower`'s own `ok=false`) into a resolvable one — a
characteristic-defining ability's entire purpose — so `foldPT` starts from `(base, baseOK)` rather than requiring
`baseOK` up front. `destroyLethalToughness` (CR 704.5f, `## State-based actions`) reads `Toughness()` instead of
`BaseToughness()`, so a creature a `LayerModifyPT` pump or an annihilated -1/-1 pile actually reduces to zero dies here
too, not only one whose printed toughness always read zero.

`PTEffect` gained `HasPower`/`HasToughness` the moment a real caller needed them: a `LayerSetPT`/`LayerCharacteristic`
effect naming only one dimension (68 real corpus `SetPower$`-only lines, 9 `SetToughness$`-only) must leave the other
exactly as it was, not reset it to zero the way the original zero-value `Power`/`Toughness` `int` fields alone would
have — `foldPT`'s own `pick` function now returns `(value, hasThisDimension)`, overwriting only when `has` is true.
`LayerModifyPT` needs neither flag: adding zero to a dimension an effect does not mention is already a no-op. Every
existing `pt_test.go` literal that builds a `LayerSetPT`/`LayerCharacteristic` `PTEffect` needed both flags added to
keep testing what it already did; `TestPowerToughnessSetPTPartialLeavesOtherDimensionAlone` is the new case that
motivated the change.

`applyContinuousPT` (`continuous.go`) is the first real (non-test) `PT.Add` caller: `Mode$ Continuous` lines carrying
`AddPower$`/`AddToughness$`/`SetPower$`/`SetToughness$`, matched against every battlefield permanent via a blanket
`Affected$` valid-string — the anthem shape (Glorious Anthem, and 2,192 of 2,426 real corpus `S:Mode$ Continuous` lines
carrying one of those four keys). Ported from `StaticAbilityContinuous.applyContinuousAbility`/`getAffectedCards`.

Recomputed from scratch on every `CheckStateBasedActions` call, not pushed once when a source or an affected creature
enters: Java's own `applyContinuousAbility` runs fresh from `GameAction.checkStateEffects` every state-based-action pass
for exactly this reason — an anthem has to reach a creature that enters after it, and stop the instant the anthem itself
leaves, neither of which a one-time push at either card's own entry could give it
(`TestApplyContinuousPTRecomputesWhenSourceLeaves`, continuous_test.go, proves the second half). Every battlefield
card's own `PT.effects` is cleared before the rebuild; safe today because nothing else this port can build yet ever adds
a `PTEffect` of its own (a real "+3/+3 until end of turn" pump would need its own duration-scoped bucket this clear
would not touch, `## Not ported yet`, below) — recomputing everything from `Mode$ Continuous` statics alone is exactly
correct until one exists, not an approximation that happens to work today.

Not resolved, each for a specific reason: `Condition$` (116 of 2,426 real lines) — a generic runtime gate
(`StaticAbility.java`'s own `checkConditions`, no equivalent for any static-ability mode in this port yet);
`AffectedDefined$`/`AffectedZone$` (0 and 24) — a targeted or `Remembered`-driven affected set, not a blanket
valid-string match; `CharacteristicDefining$ True` (265, Layer 7a) — almost always an SVar-driven value (a "\*/\*"
creature's own `Count$`-based power), the identical evaluator gap `compareMatches` (`valid.go`) already documents, so
skipped as a whole rather than chasing the rare plain-integer case; and a non-numeric `AddPower$`/`AddToughness$`/
`SetPower$`/`SetToughness$` (`X`, `Y`, `Z`, `AffectedX`, a named SVar) — the same `AbilityUtils.calculateAmount` gap
`Draw`'s own `NumCards$` already has (`## M6's first effect: Draw`, above), skipped per missing dimension rather than
per whole line.

One real fixture needed fixing because of this, not writing: `equipment-falls-off-without-destroying`'s own Grizzly
Bears carried 2 marked damage on a printed 2/2, lethal only because Sword of Body and Mind's real
`Mode$ Continuous | AddPower$ 2 | AddToughness$ 2` was not applying yet. Equipped, it is actually a 4/4 — 2 damage was
never lethal to it in a rules-correct game, only in a port that had not built this yet. The fixture's own expected
outcome was accidentally right for the wrong reason; `setup.state` now marks 4 damage, actually lethal on a 4/4, so the
scenario still demonstrates what it was written for (`cleanupDanglingAttachments`'s Equipment-vs-Aura distinction)
without depending on a gap this port no longer has.

**Not here: CR 613.6-613.8's dependency reordering.** Java sorts effects within a layer by timestamp and then
re-evaluates whether an unapplied effect has become dependent on or independent of another as each one resolves
(`GameAction.checkStaticAbilities`'s `findStaticAbilityToApply`, 1,099-line `StaticAbilityContinuous.java`). Real
`PTEffect`s exist now, but nothing in the real corpus subset this slice resolves puts two effects on one card that could
actually disagree about order (an anthem and an equipment bonus stack additively regardless of which applied first) —
`foldPT`'s plain timestamp sort remains a real port of CR 613.7's tiebreak, not yet a stand-in for 613.8's harder case.

`PT.Clear()` runs from `Move` the moment a card leaves the battlefield, the same list `Counters`, `Damage` and `Tapped`
already clear there: a continuous effect that only applied on the battlefield does not survive the trip, and this port
has no duration tracking ("until end of turn" wearing off on its own) to model the alternative anyway. That clear is now
redundant with `applyContinuousPT`'s own rebuild-from-scratch for anything `Mode$ Continuous` produces (a card that has
left is not on the battlefield to be walked as a host or an affected card on the next pass either way), but still
matters for the same real gap `applyContinuousPT` does not close: an event-driven "+3/+3 until end of turn" pump, once
built, would need `Move`'s own clear exactly the way it always has.

`Player.Counters` is new here, the same type `Card.Counters` already uses: poison is the only player-level counter any
rule reads today, but nothing about "a count that is never stored at zero" is specific to what holds it. `Game.Clone`
deep-copies it for the same reason it already deep-copies a card's — sharing the underlying map would let the AI's
lookahead poison the real game.

**Layer 4, `applyContinuousType` (`continuous.go`), and `cardtype.Line`'s new `ParseToken`/`Union`/`Without`.**
`AddType$`/`RemoveType$` are the next slice of `Mode$ Continuous` after Layer 7b/7c, and needed a real gap in
`cardtype.Line` closed first: nothing exported it a way to combine or subtract two type lines, because every existing
caller only ever needed to _read_ a printed line, never build a new one at runtime. `ParseToken` is `Parse`'s own
per-word classification (core type, then supertype, then subtype fallthrough) pulled out for a single already-split word
— no `*cardtype.Registry` needed, unlike `Parse` itself, because multiword lookahead is the only thing `Parse` uses a
`Registry` for and `AddType$`/`RemoveType$` values are already split on `" & "` into individual type names by the
compiled script. That absence is deliberate, not an oversight worked around: this port still injects no
`*cardtype.Registry`/`*carddb.DB` into the engine (`CLAUDE.md`'s own GO-2), so a Layer 4 effect built to need one would
have nothing to call. `Union` and `Without` are CR 613.4's own add/remove directions on `Line` itself.

`TypeMod` (`typemod.go`) is `PT`'s own structure, copied for Layer 4: a `[]TypeEffect` (`Timestamp`, `AddTypes`,
`RemoveTypes`), a `foldType` that sorts by `Timestamp` and folds each effect's `Union` then `Without` into the running
line — CR 613.7's tiebreak, the identical simplification `foldPT`'s own doc comment already makes for 613.8's harder
dependency-reordering case. `Card.Type()` now folds `TypeMod` over the printed `Def.Faces[0].Type` the same way
`Card.Power`/`Toughness` already fold `PT` over `BasePower`/`BaseToughness`. `Move` calls `TypeMod.Clear()` on leaving
the battlefield, next to `PT.Clear()`; `Game.Clone` deep-copies it, next to `PT`'s own clone.

`applyOneContinuousType` skips a whole line, not just the part it cannot resolve, the moment it carries anything past a
plain literal `AddType$`/`RemoveType$` token list: `ChosenType$`/`ChosenType2$`/`ImprintedCreatureType$`/
`AllBasicLandType$`/`AllNonBasicLandType$` as a token (29 of 256 real `AddType$` lines) need a runtime value this port
has no evaluator for; `AddAllCreatureTypes$` (8) needs the full creature-type enum, which needs the `Registry` this port
does not inject; and `RemoveSuperTypes$`/`RemoveCardTypes$`/`RemoveSubTypes$`/`RemoveLandTypes$`/
`RemoveCreatureTypes$`/`RemoveArtifactTypes$`/`RemoveEnchantmentTypes$` (62 of 284 real `AddType$`/`RemoveType$` lines)
are a bulk "wipe this whole category first" flag, most often paired with `AddType$` in a real "Enchanted creature is a
Turtle" shape (`StaticAbilityContinuous.java:425-448`) — applying `AddType$` alone without the wipe the line also asks
for would leave a card with both its old and new creature types, an answer actively worse than skipping the line
outright. 201 of 284 real `AddType$`/`RemoveType$` lines carry none of the above and resolve.

Intimidate's own `CantBlockBy` synthesis (`## Block legality: CantBlockBy`, below) is `SharesColorWith`'s real reason
for existing in `valid.go`, not this Layer 4 slice — the two landed together but are otherwise unrelated.

**Layer 5, `applyContinuousColor` (`continuous.go`), and `ColorMod`.** `AddColor$`/`SetColor$` are Layer 4's own
sibling, the next slice of `Mode$ Continuous` once a real caller needed `Card.Colors()` to fold something too.
`ColorMod`/`ColorEffect` (`colormod.go`) copy `TypeMod`'s own structure, with one difference `AddType$`/`RemoveType$`
did not need: `SetColor$` (Java's own `overwriteColors`) replaces the running color set outright rather than unioning
into it, so `ColorEffect` carries one `Overwrite bool` instead of two separate `cardtype.Line` fields, and `foldColor`
branches on it per effect in `Timestamp` order — `SetPower$`/`AddPower$`'s own `LayerSetPT`/`LayerModifyPT` split,
collapsed to a bool since Layer 5 has no third sub-layer to distinguish. `colorFromName` (valid.go's own `colorMatches`,
pulled out so both share it rather than duplicating the five-color switch) maps the bare color words; `colorTokens`
(continuous.go) adds the two fixed tokens Java's own `getColorsFromParam` special-cases (`"All"` → `mana.AllColors`,
`"Colorless"` → no color at all, both real corpus shapes) and skips the whole line the instant `"ChosenColor"` appears
anywhere in the `" & "`-split list (7 of 61 real lines) — a runtime value (`Card.getChosenColors()`) this port has no
evaluator for, `typeTokens`'s own "whole line, not partial" choice applied identically here. 54 of 61 real
`AddColor$`/`SetColor$` lines carry none of it and resolve. `Card.Colors()` folds `ColorMod` over the printed
`Colors:`-override-or-mana-cost base the same way `Type()` folds `TypeMod`; `Move`/`Game.Clone` treat `ColorMod`
identically to `TypeMod`/`PT`.

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
insertion-ordered keys give Java.

A legendary permanent exempted by its own `Mode$ IgnoreLegendRule` static ability (`ignoreLegendRule`,
`staticability.go`) is filtered out before grouping even starts, the same as Java's own `handleLegendRule` filters its
candidate list first (`GameAction.java`). Ported from
`StaticAbilityIgnoreLegendRule.ignoreLegendRule`/`applyIgnoreLegendRuleAbility`: every battlefield permanent is walked
as a possible host (Battlefield only, the same trim `cantBlockBy`'s own doc comment justifies), and a `ValidCard`-less
line (1 of the 11 real corpus lines, an unconditional "the legend rule doesn't apply") matches every card, exactly
Java's own `matchesValidParam` contract for an absent param. Two of the 11 carry `IsPresent$`/`PresentCompare$` (a "you
control exactly two of them" condition `StaticAbility.java`'s own generic `checkConditions` evaluates, no equivalent for
any static-ability mode in this port yet) — skipped rather than guessed at, the same safe default an unresolvable
`Toughness` leaves a creature alive under.

One of Java's own corner cases is still not here: Partner-with-a-non-legendary-creature-name pairs (Spy Kit and similar)
sharing a "true name" even though their printed names differ — needs `StaticData`'s own card-name lookup, which this
port's `carddb`/`compile` layer has no equivalent of.

## The World rule needed no new field, only the one every zone change already stamps

`resolveWorldRule` is `handleWorldRule` (CR 704.5m): at most one permanent with the World supertype may be on the
battlefield at once, across every player at once, not grouped per player the way the legend rule is above — the newest
one survives, and every other one goes to its owner's graveyard. It asks nobody anything, unlike the legend rule right
above it: Java's own version picks the newest by `getWorldTimestamp()`, a plain comparison, not a choice, so
`resolveWorldRule` takes no `PlayerController` at all.

The comparison it needs already existed: `Card.Timestamp` (`## Handles, not pointers`'s own table, above) is stamped on
every zone change for CR 613's own layer ordering, and a World permanent enters the battlefield through the same
`Move`/`put` every other permanent does, so there was no `getWorldTimestamp()`-equivalent field to add — the general
`Timestamp` already answers "which one is newest" without knowing anything about World in particular.

A tie for the newest timestamp destroys every tied permanent too, not just the older ones — Java's own
`toKeep.size() == 1` guard only spares the survivor when there is exactly one. `g.timestamp` increments on every single
`put`, so no two cards placed through the public API (`NewCard`, `Move`) ever actually share one; the tie branch is
reachable only by a test that sets `Card.Timestamp` directly, the same "kept for when it becomes reachable" position
`destroyZeroDefense`'s own stack-trigger exception is already in (`## Loyalty is not a layer`, below).

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
`Turn`, `ActivePlayer` and `ActivePhase` live on `Game` now (`StartTurn`, `AdvancePhase`, `SetTurnState`), and four
steps — Untap, Draw, CombatEnd and Cleanup — have real bodies (below). Every other step (`onPhaseBegin`'s Upkeep, Main,
the rest of combat, End of Turn) still just changes `ActivePhase` and nothing else, because casting, blocking and firing
a trigger all need machinery this port has not reached; CombatEnd's own action (CR 511.3, below) needed none of that,
just resetting state this port already has. `AdvancePhase` walks through the rest as bookkeeping only, until each one's
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

Most of `MagicStack.java` is still not here: `undoStack` (nothing to undo without an interactive priority pass to undo
it during) stays unbuilt. `addSimultaneousStackEntry` (CR 603.3b's "your own simultaneous triggers, in an order you
choose," plus APNAP order between different players' own triggers) is a real gap now, not a hypothetical one:
`checkOtherETBTriggers` (trigger.go) means one card entering the battlefield can push both its own trigger and another
permanent's own trigger watching for it, off the same event -- pushed in a fixed order (the entered card's own trigger
first, then every other battlefield permanent's own, in `Players()`/zone-iteration order) rather than any
controller-chosen or APNAP order. Every real fixture and test today has at most one trigger fire per event, so this
ordering has not yet been wrong for anything reachable, but it is not CR 603.3b's rule, just this port's own arbitrary
placeholder for it. `freezeStack`/`unfreezeStack`, though, exist to serve a second ability arriving on top of one still
resolving -- and that much can happen now: `checkETBTriggers`/`checkDiesTriggers` (trigger.go) push from inside
`permanentEffect`/`attachEffect`/`PlayLand`/every graveyard-bound SBA's own resolution, so `ResolveStack`'s loop (below)
finds a second entry on top the moment the first one finishes, not from a second caller racing the first. Nothing needs
freezing because nothing is interactive: this port has no `PlayerController` method that lets anyone respond between one
ability resolving and the next being found on top (`## Controller`), so there is never a window for a _third_ ability to
arrive while the second is still open either. `effect.go`'s `Registry` no longer holds zero implementations --
`permanentEffect` and `attachEffect` (castspell.go) are `CastSpell`'s own resolutions, and `ResolveStack` has real
(non-test) callers through both.

`ResolveStack` is also CR 117's priority algorithm, for the one case this port can play out today. Priority's real job
-- offering every player, in APNAP order, a chance to respond to what is on top before it resolves -- needs a
`PlayerController` method that can activate or cast something in response, which does not exist (`## Controller`).
`CastSpell` is a `Game` method, not a controller decision, and its own empty-stack precondition means it cannot be used
to respond to what is already on top anyway. With nobody able to respond, every priority pass is a pass in succession,
so the top item always resolves next; `ResolveStack` encodes exactly that degenerate case rather than a full
pass-tracking loop nothing could yet exercise.

`turn.go`'s `beginPhase` does not call `ResolveStack`. Nothing in the phase machinery itself ever pushes onto the stack
-- only `CastSpell` does that today, driven by an explicit player decision outside `beginPhase` entirely -- so a call to
`ResolveStack` there would resolve whatever `CastSpell` already left behind at a point in the turn structure that has
nothing to do with when a caster actually stopped passing priority. It is wired in from an explicit action instead, the
same way `CastSpell`/`PlayLand` themselves are driven by a caller's decision rather than a phase-boundary hook: the
fixture's own `resolvestack` verb (`game-state-fixture.md`) and engine tests call it directly.

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

**Flying/reach, Fear, Horsemanship and every real `S:Mode$ CantBlockBy` line are checked; Menace, Intimidate, Landwalk,
Protection and Skulk are not.** `Game.DeclareCombatBlockers`'s own eligibility computation is still only "untapped
creature the defending player controls" — `CantBlockBy` is a property of one attacker/blocker pair, not of a creature in
isolation, so it is checked afterward instead, via `CanBlock`, against the controller's own answer
(`## Block legality: CantBlockBy` has the full account, including why each of the five omissions is a specific missing
dependency rather than an oversight).

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

\*\*`defenderOf` (attack.go) is what `DeclareCombatBlockers` resolves an attacker's own `AttackTarget` to the player who
can legally block it (CR 802.4a): itself, if the target is a player; the target's controller, if it's a planeswalker or
battle. `dealCombatDamageStep` (combatdamage.go) never needed it at all — it walks `g.combat.Attackers` and
`g.combat.Blocks` directly, resolving each attacker/blocker pair on its own terms, so it never assumed one defender to
begin with. `DeclareCombatBlockers` used to: it called `defenderOf` once, for `g.combat.Attackers[0]`, and asked only
that one player to declare blocks for every attacker. It now calls `defenderOf` per attacker, groups by the result, and
asks each distinct defender in turn — only about the attacker(s) actually attacking them, offering only their own
eligible creatures (CR 506.4's "each defending player" read per defender rather than assumed singular). Defenders are
asked in the order their first attacker appears in `g.combat.Attackers`, so the sequence is deterministic across a run
(GO-12) — the same reasoning `resolveLegendRule`'s own `order` slice exists for. A defender with no eligible creature is
skipped, not asked with an empty list, matching `DeclareCombatAttackers`'s own "nothing meaningful to decide" reasoning
for an inactive player. The scenario fixture `combat-split-across-two-defending-players` (`game-state-fixture.md`) is
the first in the corpus to seat three players and exercise it end to end.

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

**`endCombat` (turn.go) is CR 511.3, wired as the End of Combat step's body.** `beginPhase`'s switch picked up a fourth
case (`## Turn structure`): the other bookkeeping-only steps (Upkeep, Main1, the rest) stay empty because they need the
stack, triggers or `SpellAbility` to do anything, but CombatEnd's real action — every creature and planeswalker/battle
stops being attacking/blocking — needs none of that, just resetting `g.combat` to its zero value the same way Java's
`PhaseHandler.endCombat` sets its `Combat` field to `null`. Before this landed, nothing cleared
`Combat.Attackers`/`AttackTargets`/`Blocks` between combats at all: `DeclareCombatAttackers` and `DeclareCombatBlockers`
only overwrite `g.combat` on the branch where something is actually declared, and both return early without touching it
when nothing is eligible (the same "nothing meaningful to decide" shortcut that makes them cheap to call
unconditionally) — a real combat's data would have silently survived into a later turn that never attacked with
anything, latent because no fixture or test happened to play two turns of combat in the same game before this one.

Not here yet: block legality beyond "untapped creature the defending player controls" — Flying/reach, menace,
protection, "must be blocked by" — waits on the general static-ability engine, above.

## Mana pool and payment

`mana.go`'s `Pool` (CR 106.4, one per `Player`) and its `Pay` method (CR 601.2h/601.2i) are M5 item 28's mana-payment
slice — the plan's own "budget the most time here" warning is about the full version, and this is deliberately not that:
`Pay` itself handles a cost's `Generic` amount plus its six "pure" shards (`ShardW`/`U`/`B`/`R`/`G`/`C`) and nothing
else, the same "plain-integer operand" discipline `valid.go`'s `compareMatches` already applies to numeric comparisons.
`Game.PayManaCost` (`manapay.go`) layers eight harder cases on top without touching `Pay` directly: `{X}` (CR 601.2b)
asks `ChoosePayX` for the value of X exactly once, before anything else in the cost resolves, and folds it into
`Generic` as `x * cost.CountX()` — every `X` symbol the cost carries stands for the same announced value, not one value
each (CR 107.3f), so a cost with two `{X}` symbols owes twice the chosen amount, and `ChoosePayX` is asked once
regardless of how many `{X}` symbols there are. A negative answer is not re-checked against anything downstream —
`PayManaCost` itself reports failure before the shard loop or `Pool.Pay` ever run, since CR 601.2b restricts X to a
non-negative integer and there is no meaningful `Pay`-level failure to delegate that to. Then a two-colour hybrid shard
(`{W/U}`) asks `ChooseHybridManaColor` which colour to pay with, substitutes the plain shard for the answer, and hands
the result to `Pay` unchanged; a monocoloured hybrid (`{2/W}`) asks `ChoosePayMonocoloredHybrid` whether to pay with
colour or with the shard's own `CMC` (2) worth of generic instead — `true` substitutes the plain colour shard the same
way the two-colour case does, `false` adds the shard's `CMC` onto the cost's `Generic` amount instead of adding a shard
at all; a colourless hybrid (`{C/W}`) asks `ChoosePayColorlessHybrid` the same true/false shape, but `false` substitutes
`mana.ShardC` for the symbol instead of touching `Generic` — its other side is a specific mana type, not an amount, so
it is resolved the same way the two-colour case's colour choice is, not the way the monocoloured case's generic choice
is; a single-colour Phyrexian shard (`{W/P}`, CR 118.4) asks `ChoosePayPhyrexian` the same true/false shape, but `false`
adds 2 to a running `life` total instead of touching `resolved` or `Generic` at all; a hybrid Phyrexian shard
(`{B/G/P}`) asks `ChoosePayHybridPhyrexian` a genuinely three-way question — its return type is `mana.Colors`, not
`bool`, since there are two colours to offer plus life, and returning the zero `mana.Colors` is how the controller picks
life over either one, the same "reuse the type, encode the third option in its zero value" shape
`ChooseHybridManaColor`'s own two-colour return already established, just extended one option further. Once every shard
is resolved, `PayManaCost` asks `ChoosePayGeneric` once per unit of the cost's `Generic` amount still owed (CR 106.6:
"any type of mana, including colorless mana, can be used to pay a generic mana cost") — a plain `mana.Shard` answer (one
of `ShardW`/`U`/`B`/`R`/`G`/`C`), appended to the resolved shards the same as every other case, then handed to `Pay`
with `Generic` itself reduced to zero. Each unit is its own call, not one combined answer for the whole amount — the
same "one decision per shard" granularity `ChooseHybridManaColor` already uses per hybrid symbol, not batched across a
whole cost. `PayManaCost` only deducts `life` from `Player.Life` directly, after `Pool.Pay` reports success, so a
payment that fails on an unrelated shard never costs life for a Phyrexian shard (either kind) it already resolved.
Paying life this way fires `LifeChanged` with `Source: NoCard` (no card causes it — `PayManaCost` takes no card
parameter today) and `Amount` as the negative life lost, the same wiring discipline `CounterChanged` got when a real
mutator needed it (`## Events, wired`, below). A snow (`{S}`, CR 106.3a) shard asks `ChoosePaySnow` which color of
floating snow mana pays it — unlike `{X}`, each `{S}` symbol in a cost is its own independent question (CR 106.3a puts
no "announced once" language on it the way CR 601.2b does for X), so a cost with two `{S}` symbols asks twice and can
take two different colors' snow mana. The answer is collected into a separate `snow []mana.Shard` slice, not `resolved`,
because a snow requirement can only be paid from `Pool`'s own snow bucket for that color, never the plain one — folding
it into `resolved` the way every other shard is would let `Pool.Pay`'s plain-pip matching spend ordinary mana for it,
which is not legal. All eight ask before `Pool.PayWithSnow` (`mana.go`) ever sees the cost; `Pay` itself (now
`PayWithSnow` called with no snow shards) is unaware any hybrid, Phyrexian, `{X}`, `{S}` or controller-chosen generic
shard exists — it always receives an already-resolved shard list, a `Generic` of zero, and an empty snow slice when
called from here.

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

**`Pay`'s own fixed generic order only fires for a caller that reaches it directly, bypassing `PayManaCost`.** CR
106.6/601.2h give the paying player free choice of which floating mana covers a generic cost; `Pay` itself still spends
colorless first, then white/blue/black/red/green, a deterministic tie-break rather than a decision, for exactly the
callers that were already calling `Pay` before `PayManaCost` existed (its own tests, `mana_test.go`). Every real answer
to that choice now goes through `ChoosePayGeneric` instead (above) — `PayManaCost` never leaves a nonzero `Generic` for
`Pay` to guess about.

**`PayManaCost` now has real `TEST-5` fixture coverage, not just unit tests.** Two gaps blocked writing one:
`setup.state`'s own `manapool=` key (Java's `GameState` format) parsed but never applied to `Player.ManaPool`
(`internal/fixture/load.go`'s `Unapplied` list carried it since before `Pool` existed), and `actions.log` had no verb to
call `PayManaCost` at all. Both are fixed: `applyManaPool` reads `manapool=`'s space-separated color letters (`"W W U"`,
`MagicColor.Color`'s own short names, not a mana cost's `"2W"` shorthand) into the pool the same
`Pool.Add`/`AddColorless` every unit test already uses, and `paymanacost <player> <cost>` (`queue paygeneric <shard>`
for its own generic answers) calls `Game.PayManaCost` directly — the same "callable ahead of a full turn" position
`DeclareCombatAttackers` was in before combat glued together (`manapay.go`'s own doc comment). `Pool.Breakdown`
(`mana.go`) is the new exported reader both the fixture harness's own `compareGames` and any future caller need to
compare two pools' full contents rather than just `Total`. `PersistentMana:` stays `Unapplied`: `Pool` tracks no
persistence, and CR 500.4's own emptying applies to every kind of floating mana this port has. Every hybrid and
Phyrexian shard has its own verb too now (`queue hybridmanacolor`, `queue paymonocoloredhybrid`,
`queue paycolorlesshybrid`, `queue payphyrexian`, `queue payhybridphyrexian` -- `game-state-fixture.md`'s own verb
table), each mirroring its `ScriptedController` method's argument shape exactly -- a bare color letter or a bool, and
`payhybridphyrexian`'s own third answer written as the literal word `life` rather than an empty value, the same "decline
explicitly" convention `queue attackers none`/`queue blocks none` already use. Five fixtures exercise six of the seven
resolved shapes end to end: `mana-payment-pays-colored-and-generic`, `mana-payment-fails-atomically`,
`mana-payment-hybrid-color-choice`, `mana-payment-monocolored-hybrid-generic` and `mana-payment-hybrid-phyrexian-life`.

**`{X}` turned out not to need a real caster after all.** Earlier passes over this section assumed CR 601.2b's "the
player announces X" belonged to the missing casting flow (M6) and left it unresolved alongside snow, which turned out to
need no caster either (below). Revisiting `{X}`: `PayManaCost` is already called standalone, ahead of any cast (the same
position every other shape here is in), and CR 601.2b's announcement is itself just one more decision `PlayerController`
can be asked before the rest of the cost resolves — no different in kind from `ChoosePayGeneric` asking which mana
covers a generic unit. `ChoosePayX(g, decider, cost) int` is that decision, asked once per cost regardless of how many
`{X}` symbols it carries, and its answer times `cost.CountX()` is added to `Generic` before the shard loop runs at all.
`queue payx <n>` (`game-state-fixture.md`) is the verb; `mana-payment-resolves-x` is the fixture, paying `{X}{R}` with
X=3 from a pool of one red and three white.

**`Pool.Add` has a real (non-test) caller now: `TapLandForMana` (`manaability.go`), CR 305.6's intrinsic land ability.**
Every fixture above preloads the pool through `manapool=` directly; nothing in the engine had ever put mana there
itself. The blocking question — how Forge derives a basic land's "T: Add [color]" ability, since
`forge-gui/res/cardsfolder/p/plains.txt` carries no `A:` line at all, only `Oracle:({T}: Add {W}.)` — turned out to live
in a file the previous search had not checked: `CardState.java`'s `getLandTraitChanges`/`getLandManaForColor` walks
`MagicColor.Color.values()`, and for each one whose `getBasicLandType()` the card's current type line has as a subtype
(`hasSubtype("Plains")` for white, and so on for the other four), synthesizes
`AB$ Mana | Cost$ T | Produced$ <color> | Secondary$ True | ...` on the fly rather than reading it from script text.
That is a fixed CR 305.6 mapping, not script content a compiled `AST` carries, so this port keys off `cardtype.Line`'s
own subtypes the same way `enchantSpec`/`resolveWorldRule` already read a type line directly (`## State-based actions`,
above) instead of waiting on M6's effect dispatch. `basicLandType` holds the five-entry map (`Plains`→White,
`Island`→Blue, `Swamp`→Black, `Mountain`→Red, `Forest`→Green); `TapLandForMana(pid, land, color)` checks control, zone,
tapped state and the matching subtype, then taps and calls `Pool.Add` in the same call, since CR 605.3 gives a mana
ability no stack to wait on. It reports `bool`, the same "declined by the rules, not a bug" contract `PayManaCost`
already carries — a dual-typed land (a Snow-Covered Plains Island) keeps two separate intrinsic abilities, but tapping
is one shared cost, so activating either one leaves the other unavailable. Whether the mana produced is snow (CR 106.3a)
is read off the land's own Snow supertype (`cardtype.Snow`) at the very end, once tapping is known to succeed —
`forge-gui/res/cardsfolder/s/snow_covered_plains.txt` writes `Types:Basic Snow Land Plains`, the identical no-`A:`-line
shape a plain Plains has, differing only in that one supertype, so nothing about the ability itself changes, only which
of `Pool`'s two buckets for that color receives it (`## Mana pool and payment`, above).
`tapformana <player> <id> <color>` is `actions.log`'s own verb for it (`game-state-fixture.md`), and
`mana-payment-tap-land-for-mana` is the first fixture where the paid mana comes from a card instead of `manapool=`.

**Snow turned out to need a bigger change than `{X}` did, but still no caster.** The earlier assumption was that snow
needed "a `Pool` redesign for snow-provenance" — true as far as it goes, but the redesign is a fixed, bounded one, not
an open-ended one: `Pool` (`mana.go`) gained a second bucket per color (`snowWhite`, `snowBlue`, ...) alongside the six
plain ones, disjoint rather than a subset count layered on the plain total, so spending never has to reconcile which
specific unit of a color was snow after the fact. `Pool.AddSnow`/`AddSnowColorless` mirror `Add`/`AddColorless` exactly;
`Pool.SnowBreakdown` mirrors `Breakdown`'s own shape for the snow half alone, and `Breakdown` itself now sums both
buckets per color (CR 106.3a: snow mana is still that color), so an existing caller reading total mana of a color is
unaffected by snow's existence. The harder part was where snow mana can substitute for plain: a same-color pip or a
generic unit accepts snow-tagged mana the same as plain (CR 106.3a again — snow is a type of that color, not a different
one), so `Pool.Pay`'s own shard loop and generic loop both fall back to the snow bucket once the plain one is empty; a
snow (`{S}`) requirement is the one thing plain mana cannot cover, so it needs its own consumption path that never
touches a plain bucket. `Pool.PayWithSnow(cost, snow []mana.Shard)` is that path — `Pay` itself is now `PayWithSnow`
called with `snow` nil, so its signature and every existing caller are unchanged. `ChoosePaySnow(g, decider) mana.Shard`
is `PayManaCost`'s own decision, asked once per `{S}` symbol independently (unlike `{X}`, CR 106.3a puts no "announced
once" language on `{S}`, so two `{S}` symbols in one cost can take two different colors' snow mana) — its answers
collect into a `snow` slice kept separate from `resolved` for exactly the reason above: folding a snow answer into
`resolved` would let the ordinary plain-pip matching spend non-snow mana for it. `queue paysnow <shard>`
(`game-state-fixture.md`) is the verb; `mana-payment-resolves-snow` is the fixture — a real Snow-Covered Plains tapped
for snow white (`TapLandForMana`'s own Snow-supertype check, above), then spent paying a bare `{S}` cost.
`setup.state`'s own `manapool=` cannot express snow mana at all: `GameState.java`'s own
`processManaPool`/`updateManaPool` iterate `ManaAtom.MANATYPES`
(`forge-core/src/main/java/forge/card/mana/ManaAtom.java` — white/blue/black/red/green/colorless, no snow entry), so the
Java oracle's own dump format has nowhere to write a snow flag either; this is not a deviation this port introduces, it
is the format's own limit, and `queue paysnow` (a `ScriptedController` answer, `game-state-fixture.md`'s existing
Crucible-only category) plus `tapformana` on a real snow land are the only way a fixture gets snow mana into a pool
today.

## Playing a land is not casting a spell

`Game.PlayLand` (`land.go`) is CR 305: no cost, no stack (CR 305.1) — the card moves straight from hand to the
battlefield. It is the first thing in this port that gets a card from a player's hand onto the battlefield through a
real game action rather than `setup.state` placing it there directly, which is why it took this long to reach even
though nothing about it needed the M6 effect-dispatch machinery `manaability.go`'s own precedent already established
mana abilities and basic-land-type checks do not need: CR 305 is a fixed rule with no script content to interpret, the
same category `TapLandForMana` and `enchantSpec`/`resolveWorldRule` are already in.

Timing is CR 305.3's own gate ("any time they could cast a sorcery"), collapsed to what this port can check without an
interactive priority system: `pid` is the active player, `ActivePhase` is `Main1` or `Main2`, and the stack is empty.
That last check is never false today — nothing pushes an ability yet outside `stack.go`'s own tests — but is checked
anyway, on the same "should not have to change again once casting exists to make it meaningful" reasoning
`ResolveStack`'s own doc comment already gives for building the resolve loop ahead of a real pusher. `mayPlay` alternate
zones, `CantBeCast` static abilities and every other `canPlayLand` condition Java checks beyond these three plus "in
hand" plus "is a land" are M5-M6 gaps this port does not have the machinery for yet (a quality-matching static-ability
engine, mostly) and are not checked, the same "a rule this port has not implemented simply never fires" position
`CheckStateBasedActions`'s own doc comment already states for its own gaps.

CR 305.2's one-land-per-turn limit is `Player.LandsPlayed` (`player.go`), a genuinely new field with a real caller —
`engine.Player` had no lands-played count at all before this, even though `setup.state`'s own `landsplayed=`/
`landsplayedlastturn=` keys have existed and parsed successfully since before `PlayLand` did, landing in `Unapplied` for
lack of anywhere to put them (the same position `manapool=` was in before `TapLandForMana`). `maxLandPlays` is Java's
own `getMaxLandPlays()` default of 1 with no `adjustLandPlays` term added — nothing in this port grants an extra land
play yet, so there is nothing to add. `cleanupStep` (`turn.go`) now rolls `LandsPlayed` into `LandsPlayedLastTurn` and
resets it to zero for every player, not just the active one — `Game.onCleanupPhase` in Java loops every registered
player the same way, the same "every player, not just the active one" scope CR 500.4's own mana-pool emptying already
has in this port. `LandsPlayedLastTurn` has no reader yet (a replacement effect keyed on "if you've played a land this
turn" would be one), but resets alongside `LandsPlayed` regardless, since nothing about resetting per-turn state should
wait on a reader existing before it starts happening correctly.

`playland <player> <id>` (`game-state-fixture.md`) is the verb, `id` from `Loaded.CardByFixtureID` the same as
`tapformana`. `Game.PlayLand`'s `bool` return is not asserted, the same "declined by the rules, not a fixture error"
convention `paymanacost`/`tapformana` already established. `land-played-then-tapped-for-mana` is the fixture: a Plains
drawn into hand at setup, played, then tapped for its own intrinsic mana the same turn and spent paying a `{W}` cost —
CR 302.6's summoning-sickness restriction is a creature's own tap-ability gate, not a land's mana ability, so
`TapLandForMana` correctly has no `SummonSick` check to get in the way of playing and tapping the same land in one turn.

`compareGames` (`scenario_test.go`) gained `LandsPlayed`/`LandsPlayedLastTurn` alongside `ManaPool`'s own two
comparisons — a field `Load` now applies has to be a field the scenario harness actually checks, or a fixture naming it
would silently assert nothing (the exact gap the fixture-level `Dump` audit that found `ManaPool`'s own missing
write-back caught, `game-state-fixture.md`'s own section on it).

## Casting a spell needed the stack for real, for the first time

`Game.CastSpell` (`castspell.go`) is CR 601, trimmed to the two shapes with nothing left to decide once a target (an
Aura) or nothing (every other permanent) is chosen. `PlayLand`'s own precedent — a fixed CR rule needs none of the M6
effect-dispatch machinery its neighbors in this file keep deferring to — turned out to reach further than land-playing
alone: casting a permanent and resolving it into a battlefield permanent is _also_ a fixed rule, not a card-script
effect, once `APIPermanentCreature`/`APIPermanentNoncreature` (`ability.go`'s generated constants) and `effect.go`'s own
`Effect`/`Registry` dispatch (built at M4, holding zero implementations since — `CLAUDE.md`'s own M4 status line) are
read together: Java's `SpellPermanent` constructs one or the other API depending on `cardstate.getType().isCreature()`,
but never through `AbilityFactory.getAbility`'s script-string dispatch the way a real effect implementation would — the
same "hardcoded, not corpus-script-driven" shape `TapLandForMana`'s own `CardState.java` precedent already established
for a different API entirely.

`castableAsPermanent` is `CardState.java`'s own `getBasicSpells` routing, read directly: a creature, artifact,
enchantment, planeswalker or Battle, and not an Aura. An Aura routes to `getAuraSpell()` in Java (`castAura`,
`## Aura targeting is a spell's own second cast-time decision`, below) since it needs a target chosen at cast time (CR
601.2c) that this no-decision branch has none of. A land is never a spell at all (CR 305.1) and needs no special case:
it is simply absent from the list `castableAsPermanent` checks, the same "excluded by not appearing" shape `PlayLand`'s
own doc comment already uses for the reverse case (a non-land declined by `PlayLand`). Instant and sorcery route to a
`SpellAbility` this port does not build yet (they resolve into a script effect, not "become a permanent") and are
excluded the same way.

Timing is `PlayLand`'s own CR 305.3 check, copied rather than shared: active player, a main phase, an empty stack. CR
601.3a gives permanent spells the identical sorcery-speed default lands have (CR 307.5), so the two checks read
identically today — they diverge the moment a real Flash-granting effect exists, which is exactly why `CastSpell` keeps
its own copy instead of factoring out a helper for a coincidence that will not stay one.

**`permanentEffect` is the first `Effect` implementation with a real (non-test) caller.** `effect.go`'s own doc comment
already predicted its shape — "Implementations are stateless shared values" — and `PermanentEffect.java`'s own `resolve`
confirms it: strip `Dash`/`Blitz`/`Warp`/`Sneak` (alternate-cast-mode keywords this port cannot grant a spell), and what
is left is `game.getAction().moveToPlay` plus `table.triggerChangesZoneAll` (CR 603 firing) —
`Game.Move(a.Source, Battlefield, a.Controller)` followed by `checkETBTriggers(a.Source)` here
(`## Trigger firing found its first mode`, below). Java gives `PermanentCreatureEffect` its own subclass only to
override `getStackDescription` (display text for the stack, showing power/toughness); this port has no stack-description
system at all, so one value answers for both `APIPermanentCreature` and `APIPermanentNoncreature` — `NewRegistry()`
registers it twice. `Move`'s own ETB logic (loyalty/defense grants, `SummonSick = true`) already existed and needed no
change: a permanent entering the battlefield by resolving off the stack is not a special case of entering, so casting a
planeswalker or Battle spell grants loyalty/defense correctly for free
(`cast-a-planeswalker-spell-resolves-to-battlefield`, `cast-a-battle-spell-reaches-the-stack`).

`ResolveStack` (`stack.go`) had zero non-test callers before this — its own doc comment said as much ("casting has no
cost-payment or targeting to drive it"). `CastSpell` pushing a real `Ability` and `NewRegistry` giving `ResolveStack`
something to dispatch to is what makes that sentence no longer true, for the one shape it can reach. `resolvestack`
(`game-state-fixture.md`) is the fixture verb, and — unlike every bool-returning verb in this file — it does not swallow
its error: `Registry.Resolve`'s own `ErrUnimplemented` is a real gap (GO-7's "a bad card fails its game"), not a
declined decision, so a fixture naming an API this port cannot resolve yet fails loud instead of silently doing nothing.

`castspell <player> <id>` is the verb for `CastSpell` itself, `id` from `Loaded.CardByFixtureID` the same as every other
card-naming verb; its `bool` return is not asserted, the same convention `paymanacost`/`tapformana`/`playland` already
established. `cast-a-creature-spell-resolves-to-battlefield` is the fixture: two Forests tapped for a real Grizzly
Bears' `{1}{G}` cost, cast, then resolved onto the battlefield — the mana comes from real lands tapped after reaching
Main1, not `setup.state`'s own `manapool=`, because `emptyManaPools` (CR 500.4) clears any preloaded pool on the very
first `startturn`/`advance` a scenario runs, the same trap `mana-payment-tap-land-for-mana`'s own fixtures already route
around by tapping mid-scenario rather than preloading.

## Aura targeting is a spell's own second cast-time decision

`castAura` (`castspell.go`) is `CastSpell`'s own branch for an Aura, `CardState.java`'s `getAuraSpell()` read alongside
`AttachEffect.java`'s own `resolve`. Java builds an Aura's cast-time ability as
`SP$ Attach | ValidTgts$ Card.CanBeEnchantedBy,Player.CanBeEnchantedBy` — a generic `Attach` spell whose own
target-choosing machinery (`TargetSelection`, interactive targeting) this port does not have. What survives the trim is
CR 601.2c's own requirement stripped to its essentials: a target is chosen before the cost is paid, from whatever the
Aura's own `Enchant` restriction (`enchantSpec`, `## The World rule needed no new field...`'s neighbor section, above —
the same helper `cleanupDanglingAttachments` already uses) actually allows.

`enchantTargets` (castspell.go) builds that eligible set: every battlefield permanent, across every player, `Matches`
(`## State-based actions`, above) accepts against the Aura's own parsed spec, `self` (`Matches`'s own `source`
parameter) being the Aura's own id — the identical convention `cleanupDanglingAttachments` already established for
re-checking an attached Aura's restriction after the fact. Two things decline the cast outright, both CR 601.2c's own "a
spell requiring a target with none legal is illegal to cast": `enchantSpec` finding nothing checkable at all (an
`Enchant Player`/`Enchant Opponent` Aura — `enchantSpec`'s own doc comment already names this gap, since `AttachedTo`
has no representation for "attached to a player"), or a checkable spec matching zero battlefield permanents. A lone
eligible target is assigned automatically, `assignAttackTargets`'s own "nothing meaningful to decide" reasoning; more
than one asks `ChooseEnchantTarget`, the new twentieth `PlayerController` method (`## Controller`, above).

`Ability` (`ability.go`) gained a `Target CardID` field for this — its own doc comment had already reserved the shape
("once casting or targeting exists to fill them") before this landed. `attachEffect` reads it back at resolution: `Move`
to the battlefield, `Attach` to `a.Target`, then `checkETBTriggers` the same as `permanentEffect`. CR 608.2b's own
fizzle check — re-validating the target is still legal right before resolving — is not ported: nothing between casting
and resolving can make a chosen target illegal in a port with no responses, so the target chosen at cast time is still
exactly as legal at resolution, and `cleanupDanglingAttachments` would catch it anyway if that ever stopped being true.

`castspell`'s own DSL verb needed no change — `CastSpell` already branches internally — but `queue enchanttarget <id>`
(`game-state-fixture.md`) is new, `queue legendarykeep`'s own "pick one id from a list" shape.
`cast-an-aura-spell- attaches-to-chosen-target` is the fixture: a real Pacifism (`{1}{W}`, `K:Enchant:Creature`) cast at
a lone Grizzly Bears on the battlefield, assigned automatically, attached at resolution.

## Trigger firing: entering, dying, attacking, blocking, casting a spell, and watching another permanent

`checkETBTriggers` (`trigger.go`) is CR 603 at its narrowest: only `Mode$ ChangesZone` with `Destination$ Battlefield`
fires — a permanent's own "when this enters" trigger. `checkDiesTriggers` is the identical narrowness applied to the
corpus's other frequent `ChangesZone` shape — `Origin$ Battlefield`, `Destination$ Graveyard`, CR 700.4's "dies" —
checked only against the dying card's own `Card.Self` triggers. `isETBTrigger`/`isDiesTrigger` factor the shared
`Mode$ ChangesZone` + zone-key check both need, on top of `hasZone`.

`checkAttacksTriggers` is CR 508.3's own mode, `Mode$ Attacks`, entirely — not a `ChangesZone` shape at all, so it needs
no zone-key check, only `ValidCard` matched against the declared attacker. Ported from `TriggerAttacks.performTest`.
Unlike `checkETBTriggers`/`checkDiesTriggers`, this needs no separate "own" and "other" loop: `TriggerAttacks` itself
never special-cases the attacker's own trigger, so `ValidCard$ Card.Self`/`Creature.Self` (1,282 of 1,606 real corpus
lines — the attacker's own trigger) and `ValidCard$ Creature.YouCtrl` (an anthem-shaped "whenever a creature you control
attacks" watcher) fall out of the identical single walk over every battlefield permanent, just with a different
`ValidCard` string and a different host — one loop where `checkOtherETBTriggers` and `checkOtherDiesTriggers` each
needed a second one. Not resolved: `Attacked$` (47 real lines) — `performTest` matches it against a `GameEntity` (a
player, planeswalker or Battle), and `Matches` (valid.go) only evaluates a `*Card`; `Alone$` (57), `FirstAttack$` (4),
`DefendingPlayerPoisoned$` (1) and `AttackDifferentPlayers$` (1) — each its own runtime condition (how many other
attackers, a creature's own attack-count history, a player's poison count, attacking more than one player at once) this
port tracks nothing for. A trigger carrying any of these five is skipped entirely, not fired unconditionally (GO-7) —
1,496 of 1,606 real lines carry none of them. Called once per declared attacker, after tapping and target assignment
both land (`Game.DeclareCombatAttackers`, attack.go) — `enginelint`'s `attack` group gained `trigger` as a dependency,
the same way `castspell`/`land`/`action` already have.

`checkBlocksTriggers` is CR 509.2's own "whenever ~ blocks" mode, `Mode$ Blocks`, ported from
`TriggerBlocks.performTest` — the identical one-walk shape `checkAttacksTriggers` already established, since
`TriggerBlocks` never special-cases the blocker's own trigger either: `ValidCard` matched against the declared blocker
covers both "when this blocks" (106 of 127 real lines, `Card.Self`) and "whenever a creature you control blocks" alike.
Not resolved: `ValidBlocked$` (8 of 127 real lines) — `performTest` matches it against the FULL collection of attackers
one blocker blocks (`AbilityKey.Attackers`), which this port's `Block` (combat.go) never groups back into a per-blocker
set, so a trigger carrying it is skipped entirely rather than checked against only the one attacker in the current
`Block`. Called once per declared `Block`, from `DeclareCombatBlockers` (block.go), after `CanBlock` and `menaceLegal`
have both already filtered the pairing down to a legal one — CR 509.2 fires only for a legally declared block, not one
either filter already dropped. `enginelint`'s `trigger` group gained `combat` as a dependency (for `Block` itself);
`block` gained `trigger`.

`checkSpellCastTriggers` is CR 603's own "a player casts a spell" mode, `Mode$ SpellCast`, ported from
`TriggerSpellAbilityCastOrCopy.performTest`. Like `checkAttacksTriggers`, one walk over every battlefield permanent
covers both a card's own trigger and another permanent watching for someone to cast a spell — Java's `performTest` never
special-cases the caster's own card either. `ValidCard` is optional here, unlike every other mode this port checks:
`matchesValidParam` (`CardTraitBase.java`) returns true for a missing param, and 100 of 1,435 real corpus lines carry no
`ValidCard` at all ("whenever you cast a spell," no restriction on which one). `ValidActivatingPlayer` is the corpus's
dominant param (1,216 of 1,435 — more common than `ValidCard` itself), matched by a new `matchesActivatingPlayer` rather
than `Matches` (valid.go): a `Player`, not a `Card`. Three bare values cover 1,191 of those 1,216 — `You`
(`activator == hostController`), `Opponent` (`activator != hostController`, the identical no-team simplification
`OppCtrl`/`OppOwn` already carry, `valid.go`'s own doc comment) and `Player` (unrestricted). A qualified form
(`Player.Opponent`, `Player.EnchantedBy`, `Player.NonActive`, `Player.Active`, `Player.Other`, `Player.Chosen` — 25
lines) has no player-valid evaluator built for it and never matches, the identical "skip rather than fire
unconditionally" contract `hasAnyParam` already gives `checkAttacksTriggers`' own five unresolved params. Also skipped
via `hasAnyParam`: `ValidSA`/`ValidSAonCard` (a `SpellAbility`, not a `Card` — `Matches` cannot evaluate one),
`TargetsValid`/`CanTargetOtherCondition` (no per-trigger target-inspection hook), `HasXManaCost`/`NoColoredMana`/
`SnowSpentForCardsColor` (no mana-payment-detail tracking past whether the cost was paid), `IsSingleTarget` (no generic
target-count reader) and `ActivatorThisTurnCast`/`ActivatorThisTurnCastEach` (a per-turn cast-history count this port
tracks nothing for). 1,163 of 1,435 real lines carry none of these. Fired from both `CastSpell` branches (castspell.go)
right where `SpellCast` (the event) already fires — cast time, not resolution, the same place Java's own
`checkTriggerEffects` call sits.

Both started out checked only against the one card's own `Triggers`, not every other permanent's own triggers watching
for someone else's zone change. `checkOtherETBTriggers`/`checkOtherDiesTriggers` close that gap for both: every
permanent already on the battlefield, other than the one that just entered (for dying, no such exclusion is even needed
— the dying card already left the battlefield by the time `checkDiesTriggers` runs, so it was never going to appear in
the walk), gets its own `Triggers` walked against the event too (`ValidCard` matched with the watcher as source and the
watcher's own controller — Impact Tremors' `Mode$ ChangesZone | Destination$ Battlefield | ValidCard$ Creature.YouCtrl`
fires off any other creature its controller's own control enters; a Blood-Artist-shaped
`Origin$ Battlefield | Destination$ Graveyard | ValidCard$ Creature.YouCtrl` does the identical thing for dying).
`TriggerZones$ Battlefield`, present on most corpus lines shaped either way, needs no separate check: only a card this
loop already found on the battlefield is walked, so a watcher not there is never considered. An earlier version of this
port's own reasoning held that dying's version of this gap could not close the same way, since the dying card is
"already gone" by the time `checkDiesTriggers` runs — that reasoning does not survive scrutiny: it is the _watcher_ that
needs to still be on the battlefield, not the dying card being matched against, and the watcher is exactly as unaffected
by some other card leaving as an ETB watcher is by one arriving. `checkOtherDiesTriggers` is the identical shape to
`checkOtherETBTriggers`, once that was noticed.

What made this reachable in the first place, back when only the ETB half existed, was a discovery rather than new work:
`compile.Face.Triggers []*Ability` has held every `T:` line's compiled form since M3 — the identical typed shape an
`A:`/`S:`/`R:` line compiles to (`compile.Ability`, `Record: Trigger`, `Name` the trigger's own `Mode$` value) — read by
nothing downstream until now. A trigger's `Execute$` key names an `SVar` whose own compiled `Ability.Name` is the API
its effect would run (`triggerEffectAPI`); `APIByName` turns that back into an `APIType` the same way `CastSpell`
already turns `c.Type().Has(cardtype.Creature)` into `APIPermanentCreature`. No new parser, no compile-pipeline change —
`T:`/ `SVar:` lines were always compiled, just never consumed.

`checkETBTriggers` is called from every real "moves onto the battlefield" site this port has —
`permanentEffect.Resolve`, `attachEffect.Resolve` (castspell.go), `Game.PlayLand` (land.go) — rather than from
`Game.Move` itself: `Matches` (valid.go) depends on `game.go`, so `game.go` cannot depend back on anything that calls it
without the exact cycle `ability.go`'s own doc comment already describes for why `Ability` moved out of `effect.go`.
`checkDiesTriggers` is called from every state-based action that can move a card to a graveyard (`action.go`):
`destroyLethalToughness`, `destroyDamagedCreatures`, `destroyZeroLoyalty`, `assignBattleProtector`'s
no-eligible-protector fallback, `destroyZeroDefense`, `resolveLegendRule`, `resolveWorldRule`,
`cleanupDanglingAttachments` — eight call sites, one per SBA that can put a permanent in a graveyard from the
battlefield today. `enginelint`'s `trigger` group sits above `game`/`valid`/`stack`; `castspell`/`land` gained it as a
dependency for the ETB half, `action` for the dies half.

A matching trigger pushes its own `Ability` onto the stack the same way `CastSpell` pushes a cast spell (CR 603.3's own
"a triggered ability becomes an object on the stack") — `ResolveStack`'s loop (`## Stack`, above) finds it on top the
instant the resolution that pushed it returns, with nothing "frozen" in between since nothing can respond either way.
`Ability` gained a `Params *compile.Ability` field (`ability.go`) to carry the trigger's own `Execute$` sub-ability
along onto the stack: `triggerEffectAPI` used to return only the `APIType`, discarding `Defined$`/`NumCards$`/every
other key the sub-ability itself carries, which worked only as long as nothing on the stack ever needed to read one back
— `drawEffect` (below) is the first thing that does. Resolving what fires is still mostly a gap: a trigger's own
`Execute$` sub-ability can be any of the 202 remaining corpus-frequency effects M6 owns (`Token`, `GainLife`,
`DealDamage`, ...), and `NewRegistry` implements four of them today (casting a permanent, an Aura, and `Draw`) —
`ResolveStack` reports `ErrUnimplemented` for the other 202. `TestCastSpellFiresETBTrigger`,
`TestDestroyLethalToughnessFiresDiesTrigger`, `TestDestroyLethalToughnessFiresOtherPermanentsWatchingDiesTrigger`,
`TestCastSpellFiresOtherPermanentsWatchingTrigger`, `TestDeclareCombatAttackersFiresAttacksTrigger`,
`TestDeclareCombatAttackersFiresOtherPermanentsWatchingAttackTrigger`,
`TestDeclareCombatAttackersSkipsTriggerWithUnresolvedParam`, `TestDeclareCombatBlockersFiresBlocksTrigger`,
`TestDeclareCombatBlockersFiresOtherPermanentsWatchingBlockTrigger`,
`TestDeclareCombatBlockersSkipsBlocksTriggerWithUnresolvedParam`,
`TestCastSpellFiresSpellCastTriggerForControllerActivatingPlayer`,
`TestCastSpellSkipsSpellCastTriggerForNonControllerActivatingPlayer`,
`TestCastSpellFiresSpellCastTriggerForOpponentActivatingPlayer` and
`TestCastSpellSkipsSpellCastTriggerWithUnresolvedParam` (trigger_test.go) prove all six modes against synthetic
Elvish-Visionary-, Rotting-Regisaur-, Blood-Artist-, Impact-Tremors- and attacking/blocking/casting-creature-shaped
cards, each compiled through the real pipeline.

Corpus-frequency: 5,688 cards carry the ETB shape (`Destination$ Battlefield`); 1,341 carry the dies shape
(`Origin$ Battlefield` + `Destination$ Graveyard`, 205 of them watching some other creature rather than themselves,
`ValidCard$ *.Other`/`*.YouCtrl` — the count behind `checkOtherDiesTriggers`); 1,606 carry `Mode$ Attacks`, 1,496 of
them resolvable; 127 carry `Mode$ Blocks`, 119 of them resolvable; 1,435 carry `Mode$ SpellCast`, 1,163 of them
resolvable.

One real fixture changed because of this: `cast-a-battle-spell-reaches-the-stack` (formerly
`...-resolves-to- battlefield`) stops at the stack rather than resolving fully, because every Battle in the corpus turns
out to carry its own "create a token" ETB trigger — `Invasion of Belenon` among them — and `TestScenarios` has no way to
assert an expected `RunActions` failure the way an internal test can with `errors.Is`. The other four permanent-type
fixtures (creature, artifact, enchantment, planeswalker) are unaffected: none of those four cards carries a `T:` line.

## Block legality: CantBlockBy

`block.go`'s own doc comment had named the gap precisely: in Forge, CR 509.1b's restrictions — flying/reach, Fear,
Horsemanship, "can't be blocked except by," every other one — all run through one general mechanism,
`StaticAbilityCantAttackBlock.cantBlockBy`/`applyCantBlockByAbility`, not a keyword-specific check. `cantBlockBy`
(`staticability.go`) ports that mechanism; `CanBlock` (`block.go`) is the new public predicate combining it with the
existing "untapped creature you control" base rule.

Flying's own restriction turns out not to be a literal `S:` line at all for the overwhelming majority of the 3,276 real
corpus cards that carry it: `CardFactoryUtil.java:3910-3913` synthesizes
`Mode$ CantBlockBy | ValidAttacker$ Creature.Self | ValidBlocker$ Creature.withoutFlying+withoutReach` from the bare
`Flying` keyword at `CardState`-build time, the identical mechanism `Fear` (`:3906-3909`, 40 cards), `Horsemanship`
(`:3932-3935`, 29 cards) and `Intimidate` (`:3939-3942`, 23 cards, below) use. `cantBlockByKeywords`
(`staticability.go`) reproduces that synthesis for those four keywords, keyed off `Card.HasKeyword` rather than a
literal compiled `S:` line — nothing in `internal/carddb/compile` performs Java's own keyword-expansion step, so this
port's own version of it has to live here instead, one call site rather than a compile-time rewrite. `Menace` (408
cards) is not among them: Forge itself does not run Menace through this engine either — `getMinMaxBlocker` hardcodes
`attacker.hasKeyword(Keyword.MENACE)` directly, a minimum-blocker-_count_ rule a per-pair `CantBlockBy` check cannot
express. `menaceLegal` (`block.go`) reproduces that same hardcoding at the one point that can see the whole group: after
`CanBlock` has already filtered a defender's answer down to individually legal pairs, `menaceLegal` groups what is left
by `Attacker` and drops every `Block` naming a Menace attacker that ended up with fewer than two distinct blockers,
entirely rather than reducing it to one — CR 702.111b makes the whole attempt illegal to declare, not partially legal.

`Intimidate` (23 cards) is now among `cantBlockByKeywords`, once `SharesColorWith` had somewhere real to live: its own
`ValidBlocker$ Creature.nonArtifact+!SharesColorWith` needed a `SharesColorWith` property `propertyMatches` (`valid.go`)
did not evaluate — before this landed, the generic `non<Type>` fallthrough it would otherwise reach read
"SharesColorWith" as a nonexistent type (`false`), which the leading `!` then negated to `true`, an actively wrong
"matches everything" rather than an absent property, which is why it stayed out until the property itself was built
rather than shipped wrong. `SharesColorWith`'s bare form (`CardProperty.java`: `card.sharesColorWith(source)`, `source`
the ability's own host) is `c.Colors().HasAny(sc.Colors())` — `mana.Colors.HasAny` already existed, so the colorless
check Java makes explicit on `card` falls out for free (`HasAny(0)` is always false regardless of which side is
colorless). A suffixed form (`SharesColorWith MostProminentColor`, `SharesColorWithOther <restriction>`, and the rest —
21 of 26 literal corpus occurrences, none of them Intimidate's own keyword-synthesized use) reads a game-wide or
remembered-list comparison this port has no evaluator for and is not matched by the exact-equality case, the same "false
for every card" fallthrough any other unimplemented property gets.

Landwalk (131 cards, `K:Landwalk:<Type>`) is also now covered, but could not join `cantBlockByKeywords`'s own fixed
table the way the other four keywords did: its own restriction, `ValidDefender$ Player.controls<Type>`, has a `<Type>`
that is the keyword's OWN argument (`K:Landwalk:Island`'s own "Island", `Landwalk.java`/`KeywordWithType.getValidType`)
— a different value per card, not a name every card carrying the keyword shares. `landwalkType` (`staticability.go`)
reads it directly off the keyword line instead, `enchantSpec`'s own precedent (`## Casting a spell`, above) for reading
a keyword's argument rather than a fixed string: `keyword.Parse(line).Args()[0]`, exactly `KeywordWithType.type`.
`ValidDefender` itself needed a new `matchesValidDefender` (staticability.go): a `Player`, not a `Card`, the identical
reason `matchesActivatingPlayer` (trigger.go) exists for `SpellCast`'s own `ValidActivatingPlayer` —
`You`/`Opponent`/`Player` are the same three bare values (4, 1 and 1 of the 8 real literal `ValidDefender$` lines),
checked against the BLOCKER's controller (`stAb.matchesValidParam("ValidDefender", blocker.getController())`) rather
than a trigger's activator. A `"Player.controls<Type>"` value — Landwalk's own entire restriction — asks whether the
blocker's controller controls at least one battlefield permanent `valid.Parse(type)` matches (`PlayerProperty.java`'s
own `"controls"` branch, `property.substring(8)`, no comparator suffix: every real corpus use of this shape is the bare
"at least one" default); `controllerControlsType` (staticability.go) is a plain battlefield scan, the same pairing
(`host.Controller`/`host.ID`) every other staticability.go check already passes to `Matches`.
`Player.Condition`/`Card.Self` (2 of the 8 real literal lines) never match, the same skip-rather-than-fire contract
every other unresolved param in this port gets. The Landwalk ignore-check (`StaticAbilityIgnoreLandwalk.java`) stays
unported: zero real corpus `S:Mode$ IgnoreLandwalk` lines exist, so nothing here can ever need to consult it.

Protection (196 cards) needs `Protection.java`'s own valid-string builder, not a fixed one. Skulk's (15 cards)
`ValidBlocker$ Creature.powerGTX` needs an SVar-driven `X`; `compareMatches` (`valid.go`) already documents a
non-numeric `Compare` operand as unresolvable, so it would just silently never match — worse than omitting it.

`cantBlockBy` walks every card on the battlefield as a possible host of a real `S:` line, not just the attacker's own:
Java's own `cantBlockBy` walks every card in `ZoneType.STATIC_ABILITIES_SOURCE_ZONES` (Battlefield, Graveyard, Exile,
Command, Stack), and a corpus frequency scan of the 364 real `^S:Mode$ CantBlockBy` lines confirms why —
`ValidAttacker$ Creature.Self`/`Card.Self` is only 235 of them; `Creature.EnchantedBy`/`EquippedBy` (34) is an Aura or
Equipment granting its own host "can't be blocked," living on a different permanent than the one it restricts.
Graveyard/Exile/Command/Stack are not walked — no real corpus line needs a source there. Within one host, `ValidBlocker`
needs no manual comma-splitting despite Java's own `.split(",")` loop: `valid.Parse` already treats a comma as OR
between `Spec.Alternatives`, so the raw param string passes straight to `Matches` unchanged, the same as any other
multi-alternative valid string this port already handles whole (`## Trigger firing`, above, for
`ValidCard$ Cleric.Other,Card.Self`). A `ValidBlocker`-less line (an unconditional "can't be blocked," 5 of the 364) is
treated as matching once `ValidAttacker` does, exactly Java's own `if (stAb.hasParam("ValidBlocker"))` skip.

Not ported from `applyCantBlockByAbility`: the "Dragon Hunter" reach exception (a `ValidBlocker` alternative containing
"withoutReach" is undone if a separate `CanBlockIfReach` static grants that specific blocker effective reach against
that specific attacker — 1 real corpus card); `ValidAttackerRelative`/`ValidBlockerRelative` (1 card).

The one architectural wrinkle: `valid.go`'s own "attacking"/"blocking" bare-form properties used to call
`g.Attackers()`/`g.Blocks()` (`attack.go`/`block.go`), which made the `valid` `enginelint` group depend on `attack` and
`block`. `cantBlockBy` needs the reverse — `block` calling into code that calls `Matches` — so those two reads were
changed to `g.combat.Attackers`/`g.combat.Blocks` directly (same package, same field, zero behavior change): `combat.go`
already held the type those two accessors were just wrapping, so `valid` only ever needed the `combat` group, not
`attack`/`block` themselves. `enginelint.json`'s `valid` allow-list dropped both; a new `staticability` group (needing
`valid`) sits between them, and `block` gained it.

`DeclareCombatBlockers` (`block.go`) could not fold this into its existing `eligible` list the way "untapped" already
is: `eligible` is computed once per defending player and shared across every attacker assigned to them, but
`CantBlockBy` is a property of one attacker/blocker _pair_, not of a creature in isolation — the same blocker can be
legal against one attacker and illegal against another in the same combat. `CanBlock` is checked instead after the
controller answers, and an illegal pairing is dropped rather than committed — the one place a controller's own answer is
re-checked, unlike every other `Choose*`/`Declare*` method (`control.go`'s own doc comment, `## Combat`, above).
Menace's own `menaceLegal` runs as a second filter on top, once `CanBlock` has already narrowed a defender's answer to
individually legal pairs.

## M6's first effect: Draw

Every trigger this port could detect (`## Trigger firing`, above) reported `ErrUnimplemented` the moment `ResolveStack`
reached it — correct as far as it went, but nothing had ever actually resolved a script-driven effect. `drawEffect`
(`draweffect.go`) is the first: `Mode$`/`DB$ Draw`, CR 120.3, ported from `DrawEffect.java`'s own `resolve`.

Reaching it needed a real gap closed first: `Ability` (`ability.go`) carried only `API`/`Source`/`Controller`/`Target`
onto the stack, nothing of the sub-ability's own params — `triggerEffectAPI` extracted just the `APIType` from a
trigger's `Execute$` sub-ability and threw the rest away, which was invisible as long as nothing on the stack ever
needed to read `Defined$`/`NumCards$`/anything else back. A new `Ability.Params *compile.Ability` field carries that
sub-ability along now; `triggerEffectAPI` returns it alongside the `APIType`, and every `PushAbility` call in
`trigger.go` sets it.

Two of Java's params are handled, the corpus-frequent shapes among a bigger vocabulary
(`AbilityUtils.calculateAmount`/`getDefinedPlayers`, both far larger than what this slice needed): `NumCards$`, only
when it is a plain base-10 integer (absent means 1, Java's own default) — a `*`-shaped or SVar-driven amount needs an
ability-context evaluator `internal/expr` does not have, the identical gap `valid.go`'s `compareMatches` already
documents for a valid-string's own numeric compare, so it errors by name rather than guessing; and `Defined$ You` (896
of 2,576 real `DB$ Draw` lines) or `Defined$ Opponent`/`Player.Opponent` (11) — every opponent still in the game,
`p.isInGame()`'s own check reproduced as `!g.Player(pid).Lost`. Everything else Java's `getTargetPlayersWithDuplicates`
can resolve (`Targeted`, `Remembered`, `TriggeredPlayer`, `TriggeredController`, plain spell targeting when `Defined$`
is absent entirely — 1,312 of the 2,576) is not: each is a named reference-resolution vocabulary this port has no
representation for yet (Memory lists exist for `IsRemembered`/`IsImprinted`, valid.go, but nothing populates one from a
trigger's own firing yet), so `drawEffect` errors by name instead of drawing for the wrong player. `Upto`,
`OptionalDecider`, `Reveal` and `RememberDrawn` are the same kind of gap — none of `PlayerController`'s methods this
port has yet covers a numeric or reveal choice — checked and rejected explicitly rather than silently ignored.

The actual draw mechanism was already built and correct: `drawStep` (`turn.go`) already drew one card for the active
player, library-empty case included. `DrawCards(pid, n)` is that same body, generalized to n cards for any player and
exported so `drawEffect` can call it — `drawStep` becomes a one-line `g.DrawCards(g.activePlayer, 1)`, not a duplicate.

`NewRegistry` (`castspell.go`) registers `APIDraw`; a new `draweffect` `enginelint` group sits above `turn` (for
`DrawCards`), `player`, `game` and `ability`, and `castspell` gained it as a dependency to register into.
`TestCastSpellFiresETBTrigger` changed from checking `ResolveStack` reports `ErrUnimplemented` naming `Draw` to checking
a real library card actually reaches hand — the same fixture, testing what is now really there instead of the gap that
used to be. Six new tests (`draweffect_test.go`) drive every resolvable and every rejected shape through the real
cast-and-resolve pipeline, `drawEffect` itself being unexported (TEST-1).

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
| `CastSpell` (on a successful cast)                                              | `SpellCast`                                         |
| `dealCombatDamageStep` (per exchange, both damage steps)                        | `DamageDealt`, plus `LifeChanged` for player damage |
| `annihilateCounters`, `dealPermanentDamage`, `Move`'s ETB loyalty/defense grant | `CounterChanged`                                    |
| `PayManaCost` (a Phyrexian or hybrid Phyrexian shard resolved to life)          | `LifeChanged`                                       |

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

Not wired, and it is a real gap rather than an oversight: anything from `PerformMulligans` itself — a mulligan is fully
visible as the `ZoneChanged` cascade `Move` already produces, and no `MulliganTaken`-shaped kind exists in the schema to
add without also bumping `SchemaVersion`, a more deliberate act than this pass earned. `SpellCast` fires for real now
too (`## Casting a spell needed the stack for real, for the first time`, above) — `CastSpell` is its first caller, once
`PushAbility`/`ResolveStack` had one worth pointing it at. `AbilityActivated`/`AbilityResolved` are wired the same way
(`## Stack`) — the same "mechanism now, content later" the effect registry already established, now with
`CastSpell`/`ResolveStack` as the first real (non-test) callers of either. `DamageDealt`/`LifeChanged` are wired for
real content too (`## Combat`, above) — every combat exchange fires `DamageDealt`, and player damage also fires
`LifeChanged` with `Amount` as the signed change (negative for the ordinary case, a loss), a sign convention this port
chose freely since nothing wired either kind before combat damage did. `LifeChanged` fires a second way now too, from
`PayManaCost` (`## Mana pool and payment`, above) when a Phyrexian or hybrid Phyrexian shard resolves to life instead of
mana — `Source: NoCard`, since paying a cost has no card of its own to name the way combat damage's attacker does.

## The scenario harness lives partly here

`TestScenarios` and `compareGames` (`scenario_test.go`) are this package's half of TEST-5's Layer 2 harness; the other
half — `Parse`/`Load`/`Dump`/`RunActions` — is `internal/fixture`'s, documented in full there
(`porting/port-log/game-state-fixture.md`'s "Scenarios" section). Worth repeating here: `compareGames` reads two
`*engine.Game`s directly rather than through `Dump`, because `Dump`'s `Id:` is a `CardID` and the two games being
compared were never going to agree on those by number.

## Not ported yet

| Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Lands |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `CardState` — face/characteristics data for transform, flip and meld                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | M5    |
| 90 of `PlayerController`'s 110 methods — everything needing `SpellAbility`, non-Aura targeting or the rest of Combat past dealing damage                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | M5-M6 |
| `AIController`, the real (non-scripted) implementation                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | M7    |
| Non-combat damage to a planeswalker or a Battle (a burn spell, an activated ability) — combat damage already removes loyalty/defense counters (CR 120.3c, 121.5); nothing outside combat deals damage at all yet                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | M5-M6 |
| The rest of CR 704.5f/704.5g's toughness — `*`, `1+*`, a `Count$` reference, or toughness a continuous effect or a counter has changed — needs `internal/expr` and the layer system, not just `strconv.Atoi`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | M5-M6 |
| The rest of the "cleanup aura" rule's legality — protection and hexproof preventing an attachment in the first place (CR 702.11h/702.16e) — needs a quality-matching static-ability engine, not the `Enchant`-restriction check itself (an Aura's own restriction against its still-present host is resolved, `## State-based actions`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | M5-M6 |
| The legend rule's own remaining corner case — Partner-with-non-legendary-creature-name pairs sharing a "true name" (needs `StaticData`'s own card-name lookup, which this port's `carddb`/`compile` layer has no equivalent of); `ignoreLegendRule` itself is ported (`## The legend rule needed CheckStateBasedActions to take a controller`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | M5-M6 |
| CR 613.6-613.8's dependency reordering within a layer — `foldPT` only sorts by timestamp, correct until two effects on one card can actually disagree about order                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | M5-M6 |
| Layers 1-3, 6 and 8 (copy, control, text, ability, rules effects) in full, plus Layer 7a (`CharacteristicDefining$`) and the rest of Layers 4/5 past a literal token list — `applyContinuousPT`/`applyContinuousType`/`applyContinuousColor` (continuous.go, `## Layer 7, Layer 4 and Layer 5`, above) resolve Layer 7b/7c's own plain-integer lines (2,192 of 2,426 real `Mode$ Continuous` lines), 201 of 284 real `AddType$`/`RemoveType$` lines and 54 of 61 real `AddColor$`/`SetColor$` lines, all `Affected$`-matched; every other layer, Layer 7a, and Layers 4/5's own dynamic-value/bulk-removal/`AddAllCreatureTypes$` remainder all need the rest of the same general engine, mostly for keys an SVar/`Count$` evaluator or a `*cardtype.Registry` this port does not inject into the engine would resolve, not a new layer number to add | M5-M6 |
| `changeZone`'s replacement effects, triggers, last-known-information and token/copy-vanishing rules                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | M5-M6 |
| `PhaseHandler`'s Upkeep, Main and End of Turn step bodies — need triggers, `SpellAbility` or the rest of Combat                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | M5-M6 |
| `Match` — a series spanning more than one game, "the loser of the last game goes first" (`DealOpeningHands` always takes CR 103.2's coin flip), Puzzle/Archenemy/Power Play's own starting-player rules                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | M5-M6 |
| The rest of CR 514.2: "until end of turn"/"this turn" effects ending — needs duration tracking this port does not have, `PT`'s own effects included                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | M5-M6 |
| A modified or unlimited maximum hand size (CR 514.1's `isUnlimitedHandSize`/a continuous effect changing it) — `MaxHandSize` is used unconditionally since layers 1-6/8 aren't built                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | M5-M6 |
| Interactive priority (`mainLoopStep`'s real APNAP pass), extra turns/phases, topsy-turvy phase order, "doesn't untap" effects — `ResolveStack` plays out only the degenerate case, nobody able to respond                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | M5-M6 |
| Original, Paris, Vancouver and Houston mulligan rules — out of scope, not deferred (PORT-6)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | never |
| `CounterChanged` for a script-written (non-named-constant) `CounterType` — `counterDetail` (`event.go`) is closed over the eight named constants; a `SpellAbility` creating an arbitrary keyword counter needs the encoding extended or replaced first (`## Events, wired`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | M5-M6 |
| `MagicStack`'s `addSimultaneousStackEntry` (CR 603.3b's controller-chosen/APNAP order for more than one trigger firing off the same event — a real gap now that `checkOtherETBTriggers` can cause it, `## Stack`, above) and `undoStack` (needs an interactive priority pass to undo mid-pass); `freezeStack`/`unfreezeStack` is no longer a gap: `checkETBTriggers`/`checkDiesTriggers` pushing during another ability's own resolution needs no freezing since nothing can respond in between either way                                                                                                                                                                                                                                                                                                                                            | M5-M6 |
| Trigger firing (CR 603) beyond "enters"/"dies"/"attacks"/"blocks"/"casts a spell" and watching another permanent do one of those — `checkETBTriggers`/`checkOtherETBTriggers`/`checkDiesTriggers`/`checkOtherDiesTriggers`/`checkAttacksTriggers`/`checkBlocksTriggers`/`checkSpellCastTriggers` (trigger.go) cover those seven; every other mode (`Tapped`, ...), `Attacks`'s own `Attacked$`/`Alone$`/`FirstAttack$`/`DefendingPlayerPoisoned$`/`AttackDifferentPlayers$`, `Blocks`'s own `ValidBlocked$`, and `SpellCast`'s own qualified `ValidActivatingPlayer$` forms and nine other unresolved params all remain gaps; resolving what fires beyond `Draw` is M6's 202 remaining corpus-frequency effects, not this                                                                                                                             | M5-M6 |
| Replacement effects (CR 616, `ReplacementHandler.java`) — same missing dependency as triggers: the ability-vocabulary port, not the value-grammar evaluator                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | M5-M6 |
| Block legality's remaining `CantBlockBy` gaps — Protection (needs `Protection.java`'s own valid-string builder) and Skulk (`ValidBlocker$ Creature.powerGTX` needs an SVar-driven `X`) — `## Block legality: CantBlockBy` (above) has the full account; flying/reach, Fear, Horsemanship, Intimidate, Landwalk, Menace and every literal `S:Mode$ CantBlockBy` line are ported                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | M5-M6 |
| Any mana ability besides a basic land's own intrinsic one (`TapLandForMana`, CR 305.6, `## Mana pool and payment`, above) — a nonbasic land, a creature, an artifact all need the M6 effect-dispatch machinery that one deliberately bypasses, since CR 305.6's ability is a fixed rule keyed off the type line, not script text                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | M6    |
