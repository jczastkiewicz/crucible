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
damage," "is discarded" and "a player casts a spell," plus another permanent watching one do any of those — detects and
queues a trigger, `matchesPlayerBase` (`valid.go`) the shared `You`/`Opponent`/`Player` dispatch four of those modes now
reuse; block legality (`staticability.go`, `CanBlock`) — flying/reach, Fear, Horsemanship, Intimidate, Landwalk, Menace
and every literal `S:Mode$ CantBlockBy` line; the legend rule's own `ignoreLegendRule` exemption (`staticability.go`) —
three slices of the general static-ability engine PORT-8 requires reading Java's own mechanism for rather than
hardcoding a keyword check; and four layers of the engine's biggest piece, `Mode$ Continuous` itself (`continuous.go`) —
`applyContinuousPT` resolves Layer 7b/7c's own `Affected$`-matched, plain-integer power/toughness lines (anthem effects,
equipment bonuses); `applyContinuousType` resolves Layer 4's own `AddType$`/`RemoveType$` lines naming only literal type
words; `applyContinuousColor` resolves Layer 5's own `AddColor$`/`SetColor$` lines naming a literal color, `All` or
`Colorless`; `applyContinuousKeyword` resolves Layer 6's own `AddKeyword$` lines naming only literal keyword lines (no
dynamic value, no `RemoveKeyword$`/`RemoveAllAbilities$`/`SharedKeywords$`/`FromDraftNotes$` combo) — the single largest
real slice of the four (1,556 of 1,857 real lines), folded through a new `KeywordMod` (`keywordmod.go`)
`Card.HasKeyword` now reads, reaching every existing keyword-driven check (`cantBlockByKeywords`, combat's own
first-strike/trample reads) for free — all four layers recomputed fresh every `CheckStateBasedActions` pass rather than
pushed once, `pt.go`'s own folding mechanism and its new `typemod.go`/`colormod.go`/`keywordmod.go` counterparts' first
real callers. `cardtype.Line` gained `ParseToken`/`Union`/`Without` to make Layer 4 possible without a
`*cardtype.Registry` this port still does not inject into the engine (`ParseToken`'s own doc comment); Landwalk's own
`ValidDefender$ Player.controls<Type>` needed a new `matchesValidDefender` (`staticability.go`), a `Player`, not a
`Card`, matched the same way `SpellCast`'s own `ValidActivatingPlayer` is. M6 in progress alongside it: `Draw`
(`draweffect.go`) is the first of the 203 script-driven effects to actually resolve rather than report
`ErrUnimplemented` — `Ability` gained a `Params` field (`ability.go`) carrying a trigger's own `Defined$`/`NumCards$`
onto the stack to make that possible. Full detail: `docs/crucible/00-master-implementation-plan.md` items 24-29,
`docs/crucible/porting/port-log/game-state.md`. Thin or missing: Layers 1-3 and 8, plus Layer 7a
(`CharacteristicDefining$`), need the rest of the same static-ability engine `Mode$ Continuous`'s own four slices do not
close, and Layers 4/5/6 themselves are only their safely-implementable literal-token subsets (a dynamic value or a
bulk-removal/`AddAllCreatureTypes$`/`SharedKeywords$` combo skips the whole line rather than applying it wrong); the
legend rule's Partner-non-legendary-name corner case; Protection and Skulk are `CantBlockBy` gaps of their own
(`game-state.md`'s "Block legality" section has the reason for each); `Attacks`'s own five unresolved params, `Blocks`'s
own `ValidBlocked$`, `DamageDone`'s own `DamageAmount$`/`ValidCause$`, `Discarded`'s own `ValidCause$`, `SpellCast`'s
own qualified `ValidActivatingPlayer$` forms, and every trigger mode past enters/dies/attacks/blocks/deals-damage/
is-discarded/casts; 202 script-driven effects past `Draw` still report `ErrUnimplemented`. **P4 exit gate's
fixture-count half met:** 342 scenarios (`testdata/scenarios/`) past the ≥300 floor; the qualitative half ("every layer,
every SBA," Plan Section 3.2) is not.
