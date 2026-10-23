# ADR-0047 — A Mutated Permanent Is One Card With a Component List

- **Status:** Accepted
- **Date:** 2026-10-23
- **Deciders:** Crucible session (B6 oracle wave)

## Context

Mutate (CR 702.140, 702.140b-d) merges a creature spell with a non-Human creature its owner controls. The result is one
permanent whose characteristics are the top component's and whose abilities are those of every component. The corpus has
34 `K:Mutate:<cost>` cards (28 distinct costs), 32 `Mode$ Mutates` triggers (31 `ValidCard$ Card.Self`), and several
`Count$TimesMutated` readers. The engine has no merged permanent: `ZoneType.Merged` exists but nothing fills it
(`subgameeffect.go` scans it empty), and `saspec.go` lists `Mutate` as unevaluated.

Java models it with a `mergedCards` list on the target (`Card.java:114`, top at index 0), a `mergedTo` back pointer on
each other component, a Layer 1 clone state built by `CardFactory.getMutatedCloneStates` (`CardFactory.java:757-783`:
the top card's state plus `addAbilitiesFrom` every other component), and `MutateEffect` (`MutateEffect.java:24-101`)
that asks the controller which card is on top (`chooseSingleEntityForEffect`), moves the spell card to `Merged`, and
fires `Mutates` once for the target. Leaving the battlefield splits the list (`GameAction.java:400-528`): every
component goes to the destination, ordered by its owner (the cause's controller for exile), each with its own last-known
copy; `Origin$ Battlefield` triggers fire once for the whole list, others once per component
(`TriggerHandler.java:445-466`). A mutate spell whose target is illegal resolves as a plain creature spell
(`MagicStack.java:628-633`, CR 702.140b).

## Decision Drivers

- GO-9: identity by `CardID`, never pointer.
- The Layer 1 `copies` slice already lets every later layer fold over `Def` unaware (`cloneeffect.go`).
- PORT-2: no script text at run time; the merged definition is built from compiled ASTs.
- Fail closed (GO-7) for every interaction not modelled.

## Considered Options

1. A list of component `CardID`s on the target, plus a Layer 1 overlay (`copyEffect`) whose `def` is synthesised from
   the components.
2. A separate `MergedPermanent` object in the arena with its own ID.
3. Treat the mutate spell card as an Aura-like attachment.

## Decision

Option 1.

**Identity.** The target keeps its `CardID` and is the permanent for ids held by combat, auras, counters and triggers.
`Card.merged []CardID` is top first (the target itself is an element). The spell card moves to `Merged`, owner's zone,
with `mergedTo = target`.

**Characteristics.** One `copyEffect` on the target (kind `mutate`) holds a synthesised `*compile.Card`: the top
component's definition with the ability, trigger, static, replacement and keyword slices of every other component
appended (`CardFactory.java:768-773`). The slices are shared AST nodes; nothing is parsed. The overlay is rebuilt, not
patched, when the list changes. `IsToken` is read from the top component (`Card.java:1066`).

**Choice.** Over or under is `ChooseCardsForEffect(host, [spell, target], 1, 1)`: the existing decision, no new
`PlayerController` method. It is asked even when forced, as in Java.

**Casting.** The keyword expands to a spell with the mutate alternative cost and
`ValidTgts$ Creature.sharesOwnerWith+nonHuman`. When the target is gone or illegal at resolution the card resolves as a
creature spell (`MagicStack.java:628-633`). `Mutates` fires once, for the target, after the merge; `Count$TimesMutated`
reads a counter on the target.

**Leaving the battlefield.** `moveByEffect` of a card with `merged` moves every component to the destination, in the
order the owner chooses (`OrderCardsForZone`; the cause's controller for exile), then clears the list. Batch triggers
with `Origin$ Battlefield` see the list once; the others fire per component.

**Fail closed.** A mutate spell refuses, with an error, if the target is face down, stickered, a commander, in a
subgame, or itself a copy; transforming, turning face down, copying or phasing a merged permanent errors rather than
guessing. `Card.java:1541` (the new top after a subgame split) is not logged as a Forge defect until a test reproduces
it (PORT-8).

## Consequences

**Good:** every id that points at the permanent stays valid; combat, auras and counters need no change; the layer pass
is untouched.

**Bad:** the synthesised definition costs an allocation per merge and per un-merge; abilities from components must keep
a stable host id (the target), so a component's own `Card.Self` references resolve to the permanent, as in Java.

**Neutral:** the Merged zone becomes populated; `subgameeffect.go` already scans it.

## Related

ADR-0009, ADR-0017, ADR-0043
