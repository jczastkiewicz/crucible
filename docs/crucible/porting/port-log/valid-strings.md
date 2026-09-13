# Port: Valid Strings

- **Java source:** `forge-game/src/main/java/forge/game/card/CardProperty.java` (2,135, the numeric-comparison branch
  ported), `forge/util/Expressions.java`, and the callers that split the param — `CardTraitBase.java:261`,
  `CardLists.java:189`
- **Go target:** `crucible/internal/valid`
- **Status:** Parsing done — M3 slice C. Evaluation started in `internal/engine` (M5): `Matches` covers the
  `Card.isValid` control flow in full and three property names (`YouCtrl`, `OppCtrl`, `Self`) plus the bare
  type/supertype/subtype fallthrough every property chain shares. The other ~925 property names are M5-M6, corpus
  frequency order

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

## Evaluation lands in `internal/engine`

`engine.Matches` (`valid.go`) is `Card.isValid` plus the one branch of `Card.hasProperty` that matters for negation, not
`CardProperty.cardHasProperty` itself yet — that 2,135-line switch is corpus-frequency work, same as `effect.go`'s
`Registry` (ADR-0011), and starts with three names: `YouCtrl`, `OppCtrl`, `Self`. A fourth case, a bare
type/supertype/subtype word used as either a `Base` or a `Property`, reaches
[`cardtype.Line.HasStringType`](../../../crucible/internal/cardtype/cardtype.go) — the Go port of
`CardType.hasStringType`, which both `Card.isValid`'s own default case and every property chain's final fallthrough call
in Java.

**A `!` on the base negates the whole alternative, not just the base.** `Card.isValid` reads as an AND-chain — the base,
then every property in order — where any failing check returns early with a shared `testFailed` flag set once from the
base's own leading `!`; only if everything passes does it return `!testFailed`. `!Creature.YouCtrl` is therefore "not (a
creature you control)" — true for an opponent's creature and for any non-creature you control alike — not "a
non-creature you control". `valid.Base.Negated` already carried the flag; `altMatches` is what reproduces the
short-circuit. A `!` on a property is the simple case: `Card.hasProperty`'s own wrapper just inverts `cardHasProperty`'s
result for that one property (`valid.Property.Negated`), independent of the base's flag.

**`YouCtrl`/`OppCtrl` read `Card.Controller` directly, not Java's LKI-derived controller.**
`CardProperty.cardHasProperty` compares against `game.getChangeZoneLKIInfo(card).getController()`, last-known
information for a card whose own zone change might be mid-resolution. This port has no LKI tracking (`game-state.md`'s
"Not ported yet"), so it reads the current `Controller` — correct except for the one moment a card's own leaving is what
a property is trying to describe, the same category of gap `Move`'s missing LKI already is.

**`OppCtrl` has no team system to be aware of.** Java's is `controller.getOpponents().contains(sourceController)`; this
port reads it as "controlled by anyone other than `sourceController`" — right for every game this port can play today
(two players, or free-for-all with no teams), wrong only once a team variant exists to disagree with it.

`Matches` takes `sourceController PlayerID, source CardID` rather than a `*Game` or an `*Ability`: nothing it currently
does needs the game, and tying it to `Ability` specifically would assume every valid-string check happens during ability
resolution, which CR 704.5f/704.5m's still-unbuilt `Enchant`-restriction check will not (its source is the Aura itself,
not anything on a stack).

## Deviations from Java

| Java                                                                   | Go                                                                                                                        |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| The property string is re-parsed on every evaluation                   | Parsed once at load into `Spec` (ADR-0007)                                                                                |
| `!` is consumed by mutating the local `incR[0]`                        | `Negated bool` on both `Base` and `Property`, so the sign is not part of the name                                         |
| A comparison is recognised by a chain of `startsWith` in the evaluator | `Compare` on the property, filled at parse time                                                                           |
| Matching happens against a `Card` and a `Game`                         | Not here (ADR-0003 stays honoured): `engine.Matches` evaluates a `Spec`, `internal/valid` never imports `internal/engine` |

## Not ported yet

| Java                                                                                                                 | When       |
| -------------------------------------------------------------------------------------------------------------------- | ---------- |
| `CardProperty.cardHasProperty` — 311 of its 314 branches; `YouCtrl`, `OppCtrl`, `Self` are ported (`engine.Matches`) | M5-M6      |
| `CardStateProperty`, `SpellAbilityProperty` — the other two of the four property chains                              | M5-M6      |
| `PlayerProperty.playerHasProperty` (517) — no `Base`/`Property` this port evaluates targets a `Player` yet           | M5-M6      |
| LKI-aware `YouCtrl`/`OppCtrl`, and a team-aware `OppCtrl`                                                            | M5-M6      |
| Property heads as a closed vocabulary, for the P2 gate                                                               | M3 slice H |

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
| `equals` and `startsWith` branches in the **four** property chains |                                   62 |
| plus subtypes, core types, supertypes, colours                     |                                    6 |
| plus the 203 keyword names                                         |                                    1 |

Three things the scrape has to get right, each of which produces false findings on its own:

- **Four chains.** `CardProperty`, `CardStateProperty`, `PlayerProperty`, `SpellAbilityProperty`.
- **`equals` and `startsWith` are not interchangeable.** `startsWith("AttachedTo")` accepts
  `AttachedTo Creature.YouCtrl`; read as exact it rejects 400 properties Forge handles.
- **`restriction` is a second receiver name**, for 12 of the branches.

One deliberate exclusion, and it is not a property vocabulary at all. `ManaReflected` reads its own `Valid$` form:
`CardUtil.java:261` tests `validCard.startsWith("Defined.")` and treats the rest as a defined name, so
`Defined.Sacrificed` never reaches `CardProperty`. Nine cards use it.

`TestEveryPropertyIsAccountedFor` holds at zero. The one defect it found is in
[card-script-defects.md](../card-script-defects.md).
