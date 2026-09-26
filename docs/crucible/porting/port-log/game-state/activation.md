# Port Log — Game State: Activated Abilities

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine)

`UnlessCost$`, activating abilities, mana abilities, `Produced$`.

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
`{B}:` ability -- reachable once `ActivateAbility` landed too,
[`## Activating an ability lands`](#activating-an-ability-lands-cr-6022), below). The other 671 real lines fail one hop
up the call chain rather than at this gate itself -- each already documented against its own effect above -- falling
into one of five remaining shapes: an instant or sorcery's own top-level line (`CastSpell`'s own doc comment: "an
instant or sorcery resolves into a script effect this port does not build",
wild_might.txt's/rhystic_shield.txt's/rhystic_scrying.txt's own real lines among them, the last two also blocked a
second, independent way -- rhystic_scrying.txt's own `DB$ Discard` is itself reached only by chaining out of that same
uncastable top-level spell); a line reached only through an unbuilt API's own `SubAbility$`/`RepeatSubAbility$`/...
chain link (`DB$ Effect`, not built, or a `DB$ DelayedTrigger` of a `Mode$` other than `Phase` --
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
`PutCounter` lines at all) still do not resolve end to end: each is reached only through `DB$ GenericChoice`, which
fails closed on a choice carrying `UnlessCost$`, and separately omits `Defined$` entirely, which `definedCounterTargets`
has no default for (`AbilityUtils.getDefinedCards`'s own null-defaults-to-"Self" is not ported).

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

---

## Activating an ability lands, CR 602.2

`Player.playSpellAbility` (by way of `PlayerControllerHuman`'s own input loop) is Java's own entry point for CR 602 -- a
real priority-window action this port had no equivalent window for at the time `ActivateAbility` (activateability.go)
first landed, so it collapsed timing to `CastSpell`'s own CR 601.3a simplification (active player, a main phase, an
empty stack) for every activated ability alike. `PassPriority` (ADR-0019,
[`## Interactive priority: CR 117 lands`](turn-stack-combat.md#interactive-priority-cr-117-lands)) replaced that with CR
307.1's real split: instant speed by default, sorcery speed only for a loyalty ability (`Planeswalker$`) or one
explicitly marked `SorcerySpeed$`.

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
([`## Registry.Resolve's own UnlessCost$ gate`](#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),
above), needed because `IsPureMana`'s own flat "no `Parts` at all" contract cannot simply add `Tap` to its own allowed
set: a bare `T` token always also parses as its own named `Part` (`namedParts`' own trailing
`{name: "T", prefix: "T", exact: true}` entry, `internal/cost/parts.go`) alongside setting the `Tap` flag itself --
`parseCostPart`'s own real Java shape, `CostPartTap` a genuine `CostPart` object described and iterated like any other
rather than only a boolean -- so `IsPureManaOrTap` allows `Parts` to hold nothing but that one lone `"T"` entry and
rejects any other. This surfaced mid-implementation, not planned: the first draft reused `IsPureMana` directly and every
`Cost$ T` case failed with `len(parsed.Parts) != 0` even though `Tap` itself parsed correctly, caught by a debug trace
before any test's own expectations were adjusted around it.

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

---

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

---

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
([`## ActivateAbility's own self-sacrifice cost`](#activateabilitys-own-self-sacrifice-cost-sac1cardname), above) -- CR
601.2h's own "any order" reasoning applies here too, so this is the same free implementation choice, not a second
approximation of it. `checkTapsForManaTriggers` (trigger.go) -- CR 603's own "taps for mana" trigger, `TapLandForMana`'s
own pairing alongside the ordinary "becomes tapped" one -- fires only when the cost actually has a Tap component: a pure
self-sacrifice mana ability (a Treasure-shaped "Sacrifice this artifact: Add one mana of any color," when it happens to
name a literal color rather than `Any`) taps nothing, so nothing "becomes tapped to produce mana," the identical
zero-Tap skip `ActivateAbility`'s own `checkTapsTriggers` call already has.

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

---

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
immediately -- [`## Produced$ Combo lands`](#produced-combo-lands-cr-6053bs-own-restricted-choice-version-of-any),
below, is this exact parameter's own second real caller, a restricted two-to-four-color subset rather than all five.

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

---

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
