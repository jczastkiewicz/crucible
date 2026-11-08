# ADR-0061 — A Card Carries Its Print, Pinned by the Deck or Chosen by Latest Art

- **Status:** Proposed
- **Date:** 2026-11-08
- **Deciders:** Crucible session (R19b), owner

## Context

`BorderColorBlack`, `BorderColorWhite` and `BorderColorSilver` (`CardProperty.java:87-89`) read `Card.borderColor()`
(`Card.java:6415-6421`): the `Border=` key of the edition file of the card's set code, Black when absent or for a basic
land (`CardEdition.java:294, 889`). The set code is the chosen print's (`CardFactory.java:178`). 26 of 685 edition files
set it (17 White, 7 Silver, 1 Black). Five corpus lines read it, in three cards: Border Guardian (Black, White, Silver),
Ass Whuppin' and Spatula of the Ages (Silver).

Java picks the print once, at deck load: a `Name|SET|art` line pins it (`DeckRecognizer.java:757-775`,
`CardDb.java:1004-1011`); otherwise `CardDb.java:1021-1024` takes the default art preference, `LATEST_ART_ALL_EDITIONS`
(`ForgePreferences.java:82`, ranked by edition date then collector number, `CardDb.java:838-848`). Game setup has no
random art. `set<CODE>` is a different rule: the name's oldest print, whatever was chosen
(`CardProperty.java:1960-1968`), which `valid.go` already reads through `boosters.OriginalEdition`.

Go keeps `deck.Entry.Edition` and the art index (`deck/parse.go:109-110`) and drops them before the game: `engine.Card`
has no print. `BorderColorBlack`/`White` error at `checkTypeWord`; `BorderColorSilver` returns false with a comment that
the database has no border data, which is wrong (the edition files have it).

## Decision Drivers

- Parity with Java on every deck, pinned or not (L2 and replay runs compare post-states, ADR-0010).
- PORT-2 and GO-2: print data compiled once and immutable, reached through the game's injected tables.
- Three cards, none in the Modern gauntlet: the cost must stay off the other 33,000.

## Considered Options

1. **Print on the card.** `Card.Print{Edition, Art}` set at seating from the deck entry; an unpinned entry takes the
   latest-art rule `boosters/pool.go`'s `fromEditions` already reproduces. `Border=` is compiled with the edition table
   (ADR-0049). Tokens, face-down cards and cards with no print read Black, as Java's null edition does.
2. **Unknown print.** Every `BorderColor*` property never matches. Rejected: Border Guardian's Black clause (and every
   ordinary black-bordered card under it) then differs from Java on every deck.
3. **Edition from the deck only.** Pinned entries carry an edition, unpinned ones none. Rejected: Java resolves the
   unpinned case to a definite print; Go answering "none" differs on the default deck, the common one.

## Decision

Option 1. `BorderColorSilver` reads the same field; its false return and comment are removed.

## Consequences

**Good:** the three cards match Java on pinned and unpinned decks, and a later print-dependent property (frame,
collector number) has a place to read from.

**Bad:** seating needs the edition table, which `card-smoke` and `run` load already but bare engine tests do not; a game
built without it seats every card with no print, so it reads Black, a documented deviation for those tests only.

**Neutral:** the rule for `set<CODE>` (oldest print) stays separate from the card's own print, as in Java.

## Related

ADR-0010, ADR-0049, open plan R19b, `porting/port-log/quirks.md`
