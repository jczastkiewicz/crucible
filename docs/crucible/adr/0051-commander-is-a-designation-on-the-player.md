# ADR-0051 — A Commander Is a Designation on the Player, Not the Commander Format

- **Status:** Proposed
- **Date:** 2026-10-30
- **Deciders:** Crucible session (You're in Command)

## Context

You're in Command (`AlterAttribute | Attributes$ Commander`, `IncludeAllComponentCards$ True`) makes a creature its
controller's commander and demotes the others. Java does it through `Player.addCommander` / `removeCommander`
(`Player.java:2838-2854`), `Card.setCommander` / `isCommander` (`Card.java:7445-7460`) and the `Commander` branch of
`AlterAttributeEffect.java:116-134`. `addCommander` also builds the Commander Effect card in the Command zone with the
903.9b replacement and a cast permission (`Player.java:3246-3300`). Commander tax (`CostAdjustment.java:57-61`), cast
counts (`MagicStack.java:385-387`), commander damage (`Player.java:689-697`) and deck setup (`Player.java:3008-3016`)
are the Commander format. Two corpus files use `Attributes$ Commander`; the gauntlet decks are 60-card constructed.
Today `Card.isCommander` is a stub returning false (`card.go`), so `IsCommander` valid strings and Freerunning's
commander clause read false.

## Decision Drivers

- One real card needs the designation; no corpus format needs the rest.
- PORT-7: reproduce Java where parity depends on it; PORT-8: never a Go workaround for a Forge bug.
- GO-12: the commander list is ordered; GO-9: identity by `CardID`.

## Considered Options

1. The designation only: per-player ordered commander list, per-card flag, the `AlterAttribute` branch, `IsCommander`.
2. The full format: command-zone effect, replacement, tax, cast count, commander damage, deck setup.
3. Keep the card refused.

## Decision

Option 1. `Player.commanders` is an ordered `[]CardID`; `Card.commander` mirrors it (Java keeps both in step).
`addCommander` / `removeCommander` change the pair and nothing else. The status survives zone changes and is keyed by
`CardID`; it is cleared only by `removeCommander`. A copy does not inherit it here (no copy path reads it yet).
`AlterAttribute` Commander skips a card whose flag already equals `Activate$`; `IncludeAllComponentCards$` adds the
merged components of each target (`SpellAbilityEffect.java:304-308`).

Not ported, listed in `game-state.md` under "Not ported yet": the Commander Effect card and its replacement and cast
permission, commander tax, commander cast counts (`commanderCasts` stays 0), commander damage and the 21-damage loss,
color identity, setup from a deck. Without the command-zone effect the tax text on the card is inert, as it is in Java
until the creature is cast from the Command zone.

## Consequences

**Good:** the card plays; `IsCommander` and Freerunning's commander clause answer from real state; no format code for
one card. **Bad:** a game that makes a commander through this card diverges from Java the moment that commander would go
to the Command zone, because the 903.9b replacement is absent. Logged in `quirks.md`. **Neutral:** a later ADR can add
the command-zone effect on top of the same list without changing this contract.

## Related

ADR-0006, ADR-0047, `porting/port-log/game-state.md`, `porting/port-log/quirks.md`
