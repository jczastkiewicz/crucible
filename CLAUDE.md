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
cd crucible && go test -run TestScenarios ./internal/engine/game -update   # regen goldens, review the diff
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
(ADR-0013). Effect dispatch scaffolding (`Effect`/`Registry`, ADR-0011, ADR-0003 puts them in `internal/engine`) exists
but holds zero implementations — that is M6's job, not M4's or M5's.

M5 in progress (rules kernel). Done: turn/phase/step loop + priority (`turn.go`, `phase.go`), including CR 511.3's end
of combat cleanup (`endCombat`, wired as `CombatEnd`'s step body — real bookkeeping, needs none of the stack/triggers
the other bookkeeping-only steps wait on); zone changes + state-based actions (`action.go`), including an Aura's own
`Enchant` restriction against its still-present host, not just the host's presence (`enchantSpec`, CR 303.4a), and the
World rule (CR 704.5m, `resolveWorldRule`, no `PlayerController` needed — newest `Card.Timestamp` wins automatically);
combat (`combat.go`, `attack.go`, `block.go`, `combatdamage.go`), including a combat split across more than one
defending player at once (CR 506.4, `DeclareCombatBlockers` groups attackers by `defenderOf` and asks each defender in
turn); mulligans (`mulligan.go`); the `engine.Matches` valid-string evaluator (`valid.go`) that SBAs and future
targeting read, built corpus-frequency-first (`port-log/valid-strings.md`); a mana pool and payment for the plain
colored-and-generic case (`mana.go`) — CR 500.4's emptying between every phase/step, not just casting a spell; the
`CounterChanged` event, wired at every counter change this port can cause (`annihilateCounters`, `dealPermanentDamage`,
`Move`'s ETB grant) with a closed `CounterDetail` encoding (`event.go`) over the eight named `CounterType` constants;
`Game.PayManaCost` (`manapay.go`), which resolves `{X}` (CR 601.2b) via `ChoosePayX` — asked once per cost regardless of
how many `{X}` symbols it carries (CR 107.3f), folded into `Generic` as `x * CountX()` before anything else — snow
(`{S}`, CR 106.3a) via `ChoosePaySnow`, asked once per `{S}` symbol independently (unlike `{X}`, two can take two
different colors' snow mana) and spent through `Pool.PayWithSnow`'s own snow-only bucket, never the plain one, a
two-color hybrid shard (`{W/U}`) via `ChooseHybridManaColor`, a monocolored hybrid shard (`{2/W}`) via
`ChoosePayMonocoloredHybrid`, a colorless hybrid shard (`{C/W}`) via `ChoosePayColorlessHybrid`, a single-color
Phyrexian shard (`{W/P}`) via `ChoosePayPhyrexian`, a hybrid Phyrexian shard (`{B/G/P}`) via `ChoosePayHybridPhyrexian`
(either kind's paid life fires `LifeChanged`, `Source: NoCard`), and each unit of a cost's generic amount via
`ChoosePayGeneric` before handing the rest to `Pay`/`PayWithSnow` unchanged — mana payment's own eight harder shapes are
now all resolved; `TapLandForMana` (`manaability.go`), CR 305.6's intrinsic basic-land mana ability (Forge synthesizes
it from the type line rather than script text — `CardState.java`'s `getLandTraitChanges`/`getLandManaForColor` — so this
port keys off `cardtype.Line`'s subtypes the same way `enchantSpec`/`resolveWorldRule` do, and off the land's own Snow
supertype for whether the mana produced is snow), `Pool.Add`'s first real (non-test) caller; `Game.PlayLand`
(`land.go`), CR 305 — playing a land is not casting a spell, no cost and no stack, sorcery-speed timing collapsed to
active player, a main phase and an empty stack, one per turn via the new `Player.LandsPlayed`/`LandsPlayedLastTurn`
fields (`cleanupStep` rolls them forward for every player each turn, CR 500.4's own "every player" scope). Thin or
missing: the stack is push/resolve only — no simultaneous-trigger ordering, no replacement effects, nothing pushes an
ability onto it yet; the layer system is the CR 613 layer _numbers_ plus power/toughness folding only, not
types/colors/abilities; `CounterDetail` has no case for a script-written counter name, unreachable until a
`SpellAbility` can create one (M6). **P4 exit gate (scenario-parity harness, ≥300 fixtures) not met:** 36 fixtures exist
today (`testdata/scenarios/`).
