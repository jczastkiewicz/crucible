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

`valid.Parse` is total: it splits a string into a base and properties and never rejects one, because Java never rejects
one either. The chain ends at `CardProperty.java:2116-2119`:

```java
} else if (!card.getCurrentState().hasProperty(property, sourceController, source, spellAbility)) {
    return false;
}
return true;
```

So an unimplemented property is not ignored — it is **false for every card**, and the alternative carrying it matches
nothing. That makes an unaccounted property a silent targeting failure, which is why it needs a gate rather than a
review habit.

The accepted vocabulary is not a list anywhere. Acceptance is a union, and it has to mirror Java's fallthrough order
rather than test a flat set, because a name can be reachable two ways:

| Source                                                             | Residual, of 928 distinct properties |
| ------------------------------------------------------------------ | -----------------------------------: |
| `equals` and `startsWith` branches in the **four** property chains |                             342 → 62 |
| plus subtypes, core types, supertypes, colours                     |                                    6 |
| plus the 203 keyword names                                         |                                    1 |

Three corrections the measurement forced, each of which had been producing false findings:

- **Four chains, not two.** `CardStateProperty` and `SpellAbilityProperty` hold the rest; the grammar doc named only
  `CardProperty` and `PlayerProperty`.
- **`equals` and `startsWith` are not interchangeable.** `startsWith("AttachedTo")` accepts
  `AttachedTo Creature.YouCtrl`; treating it as exact rejected 400 properties Forge handles.
- **`restriction` is a second receiver name** for 12 of the branches.

One deliberate exclusion, and it is not a property vocabulary at all. `ManaReflected` reads its own `Valid$` form:
`CardUtil.java:261` tests `validCard.startsWith("Defined.")` and treats the rest as a defined name, so
`Defined.Sacrificed` never reaches `CardProperty`. Nine cards use it.

The last residual was a real defect rather than a scan gap: `oracle` wrote `youCtrl` where every other card in the
corpus writes `YouCtrl`, and the lookup is case-sensitive, so the Vanguard's `{0}` ability had no legal target in any
game state. Fixed upstream and carried.

`TestEveryPropertyIsAccountedFor` now holds at zero.
