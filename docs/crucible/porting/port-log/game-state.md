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
params, not this text field yet), and the legend rule's own remaining corner case
(Partner-with-non-legendary-creature-names) — reads a characteristic the rest of the continuous-effect layer system
computes, or needs a static-ability engine this port does not have, and none of that is M5 work this has fully reached
yet. Damage dealt to a planeswalker or a Battle, which CR 120.3c/121.5 removes as loyalty/defense counters rather than
marking `Damage`, is wired too (`dealPermanentDamage`, `## Combat`, below) — combat can attack one directly, so
`destroyZeroLoyalty`/`destroyZeroDefense` are exercised by real play as well as by tests that remove counters directly.
Only _non-combat_ damage to a planeswalker or Battle is still a gap: nothing that deals damage outside combat exists yet
(no `SpellAbility`, no activated ability), so a burn spell or an ability aimed at a planeswalker's loyalty has nowhere
to come from regardless of whether the target-side plumbing is ready. A rule this port has not implemented simply never
fires, the same as a real game with no permanent that rule ever applies to — it is a coverage gap (ADR-0011), not a
wrong answer.

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
needed a second one. Not resolved: `Attacked$` (47 real lines) — `performTest` matches it against a `GameEntity` (a
player, planeswalker or Battle), and `Matches` (valid.go) only evaluates a `*Card`; `FirstAttack$` (4) — a creature's
own per-turn attack-count history (`CardDamageHistory.getCreatureAttacksThisTurn`), which this port tracks nothing for.
A trigger carrying either is skipped entirely, not fired unconditionally (GO-7) — 1,555 of 1,606 real lines carry
neither. Resolved: `Alone$` (60) — `attacksOtherCount` counts `Combat.Attackers` other than the declared attacker
itself, `CombatUtil.checkDeclaredAttacker`'s own `AbilityKey.OtherAttackers` (every other attacker declared this combat,
not just ones sharing this one's own defender — every real corpus line reads `Alone$ True`, never `False`);
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

Not resolved, skipped via `hasAnyParam`: `IsPresent$`/`PresentCompare$` (a general "is some other object true right now"
condition no trigger mode this port checks has an evaluator for) and `CheckSVar$`/`Condition$` (an arbitrary SVar-shaped
boolean condition) together account for most of the corpus's own unresolved Phase lines;
`FirstUpkeep$`/`FirstUpkeepThisGame$`/`FirstCombat$`/`TurnCount$`/`APlayerHasMoreLifeThanEachOther$`/
`APlayerHasMostCardsInHand$` are all 3 real lines or fewer each. `ValidPlayer$`'s own qualified forms
`matchesPlayerSpec` cannot resolve (`Player.EnchantedController`, 34; `Player.EnchantedBy`, 14; `You.descended`, 10;
`Player.Chosen`, 3; `Opponent.EnchantedBy`, 2; `Player.isMonarch`, 1 — 64 real lines) stay unresolved for the identical
reason `SpellCast`'s own `Player.EnchantedBy`/`Player.Chosen` do (`matchesPlayerSpec`'s own doc comment). 2,001 of 2,065
real `ValidPlayer$` lines resolve regardless (`You`, 1,832; `Player`, 113; `Opponent`, 47; `Player.Opponent`, 7;
`Player.Other`, 2).

`TestAdvancePhaseFiresPhaseTriggerAtCorrectStep`, `TestAdvancePhaseSkipsPhaseTriggerAtWrongStep`,
`TestAdvancePhaseSkipsPhaseTriggerForNonActivePlayer`, `TestAdvancePhaseFiresPhaseTriggerForOpponentValidPlayer`,
`TestAdvancePhaseFiresMainSecondTriggerOnMain2`, `TestAdvancePhaseSkipsMainSecondTriggerOnMain1`,
`TestAdvancePhaseFiresPhaseTriggerFromGraveyard` and `TestAdvancePhaseSkipsPhaseTriggerWithUnresolvedParam`
(trigger_test.go) prove all of the above against synthetic Upkeep-watcher- and Survival-second-main-phase-shaped cards,
`AdvancePhase` (turn.go) itself the real (non-test) caller through `beginPhase`'s own new
`g.checkPhaseTriggers(controller)` call, right after a step's own mechanical body (if any) and right before the
state-based-action check that already follows every phase entry.

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

