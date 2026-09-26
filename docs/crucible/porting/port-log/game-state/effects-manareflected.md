# Port Log — Game State: M6 Effects: ManaReflected

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine)

## ManaReflected researched, not ported

`ManaReflected` (47 corpus lines) stays `ErrUnimplemented`. Reason: its dominant shape, `ReflectProperty$ Produced` (22
lines), is a triggered mana ability (CR 605.1b), and this port has none. The cross-permanent reflection walk
(`CardUtil.java:237-346`) is not the blocker: it can be built from pieces the engine already has.

Java read directly: `ManaReflectedEffect.java:32-147`, `CardUtil.java:234-367` (`getReflectableManaColors`,
`canProduce`), `AbilityManaPart.java:200-226` (`produceMana`, `tapsForMana`), `AbilityManaPart.java:601-613` and
`SpellAbility.java:295-309` (`canProduce`), `TriggerTapsForMana.java:67-88`, `TriggerHandler.java:300-309` and
`:522-526` (static triggers).

### Corpus shapes

| `ReflectProperty$` | Lines | Where                                                                             | Colors from                                                            |
| ------------------ | ----: | --------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `Produced`         |    22 | `DB$` under a `Mode$ TapsForMana \| Static$ True` trigger, every one              | The triggering mana, `AbilityKey.Produced` (`CardUtil.java:295-307`)   |
| `Produce`          |    15 | `A:AB$` mana abilities (Reflecting Pool, Exotic Orchard, Fellwar Stone)           | What `Valid$` cards' abilities could produce (`CardUtil.java:308-345`) |
| `Is`               |    10 | 7 `A:AB$`; Omnath's `DB$` (`Produced$ Combo`); Tazri/Katilda `AddAbility$` grants | `Valid$` cards' colors (`CardUtil.java:283-294`)                       |

Of the 22 `Produced` lines, 20 are the trigger's own `Execute$` and 2 are chained under it (Overabundance's
`SubAbility$`, Sasaya's `RepeatEach`). `Defined$`: `You` 9, `TriggeredActivator` 12, `TriggeredCardController` 1.

**Registering it now would register a `Resolve` that handles zero lines.** The `A:AB$` lines are mana abilities, so they
belong in `ActivateManaAbility` (no stack), not `Registry.Resolve`. Every `DB$` line the registry can reach is
`Produced` or Omnath's `Produced$ Combo` (`specifyManaCombo`, a multi-color split decision this port lacks). Both would
be rejected.

### What the engine lacks

In place since ADR-0020 ([`static-triggers.md`](static-triggers.md)): `Static$ True` `TapsForMana` triggers resolve
immediately, `Game` owns its registry, and `triggeredObjects` carries the produced mana, activator and card.

| Gap                               | Where                                                                                                                                                                                                                                                        | Blocks        |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------- |
| Colorless as a choice             | `ColorOrType$ Type` adds colorless (`CardUtil.java:256`, `:303-305`, `:362-364`). `ChooseManaColor` (`control.go:387`) answers a `mana.Colors`, which has no colorless member; Java asks `chooseColorAllowColorless` (`ManaReflectedEffect.java:99`, `:108`) | `Type` lines  |
| `ManaReflected` as a mana ability | `ActivateAbility` refuses only `Name == "Mana"` (`activateability.go:233`); registering `ManaReflected` would put Reflecting Pool on the stack. `ActivateManaAbility` accepts only `Mana` (`activatemanaability.go:244`)                                     | `A:AB$` lines |
| `Valid$ Defined.*` names          | `definedCards` (`defined.go`) lacks `ExiledWith`, `ValidGraveyard ...`, and the cost-paid `Sacrificed`/`Untapped` lists                                                                                                                                      | 5 lines       |

`Static$ True` `TapsForMana` triggers are already a gap without this API: 56 of 72 corpus `Mode$ TapsForMana` lines are
`Static$ True`, and 34 of them execute `DB$ Mana`. Those are stacked today. Other modes skip `Static$` lines outright
(`checkLandPlayedTriggers`, `isBecomesTargetTrigger`); `TapsForMana` does not.

**The reflection walk is buildable, with one caveat.** A card's producible colors are its basic land subtypes through
`Type()` (`basicLandType`, `manaability.go:21`), plus each `Def.Faces[0].Abilities` mana line's `Produced$` through
`producedManaColor`/`parseComboColors`, plus the `SubAbility$` chain (`SpellAbility.java:303-309`) and trigger
`Execute$` abilities (`CardUtil.java:311-314`). That is the same ability model `ActivateManaAbility` assumes.
`Produced$` shapes it cannot read (`Chosen`, `Combo Any`, `ColorIdentity`, `Special`) and `Condition*` params
(`metConditions`, `SpellAbility.java:297`) fail closed with an `error`. `Is` needs only `Card.Colors()` (`card.go:404`).

Caveat: no continuous layer reads `AddAbility$`, so Chromatic Lantern's "lands you control have {T}: add one mana of any
color" is silently not applied, and `AddType$ AllBasicLandType` (Prismatic Omen) is silently skipped
(`applyOneContinuousType`, `continuous.go:293`). A walk over printed abilities would then under-report colors without an
error. Until Layer 6 grants exist, the walk has to find a battlefield static with `AddAbility$` or a skipped `AddType$`
whose `Affected$` matches a reflected card, and fail closed. `ActivateManaAbility` has the same blind spot, but there it
errs toward "cannot activate"; here it gives a smaller color set.

### Smallest real design, in order

1. **Triggered mana abilities (CR 605.1b).** In place (ADR-0020, [`static-triggers.md`](static-triggers.md)). What
   remains for `Produced` (22 lines) is registering `ManaReflected` itself over the recorded `produced` mana.
2. **Colorless choice.** A `PlayerController` decision offering colors plus colorless, mirroring
   `chooseColorAllowColorless`. Unblocks every `ColorOrType$ Type` line.
3. **Activated route.** `ActivateManaAbility` branches on `Name == "ManaReflected"`; `ActivateAbility` refuses it.
   `Produce` walk and `Is` colors as above, with the parents set (`CardUtil.java:275`, `:329-331`) so mutually
   reflecting lands terminate. Unblocks 22 `A:AB$` lines, less the 5 needing new `Defined$` names.

**Forge bug (PORT-8, tracked in [`forge-java-defects.md`](../../forge-java-defects.md)).** `CardUtil.java:345`: the
recursive `getReflectableManaColors(sa, ab, colors, parents)` passes the outer `sa` as `abMana`, so the nested frame's
`card` (`:243`) is the reflecting card's host, not the reflected ability's. The nested `Valid$ Defined.*` then resolves
against the wrong card (`:265`), and so does the valid-string source (`:271`). Reflecting Pool reflecting Pit of
Offerings (`Valid$ Defined.ExiledWith`) reads cards exiled with Reflecting Pool, so misses the colors Pit could produce
(CR 106.7). Correct only for the top frame.

**Researched and deferred.**

| API             | Blocker                                                                                                                                        |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `ManaReflected` | Dominant `Produced` shape (22 of 47) needs the effect itself registered over the triggering `Produced` mana (ADR-0020 infrastructure in place) |
