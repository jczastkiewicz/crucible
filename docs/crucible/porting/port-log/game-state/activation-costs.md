# Port Log — Game State: Activation Costs

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine)

`cost.Cost` shapes: PayLife, PayEnergy, Exile, tapXType, Return, Exert, counters, zones.

## cost.Cost.ActivationShape replaces IsPureManaOrTap/IsPureManaTapAndSelfSac/SelfSac; Discard<N/Card> lands

Three chunks in a row ([`## Activating an ability lands`](activation.md#activating-an-ability-lands-cr-6022),
[`## ActivateAbility's own self-sacrifice cost`](activation.md#activateabilitys-own-self-sacrifice-cost-sac1cardname),
and this port's own general mana ability landing, all above) each added one more near-identical `cost.Cost` predicate
for `ActivateAbility`/`ActivateManaAbility`'s own shared cost-shape gate: `IsPureManaOrTap`, then
`IsPureManaTapAndSelfSac`, then `SelfSac` alongside it. A corpus scan for the next real activation-cost primitive worth
building -- `Discard<...>` as a cost, 387 real non-`AB$ Mana` `A:AB$` lines naming it as the only part past mana/Tap/
self-sac, dominated by the literal `Discard<N/Card>` shape (209 `Discard<1/Card>`, 16 `Discard<2/Card>`, 3
`Discard<3/Card>` -- 228 combined; `Discard<1/CARDNAME>`, 67 real lines, always paired with `ActivationZone$ Hand` in
the samples checked, CR 701.8a's own "discard moves a card from hand to graveyard" ruling out a battlefield-only
self-discard entirely -- `ActivateAbility`'s own battlefield-only entry point can never reach that shape regardless of
whether it were built, so it was not; the rest -- `Discard<1/Land>`/`Discard<1/Creature>`/`Discard<1/Random>`/... --
each its own further restricted-choice or random-choice shape) -- made the pattern impossible to ignore: a
`Discard<N/Card>` count could not fit a bare `bool` the way `Tap`/`SelfSac` could, forcing a signature change on
whichever predicate grew a fourth branch, and three near-identical predicates already sitting in the package was already
the sign a fourth should not be a fourth (this session's own standing anti-overengineering discipline cuts both ways:
avoiding premature abstraction does not mean repeating an established pattern past the point it stops paying for
itself).

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
([`## Produced$ Combo lands`](activation.md#produced-combo-lands-cr-6053bs-own-restricted-choice-version-of-any), where
the check turned out to be provably inert and was removed): each `case` guards on the matching field still being at its
zero value (`!shape.SelfSac`, `shape.DiscardN == 0`), so a second `Sac<1/CARDNAME>` or `Discard<1/Card>` Part fails that
case's own guard and falls through to `default: return ActivationShape{}, false` instead of silently re-triggering the
first branch -- `TestActivationShape`'s own `"Sac<1/CARDNAME> Sac<1/CARDNAME>"` and `"Discard<1/Card> Discard<1/Card>"`
cases both confirm `ok == false` with no extra code past the ordinary Part-matching logic itself.

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
`ChooseManaColor`'s own guard already demonstrated for `Pool.Add`
([`## Produced$ Any lands`](activation.md#produced-any-lands-cr-6053bs-own-choose-a-color)).

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

---

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

---

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

---

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

---

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

---

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

---

## Exert<1/CARDNAME> activation cost lands

`Return<...>`'s own landing closed the third and last real "move this off the battlefield" cost shape past Sac/Exile.
The next obvious candidate by the same "self-reference cost primitive" search pattern this whole cluster has followed
was `Exert<...>` (CostExert.java) -- CR 701.42a's own keyword action, "exert this permanent," most commonly seen as an
optional choice made while declaring attackers (a wholly separate mechanic this port does not model,
`DeclareCombatAttackers` has no such choice built into it, and stays out of scope here) but also printed directly as an
activation-cost part on 36 real corpus `A:AB$` lines -- every one of them the literal self-reference shape,
`Exert<1/CARDNAME>` (35) or `Exert<1/NICKNAME>` (1). No `Exert<N/Type>` line exists in the corpus at all, unlike
Sac/Exile/Return's own each having a chosen-type sibling -- confirmed by grepping every real `Exert<[^>]*>` value before
writing any code, not assumed from the shape of the other three primitives.

`SelfExert bool` joined `ActivationShape` as `ActivationShape`'s own fourth self-reference bool (`SelfSac`'s own sibling
count: self-sacrifice, self-exile, self-return, self-exert), the parsing case sharing `isSelfReferenceField` outright --
`Return`'s own landing had already generalized that check past a bare `== "CARDNAME"` comparison, so `Exert` needed no
separate fix the way `Sac`/`Exile` originally did.

What makes Exert genuinely different from every self-reference primitive built so far: paying it moves nothing at all.
Reading `Card.exert(Player)` (`Card.java:6372`) directly settled what actually happens: `exertedByPlayer.add(p)` (a
per-player set -- this port's own simplification collapses it to a single `Card.Exerted bool`, new `card.go`, since
every real activation-cost caller is the card's own controller and control does not realistically change before that
same controller's own next untap step for any corpus card) and `TriggerType.Exerted` fires, nothing else -- no zone
change, no `GameEventCardTapped`-style event either (only `Player.payLife`'s/`Player.loseEnergy`'s own kin fire an
ordinary state-change event; exert fires none). CR 701.42a's own trigger got a new
`isExertedTrigger`/`checkExertedTriggers` (new `exertcost.go`) -- but NOT `isDiesTrigger`'s own three-sibling shape
(`checkExiledTriggers`/`checkReturnedTriggers`'s own pattern, an own-card walk plus an other-watcher walk plus a `g.LKI`
lookback for the leaving card's own dying-state fields): exerting is not a zone change at all, the card never leaves the
battlefield, so there is no "dying state" to look back at and no own/other split needed either -- `checkTapsTriggers`'s
own single unified battlefield walk (`trigger.go`, already covers both "this card's own Taps ability" via
`ValidCard$ Card.Self` and "another permanent watching a tap" via `ValidCard$ Creature.YouCtrl`, in the identical single
pass) is the correct shape to reuse here instead, and all 5 real corpus `T:Mode$ Exerted` lines are the
`ValidCard$ Creature.YouCtrl` watcher shape, confirming the choice rather than merely permitting it.

CR 701.42b's own actual cost -- "it doesn't untap during your next untap step" -- is the part that took real research to
place correctly, since `CostExert.java`'s own `doPayment` does nothing more than call `exert()`; the deferred
consequence lives entirely in `Card.untap(Player)` (`Card.java:4703`), read directly:
`if (phase != null && isExertedBy(phase)) { return false; }`, checked BEFORE the replacement-effect handler even runs,
and `Untap.java`'s own main loop (`Untap.java:163`) separately, unconditionally clears every permanent's own exerted-by-
the-active-player flag every untap step regardless of whether untapping itself was skipped, blocked, or succeeded --
"remove exerted flags from all things in play... even if they are not creatures." `untapStep` (`turn.go`) now mirrors
both halves directly: `exerted := c.Exerted; c.Exerted = false` runs first (the unconditional clear, matching Java's own
separate pass), then `if !exerted && !g.untapBlocked(c)` gates the actual untap (Java's own `isExertedBy` check taking
priority over the replacement-handler check, ordering ported exactly). `Move`'s own battlefield-leaving reset
(`game.go`, both call sites -- the ordinary `Move` and `MoveToLibraryTop`) now clears `Exerted` alongside
`Tapped`/`SummonSick` too, the identical "this state means nothing off the battlefield" contract those two already have
-- Java's own `exertedByPlayer` set does not explicitly clear on a zone change either, but nothing in Java ever consults
it for a card that has left the battlefield, so the two are observably equivalent; this port's own explicit clear just
makes that equivalence a real invariant rather than an implicit one.

A regression-toggle pass on `untapStep`'s own new exerted check found the identical clean-failure shape `PayLife<N>`'s
own guard already established, not `Discard`'s own panic: forcing `exerted` to `false` right before the check turned
`TestUntapSkipsExertedPermanentAndClearsFlag`'s own first assertion into a clean `FAIL` (the permanent untapped when it
should have stayed tapped) rather than a crash -- `Card.Tapped` has no floor the way `Player.Life`/`Counters.Add` each
have one, so there was never a panic risk here to find.

`ActivateAbility` commits `SelfExert` in the same primitive order this session's whole cluster has been building (mana,
tap, sac, exile, return, exert, discard, life, energy, tap-by-type, return-by-type), needing no feasibility check of its
own -- `SelfSac`'s own precedent, the source is already known to be on the battlefield. `ActivateManaAbility` pays it
too rather than declining it -- `PayEnergy`/`SelfExile`/`tapXType`'s own "pay it" precedent, not
`Discard`/`PayLife`/`Return`'s own "0 real benefit" one: 1 real `AB$ Mana` line needs it with no other unresolved param
(`Cost$ T Exert<1/CARDNAME> | Produced$ Any | Amount$ 2`, "T, Exert ~: Add two mana of any one color"). A second real
`AB$ Mana` line combining `Exert<1/CARDNAME>` also names `AddsKeywords$`/`AddsKeywordsValid$`/`AddsKeywordsUntil$` ("if
that mana is spent on a creature spell, it gains haste") -- already outside `manaAbilityAllowedParams` for a reason
unrelated to this landing (tagging the mana itself with a further effect this port's own `Pool` cannot carry), so it
stays unreachable regardless.

10 new tests: `activateability_test.go` gained five, `SelfExile`'s own set shape reused --
`TestActivateAbilitySelfExertCostExertsSourceAndRunsEffect` (`Card.Exerted` set, the source stays on the battlefield
unlike every earlier self-reference primitive, the ability still resolves),
`TestActivateAbilityCombinesTapAndSelfExert`, `TestActivateAbilityDeclinesForChosenExertCost` (`Exert<2/CARDNAME>`),
`TestActivateAbilityExertCostFiresOwnTrigger` (`Mode$ Exerted | ValidCard$ Card.Self`) and
`TestActivateAbilityExertCostFiresOtherWatcherTrigger` (`ValidCard$ Creature.YouCtrl`, the real corpus shape, reusing
`diesTriggerCreatureDefWithLine` outright). `activatemanaability_test.go` gained `TestActivateManaAbilityExertCost` (the
real two-mana shape end to end), plus corrected its own two pre-existing "one of `ActivationShape`'s own nine
primitives" doc comments to "ten" (`DOC-16`, the identical drive-by staleness fix every primitive landing in this
cluster keeps making for the one before it). `turn_test.go` gained `TestUntapSkipsExertedPermanentAndClearsFlag`,
proving both halves of CR 701.42b at once -- the skip, and the unconditional clear -- against the regression-toggle pass
above. `internal/cost/parsing_test.go` extended `TestActivationShape` with `Exert<1/CARDNAME>`/`Exert<1/NICKNAME>` alone
and combined with `T`, and three reject cases (`Exert<2/CARDNAME>`, a chosen valid spec, a duplicate).

---

## AddCounter<N/Type> and SubCounter<N/Type> activation costs land, CR 606's own loyalty ability

Every self-reference/chosen-type primitive built so far (`Sac`/`Exile`/`Return`/`Exert`) moves the ability's own host
off the battlefield or marks it. The next candidate by real corpus weight was `SubCounter<...>`/`AddCounter<...>`
(`CostRemoveCounter.java`/`CostPutCounter.java`) -- 950 and 354 real occurrences respectively, dominated by CR 606's own
loyalty ability: `SubCounter<N/LOYALTY>` and `AddCounter<N/LOYALTY>` pay a planeswalker's own `[+N]`/`[-N]` abilities,
confirmed by reading a real card (`ajani_goldmane.txt`) directly --
`A:AB$ GainLife | Cost$ AddCounter<1/LOYALTY> | Planeswalker$ True | ...`. A prior pass had named this "notably larger
scope" and deferred it, assuming it needed the whole loyalty/planeswalker mechanic built from scratch; re-reading
`Card.Counters`/`action.go`'s own zero-loyalty SBA (already ported) and `SpellAbility.isPwAbility()`'s own bare
`hasParam("Planeswalker")` check showed the real remaining scope was much smaller than assumed: `Planeswalker$` is a
plain param presence check, not a class of its own, and `Card.Counters` already exists to read/write.

`CostRemoveCounter`/`CostPutCounter` both default their own target field to `CARDNAME` (`CostPart.java`'s
`payCostFromSource`, `isSelfReferenceField` reused outright) -- confirmed by reading the constructors before writing any
parsing code, not assumed from the self-reference primitives that came before. `AddCounterN`/`AddCounterType` and
`SubCounterN`/`SubCounterType` joined `ActivationShape` (internal/cost) as a fourth field-pair shape, `TapTypeN`/
`TapTypeSpec`'s own pattern -- but with one real difference: `0` is a legal, common real value here.
`AddCounter<0/LOYALTY>` (54 real lines) is CR 606's own "+0" loyalty ability, and `SubCounter<0/LOYALTY>` (5 real lines)
an oddly-spelled version of the identical shape -- unlike every other `N` field in `ActivationShape` (`DiscardN`/
`PayLifeN`/... all treat `0` as absent), `AddCounterType`/`SubCounterType` being non-empty, not `N != 0`, is what
signals presence here. `namedParts` (`internal/cost/parts.go`) already had `AddCounter`/`SubCounter` registered from an
earlier chunk that never wired them into `ActivationShape` -- they were silently falling into `Cost.Mana` and failing
`mana.Parse` before this landing, a safe (if opaque) "always decline" rather than a bug.

`Card.LoyaltyAbilityActivated bool` (new field, `card.go`) is CR 606.3's own once-per-turn restriction --
`Card.planeswalkerAbilityActivated` in Java, an `int` collapsed to a `bool` here since the only reason Java counts past
one is `StaticAbilityNumLoyaltyAct`, a limit-raising static ability this port does not build (0 real corpus lines this
port can already resolve some other way depend on the raised limit). `ActivateAbility`/`ActivateManaAbility`
(activateability.go/activatemanaability.go) both check `ability.Param("Planeswalker")`'s own bare presence (not its
value -- the corpus writes both `Planeswalker$ True` and `Planeswalker$ true`) before anything else, decline if the flag
already reads `true`, and set it only once every other part of the cost has actually committed -- CR 606.3 restricts the
whole ability regardless of which shape paid for it, so a "+0" `AddCounter<0/...>` ability sets the flag exactly the
same as any other. Reset every cleanup (`cleanupStep`, `turn.go`, alongside `AttacksThisTurn`/`BecameTargetThisTurn`)
and on every battlefield-leaving `Move`/`MoveToLibraryTop` (`game.go`, alongside `Tapped`/`SummonSick`/`Exerted`) -- CR
400.7's own "a new object remembers nothing" applies here exactly as it does to those three.

`ActivateManaAbility`'s own `manaAbilityAllowedParams` gains `planeswalker`/`ultimate` (both previously outside the
allow-list, so any real `AB$ Mana` loyalty ability declined before even reaching `Cost$`) -- `Ultimate$` is purely
descriptive (`SpellAbilityView`'s own UI hint that an ability wins the game, never itself a restriction), admitted for
the same reason `SpellDescription$` already is.

CR 121.5's own floor -- "can't remove more counters than there are" -- is `CostRemoveCounter.java`'s own
`source.getCounters(cntrs) - amount >= 0`, checked before either caller commits anything
(`shape.SubCounterN > c.Counters.Count(...)`, the identical shape `PayLifeN`/`PayEnergyN`'s own floors already have).
`AddCounter` has no such floor: `CostPutCounter.java`'s own `canPay` returns `true` unconditionally when
`getAbilityAmount(ability) == 0` (the "+0" shape again), and this port's `Counters.Add` never has a reason to refuse a
positive addition either -- `canReceiveCounters`'s own "shield counters"/"can't receive that kind of counter"
static-ability gate is not ported (0 real `LOYALTY` lines would ever hit it), the identical PORT-8/GO-7 tradeoff
`putCounterEffect`'s own doc comment already makes for the same gate.

`ActivateAbility` commits `AddCounter` then `SubCounter` last in the payment order (mana, tap, sac, exile, return,
exert, discard, life, energy, tap-by-type, return-by-type, add-counter, remove-counter), reusing `emitCounterChanged`
(event.go) the identical way `putCounterEffect`/`PayEnergy` already do. `ActivateManaAbility` pays both too rather than
declining them -- the identical "corpus actually needs it" precedent `PayEnergy`/`SelfExile`/`SelfExert`/`tapXType`
already set: of the corpus's 83 real `AB$ Mana` lines naming `AddCounter<...>`/`SubCounter<...>`, 27 resolve fully (3
`AddCounter`, 24 `SubCounter`, 6 of the 27 loyalty abilities) once `Produced$`'s own literal-shape gate
(`producedManaColor`/`parseComboColors`) is checked too -- a `Produced$ R G` bare-space-separated multi-color line (not
`Combo R G`) still declines the identical way every other `Produced$` shape past the three resolvable ones already does,
catching an over-count in this landing's own first pass (31, before checking `Produced$` at all -- a mana rock's
`SubCounter<1/CHARGE>: Add one mana of any color` shape is the corpus's own dominant real one for the second).

**A real, separate bug surfaced and fixed in the same pass:** pushing a `Planeswalker$`-carrying ability onto the stack
for the first time (this port had never been able to before) immediately hit
`engine: GainLife: Planeswalker$ not resolvable yet` -- nine already-built effects
(`dealDamageEffect`/`gainLifeEffect`/`loseLifeEffect`/`pumpAllEffect`/
`putCounterEffect`/`scryEffect`/`sacrificeAllEffect`/`sacrificeEffect`/`surveilEffect`) each independently listed
`Planeswalker`/`Ultimate` (two of them) in their own unresolved-param blocklist, defensively added by an earlier chunk
that saw the param on real corpus lines without realizing it is purely a cost-side marker
(`SpellAbility.isPwAbility()`'s own bare `hasParam` check) with zero bearing on how the effect it cost-gates actually
resolves. Removed from all nine (a plain "cost params never block effect resolution" rule this session had not needed to
state explicitly until an ability carrying one could finally reach `Registry.Resolve` at all), each with its own
headline corpus count corrected upward: `dealDamageEffect` 65→72, `gainLifeEffect` 857→862, `loseLifeEffect` 300→306
(the `Defined$` branch alone; the `ValidTgts$` branch is unaffected), `putCounterEffect` 992→993, `scryEffect` 332→340,
`surveilEffect` 183→187, `sacrificeAllEffect` 91→92, `sacrificeEffect` 516→522, `pumpAllEffect` 642→668 (`Ultimate$`'s
own removal folded into the same count, since every real `PumpAll` line naming it also names `Planeswalker$`). This was
not a hypothetical found by re-reading old code for its own sake: it was found because this landing was the first thing
in the whole session to actually try pushing one of these abilities through `ResolveStack`, and every one of the nine
failed identically the first time it was tried.

16 new tests: `activateability_test.go` gained a new `planeswalkerDefWithAbility` helper (`planeswalkerDefLoyalty`'s own
sibling, built through the real compiled param parser, TEST-1) and seven tests --
`TestActivateAbilityAddCounterCostAddsLoyaltyAndRunsEffect`/`...SubCounterCostRemovesLoyaltyAndRunsEffect` (the
Ajani-shaped `[+1]`/`[-1]` abilities end to end), `...DeclinesSubCounterCostWhenNotEnoughLoyalty` (CR 121.5's floor),
`...AddCounterZeroCostStillActivatesAndSetsLoyaltyFlag` (the "+0" shape, proving the once-per-turn flag still sets),
`...DeclinesSecondLoyaltyAbilityActivationSameTurn` (CR 606.3, two different abilities on one permanent, the second
declining even though its own cost is independently payable -- `ResolveStack` run between the two activations so the
empty-stack timing gate is not what is actually being tested), `...NonLoyaltyCounterCostHasNoOncePerTurnLimit` (the gate
is conditional on `Planeswalker$`'s own presence, not a blanket rule over every counter cost) and
`...DeclinesForNonSelfAddCounterTarget` (a chosen-target `AddCounter` still fails closed). `activatemanaability_test.go`
gained five --
`TestActivateManaAbilityAddCounterLoyaltyCost`/`...SubCounterCost`/`...DeclinesSubCounterCostWhenNotEnoughCounters`/
`...DeclinesWhenLoyaltyAbilityAlreadyActivated` (proving the flag is shared state, not per-caller: set through
`ActivateManaAbility`, read the identical way `ActivateAbility` reads it). `internal/cost/parsing_test.go` extended
`TestActivationShape` with accept cases for both primitives (including the `0` shape, the explicit `CARDNAME`/
`NICKNAME` target forms, and combining the two) and reject cases (non-literal `N`, a negative `N`, a chosen target, a
duplicate part).

---

## ExileFromGrave<1/CARDNAME> activation cost lands, CR 602.2's own ActivationZone$ generalization

Corpus-frequency research for the next primitive turned up `ExileFromGrave<...>` -- `CostExile.java`'s own graveyard-
origin constructor, the identical class `Exile<1/CARDNAME>` already reads, a different `ZoneType` argument -- at 294
real occurrences, dominated by the self-reference shape (`ExileFromGrave<1/CARDNAME>`, 78, plus
`ExileFromGrave<1/ CARDNAME/this card>`, 20). Reading a real card that carries it (`rubblebelt_maverick.txt`) directly
showed why this primitive alone would pay off nothing:
`A:AB$ PutCounter | Cost$ G ExileFromGrave<1/CARDNAME> | ActivationZone$ Graveyard | ...` -- `ActivateAbility` hardcodes
`c.Zone != Battlefield` at its own top gate, so no ability naming `ActivationZone$` at all could ever activate
regardless of what its own `Cost$` could pay. `ActivationZone$` is Java's own general mechanism
(`SpellAbilityRestriction.checkZoneRestrictions`) for an ability activated from somewhere other than the battlefield --
230 real corpus lines name `Graveyard` (Escape/Unearth-style abilities), 97 name `Hand` (Cycling, Transmute) and 57 name
`Command` (emblems, sagas). This landing scopes to `Graveyard` alone, the corpus's own dominant real destination and
`ExileFromGrave`'s own natural pairing; `Hand`/`Command` stay unbuilt.

`checkZoneRestrictions` (`SpellAbilityRestriction.java:215-259`), read directly, settled the real semantics: a
Graveyard-zone ability's own "you" is the source's **owner**, not its controller -- CR 109.5's own "a card outside the
battlefield has no controller." `ActivateAbility`/`ActivateManaAbility` both switch on `ability.Param("ActivationZone")`
now, fetched right after the ability itself (reordered ahead of the old top-of-function zone/controller check, since
that check now depends on which ability is being activated rather than being a fixed precondition): absent or the
literal `Battlefield` keeps the identical `c.Controller() != pid || c.Zone != Battlefield` gate; `Graveyard` swaps it
for `c.Owner != pid || c.Zone != Graveyard`; anything else (`Hand`/`Command`/`Exile`/`Stack`, 1 real line each for the
last two) declines outright.

`SelfExileFromGrave bool` joined `ActivationShape` (`internal/cost`) as a fourteenth primitive, `isSelfReferenceField`
reused outright for its own self-reference check the identical way `SelfExile`'s already does. A Graveyard-zone ability
naming any OTHER primitive above -- `Tap`, `SelfSac`, `Discard`, `AddCounter`, ... -- declines outright rather than
committing nonsense (tapping a card that is not a permanent, sacrificing one that is not on the battlefield): 0 real
corpus lines combine `ActivationZone$ Graveyard` with anything but plain mana or `ExileFromGrave<1/CARDNAME>`, confirmed
by grepping every real line's own `Cost$` before writing the guard rather than assuming symmetry with the battlefield
primitives. Paying `SelfExileFromGrave` calls a new `exileFromGraveyard` (new `exilefromgrave.go`) -- `exileCards`'s
(exile.go) own much simpler sibling: a plain `g.Move(id, Exile, owner)`, no trigger check and no `g.LKI` snapshot at
all, since CR 603.6d's own "leaves the battlefield" family (`checkExiledTriggers`) is specifically about a permanent
leaving the battlefield, which a graveyard card never was for this move. 56 real corpus
`T:Mode$ ChangesZone | Origin$ Graveyard` lines exist -- a separate, unrelated, still-unbuilt "leaves the graveyard"
trigger family this landing does not reach either.

Of the corpus's own 220 real `A:AB$ ... | ActivationZone$ Graveyard` lines, 168 resolve at the cost-shape level (82 pure
mana, 86 mana plus `ExileFromGrave<1/CARDNAME>`); the other 52 name a chosen-type `Sac<.../Discard<.../tapXType<...`
component this decomposition does not carry for a graveyard ability (`Sac<1/Clue>`, `Mill<4>`, ... -- correctly declined
by the existing `ActivationShape` gate with no new code). Of those 168, 49 actually run an already-built effect end to
end (19 `PutCounter`, 11 `Pump`, 10 `Draw`, 3 `PumpAll`, 2 `GainLife`, 1 each of `Discard`/`Scry`/`DealDamage`/ `Mana`);
87 name `ChangeZone` (Escape's/Unearth's own dominant real effect, "return this card from your graveyard to the
battlefield") and the remainder name another API (`Token`, `MakeCard`, ...). `ActivateManaAbility` pays
`SelfExileFromGrave` too -- the sole real `A:AB$ Mana | ActivationZone$ Graveyard` line
(`Cost$ 1 ExileFromGrave<1/CARDNAME> | Produced$ Any`) needed `activationzone` added to `manaAbilityAllowedParams`
alongside the zone-check generalization.

10 new tests: `activateability_test.go` gained six -- `TestActivateAbilityExileFromGraveCostExilesSourceAndRunsEffect`
(source moves Graveyard→Exile, the ability still resolves), `...GraveyardAbilityWithPureManaCostStaysInGraveyard` (the
corpus's own dominant real shape: the source is untouched, only the effect -- not built here -- would move it),
`...DeclinesGraveyardAbilityWhenNotInGraveyard`, `...DeclinesGraveyardAbilityForNonOwner` (CR 109.5's own
owner-not-controller check, proven by activating as the non-active-turn player who nonetheless owns the card),
`...DeclinesGraveyardAbilityCombinedWithTapCost` and `...DeclinesUnsupportedActivationZone` (`Hand`).
`activatemanaability_test.go` gained `TestActivateManaAbilityExileFromGraveCost`, the real two-part shape end to end.
`internal/cost/parsing_test.go` extended `TestActivationShape` with `ExileFromGrave<1/CARDNAME>`/`<1/NICKNAME>` alone
and combined with mana, and three reject cases (a chosen count, a chosen type, a duplicate).

---

## ActivationZone$ Hand lands, Discard<1/CARDNAME> and ExileFromHand<1/CARDNAME> activation costs -- CR 702.28's own Cycling

`ExileFromGrave<1/CARDNAME>`'s own landing generalized `ActivateAbility`/`ActivateManaAbility` past a single hardcoded
`ActivationZone$` -- the natural next step was `Hand`, the corpus's own second-largest real destination at 97 lines
(behind Graveyard's 230, ahead of Command's 57, which stays unbuilt). Corpus research turned up two new self-reference
cost shapes pairing with it, neither reachable before this landing: `Discard<1/CARDNAME>` (69 real lines, CR 702.28's
own Cycling and its own kin -- "discard this card: draw a card") and `ExileFromHand<1/CARDNAME>` (14 real lines,
`CostExile.java`'s third real "from" zone, `SelfExileFromGrave`'s own sibling for the hand). Both had been silently
unreachable rather than merely undiscovered: `Discard<1/CARDNAME>`'s own literal `1/CARDNAME` shape does not match the
`p.Field(1) == "Card"` guard the existing choose-N-from-hand `DiscardN` case already has, so every real self-discard
line was falling through to the cost-shape's own `default: false` the entire time `Discard<N/Card>` has existed in this
port -- confirmed by checking whether the pre-existing `TestActivationShape` reject case for `"Discard<1/CARDNAME>"`
still held (it did, for the wrong reason: nobody had noticed the two shapes shared one token prefix with genuinely
different semantics until Cycling's own corpus weight came up in this landing's own research).

`SelfDiscard bool` and `SelfExileFromHand bool` joined `ActivationShape` (`internal/cost`) as a fifteenth primitive
pair, `isSelfReferenceField` reused for both the identical way every self-reference primitive before them already does.
`ActivateAbility`'s own three-way `ActivationZone$` switch (Battlefield/Graveyard/Hand) generalizes the same
owner-not-controller pattern `ExileFromGrave`'s own landing already established: `Hand` checks
`c.Owner != pid || c.Zone != Hand` in place of the Graveyard check's own zone. The cross-contamination guard grew a
third way too -- `nonBattlefield := fromGraveyard || fromHand` gates every battlefield-only primitive uniformly, then
four more one-line checks refuse the WRONG zone's own self-reference primitive (a Graveyard ability naming
`SelfDiscard`, a Hand ability naming `SelfExileFromGrave`, ...) -- 0 real corpus lines combine `ActivationZone$ Hand`
with any battlefield-only primitive or with the graveyard's own `ExileFromGrave`, confirmed by grep before writing the
guard, the identical discipline `ExileFromGrave`'s own landing already established.

Paying `SelfDiscard` reuses `discardCards` (discardeffect.go) wholesale -- CR 701.8's own `Mode$ Discarded` trigger
fires the identical way it already does for the existing choose-N-from-hand shape, since Cycling really is an ordinary
discard, just of a fixed, self-chosen card. This is the one real difference from
`SelfExileFromGrave`/`SelfExileFromHand` (both of which fire nothing, exile.go's own doc comment already has the CR
603.6d reasoning for why exiling from a non-battlefield zone triggers nothing): a card being cycled really is discarded,
and every existing discard-trigger machinery already built for the unrelated `DiscardN` shape applies unchanged. A new
`exileFromHand` (`exilefromgrave.go`, renamed in spirit though not in file name to cover its own second real "from" zone
now) is `exileFromGraveyard`'s exact sibling -- a plain zone move, no trigger, no `g.LKI`.

Of the corpus's own 95 real `A:AB$ ... | ActivationZone$ Hand` lines, 92 resolve at the cost-shape level; 41 of those
actually run an already-built effect end to end (25 `Pump`, 6 `DealDamage`, 3 `PutCounter`, 2 `Draw`, 2 `Mana`, 1 each
of `Sacrifice`/`GainLife`/`Discard`) and 14 name `ChangeZone` (still this port's own single largest unbuilt API). 2 of
the 92 real `AB$ Mana | ActivationZone$ Hand` lines name `ExileFromHand<1/CARDNAME>` and resolve fully; `SelfDiscard`'s
own real `AB$ Mana` payoff is 0 lines, so `ActivateManaAbility` declines it outright, `DiscardN`/
`PayLife`/`SelfReturn`'s own "0 real benefit" precedent rather than `PayEnergy`/`SelfExile`/`tapXType`'s own "pay it"
one.

9 new tests: `activateability_test.go` gained four -- `TestActivateAbilitySelfDiscardCostDiscardsSourceAndRunsEffect`
(Hand→Graveyard, `AB$ Draw`'s own real Cycling shape end to end), `...SelfExileFromHandCostExilesSourceAndRunsEffect`
(Hand→Exile), `...DeclinesHandAbilityCombinedWithTapCost` (the cross-zone guard, `ExileFromGrave`'s own identical
Tap-decline test mirrored for Hand) and a rewritten `...DeclinesUnsupportedActivationZone` (now testing `Command`, since
`Hand` itself is no longer unsupported -- the old test's own zone choice aged out from underneath it, caught by simply
running the existing suite after landing this primitive rather than by any dedicated regression check).
`activatemanaability_test.go` gained `TestActivateManaAbilityExileFromHandCost`, the real two-color-variant shape
(`Cost$ ExileFromHand<1/CARDNAME> | ActivationZone$ Hand | Produced$ R`) end to end. `internal/cost/parsing_test.go`
extended `TestActivationShape` with accept cases for both primitives (alone, combined with mana) and reject cases (a
duplicate, a chosen count, a chosen type) -- and turned the pre-existing `"Discard<1/CARDNAME>"` reject case into an
accept case, the one existing test this landing's own new parsing logic changed the answer to.
