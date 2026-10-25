# Test helper cheat sheet (`crucible/internal/engine/*_test.go`, package `engine_test`)

Read this instead of grepping for signatures. Reason: guessing them cost a failed compile per port.

| Need | Helper |
| --- | --- |
| Two-player game, synthetic cards | `g, p, other := newTwoPlayerGame(t)` |
| Same, with the real corpus | `newTwoPlayerGameOn(t, scenarioDB(t))`, cards via `corpusCard(t, "Name")` |
| Run one effect line | `resolveLine(t, g, p, ctl, "DB$ ...", "SVarName", "DB$ ...")`: casts a helper creature, needs a main phase and an empty stack. `resolveNow(t, g, p, ctl, targets, line, svars...)` also returns the error |
| Host card with a trigger or chain | `etbChainDef(t, name, trig, svars...)`, `castETBChain(t, g, p, def, ctl)` |
| Replacement or trigger host | `planarDef(t, name, typeLine, triggers, replacements, svars...)` |
| Plain creature | `creatureDefPT(t, "2", "2")`, `creatureDefPTKeywords(t, p, t, kw...)` |
| Library filler | `libraryCards(t, g, p, n)` |
| Combat | `declareAttackers(t, g, ctl)` then `declareBlockers(t, g, ctl)`, then `g.DealCombatDamage(ctl)` |
| Walk to a step | `toStep(t, g, ctl, p, engine.Upkeep, before)` |
| Cast and resolve | `g.CastSpell(p, id, ctl)`, `g.ResolveStack(engine.NewRegistry(), ctl)`, `g.TakePendingError()` |
| A cost the engine must refuse | `unsupportedCost1`, `unsupportedCost4` (`unsupportedcost_test.go`); change them, not every test, when a part is ported |

`ScriptedController` queues (one answer per decision, a missing one panics with `scriptExhausted`):

| Decision | Queue |
| --- | --- |
| Targets | `QueueTargets([]EntityID)`, one call per targeting ability, chain links in order |
| Pay generic | `QueuePayGeneric(mana.ShardG)`: ONE shard per call, one call per generic unit |
| X | `QueuePayX(n)`, hybrid `QueueHybridManaColor(c)` |
| Pay an unless or upkeep cost | `QueueConfirmPayCost(bool)` |
| Optional effect or trigger | `QueueConfirmEffect(bool)`, `QueueConfirmOptionalTrigger(bool)` |
| Number, entity, card, player | `QueueNumberChoice`, `QueueEntityChoice`, `QueueCardChoice`, `QueuePlayerChoice` |
| Named option or index | `QueueOption(i)`, `QueueOptionNamed(s)`, `QueueModeChoice([]int)` |
| Sacrifice, return, discard | `QueueSacrificeChoice`, `QueueReturnChoice`, `QueueDiscardChoice` |
| Coin call | `QueueCoinCall(heads)` |
| Attack and block | `QueueAttackers`, `QueueAttackTarget`, `QueueBlocks` |

Pool mana is added with `g.Player(p).ManaPool.Add(mana.Blue, n)`; a game has no `g.Stack()`.
