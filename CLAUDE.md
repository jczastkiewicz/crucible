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

M3 done — `internal/carddb/compile` compiles all 33,913 cards with no exemption; `internal/valid`, `expr`, `cost`,
`keyword` port the value grammars; `tools/apiscan` gates the param vocabulary two ways, both at zero; typed param
structs generated (`compile/params_gen.go`); the valid property vocabulary gate is green
(`TestEveryPropertyIsAccountedFor`, M3 item 19, `port-log/valid-strings.md`). **P2 exit gate green:** no unknowns,
allowlist empty, golden AST diff clean.

M4 done — `internal/engine/{game,card,player,zone,event,control}`; `PlayerController` (eleven decision methods) with
`ScriptedController`; `GameState` fixture load/dump, byte-identical round-trip (`internal/fixture`); event schema v1
(ADR-0013). Effect dispatch scaffolding (`Effect`/`Registry`, ADR-0011, ADR-0003 puts them in `internal/engine`) landed
holding zero implementations — M5's own `permanentEffect` (below) is the first two, but the 203 script-driven APIs in
corpus-frequency order were still M6's job, not M4's or M5's, until `Draw` (below) became the first to land.

M5 in progress (rules kernel). Real: turn/phase/step loop + priority (`turn.go`, `phase.go`); zone changes and
state-based actions (`action.go`) — both legend-rule corner cases, World rule, lethal damage, Battle protector,
dangling-attachment cleanup; combat (`combat.go`, `attack.go`, `block.go`, `combatdamage.go`) — first strike, trample,
gang blocking, a combat split across more than one defending player; mulligans (`mulligan.go`); the valid-string
evaluator (`valid.go`, `engine.Matches`), corpus-frequency-first (`port-log/valid-strings.md`); a mana pool and payment
covering all eight harder cost shapes (`mana.go`, `manapay.go`); a basic land's intrinsic mana ability and playing a
land (`manaability.go`, `land.go`); casting a spell — a non-Aura permanent or an Aura, through the stack —
(`castspell.go`); trigger firing (`trigger.go`) for most real `Mode$` kinds in the corpus, built in corpus-frequency
order (see `port-log/game-state.md` for the full per-mode breakdown), including CR 603.3b's own APNAP ordering
(`pushTriggeredAbilities`) and `CardTraitBase.meetsCommonRequirements`'s general gate; CR 614's replacement-effect
system (`replacement.go`) for `Moved`/`Untap`/`DamageDone`/`Draw`/`GainLife`; block legality (`staticability.go`) —
flying/reach/Fear/Horsemanship/Intimidate/Landwalk/Protection/Menace/Skulk and every literal `CantBlockBy` line;
continuous effects (`continuous.go`) — six real slices of `Mode$ Continuous` across all eight layers (several only
partially, see "Thin or missing" below), folded through `pt.go`/`typemod.go`/`colormod.go`/`keywordmod.go`/
`controlmod.go`/`rulesmod.go`; targeting (`targeting.go`) and SubAbility chaining (`subability.go`); last-known-
information (`Game.LKI`); activating an ability (`activateability.go`/`activatemanaability.go`) — fifteen
`cost.Cost.ActivationShape` primitives (mana; Tap; Sac/Exile/Return/Exert self-reference; Discard-N/PayLife/PayEnergy/
AddCounter/SubCounter; tapXType/Return-by-type; ExileFromGrave, self-discard and ExileFromHand for the graveyard/hand
`ActivationZone$` cases) and CR 606.3's loyalty-ability once-per-turn restriction.

M6 in progress alongside it: 12 of the corpus's 203 script-driven `Effect` APIs resolve rather than reporting
`ErrUnimplemented` — `Draw`, `DealDamage`, `GainLife`, `Pump`, `PumpAll`, `LoseLife`, `PutCounter`, `Discard`, `Scry`,
`Surveil`, `Sacrifice`, `SacrificeAll` — each with `UnlessCost$` and `SubAbility$` chaining wired through
`Registry.Resolve` (`effect.go`). `ChangeZone` (6,616 real corpus lines) is the single largest remaining gap; real
instant/sorcery casting, `DB$ Effect`/`Repeat`/`GenericChoice`/`DelayedTrigger`, and token creation are the next
largest.

Thin or missing: Layer 1 (copy effects — a separate resolution-time mechanism, not a continuous one); Layer 3 past one
real line (`GainTextOf$`); Layer 8's `MayLookAt$`/`MayPlay$`/`AddHiddenKeyword$`; most of Layers 4-6 past a literal
token list, and Layer 7a past the Valid-family SVar shape; the legend rule's own Corner Case 1 (needs a card-name lookup
across every card this game ever printed, which `*Game` holds no `*carddb.DB` reference to make); a real priority window
(`ResolveStack` plays out only the degenerate no-response case, so every activation's own timing check collapses to
"active player, a main phase, an empty stack"); most trigger-mode and replacement-family gaps `port-log/game-state.md`'s
own "Not ported yet" section lists in full.

Full detail — every primitive, every real corpus count, every design decision and its Java citation:
`docs/crucible/00-master-implementation-plan-in-progress.md` items 24-32,
`docs/crucible/porting/port-log/game-state.md`.

**P4 exit gate's fixture-count half met:** 342 scenarios (`testdata/scenarios/`) past the ≥300 floor; the qualitative
half ("every layer, every SBA," Plan Section 3.2) is not.
