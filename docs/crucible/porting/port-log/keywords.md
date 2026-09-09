# Port: Keywords

- **Java source:** `forge-game/src/main/java/forge/game/keyword/Keyword.java` (354, its enum and `getKeywordDetails`),
  the 27 `KeywordInstance` subclasses beside it
- **Go target:** `crucible/internal/keyword`
- **Status:** Parsing and the definition table done — M3 slice F. Expansion into triggers, statics and abilities needs
  the effect layer

## What it does

Reads a `K:` line into a head, its details, and the enum entry that says how the details are shaped. `Flying` carries
nothing, `Ward:2` an amount, `Dash:4 R W` a cost, `Awaken:3:4 U` both.

18,248 `K:` lines in the corpus.

## The head is not "everything before the first colon"

`Keyword.getKeywordDetails` has three cases, and the middle one is the reason:

| Line                      | Case                                               |
| ------------------------- | -------------------------------------------------- |
| `Ward:2`                  | A `:` splits head from details at the first colon  |
| `First strike`            | No colon, so the **whole line** is tried as a name |
| `Bands with other Dragon` | The whole line fails, so the first word is tried   |
| `Flying`                  | No colon, no space: the line is the name           |

Cutting at the first colon gets the first and last right and the middle two wrong. `smartValueOf` is an exact
case-insensitive match on the display name, so the space case is not a prefix rule.

A `:Flavor` suffix, with the space that follows it, is cut from the details before they are read. It is display text,
and leaving it in would make `Ward:2:Flavor Protective Ward` an amount keyword whose amount is not a number.

## What the corpus actually writes

| What                                     | Distinct |   Uses |
| ---------------------------------------- | -------: | -----: |
| Keywords the enum defines                |      199 | 18,062 |
| Pseudo-keywords the card factory handles |       11 |  1,297 |
| Rules text on a `K:` line                |       36 |    144 |

The enum has 203 constants, so four are defined and unused.

The eleven pseudo-keywords are `etbCounter` (475), `ETBReplacement` (405), `Chapter` (236), `Class` (76),
`AlternateAdditionalCost` (42), `MayEffectFromOpeningHand` (30), `Visit` (21), `DeckLimit` (5),
`MayEffectFromOpeningDeck` (4), `MustBeBlockedByAll` (2) and `Prize` (1). They are the vocabulary a compiler owes on top
of the enum, and they are in the golden by name.

The rules-text lines are counted, not listed. `CARDNAME must be blocked if able.` is a keyword only in the sense that
Forge stores it on a `K:` line; it is card text, it would move the golden every time a card is added, and Java resolves
it to `UNDEFINED` and falls back to a simple keyword holding the original string. **An unresolved head is not an error**
— this is the one place in the script language where that is by design.

## The argument shape is the class

The enum names a `KeywordInstance` subclass per keyword, and that class is the contract: `Ward:2` and `Hexproof:White`
are written identically and mean different things. The table records it.

| Shape           | Keywords | Example                        |
| --------------- | -------: | ------------------------------ |
| `Simple`        |       88 | `Flying`                       |
| `Cost`          |       54 | `Dash:4 R W`                   |
| `Amount`        |       27 | `Absorb:1`                     |
| `Special`       |       23 | `Ward`, `Protection`, `Kicker` |
| `Type`          |        5 | `Champion:Elf`                 |
| `CostAndAmount` |        3 | `Awaken:3:4 U`                 |
| `CostAndType`   |        2 |                                |

`Special` is the seventeen classes with a parser of their own plus the six that share one; their details stay text until
the keyword is expanded.

## Deviations from Java

| Java                                                          | Go                                                                                      |
| ------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `getInstance` reflects a class per keyword and parses eagerly | A table row with a `Kind`. No reflection in the engine (GO-4), and the shape is data    |
| An unresolved head silently becomes a `SimpleKeyword`         | `Entry` is nil and `Kind()` is `Unknown`, so a caller can tell text from vocabulary     |
| `KeywordWithAmount.parse` throws on a non-numeric amount      | The corpus test checks it instead: every `Amount` keyword's argument is a number or `X` |

## Not ported yet

| Java                                                              | When                                 |
| ----------------------------------------------------------------- | ------------------------------------ |
| `CardFactoryUtil.setupKeywordedAbilities` — expansion into traits | M3, after the effect registry exists |
| The 23 bespoke `KeywordInstance` parsers                          | M5, with the keywords they implement |
| Reminder text formatting                                          | Never. Display only (PORT-6)         |
