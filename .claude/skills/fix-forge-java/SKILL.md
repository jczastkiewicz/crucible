---
name: fix-forge-java
description:
  Fix one package of Forge Java defects from docs/crucible/porting/java-fix-plan.md on an upstream-pr/<slug> branch off
  upstream/master, compile it, and prepare the pull request text. Use when asked to "fix the Java", "do package N",
  "start the Java fixes", or to take a row of forge-java-defects.md upstream.
---

# Fix a Forge Java defect package

PORT-8: the bug is fixed in Forge, never compensated in Go. The plan and the package list are
`docs/crucible/porting/java-fix-plan.md`; the rows are `docs/crucible/porting/forge-java-defects.md`. Work in the main
session: only research for a doubtful site goes to `forge-oracle`.

0. **Read `CONTRIBUTING.md` first, from `upstream/master`** (`git show upstream/master:CONTRIBUTING.md`), every time:
   it is upstream's rulebook and it changes. It binds this skill: the "Using AI coding agents" section (identify the
   agent as co-author or in the PR body; no unnecessary unit or wiring tests), the build and code-style notes, and the
   project hierarchy that says which module a change belongs in. If it contradicts a step below, CONTRIBUTING.md wins
   and the step gets fixed in this file.
1. **Pick the package** the user named (or the next in the plan's order). A row under "Issues first" is never a pull
   request: draft an issue text and stop.
2. **Rules first, for every row, before reading code.** Establish how Magic itself behaves in the situation the row
   describes and which rule governs it: search the web (WebSearch; WebFetch for the Comprehensive Rules text, Gatherer
   rulings and the card's Oracle text), quote the rule number (CR section) and the ruling, and name the card examples.
   Use `mode: "extended"` for a niche interaction. A crash or an unguarded index needs only a line saying no rule is
   involved. Keep what was found in the PR text (step 7) and the row. If the rules and the Java disagree, the rules win;
   if the rules are silent or ambiguous, the row becomes an issue, not a pull request.
   **Triage by reach before any work.** The maintainers (tool4ever, MostCromulent) call latent guards "borderline" and
   dislike "expect null everywhere": a site no corpus line reaches, or a null that only a broken client produces, is
   an issue or a drop, not a pull request. Ask where the bad value comes from (a controller, a client, a script) and fix
   there if that is the cause. Reviewer rules that apply to every package are listed under "What reviewers have asked
   for" below.
3. **Re-read every site on `upstream/master`** (`git fetch upstream`, `git show upstream/master:<path>`). Line numbers
   move, and upstream may have fixed it already: then the row only needs updating on `master`. A row marked
   "Suspected" is proven by reading the Java before it is fixed; if the reading says it is not a bug, correct the row
   instead.
4. **Branch** `upstream-pr/<slug>` from a freshly fetched `upstream/master` (maintainers batch-fix scripts: a card may
   already be fixed, as Wilt-Leaf Liege and Volcanic Offering were by #12477), from a clean tree (`git status` first; commit or stash the
   rest). The branch holds the tests and the fix and nothing else: no change under `crucible/`, `docs/` or `.claude/`.
5. **Reproduce first, fix second, per site, and do not commit the test.** CONTRIBUTING.md: agents add unnecessary unit
   or wiring tests, avoid them unless needed to catch a future integration regression; reviewers reject tests for
   trivial sanity checks and for card-script fixes. Reproduce the defect with a throwaway JUnit test (under
   `forge-gui-desktop/src/test/java/forge/game/`, built on `RunTest` or the nearest existing test; a card from the
   corpus if one reaches the site, else a `SpellAbility` built from a script line), run only that test
   (`mvn -pl forge-gui-desktop -am test -Dtest=<Class> -Dsurefire.failIfNoSpecifiedTests=false`), see it fail for the
   defect's reason, fix the smallest thing that is wrong (no refactor, no neighbouring cleanup, match the surrounding
   style), see it pass, then delete the test before committing. Commit only the fix: one commit per site, message
   citing the file and line and why. Keep a test only when the logic is non-trivial engine behaviour (a rules
   calculation, not a null or index guard) and say in the PR text why it is needed.
6. **Compile:** `mvn -pl forge-game -am compile -q` (add `forge-core`/`forge-gui` only when the touched file lives
   there). A failed compile is fixed before anything else; do not run the whole test suite, only the new tests.
7. **Draft the PR text** in English, short. Reviewers have told us a simple change does not need 400 words. Per site:
   one or two sentences on what fails and when (the expression, a real card or numbers if it is not obvious), the rule
   or ruling only when one is involved (CR number, one line; otherwise omit), and the fix in one sentence. Under 150
   words per site and under 300 for the whole PR unless the logic is genuinely hard. State **reach** in one line,
   with the `rg` you ran ("latent: no line combines X and Y"); never copy a corpus claim from the defects row. No
   section on tests (there are none). End with the coding-agent line from CONTRIBUTING (the footer
   "🤖 Generated with [Claude Code](https://claude.com/claude-code)" and the `Co-Authored-By` commit trailer).
   Show it to the user. A PR already open gets its description updated with `gh pr edit <n> --repo Card-Forge/forge
   --body-file <file>` (switch the active `gh` account to the personal one for the call, then back).
8. **Stop before publishing.** Push the branch to `origin` and open the PR only after the user approves that package.
9. **Carry the fix on `master`, always.** Back on `master`, `git cherry-pick -n` the package's fix commits (the
   `Fix ...` commits, not the test commits; the tests arrive with the upstream merge) so Crucible's Java oracle runs
   the fixed behaviour. One commit per package, in the same commit add one row per touched file to **Pending upstream
   fixes** in `docs/crucible/porting/upstream-patches.md` (date, path, change, PR number and branch). The row and the
   local edit are deleted together once upstream merges the PR and a sync brings the identical content back; on a
   conflict take upstream's version. `guard-upstream.sh` only watches Write/Edit, so a cherry-pick passes it, but the
   log row is still required (REV-1). Compile with `mvn -pl forge-game -am compile -q` on `master` before committing.
10. **Defect rows:** set each row's "Upstream" cell to the branch (then the PR number once it exists), commit with the
    docs. When a sync later brings the fix, make Go match upstream's version, remove its refusal and delete the row
    (DOC-16).

## What reviewers have asked for

Collected from reviews of our pull requests, on top of `CONTRIBUTING.md` (step 0); apply them before showing a package to the user.

| Reviewer said | Do |
| --- | --- |
| "we really don't need tests for trivial sanity checks"; CONTRIBUTING: no trivial tests for simple script changes | Step 5: no committed test unless non-trivial engine logic |
| A simple change needs far fewer words than ~400 | Step 7: short body |
| "borderline", "not a fan of expecting null everywhere", "none seem high priority" | Triage by reach; look for the root cause; drop or turn latent guards into an issue |
| "why not simply `RememberDestroyed$`?" | Before writing a multi-line script workaround, check whether the effect already has a param for it (`git grep` the effect class) |
| Two or three new PRs for one review comment make review harder | Never split or duplicate a PR in answer to a comment; update the open one |
| Maintainers cannot always approve (a helper has no merge rights) | Do not post "please approve/merge" pings; wait for the maintainer |

Rebase an open PR onto the current `upstream/master` (one clean commit per site) rather than merging upstream into the
branch; the user force-pushes with `--force-with-lease`, never Claude. A reviewer's decision to close a package ends
it: remove its carried edit from `master` and its Pending rows in the same commit.

Never force-push, never edit upstream files on `master` by hand (REV-1): the only upstream change on `master` is a
cherry-pick of a package that has a pull request, logged in `upstream-patches.md`.
