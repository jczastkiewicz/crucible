# Port: Valid Strings

- **Java source:** `forge-game/src/main/java/forge/game/card/CardProperty.java` (2,135, the numeric-comparison branch
  ported), `forge/util/Expressions.java`, and the callers that split the param — `CardTraitBase.java:261`,
  `CardLists.java:189`
- **Go target:** `crucible/internal/valid`
- **Status:** Parsing done — M3 slice C. Evaluation needs a game and lands with the engine

## What it does

Turns `Creature.Green+attacking,Land.YouCtrl` into alternatives, each a base with properties. `,` is OR, `+` is AND, and
the properties do not distribute: `Instant.YouCtrl,Sorcery` is "(an instant you control) or (any sorcery)".

49,615 valid strings in the corpus, under every valid-typed param key and inside `Count$Valid` heads.

## Nothing is trimmed, on purpose

`CardTraitBase` splits the param with a plain `split(",")` and `Card.isValid` splits the alternative with
`split("\\.", 2)`. Neither trims, so `ValidTgts$ Player, Planeswalker` really does produce an alternative whose base is
`" Planeswalker"` — and `CardType.hasStringType` rejects it, because no subtype carries a leading space,
`StringUtils.capitalize` leaves one alone, and the enum lookups are exact.

Reproducing that exactly is what found the defect on two cards
([`../card-script-defects.md`](../card-script-defects.md)). The vocabulary scanner, which trims because it is only
counting, could not see it. **A parser that tidies its input cannot find the bugs that tidying would fix.**

The corpus test therefore checks two things: every valid string writes back byte for byte, and no base is padded with
space.

## Numeric comparisons

`powerGE1` is a field, an operator and an operand in one token, and the split is not derivable from the field name. Java
hardcodes an offset per field:

| Field           | Written            | Operand offset |
| --------------- | ------------------ | -------------: |
| `power`         | `powerGE1`         |              7 |
| `basePower`     | `basePowerEQ2`     |             11 |
| `toughness`     | `toughnessLE3`     |             11 |
| `baseToughness` | `baseToughnessEQ1` |             15 |
| `cmc`           | `cmcGE7`           |              5 |
| `totalPT`       | `totalPT_GE5`      |             10 |
| `numColors`     | `numColorsEQ2`     |             11 |
| `numTypes`      | `numTypesGE3`      |             10 |

`totalPT` is the one that looks wrong and is not: the corpus writes it with a trailing underscore, so its operand starts
two characters after the field name rather than none.

The operator is found by **containment**, not position: `Expressions.compare` takes the whole property string and asks
whether it contains `LT`, then `LE`, `EQ`, `GE`, `GT`, `NE`, `M2`, in that order. The order is the rule for a token that
could match twice, so it is reproduced rather than tidied into a lookup.

`M2` is modulo 2, and the corpus uses it — `cmcM20` and `cmcM21`, Gyruda and Obosh. `NE` is defined by Java and used by
no card.

Operands stay text. A number, `X`, `Chosen`, an SVar name, or a whole `Count$` expression appears there, and resolving
any of them needs a game.

## Deviations from Java

| Java                                                                   | Go                                                                                |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| The property string is re-parsed on every evaluation                   | Parsed once at load into `Spec` (ADR-0007)                                        |
| `!` is consumed by mutating the local `incR[0]`                        | `Negated bool` on both `Base` and `Property`, so the sign is not part of the name |
| A comparison is recognised by a chain of `startsWith` in the evaluator | `Compare` on the property, filled at parse time                                   |
| Matching happens against a `Card` and a `Game`                         | Not here. `valid` imports the engine when it evaluates, which is M5 (ADR-0003)    |

## Not ported yet

| Java                                                              | When       |
| ----------------------------------------------------------------- | ---------- |
| `CardProperty.cardHasProperty` — the 2,135-line evaluation switch | M5         |
| `PlayerProperty.playerHasProperty` (517)                          | M5         |
| Property heads as a closed vocabulary, for the P2 gate            | M3 slice H |

The 1,256 distinct property tokens are inventoried in `internal/carddb/vocab`'s golden. Classifying them into families
belongs with the evaluator that implements them, not with the parser.

## Gating the property vocabulary

`valid.Parse` is total: it splits a string into a base and properties and never rejects one, because Java's
`CardProperty` never rejects one either — it walks a 297-branch chain and returns false at the end. So a property the
port does not implement is indistinguishable from one that simply does not match, which is the hole M3 item 19 names.

The accepted vocabulary is not a list anywhere. `CardProperty` tests its explicit branches first and then falls through
to type and keyword checks, so acceptance is the union of four sources. Measured against the corpus's 1,256 distinct
properties:

| Source                                                                | Residual unmatched |
| --------------------------------------------------------------------- | -----------------: |
| `property.equals` / `startsWith` in `CardProperty` + `PlayerProperty` |          342 (27%) |
| plus subtypes from `TypeLists.txt`, and colours                       |           117 (9%) |
| plus core types, supertypes, and the 203 keyword names                |        expected ~0 |

The remaining 117 are exactly what the third row predicts: `Artifact`, `Creature` and `Enchantment` are `CoreType`
constants rather than `TypeLists.txt` entries; `Backup`, `Bestow`, `Blitz`, `Crew`, `Cycling`, `Dash`, `Embalm`,
`Equip`, `Flashback` and the rest are keyword names reaching `hasKeyword(property)`; `BlackSource`, `BlueSource` and
`ColorlessSource` are a colour family.

**The gate is therefore buildable from packages that already exist** — `internal/cardtype` for the three type sources
and `internal/keyword` for the fourth — and it has to reproduce `CardProperty`'s fallthrough order rather than test a
flat set, or a property that is both a keyword and an explicit branch would be attributed to the wrong one.

Not built yet. The scrape is the same shape as `tools/apiscan`'s, and that one needed six passes before its blind spots
stopped producing false findings; this one should be measured to zero residual before it blocks a build.
