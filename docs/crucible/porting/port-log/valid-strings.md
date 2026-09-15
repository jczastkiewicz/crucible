# Port: Valid Strings

- **Java source:** `forge-game/src/main/java/forge/game/card/CardProperty.java` (2,135, the numeric-comparison branch
  ported), `forge/util/Expressions.java`, and the callers that split the param — `CardTraitBase.java:261`,
  `CardLists.java:189`
- **Go target:** `crucible/internal/valid`
- **Status:** Parsing done — M3 slice C. Evaluation started in `internal/engine` (M5): `Matches` covers the
  `Card.isValid` control flow in full, three property names (`YouCtrl`, `OppCtrl`, `Self`), the five colors plus
  `Colorless`/`MultiColor` and the generic `non<Type>` fallback (`CardStateProperty`'s own chain), and the bare
  type/supertype/subtype fallthrough every property chain shares. The other ~915 property names are M5-M6, corpus
  frequency order (`tools/vocabscan -kind validProperty`)

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
resolution, which CR 704.5's still-unbuilt Aura `Enchant`-restriction check will not (its source is the Aura itself, not
anything on a stack — `game-state.md`'s "State-based actions" section has the full citation caveat: Java's own comments
do not cleanly single-letter this rule).

## Color reaches `Matches` through a second chain, `CardStateProperty`

The five colors, `Colorless`/`MultiColor` and the generic `non<Type>` fallback are not in `CardProperty.cardHasProperty`
at all — Java's own switch has no `case "White"`. `CardProperty`'s final `else` calls
`card.getCurrentState().hasProperty(property, ...)`, `CardStateProperty.hasProperty`, the second of the four property
chains (`CardProperty`, `CardStateProperty`, `PlayerProperty`, `SpellAbilityProperty`), and that is where color lives —
confirmed by grepping `CardProperty.java` for a bare color-name case and finding none, then finding all of them in
`CardStateProperty.java` instead. `colorMatches` (`valid.go`) is that chain's own color branch, ported as a small
function of its own rather than folded into `propertyMatches`'s switch, since it has real internal structure (a shared
`mustHave`/`non`-prefix computation across all five names) `propertyMatches`'s existing three cases do not.

**Card color needed a real source: `Card.Colors()`, from `compile.Face`'s new `ManaCost`/`Colors`/`HasColors`.** Neither
existed before this — `compile.Face` carried Type/Power/Toughness/Loyalty/Defense/Keywords through from `carddb.Face`
but not the mana cost or a color override, because nothing before `colorMatches` needed a card's color at all.
`Card.Colors()` is exactly `carddb.Face.dumpColors()`'s own "override, else derive from cost" logic — not a new
derivation invented for this, but a second caller of one M2's P1 gate already verified byte-identical to Forge's own
dump across the whole corpus. `internal/mana.Colors`, the bitmask type both share, already existed too
(`internal/mana`'s own doc comment, ported from `MagicColor`/`ColorSet` for M1's mana-cost work) — this is its first use
outside cost parsing.

**`colorMatches` exact-matches rather than reproducing Java's `Contains`/prefix-stripped form, on purpose.** Java folds
a `Source` suffix into the same branch (`WhiteSource`, a damage-context check comparing the color of whatever _dealt_
damage, not the candidate card `Matches` is given) by slicing the trailing characters off before the
`MagicColor.fromName` lookup. `Matches` has no damage-source context to answer that question with, so implementing the
slice without the context it exists for would either panic on an unexpected shape or (worse) silently answer the wrong
question. An exact match against `White`/`Blue`/.../`nonGreen` means `WhiteSource` simply does not match `colorMatches`
at all and falls through to `propertyMatches`'s own type-name fallthrough — false for every card, the honest "not
implemented" answer, rather than a plausible-looking wrong one.

**The generic `non<Type>` fallback is `CardStateProperty`'s own tail, reached only after color already had first
refusal.** `nonBlack` and `nonLand` look identical in shape but mean different things — one is a color negation
(`colorMatches` claims it), the other is a type negation (`c.Type().HasStringType("Land")`, inverted). Checking color
first is what keeps `nonBlack` from ever reaching the type fallback and being asked whether "Black" is a recognized
subtype (it is not, so the type fallback would answer `true` for every card — the exact wrong-answer shape colorMatches
existing to intercept color names avoids).

## Deviations from Java

| Java                                                                   | Go                                                                                                                        |
| ---------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| The property string is re-parsed on every evaluation                   | Parsed once at load into `Spec` (ADR-0007)                                                                                |
| `!` is consumed by mutating the local `incR[0]`                        | `Negated bool` on both `Base` and `Property`, so the sign is not part of the name                                         |
| A comparison is recognised by a chain of `startsWith` in the evaluator | `Compare` on the property, filled at parse time                                                                           |
| Matching happens against a `Card` and a `Game`                         | Not here (ADR-0003 stays honoured): `engine.Matches` evaluates a `Spec`, `internal/valid` never imports `internal/engine` |
| Color is read via `card.getColor(cardState)`, LKI/state-aware          | `Card.Colors()`, current state only — the same gap every other characteristic accessor on `Card` already has              |

## Not ported yet

| Java                                                                                                                                                                                                                                                                                                   | When       |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------- |
| `CardProperty.cardHasProperty` — 311 of its 314 branches; `YouCtrl`, `OppCtrl`, `Self` are ported (`engine.Matches`)                                                                                                                                                                                   | M5-M6      |
| `CardStateProperty.hasProperty` — the rest of it: `AllColors`, `MonoColor`, `ChosenColor`/`AnyChosenColor`, `EnemyColor`, `AssociatedWithChosenColor`, `Worthy`/`Outlaw`/`Party`, `HasSVar`, and everything past it (color, `Colorless`, `MultiColor` and the generic `non<Type>` fallback are ported) | M5-M6      |
| `SpellAbilityProperty` — the fourth property chain, untouched                                                                                                                                                                                                                                          | M5-M6      |
| `PlayerProperty.playerHasProperty` (517) — no `Base`/`Property` this port evaluates targets a `Player` yet                                                                                                                                                                                             | M5-M6      |
| LKI-aware `YouCtrl`/`OppCtrl`, and a team-aware `OppCtrl`                                                                                                                                                                                                                                              | M5-M6      |
| Property heads as a closed vocabulary, for the P2 gate                                                                                                                                                                                                                                                 | M3 slice H |

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
