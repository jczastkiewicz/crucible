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
reference on the printed Toughness field itself — `resolveAmount`, amount.go, is wired into `Mode$ Continuous`'s own PT
params, not this text field yet), and the legend rule's own Corner Case 1
(`## The legend rule needed CheckStateBasedActions to take a controller`, below, has the full account) — needs a
card-name lookup across every creature card this game has ever printed, not just what is on this battlefield, which this
port's `*Game` holds no reference for. Damage dealt to a planeswalker or a Battle, which CR 120.3c/121.5 removes as
loyalty/defense counters rather than marking `Damage`, is wired too (`dealPermanentDamage`, `## Combat`, below) — combat
can attack one directly, so `destroyZeroLoyalty`/`destroyZeroDefense` are exercised by real play as well as by tests
that remove counters directly. Only _non-combat_ damage to a planeswalker or Battle is still a gap: nothing that deals
damage outside combat exists yet (no `SpellAbility`, no activated ability), so a burn spell or an ability aimed at a
planeswalker's loyalty has nowhere to come from regardless of whether the target-side plumbing is ready. A rule this
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

**`hostRefusesEnchant` (staticability.go) is the OTHER half of the cleanup-aura rule: does the specific host refuse THIS
specific Aura, a card-property question `enchantSpec`'s own type-restriction check (above) is not.** It is checked from
both callers of `enchantSpec` -- `enchantTargets` (cast time) and `cleanupDanglingAttachments` (every ongoing SBA pass)
-- in addition to, not instead of, the `Matches` call each already makes. Protection is ported from
`CardFactoryUtil.java`'s own Protection branch, which synthesizes a
`Mode$ CantAttach | Target$ Card.Self | ValidCard$ <valid>` line alongside `CantBlockBy`'s own `ValidBlocker$ <valid>`
-- the identical `valid` string `protectionValid` (`## Block legality: CantBlockBy`, below) already extracts for
blocking, matched against the Aura itself here rather than a candidate blocker. Hexproof is ported from
`CardFactoryUtil.java`'s own Hexproof branch (`Mode$ CantTarget | ValidTarget$ Card.Self | Activator$ Opponent`, plus a
`ValidSource$`/`ValidSA$` when the keyword names a type): bare `K:Hexproof` (80 of 110 real lines) refuses any
opponent's Aura unconditionally, `Activator$ Opponent` collapsing to the identical `aura.Controller != h.Controller`
check every other no-team-simplified `Opponent` in this port already makes. A qualified form (`K:Hexproof:Black`,
`K:Hexproof:Enchantment`, ..., 30 of 110 real lines) additionally requires the Aura itself to match a `ValidSource$`
string -- `hexproofValidSource` builds that string the same way `KeywordWithType.parse` (`Hexproof.java`'s own
superclass) does: a bare color word (`Black`, 7 real lines) gets `Card.` prepended before it reaches `Matches` (a bare
color name is not itself a recognized `baseMatches` case, valid.go), while a bare type word (`Enchantment`, 11 real
lines) and an already-qualified `Card.<Property>` value (`Card.MonoColor`, 5 real lines) both pass straight through
unchanged -- `baseMatches`'s own ordinary type-check fallthrough handles the first, `propertyMatches`'s own color/type
dispatch the second. The ability-source shape (`Triggered`/`Activated`, 2 real lines, "Hexproof from triggered/activated
abilities") is refused rather than resolved: Java's own branch would synthesize `ValidSA$` for these
(`getTypeDescription().contains("abilities")`), and `Matches` never evaluates a `SpellAbility` -- an Aura's own
cast-time targeting is not itself a triggered or activated ability doing the targeting anyway, so refusing produces the
identical observable result here as correctly resolving it would, just for the honest reason rather than an accident of
what `Matches` happens to never match.

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

## Layer 7, Layer 4, Layer 5, Layer 6, Layer 8 and Layer 2: `PT`/`TypeMod`/`ColorMod`/`KeywordMod`/`RulesMod`/`ControlMod`, `Card.Power`/`Toughness`/`Type`/`Colors`/`HasKeyword`/`Controller`, and the first real Continuous callers

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
valid-string match. A non-numeric `AddPower$`/`AddToughness$`/`SetPower$`/`SetToughness$` naming a named SVar now
resolves through `resolveAmount` (amount.go, below) when that SVar's own body is one of the shapes it evaluates; per
missing dimension when it is not, not per whole line.

**`compile.Face.Amounts`, `resolveAmount` and Layer 7a (`CharacteristicDefining$`).** `AbilityUtils.calculateAmount`
itself is 300-some lines dispatching on eighty-some expression heads — not a port this slice attempts whole — but one
family of it, `Count$Valid[<Zone>[,<Zone>...]] <spec>` (`CardLists.getValidCardCount` against a zone,
`AbilityUtils.xCount`'s own "count valid cards on the battlefield"/"count valid cards in any specified zone/s"
branches), is exactly what `Matches` (valid.go) and this port's own `Zone` (zone.go) already evaluate everything else
with — 2,804 of the corpus's 6,186 real `Count$` expressions (45%), 2,537 of those with no operator suffix, is worth its
own evaluator slice for that reason alone.

The compiler carries the raw text a `SVar:X:Count$...` line holds; nothing before this needed to read one that was not
itself an ability (`compile.Ability`'s own `Subs`/`SubRef` mechanism resolves an ability-shaped SVar,
`Execute$ TrigDraw` and the like, but a plain-value reference like `AddPower$ X` left `"X"` as inert text with no
connection to its own SVar body). `compileAmounts` (compile.go) closes that: every SVar a face defines that is NOT
itself an ability — its own head, up to the first `$`, does not fold to a `recordKeys` entry
(`DB`/`AB`/`SP`/`ST`/`RE`/`Mode`/`Event`) — is parsed once, via `internal/expr.Parse`, into
`Face.Amounts map[string]expr.Amount`, keyed by name folded to lower case (`SVars.Get`'s own case-insensitive contract).
`expr.Parse` never fails (its own doc comment), so every non-ability SVar gets an entry even when its own body is not a
shape `resolveAmount` can use yet — the identical "record what recognizing it needs, evaluator decides whether it can"
split `Ability.Params` already has. This does not touch `WriteCanonical`/`Fingerprint` (canonical.go) at all — the
golden AST walks only `Abilities`/`Triggers`/`Statics`/`Replacements` — so `TestCorpusAST` needed no regeneration.

`resolveAmount`/`resolveAmountDepth` (amount.go, `internal/engine`) evaluate an `expr.Amount` to an int: a `Literal`
resolves directly (its own `Value` already carries the sign); a `Reference` looks its name up in `amounts` and resolves
THAT in turn, one level of indirection at a time (`maxAmountDepth`, 4, bounds the recursion — defensive, not a real
corpus need, since the one real chain this port's own research found, Roiling Horror's `Y -> Z`, carries an operator at
every hop and is refused before it would ever recurse); an `Expression` resolves only when its outer Head is `"Count"`,
`Op` is nil (no operator suffix — Roiling Horror's own shape, and 267 of the corpus's 2,804 Valid-family lines, stay
unresolved for exactly this reason), and the inner `Count$` head (`expr.ParseCount`) is one of the "Valid" family
(`expr.IsValidHead`). `validCountZones` maps the head's own zone suffix (empty is Battlefield, Java's own default;
`,`-joined for the 40-some real lines naming more than one zone) to this port's own `ZoneType` values, and `countValid`
counts every card in every one of those zones, every player's own, `Matches` accepts — the identical
`sourceController`/`source` pairing (a static ability's own host controller/id) every other valid-string check in this
port already passes. Single-zone coverage this closes: bare `Valid` (1,973 of 2,804), `ValidGraveyard` (471),
`ValidHand` (253), `ValidLibrary` (63), `ValidExile` (43) — effectively the whole family bar `ValidAll`/`ValidSelf` (8
real lines, neither an actual zone name), the operator-carrying 267, and a dozen more where the argument itself carries
a `$`-suffixed distinct-value operator (`expr.Count.DistinctProperty`, below).

A `Count$Valid<Zone> <spec>` argument can itself carry a further `$`: Tarmogoyf's own toughness SVar is
`Count$ValidGraveyard Card$CardTypes`. `xCount` (`AbilityUtils.java`) cuts the whole "head argument" string on the FIRST
`$` (`paidparts = l[0].split("\\$", 2)`) before it ever reaches `CardLists.getValidCards` — so the actual valid string
passed to it is only `Card` (Forge's own universal base, matching every object), and `CardTypes` is a completely
separate operator (`handlePaid`, `AbilityUtils.java:3719`, `countCardTypesFromList`): count the DISTINCT card types
among whatever `Card` matched, not the matches themselves. `expr.ParseCount` now splits this apart into a new
`DistinctProperty` field rather than feeding `Card$CardTypes` whole into `valid.Parse` as one base name — which would
have parsed without error (`valid.Parse` never fails, its own doc comment) into a single-alternative spec with base name
`"Card$CardTypes"`, an unrecognized base that itself matches nothing, but count.Valid would then hold `Card` correctly
once split, which DOES match every real object — silently returning a real but WRONG number (a plain match count, not a
distinct-type count) rather than failing to resolve at all. `resolveAmount` (`amount.go`) checks
`DistinctProperty != ""` and refuses the whole expression (GO-7) rather than resolve `Valid` alone. Five distinct
operators exist in the real corpus (`CardTypes`, 7; `DifferentCardPower`, 2; `GreatestCardPower`,
`GreatestCardManaCost`, `CreatureType`, 1 each — a dozen real lines total), each its own separate Java function; too
little value to build five more evaluators for, so none of them resolve.

This was caught after Batch B had already shipped and merged, not during it: writing
`TestApplyContinuousCharacteristicDefiningSkipsDistinctPropertyCount` (continuous_test.go) — a Tarmogoyf-shaped CDA, one
matching graveyard card — found `Power()` resolving to `(1, true)` (one card matches the universal `Card` base) rather
than failing to resolve, tracing back to `Count$ValidGraveyard Card$CardTypes` never having been checked against a real
corpus SVar shaped exactly like this before. `TestParseCountDistinctProperty` (`internal/expr`) is the narrower
unit-level proof.

`ptParam` (continuous.go) is `resolveAmount`'s own real caller: a plain integer resolves exactly as it always did, and
failing that the value is looked up in `amounts` by name and handed to `resolveAmount`.

`applyOneContinuousPT` now branches on `CharacteristicDefining$` before its own general
Affected$-matched
path. `applyOneCharacteristicDefiningPT` reads only `SetPower$`/`SetToughness$`. A CDA always SETS the
base value it defines (CR 613.3) and never adds to one.

No real corpus line pairs `CharacteristicDefining$` with `AddPower$` or `AddToughness$`.

The result applies to the host card alone, at `LayerCharacteristic` (7a). `StaticAbilityContinuous.getAffectedCards`'s
own CharacteristicDefining branch hardcodes the affected set to a collection holding only the host card, regardless of
any `Affected$` a real corpus line also happens to carry (revenant.txt's own redundant `Affected$ Card.Self`) — so no
`Affected$` param is read for this shape at all.

`ExcludeZone$` is not resolved: one real line, among 264 `CharacteristicDefining$ True` cards, is not a shape worth a
separate zone check for, and `applyContinuousPT`'s own battlefield-only walk already means host is on the one zone this
port could check anyway.

`LayerCharacteristic` itself needed no new folding work. `PTEffect`'s own Layer/HasPower/HasToughness machinery (this
section's own earlier paragraphs) had already carried it as a first-class layer since M5's own Layer 7b/7c work landed,
unused as a real caller until now.

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
already clear there: a continuous effect that only applied on the battlefield does not survive the trip. That clear was
redundant with `applyContinuousPT`'s own rebuild-from-scratch for anything `Mode$ Continuous` produces (a card that has
left is not on the battlefield to be walked as a host or an affected card on the next pass either way) until an
event-driven "+3/+3 until end of turn" pump actually existed: `pumpEffect`
(`## M6's fifth effect: Pump, and duration tracking`, below) is not itself a `Mode$ Continuous` static, so its own
record (`Game.pumps`) needs `Move`'s own clear to drop it the moment its target leaves the battlefield too —
`clearPumps`, called from the identical branch.

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

**Layer 6, `applyContinuousKeyword` (`continuous.go`), and `KeywordMod`.** `AddKeyword$` is the single largest real
slice of all four layers this port resolves (1,556 of 1,857 real lines, ahead even of Layer 7's own 2,192 of 2,426) --
an equipment or Aura granting Flying/Trample/Menace/Ward, the single most common continuous shape in the whole corpus.
`KeywordMod`/`KeywordEffect` (`keywordmod.go`) are simpler than `TypeMod`/`ColorMod`: `HasKeyword` (card.go) only ever
asks membership ("is this keyword present"), never "what is the current value" the way `Power`/`Type`/`Colors` do, so
there is no fold order to resolve at all -- two continuous effects both granting a keyword never disagree about
anything, so `KeywordEffect` carries no `Layer`/`Overwrite` distinction, just `Timestamp` (unused today, kept for the
same reason `TypeEffect`/`ColorEffect` keep theirs: a future remove direction will need it) and `AddKeywords []string`,
each entry a whole keyword line verbatim -- exactly what a real `K:` line would carry ("Ward:2", "First Strike",
"Protection:..."), so `HasKeyword`'s own `keyword.Parse(line).Name` reads a granted one the identical way it already
reads a printed one. `HasKeyword` gaining this fold reaches every existing call site for free: `cantBlockByKeywords`
(staticability.go) and combat's own First Strike/Double Strike/Deathtouch/Trample reads (`dealsInStep`,
`dealAttackerDamage`, combatdamage.go) all start seeing a continuously-granted keyword without changing a line of their
own code --`TestApplyContinuousKeywordGrantedFlyingAffectsCanBlock` (continuous_test.go) proves the reach past a bare
`HasKeyword` check into real block legality.

`keywordTokens` (continuous.go) splits `AddKeyword$` on `" & "` the identical way `typeTokens`/`colorTokens` do, and
skips the whole line -- not just the bad token -- the instant a dynamic-value marker (`StaticAbilityContinuous.java`'s
own `removeIf` lambda: `ChosenColor`, `ChosenType`, `ChosenNumber`, `ChosenPlayer`, `ChosenName`, `ChosenEvenOdd`,
`AllColors`/`allColors`, `CommanderColorID`, `ColorsYouCtrl`/`colorsYouCtrl`, `YourBasic` -- 42 of 1,857 real lines)
appears as a SUBSTRING anywhere within any one token, checked with `strings.Contains` rather than exact-token equality:
a real corpus token often embeds the marker as a qualifier inside a larger one
(`"Protection:Card.ChosenColor:chosenColor"` is one token, not "ChosenColor" standing alone), the identical reason
Java's own check is `input.contains(...)`, not `input.equals(...)`. `RemoveKeyword$`/ `RemoveAllAbilities$` (5 of 1,561
real `AddKeyword$` lines carrying no dynamic marker) and `SharedKeywords$`/ `FromDraftNotes$` (a
game-wide/remembered-list/draft-note keyword source rather than a fixed token list) each skip the whole line too, at the
outer `applyOneContinuousKeyword` level rather than inside `keywordTokens`, since they are static-ability PARAMS, not
tokens inside the `AddKeyword$` value itself -- applying the add half of a real "gains X, loses Y" line without the
remove half `applyOneContinuousType`'s own "becomes a Turtle" paragraph already explains why not to.

**Layer 8, `applyContinuousRules`, and `RulesMod` -- this port's first player-facing continuous effect.** Every layer
above lives on `Card`; `RulesMod`/`RulesEffect` (rulesmod.go) live on `Player` instead, matched through `Affected$`
against `matchesPlayerSpec` (valid.go) rather than `Matches`.

`StaticAbilityContinuous.getAffectedPlayers`'s own `Player.isValid` call is the identical `Affected$`-splits-then-
`Player.isValid`-each-candidate shape `getAffectedCards` already has for cards, just never needed a player-shaped target
before this.

`SetMaxHandSize$` (43 real lines) and `RaiseMaxHandSize$` (8) fold onto `HandSizeLimit` (player.go). A `SetMaxHandSize$`
effect REPLACES the running limit, and Java's own `unlimitedHandSize` flag with it -- `p.setUnlimitedHandSize(false)`
runs inside the same branch as `p.setMaxHandSize(max)`, so an ordinary numeric line always clears a previous
`Unlimited`. A `RaiseMaxHandSize$` effect ADDS to it instead. Both fold in `Timestamp` order -- `foldPT`'s own combine
convention (card.go), reused for the one other layer this port's own effects can conflict within. Java's own iteration
order for two `SetMaxHandSize$` effects active on the same player at once is not pinned down anywhere citable, so
`Timestamp` is this port's own deliberate choice here, not a rediscovery of Java's.

`AdjustLandPlays$` (27) folds onto `LandPlayLimit` (player.go) more simply: `Player.getMaxLandPlays`'s own unconditional
sum (every adjustment counts, regardless of order) and `Player.getMaxLandPlaysInfinite`'s own "any one active effect
makes it unlimited" OR. Neither has a "replace" form, so nothing here is order-dependent the way hand size is.

`"Unlimited"` (Java's own literal sentinel for `setUnlimitedHandSize(true)`/`addMaxLandPlaysInfinite`) is checked before
falling to the existing `ptParam` for the numeric case: `ptParam` itself would just report `"Unlimited"` unresolvable
(neither a plain integer nor an SVar name), correct on its own terms, but `rulesEffect` (continuous.go) needs to tell
"no maximum" apart from "could not resolve this" before it ever calls `ptParam` at all.

`HandSizeLimit`/`LandPlayLimit` both take the printed default as a parameter (`turn.go`'s own `MaxHandSize`, `land.go`'s
own `maxLandPlays`) rather than reading either constant directly. `player`'s own `enginelint` group would otherwise need
`turn` and `land` in its allow-list, and `turn`/`land` already allow `player` -- a real cycle, not the one-way
dependency every other cross-group reference in this port has been. `cleanupStep` (turn.go) and `PlayLand` (land.go) --
both of which already had a doc comment naming this exact gap, written well before this landed -- now call them instead
of comparing against the bare constants.

75 of the corpus's 78 real `SetMaxHandSize$`/`RaiseMaxHandSize$`/`AdjustLandPlays$` lines resolve. 2 carry a qualified
`Affected$` value `matchesPlayerSpec` cannot resolve (`Player.NotedForGreenAnchor`, `Player.Chosen`). 1 more --
Delirium's own "each opponent's maximum hand size is seven minus..." -- carries `Condition$`, which no static-ability
mode this port checks has an evaluator for.

Not attempted here: `MayLookAt$`/`MayPlay$` (88/181 real lines corpus-wide, the layer's own largest real params by far)
need a cast-time zone-eligibility permission `CastSpell`'s own hand-only check (castspell.go) has nowhere to consult
yet. `AddHiddenKeyword$` (19) has 8 real distinct values ("must be blocked if able," "can't attack alone," "doesn't
untap," ...), each its own separate block/attack/untap-step rule this port's combat/turn model has no hook for, none
sharing enough machinery to be worth building as one slice the way `SetMaxHandSize$`/`AdjustLandPlays$` did.
`ControlOpponentsSearchingLibrary$`, `ControlVote$`, `AdditionalVote$`, `AdditionalOptionalVote$`,
`AdditionalVillainousChoice$`, `DeclaresAttackers$` and `DeclaresBlockers$` (0-3 real lines each) are multiplayer/vote
mechanics this port has no concept of at all.

`TestApplyContinuousRulesSetsUnlimitedHandSize`, `TestApplyContinuousRulesSetsFixedHandSize`,
`TestApplyContinuousRulesRaisesHandSize`, `TestApplyContinuousRulesAdjustsLandPlays`,
`TestApplyContinuousRulesGrantsUnlimitedLandPlays`, `TestApplyContinuousRulesAffectedOpponentSkipsTheHostsOwnController`
and `TestApplyContinuousRulesSkipsLineWithCondition` (continuous_test.go) prove the fold mechanism directly;
`TestCleanupDoesNotDiscardWithUnlimitedHandSize` (turn_test.go) and
`TestPlayLandSucceedsPastTheDefaultLimitWithAdjustLandPlays`/`TestPlayLandSucceedsRepeatedlyWithUnlimitedLandPlays`
(land_test.go) prove the two real callers actually read it. The first attempt at the land-play tests used a
single-player game, `TestPlayLandMovesCardToBattlefieldAndCountsIt`'s own precedent -- and failed, because
`CheckStateBasedActions`'s own win-condition check (`remaining == 1` --> `Won = true`, `action.go`) returns before
`applyContinuousPT`/.../`applyContinuousRules` ever run in a one-player game, something every earlier single-player
`land_test.go` case had simply never called `CheckStateBasedActions` at all to notice. A real, self-caught ordering bug
in the test, not the implementation -- fixed by using a two-player game with both players' `Life` set, this section's
own established convention for any test that calls `CheckStateBasedActions` directly.

**Layer 2, `applyContinuousControl`, and `ControlMod` -- this port's first controller-change mechanism.**
`Card.Controller` stops being a plain field, set once in `NewCard` and never mutated anywhere else this port had built
until now, and becomes `Card.Controller()` (card.go): a fold over a new `ControlMod`/`ControlEffect` (controlmod.go) the
identical "highest `Timestamp` wins" shape `Card.Power`/`Toughness` already use for `PT`.

Ported from `Card.java`'s own `tempControllers` (a `NavigableMap<Long, Player>`) and `getController()`: the
highest-timestamp entry wins, if any exist, else the base falls through. Java's own `getController()` carries an extra
guard -- a temp-controller only wins if its own timestamp beats `controllerTimestamp`, the stamp on Java's own
`setController` (an explicit "gain control permanently" one-shot effect, `## Not ported yet`, below) -- which collapses
away here: this port builds no equivalent of `setController` yet, so there is no base-controller timestamp for a
temp-controller to ever lose to, and any `ControlEffect` unconditionally outranks the base.

`applyContinuousControl`/`applyOneContinuousControl` (continuous.go) resolve `GainControl$ You`:
`StaticAbilityContinuous.java`'s own CONTROL branch calls
`AbilityUtils.getDefinedPlayers(hostCard, params.get("GainControl"), stAb).get(0)`, a "defined player" lookup rather
than the `Affected$`-for-players membership test `RulesMod`'s own dispatch is, since `GainControl$` names WHO gains
control instead of describing a set to test candidates against -- `getDefinedPlayers`'s own `"You"` case is
`players.add(player)`, and `player` is `card.getController()` whenever `sa` is not a `SpellAbility` (every real
`Mode$ Continuous` static here), i.e. the effect's own host, `host.Controller()` below.

Corpus-frequency research here first had to separate two unrelated mechanics sharing one param name: a naive
`grep -rn "GainControl\$"` finds 58 real lines, but only 44 of them are `S:Mode$ Continuous` lines -- the other 14 are
`DB$ ChangeZone | ... | GainControl$ True` (Restoration Angel, Rise from the Grave) or
`DB$ Dig | ... | GainControl$ True`, `ChangeZoneEffect`'s own one-shot "put onto the battlefield under your control"
effect, entirely unrelated to this layer and part of M6's own remaining 202 script effects. Of the 44 real
`Mode$ Continuous` lines, 43 name `GainControl$ You`; the last, `GainControl$ Player.isMonarch`, is a qualified
`getDefinedPlayers` form (the `else` branch's own `game.getPlayersInTurnOrder()` filtered by
`PlayerPredicates.restriction`) this port has no monarch mechanic to filter by, so the whole line is skipped
(PORT-8/GO-7) rather than guessing "the controller" and being wrong the instant any game actually changes hands.
`Affected$` on the 44 real lines is overwhelmingly `Card.EnchantedBy`/`Permanent.EnchantedBy`/`Creature.EnchantedBy` (42
of 44, Control Magic's own shape -- the Aura's own host), needing nothing new: the identical `Matches`-driven
valid-string match `applyOneContinuousPT`'s own `Affected$` already does.

`applyContinuousControl` runs FIRST among the six appliers (`CheckStateBasedActions`, action.go), ahead of
`applyContinuousPT`/`Type`/`Color`/`Keyword`/`Rules`: CR 613.1 puts the control layer before every one of them, and
concretely, several of their own `Affected$` specs can themselves read `Controller()` (a `"YouCtrl"` property) -- a
stale value there would evaluate an anthem's own `Affected$ Creature.YouCtrl` against last pass's controller, not this
one's, the moment a `GainControl$` effect and an anthem effect are both in play at once.
`TestApplyContinuousControlRunsBeforeKeywordSoYouCtrlSeesTheNewController` (continuous_test.go) proves the ordering
directly, not just the individual fold.

Converting `Card.Controller` from a field to a method meant converting every read of it, roughly eighty call sites
across `staticability.go`, `valid.go`, `manaability.go`, `draweffect.go` (an `Ability.Controller` read, a different
field on a different type, left alone), `action.go`, `stack.go` (also `Ability.Controller`, left alone), `land.go`,
`trigger.go` (both `Card.Controller` reads AND `Ability.Controller` reads share the file; only the former needed
converting -- `pushTriggeredAbilities`'s own `a.Controller` is the pushed ability's controller, not a card's),
`combatdamage.go`, `continuous.go`, `attack.go`, `castspell.go`, plus `internal/fixture`'s own `dump.go`/`load_test.go`
(a different package, needing the same field-to-method rename since it reads a card's controller for its own dump/load
round-trip check). Two engine tests (`action_test.go`) that directly wrote `g.Card(x).Controller = b` to fake "someone
already controls this" for `hostRefusesEnchant`/`cleanupDanglingAttachments` coverage could not keep doing that: a
direct field write has nothing left to write to, and even if it did, `applyContinuousControl`'s own
clear-and-rebuild-every-pass contract would wipe it the instant `CheckStateBasedActions` runs. Both now build a real
`GainControl$ You | Affected$ Card.IsRemembered` permanent and `Memory.Remember` the target instead -- the real
mechanism exercising the exact behavior the test wants, rather than a field poke standing in for it.

`ControlEffect`'s own field is named `Controller`, not `Player`: `enginelint` walks every identifier in the package,
including a struct field's own name and a struct-literal key, and cannot tell a field named `Player` apart from a
reference to the `Player` type itself (`declaredIn["Player"]` resolves to `player.go` either way) -- `Player` would have
been flagged as `parts` illegally referencing `player`, a real false positive in the tool's own identifier scan, not a
real dependency (`topLevelNames`, tools/enginelint/lint.go, already excludes methods from this map for the same reason;
a struct field is not a method, so it is not excluded). `Controller` avoids the collision outright: nothing top-level in
the package is named that (only the _method_ `Card.Controller()` is, and methods are excluded).

`TestApplyContinuousControlGrantsControlOfEnchantedCreature`,
`TestApplyContinuousControlLeavesUnenchantedCreaturesAlone`,
`TestApplyContinuousControlSkipsUnresolvedGainControlValue`, `TestApplyContinuousControlSkipsLineWithCondition` and
`TestApplyContinuousControlRunsBeforeKeywordSoYouCtrlSeesTheNewController` (continuous_test.go) prove the fold mechanism
directly; `TestCheckStateBasedActionsAuraGoesToOwnersGraveyard` and
`TestCheckStateBasedActionsAuraGoesToGraveyardWhenEnchantPropertyStopsMatching` (action_test.go, rewritten as above)
prove a real caller reads it.

Not resolved: the qualified `GainControl$ Player.isMonarch` (1 of 44, above). Layer 1 (copy effects) and Layer 3
(`GainTextOf$`) stay untouched -- `## Not ported yet`, below, has the reasons.

### `Condition$`: the one gate all six appliers share

Every one of the six appliers above (`applyOneContinuousPT`/`Type`/`Color`/`Keyword`/`Rules`/`Control`) used to skip a
line outright the instant it carried a `Condition$` param at all, each one's own doc comment naming this the same
missing piece. `continuousConditionMet` (continuous.go) closes most of it: `StaticAbility.checkConditions`'s own
`Condition$` switch (StaticAbility.java), called from all six in place of the blanket skip, evaluated fresh every
`CheckStateBasedActions` pass the same as everything else Layer 4-8/2 fold, since CR 613 gives a continuous effect no
memory of its own last evaluation.

The corpus's own 317 real `S:Mode$ Continuous | Condition$` lines, tallied directly off `S:` lines carrying
`Mode$ Continuous` (a plain corpus grep for bare `Condition$` also catches values on `T:`/`A:` lines Trigger.java's own
`meetsRequirementsOnTriggeredObjects` and SpellAbilityCondition.java handle separately -- a different switch on the same
param name, not this port's problem here):

| Value           | Real lines | Resolved | Player state read                                                             |
| --------------- | ---------: | :------: | ----------------------------------------------------------------------------- |
| `PlayerTurn`    |        141 |   yes    | `Game.ActivePlayer() == host.Controller()`                                    |
| `Threshold`     |         61 |   yes    | `len(Zone(Graveyard, controller).Cards()) >= 7` (`Player.hasThreshold`)       |
| `MaxSpeed`      |         40 |    no    | Alchemy's own speed counter -- tracked nowhere in this port                   |
| `Delirium`      |         23 |   yes    | four-plus distinct core types unioned across the graveyard (below)            |
| `Metalcraft`    |         18 |   yes    | three-plus battlefield permanents whose `Type()` carries Artifact             |
| `Blessing`      |          9 |    no    | City's Blessing (ten-plus permanents, sticky) -- no such flag on `Player` yet |
| `NotPlayerTurn` |          8 |   yes    | the inverse of `PlayerTurn`                                                   |
| `Hellbent`      |          8 |   yes    | `len(Zone(Hand, controller).Cards()) == 0` (`Player.hasHellbent`)             |
| `EnduringStory` |          4 |    no    | a Saga's own lore-counter/chapter state -- Sagas are not ported               |
| `FatefulHour`   |          3 |   yes    | `Player.Life <= 5`                                                            |
| `Monarch`       |          2 |    no    | no monarch mechanic (same gap Layer 2's own qualified value has, above)       |

262 of 317 resolve. The four that do not (55 lines) are each its own untracked mechanic, so (the identical "cannot
evaluate, so do not apply" rule an unresolved `Affected$` value already has, GO-7) the whole line is skipped, same as
before this slice existed -- `continuousConditionMet`'s own `default` case, which also catches any value the real corpus
does not carry today.

`Delirium` is `AbilityUtils.countCardTypesFromList(graveyard, false)` (`graveyardCoreTypeCount`): every graveyard card's
own _current_ (Layer-4-folded) `Type()` unioned into one running `cardtype.Line` via `Union` -- reusing item 27's own
type-folding machinery rather than re-deriving a card's type from its printed face -- then `len(.CoreTypes())` against
4, core types only (not supertypes, not subtypes), matching `CardType.CoreType`'s own enum exactly. `Metalcraft` is
`battlefieldArtifactCount`: the same `Type()` read, `.Has(cardtype.Artifact)`, over the controller's own battlefield.

One real line resolves its `Condition$` but still does not apply for an unrelated reason: Winter, Misanthropic Guide's
`Condition$ Delirium | Affected$ Opponent | SetMaxHandSize$ Y` (Layer 8) now passes its own `Condition$` check, but `Y`
is `Number$7/Minus.X` and `X` is `Count$ValidGraveyard Card.YouOwn$CardTypes` -- a `DistinctProperty` expression
`resolveAmount` (amount.go, item 27's own Tarmogoyf-shaped gap) already skips, feeding an arithmetic
`Number$.../Minus.X` SVar shape this port's amount resolution has no head for either way -- `rulesEffect` itself still
returns not-ok, so the line still does not apply, correctly, just no longer for the reason its own name once suggested.

`TestApplyContinuousPTSkipsConditionParam` moved from `Condition$ PlayerTurn` (now resolvable, and coincidentally false
in a fresh test game with no active player set -- `NoPlayer` matches no real `PlayerID`) to `Condition$ MaxSpeed` to
keep proving what its name claims: an unresolvable value skips the line regardless of game state.
`TestApplyContinuousRulesSkipsLineWithCondition`/`TestApplyContinuousControlSkipsLineWithCondition` moved the same way.
Ten new tests (`TestApplyContinuousPTAppliesWhen*ConditionMet`/`SkipsWhen*ConditionNotMet`, continuous_test.go) prove
each resolvable value both ways, driven through `g.StartTurn` (`PlayerTurn`) or a matching zone/type/life setup
(`Threshold`/`Hellbent`/`Metalcraft`/`Delirium`/`FatefulHour`) -- `Rules`/`Control` reuse the identical shared function,
so are not re-proven per value there.

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
permanent in the game (not just the active player's, unlike `untapStep`) is the only piece built here; discarding to
hand size (CR 514.1, `Player.HandSizeLimit`) and ending "until end of turn" effects (514.2's other half, `Game.pumps`)
land in later chunks (`## M6's fifth effect: Pump, and duration tracking`, below, has the second).

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

Java's own `handleLegendRule` carries two more corner cases past the ordinary same-name grouping above, its own names
for them (`GameAction.java`'s own comments, "Corner Case 1"/"Corner Case 2"). Corner Case 2 is built now; Corner Case 1
is not.

**Corner Case 2**: a permanent whose own `Card.hasNonLegendaryCreatureNames()` is true — Spy Kit's own real "has all
names of nonlegendary creature cards in addition to its name" — answers to every non-legendary creature's own printed
name, including every OTHER such permanent's, so two or more of them clash with each other even when their own printed
names differ. `Card.HasNonLegendaryCreatureNames` (card.go) is the port's own version of that flag — a plain bool, not
folded by timestamp the way `TypeMod`/`ColorMod`/`KeywordMod` are, since nothing else ever needs to know more than one
source's own contribution at once (two Spy Kits on the identical creature answer the identical "yes" a single one
would). `applyContinuousNames` (continuous.go) is the new Layer 3 continuous applier that sets it, recomputed fresh
every `CheckStateBasedActions` pass the same as every other layer's own applier — resolving the one real value
`AddNames$` takes corpus-wide, `AllNonLegendaryCreatureNames` (1 real line, Spy Kit's own).

Spy Kit's own real shape needed one thing no other applier's own `Affected$` dispatch had needed yet:
`AffectedDefined$ Equipped` — "the creature this Equipment currently equips," not a blanket battlefield-wide `Affected$`
match. Every other layer's own applier (`applyOneContinuousPT`/`Type`/`Color`/`Keyword`) refuses outright the instant
`AffectedDefined$` is present (0 real lines pair it with any of their own keys, their own doc comments' skip-lists), a
convention this applier could not reuse without making itself dead code against the one real corpus line it exists for.
`Card.AttachedTo()` (card.go) already answers the question directly — "what this card [the Equipment] is attached to" is
exactly Java's own `AbilityUtils.getDefinedCards(hostCard, "Equipped", ...)`, since this port already models Equipment
attachment the identical way an Aura's is (`Game.Attach`/`Unattach`, `## Handles, not pointers`) — `Affected$`'s own
valid-string still filters that single card afterward, the identical two-step Java's own `getAffectedCards` does
(`AffectedDefined$` first, `Affected$` second, `StaticAbilityContinuous.java`).

`resolveLegendRule` (action.go) reads the flag after the ordinary name-grouping above has already run: every legendary
permanent carrying it, that the name-grouping has not already sent to its own owner's graveyard, is gathered into one
more group and resolved the identical way a same-name duplicate is (`ChooseLegendaryToKeep`, reused) — skipping anything
the name-grouping already removed is what keeps two same-named, both-flagged legendaries from being asked about twice.

**Corner Case 1** stays unbuilt: whether a Corner-Case-2 permanent's own borrowed names collide with some OTHER
legendary's own literal printed name (`StaticData.instance().getCommonCards().isNonLegendaryCreatureName`,
`GameAction.java`) needs a lookup across every creature card this game has ever printed, not just what is on this
battlefield today — a card-name-to-type index over the WHOLE corpus, not the handful of cards any one game ever touches.
This port's `*Game` holds no `*carddb.DB` reference to ask (GO-2's own guidance to inject one has never actually been
done for the engine package), and adding one now, only for this, would mean threading it through every `*Game`
constructor across the entire test suite — a disproportionately large refactor for the one corpus card (Spy Kit) it
would unlock, the identical "narrow correct slice over a large risky one" call the Registry-avoidance chunks
(`## Draw's own ReplaceWith$`) already made for a different wall (PORT-8).

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

`GameAction.changeZone` (2,897 LOC, most of it replacement effects and triggers this port has not reached) is not
ported. Two pieces of it are: the part that exists only because Go's cards do not work the way Java's do, and — since
"## Last-known-information lands," below — the one slice of `CardCopyService.getLKICopy()`'s own last-known-information
bookkeeping the real corpus's own dies triggers actually read.

Java rebuilds a `Card` as a new object on every zone change (`CardCopyService.copyCard`), so a field the new object does
not carry — tapped, damage, counters, summoning sickness — is simply gone, free of charge. ADR-0009 chose the opposite:
a `CardID` is stable for the card's whole life in the game, so the same struct that was tapped on the battlefield is
still tapped after `Move` if nothing clears it. `Move` now does that clearing explicitly: leaving the battlefield
freezes an LKI copy of the card (below) before clearing `Counters`, `Damage`, `PT`, `TypeMod`, `ColorMod`, `KeywordMod`,
`Tapped` and the card's own attachment; entering it sets `SummonSick`, since a freshly-arrived permanent has not been
under its controller's control since their last turn began (CR 302.6).

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
choose," plus APNAP order between different players' own triggers) is ported as `pushTriggeredAbilities` (trigger.go):
every one of the ten trigger-check functions collects its own matches into a `[]Ability` first (its "own" and "other"
halves feeding the same slice, where it has both) and calls `pushTriggeredAbilities` once at the end, rather than
calling `PushAbility` the instant each match is found. `pushTriggeredAbilities` walks `playersInAPNAPOrder` (trigger.go
— `Game.ActivePlayer()` first, then `nextPlayerAfter`, turn.go's own seating-order/skip-a-lost-player rule, until every
seated player has appeared once) and pushes each player's whole group in that order. Because the stack is LIFO and
`MagicStack.addAllTriggeredAbilitiesToStack` itself pushes the active player's group first (so it ends up on the
bottom), the group pushed LAST — the player immediately before the active player in turn order — is the one on top,
resolving FIRST; the active player's own group resolves last, ported faithfully from
`MagicStack.chooseOrderOfSimultaneousStackEntry`'s own player-iteration order. `Player.orderAndPlaySimultaneousSa`
itself — a player choosing the order among more than one of their OWN simultaneous triggers — is not ported: this port
has no `PlayerController` method for that choice (`## Controller`'s own "90 of 110 methods" gap), so a single player's
own multiple matches stay in the deterministic order the caller found them (GO-12), the same simplification every other
real-choice gap already makes. This was caught and fixed via `TestPlayLandPushesETBTriggersInAPNAPOrder`
(trigger*test.go): before `pushTriggeredAbilities`, `otherETBTriggerMatches` meant one card entering the battlefield
could push both its own trigger and another permanent's own trigger watching for it, off the same event, in a fixed
order (the entered card's own trigger first, then every other battlefield permanent's own, in `Players()`/zone-iteration
order) rather than CR 603.3b's APNAP one — every real fixture and test before this had at most one trigger fire per
event, so the wrong order was never actually exercised until this test built one on purpose.
`freezeStack`/`unfreezeStack`, though, exist to serve a second ability arriving on top of one still resolving -- and
that much can happen now: `checkETBTriggers`/`checkDiesTriggers` (trigger.go) push from inside
`permanentEffect`/`attachEffect`/`PlayLand`/every graveyard-bound SBA's own resolution, so `ResolveStack`'s loop (below)
finds a second entry on top the moment the first one finishes, not from a second caller racing the first. Nothing needs
freezing because nothing is interactive: this port has no `PlayerController` method that lets anyone respond between one
ability resolving and the next being found on top (`## Controller`), so there is never a window for a \_third* ability
to arrive while the second is still open either. `effect.go`'s `Registry` no longer holds zero implementations --
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

**Flying/reach, Fear, Horsemanship, Intimidate, Landwalk, Protection, Skulk, Menace, and every real
`S:Mode$ CantBlockBy` line are checked.** `Game.DeclareCombatBlockers`'s own eligibility computation is still only
"untapped creature the defending player controls" — `CantBlockBy` is a property of one attacker/blocker pair, not of a
creature in isolation, so it is checked afterward instead, via `CanBlock`, against the controller's own answer
(`## Block legality: CantBlockBy` has the full account). Menace is a group-cardinality check (`menaceLegal`) rather than
a `CanBlock` one, for the same reason Forge itself does not run it through the static-ability engine either.

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

## Trigger firing: entering, dying, attacking, blocking, dealing damage, being discarded, becoming tapped, tapping for mana, casting a spell, the beginning of a step or phase, a player attacking, drawing a card, and watching another permanent

`checkETBTriggers` (`trigger.go`) is CR 603 at its narrowest: only `Mode$ ChangesZone` with `Destination$ Battlefield`
fires — a permanent's own "when this enters" trigger. `checkDiesTriggers` is the identical narrowness applied to the
corpus's other frequent `ChangesZone` shape — `Origin$ Battlefield`, `Destination$ Graveyard`, CR 700.4's "dies" —
checked only against the dying card's own `Card.Self` triggers. `isETBTrigger`/`isDiesTrigger` factor the shared
`Mode$ ChangesZone` + zone-key check both need, on top of `hasZone`, and both port `TriggerChangesZone.performTest`'s
own `Origin$`/`Destination$` semantics exactly rather than the narrower literal-only match they started with:
`hasZoneOrAny` treats a key that is absent, or present naming the literal value `"Any"`, as no restriction at all
(Java's own `!hasParam(key)` and `getParam(key).equals("Any")`), falling back to `hasZone`'s membership check only when
the param names something else. This closed two real gaps at once, not a hypothetical cleanup: `isDiesTrigger`'s own
`Destination$` used to require the literal value `"Graveyard"`, so 253 real `Destination$ Any` lines and 11 more with no
`Destination$` at all — CR 603.6c's own unqualified "leaves the battlefield" — never fired even on an ordinary death;
its `Origin$` used to require the literal value `"Battlefield"`, so 31 real lines naming only `Destination$ Graveyard`
("put into a graveyard from anywhere") missed the battlefield-origin instance of themselves too. `isETBTrigger` gained a
real `origin ZoneType` parameter for the identical reason on its own `Origin$` side — 21 real lines (12
`Origin$ Graveyard`, a reanimation-flavored "enters from a graveyard," plus a handful of
`Hand`/`Stack`/`Exile`/`AttractionDeck`) used to fire unconditionally regardless of where the card actually came from,
an over-firing bug this port had until `checkETBTriggers`/`otherETBTriggerMatches` threaded `origin` through from the
`origin := c.Zone` local already computed at each of the three real call sites (`permanentEffect`/`attachEffect`,
castspell.go; `Game.PlayLand`, land.go — read for `checkMovedReplacement` before `Game.Move` overwrites it).
`changesZoneResolvable` skips a `Mode$ ChangesZone` line naming
`ValidCause$`/`NotThisAbility$`/`ConditionYouCastThisTurn$`/`CheckOnTriggeredCard$`/`ExcludedOrigins$`/
`ExcludedDestinations$` (12 of 7,609 real lines combined) rather than firing unconditionally and guessing wrong
(PORT-8/GO-7) — `TriggerChangesZone.performTest`'s own remaining params this port has no reference vocabulary or
per-turn-cast-count tracking for.

`checkAttacksTriggers` is CR 508.3's own mode, `Mode$ Attacks`, entirely — not a `ChangesZone` shape at all, so it needs
no zone-key check, only `ValidCard` matched against the declared attacker. Ported from `TriggerAttacks.performTest`.
Unlike `checkETBTriggers`/`checkDiesTriggers`, this needs no separate "own" and "other" loop: `TriggerAttacks` itself
never special-cases the attacker's own trigger, so `ValidCard$ Card.Self`/`Creature.Self` (1,282 of 1,606 real corpus
lines — the attacker's own trigger) and `ValidCard$ Creature.YouCtrl` (an anthem-shaped "whenever a creature you control
attacks" watcher) fall out of the identical single walk over every battlefield permanent, just with a different
`ValidCard` string and a different host — one loop where `otherETBTriggerMatches` and `otherDiesTriggerMatches` each
needed a second one.

`Attacked$` (47 real lines) and `FirstAttack$` (4) are resolved now too. `performTest` matches `Attacked$` against
`AbilityKey.Attacked`, a single `GameEntity` (a player, planeswalker or Battle) rather than a `*Card` — resolved through
`attackedTargetMatches` (built for `AttackersDeclared`'s own `AttackedTarget$`, below, which faces the identical
player-shaped/card-shaped mixed-token dispatch problem for a whole collection of attacked entities) passed a one-element
`[]EntityID{g.combat.AttackTargets[attacker]}`, no new dispatch logic needed for the single-entity case. `FirstAttack$`
reads a new `Card.AttacksThisTurn` (card.go) — `CardDamageHistory.getCreatureAttacksThisTurn`'s own per-card counter,
incremented for each declared attacker right before `checkAttacksTriggers` runs (`DeclareCombatAttackers`, attack.go)
and reset every cleanup (`cleanupStep`, turn.go) alongside `Damage`/`LandsPlayed`/`CardsDrawnThisTurn` — checked as
`> 1` (ported directly from `performTest`'s own skip condition) immediately after the increment, so 1 means this is the
first attack this turn. 1,555 of 1,606 real lines carry neither param, unaffected either way.

Resolved: `Alone$` (60) — `attacksOtherCount` counts `Combat.Attackers` other than the declared attacker itself,
`CombatUtil.checkDeclaredAttacker`'s own `AbilityKey.OtherAttackers` (every other attacker declared this combat, not
just ones sharing this one's own defender — every real corpus line reads `Alone$ True`, never `False`);
`DefendingPlayerPoisoned$` (1) — `defenderOf(attacker)` (attack.go) is `AbilityKey.DefendingPlayer`, and
`Counters.Count(Poison)` (counters.go) is `Player.getPoisonCounters()`; `AttackDifferentPlayers$` (1) —
`attacksMultiplePlayers` walks `Combat.Attackers`/`Combat.AttackTargets` the same way `performTest` walks
`AbilityKey.Defenders`, since this port has no separate "defenders actually attacked" list of its own to build. Called
once per declared attacker, after tapping and target assignment both land (`Game.DeclareCombatAttackers`, attack.go) —
`enginelint`'s `attack` group gained `trigger` as a dependency, the same way `castspell`/`land`/`action` already have;
`trigger` itself gained `player` (the `Player` type `g.Player(defenderOf(attacker))` returns, player.go) and `parts`
(`Counters.Count`, counters.go).

`checkBlocksTriggers` is CR 509.2's own "whenever ~ blocks" mode, `Mode$ Blocks`, ported from
`TriggerBlocks.performTest` — the identical one-walk shape `checkAttacksTriggers` already established, since
`TriggerBlocks` never special-cases the blocker's own trigger either: `ValidCard` matched against the declared blocker
covers both "when this blocks" (106 of 127 real lines, `Card.Self`) and "whenever a creature you control blocks" alike.
`ValidBlocked$` (8 of 127 real lines, every one an "or blocks/becomes blocked by one or more X creatures" description)
is checked against `blk.Attacker` directly: `performTest` itself matches it against the FULL collection of attackers one
blocker blocks (`AbilityKey.Attackers`), ANY of which satisfying it fires the trigger once, but `checkBlocksTriggers` is
already called once per declared `Block` (a per-pair granularity, next paragraph), never once per blocker with every
attacker gathered — so checking the one attacker each call already has stands in for "any member of the collection"
correctly for the overwhelming single-attacker case, and no worse than the existing per-pair granularity for the rare
double-block one (a blocker legally blocking two attackers at once now fires once per matching attacker, where Java
fires once total — an existing divergence, not a new one this param introduces). Called once per declared `Block`, from
`DeclareCombatBlockers` (block.go), after `CanBlock` and `menaceLegal` have both already filtered the pairing down to a
legal one — CR 509.2 fires only for a legally declared block, not one either filter already dropped. `enginelint`'s
`trigger` group gained `combat` as a dependency (for `Block` itself); `block` gained `trigger`.

`checkSpellCastTriggers` is CR 603's own "a player casts a spell" mode, `Mode$ SpellCast`, ported from
`TriggerSpellAbilityCastOrCopy.performTest`. Like `checkAttacksTriggers`, one walk over every battlefield permanent
covers both a card's own trigger and another permanent watching for someone to cast a spell — Java's `performTest` never
special-cases the caster's own card either. `ValidCard` is optional here, unlike every other mode this port checks:
`matchesValidParam` (`CardTraitBase.java`) returns true for a missing param, and 100 of 1,435 real corpus lines carry no
`ValidCard` at all ("whenever you cast a spell," no restriction on which one). `ValidActivatingPlayer` is the corpus's
dominant param (1,216 of 1,435 — more common than `ValidCard` itself), matched by a new `matchesActivatingPlayer` rather
than `Matches` (valid.go): a `Player`, not a `Card`. Three bare values cover 1,191 of those 1,216 — `You`
(`activator == hostController`), `Opponent` (`activator != hostController`, the identical no-team simplification
`OppCtrl`/`OppOwn` already carry, `valid.go`'s own doc comment) and `Player` (unrestricted), via `matchesPlayerSpec`
(valid.go's own doc comment has the reason `matchesActivatingPlayer` calls it rather than `matchesPlayerBase` directly).
A qualified form (`Player.Opponent`, `Player.EnchantedBy`, `Player.NonActive`, `Player.Active`, `Player.Other`,
`Player.Chosen` — 25 lines) resolves 19 of those through `matchesPlayerSpec`'s own dotted-property layer:
`Player.Opponent` (12), `Player.NonActive` (4), `Player.Active` (2), `Player.Other` (1) and `Opponent.NonActive` (1).
`Player.EnchantedBy` (5) and `Player.Chosen` (1) stay unresolved — a player-attached Aura and a `ChosenPlayer` memory
slot this port tracks nothing for — the identical "skip rather than fire unconditionally" contract `hasAnyParam` already
gives `checkAttacksTriggers`' own five unresolved params. Also skipped via `hasAnyParam`: `ValidSA`/`ValidSAonCard` (a
`SpellAbility`, not a `Card` — `Matches` cannot evaluate one), `TargetsValid`/`CanTargetOtherCondition` (no per-trigger
target-inspection hook), `HasXManaCost`/`NoColoredMana`/ `SnowSpentForCardsColor` (no mana-payment-detail tracking past
whether the cost was paid), `IsSingleTarget` (no generic target-count reader) and
`ActivatorThisTurnCast`/`ActivatorThisTurnCastEach` (a per-turn cast-history count this port tracks nothing for). 1,163
of 1,435 real lines carry none of these. Fired from both `CastSpell` branches (castspell.go) right where `SpellCast`
(the event) already fires — cast time, not resolution, the same place Java's own `checkTriggerEffects` call sits.

`checkDamageDoneTriggersToCard`/`checkDamageDoneTriggersToPlayer` are CR 603's own "whenever ~ deals damage" mode,
`Mode$ DamageDone`, ported from `TriggerDamageDone.performTest` — split in two because Java's own `DamageTarget` is a
`GameEntity` that can be either a `Card` or a `Player`, and `ValidTarget` needs a different evaluator for each:
`Matches` (valid.go) for the first, `matchesPlayerSpec` (the same one `matchesActivatingPlayer` uses) for the second — 4
of the 5 real qualified `ValidTarget$ Player.*` lines resolve this way (`Player.Opponent` x3, `Player.Other` x1); the
fifth, `Player.EnchantedBy`, does not (`matchesPlayerSpec`'s own doc comment, valid.go). `damageDoneMatches` is
everything the two share (one walk, `ValidSource`, `CombatDamage$`) except that one check. `ValidSource` is optional the
same way `SpellCast`'s own `ValidCard` is (`matchesValidParam`'s absent-is-a-pass contract); `CombatDamage$` is checked
against a hardcoded `true` at both real call sites (`dealPermanentDamage`/`dealPlayerDamage`, combatdamage.go), since
nothing outside combat deals damage in this port yet — a `CombatDamage$ False` line (a rare "whenever ~ deals noncombat
damage" shape) can never fire, and one carrying `True` or neither always passes that half. Resolved: `DamageAmount$` (8
of 1,080 real lines) — `damageAmountMatches` ports `performTest`'s own hand-rolled parse directly
(`fullParam.substring(0,2)`/`substring(2)`, never `AbilityUtils.calculateAmount` — every real line is a plain integer or
the literal `TargetToughness`, never an SVar reference), reusing `compareOp` (valid.go), the same
`Expressions.compare`-ported switch `compareMatches`'s own numeric-comparison branch already has, rather than a second
one. `TargetToughness` reads the damaged card's own folded `Toughness()` at the moment of damage — only meaningful for
`checkDamageDoneTriggersToCard`, so `checkDamageDoneTriggersToPlayer` passes `hasToughness = false` and a line naming it
there is skipped (a shape that cannot arise for real; Java itself would throw `ClassCastException` casting the player to
a `Card`). Not resolved: `ValidCause$` (1) — a `SpellAbility`, not a `Card`; `TargetRelativeToCause$`/
`TargetRelativeToSource$` (0 real lines alongside the shapes above) — a `GameEntity`-vs-`GameEntity` relative match this
port has no evaluator for. 1,079 of 1,080 real lines carry neither. Both `checkDamageDoneTriggersToCard`/`ToPlayer` and
their two real call sites (`dealPermanentDamage`/`dealPlayerDamage`, combatdamage.go) now thread the actual `amount`
dealt through, fired right after each already emits `DamageDealt`/`LifeChanged` — CR 510.2's "simultaneous" damage is
still applied one exchange at a time (`dealCombatDamageStep`'s own doc comment), so the trigger fires per exchange too,
the identical simplification. `enginelint`'s `combatdamage` group gained `trigger` as a dependency.

`checkDiscardedTriggers`/`otherDiscardedTriggerMatches` are CR 603's own "whenever ~ is discarded" mode,
`Mode$ Discarded`, ported from `TriggerDiscarded.performTest` — and the one mode so far that could NOT reuse the
single/own-plus-other walk shape unchanged. A "Card.Self" Discarded trigger (14 of 105 real lines, the Madness-adjacent
"when this card is discarded, you may cast it" shape) lives on a card that is never on the battlefield at the moment it
fires — it is discarded FROM HAND. Every other mode this port checks has its own `TriggerZones$` synthesized as
`Battlefield`/`Stack` by `CardFactoryUtil.java`, so a single battlefield walk always finds a "Card.Self" host among the
permanents being walked; Java's own `TriggerReplacementBase.zonesCheck` is unrestricted by default
(`validHostZones == null` passes regardless of the host's current zone), and a real corpus Discarded line commonly
carries no `TriggerZones$` at all, so it is checked wherever its host card currently sits. This was caught by
`TestCleanupFiresDiscardedTrigger` failing on the first implementation attempt (a battlefield-only walk, mirroring every
other mode) — a real, self-caught gap, not a hypothetical one. `checkDiscardedTriggers` now has an explicit "own" half,
checking the discarded card's own `Triggers` directly (its own Move-preserved `Controller` as source,
`checkDiesTriggers`' own precedent for reading a card no longer on the battlefield), before
`otherDiscardedTriggerMatches`' battlefield walk for a watcher ("Whenever you discard a card, ..."). `ValidPlayer` — a
`Player`, not a `Card` — is `matchesPlayerBase` again. Not resolved: `ValidCause$` (11 of 105 real lines) — a
`SpellAbility`, not a `Card`. 94 of 105 real lines carry none of it. Fired from `cleanupStep`'s own CR 514.1
discard-to-hand-size loop (turn.go), the only place this port discards a card at all today, right after `Move` —
`checkDiesTriggers`' own after-the-fact timing. `enginelint`'s `turn` group gained `trigger` as a dependency.

`checkTapsTriggers` is CR 603's own "whenever ~ becomes tapped" mode, `Mode$ Taps`, ported from
`TriggerTaps.performTest` — the identical single-walk shape `checkAttacksTriggers`/`checkBlocksTriggers`/
`checkDamageDoneTriggersToCard` already established. `player` is the tapped card's own controller at both real tap sites
this port has (`DeclareCombatAttackers`, attack.go; `TapLandForMana`, manaability.go) — neither models anyone else's
action tapping a permanent, so `ValidPlayer`'s own dominant real value ("You," all 4 of the real lines that carry it) is
exactly this. `Attacker$` (2 of 177 real lines) IS resolved: `isAttacker` reports whether the tap was caused by
attacking or something else, the same boolean `TriggerTaps.performTest` itself compares against `AbilityKey.Attacker`.
Not resolved: `FirstTime$` (1) — "the first time a permanent taps this turn," per-card-per-turn state this port tracks
nothing for; `Teamwork$` (1) — `CostTeamwork`, a cost-type this port has no concept of; `ValidCause$` (0). 173 of 177
real lines carry none of the three skipped params. `attack.go`'s own Vigilance check (only a non-Vigilant attacker
actually taps, CR 508.1f) is what `checkTapsTriggers` is called from inside, not unconditionally per declared attacker —
a Vigilance attacker never taps, so it correctly never fires the trigger either.

`checkTapsForManaTriggers` is `Mode$ TapsForMana`'s own narrower mode, ported from `TriggerTapsForMana.performTest` — a
mana ability specifically, its own separate Java `Trigger` subclass, so its own separate check here too rather than a
param on `checkTapsTriggers`. `Activator` — a `Player`, `matchesPlayerSpec`'s own job — is `player` again, the identical
"the tapped card's own controller" simplification, since `TapLandForMana` is the only real mana-ability call site this
port has, but unlike `checkTapsTriggers`'s own `ValidPlayer` this one real corpus line qualifies it:
`Activator$ Player.NonActive` resolves through `matchesPlayerSpec`'s own `Active`/`NonActive` property
(`Game.ActivePlayer()`) — `TapLandForMana` has no active-player restriction of its own (unlike `CastSpell`'s sorcery-
speed-only one, castspell.go), so both a `Player.Active` and a `Player.NonActive` activator are real, distinguishable
cases here. Not resolved: `Produced$` (3 of 65 real lines) — `"C"` (2) can never match anyway, since `TapLandForMana`
only ever produces one of the five colors, never colorless; `"ChosenColor"` (1) needs a runtime value this port has no
evaluator for; skipped together. 62 of 65 real lines carry none of it. Both `checkTapsTriggers` and
`checkTapsForManaTriggers` are methods, not free functions or types — `enginelint`'s own dependency checker only tracks
package-level free functions/types (`topLevelNames`' own doc comment, `tools/enginelint/lint.go`), so calling either
from `attack.go`/`manaability.go` needed no new allow-list entry, unlike every earlier `checkXTriggers` call site this
port has (`combatdamage`/`turn`'s own `trigger` allow-list entries were added on the same assumption, ahead of time, and
turned out unnecessary once this was understood — harmless, since an unused allow-list entry is not itself a violation).

`checkPhaseTriggers` is CR 500's own "at the beginning of a step or phase" mode, `Mode$ Phase`, ported from
`TriggerPhase.performTest` plus the base `Trigger` class's own `phasesCheck` (`Trigger.java`) — corpus-frequency
research, done only after the first nine modes above had already landed, found this the corpus's SECOND most frequent
trigger mode of all (2,362 real lines, ahead of `Attacks`' own 1,606), even though `phase.go`'s own
`PhaseByName`/`PhaseType.String` and `TestPhaseNamesRoundTrip` (event_test.go) were built with exactly this mode in mind
from M5's very first turn-structure work — plumbing that sat unused until now. `TriggerPhase.performTest` itself has no
`ValidCard` at all: there is no object a step or phase change happens TO, only the change itself, so this is a single
condition-only walk rather than a match-against-something one. Two differences from every earlier mode: `ValidPlayer$`
is matched against the ACTIVE player (`AbilityKey.Player`, set to `phaseHandler.getPlayerTurn()` in
`PhaseHandler.onPhaseBegin`), not the trigger's own host controller the way `SpellCast`'s `ValidActivatingPlayer`/
`DamageDone`'s `ValidTarget`-as-a-player/`TapsForMana`'s `Activator` all are — resolved through the identical
`matchesPlayerSpec` (valid.go) regardless, since the function itself only ever compares two `PlayerID`s and does not
care which one is "the active player" and which is "the host's controller." And `TriggerZones$` is not implicit the way
every earlier mode's is: `Attacks`/`Blocks`/`DamageDone`/etc. all only ever fire from a card already on the battlefield
in the real corpus, so their own single walk over `Zone(Battlefield, pid)` stands in for a `TriggerZones$` check no one
has needed yet, but `Mode$ Phase` is real from the graveyard (28 real lines), exile (3) and command zone (84) too — a
suspend/exiled-with-a-ticking-trigger shape, and (speculatively) an emblem's own upkeep trigger, though this port has no
way to create a Command-zone object yet either. `phaseTriggerZones` (trigger.go) is the four real zones (`Battlefield`,
`Command`, `Graveyard`, `Exile`) `checkPhaseTriggers` walks instead of `Battlefield` alone, and
`phaseTriggerZoneMatches` is `TriggerReplacementBase.zonesCheck`'s own contract (`TriggerZones$` absent passes
regardless of zone, 26 of 2,362 real lines carry none) applied per zone in that walk.

`Phase$` itself (`phaseTriggerMatches`, trigger.go) resolves through the existing `PhaseByName` for every bare token the
real corpus writes (`Upkeep`, `BeginCombat`, `Draw`, `Main1`, `EndCombat`, `Main2`, `Cleanup`, `Untap`,
`Declare Attackers`, and a `Main1,Main2` comma-list, 1 real line) — a new `phaseNameFold`, `PhaseByName`'s own
case-insensitive twin, is needed only for `End of Turn`'s own 3 real lines spelled `End Of Turn` (a capital `O`), the
one phase name the corpus itself is inconsistent about (`ZoneByName`'s own doc comment says zone names never are);
`PhaseByName` itself stays exact, since its other two callers (`fixture.go`'s `GameState` parser, its own test) have no
such inconsistency to tolerate. `Main` bare (29 real lines, every one paired with `PhaseCount$ 2` — Survival's own
cards, "at the beginning of your second main phase") is the one token `PhaseByName` deliberately refuses
(`TestPhaseNamesRoundTrip`'s own assertion, event_test.go, written well before this trigger mode existed to consume it)
— `PhaseType.parseRange`'s own special case for it (`PhaseType.java`) expands to both `Main1` and `Main2` when no
`PhaseCount$` narrows it to the second one alone, ported directly rather than through Java's own
`phaseHandler.getNumMain()` counting trick, since this port's own `PhaseType` already has separate `Main1`/`Main2`
constants Java's single `MAIN` constant does not.

`IsPresent$`/`PresentCompare$` (272, 104 real lines) and `CheckSVar$`/`SVarCompare$` (310) are resolved now too --
`checkPhaseTriggers`'s own `hasAnyParam` pre-filter used to name all three anyway, a leftover from before
`triggerCommonRequirementsMet` (below) existed that kept 686 real lines combined skipped even after that general
mechanism landed and started resolving the identical params for every other trigger mode. Removing them from the
pre-filter was the whole fix: `triggerEffectAPI`, which `checkPhaseTriggers` already called for every match that got
past the pre-filter, already ran `triggerCommonRequirementsMet` first, so nothing else needed to change for
`Mode$ Phase` triggers to start reaching `isPresentMatches`/`checkSVarMatches` (item 26's own `meetsCommonRequirements`
paragraph) the same way `Draw`-carrying ETB triggers already did.

Not resolved, still skipped via `hasAnyParam`: `Condition$` (`SpellAbilityCondition`'s own separate gate on the ability
itself, distinct from `CheckSVar$`/`SVarCompare$`'s now-resolved `CardTraitBase` shape -- 0 real `Mode$ Phase` lines
carry the bare key today; a real corpus check found only 6 lines carrying it at all, corpus-wide, none reachable through
this port's own static trigger walk); `APlayerHasMoreLifeThanEachOther$`/`APlayerHasMostCardsInHand$` (2 and 1 real
lines -- `Trigger.requirementsCheck`'s own whole-table comparisons, a third general gate distinct from both
`meetsCommonRequirements` and `phasesCheck`, below). `FirstUpkeep$`/`FirstUpkeepThisGame$`/`FirstCombat$`/`TurnCount$`
are gone from this pre-filter entirely now -- `## Trigger.phasesCheck lands`, below, resolves (or correctly skips) all
four generically, the identical "remove the now-redundant special case once the general mechanism lands" fix this exact
paragraph already made once for `IsPresent$`/`CheckSVar$`, above. `ValidPlayer$`'s own qualified forms
`matchesPlayerSpec` cannot resolve (`Player.EnchantedBy`, 14; `Player.Chosen`, 3; `Opponent.EnchantedBy`, 2;
`Player.isMonarch`, 1 — 20 real lines) stay unresolved for the identical reason `SpellCast`'s own
`Player.EnchantedBy`/`Player.Chosen` do (`matchesPlayerSpec`'s own doc comment); `Player.EnchantedController` (34) and
`You.descended` (10) resolve now too ("`Phase`'s own qualified `ValidPlayer$`: `EnchantedController` and `descended`,"
below). 2,001 of 2,065 real `ValidPlayer$` lines resolve regardless of any of this (`You`, 1,832; `Player`, 113;
`Opponent`, 47; `Player.Opponent`, 7; `Player.Other`, 2).

`TestAdvancePhaseFiresPhaseTriggerAtCorrectStep`, `TestAdvancePhaseSkipsPhaseTriggerAtWrongStep`,
`TestAdvancePhaseSkipsPhaseTriggerForNonActivePlayer`, `TestAdvancePhaseFiresPhaseTriggerForOpponentValidPlayer`,
`TestAdvancePhaseFiresMainSecondTriggerOnMain2`, `TestAdvancePhaseSkipsMainSecondTriggerOnMain1`,
`TestAdvancePhaseFiresPhaseTriggerFromGraveyard`, `TestAdvancePhaseFiresPhaseTriggerWhenIsPresentConditionMet`,
`TestAdvancePhaseSkipsPhaseTriggerWhenIsPresentConditionNotMet` and
`TestAdvancePhaseSkipsPhaseTriggerWithUnresolvedParam` (trigger_test.go, the last two proving the fix above -- one a
tapped-permanent positive fire, the other `Condition$`'s own remaining skip) prove all of the above against synthetic
Upkeep-watcher- and Survival-second-main-phase-shaped cards, `AdvancePhase` (turn.go) itself the real (non-test) caller
through `beginPhase`'s own new `g.checkPhaseTriggers(controller)` call, right after a step's own mechanical body (if
any) and right before the state-based-action check that already follows every phase entry.

`checkAttackersDeclaredTrigger` is CR 508.1's own "whenever a player attacks" mode, `Mode$ AttackersDeclared`, ported
from `TriggerAttackersDeclared.performTest` and the exact Java call site that fires it,
`PhaseHandler.declareAttackersStep`: once per combat, right after every declared attacker is tapped and assigned a
target, and only `if (!combat.getAttackers().isEmpty())` -- `DeclareCombatAttackers` (attack.go) carries the identical
guard, so a combat with no declared attacker never reaches the check at all. Corpus-frequency research (286 real
`S:T:Mode$ AttackersDeclared` lines) turned this mode up once `Phase`'s own remaining gaps stopped being worth chasing
further -- bigger than what `SpellCast`'s own unresolved remainder had left, and cheap here specifically because it
reuses three pieces of machinery already built for other modes rather than needing any of its own.

`AttackingPlayer$` (175 of 286 real lines) resolves through the existing `matchesPlayerSpec`, checked against
`g.activePlayer` rather than the trigger's own host controller -- `Combat.getAttackingPlayer()` is always the active
player in this port's own combat model (only the active player ever calls `DeclareCombatAttackers`), the identical
"compare two `PlayerID`s, whichever they represent" indifference `Phase`'s own `ValidPlayer$` check already relies on.
`TriggerZones$` reuses `phaseTriggerZones`/`phaseTriggerZoneMatches` outright rather than a second, mode-specific zone
list: 273 of 286 real lines carry `Battlefield`, but 7 carry `Command` and 5 carry `Graveyard`, the identical
minority-but-real split that made `Phase`'s own four-zone walk necessary in the first place, so nothing here needed its
own version of that decision.

`AttackedTarget$` (63 of 286) is the one genuinely new piece: `attackedTargetMatches` (trigger.go) ports
`CardTraitBase.matchesValid`'s own `Iterable` branch, which tries every attacked entity against the whole comma-split
spec and reports true the instant any one of them matches any one token -- real specs mix player-shaped tokens (`You`,
`Player,Planeswalker`) and card-shaped tokens (`Planeswalker.YouCtrl`) in the very same comma list
(`You,Planeswalker.YouCtrl`, 4 real lines), and Java's own dispatch is by the CANDIDATE's type (`Player.isValid` vs
`Card.isValid`), not the token's own syntax. `attackedTargetMatches` mirrors that: for every attacked entity and every
token, it tries `matchesPlayerSpec` when the entity is a player and `Matches` (valid.go) when it is a card, and a
mismatched attempt (a card-shaped token against a player entity, or the reverse) simply reports "not recognized" and
moves on -- `matchesPlayerSpec`'s own `ok=false` for an unrecognized base already gives this for free, and `valid.Parse`
building a `Spec` no real player or card token like `"You"` ever satisfies as a card base gives it for the reverse case
just as cheaply, with no new dispatch logic required. `attackedTargetsOf` (trigger.go) collects the distinct entities
actually attacked this combat, straight off `Combat.AttackTargets`, the same "only what someone is actually attacking"
set `PhaseHandler.java`'s own `for (GameEntity ge : combat.getDefenders()) if (!combat.getAttackersOf(ge).isEmpty())`
filter builds. A qualified player token `matchesPlayerSpec` cannot resolve (`Player.EnchantedBy`, 9;
`Player.hasInitiative`, `Player.IsPoisoned`, `Opponent.lifeGTX`, 1 each -- 12 of 63 combined) never matches through
either branch, so a trigger naming one of these simply never fires -- GO-7's usual outcome, reached here by never
matching rather than a separate skip check, since Java's own per-candidate dispatch has no "unresolvable, abort" case of
its own to mirror.

`ValidAttackers$`/`ValidAttackersAmount$` (123 of 286) is `validAttackersCountMatches` (trigger.go): how many of the
attackers its own caller passes in the `ValidAttackers$` spec matches (the existing `Matches`, valid.go) -- for this
mode, always `Combat.Attackers`, every attacker declared this combat; `Mode$ AttackersDeclaredOneTarget`'s own paragraph
below passes a narrower subset -- compared against `ValidAttackersAmount$` -- default `"GE1"`, Java's own
`getParamOrDefault("ValidAttackersAmount", "GE1")`, "one or more," matching the corpus's own dominant
`TriggerDescription$` phrasing, "whenever one or more Knights you control attack." Every real `ValidAttackersAmount$`
value is a plain two-letter-operator-plus-digit shape (`GE2`, `GE3`, `EQ1`, ...), never an SVar or `"X"` the way Java's
own code path technically allows for, so this reads the digits directly through the existing `compareOp` (valid.go)
rather than resolving an amount through `resolveAmount` (amount.go) -- the identical simplification
`damageAmountMatches` already made for `DamageDone`'s own `DamageAmount$`.

`IsPresent$`/`PresentCompare$` (14, 4) and `CheckSVar$` (13) are resolved now too, the identical fix `Phase`'s own
paragraph above describes: this function's own `hasAnyParam` pre-filter used to name all three anyway, blocking
`triggerCommonRequirementsMet` (already run by `triggerEffectAPI`, this function's own final step) from ever evaluating
them for a `Mode$ AttackersDeclared` trigger even after that general mechanism could.

Not resolved, still skipped via `hasAnyParam`, the same "whole line, not a guess" contract every other mode's own
skip-list already has: `Condition$` (1) -- `StaticAbility.java`'s own runtime gate, no equivalent for any trigger mode
yet.

`TestDeclareCombatAttackersFiresAttackersDeclaredTrigger`,
`TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithNoAttackers`,
`TestDeclareCombatAttackersFiresAttackingPlayerYouTrigger`,
`TestDeclareCombatAttackersSkipsAttackingPlayerYouTriggerForDefender`,
`TestDeclareCombatAttackersFiresAttackedTargetYouTrigger`,
`TestDeclareCombatAttackersSkipsAttackedTargetYouTriggerForAttacker`,
`TestDeclareCombatAttackersFiresValidAttackersAmountTrigger`,
`TestDeclareCombatAttackersSkipsValidAttackersAmountTriggerBelowThreshold`,
`TestDeclareCombatAttackersFiresAttackersDeclaredTriggerWhenCheckSVarConditionMet`,
`TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWhenCheckSVarConditionNotMet` and
`TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithUnresolvedParam` (trigger_test.go, the last two of these
proving the fix above -- a met/unmet `CheckSVar$` pair against a literal SVar, and `Condition$`'s own remaining skip)
prove all of the above against a synthetic watcher-permanent def, `DeclareCombatAttackers` (attack.go) itself the real
(non-test) caller through its own new `g.checkAttackersDeclaredTrigger()` call, right after the per-attacker
`checkAttacksTriggers` loop.

### `Mode$ AttackersDeclaredOneTarget`: the identical trigger, fired per defender

`TriggerType.java`'s own enum entry names it plainly: `AttackersDeclaredOneTarget(TriggerAttackersDeclared.class)` --
the exact same Java `Trigger` subclass `Mode$ AttackersDeclared` already ports, registered a second time under a
different `TriggerType` so `PhaseHandler.java`'s own `declareAttackersStep` can fire it at a second granularity. Reading
that method (`## checkAttackersDeclaredTrigger`, above, already covers half of it) shows the real split: right before
the one `runTrigger(AttackersDeclared, ...)` call that carries every attacker and every attacked defender at once, a
loop over `combat.getDefenders()` fires `runTrigger(AttackersDeclaredOneTarget, ...)` once for every defender that has
at least one attacker, `AbilityKey.Attackers` narrowed to `combat.getAttackersOf(ge)` and `AbilityKey.AttackedTarget`
narrowed to a singleton list holding just that one `ge`. Two different `RunParams` shapes feeding the identical
`performTest` body -- not two mechanisms to port, one mechanism called twice with different inputs.

`checkAttackersDeclaredOneTargetTrigger` (trigger.go, new) is that second call site, `checkAttackersDeclaredTrigger`'s
own sibling: for every distinct entity `attackedTargetsOf` (above) already finds attacked this combat, it computes
`attackersTargeting` (new) -- the subset of `Combat.Attackers` whose own `AttackTargets` entry is that one entity, in
`Combat.Attackers`' own declaration order, `combat.getAttackersOf(ge)` ported directly -- then walks the identical
four-zone/per-face/per-trigger loop `checkAttackersDeclaredTrigger` already has, just against
`isAttackersDeclaredOneTargetTrigger` instead of `isAttackersDeclaredTrigger` and this one defender's own
attacker/target pair instead of the whole combat's.

The two functions no longer duplicate the actual param dispatch: a new `attackersDeclaredParamsMatch` (trigger.go) holds
`Condition$`'s own skip and the `AttackingPlayer$`/`AttackedTarget$`/`ValidAttackers$` checks both callers already had
inline, taking the attacker subset and target list as plain parameters rather than reading
`g.combat.Attackers`/`attackedTargetsOf` itself -- `checkAttackersDeclaredTrigger` passes the whole-combat pair,
`checkAttackersDeclaredOneTargetTrigger` passes the per-defender one. `validAttackersCountMatches` (above) changed the
identical way, taking `attackers []CardID` as a parameter instead of hardcoding `g.combat.Attackers` -- its only other
caller updated to pass that explicitly, and its own doc comment above updated to describe both shapes rather than only
the first one it had before this mode existed.

35 of the corpus's own 35 real `Mode$ AttackersDeclaredOneTarget` lines resolve end to end -- a full corpus scan of the
mode's own `[A-Za-z0-9]+\$` vocabulary (vocabscan-style, by hand) turns up only `TriggerZones$`/`TriggerDescription$`/
`Mode$`/`Execute$`/`AttackedTarget$` (35 each), `ValidAttackers$` (28), `AttackingPlayer$` (6), `ValidAttackersAmount$`
(5) and `Secondary$` (1) -- every one of those already resolved by
`attackersDeclaredParamsMatch`/`phaseTriggerZoneMatches`, the identical dispatch `checkAttackersDeclaredTrigger` already
has. 0 real lines carry `Condition$`/`OptionalDecider$`/`CheckDefinedPlayer$`/`IsPresent$`, the params that stay
unresolved for the plain `AttackersDeclared` mode's own remainder (`## AttackersDeclared`, above) -- this mode's own
real corpus population happens to avoid every one of those, not a claim this file makes about the mode in general.

`DeclareCombatAttackers` (attack.go) calls `g.checkAttackersDeclaredOneTargetTrigger(controller)` right before
`g.checkAttackersDeclaredTrigger(controller)`, `PhaseHandler.java`'s own call order (every per-defender firing inside
the loop, the one whole-combat firing after it) -- CR 603.3b's own APNAP ordering still applies within each call
separately (`pushTriggeredAbilities`, `## Stack`, above), called twice for two related but independent trigger sweeps
rather than once for a combined one, the same "each check function owns its own push" pattern every other trigger mode
in this file already has.

Four new tests in `trigger_test.go`, `attackersDeclaredOneTargetTriggerDef` the new helper
(`attackersDeclaredTriggerDef`'s own sibling, `Mode$ AttackersDeclaredOneTarget` in place of `Mode$ AttackersDeclared`):
`TestDeclareCombatAttackersFiresAttackersDeclaredOneTargetTriggerOncePerDefender` (two attackers split across the
defending player and a planeswalker they control fires a bare watcher twice, not once -- the mode's own defining
difference from `AttackersDeclared`, proven by drawing two cards rather than one);
`TestDeclareCombatAttackersOneTargetAttackedTargetMatchesOnlyThatDefender` (`AttackedTarget$ You` matches only the
player-targeted firing, not the planeswalker-targeted one);
`TestDeclareCombatAttackersOneTargetValidAttackersCountsOnlyThatDefendersOwnAttackers`/
`TestDeclareCombatAttackersOneTargetValidAttackersCountsBothAttackingOneDefender` (the same two attackers split across
two defenders fails `ValidAttackersAmount$ GE2` for both firings -- neither defender has two attackers of its own, even
though `Combat.Attackers` has two total -- while sending both at the SAME defender satisfies it, the positive twin
proving the negative one is not simply "never fires"). The whole-combat/per-defender split itself was regression-checked
by reverting `DeclareCombatAttackers`' own new call and confirming three of the four failed with the expected wrong card
count before restoring it (the fourth, a "must not fire" case, stays trivially true either way, so toggling it proves
nothing on its own).

### CR 603.3d's own "may" triggered ability

`OptionalDecider$` is the corpus's own single largest unresolved trigger param by real line count in this file -- 1,584
real lines corpus-wide, 1,497 of them on a `T:` line -- and, until now, this port had no resolution-time hook to ask
anyone anything at all. Reading `TriggerHandler.java`'s own `registerActiveTrigger` (the method every trigger's own
fire, `pushTriggeredAbilities`'s own Java counterpart, calls before the ability ever reaches the stack) shows why this
could not be a trigger-fire-time gate the way every other unresolved restriction in this file already is:
`OptionalDecider$` never stops an ability from being pushed. `sa.setOptionalTrigger(true)` and
`decider = AbilityUtils.getDefinedPlayers(host, regtrig.getParam("OptionalDecider"), sa).get(0)` run unconditionally the
moment a `T:` line names the key at all, and the built `WrappedAbility` goes onto the stack the identical way a
mandatory trigger's own does. The confirmation itself happens only once `WrappedAbility.resolve()` runs -- right before
its own `getActivatingPlayer().getController().playSpellAbilityNoStack(sa, false)` call, the literal line that runs the
ability's own body:

```java
if (decider != null) {
    if (!decider.isInGame()) {
        decider = SpellAbilityEffect.getNewChooser(sa, decider);
    }
    if (!decider.getController().confirmTrigger(this)) {
        return;
    }
}
```

A decline is a hard `return` out of `resolve()` -- before the ability's own body runs, and before anything chained onto
it does either, since Java's own `resolveSubAbilities` call happens inside `playSpellAbilityNoStack`'s own resolution
path, never reached at all once `resolve()` has already returned. "May" is a property of the WHOLE ability, chain
included, not a gate on its own top-level effect alone.

This port's own equivalent: `Ability` (ability.go) gained an `Optional bool` field, and `Registry.Resolve` (effect.go)
checks it first, before anything else that function does (including the `ErrUnimplemented` check for an API this port
has not built -- a declined "may" is real Magic's own outcome regardless of whether this port could have run the ability
behind it, so asking first matches CR 603.3d more closely than erroring out a card a controller would have declined
anyway):

```go
func (r *Registry) Resolve(g *Game, a *Ability, controller PlayerController) error {
	if a.Optional && !controller.ConfirmOptionalTrigger(g, a.Controller, a.Source) {
		return nil
	}
	...
}
```

A new `PlayerController.ConfirmOptionalTrigger` (control.go, the interface's own twenty-fifth method) is
`WrappedAbility.resolve()`'s own `decider.getController().confirmTrigger(this)`, decider always `a.Controller` --
`Ability.Optional`'s own doc comment has the reason no separate decider field exists yet: only `OptionalDecider$ You` is
resolved, and "You" always means the ability's own controller (`AbilityUtils.getDefinedPlayers(host, "You", sa)`'s own
single-element result), already exactly what `Ability.Controller` already carries at every one of this port's own call
sites. `true` runs the ability (and its own `SubAbility$` chain via `resolveSubAbility`, subability.go,
`Registry.Resolve`'s own recursive call for it) exactly as if it had never been optional; `false` skips both,
`Registry.Resolve`'s own early `return nil` before `resolveSubAbility` is ever called at all -- the identical "whole
ability, chain included" semantics `WrappedAbility.resolve()`'s own early return already has.

`Ability.Optional` itself is set by a new `triggerIsOptional` (trigger.go), folded into `triggerEffectAPI`'s own shared
gate alongside `triggerPhasesCheck`/`triggerCommonRequirementsMet` -- every one of `triggerEffectAPI`'s own twenty-eight
call sites across every trigger mode this port has built runs through it for free, no per-mode code needed. 1,506 of the
corpus's own 1,584 real `OptionalDecider$` lines (95%) name "You": `triggerIsOptional` resolves exactly that value and
refuses every other one (`TriggeredCardController`, 43 real lines; `True`, 11; `TriggeredSourceController`, 5;
`TriggeredPlayer`/`Opponent`, 4 each; `EnchantedController`, 3;
`TriggeredAttackingPlayer`/`TriggeredActivator`/`TargetedController`, 2 each; eleven more distinct values, 1 real line
apiece) -- each names a decider this port has no resolver for, a further `AbilityUtils.getDefinedPlayers`-shaped
question distinct from the "You" case `host.Controller()` already answers directly, with nobody correct for
`ConfirmOptionalTrigger` to ask yet. `triggerEffectAPI` reports `ok=false` for those, the identical "skip the whole
line, don't guess" contract every other unresolved trigger restriction in this file already has (GO-7) -- chosen over
Java's own "push regardless, ask nobody in particular" fallback, since a wrong guess at WHO decides is worse than not
firing at all:

```go
func triggerIsOptional(t *compile.Ability) (optional, ok bool) {
	decider, present := t.Param("OptionalDecider")
	if !present {
		return false, true
	}
	if !strings.EqualFold(decider, "You") {
		return false, false
	}
	return true, true
}
```

Three already-built modes resolve real lines they could not before, simply by reaching this shared gate for the first
time -- none needed their own code changed, only their own `hasAnyParam` skip-list's own explicit `"OptionalDecider"`
entry removed, where one existed: `Mode$ Untaps`'s own remaining 3 real lines (`## checkUntapsTriggers`, above -- 30 of
30 now), `Mode$ LifeGained`'s own 7 (`## M6's fourth effect: GainLife, and Mode$ LifeGained`, below -- 93 of 98),
`Mode$ BecomesTarget`'s own 12 (`## Mode$ BecomesTarget lands`, above -- 101 of 118, none of the 12 also naming
`Valiant$`/`ActivationLimit$`/`Static$`). A fourth, `Mode$ LandPlayed`'s own 3 real lines (`## Mode$ LandPlayed lands`,
above -- 38 of 42 now), is a real correctness fix rather than only a new resolution: `checkLandPlayedTriggers`'s own
`hasAnyParam` call never named `"OptionalDecider"` at all, so search_the_city.txt's/jokulmorder.txt's/burgeoning.txt's
own real "you may..." lines were reaching the old `triggerEffectAPI` (which had no check for the key either) and firing
unconditionally -- the identical class of bug `LifeGained`'s own `ActivationLimit$` fix
(`## M6's fourth effect: GainLife, and Mode$ LifeGained`, below) already caught and fixed earlier this session, found
here by the same routine due-diligence rather than by a bug report.

83 more real lines name `OptionalDecider$` on a sub-ability's own SVar body
(`SVar:TrigFoo:DB$ ... | OptionalDecider$ You | ...`) rather than a `T:` line -- a chained `SubAbility$`'s own
independent "may," CR 700.2's own "then" text sometimes itself optional. A smaller, separate gap this change does not
reach: `resolveSubAbility` (subability.go) builds its own `child := Ability{...}` with no `Optional` field set at all,
so a chained sub-ability is never treated as optional regardless of its own `OptionalDecider$`, deliberately out of
scope for this chunk (`Ability.Optional`'s own doc comment names it explicitly).

Six new tests in a new `optionaltrigger_test.go`, `optionalDeciderTriggerDef` the new helper (an Enchantment with one
`Mode$ AttackersDeclared` trigger naming `OptionalDecider$ <decider>`, `Execute$` chaining `DB$ Draw` into a
`SubAbility$ DB$ LoseLife` -- SubAbility chaining, `## Sub-ability chaining lands`, above, reused to prove a decline
skips the WHOLE ability): `TestConfirmedOptionalTriggerRunsWholeAbilityChain` (confirmed, both the draw and the chained
life loss happen); `TestDeclinedOptionalTriggerSkipsWholeAbilityChain` (declined, neither does -- `Registry.Resolve`'s
own early return before `resolveSubAbility` is ever reached, not merely the top-level `DB$ Draw` itself refusing);
`TestUnresolvedOptionalDeciderSkipsTriggerWithoutAsking` (`OptionalDecider$ TriggeredCardController` skips before ever
calling `ConfirmOptionalTrigger` at all -- proven by a clean run against a `ScriptedController` with an EMPTY
`optionalTrigger` queue: asking at all would panic, `scriptExhausted`'s own contract, so a clean pass proves the
question was never posed, not merely answered no). Three more in `untaps_test.go`, against `Mode$ Untaps`'s own real
shape rather than a synthetic mode: `TestStartTurnFiresUntapsTriggerNamingOptionalDeciderWhenConfirmed`/
`TestStartTurnSkipsUntapsTriggerNamingOptionalDeciderWhenDeclined`/
`TestStartTurnSkipsUntapsTriggerNamingUnresolvedOptionalDecider` (this file's own former
`TestStartTurnSkipsUntapsTriggerNamingOptionalDecider`, rewritten now that `OptionalDecider$ You` no longer means
"always skip" -- DOC-16). Two more in `trigger_test.go`, against `Mode$ LandPlayed`'s own real burgeoning.txt shape:
`TestPlayLandFiresLandPlayedTriggerNamingOptionalDeciderWhenConfirmed`/
`TestPlayLandSkipsLandPlayedTriggerNamingOptionalDeciderWhenDeclined`.

Every new gate was regression-checked: `Registry.Resolve`'s own confirm check, temporarily removed, made both
`TestStartTurnSkipsUntapsTriggerNamingOptionalDeciderWhenDeclined` and
`TestDeclinedOptionalTriggerSkipsWholeAbilityChain` fail with the ability having run anyway, before being restored;
`triggerIsOptional`'s own `"You"`-only check, temporarily widened to accept any decider, made
`TestStartTurnSkipsUntapsTriggerNamingUnresolvedOptionalDecider` panic on the scripted controller's own empty queue
(proving it would otherwise have asked), before being restored. `ScriptedController` gained a twenty-fifth queue,
`optionalTrigger []bool`, and `QueueConfirmOptionalTrigger`/`ConfirmOptionalTrigger` alongside it, the identical
slice-pop-and-panic-when-empty shape every other queued decision already has. `scriptedMulliganController`
(mulligan_test.go), the only other real `PlayerController` implementer, gained a `panic`-stub `ConfirmOptionalTrigger`
too, `ChooseEnchantTarget`'s own sibling stub for a method the mulligan tests never reach.

`checkDrawnTriggers` is CR 120.3's own "whenever you draw a card" mode, `Mode$ Drawn`, ported from
`TriggerDrawn.performTest`, called from `DrawCards`' own per-card loop (turn.go) -- a call site that already existed in
exactly the shape this needed before this mode existed to consume it: `DrawCards`' own doc comment always drew one card
at a time rather than moving `n` at once, reasoning that "matters once something reacts to an individual draw rather
than the batch," written well before this trigger mode was the thing that did.

161 real `S:T:Mode$ Drawn` lines corpus-wide (vocabscan), reusing `phaseTriggerZones`'s own four-zone walk again (150 of
161 real lines carry `TriggerZones$ Battlefield`, but 6 carry `Command` and 3 carry `Graveyard` -- the identical
minority-but-real split every other mode's own zone walk exists for). `ValidCard$` (156 of 161) is checked against the
drawn card itself through the existing `Matches` (valid.go): every real value (`Card.YouCtrl`, `Card.OppOwn`,
`Card.YouOwn`, `Card.OwnedBy`, a bare `Card`, ...) is an ordinary valid-string this port's evaluator already covers,
needing nothing new -- unlike `AttackersDeclared`'s own `AttackedTarget$`, nothing here mixes a player-shaped token with
a card-shaped one, so there is no dispatch-by-candidate-type problem to solve. `ValidPlayer$` (13) resolves through the
existing `matchesPlayerSpec`, against the player who actually drew (`TriggerDrawn.performTest`'s own
`AbilityKey.Player`, set to the drawing player in `Player.java`'s own `drawCard`) rather than the trigger's own host
controller -- the identical "compare two `PlayerID`s, whichever they represent" indifference `Phase`'s own
`ValidPlayer$` and `AttackersDeclared`'s own `AttackingPlayer$` checks already rely on.

`Number$` (79 of 161) is the one genuinely new piece: a new `Player.CardsDrawnThisTurn` (player.go) -- `LandsPlayed`'s
own per-turn-counter shape, ported from Java's own `numDrawnThisTurn` (`Player.java`). `DrawCards` (turn.go) increments
it once per card, in the identical order Java's own `drawCard` does: the increment happens BEFORE the trigger check
runs, so `Number$ 2` means "the second card this player has drawn this turn, counting this one," not "about to draw its
second." `cleanupStep` (turn.go) resets it to zero for every player alongside `LandsPlayed`, the same per-turn reset
scope (`Player.java`'s own `onCleanupPhase` resets `numDrawnThisTurn` in the identical place). Not resolved, skipped via
`hasAnyParam`: `FirstCardInDrawStep$` (5) -- Java's own separate `numDrawnThisDrawStep`, a narrower per-draw-STEP
counter (as opposed to per-turn) this port tracks nothing for, since nothing else needs it yet; `ForReveal$` (5) --
Java's own `AbilityKey.CanReveal`, a reveal-while-drawing flag (Sensei's Divining Top-adjacent shapes) this port's own
`DrawCards` has no equivalent state for.

`TestDrawCardsFiresDrawnTriggerForCardYouCtrl`, `TestDrawCardsSkipsDrawnTriggerForCardYouCtrlWhenHostControlledByOther`,
`TestDrawCardsFiresDrawnTriggerForValidPlayerOpponent`,
`TestDrawCardsSkipsDrawnTriggerForValidPlayerOpponentWhenHostIsDrawer`,
`TestDrawCardsFiresDrawnTriggerForMatchingNumber`, `TestDrawCardsSkipsDrawnTriggerForNonMatchingNumber` and
`TestDrawCardsSkipsDrawnTriggerWithUnresolvedParam` (trigger_test.go) prove all of the above, `DrawCards` (turn.go)
itself the real (non-test) caller through its own new `g.checkDrawnTriggers(pid, id, ...)` call, right after the
`CardDrawn` event each drawn card already emits.

`matchesPlayerBase` (valid.go) is the shared `You`/`Opponent`/`Player` three-way dispatch, factored out once a THIRD
caller needed the identical switch `matchesActivatingPlayer` and `matchesValidDefender` had each already written
separately — `SpellCast`'s own `ValidActivatingPlayer`, `CantBlockBy`'s own `ValidDefender`, `DamageDone`'s own
`ValidSource`/`ValidTarget`-as-a-player, `Discarded`'s own `ValidPlayer`, `Taps`'s own `ValidPlayer` and `TapsForMana`'s
own `Activator` all reuse it, returning `(matched, ok)` so a caller with its own additional dispatch
(`matchesValidDefender`'s own `"Player.controls<Type>"`) can tell "does not match" apart from "not a shape this function
recognizes at all, keep looking." It lives in `valid.go` rather than `trigger.go`/`staticability.go` specifically so
both groups can reach it without a new `enginelint` cross-dependency: both already allow `valid`.

`matchesPlayerSpec`/`matchesPlayerProperty` (valid.go) sit on top of `matchesPlayerBase`, the identical `Base.Property`
split `Player.isValid` (Player.java) itself does on the first `.` before ANDing every `+`-joined property via
`hasProperty`/`PlayerProperty.playerHasProperty` — ported once `SpellCast`'s own `ValidActivatingPlayer` turned out to
need it for 19 of its 25 real qualified lines (above). `matchesPlayerSpec` checks the base clause through
`matchesPlayerBase` first, then the property (when there is one) through `matchesPlayerProperty`, which itself tries
`matchesPlayerBase` again first (Java's own "Opponent"/"You" property branches reuse the identical base check a property
token gets) before its own two further conditions: `Active`/`NonActive` (`Game.ActivePlayer()`, the same accessor
`Matches`' own `ActivePlayerCtrl` property already reads for a `*Card`) and `Other` (not `sourceController` — collapses
into the identical check as `Opponent` under this port's own no-team simplification). Only `matchesActivatingPlayer`
(`SpellCast`), `checkDamageDoneTriggersToPlayer` (`DamageDone`'s own `ValidTarget`-as-a-player) and
`checkTapsForManaTriggers` (`TapsForMana`'s own `Activator`) call it — the other three `matchesPlayerBase` callers
(`CantBlockBy`'s `ValidDefender`, `Discarded`'s `ValidPlayer`, `Taps`'s `ValidPlayer`) keep calling `matchesPlayerBase`
directly, verified against the real corpus to carry zero qualified lines for those exact params, so routing them through
the extra split would add a dependency with nothing real to resolve.

Both started out checked only against the one card's own `Triggers`, not every other permanent's own triggers watching
for someone else's zone change. `otherETBTriggerMatches`/`otherDiesTriggerMatches` close that gap for both: every
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
by some other card leaving as an ETB watcher is by one arriving. `otherDiesTriggerMatches` is the identical shape to
`otherETBTriggerMatches`, once that was noticed.

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
`Execute$` sub-ability can be any of the 203 corpus-frequency effects M6 owns; `NewRegistry` implements `Draw`,
`DealDamage`, `GainLife`, `Pump`, `PumpAll` and `LoseLife` today (alongside casting a permanent and an Aura, M5's own
two) — `ResolveStack` reports `ErrUnimplemented` for the other 196 once `PutCounter` is counted alongside them too.
`TestCastSpellFiresETBTrigger`, `TestDestroyLethalToughnessFiresDiesTrigger`,
`TestDestroyLethalToughnessFiresOtherPermanentsWatchingDiesTrigger`, `TestCastSpellFiresOtherPermanentsWatchingTrigger`,
`TestDeclareCombatAttackersFiresAttacksTrigger`, `TestDeclareCombatAttackersFiresOtherPermanentsWatchingAttackTrigger`,
`TestDeclareCombatAttackersFiresAttacksTriggerWhenAttackedConditionMet`,
`TestDeclareCombatAttackersSkipsAttacksTriggerWhenAttackedConditionNotMet`,
`TestDeclareCombatAttackersFiresFirstAttackTriggerOnFirstAttackThisTurn`,
`TestDeclareCombatAttackersSkipsFirstAttackTriggerWhenAlreadyAttackedThisTurn`,
`TestDeclareCombatBlockersFiresBlocksTrigger`, `TestDeclareCombatBlockersFiresOtherPermanentsWatchingBlockTrigger`,
`TestDeclareCombatBlockersSkipsBlocksTriggerWithUnresolvedParam`,
`TestCastSpellFiresSpellCastTriggerForControllerActivatingPlayer`,
`TestCastSpellSkipsSpellCastTriggerForNonControllerActivatingPlayer`,
`TestCastSpellFiresSpellCastTriggerForOpponentActivatingPlayer`,
`TestCastSpellSkipsSpellCastTriggerWithUnresolvedParam`, `TestDealCombatDamageFiresDamageDoneTriggerToPlayer`,
`TestDealCombatDamageFiresDamageDoneTriggerToCard`, `TestDealCombatDamageFiresOtherPermanentsWatchingDamageDoneTrigger`,
`TestDealCombatDamageFiresDamageDoneTriggerWithMatchingDamageAmount`,
`TestDealCombatDamageSkipsDamageDoneTriggerWithNonMatchingDamageAmount`,
`TestDealCombatDamageFiresDamageDoneTriggerWithMatchingTargetToughness`,
`TestDeclareCombatAttackersFiresAloneTriggerWhenAttackingAlone`,
`TestDeclareCombatAttackersSkipsAloneTriggerWithAnotherAttacker`,
`TestDeclareCombatAttackersFiresDefendingPlayerPoisonedTrigger`,
`TestDeclareCombatAttackersSkipsDefendingPlayerPoisonedTriggerWithNoPoison`,
`TestDeclareCombatAttackersFiresAttackDifferentPlayersTrigger`,
`TestDeclareCombatAttackersSkipsAttackDifferentPlayersTriggerAgainstOnePlayer`, `TestCleanupFiresDiscardedTrigger`,
`TestCleanupFiresOtherPermanentsWatchingDiscardedTrigger`, `TestCleanupSkipsDiscardedTriggerWithUnresolvedParam`,
`TestDeclareCombatAttackersFiresTapsTrigger`, `TestDeclareCombatAttackersSkipsTapsTriggerForVigilantAttacker`,
`TestTapLandForManaFiresOtherPermanentsWatchingTapsTrigger`,
`TestDeclareCombatAttackersSkipsTapsTriggerWithUnresolvedParam`, `TestTapLandForManaFiresTapsForManaTrigger` and
`TestDeclareCombatAttackersDoesNotFireTapsForManaTrigger` (trigger_test.go) prove nine of the twelve modes against
synthetic Elvish-Visionary-, Rotting-Regisaur-, Blood-Artist-, Impact-Tremors- and attacking/blocking/damage-dealing/
discarded/tapping/casting-creature-shaped cards, each compiled through the real pipeline; the tenth, eleventh and
twelfth, `Phase`, `AttackersDeclared` and `Drawn`, each have their own test list right after their own descriptive
paragraph, above.

Corpus-frequency: 5,688 cards carry the ETB shape (`Destination$ Battlefield`); 1,341 carry the dies shape
(`Origin$ Battlefield` + `Destination$ Graveyard`, 205 of them watching some other creature rather than themselves,
`ValidCard$ *.Other`/`*.YouCtrl` — the count behind `otherDiesTriggerMatches`); 1,606 carry `Mode$ Attacks`, 1,555 of
them resolvable; 127 carry `Mode$ Blocks`, 119 of them resolvable; 1,080 carry `Mode$ DamageDone`, 1,079 of them
resolvable; 105 carry `Mode$ Discarded`, 94 of them resolvable; 177 carry `Mode$ Taps`, 173 of them resolvable; 65 carry
`Mode$ TapsForMana`, 62 of them resolvable; 1,435 carry `Mode$ SpellCast`, 1,163 of them resolvable; 2,362 carry
`Mode$ Phase` — the corpus's second most frequent mode of all, ahead of `Attacks` itself — most of them resolvable (the
`checkPhaseTriggers` paragraph, above, has the precise breakdown across `Phase$`/`ValidPlayer$`/`TriggerZones$` and what
remains unresolved); 286 carry `Mode$ AttackersDeclared` — most of them resolvable (the `checkAttackersDeclaredTrigger`
paragraphs, above, have the precise breakdown across `AttackingPlayer$`/`AttackedTarget$`/`ValidAttackers$` and what
remains unresolved); 161 carry `Mode$ Drawn`, 156 of them resolvable through `ValidCard$` alone, most of the rest
through `ValidPlayer$`/`Number$` too (the `checkDrawnTriggers` paragraph, above, has the precise breakdown).

One real fixture changed because of this: `cast-a-battle-spell-reaches-the-stack` (formerly
`...-resolves-to- battlefield`) stops at the stack rather than resolving fully, because every Battle in the corpus turns
out to carry its own "create a token" ETB trigger — `Invasion of Belenon` among them — and `TestScenarios` has no way to
assert an expected `RunActions` failure the way an internal test can with `errors.Is`. The other four permanent-type
fixtures (creature, artifact, enchantment, planeswalker) are unaffected: none of those four cards carries a `T:` line.

### `Mode$ AttackerBlocked` and `Mode$ AttackerBlockedByCreature`: CR 509.2's own other side

`checkBlocksTriggers` (above) is the blocker's own "whenever this blocks." CR 509.2 groups a second, symmetric family
into the same declare-blockers step: the attacker's own "whenever this becomes blocked" (`Mode$ AttackerBlocked`, 127
real lines, `TriggerAttackerBlocked.java`) and "whenever this becomes blocked by a creature"
(`Mode$ AttackerBlockedByCreature`, 102 real lines, `TriggerAttackerBlockedByCreature.java`), both real now.

The two Java classes differ in granularity, and `checkAttackerBlockedTriggers`/`checkAttackerBlockedByCreatureTriggers`
(trigger.go) each keep it: `AttackerBlockedByCreature` fires once per (attacker, blocker) pair, `checkBlocksTriggers`'s
own exact mirror image — `ValidCard$` matched against `blk.Attacker` where `checkBlocksTriggers` matches it against
`blk.Blocker`, `ValidBlocker$` matched against `blk.Blocker` where `checkBlocksTriggers` matches `ValidBlocked$` against
`blk.Attacker` — called from the identical per-`Block` loop in `DeclareCombatBlockers` (block.go) `checkBlocksTriggers`
already runs in. `AttackerBlocked` fires once per attacker instead, with every legal blocker gathered first:
`DeclareCombatBlockers` now builds a `map[CardID][]CardID` (blockers by attacker) alongside `blocks` itself while it
already walks them for `checkBlocksTriggers`/`checkAttackerBlockedByCreatureTriggers`, then calls
`checkAttackerBlockedTriggers` once per distinct attacker afterward, the whole group in hand.

`ValidCard$` (127 of 127, 74 with nothing else — an unqualified "becomes blocked") matches the attacker directly, the
same shape every other trigger mode's own `ValidCard$` already does. `ValidBlocker$`/`ValidBlockerAmount$` (53 of 127
carry one or both) needed a new `validCardsCountMatches` (trigger.go): `validAttackersCountMatches`'s own
count-and-compare shape (item 26's own `AttackersDeclared` account, above) generalized from `g.combat.Attackers`
specifically to any `[]CardID`, since the group here is one attacker's own blockers, gathered fresh per attacker, never
the whole combat's. Neither mode needs a separate own/other loop: `TriggerAttackerBlocked`/
`TriggerAttackerBlockedByCreature` never special-case the attacker's own trigger any more than `TriggerBlocks` did for
the blocker's, so one battlefield walk already covers "this creature becomes blocked" and "a creature you control
becomes blocked" alike — `TestDeclareCombatBlockersFiresOtherPermanentsWatchingAttackerBlockedTrigger` (trigger_test.go)
proves it directly, the identical shape `TestDeclareCombatBlockersFiresOtherPermanentsWatchingBlockTrigger` already
proved for `Blocks`.

Not resolved: `ValidCard$ LessPowerThanBlocker`/`ValidBlocker$ LessPowerThanAttacker` (1 real line each) — a hardcoded
power comparison rather than a valid-string (Skulk's own hardcoded-`X` shape, `skulkBlocks`, staticability.go, for a
different pairing), explicitly refused in both `checkAttackerBlockedTriggers`/`checkAttackerBlockedByCreatureTriggers`
rather than left to fall through to a bare-word valid-string parse that would silently match no card and never fire —
the identical observable result, but for the wrong reason, PORT-8's own concern regardless of whether it happens to look
harmless here. `Mode$ AttackerBlockedOnce` (3 real lines, its own once-per-turn Java class,
`TriggerAttackerBlockedOnce.java`) is not built.

`TestDeclareCombatBlockersFiresAttackerBlockedTrigger`/
`TestDeclareCombatBlockersFiresOtherPermanentsWatchingAttackerBlockedTrigger`/
`TestDeclareCombatBlockersFiresAttackerBlockedTriggerWhenBlockerAmountMatches`/
`TestDeclareCombatBlockersSkipsAttackerBlockedTriggerWhenBlockerAmountDoesNotMatch` prove `AttackerBlocked`, the last
two via a real two-blocker gang block;
`TestDeclareCombatBlockersFiresAttackerBlockedByCreatureTriggerWhenValidBlockerMatches`/
`TestDeclareCombatBlockersSkipsAttackerBlockedByCreatureTriggerWhenValidBlockerDoesNotMatch`/
`TestDeclareCombatBlockersFiresAttackerBlockedByCreatureTriggerOncePerBlocker` prove `AttackerBlockedByCreature`, the
last proving the per-pair granularity directly: two matching blockers draw two cards, not one.

### `CardTraitBase.meetsCommonRequirements`: the one gate every trigger mode shares

Every check-triggers function above shares one blind spot: `Trigger.java`'s own `performTest` never runs at all unless
`meetsCommonRequirements` passes first (CardTraitBase.java, `## Layer 7, Layer 4...` above already ports its
`IsPresent$`/`Condition$` cousins for static abilities), and until now this port checked none of it for a trigger -- a
real card naming `IsPresent$`/`CheckSVar$`/`Threshold$`/whatever alongside an already-resolved mode fired
unconditionally, the gate silently never consulted. A corpus tally directly against `T:` lines (grepping the bare param
name catches some `S:`/`A:` lines too, a different switch on the identical name -- `StaticAbility. checkConditions`'s
own `Condition$`, `SpellAbilityCondition`'s own `Condition$`/`ConditionPresent$`, neither this gate's problem) puts
every param `meetsCommonRequirements` reads at ~1,271 real `T:` lines.

`triggerCommonRequirementsMet` (trigger.go, new) closes 1,148 of them. It is called from inside `triggerEffectAPI`
itself, not duplicated at each of the eighteen check-triggers call sites: every one of them already funnels its own
match through that one function to turn it into a pushed `Ability`, so `triggerEffectAPI` gaining `g`/`host`/`amounts`
parameters (threaded from the identical `for _, face := range h.Def.Faces { for _, t := range face.Triggers {` loop
every caller already has `face.Amounts` inside) reaches every mode for free. The eighteen call sites themselves needed
only their own argument list updated -- `triggerEffectAPI(t)` to `triggerEffectAPI(g, h, face.Amounts, t)` (or `c`/`w`
in the two own/other split functions still walking a bare `*Card` rather than a `host` id) -- nothing about their own
matching logic changed.

`isPresentMatches` ports `IsPresent$`/`PresentCompare$`/`PresentZone$`/`PresentPlayer$` (624 of ~1,271, and the
identical `IsPresent2$` pair counted with it) -- `PresentZone$` a comma list defaulting to `Battlefield`, `ZoneByName`
per entry; `PresentPlayer$` `"You"` (host's own controller only) or the corpus's own default `"Any"` (every player --
Java's own three additive You/Opponent/Allies blocks collapsed to the one partition a single-valued param actually
produces, since a real line is never all three sources at once); the counted set run through `Matches` the identical way
every other valid-string check in this port already is. `PresentDefined$` (40 of 624) skips: no
Defined$-to-cards resolver for an arbitrary reference exists yet, `drawDefinedPlayers`' own narrow You/Opponent form
(draweffect.go) being the only `Defined$`
evaluator built so far, and it resolves players, not cards.

`checkSVarMatches` ports `CheckSVar$`/`SVarCompare$` (474) -- both sides resolved through a new `resolveNamedAmount`
(amount.go): `ptParam`'s own literal-or-named-SVar shape (continuous.go), factored out once this needed the identical
resolution against a `*Card` rather than reading one specific `*compile.Ability` param directly; `ptParam` itself is now
a two-line wrapper calling it. A line also naming `CheckSecondSVar$` (0 real `T:` lines today) skips outright -- Java
ORs a second check against the first (the identical "secondCheck" shape `SpellAbilityCondition.areMet` already has for
its own `ConditionCheckSVar$`/`OrOtherConditionSVarCompare$` pair), and nothing forces guessing at that shape blind when
the real corpus does not exercise it.

`boolFlagMatches` ports the six-times-repeated `"True".equalsIgnoreCase(params.get(key)) != predicate()` shape for
`Metalcraft$`/`Delirium$`/`Threshold$`/`Hellbent$`/ `FatefulHour$` (38 combined) -- reusing `continuousConditionMet`'s
own underlying predicates (`## Condition$: the one gate all six appliers share`, above): the identical player-state
question, asked here as an explicit flag (`Threshold$ False` meaning "must NOT have threshold" is as real a line as
`Threshold$ True`) rather than as the whole condition the way a static ability's own `Condition$ Threshold` is. Moving
them mattered structurally, not just for reuse: `battlefieldArtifactCount`/`graveyardCoreTypeCount` lived in
continuous.go, and leaving them there while `resolveNamedAmount` needed to serve both `ptParam` (continuous.go) and
`triggerCommonRequirementsMet` (trigger.go) would have made `continuous`→`trigger` and `trigger`→`continuous` both real
edges -- a dependency cycle `tools/enginelint`'s own acyclic-parts rule (ADR-0003's own premise, `javacycles` at the
Java-package level, this tool's own equivalent inside `internal/engine`) catches immediately, not a false positive the
way `ControlEffect`'s own `Player`-named field or `compile.Ability`'s own textual collision with `ability.go`'s
`Ability` were (`## Layer 7...`/`## Replacement effects` above, both). Both functions moved to a new `playerstate.go`
(its own `enginelint.json` group, depending on nothing either `continuous` or `trigger` themselves provide), and
`resolveNamedAmount` moved to amount.go, next to `resolveAmount` itself -- neither shared function belongs to just one
caller, so neither stayed in either caller's own file.

`lifeTotalMatches` ports `LifeTotal$`/`LifeAmount$` (12) -- `"You"` (`g.Player(host.Controller()).Life`) and
`"ActivePlayer"` (`g.Player(g.ActivePlayer()).Life`), the corpus's own two real `T:` values;
`OpponentSmallest`/`OpponentGreatest` carry none and stay unresolved.

Not resolved, each skipped whole rather than treated as met (GO-7, the identical "cannot evaluate, so do not fire" rule
an unresolved `Affected$`/`Condition$` value already has elsewhere in this port): `Revolt$` (25) -- no
`Game.leftBattlefieldThisTurn`-equivalent tracked anywhere; `WerewolfTransformCondition$`/
`WerewolfUntransformCondition$` (65) -- Innistrad's own day/night mechanic, keyed off a "spells cast last turn" list
this port tracks nowhere; `CheckDefinedPlayer$` (20) -- every real line qualifies it with `isMonarch`, `hasInitiative`,
`withMostLife` or `withMostType`, mechanics this port has none of, not a shape a general Defined$-to-players resolver
could close on its own even if one existed.

`ManaSpent$`/`ManaNotSpent$` (8) -- no paying-colors-by-cast tracked; `Adamant$` (1); `Bloodthirst$`, `Monarch$`,
`EnduringStory$`, `DayTime$` and `ClassLevel$` (0 real `T:` lines each, dormant rather than actively skipped).

Eleven new tests (`TestPlayLandFiresETBTriggerWhen*`/`SkipsETBTriggerWhen*`, trigger_test.go) prove `IsPresent$` both
ways plus its `PresentZone$`/`PresentDefined$` variants, `CheckSVar$` both ways, `Threshold$` both ways, `LifeTotal$`
both ways, and `Revolt$`'s own unresolved skip -- each driven through a real `Game.PlayLand` rather than a synthetic
call, `commonReqTriggerLandDef`'s own new helper mirroring `continuousDef`'s reasoning (a land needs no mana cost to
move, so the setup stays about the common-requirements param under test).

## `Mode$ BecomesTarget` lands, and targeting gets a second real event

CR 115/603.3's own "whenever CARDNAME becomes the target of a spell or ability" -- `TriggerBecomesTarget`, ported at the
shape this port's own targeting mechanism (`## Targeting itself lands`, below -- written earlier in this file, but
landed earlier in the port) can reach. Java's own trigger point is `MagicStack.add`, right after a spell's own targets
are chosen and right before `SpellCastOrCopy` fires: "Run BecomesTarget triggers... Create a new object, since the
triggers aren't happening right away," walking every distinct object across every `TargetChoices` the cast collected (a
`Set<GameObject>` dedup, "so Becomes targets don't trigger for things like Seeds of Strength" -- one spell hitting the
same object twice from two different sub-targets fires the trigger once, not twice).

This port has exactly two places that finish choosing a target for anything today, and both are where
`checkBecomesTargetTriggers` (trigger.go, new) is called from: `pushTriggeredAbilities` (trigger.go), right after each
`PushAbility` -- a triggered ability's own `a.Targets`, freshly set by `resolveTargets` two lines earlier in the same
function -- and `castAura` (castspell.go), for an Aura's own single cast-time attach target. A targeted Instant/Sorcery
would be a third real site (CR 601.2c's own general spell-targeting case), but `CastSpell` only casts a permanent or an
Aura so far (`## Casting a spell needed the stack for real, for the first time`, above) -- the identical "mechanism now,
content later" gap `resolveTargets`'s own doc comment already names for exactly this future caller.

Deduplication is the identical `distinctObjects` set Java's own loop builds, just local to each call rather than
threaded through a second `AbilityKey.Targets`/`BecomesTargetOnce` pass (`BecomesTargetOnce`'s own real corpus role -- a
single trigger per spell naming EVERY distinct target it chose, rather than once per target -- has 0 real lines combined
with a resolvable shape this port could tell apart from `BecomesTarget` itself, so it is not built at all, the identical
"zero real lines, stays dormant" bucket `EnduringStory$`/`Monarch$` already sit in elsewhere in this file).

`ValidTarget$` (present on all 118 real lines) is matched with `attackedTargetMatches` (trigger.go,
`checkAttackersDeclaredTrigger`'s own dispatch for `AttackedTarget$`, reused outright): the identical
one-entity-of-either-kind problem a becomes-target event has, since a real target is a player
(`ValidTarget$ You,Permanent.YouCtrl+inZoneBattlefield`, Rayne, Academy Chancellor's own real line) or a card, never
both spelled out as one nested spec. `Card.AttachedBy`/`EnchantedBy` (Ice Cage's own real "enchanted creature becomes
the target of a spell or ability, destroy CARDNAME") needed no new code at all -- `Matches`' own existing
`EnchantedBy`/`AttachedBy` case (`## The card's mutable parts`'s neighbor section, `valid.go`) already reads its own
`source` argument as "the object being checked for being attached to," and `host.ID` -- the Aura itself -- is exactly
that for a trigger living on the Aura naming its own enchanted creature.

`FirstTime$` (Glyph Keeper's own real "for the first time each turn, counter it") reads a new
`Card.BecameTargetThisTurn` (card.go) -- `Card.AttacksThisTurn`'s own boolean sibling, not its counting one: Java's own
`hasBecomeTargetThisTurn()` is `!targetedFromThisTurn.isEmpty()`, a per-target `Player` set, but no real `FirstTime$`
line on a `Mode$ BecomesTarget` trigger ever asks WHICH players are in that set, only whether it is empty -- the
identical simplification this port already made for `Player`-shaped state it tracks nowhere else (`matchesPlayerBase`'s
own "no team support" doc comment). Set unconditionally the moment a distinct target is checked, before any
`ValidTarget$` match is even attempted -- Java's own `addTargetFromThisTurn` runs in the SAME loop iteration as the
trigger's own `runTrigger` call, both fed by the same "was this the first time" check computed BEFORE either -- matching
`TestBecomesTargetSkipsWhenValidTargetDoesNotMatch`'s own proof that a card becomes a target (the flag flips)
independently of whether any trigger's own restriction happens to fire. Reset every cleanup (`cleanupStep`, turn.go)
alongside `AttacksThisTurn`.

`ValidSource$` resolves too now, through a new `becomesTargetSourceMatches` (trigger.go) -- matched against the
triggering `SpellAbility` itself (`AbilityKey.SourceSA` in Java), not a `Card`, `SpellAbility.isValid`'s own restriction
split (`incR[0]` against `Spell`/`Ability`/`Triggered`/`Activated`/`SpellAbility`, then a `.`-qualified property).
Rather than adding a general Spell/Activated/Triggered kind classifier to `Ability`, this reads the ONLY two ability
kinds this port's own two real call sites can ever produce: `castAura`'s own Aura is always a Spell
(`isSpellSource=true`), and a triggered ability pushed through `pushTriggeredAbilities` is always Java's own
`isTrigger()`/`isAbility()` pair at once (`isSpellSource=false`) -- this port has no activated-ability targeting built
yet, so `Ability` and `Triggered` collapse to the identical "not a Spell" test for now, a real simplification narrower
than Java's own three-way split but exact for every real corpus line either caller can ever reach. `SpellAbility` itself
matches unconditionally (Java's own "match anything" case, `SpellAbility.OppCtrl`'s/`SpellAbility.YouCtrl`'s own 46
combined real lines); `.YouCtrl`/`.OppCtrl` compare the ability's own controller (`sourceController`, threaded through
as a new parameter from each call site -- `pid` at `castAura`, `matches[i].Controller` at `pushTriggeredAbilities`)
against the watching trigger's own host controller, the identical YouCtrl/OppCtrl contract every other property in this
port already has; `.Aura` is trivially true whenever the kind itself is Spell, since this port's only Spell source
reaching here IS an Aura being cast (`castAura`'s own doc comment) -- `Spell.Aura`'s own 5 real lines need no further
check at all. 71 of the corpus's own 77 real lines naming `ValidSource$` resolve; 6 stay unresolved:
silverfur_partisan.txt's/wild_defiance.txt's own real `Instant,Sorcery` (a card-type check neither of this port's two
sources, a Trigger or an Aura, can ever satisfy) and four more real lines combining a kind with a property past
YouCtrl/OppCtrl/Aura (`namedGoblin Artisans`, `numTargets EQ1`, `Land+named...`, `Backup`), each its own further
mechanic, refused by the same fall-through `default: return false` every unrecognized head or property already hits
(GO-7).

101 of the corpus's own 118 real `Mode$ BecomesTarget` lines resolve now (`OptionalDecider$`, 12, every real line "You"
and none also naming `Valiant$`/`ActivationLimit$`/`Static$`, resolves too through `triggerEffectAPI`'s own
`triggerIsOptional`, "`CR 603.3d's own "may" triggered ability`," below). Not resolved, each failing loudly by name
rather than matching unconditionally (PORT-8/GO-7): `Valiant$` (10) -- `Card.isValiant`'s own separate per-activator
"have you not targeted this before" set (`getController().equals(p) && !targetedFromThisTurn.contains(p)`), a different
question than `FirstTime$`'s plain bool can answer even if it wanted to; `ActivationLimit$` (3) and `Static$` (1) --
each its own further mechanic (goblin_artisans.txt's own `Static$ True` line is a static ability synthesizing a
trigger-shaped check, not a real `T:` line at all, distinct from the `T:`-prefixed corpus-frequency count above).

Ten tests (`becomestarget_test.go`): `TestPushTriggeredAbilitiesFiresBecomesTargetOnCardTarget` and
`TestCastAuraFiresBecomesTargetTriggerOnEnchantedCreature` prove the two real call sites both fire, each isolating the
BecomesTarget-triggered `GainLife`'s own life change from whatever the parent ability itself does (an ETB trigger's own
`DB$ LoseLife | ValidTgts$ Creature.YouCtrl` finds no player among a card-shaped target and does nothing, the identical
isolation technique `## SubAbility chaining reaches every effect`'s own tests already use for a different reason);
`TestBecomesTargetFirstTimeOnlyFiresOnceEachTurn` proves the flag's own once-per-turn contract across two separate
targeting events; `TestBecomesTargetSkipsWhenValidTargetDoesNotMatch` proves `BecameTargetThisTurn` still flips even
when no trigger's own `ValidTarget$` matches. `TestBecomesTargetFiresForMatchingSpellAbilityController`/
`TestBecomesTargetSkipsForNonMatchingSpellAbilityController` prove `SpellAbility.YouCtrl`/`.OppCtrl` both ways;
`TestBecomesTargetFiresForAuraSpellSource`/`TestBecomesTargetSkipsAuraSpellSourceWhenValidSourceRequiresTriggered` prove
the kind check both ways, the second isolating it from any controller comparison at all;
`TestBecomesTargetSkipsWhenSourceKindDoesNotMatch` is the original unresolved-shape regression test, kept and
re-described now that `Spell.OppCtrl` resolves (its own kind mismatch and controller mismatch both independently refuse
the line, so the two tests above are what isolate each reason on its own);
`TestBecomesTargetSkipsUnresolvedValidSourceShape` proves the truly-unrecognized-head case (`Instant,Sorcery`) refuses.
Every new branch was regression-checked by temporarily disabling it and confirming the corresponding test failed with
the expected wrong life total before restoring it -- the whole-dispatch toggle alone flipped all four "fires" tests to
failing, the `.YouCtrl`/`.OppCtrl` toggle flipped `TestBecomesTargetSkipsForNonMatchingSpellAbilityController`, and the
unresolved-head fallback toggle flipped `TestBecomesTargetSkipsUnresolvedValidSourceShape`.

## `Trigger.phasesCheck` lands

A general gate every trigger mode carries regardless of what it fires on -- `TriggerHandler.isTriggerActive` checks it
BEFORE `canRunTrigger`/`performTest` even runs, distinct from `CardTraitBase.meetsCommonRequirements`
(`triggerCommonRequirementsMet`, above), a different Java method on a different class entirely, called from INSIDE a
mode's own `performTest` rather than before it. This port had never checked either general gate's own params at all
until now: `triggerPhasesCheck` (trigger.go, new) ports `Trigger.phasesCheck` at the shape the real corpus uses, called
from `triggerEffectAPI` right before `triggerCommonRequirementsMet` -- Java's own ordering, `phasesCheck` gating whether
a trigger is even active before `canRunTrigger` asks anything else.

`Phase$` (19 real `T:` lines outside `Mode$ Phase`'s own dispatch) restricts a trigger of ANY mode to firing only during
the named step(s)/phase(s) -- reusing `phaseTriggerMatches` (`checkPhaseTriggers`'s own dispatch function, above)
generically, since the question is identical either way: does `Phase$` name the current phase. This closes a genuine
naming collision this port had not noticed until reading `Trigger.java` line by line: `Mode$ Phase`'s own
`TriggerPhase.performTest` checks only `ValidPlayer$` -- `Phase$` on a `Mode$ Phase` line is not that mode's own
dispatch param at all, it is THIS SAME general gate, just happening to restrict the one mode whose entire purpose is
firing at a phase boundary. A `Mode$ Phase` line now reaches `triggerPhasesCheck` too (through `triggerEffectAPI`, which
`checkPhaseTriggers` already calls), re-asking the identical question against the identical inputs
`checkPhaseTriggers`'s own explicit `phaseTriggerMatches` call already answered -- redundant, but harmless, and left
alone rather than refactored away: `checkPhaseTriggers` is tested, working code, and the redundancy costs nothing a real
game would ever notice.

`PlayerTurn$` (61 real lines) / `NotPlayerTurn$` (0, ported anyway for symmetry with Java's own
hasParam-not-value-checked contract -- neither key is ever value-sensitive in Java, so a hypothetical
`PlayerTurn$ False` would still restrict positively) restrict to the trigger's own host controller's turn, or explicitly
not it -- `Trigger.java`'s own `isPlayerTurn(hostController)` check, ported directly. sentinel_tower.txt's own real
"Whenever an instant or sorcery spell is cast during your turn, CARDNAME deals 1 damage to each opponent" is
`PlayerTurn$ True` on a `Mode$ SpellCast` line -- one of 43 real lines across six already-built modes (`SpellCast` 12+2,
`ChangesZone` 9+11, `LifeGained` 5, `Taps` 2, `Discarded` 1, `Drawn` 1) that fired **unconditionally** until this
landed: a wrong answer, not a coverage gap (PORT-8/GO-7) -- this port had simply never read either key before.
`OpponentTurn$` (23, `SpellCast`/`Drawn`) is `Player.isOpponentOf`'s own question, which collapses to the identical
check `NotPlayerTurn$` already makes in this port's own no-team model (`matchesPlayerBase`'s own doc comment: with no
teams, "not my turn" and "my opponent's turn" are the same fact).

`FirstCombat$` (6, `Attacks`/`AttackersDeclared`, both already built) is `PhaseHandler.isFirstCombat`'s own
`nCombatsThisTurn==1` -- always true here: this port has no extra-combat mechanism (an `AddCombat` effect is not built,
`turn.go`'s own doc comment: "extra turns/phases... not here"), so no real game this port can play ever reaches a second
combat phase in the same turn, making a hardcoded `true` the CORRECT answer today rather than a guess -- the identical
reasoning `combatdamage.go`'s own `CombatDamage$` check already uses for the identical "no mechanism makes this false
yet" shape. 0 real lines write `FirstCombat$ False`, so the reverse case needs no answer here; if
`balthier_and_fran.txt`'s own real `TrigAddCombat` (an extra-combat grant, M6's own remaining scope) is ever built, this
hardcoded answer becomes wrong and needs a real per-turn counter -- a coverage gap this doc comment flags in advance
rather than after the fact.

Not resolved, each skipping the whole line rather than guessing (GO-7): `FirstUpkeep$` (1) / `FirstUpkeepThisGame$` (2,
both `Mode$ Phase` only) -- `PhaseHandler.isFirstUpkeep`/`isFirstUpkeepThisGame`'s own per-turn/per-game upkeep-step
counters; unlike `FirstCombat$`, `FirstUpkeepThisGame$` genuinely can be false on any turn after the first even without
a new mechanism (an ordinary game has one upkeep every turn, but only the very first one is the game's own first), and
this port tracks neither; `TurnCount$` (0 real lines, dormant).

`checkPhaseTriggers`'s own `hasAnyParam` pre-filter no longer names `FirstUpkeep$`/`FirstUpkeepThisGame$`/
`FirstCombat$`/`TurnCount$` -- `triggerPhasesCheck` resolves (or correctly skips) all four generically now, the
identical "remove the now-redundant special case" fix this file's own trigger-firing section already made once for
`IsPresent$`/`CheckSVar$` (above).

Eight new tests (`triggerphases_test.go`), all built on `spellCastWatcherDefExtra` (`spellCastWatcherDef`'s own sibling,
carrying whatever extra general param a test needs): `TestSpellCastFiresTriggerWhenPlayerTurnMatches`/
`SkipsTriggerWhenPlayerTurnDoesNotMatch`, `...WhenOpponentTurnMatches`/`SkipsTriggerWhenOpponentTurnDoesNotMatch` and
`...WhenPhaseMatches`/`SkipsTriggerWhenPhaseDoesNotMatch` each prove one restriction fires when it should and stays
silent when it should not -- the three `Skips` tests are the regression proof, each confirmed to fail exactly as
expected when `triggerPhasesCheck`'s own call from `triggerEffectAPI` was temporarily removed and the suite rerun, then
confirmed to pass again once restored. `TestAttacksFiresFirstCombatTrigger` proves `FirstCombat$ True` resolves through
a real `DeclareCombatAttackers` call (raph_leo_sibling_rivals.txt's own real shape).

## `Mode$ Untaps` lands

CR 502.3/603's own "whenever CARDNAME becomes untapped" -- `TriggerUntaps`, `Taps`'s own mirror image at the opposite
end of the identical event (a card's own `Tapped` field flipping). Java's own trigger point is `Card.untap()`: an early
`if (!tapped) return false` before anything else runs (a card already untapped generates no event at all), then
`ReplacementType.Untap`'s own "doesn't untap" check (`untapBlocked`, `## Replacement effects`, below -- landed well
before this trigger did), then `TriggerType.Untaps` fires, then `tapped` is actually cleared.

`untapStep` (turn.go) already had the middle piece (`untapBlocked`) from an earlier chunk; this one adds the first and
third. A new `wasTapped := c.Tapped` local, read before `untapBlocked`'s own check, stands in for Java's own early
return -- `checkUntapsTriggers` (trigger.go, new) is only ever called when `wasTapped` was true AND `untapBlocked` did
not block it, the exact two-part gate `Card.untap()` itself has. `untapStep` gained a `controller PlayerController`
parameter for this (`beginPhase`'s own call site, turn.go, already had one to hand it -- the identical "every
state-based-action-adjacent call site already carries a controller by now" pattern every earlier controller-threading
chunk this port has done already relied on).

`checkUntapsTriggers` is structurally `checkTapsTriggers`' own exact twin: `TriggerUntaps.performTest` never
special-cases its own host's trigger either (unlike `checkDiesTriggers`/`checkETBTriggers`, which need a separate "own"
and "other" half), so one battlefield walk covers both a card's own "Inspired" trigger (`ValidCard$ Card.Self`, the
corpus's own dominant real shape -- untapping during your own untap step and paying a cost for an effect) and
mesmeric_orb.txt's own bare "whenever a permanent becomes untapped" (`ValidCard$ Card`, matching anyone's). `ValidCard$`
absent is a pass, the identical contract `checkTapsTriggers` already has for it.

30 of the corpus's own 30 real `Mode$ Untaps` lines resolve now. `Phase$`/`CheckSVar$` (1 each) fold in for free through
`triggerPhasesCheck`/`triggerCommonRequirementsMet` (`triggerEffectAPI`'s own two general gates, both landed earlier
this session) -- `checkUntapsTriggers` needed no code of its own for either. `Secondary$` (1) is a pure display flag
(`CardTraitBase.isSecondary`, consulted only by `Card.java`'s own rules-text generation to avoid printing a duplicate
ability line, never by any `performTest`/gating path anywhere in Java) -- this port already ignores it everywhere else
for the identical reason, so `checkUntapsTriggers` does too, by simply never reading it. `OptionalDecider$` (3, every
real line "You") resolves too now, through `triggerEffectAPI`'s own `triggerIsOptional`
("`CR 603.3d's own "may" triggered ability`," below).

Six tests (`untaps_test.go`), all built on `untapsCreatureDef` (`becomestarget_test.go`'s own
inert-Execute$-for-isolation pattern, reused): `TestStartTurnFiresUntapsTriggerForSelf` proves the Inspired shape;
`TestStartTurnFiresUntapsTriggerForOtherPermanent` proves the bare-`Card` shape fires for a SEPARATE permanent untapping
(a plain nil-`Def` card stands in for the untapping permanent itself, since `baseMatches`'s own bare `"Card"` case needs
no `Type()` at all to answer true); `TestStartTurnSkipsUntapsTriggerForAlreadyUntappedCard` is the `wasTapped` guard's
own regression proof, asserting `StackLen() == 0` rather than just an unchanged life total, since nothing ever resolves
an ability this port never pushes in the first place.

`TestStartTurnFiresUntapsTriggerNamingOptionalDeciderWhenConfirmed` and
`TestStartTurnSkipsUntapsTriggerNamingOptionalDeciderWhenDeclined` prove `OptionalDecider$ You` both ways against this
mode's own real shape: a confirmed `ConfirmOptionalTrigger` runs the body, a declined one leaves life unchanged.
`TestStartTurnSkipsUntapsTriggerNamingUnresolvedOptionalDecider` proves an `OptionalDecider$` value this port cannot
resolve (`TriggeredCardController`) skips the whole line before ever asking -- if it asked, the scripted controller's
own empty queue would panic, so a clean run proves the question was never posed at all. The general mechanism's own
section below has its own regression-check account: a `Registry.Resolve` toggle, plus a `triggerIsOptional` toggle
proving the unresolved-decider skip specifically.

## `ReplacementEffect.requirementsCheck` lands

A general gate every replacement carries regardless of its own `Event$`, checked before its own shape-specific
`canReplace` -- `Trigger.phasesCheck`'s own exact sibling ("`Trigger.phasesCheck` lands," above), a different Java
method on a different class, not the same one reused twice: `Trigger` and `ReplacementEffect` are sibling subclasses of
`TriggerReplacementBase` rather than one inheriting from the other.

`replacementRequirementsCheck` (`replacement.go`, new) ports it at the shape this port can reach.

`PlayerTurn$` (8 real `R:` lines combined across `DamageDone`/`Draw`/`CreateToken`/`LifeReduced`/`TurnFaceUp`, every one
the literal value `True` -- 0 real lines use Java's own Defined$-reference else-branch, so only the literal-`True`
branch is ported) checks `isPlayerTurn(hostController)` directly.

`ActivePhases$` (1 real line, island_sanctuary.txt's own `Draw` shape) reuses `phaseTriggerMatches` (trigger.go,
`Mode$ Phase`'s own dispatch function) at its own key rather than `Phase$`'s. `phaseTriggerMatches` gained a `key`
parameter for this; its two existing call sites (`checkPhaseTriggers`, `triggerPhasesCheck`) both updated to pass the
literal `"Phase"` explicitly, so the identical phase-list parser now serves three callers under three different param
names for the identical underlying question.

`triggerCommonRequirementsMet` (`CardTraitBase.meetsCommonRequirements`'s own port, trigger firing above) is called
outright last -- `ReplacementEffect.requirementsCheck`'s own final line calls the identical Java method a `Trigger`'s
own `performTest` already does, so this port's identical shared function serves both for the same reason.

Every existing consumer in `replacement.go` (`damagePreventionMatches`, `untapReplacementMatches`,
`replacementTapsOnMove`) had its own separate, narrower allow-list of extra params it tolerated before this landed; each
now calls `replacementRequirementsCheck` and widens its own allow-list to admit the keys it reads, closing 7 of the 10
previously-skipped real `DamageDone`|`Prevent$` lines (`PlayerTurn$` 4 -- guardian_naga_banishing_coils.txt's own real
"can't be dealt damage during your turn" among them; `CheckSVar$`/`SVarCompare$` 2; `IsPresent$` 1) and 5 of the 7
previously-skipped `Untap`|`CantHappen` lines (`IsPresent$` 4; `CheckSVar$`/`SVarCompare$` 1) for free -- neither
function needed a single line of its own new logic for either, just a wider switch and one extra call. The exact counts
are below, in each shape's own section.

Folding the general gate into `replacementTapsOnMove` also fixed a real, if narrow, wrong-firing bug that predates this
chunk: `replacementTapsOnMove` carried no allow-list at all before now (unlike its two siblings), so ANY extra param on
an "enters tapped" `R:` line was silently ignored rather than gating anything. archelos_lagoon_mystic.txt's own real
toggle -- "As long as CARDNAME is tapped, other permanents you control enter tapped. As long as CARDNAME is untapped,
other permanents enter untapped" -- writes exactly this as two
`Event$ Moved | ... | IsPresent$ Card.Self+tapped/+untapped | ReplaceWith$ ETBTapped/ETBUntapped` lines, restricting
each half to fire only while Archelos ITSELF is tapped or untapped respectively. Before this chunk, `IsPresent$` was
never checked at all, so the `ETBTapped` half (`ReplaceWith$` resolving to a plain `DB$ Tap`, `tapAbilityResolvesTap`'s
own recognized shape) would apply regardless of whether Archelos was actually tapped -- the `ETBUntapped` half never
mattered either way, since `tapAbilityResolvesTap` only recognizes `Tap`-named sub-abilities, not `Untap`. A corpus
check confirmed this is the ONLY real line among all 618 `ETBTapped`/`LandTapped`-shaped `Moved` lines carrying any of
`triggerCommonRequirementsMet`'s own hard-skip keys or the general gate's own `PlayerTurn$`/`ActivePhases$`, so folding
the check in here changes no other real card's behavior, confirmed and not merely assumed.

Two new consumers reuse the same general gate directly for a shape neither `checkMovedReplacement` nor
`damagePrevented`/`untapBlocked` cover: **`drawPrevented`/`gainLifePrevented`** (`replacement.go`, new) resolve CR
120.3's/119's own `Prevent$ True` shape for `Event$ Draw`/`GainLife` -- CR 119's own "life gain replacement" family this
port's own `gainLifeEffect` doc comment already flagged as entirely unbuilt has its one directly resolvable real shape
now. 2 of the corpus's own 39 real `Draw` lines resolve end to end: possessed_portal.txt's own bare
`ValidPlayer$ Player | Prevent$ True` ("if a player would draw a card, that player skips that draw instead") and
living_conundrum.txt's own `IsPresent$ Card.YouOwn | PresentZone$ Library | PresentCompare$ EQ0`-qualified form ("if you
would draw a card while your library has no cards in it") -- resolved through `replacementRequirementsCheck`'s own
`triggerCommonRequirementsMet` fold-in with no code of its own needed, the identical zone-presence check `LandTapped`'s
own checkland condition already exercises. obstinate_familiar.txt's own third real `Prevent$` line (`Optional$ True`)
stays unresolved: an interactive "may" confirm this port's own `PlayerController` has no hook for, the identical gap
`Discard`'s own `Optional$`/`BecomesTarget`'s own `OptionalDecider$` already document. 1 of the corpus's own 21 real
`GainLife` lines resolves end to end -- sulfuric_vortex.txt's own bare "if a player would gain life, that player gains
no life instead," and the ONLY one of the 21 naming `Prevent$` at all.

`drawPrevented` is checked from `DrawCards` (turn.go) one card at a time, BEFORE the empty-library check runs at all --
Java's own `Player.doDraw` checks its `Event$ Draw` replacement before it ever looks at whether the library is empty,
ported directly: a card `drawPrevented` reports true for never reaches the empty-library branch below it, so a draw CR
614 prevents cannot also be CR 704.5b's own "attempted to draw from an empty library" loss -- possessed_portal.txt's own
real shield would otherwise still lose its own controller to state-based action 704.5b even though the draw it prevented
was the only thing that could have exposed the empty library to begin with. `gainLifePrevented` is checked from
`gainLifeEffect` (gainlifeeffect.go) per player, before `Player.Life` is touched at all.

The other 36 real `Draw` lines and 20 real `GainLife` lines all name `ReplaceWith$` instead of `Prevent$` --
`DrawTwo`/`Dig`/`ExileTop`/... for `Draw`, `GainDouble`/`RLoseLife`/`Draw` for `GainLife`. At the time this section was
written, every one of these needed "the amount that would have been drawn/gained" as a runtime `X`/`Y` value (Java's own
`AbilityKey.ReplacedAmount` threading into a fresh `calculateAmount` call) this port's `resolveAmount` had no way to
read back. `## Draw`'s own `ReplaceWith$`, below, and `## GainLife`'s own `ReplaceWith$`, further below, resolve 3 and 4
of these two totals respectively, including `nefarious_lich.txt`'s own real `GainLife`-into-`Draw` line named here
originally as an example of the gap -- `resolveGainLifeReplacementAmount`'s own narrow `ReplaceCount$LifeGained` case
reads that runtime value directly from the raw amount `gainLifeEffect.Resolve` already computes, rather than through
`resolveAmount`/`resolveNamedAmount` at all. `rain_of_gore.txt`'s own line stays unresolved, but for a different reason
than "the amount": its own restriction is `ValidSource$ SpellAbility | SourceController$ True`, not a `ValidPlayer$`
this dispatch's own allow-list recognizes at all.

Fourteen new tests total: five in `replacement_test.go`
(`TestUntapBlockedWhenIsPresentConditionMet`/`TestUntapNotBlockedWhenIsPresentConditionNotMet` replace the old
`TestUntapNotBlockedByUnresolvedExtraParam`, since `IsPresent$` no longer means "skip" -- the OLD test's own real shape,
`IsPresent$ Creature.YouCtrl` checked against a lock creature that IS itself the only creature its controller controls,
now genuinely blocks, the opposite of what the old test asserted; `TestDamageToCreatureNotPreventedByUndefinedCheckSVar`
replaces `TestDamageToCreatureNotPreventedByUnresolvedExtraParam` for the identical reason, its own numeric assertion
unchanged since `CheckSVar$ X` with no `SVar:X` on the card still fails closed through `resolveNamedAmount`'s own
unresolvable-reference return, just for the honest reason of an undefined reference rather than an unrecognized param
name now; `TestDamageToPlayerPreventedWhenPlayerTurnMatches`/`TestDamageToPlayerNotPreventedWhenPlayerTurnDoesNotMatch`
prove the new `PlayerTurn$` fold-in both ways) and six in a new `drawgainlifeprevented_test.go`
(`TestDrawPreventedByBareReplacement`, `TestDrawPreventedDoesNotCauseEmptyLibraryLoss` -- the CR-faithful ordering proof
above, `TestDrawPreventedWhenIsPresentConditionMet`/`TestDrawNotPreventedWhenIsPresentConditionNotMet`,
`TestDrawNotPreventedByUnresolvedOptional`, `TestGainLifePreventedByBareReplacement`). All five renamed/new
`replacement_test.go` cases and the four `drawPrevented`/`gainLifePrevented`-proving cases were each regression-checked
by temporarily removing the corresponding code path and confirming the suite fails exactly as expected before restoring
it.

`enginelint.json`'s own `"replacement"` group gained `"trigger"` in its allow-list -- `replacementRequirementsCheck`
calls `phaseTriggerMatches`/`triggerCommonRequirementsMet` directly now, and `"condition"` (already in `"replacement"`'s
allow-list) already reached into `"trigger"` itself, so this adds no new cycle, just a direct edge alongside an existing
indirect one the tool's own per-file check does not treat as equivalent.

## `Phase`'s own qualified `ValidPlayer$`: `EnchantedController` and `descended`

`matchesPlayerProperty` (valid.go), `matchesPlayerSpec`'s own property half, gains two more real cases -- both reused
for free by every one of its nine existing callers (`checkSpellCastTriggers`'s own `matchesActivatingPlayer`,
`checkDamageDoneTriggersToPlayer`, `checkTapsForManaTriggers`, `checkPhaseTriggers`, `checkAttackersDeclaredTrigger`,
`attackedTargetMatches`, `checkDrawnTriggers`, `checkLifeGainedTriggers` in trigger.go; `applyOneContinuousRules` in
continuous.go; `targetCandidates` in targeting.go; `damagePreventedPlayer`/`drawPreventionMatches`/
`gainLifePreventionMatches` in replacement.go), since both `matchesPlayerSpec` and `matchesPlayerProperty` gained a
`source CardID` parameter -- the ability's own host card, threaded alongside the controller `PlayerID` they already took
-- purely mechanical plumbing through every call site, not a behavior change for any of the nine.

`EnchantedController` (`Player.EnchantedController` in Java's own `PlayerProperty.playerHasProperty`) reads
`source.getEnchantingCard()`, ported as `source.AttachedTo()` (`Card.go`'s own attachment link, the identical one Layer
2's own `GainControl$ You | Affected$ Card.EnchantedBy` already reads the other direction, item 27) -- the card the
trigger's own host (an Aura) is attached to -- then checks whether the candidate player controls it.
righteous_authority.txt's own real "at the beginning of the draw step of enchanted creature's controller, that player
draws an additional card" is the corpus's own dominant shape: 34 of `Mode$ Phase`'s own real qualified `ValidPlayer$`
lines resolve this way (a 35th, on `Mode$ AttackerUnblocked`, is moot -- that mode is not built at all).

`descended` (`Player.descended`/`getDescended()` in Java, incremented in `Zone.add`'s own `!rollback` branch whenever a
permanent, non-token card enters a graveyard from anywhere) is a new `Player.DescendedThisTurn bool` (player.go) --
Java's own field is a per-turn count, but every real corpus line only ever asks `< 1`, so a bool is enough. Set in
`Game.Move` (game.go): `isPermanent := c.Type().IsPermanent()` is read at the top of `Move`, before the
battlefield-leaving branch clears anything, and `kind == Graveyard && isPermanent` sets `DescendedThisTurn` on `owner`
right after the existing zone-transition switch -- every real `Move`-to-`Graveyard` call site in this port already
passes `owner` as the moving card's own `Owner` field, so this needed no new parameter threading of its own. Java's own
check also excludes a token; this port has no token-creation effect yet (M6's own remaining territory), so every card
that can ever reach this line is non-token by construction, the identical "holds by construction" reasoning
`untapStep`'s own `ValidStepTurnToController$` skip already documents ("`Trigger.phasesCheck` lands," above). Reset for
every player at `cleanupStep` (turn.go) alongside `LandsPlayed`/`CardsDrawnThisTurn`. 10 of the corpus's own real
`Mode$ Phase` lines resolve, ruin_lurker_bat.txt's own "at the beginning of your end step, if you descended this turn"
among them.

`Player.EnchantedBy` (14 real lines) and `Opponent.EnchantedBy` (2) stay unresolved -- an Aura enchanting a player
directly (CR 303.4h), which this port has no mechanism for at all: `castAura`'s own `enchantTargets` only ever offers a
battlefield permanent as a legal target, never a player. `Player.Chosen` (3) needs a "choose a player" ability this port
does not have. `Player.isMonarch` (1) needs a monarch tracker this port does not have -- the identical reason Layer 2's
own qualified `GainControl$ Player.isMonarch` stays unresolved (item 27).

While researching this chunk's own real corpus counts, two stale figures elsewhere in this port's own comments turned
out to be wrong and were corrected in place (DOC-16): `checkPhaseTriggers`'s own doc comment (trigger.go) attributed its
`Condition$` skip -- `SpellAbilityCondition`'s own separate switch, a key this pre-filter genuinely never matches on a
real `Mode$ Phase` line today -- the count "65" that actually belongs to a wholly different key,
`WerewolfTransformCondition$`/`WerewolfUntransformCondition$` (Innistrad's own day/night mechanic, unresolved for
`CardTraitBase.meetsCommonRequirements`'s own reason, "`Trigger.phasesCheck` lands," above); a real corpus check
confirms 0 lines carry the bare `Condition$` key on a `Mode$ Phase` line, and only 6 carry it at all corpus-wide, none
reachable through this port's own static trigger walk. `docs/crucible/00-master-implementation-plan.md`'s item 26 had
the identical figure in the identical place, fixed the same way.

Eight new tests, all in `triggerphases_test.go`: `TestAdvancePhaseFiresPhaseTriggerWhenEnchantedControllerMatches`/
`TestAdvancePhaseSkipsPhaseTriggerWhenEnchantedControllerDoesNotMatch` (an Aura's own controller and the enchanted
creature's controller deliberately different players, so the test isolates "did the trigger fire" from "who the Aura's
own `Execute$` benefits" -- this port does not thread a `checkPhaseTriggers`-set `TriggeredPlayer` back through
`Defined$` yet); `TestMoveSetsDescendedThisTurnForPermanentCardToGraveyard`/
`TestMoveDoesNotSetDescendedThisTurnForNonPermanentCardToGraveyard` (the `isPermanent` branch, an Instant proving the
negative);
`TestAdvancePhaseFiresPhaseTriggerWhenDescendedThisTurn`/`TestAdvancePhaseSkipsPhaseTriggerWhenNotDescendedThisTurn`;
`TestCleanupStepResetsDescendedThisTurn`. Every fire-side test was regression-checked by temporarily removing the
corresponding `matchesPlayerProperty` case or `Game.Move` hook and confirming the suite fails exactly as expected before
restoring it.

`enginelint.json`'s own `"valid"` group gained `"player"` in its allow-list -- `matchesPlayerProperty` reads
`g.Player(candidate).DescendedThisTurn` directly now, a type declared in player.go valid.go had never referenced before.

## `Draw`'s own `ReplaceWith$`: CR 616's "the event is replaced"

Every replacement shape this file resolved before now (`checkMovedReplacement`, `untapBlocked`, `damagePrevented`,
`drawPrevented`, `gainLifePrevented`) either applies a fixed outcome (`Tapped = true`, blocked, prevented -- CR 616's
own "more than one could apply" choice is moot because applying any of them twice is a no-op) or is a bare
`Prevent$ True` (the event simply does not happen). `ReplaceWith$` -- CR 616's own "the event is replaced by a DIFFERENT
event" -- is real content for the first time, for `Draw` alone.

The obvious design, resolving a `ReplaceWith$` target through `Registry.Resolve` (effect.go) the way `resolveSubAbility`
(subability.go) already chains a `SubAbility$`, does not work: `Registry.Resolve` needs a `*Registry`, and `DrawCards`'
own call chain (`drawStep`/`beginPhase`/`AdvancePhase`, turn.go, and `drawEffect.Resolve`, draweffect.go) has no way to
reach one without extending `AdvancePhase`'s own public signature -- which every existing `AdvancePhase` caller across
the whole test suite would then need to pass one to, a blast radius wildly disproportionate to unlocking a handful of
real corpus lines. `Effect.Resolve` itself also returns no error `DrawCards` (a `void` function) has anywhere to
propagate.

`drawReplaced` (replacement.go, new) sidesteps both problems the way `tapAbilityResolvesTap` (`## Replacement effects`,
below) already does for a different Event$: recognize a narrow, hand-verified-safe shape and run its own underlying
primitive directly, rather than going through the general effect machinery at all. Two shapes are recognized,
`applyDrawReplacement`'s own dispatch.

The first is a plain `DB$ Draw | Defined$ You | NumCards$ N`. `drawOneCard` (turn.go) runs it -- `DrawCards`' own
per-card loop body, factored out into its own primitive for this. The second is a plain
`DB$ PutCounter | CounterType$ X | CounterNum$ N | Defined$ Self`. `Card.Counters.Add` runs it directly, the identical
shape `putCounterEffect` already resolves, hand-run here instead of called.

Both refuse outright the moment the target ability itself names `SubAbility$` -- return false, meaning the line is
skipped and the original draw proceeds unreplaced. Chaining one needs the identical `*Registry` this call site cannot
reach, and running the leaf half alone while silently dropping the chained half would be a wrong answer, not an honest
gap (GO-7). blood_scrivener.txt's own real "draw two cards, then lose 1 life" is the corpus's own example of exactly
this shape.

`Defined$` on the target ability is read as "the player who would have drawn" directly (the `player` parameter already
threaded through `drawReplaced`) rather than through the general `definedPlayers` (defined.go): every real corpus line
needing this dispatch writes the literal token `You`, and `ValidPlayer$` (checked one level up, in `drawReplaced`
itself) already restricts the whole dispatch to the one case where `player` equals the host's own controller anyway, so
the two readings are the identical answer for every real line this covers -- not a general `Defined$` resolver, a narrow
one scoped to what `drawReplaced` alone needs.

The recursion question this design had to answer explicitly: thought_reflection.txt's own real "if you would draw a
card, draw two cards instead" -- what stops its own substitute draws from being checked against thought_reflection's OWN
line again, forever? Java's answer is `ReplacementHandler`'s own `hasRun` set (ReplacementHandler.java): the specific
`ReplacementEffect` object currently executing is excluded from `getReplacementList`'s own candidates while its own
substitute ability is still resolving, cleared only once that resolution fully returns. This port builds no equivalent
per-line guard at all -- instead, `drawOneCard` is called directly for the substitute's own N draws, never routing back
through `DrawCards`/`drawPrevented`/`drawReplaced` itself, so there is nothing left for thought_reflection's own line to
recheck against. The real, narrow cost: the substitute's own draws are not checked against ANY other replacement or
prevention effect on the battlefield either, not just the one that produced them -- a card with two independent
Draw-replacing permanents would see only the first apply, not both compounding the way CR 616 (and Java's own
`hasRun`-guarded recursion) actually allows. No real corpus deck combines two such permanents today, so this is not
observable against the corpus, and is flagged rather than silently accepted.

7 of the corpus's own 36 real `Event$ Draw | ReplaceWith$` lines resolve end to end: thought_reflection.txt's own bare
form (`ValidPlayer$ You`, target `DB$ Draw | Defined$ You | NumCards$ 2`); phial_of_galadriel.txt's own
`Hellbent$ True`-qualified identical shape ("while you have no cards in hand," resolved through
`replacementRequirementsCheck`'s own `triggerCommonRequirementsMet` fold-in with no code of its own needed);
ormos_archive_keeper.txt's own `IsPresent$ Card.YouOwn | PresentZone$ Library | PresentCompare$ EQ0`-qualified line,
whose own target is `DB$ PutCounter | CounterType$ P1P1 | CounterNum$ 5 | Defined$ Self` rather than a `Draw` at all
("if your library has no cards in it, instead put five +1/+1 counters on CARDNAME"); and 4 more resolved through
`NotFirstCardInDrawStep$`'s own gate, the next section below.

Not resolved, each for its own real reason found while dumping every real `Event$ Draw | ReplaceWith$` line and checking
what its own target ability actually needs: reed_richards_smartest_man.txt's own `FirstExtraCardDrawnThisTurn$` (1);
hullbreacher.txt's own `NotFirstCardInDrawStep$` shape, whose own target is `DB$ Token | TokenScript$ c_a_treasure_sac`
rather than `Draw`/`PutCounter` -- `CreateToken` is not a built `Effect` yet, unrelated to the gate the next section
resolves (1); magus_of_the_chains.txt's/chains_of_mephistopheles.txt's own `Defined$ ReplacedPlayer` plus
`SubAbility$ DBDraw` and breathstealers_crypt.txt's/sea_of_sand.txt's own `Defined$ ReplacedPlayer` plus `SubAbility$`
(4 combined -- "the player who would have drawn" as a general `Defined$` token, distinct from this dispatch's own narrow
`player`-parameter reading above, which only ever covers the literal token `You`); blood_scrivener.txt's own
`SubAbility$ DBLoseLife` alone (1, the chained-target refusal above); booby_trap.txt's own `ValidPlayer$ Player.Chosen`
(1, `matchesPlayerSpec`'s own doc comment) and pursuit_of_knowledge.txt's own `Optional$ True` (1, `Discard`'s own
`Optional$` gap). `alms_collector.txt`'s/`quantum_riddler.txt`'s own two real lines name `Event$ DrawCards` rather than
`Event$ Draw` -- Java's own OUTER, once-per-batch-call replacement check (`Player.drawCards`'s own
`ReplacementType.DrawCards` dispatch, checked before its per-card loop even starts, distinct from `ReplacementType.Draw`
inside `doDraw()` that every other real line in this file targets) -- a different granularity this port's own per-card
`DrawCards` loop has nowhere to hook, not attempted.

`GainLife`'s own 20 real `ReplaceWith$` lines split the same way: 15 target `DB$ ReplaceEffect`, a dedicated API this
port does not build, and the other 5 name `ReplaceCount$LifeGained` -- "the amount of life that would have been gained"
as a runtime value, distinct from an ordinary `Count$` expression `resolveAmount` evaluates. `## GainLife`'s own
`ReplaceWith$`, further below, resolves 4 of those 5 (nefarious_lich.txt's/lich.txt's/tainted_remedy.txt's/
plague_drone.txt's own real lines) once `resolveGainLifeReplacementAmount` reads that runtime value directly rather than
through `resolveAmount`; rain_of_gore.txt's own line stays unresolved for its own separate reason, that section's own
doc comment has it.

Seven new tests, all in a new `drawreplaced_test.go`: `TestDrawReplacedByBareReplacement`,
`TestDrawReplacedSetsDrewFromEmptyLibraryWhenItRunsOut` (the substitute's own draws are real draw attempts, not skipped
ones -- unlike `Prevent$`, running out of library partway through a replaced draw still sets `DrewFromEmptyLibrary`),
`TestDrawReplacedWhenHellbentConditionMet`/`TestDrawNotReplacedWhenHellbentConditionNotMet`,
`TestDrawReplacedWithPutCounterInstead`, `TestDrawNotReplacedByTargetAbilityWithSubAbility` (two cards in the library,
so a wrongly-permitted double-draw and a correctly-refused single draw are actually distinguishable -- the first draft
of this test used only one library card and passed either way, a self-caught weak assertion fixed before finalizing),
`TestDrawNotReplacedByUnrecognizedTargetAbility`. Every fire-side test and the `SubAbility$` refusal were
regression-checked by temporarily removing the corresponding code path and confirming the suite fails exactly as
expected before restoring it.

`enginelint.json`'s own `"replacement"` group gained `"control"`, `"amount"`, `"putcountereffect"`, `"parts"` and
`"event"` in its allow-list -- `drawReplaced`'s own hand-run dispatch reaches `PlayerController` (a type), and
`resolveNamedAmount`/`putCounterType`/`Counters`/`emitCounterChanged` (functions and a type), none of which
replacement.go referenced before.

## `Draw`'s own `NotFirstCardInDrawStep$`: "except the first" and `notion_thief.txt`'s own divert

`ReplaceDraw.canReplace` (Java) carries a second gate past `ValidPlayer$`/`ValidCause$`: `NotFirstCardInDrawStep$ True`
exempts exactly one draw from the whole line -- the very first card the affected player draws while the game's current
phase is that player's own Draw step. Java's own two-part check is `p.numDrawnThisDrawStep() == 0 && ownDraw`, where
`ownDraw` is "the current phase is Draw AND `p` is the active player" -- a later draw in the identical step, or any draw
of `p`'s outside `p`'s own Draw step entirely (an instant-speed effect during another player's turn, or during `p`'s own
non-Draw phase), is never exempt.

`Player.DrawnThisDrawStep` (player.go, new) is Java's own `numDrawnThisDrawStep` -- distinct from `CardsDrawnThisTurn`'s
whole-turn scope. `drawStep` (turn.go) resets it for every player at the start of each Draw step (`PhaseHandler.java`'s
own per-player reset loop, ported directly), and `drawOneCard` (turn.go) increments it for whichever player actually
draws, whenever the game's current phase is Draw -- Java's own unconditional `game.getPhaseHandler().is(PhaseType.DRAW)`
check, with no player comparison, so a non-active player who draws an extra card while the active player's own Draw step
is still current counts too. A skipped Draw step (CR 103.7a, the first player's own first turn in a two-player game)
resets nothing, matching Java's own case body never running when the step itself is skipped.

`notFirstCardInDrawStepExempts` (replacement.go, new) reads both fields to answer Java's own two-part question directly:
`g.activePhase == Draw && g.activePlayer == player` for `ownDraw`, `g.Player(player).DrawnThisDrawStep == 0` for the
count -- both must hold for the exemption to apply. `drawReplaced` calls it right after the `ValidPlayer$` match,
skipping the whole replacement line (the draw proceeds normally) when it returns true.

teferis_ageless_insight.txt's/alhammarrets_archive.txt's/bard_king_of_dale.txt's own real "except the first one you draw
in each of your draw steps, draw two cards instead" (`ValidPlayer$ You`) resolve through this gate onto the
already-built plain `DB$ Draw | Defined$ You | NumCards$ 2` target `drawReplaced` already knew how to run.

notion_thief.txt's own real "if an opponent would draw a card except the first one they draw in each of their draw
steps, instead you draw a card" (`ValidPlayer$ Opponent`) resolves through the identical gate but needed one more fix:
`applyDrawReplacementDraw`'s own `Defined$ You` used to be read as `player` directly -- the event's own affected player
-- because every real line it had ever covered also carried `ValidPlayer$ You`, making the two the same player by
construction. notion_thief's own `ValidPlayer$ Opponent` breaks that coincidence: "you" in the replacement's own text is
Notion Thief's controller, not the opponent whose draw is being replaced. `applyDrawReplacementDraw` now reads
`Defined$ You` as `host.Controller()` instead -- Java's own `AbilityUtils.getDefinedPlayers` resolves a replacement
ability's own "You" to its activating player, always the host card's controller, never the event's own affected player
-- which happens to be an identical answer for every previously-resolved line (their own `ValidPlayer$ You` already
forced `player == host.Controller()`) and the correct one for notion_thief's. `player` was consequently dropped from
`applyDrawReplacementDraw`'s and `applyDrawReplacement`'s own signatures -- both had it only for that one now-corrected
read.

hullbreacher.txt's own identical `NotFirstCardInDrawStep$`/`ValidPlayer$ Opponent` shape stays unresolved: its own
`ReplaceWith$` targets `DB$ Token | TokenScript$ c_a_treasure_sac`, and `CreateToken` is not a built `Effect` in this
port -- an unrelated gap the gate itself does nothing to close.

`drawReplacementMatches`'s own allow-list gained `"notfirstcardindrawstep"` -- without it every real line carrying the
param would be rejected outright (the general "any param besides these skips the whole line" rule, GO-7), never reaching
`notFirstCardInDrawStepExempts` at all. `enginelint.json`'s own `"replacement"` group gained `"phase"` in its allow-list
for the new `g.activePhase == Draw` reference (`Draw`, a `PhaseType` constant declared in phase.go).

Eight new tests in `drawreplaced_test.go`: `TestDrawNotReplacedOnFirstDrawOfOwnDrawStep`/
`TestDrawReplacedOnSecondDrawOfOwnDrawStep` (the exemption's own count half, `DrawnThisDrawStep` set directly to isolate
it from the increment), `TestDrawnThisDrawStepAdvancesAcrossConsecutiveDraws` (the increment actually runs, proven by
two real `DrawCards` calls in the same Draw step rather than poking the field),
`TestDrawnThisDrawStepResetsAtEachDrawStep` (drives a full `AdvancePhase` cycle from turn 2's Draw step to turn 3's,
proving the counter does not accumulate across turns), `TestDrawReplacedOutsideOwnDrawStepEvenOnFirstDraw` (the
`ownDraw` half alone: a draw during Main1 is never exempt even at count 0),
`TestDrawReplacedByNotionThiefShapeDivertsToHostController`/
`TestDrawNotReplacedByNotionThiefShapeOnOpponentsOwnFirstDraw` (the `Defined$ You` fix, and its own exemption mirror).
Every new gate -- the exemption function itself, the allow-list entry, the increment, the reset, and the
`host.Controller()` reading -- was regression-checked by temporarily disabling it and confirming the corresponding test
failed with the expected wrong count before restoring it.

## `GainLife`'s own `ReplaceWith$`: the one runtime value `Draw`'s dispatch never needed

`gainLifeReplaced` (replacement.go, new) is `drawReplaced`'s own sibling for CR 119's "gain life" event, built the
identical way for the identical reason -- a `*Registry` `gainLifeEffect.Resolve`'s own call chain cannot reach, a
`SubAbility$`-carrying target refused outright rather than run with the chained half dropped. What it resolves that
`drawReplaced` never had to: `ReplaceCount$LifeGained`, an `expr.Amount` with `Kind == Expression` and
`Head == "ReplaceCount"` that `resolveAmount` (amount.go) does not evaluate at all -- its own `Expression` case requires
`Head == "Count"` exactly, so this is not a gap in that function's own dispatch table so much as a completely different
question: not "what does the game currently look like," but "what would this specific event have done." Nothing else
this port has built needed that distinction before now.

A new `resolveGainLifeReplacementAmount` (replacement.go) answers it without touching `resolveAmount`/
`resolveNamedAmount` at all: it tries a literal integer first, then checks whether `amounts[value]` is itself a
`ReplaceCount`-headed `Expression` and, if so, returns `replaced` directly -- the raw `LifeAmount$`
`gainLifeEffect.Resolve` already computed for the event now being replaced, threaded through `gainLifeReplaced`'s own
parameter of the same name -- falling through to the ordinary `resolveNamedAmount` for every other named reference. This
is deliberately not a new case inside `resolveAmount` itself: every other caller of that function (a continuous effect's
own numeric param, a trigger's own `NumDmg$`, ...) has no "replaced amount" in scope at all, so threading one through
its general signature would mean carrying a meaningless zero value almost everywhere it is called -- narrower and
correct beats general and wrong here.

Two already-built leaf effects are recognized as `ReplaceWith$` targets, `applyGainLifeReplacement`'s own dispatch: a
plain `DB$ Draw | Defined$ You | NumCards$` naming a `ReplaceCount$LifeGained` SVar (`applyGainLifeReplacementDraw`,
`applyDrawReplacementDraw`'s own shape reused, calling the identical `drawOneCard` primitive) and a plain
`DB$ LoseLife | LifeAmount$` naming one | `Defined$ You` or `Defined$ ReplacedPlayer`
(`applyGainLifeReplacementLoseLife`). `ReplacedPlayer` -- "the player who would have gained the life this replaces" --
resolves to the `player` parameter directly, the identical narrow `Defined$` reading `applyDrawReplacementDraw`'s own
doc comment already gives for its own literal `You` case: every real corpus line needing this dispatch writes one of
these two tokens, and both mean the same player `gainLifeReplaced`'s own `ValidPlayer$` check already narrowed things
down to.

Both of these are CR 616's own full-substitution ("Replaced") outcome: the gain event never happens at all, something
else happens instead. `gainLifeReplaced` originally reported that outcome as a plain `bool`, `gainLifeEffect.Resolve`'s
own `if g.gainLifeReplaced(...) { continue }` skipping the normal gain entirely on a match -- adequate for exactly this
outcome, but with nowhere to put CR 616's OTHER outcome, "Updated" (the same gain event, a different number), which
`damageReplaced` (above) already carries for a different `Event$`. `DB$ ReplaceEffect | VarName$ LifeGained` lines -- 15
real ones, angel_of_vitality.txt's/rhox_faithmender.txt's/... own "double"/"plus 1" life-gain shapes -- are exactly that
outcome, so `gainLifeReplaced`'s own signature changed from `bool` to `int`: it now returns the amount that should
actually be gained -- `amount` itself, unchanged, when nothing matches (`NotReplaced`); `0` when a full substitution
matched (`applyGainLifeReplacement`, unchanged, now signaling "nothing left to gain" the identical way
`applyDamageReplaceCounter`'s own full substitution already does by returning 0); or the resized amount when
`applyGainLifeReplaceEffect` (new, below) matches. `gainLifeEffect.Resolve`'s own call site changed from
`if g.gainLifeReplaced(...) { continue }` to `gain := g.gainLifeReplaced(...); if gain <= 0 { continue }` -- one check
that folds Java's own two separate ones (`Player.gainLife`, `if (!canGainLife() || lifeGain <= 0) return false` before
running any replacement at all, and `if (lifeGain <= 0) return false` again right after) into the single point this
port's own call ordering needs it at, matching Java's behavior for both directions without adding a second explicit
guard.

`applyGainLifeReplaceEffect` (replacement.go, new) is `applyDamageReplaceEffect`'s own sibling for a `DB$ ReplaceEffect`
line naming `VarName$ LifeGained` instead of `VarName$ DamageAmount`, resolving `VarValue$` through the newly
generalized `resolveReplaceCountAmount` (renamed from `resolveDamageReplaceCountAmount`, its own doc comment above --
the function itself was untouched beyond taking `wantBody` as a parameter instead of hardcoding `"DamageAmount"`) read
against `"LifeGained"`. The real corpus shape is narrower than `DamageAmount`'s own: every one of the 15 real lines
carries exactly `Twice` (7 -- rhox_faithmender.txt's/the_wind_crystal.txt's/selenia_the_cursed_heart.txt's/
alhammarrets_archive.txt's/doctor_strange_surgeon.txt's/boon_reflection.txt's/phial_of_galadriel.txt's own real "gain
twice that much life instead") or `Plus.1` (8 -- angel_of_vitality.txt's/heron_of_hope.txt's/honor_troll.txt's/
cleric_class.txt's/bilbo_birthday_celebrant.txt's/knight_of_dawns_light.txt's/leyline_of_hope.txt's/ pest_rescuer.txt's
own real "gain that much life plus 1 instead") -- no `Thrice`/`HalfDown`/`Minus` among them, though
`resolveReplaceCountAmount` resolves those too, generically, for free. A target ability naming `SubAbility$` is refused
outright, the identical chained-target refusal `applyGainLifeReplacement`'s own doc comment already gives (no real
corpus line among the 15 needs it).

19 of the corpus's own 20 real `Event$ GainLife | ReplaceWith$` lines resolve end to end now: the 4 already named above
(`Draw`/`LoseLife`'s own full substitution) plus the 15 `DB$ ReplaceEffect` lines just described. Both real
full-substitution pairs also carry `AILogic$` (`LichDraw`/`LoseLife`) and every real `DB$ ReplaceEffect` pair carries
`AILogic$ DoubleLife` -- a pure AI hint this port's own resolution never reads, already on
`gainLifeReplacementMatches`'s own allow-list alongside the params `drawReplacementMatches` already tolerates.

Not resolved: rain_of_gore.txt's own real "if a spell or ability would cause its controller to gain life, that player
loses that much life instead" -- `ValidSource$ SpellAbility | SourceController$ True`, no `ValidPlayer$` at all, a
restriction on what CAUSED the event (was it a spell/ability, and is its own controller the one gaining) rather than who
the event affects, a shape neither `gainLifeReplacementMatches`' own allow-list nor any sibling function in this file
has ever needed to check before -- falls through and skips the whole line (GO-7) rather than guessing.

Eight new tests, all in `gainlifereplaced_test.go`: `TestGainLifeReplacedByDrawInstead` (LifeAmount$ 3 threading through
as exactly 3 cards drawn, not a hardcoded 1 -- the test that actually exercises `resolveGainLifeReplacementAmount`'s own
runtime-value reading rather than a coincidence), `TestGainLifeNotReplacedWhenValidPlayerDoesNotMatch`,
`TestGainLifeReplacedByLoseLifeInstead`, `TestGainLifeNotReplacedByUnrecognizedSourceRestriction` (rain_of_gore.txt's
own real shape), `TestGainLifeDoubledByReplaceEffect` (rhox_faithmender.txt's own real `Twice` shape, and the only one
of the eight that also checks `LifeGainedTimesThisTurn` advances -- proving a resize still runs the normal gain path,
unlike a full substitution), `TestGainLifePlusOneByReplaceEffect` (the `Plus` operator, not just `Twice`),
`TestGainLifeNotReplacedByChainedReplaceEffect`, `TestGainLifeNotReplacedByTargetAbilityWithSubAbility`. Every fire-side
test, both `SubAbility$` refusals, and the `ReplaceCount` branch of `resolveGainLifeReplacementAmount` itself were
regression-checked by temporarily removing the corresponding code path and confirming the suite fails exactly as
expected before restoring it.

## Replacement effects: entering the battlefield tapped

CR 614's own replacement-effect system (`forge-game/src/main/java/forge/game/replacement/`, 3,742 LOC across
`ReplacementHandler`/`ReplacementEffect`/44 per-event `ReplaceXxx` subclasses, `ReplaceMoved` among them) had zero real
content until now: item 26's own closing line named it "the whole replacement-effect system remains a gap." A corpus
tally (44 files, `Event$` on `R:` lines) puts it at 2,210 real lines across 1,646 cards -- comparable in size to the
trigger vocabulary above, and led by `Event$ Moved` (969, CR 614.1's own "replace a zone-change event") ahead of
`DamageDone` (218), `Untap` (158) and `Counter` (118).

`ReplaceWith$`'s own value distribution on `Event$ Moved` lines turned up the single largest atomic real shape in the
entire replacement-effect corpus: `ETBTapped` (617) and `LandTapped` (135) together are 752 of 969 Moved lines --
"enters the battlefield tapped," MTG's own most common rules text, unimplemented in this port until now. Reading each
named SVar's own body (`grep SVar:ETBTapped:`, six distinct bodies corpus-wide) found the real shape underneath the
name: `DB$ Tap | Defined$ Self | ETB$ True` (587 of 618 real ETBTapped lines) or the identical shape naming
`Defined$ ReplacedCard` instead (31, the "another permanent enters tapped" shape -- `ReplacedCard` is Java's own way of
naming "the card actually moving" when the replacement's own host is a different permanent). Six lines chain a
`SubAbility$` (a counter grant) and stay unresolved.

`LandTapped`'s own 140 real `DB$ Tap` lines carry a Condition-family param too -- Rootbound Crag's own checkland text,
"enters tapped unless you control a Mountain or a Forest" -- and are resolved now: `subAbilityConditionMet`
(`condition.go`, new) ports `SpellAbilityCondition.areMet`'s own gate, trimmed to the two shapes these lines actually
use (`ConditionPresent$`/`ConditionCompare$`, a zone-presence count, 106/103 of the 140; `ConditionCheckSVar$`/
`ConditionSVarCompare$`, a named-SVar comparison, 34/33) -- SpellAbilityCondition is a wholly different Java class from
`CardTraitBase.meetsCommonRequirements` a trigger already checks (`triggerCommonRequirementsMet`, above), gating an
_ability's own resolution_ rather than whether a trigger fires at all, but the two families share the identical param
shapes under different key names: `isPresentMatches`/`checkSVarMatches` (trigger.go) were both already parameterized, or
made so, by the exact key names each caller needs (`checkSVarMatches` gained `checkKey`/`compareKey`/`secondKey`
parameters once this became its second caller), so `subAbilityConditionMet` is a thin wrapper passing
`"ConditionPresent"`/`"ConditionCompare"`/`"ConditionDefined"`/`"ConditionZone"` and
`"ConditionCheckSVar"`/`"ConditionSVarCompare"`/`"OrOtherConditionSVarCompare"` rather than new logic.

A few of the 140 carry a param past that pair, and skip the whole line via `subAbilityUnresolvedParams` (condition.go)
and `tapAbilityResolvesTap`'s own allow-list (below) rather than tapping unconditionally and guessing wrong
(PORT-8/GO-7): applying half of "enters tapped unless you control a Mountain" would be a wrong answer, not a partial
one.

- 15 carry `SubAbility$`, a chained counter grant -- `resolveSubAbility`'s own chaining mechanism ("SubAbility chaining
  itself lands," further below) is specific to a stack-resolving `Ability`; a replacement effect applies inline, outside
  `Registry.Resolve` entirely, so it would need its own separate integration this port does not have.
- 7 carry `ConditionDefined$`, an arbitrary reference with no Defined$-to-objects resolver built.
- 2 carry `ConditionPlayerTurn$` or `ConditionPhases$`, each its own mechanic.

A third Condition-family shape, the plain `Condition$` flag (SpellAbilityCondition's own separate
Threshold/Metalcraft/... switch), carries zero real `DB$ Tap` lines and stays unresolved for the identical reason.

`checkMovedReplacement` (`replacement.go`, new) resolves the 618 unconditional and 116 of the 140 conditional lines. It
reads `Face.Replacements` (`compile.go`) -- M3's own compiled field, typed identically to `Face.Triggers`/`Face.Statics`
since `R:` lines share the exact `Key$ Value` grammar, compiled the same way, and never read by the engine before now.
`ReplaceWith` is already one of `subAbilityKeys` (compile.go), so `ReplaceWith$ ETBTapped` resolves to `Ability.Subs`
for free -- zero new compiler work, the SVar it names already sitting there the identical way a trigger's own `Execute$`
sub-ability does.

Checked against two sets of Replacements, the identical own/other split `checkETBTriggers`/`otherETBTriggerMatches`
(above) already established: the moved card's own (`ValidCard$ Card.Self`, 587 of 618) and every OTHER battlefield
permanent's (`ValidCard$ Creature.OppCtrl`/`Land.OppCtrl`/..., 31 of 618 -- a static "creatures your opponents control
enter tapped" effect). `Destination$`/`Origin$`, present on 624 and 2 of the real ETBTapped-named lines respectively,
are checked only when present (`replacementZoneMatches` -- unlike `trigger.go`'s own `hasZone`, which a trigger's
`isETBTrigger`/`isDiesTrigger` always require, `ReplaceMoved.java`'s own `hasParam` guard around each check means an
absent one is unrestricted, not a non-match). `origin` is threaded in from each of the three real "enters the
battlefield" call sites -- `permanentEffect`/`attachEffect` (castspell.go), `Game.PlayLand` (land.go) -- read off the
card's own `Zone` field before `Game.Move` changes it, the one piece of state `checkETBTriggers` never needed since no
trigger mode this port resolves reads a moved card's own `Origin$`.

Called right after `Game.Move`, before `checkETBTriggers`: CR 614.1 puts a replacement before CR 603's own triggers, so
a tapped-on-entry permanent must already be tapped by the time a "when this enters" trigger looks at it -- the identical
ordering concern `applyContinuousControl` running first among the six continuous appliers already proved matters (item
27, above), applied here between two different mechanisms instead of within one. Unlike a trigger match, no APNAP
ordering or collect-then-push step is needed: the one outcome this file produces, `Card.Tapped = true`, is idempotent,
so CR 616's own "more than one replacement effect could apply, the affected player chooses" procedure -- needing a
`PlayerController` hook this port does not have, the identical gap a single player's own multiple simultaneous triggers
already has (above) -- has no observable answer to get wrong here: the first match found in either loop is applied and
the search stops immediately.

`TestPlayLandEntersTappedViaReplacement`/`TestCastSpellCreatureEntersTappedViaReplacement` (replacement_test.go) prove
the two real call sites; `TestCheckMovedReplacementAppliesToOtherPermanentsEntering` proves the "other" half;
`TestPlayLandDoesNotEnterTappedWhenDestinationDoesNotMatch` and `TestCheckMovedReplacementSkipsSubAbilityChain` prove
two of the ways a line does not resolve at all; `TestCheckMovedReplacementTapsCheckland`/
`TestCheckMovedReplacementDoesNotTapChecklandWhenConditionUnmet` and
`TestCheckMovedReplacementTapsWhenConditionCheckSVarIsMet`/`TestCheckMovedReplacementDoesNotTapWhenConditionCheckSVarIsNotMet`
each prove a met/unmet pair for the two now-resolved Condition-family shapes.

Not resolved: `ETBTapped`/`LandTapped` naming a `SubAbility$`, `ConditionDefined$`, `ConditionPlayerTurn$`,
`ConditionPhases$` or `Condition$` itself (above); `ReplaceWith$ Exile`/`DBTap`/`DBExile`/`DoDay`/`PayBeforeETB`/...
(352 of the remaining 969 Moved lines); `Untap`'s and `DamageDone`'s own `ReplaceWith$`-driven remainders (above);
`Draw`'s and `GainLife`'s own `ReplaceWith$`-driven remainders (33 and 16 real lines respectively, 3 and 4 of their own
36 and 20 real totals resolved by `## Draw`'s own `ReplaceWith$` and `## GainLife`'s own `ReplaceWith$`, both further
below); every `Event$` value past `Moved`/`Untap`/`DamageDone`/`Draw`/`GainLife` (`Counter` -- CR 701.5's own "can't be
countered," not the +1/+1-counter mechanic, moot until this port builds a spell-countering effect to protect against;
`AddCounter`, `CreateToken`, `BeginPhase`, `GameLoss`, `ProduceMana`, ... -- most naming a
`DB$ ReplaceCounter`/`ReplaceEffect`-shaped special replacement-modifying ability this port does not have a general
mechanism for, distinct from an ordinary script effect); and CR 616's own general layering/ordering procedure entirely,
moot for every resolved shape's own idempotent outcome but real the moment a second resolvable replacement effect
produces a different one.

`enginelint.json` gained a `"replacement"` group (`replacement.go`), added to `"land"`'s and `"castspell"`'s own allow
lists (both now call `checkMovedReplacement`) and, like `"trigger"`/`"continuous"` before it, to its own allow list an
`"ability"` entry it does not actually depend on: `compile.Ability` collides textually with the top-level `Ability`
declared in `ability.go` the identical way `ControlEffect.Player` once collided with the `Player` type (item 27's own
`ControlMod` paragraph) -- `enginelint`'s own identifier scan cannot tell a qualified external reference apart from an
unqualified same-package one. A new `"condition"` group (`condition.go`) sits between it and `"trigger"`:
`"replacement"` and `"dealdamageeffect"` both gained `"condition"` (`subAbilityConditionMet`'s two real callers), and
`"condition"` itself gained `"trigger"` (`isPresentMatches`/`checkSVarMatches`, both parameterized by key name once this
became their second caller) and the identical `"ability"` collision entry `compile.Ability` forces everywhere it
appears.

### `Event$ Untap`: CR 502.3/614.17's own "doesn't untap"

`untapStep`'s own doc comment (`turn.go`) had named this gap exactly the way `block.go`'s own named CantBlockBy:
"Effects that skip a permanent's untap (CR 502.3) are not modeled -- no card can grant that yet -- so every permanent
the active player controls untaps unconditionally." A corpus tally (`Event$ Untap` on `R:` lines) puts it at 158 real
lines, the corpus's own third most frequent `Event$` value behind `Moved` and `DamageDone`.

156 of those 158 carry `Layer$ CantHappen` -- CR 614.17's own dedicated layer, checked by `Card.canUntap`'s own
`cantHappenCheck` before `Untap.doUntap`'s own filter
(`untapList = CardLists.filter(untapList, c -> c.canUntap(active, false))`) ever considers a permanent for untapping at
all -- `ReplaceUntap.canReplace` ported directly: `ValidCard$` (present on every real line) matched against the affected
card, and `ValidStepTurnToController$` (154 of 156, always `"You"`) matched against the untap-step's own active player
relative to the affected card's controller. The other 2 name `ReplaceWith$` instead, a genuine substitution -- not the
"the event doesn't happen" shape `Layer$ CantHappen` itself already is -- and stay unresolved.

Reading `Untap.java`'s own `doUntap` closely turned up why `ValidStepTurnToController$` needs no separate check here at
all: its main loop (`for (final Card c : untapList)`, `untapList` built from `active.getCardsIn(Battlefield)`) only ever
considers permanents the untap-step's own active player already controls, so "the untapping player is this card's own
controller" -- `"You"`'s own contract -- already holds by construction for every real value the param carries.
`untapStep` (turn.go) has the identical invariant (`g.Zone(Battlefield, g.activePlayer).Cards()`), so `untapBlocked`
(`replacement.go`, new) checks only `Layer$`/`ValidCard$`/`ActiveZones$` and leaves `ValidStepTurnToController$` unread
-- reading it would just re-check something the loop already guarantees. `Untap.java`'s own second loop
(`active.getAllOtherPlayers().getCardsIn(Battlefield)`, granting permission to untap an opponent's permanent via
`StaticAbilityUntapOtherPlayer`) is the one case where the param would matter, and is not built: no card grants that
permission in this port yet, the identical reason `untapStep`'s own prior doc comment already gave for the whole gap.

`untapBlocked` walks every player's own Battlefield and Command zones (`replacementZones`, new -- 105 of 156 real lines
name `ActiveZones$ Battlefield` explicitly, the corpus's own default when absent; 2 name `Command`, an emblem-shaped
host rather than a permanent) looking for a match against the card about to untap, the identical own/other split every
replacement and trigger check in this port already has, collapsed here into one loop since a lock's own host is never
the card it locks. The first match found blocks the untap outright and the search stops -- CR 616's own "more than one
could apply" choice producing the identical outcome (blocked) no matter which is picked, `checkMovedReplacement`'s own
reasoning applied to a different outcome. `IsPresent$` (4) now resolves too, folded in through
`replacementRequirementsCheck` ("`ReplacementEffect.requirementsCheck` lands," above) --
alirios_enraptured.txt's/merseine.txt's/cocoon.txt's/winters_rest.txt's own real zone-scan conditions among them.
`CheckSVar$`/`SVarCompare$` (1, walking_dream.txt's own "if an opponent controls two or more creatures") also reaches
the general gate now rather than being skip-listed, but does not actually produce the CR-correct answer: its own `X`
SVar is `PlayerCountOpponents$HighestValid Creature.YouCtrl`, a distinct SVar family `resolveNamedAmount` does not
evaluate (the `Count$Valid` family `resolveAmount` resolves is a different one), so `checkSVarMatches` fails closed
(condition never met) and this one card's own lock never actually blocks anything -- the identical observable behavior
it had before this chunk (skip-listed then, unresolvable-and-thus-unmet now), just reached through the honest general
mechanism instead of an ad-hoc allow-list rejection. `EnduringStory$`/`AddSVar$` (2 of 156, each its own further
restriction this file cannot evaluate) still skip the whole line rather than blocking unconditionally (PORT-8/GO-7) --
an allow-list of exactly the params real corpus lines pair with this shape
(`Event$`/`Layer$`/`Description$`/`ValidCard$`/`ValidStepTurnToController$`/`ActiveZones$`/`Secondary$`, plus the keys
`replacementRequirementsCheck` itself reads), the identical style `tapAbilityIsPlainTap` already has, rather than a
reject-list of the ones found: a param neither list has seen yet skips by construction instead of silently passing.

`untapStep` (turn.go) calls `untapBlocked` per permanent before clearing `Tapped`, leaving `SummonSick`'s own clearing
unconditional: CR 502.3's own "doesn't untap" restricts only the untapping action, not CR 302.6's own continuous-control
question a doesn't-untap lock has nothing to do with.

`TestUntapBlockedBySelfCantHappenReplacement` proves the simplest real shape and the SummonSick independence;
`TestUntapBlockedByOpponentsCantHappenReplacement` proves the "other" half; `TestUntapBlockedWhenHostInCommandZone`
proves the `ActiveZones$` generalization; `TestUntapNotBlockedWhenValidCardDoesNotMatch` and
`TestUntapNotBlockedByReplaceWithShape` (replacement_test.go) prove two of the ways a line does not block;
`TestUntapBlockedWhenIsPresentConditionMet`/`TestUntapNotBlockedWhenIsPresentConditionNotMet` prove the new `IsPresent$`
fold-in both ways ("`ReplacementEffect.requirementsCheck` lands," above, has the full account).

### `Event$ DamageDone`: CR 614's own "prevent all of this damage"

218 real `Event$ DamageDone` lines, the corpus's own second most frequent `Event$` value. 72 name `Prevent$ True`; the
other 146 name `ReplaceWith$`, split by the target's own underlying `DB$` API (each counted only when a literal
top-level `R:` line actually names it, `face.Replacements`' own walk): `DB$ ReplaceEffect` (71 total, split by its own
`VarName$` -- 59 name `VarName$ DamageAmount`, raphael_the_muscle.txt's own "double all damage" among them, a
doubling/adding/subtracting-a-computed-amount-from-the-in-flight-`DamageAmount$` API, its own section below; the other
12 name `VarName$ Affected`/`LifeGained`/`Number`/`Ignore` instead, an entirely different substitution -- redirecting
who is damaged or what else changes, not resizing the damage itself -- still unresolved), `DB$ ReplaceDamage` (27,
"prevent N of that damage" -- a flat reduction, not a doubling; its own section below), `DB$ RemoveCounter` (17) and
`DB$ PutCounter` (14, "damage becomes counters" instead, 31 combined -- its own section below too), and a dozen smaller
shapes (`DB$ Mill`/`ChangeZone`/`Dig`/`ImmediateTrigger`/`RollDice`/`Draw`/`Token`/`Sacrifice`/`ChoosePlayer`/
`GainLife`/`HealDamage`, 1-3 lines each) -- no shape past those three anywhere near `ETBTapped`'s own 618-line
concentration, so none of the rest was worth building on its own; those 49 stay unresolved.

`Prevent$ True` needed no sub-ability at all to resolve, `ReplacementHandler.java`'s own dispatch read directly: a
`Prevent$ True` (or `PreventionEffect$`) line returns `ReplacementResult.Prevented` straight off `getParam("Prevent")`,
nothing replaces the event, it simply does not happen -- the identical "the event doesn't happen" shape `Event$ Untap`'s
own `Layer$ CantHappen` already is, for a different `Event$` value. `ReplaceDamage.canReplace` is the resolvable half
read for its own `canReplace` gate -- `ValidSource$`/`ValidTarget$` matched the identical way `damageDoneMatches`'s own
pair already is (trigger firing, above), split into a `*Card`/ `*Player` pair for the reason
`checkDamageDoneTriggersToCard`/`ToPlayer` already are (`damagePrevented`/ `damagePreventedPlayer`, `replacement.go`,
new), and `IsCombat$` compared against a hardcoded `true` the identical way every real `DamageDone` trigger check
already does, since nothing outside combat deals damage in this port yet. `PlayerTurn$` (4), `CheckSVar$`/`SVarCompare$`
(2) and `IsPresent$` (1) now resolve too, the identical `replacementRequirementsCheck` fold-in `untapReplacementMatches`
above just got --
guardian_naga_banishing_coils.txt's/gideon_blackblade.txt's/frodo_determined_hero.txt's/personal_sanctuary.txt's own
real "can't be dealt damage during your turn" lines among the `PlayerTurn$` four. `DamageAmount$` (1 of 72 --
callous_giant.txt's own "if a source would deal 3 or less damage to CARDNAME, prevent that damage") resolves too now,
reusing `damageAmountMatches` (trigger firing, above) against the ORIGINAL amount about to be dealt -- the identical
operator/operand split a trigger's own `DamageAmount$` already reads once damage actually happens, just consulted here
before the replacement decides whether to apply at all. The remaining 2 of 72 stay unresolved: one names
`ValidCause$`/`CauseIsSource$` together (a SpellAbility comparison `Matches` cannot evaluate), the other
`RelativeToSource$` (a GameEntity-vs-GameEntity relative match this port has no evaluator for) -- each its own further
restriction `ReplaceDamage.canReplace` itself reads but this file cannot, still skipping the whole line via the
identical allow-list style `untapReplacementMatches` above has, rather than preventing unconditionally.

`dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go) call `damagePrevented`/`damagePreventedPlayer` first, before
marking any damage, emitting `DamageDealt`, or checking CR 603's own "deals damage" trigger -- a prevented damage
instance never happened at all, the identical "look at the event before it happens" ordering CR 614.1 already has over
CR 603 for `checkMovedReplacement`, applied here to a different `Event$` value and a different outcome (nothing, rather
than `Tapped = true`).

`TestDamageToPlayerPreventedByReplacement`/`TestDamageToCreaturePreventedByReplacement` (replacement_test.go) prove the
`*Player`/`*Card` split; `TestDamageToPlayerNotPreventedWhenValidTargetDoesNotMatch`/
`TestDamageToCreatureNotPreventedWhenValidSourceDoesNotMatch` prove `ValidTarget$`/`ValidSource$` are checked, not
assumed; `TestDamageToCreatureNotPreventedByUndefinedCheckSVar` proves an unresolvable `CheckSVar$` reference fails
closed rather than preventing; `TestDamageToPlayerPreventedWhenPlayerTurnMatches`/
`TestDamageToPlayerNotPreventedWhenPlayerTurnDoesNotMatch` prove the new `PlayerTurn$` fold-in both ways;
`TestDamageToCreaturePreventedWhenDamageAmountGateMatches`/`TestDamageToCreatureNotPreventedWhenDamageAmountExceedsGate`
prove the new `DamageAmount$` gate both ways.

#### `DB$ ReplaceDamage`: CR 616's own "Updated" outcome, "prevent N of that damage"

`ReplaceDamageEffect.resolve` (Java) has two outcomes past `Prevent$ True`'s own "the event does not happen": if
reducing the incoming amount by `Amount$ N` brings it to zero or below, the event is `Replaced` -- the identical "no
damage happens" outcome `Prevent$ True` already gives; otherwise it is `Updated` -- the SAME `DamageDealt` event still
happens, marking/`checkDamageDoneTriggersToCard`/`ToPlayer`/emitting the event all still run, just against the smaller
number. Neither outcome existed in this port before now: `damagePrevented`'s own contract is a plain bool, nothing in
between "the whole event happens" and "none of it does."

`damageReplaced`/`damageReplacedPlayer` (`replacement.go`, new) are `damagePrevented`'s/`damagePreventedPlayer`'s own
siblings, called from `dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go) right after them, reassigning the same
`amount` local both functions already thread through to marking/the event/the trigger check below -- a trigger matching
`DamageAmount$` (`damageAmountMatches`, trigger firing above) sees the number damage was actually reduced to, not the
number that would have applied before any shield intervened, CR 616's own ordering (replacement effects apply before the
event, so before anything downstream ever observes it). Each guards `amount <= 0` immediately after the reduction the
identical way the top of `dealPermanentDamage` already guards the original `amount`, folding a full reduction into the
identical early return `damagePrevented` already gives -- no separate `Replaced` state to carry.

Of the corpus's own 39 real `DB$ ReplaceDamage` SVar definitions, only 27 are ever named by a literal top-level `R:`
line at all -- `damageReplacementMatches`' own `face.Replacements` walk, the identical mechanism every other dispatch in
this file already uses, can only ever see these 27. 18 of them resolve end to end through `damageReplacementMatches`
(`DamageDone`, `ReplaceWith$` present, otherwise the identical
`ActiveZones$`/`ValidSource$`/`IsCombat$`/`DamageAmount$`/ `replacementRequirementsCheck` gate `damagePreventionMatches`
already has) and `applyDamageReplaceDamage` (the `applyDrawReplacement`-style "recognize the one shape, run it by hand"
dispatch, since neither call site can reach the `*Registry` chaining a `SubAbility$` would need): a plain integer
`Amount$`, no `SubAbility$` and no `DivideShield$` (a shield's own remaining capacity split across more than one
simultaneous instance of damage in the same resolution, a fold this dispatch keeps no state for) --
guardian_seraph.txt's/orbs_of_warding.txt's/the five Sphere-of-\*.txt's own real "prevent N of that damage" lines among
them, plus reidane_god_of_the_worthy_valkmira_protectors_shield.txt's/plated_pegasus.txt's own
`ValidTarget$ You,Permanent.YouCtrl`/`Permanent,Player` (`matchesPlayerSpec`'s own comma-OR split, below, closes their
own player-target half). The remaining 9 of the 27 name a named-SVar `Amount$` this dispatch cannot resolve
(`ShieldAmount`/`X`/`PaidAmount`/`AlchemicX`, each its own further mechanic -- a depleting shield counter that rewrites
its own `Number$` body downward as it absorbs hits, an X spent casting the spell, mana paid for an activated ability,
...), one of them (rock_hydra.txt's) also chaining its own `SubAbility$`.

The other 12 of the 39 corpus-wide `DB$ ReplaceDamage` definitions are never named by any literal `R:` line at all, so
this dispatch's own walk cannot reach them regardless of its own shape -- each blocked by a wholly different missing
mechanism: hedron_field_purists.txt's own 2 (`DBReplace1`/`DBReplace2`, one per Level-up tier) are referenced only
through a Layer 6 `AddReplacementEffect$` on a `Mode$ Continuous` line (`applyContinuousKeyword`'s own doc comment,
above, resolves `AddKeyword$` only, not this key), so the replacement text never becomes a compiled `Replacements` entry
at all. The other 10 (forcefield.txt's/ajani_steadfast.txt's/urza_academy_headmaster.txt's/
divine_deflection.txt's/refraction_trap.txt's/healing_grace.txt's/barbed_wire.txt's/dark_sphere.txt's/
tornellan_protector.txt's/torrent_of_lava.txt's own) are created dynamically at resolution time by an activated or
triggered ability's own `DB$ Effect | ReplacementEffects$ <SVar>` (CR 611.2c/113.6's "create an effect with an ability"
-- forcefield.txt's own real `{1}: The next time an unblocked creature... prevent all but 1 of that damage` is the
corpus's own example: an activated ability creates a temporary `Effect` object carrying the replacement text as one of
its own params, not as a card's own printed `R:` line) -- this port does not model `Effect` objects or `DB$ Effect`
itself at all (M6's own remaining script-effect territory), so there is no compiled card anywhere for
`face.Replacements` to have found this text on in the first place.

`TestDamageToPlayerReducedByReplaceDamage`/`TestDamageToCreatureReducedByReplaceDamage` (replacement_test.go) prove the
`*Player`/`*Card` split for the reduction itself; `TestDamageToPlayerReductionClampsAtZero` proves a reduction bigger
than the incoming damage stops at zero rather than going negative; `TestDamageToCreatureNotReducedByChainedSubAbility`/
`TestDamageToPlayerNotReducedByDivideShieldAmount` prove the two refusals (GO-7);
`TestDamageToPlayerNotReducedWhenValidSourceDoesNotMatch` proves `ValidSource$` gates the ReplaceWith$ shape the
identical way it already gates `Prevent$ True`. Every new gate was regression-checked by temporarily disabling it and
confirming the corresponding test failed with the expected wrong number before restoring it.

#### `matchesPlayerSpec`'s own comma-OR split -- closing `DamageDone`'s own mixed player/card `ValidTarget$`

`Player.isValid` (Java) splits its own restriction string on comma the identical way `Card.isValid` does --
`CardTraitBase.java:261`, the same split `internal/valid.Parse` already gives the `*Card` side (`valid.go`'s own doc
comment). `matchesPlayerSpec` (`valid.go`) never did: it cut the whole spec on the first `.` and looked the resulting
base up directly, so a comma-joined spec like `You,Permanent.YouCtrl` never matched `matchesPlayerBase`'s own literal
`You`/`Opponent`/`Player` table at all -- the whole string, comma included, was the "base." Every one of
`matchesPlayerSpec`'s nine callers across `trigger.go`/`replacement.go`/`continuous.go`/`targeting.go` inherited the gap
for free.

The real shape only ever mixes one player-shaped alternative with one card-shaped alternative in the same OR
(`reidane_god_of_the_worthy_valkmira_protectors_shield.txt`'s/`plated_pegasus.txt`'s own `DB$ ReplaceDamage` lines,
`gratuitous_violence.txt`'s own `DB$ ReplaceEffect` line, all three `DamageDone`'s own `ValidTarget$`, the paragraph
above/below) -- "prevent/double damage dealt to you or a permanent you control," "... to a permanent or player." Fixed
by splitting `spec` on comma inside `matchesPlayerSpec` itself, evaluating each alternative independently and OR-ing the
results: an alternative whose base is not `You`/`Opponent`/`Player` matches nothing rather than aborting the whole spec
-- `valid.Parse`'s own contract for a base it does not recognize (valid.go's own doc comment: "a base it does not
recognise simply matches nothing"), reused here rather than invented, since a card-shaped alternative like
`Permanent.YouCtrl` is not a player spec this port fails to understand, it is a spec for a different kind of object
entirely, the reason the OR combines the two in the first place. `ok` stays false only when no alternative matches and
at least one player-shaped alternative names a property `matchesPlayerProperty` does not recognize -- GO-7's usual "skip
rather than guess," now scoped to the alternative that actually needs it instead of the whole spec.

For a spec with no comma at all -- every existing caller's every existing test, all nine call sites' every real corpus
line still resolvable before this change -- the loop runs exactly once over the whole spec, identical to the prior
single `Cut`/`matchesPlayerBase` pair: no behavior changes for a spec this port already resolved.

`TestDamageToPlayerReducedByReplaceDamageWithCommaValidTarget` (replacement_test.go) proves reidane's/plated_pegasus's
own real `You,Permanent.YouCtrl` shape now prevents 1 of 3 combat damage dealt to a player, not just to a permanent;
`TestDamageToPlayerDoubledByReplaceEffectWithCommaValidTarget` proves gratuitous_violence.txt's own real
`Permanent,Player` shape now doubles combat damage dealt to a player too -- a real correctness fix, not just a new
resolution, since that card's own `ValidTarget$` was already counted resolved for its card-target half
(`DB$ ReplaceEffect`, below) and was silently never doubling a hit against a player at all before this. Both were
regression-checked by reverting `matchesPlayerSpec` to its own pre-split form and confirming each failed with the
pre-fix (undoubled/unreduced) life total before restoring it.

#### `DB$ ReplaceEffect`: CR 616's own "Updated" outcome, a computed replacement

`ReplaceEffect.java`'s own `resolve` has several `VarType$`-selected branches (Card/Player/GameEntity/Map/CardSet, each
redirecting a different replacing-object field to a different value); the default branch, taken when `VarType$` is
absent, is the one this dispatch resolves: `params.put(varName, AbilityUtils.calculateAmount(card, varValue, sa))`,
recomputing whichever field `VarName$` names as a plain number. For `VarName$ DamageAmount` specifically, that number is
the SAME `DamageAmount$` this dispatch's own two callers (`damageReplaced`/`damageReplacedPlayer`, above) already have
in scope as the pre-replacement amount -- CR 616's own "Updated" outcome again, computing a new number rather than
skipping the event (`Prevent$ True`) or reducing it by a flat amount (`DB$ ReplaceDamage`, above). `VarValue$` is either
a plain integer (a flat replacement, ignoring the original amount entirely) or a named SVar whose own body is
`ReplaceCount$DamageAmount/<operator>` -- `AbilityUtils.calculateAmount`'s own `ReplaceCount$` branch
(`root.getReplacingObject(AbilityKey.fromString(l[0]))`, the game's own replacing-object map; the pre-replacement amount
already in scope stands in for it directly) feeding `AbilityUtils.doXMath` for the operator suffix.
`resolveReplaceCountAmount`/`applyDamageReplaceEffect` (`replacement.go`, new) port the five `doXMath` branches every
real corpus line pairs with this shape: `Twice`/`Thrice` (multiply), `HalfDown` (integer-divide by 2, safe since damage
is never negative), and `Plus`/`Minus` (a literal digit or a further-resolvable SVar operand, `resolveNamedAmount`
reused the identical way `applyDamageReplaceDamage`'s own `Amount$` already is) -- no other `doXMath` operator
(`HalfUp`, `ThirdUp`/`Down`, `Negative`, `Times`, `Pow`, `Divide*`, `Mod`, `Abs`, `LimitMax`/`Min`) pairs with a real
`ReplaceCount$DamageAmount` line. The result is clamped at 0 defensively, matching `applyDamageReplaceDamage`'s own
contract, though every real caller already guards the original amount positive before either dispatch runs.

Of the 59 real lines naming `DB$ ReplaceEffect | VarName$ DamageAmount`, 56 resolve end to end:
raphael_the_muscle.txt's/ gratuitous_violence.txt's/furnace_of_rath.txt's/... own real "double all damage" (`Twice`, the
corpus's own dominant shape), torbran_thane_of_red_fell.txt's own "plus 2" (`Plus.2`),
benevolent_unicorn.txt's/lashknife_barrier.txt's own "minus 1" (`Minus.1`), ghosts_of_the_innocent.txt's own "half,
rounded down" (`HalfDown`), fiery_emancipation.txt's/ city_on_fire.txt's own "triple" (`Thrice`), and
forethought_amulet.txt's/divine_presence.txt's own flat "deals N damage instead" (a plain integer `VarValue$`, gated by
the R: line's own `DamageAmount$` threshold matching the ORIGINAL amount -- the identical `DamageAmount$` gate
`damagePreventionMatches`'s own new fold-in just added, `damageReplacementMatches` above, folded in here too since these
two real lines are the only `ReplaceWith$` shape that needs it). 3 stay unresolved:
fated_firepower.txt's/hawkeye_young_avenger.txt's own `Plus.Y` operand (`Count$CardCounters.FIRE`/`Count$CardPower`,
neither the Valid family `resolveAmount` evaluates -- an amount head this port has no evaluator for, not anything
specific to this dispatch) and ojer_axonil_deepest_might_temple_of_power.txt's own bare `Count$CardPower` `VarValue$`
(no `ReplaceCount$` at all -- damage set equal to the host's own power, not measured off the original amount at all, the
identical unsupported `Count$CardPower` head). The other 12 of the 71 corpus-wide `DB$ ReplaceEffect` references naming
`VarName$` something other than `DamageAmount` (`Affected`/`LifeGained`/`Number`/`Ignore`) are filtered out by requiring
`VarName$ DamageAmount` specifically, not resolved by anything in this file.

`TestDamageToPlayerDoubledByReplaceEffect`/`TestDamageToCreatureTripledByReplaceEffect` (replacement_test.go) prove the
`*Player`/`*Card` split for `Twice`/`Thrice`; `TestDamageToPlayerReplaceEffectPlusLiteral`/
`TestDamageToPlayerReplaceEffectMinusClampsAtZero`/`TestDamageToPlayerReplaceEffectHalfDown` prove the remaining three
operators, the Minus case also proving the same zero-clamp `TestDamageToPlayerReductionClampsAtZero` already proves for
`DB$ ReplaceDamage`; `TestDamageToPlayerReplaceEffectFlatReplacementAboveGate`/
`TestDamageToPlayerReplaceEffectFlatReplacementBelowGateNotApplied` prove the flat-integer shape and its own
`DamageAmount$` gate both ways; `TestDamageToPlayerReplaceEffectUnresolvedOperandNotApplied` proves an unresolvable
`Plus` operand refuses outright (GO-7) rather than treating it as zero. Every new gate and branch was regression-checked
the identical way `DB$ ReplaceDamage`'s own were.

#### `DB$ RemoveCounter`/`DB$ PutCounter`: CR 616's own "Replaced" outcome

A third real `ReplaceWith$` shape, and a different CR 616 outcome from either of the first two:
`ReplacementHandler.java`'s own dispatch (line ~360) sets `ReplacementResult.Updated` only for `ApiType.ReplaceDamage`/
`ReplaceSplitDamage`/`ReplaceEffect` (`damageReplaced`'s/`applyDamageReplaceEffect`'s own outcomes, above) or
`ReplaceToken`/`ReplaceMana` (not built) -- every other `ApiType`, `RemoveCounter`/`PutCounter` among them, gets the
default `ReplacementResult.Replaced`: the ORIGINAL `DamageDone` event does not happen at all (no marking, no event, no
trigger check), a counter changes on some object INSTEAD -- the identical "different thing happens, not a smaller
version of the same thing" shape `drawReplaced`'s/`gainLifeReplaced`'s own substitutions already have, not
`damageReplaced`'s/`applyDamageReplaceEffect`'s own reduced-or-computed-but-still-damage shape.
`applyDamageReplaceCounter` (`replacement.go`, new) reports whether a recognized shape ran; `damageReplaced`/
`damageReplacedPlayer` return `amount` 0 when it does, the same "nothing left to mark" outcome a full
`DB$ ReplaceDamage` reduction already folds into.

`Defined$` resolves three ways: `Self` (the replacement's own host -- the dominant real shape, every "Phantom"
creature's own real "if damage would be dealt to CARDNAME, prevent that damage. Remove a +1/+1 counter" among them, 10
real cards sharing the identical line); `Equipped` (panther_habit.txt's own real "if equipped creature would be dealt
damage, prevent that damage and put that many +1/+1 counters on it," `Card.AttachedTo()` -- `applyContinuousNames`'s own
precedent, reused outright); and `ReplacedTarget` (the object the damage would have hit, threaded straight through from
`damageReplaced`'s/`damageReplacedPlayer`'s own `target` parameter -- soul_scar_mage.txt's own real "if a source you
control would deal noncombat damage to a creature an opponent controls, put that many -1/-1 counters on that creature
instead," the counters landing on the DAMAGED creature, not the replacement's own host). `CounterType$` reuses
`putCounterType` (`putcountereffect.go`) outright -- `RemoveCounter`/`PutCounter` share the identical param.
`CounterNum$` resolves through `resolveReplaceCountAmount` (above), now generalized to accept a bare, operator-less
`ReplaceCount$DamageAmount` -- `AbilityUtils.doXMath`'s own `operators == null` identity, `original` unchanged -- the
dominant real shape for THIS dispatch specifically (lichenthrope.txt's/phytohydra.txt's own real `CounterNum$ X`,
`SVar:X:ReplaceCount$DamageAmount`, among 20 of the 25 resolvable lines), distinct from `applyDamageReplaceEffect`'s own
suffixed `Twice`/`Plus`/... branches, which 0 real lines here pair with. `SubAbility$` refuses outright, the identical
chained-target refusal every other hand-run dispatch in this file already gives (5 real lines, underdark_beholder.txt's
own "remove counters, then sacrifice if none left" among them). `AlwaysReplace$`/`ExecuteMode$` (real on several of
these 25 lines) are allow-listed in `damageReplacementMatches` as pure no-ops: `ReplacementHandler.java`'s own dispatch
only reads `AlwaysReplace$` when `NoPreventDamage` is set on the runParams -- a "damage can't be prevented" flag this
port's own damage pipeline has no equivalent state for at all -- and `ExecuteMode$`'s own `PerSource`/`PerTarget` split
only matters when Java batches more than one simultaneous damage instance into one replacement pass, something this
port's own `dealPermanentDamage`/`dealPlayerDamage` never do (each call is already exactly one source and one target).

25 of the corpus's own 31 real `Event$ DamageDone` lines naming `DB$ RemoveCounter`/`DB$ PutCounter` resolve end to end.
6 stay unresolved: the 5 chaining `SubAbility$` above, and jared_carthalion_true_heir.txt's own real R: line naming
`CheckDefinedPlayer$ You.isMonarch` -- no monarch mechanic this port tracks (`GainLife`'s own `Player.isMonarch` gap,
above) -- skipped by `damageReplacementMatches`'s own allow-list (which never lists `CheckDefinedPlayer` at all) before
`applyDamageReplaceCounter` is ever reached; `triggerCommonRequirementsMet`'s own pre-existing `CheckDefinedPlayer`
skip-list entry (`## CardTraitBase.meetsCommonRequirements`, above) would refuse it a second time even if the outer
allow-list somehow let it through, confirmed by disabling both gates together during regression-checking and watching
the test fail for the expected reason.

`TestDamageToCreatureReplacedByRemoveCounter` proves the "Phantom" shape (`Defined$ Self`);
`TestDamageToPlayerReplacedByPutCounterUsingBareReplaceCount` proves the bare `ReplaceCount$DamageAmount` read;
`TestDamageToCreatureReplacedByPutCounterOnReplacedTarget`/`TestDamageToCreatureReplacedByPutCounterOnEquipped` prove
`Defined$ ReplacedTarget`/`Equipped`; `TestDamageToCreatureNotReplacedByRemoveCounterWithSubAbility` proves the
`SubAbility$` refusal; `TestDamageToPlayerNotReplacedByPutCounterNamingCheckDefinedPlayer` proves the
`CheckDefinedPlayer$` gap end to end against the real corpus card's own shape. Every new gate was regression-checked by
temporarily disabling it and confirming the corresponding test failed with the expected wrong `Damage.Marked`/counter
count before restoring it.

All five shapes -- `Prevent$ True`, both halves each of `ReplaceWith$ DB$ ReplaceDamage`/`DB$ ReplaceEffect`, and
`DB$ RemoveCounter`/`DB$ PutCounter` -- share `replacementActiveZones`/`hostInActiveZones` (`replacement.go`),
generalizing `ActiveZones$` past Battlefield alone -- absent means Battlefield, a comma list otherwise, an unrecognized
zone name skipping the whole line the identical contract `validCountZones` (amount.go) already has for a
`Count$Valid<Zone>` suffix -- `replacementZones`, the `[Battlefield, Command]` pair `untapBlocked`/`damagePrevented`/
`damagePreventedPlayer`/`damageReplaced`/`damageReplacedPlayer` all walk across every player, since 2 real lines of each
shape name a Command-zone host -- and now `damageAmountMatches` (trigger firing, above), the identical operator/operand
split a trigger's own `DamageAmount$` reads, reused by `damagePreventionMatches` and `damageReplacementMatches` alike
against the ORIGINAL amount before either decides whether its own line applies at all.

## `Mode$ LandPlayed` lands

CR 305/603.5's own "whenever a player plays a land" -- `TriggerLandPlayed`, ported at the one real fire site this port
has, `PlayLand` (land.go). Java's own real ordering, `Player.playLand`: `moveTo` fires ETB triggers internally as part
of the zone change, then the explicit `runTrigger(TriggerType.LandPlayed, ...)` call, then `addLandPlayedThisTurn()` --
`checkLandPlayedTriggers` (trigger.go, new) is called right after `checkETBTriggers`, and `PlayLand`'s own
`LandsPlayed++` was moved to run last to match, since `NotFirstLand$` (below) needs to read the count of lands played
strictly BEFORE this one, the same value Java's own `performTest` sees.

`ValidCard$` is matched the ordinary way (`Matches`). `Origin$` resolves through `hasZoneOrAny` (ETB triggers' own
dispatch, `## Last-known-information`, above -- reused outright, no new code) against the land's own origin zone -- this
port's own `PlayLand` only ever moves a card out of Hand (`c.Zone != Hand` is one of its own rejection conditions), no
`MayPlay$` permission to play from elsewhere yet (Layer 8's own remaining gap, `MayLookAt$`/`MayPlay$`, game-state.md's
own "Thin or missing" account), so 8 of the corpus's 9 real non-`Static$` `Origin$` lines (all naming Exile, or
`Ante,Command,Exile,Graveyard,Library`, "from anywhere other than your hand") never actually satisfy it today --
`hasZoneOrAny` itself is correct, the ability it gates is simply unreachable given this port's own current scope, the
identical "mechanically correct, presently unreachable" gap `DB$ ReplaceDamage`'s own `hedron_field_purists.txt` lines
already have (`#### DB$ ReplaceDamage`, above). The 9th, Undying Vengeance's own real `Origin$ Hand` line, fires
normally.

`NotFirstLand$` (1, a bare presence check -- `TriggerLandPlayed.java` never reads its own value) resolves through a new
pre-increment read of `Player.LandsPlayed` (player.go): `LandsPlayed < 1` fails the trigger on the very first land of
the turn (the count is still 0 at that point) and passes on every land after (the count already reflects every earlier
land this turn). `ValidActivatingPlayer$` (1, "You") resolves through `matchesActivatingPlayer` (trigger firing, above
-- reused outright) against `player`, the land-playing player threaded through as a new explicit parameter
(`checkLifeGainedTriggers`'s own `gainer PlayerID` precedent). `IsPresent$` (3) resolves generically through
`triggerEffectAPI`'s own `triggerCommonRequirementsMet` fold-in (`## CardTraitBase.meetsCommonRequirements`, above) --
`checkLandPlayedTriggers` needed no code of its own for it.

38 of the corpus's own 42 real `T:Mode$ LandPlayed` lines resolve now. `Static$`/`ValidSA$` (5/4, 2 lines naming both --
"Once during each of your turns, you may play a historic land or cast a historic permanent spell from your graveyard"
among them) skip via `hasAnyParam`, the identical "whole line, not a guess" contract every other unresolved-shape gap in
this port already has: `Static$` marks a trigger ability that resolves without going on the stack at all, a mechanism
this port's own `pushTriggeredAbilities` does not model (every trigger this port fires goes on the stack the same way);
`ValidSA$` (always `SpellAbility.MayPlaySource` here) matches a `SpellAbility`, an object `Matches` cannot evaluate.
`OptionalDecider$` (3, every real line "You") resolves too now, through `triggerEffectAPI`'s own `triggerIsOptional`
("`CR 603.3d's own "may" triggered ability`," below) -- `checkLandPlayedTriggers`'s own `hasAnyParam` never named this
key at all, so search_the_city.txt's/jokulmorder.txt's/burgeoning.txt's own real "you may..." lines were firing
unconditionally before this, a real correctness fix (PORT-8/GO-7) rather than only a new resolution.

Eleven new tests (`trigger_test.go`): `TestPlayLandFiresLandPlayedTrigger`/
`TestPlayLandSkipsLandPlayedTriggerForNonMatchingValidCard` prove `ValidCard$` both ways;
`TestPlayLandFiresLandPlayedTriggerWithMatchingOrigin`/`TestPlayLandSkipsLandPlayedTriggerWithMismatchedOrigin` prove
`Origin$` both ways; `TestPlayLandSkipsLandPlayedTriggerOnFirstLandWithNotFirstLand`/
`TestPlayLandFiresLandPlayedTriggerOnSecondLandWithNotFirstLand` prove `NotFirstLand$` across two lands played the same
turn (`AdjustLandPlays$ Unlimited` lifting the per-turn limit, `land_test.go`'s own existing helper reused);
`TestPlayLandFiresLandPlayedTriggerForMatchingActivatingPlayer`/
`TestPlayLandSkipsLandPlayedTriggerForNonMatchingActivatingPlayer` prove `ValidActivatingPlayer$` both ways;
`TestPlayLandSkipsLandPlayedTriggerNamingStaticAndValidSA` proves the real combined `Static$`/`ValidSA$` shape refuses
outright; `TestPlayLandFiresLandPlayedTriggerNamingOptionalDeciderWhenConfirmed`/
`TestPlayLandSkipsLandPlayedTriggerNamingOptionalDeciderWhenDeclined` prove `OptionalDecider$ You` both ways against
burgeoning.txt's own real shape. Every new gate was regression-checked by temporarily disabling it and confirming the
corresponding test failed with the expected wrong `StackLen()`/hand count before restoring it -- including the call site
itself (`checkLandPlayedTriggers` commented out of `PlayLand`), which every "fires" test caught and every "skips" test
correctly stayed green through.

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

Protection is also now covered, `protectionValid` (staticability.go) built the same per-card way `landwalkType` is:
Java's own `keyword.startsWith("Protection")` branch (`CardFactoryUtil.java:3953-3963`) calls
`Protection.getProtectionValid(keyword, false)` (`damage=false`, the block-legality call, distinct from the
damage-prevention one this port never makes) to build `ValidBlocker$`, and that builder's own output differs per card (a
color, a type, a subtype) the identical reason Landwalk's `<Type>` does. Two real corpus shapes, both resolved: the
natural-language form (`K:Protection from red`, 154 of roughly 219 real lines) — `protectionColorValid` maps each of the
five colors, `"colorless"` and `"everything"` to the exact `"Card.<X>,Emblem.<X>"` string
`Protection.getProtectionValid`'s own final wrap produces (`Emblem` never matches anything in this port, `baseMatches`'s
own hardcoded `false` for it, since nothing here creates one yet — harmless, `Card.<X>` is the half that ever does); and
the colon-structured form (`K:Protection:Artifact`, 65 lines) — `keyword.Parse` already splits `Details` at the first
colon (`Name="Protection"`, `Details="Artifact"` or `"from red"`, `keyword.go`'s own doc comment on the space-vs-colon
split), and Java's own early-return path for this branch (`Protection.java:26`) turns out to never need the
`Card.`/`Emblem.` wrap at all in the real corpus: every real characteristic here is either a bare type/subtype word
(`Artifact`, `Vampire`, `Dragon`, ...), which `baseMatches`'s own default type-fallthrough already resolves the
identical way Java's own `getType().hasStringType(incR[0])` does, or already dot-qualified (`Card.MultiColor`,
`Card.cmcGE3`), which `Matches` already resolves whole. `"protection from everything"` (1 real line) is Java's own
empty-`validSource` case — `CardFactoryUtil` then omits `ValidBlocker$` entirely, an unconditional CantBlockBy
`applyCantBlockBy`'s own `hasValidBlocker=false` contract already gives for free.

Skulk (15 cards, `K:Skulk`, CR 702.118a's "can't be blocked by creatures with greater power") is covered too, but not
through `cantBlockByKeywords`' fixed-string table or a `protectionValid`/`landwalkType`-shaped per-card-argument reader:
its own `ValidBlocker$ Creature.powerGTX` names a `Compare` property whose `X` operand is neither a fixed string nor a
per-card script value. `CardFactoryUtil.java`'s own Skulk branch hardcodes `st.setSVar("X", "Count$CardPower")` directly
on the synthesized `StaticAbility`, and `CardProperty.java`'s own `power`-comparison branch always resolves that operand
against `source` — the ability's own host, always the attacker itself since `ValidAttacker$` is hardcoded to
`Creature.Self`. So `X` is always "the attacker's own power," a constant semantic rather than a `Count$`/SVar question
at all: `skulkBlocks` (staticability.go) reads it directly as `host.Power()` (already the full Layer-7-folded value, not
the printed one) and compares it to the candidate blocker's own `Power()`, the identical direct-comparison shape
`menaceLegal` already uses for Menace rather than routing through `Matches`/`compareMatches`. `compareMatches`'s own
non-numeric-operand gap stays real for every OTHER `Compare` property a script writes a genuine dynamic SVar for — Skulk
just never was one of those once the hardcoding was read precisely (PORT-8). This was the one real `CantBlockBy` gap the
block-legality section named; block legality has none left.

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
(`AbilityUtils.calculateAmount`/`getDefinedPlayers`, both far larger than what this slice needed): `NumCards$` (absent
means 1, Java's own default), through `resolveNamedAmount` (amount.go) — a plain integer or a named SVar this face
defines, upgraded from a bare `strconv.Atoi` once `dealDamageEffect`'s own `NumDmg$` needed the identical resolution and
`Ability` grew an `Amounts` field to carry it (`## M6's second effect: DealDamage`, below); a `*`-shaped amount or one
outside `resolveAmount`'s own Valid family still errors by name rather than guessing (the identical gap `valid.go`'s
`compareMatches` already documents for a valid-string's own numeric compare); and `Defined$ You` (896 of 2,576 real
`DB$ Draw` lines) or `Defined$ Opponent`/`Player.Opponent` (11) — every opponent still in the game, `p.isInGame()`'s own
check reproduced as `!g.Player(pid).Lost`, through a new `definedPlayers` (`defined.go`) once `dealDamageEffect` needed
the identical resolution too (below) — this file's own version, `drawDefinedPlayers`, relocated and renamed since
neither effect owns it outright. Everything else Java's `getTargetPlayersWithDuplicates` can resolve (`Targeted`,
`Remembered`, `TriggeredPlayer`, `TriggeredController`, plain spell targeting when `Defined$` is absent entirely — 1,312
of the 2,576) is not: each is a named reference-resolution vocabulary this port has no representation for yet (Memory
lists exist for `IsRemembered`/`IsImprinted`, valid.go, but nothing populates one from a trigger's own firing yet), so
`drawEffect` errors by name instead of drawing for the wrong player. `Upto`, `OptionalDecider`, `Reveal` and
`RememberDrawn` are the same kind of gap — none of `PlayerController`'s methods this port has yet covers a numeric or
reveal choice — checked and rejected explicitly rather than silently ignored.

The actual draw mechanism was already built and correct: `drawStep` (`turn.go`) already drew one card for the active
player, library-empty case included. `DrawCards(pid, n)` is that same body, generalized to n cards for any player and
exported so `drawEffect` can call it — `drawStep` becomes a one-line `g.DrawCards(g.activePlayer, 1)`, not a duplicate.

`NewRegistry` (`castspell.go`) registers `APIDraw`; a new `draweffect` `enginelint` group sits above `turn` (for
`DrawCards`), `player`, `game` and `ability`, and `castspell` gained it as a dependency to register into.
`TestCastSpellFiresETBTrigger` changed from checking `ResolveStack` reports `ErrUnimplemented` naming `Draw` to checking
a real library card actually reaches hand — the same fixture, testing what is now really there instead of the gap that
used to be. Six new tests (`draweffect_test.go`) drive every resolvable and every rejected shape through the real
cast-and-resolve pipeline, `drawEffect` itself being unexported (TEST-1).

`drawEffect` never named `SubAbility$` among its own unresolved params, so once `resolveSubAbility` ("SubAbility
chaining itself lands," further below) landed, chaining started working for `Draw` for free: 170 of the corpus's own 747
real SVar-defined `Draw` lines naming `SubAbility$` chain to an already-built leaf ability and resolve end to end --
Rousing Read's own real "draw two cards, then discard a card" (`DB$ Draw`, chaining into `DB$ Discard`) among them.

## M6's second effect: DealDamage

`DealDamage` (CR 119/120.1, `DamageDealEffect.java`) is the corpus's single most frequent `AB$`/`DB$` API after
`ChangeZone` -- 2,219 real `(AB|DB)$ DealDamage` lines. Java's own `resolve` batches every target's damage into a
`CardDamageTable` and applies the whole table at once through `GameAction.dealDamage`; this port's own damage machinery,
`dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go), already applies one target's damage immediately -- marking,
the `DamageDealt` event, CR 614's own prevention and CR 603's own trigger, all in one call -- the identical
simplification `dealCombatDamageStep`'s own doc comment already gives for combat's own multi-target exchanges: nothing
between two sequential applications can observe or react differently yet (no interactive priority pass exists), so
sequential produces the identical final state "simultaneous" would.

Scoped to the corpus's single largest resolvable slice: a plain-or-named-SVar `NumDmg$` dealt to a `Defined$` player
(`You`/`Opponent`/`Player.Opponent`) or the ability's own host (`Self`), sourced from that same host -- 62 of the 822
real lines naming any `Defined$` value at all (2,219 total). `DamageSource$` (17 of 822) names a source other than the
host and is not resolved, no reference vocabulary for it existing yet; every other real line either carries no
`Defined$` at all (`ValidTgts$`-driven targeting, this port's own "90 of `PlayerController`'s 110 methods" gap,
`## Not ported yet` below) or one of the ~15 other `Defined$` shapes real corpus lines use (`TriggeredPlayer`,
`Remembered`, `Targeted`, ...), none of which this port has a reference-resolution vocabulary for yet -- the identical
gap `drawEffect`'s own doc comment already names for `Draw`.

`dealDamageEffect` (`dealdamageeffect.go`, new) reads `NumDmg$` through `resolveNamedAmount` (amount.go), and `Defined$`
two ways: `"Self"` resolves directly to `a.Source` (the ability's own host, `dealPermanentDamage`); anything else goes
through `definedPlayers` (`defined.go`, new -- `drawEffect`'s own `drawDefinedPlayers` renamed and relocated, below).
`HasKeyword("Deathtouch")` read off the source card supplies `dealPermanentDamage`'s own deathtouch flag the identical
way combat's own attacker/blocker already do -- CR 702.2b's own "any nonzero deathtouch damage is lethal" then applies
through the ordinary lethal-damage state-based action, no special case needed for a script source versus a creature.

Reusing `dealPermanentDamage`/`dealPlayerDamage` directly (rather than a parallel script-only damage path) needed one
real generalization: both were combat-only until now, each hardcoding `true` for `isCombat` at their own
`damagePrevented`/`checkDamageDoneTriggersToCard`-style calls and their own emitted event's `FlagCombat`. Both gained an
explicit `isCombat bool` parameter -- every existing call site in `combatdamage.go` now passes `true` literally,
`dealDamageEffect` the first to pass `false` -- and `FlagCombat`'s own doc comment ("marks damage dealt in combat rather
than by an effect") finally has a second case to distinguish,
`var flags EventFlags; if isCombat { flags |= FlagCombat }` in place of the bare literal each function used to emit
unconditionally.

`resolveNamedAmount` needed a way to reach a card that isn't a trigger's own host: `Ability` (`ability.go`) gained an
`Amounts map[string]expr.Amount` field, alongside `Params`, so a script-driven effect's own `Resolve` can read the
compiling face's SVar table the identical way `triggerCommonRequirementsMet`/`ptParam` already do. Every one of the
eighteen `Ability{API: api, Source: ..., Controller: ..., Params: sub}` constructions across `trigger.go`'s own
check-triggers functions gained `Amounts: face.Amounts` -- `face` already in scope at each, `triggerEffectAPI` itself
already taking it as a parameter (`## CardTraitBase.meetsCommonRequirements`, above) -- confirmed mechanical by a
one-line `sed` substitution matched against the exact count of eighteen. `drawEffect`'s own `NumCards$` was upgraded to
`resolveNamedAmount` too once the field existed, closing a named-SVar shape it used to reject outright for free (above).

`definedPlayers` is `drawDefinedPlayers` (draweffect.go) verbatim, moved to a new `defined.go` and renamed once
`dealDamageEffect` needed the identical `You`/`Opponent`/`Player.Opponent` resolution its own `Defined$` already has --
neither effect owns it outright, the same "shared, so neither" reason `amount.go`'s own `resolveAmount` sits apart from
its callers.

Not resolved, each skipped whole via an allow-list of the params real corpus lines pair with this shape rather than a
reject-list of the ones found (`tapAbilityResolvesTap`'s own identical style, replacement.go) -- a param neither list
has seen skips by construction instead of silently applying (PORT-8/GO-7): `Planeswalker$`/
`ValidTgts$`/`TriggeredSpellAbility$`/`DamageMap$`/`CounterNum$`/`Optional$`/`TgtPrompt$` (each its own further
mechanic, no real line among the 822 combining more than one); `NoPrevention$` (1) -- this port's own
`damagePrevented`/`damagePreventedPlayer` would otherwise wrongly apply where Java's own `AbilityKey.NoPreventDamage`
says the damage cannot be prevented at all. `SubAbility$` (80 of 822) no longer blocks -- removed from
`dealDamageUnresolvedParams` once `resolveSubAbility` ("SubAbility chaining itself lands," below) landed: 9 of the
corpus's own 316 real SVar-defined `DealDamage` lines naming `SubAbility$` chain to an already-built leaf ability and
resolve end to end. `UnlessPayer$`/`UnlessCost$`/`UnlessResolveSubs$` no longer block either, removed once
`resolveUnlessCost` ("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed: 3 of the corpus's own 31 real
`DealDamage` lines naming `UnlessCost$` resolve past that gate.

`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` (5 of the 822, once
`SubAbility$`/`DamageSource$`/every other still-unresolved param above is excluded) are resolved now too, the exact same
fix `LandTapped`'s own checkland shape needed (`## Replacement effects`, above): `subAbilityConditionMet`
(`condition.go`, new) is `SpellAbilityCondition.areMet`'s own gate, shared because both a replacement's `DB$ Tap` and a
script effect's own top-level ability need the identical two shapes. A met condition lets `dealDamageEffect.Resolve` run
as normal; an unmet one returns `nil` rather than an error -- `SpellAbilityCondition.areMet`'s own contract is "the
ability does nothing," the identical "declined by the rules" outcome `PlayLand`/`PayManaCost` already report as a plain
`false`/`nil`. Two params stay in `dealDamageUnresolvedParams`, failing loudly by name the identical way
`DamageSource$`/`SubAbility$` already do: `Condition$` itself (SpellAbilityCondition's own separate
Threshold/Metalcraft/... flag switch) and `ConditionDefined$` (an arbitrary reference this port has no
Defined$-to-objects resolver for). `condition.go`'s own generic unresolved-param guard would otherwise silently no-op
these two specifically, which this file's own established contract (every unresolvable param fails loudly, never
silently) does not allow.

`NewRegistry` (`castspell.go`) registers `APIDealDamage`; new `enginelint` groups `defined` (above `game`/`player`),
`dealdamageeffect` (above `card`/`game`/`player`/`ability`/`combatdamage`/`defined`/`amount`/`condition`) and
`condition` (above `id`/`card`/`game`/`ability`/`trigger`, shared with `replacement`), `castspell` gaining
`dealdamageeffect` as a dependency to register into, `draweffect` gaining `card`/`defined`/`amount` for its own upgraded
`NumCards$` and shared `definedPlayers`. `TestCastSpellFiresOtherPermanentsWatchingTrigger` (Impact Tremors, item 26's
own "checking the mechanism, not the content" trigger fixture) changed from checking `ResolveStack` reports
`ErrUnimplemented` naming `DealDamage` to checking both opponents' life actually drops -- the same fixture, testing what
is now really there. Thirteen tests (`dealdamageeffect_test.go`) drive every resolvable and every rejected shape through
the real cast-and-resolve pipeline, `dealDamageEffect` itself being unexported (TEST-1); one of them (a Deathtouch
source damaging itself) is split into two -- a Deathtouch-free case proving `Damage.Marked`/`Deathtouch` directly, and a
separate Deathtouch case proving the creature destroys itself instead, since any nonzero deathtouch damage is lethal (CR
702.2b) and a dead creature no longer meaningfully has the fields the first case checks. A genuinely single-player
`newGame` combined with a multi-ability `ResolveStack` pass surfaced a real, separate, pre-existing engine property
while debugging this: `CheckStateBasedActions`' own CR 104.2a check ("one player left standing wins") ends the game the
moment exactly one player remains un-lost, which is _every_ single-player game from its very first check onward -- fine
for every earlier test, which either never needed a second `ResolveStack` loop iteration or completed its own
assertion-relevant work before that first check ran, but fatal to a test relying on `ResolveStack` to loop back around
for a trigger a first resolution pushes. Not a bug to fix, since a real game is never single-player; the fix was the
test's own player count, not the engine.

Three of those thirteen are the Condition-family additions: a met/unmet pair,
`TestDealDamageEffectFiresWhenConditionCheckSVarIsMet`/`TestDealDamageEffectNoOpsWhenConditionCheckSVarIsNotMet`, and
`TestDealDamageEffectRejectsConditionItself` (proving `Condition$` itself still fails loudly, distinct from the resolved
`ConditionCheckSVar$`/`ConditionPresent$` pair).

## M6's fourth effect: GainLife, and `Mode$ LifeGained`

`GainLife` (CR 119.1, `LifeGainEffect.java`) is the corpus's single largest resolvable slice past `DealDamage` -- 1,700
real `(AB|DB)$ GainLife` lines, and once scoped to `Defined$ You`/`Player.Opponent` with no other unresolved param, 857
of them resolve, more than `DealDamage`'s own 62. `gainLifeEffect` (`gainlifeeffect.go`, new) is `dealDamageEffect`'s
own shape almost verbatim: `LifeAmount$` through `resolveNamedAmount` (amount.go, `NumDmg$`'s own mechanism), `Defined$`
through `definedPlayers` (defined.go, shared with `DealDamage`/`Draw`), and `subAbilityConditionMet` (condition.go)
gating resolution the identical way it now gates `DealDamage`'s and a checkland's `DB$ Tap` --
`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` resolve, `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionOptionalPaid$` still fail loudly by name. Unlike `DealDamage`, `GainLife`
has no `Self` shape at all (a player gains life, never a card). CR 119's own "life gain replacement" family
(`Event$ GainLife`, 21 real replacement lines) has its one directly resolvable real shape now: `gainLifePrevented`
(replacement.go, "`ReplacementEffect.requirementsCheck` lands," above) resolves sulfuric_vortex.txt's own bare
`Prevent$ True` -- the only one of the 21 naming `Prevent$` at all -- checked per player before `Player.Life` is
touched; the other 20 name `ReplaceWith$` instead, a real substitution needing a runtime value this port cannot read
back, not built. `Player.Life` gains directly; `LifeChanged` (`dealPlayerDamage`'s own event for a life LOSS,
combatdamage.go) is emitted with a positive `Amount` for the gain, reused rather than duplicated. `SubAbility$` no
longer blocks this effect's own resolution either -- removed from its own unresolved-param list once `resolveSubAbility`
("SubAbility chaining itself lands," further below) landed: 18 of the corpus's own 253 real SVar-defined `GainLife`
lines naming `SubAbility$` chain to an already-built leaf ability and resolve end to end.

**`Mode$ LifeGained` (CR 119.1's own "whenever you gain life" trigger, `TriggerLifeGained.performTest`) is real now
too** -- `checkLifeGainedTriggers` (trigger.go, new), `gainLifeEffect`'s own real (non-test) caller, once per player who
actually gained life. No `ValidCard$` at all, the identical no-object-the-event-happens-to shape `Mode$ Phase`/
`Mode$ Drawn` already have, so it reuses `phaseTriggerZones`'s own four-zone walk outright (95 of 98 real lines name
`TriggerZones$ Battlefield`, 2 `Graveyard`, 1 `Command` -- the identical minority-but-real split every earlier reuser
already justified) and `matchesPlayerSpec` for `ValidPlayer$` (present on every real line -- `You`, 95; `Opponent`, 2;
one qualified `Player.Opponent+Active+controlsArtifact.named<card>` this port cannot resolve), matched against the
gaining player rather than the host's own controller, the identical "compare two `PlayerID`s, whichever they represent"
indifference `Phase`'s own `ValidPlayer$` already relies on. Since `ValidPlayer$` is the mode's only real dispatch key
(there is no `ValidCard$` to fall back to), its absence is treated as no match rather than unrestricted --
`checkAttacksTriggers`'s own contract for `ValidCard$`, applied here to `ValidPlayer$` instead, since 0 real lines omit
it. `FirstTime$` (6) resolves too -- Java's own `AbilityKey.FirstTime` (`lifeGainedTimesThisTurn == 0`, computed in
`Player.gainLife` BEFORE the counter itself increments), a new pre-increment read of a new
`Player.LifeGainedTimesThisTurn` (player.go) -- the identical pre-increment-count contract `checkLandPlayedTriggers`'s
own `NotFirstLand$` already has (`## Mode$ LandPlayed lands`, above), a count of GAIN EVENTS rather than the amount
gained (Java's own separate `lifeGainedThisTurn` field, not tracked here since no real corpus line needs it),
incremented once per `gainLifeEffect` resolution -- the only life-gain call site this port has, so no other site needs
to touch it. `ActivationLimit$` (4) now skips the whole line too -- a real correctness fix (PORT-8/GO-7), not a new
resolution: this port never checked the key at all before, so those 4 real lines were firing every single time they
could rather than up to their own per-turn/per-game cap, a wrong answer this port had, not a coverage gap it was honest
about -- the identical unbuilt per-trigger-activation counter `checkBecomesTargetTriggers`'s own doc comment already
names for `ActivationLimit$` there. 93 of the corpus's own 98 real lines resolve now (`OptionalDecider$`, 7, every real
line "You", resolves too through `triggerEffectAPI`'s own `triggerIsOptional`,
"`CR 603.3d's own "may" triggered ability`," below). Not resolved, skipped via `hasAnyParam`: `ValidSource$`/`Spell$` (1
line, both named together) -- matched against the triggering `SpellAbility` itself, the identical ability-kind
classifier `becomesTargetSourceMatches` (`## Mode$ BecomesTarget lands`, above) has for a different mode, not built here
since this one real line stays blocked by `Spell$` regardless of whether `ValidSource$` resolves; `ResolvedLimit$` (1)
-- `Trigger.getResolvedThisTurn`'s own separate per-trigger resolution counter, a different mechanic from
`ActivationLimit$`'s own per-trigger activation counter.

14 tests (`gainlifeeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `gainLifeEffect` itself being unexported (TEST-1) -- the identical set `dealdamageeffect_test.go` has minus
the Deathtouch/`FlagCombat` pair (neither applies to a player-only effect), plus
`TestGainLifeEffectFiresLifeGainedTrigger` (a separate watcher's own `Mode$ LifeGained` fires and its `Execute$ Draw`
resolves) and `TestGainLifeEffectSkipsLifeGainedTriggerForOpponent` (the negative control -- `ValidPlayer$ You` never
matches an opponent's own gain), plus three more proving the new work:
`TestGainLifeEffectFiresLifeGainedTriggerFirstTimeOnFirstGain`/
`TestGainLifeEffectSkipsLifeGainedTriggerFirstTimeOnSecondGain` prove `FirstTime$` across two separate gains the same
turn, and `TestGainLifeEffectSkipsLifeGainedTriggerWithActivationLimit` proves the `ActivationLimit$` correctness fix.
Both new gates were regression-checked by temporarily disabling them and confirming the corresponding test failed with
the expected wrong library-card zone before restoring them. `NewRegistry` (`castspell.go`) registers `APIGainLife`; new
`enginelint` group `gainlifeeffect` (above
`card`/`game`/`player`/`ability`/`event`/`trigger`/`defined`/`amount`/`condition` -- `gainlifeeffect` is
`dealdamageeffect`'s own first sibling to call `trigger.go` directly rather than through `combatdamage.go`'s own
intermediary, since no combat-style damage-marking layer exists for life to route through), `castspell` gaining it as a
dependency to register into.

## M6's fifth effect: Pump, and duration tracking

`Pump` (CR 611, `PumpEffect.java`) is the corpus's own single largest script-driven effect by real line count after
`ChangeZone`/`Draw` -- 4,103 real `(AB|DB)$ Pump` lines. Java's own `resolve` reads two dozen params across a targeted
grant, a `Defined$` grant, `Radiance$`'s own fan-out, `SharedKeywordsZone$`'s own zone scan, `DefinedKW$`'s own
placeholder substitution and more; this port scopes to `Defined$ Self`/`Enchanted`/`Equipped` -- no real target, 1,335
of the 4,103 real lines -- and within that, 1,147 resolve. A new `definedCards` (defined.go, `definedPlayers`'s own
sibling) resolves `Self` to the ability's own host card directly and `Enchanted`/`Equipped` to what the host is
currently attached to (`Card.AttachedTo`, an Aura or Equipment's own reference to its host) -- `pumpEffect`'s own first
caller, matching Java's `AbilityUtils.getDefinedCards` for exactly these two shapes and no others. `NumAtt$`/`NumDef$`
resolve through `resolveNamedAmount` (`NumDmg$`'s own mechanism), `KW$` through `keywordTokens` (continuous.go,
`AddKeyword$`'s own " & "-separated token split and dynamic-marker rejection, reused rather than duplicated -- a `KW$`
naming a marker like `ChosenType` with no real corpus line combining that with a resolved shape here anyway), and
`PumpZone$` through a new `pumpZoneMatches` (absent means Battlefield alone, `ZoneType.listValueOf`'s own Java default;
present, a comma list checked via `hasZone`, trigger.go's own `TriggerZones$` mechanism reused for a resolving ability
instead of a trigger). `subAbilityConditionMet` (condition.go) gates resolution the identical way it gates
`DealDamage`'s/`GainLife`'s.

**This is the first script-driven effect whose own contribution outlives its `Resolve` call.** Every prior effect
(`Draw`, `DealDamage`, `GainLife`) changes game state once and is done; a `Pump` with no `Duration$` (1,282 of the 1,335
real `Defined$ Self`/`Enchanted`/`Equipped` lines) has to keep applying until end of turn, and this port had never
needed a duration-scoped continuous effect before -- `applyContinuousPT`'s own doc comment used to name this as the one
thing keeping its own blanket per-pass rebuild correct only by accident: nothing yet ever added a `PTEffect` outside a
`Mode$ Continuous` static line. A new `Game.pumps` ledger (`pumpRecord`, game.go -- `Card`, a `Timestamp`,
`Power`/`Toughness`, `Keywords`, `Permanent`) records each resolved `Pump`'s own contribution there instead.
`applyPumpEffects` (continuous.go, new) re-adds every record into its target's own `PT`/`KeywordMod` every
`CheckStateBasedActions` pass, called right after `applyContinuousPT`/`applyContinuousKeyword` so their own per-pass
`Clear()` has already emptied every battlefield card's effects for this pass -- the identical "recompute fresh every
pass" contract a `Mode$ Continuous` static already has, just fed from `Game.pumps` instead of a card's own script.
`cleanupStep` (turn.go) drops every non-`Permanent` record at end of turn -- CR 514.2's own "until end of turn" effects
wearing off, the half of 514.2 its own doc comment used to name as unbuilt. `Duration$ Permanent` (19 of 1,335) marks a
record that survives cleanup; every other real value (`Perpetual`, 12; `UntilEndOfCombat`, 4; `UntilYourNextUpkeep`, 1)
is not resolved, each its own further expiry hook this port has no equivalent of.

A record also has to stop applying the moment its own target leaves the battlefield, before cleanup ever runs --
`Game.Move`'s existing `PT.Clear()`/`KeywordMod.Clear()` branch (the same one `Counters`/`Damage`/`Tapped` already reset
there) now calls a new `clearPumps` (game.go) too, dropping every record naming that card. Without it, `CardID` being
stable across zone changes here (ADR-0009) would let a `Permanent` record -- or even a same-turn `Duration$`-less one --
silently survive a trip to the graveyard and reapply the moment a Raise Dead-style effect returned the same `CardID` to
the battlefield: Java's own `applyPump` guards against exactly this with a per-instance game-timestamp check
(`applyTo.equalsWithGameTimestamp(gameCard)`) this port has no equivalent of, so dropping the record on exit gets the
same real-world answer without one.

`SubAbility$` (119 of 1,335) no longer blocks -- removed from `pumpUnresolvedParams` once `resolveSubAbility`
("SubAbility chaining itself lands," below) landed: 17 of the corpus's own 571 real SVar-defined `Pump` lines naming
`SubAbility$` chain to an already-built leaf ability and resolve end to end. `UnlessCost$`/`UnlessPayer$`/
`UnlessSwitched$` no longer block either, removed once `resolveUnlessCost` ("`Registry.Resolve`'s own `UnlessCost$`
gate," below) landed: 2 of the corpus's own 15 real `Pump` lines naming `UnlessCost$` resolve past that gate and are
actually reachable at all -- spitting_slug.txt's own real "gains first strike... unless you pay {1}{G}," chaining
`UnlessResolveSubs$ WhenNotPaid` into `PumpAll` when the cost goes unpaid, and, once `ActivateAbility` landed too
(`## Activating an ability lands`, below), nakaya_shade.txt's own real activated `{B}:` ability itself gated by its own
nested "unless any player pays {2}" -- the two gates compose with no special-casing needed, since `ActivateAbility`
threads the ability's own compiled `Params` (`UnlessCost$` included) through unchanged and `Registry.Resolve` reads
`UnlessCost$` off whatever pushed the ability, activation included. The other 13 name a non-pure-mana cost, an
unresolvable `UnlessPayer$`, or a top-level spell on an Instant (`CastSpell` does not cast an instant or sorcery at
all), pushing the `Defined$`-shape count from 1,147 to 1,148 of 1,335 (nakaya_shade.txt's own line already counted in
that 1,148 once its own outer activation and inner `UnlessCost$` both resolve).

Not resolved, each failing loudly by name rather than guessing (PORT-8/GO-7): `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$`/`ConditionActivationLimit$` (0/19/0/4) --
`SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does not cover, the identical
`DealDamage`/`GainLife`-shaped gap; `PlayerTurn$` (2) -- unclear semantics on a `Pump` line, not worth guessing at from
two real lines; `NumAtt$`/`NumDef$` naming the literal `Double`/`Triple` (1 combined) -- the target's own power or
toughness doubled or tripled, a hardcoded special case rather than a named SVar `resolveNamedAmount` could resolve; a
`KW$` token starting with `HIDDEN` (22) -- a hidden-keyword phrase (`gameCard.addHiddenExtrinsicKeywords`), its own
separate mechanic; `CanBlockAmount$`/`CanBlockAny$` (4/0) -- an additional-blocker grant this port's own block-legality
gate (staticability.go) has nowhere to consult a one-shot record from; `DefinedKW$`/`KWChoice$`/`RandomKeyword$` (3/3/1)
-- a placeholder substitution, an interactive choice and a random draw, none of which this port's own `KW$` handling
does; `SharedKeywordsZone$`/`SharedRestrictions$` (2/2) -- `CardFactoryUtil.sharedKeywords`'s own zone scan;
`ValidTgts$` (1) -- a real target past the `Defined$` card this effect already resolves; `AtEOT$` (9) --
`registerDelayedTrigger`, a new trigger this effect would silently fail to create; `ImprintCards$` (1) and
`DefinedLandwalk$`/`ForgetObjects$`/`RememberObjects$`/`RememberPumped$`/`LeaveBattlefield$`/`ForgetImprinted$`/
`NoteCards$`/`NoteCardsFor$`/`ClearNotedCardsFor$`/`NoteNumber$` (0 each in this scope) -- each its own further tracking
mechanic; `IsPresent$` (6) -- unclear semantics on a resolving (not triggering) `Pump` line, skipped rather than assumed
harmless; `Optional$`/`OptionQuestion$` (0/0) -- a "may" confirmation this port's own `PlayerController` has no hook
for; `Radiance$` (0) -- `CardUtil.getRadiance`'s own "and everything else that shares a color" fan-out.

11 new tests (`pumpeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `pumpEffect` itself being unexported (TEST-1): `Defined$ Self` granting power/toughness and a keyword,
`Duration$`'s default wearing off at cleanup and `Permanent` surviving it, `SubAbility$`/`Double`/a `HIDDEN` keyword
each rejected loudly, `ConditionCheckSVar$`'s own met/unmet pair, `PumpZone$` rejecting a target not in the zone it
names, and an Aura's own `Defined$ Enchanted` shape end to end -- cast, attach, and the enchanted creature (not the Aura
itself) gets the boost. `NewRegistry` (`castspell.go`) registers `APIPump`; new `enginelint` group `pumpeffect` (above
`id`/`card`/`game`/`ability`/`defined`/`amount`/`condition`/`trigger`/`continuous`/`zone` -- the first effect group
needing `continuous` directly, to call `keywordTokens`, and `zone`, for `Battlefield`/`ZoneType` in `pumpZoneMatches`),
`castspell` gaining it as a dependency to register into.

## M6's sixth effect: PumpAll, the blanket sibling

`PumpAll` (CR 611, `PumpAllEffect.java`) is `Pump`'s own blanket counterpart: a `ValidCards$`-matched set rather than a
single `Defined$`/targeted card. 833 real `(AB|DB)$ PumpAll` lines, and unlike `Pump` itself, the dominant real shape
carries no `Defined$` and no target at all -- 818 of the 833 -- Java's own
`!sa.usesTargeting() && !sa.hasParam("Defined")` branch, `game.getCardsIn(affectedZones)` filtered by `ValidCards$` with
nothing narrowing it to specific players first. That is this port's entire real scope, since neither targeting nor most
`Defined$` shapes exist; within it, plus the 3 real `Defined$ You`/`Player.Opponent` lines `definedPlayers` already
covers, 642 of 833 resolve -- a much higher hit rate than `Pump`'s own 1,147 of 4,103, since a blanket `ValidCards$`
match needs no card-reference resolver at all, only the same `Matches` evaluator `applyOneContinuousPT`'s own
`Affected$` already reuses for the identical "every battlefield permanent this valid string matches" shape
(`## Continuous effects: Layer 7's own PT`, above).

`pumpAllEffect` (`pumpalleffect.go`, new) reuses `Pump`'s own machinery outright rather than duplicating it:
`pumpAmount`/`pumpKeywords` (pumpeffect.go) both gained an `effect string` parameter once `PumpAll` became their second
caller, naming the effect (`Pump`/`PumpAll`) in their own error text instead of hardcoding it; `Game.pumps`/
`applyPumpEffects`/`cleanupStep`'s own duration tracking (`## M6's fifth effect: Pump, and duration tracking`, above)
needs no changes at all, since a `pumpRecord` never itself distinguishes which effect created it -- `PumpAll`'s own
resolved lines just append more of them. `PumpZone$`'s own meaning shifts from a single-target zone check
(`pumpZoneMatches`, `Pump`'s own reader) to the list of zones actually scanned: a new `pumpAllZones` parses the same
comma list (`ZoneByName`, zone.go) into `[]ZoneType`, defaulting to `Battlefield` alone -- `ZoneType.listValueOf`'s own
Java default -- rather than checking one card's membership in it. `Defined$ You`/`Player.Opponent` (3 real lines)
resolve through `definedPlayers` (defined.go) to narrow the player set scanned before `ValidCards$` filters each of
their own zones, rather than scanning every player unconditionally.

`SubAbility$` (90 of 833) no longer blocks -- removed from `pumpAllUnresolvedParams` once `resolveSubAbility`
("SubAbility chaining itself lands," below) landed: 6 of the corpus's own 75 real SVar-defined `PumpAll` lines naming
`SubAbility$` chain to an already-built leaf ability and resolve end to end. `UnlessCost$`/`UnlessPayer$` no longer
block either, removed once `resolveUnlessCost` ("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed: 0 of the
corpus's own 3 real `PumpAll` lines naming `UnlessCost$` resolve, though -- rhystic_shield.txt's own real "get +0/+2...
unless any player pays {2}" is the only one clearing that gate's own pure-mana-cost/resolvable-payer filter, and it is a
top-level `A:SP$ PumpAll` line on an Instant, which `CastSpell` cannot cast at all; the other 2 name a controller-
derived `UnlessPayer$` and a non-mana `UnlessCost$` neither resolvable here regardless.

Not resolved, each failing loudly by name rather than guessing (PORT-8/GO-7): `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$`/`ConditionManaSpent$`/`ConditionManaNotSpent$` (4/5/3/1/4/0)
-- `SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does not cover, the identical `Pump`-shaped gap;
`ValidTgts$` (12) -- a real target past the blanket `ValidCards$` match; targeting itself now exists ("Targeting itself
lands," below), `PumpAll` just has not been extended to read `Targeted` back yet; `Planeswalker$`/`Ultimate$` (26/13) --
unclear semantics on a `PumpAll` line, not worth guessing at from either; `RememberPumped$` (8) -- `Card.Memory` has no
writer wired to a blanket multi-card grant; `SharedKeywordsZone$`/`SharedRestrictions$` (4/4) --
`CardFactoryUtil.sharedKeywords`'s own zone scan, a further mechanic; `ModeCost$`/`Exhaust$` (3/4) -- each its own
further activation mechanic.

11 new tests (`pumpalleffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `pumpAllEffect` itself being unexported (TEST-1): a blanket `ValidCards$ Creature.YouCtrl` match pumping every
one of the caster's own creatures including the entering one itself, an opponent's creature correctly excluded, `KW$`
granting a keyword, `Duration$`'s default wearing off at cleanup and `Permanent` surviving it,
`SubAbility$`/`ValidTgts$` each rejected loudly, `Defined$ You` narrowing the scan away from a broader
`ValidCards$ Creature` match that would otherwise also catch the opponent's own creature, `ConditionCheckSVar$`'s own
met/unmet pair, and the default zone scan (Battlefield alone) never reaching a matching card sitting in Hand.
`NewRegistry` (`castspell.go`) registers `APIPumpAll`; `enginelint` group `pumpeffect` gains `pumpalleffect.go`
alongside `pumpeffect.go` (sharing every private helper freely within the one group, `pumpAmount`/`pumpKeywords`
included) and gains `valid` in its own allow-list, for `Matches`/`valid.Parse` -- `pumpAllEffect`'s first real need for
the general valid-string evaluator, `Pump` itself never having needed one since every real `Defined$` shape it resolves
already names an exact card.

## M6's seventh effect: LoseLife, GainLife's own mirror image

`LoseLife` (CR 119.3, `LifeLoseEffect.java`) is `GainLife`'s own mirror image -- a plain-or-named-SVar `LifeAmount$`
subtracted from a `Defined$` player rather than added. 445 real `(AB|DB)$ LoseLife` lines name
`Defined$ You`/`Opponent`/`Player.Opponent`, and 226 of those carry no other unresolved param and resolve (a later
chunk, "Targeting itself lands," below, extends this to 300 by also reading a targeted player directly).
`loseLifeEffect` (`loselifeeffect.go`, new) reuses `gainLifeEffect`'s own machinery outright: `resolveNamedAmount` for
`LifeAmount$`, `definedPlayers` for `Defined$`, `subAbilityConditionMet` gating resolution the identical way -- the two
files are close enough to be near-mirrors of each other, `-=` and a negated `Amount` the only real difference in the
resolve path itself.

Unlike `GainLife`, this calls no trigger check at all. Java's own `Player.loseLife` fires `TriggerType.LifeLost`
directly, and `LifeLoseEffect.resolve` separately fires `TriggerType.LifeLostAll` once more over every player who
actually lost life -- two trigger points where `GainLife` has one (`TriggerLifeGained`, `checkLifeGainedTriggers`'s own
real caller). But `T:Mode$ LifeLost` and `T:Mode$ LifeLostAll` both carry 0 real lines corpus-wide (a `grep` against the
whole cardsfolder for either, case-exact), against `Mode$ LifeGained`'s own 98 -- there is nothing for a
`checkLifeLostTriggers` to ever match, so none is built. The identical `LifeChanged` event `dealPlayerDamage`/
`gainLifeEffect` already emit is reused here too, with a negative `Amount` for the loss -- the third real emitter after
those two, still the one event both directions of a life total change ever fire.

Two gaps `GainLife`'s own paragraph (above) already names apply here unchanged, not newly discovered: Java's own
`Player.gainLife`/`loseLife` are both gated by a `canGainLife`/`canLoseLife` check (`StaticAbilityCantGainLosePayLife`,
an S: line this port's own static-ability engine has no reader for), and CR 119's own life-total replacement family
exists for a loss too (`ReplacementType.LifeReduced`, `GainLife`'s own `ReplacementType.GainLife` counterpart) --
neither built, the identical real-gap-not-a-wrong-answer every other unbuilt replacement remainder already is.

Not resolved, each failing loudly by name rather than draining the wrong amount from the wrong player (PORT-8/GO-7):
`Condition$` itself and `ConditionDefined$`/`ConditionZone$` (0/14/1) -- `SpellAbilityCondition`'s own shapes
`subAbilityConditionMet` does not cover, the identical `GainLife`-shaped gap; `Planeswalker$` (6, counted across the
wider 823-line real `Defined$` set) -- each its own further mechanic;
`Ultimate$`/`IsPresent$`/`PresentCompare$`/`NumCards$`/`ModeCost$` (1/2/2/2/1) -- unclear semantics on a `LoseLife`
line, not worth guessing at from a handful of real lines. `ValidTgts$` (163 of the 823) no longer blocks -- "Targeting
itself lands," below, is why. `SubAbility$` (210 of 445) no longer blocks either -- "SubAbility chaining itself lands,"
further below, removed it from this file's own unresolved-param list: Sphinx Sovereign's own real "gain 3 life if
untapped, otherwise each opponent loses 3" (one `DB$ LoseLife` with a `SubAbility$ DB$ GainLife`, the negated condition
split across the two) is exactly why that chain has to run regardless of whether `subAbilityConditionMet` let this
effect's own body run. 144 of the corpus's own 382 real SVar-defined `LoseLife` lines naming `SubAbility$` chain to an
already-built leaf ability and resolve end to end. `UnlessPayer$`/`UnlessCost$`/`UnlessSwitched$` no longer block
either, removed once `resolveUnlessCost` ("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed: 0 of the
corpus's own 42 real `LoseLife` lines naming `UnlessCost$` resolve, though -- delaying_shield.txt's own real pure-mana
"{1}{W}" is the only one clearing that gate's own pure-mana-cost/resolvable-payer filter, and it is reached only through
`DB$ Repeat`'s own `RepeatSubAbility$` (not the plain `SubAbility$` chaining `resolveSubAbility` reads), a general
repeat-N-times mechanic this port does not build; every other real line names a `Sac<.../Discard<.../PayLife<.../...`
cost part or a controller-derived `UnlessPayer$` this port cannot resolve.

9 new tests (`loselifeeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `loseLifeEffect` itself being unexported (TEST-1) -- `gainLifeEffect_test.go`'s own set minus the two
`Mode$ LifeGained`-firing tests neither this effect nor any trigger mode here has an equivalent of. `NewRegistry`
(`castspell.go`) registers `APILoseLife`; new `enginelint` group `loselifeeffect` (`id`/`card`/`game`/
`player`/`ability`/`event`/`defined`/`amount`/`condition` -- no `trigger`, the one dependency `gainlifeeffect`'s own
allow-list has that this group does not, since nothing here ever calls one), `castspell` gaining it as a dependency to
register into.

## M6's eighth effect: PutCounter, the corpus's second-largest resolvable slice

`PutCounter` (CR 121.1, `CountersPutEffect.java`) is the corpus's own second-largest resolvable script-driven effect
after `Pump` -- 3,165 real `(AB|DB)$ PutCounter` lines. `CountersPutEffect.java` itself is 800 lines: `Bolster$`,
`Monstrosity$`, `Adapt$`, `Support$` and `PowerUp$` are each `buildSpellAbility`'s own rewrite of a simpler keyword line
into this shape; `Choices$`/`ChoiceZone$`/`ChoiceTitle$` drive an interactive multi-card pick;
`DividedAsYouChoose$`/`DividedRandomly$`/`SplitAmount$` divide one pool of counters across several targets;
`PutOnEachOther$`/`PutOnDefined$`/`EachFromSource$`/`ChooseDifferent$`/`EachExistingCounter$`/`UniqueType$` each
retarget or re-derive the counter's own kind per object; `ETB$` stages the grant into a `GameEntityCounterTable` instead
of applying it directly, CR 614's own counters-added-simultaneously replacement mechanism -- the identical batching risk
`ChangesZoneAll`'s own gap already documents, not attempted here either. This port keeps only the plain shape underneath
all of that: a single literal `CounterType$`, a `CounterNum$` amount, and `Defined$` naming who or what receives it --
992 of the 3,165 real lines carry nothing past that and resolve.

`putCounterType` (new) reads `CounterType$`, rejects a comma-separated list (22 real lines, an interactive choice among
several kinds this port's `PlayerController` has no hook for), `ExistingCounter` (8, `EachExistingCounter$`'s own
hardcoded literal) and `Any` (`CounterType.getType`'s own case-insensitive nil sentinel -- "counters of any kind," not a
concrete one to add) -- then uppercases whatever is left. That uppercasing matters: real corpus lines write `Stun` (74)
and `STUN` (23) for the same counter, and Java's own `CounterEnumType.getType` canonicalizes both to one enum constant
via `name.toUpperCase(Locale.ROOT)` before ever touching a card's own counter multiset; without the identical step here,
this port's own open-string `CounterType` (`type CounterType string`, counters.go) would silently track `Stun` and
`STUN` as two different kinds of counter on the same card. This port has no keyword-counter/custom-type split Java's own
dual namespace (`CounterKeywordType`, keyed by a keyword's own title-case string, `CounterCustomType` for a plain custom
name) otherwise maintains -- nothing downstream reads a counter's own kind to grant the keyword it names yet, so
uppercasing every real value uniformly costs nothing correctness-wise today and is the simpler rule to state.
`CounterNum$` defaults to `1` (`getParamOrDefault("CounterNum", "1")`, Java's own default) and otherwise resolves
through `resolveNamedAmount` exactly as `NumDmg$`/`LifeAmount$` already do.

A new `definedCounterTargets` (putcountereffect.go) dispatches `Defined$` to `definedCards` (`Self`/`Enchanted`/
`Equipped`, unchanged) or `definedPlayers` (`You`/`Opponent`/`Player.Opponent`, unchanged) by which one the value itself
names -- mirroring `CountersPutEffect.resolvePerType`'s own dispatch, which is on the resolved `GameEntity`'s own
runtime type (`obj instanceof Player` vs `obj instanceof Card`), never on `CounterType$`: a player-only counter kind
(energy, poison, experience, ticket -- `getStackDescription`'s own `playerCounters` list, cosmetic text only, no bearing
on `resolve()`'s real dispatch) is only ever reached because `Defined$` itself names a player. Neither helper gained a
new case for this: both already covered every string `Defined$` needs here.

`Card.Counters`/`Player.Counters` gain their first script-driven writer -- every prior counter this port ever put on
anything came from a hardcoded rule (a planeswalker's own starting loyalty on entry, `Game.Move`). `emitCounterChanged`
(event.go) gets its first real caller past that hardcoded path too, and its own documented gap (`counterDetail`'s own
closed eight-constant switch) fires for real for the first time: a script-written `CounterType$` past those eight
(`ENERGY`, and everything else CR 122's own `+`/keyword-counter/custom-counter families cover) still gets the counter --
`Counters.Add` has no such limit -- but emits no `CounterChanged` event at all, rather than one whose own `Detail` would
lie about what kind changed. That gap is unchanged by this, not closed: extending `CounterDetail`'s own switch (or
replacing it with a per-`Game` interning table, `CounterDetail`'s own doc comment already names this as the shape to
reach for) is still whichever later chunk a real caller forces the choice for -- this chunk is that forcing caller, but
does not itself do the extending.

`SubAbility$` (769 of 3,165) no longer blocks -- removed from `putCounterUnresolvedParams` once `resolveSubAbility`
("SubAbility chaining itself lands," below) landed: 56 of the corpus's own 623 real SVar-defined `PutCounter` lines
naming `SubAbility$` chain to an already-built leaf ability and resolve end to end.

Not resolved, each failing loudly by name rather than guessing (PORT-8/GO-7): `ValidTgts$`/`TargetMin$`/`TargetMax$`
(807/162/162) -- a real target; targeting itself now exists ("Targeting itself lands," below), `PutCounter` just has not
been extended to read `Targeted` back yet; `ETB$` (154, above); `Choices$` and its own six further params (46 combined,
above); `DividedAsYouChoose$`/`DividedRandomly$`/`SplitAmount$` (above);
`Monstrosity$`/`Adapt$`/`Bolster$`/`Support$`/`PowerUp$`/`Exhaust$` (above);
`EachFromSource$`/`PutOnEachOther$`/`PutOnDefined$`/`ChooseDifferent$`/
`EachExistingCounter$`/`UniqueType$`/`CounterTypePerDefined$`/`CounterNumPerDefined$`/`OnlyNewKind$`/
`SkipReceiveCounters$`/`RandomType$` (above); `CounterTypes$` (17, plural -- a different multi-type shape, not this
one); `ForColor$`/`SharedKeywords$`/`SharedKeywordsDefined$`/`SharedKeywordsZone$`/`SharedRestrictions$`/
`TriggeredCounterMap$`/`CounterMapValues$`/`SpecifyCounter$`/`Placer$`/`RememberCards$`/`RemovePhase$`/
`Planeswalker$`/`Optional$`/`UpTo$`/`UpToMin$` -- each its own further mechanic. `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$`/`ConditionActivationLimit$`/`ConditionPresent2$`/
`ConditionCompare2$` -- `SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does not cover, the identical
`Pump`/`GainLife`-shaped gap.

12 new tests (`putcountereffect_test.go`) drive every resolvable and every rejected shape through the real
cast-and-resolve pipeline, `putCounterEffect` itself being unexported (TEST-1): `Defined$ Self` adding to the ability's
own host, `Defined$ You` adding to a player instead, `CounterNum$`'s own default and its named-SVar form, case
canonicalization (`Stun` and `STUN` landing on the identical key), a missing `CounterType$`/a comma list/
`SubAbility$`/`ValidTgts$` each rejected loudly, `ConditionCheckSVar$`'s own met/unmet pair, and the `CounterChanged`
event firing for a named `CounterType` (`P1P1`) but not for one outside the closed set (`ENERGY`) -- the counter itself
still added either way. `NewRegistry` (`castspell.go`) registers `APIPutCounter`; new `enginelint` group
`putcountereffect` (`id`/`card`/`game`/`player`/`ability`/`event`/`defined`/`amount`/`condition`/`parts` -- `parts` for
`Counters`/`CounterType` themselves, counters.go), `castspell` gaining it as a dependency to register into.

## M6's ninth effect: Discard, and `Effect.Resolve`'s new `PlayerController` parameter

`Discard` (CR 701.8, `DiscardEffect.java`) is the first script-driven effect that has to ask the resolving player
anything mid-resolution rather than reading game state outright -- every effect before it (`Draw` through `PutCounter`)
computes its whole outcome from `*Game`/`*Ability` alone. `DiscardEffect.java` itself switches on nine `Mode$` values
(`TgtChoose`, `Hand`, `Random`, `YouChoose`, `LookYouChoose`, `RevealYouChoose`, `RevealTgtChoose`, `RevealDiscardAll`,
`Defined`), each its own resolution shape; this port builds only `TgtChoose` -- the discarding player picks their own
cards out of their own hand -- the corpus's largest at 728 of the corpus's 942 real `(AB|DB)$ Discard` lines on its own.
285 of those 728 also name `Defined$ You`/`Opponent`/`Player`/`Player.Opponent` and carry no other unresolved param, and
resolve.

`TgtChoose`'s own decision -- "which N of your hand do you discard" -- is exactly the shape `DiscardToHandSize`
(control.go) already has for CR 514.1's own cleanup discard, and it would have been tempting to just call that. Java
itself does not: `DiscardEffect.resolve`'s own `TgtChoose`/`YouChoose` branch calls `chooseCardsToDiscardFrom`, a
different `PlayerController` method from `chooseCardsToDiscardToMaximumHandSize` (the one `PhaseHandler`'s own cleanup
step calls), because `chooseCardsToDiscardFrom` additionally takes a `validCards` set (a `DiscardValid$`-filtered subset
of hand, not modeled here) and a `min`/`max` pair that need not be equal (an `AnyNumber$`/`Optional$` range, also not
modeled here). Since a real behavioral difference exists between the two even though this chunk's own
`min == max == NumCards$` case looks identical to `DiscardToHandSize`'s contract, this port adds a new, separately named
and separately queued `ChooseCardsToDiscard` (control.go) rather than reusing `DiscardToHandSize` outright -- Java's own
separation is kept rather than collapsed for convenience, the same "trust the controller's answer" contract every other
decision in `PlayerController` already has. `ScriptedController` gained a `discardChoices [][]CardID` queue and a
`QueueDiscardChoice` alongside it, deliberately not sharing `discards`/`QueueDiscard`: a fixture scripting both
decisions in the same scenario must not have one accidentally answer from the other's queue.

Getting a controller into `discardEffect.Resolve` at all needed a real signature change: `Effect.Resolve` gained a
`PlayerController` parameter, threaded through `Registry.Resolve` and `ResolveStack`'s own call into it. `ResolveStack`
already took a `controller PlayerController` argument -- it has since the stack first landed, for its own
`CheckStateBasedActions` call after each resolution -- so no caller of `ResolveStack` itself changed; only the dispatch
call one level in did. Every effect from `permanentEffect` through `putCounterEffect` picked up an unused
`_ PlayerController` parameter and otherwise did not change at all, confirming the doc comment `Effect`'s own interface
already carries: "everything an effect needs comes from the game and the ability it is handed" was true for eight
effects running and stops being true for a ninth needing to ask someone something.

`NumCards$` resolves through `resolveNamedAmount` exactly as `CounterNum$`/`LifeAmount$` already do, defaulting to `1`
when absent (`Discard`'s own line in `rotting_rats.txt`, `Defined$ Player | NumCards$ 1 | Mode$ TgtChoose`, states it
explicitly, but not every real line does). The resolved amount is clamped to the discarding player's actual hand size,
matching Java's own `numCards = Math.min(numCards, numCardsInHand)` -- asking for more cards than a hand holds would
either panic a fixture's queued answer or force a `ScriptedController` to over-answer, neither a faithful reading of
"discard N cards" when N exceeds the hand. A hand already at zero skips the controller call entirely (Java's own
`if (dPHand.isEmpty()) continue`, inside the shared `YouChoose`/`TgtChoose` branch) rather than asking for zero cards --
the identical "nothing meaningful to decide" reasoning `cleanupStep`'s own doc comment already gives for an
already-small hand at `DiscardToHandSize`. Each chosen card moves to its own owner's graveyard and fires
`checkDiscardedTriggers` (trigger.go, already built for `DiscardToHandSize`'s own cleanup discard) exactly the way
`cleanupStep` already does per card.

`definedPlayers` (`defined.go`) gained a `"Player"` case: `AbilityUtils.getDefinedPlayers`'s own final `else` branch
(`players.addAll(game.getPlayersInTurnOrder())`, unfiltered) is what a `defined` string matching none of the method's
many named cases falls through to -- a bare `"Player"` is exactly such a string, so it resolves to every player in the
game, not just the ability's own controller or their opponents. This is a different path from `"Player.Opponent"`, which
never reaches that fallthrough at all: Java's own `incR = changedDef.split("\\.", 2)` splits `"Player.Opponent"` into
`incR[0] = "Player"` and a dotted suffix `"Opponent"`, and _that_ suffix is what filters the same unfiltered
`getPlayersInTurnOrder()` set down to opponents only -- the identical result `"Opponent"` produces directly without any
splitting at all. Both ports to the one `"Opponent", "Player.Opponent":` case `definedPlayers` already had before this
chunk; only bare `"Player"` needed a new one. `rotting_rats.txt`'s own `Defined$ Player | NumCards$ 1 | Mode$ TgtChoose`
-- "each player discards a card" -- is the real line this closes, and any other effect naming a bare `Defined$ Player`
gets it for free, `definedPlayers` being shared rather than owned by any one effect.

`SubAbility$` (196 of 728) no longer blocks -- removed from `discardUnresolvedParams` once `resolveSubAbility`
("SubAbility chaining itself lands," below) landed: 11 of the corpus's own 254 real SVar-defined `Discard` lines naming
`SubAbility$` chain to an already-built leaf ability and resolve end to end. `UnlessCost$`/`UnlessPayer$`/
`UnlessSwitched$`/`UnlessResolveSubs$` no longer block either, removed once `resolveUnlessCost` ("`Registry.Resolve`'s
own `UnlessCost$` gate," below) landed: 0 of the corpus's own 16 real `Discard` lines naming `UnlessCost$` resolve,
though -- rhystic_scrying.txt's own real pure-mana "{2}" is the only one clearing that gate's own pure-mana-cost/
resolvable-payer filter, and its own `DB$ Discard` is reached only by chaining out of a top-level `A:SP$ Draw` line on a
Sorcery, which `CastSpell` cannot cast at all; every other real line names a `PayEnergy<.../Sac<.../Return<.../...` cost
part or a controller-derived `UnlessPayer$` this port cannot resolve.

Not resolved, each failing loudly by name rather than discarding the wrong cards from the wrong player (PORT-8/GO-7):
every `Mode$` other than `TgtChoose` (above, 214 real lines combined); `ValidTgts$`/`TargetMin$`/`TargetMax$` (98/3/3)
-- a real target; targeting itself now exists ("Targeting itself lands," below), `Discard` just has not been extended to
read `Targeted` back yet; `Optional$` (38) -- an interactive confirm this port's own `PlayerController` has no hook for;
`AnyNumber$` (16) -- a variable, 0-to-hand-size count, a different shape from `ChooseCardsToDiscard`'s own exact-count
contract; `DiscardValid$`/`DiscardValidDesc$` (18) -- a filtered choice set, the identical gap `PutCounter`'s own
`Choices$` family already documents; `UnlessType$` (14) -- Java's own separate `chooseCardsToDiscardUnlessType`
controller method, a different sub-flow entirely; `RevealNumber$` -- a reveal-then-choose-a-subset step ahead of the
discard itself, not modeled; `RememberDiscarded$`/`RememberDiscardingPlayers$`/`RememberDiscardingPlayer$` (88 combined)
-- no `Defined$ Remembered` resolver exists to ever read the value back (chaining itself existing does not help here:
`Discard` still blocks `SubAbility$` outright, and even unblocked, `defined.go` has no `"Remembered"` case), the
identical "blocked outright rather than silently no-op'd" choice `PutCounter`'s own `RememberCards$` already made.
`Condition$` itself and `ConditionDefined$`/`ConditionZone$` -- `SpellAbilityCondition`'s own shapes
`subAbilityConditionMet` does not cover, the identical `Pump`/`GainLife`/`LoseLife`/`PutCounter`-shaped gap;
`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` are resolved through it exactly as
those four already are.

15 new tests (`discardeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `discardEffect` itself being unexported (TEST-1): `Defined$ You` choosing from the caster's own hand,
`Defined$ Opponent` reaching the opponent's hand instead and leaving the caster's alone, `Defined$ Player` asking both
players in turn order with two independently queued answers, `NumCards$`'s own default and its named-SVar form, clamping
a `NumCards$` above the actual hand size, skipping the controller entirely on an empty hand (an unconsumed queue slot
proving it), a missing `Mode$`/an unsupported one (`Random`)/`SubAbility$`/`ValidTgts$` each rejected loudly, and
`ConditionCheckSVar$`'s own met/unmet pair -- the unmet case doubling as proof the controller is never asked when the
ability does nothing. `control_test.go`'s own `TestScriptedControllerEachQueuePanicsWhenExhausted` table gained a
`"discard choice"` entry alongside `"discard"`, and `mulligan_test.go`'s `scriptedMulliganController` (a second,
hand-rolled `PlayerController` implementation used nowhere near discard) gained a panicking `ChooseCardsToDiscard` stub
to keep satisfying the interface. `NewRegistry` (`castspell.go`) registers `APIDiscard`; new `enginelint` group
`discardeffect` (`id`/`card`/`game`/`player`/`ability`/`defined`/`amount`/`condition`/`control`/`zone` -- `control` for
`PlayerController` itself, the first effect group needing it), and `control`'s own group gained an allowance for itself
in every effect group's own list (`effect`, `dealdamageeffect`, `draweffect`, `gainlifeeffect`, `loselifeeffect`,
`pumpeffect`, `putcountereffect`) once `Effect.Resolve`'s new parameter named the type in each -- not because any of the
other eight ever calls a `PlayerController` method, but because naming a type in a function signature is itself a
reference `enginelint`'s dependency gate has to see permitted.

## M6's tenth effect: Scry, and a library's own top for the first time

`Scry` (CR 701.19, `ScryEffect.java` + `GameAction.scry`) is the second script-driven effect that has to ask the
resolving player a real question mid-resolution, and the first where the question is "reorder this set of cards" rather
than "choose a subset of it" -- `Discard`'s own `ChooseCardsToDiscard` (M6's ninth effect, above) returns one slice;
scrying returns two, and their own internal order is part of the answer, not incidental to it. 332 of the corpus's 415
real `(AB|DB)$ Scry` lines resolve.

`ScryEffect.java` itself is thin -- 51 lines, mostly stack-description text -- because it hands the real work straight
to `GameAction.scry`. That method's own shape drove this port's own design directly: for each deciding player, take the
top `ScryNum$` cards of their library (or all of them, if the library holds fewer -- CR 701.19a's own "look at all of
them" case), ask `arrangeForScry` for a `(toTop, toBottom)` pair, then apply both halves. A player told to scry `0`
never reaches this at all (CR 701.22b, `GameAction.scry`'s own early `if (numScry <= 0) return`) -- `scryEffect.Resolve`
(scryeffect.go) checks the resolved `ScryNum$` amount for `<= 0` and returns before ever touching a library or a
controller, the identical "nothing meaningful to decide" reasoning `cleanupStep`'s own doc comment already gives for
`DiscardToHandSize`/`ChooseCardsToDiscard`.

`Defined$` needed a genuinely different default from every other M6 effect built so far:
`AbilityUtils. getDefinedPlayers`'s own first line,
`changedDef = (def == null) ? "You" : applyAbilityTextChangeEffects(def, sa)`, means an absent `Defined$` is not a
missing param at all -- it is Java's own spelling of "the ability's own controller." Every prior effect
(`DealDamage`/`GainLife`/`Pump`/`LoseLife`/`PutCounter`/`Discard`) requires `Defined$` explicitly and fails loudly
without it, because none of their own real corpus lines omit it in numbers worth building a default for. `Scry` inverts
that: 405 of its 415 real lines carry no `Defined$` at all, and every one of them means `You`. `scryEffect.Resolve`
reads `Defined$` with `a.Params.Param("Defined")`'s own two-value form and substitutes the literal string `"You"` when
the second value is `false`, then calls the unchanged `definedPlayers` (defined.go) the same way `Discard`'s own
resolved `Defined$` values already do -- `Opponent`/`Player`/`Player.Opponent` (the eight real lines that do name one)
go through the identical dispatch.

Applying `arrangeForScry`'s own answer needed a new low-level primitive this port had never had a reason to build:
putting a card on the _top_ of a zone. `Game.Move` (game.go) always appends a card to a zone's own end -- for the
library, that end is the bottom (index `len-1`), which is exactly right for `mulligan.go`'s own tuck and now for
`toBottom`'s own half of a scry (`g.Move(id, Library, pid)` for each element of `toBottom`, in the order the controller
returned it, reproducing `GameAction.scry`'s own per-element `moveToBottomOfLibrary` loop exactly: each successive call
lands one position further down, so `toBottom`'s own last element ends up the true bottom card). `toTop` needs the
opposite end, and nothing in `collect.OrderedSet` could reach it -- `Add` only ever appends. A new `OrderedSet.Prepend`
(`pkg/collect/orderedset.go`), `Add`'s own mirror inserting at index `0` instead, closes that gap generically -- the
exact shape a future caller anywhere else in the engine needing "put at the front" would also reach for, not a
scry-specific hack. `Game.MoveToLibraryTop` (game.go) is `Move`'s own mirror built on top of it: same signature shape,
same Battlefield-transition cleanup (`Counters`/`Damage`/`PT`/`TypeMod`/`ColorMod`/`KeywordMod` cleared,
`Tapped`/`SummonSick` reset, unattached, pumps cleared -- copied rather than shared with `Move` itself, since
refactoring a load-bearing, already-tested primitive for one new caller carried more risk than nine duplicated lines),
same `ZoneChanged` event, but landing via `putFront` (a new `put` sibling using `Prepend`) instead of `put`. Applying
`toTop` in the _reverse_ of its own given order and prepending each -- the identical trick `GameAction.scry`'s own
`Collections.reverse(toTop)` then per-element `moveToLibrary` (Java's own default `libPosition` of `0`) plays -- makes
`toTop[0]` land truly on top: the last prepend wins the front position, and `toTop[0]` is deliberately the last one
prepended.

`PlayerController` gained its second decision needing the new `Effect.Resolve` parameter (`Discard`'s own chunk, above,
built the first): `ArrangeForScry(g *Game, decider PlayerID, topN []CardID) (toTop, toBottom []CardID)` (control.go),
Forge's own `arrangeForScry` with the identical "trust the controller's answer" contract every other decision here
already has -- neither returned slice is checked against `topN` for completeness, the same reasoning
`ChooseLegendaryToKeep`'s own doc comment gives. `ScriptedController` gained a `scryDecisions []scryDecision` queue (a
small two-field struct, `toTop`/`toBottom` together, rather than two parallel slices that could desync under a partial
scenario edit) and a `QueueScry(toTop, toBottom []CardID)` to fill it.

`SubAbility$` (72 of 415) no longer blocks -- removed from `scryUnresolvedParams` once `resolveSubAbility` ("SubAbility
chaining itself lands," below) landed: 31 of the corpus's own 57 real SVar-defined `Scry` lines naming `SubAbility$`
chain to an already-built leaf ability and resolve end to end.

Not resolved, each failing loudly by name rather than scrying the wrong cards (PORT-8/GO-7): `ValidTgts$` (2) -- a real
target; targeting itself now exists ("Targeting itself lands," below), `Scry` just has not been extended to read
`Targeted` back yet; `Optional$` (4) -- an interactive confirm this port's own `PlayerController` has no hook for;
`Planeswalker$` (8) -- its own further mechanic. `Condition$` itself and
`ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$` (5) already skip the whole line through
`subAbilityConditionMet`'s own unresolved-param list, the identical silent-skip (not a loud error) every other effect
reading it already gets; `ConditionPresent$`/`ConditionCompare$`/ `ConditionCheckSVar$`/`ConditionSVarCompare$` resolve
through it exactly as `Discard`'s/`PutCounter`'s own already do. CR 614's own `Scry` replacement type and `Mode$ Scry`
trigger are not merely unresolved but skipped outright: 0 real corpus lines name either one, unlike `GainLife`'s own
`Mode$ LifeGained` (98 real lines, `checkLifeGainedTriggers`) -- there is nothing here to wire either mechanism into.

14 new tests (`scryeffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `scryEffect` itself being unexported (TEST-1): an absent `Defined$` defaulting to `You`, `Defined$ Opponent`
reaching the opponent's own library, a full five-card arrangement (two kept on top in a chosen order, one sent to the
bottom, two untouched cards underneath staying exactly where they were) and its all-to-bottom mirror, `ScryNum$`'s own
default and its named-SVar form (proved against a three-card library, where only a correctly-resolved count of exactly
two leaves the third, untouched card as the new top), a scry of `0` never reaching the controller, clamping `ScryNum$`
above the library's own size, skipping the controller on an empty library, `SubAbility$`/`ValidTgts$`/`Optional$` each
rejected loudly, and `ConditionCheckSVar$`'s own met/unmet pair. Two more `ScriptedController` tests gained a
queue-exhaustion entry each: `control_test.go`'s own `TestScriptedControllerEachQueuePanicsWhenExhausted` table gained a
`"scry"` row, and `mulligan_test.go`'s `scriptedMulliganController` gained a panicking `ArrangeForScry` stub.
`pkg/collect/orderedset_test.go` gained two tests of its own for `Prepend` -- insertion order and re-insertion stability
(`Add`'s own mirror image test), and index integrity after a `Prepend` the identical way `Add`'s own already proves it
after a `Remove`. `NewRegistry` (`castspell.go`) registers `APIScry`; new `enginelint` group `scryeffect`
(`id`/`card`/`game`/`player`/`ability`/`defined`/`amount`/`condition`/`control`/`zone`), `castspell` gaining it as a
dependency to register into.

## M6's eleventh effect: Surveil, Scry's own sibling decision reused wholesale

`Surveil` (CR 701.42, `SurveilEffect.java` + `Player.surveil`) landed the moment `Scry`'s own chunk (above) closed,
because CR 701.42's own shape is CR 701.19's with one substitution: look at the top `Amount$` cards of the deciding
player's own library, split them between a chosen top order and a second pile, and put that second pile into the
graveyard instead of the bottom of the library. Everything else -- the interactive reordering decision, the
default-to-`You` `Defined$` reading, the zero-amount early return, the per-player loop, the clamp to actual library
size, the skip on an empty library -- carries over from `Scry` unchanged. 183 of the corpus's 208 real
`(AB|DB)$ Surveil` lines resolve.

`PlayerController` gained a twenty-third method, `ArrangeForSurveil` (control.go): the identical
`(topN []CardID) (toTop, toSecondPile []CardID)` shape `ArrangeForScry` already has, differing only in what the second
returned slice means to its own caller. `ScriptedController`'s own answer storage did not need a second struct type for
it: `scryDecision` (a `toTop`/`toBottom` pair, already built for `QueueScry`) is reused directly for `QueueSurveil` too,
`toBottom` simply standing for "the graveyard" in a surveil decision's own context rather than "the bottom of the
library" -- the two decisions still keep entirely separate queues (`scryDecisions`/`surveilDecisions`), so a scenario
scripting both in the same test cannot have one accidentally answer from the other's slot, the identical reasoning
`QueueDiscardChoice`'s own doc comment already gives for staying apart from `QueueDiscard`.

Applying the answer needed no new primitive at all, unlike `Scry`'s own chunk: the "put back on top" half reuses
`Game.MoveToLibraryTop` (game.go) outright, in the identical reverse-then-prepend order `scryEffect`'s own loop already
uses (so `toTop[0]` ends up the new top card); the "send to the graveyard" half is a plain `Game.Move` call per card --
`Game.Move` already goes wherever its `kind` argument names, so there was never a "the library's own bottom is the only
reachable end" gap to work around here the way there was for `toTop`. `surveilEffect.go` calls
`g.Move(id, Graveyard, g.Card(id).Owner)` rather than `g.Move(id, Graveyard, pid)` -- mirroring Java's own
`getGame().getAction().moveToGraveyard(c, cause, params)`, which reads the card's own owner rather than the deciding
player, even though the two are always identical here (a card fetched from `pid`'s own library is, by Forge's own
zone-ownership invariant, always owned by `pid`) -- translating what Java's call site actually reads is worth more than
the one field access saved by assuming the equivalence holds forever.

CR 702's own Surveil-number static modifier (`StaticAbilitySurveilNum.surveilNumMod`, added to the number surveiled
before the top cards are even fetched) is not ported: 0 real corpus lines carry a keyword that would trigger it.
`T:Mode$ Surveil` is, the identical reason `T:Mode$ Scry` already is, 0 real lines corpus-wide, so nothing here checks a
trigger either. `SubAbility$` (23 of 208) no longer blocks -- removed from `surveilUnresolvedParams` once
`resolveSubAbility` ("SubAbility chaining itself lands," below) landed: 2 of the corpus's own 15 real SVar-defined
`Surveil` lines naming `SubAbility$` chain to an already-built leaf ability and resolve end to end.

Not resolved, each failing loudly by name rather than surveiling the wrong cards (PORT-8/GO-7): `ValidTgts$` --
targeting itself now exists ("Targeting itself lands," below), `Surveil` just has not been extended to read `Targeted`
back yet; `Planeswalker$` (5) -- its own further mechanic; `RememberMoved$`/`RememberKept$` (2/1) -- no
`Defined$ Remembered` resolver exists to ever read the value back, the identical "blocked outright rather than silently
no-op'd" choice `PutCounter`'s own `RememberCards$` already made; `Optional$` -- present on 0 real `Surveil` lines
today, blocked anyway for symmetry with `Scry`'s own identical param, in case a future card adds it. `Condition$` itself
and `ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$` already skip the whole line through
`subAbilityConditionMet`'s own unresolved-param list, the identical silent-skip `Scry`'s own already gets;
`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` resolve through it exactly as
`Scry`'s/`Discard`'s/`PutCounter`'s own already do.

13 new tests (`surveileffect_test.go`) drive every resolvable and every rejected shape through the real cast-and-resolve
pipeline, `surveilEffect` itself being unexported (TEST-1): an absent `Defined$` defaulting to `You`,
`Defined$ Opponent` reaching the opponent's own library, a full five-card arrangement (two kept on top in a chosen
order, one sent to the graveyard, two untouched cards underneath staying exactly where they were) and its
all-to-graveyard mirror, `Amount$`'s own default and its named-SVar form (proved against a three-card library the
identical way `Scry`'s own equivalent test is), a surveil of `0` never reaching the controller, clamping `Amount$` above
the library's own size, skipping the controller on an empty library, `SubAbility$`/`ValidTgts$` each rejected loudly,
and `ConditionCheckSVar$`'s own met/unmet pair. `control_test.go`'s own
`TestScriptedControllerEachQueuePanicsWhenExhausted` table gained a `"surveil"` row, and `mulligan_test.go`'s
`scriptedMulliganController` gained a panicking `ArrangeForSurveil` stub. `NewRegistry` (`castspell.go`) registers
`APISurveil`; new `enginelint` group `surveileffect`
(`id`/`card`/`game`/`player`/`ability`/`defined`/`amount`/`condition`/`control`/`zone`), `castspell` gaining it as a
dependency to register into.

## M6's twelfth effect: Sacrifice, and CR 701.20's own Mode$ Sacrificed trigger

`Sacrifice` (CR 701.20, `SacrificeEffect.java` + `GameAction.sacrifice`/`sacrificeDestroy`) is the corpus's own dominant
real shape past `SacValid$` absent or the literal value `Self`: 516 of the corpus's 792 real `(AB|DB)$ Sacrifice` lines
resolve (51 of them past `resolveUnlessCost`'s own new gate, "`Registry.Resolve`'s own `UnlessCost$` gate," below), more
real lines than the "no `SacValid$` at all" default this port's own effects usually resolve first.

An absent `SacValid$`, or the literal value `Self`, sacrifices the ability's own host outright -- Java's own
`valid.equals("Self")` branch -- but only if the host is still on the battlefield and still controlled by the ability's
own `Controller` (`sacrificeeffect.go`'s own `sacrificeEffect.Resolve`, the identical
`host.getController().equals(activator)` guard `SacrificeEffect.resolve` itself has), no choice asked at all: a
wrongly-reached controller call would panic the scripted controller's own empty queue.

Any other `SacValid$` value asks each of `Defined$`'s own players (default `You`, `AbilityUtils.getDefinedPlayers`'s own
null default, `scryEffect`'s own identical shape) to choose `Amount$` of their own battlefield permanents matching it,
through a new `PlayerController` method, `ChoosePermanentsToSacrifice` (its twenty-sixth) -- `ChooseCardsToDiscard`'s
own shape (`discardeffect.go`) reused for a second exactly-N-of-a-set decision, clamped to
`min(Amount$, len(candidates))` the identical way `NumCards$` already is, rather than `StrictAmount$`'s own further
"fewer than asked, sacrifice none of them" rule (not modeled). `ValidTgts$` resolves too -- `loseLifeEffect`'s own
bypass-`Defined$` pattern (`loseLifeEffect.Resolve`'s own doc comment) reused directly, since every one of the 41 real
player-shaped `ValidTgts$` lines on a `Sacrifice` line is `Opponent`/`Player`, the identical two bases `LoseLife`'s own
already covers.

`RememberSacrificed$` writes the sacrificed card onto the ability's own host card's `Memory` (`Card.Memory`,
`memory.go`) -- this port's first real writer of `Memory.Remember` at all, that method having sat dormant since
`memory.go` was first built with no caller (`memory.go`'s own doc comment). No `Defined$ Remembered` reader exists yet
to read it back through a chained `SubAbility$` (`Discard`'s own `RememberDiscarded$`/`PutCounter`'s own
`RememberCards$` already document the identical gap on the read side), so this write is presently only observable by
inspecting `Memory.Remembered()` directly.

`SubAbility$` no longer blocks: `resolveSubAbility` (subability.go, `Registry.Resolve`, effect.go) chains it through
once `Sacrifice`'s own body finishes, whether or not `subAbilityConditionMet` let that body run at all, the identical
shape every other M6 effect already has.

Not resolved, each failing loudly by name rather than sacrificing the wrong permanent, the wrong count, or silently
skipping a choice (PORT-8/GO-7): `Optional$` (46) -- an interactive "may sacrifice" confirm, the identical
ability-body-level gap `Discard`'s own `Optional$`/`Pump`'s own `Optional$` already document, distinct from CR 603.3d's
own `OptionalDecider$` a trigger carries; `ConditionDefined$` (19) and `ConditionActivationLimit$` (0) --
`SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does not cover, the identical `GainLife`/`LoseLife`-shaped
gap; `Planeswalker$` (11) -- unclear semantics on a `Sacrifice` line, not worth guessing at; `ChangeNum$` (5) --
`SacrificeAll`'s own param, never read by this `ApiType` at all, so its presence marks a line this port would
misclassify rather than one it can safely ignore; `ValidCard$` (3) -- `SacrificeEffect.java` never reads this key at
all, so its real meaning on the handful of lines naming it is unclear; `SorcerySpeed$` (1) -- a cost-restriction flag
with no cost-payment site to attach to; `SacEachValid$` (1) -- a comma-list of several `SacValid$` specs sacrificed
independently, a distribution mechanic; `Random$` (1) -- `Aggregates.random`, a randomized choice this port's own
`ChoosePermanentsToSacrifice` contract does not carry; `Destroy$` (2) -- CR 701.7's own destroy rather than sacrifice, a
different `GameAction` call and a different `Mode$` trigger entirely; `StrictAmount$` (2) -- the clamp's own opposite,
above; `Echo$`/`CumulativeUpkeep$` -- `SacrificeEffect.java`'s own two leading special-cased branches, each a whole
further upkeep-cost mechanic ahead of the ordinary sacrifice this port ports, 0 real lines combining either with
`SacValid$`/`Defined$`/`Amount$` at all. `ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/
`ConditionSVarCompare$` resolve through `subAbilityConditionMet` exactly as `Scry`'s/`Discard`'s/`PutCounter`'s own
already do. `SacrificeAll` (140 real lines, `SacrificeAllEffect.java`) is built too now ("M6's thirteenth effect:
SacrificeAll," below).

`UnlessPayer$`/`UnlessCost$`/`UnlessResolveSubs$`/`UnlessSwitched$` no longer block, removed once `resolveUnlessCost`
("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed: 51 of the corpus's own 155 real `Sacrifice` lines naming
`UnlessCost$` resolve past that gate and are actually reachable at all -- 6 of the 57 real lines that clear
`resolveUnlessCost`'s own pure-mana-cost/resolvable-payer filter still are not, each a
`S:Mode$ Continuous | AddTrigger$` line's own dynamically granted trigger (aura_flux.txt's/magus_of_the_tabernacle.txt's
own real "other permanents have 'sacrifice this unless you pay...'" among them) -- `AddTrigger$` is not a built
continuous-effect param, so the trigger it would grant never exists in this port's own game at all.

Sacrificing a card also fires CR 701.20's own `Mode$ Sacrificed` trigger, ported from `TriggerSacrificed.performTest`
into a new `checkSacrificedTriggers` (trigger.go), called from a new shared `sacrificeCards` (sacrificeeffect.go, both
the `Self` branch and the `SacValid$` branch call it) once per card actually sacrificed -- right before the zone change,
`Player.addSacrificedThisTurn`'s own call site in `GameAction.sacrifice` running before `sacrificeDestroy`'s own
`moveToGraveyard`, both ported directly, so `ValidCard$` matches the card's still-live pre-move battlefield state
(counters, PT, keywords all still there) rather than the post-move Graveyard state the separate, later
`checkDiesTriggers` call gives instead.

`checkSacrificedTriggers` does NOT split into an own-half-plus-other-half pair the way `checkDiscardedTriggers` does:
the sacrificed card is still physically on the battlefield at check time (this runs before the move), so it is already
one of the permanents a single battlefield-wide walk visits, and its own trigger is checked against itself the identical
way any other watching permanent's is. `Discard`'s own discarded card is never on the battlefield to begin with (always
a hand card), which is exactly why that dispatch needs a dedicated own-half walk and this one must not duplicate it --
walking both here would fire a sacrificed permanent's own trigger twice, a bug caught and fixed before it ever reached a
commit (an initial two-part draft was rewritten into the single walk before any test ran against it).

`sacrificedTriggerMatches` (trigger.go) checks `ValidCard$` against the sacrificed card and `ValidPlayer$` against its
own controller at the moment of sacrifice, through `matchesPlayerBase` -- `discardedTriggerMatches`'s own shape.
`PlayerTurn$`/`OptionalDecider$`/the whole `IsPresent$`/`CheckSVar$`/... family all resolve too, but generically,
through `triggerEffectAPI`'s own shared gate (`triggerPhasesCheck`/`triggerCommonRequirementsMet`/ `triggerIsOptional`)
rather than anything special-cased in this dispatch. 106 of the corpus's own 115 real `T:Mode$ Sacrificed` lines
resolve. Not resolved: `ValidCause$` (0 real lines) -- a `SpellAbility`, not a `Card`, `Matches` cannot evaluate one;
`WhileKeyword$` (1) -- `whileKeywordCheck`, a further mechanic this port does not have; `ActivationLimit$` (7) and
`ResolvedLimit$` (1) -- the identical per-turn-cap gap `LifeGained`'s own `ActivationLimit$` already documents, this
port tracking no such counter. A trigger carrying any of these four is skipped entirely, not fired unconditionally
(GO-7).

12 new tests (`sacrificeeffect_test.go`) drive every resolvable and every rejected shape through the real
cast-and-resolve pipeline, `sacrificeEffect` itself being unexported (TEST-1): the default `Self` shape sacrificing the
host with no ask, `SacValid$` asking the decider from their own battlefield, `Defined$ Opponent` reaching the opponent's
own battlefield, an absent `Defined$` defaulting to `You`, an empty candidate set skipping the controller entirely,
`RememberSacrificed$` writing `Memory.Remembered()`, a `SubAbility$` chain running after the sacrifice, `UnlessCost$`
rejected loudly, `ConditionCheckSVar$`'s own met/unmet pair, and two trigger-firing tests proving `Mode$ Sacrificed`
fires for the sacrificed card's own trigger and for a separate watching permanent's, `ValidPlayer$` narrowing which
player's own sacrifice a watcher reacts to. `mulligan_test.go`'s own `scriptedMulliganController` gained a panicking
`ChoosePermanentsToSacrifice` stub. `NewRegistry` (`castspell.go`) registers `APISacrifice`; new `enginelint` group
`sacrificeeffect` (`id`/`card`/`game`/`ability`/`defined`/`amount`/`condition`/`control`/`zone`/ `valid`/`parts`),
`castspell` gaining it as a dependency to register into.

## M6's thirteenth effect: SacrificeAll, Sacrifice's own blanket sibling

`SacrificeAll` (`SacrificeAllEffect.java`) landed the moment `Sacrifice`'s own chunk (above) closed, `pumpAllEffect`'s
own shape (pumpalleffect.go, "M6's sixth effect: PumpAll," above) reused for a second blanket effect rather than
rebuilt: an absent `Defined$` scans every battlefield in the game (Java's own `game.getCardsIn(Battlefield)`),
`ValidCards$`-filtered if present (`AbilityUtils.filterListByType`) -- 72 of the corpus's own 140 real
`(AB|DB)$ SacrificeAll` lines name no `Defined$` at all, the corpus's own dominant real shape -- and a present
`Defined$` names specific cards through `definedCards` (defined.go) instead: `Self`/`Enchanted`/`Equipped`/`Targeted`
resolve, an unrecognized value (`TriggeredObjectLKICopy`/`ChosenCard`/`Remembered`/`EffectSource`/... -- real corpus
values with no resolver, `definedCards`'s own existing error return) failing the whole line loudly rather than silently
sacrificing nothing.

`Controller$`, when present, narrows either set further to cards controlled by one of its own resolved players
(`definedPlayers`, defined.go) -- Java's own "do the controller check after LKI got updated" step, reordered here since
this port takes no LKI snapshot until the actual sacrifice happens (`sacrificeCards`, sacrificeeffect.go, below); an
unrecognized `Controller$` value (`TriggeredPlayer`, the one real corpus line naming it) fails loudly the identical way
`definedPlayers`'s own existing error return already does for `Sacrifice`.

`sacrificeAllEffect.Resolve` calls `sacrificeCards` (sacrificeeffect.go) directly, the exact same shared helper
`sacrificeEffect`'s own two branches already call -- no separate sacrifice-and-trigger logic needed for the blanket
shape at all. This means `RememberSacrificed$` and CR 701.20's own `Mode$ Sacrificed` trigger
(`checkSacrificedTriggers`, trigger.go, "M6's twelfth effect: Sacrifice," above) both come along for free, firing once
per card in the whole gathered set rather than once for the ability as a whole: a watching permanent's own life-gain
trigger fires twice for two sacrificed creatures in one `SacrificeAll` resolution, and `Memory.Remembered()` ends up
holding every sacrificed card, not just the last one.

91 of the corpus's own 140 real lines resolve. Not resolved, each failing loudly by name rather than sacrificing the
wrong set (PORT-8/GO-7): `ConditionDefined$` (3) -- `SpellAbilityCondition`'s own shape `subAbilityConditionMet` does
not cover, the identical `GainLife`/`LoseLife`/`Sacrifice`-shaped gap; `Planeswalker$` (1) -- unclear semantics, not
worth guessing at; `Activator$` (1) -- a restriction on who activated the ability rather than on what it affects, a
further mechanic; `SorcerySpeed$` (1) -- a cost-restriction flag with no cost-payment site to attach to;
`ImprintSacrificed$` (1) -- `Card.Memory` has an `Imprint` writer (memory.go) but no caller yet, not worth building for
the one real line naming it. `SubAbility$` no longer blocks ("SubAbility chaining itself landed," below): chains through
`resolveSubAbility` the identical way every other M6 effect already does.
`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` resolve through
`subAbilityConditionMet` exactly as `Sacrifice`'s own already do. `UnlessCost$`/`UnlessPayer$` no longer block either,
removed once `resolveUnlessCost` ("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed -- the identical "unless
a cost is paid" gap `Sacrifice`'s own already documented: 0 of the corpus's own 6 real `SacrificeAll` lines naming
`UnlessCost$` resolve, though. ashling*the_limitless.txt's own real pure-mana "{W}{U}{B}{R}{G}" is the only one clearing
`resolveUnlessCost`'s own pure-mana-cost/resolvable-payer filter, and its own `Defined$ DelayTriggerRememberedLKI` is
reached only through `DB$ DelayedTrigger`, a general delayed-trigger mechanic this port does not build; every other real
line names a
`PayEnergy<.../DefinedCost*.../X`-shard `UnlessCost$` or a
controller-derived `UnlessPayer$` (`EnchantedController`) this
port cannot resolve.

8 new tests (`sacrificealleffect_test.go`) drive every resolvable and every rejected shape through the real
cast-and-resolve pipeline, `sacrificeAllEffect` itself being unexported (TEST-1): a battlefield-wide `ValidCards$` scan
reaching an opponent's own permanents (not just the caster's), `.Other` excluding the ability's own host, `Controller$`
narrowing the scan to one player, the `Defined$` branch sacrificing a specific card, an unresolved `Defined$` value
rejected loudly, `UnlessCost$` rejected loudly, a `SubAbility$` chain running after the sacrifice, `RememberSacrificed$`
writing every sacrificed card (not just one) onto `Memory.Remembered()`, and `Mode$ Sacrificed` firing once per
sacrificed card rather than once for the whole ability. `NewRegistry` (`castspell.go`) registers `APISacrificeAll`;
`enginelint` group `sacrificeeffect` gained a second file (`sacrificealleffect.go`), its own existing allow-list already
covering everything the new file needs.

## Registry.Resolve's own UnlessCost$ gate, CR's own "unless a cost is paid"

`AbilityUtils.handleUnlessCost` is Java's own alternative to the ordinary `sa.resolve(); resolveSubAbilities(sa, game)`
pairing `resolveApiAbility` otherwise runs: an ability naming `UnlessCost$` decides both whether its own body runs AND
whether/when its own `SubAbility$` chains, in one place, rather than falling through to the identical unconditional
trailing call every other ability gets. A new `resolveUnlessCost` (effect.go) ports it as a branch `Registry.Resolve`
takes instead of its own `Effect.Resolve`/`resolveSubAbility` pairing whenever `a.Params` names `UnlessCost$` (a nil
`a.Params` -- `APIPermanentCreature`/`APIPermanentNoncreature`/`APIAttach`, `Ability.Params`'s own doc comment -- is
checked first, a real nil-pointer panic caught before any test ran against it: casting an ordinary creature with no
chained ability at all used to crash the moment this gate's own unconditional `a.Params.Param(...)` call ran against
it).

Each of `UnlessPayer$`'s own players (`definedPlayers`, reused; an absent value is Java's own "TargetedController"
default, not resolved -- no real corpus line among this gate's own reachable subset needs it, below) is asked a new
`PlayerController` method, `ConfirmPayCost` (its twenty-seventh), and a `true` answer actually charged through
`PayManaCost` (manapay.go) -- `payCostToPreventEffect`'s own "decide, then pay" pairing, split the identical way every
other mana decision on the interface already is (`ChoosePayMonocoloredHybrid`, ...): a confirmed payment that
`PayManaCost` cannot actually afford is not a paid cost, the identical "declined by the rules, not a bug" outcome a
failed `PayManaCost` call already carries everywhere else. `paid` accumulates across every payer with `||` -- Java's own
`alreadyPaid |= payer.getController().payCostToPreventEffect(...)` -- so any one of several payers succeeding is enough.
The ability's own body runs when `paid == isSwitched` (`handleUnlessCost`'s own comparison, ported directly),
`isSwitched` false by default and flipped by `UnlessSwitched$`'s own presence -- "pay to make it happen instead."
`UnlessResolveSubs$` decides whether the chained `SubAbility$` still runs regardless (absent, Java's own "Always") or
only on one particular outcome ("WhenPaid"/"WhenNotPaid").

Trimmed to the corpus's own one resolvable shape: a pure-mana `UnlessCost$` and an explicit `UnlessPayer$` naming
`You`/`Player`/`Opponent`/`Player.Opponent`. "Pure mana" is a new `cost.Cost.IsPureMana` (`internal/cost`) -- no named
`Part` (`Sac<.../Discard<.../PayLife<...`), no `Tap`/`Untap`/`Mandatory` token, no `XMin` -- rather than reading those
fields directly in effect.go: a first draft did read `parsed.Untap` directly and tripped `enginelint`'s own
plain-identifier matching (`group "effect" may not reference "phase"`), since `phase.go` happens to declare an unrelated
package-level `Untap` `PhaseType` constant and the tool cannot tell a struct field selector from a bare identifier
reference without full type information (its own doc comment: "go/types is not needed"). Moving the field reads into
`internal/cost` itself -- a package `enginelint` never scans at all -- was the fix, and `IsPureMana` is a genuinely
general predicate for any caller that can only pay mana, not an engine-specific workaround living in the wrong package.
A parsed `Mana` cost is then handed to `mana.Parse` directly (space-joined, mana.Parse's own accepted spelling), and
rejected if it carries an `X` shard -- deciding an X amount mid-resolution is a further mechanic this gate does not have
a question for.

56 of the corpus's 727 real `UnlessCost$` lines resolve past this gate and are actually reachable by this port at all,
spread across three already-built effects: 51 of `Sacrifice`'s own 155 (nicol_bolas.txt's own real "sacrifice CARDNAME
unless you pay {U}{B}{R}" shape, reached through an already-built `T:Mode$ Phase`/`Mode$ Attacks` trigger in every real
case, never an activated ability's own `Cost$`), 3 of `DealDamage`'s own 31 (force_of_nature.txt's own real "deals 8
damage to you unless you pay {G}{G}{G}{G}", `T:Mode$ Phase` again), and 2 of `Pump`'s own 15 (spitting_slug.txt's own
real "gains first strike... unless you pay {1}{G}", `T:Mode$ AttackerBlocked`/`Mode$ Blocks`, chaining
`UnlessResolveSubs$ WhenNotPaid` into `PumpAll` when the cost goes unpaid, and nakaya_shade.txt's own real activated
`{B}:` ability -- reachable once `ActivateAbility` landed too, `## Activating an ability lands`, below). The other 671
real lines fail one hop up the call chain rather than at this gate itself -- each already documented against its own
effect above -- falling into one of five remaining shapes: an instant or sorcery's own top-level line (`CastSpell`'s own
doc comment: "an instant or sorcery resolves into a script effect this port does not build",
wild_might.txt's/rhystic_shield.txt's/rhystic_scrying.txt's own real lines among them, the last two also blocked a
second, independent way -- rhystic_scrying.txt's own `DB$ Discard` is itself reached only by chaining out of that same
uncastable top-level spell); a line reached only through an unbuilt API's own `SubAbility$`/`RepeatSubAbility$`/...
chain link (`DB$ Effect`, `DB$ Repeat`, `DB$ GenericChoice`, `DB$ DelayedTrigger`, none built --
valiant_batrider.txt's/delaying_shield.txt's/ashling_the_limitless.txt's own real lines among them); a
`S:Mode$ Continuous | AddTrigger$` line's own dynamically granted trigger (not a built continuous-effect param,
aura_flux.txt's/magus_of_the_tabernacle.txt's own real lines among them); a non-mana cost part or an X shard once
parsed; or an unresolvable `UnlessPayer$` value (`TriggeredPlayer`, `EnchantedController`, `RememberedController`, ...).

`UnlessCost$`/`UnlessPayer$`/`UnlessResolveSubs$`/`UnlessSwitched$` no longer block any of the eight already-built
effects that named them in their own unresolved-param lists (`sacrificeEffect`/`sacrificeAllEffect`/`dealDamageEffect`/
`pumpEffect`/`pumpAllEffect`/`gainLifeEffect`/`loseLifeEffect`/`discardEffect`, each updated above) -- removing the dead
keys mattered, not just cosmetically: leaving `UnlessCost` in an effect's own blocklist after this gate already consumed
it would have re-rejected every line this gate correctly let through the moment `e.Resolve` ran. `Draw` and `PutCounter`
never blocked either key at all, a real pre-existing silent-wrong-firing gap for any line combining `UnlessCost$` with
an otherwise-resolvable shape -- closed for free now that this gate intercepts before either effect's own `Resolve`
runs, though `isu_the_abominable.txt`'s own three real `PutCounter` lines (the corpus's only `UnlessCost$`-naming
`PutCounter` lines at all) still do not resolve end to end: each is reached only through `DB$ GenericChoice` (not built)
and separately omits `Defined$` entirely, which `definedCounterTargets` has no default for
(`AbilityUtils.getDefinedCards`'s own null-defaults-to-"Self" is not ported).

8 new tests (`unlesscost_test.go`) drive `resolveUnlessCost` through the real cast-and-resolve pipeline via
`Sacrifice`'s own ETB-trigger fixture (`etbSacrificeTriggerDefParams`, sacrificeeffect_test.go), `sacrificeEffect`/
`gainLifeEffect`/`pumpEffect` all being unexported (TEST-1): a decline running the ability with the pool untouched, a
confirmed and successful payment preventing it with the pool actually charged, a confirmed but unaffordable payment
still running the ability (`ConfirmPayCost` and `PayManaCost` are two separate steps), `UnlessSwitched$` inverting the
outcome both directions, `UnlessResolveSubs$ WhenNotPaid` gating a chained `SubAbility$` both directions,
`UnlessPayer$ Player` asking every player in turn and stopping at the first success, and an absent `UnlessPayer$`
rejected loudly naming itself rather than `UnlessCost$`. `control_test.go`'s own
`TestScriptedControllerEachQueuePanicsWhenExhausted` table gained a `"confirm pay cost"` row, and `mulligan_test.go`'s
own `scriptedMulliganController` gained a panicking `ConfirmPayCost` stub. Regression-verified by temporarily
short-circuiting `Registry.Resolve`'s own new branch back to the plain `Effect.Resolve`/`resolveSubAbility` pairing and
confirming six of the eight new tests fail exactly as expected (the decline-only and chained-decline cases pass
trivially either way), then restoring it. `enginelint` group `effect` gained `defined`/`manapay` to its own allow-list
(`definedPlayers` and `PayManaCost`, both now reached directly from effect.go); no new group.

## Activating an ability lands, CR 602.2

`Player.playSpellAbility` (by way of `PlayerControllerHuman`'s own input loop) is Java's own entry point for CR 602 -- a
real priority-window action this port has no equivalent window for at all (`## The scenario harness lives partly here`,
below -- `ResolveStack` plays out only the degenerate case, nobody able to respond). A new `ActivateAbility`
(activateability.go) ports it anyway, collapsing timing to the identical sorcery-speed shape `CastSpell`'s own CR 601.3a
simplification already uses (active player, a main phase, an empty stack) -- a real instant-speed activation needs the
priority window built first, not a special case here.

10,879 real `(A:)AB$` lines exist corpus-wide -- more than any one trigger mode past `Mode$ ChangesZone` itself, and
this port's own single largest remaining action by real line count. Trimmed to the corpus's own two dominant real
`Cost$` shapes: pure mana, and pure mana plus a single Tap-self token. 6,246 of the 10,879 carry that shape (2,515 bare
`T` alone, 1,090 bare mana, 995 two mana symbols, 930 mana-plus-`T`, a long tail past those four); excluding `AB$ Mana`
itself (1,845 -- CR 605.3a's own no-stack immediate resolution, this port's own `TapLandForMana`, manaability.go,
covering only a basic land's own intrinsic version of it) leaves 4,401 real non-mana activated abilities reachable at
the shape level, 1,987 of them already naming one of the twelve already-built effects: `Pump` 993, `PutCounter` 293,
`DealDamage` 221, `Draw` 196, `PumpAll` 119, `LoseLife` 41, `GainLife` 39, `Scry` 31, `Discard` 27, `Surveil` 27 -- real
lines none of those effects' own previously-published "N of M resolves" counts include yet, since every one was computed
against cast/trigger reachability alone; recomputing each against activated-ability reachability too is a further
chunk's own work, not done here.

`cost.Cost.IsPureManaOrTap` (`internal/cost`) is `IsPureMana`'s own sibling
(`## Registry.Resolve's own UnlessCost$ gate`, above), needed because `IsPureMana`'s own flat "no `Parts` at all"
contract cannot simply add `Tap` to its own allowed set: a bare `T` token always also parses as its own named `Part`
(`namedParts`' own trailing `{name: "T", prefix: "T", exact: true}` entry, `internal/cost/parts.go`) alongside setting
the `Tap` flag itself -- `parseCostPart`'s own real Java shape, `CostPartTap` a genuine `CostPart` object described and
iterated like any other rather than only a boolean -- so `IsPureManaOrTap` allows `Parts` to hold nothing but that one
lone `"T"` entry and rejects any other. This surfaced mid-implementation, not planned: the first draft reused
`IsPureMana` directly and every `Cost$ T` case failed with `len(parsed.Parts) != 0` even though `Tap` itself parsed
correctly, caught by a debug trace before any test's own expectations were adjusted around it.

`Cost$`'s own remaining shapes -- a named Part other than `T` (`Sac<.../Discard<.../PayLife<.../...`), `Untap`/`Q`,
`Mandatory`, `XMin` -- all fail `IsPureManaOrTap` and decline the whole activation outright (PORT-8/GO-7), each its own
further payment primitive this port does not have. `AB$ Mana` itself declines too, by API name rather than by cost
shape: CR 605.3a's own no-stack immediate resolution is a different mechanism entirely, and this port's only version of
it stays land-only.

A Tap-self cost checks CR 602.5b/302.6 first, with no side effect yet: already tapped, or summoning-sick without haste,
both decline outright -- `DeclareCombatAttackers`'s own identical `SummonSick`/`HasKeyword("Haste")` check (attack.go),
reused rather than re-derived. The mana half pays through `PayManaCost` exactly as `CastSpell`'s own does; only once
that succeeds does the tap itself actually happen (`Card.Tapped` set, `checkTapsTriggers` fired) -- CR 602.2g's own
"costs are paid together" is approximated as "check every cost for feasibility first, then commit each one," so a failed
mana payment never leaves the permanent tapped for nothing.

A successful activation pushes through `pushTriggeredAbilities` (trigger.go) with the activating player as its own sole
entry -- resolving `ValidTgts$` (targeting.go) and firing CR 115's own "becomes the target" check the identical way a
triggered ability's own push already does, APNAP ordering a harmless no-op over the one player activating. This is also
why no new effect code was needed at all: every activated ability this shape reaches dispatches through the identical
`Registry.Resolve` (effect.go) a cast spell's own trigger or a triggered ability already does, so
`UnlessCost$`/`SubAbility$` chaining/`ConditionCheckSVar$`/... all come along for free the moment an ability is pushed
this way. No "activates an ability" trigger mode is checked afterward: CR 603's own remaining gap, this port has none
built to fire.

The compile layer needed no new work at all: `compile.Face.Abilities` (compile.go) has carried every `A:` line's own
compiled `Ability` since M3, `A:AB$` (`Record` `Activated`) and `A:SP$` (`Record` `Spell`, an Instant's/Adventure's own
spell half) sharing the one slice and told apart by `Record` alone -- `ActivateAbility` is simply this field's first
real reader, filtering to `Record == compile.Activated` and rejecting anything else (index selects by position within
the raw, unfiltered slice, the same "caller already knows the card's own script" contract a fixture author already has
for everything else this port drives by index rather than by name).

9 new tests (`activateability_test.go`) drive `ActivateAbility` through the real cast-and-resolve pipeline via a new
`creatureDefWithAbility` helper (`carddb.Card`/`compile.Compile`, the real param parser rather than a hand-built
`compile.Ability`, TEST-1): a bare `Cost$ T` ability tapping the source and running `Pump` through to `ResolveStack`, a
decline when already tapped, a decline when summoning-sick without haste, a pure-mana `Cost$` paying and leaving
`Tapped` false, a decline on an unaffordable mana cost, a decline for `API Mana`, a decline for a `Sac<...>` cost, a
decline for an `A:SP$` line at the same index, and a decline outside the collapsed timing window. `enginelint` group
`castspell` gained a second file (`activateability.go`), its own existing allow-list already covering everything the new
file needs (`trigger`, `manapay`, `ability`, `control`, ...). `internal/cost`'s own coverage floor needed a new table
test too (`TestIsPureManaAndIsPureManaOrTap`, parsing_test.go): `IsPureMana` itself had sat at 0% coverage within the
package's own test suite since it was added (M5 item 26's own `UnlessCost$` chunk, above) without tripping TEST-12's 90%
floor only because the package's own total statement count was large enough to absorb one small uncovered function;
`IsPureManaOrTap`'s own larger branch set (the loop over `Parts`) dropped the package to 85.5%, caught by `covergate`
before commit, closed by covering both predicates together.

## ActivateAbility's own self-sacrifice cost, `Sac<1/CARDNAME>`

`IsPureManaOrTap`'s own doc comment (above) names its remaining rejected shapes as "each its own further payment
primitive this port does not have." A corpus scan for the single largest of them found `Sac<...>` naming 1,757 of the
corpus's own 8,745 real non-`AB$ Mana` `A:AB$` lines -- more than every other rejected named Part combined (`SubCounter`
851, `AddCounter` 401, `Discard` 395, ...) -- and, within that, `Sac<1/CARDNAME>` alone (the literal self-reference
token, "sacrifice this permanent," rather than a chosen count or a chosen valid spec) naming 1,005 of the 1,757, 954 of
those with nothing else in the cost but mana and/or a Tap token. This port already has the exact payment primitive that
shape needs: `sacrificeCards` (sacrificeeffect.go), `Sacrifice`'s own "Self" branch already built for CR 701.20
(`## The eleventh effect: Sacrifice`, further up this file). Reusing it wholesale, rather than a new sacrifice-as-cost
mechanism, was the whole implementation.

`cost.Cost.IsPureManaTapAndSelfSac` (`internal/cost`) is `IsPureManaOrTap`'s own sibling, its identical
`Untap`/`Mandatory`/`XMin` rejection plus a loop over `Parts` admitting a lone `"T"` entry (as before) AND, at most
once, a `Sac` entry whose own `Field(0)`/`Field(1)` read exactly `"1"`/`"CARDNAME"` -- a chosen count
(`Sac<2/CARDNAME>`) or a chosen valid spec (`Sac<1/Creature.Other/...>`) both fail the whole predicate rather than being
silently treated as a self-sac. `Cost.SelfSac` answers whether that Part is actually present, but is not merely "safe to
call once a caller has already checked `IsPureManaTapAndSelfSac`" -- it re-checks that predicate itself first, so a cost
`IsPureManaTapAndSelfSac` would reject also answers `false` from `SelfSac` on its own, rather than reporting `true` for
a stray `Sac<1/CARDNAME>` sitting alongside some other, unrelated, unsupported Part. This was caught by
`TestIsPureManaTapAndSelfSacAndSelfSac`'s own table test itself: the first draft of `SelfSac` scanned `Parts` for the
first `Sac` entry and reported its own shape directly, with no regard for the rest of the cost, and five of the table's
own "reject everything but the bare self-sac shape" cases (two Sac Parts, `Untap`/`Mandatory`/`XMin` alongside one,
`Sac<1/CARDNAME>` next to an unrelated `Discard<...>`) failed by reporting `true` -- fixed by having `SelfSac` gate on
`IsPureManaTapAndSelfSac` itself before ever walking `Parts`.

`ActivateAbility` builds its own runtime `Ability` (the one `pushTriggeredAbilities` eventually pushes) before paying
any part of the cost now, rather than only at the very end -- `sacrificeCards` takes one, reading `RememberSacrificed$`
off the identical `Params` the pushed ability itself carries (never actually set on a real activation-cost line in the
corpus today, but the identical `*Ability` value either way, no special-cased second struct). Payment order stays mana
first, tap second (unchanged from `IsPureManaOrTap`'s own landing), with the self-sacrifice committed last, after both:
CR 601.2h's own "a cost's components may be paid in any order" makes this a free implementation choice rather than an
approximation of Java's own `CostPayment` (a part-by-part, player-cancellable payment loop with its own undo-on-cancel
machinery this port does not build) -- committing the irreversible zone change last means a failed mana payment, or a
Tap-cost decline, never leaves a permanent sacrificed for nothing, the identical "check every cost for feasibility
first" reasoning `IsPureManaOrTap`'s own landing already used for tap-after-mana. `sacrificeCards` is called with the
single activated card as its own one-element `ids` slice -- CR 701.20's own "dies" trigger (`checkSacrificedTriggers`,
then the ordinary `checkDiesTriggers`) and the batched `Mode$ ChangesZoneAll` firing all come free, exactly as they
already do for `Sacrifice`'s own "Self" branch, with no new trigger-firing code in this file at all. The pushed ability
still resolves normally afterward even though `card` is now in its owner's graveyard (CR 112.7a, "an activated ability
exists independently of its source once it is activated") -- `Ability.Source` is a plain `CardID`, and this port's own
`Defined$ Self` dispatch (`definedCards`, defined.go) already reads through it with no zone check, the identical
behavior a `SubAbility$` chain into a just-sacrificed card's own `Defined$ Self` already relies on for `Sacrifice`'s own
body (`RememberSacrificed$`, CR 701.20, above).

947 more of the corpus's own real non-`AB$ Mana` `A:AB$` lines are reachable at the shape level through this one
extension (`ChangeZone` 164, `Draw` 145, `Destroy` 94, `DealDamage` 91, `Pump` 60, `GainLife` 52, `Token` 44,
`PutCounter` 31, `Counter` 24, `PumpAll` 24, a long tail past those); 441 of the 947 already name one of the twelve
already-built effects (`Draw` 145, `DealDamage` 91, `Pump` 60, `GainLife` 52, `PutCounter` 31, `PumpAll` 24, `Discard`
16, `LoseLife` 9, `Scry` 7, `Sacrifice` 3, `Surveil` 3). Recomputing each already-built effect's own "N of M resolves"
count against this shape too -- alongside `IsPureManaOrTap`'s own identical deferred 1,987 -- stays a further chunk's
own work, not done here.

4 new tests (`activateability_test.go`): a bare `Sac<1/CARDNAME>` cost sacrificing the source and still running
`GainLife` through to `ResolveStack` with the source already gone; mana, tap and self-sac combined on one line (mana
charged, source ends in the graveyard); a decline for `Sac<2/CARDNAME>` (a chosen count past the one self-sac shape);
the existing `TestActivateAbilityDeclinesForNonPureManaCost` (a chosen-target `Sac<1/Creature>`) needed no change at
all, since `IsPureManaTapAndSelfSac` rejects it the identical way `IsPureManaOrTap` already did. `internal/cost` gained
its own table test (`TestIsPureManaTapAndSelfSacAndSelfSac`, parsing_test.go) for both new predicates together, the
identical pairing `IsPureMana`/`IsPureManaOrTap` already share one for. Regression-toggle: short-circuiting the new
`SelfSac`-gated payment branch failed exactly the two new tests that exercise it and no others, restored after
confirming.

## ActivateManaAbility lands, CR 605.3 past a basic land's own intrinsic version

`TapLandForMana` (manaability.go) is CR 305.6's own synthesized mana ability -- a land with a basic land type gets it
whether or not its own printed text carries an `A:` line at all. Every other real card that carries a printed
`A:AB$ Mana` line -- rocks, dorks, Treasures -- stayed unreachable until now, even though nothing about CR 605.3's own
payment rule differs between the two: a mana ability just resolves without the stack, whoever prints it. A fresh corpus
scan (the same tokenizer-based one `ActivateAbility`'s own self-sac chunk built, above) found 2,156 real `A:AB$ Mana`
lines corpus-wide, 1,946 of them already matching `IsPureManaTapAndSelfSac` (internal/cost) -- the identical predicate
`ActivateAbility` already uses, reused outright rather than a second one: `ActivateAbility` itself refuses API `"Mana"`
and `ActivateManaAbility` refuses anything else, so the two never compete for the same line.

`Produced$`'s own values split the 1,946: 518 write `C` (colorless), 487 combined write a single literal WUBRG letter
(146 `G`, 92 `U`, 87 `R`, 84 `B`, 78 `W`), 476 a fixed multi-symbol `Combo` list, 334 write `Any` (CR 605.3b's own
"choose a color," a player decision this port has no `PlayerController` hook for yet), 23 `Chosen` (a color picked
earlier in the same resolution, an SVar-like reference this dispatch does not follow), 108 something else or nothing at
all. `producedManaColor` (activatemanaability.go) resolves the dominant literal shape only -- the 1,005 real `C`/WUBRG
lines -- returning early for `Any`/`Combo`/`Chosen`/an absent `Produced$` rather than guessing a color (PORT-8/GO-7); CR
605.3b's own chooser is a further chunk's own work, not built here.

`manaAbilityAllowedParams` (activatemanaability.go) is a positive allow-list rather than the growing per-effect
blocklists every script-driven `Effect` in this file uses (`pumpUnresolvedParams`, `sacrificeUnresolvedParams`, ...):
`compile.Ability.Params` is directly enumerable (`[]vocab.Param`, compile.go), and a mana ability's own real param
vocabulary is small enough (`AB$`/`Cost$`/`SpellDescription$`/`Produced$`/`Amount$`, five keys, against roughly twenty
real ones corpus-wide) that naming the ones this dispatch reads is shorter than naming everything it does not. Of the
1,005 real lines clearing `producedManaColor`, 850 clear this allow-list too and resolve end to end; the other 155 name
at least one further param this port has no resolver for: `RestrictValid$` (53) tags the mana itself with a spending
restriction (CR 106.6a's own "this mana can only be spent on...") this port's own `Pool` has no bucket for at all,
distinct from every other cost or effect restriction this port already tracks; `SubAbility$` (28) chains a further
ability this immediate, no-stack resolution has nowhere to route through `Registry.Resolve` -- a mana ability's own
chain runs before the mana itself is even added, so reusing `resolveSubAbility` here would need a `*Registry` this
function does not carry and an ordering question (`Registry.Resolve`'s own trailing-chain contract, effect.go, assumes
its own effect body already ran) this file's own immediate-resolution shape does not fit cleanly; a bare `IsPresent$`/
`ConditionCheckSVar$`/`ConditionSVarCompare$`/`PresentCompare$`/`CheckSVar$`/`SVarCompare$`/`OpponentTurn$` (26
combined) is an "Activate only if..." restriction on the ability itself -- distinct from `subAbilityConditionMet`'s own
`Condition$`-prefixed pair, which no real resolving `Mana` line in the corpus actually carries, so nothing here silently
reuses that gate for the wrong key family; `TriggersWhenSpent$`/`AddsKeywords$`/`AddsKeywordsValid$`/
`AddsKeywordsUntil$` (16 combined) tag the mana itself with a further effect once it is spent, a mechanic this port's
own `Pool` cannot carry (mana in the pool is just a color and a snow flag, `Pool`'s own doc comment); `AILogic$`/
`AINoRecursiveCheck$`/`PrecostDesc$`/`Activation$` (13 combined) are AI hinting or a further cost-description gate,
neither read by anything real here.

`Amount$` resolves through `resolveNamedAmount` (amount.go) -- `pumpAmount`'s own identical plain-integer-or-named-SVar
reading, reused rather than re-derived -- defaulting to 1 when absent (`AbilityUtils.getParamOrDefault`'s own real
default for this key). Payment order matches `ActivateAbility`'s own exactly: mana first, tap second, a self-sacrifice
cost last, reusing `sacrificeCards` (sacrificeeffect.go) wholesale the identical way
(`## ActivateAbility's own self-sacrifice cost`, above) -- CR 601.2h's own "any order" reasoning applies here too, so
this is the same free implementation choice, not a second approximation of it. `checkTapsForManaTriggers` (trigger.go)
-- CR 603's own "taps for mana" trigger, `TapLandForMana`'s own pairing alongside the ordinary "becomes tapped" one --
fires only when the cost actually has a Tap component: a pure self-sacrifice mana ability (a Treasure-shaped "Sacrifice
this artifact: Add one mana of any color," when it happens to name a literal color rather than `Any`) taps nothing, so
nothing "becomes tapped to produce mana," the identical zero-Tap skip `ActivateAbility`'s own `checkTapsTriggers` call
already has.

Snow mana (CR 106.3a) is read the identical way `TapLandForMana` already reads it --
`Card.Type().HasSupertype( cardtype.Snow)` -- even though no real non-land permanent in the corpus carries both the Snow
supertype and a resolvable `AB$ Mana` line today: the check costs three lines and keeps this function's own contract
exactly as general as `TapLandForMana`'s, rather than silently wrong the day a set prints one.

8 new tests (`activatemanaability_test.go`): a literal single-color mana dork (`Cost$ T`) tapping and producing;
`Produced$ C` adding colorless rather than a sixth color; a plain-integer `Amount$` multiplying the mana produced; a
self-sacrifice cost composing the identical way it does for `ActivateAbility`; a decline for a summoning-sick,
haste-less Tap-cost source; a decline for any API but `"Mana"`; a decline for `Produced$ Any` (`ChooseManaColor`'s own
landing, below, replaced this with a positive test once the chooser existed); a decline for a `RestrictValid$`-bearing
line. `enginelint` group `manaability` gained a second file (`activatemanaability.go`) and its own allow-list grew to
admit `ability`/`manapay`/`amount`/`sacrificeeffect` -- `manaAbility`'s own runtime `Ability` literal, `PayManaCost`,
`resolveNamedAmount` and `sacrificeCards` respectively -- alongside the
`id`/`zone`/`card`/`game`/`player`/`control`/`trigger` it already had. Regression-toggle: forcing `Amount$` to always
resolve to 0 failed exactly the three tests that read a nonzero pool afterward (the plain single-color case, the
colorless case, and the self-sac case) and no others, restored after confirming.

## Produced$ Any lands, CR 605.3b's own "choose a color"

`ActivateManaAbility`'s own landing (above) left `Produced$ Any` -- 334 of the 1,946 real `A:AB$ Mana` lines matching
`IsPureManaTapAndSelfSac` -- declining outright, "no `PlayerController` hook to ask with yet." Real examples
(`resonating_lute.txt`, `radiant_lotus.txt`, `rift_sower.txt` among them) confirmed the dominant shape directly: "Add
one mana of any color" (`Amount$` absent, one unit) or "Add two mana of any one color" (`Amount$` 2) -- a single color
chosen once, every unit of it the same color, never a mixed combination -- exactly CR 605.3b's own text.

A new `PlayerController` method, `ChooseManaColor` (its 28th, control.go), asks the decider directly, taking an
`options mana.Colors` set -- `Any` itself always passes `mana.AllColors`. This is not a reuse of `ChooseHybridManaColor`
(CR 601.2h's own hybrid-payment decision, manapay.go): that method's own doc comment says its own `options` parameter
"is exactly the two colours the symbol offers" -- a real precondition, not a suggestion -- which a five-color "Any"
choice would leave silently wrong for one of that method's two callers. `options` earns its keep past `Any` alone almost
immediately -- `## Produced$ Combo lands`, below, is this exact parameter's own second real caller, a restricted
two-to-four-color subset rather than all five.

`ActivateManaAbility` (activatemanaability.go) validates the answer before using it: `mana.Colors.Count() != 1` returns
`false` (declines the whole activation) rather than passing whatever came back straight to `Pool.Add`, which panics on
anything but exactly one of White/Blue/Black/Red/Green (`Pool.Add`'s own doc comment). `ChooseHybridManaColor`'s own
real caller (`Game.PayManaCost`, manapay.go) already has this identical shape -- `mana.PureShard(choice)` returning
`ok=false` rather than trusting the answer into a panic-prone call -- so this is the established pattern for a "not
re-checked, trust the controller's answer" method whose answer feeds something that can panic, not a new one invented
for this landing. The regression-toggle check demonstrated the guard is load-bearing rather than defensive padding: a
short-circuited `&& false` on the `Count() != 1` condition turned `TestActivateManaAbilityDeclinesForInvalidChosenColor`
from a clean failed-assertion `FAIL` into an actual `panic: engine: Pool.Add wants exactly one color`, caught and
confirmed before the guard was restored.

2 new tests (`activatemanaability_test.go`): a queued `ChooseManaColor` answer producing exactly that color (replacing
the prior chunk's own decline-for-`Any` test, which this landing makes obsolete); a two-color queued answer
(`mana.Red|mana.Green`) declining rather than panicking. `PlayerController` gaining a 28th method touched both of its
real implementers the identical way every prior interface addition has: `ScriptedController` (`QueueManaColor`/
`ChooseManaColor`, control.go, `QueueHybridManaColor`'s own exact shape) and `scriptedMulliganController`
(mulligan_test.go, a stub panicking "was not expected to be called," the same as every other method that struct never
actually exercises).

## Produced$ Combo lands, CR 605.3b's own restricted-choice version of "Any"

`ChooseManaColor`'s own landing (above) closed `Produced$ Any` on the claim that "every real `Produced$ Any` line offers
all five, never a restricted subset." That claim held for `Any` specifically, but a further corpus scan of the 476 real
pure-cost `Produced$ Combo <...>` lines -- left declining outright at `ChooseManaColor`'s own landing, "a fixed
multi-symbol list whose real semantics this port has not researched" -- found exactly the restricted subset
`ChooseManaColor`'s own doc comment had already anticipated and left room for. Real examples (`rootbound_crag.txt`:
`Produced$ Combo R G`, "Add {R} or {G}"; `rattleclaw_mystic.txt`: `Produced$ Combo G U R`, "Add {G}, {U}, or {R}")
confirmed the dominant shape directly: a fixed list of two to four literal WUBRG letters, choose exactly one, add one
unit of it -- CR 605.3b's own restricted-choice sibling to "Any," not a distinct mechanic.

A new `parseComboColors` (activatemanaability.go) reads `"Combo W U"` into the `mana.Colors` bitmask `{White, Blue}`,
then `ActivateManaAbility` calls the identical `ChooseManaColor` `Any` already calls, passing that narrower set as
`options` instead of `mana.AllColors` -- the parameter `ChooseManaColor`'s own landing added specifically so a second
restricted-choice caller would not need a second interface method, now used for exactly that. The scan surfaced three
further real `Produced$` shapes past the literal letter list, all left unbuilt: `Combo Any`/`Combo AnyDifferent` (22 and
2 lines, always paired with `Amount$ 2` -- "add two mana in any combination of colors"/"...of different colors," CR
605.3b's own per-unit-independent-choice text, two separate color decisions rather than one repeated, a shape this
single-`ChooseManaColor`-call-per-activation dispatch does not model); `ColorIdentity` (6, Commander's own
color-identity set -- a format concept, distinct from a card's own printed colors, this port does not track anywhere);
and a `Chosen` token inside the list (`thriving_isle.txt`'s own real "Add {U} or one mana of the chosen color" --
`producedManaColor`'s own identical unresolved reference, an externally chosen color this dispatch has no memory slot to
read). `parseComboColors` rejects each by construction: every one of those fails "every token past `Combo` is a single
literal WUBRG letter" outright, so the whole match fails and the ability declines rather than resolving half a choice.

`parseComboColors` itself does not reject a repeated letter (`Combo R R`, not a real corpus shape but a structurally
possible one) -- `options` is a bitmask, so ORing the same bit twice changes nothing. A duplicate-rejecting check inside
`parseComboColors`' own parsing loop (an identical-looking `options.Has(color)` call, but checking the letter just
parsed against the ones already accumulated, not the final chosen color) was added during development, then toggled off
to verify it -- every test stayed green, proof the check caught nothing a real corpus line could ever trigger, and it
was dropped rather than kept as inert complexity (the anti-overengineering half of this session's own standing
discipline, not a PORT-8/GO-7 correctness question -- accepting a harmless duplicate is not "applying half a script and
guessing at the rest").

`ActivateManaAbility` itself still validates the real controller answer twice, mirroring `Any`'s own pair of checks
exactly: `color.Count() != 1` (the controller answered with something other than a single color) and
`!options.Has(color)` (the controller answered with a color the `Combo` list never offered, a controller-side bug this
dispatch catches rather than trusts) -- both decline rather than reaching `Pool.Add`'s own panic. A regression-toggle
run on this pair confirmed the `options.Has` half specifically is load-bearing: disabling just it failed exactly
`TestActivateManaAbilityDeclinesForOutOfComboColor` and no other test.

367 of the corpus's own 476 real pure-cost `Produced$ Combo` lines resolve end to end (the literal-letter-list shape,
past `manaAbilityAllowedParams`' own existing gate -- `SubAbility$`/`IsPresent$`/`ActivationLimit$`/`RestrictValid$`/
`PlayerTurn$` block the remaining 109 the identical way they already block a literal-color or `Any` line naming them, no
new blocking logic needed).

3 new tests (`activatemanaability_test.go`): a queued answer inside a `Combo R G` land's own offered set producing
exactly that color; a queued answer naming a color the land never offered (`Black` on a `Combo R G` land) declining;
`Produced$ Combo Any` (the per-unit-independent-choice shape, above) declining outright since `parseComboColors` never
matches it. `ChooseManaColor`'s own interface signature changed to add the `options` parameter -- the second edit to
this exact method inside two commits, both of its real implementers (`ScriptedController`, `scriptedMulliganController`)
updated the identical way `Any`'s own landing already updated them for the method's first addition.

## cost.Cost.ActivationShape replaces IsPureManaOrTap/IsPureManaTapAndSelfSac/SelfSac; Discard<N/Card> lands

Three chunks in a row (`## Activating an ability lands`, `## ActivateAbility's own self-sacrifice cost`, and this port's
own general mana ability landing, all above) each added one more near-identical `cost.Cost` predicate for
`ActivateAbility`/`ActivateManaAbility`'s own shared cost-shape gate: `IsPureManaOrTap`, then `IsPureManaTapAndSelfSac`,
then `SelfSac` alongside it. A corpus scan for the next real activation-cost primitive worth building -- `Discard<...>`
as a cost, 387 real non-`AB$ Mana` `A:AB$` lines naming it as the only part past mana/Tap/ self-sac, dominated by the
literal `Discard<N/Card>` shape (209 `Discard<1/Card>`, 16 `Discard<2/Card>`, 3 `Discard<3/Card>` -- 228 combined;
`Discard<1/CARDNAME>`, 67 real lines, always paired with `ActivationZone$ Hand` in the samples checked, CR 701.8a's own
"discard moves a card from hand to graveyard" ruling out a battlefield-only self-discard entirely -- `ActivateAbility`'s
own battlefield-only entry point can never reach that shape regardless of whether it were built, so it was not; the rest
-- `Discard<1/Land>`/`Discard<1/Creature>`/`Discard<1/Random>`/... -- each its own further restricted-choice or
random-choice shape) -- made the pattern impossible to ignore: a `Discard<N/Card>` count could not fit a bare `bool` the
way `Tap`/`SelfSac` could, forcing a signature change on whichever predicate grew a fourth branch, and three
near-identical predicates already sitting in the package was already the sign a fourth should not be a fourth (this
session's own standing anti-overengineering discipline cuts both ways: avoiding premature abstraction does not mean
repeating an established pattern past the point it stops paying for itself).

`cost.Cost.ActivationShape` (`internal/cost/cost.go`) replaces all three: one method returning a small struct
(`Tap bool`, `SelfSac bool`, `DiscardN int`) and a second `ok bool` result, false for anything the struct's three fields
cannot represent -- Untap/Mandatory/XMin, a chosen or SVar-sized `Sac<...>`, a `Discard<...>` past the literal
`"N/Card"` shape, or any other named `Part`, the identical rejection set the three predicates it replaces already had
between them, just unified into one walk over `Cost.Parts` instead of three separate ones. `IsPureMana` itself is
untouched -- `resolveUnlessCost`'s own "unless a cost is paid" gate (effect.go) needs a genuinely different question (no
Tap/SelfSac/Discard allowed at all, not even optionally), so it stays its own predicate rather than folding into
`ActivationShape` too.

`ActivationShape`'s own single `switch` over `Cost.Parts` gets a second-`Sac`/second-`Discard` rejection for free,
without a dedicated duplicate check the way `parseComboColors`' own first draft briefly needed one
(`## Produced$ Combo lands`, above, where the check turned out to be provably inert and was removed): each `case` guards
on the matching field still being at its zero value (`!shape.SelfSac`, `shape.DiscardN == 0`), so a second
`Sac<1/CARDNAME>` or `Discard<1/Card>` Part fails that case's own guard and falls through to
`default: return ActivationShape{}, false` instead of silently re-triggering the first branch -- `TestActivationShape`'s
own `"Sac<1/CARDNAME> Sac<1/CARDNAME>"` and `"Discard<1/Card> Discard<1/Card>"` cases both confirm `ok == false` with no
extra code past the ordinary Part-matching logic itself.

`ActivateAbility` (activateability.go) pays a `Discard<N/Card>` cost by checking the hand's own size against `DiscardN`
first (a feasibility check, alongside the Tap-self SummonSick/Haste check, both run before any commitment the same way
they always have), then -- once mana, tap, and self-sac have all committed -- asking `ChooseCardsToDiscard` for exactly
`DiscardN` cards and discarding them through a new `discardCards` (discardeffect.go), factored out of `discardEffect`'s
own per-player loop the identical way `sacrificeCards` was already factored out of `sacrificeEffect`'s: a helper two
real callers share rather than one duplicating the other's move-and-trigger pairing. The hand-size feasibility check is
load-bearing, not decorative -- the regression-toggle pass proved it directly: disabling it turned
`TestActivateAbilityDeclinesWhenHandTooSmallForDiscardCost` from a clean failed assertion into an actual
`panic: engine: scripted controller ran out of discard choice decisions` (`ChooseCardsToDiscard` asked for more cards
than any queued answer could ever supply), the identical "trust ends at the guard, not at the panic-prone call" shape
`ChooseManaColor`'s own guard already demonstrated for `Pool.Add` (`## Produced$ Any lands`, above).

`ActivateManaAbility` (activatemanaability.go) declines outright whenever `shape.DiscardN > 0` rather than silently
ignoring it: 0 real corpus `A:AB$ Mana` lines ever name `Discard<...>` as part of their own cost, so there is no real
shape to execute and no `discardCards` call site to add here -- letting an unhandled `DiscardN` through would mean
reporting the cost as fully paid while a real script asking for a discard never actually got one, exactly the "apply
half a script and guess at the rest" PORT-8/GO-7 forbids, even though no real corpus line can trigger it today.

228 of the corpus's own real non-`AB$ Mana` `A:AB$` lines are reachable at the shape level through `Discard<N/Card>`
(real examples: `ravenous_bloodseeker.txt`'s own bare `Cost$ Discard<1/Card>` naming `Pump`; `ridged_kusite.txt`'s own
`Cost$ 1 B T Discard<1/Card>`, mana and Tap and Discard combined on one line; `reverberating_summons.txt`'s own
`Cost$ 1 R Discard<1/Hand> Sac<1/CARDNAME/this enchantment>`, which does NOT resolve here -- `Discard<1/Hand>` is a
different shape, "discard your entire hand," not `producedManaColor`'s -- sorry, `ActivationShape`'s -- own
literal-count `"N/Card"` match). Recomputing each already-built effect's own "N of M resolves" count against this shape
too stays the identical deferred further-chunk work every prior `ActivationShape` extension has already left undone.

7 new tests: `activateability_test.go` gained `TestActivateAbilityDiscardCostDiscardsChosenCardsAndRunsEffect`,
`TestActivateAbilityDeclinesWhenHandTooSmallForDiscardCost`, `TestActivateAbilityCombinesTapAndDiscardCost`, and
`TestActivateAbilityDeclinesForNonLiteralDiscardCost` (a `Discard<1/CARDNAME>` cost, declining outright);
`activatemanaability_test.go` gained `TestActivateManaAbilityDeclinesForDiscardCost`. `internal/cost/parsing_test.go`
replaced its own two predicate-specific table tests (`TestIsPureManaAndIsPureManaOrTap`,
`TestIsPureManaTapAndSelfSacAndSelfSac`) with `TestIsPureMana` (the one predicate that survives untouched) and a new
`TestActivationShape` covering Tap/SelfSac/Discard alone and in every real combination, including the
`Sac<1/CARDNAME> Discard<1/Card>` case the old `TestIsPureManaTapAndSelfSacAndSelfSac` had asserted `false` for --
correctly, under the predicate that existed then -- and which now, under `ActivationShape`, correctly asserts `true`
instead: not a bug in either test at the time it was written, just the exact shape of question the refactor was for.

## PayLife<N> activation cost lands

`ActivationShape`'s own landing (above) already anticipated this: its own doc comment named `PayLife<.../PayEnergy<...`
as further payment primitives a fifth predicate would otherwise have been needed for. A fresh corpus scan for the next
real activation-cost primitive worth building -- `PayLife<...>` as a cost, 114 real non-`AB$ Mana` `A:AB$` lines naming
it as the only part past mana/Tap/self-sac/discard, dominated by a literal positive integer (52 `PayLife<1>`, 31
`PayLife<2>`, 10 `PayLife<3>`, 6 `PayLife<4>`, 3 `PayLife<5>`, 2 `PayLife<7>`, 2 `PayLife<8>`, 1 `PayLife<10>`, 1
`PayLife<50>` -- 108 combined; 6 more name `PayLife<X>`, an amount tied to the spell's own X value this port has no
resolver to plug in here, `IsPresent$`/`ConditionCheckSVar$`-shaped gap `ActivationShape`'s own doc comment already
generalizes as "no resolver," not a new omission) -- confirmed `PayLife<N>` was the natural next primitive rather than a
fresh mechanic needing its own new machinery.

`PayLifeN int` joined `ActivationShape` directly (`internal/cost/cost.go`) -- the fourth field on the struct, not a
fifth predicate: the struct's own doc comment says so explicitly ("PayLife slotted into the same struct rather than
becoming that fourth predicate all over again," "fourth" naming the near-identical-predicate count `Discard<N/Card>`'s
own landing had already stopped at). The parsing case mirrors `Discard`'s own exactly --
`case p.Name == "PayLife" && shape.PayLifeN == 0:` guards the identical way `shape.DiscardN == 0` already does, so a
second `PayLife<...>` Part falls through to `default: return ActivationShape{}, false` on its own, no dedicated
duplicate check needed, the identical "the switch's own zero-value guard already covers it" fact the prior landing's own
regression-toggle pass confirmed for `Sac`/ `Discard`.

Before writing `ActivateAbility`'s own execution code, `CostPayLife.java` and `Player.payLife` (both real Forge source,
read directly rather than assumed) settled a real question: does paying life for a cost fire the identical event/
trigger machinery an ordinary life-losing effect does, or is it its own separate thing (CR 119.3's own distinct "losing
life" vs. "paying life" language could plausibly mean either)? `Player.payLife` (Player.java:557) answers it directly --
it calls `loseLife(lifePayment, false, false, cause)` internally, the identical method `LifeLoseEffect`'s own resolve
calls, and then separately runs `TriggerType.PayLife` (a trigger mode 0 real corpus `T:` lines use, per the identical
corpus-frequency research this port's own trigger-mode landings already apply) alongside whatever `TriggerType.LifeLost`
`loseLife` itself would fire. So paying life for a cost IS the identical "life total changed" event this port's own
`LifeChanged` (event.go) already models for `loseLifeEffect` -- not a distinct, unmodeled event kind. A first draft of
this section's own doc comment claimed the opposite ("no LifeChanged event... the identical distinction loseLifeEffect's
own doc comment already draws") before the Java source was actually read -- caught and corrected in place before the
code was written to match the wrong claim, not after a test failure exposed it: reading `CostPayLife.java` was cheap,
writing an event-emission bug and finding it later would not have been.

`ActivateAbility`'s own execution (activateability.go) therefore mirrors `loseLifeEffect`'s own exactly: subtract
`PayLifeN` from `Player.Life`, then
`g.sink.Emit(Event{Kind: LifeChanged, Source: card, Target: PlayerEntity(pid), Amount: -int32(shape.PayLifeN)})` -- the
identical call shape `loseLifeEffect`'s own last line already has, `card` (the activated ability's own host) standing in
for `a.Source`. `Mode$ LifeLost`/`LifeLostAll` still fires no trigger check, the identical omission `loseLifeEffect`'s
own doc comment already justifies (0 real corpus `T:` lines name it, regardless of what caused the loss) -- confirmed
still true for this cause too, not merely assumed to carry over. Feasibility runs before commitment the same way
Tap/Discard's own already do: `shape.PayLifeN > g.Player(pid).Life` declines outright, CR 119.4's own "a life payment
can never bring the payer below 0" (paying down to exactly 0 is legal, the identical boundary `>` rather than `>=`
already encodes). The regression-toggle pass confirmed this guard is load-bearing: disabling it left every other test
green and failed only `TestActivateAbilityDeclinesWhenLifeTooLowForPayLifeCost`, a clean assertion failure rather than a
panic this time (`Player.Life` going negative is not, on its own, an engine invariant this port panics on -- unlike
`ChooseCardsToDiscard` asking for more cards than a queue holds).

`ActivateManaAbility` declines any `PayLifeN > 0` the identical way it already declines `DiscardN > 0` -- 0 real
`AB$ Mana` lines carry `PayLife<...>` either, so there is no execution path to reuse and letting the shape through
unhandled would mean claiming the cost was paid while no life was actually lost (PORT-8/GO-7).

Adding `g.Player(pid).Life` to `activateability.go` surfaced a real `enginelint` gap: the `castspell` group's own
allow-list had never needed the `player` group before (every prior primitive -- mana, Tap, self-sac, Discard -- reads
`Card`/`Ability`/`PlayerController` state, never `Player` directly), so `g.Player(pid).Life` failed enginelint outright
until `player` was added to `castspell`'s own allow-list (enginelint.json) -- the identical kind of real, caught-before-
commit gap `manaability`'s own group needed the same fix for earlier in this session.

7 new tests (`activateability_test.go`): a `PayLife<2>` cost on a `Draw` ability subtracting life, emitting
`LifeChanged`, and still resolving the draw (a library card seeded first --
`TestDiscardEffectSkipsControllerWhenHandEmpty`'s own sibling gap, a fresh `newGame` starts every player's library
empty); a decline when life is below `PayLifeN` with no side effect; `Tap` and `PayLife` composing on one line; a
decline for `PayLife<X>` (the non-literal shape); `activatemanaability_test.go` gained
`TestActivateManaAbilityDeclinesForPayLifeCost`; `internal/cost/parsing_test.go` extended `TestActivationShape`'s own
table with `PayLife` alone, combined with each of the other three primitives, and three more reject cases (`PayLife<0>`,
`PayLife<X/...>`, a duplicate `PayLife<1> PayLife<1>`).

## PayEnergy<N> activation cost lands

`ActivationShape`'s own doc comment (above, `PayLife<N>`'s own landing) already named `PayEnergy<...` as the next
candidate primitive. A corpus scan for it: 59 real non-`AB$ Mana` `A:AB$` lines name `PayEnergy<...>` as a cost part, 56
of them a literal positive integer (15 `PayEnergy<1>`, 13 `PayEnergy<2>`, 12 `PayEnergy<3>`, 5 `PayEnergy<6>`, 4
`PayEnergy<8>`, 4 `PayEnergy<4>`, 2 `PayEnergy<5>`, 1 `PayEnergy<50>`), 3 more naming `PayEnergy<X>` -- the identical
"no X-value resolver" gap `PayLife<X>` already has, not a new omission. Every real line combines it with only mana/
Tap/self-sac/PayLife -- one line (`Cost$ PayEnergy<8> Exile<1/CARDNAME>`) also names `Exile<...>`, a primitive
`ActivationShape` does not carry, so that one line stays unreachable regardless of this landing (PORT-8/GO-7's "skip the
whole line").

`PayEnergyN int` joined `ActivationShape` directly (`internal/cost/cost.go`) as its sixth field, the parsing case
mirroring `PayLife`'s own exactly (`case p.Name == "PayEnergy" && shape.PayEnergyN == 0:`, the identical zero-value
guard, no separate duplicate check needed).

Reading `CostPayEnergy.java` and `Player.payEnergy`/`loseEnergy` (`Player.java:611-629`) directly answered the same
question `PayLife<N>`'s own landing asked, with a simpler result this time: `Player.payEnergy` calls
`canPayEnergy(n) && loseEnergy(n)`, and `loseEnergy` calls `subtractCounter(CounterEnumType.ENERGY, n, this)` -- an
ordinary counter removal with **no trigger fired at all**, unlike `Player.payLife`'s own `TriggerType.PayLife` call.
Forge's own `TriggerType` enum has no `PayEnergy` (or `EnergyPaid`) mode whatsoever -- not "0 real corpus lines use it,"
the way `PayLife`'s mode is, but the mode does not exist to check for in the first place, confirmed by grepping both
`TriggerType.java` and the whole corpus for any `Mode$` naming energy. So this cost primitive needed no "skip this
trigger check" doc-comment caveat the way `PayLife`/`Discard` both do -- there is nothing here Java itself ever checks.

`Energy CounterType = "ENERGY"` joined the named constants in `counters.go`, alongside `Poison` -- the port's second
player-level counter kind. `CounterDetailEnergy` joined `event.go`'s own closed `CounterDetail` set (now nine values,
not eight) so `emitCounterChanged` -- `putCounterEffect`'s own event-emission helper, reused wholesale rather than
duplicated -- actually emits a `CounterChanged` event for it instead of silently dropping one the way it still does for
every counter kind outside the closed set. This closed-set extension had one unplanned side effect:
`TestPutCounterEffectSkipsEventForUnnamedType` (`putcountereffect_test.go`) had picked `CounterType$ ENERGY` as its own
example of a counter kind past the closed set -- true when that test was written, false the instant `Energy` joined it.
Caught by the gate sweep's own full test run, not by any special review step; fixed by swapping the example to
`EXPERIENCE` (a real Magic counter kind this port still does not name), which is still genuinely outside the closed set.
The unrelated `TestPutCounterEffectDefinedYouAddsPlayerCounter`, which also uses `CounterType$ ENERGY` as a plain
example value with no event assertion, needed no change -- it never depended on `ENERGY` being unnamed.

`ActivateAbility`'s own execution (`activateability.go`) mirrors `putCounterEffect`'s own player-counter branch:
subtract `PayEnergyN` from `g.Player(pid).Counters` via `Counters.Add(Energy, -shape.PayEnergyN)`, then
`emitCounterChanged(g.sink, card, PlayerEntity(pid), Energy, -shape.PayEnergyN)`. Feasibility runs before commitment the
identical way every other primitive's own does: `shape.PayEnergyN > g.Player(pid).Counters.Count(Energy)` declines
outright. The regression-toggle pass found a third distinct shape for what disabling a feasibility guard produces --
Discard's own disabled guard panics (`ChooseCardsToDiscard` has no cards left to hand out), `PayLife`'s own produces a
clean failed assertion (`Player.Life` has no floor of its own), and disabling this one **also** produces a clean failed
assertion, but for a different structural reason: `Counters.Add`'s own delta-clamp (`counters.go`, "the total never goes
below zero") silently clamps the over-large subtraction to 0 rather than going negative _or_ panicking, so the only
symptom is the activation wrongly succeeding and the count wrongly landing at 0 instead of unchanged -- the guard is
still load-bearing, just masked by a different piece of existing code than either of the other two primitives' guards
are.

Unlike every primitive before it, `PayEnergy` is **not** declined by `ActivateManaAbility` -- 4 real `AB$ Mana` lines
actually carry it (`aether_hub.txt`'s/`servant_of_the_conduit.txt`'s/`solar_transformer.txt`'s own real
`Cost$ T PayEnergy<1> | Produced$ Any`, "T, Pay one energy counter: Add one mana of any color," plus
`conversion_apparatus.txt`'s own `Cost$ T PayEnergy<3> | Produced$ Combo Any`, still unreachable regardless since
`Combo Any` itself is not a built `Produced$` shape). `ActivateManaAbility` pays it the identical way `ActivateAbility`
does (feasibility check, then `Counters.Add`/`emitCounterChanged`, committed after self-sac and before the pool actually
receives mana), rather than refusing a shape the corpus needs the way it still refuses `Discard`/`PayLife` (0 real
`AB$ Mana` lines for either of those). This is the first `ActivationShape` primitive both dispatch functions actually
execute rather than one declining what the other runs.

Adding `g.Player(pid).Counters` to both `activateability.go` and `activatemanaability.go` surfaced two `enginelint` gaps
at once: neither the `castspell` nor the `manaability` group's own allow-list had ever needed the `parts` group before
(`Counters`/`Energy` are declared in `counters.go`, grouped under `parts` alongside `pt.go`/`typemod.go`/...), and
`manaability` additionally needed `event` added for `emitCounterChanged` (`castspell` already had `event` from
`PayLife<N>`'s own `LifeChanged` emission). Both fixed in `enginelint.json` before the sweep went green.

9 new tests: `activateability_test.go` gained four, mirroring `PayLife`'s own set exactly -- a `PayEnergy<2>` cost on a
`Draw` ability subtracting the Energy counter, emitting `CounterChanged` with `Detail == CounterDetailEnergy`, and still
resolving the draw; a decline when Energy is below `PayEnergyN` with no side effect; `Tap` and `PayEnergy` composing on
one line; a decline for `PayEnergy<X>` (the non-literal shape). `activatemanaability_test.go` gained three: a resolving
`Cost$ T PayEnergy<1> | Produced$ Any` activation (the real `aether_hub.txt` shape) that taps, pays energy, and adds the
chosen color to the pool; a decline when Energy is too low, asserting the source stays untapped; the pre-existing
`TestActivateManaAbilityDeclinesForPayLifeCost`/`TestActivateManaAbilityDeclinesForDiscardCost` doc comments' own stale
"one of `ActivationShape`'s own four/three primitives" counts corrected to "five" in the same pass (`DOC-16`, caught
while writing this landing's own doc comment, not a new gap this landing introduced). `internal/cost/parsing_test.go`
extended `TestActivationShape`'s own table with `PayEnergy` alone, combined with each of the other four primitives, and
three more reject cases (`PayEnergy<0>`, `PayEnergy<X>`, a duplicate `PayEnergy<1> PayEnergy<1>`).

## Exile<1/CARDNAME> activation cost lands

`ActivationShape`'s own updated doc comment (`PayEnergy<N>`'s own landing) flagged `Exile<1/CARDNAME>` as
`Sac<1/CARDNAME>`'s own unbuilt sibling. A corpus scan: 61 real non-`AB$ Mana` `A:AB$` lines name `Exile<1/CARDNAME>` as
the only unbuilt cost part (everything else on the same line is mana/`T`/`PayLife`/`PayEnergy`, already carried). Three
lines stay unreachable regardless of this landing: one names `SubCounter<X/TIME>` alongside it, one names
`ExileFromGrave<2/Card>` alongside it, one names two MORE `Exile<...>` parts for different things -- each a second part
`ActivationShape` does not carry, PORT-8/GO-7's "skip the whole line." A fourth, `PayEnergy<8> Exile<1/CARDNAME>`, was
already counted unreachable in `PayEnergy<N>`'s own landing and becomes reachable in this one instead, both primitives
now built.

`SelfExile bool` joined `ActivationShape` as a plain field, the identical shape `SelfSac` already has -- unlike
`DiscardN`/`PayLifeN`/`PayEnergyN`, `Exile<1/CARDNAME>` carries no count of its own to track (the literal token is
either present or it is not), so this is the second boolean field on the struct, not a numeric one. The parsing case
sits directly after `Sac`'s own, the identical `Field(0) == "1" && Field(1) == "CARDNAME"` self-reference check.

Unlike every primitive built so far, this one needed genuinely new engine machinery, not just a new `ActivationShape`
field plus a call into an existing effect's own shared helper. `Sac<1/CARDNAME>` reuses `sacrificeCards`, itself CR
701.20's own dedicated mechanism (a `Mode$ Sacrificed` trigger already built, `Mode$ ChangesZone`'s own
Battlefield-to-Graveyard "dies" case already built). `Discard<N/Card>` reuses `discardCards`, CR 701.8's own dedicated
`Mode$ Discarded` trigger. Exile has no such dedicated corpus-relevant trigger mode: Forge's own `TriggerType.Exiled`
class exists, but only 3 real corpus `T:` lines name `Mode$ Exiled` at all -- not worth its own fourth "per-cause"
trigger dispatch alongside `checkSacrificedTriggers`/`checkDiscardedTriggers`/`checkTapsTriggers`. What a card exiled
via this cost DOES need is the general "leaves the battlefield" trigger family CR 603.6d describes -- and nothing in
this port had ever fired that family for any destination but the graveyard (`checkDiesTriggers`). Reading
`GameAction.exile`/`GameAction.moveTo` (`GameAction.java`) directly settled whether that gap mattered: `moveTo` fires
`TriggerType.ChangesZone` (deferred, `runnable = false`) for **every** zone move, unconditionally -- `checkDiesTriggers`
is this port's own specialization of that one generic Java firing point for the one destination its own call sites have
ever needed. Exiling a permanent as a cost is the first call site that needs a different destination, so this landing
built that specialization's sibling rather than skip the whole rules family:
`isExiledTrigger`/`checkExiledTriggers`/`otherExiledTriggerMatches` (new `exile.go`) are
`isDiesTrigger`/`checkDiesTriggers`/`otherDiesTriggerMatches`'s (`trigger.go`) own line-for-line structural copies,
`hasZoneOrAny(t, "Destination", Exile)` in place of `hasZoneOrAny(t, "Destination", Graveyard)`, everything else -- the
own-card walk, the other-watcher walk, the `g.LKI` read for the leaving card's own dying-state fields, collecting
matches before one `pushTriggeredAbilities` call -- identical. A corpus check for how much this actually reaches: 35
real `T:Mode$ ChangesZone` lines name `Destination$` with `Exile` in its own comma list, most already permitting
Battlefield as `Origin$` (absent, `Any`, `Battlefield`, or `Battlefield,Graveyard`); every real "whenever ~ leaves the
battlefield" line with no `Destination$` restriction at all was already counted reachable by `isDiesTrigger`'s own
wildcard and now correctly fires for an exile-caused departure too, not only a death -- a previously-silent gap this
landing closes for free everywhere it applies, not only for the one card whose own cost this chunk set out to build.

`g.LKI`'s own dying-state freeze (`game.go`'s `Move`) needed no change at all: its own guard is
`from == Battlefield && kind != Battlefield`, already general to any destination, not hardcoded to Graveyard --
confirmed by reading it before assuming a fix would be needed here, the identical "verify before assuming a gap"
discipline `PayLife<N>`'s own landing already modeled for a different question.

A new `exileCards` (`exile.go`) is `sacrificeCards`'s (`sacrificeeffect.go`) own structural sibling, simplified by one
real fact: 0 real `Exile<1/CARDNAME>` cost lines combine with anything resembling `RememberSacrificed$`'s own
`RememberExiled$` equivalent, so `exileCards` takes no `*Ability` parameter at all -- just `ids []CardID` -- rather than
threading one through purely to read a param that never appears on a real line reaching it. It still calls
`checkExiledTriggers` per card (after `Move`, so `g.LKI` is already frozen) and `checkChangesZoneAllTriggers` once for
the whole batch -- CR 603.6d's own batched trigger, reused outright with `Battlefield`/`Exile` as its own
origin/destination pair, since that function already takes both as parameters and needed no change to serve a third
destination.

`ActivateAbility`/`ActivateManaAbility` both call `exileCards` the identical way they call `sacrificeCards` -- no
feasibility check of its own (`SelfSac`'s own precedent: the source is already known to be on the battlefield by the
time any cost is paid, so there is nothing to check in advance the way `DiscardN`/`PayLifeN`/`PayEnergyN` each need).
`ActivateManaAbility` pays it rather than declining it, `PayEnergy`'s own precedent rather than `Discard`/`PayLife`'s: 1
real `AB$ Mana` line needs it (`mirrored_lotus.txt`'s own real `Cost$ T Exile<1/CARDNAME> | Produced$ Any | Amount$ 3`,
"T, Exile CARDNAME: Add three mana of any one color"). Two more real `AB$ Mana` lines also name `Exile<1/CARDNAME>`
(`ether.txt`, `black_tulip.txt`) but stay unreachable regardless -- one names `SubAbility$`, the other
`CheckSVar$`/`SVarCompare$`, both already outside `manaAbilityAllowedParams` for reasons unrelated to this landing.

A new `exile` `enginelint` group (`exile.go`) needed `id`/`zone`/`card`/`game`/`control`/`valid`/`trigger`/`ability` on
its own allow-list -- `ability` came as a surprise on the first run (`Ability`, the struct `checkExiledTriggers`/
`otherExiledTriggerMatches` build and return, is declared in `ability.go`, not `trigger.go`, the identical gap every
other trigger-checking function already crosses via its own group's `ability` entry). `castspell` and `manaability` both
needed `exile` added to their own allow-lists for the two new `exileCards` call sites.

9 new tests, mirroring `SelfSac`'s own set plus two proving the new trigger machinery directly:
`activateability_test.go` gained `TestActivateAbilitySelfExileCostExilesSourceAndRunsEffect` (zone becomes `Exile`, the
ability still resolves with its source gone), `TestActivateAbilityCombinesTapAndSelfExile`,
`TestActivateAbilityDeclinesForChosenExileCost` (`Exile<2/CARDNAME>`), and two new ones with no `PayLife`/`PayEnergy`
equivalent: `TestActivateAbilityExileCostFiresOwnLeavesBattlefieldTrigger` (the exiled card's own
`Mode$ ChangesZone | Destination$ Exile | ValidCard$ Card.Self` trigger fires, `checkExiledTriggers`' own-card half) and
`TestActivateAbilityExileCostFiresOtherWatcherLeavesBattlefieldTrigger` (a separate permanent watching `Card.Elf` fires
too, `otherExiledTriggerMatches`), both asserting `StackLen() == 2` right after `ActivateAbility` returns, no
`ResolveStack` needed -- `pushTriggeredAbilities` commits to the stack synchronously, the identical assertion shape
`TestDestroyLethalToughnessFiresDiesTrigger`/`...FiresOtherPermanentsWatchingDiesTrigger` (`trigger_test.go`) already
use for the graveyard-destination case. A new shared `creatureDefWithAbilityAndTrigger` helper
(`activateability_test.go`) builds a card carrying both an `A:` and a `T:` line at once, reusing the existing
`diesTriggerCreatureDefWithLine` helper (`trigger_test.go`, same test package) unchanged for the watcher-only card in
the second new test -- cross-file test-helper reuse the module-level `package engine_test` boundary already permits.
`activatemanaability_test.go` gained `TestActivateManaAbilitySelfExileCost`, the real `mirrored_lotus.txt` shape end to
end (tap, exile, three green mana chosen and added). `internal/cost/parsing_test.go` extended `TestActivationShape`'s
own table with `Exile<1/CARDNAME>` alone and combined, plus three reject cases (a chosen valid spec,
`Exile<2/CARDNAME>`, a duplicate).

No feasibility guard exists for this primitive (unlike every numeric one before it), so no regression-toggle pass was
run -- there is no guard to disable. What WOULD have been worth toggling, `isExiledTrigger`'s own `Destination$ Exile`
match, was instead proven directly and positively by the two new trigger tests above, rather than by disabling anything:
a wrong wildcard there would show up as `StackLen() == 1` in either test, not a panic or a silent miss.

## tapXType<N/Type> activation cost lands

The obvious next `ActivationShape` primitive by corpus size wasn't a small numeric field this time: `SubCounter<...`
(655 files, real loyalty-cost territory this port has no Planeswalker mechanic for yet, correctly deferred as its own
much larger scope item) and `tapXType<...` (191 files, 201 real `A:AB$` lines) were the two real candidates a corpus
scan turned up, and unlike every primitive built so far in this cluster, `tapXType` is a genuine choice-among-many --
"tap N untapped permanents of a type you control" -- not a self-reference (`Sac`/`Exile`) or a fixed-spec hand pick
(`Discard`). Read `CostTapType.java` in full before writing anything: `canPay`/`getMaxAmountX` build a candidate list
via `CardLists.getValidCards(payer.getCardsIn(Battlefield), type.split(";"), ...)` filtered to `CAN_TAP` (untapped, no
static restriction this port tracks), optionally removing the ability's own host card when `canTapSource` is false;
`doListPayment` taps every chosen card and fires `TriggerType.TapAll`.

Corpus accounting: 201 real `A:AB$ tapXType<...>` lines total (191 distinct files -- some cards carry it more than
once), 22 more real `A:AB$ Mana` lines. 12 name `tapXType<X/...>` (no X-value resolver, the identical gap
`PayLife<X>`/`PayEnergy<X>` already have). 19 name `.Other` in the type field explicitly. 3 combine
`withTotalPowerGE`/`sharesCreatureTypeWith` -- CostTapType's own two special-cased shapes, "total power N or greater"
and "any two share a creature type," each a genuinely different feasibility QUESTION than "N cards matching a spec," not
merely a different spec to plug into the identical machinery -- excluded outright. The type field itself is otherwise a
plain literal-type-list grammar this port's own `internal/valid`/`Matches` already handles: `Creature`, `Ally`,
`Wizard`, `Artifact;Creature` (a semicolon OR), `Creature.Other`, `Creature.Legendary`, `Permanent.token`, and so on --
nothing here needed a new valid-string property, only a syntax translation (below).

`ActivationShape` gained its first non-boolean, non-count pair: `TapTypeN int` and `TapTypeSpec string`, the type field
carried through completely raw and unvalidated. This is a deliberate departure from `DiscardN`'s own pattern (which
hardcodes the literal check `Field(1) == "Card"` right in `cost.go`): `tapXType`'s own type field has no single dominant
literal the way Discard's does, and validating an arbitrary valid-string shape needs `internal/valid`, a package
`internal/cost` does not and should not import (GO-14, near-zero dependencies) -- so the cost-layer parse stays purely
syntactic (a literal positive `N`, a non-empty type field, no second `tapXType` part) and the semantic question -- is
this spec something `Matches` can actually evaluate -- moves entirely to the engine layer, the identical split
`SacValid$`'s own dispatch (`sacrificeeffect.go`) already has between "what the cost part says" and "whether the engine
can act on it."

A new `taptype.go` holds the engine-layer machinery, all of it new: `tapTypeResolvable` refuses the two special-cased
suffixes and the one meaningless-here literal (`OriginalHost`, "the permanent this ability was originally printed on," a
card-copy concept this port's own `Card.Def` does not distinguish); `tapTypeCandidates` is `CostTapType.canPay`'s own
port -- `strings.ReplaceAll(rawSpec, ";", ",")` before `valid.Parse`, since Cost syntax's own OR separator is `;`
specifically because a literal `,` can appear in a Cost part's own trailing description field (the third body field this
dispatch never reads), while `valid.Parse`'s own OR separator is `,` (`internal/valid`'s own doc comment: "split on
`,`... because CardTraitBase splits the param that way"); `CAN_TAP` becomes a plain `!c.Tapped` read, since this port
tracks no CantTap-shaped static ability the way Java's own `CardPredicates.CAN_TAP` consults; `tapChosenPermanents` is
`CostTapType.doListPayment`'s own port, minus `TriggerType.TapAll` (2 real corpus `T:` lines, not worth a fourth
batched-tap trigger mode alongside `Mode$ Sacrificed`/`Mode$ Discarded`/the leaves-the-battlefield family) -- each
tapped card still fires the ordinary "becomes tapped" trigger (`checkTapsTriggers`) individually, the identical
simplification `Mode$ Exiled`'s own 3-line irrelevance already justified for the previous landing.

The one real correctness nuance is `CostTapType.java`'s own `canTapSource = !costHasTapSource`: when the SAME cost also
taps its own source through a separate plain `T` token, the source can never also count toward `tapXType`'s own total,
even when the type spec itself carries no `.Other` restriction at all -- 12 real corpus lines combine `T` with a bare
`tapXType<N/Creature>` naming no `.Other` (`Cost$ T tapXType<1/Creature>`, 9 lines; `Cost$ 1 T tapXType<1/Creature>`, 3
more), every one of which would silently let the source double-count for both cost components without this exclusion.
`tapTypeCandidates` takes `excludeSelf bool` for exactly this, both call sites passing `shape.Tap` itself rather than
inspecting the type spec's own text for `.Other` -- the two mechanisms are independent in Java (a spec CAN name `.Other`
even with no `T` component, `rogue_refiner.txt`'s own `tapXType<1/Rogue.Other>` among the 2 real such lines, and
`Matches`' own already-built `Other` property handles that half for free) and this landing only had to add the SECOND,
cost-shape-driven half.

A new `PlayerController` method, `ChoosePermanentsToTap` (its 29th -- `control.go`'s own top doc comment corrected from
"twenty-seven" to "twenty-nine," stale since `ChooseManaColor`'s own landing two chunks ago already made it wrong and
nobody had touched that sentence since), is `ChoosePermanentsToSacrifice`'s own shape reused for a third
exactly-N-of-a-set decision -- `QueueTapChoice`/`ScriptedController.ChoosePermanentsToTap` mirror
`QueueSacrificeChoice`/`ChoosePermanentsToSacrifice` line for line. A second `PlayerController` implementer this
package's own tests carry, `scriptedMulliganController` (`mulligan_test.go`), needed the identical new stub method (a
panic, "was not expected to be called") to keep satisfying the interface -- the one place in this session where adding
an interface method touched a file outside the immediate primitive's own chunk.

`ActivateAbility`/`ActivateManaAbility` both compute `tapTypeCandidates` once during feasibility (mirroring `hand`'s own
once-computed-reused pattern for `Discard`, not recomputing the battlefield walk a second time at commit) and decline
outright when the count falls short of `TapTypeN`, before anything else commits -- the identical feasibility-first
discipline every primitive since `PayLife<N>` has had. `ActivateManaAbility` pays `tapXType` rather than declining it,
the identical `PayEnergy`/`SelfExile` precedent: 22 real `AB$ Mana` lines need it, dominated by
`birchlore_rangers.txt`'s own real `Cost$ tapXType<2/Elf> | Produced$ Any` ("Tap two untapped Elves you control: Add one
mana of any color," no separate `T` of its own at all) -- a tapXType-tapped permanent fires only the ordinary "becomes
tapped" trigger, never "taps for mana" (`checkTapsForManaTriggers`), since it did not itself produce the mana the way
the source card's own `T` component would.

Three regression-toggle passes, one genuinely surprising: disabling the candidate-count feasibility guard turned a clean
decline into an ACTUAL PANIC in `TestActivateAbilityTapTypeExcludesSourceWhenCostAlsoTapsIt`
(`scripted controller ran out of tap choice decisions`) -- that test never queues an answer at all, since it expects a
decline before `ChoosePermanentsToTap` is ever called, so removing the guard reaches an unqueued controller call and
crashes the whole test binary, the identical shape `Discard<N/Card>`'s own first regression-toggle pass already found.
Disabling the `excludeSelf` exclusion on that SAME test flips it from a correct decline to the identical panic for a
different reason -- the source becomes its own eligible candidate (1 of 1 needed), the activation now "succeeds," and
the very next thing it does is call the unqueued `ChoosePermanentsToTap` -- confirming the exclusion is load-bearing
without needing a second, differently-shaped test. The third toggle, on `tapTypeResolvable` itself, was the surprising
one: disabling it left every test green, including `TestActivateAbilityDeclinesForUnresolvableTapTypeSpec`
(`Creature+withTotalPowerGE3` against a power-5 creature, which would satisfy the described restriction if this port
implemented it) -- `internal/valid`'s own documented fail-safe contract for an unrecognized base extends, empirically,
to an unrecognized Property too, so `Matches` already returns false for every candidate against that spec,
`tapTypeCandidates` already returns zero candidates, and the already-proven candidate-count guard already declines the
line on its own. `tapTypeResolvable` is, today, provably redundant with that fail-safe rather than the only thing
standing between a correct decline and a wrong tap -- kept in the code anyway (and its own doc comment rewritten to say
so explicitly) for the same reason this port already names unresolved params explicitly everywhere else rather than
trusting an implicit fail-safe three files away: a future change to `Matches`' own property dispatch could silently
start matching one of these two suffixes, and this guard is what would catch that before it ever reached a real game,
not after.

11 new tests: `activateability_test.go` gained six -- `TestActivateAbilityTapTypeCostTapsChosenPermanentsAndRunsEffect`
(two chosen Creatures tapped, the ability still resolves), `TestActivateAbilityDeclinesWhenNotEnoughTapTypeCandidates`,
`TestActivateAbilityTapTypeExcludesSourceWhenCostAlsoTapsIt` (the `canTapSource` proof, above),
`TestActivateAbilityDeclinesForNonLiteralTapTypeCost` (`tapXType<X/Creature>`),
`TestActivateAbilityDeclinesForUnresolvableTapTypeSpec` (`withTotalPowerGE`), and
`TestActivateAbilityTapTypeMatchesSemicolonSeparatedTypeList` (an Artifact matched by `Artifact;Creature`, proving the
`;`-to-`,` translation). `activatemanaability_test.go` gained two: `TestActivateManaAbilityTapTypeCost` (the real
`birchlore_rangers.txt` shape end to end) and `TestActivateManaAbilityDeclinesWhenNotEnoughTapTypeCandidates` (again
proving the source itself counts as a candidate when the cost carries no separate `T`, this time by requiring one more
than the board can supply rather than by excluding it). `mulligan_test.go` gained the one-line
`scriptedMulliganController.ChoosePermanentsToTap` stub. `internal/cost/parsing_test.go` extended `TestActivationShape`
with `tapXType` alone, combined with `T`/`PayLife`/`PayEnergy`, and three reject cases (`tapXType<X/Creature>`,
`tapXType<0/Creature>`, a duplicate).

## Return<1/CARDNAME> and Return<N/Type> activation costs land; Sac/Exile's own NICKNAME gap closes

`tapXType<N/Type>`'s own landing left one obvious next candidate on the table: `Return<...` (CostReturn.java), the third
and last real "move this off the battlefield" cost shape past Sac/Exile. A corpus scan split it in two, the identical
split `Sac`/`Exile` already have between a literal self-reference and a chosen count: 16 real `Return<1/CARDNAME>` lines
(`SelfSac`/`SelfExile`'s own third sibling) and 34 real `Return<N/Type>` lines (`tapXType`'s own sibling, "return N
permanents of a type you control to their owner's hand" rather than "tap N"). Both combine cleanly with mana/`T`/the six
already-built primitives; the only combos this landing still cannot reach are
`Sac<1/Creature> Return<1/CARDNAME>`/`PayEnergy<X> Sac<1/Creature> Return<1/CARDNAME>` (a chosen `SacValid$` mixed with
self-return, `ActivationShape`'s existing "only one self-reference primitive per line" contract already refuses
regardless of which one) and `T SubCounter<1/EON> Return<1/CARDNAME>` (`SubCounter`, still its own undeferred
mechanism).

Read `CostReturn.java` in full before writing anything, the identical discipline `CostTapType.java` got. The one real
design question it settled: unlike `CostTapType.java`'s own `canTapSource = !costHasTapSource`, `CostReturn`'s own
`canPay`/`getMaxAmountX` never exclude the ability's own host from the type-list branch's own candidates at all --
CostReturn has no tapped-state filter (CAN_TAP) either, since returning a permanent to hand has nothing to do with
whether it can currently tap. A permanent already tapped by an earlier `T` component of the SAME cost is therefore still
a legal `Return<N/Type>` candidate: `Cost$ T Return<1/Land>` (1 real line) can return the very land that just got tapped
for a different reason, no contradiction, since the tap happens first (`CostTap`'s own `paymentOrder()` of -1 runs
before `CostReturn`'s own 10) and nothing about the return branch cares that it did. So `returnTypeCandidates` (new
`returncost.go`) takes no `excludeSelf` parameter at all, a genuine, deliberate divergence from `tapTypeCandidates`'s
own shape rather than an oversight -- verified by reading the Java source rather than assumed to carry over from the
tapXType landing next door.

Building `Return<1/CARDNAME>`'s own self-reference check surfaced a real gap in the TWO EARLIER landings this session
already shipped: `CostPart.java`'s own `payCostFromSource()` -- the method `Sac<1/CARDNAME>`'s own check and
`Exile<1/CARDNAME>`'s own check are both, in effect, hand-inlined copies of -- has always accepted `NICKNAME` as equally
literal a self-reference token as `CARDNAME`
(`return this.getType().equals("CARDNAME") || this.getType().equals("NICKNAME")`, an alternate-name reference some real
cards carry). Neither the `Sac<N/CARDNAME>` landing nor the `Exile<N/CARDNAME>` landing ever checked for it. A corpus
grep the instant this was noticed: 10 real `Sac<1/NICKNAME>` lines
(`syr_ginger_the_meal_ender.txt`/`kagemaro_first_to_suffer.txt`/`linvala_shield_of_sea_gate.txt` among them) and 1 real
`Exile<1/NICKNAME>` line -- 11 real corpus lines that were silently declining every activation attempt rather than
paying the cost, for both this session's earlier chunks' whole lifetime until now. Not a hypothetical found by
re-reading old code for its own sake: it was found because writing `Return`'s own check the third time made the missing
generalization impossible not to notice. Fixed for all three primitives in the identical pass, with one new shared
`isSelfReferenceField(field string) bool` helper (`internal/cost/cost.go`) replacing three separate `== "CARDNAME"`
comparisons that would otherwise have needed the identical `|| == "NICKNAME"` tacked onto each independently -- the
exact "three near-identical predicates" smell `ActivationShape`'s own original landing already named as the reason to
build one struct instead of separate checks, recurring one level down inside a single field comparison instead of at the
top-level shape-detection layer this time.

`ActivationShape` gained `SelfReturn bool` (`SelfSac`'s own third sibling) and `ReturnTypeN int`/`ReturnTypeSpec string`
(`TapTypeN`/`TapTypeSpec`'s own sibling) -- both `Return` cases live in the same switch, disambiguated by
`isSelfReferenceField(p.Field(1))`/`!isSelfReferenceField(p.Field(1))`: Go permits two `case` arms naming the identical
`p.Name` as long as their own boolean guards are mutually exclusive, which these are by construction (a `Return` part's
own type field is either a self-reference or it is not, never both). Both cases also share a guard against EITHER
already being set (`!shape.SelfReturn && shape.ReturnTypeN == 0`), so a second `Return` part of any shape -- two
self-returns, two type-choices, or one of each -- falls through to the same `default: return false` a duplicate
`Sac`/`Discard`/`PayLife` part already does, with no separate duplicate-detection code needed for the
two-shapes-in-one-Part-name situation this is the first primitive to actually have.

A new `returncost.go` holds `returnCards` (`exileCards`'s own structural sibling, `exile.go` -- Move to Hand instead of
Exile, the identical no-`*Ability`-parameter simplification for the identical reason: 0 real `Return<...>` cost lines
carry a Remember-shaped param) and the third sibling of CR 603.6d's own "leaves the battlefield" trigger family this
session has now built: `isReturnedTrigger`/`checkReturnedTriggers`/`otherReturnedTriggerMatches`, `isExiledTrigger`'s
own exact structural copy with `Destination$ Hand` in place of `Exile`. `Mode$ Exiled`'s own 3-real-line irrelevance has
no analogue to check for Return at all -- Forge's own `TriggerType` has no dedicated "returned to hand" mode whatsoever,
only the generic `Mode$ ChangesZone` family this landing already builds for it.

A new `PlayerController` method, `ChoosePermanentsToReturn` (its 30th -- `control.go`'s own top doc comment corrected
from "twenty-nine" to "thirty," the identical one-chunk-stale gap `tapXType`'s own landing already found and fixed for
the count before it), is `ChoosePermanentsToTap`'s own shape reused for a fourth exactly-N-of-a-set decision.
`scriptedMulliganController` (`mulligan_test.go`) needed the matching stub again, the identical mechanical consequence
`ChoosePermanentsToTap`'s own landing already had.

`ActivateManaAbility` declines both new shapes outright, `Discard`/`PayLife`'s own "0 real benefit" precedent rather
than `PayEnergy`/`SelfExile`/`tapXType`'s own "pay it" one: the sole real `AB$ Mana` line naming `Return<1/CARDNAME>`
(`Cost$ R Return<1/CARDNAME> | Produced$ C C R | Amount$ 1 | SorcerySpeed$ True`) is already unreachable for a reason
unrelated to this landing (`SorcerySpeed$`, not in `manaAbilityAllowedParams`, an "Activate only as a sorcery"
restriction this dispatch has never read), and 0 real `AB$ Mana` lines name `Return<N/Type>` at all -- this landing
checked both counts before deciding, rather than assuming the `tapXType` precedent would automatically repeat.

One regression-toggle pass: disabling the `ReturnTypeN` candidate-count feasibility guard produced the identical panic
shape `tapXType`'s own guard toggle already found -- `TestActivateAbilityDeclinesWhenNotEnoughReturnTypeCandidates`
never queues a `ChoosePermanentsToReturn` answer since it expects a decline first, so removing the guard reaches the
unqueued controller call and crashes the whole test binary (`scripted controller ran out of return choice decisions`)
rather than failing its own assertion cleanly. The `isReturnedTrigger`/`Destination$ Hand` match itself was not toggled,
for the identical reason `isExiledTrigger`'s own was not: it was proven directly and positively by two new trigger tests
instead (below), the same choice `Exile<1/CARDNAME>`'s own landing already made.

13 new tests: `activateability_test.go` gained eight, mirroring the `SelfExile`/`tapXType` sets' own shapes combined --
`TestActivateAbilitySelfReturnCostReturnsSourceAndRunsEffect`, `TestActivateAbilityCombinesTapAndSelfReturn`,
`TestActivateAbilityDeclinesForChosenReturnCost` (`Return<2/CARDNAME>`),
`TestActivateAbilityReturnCostFiresOwnLeavesBattlefieldTrigger` and
`TestActivateAbilityReturnCostFiresOtherWatcherLeavesBattlefieldTrigger` (`Exile<1/CARDNAME>`'s own two trigger tests,
mirrored exactly, `Destination$ Hand` in place of `Exile`, reusing `creatureDefWithAbilityAndTrigger`/
`diesTriggerCreatureDefWithLine` outright), `TestActivateAbilityReturnTypeCostReturnsChosenPermanentsAndRunsEffect`,
`TestActivateAbilityDeclinesWhenNotEnoughReturnTypeCandidates`, and
`TestActivateAbilityDeclinesForNonLiteralReturnTypeCost` (`Return<X/Creature>`). `activatemanaability_test.go` gained
two decline tests (`TestActivateManaAbilityDeclinesForSelfReturnCost`/`...ForReturnTypeCost`), plus corrected two more
of its own pre-existing doc comments' stale "one of `ActivationShape`'s own five primitives" counts to "nine" in the
same pass (`DOC-16`, the identical kind of drive-by staleness fix `tapXType`'s own landing already made for a different
pair of comments). `mulligan_test.go` gained the `ChoosePermanentsToReturn` stub. `internal/cost/parsing_test.go`
extended `TestActivationShape` with `Return<1/CARDNAME>`/`Return<1/NICKNAME>` alone and combined with `T`,
`Return<N/Type>` alone and combined, `Sac<1/NICKNAME>`/`Exile<1/NICKNAME>` (the fixed gap, above), and five reject cases
(`Return<X/Land>`, `Return<0/Land>`, a duplicate self-return, a duplicate type-return, and one self-return plus one
type-return on the same line).

## Mode$ ChangesZoneAll lands, CR 603.6d's own batched trigger

`TriggerChangesZoneAll.performTest` is `Mode$ ChangesZone`'s own batched sibling: rather than firing once per card the
way the ordinary Dies/ETB triggers do, it fires once for a whole GROUP of cards that changed zones together in one game
action -- the real reason a board wipe's own "whenever one or more creatures you control die" trigger asks `Amount$`
(how many died) rather than firing five separate times for five simultaneous deaths. Java's own mechanism is
`CardZoneTable`: any `moveTo` call during one ability's resolution (or one SBA pass) accumulates into a shared table,
consulted once at the very end via `zoneMovements.triggerChangesZoneAll(game, sa)`.

This port does not build a general `CardZoneTable` threaded through every mover in the engine -- that would touch every
multi-card `Move` loop this port has, a disproportionately large refactor for a trigger mode whose own real corpus lines
split fairly evenly across several different real-world "batch" shapes this port cannot all reach yet
(`Destination$ Battlefield` batches need a token-creation effect this port does not have, `Destination$ Exile` needs a
batch-exile effect this port does not have either). Instead, `checkChangesZoneAllTriggers` (new, trigger.go) takes the
already-gathered batch directly: `cards []CardID`, plus one shared `origin`/`destination` pair rather than a per-card
table -- every real call site this port has today moves its whole batch through one uniform zone pair (Battlefield to
Graveyard), so this loses nothing observable yet. A future call site mixing origins within a single batch would need a
richer per-card table, not built.

Two real call sites feed it: `sacrificeCards` (sacrificeeffect.go) --
`SacrificeEffect.java`'s/`SacrificeAllEffect.java`'s own trailing `zoneMovements.triggerChangesZoneAll(game, sa)` call,
ported directly, so a plain `Sacrifice` fires it with a one-card batch and `SacrificeAll` with however many it actually
sacrificed -- and `destroyLethalToughness`/ `destroyDamagedCreatures` (action.go, CR 704.5f/CR 704.5g+h's own
simultaneous SBA sweeps): each already collects its own `dead []CardID` before moving any of them (`## Not ported yet`'s
own note that every SBA in this file collects candidates before `Move` runs), so calling the new dispatch once after
each sweep's own move-and-trigger loop was the whole change.

Every card in a batch has already left the battlefield by the time `checkChangesZoneAllTriggers` runs (every real call
site moves first, checks second), so unlike `checkSacrificedTriggers` -- which runs _before_ the move, and needs no
separate own-half walk for exactly that reason -- there is no own-half/other-half split to get wrong here either, for
the mirror-image reason: a card that was itself part of the batch is no longer on the battlefield to be asked about its
own trigger, `otherDiesTriggerMatches`'s own doc comment gives the identical reasoning. A single walk over every
remaining battlefield permanent, watching for the batch, is the whole dispatch.

`Destination$`/`Origin$` resolve through `hasZoneOrAny` (trigger.go), reused outright from the ETB/Dies dispatch built
for the identical params on `Mode$ ChangesZone` itself. `ValidCards$` (`changesZoneAllMatchingCards`, new) matches each
card in the batch against `g.LKI(id)` when a snapshot exists rather than the card's live state -- the identical "look
back in time" `checkDiesTriggers` already needs for a `Destination$ Graveyard` line to see the card's own pre-move
power/toughness/type/keywords/counters rather than the printed-only state `Move` has already reset it to.
`PlayerTurn$`/`OptionalDecider$`/the whole `IsPresent$`/`CheckSVar$`/... family all resolve too, but generically,
through `triggerEffectAPI`'s own shared gate, the identical free ride every other trigger mode reaching that chokepoint
already gets.

77 of the corpus's own 126 real `T:Mode$ ChangesZoneAll` lines resolve. Not resolved: `ActivationLimit$` (41) -- the
identical per-turn-cap gap `LifeGained`'s own `ActivationLimit$` already documents, this port tracking no such counter;
`ValidCause$` (4) -- a `SpellAbility`, not a `Card`, `Matches` cannot evaluate one; `ResolvedLimit$` (3) -- the
identical unresolved family several other dispatches already skip; `NoResolvingCheck$`/`InvertValidCause$` (1 each) --
each unclear semantics, not worth guessing at from one real line; `FirstTime$` (1) -- `CardUtil.getThisTurnEntered`, a
further "already entered earlier this turn" mechanic this port does not build. A trigger carrying any of these six is
skipped entirely, not fired unconditionally (GO-7).

`destroyLethalToughness` and `destroyDamagedCreatures` each fire their own separate `ChangesZoneAll` batch: a real,
narrow gap against CR 704.3's own "all applicable state-based actions are performed simultaneously as a single event" --
two creatures, one killed by each of this port's own two separate CR 704.5 functions in the identical
`CheckStateBasedActions` call, fire two batches rather than one shared one. This port splits CR 704.5 across one
function per clause rather than Java's single combined pass (`CheckStateBasedActions`'s own doc comment already notes
the split), and unifying the two into one shared batch would mean threading a table through `CheckStateBasedActions`
itself rather than each SBA function owning its own -- not built, and not observable against a corpus with no real card
whose own `ChangesZoneAll` line cares which SBA clause killed which creature, only that one or more matching creatures
died at all.

5 new tests (`changeszoneall_test.go`) drive the dispatch through the real cast-and-resolve pipeline and through
`CheckStateBasedActions` directly, `checkChangesZoneAllTriggers` itself being unexported (TEST-1): a `SacrificeAll`
sacrificing two creatures fires the trigger once (life +5, not +10, proving "once for the batch" rather than "once per
card" without needing `Amount$`, which this port does not resolve), `ValidCards$` filtering out a batch with no matching
card, `Destination$` rejecting a batch whose real destination does not match, two creatures reduced to zero toughness
and killed by the identical `destroyLethalToughness` SBA sweep firing the trigger once (proving the action.go wiring
specifically), and `ActivationLimit$` skipping the whole line. Regression-verified by temporarily removing the new call
from `sacrificeCards` and confirming the batch test fails exactly as expected, then restoring it.

## Mode$ DamageDoneOnce lands, CR 603's own damage-batched trigger

`Mode$ DamageDoneOnce` is `Mode$ DamageDone`'s own batched sibling -- `Mode$ ChangesZoneAll`'s own shape (above) applied
to damage instead of zone changes -- and the corpus's own single largest remaining trigger mode at 206 real lines, ahead
of `ChangesZoneAll`'s own 126. `TriggerDamageDoneOnce.performTest` fires once per target that was dealt damage within
one damage-dealing action, summing every source that hit it, rather than once per `(source, target)` pair the way the
ordinary `DamageDone` trigger already fires. CR 510.2's own "all combat damage is dealt simultaneously" is why: a
creature blocked by two others takes damage from both blockers in the identical combat damage step, and a card reading
"whenever a creature is dealt damage" has to see the combined total once, not the same creature's own trigger firing
twice.

Java's own mechanism is `CardDamageTable`, a `Table<Card source, GameEntity target, Integer amount>` built up over one
whole damage-dealing action (`GameAction.dealDamage`'s own `damageMap`/`preventMap` parameters) and consumed once at its
very end via `damageMap.triggerDamageDoneOnce(isCombat, game)` -- itself firing four different trigger types from the
one table (`DamageDoneOnce` grouped by target, `DamageDealtOnce` grouped by source, `DamageDoneOnceByController` grouped
by target-and-controller, and `DamageAll` for the whole table at once). This port builds only the biggest of the four,
`DamageDoneOnce` itself; the other three are each their own further grouping over the identical table, not built.

`damageTable` (`[]damageEntry`, new, trigger.go) is `CardDamageTable`'s own port: `Source CardID`, `Target EntityID` (a
card or a player, the same mixed shape `AttackersDeclared`'s own `AttackedTarget$` already needed
`attackedTargetMatches` for), `Amount int` -- the actual amount dealt after prevention/replacement, never the raw
pre-reduction number a caller first computed. Declared in trigger.go rather than combatdamage.go, where it is built:
enginelint's own layering would otherwise need `trigger` to depend on `combatdamage` on top of `combatdamage` already
depending on `trigger` (to call `checkDamageDoneTriggersToCard`/`ToPlayer`), a cycle the tool refuses -- every builder
of a table already reaches trigger.go through its own existing dependency on it, so declaring the type there instead
costs nothing.

`dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go) each gained a `table *damageTable` parameter: when non-nil,
the actual dealt amount is appended to it right where the existing per-exchange `checkDamageDoneTriggersToCard`/
`ToPlayer` call already sits, so the two happen at the identical point in the code for the identical reason -- this is
the amount that actually happened, after `damagePrevented`/`damageReplaced` (or their player-shaped twins) have already
run. `dealAttackerDamage`/`dealAttackTargetDamage` (combatdamage.go) just thread the pointer through unchanged. A nil
table (no real call site passes one today) means "not accumulating," the identical opt-out a missing parameter would
otherwise force every caller to build an unused table just to satisfy the signature.

Two real callers build one and consume it: `dealCombatDamageStep` (combatdamage.go) declares `var table damageTable`
once per first-strike-or-regular damage sub-step, threads `&table` through every exchange the step makes, and calls
`g.checkDamageDoneOnceTriggers(controller, table, true)` once after its own loop finishes -- CR 510.2's own simultaneity
boundary is exactly one sub-step, not the whole combat phase (first strike damage and regular damage are two genuinely
separate events, CR 510.4). `dealDamageEffect` (dealdamageeffect.go) builds its own local table per resolution -- more
than one entry when `Defined$` names several players at once (`Defined$ Player`, every player) -- and calls
`checkDamageDoneOnceTriggers` with `isCombat` false once its own loop of `dealPlayerDamage` calls (or the single
`Defined$ Self` call) finishes, so a script-driven ability's own damage is one batch too, not just a combat step's.

`checkDamageDoneOnceTriggers` (trigger.go) groups the table by target first, in first-seen order (GO-12) -- a plain
`map[EntityID][]damageEntry` keyed off an `order []EntityID` slice, since Go's own map iteration order is not stable
enough to trust for anything a card script's own resolution order could depend on. For each target, every watching
permanent's own `Mode$ DamageDoneOnce` trigger is checked once: `CombatDamage$` against `isCombat`
(`TriggerDamageDoneOnce.performTest`'s own first check); the summed amount -- `damageDoneOnceAmount`, new, ports
`TriggerDamageDoneOnce.getDamageAmount` directly: every entry in the target's own group, filtered first to only the ones
whose `Source` matches `ValidSource$` when the line names one, summed -- checked against `DamageAmount$`
(`damageAmountMatches`, `DamageDone`'s own dispatch, trigger.go, reused outright, the identical operator/operand parse
and the identical `TargetToughness` special case, computed once per target rather than once per line since every line
checked against the identical target shares the identical toughness); and `ValidTarget$` against the target itself
(`attackedTargetMatches([]EntityID{target}, ...)`, `AttackersDeclared`'s own dispatch, reused at its one-element case,
the identical shape `checkDamageDoneOnceTriggers`' own damage-batched sibling `checkChangesZoneAllTriggers` (above)
already reuses a sibling dispatch for). Every target's own live state is still current when this runs (called before any
state-based action can move a lethally damaged creature to the graveyard), so no `g.LKI` lookback is needed the way
`checkSacrificedTriggers`'/`checkChangesZoneAllTriggers`' own damage-adjacent siblings need one -- those run after their
own move, this runs before any SBA has had a chance to.

`PlayerTurn$`/`OptionalDecider$`/the whole `IsPresent$`/`CheckSVar$`/... family all resolve too, but generically,
through `triggerEffectAPI`'s own shared gate, the identical free ride every other trigger mode reaching that chokepoint
already gets. 200 of the corpus's own 206 real lines resolve. Not resolved: `ResolvedLimit$` (2) and `ActiveZones$` (2)
-- neither read by `TriggerDamageDoneOnce.performTest` at all, real meaning on the handful of lines naming either
unclear; `DamageSource$` (1) -- an object reference this port has no resolver for; `FirstTime$` (1) --
`GameEntity.getAssignedDamage`, a per-target running total across the whole turn this port tracks nowhere. A trigger
carrying any of these four is skipped entirely, not fired unconditionally (GO-7).

6 new tests (`damagedoneonce_test.go`) drive the dispatch through the real combat-damage and cast-and-resolve pipelines,
`checkDamageDoneOnceTriggers` itself being unexported (TEST-1): a double-blocked attacker firing once for its combined
1+1 damage (life +5, not +10, proving "once for the batch" without needing `DamageAmount$`'s own value at all -- the
identical no-`Amount$`-needed proof `ChangesZoneAll`'s own tests already used), two unblocked attackers hitting one
player firing once for their combined 3+4, `ValidSource$` filtering an Elf-and-Goblin double block's own summed damage
down to the Elf blocker's 1 before `DamageAmount$ EQ1` is checked against it (an unfiltered sum of 2 would fail that
check and never fire, so a pass proves the filter genuinely ran first), a `DealDamage` hitting every player firing once
per player rather than merging every player into one shared batch, `CombatDamage$ True` rejecting a non-combat
`DealDamage`, and `ResolvedLimit$` skipping the whole line. Regression-verified by temporarily removing the new call
from `dealCombatDamageStep` and confirming the double-block test fails exactly as expected, then restoring it.

## Mode$ DamageDealtOnce and Mode$ DamageAll land, DamageDoneOnce's own table-sharing siblings

`CardDamageTable.triggerDamageDoneOnce` (Java, already quoted in the previous section) does not fire only
`Mode$ DamageDoneOnce` off its own table -- it fires four trigger types off the identical one, in sequence:
`DamageDealtOnce` (grouped by source), `DamageDoneOnce` (grouped by target, "M6's own damage-batched trigger," above),
`DamageDoneOnceByController` (grouped by target and by each of the damaging sources' own controllers), and `DamageAll`
(the whole table at once, no grouping). This port already had the table (`damageTable`, trigger.go) and the two real
call sites that build one (`dealCombatDamageStep`, combatdamage.go; `dealDamageEffect`, dealdamageeffect.go) from the
previous chunk -- reusing both for two more of Java's own four trigger types was cheap enough to do in the same sitting
once `DamageDoneOnce` itself was done and tested.

A new `checkDamageTableTriggers` (trigger.go) is what both real call sites actually invoke now, in place of a bare
`checkDamageDoneOnceTriggers` call: it runs all three built dispatches off the one table, `CardDamageTable`'s own real
sequencing ported directly (`DamageDoneOnceByController`, the fourth, is not built -- 0 real corpus lines name it, so
there is nothing to wire it into). A future fifth table-driven mode, if the corpus ever needs one, has exactly one call
site to add to, not two.

`checkDamageDealtOnceTriggers` (`Mode$ DamageDealtOnce`, ported from `TriggerDamageDealtOnce.performTest`) is
`checkDamageDoneOnceTriggers`'s own mirror image: `bySource`, not `bySource[e.Target]`, groups the table -- a
gang-blocked attacker splitting its power between two blockers is one source (the attacker) dealing damage to two
targets (the blockers) in the same combat damage step, and this fires once for the attacker's own combined total, not
once per blocker it hit. `ValidSource$` matches the source directly through the ordinary `Matches` (a `Card`, unlike
`DamageDoneOnce`'s own mixed card-or-player target), the dominant real shape being the literal `Card.Self` -- "whenever
this creature deals damage" -- 49 of the corpus's own 49 real lines name it. `ValidTarget$`, when present, plays
`ValidSource$`'s own dual role from `checkDamageDoneOnceTriggers` in reverse: it both filters which of the group's own
entries count and sums only those (`damageDealtOnceAmount`, new, `TriggerDamageDealtOnce.getDamageAmount`'s own
dispatch, ported directly, `attackedTargetMatches` reused at its one-element case for the mixed target shape), and gates
the whole line on that filtered sum being positive -- the identical
`if hasParam(ValidTarget) { if amount <= 0 return false }` shape `DamageDoneOnce`'s own `ValidSource$` check already
has, mirrored. `DamageAmount$` is not a real param on this mode at all (`TriggerDamageDealtOnce.performTest` never reads
it), so no `damageAmountMatches` call is needed here the way `DamageDoneOnce`'s own dispatch has one. 47 of the corpus's
own 49 real lines resolve; not resolved: `AtLeastOneInstance$` (1) -- "at least one single damage instance meets this
comparison" (a `fullParam.substring`/`Expressions.compare` check against each individual entry's own amount, not the
summed total -- a genuinely different shape this dispatch has no evaluator for); `ActivationLimit$` (1) -- the identical
per-turn-cap gap `LifeGained`'s own already documents. A trigger carrying either is skipped entirely, not fired
unconditionally (GO-7).

`checkDamageAllTriggers` (`Mode$ DamageAll`, ported from `TriggerDamageAll.performTest`) is the simplest of the three:
no grouping at all, firing once for the whole action whenever `table.filteredMap(ValidSource$, ValidTarget$, ...)` would
be non-empty in Java -- ported as `damageAllTableMatches` (new), which short-circuits on the first table entry matching
BOTH `ValidSource$` and `ValidTarget$` together (either absent is a pass for its own half) rather than building and
returning the filtered table itself, since nothing downstream of the emptiness check ever reads it back. 9 of the
corpus's own 9 real `T:Mode$ DamageAll` lines resolve -- every param this mode's own real lines carry
(`ValidSource$`/`ValidTarget$`/`CombatDamage$`/`PlayerTurn$`/`OptionalDecider$`) already has a resolver somewhere in
this port, the first trigger mode built this session with zero real unresolved lines left over.

6 new tests (`damagetabletriggers_test.go`) drive both dispatches through the real combat-damage and cast-and-resolve
pipelines, `checkDamageDealtOnceTriggers`/`checkDamageAllTriggers` both being unexported (TEST-1): a gang-blocked
attacker's own split damage firing `DamageDealtOnce` once for the combined 2+3 total (not twice), `ValidTarget$`
filtering an Elf-and-Goblin double block's own per-target amounts down to the Elf blocker's own 0 (a real negative
control: assigning 0 to the Elf blocker and 5 to the Goblin blocker means an unfiltered implementation summing every
target the attacker hit would see 5 and wrongly fire, while the correctly filtered sum is 0 and must not),
`ActivationLimit$` skipping `DamageDealtOnce`, `DamageAll` firing once for a double block's own four separate exchanges
rather than once per exchange, `DamageAll` rejecting `ValidTarget$ Player` against an all-creature combat (proving the
filter is genuinely checked, not a pass-through), and `DamageAll` firing for a non-combat `DealDamage` too, the
identical wiring proof `DamageDoneOnce`'s own tests already gave. Regression-verified by temporarily narrowing
`checkDamageTableTriggers` back down to just its own `checkDamageDoneOnceTriggers` call and confirming every new
positive test in this chunk fails exactly as expected, then restoring it.

## Targeting itself lands

Every M6 effect built so far -- `Pump`, `PumpAll`, `LoseLife`, `PutCounter`, `Discard`, `Scry`, `Surveil` -- blocks
`ValidTgts$` outright in its own `Resolve`, each one's own doc comment naming it "this port's own targeting gap." CR
601.2c (a spell) and 603.3b (a triggered ability) both put "choose targets" at the moment the ability goes on the stack,
alongside the identical-timing choices this port already has real content for (`Ability.Target`, `Attach`'s own
single-Aura-target shape, `castAura`, castspell.go) -- targeting itself was always going to need a real answer before
most of those blocked lines could ever resolve, and M6's own effect-by-effect progress had reached the point where it
was the single most repeated line in every "not resolved" list. This is the M5 chunk that closes it.

`resolveTargets` (new `targeting.go`) is the CR 601.2c/603.3b moment itself, called from the one place this port has
that puts an ability on the stack at all today: `pushTriggeredAbilities` (trigger.go). `CastSpell` (castspell.go) does
not need to call it yet -- it only casts a permanent or an Aura, and neither carries `ValidTgts$` on its own top-level
record the way an Instant or Sorcery would (not built: this port has no cast path for either). It reports whether the
ability stays eligible to be pushed at all -- `bool`, not `(bool, error)` -- for a reason worth stating plainly: CR
603.3c's own real rule ("if the ability requires a target and there are no legal targets, it doesn't go on the stack")
and "a target shape this port cannot parse" are indistinguishable from the caller's own vantage point. Both mean the
ability does nothing. Giving the second case a loud error and the first a quiet `false` would draw a distinction nothing
outside `resolveTargets` itself could act on differently, and GO-7's own "fail one game, not the batch" reasoning has
nothing to grab onto for a card that was never going to resolve either way. A small blocklist (`targetUnresolvedParams`:
`Radiance$`, 4 real corpus lines -- "and each other permanent that shares a color with it," a second, derived candidate
set no single `ValidTgts$` evaluation produces on its own; `TargetsForEachPlayer$`/
`TargetsWithDefinedController$`/`TargetUnique$`, 0 real lines each) folds into that same `false` rather than being
checked by every future consumer separately.

`TargetMin$`/`TargetMax$` resolve through `resolveNamedAmount` exactly as every other numeric param already does,
defaulting to `1`/`1` when neither is named -- `TargetRestrictions.java`'s own `getOrDefault`. Candidate computation
(`targetCandidates`) has to decide first whether `ValidTgts$` names players or cards, and does it with one trial call
rather than a hand-rolled string check: `matchesPlayerSpec` (valid.go, already built for `Phase`'s/`DamageDone`'s/
`SpellCast`'s own qualified player specs) reports `ok=false` whenever its own base token is not one of `You`/
`Opponent`/`Player`, regardless of which candidate is asked, so a single call against any placeholder pair settles the
shape for the whole spec before ever walking a real candidate list. Player-shaped candidates are every player still in
the game -- a player who has lost is filtered out here the identical way `definedPlayers`'s own `if (!p.isInGame())`
reading already is, a real correctness gap this chunk caught while writing `targetCandidates` rather than one carried
over from anywhere else. Card-shaped candidates are every card on any player's battlefield, via the unchanged `Matches`
(valid.go) -- CR's own implicit "target creature" scope, and the only zone 0 real corpus `TgtZone$` lines across the
whole vocabulary ever ask this port to look anywhere else than.

`PlayerController` gained a twenty-fourth method, `ChooseTargets` (control.go), the identical "trust the controller's
answer" contract every other decision here already has -- the returned slice's own length and membership are not
re-checked against `TargetMin$`/`TargetMax$` or the candidate list. `ScriptedController` gained a `targets [][]EntityID`
queue and `QueueTargets` to fill it, following `QueueDiscardChoice`'s/`QueueScry`'s own established shape exactly.

Getting a controller into `resolveTargets` at all, called from `pushTriggeredAbilities`, meant `pushTriggeredAbilities`
itself needed one -- and it is called from sixteen places, all within trigger.go (`checkETBTriggers`,
`checkDiesTriggers`, `checkAttacksTriggers`, `checkSpellCastTriggers`, `checkBlocksTriggers`,
`checkAttackerBlockedTriggers`, `checkAttackerBlockedByCreatureTriggers`, `checkDamageDoneTriggersToCard`,
`checkDamageDoneTriggersToPlayer`, `checkDiscardedTriggers`, `checkTapsTriggers`, `checkTapsForManaTriggers`,
`checkPhaseTriggers`, `checkAttackersDeclaredTrigger`, `checkDrawnTriggers`, `checkLifeGainedTriggers`), each of which
needed the parameter added to its own signature and threaded to every one of ITS OWN external callers in turn. That
fan-out reached into `action.go` (`destroyLethalToughness`/`destroyDamagedCreatures`/`destroyZeroLoyalty`/
`destroyZeroDefense`/`cleanupDanglingAttachments`/`resolveWorldRule`, all called from `CheckStateBasedActions`),
`attack.go` (`DeclareCombatAttackers`'s own body), `block.go` (`DeclareCombatBlockers`'s own body), `combatdamage.go`
(`dealPermanentDamage`/`dealPlayerDamage`/`dealAttackTargetDamage`, called from
`dealAttackerDamage`/`DealCombatDamage`'s own chain and from `dealDamageEffect`), `manaability.go` (`TapLandForMana`),
`turn.go` (`drawStep`/`DrawCards`, called from `beginPhase` and from `drawEffect`), `land.go` (`PlayLand`), and
`castspell.go` (`permanentEffect`/`attachEffect`'s own `_ PlayerController` params, unused until now, finally read).
Every path bottomed out at a function some earlier chunk had already given a controller to -- `CheckStateBasedActions`,
`DeclareCombatAttackers`, `DeclareCombatBlockers`, `DealCombatDamage`, `beginPhase`, `CastSpell`, or `Effect.Resolve`'s
own parameter (the Discard chunk's own addition) -- so the cascade, while wide, never had to reach further than one or
two calls past a controller already in scope. `PlayLand` and `TapLandForMana` had no internal caller at all before this
(only tests and `internal/fixture`'s own `actions.go`, the `testdata/scenarios/` walker, which picked up the same two
calls), so gaining the parameter there was a clean addition rather than a threading exercise.

`definedPlayers`/`definedCards` (defined.go) both gained a `targets []EntityID` parameter and a
`"TargetedPlayer"`/`"Targeted"` (players) or `"Targeted"`/`"ThisTargetedCard"` (cards) case reading it --
`AbilityUtils.getDefinedPlayers`'s/`getDefinedCards`'s own literal cases for a `Defined$` value that explicitly names
what got targeted, the shape a sub-ability written with `Defined$ Targeted` reads back from its own parent's choice.
Every one of the eleven existing call sites across `dealdamageeffect.go`/`draweffect.go`/
`discardeffect.go`/`gainlifeeffect.go`/`loselifeeffect.go`/`pumpalleffect.go`/`pumpeffect.go`/`surveileffect.go`/
`scryeffect.go`/`putcountereffect.go` (twice, via its own `definedCounterTargets` wrapper) now passes `a.Targets`
through, whether or not that particular effect's own real corpus lines ever reach the new case yet.

`loseLifeEffect` is targeting's first real consumer, and it does not go through those new `defined.go` cases at all:
`LifeLoseEffect.java`'s own `getTargetPlayers(sa)` (`SpellAbilityEffect.java`'s own base helper) reads
`sa.getTargets().getTargetPlayers()` directly the moment the ability uses targeting at all, never falling through to
`AbilityUtils.getDefinedPlayers`'s own `Defined$` switch in that case -- and 0 real `LoseLife` lines combine
`ValidTgts$` with a `Defined$` of their own, confirming the corpus never relies on the fallback coexisting.
`loseLifeEffect.Resolve` mirrors that exactly: when `ValidTgts$` is present, read `a.Targets` straight into the player
list, bypassing `Defined$` resolution outright; only when it is absent does `Defined$` get read at all. 300 of the
corpus's 445 real `(AB|DB)$ LoseLife` lines resolve now (226 by `Defined$` alone, 74 more via
`ValidTgts$ Opponent`/`Player`) -- the other 2 real `ValidTgts$` lines name a qualified base
(`Player.wasDealtDamageThisTurnBySource`/`Player.LostLifeThisTurn`) `matchesPlayerProperty` does not recognize, so
`matchesPlayerSpec`'s own trial call reports `ok=false` for them, `targetCandidates` falls through to the card-shaped
branch, finds no card matching a `Player`-rooted spec, and the ability lands on CR 603.3c's own "no legal targets"
outcome -- the identical bucket a genuinely unresolvable shape already shares, not a wrong answer.

Building this surfaced two real regressions in already-shipped tests, both from the same root cause: five
"`RejectsValidTgts`" tests (`pumpalleffect_test.go`, `putcountereffect_test.go`, `discardeffect_test.go`,
`scryeffect_test.go`, `surveileffect_test.go`) had proved their own effect rejects `ValidTgts$` by casting a line naming
it and asserting `ResolveStack` returns an error -- but `resolveTargets` now resolves that same `ValidTgts$` line
successfully before the ability ever reaches `Resolve`, calling `ChooseTargets` on a `ScriptedController` none of the
five had queued an answer on, panicking on the exhausted-queue check every other decision here already has. The fix in
each case was not to change what the test proves (each effect's own `Resolve` still names `ValidTgts$` in its own
blocked-param list, so the assertion -- "this effect rejects it" -- is still true) but to queue a plausible target
answer first so `resolveTargets` itself succeeds and the ability actually reaches the effect's own check.
`draweffect_test.go`'s own `TestDrawEffectUnsupportedDefinedErrors` had a third, different regression: it used
`Defined$ Targeted` with no `ValidTgts$` at all to represent an unsupported `Defined$` shape, which now resolves (via
`definedPlayers`'s new case) to zero players -- a real change, not a bug, since a card writing `Defined$ Targeted` with
nothing to target is not a shape any real corpus line produces; the test was repointed at `Defined$ Remembered` instead,
still genuinely unsupported.

6 new tests (`loselifeeffect_test.go`) prove the mechanism through its first real consumer: `ValidTgts$ Opponent`
draining the chosen opponent and leaving the caster untouched, `ValidTgts$ Player` legally choosing the caster
themselves, `ValidTgts$` winning over a `Defined$` present on the identical line (0 real lines combine them, but the
dispatch itself should not silently prefer the wrong one if it ever happened), the unrecognized-property line resolving
to zero legal targets rather than a wrong one, `Radiance$` folding into that same "no legal targets" bucket, and
`TargetMax$ 2` reaching `ChooseTargets` for two opponents at once. `control_test.go`'s own
`TestScriptedControllerEachQueuePanicsWhenExhausted` table gained a `"targets"` row, and `mulligan_test.go`'s
`scriptedMulliganController` gained a panicking `ChooseTargets` stub. New `enginelint` group `targeting`
(`id`/`card`/`game`/`player`/`ability`/`control`/`valid`/`amount`/`zone`), `trigger` gaining it as a dependency;
`land`/`manaability` each gained `control` (and `land` gained nothing else new, `manaability` gained `trigger` too, both
newly needing to call into groups their own files had not referenced before this chunk).

## SubAbility chaining itself lands

`SubAbility$` sits in every M6 effect's own "not resolved" list built so far -- this port's own second-most-cited gap
after targeting (above). 16,022 real corpus lines name it, 12% of the whole corpus, across 9,446 distinct files.
`AbilityFactory.getAbility`/`getSubAbility` already resolve the whole reference chain at compile time
(`compile.Ability.Subs`, `internal/carddb/compile/compile.go`, ADR-0007) -- the compiled tree has always carried the
next link, nothing at the engine layer had ever walked it.

`AbilityUtils.resolveApiAbility` is the Java shape ported: check the ability's own `metConditions()`, resolve if it
holds, then call `resolveSubAbilities` regardless of whether it did. That "regardless" is the whole feature. Sphinx
Sovereign is the real card that makes it concrete: "At the beginning of your end step, you gain 3 life if Sphinx
Sovereign is untapped. Otherwise, each opponent loses 3 life" compiles to one `DB$ LoseLife` (`ConditionDefined$ Self`
`ConditionPresent$ Card.tapped`, untested here -- game-state.md's own "Not ported yet") with a
`SubAbility$ DB$ GainLife` carrying the identical `Condition$` pair negated (`ConditionCompare$ EQ0`). Whichever half's
own condition fails, the OTHER half still has to run -- an ability that only chained when its own parent's body executed
would silently drop exactly the branch Sphinx Sovereign needs half the time.

`resolveSubAbility` (new `subability.go`) is called from `Registry.Resolve` (`effect.go`) itself, right after its own
`e.Resolve(g, a, controller)` call succeeds -- the direct Go analog of `resolveApiAbility`'s own
`sa.resolve(); resolveSubAbilities(sa, game);` pairing, both statements inside the one function rather than split across
a caller and a callee. It looks for the one `Subs` entry (`compile.Ability.Subs`) whose own `Key` matches `"SubAbility"`
case-insensitively (`mergeParams`, compile.go, already collapses a repeated key to one value, so at most one exists),
builds a child `Ability` carrying the parent's own `Source`/`Controller`/`Target`/`Targets`/`Amounts` unchanged, and
calls `r.Resolve(g, &child, controller)` -- recursing through the SAME method rather than dispatching to
`r[api].Resolve` directly, so a chain more than one hop deep just keeps going without this function needing a loop of
its own (`compile.Ability.Subs` already holds the whole tree). 10,466 real references are exactly one hop, 3,856 exactly
two hops past that, 1,172 three hops past that, and it keeps going all the way to 13 hops deep once.

Only the literal `SubAbility$` key auto-chains this way. `compile.go`'s own `subAbilityKeys` map has a much longer list
-- `PreventionSubAbility$`, and every "additional ability" key `AbilityFactory.java` attaches
(`WinSubAbility$`/`ChooseSubAbility$`/`ResultSubAbilities$`/`Choices$`, ...) -- but those are all fetched and resolved
explicitly by their own effect's own Go code once that effect exists (`FlipCoinEffect.java`, `ChoosePlayerEffect.java`,
`RollDiceEffect.java`, ...), not through this port's generic post-resolve chain the way Java's own `sa.getSubAbility()`
is. None of those effects are built yet, so `"SubAbility"` is the only key this chunk has a caller for.

Propagating the parent's own `Targets`/`Target` unchanged onto the child means a sub-ability naming `Defined$ Targeted`
(`definedPlayers`/`definedCards`'s own case, "Targeting itself lands," above) reads the SAME chosen target the parent's
own `ValidTgts$` resolved -- `scavenging_ooze.txt`'s/`hellhole_rats.txt`'s/dozens more real corpus lines' own shape. A
sub-ability naming its OWN `ValidTgts$` (891 of the 16,022 real referenced lines, 5.6%) is a different, unbuilt story:
`resolveTargets` runs exactly once, on the ability actually pushed onto the stack, before any of this -- there is no
second targeting pass for a node two levels down the tree. Such a sub-ability simply inherits whatever `Targets` the
parent had (often nothing) and an effect gating on `ValidTgts$` presence finds no candidates to act on -- the identical
"an unsupported shape observably folds into no legal targets" choice `resolveTargets`'s own doc comment already
committed to for the top-level case, not a new wrong-guess category this chunk introduces.

Chaining into an API this port has not registered an `Effect` for yet still fails with `ErrUnimplemented` naming it --
`Registry.Resolve`'s own existing contract for a top-level ability, inherited for free the moment the recursion runs
back through that same method rather than a separate code path. `riverwise_augur.txt`'s own real
`DB$ Draw | Defined$ You | NumCards$ 3 | SubAbility$ DBChangeZone` proves it end to end: the three cards are already in
hand (`drawEffect`'s own body ran and returned `nil` before the chain was ever attempted) by the time
`resolveSubAbility` reaches `DB$ ChangeZone` (203 script-driven APIs away from built, and the single most-referenced
`SubAbility$` target in the whole corpus at 1,505 real lines) and the whole `ResolveStack` call fails naming it. That
partial visible state is deliberate, not a rollback bug: CR's own sequential resolution means the parts of a multi-part
ability that already happened stay happened even when a later part cannot -- "draw two cards, then [something this port
cannot do]" really did draw two cards in a real game too.

A `SubAbility$` SVar body whose own leading value `ApiType.java` has no constant for -- an `APIByName` miss -- is not
reachable against the real corpus today: `ApiType.java`'s own generated vocabulary (`ability.go`) and the
apiscan/vocabscan gates (M3) already require every real API string to resolve. `resolveSubAbility` still checks for it
and returns an error naming the unrecognized value, the identical PORT-8 "a card cannot be trusted not to be the first"
reasoning `triggerEffectAPI`'s own identical defensive check (trigger.go) already used for a trigger's `Execute$` --
`compile.Compile` itself never validates an API name against any vocabulary at all (that check happens only at resolve
time), so nothing upstream of this would have caught it either.

Getting a real second half of a chain to actually run meant unblocking `SubAbility$` in at least one effect capable of
gating on `Condition$` at all -- every effect wired to `subAbilityConditionMet` still named `"SubAbility"` in its own
unresolved-param list (`dealdamageeffect.go`, `pumpeffect.go`, `pumpalleffect.go`, `loselifeeffect.go`,
`putcountereffect.go`), meaning the mechanism above would never actually have fired for any of them without a second
change. `gainLifeEffect`/`loseLifeEffect` are the pair unblocked this chunk -- the same pair "Targeting itself lands"
(above) already extended once, kept together since they are each other's mirror image and Sphinx Sovereign's own real
shape needs exactly this pair. `SubAbility` is simply removed from both `gainLifeUnresolvedParams` and
`loseLifeUnresolvedParams`; nothing else in either file changes, since the chain itself runs one level up, inside
`Registry.Resolve`. 18 of the corpus's own 253 real SVar-defined `GainLife` lines and 144 of 382 real SVar-defined
`LoseLife` lines naming `SubAbility$` now chain to an already-built leaf ability (no further `SubAbility$` of its own)
and resolve end to end -- a chain more than one hop deep, or one whose target is not built yet, is not counted by either
figure, since each of those targets' own effect already tracks that half of the question on its own terms. `drawEffect`
never named `SubAbility$` among its OWN unresolved params in the first place (there was simply nowhere for the reference
to go before now), so it started chaining for free the moment `resolveSubAbility` existed: 170 of 747 real SVar-defined
`Draw` lines. `DealDamage`/`Pump`/`PumpAll`/`PutCounter`/`Discard`/`Scry`/`Surveil` still block `SubAbility$` outright
in their own `Resolve` this chunk -- the mechanism exists for any of them, unblocking each one is a later chunk's own
job ("SubAbility chaining reaches every effect," further below, is that job).

Two existing regression tests broke for the identical reason as targeting's own five:
`TestGainLifeEffectRejectsSubAbilityChain` and `TestLoseLifeEffectRejectsSubAbilityChain` had proved their own effect
errors on a `SubAbility$` line naming `DB$ Cleanup` (an unbuilt API) -- true before this chunk because `"SubAbility"`
itself was rejected first, no longer true now that the param is unblocked and the chain actually reaches `Cleanup`'s own
real `ErrUnimplemented`. Both were rewritten as
`TestGainLifeEffectChainsIntoSubAbility`/`TestLoseLifeEffectChainsIntoSubAbility`, repointing the chained line at an
already-built leaf (`GainLife`'s own chains into `LoseLife`, `LoseLife`'s own chains into `GainLife`,
`radiant_epicure.txt`'s real shape with a plain integer standing in for its own unresolved Converge-driven `X`) and
asserting BOTH halves' own state change now happens, rather than asserting a rejection that no longer occurs.

6 new tests (`subability_test.go`) prove the mechanism itself rather than any one effect's own dispatch: Rousing Read's
real "draw two cards, then discard a card" chain resolving both halves; a synthetic `GainLife`-into-`LoseLife` chain
(the zone-scan `ConditionPresent$`/`ConditionCompare$` family standing in for Sphinx Sovereign's own unresolved
`ConditionDefined$`) proving the chained half runs even though the parent's own condition failed; a synthetic
`LoseLife`-into-`GainLife` chain proving `Targets` propagates unchanged onto the child; a synthetic three-level
`LoseLife`-into-`GainLife`-into-`Draw` chain proving the recursion itself keeps going past one hop; riverwise_augur's
real chain into unbuilt `ChangeZone` proving `ErrUnimplemented` propagates naming it, with the already-resolved half
staying resolved; and a synthetic unrecognized-API SVar body proving the defensive `APIByName` check errors rather than
silently dropping the chain. New `enginelint` group `subability` (`id`/`game`/`ability`/`control`/`effect`), `effect`
gaining it as a dependency to call into.

## SubAbility chaining reaches every effect

The prior chunk's own mechanism (`resolveSubAbility`, subability.go, above) had exactly two real consumers --
`gainLifeEffect`/`loseLifeEffect` -- because unblocking `SubAbility$` needed touching each effect's own unresolved-param
list individually, and the other seven `Condition$`-and-non-`Condition$`-capable effects (`dealDamageEffect`,
`pumpEffect`, `pumpAllEffect`, `putCounterEffect`, `discardEffect`, `scryEffect`, `surveilEffect`) still named
`"SubAbility"` there, rejecting the param outright before `Registry.Resolve`'s own post-resolve chain call ever got a
chance to run. `SubAbility` is removed from all seven files' own unresolved-param arrays (`dealdamageeffect.go`,
`pumpeffect.go`, `pumpalleffect.go`, `putcountereffect.go`, `discardeffect.go`, `scryeffect.go`, `surveileffect.go`) --
the identical one-line change each of the first two got, nothing else in any of the seven files changes, since the chain
itself still runs one level up inside `Registry.Resolve`.

Real corpus grounding for three of the seven: `sword_of_fire_and_ice_and_war_and_peace.txt`'s own `DB$ DealDamage`
chaining into `DB$ GainLife`; `rabaroo_troop.txt`'s own `DB$ Pump | Defined$ Self | KW$ Flying` chaining into
`DB$ GainLife | Defined$ You | LifeAmount$ 1`; `well_rested.txt`'s own
`DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 2` chaining into `DB$ GainLife` (that real card's own
`GainLife` half omits `Defined$` entirely, which would default to `You` in Java but errors in this port today --
`definedPlayers` has no such default, a separate, unrelated gap that stays open; the port's own test and doc-comment
examples name `Defined$ You` explicitly to sidestep it rather than exercise it). The other four (`PumpAll`, `Discard`,
`Scry`, `Surveil` as parents) use a synthetic chain into `GainLife` instead, since no clean real-corpus example
combining a resolvable parent shape with an already-built leaf turned up for those four specifically.

Counting "how many real lines now resolve end to end" needed the identical script shape the first two effects' own
counts used, extended to cover all seven: for each real `DB$ <Effect>` line naming `SubAbility$`, check the effect's OWN
remaining unresolved-param list (now without `"SubAbility"`) plus whatever else its own dispatch requires (`PumpAll`
needs `ValidCards$` present and, if named, a resolvable `Defined$`; `Discard` needs `Mode$ TgtChoose` and a resolvable
`Defined$`; the rest just need a resolvable `Defined$` where their own dispatch reads one), then check the referenced
sub-ability's own leading API name against the ten built effects and that its own params pass the identical gate one
level down (a leaf only -- a further `SubAbility$` two hops deep is not counted, the identical conservative choice the
first two effects' own counts already made). 9 of 316 real SVar-defined `DealDamage` lines, 17 of 571 `Pump`, 6 of 75
`PumpAll`, 56 of 623 `PutCounter`, 11 of 254 `Discard`, 31 of 57 `Scry`, and 2 of 15 `Surveil` naming `SubAbility$` now
chain to an already-built leaf ability and resolve end to end.

9 new tests, one per effect (`dealdamageeffect_test.go`, `pumpeffect_test.go`, `pumpalleffect_test.go`,
`putcountereffect_test.go`, `discardeffect_test.go`, `scryeffect_test.go`, `surveileffect_test.go`), each rewritten from
its own `Test<Effect>EffectRejectsSubAbilityChain` (which had proved rejection via a `DB$ Cleanup` target, the identical
now-false assumption `TestGainLifeEffectRejectsSubAbilityChain`/`TestLoseLifeEffectRejectsSubAbilityChain` already had)
into `Test<Effect>EffectChainsIntoSubAbility`, asserting both the parent's own body and the chained `GainLife`'s own
life change actually happen. `PutCounter`'s own rewritten test surfaced the `Defined$`-default gap directly: its old
test's chained target named no `Defined$` at all and, once `SubAbility$` stopped blocking it, `definedCounterTargets`
reached that absent value and errored -- naming `Defined$ Self` explicitly on the new test's own `PutCounter` line
(which the old, always-rejected test never needed to get right) fixed it without touching the separate
default-resolution gap itself.

Every one of the ten script-driven effects built so far now chains a `SubAbility$` it names, closing this port's own
second-most-cited gap (after targeting) completely at the mechanism level -- what remains is 193 more script-driven
effects each becoming a leaf (or a parent) other chains can reach, M6's own ordinary remaining scope, not a further
SubAbility-specific gap.

## Last-known-information lands

CR 603.6d's "look back in time": an object that leaves a zone is checked, for the purposes of anything watching it
leave, using its characteristics as they were immediately before it left, not as a new, reset object in its destination
zone. Java gets this from `CardCopyService.getLKICopy()` (`Card.java`'s own 8,105 LOC neighbor), called from
`GameAction.changeZone` before the card's own fields are cleared for its new zone -- a several-dozen-field copy covering
everything from P/T to exile history to cast-from information. This port needed exactly one slice of it: 116 of the
corpus's own 7,574 real `Mode$ ChangesZone` lines whose `Destination$` permits Graveyard name a `ValidCard$` testing the
dying card's own power, toughness, type, color, a keyword or a counter (Retched Wretch's own real "when CARDNAME dies,
if it had a -1/-1 counter on it, you gain 2 life" -- `Card.Self+counters_GE1_M1M1`; Reyhan, Last of the Abzan's own
"whenever a creature you control with a +1/+1 counter on it dies" -- `Creature.YouCtrl+counters_GE1_P1P1`).

`checkDiesTriggers`/`otherDiesTriggerMatches` (trigger.go, `## Trigger firing`, above) already carried a doc comment
claiming no lookback was needed at all: `Card.Def` is fixed at compile time regardless of zone, and `Card.Controller()`
is not one of the fields `Move`'s own battlefield-leaving branch clears, so both keep reading correctly after the card
has already moved. That reasoning held for those two fields and stopped there -- it did not extend to
`Counters`/`PT`/`TypeMod`/`ColorMod`/`KeywordMod`, every one of which `Move` clears immediately, before either
trigger-check function ever runs (`destroyDamagedCreatures`/`destroyLethalToughness`/... in action.go call `Move` then
`checkDiesTriggers` back to back). A `ValidCard$` reading any of those five would silently under-fire: a creature that
died carrying a `+1/+1` counter would test as counterless by the time its own or a watcher's dies trigger ran, the same
"a card cannot be trusted not to be the first" corpus-frequency finding (116 real lines, not zero) that turned this from
a hypothetical into a real, if narrow, bug.

`Game.lki` (`game.go`) is the fix: a new `map[CardID]*Card`, written by exactly one call site -- the first line of
`Move`'s own battlefield-leaving branch, a plain `snap := *c; g.lki[id] = &snap` taken before any of the five fields
above are cleared. `Game.LKI(id)` reads it back, returning `nil` for a card that has never left the battlefield.
`checkDiesTriggers`/`otherDiesTriggerMatches` both now prefer `g.LKI(left)` over `g.Card(left)` for every read the
matching pass makes against the dying card -- `Def`/`Controller()` included, since using the frozen copy for those too
is free (both are identical either way) and simpler than special-casing which fields need the swap. `Game.Clone` (M7's
own AI lookahead) gained the identical per-field independent-copy treatment `PT`/`TypeMod`/`ColorMod`/
`KeywordMod`/`ControlMod`/`Counters`/`Memory`/`attachments` already get for a live arena card, applied to each stored
snapshot instead -- a clone's own LKI copy has to be as independent of the original's as everything else Clone already
guarantees (`## Cloning`, above).

The snapshot is a plain struct copy, not Java's own field-by-field reconstruction: nothing else this port's own
`Matches`/`compareFieldValue` (valid.go) reads is affected by leaving the battlefield the way those five ledgers are, so
there was nothing else worth capturing. It is overwritten whole on every subsequent trip off the battlefield, never
merged with an earlier one -- `getLKICopy()`'s own contract, and the one a card leaving, returning, and leaving again
with different counters needs (`TestLKIOverwrittenOnEachSubsequentLeave`, lki_test.go).

Four new tests (`lki_test.go`): `TestLKIAbsentBeforeLeavingBattlefield`, `TestLKIFreezesStateAtTheMomentOfLeaving`,
`TestLKIOverwrittenOnEachSubsequentLeave` and `TestCloneCopiesLKI`. Two more (`trigger_test.go`) prove the actual bug
this closes, each checked against the OLD behavior directly (reverting the `g.LKI` lookup made both fail exactly as
expected before being restored): `TestDestroyDamagedCreaturesDiesTriggerSeesCounterAtTimeOfDeath` (a creature's own
`Card.Self+counters_GE1_P1P1` dies trigger) and `TestDestroyDamagedCreaturesOtherDiesTriggerSeesCounterAtTimeOfDeath` (a
separate watcher's own `Creature.YouCtrl+counters_GE1_P1P1` dies trigger, `otherDiesTriggerMatches`'s own half).

Not resolved: everything else `getLKICopy()` copies (exiled-with, cast-from, damage history, remembered/imprinted cards,
...) -- each real only once some other still-unbuilt mechanism would ever read a graveyard/exile card's own past-tense
state through it, PORT-8's "build the consumer's own real need, not the whole Java method" the identical discipline
`resolveAmount`/`subAbilityConditionMet` already followed for their own Java counterparts. The legend rule's own Corner
Case 1 is item 25's other named gap and is unrelated to LKI at all -- it needs a card-name lookup across every creature
card this game has ever printed, and this port's `*Game` holds no `*carddb.DB` reference to ask
(`## The legend rule needed CheckStateBasedActions to take a controller`, above).

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

| Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | Lands |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `CardState` — face/characteristics data for transform, flip and meld                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | M5    |
| 90 of `PlayerController`'s 110 methods — everything needing `SpellAbility`, non-Aura targeting or the rest of Combat past dealing damage                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | M5-M6 |
| `AIController`, the real (non-scripted) implementation                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | M7    |
| Non-combat damage to a planeswalker or a Battle (a burn spell, an activated ability) — combat damage already removes loyalty/defense counters (CR 120.3c, 121.5); nothing outside combat deals damage at all yet                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | M5-M6 |
| The rest of CR 704.5f/704.5g's toughness — `*`, `1+*`, a `Count$` reference, or toughness a continuous effect or a counter has changed — needs `internal/expr` and the layer system, not just `strconv.Atoi`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | M5-M6 |
| CR 613.6-613.8's dependency reordering within a layer — `foldPT` only sorts by timestamp, correct until two effects on one card can actually disagree about order                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | M5-M6 |
| Layers 1 and 3 in full (copy -- not even part of `StaticAbilityContinuous.java`'s own switch in Forge itself, a separate resolution-time mechanism; text, `GainTextOf$`, 1 real line, needs its own single-card text-copying mechanism), plus Layer 8's own remainder (`MayLookAt$`/`MayPlay$`, `AddHiddenKeyword$`, vote/villainous-choice params -- the Layer 7/4/5/6/8/2 section's own `RulesMod` paragraph, above, has the exact reasons and counts) and the rest of Layers 4/5/6 past a literal token list and Layer 7a's own SVar shapes outside the Valid family (`xPaid`, `CardCounters`, `Devotion`, ... -- the same section's own `resolveAmount` paragraph) -- `applyContinuousControl`/`applyContinuousPT`/`applyContinuousType`/`applyContinuousColor`/`applyContinuousKeyword`/`applyContinuousRules` (continuous.go) resolve Layer 2's own `GainControl$ You` (43 of 44 real lines), Layer 7b/7c's own plain-integer AND now Count$Valid-SVar-driven lines, Layer 7a's own Count$Valid-SVar-driven `CharacteristicDefining$` lines (`applyOneCharacteristicDefiningPT`), 201 of 284 real `AddType$`/`RemoveType$` lines, 54 of 61 real `AddColor$`/`SetColor$` lines, 1,556 of 1,857 real `AddKeyword$` lines and 75 of 78 real `SetMaxHandSize$`/`RaiseMaxHandSize$`/`AdjustLandPlays$` lines, all `Affected$`-matched; the rest all need the rest of the same general engine, mostly for keys a full `AbilityUtils.calculateAmount` port, a `*cardtype.Registry` this port does not inject into the engine, or a cast-time zone permission this port has never needed before would resolve, not a new layer number to add                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | M5-M6 |
| `changeZone`'s replacement effects, triggers and token/copy-vanishing rules; last-known-information is real now only at the one subset the real corpus's own dies triggers read (`Game.LKI`, `## Last-known-information lands`, below) -- `getLKICopy()`'s several dozen other copied fields (exiled-with, cast-from, damage history, ...) stay unported                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | M5-M6 |
| `PhaseHandler`'s Upkeep, Main and End of Turn step bodies — need triggers, `SpellAbility` or the rest of Combat                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | M5-M6 |
| `Match` — a series spanning more than one game, "the loser of the last game goes first" (`DealOpeningHands` always takes CR 103.2's coin flip), Puzzle/Archenemy/Power Play's own starting-player rules                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | M5-M6 |
| A modified or unlimited maximum hand size (CR 514.1's `isUnlimitedHandSize`/a continuous effect changing it) — `MaxHandSize` is used unconditionally since layers 1-6/8 aren't built                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | M5-M6 |
| Interactive priority (`mainLoopStep`'s real APNAP pass), extra turns/phases, topsy-turvy phase order — `ResolveStack` plays out only the degenerate case, nobody able to respond ("doesn't untap" effects are no longer a gap here: `untapBlocked`, `## Replacement effects`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | M5-M6 |
| Original, Paris, Vancouver and Houston mulligan rules — out of scope, not deferred (PORT-6)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | never |
| `CounterChanged` for a script-written (non-named-constant) `CounterType` — `counterDetail` (`event.go`) is closed over the eight named constants; a `SpellAbility` creating an arbitrary keyword counter needs the encoding extended or replaced first (`## Events, wired`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | M5-M6 |
| `MagicStack`'s `undoStack` (needs an interactive priority pass to undo mid-pass); `freezeStack`/`unfreezeStack` is no longer a gap: `checkETBTriggers`/`checkDiesTriggers` pushing during another ability's own resolution needs no freezing since nothing can respond in between either way. `addSimultaneousStackEntry` itself is resolved (`pushTriggeredAbilities`, `## Stack`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | M5-M6 |
| Trigger firing (CR 603) beyond "enters"/"dies"/"attacks"/"blocks"/"becomes blocked"/"becomes blocked by a creature"/"deals damage"/"is discarded"/"becomes tapped"/"becomes untapped"/"taps for mana"/"casts a spell"/"beginning of a step or phase"/"a player attacks"/"a player draws a card"/"gains life"/"becomes the target of a spell or ability"/"plays a land" and watching another permanent do one of those — `checkETBTriggers`/`otherETBTriggerMatches`/`checkDiesTriggers`/`otherDiesTriggerMatches`/`checkAttacksTriggers`/`checkBlocksTriggers`/`checkAttackerBlockedTriggers`/`checkAttackerBlockedByCreatureTriggers`/`checkDamageDoneTriggersToCard`/`checkDamageDoneTriggersToPlayer`/`checkDiscardedTriggers`/`otherDiscardedTriggerMatches`/`checkTapsTriggers`/`checkUntapsTriggers`/`checkTapsForManaTriggers`/`checkSpellCastTriggers`/`checkPhaseTriggers`/`checkAttackersDeclaredTrigger`/`checkAttackersDeclaredOneTargetTrigger`/`checkDrawnTriggers`/`checkLifeGainedTriggers`/`checkBecomesTargetTriggers`/`checkLandPlayedTriggers`/`checkSacrificedTriggers`/`checkChangesZoneAllTriggers`/`checkDamageDoneOnceTriggers`/`checkDamageDealtOnceTriggers`/`checkDamageAllTriggers` (trigger.go) cover those twenty-eight, each now pushed through `pushTriggeredAbilities`' own CR 603.3b APNAP ordering rather than a fixed order (`## Stack`, above), and each now also passing through the general `Trigger.phasesCheck` gate (`triggerPhasesCheck`, `## Trigger.phasesCheck lands`, above — `Phase$`/`PlayerTurn$`/`NotPlayerTurn$`/`OpponentTurn$`/`FirstCombat$` resolved, `FirstUpkeep$`/`FirstUpkeepThisGame$`/`TurnCount$` still skip); every other mode (`Countered`, `Exiled`, ...), `LandPlayed`'s own `Static$`/`ValidSA$`, `Phase`'s own `Condition$` and its own remaining qualified `ValidPlayer$` forms (`Player.EnchantedBy`/`Player.Chosen`/`Opponent.EnchantedBy`/`Player.isMonarch` -- `Player.EnchantedController`/`You.descended` are resolved now, "`Phase`'s own qualified `ValidPlayer$`: `EnchantedController` and `descended`," above), `DamageDone`'s own `ValidCause$`/`TargetRelativeToCause$`/`TargetRelativeToSource$`, `Discarded`'s own `ValidCause$`, `Taps`'s own `FirstTime$`/`Teamwork$`, `TapsForMana`'s own `Produced$`, `SpellCast`'s own `Player.EnchantedBy`/`Player.Chosen` qualified `ValidActivatingPlayer$` forms, `AttackersDeclared`'s own `Condition$` and its own qualified `AttackedTarget$` forms (`Player.EnchantedBy`, ...), `Drawn`'s own `FirstCardInDrawStep$`/`ForReveal$`, `LifeGained`'s own `ValidSource$`+`Spell$`/`ResolvedLimit$`, `BecomesTarget`'s own `Valiant$`/`ActivationLimit$`/`Static$` and 6 of its own 77 real `ValidSource$` lines remain gaps -- a trigger's own `OptionalDecider$` no longer among them, `Ability.Optional`/`Registry.Resolve`/`PlayerController.ConfirmOptionalTrigger` (`### CR 603.3d's own "may" triggered ability`, above) resolving all but the 83 real lines naming it on a chained `SubAbility$`'s own SVar body instead of a `T:` line (`Attacks`'s own `Alone$`/`DefendingPlayerPoisoned$`/`AttackDifferentPlayers$`/`Attacked$`/`FirstAttack$`, `Blocks`'s own `ValidBlocked$` and `DamageDone`'s own `DamageAmount$` are all resolved too, `attacksOtherCount`/`attacksMultiplePlayers`/`Card.AttacksThisTurn`/`checkBlocksTriggers`/`damageAmountMatches` each covering its own, and `matchesPlayerSpec`'s own Active/NonActive/Other property closed most of `SpellCast`'s/`DamageDone`'s/`TapsForMana`'s/`Phase`'s own qualified player specs); resolving what fires beyond `Draw`/`DealDamage`/`GainLife`/`Pump`/`PumpAll`/`LoseLife`/`PutCounter`/`Discard`/`Scry`/`Surveil`/`Sacrifice`/`SacrificeAll` is M6's 191 remaining corpus-frequency effects, not this | M5-M6 |
| CR 616's own general "more than one replacement effect could apply, the affected player chooses" ordering procedure — needs a `PlayerController` hook this port does not have; moot for four resolved shapes' own idempotent outcome (`Tapped = true`, blocked, prevented) but real for `drawReplaced`'s/`gainLifeReplaced`'s/`damageReplaced`'s/`damageReplacedPlayer`'s own non-idempotent ones (no real corpus deck combines two Draw-, GainLife- or damage-reducing permanents of the same shape today, so not yet observable). Most `Event$` values past `Moved`/`Untap`/`DamageDone`/`Draw`/`GainLife` (`Counter`, `AddCounter`, `CreateToken`, `BeginPhase`, `GameLoss`, `ProduceMana`, ...), `Draw`'s own remaining 29 real `ReplaceWith$` lines, `GainLife`'s own remaining 1 (rain_of_gore.txt's own `ValidSource$ SpellAbility`/`SourceController$ True` restriction on what caused the event, not who it affects) and `DamageDone`'s own remaining 47 (9 of the 27 `DB$ ReplaceDamage`lines naming a named-SVar`Amount$` this dispatch cannot resolve, 3 of the 59 `DB$ ReplaceEffect`'s own `VarName$ DamageAmount`lines unresolvable for an amount head this port has no evaluator for, 12 more naming`DB$ ReplaceEffect`with a different`VarName$` entirely, 6 of the 31 `DB$ RemoveCounter`/`DB$ PutCounter`lines unresolvable (5 chaining`SubAbility$`, 1 naming `CheckDefinedPlayer$`), and 17 naming a different `DB$` API altogether -- `matchesPlayerSpec`'s own comma-OR split (`## matchesPlayerSpec's own comma-OR split`, above) closed the other 2, a mixed player/card `ValidTarget$` `damageReplacedPlayer` could not resolve its own player-target half of before), stay unresolved too (`## Replacement effects`, `## ReplacementEffect.requirementsCheck lands`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | M5-M6 |
| The legend rule's own Corner Case 1 — a Corner-Case-2 permanent's own borrowed names colliding with some OTHER legendary's own literal printed name (`StaticData.instance().getCommonCards().isNonLegendaryCreatureName`, `GameAction.java`) needs a lookup across every creature card this game has ever printed, and this port's `*Game` holds no `*carddb.DB` reference to ask; Corner Case 2 itself is resolved now (`## The legend rule needed CheckStateBasedActions to take a controller`, above). Block legality's own `CantBlockBy` has no remaining gap: flying/reach, Fear, Horsemanship, Intimidate, Landwalk, Protection, Skulk, Menace and every literal `S:Mode$ CantBlockBy` line are all ported (`## Block legality: CantBlockBy`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | M5-M6 |
| Any mana ability besides a basic land's own intrinsic one (`TapLandForMana`, CR 305.6, `## Mana pool and payment`, above) — a nonbasic land, a creature, an artifact all need the M6 effect-dispatch machinery that one deliberately bypasses, since CR 305.6's ability is a fixed rule keyed off the type line, not script text                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | M6    |
