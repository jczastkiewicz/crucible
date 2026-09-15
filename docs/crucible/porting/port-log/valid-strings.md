# Port: Valid Strings

- **Java source:** `forge-game/src/main/java/forge/game/card/CardProperty.java` (2,135, the numeric-comparison branch
  ported), `forge/util/Expressions.java`, and the callers that split the param — `CardTraitBase.java:261`,
  `CardLists.java:189`
- **Go target:** `crucible/internal/valid`
- **Status:** Parsing done — M3 slice C. Evaluation started in `internal/engine` (M5): `Matches` covers the
  `Card.isValid` control flow in full; controller/owner relative to `sourceController` (`YouCtrl`, `YouDontCtrl`,
  `OppCtrl`, `YouOwn`, `YouDontOwn`, `OppOwn`); identity relative to `source` (`Self`, `Other`, `StrictlyOther`); the
  five colors plus `Colorless`/`MultiColor`; a keyword check under three spellings (`with`/`without`/`hasKeyword`);
  `tapped`/`untapped`; the numeric comparisons (`power`, `basePower`, `toughness`, `baseToughness`, `cmc`, `totalPT`,
  `numColors`, `numTypes`, crossed with `LT`/`LE`/`EQ`/`GE`/`GT`/`NE`/`M2`) for a plain-integer operand; the generic
  `non<Type>` fallback (`CardStateProperty`'s own chain); and the bare type/supertype/subtype fallthrough every property
  chain shares. The other ~905 property names are M5-M6, corpus frequency order (`tools/vocabscan -kind validProperty`)
  — a rough figure the numeric-comparison batch does not update, since it collapses many raw tokens (`power` alone spans
  33, per `vocabscan -kind validProperty`) into one mechanism and no earlier count of this file's own "~905" was
  computed at that granularity either

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

## Ownership, identity and keyword properties: back in `CardProperty` proper

The second highest-frequency gap after color (`tools/vocabscan -kind validProperty`) turned out to already be in the
chain `Matches` was reading — `CardProperty.cardHasProperty` itself, just further down than `YouCtrl`/`OppCtrl`/`Self`
reach: `YouDontCtrl`, `YouOwn`/`YouDontOwn`/`OppOwn` sit right next to the three ported names, and
`Other`/`StrictlyOther` sit right next to `Self`. Corpus weight alone justified this batch — `Other` (2,068 occurrences)
and `YouOwn` (1,853) outrank every property but `Self` and `YouCtrl` themselves.

**`YouOwn`/`YouDontOwn`/`OppOwn` are `YouCtrl`/`YouDontCtrl`/`OppCtrl` against `Card.Owner` instead of
`Card.Controller`.** Distinct once something steals control (an Owner and Controller can differ, `## Cloning`'s own
table in game-state.md has the reason both fields exist), otherwise identical in shape and in every LKI/team
simplification `YouCtrl`/`OppCtrl` already carry (this doc's own "Evaluation lands in `internal/engine`" section).

**`Other`/`StrictlyOther` are `Self`'s negation, collapsed to one behavior.** Java's `Strictly` forms
(`equalsWithGameTimestamp`) exist to tell a card from a same-named copy of itself apart across a zone change this port
has no game-timestamp/LKI tracking for (game-state.md's "Not ported yet") — the same gap `YouCtrl`'s own LKI paragraph
already documents, reached from a different direction. `StrictlyOther` therefore reads exactly like `Other`:
`c.ID != source`.

**Keyword properties reach `Card.HasKeyword` under three spellings Forge's own corpus uses interchangeably:
`with<Keyword>`, `without<Keyword>`, `hasKeyword<Keyword>`.** This is a fully generic mechanism, not Flying-specific —
`HasKeyword`'s own exact-match-on-parsed-name behavior (`card.go`'s own doc comment) already handles every keyword the
corpus has a name for, so porting the three prefixes once covers all of them. `without` is checked before the shorter
`with` prefix on purpose: `without` itself starts with the four characters `with`, so checking `with` first and
stripping only four characters would leave `"outFlying"` where `"Flying"` belongs — the same ordering mistake Java's own
nested `if` (`property.startsWith("without") && ...` checked inside the outer `property.startsWith("with")`) avoids by
checking the longer, more specific prefix first.

**`tapped`/`untapped` read `Card.Tapped` directly** — the same battlefield-only field `Game.Move` already clears on
leaving it (game-state.md's "The card's mutable parts").

## Numeric comparisons land in `engine.Matches`, plain-integer operands only

`internal/valid` already split `powerGE1` into `Compare{Field: "power", Operator: "GE", Operand: "1"}` at parse time
(this doc's own "Numeric comparisons" section, M3). `propertyMatches` (`valid.go`) checks `p.Compare` first, before its
name-based switch, and hands off to `compareMatches` — the corpus-frequency-first mechanism this is, rather than eight
one-off branches, is the same shape `effect.go`'s `Registry` grows in (ADR-0011): one field-to-value table
(`compareFieldValue`) and one operator table (`compareOp`, `Expressions.compare` ported operator for operator, including
`M2`'s modulo-2 equality) cover every field/operator combination at once.

**Only a plain base-10 `Operand` is handled.** Java resolves it with `AbilityUtils.calculateAmount`, which also accepts
`X`, `Chosen` (`source.getChosenNumber()`), and an SVar name — none of which this port can resolve without an
ability-context evaluator `internal/expr` does not have yet (`compare.go`'s own doc comment: "resolving any of them
needs a game"). `strconv.Atoi` failing is the signal: the property matches nothing, the same "false for every card"
answer any other unimplemented property gives, not a wrong one — `powerGEX` and `powerGEChosen` are both this gap, not
special cases of it.

**`basePower`/`baseToughness` measure Java's `getCurrentPower`/`getCurrentToughness` (`Card.java:4407,4450`), not
`getBasePower`/`getBaseToughness`.** The names are Java's own trap: `getCurrentPower` is base folded with Layer 7's
`LayerCharacteristic`/`LayerSetPT` sub-layers, counters and the power/toughness-switch keyword excluded, while
`getBasePower` is the printed value alone — this port's own `BasePower`/`BaseToughness` accessors. Reusing `Power`'s own
Layer-7-fold (`foldPT`) rather than reaching for `BasePower` was the fix: `Power`/`Toughness` now split into a new
`layer7Power`/`layer7Toughness` step (base folded with Layer 7, no counters yet) and the counters addition that used to
be inline in one function, so `compareFieldValue`'s `basePower`/`baseToughness` cases read the same intermediate value
Java's `getCurrentPower`/`getCurrentToughness` compute, and `power`/`toughness` (Java's `getNetPower`/`getNetToughness`)
keep reading the full `Power`/`Toughness`, counters included. Neither this port's `Power`/`Toughness` nor `getNetPower`
folds in "CARDNAME's power and toughness are switched" here, so a switched creature's numeric comparisons share the same
gap every other switch-blind read on `Card` already has (game-state.md's "Not ported yet").

**`cmc` needed a new accessor, `Card.CMC()`** — nothing before this read a card's own mana value; `mana.Cost.CMC()`
already existed (M1) and `compile.Face.ManaCost` already existed (this doc's own "Color reaches `Matches`" section, for
the same reason), so `Card.CMC()` is a one-line composition of the two, not a new derivation.

**`totalPT`, `numColors` and `numTypes` have no unresolvable form.** `totalPT` is full `Power`+`Toughness` (both can
still be individually unresolvable, propagated as `ok=false` the same way `BasePower`'s own `ok` does); `numColors` is
`Card.Colors().Count()` (already used by `MultiColor`) and `numTypes` is `len(Card.Type().CoreTypes())`, both always
resolvable since neither a color set nor a type line is ever a printed `"*"`.

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
| `CardProperty.cardHasProperty` — the great majority of its branches; ported so far: `YouCtrl`/`YouDontCtrl`/`OppCtrl`, `YouOwn`/`YouDontOwn`/`OppOwn`, `Self`/`Other`/`StrictlyOther`, `with`/`without`/`hasKeyword`, `tapped`/`untapped`, the numeric comparisons (`engine.Matches`)                  | M5-M6      |
| `AbilityUtils.calculateAmount` for a numeric-comparison `Operand` that is not a plain integer — `X`, `Chosen`, an SVar name; needs an ability-context evaluator `internal/expr` does not have yet                                                                                                      | M5-M6      |
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
