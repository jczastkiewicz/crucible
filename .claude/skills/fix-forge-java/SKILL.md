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
2. **Rules first, for every row, before reading code.** Establish how Magic itself behaves in the situation the row
   describes and which rule governs it: search the web (WebSearch; WebFetch for the Comprehensive Rules text, Gatherer
   rulings and the card's Oracle text), quote the rule number (CR section) and the ruling, and name the card examples.
   Use `mode: "extended"` for a niche interaction. A crash or an unguarded index needs only a line saying no rule is
   involved. Keep what was found in the PR text (step 7) and the row. If the rules and the Java disagree, the rules win;
   if the rules are silent or ambiguous, the row becomes an issue, not a pull request.
3. **Re-read every site on `upstream/master`** (`git fetch upstream`, `git show upstream/master:<path>`). Line numbers
   move, and upstream may have fixed it already: then the row only needs updating on `master`. A row marked
   "Suspected" is proven by reading the Java before it is fixed; if the reading says it is not a bug, correct the row
   instead.
4. **Branch** `upstream-pr/<slug>` from `upstream/master`, from a clean tree (`git status` first; commit or stash the
   rest). The branch holds the tests and the fix and nothing else: no change under `crucible/`, `docs/` or `.claude/`.
5. **Test first, fix second, per site.** Before touching the Java, commit a test that reproduces the defect (JUnit under
   `forge-gui-desktop/src/test/java/forge/game/`, built on `RunTest` or the nearest existing test; a card from the corpus
   if one reaches the site, else a `SpellAbility` built from a script line). Run only that test
   (`mvn -pl forge-gui-desktop -am test -Dtest=<Class> -Dsurefire.failIfNoSpecifiedTests=false`) and see it fail for the
   defect's reason (the exception or wrong result the row names), not for a setup error. Then fix the smallest thing that
   is wrong: no refactor, no neighbouring cleanup, match the surrounding style. Run the test again and see it pass. One
   test commit then one fix commit per site, messages citing the file and line and why. If no test can reach the site,
   say so in the PR text and the row; never skip silently.
6. **Compile:** `mvn -pl forge-game -am compile -q` (add `forge-core`/`forge-gui` only when the touched file lives
   there). A failed compile is fixed before anything else; do not run the whole test suite, only the new tests.
7. **Draft the PR text** in English. A reviewer who has never seen the row must be able to follow it, so each site gets
   an analysis, not a one-liner:
   - **What the code does** in one or two sentences (the function, the expression it receives, what each variable holds).
   - **The mechanism of the defect** step by step, with a worked example using real numbers or a real card (input,
     each intermediate value, the wrong result, the right one). Show the old line and the new line.
   - **The rule or ruling** it breaks (CR number, card), or one line saying no rule is involved and why.
   - **Reach:** if a site is latent (the engine contains the defect but no card script triggers it today), say so in
     those words and state the evidence you ran: the `rg` over `forge-gui/res/cardsfolder` and its result (the
     cards that do reach the site, or "no line combines X and Y"). Never copy a corpus claim from the defects row
     without re-running the search; a search that timed out is not evidence, say it was not verified.
   - **The fix** in one sentence, and why it is the smallest one (name the alternative you rejected when there was
     one).
   - **The tests:** one line each on what it sets up, what it fails with on `master`, and any harness trick (a
     controller posing as a human, a counting controller that fails after N repeats instead of hanging).
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

Never force-push, never edit upstream files on `master` by hand (REV-1): the only upstream change on `master` is a
cherry-pick of a package that has a pull request, logged in `upstream-patches.md`.