`ValidAttackers$`/`ValidAttackersAmount$` (123 of 286) is `validAttackersCountMatches` (trigger.go): how many of
`Combat.Attackers` the `ValidAttackers$` spec matches (the existing `Matches`, valid.go), compared against
`ValidAttackersAmount$` -- default `"GE1"`, Java's own `getParamOrDefault("ValidAttackersAmount", "GE1")`, "one or
more," matching the corpus's own dominant `TriggerDescription$` phrasing, "whenever one or more Knights you control
attack." Every real `ValidAttackersAmount$` value is a plain two-letter-operator-plus-digit shape (`GE2`, `GE3`, `EQ1`,
...), never an SVar or `"X"` the way Java's own code path technically allows for, so this reads the digits directly
through the existing `compareOp` (valid.go) rather than resolving an amount through `resolveAmount` (amount.go) -- the
identical simplification `damageAmountMatches` already made for `DamageDone`'s own `DamageAmount$`.

Not resolved, skipped via `hasAnyParam`, the same "whole line, not a guess" contract every other mode's own skip-list
already has: `IsPresent$`/`PresentCompare$` (14, 4) -- the identical general "is some other object true right now" gate
`Phase`'s own `Condition$` has no evaluator for either; `CheckSVar$` (13) -- an SVar comparison `resolveAmount` does not
cover for every real shape.

`TestDeclareCombatAttackersFiresAttackersDeclaredTrigger`,
`TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithNoAttackers`,
`TestDeclareCombatAttackersFiresAttackingPlayerYouTrigger`,
`TestDeclareCombatAttackersSkipsAttackingPlayerYouTriggerForDefender`,
`TestDeclareCombatAttackersFiresAttackedTargetYouTrigger`,
`TestDeclareCombatAttackersSkipsAttackedTargetYouTriggerForAttacker`,
`TestDeclareCombatAttackersFiresValidAttackersAmountTrigger`,
`TestDeclareCombatAttackersSkipsValidAttackersAmountTriggerBelowThreshold` and
`TestDeclareCombatAttackersSkipsAttackersDeclaredTriggerWithUnresolvedParam` (trigger_test.go) prove all of the above
against a synthetic watcher-permanent def, `DeclareCombatAttackers` (attack.go) itself the real (non-test) caller
through its own new `g.checkAttackersDeclaredTrigger()` call, right after the per-attacker `checkAttacksTriggers` loop.

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
`SubAbility$` (a counter grant) and stay unresolved; `LandTapped`'s own 135 real lines almost all carry
`ConditionPresent$`/`ConditionCompare$` (a checkland, "unless you control a Mountain or a Forest") or
`ConditionCheckSVar$`/`ConditionSVarCompare$` (a numeric SVar gate) alongside the identical `DB$ Tap`, and stay
unresolved too -- PORT-8/GO-7: a conditional tap is not a shape this slice tries to guess at by tapping unconditionally.

`checkMovedReplacement` (`replacement.go`, new) resolves the 618 unconditional lines. It reads `Face.Replacements`
(`compile.go`) -- M3's own compiled field, typed identically to `Face.Triggers`/`Face.Statics` since `R:` lines share
the exact `Key$ Value` grammar, compiled the same way, and never read by the engine before now. `ReplaceWith` is already
one of `subAbilityKeys` (compile.go), so `ReplaceWith$ ETBTapped` resolves to `Ability.Subs` for free -- zero new
compiler work, the SVar it names already sitting there the identical way a trigger's own `Execute$` sub-ability does.

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
`TestPlayLandDoesNotEnterTappedWhenDestinationDoesNotMatch`, `TestCheckMovedReplacementSkipsSubAbilityChain` and
`TestCheckMovedReplacementSkipsConditionalTap` prove each of the three ways a line does not resolve.

