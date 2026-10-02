# Port Log — Game State: ability-context amounts

- **Parent:** [`game-state.md`](../game-state.md) — Java counterpart, model, index, `Not ported yet`
- **Go:** [`internal/engine`](../../../../../crucible/internal/engine) — `amountcontext.go`, `amount.go`
  (`expressionValue`), `effect.go` (`Game.resolving`), `chosencosts.go` (`Ability.paid`), `trigger.go` (`card` of an
  enters trigger)

## `Targeted$`, `TriggeredCard$`, `Remembered$` and the rest

`AbilityUtils.calculateAmount` (`AbilityUtils.java:640-715`) reads an SVar written `<Object>$<measure>` by building a
list of cards from the resolving ability and measuring it (`handlePaid`). About 1,100 corpus uses: `Remembered$` (~500),
`Targeted$` (~330), `TriggeredCard$` (~220), `Sacrificed$` (~130), `Imprinted$`, `TriggeredAttacker$`,
`TriggeredBlocker$`, `Exiled$`, `Discarded$`.

`Registry.resolve` sets `Game.resolving` to the ability it runs and restores the previous one afterwards, so a
sub-ability reads its own (the chain copies targets, triggering objects and paid cards). `expressionValue` sends the
heads in `contextHeads` to `contextValue`, which returns unresolved (never 0) outside a resolution:

| Head                                | Cards                                                                                                          |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `Targeted`                          | the card targets of the ability                                                                                |
| `TriggeredCard`                     | the card the trigger recorded: the one that entered (`checkETBTriggers`), died, was discarded, tapped for mana |
| `TriggeredAttacker`/`Blocker`       | the attacker or blocker an attack or block trigger recorded                                                    |
| `Remembered`, `Imprinted`           | the host's `Memory` lists                                                                                      |
| `Sacrificed`, `Exiled`, `Discarded` | the cards the cost used up (`Ability.paid`)                                                                    |

A card that left the battlefield (a sacrificed or dying creature) is read from its last-known information (`Game.LKI`),
which is what Java's copy of it holds. The measure is `Amount` (how many), `Valid <spec>` (how many match, from the
host), or a per-card measure summed or folded with `Greatest`/`Least`/`Different`: `CardPower`, `CardToughness`,
`CardManaCost`, `CardCounters.<TYPE>`, `CardNumColors` (`liveCardMeasure`; `perCardMeasure` keeps refusing power and
toughness because it runs while Layer 7 is rebuilt). Anything else is unresolved. `resolveNamedAmount` also strips a
leading sign (`NumAtt$ +X`) as `calculateAmount` does, which Pump's `+X` needed.

Not resolved: the player-list heads (`TriggeredPlayer$`, `TriggeredTarget$`, `TargetedPlayer$`,
`PlayerCountRemembered$`), `Exiled$`/`Revealed$` lists that no cost or effect records yet, the `LKI` spellings
(`RememberedLKI$`), `AllTargeted$`, `ParentTargeted$`, `TriggerObjects*$`, `TriggeredSpellAbility$CardManaCostLKI`,
`ThisTargeted$`. Tests: `contextamounts_test.go`, `chosencosts_test.go`.
