# TOOL — Claude Code Setup Plan

- **Status:** Draft
- **Applies to:** `.claude/` at repo root, `/CLAUDE.md`, docs Claude reads every session
- **Rule IDs:** cite as `TOOL-n`

Plan for making Claude Code enforce Crucible rules mechanically instead of from memory. Today `.claude/` holds no
settings, hooks, skills or agents; every rule lives only in `/CLAUDE.md` and `guidelines/`, so each gate runs only when
Claude remembers it.

---

## Why this exists

| Symptom                                                                          | Cost                                                               |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| REV-1 (no upstream edits) is a sentence in `CLAUDE.md`, nothing blocks it        | One slip = future rebase conflict                                  |
| Seven CI gates listed in `CLAUDE.md`, run by hand                                | Red gate found at CI, not at edit time                             |
| "Adding an M6 effect" checklist loads every session, used only when porting      | Context spent on sessions that never touch effects                 |
| `porting/port-log/game-state.md` = 6,719 lines, edited by every effect           | Large reads per effect; `grep -c '^\| ---'` = `6` check is fragile |
| `enginelint.json` = 1,314 lines, `NewRegistry()` in `castspell.go` hand-extended | Every parallel effect port conflicts on the same three files       |
| Java semantics research happens in main context                                  | `forge-game/` reads crowd out Go work in the same session          |

---

## TOOL-1 — Hooks enforce gates at edit time

Hooks run in the harness, not the model. Rule cannot be forgotten.

Implemented: `.claude/settings.json` (committed), scripts in `.claude/hooks/`, gate runner `crucible/scripts/gates.sh`.

| Hook                                  | Action                                                                          | Blocks | Rule enforced |
| ------------------------------------- | ------------------------------------------------------------------------------- | ------ | ------------- |
| `PreToolUse` on Bash `git commit`     | `gates.sh full`: every CI gate incl. `golangci-lint`, `go test -race`, prettier | yes    | CI parity     |
| `PostToolUse` on `Edit\|Write` `*.go` | `gofmt -s -w <file>`                                                            | no     | GO-1          |
| `PostToolUse` on `Edit\|Write` `*.md` | `prettier --write <file>`                                                       | no     | DOC-14        |
| `Stop`                                | `gates.sh fast` when `crucible/` or `docs/crucible/` dirty; `systemMessage`     | no     | early warning |
| `PreToolUse` on `Edit\|Write`         | Reject path outside `crucible/`, `docs/crucible/`, `.claude/`, `/CLAUDE.md`     | yes    | REV-1 — to do |

Measured: `fast` `~16 s`, `full` `~80 s` (`go test -race` on `internal/engine` alone `~42 s`). `full` too slow for every
`Stop`, so commit is the hard gate. `Stop` reports only: blocking loops on a failure Claude cannot fix in-turn.

Why commit hook exists: CI run `35962658352` failed on 33 `revive` findings. `golangci-lint` was CI-only, nothing ran it
before push.

Upstream `.gitignore:107` ignores `.claude`. Files there are force-added (`git add -f`), not un-ignored. Reason: editing
`.gitignore` is an upstream patch (REV-1).

Upstream-path hook allows edits listed in `porting/upstream-patches.md` only via explicit user approval, never silently.
Reason: PORT-8 fixes upstream are legitimate but must be logged in same commit.

---

## TOOL-2 — Skills replace per-task checklists

Skill loads on demand. `CLAUDE.md` keeps one line pointing at it.

| Skill            | Contents                                                                                                 | Replaces                                      |
| ---------------- | -------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| `port-effect`    | `<api>effect.go` template (param rejection, `subAbilityConditionMet`), 2-player test template, doc steps | `CLAUDE.md` "Adding an M6 effect"             |
| `port-java-unit` | Locate Java class → apply PORT-n → port-log note → `module-map.md` row                                   | Reading `02-java-to-go-translation.md` ad hoc |
| `add-scenario`   | Scaffold `testdata/scenarios/<case>/{setup.state,actions.log,expect.state,expect.events}`, run on oracle | TEST-5 by hand                                |
| `gates`          | Every CI gate in CI order, failures summarised                                                           | Seven command lines in `CLAUDE.md`            |

Templates live in skill directory, not in repo docs. Reason: DOC-11 — guideline stays source of truth, skill links to
it.

---

## TOOL-3 — Subagents isolate heavy reading

