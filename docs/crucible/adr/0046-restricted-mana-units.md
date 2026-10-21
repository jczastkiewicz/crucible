# ADR-0046 — Restricted Mana Units Carry Their Spending Restriction

- **Status:** Accepted
- **Date:** 2026-10-21
- **Deciders:** Crucible session (M6 B1-B3)

## Context

`RestrictValid$` on a mana ability limits what the mana it makes can pay for: "Spend this mana only to cast creature
spells" (`Spell.Creature`, 19 corpus lines), instants and sorceries (17), "artifact spells or to activate abilities of
artifacts" (`Spell.Artifact,Activated.Artifact+inZoneBattlefield`, 8), any activated ability (`Activated`, 6), plus
`CantCastNonArtifactSpells`, `CumulativeUpkeep`, `CostContainsX`, `CantCastSpellFromHand`. 191 card files name it; 14
cards refuse in the smoke probe today (`Mana: RestrictValid$ not resolvable`), and every `A:AB$ Mana` line that names it
is refused by `manaAbilityAllowedParams` before its other params are read.

`Pool` (`internal/engine/mana.go`, ADR-0042) holds twelve counters plus an ordered `units` list of `{bucket, source}`.
Payment (`payWithSnow`) works on the counters alone and knows nothing of what is being paid for. Java's `Mana` carries
its `AbilityManaPart`, and `ManaPool.tryPayCostWithColor` skips a unit unless `Mana.meetsManaRestrictions(saPaidFor)`
(`ManaPool.java:228-250`, `AbilityManaPart.java:365-453`). The check calls `SpellAbility.isValid(restriction, ...)`,
which Crucible already ports as `stackItemMatches` (`targeting.go`).

## Decision Drivers

- GO-12: the order units were added stays the order they pay.
- `TestCloneAllocationsStayBounded`: a pool without restricted mana clones and pays with no extra allocation.
- Existing payment rules (plain before snow, `ChoosePayGeneric`, `payWithSnow` all-or-nothing) stay as tested.
- GO-7: a restriction this port cannot judge must refuse the card, not be treated as met or unmet.
- No new `PlayerController` method: which mana pays stays the engine's rule.

## Considered Options

1. **Tag each unit with its restriction string; pay from a view of the usable units.** `poolUnit.restrict string`. A
   payment builds a view pool of the units the paid-for object may use, pays there with the unchanged algorithm, then
   removes the spent units from the real pool. The counters keep counting restricted mana, so `Total`, `Breakdown` and
   `Empty` stay right.
2. A second pool of restricted units beside the counters. Rejected: every reader of "what is floating" (`Total`,
   `Breakdown`, fixture dump, mana burn checks) would need to add both.
3. Pass the paid-for object into `Pool.payWithSnow` and test each unit inside its bucket walk. Rejected: the walk is
   counter arithmetic, not a unit walk; restricted units would need a second code path through every case of it.
4. Ask the controller which unit pays. Rejected: Forge's `chooseManaFromPool` is a UI choice; the engine rule below
   (restricted first) is what an optimal player does.

## Decision

Option 1.

**Production.** `producedMana` carries `restrict string`. `ActivateManaAbility`, a resolving `Mana` and `ManaReflected`
copy `RestrictValid$` onto every unit they add. `manaRestrictionSupported` checks the string when the mana is made and
returns an error for any alternative it cannot judge (`wasCastFrom*`, `hasXCost`, `cmcGE*`, `CantPayGenericCosts`, an
unknown property); the card stays refused instead of making mana that pays wrongly (GO-7).

**Payment context.** `Game.payFor` names what a payment is for: the spell being cast (`castspell.go`), the ability being
activated (`activateability.go`, `activatemanaability.go`), an unless-cost of a resolving ability (`unlesscost.go`), or
a special action (foretell, unlock a door), which names itself in `payContext.special` for `Static.Foretelling` and
`Static.Unlock`. It holds an `*Ability` (kind, host card, controller) and the mana cost being paid. A payment with no
context (fixtures, foretell, room unlock) may use no restricted unit.

**Judging a unit.** `restrictionMet` follows `AbilityManaPart.meetsManaRestrictions` alternative by alternative:
`nonSpell`, `CostContainsX`, `CantCastSpellFromHand`, `CantCastNonArtifactSpells` answer as Java does; a payment for a
resolving ability meets none of the others (`AbilityManaPart.java:443-445`); the rest go through `stackItemMatches` with
the mana's source card as the source and its controller as the controller, the arguments Java passes.

**Paying.** A payment that finds no restricted unit runs the existing code. Otherwise it pays from a view holding only
the usable units, restricted ones first within a bucket (a player spends mana that would otherwise be wasted), then
deletes one equal unit per spent unit from the real pool. The view pays with the same algorithm, so snow, hybrid and
generic rules are untouched.

**Draining.** `merge` and `clone` copy units with their restriction. `Empty` drops them with the rest.

## Consequences

**Good:** the 14 refused cards and every restricted `A:AB$ Mana` line whose restriction is supported play; unsupported
restrictions keep refusing at the point the mana is made. A pool without restricted mana pays and clones exactly as
before.

**Bad:** a payment with restricted mana allocates a view. Java spends the first qualifying unit in add order; Crucible
spends restricted units before plain ones within a bucket, so a spell paid from mixed units can attribute a different
source than Java would (`quirks.md`). Restrictions beyond the supported set refuse the card; the set grows by adding
cases to `stackItemHasProperty`, not by changing this design.

**Neutral:** fixtures carry no restricted mana (the `GameState` format has no field for it).

## Related

ADR-0042, ADR-0009, ADR-0005
