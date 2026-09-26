# ADR-0027 — CR 608.2b: Every Target Re-Checked at Resolution, Per Entity

- **Status:** Accepted
- **Date:** 2026-09-26
- **Deciders:** `mc@archlab.pl`
- **Supersedes:** ADR-0018 Decision point 3's scope — the fizzle check limited to an Aura's own `Target`. The rest of
  ADR-0018 stands.

## Context

ADR-0018 scoped CR 608.2b to an Aura's single target because nothing could make a chosen target illegal before it
resolved. ADR-0019 (priority) and ADR-0026 (turn driver) removed that premise: a response now resolves above a targeted
spell, and a Bolt whose creature died in response still resolved its sub-abilities (Lightning Helix still gained 3
life).

Java's mechanism is `MagicStack.hasFizzled` (`MagicStack.java:704-752`): each target is checked on its own —
`equalsWithGameTimestamp` (`:716-722`, the object changed zones since targeting) OR `!sa.canTarget(entity, true)`
(`SpellAbility.java:1398-1609`, the whole restriction gauntlet ending in `canBeTargetedBy`). Illegal targets are removed
(`:748-750`); with none left the ability fizzles unless `CantFizzle$` (`:736-740`).

The obvious port — recompute the push-time candidate scan and intersect — was tried under ADR-0018 and broke
`TestRemoveFromGameSpellOnStack`, which targets a spell on the stack through a plain `ValidTgts$ Card` that
`targetCandidates`' battlefield scan never lists.

This port's `Card.Timestamp` cannot stand in for Java's `gameTimestamp`: it is restamped on transform too
(`setstateeffect.go`) for layer order, and a transformed permanent is the same object.

## Decision Drivers

- Parity with `hasFizzled`'s per-target shape and its partial-target stripping.
- A target is never held to a rule it was not chosen under: the re-check must be a subset of the choice-time checks, or
  a legally chosen target fizzles for a reason no diff against Java explains.
- GO-9: identity by ID, so object identity across a zone change needs its own recorded value.

## Considered Options

1. **Recompute candidates and intersect.** Rejected: breaks `TestRemoveFromGameSpellOnStack` (Context).
2. **Per entity, using `Card.Timestamp` as identity.** Rejected: a transform would fizzle a spell targeting the
   transformed permanent.
3. **Per entity, with a separate `zoneStamp`.** Chosen.

## Decision

1. **`Card.zoneStamp`** is Java's `gameTimestamp`: set only as a card enters a zone (`put`/`putFront`,
   `GameAction.java:370`). A control change moves no card and leaves it unchanged.
2. **`PushAbility` records** the `zoneStamp` of every card target of the ability and each Charm mode (`targetStamps`).
   `ChangeTargets` records again for the targets it rewrites.
3. **`targetStillLegal`** checks each target on its own: a card is illegal when its `zoneStamp` differs from the
   recorded one, it is phased out, or it no longer matches the ability's `ValidTgts$`; a player is illegal when they
   left the game or no longer match. Anything else (an ability targeted by `ChangeTargets`) is kept. An ability with no
   recorded stamps (resolved without going on the stack) gets the other checks only.
4. **`dropIllegalTargets`** removes illegal targets from the ability and its modes; the ability fizzles when at least
   one target was chosen and none is left, unless `CantFizzle$`. An Aura's own `Target` keeps `auraTargetStillLegal`.
5. **Checks are limited to what choosing a target checks.** Hexproof, shroud, protection and ward
   (`StaticAbilityCantTarget`) and `canTarget`'s multi-target params (`TargetUnique`, `SameController`, ...) are checked
   at neither point. They are one gap, closed at both points together.

## Consequences

**Good:** a response can counter a spell by removing its target, as in Java; partial targets resolve for the legal ones
only; a fizzled spell's sub-abilities do not run.

**Bad:** `Card` carries two timestamps with different lifetimes; a reader must know which one identity uses. Java's
per-level `fizzle` accumulator across targeted sub-abilities (`:707-710`, `:743-744`) is not reproduced, since this port
does not target a `SubAbility$` separately.

**Neutral:** no new event; `AbilityResolved` is simply not emitted for a fizzled ability, as before.

## Related

ADR-0018 (the Aura-only scope this supersedes), ADR-0019, ADR-0026, ADR-0009 (identity by ID),
[04-adr-process](../guidelines/04-adr-process.md)