Not resolved: `ETBTapped`/`LandTapped` naming a `SubAbility$` or a `ConditionPresent$`/`ConditionCheckSVar$` pair
(above); `ReplaceWith$ Exile`/`DBTap`/`DBExile`/`DoDay`/`PayBeforeETB`/... (352 of the remaining 969 Moved lines);
`Untap`'s and `DamageDone`'s own `ReplaceWith$`-driven remainders (below); every `Event$` value past `Moved`/`Untap`/
`DamageDone` (`Counter`, `Draw`, `AddCounter`, `CreateToken`, `GainLife`, `BeginPhase`, `GameLoss`, `ProduceMana`, ...
-- a majority of the corpus's own real replacement lines); and CR 616's own general layering/ordering procedure
entirely, moot for this slice's one idempotent outcome but real the moment a second resolvable replacement effect
produces a different one.

`enginelint.json` gained a `"replacement"` group (`replacement.go`), added to `"land"`'s and `"castspell"`'s own allow
lists (both now call `checkMovedReplacement`) and, like `"trigger"`/`"continuous"` before it, to its own allow list an
`"ability"` entry it does not actually depend on: `compile.Ability` collides textually with the top-level `Ability`
declared in `ability.go` the identical way `ControlEffect.Player` once collided with the `Player` type (item 27's own
`ControlMod` paragraph) -- `enginelint`'s own identifier scan cannot tell a qualified external reference apart from an
unqualified same-package one.

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
reasoning applied to a different outcome. `IsPresent$`/`SVarCompare$`/`CheckSVar$`/ `EnduringStory$`/`AddSVar$` (7 of
156, each its own further restriction this file cannot evaluate) skip the whole line rather than blocking
unconditionally (PORT-8/GO-7) -- an allow-list of exactly the params real corpus lines pair with this shape
(`Event$`/`Layer$`/`Description$`/`ValidCard$`/`ValidStepTurnToController$`/`ActiveZones$`/ `Secondary$`), the identical
style `tapAbilityIsPlainTap` already has, rather than a reject-list of the ones found: a param neither list has seen yet
skips by construction instead of silently passing.

`untapStep` (turn.go) calls `untapBlocked` per permanent before clearing `Tapped`, leaving `SummonSick`'s own clearing
unconditional: CR 502.3's own "doesn't untap" restricts only the untapping action, not CR 302.6's own continuous-control
question a doesn't-untap lock has nothing to do with.

`TestUntapBlockedBySelfCantHappenReplacement` proves the simplest real shape and the SummonSick independence;
`TestUntapBlockedByOpponentsCantHappenReplacement` proves the "other" half; `TestUntapBlockedWhenHostInCommandZone`
proves the `ActiveZones$` generalization; `TestUntapNotBlockedWhenValidCardDoesNotMatch`,
`TestUntapNotBlockedByReplaceWithShape` and `TestUntapNotBlockedByUnresolvedExtraParam` (replacement_test.go) prove each
of the three ways a line does not block.

### `Event$ DamageDone`: CR 614's own "prevent all of this damage"

218 real `Event$ DamageDone` lines, the corpus's own second most frequent `Event$` value. 72 name `Prevent$ True`; the
other 146 name `ReplaceWith$` naming `DB$ ReplaceEffect` (71), `DB$ ReplaceDamage` (27, an amount modifier --
double/plus/minus damage), `DB$ RemoveCounter`/`PutCounter` (31 combined, "damage becomes counters" instead) and a dozen
smaller shapes -- no single sub-ability anywhere near `ETBTapped`'s own 618-line concentration, so none of them was
worth building on its own; all 146 stay unresolved.

`Prevent$ True` needed no sub-ability at all to resolve, `ReplacementHandler.java`'s own dispatch read directly: a
`Prevent$ True` (or `PreventionEffect$`) line returns `ReplacementResult.Prevented` straight off `getParam("Prevent")`,
nothing replaces the event, it simply does not happen -- the identical "the event doesn't happen" shape `Event$ Untap`'s
own `Layer$ CantHappen` already is, for a different `Event$` value. `ReplaceDamage.canReplace` is the resolvable half
read for its own `canReplace` gate -- `ValidSource$`/`ValidTarget$` matched the identical way `damageDoneMatches`'s own
pair already is (trigger firing, above), split into a `*Card`/ `*Player` pair for the reason
`checkDamageDoneTriggersToCard`/`ToPlayer` already are (`damagePrevented`/ `damagePreventedPlayer`, `replacement.go`,
new), and `IsCombat$` compared against a hardcoded `true` the identical way every real `DamageDone` trigger check
already does, since nothing outside combat deals damage in this port yet.
`PlayerTurn$`/`SVarCompare$`/`IsPresent$`/`CheckSVar$`/`ValidCause$`/`RelativeToSource$`/`DamageAmount$`/
`CauseIsSource$` (10 of 72, each carrying its own further restriction `ReplaceDamage.canReplace` itself reads but this
file cannot) skip the whole line via the identical allow-list style `untapReplacementMatches` above has, rather than
preventing unconditionally.

`dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go) call `damagePrevented`/`damagePreventedPlayer` first, before
marking any damage, emitting `DamageDealt`, or checking CR 603's own "deals damage" trigger -- a prevented damage
instance never happened at all, the identical "look at the event before it happens" ordering CR 614.1 already has over
CR 603 for `checkMovedReplacement`, applied here to a different `Event$` value and a different outcome (nothing, rather
than `Tapped = true`).

`TestDamageToPlayerPreventedByReplacement`/`TestDamageToCreaturePreventedByReplacement` (replacement_test.go) prove the
`*Player`/`*Card` split; `TestDamageToPlayerNotPreventedWhenValidTargetDoesNotMatch`/
`TestDamageToCreatureNotPreventedWhenValidSourceDoesNotMatch` prove `ValidTarget$`/`ValidSource$` are checked, not
assumed; `TestDamageToCreatureNotPreventedByUnresolvedExtraParam` proves the tenth unresolved param skips rather than
prevents.

Both new shapes share `replacementActiveZones`/`hostInActiveZones` (`replacement.go`), generalizing `ActiveZones$` past
Battlefield alone -- absent means Battlefield, a comma list otherwise, an unrecognized zone name skipping the whole line
the identical contract `validCountZones` (amount.go) already has for a `Count$Valid<Zone>` suffix -- and
`replacementZones`, the `[Battlefield, Command]` pair both `untapBlocked` and `damagePrevented`/`damagePreventedPlayer`
walk across every player, since 2 real lines of each shape name a Command-zone host.

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
reject-list of the ones found (`tapAbilityIsPlainTap`'s own style, replacement.go) -- a param neither list has seen
skips by construction instead of silently applying (PORT-8/GO-7): `SubAbility$` (80 of 822) -- no ability-chaining
mechanism exists at the stack level yet, `ResolveStack` resolves one top-level `AB$`/`DB$` record and stops, never its
own `SubAbility$` in turn; `Condition$`/`ConditionPresent$`/`ConditionCompare$`/`ConditionDefined$`/
`ConditionSVarCompare$`/`ConditionCheckSVar$` (83) -- `SpellAbilityCondition`'s own gate on the ability itself, distinct
from `CardTraitBase.meetsCommonRequirements` a trigger already has; `Planeswalker$`/`UnlessPayer$`/
`UnlessCost$`/`UnlessResolveSubs$`/`ValidTgts$`/`TriggeredSpellAbility$`/`DamageMap$`/`CounterNum$`/`Optional$`/
`TgtPrompt$` (each its own further mechanic, no real line among the 822 combining more than one); `NoPrevention$` (1) --
this port's own `damagePrevented`/`damagePreventedPlayer` would otherwise wrongly apply where Java's own
`AbilityKey.NoPreventDamage` says the damage cannot be prevented at all.

`NewRegistry` (`castspell.go`) registers `APIDealDamage`; new `enginelint` groups `defined` (above `game`/`player`) and
`dealdamageeffect` (above `card`/`game`/`player`/`ability`/`combatdamage`/`defined`/`amount`), `castspell` gaining
`dealdamageeffect` as a dependency to register into, `draweffect` gaining `card`/`defined`/`amount` for its own upgraded
`NumCards$` and shared `definedPlayers`. `TestCastSpellFiresOtherPermanentsWatchingTrigger` (Impact Tremors, item 26's
own "checking the mechanism, not the content" trigger fixture) changed from checking `ResolveStack` reports
`ErrUnimplemented` naming `DealDamage` to checking both opponents' life actually drops -- the same fixture, testing what
is now really there. Ten new tests (`dealdamageeffect_test.go`) drive every resolvable and every rejected shape through
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

| Missing                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | Lands |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `CardState` — face/characteristics data for transform, flip and meld                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | M5    |
| 90 of `PlayerController`'s 110 methods — everything needing `SpellAbility`, non-Aura targeting or the rest of Combat past dealing damage                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | M5-M6 |
| `AIController`, the real (non-scripted) implementation                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             | M7    |
| Non-combat damage to a planeswalker or a Battle (a burn spell, an activated ability) — combat damage already removes loyalty/defense counters (CR 120.3c, 121.5); nothing outside combat deals damage at all yet                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | M5-M6 |
| The rest of CR 704.5f/704.5g's toughness — `*`, `1+*`, a `Count$` reference, or toughness a continuous effect or a counter has changed — needs `internal/expr` and the layer system, not just `strconv.Atoi`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | M5-M6 |
| The legend rule's own remaining corner case — Partner-with-non-legendary-creature-name pairs sharing a "true name" (needs `StaticData`'s own card-name lookup, which this port's `carddb`/`compile` layer has no equivalent of); `ignoreLegendRule` itself is ported (`## The legend rule needed CheckStateBasedActions to take a controller`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              | M5-M6 |
| CR 613.6-613.8's dependency reordering within a layer — `foldPT` only sorts by timestamp, correct until two effects on one card can actually disagree about order                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | M5-M6 |
| Layers 1 and 3 in full (copy -- not even part of `StaticAbilityContinuous.java`'s own switch in Forge itself, a separate resolution-time mechanism; text, `GainTextOf$`, 1 real line, needs its own single-card text-copying mechanism), plus Layer 8's own remainder (`MayLookAt$`/`MayPlay$`, `AddHiddenKeyword$`, vote/villainous-choice params -- the Layer 7/4/5/6/8/2 section's own `RulesMod` paragraph, above, has the exact reasons and counts) and the rest of Layers 4/5/6 past a literal token list and Layer 7a's own SVar shapes outside the Valid family (`xPaid`, `CardCounters`, `Devotion`, ... -- the same section's own `resolveAmount` paragraph) -- `applyContinuousControl`/`applyContinuousPT`/`applyContinuousType`/`applyContinuousColor`/`applyContinuousKeyword`/`applyContinuousRules` (continuous.go) resolve Layer 2's own `GainControl$ You` (43 of 44 real lines), Layer 7b/7c's own plain-integer AND now Count$Valid-SVar-driven lines, Layer 7a's own Count$Valid-SVar-driven `CharacteristicDefining$` lines (`applyOneCharacteristicDefiningPT`), 201 of 284 real `AddType$`/`RemoveType$` lines, 54 of 61 real `AddColor$`/`SetColor$` lines, 1,556 of 1,857 real `AddKeyword$` lines and 75 of 78 real `SetMaxHandSize$`/`RaiseMaxHandSize$`/`AdjustLandPlays$` lines, all `Affected$`-matched; the rest all need the rest of the same general engine, mostly for keys a full `AbilityUtils.calculateAmount` port, a `*cardtype.Registry` this port does not inject into the engine, or a cast-time zone permission this port has never needed before would resolve, not a new layer number to add                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | M5-M6 |
| `changeZone`'s replacement effects, triggers, last-known-information and token/copy-vanishing rules                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | M5-M6 |
| `PhaseHandler`'s Upkeep, Main and End of Turn step bodies — need triggers, `SpellAbility` or the rest of Combat                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | M5-M6 |
| `Match` — a series spanning more than one game, "the loser of the last game goes first" (`DealOpeningHands` always takes CR 103.2's coin flip), Puzzle/Archenemy/Power Play's own starting-player rules                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | M5-M6 |
| The rest of CR 514.2: "until end of turn"/"this turn" effects ending — needs duration tracking this port does not have, `PT`'s own effects included                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                | M5-M6 |
| A modified or unlimited maximum hand size (CR 514.1's `isUnlimitedHandSize`/a continuous effect changing it) — `MaxHandSize` is used unconditionally since layers 1-6/8 aren't built                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | M5-M6 |
| Interactive priority (`mainLoopStep`'s real APNAP pass), extra turns/phases, topsy-turvy phase order — `ResolveStack` plays out only the degenerate case, nobody able to respond ("doesn't untap" effects are no longer a gap here: `untapBlocked`, `## Replacement effects`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | M5-M6 |
| Original, Paris, Vancouver and Houston mulligan rules — out of scope, not deferred (PORT-6)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | never |
| `CounterChanged` for a script-written (non-named-constant) `CounterType` — `counterDetail` (`event.go`) is closed over the eight named constants; a `SpellAbility` creating an arbitrary keyword counter needs the encoding extended or replaced first (`## Events, wired`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | M5-M6 |
| `MagicStack`'s `undoStack` (needs an interactive priority pass to undo mid-pass); `freezeStack`/`unfreezeStack` is no longer a gap: `checkETBTriggers`/`checkDiesTriggers` pushing during another ability's own resolution needs no freezing since nothing can respond in between either way. `addSimultaneousStackEntry` itself is resolved (`pushTriggeredAbilities`, `## Stack`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | M5-M6 |
| Trigger firing (CR 603) beyond "enters"/"dies"/"attacks"/"blocks"/"becomes blocked"/"becomes blocked by a creature"/"deals damage"/"is discarded"/"becomes tapped"/"taps for mana"/"casts a spell"/"beginning of a step or phase"/"a player attacks"/"a player draws a card" and watching another permanent do one of those — `checkETBTriggers`/`otherETBTriggerMatches`/`checkDiesTriggers`/`otherDiesTriggerMatches`/`checkAttacksTriggers`/`checkBlocksTriggers`/`checkAttackerBlockedTriggers`/`checkAttackerBlockedByCreatureTriggers`/`checkDamageDoneTriggersToCard`/`checkDamageDoneTriggersToPlayer`/`checkDiscardedTriggers`/`otherDiscardedTriggerMatches`/`checkTapsTriggers`/`checkTapsForManaTriggers`/`checkSpellCastTriggers`/`checkPhaseTriggers`/`checkAttackersDeclaredTrigger`/`checkDrawnTriggers` (trigger.go) cover those eighteen, each now pushed through `pushTriggeredAbilities`' own CR 603.3b APNAP ordering rather than a fixed order (`## Stack`, above); every other mode (`Countered`, `Exiled`, `Sacrificed`, ...), `Attacks`'s own `Attacked$`/`FirstAttack$`, `Phase`'s own `IsPresent$`/`PresentCompare$`/`CheckSVar$`/`Condition$`/`FirstUpkeep$`/`FirstUpkeepThisGame$`/`FirstCombat$`/`TurnCount$` and its own qualified `ValidPlayer$` forms, `DamageDone`'s own `ValidCause$`/`TargetRelativeToCause$`/`TargetRelativeToSource$`, `Discarded`'s own `ValidCause$`, `Taps`'s own `FirstTime$`/`Teamwork$`, `TapsForMana`'s own `Produced$`, `SpellCast`'s own `Player.EnchantedBy`/`Player.Chosen` qualified `ValidActivatingPlayer$` forms, `AttackersDeclared`'s own `IsPresent$`/`PresentCompare$`/`CheckSVar$` and its own qualified `AttackedTarget$` forms (`Player.EnchantedBy`, ...), `Drawn`'s own `FirstCardInDrawStep$`/`ForReveal$`, and nine other unresolved params all remain gaps (`Blocks`'s own `ValidBlocked$`, `Attacks`'s own `Alone$`/`DefendingPlayerPoisoned$`/`AttackDifferentPlayers$`, `DamageDone`'s own `DamageAmount$`, `Phase`, `AttackersDeclared` and `Drawn` itself are all resolved now, and `matchesPlayerSpec`'s own Active/NonActive/Other property closed most of `SpellCast`'s/`DamageDone`'s/`TapsForMana`'s/`Phase`'s own qualified player specs); resolving what fires beyond `Draw` is M6's 202 remaining corpus-frequency effects, not this | M5-M6 |
| CR 616's own general "more than one replacement effect could apply, the affected player chooses" ordering procedure — needs a `PlayerController` hook this port does not have; moot for every outcome the three resolved shapes produce (`Tapped = true`, blocked, prevented — each idempotent) but real the moment a second resolvable replacement effect on the same event can disagree. Most `Event$` values past `Moved`/`Untap`/`DamageDone` (`Counter`, `Draw`, `AddCounter`, `CreateToken`, `GainLife`, ...), and each shape's own `ReplaceWith$`-driven remainder, stay unresolved too (`## Replacement effects`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | M5-M6 |
| The legend rule's own remaining corner case — Partner-with-a-non-legendary-creature-name pairs sharing a "true name" (needs `StaticData`'s own card-name lookup, which this port's `carddb`/`compile` layer has no equivalent of, and injecting one into the engine would violate GO-2); `ignoreLegendRule` itself is ported (`## The legend rule needed CheckStateBasedActions to take a controller`, above). Block legality's own `CantBlockBy` has no remaining gap: flying/reach, Fear, Horsemanship, Intimidate, Landwalk, Protection, Skulk, Menace and every literal `S:Mode$ CantBlockBy` line are all ported (`## Block legality: CantBlockBy`, above)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | M5-M6 |
| Any mana ability besides a basic land's own intrinsic one (`TapLandForMana`, CR 305.6, `## Mana pool and payment`, above) — a nonbasic land, a creature, an artifact all need the M6 effect-dispatch machinery that one deliberately bypasses, since CR 305.6's ability is a fixed rule keyed off the type line, not script text                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | M6    |
