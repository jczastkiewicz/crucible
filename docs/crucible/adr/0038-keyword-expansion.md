# ADR-0038 — Keyword Expansion: Script-Expressible Keywords Compile Into Traits, the Rest Stay Rules Reads

- **Status:** Accepted
- **Date:** 2026-10-01
- **Deciders:** Crucible session (M5/M6 rules kernel)

## Context

18,245 `K:` lines carry 252 distinct heads. Java turns about 204 of them into traits when a card is built:
`CardFactoryUtil.addTriggerAbility` (`CardFactoryUtil.java:585`), `addReplacementEffect` (`:2041`), `addSpellAbility`
(`:2614`) and `addStaticAbility` (`:3749`) each build the keyword's `T:`/`R:`/`A:`/`S:` script text and parse it, for a
printed keyword and for a granted one alike (`KeywordInterface.createTraits`). Crucible reads a keyword only as "is the
bare word present" (`Card.HasKeyword`, `port-log/keywords.md`), so a keyword that stands for a trait does nothing:

| Keyword (`K:` lines)                                              | What Java builds                                      | Crucible today                     |
| ----------------------------------------------------------------- | ----------------------------------------------------- | ---------------------------------- |
| Equip (650)                                                       | `AB$ Attach` at sorcery speed                         | an Equipment can never be attached |
| Cycling (306), TypeCycling (106)                                  | `AB$ Draw` from hand, discard-self cost               | no cycling                         |
| Flashback (217), Unearth (58), Echo (52), Cumulative upkeep (80)  | cast-from-graveyard, upkeep and exile triggers        | nothing                            |
| Prowess (104), Exalted (35), Persist/Undying, Evolve, Extort, ... | `T:Mode$ SpellCast`/`Attacks`/`ChangesZone` triggers  | nothing                            |
| Lifelink (390), Infect (45), Wither, Toxic (44), Flash (638)      | no script: the damage and cast-timing code reads them | native reads, done                 |
| Kicker (239), Convoke (106), Delve, Affinity (77), Buyback (40)   | casting-cost shape, not a trait                       | open (casting model)               |

Two accepted ADRs disagree on how a keyword becomes behavior. ADR-0007 says "keyword-synthesised abilities are
compile-time work": `compileFace` already expands `Dungeon` and `ETBReplacement` that way. ADR-0028 built Ward natively
inside the engine from `KeywordLines`, because Ward's trigger needed a `wardCounters` field no script line carries. A
third route (a per-keyword `if HasKeyword` in the engine) is how Shadow, Lifelink and Flash were done, and is right only
where Java reads the keyword at one point instead of building a trait.

## Decision Drivers

- PORT-2: scripts compile once at load; no keyword text is parsed at runtime.
- ADR-0023: a granted trait is a compiled trait in a timestamped overlay. A granted keyword (`AddKeyword$ Prowess`) must
  behave like a printed one, and Layer 6 can remove it again.
- GO-4/GO-8: no reflection, typed params.
- The golden AST (`TestCorpusAST`) is the review surface for what a card compiles to.

## Considered Options

1. **Native engine code per keyword**, as Ward. Rejected for script-expressible keywords: 200 hand-written triggers and
   abilities that Java gets from one template each, with granted keywords needing a second mechanism.
2. **Expand at runtime from `KeywordLines`**, as Java. Rejected: PORT-2.
3. **Expand at compile time into the face's trait slices; expand a granted keyword to the same compiled traits at load
   and hand them to ADR-0023's overlay.** Chosen for the script-expressible class.

## Decision

1. **Classes.** A keyword is _expressible_ when `CardFactoryUtil` builds its behavior from a script string (Equip,
   Cycling, Flashback, Prowess, ...). It is _a rules read_ when Java reads the bare keyword at one point (`Lifelink` in
   `GameAction.dealDamage`, `Flash` in `SpellAbility.withFlash`). It is _casting infrastructure_ when it changes how a
   spell is cast or paid (Kicker, Convoke). Only the first class expands; the other two stay native reads, and casting
   infrastructure gets its own decision.
2. **One expander.** `internal/keyword` gains `Expand(Keyword) []Template`, a port of the `CardFactoryUtil` branches
   that returns the same `Key$ Value` script text Java builds, plus the SVars it names, keyed by keyword head. Adding a
   keyword is adding a template and its test; a head with no template stays inert and is listed by the coverage command
   (GO-7), never silently dropped.
3. **Printed keywords expand in `compileFace`**, appended to `Face.Abilities`/`Triggers`/`Statics`/`Replacements` after
   the card's own lines (Java's order: `setupKeywordedAbilities` runs after `addAbilityFactoryAbilities`). Each
   synthesized `compile.Ability` records the keyword line it came from (`Ability.Keyword`), which is what makes a
   removal (`RemoveKeyword$`, `RemoveAllAbilities$`) and the golden diff readable.
4. **Granted keywords** use ADR-0023's overlay: `compile` expands every distinct keyword line a grant names
   (`AddKeyword$` and the keyword params of `KW$`, `Keywords$`) at load into the same compiled traits, and the overlay
   entry carries them. No expansion happens at runtime. A grant naming a keyword with no template fails closed when
   applied (ADR-0023 decision 4).
5. **Ward stays native** (ADR-0028): its trigger needs state no script line carries. A keyword that needs the same gets
   the same treatment, with its own ADR.
6. **Order of work**, by corpus count and by how much a gauntlet notices: Equip, Cycling, Flashback, Prowess, then
   Exalted/Persist/Undying/Evolve/Extort, then the upkeep-cost group (Echo, Cumulative upkeep). Each lands with the
   golden regenerated in the same commit and a scenario fixture.

## Consequences

**Good:** one mechanism for printed and granted keywords; ADR-0007 and ADR-0023 stop disagreeing with the code; each
keyword is a template plus a fixture, and the golden shows exactly which cards it touches (Equip alone is 650 cards).
**Bad:** the golden AST diff is large per keyword; a mistranscribed template misbehaves on every card that prints the
keyword, so each needs a Java-oracle scenario. `compile` grows a dependency on `internal/keyword`'s template table.
**Neutral:** `HasKeyword` stays the read for rules-read keywords, so both access paths coexist by design.

The implementing PRs add `keyword.Expand`, the `compileFace` call, `Ability.Keyword`, and the coverage-command report.

## Related

ADR-0007, ADR-0023, ADR-0028, [`keywords.md`](../porting/port-log/keywords.md),
[`card-compilation-pipeline.md`](../design/card-compilation-pipeline.md)
