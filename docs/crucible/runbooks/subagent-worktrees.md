# Runbook — Subagents in worktrees

- **Status:** Active
- **Applies to:** every subagent spawned with `isolation: "worktree"` (`port-batch` skill, `effect-porter*`, batch
  prompts written by hand)

Failures seen in M5 batches A-I, each with the rule that avoids it. Put the **Prompt block** at the end into every
worktree-subagent prompt that is not an `effect-porter*` definition (those already carry it).

---

## Rules

| Failure                                                                                           | Rule                                                                                                                                       |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Every `git` refused: the user's global `rtk` hook rewrites it to `rtk git`, the guard refuses it  | Never run `git`, never bypass (`/usr/bin/git`, `-C`, `env`, `command`, `rtk proxy`). Leave work uncommitted. Main session commits (below)  |
| Bypass commits skipped the commit hook's gates                                                    | Main session commits with `crucible/scripts/merge-porters.sh commit BRANCH MSG`; rerun `gates.sh full` on the merged result                |
| Compound commands refused as "too complex to verify": `cd && ...`, heredocs, loops, inline python | One simple command per call. Write files with the Write/Edit tool, never a heredoc. Script = Write to the scratchpad, then run it          |
| BSD `sed -i` misuse on macOS (`sed -i s/...` eats the next argument)                              | No `sed -i`. Use the Edit tool, or `sed -i '' ...`                                                                                         |
| Recursive greps over `forge-gui/res/cardsfolder` (34k files) timed out                            | `rg -l` scoped to a letter directory, or `crucible/scripts/unported-apis.sh`; never `grep -r` over the whole corpus                        |
| `rtk golangci-lint` not on PATH                                                                   | `crucible/scripts/ensure-golangci.sh -check` prints the binary; `gates.sh` uses it                                                         |
| Two agents' gates collided: "parallel golangci-lint is running"                                   | `gates.sh` passes `--allow-parallel-runners`; if it still fails, rerun once                                                                |
| Scratch scripts overwritten by another agent                                                      | The scratchpad is shared: prefix every scratch file with the agent's own name or branch                                                    |
| `gates.sh` blocked at `genregistry`: a new API changes the count, docs said the old one           | A porter that adds an effect updates the resolved-API count in `CLAUDE.md` and `00-master-implementation-plan-in-progress.md` itself       |
| enginelint failed twice per new file (ungrouped file, cross-group reference)                      | New engine file: add its group and allow-list in `enginelint.json` in the same step, and run `go run ./tools/enginelint` before `gates.sh` |
| covergate failed at 89.8-89.9% (`internal/engine` floor is 90.0%)                                 | The floor is tight: every new function ships with a module test, error paths included. Run `gates.sh full` before the final message        |
| Stale fail-closed and golden tests after a shape became supported                                 | Grep the tests for the old "unsupported" example; regenerate goldens and diff them                                                         |
| Runs died when the machine slept (stream watchdog) or hit the session limit (429)                 | Not fixable in config. Worktree state survives; resume the agent with `SendMessage`, or start a new one in the same worktree directory     |

---

## Prompt block

```text
You are in a git worktree. Never run git and never bypass the refusal (no /usr/bin/git, -C, env, command, rtk proxy):
leave all work uncommitted, the main session commits it. One simple shell command per call: no cd &&, heredocs, loops
or inline scripts; use Write/Edit for files and no sed -i. No recursive grep over forge-gui/res/cardsfolder: use rg -l
on a letter directory. Prefix scratch files with your branch name (the scratchpad is shared). New engine file: add its
enginelint group first. If you add an effect, update the resolved-API count in CLAUDE.md and the in-progress plan. Run
gates.sh fast after each piece and gates.sh full before your final message (covergate floor is 90.0%: test error paths).
Final report: what landed, what did not and why, exact "Not ported yet" rows to delete or reword, stale docs noticed,
commands that failed and why.
```
