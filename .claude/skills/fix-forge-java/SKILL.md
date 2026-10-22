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

1. **Pick the package** the user named (or the next in the plan's order). A row under "Issues first" is never a pull
   request: draft an issue text and stop.
2. **Re-read every site on `upstream/master`** (`git fetch upstream`, `git show upstream/master:<path>`). Line numbers
   move, and upstream may have fixed it already: then the row only needs updating on `master`. A row marked
   "Suspected" is proven by reading the Java before it is fixed; if the reading says it is not a bug, correct the row
   instead.
3. **Branch** `upstream-pr/<slug>` from `upstream/master`, from a clean tree (`git status` first; commit or stash the
   rest). The branch holds the fix and nothing else: no change under `crucible/`, `docs/` or `.claude/`.
4. **Fix the smallest thing that is wrong.** No refactor, no neighbouring cleanup, match the surrounding style. One
   commit per site, message citing the file and line it fixes and why.
5. **Compile:** `mvn -pl forge-game -am compile -q` (add `forge-core`/`forge-gui` only when the touched file lives
   there). A failed compile is fixed before anything else; do not run the whole test suite.
6. **Draft the PR text** in English, as in the plan: per site one sentence on the defect and one on the fix. Show it to
   the user.
7. **Stop before publishing.** Push the branch to `origin` and open the PR only after the user approves that package.
8. **Back on `master`:** set each row's "Upstream" cell to the branch (then the PR number once it exists), commit with
   the docs. When a sync later brings the fix, make Go match upstream's version, remove its refusal and delete the row
   (DOC-16).

Never force-push, never edit upstream files on `master` (REV-1), never carry the fix on `master` unless Crucible needs it
now (then log it in `upstream-patches.md`).