Implemented in `.claude/agents/`. Main session delegates by name ("use forge-oracle on SetState").

| Agent            | Model  | Tools                       | Job                                                                                     |
| ---------------- | ------ | --------------------------- | --------------------------------------------------------------------------------------- |
| `forge-oracle`   | Sonnet | Read, Grep, Glob, Bash (ro) | API name in; Java params, resolution order, corpus shapes, PORT-7/8 quirks, `path:line` |
| `rules-reviewer` | Opus   | Read, Grep, Glob, Bash (ro) | Diff against GO-2/3/4/7/8/9/12, PORT-2/8, TEST-1/8, REV-1, DOC-12; findings only        |
| `gate-runner`    | Haiku  | Bash, Read                  | `gates.sh fast\|full` or one `go test -run`; failures verbatim, max 15 lines each       |

Model per agent: research needs reading judgment (Sonnet); review misses are costly (Opus); running a script and
trimming output is mechanical (Haiku). `(ro)` = prompt restricts Bash to read commands; not enforced by harness.

Reason: Java reads and test logs are large, disposable. Main context keeps Go work only.

---

## TOOL-4 — Split `game-state.md`

Move per-effect sections to `porting/port-log/effects/<api>.md`. `game-state.md` keeps primitives, index table and
`## Not ported yet`.

| Before                                         | After                                        |
| ---------------------------------------------- | -------------------------------------------- |
| 6,719 lines, every effect port edits same file | One new file per effect, index row in parent |
| Table-count check `grep -c '^\| ---'` = `6`    | Check dropped; `docgate` verifies index rows |
| Exceeds DOC-10 limit (~400 lines) 16×          | Each file under limit                        |

Needs `docgate` change to accept `effects/` directory. Lands in same commit (DOC-12).

---

## TOOL-5 — Remove merge hot spots for parallel ports

Effects are mostly independent. Parallel worktree agents (one API batch each) collide only on:

| File                                     | Conflict                        | Fix                                                                     |
| ---------------------------------------- | ------------------------------- | ----------------------------------------------------------------------- |
| `castspell.go` `NewRegistry()` + comment | Every effect appends lines      | Generate `registry_gen.go` from `*effect.go` via `go generate`          |
| `enginelint.json`                        | Every effect adds group         | One file per group under `enginelint.d/`, or generator from file header |
| Remaining-API counts in 3 docs           | Every effect bumps same numbers | Generate count from registry; docs cite command, not number             |

`NewRegistry()` explicit wiring is required by ADR-0003. Generated file keeps wiring explicit and visible (output is
committed Go), but change still needs ADR amendment before implementation (ADRP-4).

---

## TOOL-6 — Small items

| Item                                                | Gain                                                                                       |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `/fewer-permission-prompts` → allowlist in settings | `go test`, `go run ./tools/...`, `prettier`, `mvn -pl crucible/oracle-java` run unprompted |
| gopls LSP plugin (`gopls` not installed locally)    | Go-to-definition, diagnostics instead of grep                                              |
| `crucible/scripts/gates.sh`                         | One command for hooks, skill and humans; `CLAUDE.md` shrinks                               |
| Commit per effect batch                             | Working tree now holds ~50 uncommitted files; bisect and review suffer                     |

---

## Order

| #   | Step                                     | Effort | Depends on |
| --- | ---------------------------------------- | ------ | ---------- |
| 1   | `scripts/gates.sh` done                  | S      | —          |
| 2   | TOOL-1 hooks done, REV-1 guard to do     | S      | 1          |
| 3   | TOOL-6 permission allowlist              | S      | —          |
| 4   | TOOL-2 `port-effect` + `gates` skills    | M      | 1          |
| 5   | TOOL-3 subagents done                    | S      | —          |
| 6   | TOOL-4 `game-state.md` split + `docgate` | M      | —          |
| 7   | ADR amendment for generated registry     | S      | —          |
| 8   | TOOL-5 generators                        | L      | 7          |
| 9   | Remaining TOOL-2 skills                  | M      | 4          |

Steps 1-4 give most value per hour: rules enforced by harness, `CLAUDE.md` shorter.

---

## Related

- [/CLAUDE.md](../../../CLAUDE.md)
- [guidelines/README.md](../guidelines/README.md)
- [ADR-0003](../adr/0003-go-project-layout.md)
- [port-log/game-state.md](../porting/port-log/game-state.md)
