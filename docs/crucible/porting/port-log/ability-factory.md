# Port: AbilityFactory

- **Java source:** `forge-game/src/main/java/forge/game/ability/AbilityFactory.java` (getAbility, getSubAbility,
  additionalAbilityKeys), `forge-core/src/main/java/forge/util/FileSection.java` (parseToMap)
- **Go target:** `crucible/internal/carddb/compile`
- **Status:** Structure done — M3 slice A. Param values are still text; typing them is the generated layer

## What it does

Turns an ability line into a node: what kind of record it is, which API it names, its params, and its sub-abilities
resolved into direct references.

`SubAbility$ DBFoo` means "resolve `SVar:DBFoo` next", so a card is a linked list of effects. Java walks that list by
name every time a `Card` is constructed; Crucible resolves it once at load, which is ADR-0007's whole point.

## The four reference shapes

All four are AbilityFactory's, and the API gate on the last two is not decoration — `Choices$` on any other API is a
valid string, and resolving it as a list of SVar names would fail on cards that are correct.

| Shape                         | Resolves to      | Gate                                                                               |
| ----------------------------- | ---------------- | ---------------------------------------------------------------------------------- |
| `SubAbility$ X`               | One SVar         | None                                                                               |
| 29 additional-ability keys    | One SVar each    | None. Java's `additionalAbilityKeys`, plus `SubAbility` and `PreventionSubAbility` |
| `Choices$ A,B,C`              | A list           | API is `Charm`, `GenericChoice`, `AssignGroup`, `VillainousChoice` or `Vote`       |
| `ResultSubAbilities$ 1:A,2:B` | `key:svar` pairs | API is `RollDice`                                                                  |

## Param maps are maps, not lists

`FileSection.parseToMap` reads params into a `TreeMap` with `String.CASE_INSENSITIVE_ORDER`. Two consequences, both
load-bearing and both measured in the corpus:

| Consequence                              | Corpus                                                 |
| ---------------------------------------- | ------------------------------------------------------ |
| A repeated key keeps only its last value | 29 lines on 24 cards repeat a key                      |
| Key spelling ignores case                | 5 keys have two spellings, including one `SubABility$` |

Reading the params as a list instead chains a sub-ability Forge never chains — `the_eagles_are_coming` writes
`SubAbility$` twice, and only the second exists — and misses one it does.

Order is kept anyway, because Java's TreeMap loses it and nothing should depend on that.

## Deviations from Java

| Java                                                            | Go                                                                                                                    |
| --------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| An unresolved `SubAbility$` prints to stdout and returns `null` | `error` naming the card and the reference. A chain that ends early is a defect, not a state (PORT-8)                  |
| A cycle would recurse until the stack ends                      | `ErrCycle`, detected by the set of SVars on the current chain                                                         |
| `AbilityRecordType` covers `AB`, `SP`, `ST`, `DB`               | Seven records: those four plus `RE`, and `Mode$` split into `Trigger` and `StaticEffect` by the line carrying it      |
| Resolution happens per `Card` construction                      | Once per script at load (ADR-0007). A game builds ~120 cards, and a million-game run would repeat the work 10^8 times |

## Not ported yet

| Java                                                                             | When       |
| -------------------------------------------------------------------------------- | ---------- |
| Typed params. Values are text here                                               | M3 slice C |
| Costs, valid strings, count expressions -- the values themselves                 | M3 slice D |
| Keyword expansion (`CardFactoryUtil.setupKeywordedAbilities`)                    | M3         |
| SVars naming statics, triggers or replacements (`StaticAbilities$`, `Triggers$`) | M4         |
| Functional variants -- `Variant:` faces have their own lines                     | M3 slice C |

## Golden AST diff

`compile.WriteCanonical` writes a compiled card as deterministic, indented text; `compile.Fingerprint` hashes it.
`testdata/ast.golden` carries one `filename<TAB>hash` line per card, and `testdata/ast-shapes.golden` carries the full
text of six cards chosen to cover every record type and every way one ability names another.

Two files rather than one because they answer different questions. The hash golden says **which** of 33,689 cards
changed, which is the only thing a 34,000-line diff can usefully say. The shapes golden says **what** a change looks
like, which is what a reviewer actually needs.

Regenerate with `-update`, and read the diff before committing it. A golden updated without being read converts an
unexplained change into an approved one, which is worse than having no golden.

## `ReplaceWith$` and three more keys the list misses

