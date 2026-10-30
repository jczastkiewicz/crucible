# Booster generation

Ported from `forge-core` for Booster Tutor (ADR-0049). The reference is `crucible/testdata/boosters.golden`, written by `crucible/oracle-java/dump-boosters.sh` from Forge's own code: two JVMs give identical output, so the order is deterministic.

## State

| Layer | State |
| --- | --- |
| Edition files (`edition.go`) | done: metadata, card sections sorted as `EditionEntry.compareTo`, BoosterSlots, custom sections |
| Card pool (`pool.go`) | done: `CardDb.initialize` order, art numbering per edition, variant rules in their own pool, flavor names, `getCard` with the latest-art preference |
| Print sheets (`sheet.go`, `jorder.go`, `makesheet.go`) | done: `ConcurrentHashMap` order emulated, 810 of 810 oracle sheets equal |
| Generator, effect | not yet: `Booster$` stays refused |

## Quirks reproduced

| Quirk | Java |
| --- | --- |
| A sheet iterates in `ConcurrentHashMap` order: bins by `PaperCard.hashCode`, `transfer` reverses the nodes before the last run, `tryPresize` doubles a small table up to three times when a bin reaches eight | `ItemPool.java:115`, `PaperCard.java:303` |
| `cardsInSet` is sorted by name without case, sortable collector number, rarity; the section order of a `HashMap` only breaks ties | `CardEdition.java:337-338` |
| Printing numbers (`artIndex`) count entries of one name in `cardsInSet` order across all sections | `CardDb.java:446-452` |
| A name an edition lists that no script has becomes a placeholder print with the edition's rarity for it, so only an edition's own rarity can pass it through a sheet filter | `CardDb.java:1474-1506` |
| Variant rules (Vanguard, Scheme, Plane, Phenomenon, Conspiracy, Dungeon, Attraction, Contraption, Stickers) are in a second database a print-sheet line falls back to | `CardRules.java:152`, `CardPool.java:76-97` |

## Not ported

A bin of eight entries in a table of 64 or more would be treeified in Java; `Sheet.add` returns an error there (The List's `PLST cards`). A print-sheet request for a name that exists in no edition of its set needs the card-art preference of other settings than `LATEST_ART_ALL_EDITIONS`.
