# ADR-0049 — Booster Templates Are Compiled From Edition Files Into an Immutable Table

- **Status:** Accepted
- **Date:** 2026-10-28
- **Deciders:** Crucible session (B9)

## Context

Booster Tutor (`MakeCard | Booster$ True | Zone$ Hand`) is the only corpus line that opens a booster. Java draws one
template from every edition's booster types, builds a pack with `BoosterGenerator`, and offers its cards as the choice
list (`MakeCardEffect.java:81-119, 153-160`, `StaticData.java:557-560`, `Aggregates.java:131-154`,
`BoosterPack.java:55-67`, `BoosterGenerator.java:92-104`). The draw order is the contract: one `nextInt` per template in
a min-scan (strict less-than, TreeMap order of edition code plus affix, case-insensitive), `nextInt` of the picture
count whose result is unused, the foil chance, then every slot pick. `carddb` has no edition or print-sheet data
(`forge-gui/res/editions` has 685 files).

The card's rules are physical: the player opens a sealed booster pack they have not seen, any pack works including ones
illegal in the format, and the chosen card leaves the deck before the next game (Gatherer ruling of 2020-02-29,
[mirror](https://www.mtg.wtf/card/und/33/Booster-Tutor)). Forge approximates that with a random edition template; the
approximation is reproduced because parity depends on it (PORT-7), not because it models the rules. The "remove from
your deck before the next game" clause has no Java implementation and none here.

## Decision Drivers

- GO-12 and ADR-0006: draws must follow Java's order or a replay diverges after the first booster.
- PORT-2 and GO-2: compile once, immutable, no package-level state; reached through `*carddb.DB`.
- One card needs it. The cost must not leak into the 33,000 cards that do not.

## Considered Options

1. Compile editions at load into `DB.Boosters`, a table of templates and print sheets; port `BoosterGenerator`.
2. Keep Booster Tutor refused (one card, large data surface).
3. Load edition files lazily on first use.

## Decision

Option 1: the owner wants the card.

**Data.** `carddb` gains `Edition{Code, Affix, BoosterTypes, Sheets, PictureCounts, FoilChance, FoilType}` parsed once
at load by a loader beside the card compiler; the table is ordered as Java's TreeMap (code plus affix, case-folded) in a
`collect.OrderedMap`. Lazy loading (Option 3) is rejected: it makes the DB mutable after load and breaks GO-2.
**Generator.** A `boosters` package ports `BoosterGenerator` with the draw order above, taking `*javarand.Rand` from the
`Game`. **Effect.** `makecardeffect.go` reads `Booster$` only to enter the branch (Java never reads its value, PORT-7),
asks `chooseCardName` over the main-part faces, creates the chosen face and moves it to the hand. **Gate.** A scenario
fixture whose expected pack is produced by the Java oracle for a fixed seed.

## Consequences

**Good:** closes the last data-driven refusal; the generator can serve later draft or sealed work.

**Bad:** a parser and a few MB of immutable data load per process for one card.

**Neutral:** `card-smoke` gains one playable card.

## Related

ADR-0006, ADR-0007, ADR-0011, open plan row B9
