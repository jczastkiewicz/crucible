# Port Log — Game State: M6 Effects: PutCounter to SacrificeAll

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine)

M6's next single-effect sections.

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
(["SubAbility chaining itself lands"](targeting-and-chaining.md#subability-chaining-itself-lands),) landed: 56 of the
corpus's own 623 real SVar-defined `PutCounter` lines naming `SubAbility$` chain to an already-built leaf ability and
resolve end to end.

Not resolved, each failing loudly by name rather than guessing (PORT-8/GO-7): `ValidTgts$`/`TargetMin$`/`TargetMax$`
(807/162/162) -- a real target; targeting itself now exists
(["Targeting itself lands"](targeting-and-chaining.md#targeting-itself-lands),), `PutCounter` just has not been extended
to read `Targeted` back yet; `ETB$` (154, above); `Choices$` and its own six further params (46 combined, above);
`DividedAsYouChoose$`/`DividedRandomly$`/`SplitAmount$` (above);
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

---

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
(["SubAbility chaining itself lands"](targeting-and-chaining.md#subability-chaining-itself-lands),) landed: 11 of the
corpus's own 254 real SVar-defined `Discard` lines naming `SubAbility$` chain to an already-built leaf ability and
resolve end to end. `UnlessCost$`/`UnlessPayer$`/ `UnlessSwitched$`/`UnlessResolveSubs$` no longer block either, removed
once `resolveUnlessCost` ("`Registry.Resolve`'s own `UnlessCost$` gate," below) landed: 0 of the corpus's own 16 real
`Discard` lines naming `UnlessCost$` resolve, though -- rhystic_scrying.txt's own real pure-mana "{2}" is the only one
clearing that gate's own pure-mana-cost/ resolvable-payer filter, and its own `DB$ Discard` is reached only by chaining
out of a top-level `A:SP$ Draw` line on a Sorcery, which `CastSpell` cannot cast at all; every other real line names a
`PayEnergy<.../Sac<.../Return<.../...` cost part or a controller-derived `UnlessPayer$` this port cannot resolve.

Not resolved, each failing loudly by name rather than discarding the wrong cards from the wrong player (PORT-8/GO-7):
every `Mode$` other than `TgtChoose` (above, 214 real lines combined); `ValidTgts$`/`TargetMin$`/`TargetMax$` (98/3/3)
-- a real target; targeting itself now exists
(["Targeting itself lands"](targeting-and-chaining.md#targeting-itself-lands),), `Discard` just has not been extended to
read `Targeted` back yet; `Optional$` (38) -- an interactive confirm this port's own `PlayerController` has no hook for;
`AnyNumber$` (16) -- a variable, 0-to-hand-size count, a different shape from `ChooseCardsToDiscard`'s own exact-count
contract; `DiscardValid$`/`DiscardValidDesc$` (18) -- a filtered choice set, the identical gap `PutCounter`'s own
`Choices$` family already documents; `UnlessType$` (14) -- Java's own separate `chooseCardsToDiscardUnlessType`
controller method, a different sub-flow entirely; `RevealNumber$` -- a reveal-then-choose-a-subset step ahead of the
discard itself, not modeled; `RememberDiscarded$`/`RememberDiscardingPlayers$`/`RememberDiscardingPlayer$` (88 combined)
-- `defined.go` now reads `Defined$ Remembered` back, but `Discard` itself does not write these yet, the identical
"blocked outright rather than silently no-op'd" choice `PutCounter`'s own `RememberCards$` already made. `Condition$`
itself and `ConditionDefined$`/`ConditionZone$` -- `SpellAbilityCondition`'s own shapes `subAbilityConditionMet` does
not cover, the identical `Pump`/`GainLife`/`LoseLife`/`PutCounter`-shaped gap;
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

---

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
target; targeting itself now exists (["Targeting itself lands"](targeting-and-chaining.md#targeting-itself-lands),),
`Scry` just has not been extended to read `Targeted` back yet; `Optional$` (4) -- an interactive confirm this port's own
`PlayerController` has no hook for; `Planeswalker$` (8) -- its own further mechanic. `Condition$` itself and
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

---

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
`resolveSubAbility` (["SubAbility chaining itself lands"](targeting-and-chaining.md#subability-chaining-itself-lands),)
landed: 2 of the corpus's own 15 real SVar-defined `Surveil` lines naming `SubAbility$` chain to an already-built leaf
ability and resolve end to end.

Not resolved, each failing loudly by name rather than surveiling the wrong cards (PORT-8/GO-7): `ValidTgts$` --
targeting itself now exists (["Targeting itself lands"](targeting-and-chaining.md#targeting-itself-lands),), `Surveil`
just has not been extended to read `Targeted` back yet; `Planeswalker$` (5) -- its own further mechanic;
`RememberMoved$`/`RememberKept$` (2/1) -- not written yet, although `defined.go` now reads `Defined$ Remembered` back,
the identical "blocked outright rather than silently no-op'd" choice `PutCounter`'s own `RememberCards$` already made;
`Optional$` -- present on 0 real `Surveil` lines today, blocked anyway for symmetry with `Scry`'s own identical param,
in case a future card adds it. `Condition$` itself and `ConditionDefined$`/`ConditionZone$`/`ConditionPlayerTurn$`
already skip the whole line through `subAbilityConditionMet`'s own unresolved-param list, the identical silent-skip
`Scry`'s own already gets; `ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` resolve
through it exactly as `Scry`'s/`Discard`'s/`PutCounter`'s own already do.

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

---

## M6's twelfth effect: Sacrifice, and CR 701.20's own Mode$ Sacrificed trigger

`Sacrifice` (CR 701.20, `SacrificeEffect.java` + `GameAction.sacrifice`/`sacrificeDestroy`) is the corpus's own dominant
real shape past `SacValid$` absent or the literal value `Self`: 516 of the corpus's 792 real `(AB|DB)$ Sacrifice` lines
resolve (51 of them past `resolveUnlessCost`'s own new gate,
["`Registry.Resolve`'s own `UnlessCost$` gate"](activation.md#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),),
more real lines than the "no `SacValid$` at all" default this port's own effects usually resolve first.

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
`memory.go` was first built with no caller (`memory.go`'s own doc comment). A chained `SubAbility$` reads it back
through `Defined$ Remembered` (`defined.go`).

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
(["`Registry.Resolve`'s own `UnlessCost$` gate"](activation.md#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),)
landed: 51 of the corpus's own 155 real `Sacrifice` lines naming `UnlessCost$` resolve past that gate and are actually
reachable at all -- 6 of the 57 real lines that clear `resolveUnlessCost`'s own pure-mana-cost/resolvable-payer filter
still are not, each a `S:Mode$ Continuous | AddTrigger$` line's own dynamically granted trigger
(aura_flux.txt's/magus_of_the_tabernacle.txt's own real "other permanents have 'sacrifice this unless you pay...'" among
them) -- `AddTrigger$` is not a built continuous-effect param, so the trigger it would grant never exists in this port's
own game at all.

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

---

## M6's thirteenth effect: SacrificeAll, Sacrifice's own blanket sibling

`SacrificeAll` (`SacrificeAllEffect.java`) landed the moment `Sacrifice`'s own chunk (above) closed, `pumpAllEffect`'s
own shape (pumpalleffect.go,
["M6's sixth effect: PumpAll"](effects-m6-first.md#m6s-sixth-effect-pumpall-the-blanket-sibling),) reused for a second
blanket effect rather than rebuilt: an absent `Defined$` scans every battlefield in the game (Java's own
`game.getCardsIn(Battlefield)`), `ValidCards$`-filtered if present (`AbilityUtils.filterListByType`) -- 72 of the
corpus's own 140 real `(AB|DB)$ SacrificeAll` lines name no `Defined$` at all, the corpus's own dominant real shape --
and a present `Defined$` names specific cards through `definedCards` (defined.go) instead:
`Self`/`Enchanted`/`Equipped`/`Targeted` resolve, an unrecognized value
(`TriggeredObjectLKICopy`/`ChosenCard`/`Remembered`/`EffectSource`/... -- real corpus values with no resolver,
`definedCards`'s own existing error return) failing the whole line loudly rather than silently sacrificing nothing.

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
removed once `resolveUnlessCost`
(["`Registry.Resolve`'s own `UnlessCost$` gate"](activation.md#registryresolves-own-unlesscost-gate-crs-own-unless-a-cost-is-paid),)
landed -- the identical "unless a cost is paid" gap `Sacrifice`'s own already documented: 0 of the corpus's own 6 real
`SacrificeAll` lines naming `UnlessCost$` resolve, though. ashling*the_limitless.txt's own real pure-mana
"{W}{U}{B}{R}{G}" is the only one clearing `resolveUnlessCost`'s own pure-mana-cost/resolvable-payer filter, and its own
`Defined$ DelayTriggerRememberedLKI` is reached only through a `DB$ DelayedTrigger` whose own `Mode$` is not `Phase` --
the one mode delayed triggers fire in (delayedtrigger.go); every other real line names a
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