`AbilityFactory.additionalAbilityKeys` is not the whole set of params whose value is an SVar holding an ability. Four
more are resolved by the handler or the effect that needs them, and reading only the list leaves them unfollowed:

| Key                              | Resolved by                   | Corpus uses |
| -------------------------------- | ----------------------------- | ----------: |
| `ReplaceWith`                    | `ReplacementHandler.java:843` |       1,581 |
| `ExtraTurnDelayedTriggerExecute` | `AddTurnEffect.java:50`       |           7 |
| `ExtraPhaseDelayedTriggerExcute` | `AddPhaseEffect.java:72`      |           4 |
| `Else`                           | `RollDiceEffect.java:474`     |           0 |

The misspelling in `ExtraPhaseDelayedTriggerExcute` is Forge's own and is load-bearing: the param map is keyed on it, so
correcting it in the Go port would stop the key matching.

`ReplaceWith` is the one that mattered. Every replacement effect in the corpus names its ability that way, so before
this the compiled AST stopped at the replacement line for 1,276 cards, and `TestCorpusCompiles` could not have found a
dangling `ReplaceWith$` -- the same defect class as the five dangling `SubAbility$` references it did find. The corpus
has none today, but it was not being checked.

The golden AST diff is how this surfaced: `leader_super_genius` showed `param ReplaceWith$ "DBDraw"` with no `sub` line
under it.

## What type is a param?

ADR-0007 says parameters become typed structs per API — `DealDamageParams` holding an `expr.Amount` and a `valid.Spec`,
not two strings. Nothing in Forge declares those types, so they have to be recovered, and `tools/apiscan -kinds` is the
measurement. Two independent signals, because neither is sufficient:

| Signal             | Settles                                                                             | Cannot settle                                      |
| ------------------ | ----------------------------------------------------------------------------------- | -------------------------------------------------- |
| The Java call site | `calculateAmount` → amount, `ZoneType.smartValueOf` → zone, `getValidCards` → valid | A key with no distinctive consumer                 |
| The written values | Every value `True`/`False` → flag; an expression head → amount; spaces → prose      | Zone vs valid string vs SVar name: all bare tokens |

Over the 1,139 param keys at least one card writes:

| Evidence                   | Keys | Share |
| -------------------------- | ---: | ----: |
| Typed by a Java call site  |  257 |   23% |
| Every value `True`/`False` |  421 |  flag |
| Neither signal             |  445 |   39% |

**So the generator cannot type everything, and should not pretend to.** A key with no evidence gets a `string` field,
which is what Java has anyway; the win is that the ~680 with evidence get a real type, and GO-8's actual requirement —
no `any`, no `map[string]string` — is met by the struct either way.

The signals also disagree on real keys, and that is the finding worth keeping rather than averaging away:

```text
AttachedTo   valid:9,defined:11
Choices      valid:40,defined:2
NumCards     amount:24,int:2
```

`AttachedTo` is a valid string on some effects and an object selector on others. A per-key type would be wrong for one
of them, which is why the type belongs to the (API, key) pair and not to the key — the same reason the vocabulary itself
had to be attributed per effect.

`testdata/param-kinds.golden` pins all 1,139 rows, so an upstream refactor that moves a `getParam` call changes a diff
someone reads rather than the type of a generated field.

## An unknown key is not an error

Java accepts any key. `AbilityFactory` builds the map, the effect asks for what it knows, and a key nobody asks for is
neither rejected nor logged. The Go compile matches that — refusing an unknown key would reject cards Forge plays — so
the check lives outside the compiler, in `tools/apiscan -check`, which fails the build on a key no Java code reads
anywhere.

Two Java behaviours the port depends on, both found by that gate:

| Behaviour                                                     | Where                                    |
| ------------------------------------------------------------- | ---------------------------------------- |
| `matchesValidParam` returns **true** when the param is absent | `CardTraitBase.java:259-265`             |
| An absent param takes the effect's default, not a failure     | e.g. `getParamOrDefault` on every effect |

The first is the one that bites: a missing `Valid…` key does not narrow a restriction, it removes it.

## Known defects it surfaces

Compiling the corpus found five cards whose `SubAbility$` names an SVar that does not exist. All five are fixed and
carried until upstream merges them, so the corpus compiles whole with no exemption. Each is in
[`../card-script-defects.md`](../card-script-defects.md) with the printed text it was checked against.

The param gate found thirteen more keys that no Java code reads, across seventeen cards, in the same file.
