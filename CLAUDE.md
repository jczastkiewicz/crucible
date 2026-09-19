# CLAUDE.md — Crucible

Fork of [Card-Forge/forge](https://github.com/Card-Forge/forge). Java MTG engine.

**Crucible** = Go port of that engine + automated deck testing / optimization suite. Runs batch simulations of a target
deck vs a meta gauntlet, captures telemetry (dead cards, mana health, per-card impact), emits deck-improvement reports.

Plan: `docs/crucible/00-master-implementation-plan.md`

---

## Read before working

| Doing                                               | Read first                                              |
| --------------------------------------------------- | ------------------------------------------------------- |
| Writing **any** doc, ADR, comment, commit body      | `docs/crucible/guidelines/00-documentation-style.md`    |
| Writing Go                                          | `docs/crucible/guidelines/01-go-coding-standards.md`    |
| Porting a Java file                                 | `docs/crucible/guidelines/02-java-to-go-translation.md` |
| Writing tests                                       | `docs/crucible/guidelines/03-testing-standards.md`      |
| Making an architectural decision                    | `docs/crucible/guidelines/04-adr-process.md`            |
| Writing anything under `architecture/` or `design/` | `docs/crucible/guidelines/06-architecture-docs.md`      |
| Committing / reviewing                              | `docs/crucible/guidelines/05-commit-and-review.md`      |

Index: `docs/crucible/guidelines/README.md`

Rules have stable IDs — `DOC-4`, `GO-2`, `TEST-1`, `PORT-2`, `ADRP-1`, `REV-8`, `ARCH-2`. Cite them in comments and
commit bodies.

---

## Layout — do not deviate

| Path                    | Contents                                                     |
| ----------------------- | ------------------------------------------------------------ |
| `crucible/`             | **All** Go code. Own `go.mod`                                |
| `docs/crucible/`        | **All** Crucible docs                                        |
| `crucible/oracle-java/` | Only place Java may be added — test-scoped dumpers/recorders |
| everything else         | Upstream Forge. **Do not edit**                              |

Unavoidable upstream edit → log it in `docs/crucible/porting/upstream-patches.md`, same commit. Reason: every touched
line outside `crucible/` and `docs/crucible/` is a future rebase conflict (REV-1).

---

## Non-negotiables

Violated most often. Full reasoning in the guideline files.

1. **No package-level mutable state** in `internal/engine/...` or `internal/carddb/...`. Inject `*carddb.DB` and
   `*javarand.Rand` via `*Game`. Engine runs goroutine-per-game. Java's `StaticData` / `FModel` / `MyRandom` singletons
   must not be translated. (GO-2)
2. **No mutex in the engine.** Games share only immutable data. Mutex there = design bug. (GO-3)
3. **No reflection in the engine.** `ApiType` → effect dispatch uses a generated registry, not `reflect`. (GO-4)
4. **No `any`** in engine or carddb packages. Ability params are generated typed structs, not `map[string]string`.
   (GO-8)
5. **Compile card scripts once at load** into an immutable typed AST. Never re-interpret script strings at runtime the
   way Java does. (PORT-2)
6. **Identity by ID**, never pointer. `CardID uint32` into a per-`Game` arena. (GO-9)
7. **Iteration order is load-bearing.** Use `collect.OrderedSet` / `OrderedMap` where Java used `FCollection`. Bare Go
   `map` breaks trigger ordering and replay parity. (GO-12)
8. **`error` for anything a card script can cause. `panic` only on engine invariant breach**, recovered at the game
   boundary. One bad card must not kill a 100k-game batch. (GO-7)
9. **Docs land in the same commit as code.** New package → row in `architecture/module-map.md`. Ported unit →
   `porting/port-log/<unit>.md` note. ADR merges _before_ its implementing PR. (DOC-12, ADRP-4)
10. **Near-zero dependencies.** Non-stdlib import needs an ADR. Currently allowed: `github.com/google/go-cmp`, tests
    only. (GO-14)
11. **A Forge bug is reported, never worked around.** Malformed script, param that never reaches its effect, method that
    cannot do what its name says → stop, name the file and line, fix it upstream. Carry the fix on a branch and log it
    in `porting/upstream-patches.md` if Crucible needs it now. Never Go code that compensates. Reason: a workaround
    makes Crucible disagree with the oracle for a reason no diff can explain. Distinct from PORT-7, where a quirk is
    reproduced because parity depends on it. (PORT-8)

---

## Testing — module first

**Default: test the whole package through its public API.** Use `package x_test`, not `package x`. Compiler then blocks
reaching into internals. (TEST-1)

Reason: port work rewrites internals constantly. Tests bound to functions lock structure and block the refactors the
port needs. Tests bound to package behavior survive rewrites.

Drop below module level only per this table (TEST-2):

| Situation                                                              | Level                                           |
| ---------------------------------------------------------------------- | ----------------------------------------------- |
| Behavior visible through public API                                    | **Module** — `package x_test`. Default          |
| Pure function, huge input space (parsers, mana cost, valid strings)    | File-level table test + `testing.F` fuzz        |
| Invariant not observable from outside (layer ordering, solver pruning) | Internal test — `package x`, with a why-comment |
| Whole-engine rules behavior                                            | **Fixture directory**, not a Go func            |
| Bug fix                                                                | Module-level regression first                   |

Also:

- Rules tests are **data**: `testdata/scenarios/<case>/{setup.state,actions.log,expect.state,expect.events}`. One Go
  test walks the tree. Adding a test = adding a directory. (TEST-5)
- `setup.state` uses Forge's `GameState` text format so the same fixture runs against the Java oracle.
- `t.Parallel()` by default. CI runs `go test -race ./...` every commit. (TEST-7)
- **No mocking library.** Pass in a different DB / RNG / controller. Needing a mock = wrong seam. (TEST-8)
- Test files named for behavior (`layers_test.go`), never mirroring source files (`card_test.go`). (TEST-3)

---

## Doc style

All docs use the compressed style in `guidelines/00-documentation-style.md`:

- Drop articles, filler, hedging, pleasantries. Fragments fine.
- **Never** drop technical content — identifiers, paths with line numbers, numbers with units, error strings stay exact.
  (DOC-2)
- Every rule carries a reason. (DOC-4)
- 3+ parallel items → table. (DOC-5)
- Code blocks and code comments stay normal English, uncompressed. (DOC-7)
- Show Bad → Good pairs. (DOC-8)
- Write in full sentences for destructive steps, ordered procedures, security and licensing. (DOC-9)
- **State, never history.** No `Correction` / `Update` / changelog sections, no "this previously said". Fix the fact in
  place; git holds the diff. Same for ADRs — a new decision supersedes, everything else is edited in place. (DOC-16,
  ADRP-3)
- **Run `prettier --write .` before every commit.** Config `/.prettierrc`, `printWidth: 120`, `proseWrap: always`.
  Prettier owns table alignment and wrapping — do not hand-align. (DOC-14)

Audience is an IT engineer. Compress grammar, not substance.

---

## Commands

```bash
# Markdown — both required before every commit (DOC-14, DOC-15)
prettier --write .            # format; .prettierignore excludes all upstream Forge files
prettier --check .            # CI gate
npx markdownlint-cli2 "CLAUDE.md" "docs/crucible/**/*.md"   # semantic lint
```

```bash
# Go
cd crucible && go test -race ./...
cd crucible && go test -race -coverprofile=cover.out ./... && go run ./tools/covergate -profile cover.out   # TEST-12 floors
cd crucible && go test ./internal/carddb/compile -run TestCorpusAST -update   # regen the golden AST fingerprints, review the diff
cd crucible && golangci-lint run

# Java oracle
mvn -pl crucible/oracle-java -am test

# Upstream Java suite: TestNG, needs a display
mvn -U -B clean test          # CI runs this under Xvfb
```

---

## Current state

M0 done — 16 ADRs `Accepted`, 7 guidelines, 4 design docs, 6 DSL grammars.

M1 done — `pkg/collect`, `pkg/javarand` (bit-matches Java over the committed golden), `internal/mana` (round-trips every
cost in the corpus), `internal/cardtype`, `tools/javacycles`, `tools/enginelint`, `tools/docgate`, `tools/covergate`,
`oracle-java`.

M2 done — `internal/carddb`, `internal/deck`, `tools/carddump`, `crucible corpus-coverage`. P1 gate green: the canonical
dump is byte-identical to Forge's own `CardRules.Reader` across the whole corpus, and no script key is exempt from the
parser.

M3 done — `internal/carddb/compile` compiles all 33,697 cards with no exemption; `internal/valid`, `expr`, `cost`,
`keyword` port the value grammars; `tools/apiscan` gates the param vocabulary two ways, both at zero; typed param
structs generated (`compile/params_gen.go`); the valid property vocabulary gate is green
(`TestEveryPropertyIsAccountedFor`, M3 item 19, `port-log/valid-strings.md`). **P2 exit gate green:** no unknowns,
allowlist empty, golden AST diff clean.

M4 done — `internal/engine/{game,card,player,zone,event,control}`; `PlayerController` (eleven decision methods) with
`ScriptedController`; `GameState` fixture load/dump, byte-identical round-trip (`internal/fixture`); event schema v1
(ADR-0013). Effect dispatch scaffolding (`Effect`/`Registry`, ADR-0011, ADR-0003 puts them in `internal/engine`) landed
holding zero implementations — M5's own `permanentEffect` (below) is the first two, but the 203 script-driven APIs in
corpus-frequency order were still M6's job, not M4's or M5's, until `Draw` (below) became the first to land.

M5 in progress (rules kernel). Done: turn/phase/step loop + priority (`turn.go`, `phase.go`); zone changes + state-based
actions (`action.go`) — legend rule, World rule, lethal damage, Battle protector, dangling-attachment cleanup; combat
(`combat.go`, `attack.go`, `block.go`, `combatdamage.go`) — first strike, trample, gang blocking, a combat split across
more than one defending player; mulligans (`mulligan.go`); the valid-string evaluator (`valid.go`, `engine.Matches`)
SBAs and targeting both read, built corpus-frequency-first (`port-log/valid-strings.md`), including `SharesColorWith`'s
own bare form; a mana pool and payment covering all eight harder cost shapes (`mana.go`, `manapay.go`); a basic land's
intrinsic mana ability (`manaability.go`) and playing a land (`land.go`); casting a spell — a non-Aura permanent or an
Aura, through the stack — (`castspell.go`), the first two real `Effect` implementations
(`permanentEffect`/`attachEffect`); trigger firing (`trigger.go`) — "enters," "dies," "attacks," "blocks," "deals
damage," "is discarded," "becomes tapped," "taps for mana," "a player casts a spell," "the beginning of a step or
phase," "a player attacks" and "a player draws a card" (`checkPhaseTriggers` — CR 500, `Mode$ Phase`, the corpus's own
SECOND most frequent trigger mode at 2,362 real lines, ahead of `Attacks` itself, resolved once corpus-frequency
research found it well after the first nine modes had already landed; unlike every other mode, it walks four zones —
`Battlefield`/`Command`/`Graveyard`/`Exile` (`phaseTriggerZones`) — not `Battlefield` alone, and matches `ValidPlayer$`
against the active player, not the trigger's own host controller; `checkAttackersDeclaredTrigger` — CR 508.1,
`Mode$ AttackersDeclared`, 286 real lines, fires once per combat rather than once per attacker the way `Attacks` itself
does, reusing `phaseTriggerZones`'s own four-zone walk and a new `attackedTargetMatches`/`validAttackersCountMatches`
pair for `AttackedTarget$`/`ValidAttackers$`; `checkDrawnTriggers` — CR 120.3, `Mode$ Drawn`, 161 real lines, called
from `DrawCards`' own per-card loop (turn.go, already written that way before this mode existed to consume it) with a
new `Player.CardsDrawnThisTurn` counter — `LandsPlayed`'s own per-turn-counter shape — for `Number$`'s own "the Nth card
you draw each turn" real corpus shape), plus another permanent watching one do any of those — detects and queues a
trigger, pushed through a new `pushTriggeredAbilities` (`trigger.go`) that ports CR 603.3b's own APNAP ordering for the
first time (`playersInAPNAPOrder` — `Game.ActivePlayer()`, then turn order — walking each player's own group in turn, so
the non-active player's own trigger resolves before the active player's when more than one fires off the same event, the
stack's own LIFO order applied to `MagicStack`'s own player-iteration sequence); every trigger-check function now
collects its own matches first and calls it once, rather than calling `PushAbility` the instant each match is found.
`matchesPlayerBase` (`valid.go`) is the shared `You`/`Opponent`/`Player` dispatch several of those modes now reuse;
block legality (`staticability.go`, `CanBlock`) — flying/reach, Fear, Horsemanship, Intimidate, Landwalk, Protection,
Menace and every literal `S:Mode$ CantBlockBy` line; the legend rule's own `ignoreLegendRule` exemption
(`staticability.go`) — three slices of the general static-ability engine PORT-8 requires reading Java's own mechanism
for rather than hardcoding a keyword check; and four layers of the engine's biggest piece, `Mode$ Continuous` itself
(`continuous.go`) — `applyContinuousPT` resolves Layer 7b/7c's own `Affected$`-matched, plain-integer power/toughness
lines (anthem effects, equipment bonuses); `applyContinuousType` resolves Layer 4's own `AddType$`/`RemoveType$` lines
naming only literal type words; `applyContinuousColor` resolves Layer 5's own `AddColor$`/`SetColor$` lines naming a
literal color, `All` or `Colorless`; `applyContinuousKeyword` resolves Layer 6's own `AddKeyword$` lines naming only
literal keyword lines (no dynamic value, no `RemoveKeyword$`/`RemoveAllAbilities$`/`SharedKeywords$`/`FromDraftNotes$`
combo) — the single largest real slice of the four (1,556 of 1,857 real lines), folded through a new `KeywordMod`
(`keywordmod.go`) `Card.HasKeyword` now reads, reaching every existing keyword-driven check (`cantBlockByKeywords`,
combat's own first-strike/trample reads) for free — all four layers recomputed fresh every `CheckStateBasedActions` pass
rather than pushed once, `pt.go`'s own folding mechanism and its new `typemod.go`/`colormod.go`/`keywordmod.go`
counterparts' first real callers. `cardtype.Line` gained `ParseToken`/`Union`/`Without` to make Layer 4 possible without
a `*cardtype.Registry` this port still does not inject into the engine (`ParseToken`'s own doc comment); Landwalk's own
`ValidDefender$ Player.controls<Type>` needed a new `matchesValidDefender` (`staticability.go`), a `Player`, not a
`Card`, matched the same way `SpellCast`'s own `ValidActivatingPlayer` is; Protection's own CantBlockBy restriction
(`protectionValid`, `staticability.go`) is built per card from the keyword's own argument the identical way Landwalk's
is, both real corpus shapes (the natural-language "Protection from red" and the colon-structured "Protection:Artifact")
resolving through the existing valid-string evaluator with no new property needed; `Blocks`'s own `ValidBlocked$` is
checked against the declared attacker directly, the per-pair granularity `checkBlocksTriggers` already has standing in
for Java's own full-attacker-collection match; and `matchesPlayerSpec`/`matchesPlayerProperty` (`valid.go`) extend
`matchesPlayerBase` with the one dotted-property layer (`Active`/`NonActive`/`Other`, `Game.ActivePlayer()`) real corpus
lines put on top of it, closing 19 of `SpellCast`'s own 25 qualified `ValidActivatingPlayer$` lines plus `DamageDone`'s
own qualified `ValidTarget$` and `TapsForMana`'s own qualified `Activator$`. A new `compile.Face.Amounts` (compile.go)
parses every non-ability SVar a face defines (`internal/expr`) at compile time, and a new `resolveAmount` (`amount.go`)
evaluates the one family of it this port's own `Matches` already can, `Count$Valid[<Zone>...] <spec>` — 2,804 of the
corpus's 6,186 real `Count$` expressions — closing `ptParam`'s own
dynamic-`AddPower$`/`AddToughness$`/`SetPower$`/`SetToughness$` gap for that shape and, with it, Layer 7a itself:
`applyOneCharacteristicDefiningPT` (`continuous.go`) resolves a `CharacteristicDefining$ True` line's own
`SetPower$`/`SetToughness$` and applies the result to its host alone, at `LayerCharacteristic` — a layer `PTEffect`'s
own folding already carried, unused until now. Skulk's own `ValidBlocker$ Creature.powerGTX` closes block legality's
last `CantBlockBy` gap: `skulkBlocks` (`staticability.go`) reads Java's own hardcoded `X` (`Count$CardPower` against the
ability's own host, always the attacker) as a direct `Power()` comparison rather than a `Compare`/SVar question at all,
the same hardcoded-comparison shape `menaceLegal` already has for Menace. `hostRefusesEnchant` (`staticability.go`)
closes the "cleanup aura" rule's own Protection/bare Hexproof gap — reusing `protectionValid` against the aura itself
rather than a candidate blocker, plus bare Hexproof's own unconditional "any opponent" form — checked both when an Aura
is cast (`enchantTargets`, `castspell.go`) and on every ongoing SBA pass (`cleanupDanglingAttachments`, `action.go`);
building it surfaced a real, separate gap (`protectionValid`/`landwalkType` read only a card's PRINTED keywords, missing
one a continuous effect grants), closed by a new `Card.KeywordLines` (`card.go`) both now share with `HasKeyword`. A
qualified Hexproof (`Hexproof:Black`, `Hexproof:Enchantment`, ...) resolves too now — `hexproofValidSource`
(`staticability.go`) ports `KeywordWithType.parse`'s own bare-color-word case (`Black` becomes `Card.Black` before it
ever reaches `Matches`, since a bare color name is not itself a recognized valid-string base) alongside its bare-type
fallthrough (`Enchantment` stays as-is, an ordinary type check); the ability-source shape (`Hexproof:Triggered`,
`Hexproof:Activated`, `ValidSA$` in Java) still refuses rather than resolves, since `Matches` never evaluates a
`SpellAbility`. `Attacks`'s own `Alone$`, `DefendingPlayerPoisoned$` and `AttackDifferentPlayers$` all resolve now too —
`attacksOtherCount`/`attacksMultiplePlayers` (`trigger.go`) read `Combat.Attackers`/`Combat.AttackTargets` (combat.go,
attack.go) the same way `CombatUtil.checkDeclaredAttacker`'s own `AbilityKey.OtherAttackers`/`Defenders` do, and
`DefendingPlayerPoisoned$` reads `defenderOf(attacker)`'s own `Counters.Count(Poison)` directly. `DamageDone`'s own
`DamageAmount$` resolves too — `damageAmountMatches` (`trigger.go`) ports `TriggerDamageDone.performTest`'s own
hand-rolled operator/operand split (never `AbilityUtils.calculateAmount`, since every real line is a plain integer or
the literal `TargetToughness`) onto the existing `compareOp` (`valid.go`), reusing `Expressions.compare`'s own
vocabulary rather than adding a second one; both `checkDamageDoneTriggersToCard`/`ToPlayer` (trigger.go) and their two
real call sites (`dealPermanentDamage`/ `dealPlayerDamage`, combatdamage.go) now thread the actual damage amount
through. Layer 8 has real content too now — `applyContinuousRules`/`applyOneContinuousRules` (continuous.go) resolve
`SetMaxHandSize$`/`RaiseMaxHandSize$`/`AdjustLandPlays$` (75 of 78 real lines), a new `RulesMod`/`RulesEffect`
(rulesmod.go) on `Player` rather than `Card` — this port's first player-facing continuous effect — folded by two new
`Player` methods, `HandSizeLimit`/`LandPlayLimit`, that `cleanupStep`/`PlayLand` (turn.go/land.go) now read instead of
the bare `MaxHandSize`/`maxLandPlays` constants those two files already had waiting for exactly this. Layer 2 has real
content now too — `applyContinuousControl`/`applyOneContinuousControl` (continuous.go) resolve `GainControl$ You` (43 of
the corpus's 44 real `S:Mode$ Continuous` lines naming `GainControl$` — distinct from an unrelated
`DB$ ChangeZone`/`DB$ Dig`'s own one-shot `GainControl$ True`, "put onto the battlefield under your control," M6's own
remaining script-effect territory, not this layer at all) through a new `ControlMod`/`ControlEffect` (controlmod.go) —
this port's first controller-change mechanism. `Card.Controller`, a plain field until now, is `Card.Controller()`
(card.go), a folding method reading `ControlMod` the identical highest-Timestamp-wins pattern `RulesMod`/`PTEffect`
already use; `applyContinuousControl` runs first among the six appliers, ahead of Layers 4/5/6/7/8, since CR 613.1 puts
the control layer before every one of them and their own `Affected$` specs can themselves read `Controller()` (a
`YouCtrl` property) — a stale value there would evaluate against last pass's controller, not this one's. The qualified
`GainControl$ Player.isMonarch` (1 of 44) stays unresolved: no monarch mechanic to filter by (PORT-8/GO-7). M6 in
progress alongside it: `Draw` (`draweffect.go`) is the first of the 203 script-driven effects to actually resolve rather
than report `ErrUnimplemented` — `Ability` gained a `Params` field (`ability.go`) carrying a trigger's own
`Defined$`/`NumCards$` onto the stack to make that possible. Full detail:
`docs/crucible/00-master-implementation-plan.md` items 24-29, `docs/crucible/porting/port-log/game-state.md`. Thin or
missing: Layer 1 (copy effects — not even part of `StaticAbilityContinuous.java`'s own switch in Forge itself; zero real
references to `StaticAbilityLayer.COPY` anywhere in it, a wholly separate "become a copy of a card" mechanism at
resolution time, not a recomputed-each-pass continuous effect at all); Layer 3 (`GainTextOf$`, 1 real line, needs its
own card-text-copying mechanism for a single card); Layer 8's own remainder (`MayLookAt$`/`MayPlay$`, 88/181 real lines
— a cast-time zone-eligibility permission `CastSpell`'s hand-only check has nowhere to consult yet; `AddHiddenKeyword$`,
19, each of its 8 real distinct values its own separate block/attack/untap-step mechanic; vote/villainous-choice params,
0-3 real lines each); plus the rest of Layers 4/5/6 past a literal token list and Layer 7a's own SVar shapes outside the
Valid family (`xPaid`, `CardCounters`, `Devotion`, ...), plus a dozen more where the Valid argument itself carries a
`$`-suffixed distinct-value operator (Tarmogoyf's own `Card$CardTypes` — a new `expr.Count.DistinctProperty` field now
catches this rather than silently misparsing it, a real bug caught and fixed after the fact, not a hypothetical one) — a
dynamic value or a bulk-removal/`AddAllCreatureTypes$`/`SharedKeywords$` combo still skips the whole line rather than
applying it wrong; the legend rule's Partner-non-legendary-name corner case (needs a card-name lookup injecting into the
engine would violate GO-2); `Attacks`'s own `Attacked$`/`FirstAttack$`, `DamageDone`'s own `ValidCause$`, `Discarded`'s
own `ValidCause$`, `Taps`'s own `FirstTime$`/`Teamwork$`, `TapsForMana`'s own `Produced$`, `SpellCast`'s own
`Player.EnchantedBy`/`Player.Chosen` qualified `ValidActivatingPlayer$` forms, `Phase`'s own
`IsPresent$`/`PresentCompare$`/`CheckSVar$`/`Condition$`/`FirstUpkeep$`/`FirstUpkeepThisGame$`/`FirstCombat$`/
`TurnCount$` and its own qualified `ValidPlayer$` forms, and every trigger mode past
enters/dies/attacks/blocks/deals-damage/is-discarded/becomes-tapped/taps-for-mana/casts/beginning-of-a-step-or-phase;
202 script-driven effects past `Draw` still report `ErrUnimplemented`. **P4 exit gate's fixture-count half met:** 342
scenarios (`testdata/scenarios/`) past the ≥300 floor; the qualitative half ("every layer, every SBA," Plan Section 3.2)
is not.
