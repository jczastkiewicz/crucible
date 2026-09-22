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

M5 in progress (rules kernel). Done: turn/phase/step loop + priority (`turn.go`, `phase.go`); zone changes + state-based
actions (`action.go`) — legend rule, World rule, lethal damage, Battle protector, dangling-attachment cleanup; combat
(`combat.go`, `attack.go`, `block.go`, `combatdamage.go`) — first strike, trample, gang blocking, a combat split across
more than one defending player; mulligans (`mulligan.go`); the valid-string evaluator (`valid.go`, `engine.Matches`)
SBAs and targeting both read, built corpus-frequency-first (`port-log/valid-strings.md`), including `SharesColorWith`'s
own bare form; a mana pool and payment covering all eight harder cost shapes (`mana.go`, `manapay.go`); a basic land's
intrinsic mana ability (`manaability.go`) and playing a land (`land.go`); casting a spell — a non-Aura permanent or an
Aura, through the stack — (`castspell.go`), the first two real `Effect` implementations
(`permanentEffect`/`attachEffect`); trigger firing (`trigger.go`) — "enters," "dies," "attacks," "blocks," "becomes
blocked," "becomes blocked by a creature," "deals damage," "is discarded," "becomes tapped," "taps for mana," "a player
casts a spell," "the beginning of a step or phase," "a player attacks" and "a player draws a card" (`checkPhaseTriggers`
— CR 500, `Mode$ Phase`, the corpus's own SECOND most frequent trigger mode at 2,362 real lines, ahead of `Attacks`
itself, resolved once corpus-frequency research found it well after the first nine modes had already landed; unlike
every other mode, it walks four zones — `Battlefield`/`Command`/`Graveyard`/`Exile` (`phaseTriggerZones`) — not
`Battlefield` alone, and matches `ValidPlayer$` against the active player, not the trigger's own host controller;
`checkAttackersDeclaredTrigger` — CR 508.1, `Mode$ AttackersDeclared`, 286 real lines, fires once per combat rather than
once per attacker the way `Attacks` itself does, reusing `phaseTriggerZones`'s own four-zone walk and a new
`attackedTargetMatches`/`validAttackersCountMatches` pair for `AttackedTarget$`/`ValidAttackers$`; `checkDrawnTriggers`
— CR 120.3, `Mode$ Drawn`, 161 real lines, called from `DrawCards`' own per-card loop (turn.go, already written that way
before this mode existed to consume it) with a new `Player.CardsDrawnThisTurn` counter — `LandsPlayed`'s own
per-turn-counter shape — for `Number$`'s own "the Nth card you draw each turn" real corpus shape), plus another
permanent watching one do any of those — detects and queues a trigger, pushed through a new `pushTriggeredAbilities`
(`trigger.go`) that ports CR 603.3b's own APNAP ordering for the first time (`playersInAPNAPOrder` —
`Game.ActivePlayer()`, then turn order — walking each player's own group in turn, so the non-active player's own trigger
resolves before the active player's when more than one fires off the same event, the stack's own LIFO order applied to
`MagicStack`'s own player-iteration sequence); every trigger-check function now collects its own matches first and calls
it once, rather than calling `PushAbility` the instant each match is found. CR 614.1's own replacement-effect system has
its first real content too: `checkMovedReplacement` (`replacement.go`) resolves the corpus's single largest real
`Event$ Moved` shape — a permanent entering the battlefield already tapped, `ReplaceWith$` naming a bare `DB$ Tap` (618
of 969 real `Moved` lines, 618 of 2,210 real replacement lines corpus-wide) — called from every real "enters the
battlefield" site (`permanentEffect`/`attachEffect`, castspell.go; `Game.PlayLand`, land.go) before `checkETBTriggers`
runs, CR 614.1's own ordering over CR 603. CR 509.2's own "becomes blocked" family is real now too —
`checkAttackerBlockedTriggers` (`Mode$ AttackerBlocked`, 127 real lines, fires once per attacker with its whole blocker
group gathered, `ValidBlocker$`/`ValidBlockerAmount$` counted via a new `validCardsCountMatches` —
`validAttackersCountMatches`'s own shape generalized past `g.combat.Attackers`) and
`checkAttackerBlockedByCreatureTriggers` (`Mode$ AttackerBlockedByCreature`, 102 real lines, `checkBlocksTriggers`'s own
exact mirror image — `ValidCard$` against the attacker, `ValidBlocker$` against one blocker, fired per Block the
identical per-pair granularity `checkBlocksTriggers` already has), both called from `DeclareCombatBlockers` (block.go)
alongside it. `CardTraitBase.meetsCommonRequirements` — Java's own general gate checked before ANY trigger mode's own
`performTest` runs — is real now too, closing 1,148 of ~1,271 real T: lines carrying at least one of its params, folded
into `triggerEffectAPI` itself (`trigger.go`) rather than duplicated at all eighteen check-triggers call sites:
`IsPresent$`/`PresentCompare$`/`PresentZone$`/`PresentPlayer$` and the identical `IsPresent2$` pair (624, a zone-scan
`Matches` count, `PresentDefined$` unresolved); `CheckSVar$`/`SVarCompare$` (474, both sides through a new
`resolveNamedAmount`, `amount.go`, `ptParam`'s own shape now shared); `Metalcraft$`/`Delirium$`/`Threshold$`/
`Hellbent$`/`FatefulHour$` as a True/False flag (38, reusing `continuousConditionMet`'s own predicates, moved to a new
`playerstate.go` so neither file owns them); `LifeTotal$`/`LifeAmount$` (12). `Revolt$`/`WerewolfTransformCondition$`/
`WerewolfUntransformCondition$`/`CheckDefinedPlayer$`/`ManaSpent$`/`ManaNotSpent$` (118) stay unresolved, each its own
untracked mechanic. `matchesPlayerBase` (`valid.go`) is the shared `You`/`Opponent`/`Player` dispatch several of those
modes now reuse; block legality (`staticability.go`, `CanBlock`) — flying/reach, Fear, Horsemanship, Intimidate,
Landwalk, Protection, Menace and every literal `S:Mode$ CantBlockBy` line; the legend rule's own `ignoreLegendRule`
exemption (`staticability.go`) — three slices of the general static-ability engine PORT-8 requires reading Java's own
mechanism for rather than hardcoding a keyword check; and four layers of the engine's biggest piece, `Mode$ Continuous`
itself (`continuous.go`) — `applyContinuousPT` resolves Layer 7b/7c's own `Affected$`-matched, plain-integer
power/toughness lines (anthem effects, equipment bonuses); `applyContinuousType` resolves Layer 4's own
`AddType$`/`RemoveType$` lines naming only literal type words; `applyContinuousColor` resolves Layer 5's own
`AddColor$`/`SetColor$` lines naming a literal color, `All` or `Colorless`; `applyContinuousKeyword` resolves Layer 6's
own `AddKeyword$` lines naming only literal keyword lines (no dynamic value, no
`RemoveKeyword$`/`RemoveAllAbilities$`/`SharedKeywords$`/`FromDraftNotes$` combo) — the single largest real slice of the
four (1,556 of 1,857 real lines), folded through a new `KeywordMod` (`keywordmod.go`) `Card.HasKeyword` now reads,
reaching every existing keyword-driven check (`cantBlockByKeywords`, combat's own first-strike/trample reads) for free —
all four layers recomputed fresh every `CheckStateBasedActions` pass rather than pushed once, `pt.go`'s own folding
mechanism and its new `typemod.go`/`colormod.go`/`keywordmod.go` counterparts' first real callers. `cardtype.Line`
gained `ParseToken`/`Union`/`Without` to make Layer 4 possible without a `*cardtype.Registry` this port still does not
inject into the engine (`ParseToken`'s own doc comment); Landwalk's own `ValidDefender$ Player.controls<Type>` needed a
new `matchesValidDefender` (`staticability.go`), a `Player`, not a `Card`, matched the same way `SpellCast`'s own
`ValidActivatingPlayer` is; Protection's own CantBlockBy restriction (`protectionValid`, `staticability.go`) is built
per card from the keyword's own argument the identical way Landwalk's is, both real corpus shapes (the natural-language
"Protection from red" and the colon-structured "Protection:Artifact") resolving through the existing valid-string
evaluator with no new property needed; `Blocks`'s own `ValidBlocked$` is checked against the declared attacker directly,
the per-pair granularity `checkBlocksTriggers` already has standing in for Java's own full-attacker-collection match;
and `matchesPlayerSpec`/`matchesPlayerProperty` (`valid.go`) extend `matchesPlayerBase` with the one dotted-property
layer (`Active`/`NonActive`/`Other`, `Game.ActivePlayer()`) real corpus lines put on top of it, closing 19 of
`SpellCast`'s own 25 qualified `ValidActivatingPlayer$` lines plus `DamageDone`'s own qualified `ValidTarget$` and
`TapsForMana`'s own qualified `Activator$`. A new `compile.Face.Amounts` (compile.go) parses every non-ability SVar a
face defines (`internal/expr`) at compile time, and a new `resolveAmount` (`amount.go`) evaluates the one family of it
this port's own `Matches` already can, `Count$Valid[<Zone>...] <spec>` — 2,804 of the corpus's 6,186 real `Count$`
expressions — closing `ptParam`'s own dynamic-`AddPower$`/`AddToughness$`/`SetPower$`/`SetToughness$` gap for that shape
and, with it, Layer 7a itself: `applyOneCharacteristicDefiningPT` (`continuous.go`) resolves a
`CharacteristicDefining$ True` line's own `SetPower$`/`SetToughness$` and applies the result to its host alone, at
`LayerCharacteristic` — a layer `PTEffect`'s own folding already carried, unused until now. Skulk's own
`ValidBlocker$ Creature.powerGTX` closes block legality's last `CantBlockBy` gap: `skulkBlocks` (`staticability.go`)
reads Java's own hardcoded `X` (`Count$CardPower` against the ability's own host, always the attacker) as a direct
`Power()` comparison rather than a `Compare`/SVar question at all, the same hardcoded-comparison shape `menaceLegal`
already has for Menace. `hostRefusesEnchant` (`staticability.go`) closes the "cleanup aura" rule's own Protection/bare
Hexproof gap — reusing `protectionValid` against the aura itself rather than a candidate blocker, plus bare Hexproof's
own unconditional "any opponent" form — checked both when an Aura is cast (`enchantTargets`, `castspell.go`) and on
every ongoing SBA pass (`cleanupDanglingAttachments`, `action.go`); building it surfaced a real, separate gap
(`protectionValid`/`landwalkType` read only a card's PRINTED keywords, missing one a continuous effect grants), closed
by a new `Card.KeywordLines` (`card.go`) both now share with `HasKeyword`. A qualified Hexproof (`Hexproof:Black`,
`Hexproof:Enchantment`, ...) resolves too now — `hexproofValidSource` (`staticability.go`) ports
`KeywordWithType.parse`'s own bare-color-word case (`Black` becomes `Card.Black` before it ever reaches `Matches`, since
a bare color name is not itself a recognized valid-string base) alongside its bare-type fallthrough (`Enchantment` stays
as-is, an ordinary type check); the ability-source shape (`Hexproof:Triggered`, `Hexproof:Activated`, `ValidSA$` in
Java) still refuses rather than resolves, since `Matches` never evaluates a `SpellAbility`. `Attacks`'s own `Alone$`,
`DefendingPlayerPoisoned$` and `AttackDifferentPlayers$` all resolve now too —
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
`GainControl$ Player.isMonarch` (1 of 44) stays unresolved: no monarch mechanic to filter by (PORT-8/GO-7). `Condition$`
— the one gate all six appliers share, `StaticAbility.checkConditions`'s own switch — is real too now:
`continuousConditionMet` (continuous.go) resolves `PlayerTurn`/`NotPlayerTurn`/`Threshold`/`Metalcraft`/`Delirium`/
`Hellbent`/`FatefulHour` (262 of the corpus's 317 real `Mode$ Continuous | Condition$` lines) in place of the blanket
"any `Condition$` present, skip the line" every applier had; `MaxSpeed`/`Blessing`/`EnduringStory`/`Monarch` (55) stay
unresolved, each its own untracked mechanic. CR 502.3/614.17's own "doesn't untap" replacement effects are real now too
— `untapBlocked` (`replacement.go`) resolves 149 of the corpus's 158 real `Event$ Untap` lines (`Layer$ CantHappen`,
`ReplaceUntap.canReplace` ported directly), called from `untapStep` (turn.go) before clearing `Tapped`;
`IsPresent$`/`SVarCompare$`/`CheckSVar$`/`EnduringStory$`/`AddSVar$` (7) skip the line, and the other 2 name
`ReplaceWith$` instead, a genuine substitution not built. `ValidStepTurnToController$` (154 of 156 real
`Layer$ CantHappen` lines, always "You") is not checked at all: `untapStep`'s own loop only ever considers cards
`g.activePlayer` already controls, so "the untapping player is this card's own controller" already holds by construction
for every real value that param carries. CR 614's own "prevent all of this damage" family is real too —
`damagePrevented`/`damagePreventedPlayer` (`replacement.go`) resolve 63 of the corpus's 218 real `Event$ DamageDone`
lines (`Prevent$ True`, `ReplaceDamage.canReplace`'s own resolvable half plus `ReplacementHandler`'s own unconditional-
void dispatch for that value), called from `dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go) before marking any
damage or emitting `DamageDealt`; `PlayerTurn$`/`SVarCompare$`/`IsPresent$`/`CheckSVar$` resolve generically through
`replacementRequirementsCheck` (below) and `DamageAmount$` resolves too now, reusing `damageAmountMatches` (trigger
firing, above) against the ORIGINAL amount about to be dealt — `ValidCause$`/`RelativeToSource$`/`CauseIsSource$` (2 of
72, one line naming the first and third together, the other the second alone) still skip the line, each its own further
restriction this file cannot evaluate. The other 146 name `ReplaceWith$` instead — most a real sub-ability substitution
(`Mill`/`ChangeZone`/`Dig`/... — no shape anywhere near Moved's own 618-line concentration, not built), but three do: CR
616's own "Updated" outcome (the event still happens, with a different number, rather than being skipped or substituted
outright) is real now too, for both a flat reduction and a computed replacement —
`damageReplaced`/`damageReplacedPlayer` (replacement.go) resolve 18 of the 27 real `DB$ ReplaceDamage | Amount$ N` lines
this file's own `face.Replacements` walk can even reach ("prevent N of that damage," `ReplaceDamageEffect.resolve`'s own
two-outcome half this dispatch can compute without a `*Registry`) and, through the identical two callers, 56 of the 59
real `DB$ ReplaceEffect | VarName$ DamageAmount | VarValue$ ...` lines it can reach too (`ReplaceEffect.resolve`'s own
default "amount" `VarType$` branch, `AbilityUtils.calculateAmount` — a flat integer, or a named SVar naming
`ReplaceCount$DamageAmount/<op>` and one of `AbilityUtils.doXMath`'s own `Twice`/`Thrice`/`HalfDown`/`Plus`/`Minus`
branches, `resolveReplaceCountAmount`/`applyDamageReplaceEffect` — a doubling/tripling/halving/plus/minus rather than
`ReplaceDamage`'s own flat "prevent N," raphael_the_muscle.txt's/
torbran_thane_of_red_fell.txt's/ghosts_of_the_innocent.txt's own real "double"/"plus 2"/"half, rounded down" damage
among them, plus forethought_amulet.txt's/divine_presence.txt's own flat "deals N damage instead," gated by the R:
line's own `DamageAmount$` threshold the identical way `damagePreventionMatches`'s new fold-in now reads it too), both
called from `dealPermanentDamage`/`dealPlayerDamage` right after `damagePrevented`/`damagePreventedPlayer` and reducing
or resizing the same `amount` the marking/event/trigger-check below it already reads, so a trigger checking
`DamageAmount$` sees the final number. Of `ReplaceDamage`'s own 27 reachable lines, a named-SVar `Amount$`
(`ShieldAmount`/`X`/`PaidAmount`/`AlchemicX`, 9 lines — a depleting shield counter, an X spent on the spell, mana paid,
...), one also chaining its own `SubAbility$` (the identical chained-target refusal `applyDrawReplacement` already
gives), stay unresolved. `reidane_god_of_the_worthy_valkmira_protectors_shield.txt`'s/`plated_pegasus.txt`'s own real
`ValidTarget$ You,Permanent.YouCtrl`/`Permanent,Player` resolve too now — `matchesPlayerSpec` (valid.go) splits a spec
on comma the way `valid.Parse` already does, an alternative whose base is not `You`/`Opponent`/`Player` matching nothing
rather than aborting the whole spec (`valid.Parse`'s own contract for an unrecognized base), the fix that also makes
`gratuitous_violence.txt`'s own `Permanent,Player` shape (`ReplaceEffect`, below) double damage dealt to a player, not
just to a permanent, a real correctness fix since that line was already counted resolved for its card-target half alone.
Of `ReplaceEffect`'s own 59 reachable `VarName$ DamageAmount` lines, 3 stay unresolved:
fated_firepower.txt's/hawkeye_young_avenger.txt's own `Plus.Y` operand (`Count$CardCounters.FIRE`/ `Count$CardPower`,
neither the Valid family `resolveAmount` evaluates) and ojer_axonil_deepest_might_temple_of_power.txt's own bare
`Count$CardPower` `VarValue$` (no `ReplaceCount$` at all) — each an amount head this port has no evaluator for. 12 more
real `DB$ ReplaceEffect` lines name `VarName$ Affected`/`LifeGained`/`Number`/`Ignore` instead of `DamageAmount` — an
entirely different substitution, redirecting who is damaged or what else changes rather than resizing the damage itself
— filtered out, not resolved by anything here. Of the corpus's own 39 real `DB$ ReplaceDamage` SVar definitions, the
other 12 are never named by any literal top-level `R:` line at all: `hedron_field_purists.txt`'s own 2 are referenced
only through a Layer 6 `AddReplacementEffect$` on a Level-up `Mode$ Continuous` line, and 10 more
(`forcefield.txt`'s/`ajani_steadfast.txt`'s/`torrent_of_lava.txt`'s among them) are created dynamically at resolution
time by `DB$ Effect`'s own `ReplacementEffects$` param (CR 611.2c) — neither mechanism this port's own script-effect
dispatch builds, so `face.Replacements` never discovers them regardless of this dispatch's own shape. A third real shape
resolves too now — `DB$ RemoveCounter`/`DB$ PutCounter` (`applyDamageReplaceCounter`, replacement.go) — CR 616's own
"Replaced" outcome this time, not "Updated": the damage does not happen at all, a counter changes on some object instead
(`ReplacementHandler.java`'s own default `ReplacementResult.Replaced`, every `ApiType` past
`ReplaceDamage`/`ReplaceSplitDamage`/`ReplaceEffect`/`ReplaceToken`/`ReplaceMana`) — every "Phantom" creature's own real
"prevent that damage, remove a +1/+1 counter" among them (`Defined$ Self`), soul_scar_mage.txt's own "put -1/-1 counters
on that creature instead" (`Defined$ ReplacedTarget`, the damaged object itself, threaded straight through from
`damageReplaced`'s/`damageReplacedPlayer`'s own `target` parameter), and panther_habit.txt's own "put +1/+1 counters on
equipped creature instead" (`Defined$ Equipped`, `Card.AttachedTo()` reused). `CounterNum$` resolves through
`resolveReplaceCountAmount` (above), now generalized to accept a bare, operator-less `ReplaceCount$DamageAmount` too —
`doXMath`'s own `operators == null` identity, the dominant real shape for this dispatch specifically
(lichenthrope.txt's/phytohydra.txt's own real `CounterNum$ X`, `X:ReplaceCount$DamageAmount` among them). 25 of the
corpus's own 31 real lines resolve; `SubAbility$` (5, underdark_beholder.txt's own "remove counters, then sacrifice if
none left" among them) refuses outright, the identical chained-target refusal every other hand-run dispatch in this file
already gives, and jared_carthalion_true_heir.txt's own real R: line naming `CheckDefinedPlayer$ You.isMonarch` (1 — no
monarch mechanic this port tracks) is skipped by `damageReplacementMatches`'s own allow-list before ever reaching this
dispatch. All four families share a new `replacementActiveZones`/`hostInActiveZones` (replacement.go), generalizing
`ActiveZones$` past Battlefield alone to the 2 real Command-zone lines each carries — the identical comma-list
`checkPhaseTriggers`'s own `TriggerZones$` already has, for a replacement's own zone restriction instead of a trigger's.
M6 in progress alongside it: `Draw` (`draweffect.go`) is the first of the 203 script-driven effects to actually resolve
rather than report `ErrUnimplemented` — `Ability` gained a `Params` field (`ability.go`) carrying a trigger's own
`Defined$`/`NumCards$` onto the stack to make that possible. `DealDamage` (`dealdamageeffect.go`) is the second — 62 of
the corpus's 2,219 real `(AB|DB)$ DealDamage` lines that also name `Defined$ You`/`Player.Opponent`/ `Opponent`/`Self`
(out of 822 naming any `Defined$` at all) and carry no other unresolved param — reusing combat's own damage machinery
directly: `dealPermanentDamage`/`dealPlayerDamage` (combatdamage.go) gained an `isCombat bool` parameter (every prior
call site combat's own, now passing `true` explicitly; `DealDamage` is the first to pass `false`), threading through to
`damagePrevented`/`damagePreventedPlayer` (CR 614's own "prevent all of this damage," item 26) and to a conditional
`FlagCombat` (event.go's own doc comment: "marks damage dealt in combat rather than by an effect," dormant until now).
`Ability` also gained an `Amounts` field, threaded through all eighteen check-triggers call sites (`face.Amounts`,
already in scope at each) — `NumDmg$`'s own named-SVar shape resolves through `resolveNamedAmount` (amount.go) the
identical way a continuous effect's own numeric params already do, and `Draw`'s own `NumCards$` was upgraded to the same
resolver for free. `definedPlayers` (new `defined.go`) is `drawDefinedPlayers` renamed and relocated once `DealDamage`
needed the identical `You`/`Opponent`/`Player.Opponent` resolution — neither effect owns it outright. `Defined$ Self`
resolves against the ability's own host card directly, `HasKeyword` reading its own `Deathtouch` for
`dealPermanentDamage`'s own flag exactly as combat already does.
`ConditionPresent$`/`ConditionCompare$`/`ConditionCheckSVar$`/`ConditionSVarCompare$` (5 of 822) are resolved too
(`subAbilityConditionMet`, below). Not resolved: `DamageSource$` (17 of 822 real `Defined$` lines — a source other than
the ability's own host); `SubAbility$` no longer blocks (9 of 316 real SVar-defined lines naming it chain to an
already-built leaf ability and resolve end to end, `subability.go`); `Condition$` itself and `ConditionDefined$`
(`SpellAbilityCondition`'s own separate flag switch and an arbitrary reference this port has no resolver for, distinct
from the two resolved shapes above); `Planeswalker$`/`UnlessPayer$`/`UnlessCost$`/`UnlessResolveSubs$`/`ValidTgts$`/
`TriggeredSpellAbility$`/`DamageMap$`/`CounterNum$`/`Optional$`/`TgtPrompt$` (each its own mechanic); `NoPrevention$` (1
— this port's own prevention would otherwise wrongly apply). `isETBTrigger`/`isDiesTrigger` (trigger.go) now port
`TriggerChangesZone.performTest`'s own `Origin$`/`Destination$` semantics exactly — absent or the literal value `"Any"`
means unrestricted, ported as a new `hasZoneOrAny` — rather than the literal-only match they started with, closing two
real gaps: `isDiesTrigger`'s own `Destination$` used to require the literal `"Graveyard"`, missing 264 real lines naming
`Destination$ Any`/no `Destination$` at all (CR 603.6c's own unqualified "leaves the battlefield") even on an ordinary
death; its `Origin$` used to require the literal `"Battlefield"`, missing 31 more naming only `Destination$ Graveyard`.
`isETBTrigger` gained a real `origin ZoneType` parameter (threaded from the `origin := c.Zone` local every real ETB call
site already computes for `checkMovedReplacement`) to close the matching over-firing bug on its own `Origin$` side — 21
real lines, mostly `Origin$ Graveyard`, used to fire regardless of where the card actually came from. A
`Mode$ ChangesZone` line naming `ValidCause$`/`NotThisAbility$`/`ConditionYouCastThisTurn$`/
`CheckOnTriggeredCard$`/`ExcludedOrigins$`/`ExcludedDestinations$` (12 of 7,609 real lines combined) now skips rather
than firing unconditionally (`changesZoneResolvable`). `checkPhaseTriggers`/`checkAttackersDeclaredTrigger` each carried
their own `hasAnyParam` pre-filter still naming `IsPresent$`/`PresentCompare$`/`CheckSVar$` after
`triggerCommonRequirementsMet` (item 26's own `meetsCommonRequirements` paragraph) already resolved exactly those params
generically — a leftover never revisited once that mechanism landed, silently keeping 686 real `Phase` lines and 31 real
`AttackersDeclared` lines skipped regardless of whether their own condition held. Removing the three keys from each
pre-filter was the whole fix; `Condition$` stays skipped in both (a separate, still-unresolved `SpellAbilityCondition`
gate). `checkAttacksTriggers` resolves `Attacked$` (47 real lines, `attackedTargetMatches` — built for
`AttackersDeclared`'s own `AttackedTarget$`, reused here at its one-element case against `AbilityKey.Attacked`'s own
single `GameEntity`) and `FirstAttack$` (4, a new `Card.AttacksThisTurn` per-card counter, incremented per declared
attacker and reset every cleanup) now too. A new shared `subAbilityConditionMet` (`condition.go`) ports
`SpellAbilityCondition.areMet`'s own gate — a different Java class from `CardTraitBase.meetsCommonRequirements`, gating
an ability's own resolution rather than whether a trigger fires — trimmed to the two shapes real corpus lines use,
`ConditionPresent$`/`ConditionCompare$` and `ConditionCheckSVar$`/`ConditionSVarCompare$`, reusing
`isPresentMatches`/`checkSVarMatches` (trigger.go, both generalized to take either family's own key names). It closed
two real gaps at once: `LandTapped`'s own 140 real `DB$ Tap` lines (a checkland's "enters tapped unless you control a
Mountain or a Forest," `tapAbilityResolvesTap` in replacement.go, renamed from `tapAbilityIsPlainTap` since it now
reports whether a shape is recognized separately from whether it actually taps) and 5 more real `DealDamage` lines. Full
detail: `docs/crucible/00-master-implementation-plan.md` items 24-29, `docs/crucible/porting/port-log/game-state.md`.
Thin or missing: Layer 1 (copy effects — not even part of `StaticAbilityContinuous.java`'s own switch in Forge itself;
zero real references to `StaticAbilityLayer.COPY` anywhere in it, a wholly separate "become a copy of a card" mechanism
at resolution time, not a recomputed-each-pass continuous effect at all); Layer 3 (`GainTextOf$`, 1 real line, needs its
own card-text-copying mechanism for a single card); Layer 8's own remainder (`MayLookAt$`/`MayPlay$`, 88/181 real lines
— a cast-time zone-eligibility permission `CastSpell`'s hand-only check has nowhere to consult yet; `AddHiddenKeyword$`,
19, each of its 8 real distinct values its own separate block/attack/untap-step mechanic; vote/villainous-choice params,
0-3 real lines each); plus the rest of Layers 4/5/6 past a literal token list and Layer 7a's own SVar shapes outside the
Valid family (`xPaid`, `CardCounters`, `Devotion`, ...), plus a dozen more where the Valid argument itself carries a
`$`-suffixed distinct-value operator (Tarmogoyf's own `Card$CardTypes` — a new `expr.Count.DistinctProperty` field now
catches this rather than silently misparsing it, a real bug caught and fixed after the fact, not a hypothetical one) — a
dynamic value or a bulk-removal/`AddAllCreatureTypes$`/`SharedKeywords$` combo still skips the whole line rather than
applying it wrong; the legend rule's own Corner Case 1 (a Corner-Case-2 permanent's own borrowed names colliding with
some OTHER legendary's own literal printed name needs a card-name lookup across every creature card this game ever
printed, which this port's `*Game` holds no `*carddb.DB` reference to ask); `DamageDone`'s own `ValidCause$`,
`Discarded`'s own `ValidCause$`, `Taps`'s own `FirstTime$`/`Teamwork$`, `TapsForMana`'s own `Produced$`, `SpellCast`'s
own `Player.EnchantedBy`/`Player.Chosen` qualified `ValidActivatingPlayer$` forms, `Phase`'s own `Condition$` and its
own qualified `ValidPlayer$` forms, and every trigger mode past
enters/dies/attacks/blocks/deals-damage/is-discarded/becomes-tapped/becomes-untapped/taps-for-mana/casts/beginning-of-a-step-or-phase/
a-player-attacks/a-player-draws-a-card/gains-life (`Mode$ LifeGained`, `checkLifeGainedTriggers`, `ValidPlayer$` matched
against the gainer through `matchesPlayerSpec`, reusing `phaseTriggerZones`'s own four-zone walk; `FirstTime$` (6)
resolves through a new pre-increment read of `Player.LifeGainedTimesThisTurn`, the identical contract
`checkLandPlayedTriggers`'s own `NotFirstLand$` already has; `ActivationLimit$` (4) now skips the whole line too, a real
correctness fix rather than a new resolution — this port never checked it before, so those lines were wrongly firing
every time rather than up to their own per-turn cap — 93 of 98 real lines resolve now (`OptionalDecider$`, 7, every real
line "You", resolves too through `triggerEffectAPI`'s own `triggerIsOptional`, below), `ValidSource$`+`Spell$` (1,
combined on the identical real line)/`ResolvedLimit$` (1) unresolved)/becomes-the-target-of-a- spell-or-ability
(`Mode$ BecomesTarget`, `checkBecomesTargetTriggers` — CR 115/603.3, called from `pushTriggeredAbilities` right after
every `PushAbility` and from `castAura` for an Aura's own cast-time target, the two places this port ever finishes
choosing a target for something — `ValidTarget$` matched with `attackedTargetMatches` (`AttackersDeclared`'s own
dispatch, reused), a new `Card.BecameTargetThisTurn` closing `FirstTime$` the identical way `Card.AttacksThisTurn`
already closes `Attacks`'s own, and `ValidSource$` (71 of 77 real lines naming it) resolving through
`becomesTargetSourceMatches` — SpellAbility.isValid's own restriction split, a Spell/Triggered ability-kind classifier
built from this dispatch's own two real call sites rather than a general field on `Ability`: `castAura`'s own Aura is
always a Spell, and a triggered ability pushed through `pushTriggeredAbilities` is always Java's own
`isTrigger()`/`isAbility()` pair (no activated-ability targeting exists in this port yet, so `Ability`/ `Triggered`
collapse to the identical "not a Spell" check); `SpellAbility` itself matches unconditionally, `.YouCtrl`/ `.OppCtrl`
compare the ability's own controller against the watching trigger's host controller, and `Spell.Aura`'s own qualifier is
trivially true whenever the kind is Spell, since this port's only Spell source reaching here IS an Aura — 101 of 118
real lines resolve now (`OptionalDecider$`, 12, every real line "You" and none also naming
`Valiant$`/`ActivationLimit$`/`Static$`, resolves too through `triggerEffectAPI`'s own `triggerIsOptional`, below),
`Valiant$`/`ActivationLimit$`/`Static$` (14 combined) and 6 of the 77 `ValidSource$` lines (an `Instant,Sorcery`
card-type check neither real source can ever satisfy, or a property past YouCtrl/OppCtrl/Aura) still
unresolved)/plays-a-land (`Mode$ LandPlayed`, `checkLandPlayedTriggers` — CR 305/603.5, called from `PlayLand` (land.go)
right after `checkETBTriggers`, `Player`'s own real Java ordering, `Player.playLand`'s own
`moveTo`-then-`runTrigger`-then-`addLandPlayedThisTurn` sequence, which is why `PlayLand`'s own `LandsPlayed++` now runs
last too — `ValidCard$` matched the usual way, `Origin$` resolved through `hasZoneOrAny` (ETB triggers' own dispatch,
reused) against the land's own origin zone (always Hand today, no `MayPlay$` permission to play from elsewhere yet, so 8
of the corpus's 9 real non-`Static$` `Origin$` lines naming Exile or a Hand-excluding zone list never actually fire, the
identical "mechanically correct, presently unreachable" gap `DB$ ReplaceDamage`'s own `hedron_field_purists.txt` lines
already have), `ValidActivatingPlayer$` (1, "You") through `matchesActivatingPlayer` (reused), and `NotFirstLand$` (1)
through a new pre-increment read of `Player.LandsPlayed` (player.go) — 38 of 42 real lines resolve, `Static$`/`ValidSA$`
(7 combined, "Once during each of your turns, you may play a historic land..." shapes — `Static$` a trigger ability that
resolves off the stack, `ValidSA$` a `SpellAbility` `Matches` cannot evaluate) skip via `hasAnyParam`;
`OptionalDecider$` (3, every real line "You") resolves too now, through `triggerEffectAPI`'s own `triggerIsOptional`
(below) -- `checkLandPlayedTriggers`'s own `hasAnyParam` never named it, so these 3 real lines
(search_the_city.txt's/jokulmorder.txt's/burgeoning.txt's own real "you may...") were firing unconditionally before
this, a real correctness fix (PORT-8/GO-7) rather than only a new resolution. **`Trigger.phasesCheck` itself lands too**
(`triggerPhasesCheck`, trigger.go) — a general gate every trigger mode carries regardless of what it fires on, checked
before any mode-specific dispatch runs at all: `Phase$` restricts a trigger of any mode to firing only during named
step(s)/phase(s) (reusing `phaseTriggerMatches`, `Mode$ Phase`'s own dispatch, generically); `PlayerTurn$`/
`NotPlayerTurn$`/`OpponentTurn$` restrict to (or away from) the host's own controller's turn — `OpponentTurn$`
collapsing to `NotPlayerTurn$`'s own check in this port's no-team model; `FirstCombat$` resolves to a hardcoded `true`
(this port has no extra-combat mechanism to ever make a second combat phase reachable, the identical reasoning
`combatdamage.go`'s own `CombatDamage$` check already uses). Closes 43 real lines across six already-built modes
(`SpellCast` 12+2, `ChangesZone` 9+11, `LifeGained` 5, `Taps` 2, `Discarded` 1, `Drawn` 1) that were firing
**unconditionally** until now — a wrong answer, not a coverage gap, since this port had never checked either key before
(sentinel_tower.txt's own real "deals damage... during your turn" among them) — plus 6 real `Attacks`/
`AttackersDeclared` lines naming `FirstCombat$`. Not resolved: `FirstUpkeep$`/`FirstUpkeepThisGame$` (1/2, `Mode$ Phase`
only, a per-game upkeep-step counter this port tracks nowhere); `TurnCount$` (0 real lines, dormant). **`Mode$ Untaps`
lands too** (`checkUntapsTriggers`, trigger.go) — CR 502.3/603's own "becomes untapped," `Taps`'s own mirror image,
called once per card from `untapStep` (turn.go) for every card that actually untaps that step (a card already untapped
generates no event, `Card.untap()`'s own early return ported as a `wasTapped` guard). One battlefield walk covers both a
card's own "Inspired" trigger and mesmeric_orb.txt's own bare "whenever a permanent becomes untapped," the identical
single-walk shape `checkTapsTriggers` already has — 30 of 30 real lines resolve now (`OptionalDecider$`, 3, every real
line "You", resolves too through `triggerEffectAPI`'s own `triggerIsOptional`, below).
**`ReplacementEffect.requirementsCheck` itself lands too** (`replacementRequirementsCheck`, replacement.go) — a general
gate every replacement carries regardless of its own `Event$`, mirroring `triggerPhasesCheck`'s own role for triggers:
`PlayerTurn$` (8 real lines, literal `True` only), `ActivePhases$` (1, reusing `phaseTriggerMatches` — now taking its
own key as a parameter rather than hardcoding `"Phase"`, so `Mode$ Phase`'s own dispatch and this general gate share the
identical parser), then `triggerCommonRequirementsMet` outright (the identical Java method a trigger's own `performTest`
already calls). Folded into `damagePreventionMatches`/`untapReplacementMatches`/`replacementTapsOnMove`, closing 7 of 10
previously-skipped real `DamageDone`|`Prevent$` lines and 5 of 7 previously-skipped `Untap`|`CantHappen` lines for free,
plus fixing a real, if narrow, wrong-firing bug: archelos_lagoon_mystic.txt's own "enters tapped" toggle names
`IsPresent$ Card.Self+tapped/+untapped` restricting its own two replacement lines to only apply while Archelos itself is
tapped/untapped — unchecked before this, `replacementTapsOnMove` carried no allow-list at all to skip on, so both lines
matched regardless of Archelos's own state. Two new consumers reuse it outright: `drawPrevented`/`gainLifePrevented`
(replacement.go) resolve CR 120.3's/119's own `Prevent$ True` shape for `Draw`/ `GainLife` — 2 of 39 real `Draw` lines
(possessed_portal.txt's own bare form; living_conundrum.txt's own `IsPresent$`-qualified "while your library has no
cards") and 1 of 21 real `GainLife` lines (sulfuric_vortex.txt's own bare form) resolve end to end, wired into
`DrawCards` (turn.go, checked before the empty-library check so a prevented draw cannot also trigger CR 704.5b's own
loss condition) and `gainLifeEffect` (gainlifeeffect.go) respectively. The other 36 real `Draw` lines and 20 real
`GainLife` lines name `ReplaceWith$` instead — a real substitution, part of which resolves further down
(`drawReplaced`/`gainLifeReplaced`, below). `matchesPlayerProperty` (valid.go) gains two more real Player properties,
both reused for free by every one of its nine existing callers across
`trigger.go`/`continuous.go`/`targeting.go`/`replacement.go`, now threading the ability's own host card through as a
`source CardID` parameter (`matchesPlayerSpec`'s own signature, alongside it) rather than only a controller:
`EnchantedController` (34 of `Mode$ Phase`'s own real qualified `ValidPlayer$` lines, righteous_authority.txt's own "at
the beginning of the draw step of enchanted creature's controller" shape) reads `source.AttachedTo()` (`Card.go`) — the
same attachment link Layer 2's own `GainControl$` already reads — to find the controller of whatever the trigger's own
host card enchants; `descended` (10, ruin_lurker_bat.txt's own "if you descended this turn" shape, CR's own descend
mechanic) reads a new `Player.DescendedThisTurn` (player.go), set in `Game.Move` (game.go) whenever a permanent,
non-token card is moved into a graveyard from any zone — this port has no token-creation effect yet (M6's own remaining
territory), so the token half of Java's own check holds by construction for every card this port can ever move — and
reset for every player at `cleanupStep` (turn.go) the identical way `LandsPlayed`/`CardsDrawnThisTurn` already are.
`Player.EnchantedBy`/`Player.Chosen`/`Opponent.EnchantedBy`/ `Player.isMonarch` (14, 3, 2, 1) stay unresolved, each
needing its own separate mechanic this port does not have (an Aura enchanting a player directly, a chosen-player memory
slot, a monarch tracker). **CR 616's own "the event is replaced by a different one" outcome is real now too**, for
`Draw` — `drawReplaced` (replacement.go) recognizes a `ReplaceWith$` target naming a plain
`DB$ Draw | Defined$ You | NumCards$ N` or `DB$ PutCounter | CounterType$ X | CounterNum$ N | Defined$ Self`, run by
hand (`applyDrawReplacementDraw`/ `applyDrawReplacementPutCounter`) rather than through `drawEffect`/`putCounterEffect`
— both need a `*Registry` (`effect.go`) to chain a `SubAbility$` that `DrawCards`' own call chain (turn.go) has no way
to reach, so a target ability naming one is refused outright rather than run with the chained half silently dropped.
`DrawCards`' own per-card loop gained a new primitive, `drawOneCard`, the replacement's own substitute draws call
directly rather than recursing back through `DrawCards`/`drawPrevented`/`drawReplaced` itself — Java's own
`ReplacementHandler` guards a replacement effect against reapplying to an event its own resolution produced (`hasRun`),
a per-line recursion guard this port does not build, so reusing the unguarded primitive instead sidesteps needing one,
at the cost of a real, narrow simplification: the replacement's own draws are not themselves checked against any other
replacement or prevention effect on the battlefield, not observable against a corpus with no two Draw-replacing
permanents on one battlefield today. 7 of the corpus's own 36 real `Event$ Draw | ReplaceWith$` lines resolve end to
end: thought_reflection.txt's own bare "draw two cards instead," phial_of_galadriel.txt's own `Hellbent$ True`-qualified
identical shape, ormos_archive_keeper.txt's own `IsPresent$`-qualified line whose own target is `PutCounter` rather than
`Draw`, and teferis_ageless_insight.txt's/alhammarrets_archive.txt's/bard_king_of_dale.txt's own real "except the first
one you draw in each of your draw steps, draw two cards instead" (`NotFirstCardInDrawStep$ True`) — a new
`Player.DrawnThisDrawStep` (player.go), reset for every player at the start of each Draw step (`drawStep`) and
incremented in `drawOneCard` (both turn.go) whenever the current phase is Draw, closes `notFirstCardInDrawStepExempts`'s
own gate (replacement.go): only the very first card a player draws during their own current Draw step is exempt: a later
draw in the same step, or any draw outside that player's own Draw step entirely, is not. notion_thief.txt's own real
"except the first one they draw ..., instead you draw a card" (`ValidPlayer$ Opponent`) resolves through the identical
gate too, needing `applyDrawReplacementDraw`'s own `Defined$ You` reading corrected from the event's own affected player
to the replacement's host controller instead — every previously-resolved line's own `ValidPlayer$` happened to be `You`
as well, so the two had never needed telling apart before. Not resolved: reed_richards_smartest_man.txt's own
`FirstExtraCardDrawnThisTurn$`; hullbreacher.txt's own identical `NotFirstCardInDrawStep$` shape, whose own
`ReplaceWith$` targets `DB$ Token` rather than `Draw`/`PutCounter` (`CreateToken` is not a built `Effect` yet, unrelated
to the gate itself); magus_of_the_chains.txt's/chains_of_mephistopheles.txt's/
breathstealers_crypt.txt's/sea_of_sand.txt's own `Defined$ ReplacedPlayer` (4) and blood_scrivener.txt's own chained
`SubAbility$` (1) stay unresolved for the reasons already given above; booby_trap.txt's own `Player.Chosen` and
pursuit_of_knowledge.txt's own `Optional$` (1 each) are the identical already-documented gaps. **`GainLife` gets the
same `ReplaceWith$` dispatch too now** — `gainLifeReplaced` (replacement.go) is `drawReplaced`'s own sibling, but
carries both of CR 616's own outcomes rather than just "Replaced," the way `damageReplaced` already does for a different
`Event$`: a full substitution (`applyGainLifeReplacement`, reporting a gain of 0 the identical way
`applyDamageReplaceCounter`'s own full substitution already does) or a resized gain (`applyGainLifeReplaceEffect`,
below, returning the new amount, still granted through the normal path — `gainLifeEffect.Resolve`'s own
`if gain <= 0 { continue }` folds Java's own pre- and post-replacement `lifeGain <= 0` checks, `Player.gainLife`, into
the one this port's call ordering needs). `ReplaceCount$LifeGained`, "the amount of life that would have been gained,"
reads straight off the raw `LifeAmount$` `gainLifeEffect.Resolve` already has in scope. 19 of the corpus's own 20 real
`Event$ GainLife | ReplaceWith$` lines resolve end to end now: lich.txt's/nefarious_lich.txt's own "draw that many cards
instead" (`ValidPlayer$ You`, target `DB$ Draw | Defined$ You | NumCards$` naming that SVar) and
tainted_remedy.txt's/plague_drone.txt's own "that player loses that much life instead" (`ValidPlayer$ Opponent`, target
`DB$ LoseLife | LifeAmount$` naming it | `Defined$ ReplacedPlayer` — read as the replaced player directly, the identical
narrow `Defined$` reading `drawReplaced` already has for its own "You"), plus 15 more real lines targeting
`DB$ ReplaceEffect | VarName$ LifeGained | VarValue$ ...` (`applyGainLifeReplaceEffect`) — rhox_faithmender.txt's/
the_wind_crystal.txt's/... own real "gain twice that much life instead" (`Twice`) and angel_of_vitality.txt's/
heron_of_hope.txt's/... own real "gain that much life plus 1 instead" (`Plus.1`), through a newly generalized
`resolveReplaceCountAmount` (renamed from `resolveDamageReplaceCountAmount`, item 26's own `DB$ ReplaceEffect` section
above) read against `"LifeGained"` instead of `"DamageAmount"`. Not resolved: rain_of_gore.txt's own real
`ValidSource$ SpellAbility | SourceController$ True` restriction (no `ValidPlayer$` at all — a restriction on what
CAUSED the event, not who it affects, a shape this dispatch's own allow-list has never needed before).
**`Mode$ AttackersDeclaredOneTarget` is real now too** — `checkAttackersDeclaredOneTargetTrigger` (trigger.go) is
`checkAttackersDeclaredTrigger`'s own sibling, `TriggerType.java`'s own identical `TriggerAttackersDeclared` class fired
at a different granularity (`PhaseHandler.java`'s own `declareAttackersStep`: once per defender that has at least one
attacker, `Attackers`/`AttackedTarget` narrowed to just that one defender, rather than once per combat with every
attacker/every attacked defender gathered) — both share a new `attackersDeclaredParamsMatch`, and
`validAttackersCountMatches` now takes the attacker subset as a parameter instead of always reading
`g.combat.Attackers`, so `ValidAttackers$`/`ValidAttackersAmount$` count only the firing's own defender's attackers
under this mode. 35 of the corpus's own 35 real lines resolve — every real line's own param vocabulary
(`TriggerZones$`/`AttackedTarget$`/`ValidAttackers$`/`ValidAttackersAmount$`/`AttackingPlayer$`) is already resolved by
the shared dispatch, 0 real lines naming `Condition$`/`OptionalDecider$`/`CheckDefinedPlayer$`/`IsPresent$` the way the
plain `AttackersDeclared` mode's own remainder does. **CR 603.3d's own "may" triggered ability is real now too** --
`Ability` gained an `Optional bool` field, true only for `OptionalDecider$ You` (`triggerEffectAPI`'s own new
`triggerIsOptional`, trigger.go, folded into the shared gate all twenty-eight of its own call sites already run through)
-- `Registry.Resolve` (effect.go) asks a new `PlayerController.ConfirmOptionalTrigger` (its twenty-fifth method) before
dispatching to the effect OR chaining its own `SubAbility$` at all, `WrappedAbility.resolve()`'s own
`decider.getController().confirmTrigger(this)` ported directly: a decline skips the whole ability, chain included, the
identical early return Java's own version gives before ever reaching `playSpellAbilityNoStack`. 1,506 of the corpus's
own 1,584 real `OptionalDecider$` lines (95%) name "You" -- the ability's own `Controller`, already in scope everywhere
this is checked, so no new decider-resolution machinery was needed for the dominant shape; every other real value
(`TriggeredCardController`, 43; `True`, 11; `TriggeredSourceController`, 5; a dozen more, 1-4 real lines each) skips the
whole trigger line rather than asking the wrong player or firing unconditionally (GO-7) -- this port's own
`ConfirmOptionalTrigger` has nobody correct to ask for those yet. Newly resolved for free across three already-built
modes, each simply reaching this shared gate for the first time: `Mode$ Untaps`'s own remaining 3 real lines (30 of 30
now), `Mode$ LifeGained`'s own 7 (93 of 98), `Mode$ BecomesTarget`'s own 12 (101 of 118) -- and a real correctness fix
for `Mode$ LandPlayed`'s own 3 (38 of 42), which `checkLandPlayedTriggers`'s own `hasAnyParam` never named at all, so
search_the_city.txt's/jokulmorder.txt's/burgeoning.txt's own real "you may..." lines were firing unconditionally before
this, not merely unresolved. 83 more real lines name `OptionalDecider$` on a sub-ability's own SVar body rather than a
`T:` line -- a chained `SubAbility$`'s own independent "may" -- a smaller, separate gap this change does not reach,
since `resolveSubAbility` (subability.go) builds its own child `Ability` with no `Optional` field set. `gainLifeEffect`
(`gainlifeeffect.go`) is M6's third script-driven effect, `dealDamageEffect`'s own shape reused for a player-only gain
(`LifeAmount$`/`Defined$`/`subAbilityConditionMet`, no `Self` shape) — 857 of 1,700 real `GainLife` lines resolve, the
corpus's largest slice past `DealDamage`. `pumpEffect` (`pumpeffect.go`) is M6's fourth script-driven effect, the
corpus's own single largest by real line count after `ChangeZone`/`Draw` (4,103 real `(AB|DB)$ Pump` lines) and the
first whose own contribution outlives its `Resolve` call: `Duration$`'s default, "until end of turn," is a continuous
effect this port never needed a duration for before, closed by a new `Game.pumps` ledger (`pumpRecord`, game.go)
re-added into its target's own `PT`/`KeywordMod` every `CheckStateBasedActions` pass (`applyPumpEffects`, continuous.go)
and dropped at `cleanupStep` (`turn.go`) unless `Duration$ Permanent` names it durable — CR 514.2's own "until end of
turn" effects wearing off, closing the gap `applyContinuousPT`'s own doc comment used to name.
`Defined$ Self`/`Enchanted`/`Equipped` (`definedCards`, defined.go) — no target — cover 1,147 of 4,103 real `Pump`
lines: `NumAtt$`/`NumDef$` (a plain integer or a named SVar) and/or `KW$` (a literal keyword list), gated by
`PumpZone$`'s own zone restriction (default Battlefield alone) and `subAbilityConditionMet`'s own Condition-family pair
the identical way `DealDamage`'s/`GainLife`'s already are. `pumpAllEffect` (`pumpalleffect.go`) is M6's fifth,
`pumpEffect`'s own blanket sibling — a `ValidCards$`-matched set across every player (or, with `Defined$`, only the
named players' own battlefield) rather than a single `Defined$` card, sharing its duration tracking
(`Game.pumps`/`applyPumpEffects`/`cleanupStep`) outright — 642 of 833 real `(AB|DB)$ PumpAll` lines resolve, 818 of them
the real corpus's own dominant no-target, no-`Defined$` "anthem spell" shape (Overrun, ...). `loseLifeEffect`
(`loselifeeffect.go`) is M6's sixth, `gainLifeEffect`'s own mirror image — `LifeAmount$` subtracted from `Defined$`'s
players instead of added, the identical `LifeChanged` event with a negative `Amount` — but calls no trigger check at
all: `Mode$ LifeLost`/`LifeLostAll` carry 0 real `T:` lines corpus-wide, unlike `Mode$ LifeGained`'s own 98. 300 of 445
real `(AB|DB)$ LoseLife` lines naming `Defined$ You`/`Opponent`/`Player.Opponent` or a resolvable `ValidTgts$` resolve
(226 by `Defined$` alone, 74 more once targeting landed, below). `putCounterEffect` (`putcountereffect.go`) is M6's
seventh, the corpus's own second-largest resolvable slice after `Pump` — 992 of 3,165 real `(AB|DB)$ PutCounter` lines
naming a single literal `CounterType$` and `Defined$ Self`/`Enchanted`/`Equipped`/`You` resolve, dispatching to
`Card.Counters`/`Player.Counters` by which one `Defined$` names (`definedCounterTargets`, new) rather than by
`CounterType$` itself, the identical dispatch `CountersPutEffect.resolvePerType`'s own `instanceof` check makes.
`CounterType$` is uppercased before it becomes a `Counters` key (`CounterEnumType.getType`'s own canonicalization), so a
corpus line writing `Stun` and another writing `STUN` land on the identical kind rather than two. `CounterNum$` defaults
to `1`, matching Java's own `getParamOrDefault`. `discardEffect` (`discardeffect.go`) is M6's eighth — 285 of 942 real
`(AB|DB)$ Discard` lines naming `Mode$ TgtChoose` and `Defined$ You`/`Opponent`/`Player`/`Player.Opponent` resolve, the
first script-driven effect that asks the resolving player anything mid-resolution rather than reading game state
outright: `Effect.Resolve` gained a `PlayerController` parameter for it (`effect.go`'s own doc comment), and
`PlayerController` gained a twenty-first method, `ChooseCardsToDiscard` — Forge's own `chooseCardsToDiscardFrom`, a
different decision from `DiscardToHandSize`'s own CR 514.1 cleanup discard even though both ask for exactly `N` cards
out of the same hand. `NumCards$` is clamped to the discarding player's actual hand size, matching Java's own
`Math.min(numCards, numCardsInHand)`, and an already-empty hand skips the controller call entirely rather than asking
for zero cards. `definedPlayers` (`defined.go`) gained a `"Player"` case alongside `You`/`Opponent`/`Player.Opponent` —
`AbilityUtils.getDefinedPlayers`'s own fallthrough `else` branch, every player in the game unfiltered, closing
`rotting_rats.txt`'s own "each player discards a card" shape and any other effect's bare `Defined$ Player` for free.
`scryEffect` (`scryeffect.go`) is M6's ninth — 332 of 415 real `(AB|DB)$ Scry` lines resolve, the first effect where an
absent `Defined$` itself means `You` (`AbilityUtils.getDefinedPlayers`'s own `changedDef = (def == null) ? "You" : ...`
default) rather than a rejected line, and the first to ask the player to reorder cards rather than choose a subset of
them: `PlayerController` gained a twenty-second method, `ArrangeForScry`, and a new `Game.MoveToLibraryTop` (`game.go`)
puts a card on top of a library — `Game.Move` only ever appends to a zone's own end, the library's own bottom.
`ScryNum$` cards come off the top, the controller's own answer splits them between a chosen top order and a chosen
bottom order, and a scry of `0` (`CR 701.22b`) never reaches the controller at all. `surveilEffect` (`surveileffect.go`)
is M6's tenth — `ArrangeForScry`'s own sibling decision (`ArrangeForSurveil`, `PlayerController`'s twenty-third method)
reused wholesale for CR 701.42: 183 of 208 real `(AB|DB)$ Surveil` lines resolve, the only real difference from `Scry`
being that the cards not kept on top go to the graveyard rather than to the bottom of the library. `sacrificeEffect`
(`sacrificeeffect.go`) is M6's eleventh script-driven effect — CR 701.20, 465 of the corpus's 792 real
`(AB|DB)$ Sacrifice` lines: an absent `SacValid$` or the literal value `Self` sacrifices the ability's own host
outright, no choice asked (`SacrificeEffect.java`'s own `valid.equals("Self")` branch); any other `SacValid$` value asks
each of `Defined$`'s players (default `You`, `AbilityUtils.getDefinedPlayers`'s own null default, `Scry`'s own identical
shape) to choose `Amount$` of their own matching battlefield permanents through a new `PlayerController` method,
`ChoosePermanentsToSacrifice` (its twenty-sixth) — `ChooseCardsToDiscard`'s own shape reused for a second
exactly-N-of-a-set decision. `ValidTgts$` resolves too, `LoseLife`'s own bypass-`Defined$` pattern reused (41 real
player-shaped lines). `RememberSacrificed$` is this port's first real writer of `Memory.Remember` (`memory.go`, dormant
scaffolding until now). Not resolved: `UnlessPayer$`/`UnlessCost$` (155 combined, always co-occurring) — a further
"unless a cost is paid" mechanic; `Optional$` (46) — the identical ability-body-level "may" gap `Discard`'s/ `Pump`'s
own already document, distinct from CR 603.3d's own `OptionalDecider$`; `Planeswalker$`/`ChangeNum$`/
`ConditionDefined$`/`UnlessResolveSubs$`/`UnlessSwitched$`/`ValidCard$`/`SorcerySpeed$`/`SacEachValid$`/`Random$`/
`Destroy$`/`StrictAmount$`/`Echo$`/`CumulativeUpkeep$` (each its own further mechanic or unclear semantics). Sacrificing
a card also fires CR 701.20's own new `Mode$ Sacrificed` trigger (`checkSacrificedTriggers`, `trigger.go`, ported from
`TriggerSacrificed.performTest`) right before the zone change — `Player.addSacrificedThisTurn`'s own ordering ahead of
`sacrificeDestroy`'s own `moveToGraveyard`, both ported directly. Unlike every other per-card trigger dispatch this port
has, this one walks the battlefield only once: the sacrificed card is still physically there at check time, so a
separate own-half walk (`checkDiscardedTriggers`'s own shape) would fire its own trigger twice. 106 of 115 real lines
resolve (`ValidCard$`/`ValidPlayer$` through the usual dispatch, `PlayerTurn$`/`OptionalDecider$`/the whole
`IsPresent$`/`CheckSVar$`/... family through the shared `triggerEffectAPI` gate); `ActivationLimit$`/`ResolvedLimit$`
(7/1, the identical per-turn-cap gap `LifeGained`'s own already documents) and `WhileKeyword$` (1) stay unresolved.
`sacrificeAllEffect` (`sacrificealleffect.go`) is M6's twelfth script-driven effect, `Sacrifice`'s own blanket sibling
(`pumpAllEffect`'s own shape, `pumpalleffect.go`, reused for a second blanket effect): an absent `Defined$` scans every
battlefield in the game, `ValidCards$`-filtered if present — 72 of the corpus's own 140 real `(AB|DB)$ SacrificeAll`
lines, the corpus's own dominant real shape — and a present `Defined$` names specific cards through `definedCards`
instead (`Self`/`Enchanted`/`Equipped`/`Targeted`, an unrecognized value failing loudly rather than sacrificing
nothing). `Controller$`, when present, narrows either set further to one of its own resolved players' own permanents. 91
of the corpus's own 140 real lines resolve; `UnlessCost$`/`UnlessPayer$` (6/6) — the identical "unless a cost is paid"
gap `Sacrifice`'s own already documents; `ConditionDefined$` (3), `Planeswalker$`/`Activator$`/`SorcerySpeed$`/
`ImprintSacrificed$` (1 each) stay unresolved. It shares `sacrificeCards` (sacrificeeffect.go) outright with
`sacrificeEffect`, so `RememberSacrificed$` and CR 701.20's own `Mode$ Sacrificed` trigger both fire once per card in
the whole blanket set, not once for the ability as a whole. CR 603.6d's own `Mode$ ChangesZoneAll` — the batched sibling
of `Mode$ ChangesZone` itself, firing once for a whole group of cards that changed zones together rather than once per
card — is real now too (`checkChangesZoneAllTriggers`, `trigger.go`, ported from `TriggerChangesZoneAll.performTest`).
Called once per uniform-origin/uniform-destination batch a single game action moves together — `sacrificeCards`
(sacrificeeffect.go, both `Sacrifice`'s and `SacrificeAll`'s own shared caller) and
`destroyLethalToughness`/`destroyDamagedCreatures` (`action.go`, CR 704.5f-h's own simultaneous SBA sweeps) — rather
than through a general `CardZoneTable`-style architecture threaded through every mover in the engine: every call site
this port has today moves its own batch through one uniform zone pair, so a `cards []CardID` triple with a shared
`origin`/`destination` loses nothing observable yet. `ValidCards$` matches each card's `g.LKI` snapshot when one exists,
`checkDiesTriggers`'s own pattern, so a `Destination$ Graveyard` line still sees pre-move state. 77 of the corpus's own
126 real lines resolve (`Destination$`/`Origin$` through `hasZoneOrAny`, reused from ETB/Dies;
`PlayerTurn$`/`OptionalDecider$`/the whole `IsPresent$`/`CheckSVar$`/... family through the shared `triggerEffectAPI`
gate); `ActivationLimit$`/`ValidCause$`/`ResolvedLimit$`/`NoResolvingCheck$`/`InvertValidCause$`/`FirstTime$` (41/4/3/
1/1/1) skip the whole line rather than firing unconditionally (GO-7). Two creatures killed by `destroyLethalToughness`
and `destroyDamagedCreatures` in the same `CheckStateBasedActions` call fire two separate batches rather than one shared
one — this port's own SBA split into one function per CR 704.5 clause rather than Java's single combined pass, narrower
than CR 704.3's own full simultaneity but not observable against a corpus with no card that cares which SBA clause
killed which creature. CR 603's own `Mode$ DamageDoneOnce` — `Mode$ DamageDone`'s own batched sibling, the corpus's own
single largest remaining trigger mode (206 real lines) — is real now too (`checkDamageDoneOnceTriggers`, `trigger.go`,
ported from `TriggerDamageDoneOnce.performTest`). A new `damageTable` (`[]damageEntry`, trigger.go — CardDamageTable's
own port) accumulates every `(source, target, amount)` triple a single damage-dealing action actually deals (after
prevention/replacement), consumed once rather than checked per exchange the way the ordinary `DamageDone` trigger
already is — CR 510.2's own "all combat damage is dealt simultaneously" means a gang-blocked attacker's own trigger has
to see every blocker's damage combined into one firing. `dealPermanentDamage`/`dealPlayerDamage` (`combatdamage.go`)
gained a `table *damageTable` parameter, appending to it whenever non-nil rather than every call site being forced to
build one; `dealCombatDamageStep` builds one per damage sub-step and `dealDamageEffect` (`dealdamageeffect.go`) one per
resolution, each calling `checkDamageDoneOnceTriggers` once after every exchange it made has run. `ValidTarget$` matches
the target itself (`attackedTargetMatches`, reused at its one-element case), `CombatDamage$` against `isCombat`, and the
summed amount — filtered first to only the entries whose own source matches `ValidSource$`, when the line names one
(`damageDoneOnceAmount`, `TriggerDamageDoneOnce.getDamageAmount`'s own dispatch) — against `DamageAmount$`
(`damageAmountMatches`, `DamageDone`'s own dispatch, reused). 200 of the corpus's own 206 real lines resolve;
`ResolvedLimit$`/`ActiveZones$`/`DamageSource$`/`FirstTime$` (2/2/1/1) skip the whole line rather than firing
unconditionally (GO-7). `checkDamageTableTriggers` (new, trigger.go) is the shared caller `dealCombatDamageStep`/
`dealDamageEffect` call once per damage-dealing action instead of calling three separate dispatches — it runs
`checkDamageDoneOnceTriggers` alongside two of the table's further real siblings in Java, now built too:
`checkDamageDealtOnceTriggers` (`Mode$ DamageDealtOnce`, the identical table grouped by `Source` instead of `Target` —
47 of the corpus's own 49 real lines resolve, `ValidSource$` matched directly against the source, `ValidTarget$` both
filtering and summing the group's own entries the identical role `ValidSource$` plays for `DamageDoneOnce`;
`AtLeastOneInstance$`/`ActivationLimit$`, 1 each, skip the whole line) and `checkDamageAllTriggers` (`Mode$ DamageAll`,
no grouping at all — fires once whenever the table, filtered by `ValidSource$`/`ValidTarget$` together, still has any
entry left; 9 of 9 real lines resolve, every param this mode carries already has a resolver).
`DamageDoneOnceByController` — the table's fourth real sibling, grouping by a target's every damaging controller — is
not built: 0 real corpus lines name it. **Targeting itself landed** (`targeting.go`) — CR 601.2c/603.3b's own "choose
targets," this port's own most-cited gap across every effect built so far (`ValidTgts$` in every one of their own "not
resolved" lists above). `resolveTargets` runs the moment an ability is pushed onto the stack (`pushTriggeredAbilities`,
`trigger.go`, this port's only pusher today), computing `ValidTgts$`'s own legal candidates — every player still in the
game (`matchesPlayerSpec`, reused) or every card on any battlefield (`Matches`, reused) — and asking a new
`PlayerController` method, `ChooseTargets` (its twenty-fourth), for `TargetMin$`/`TargetMax$` of them (1/1 when neither
is named). A structural shape this port does not parse
(`Radiance$`/`TargetsForEachPlayer$`/`TargetsWithDefinedController$`/`TargetUnique$`, each rare-to-zero real lines)
folds into CR 603.3c's own "no legal targets, doesn't go on the stack" outcome rather than erroring — the two are
indistinguishable from outside, and both mean the ability does nothing. Threading a `PlayerController` down to
`pushTriggeredAbilities` touched every one of its sixteen callers across `action.go`/`attack.go`/`block.go`/
`combatdamage.go`/`manaability.go`/`turn.go`/`land.go`/`castspell.go` — mechanical, and every path already bottomed out
at a function some earlier chunk had already given a controller to, so the cascade stayed contained. `definedPlayers`/
`definedCards` (`defined.go`) gained `"Targeted"`/`"TargetedPlayer"` cases reading `Ability.Targets` (new field) for a
sub-ability that names it explicitly; `loseLifeEffect`'s own dispatch instead mirrors `LifeLoseEffect.java`'s own
`getTargetPlayers(sa)` directly — `ValidTgts$` present means read `a.Targets`, bypassing `Defined$` outright, since 0
real `LoseLife` lines combine the two. `LoseLife` is targeting's first real consumer; `PutCounter`/`Discard`/`Scry`/
`PumpAll`/`Surveil` still block `ValidTgts$` outright in their own `Resolve` — the mechanism exists, extending each
effect to read `Targeted` back through it is not yet done. **SubAbility chaining itself landed too** (`subability.go`) —
`AbilityUtils.resolveApiAbility`'s own `resolveSubAbilities` call, this port's own second-most-cited gap after targeting
(`SubAbility$` named in every effect's own "not resolved" list above, 16,022 real corpus lines, 12% of the whole
corpus). `Registry.Resolve` (`effect.go`) chains an ability's own `SubAbility$` reference, if it names one, right after
its own `Effect.Resolve` call — recursive through that same method for a chain more than one deep (1,172 real lines
chain exactly two hops past the first, up to 13 deep once) — and runs whether or not `subAbilityConditionMet` let the
parent's own body run at all: Sphinx Sovereign's own "gain 3 life if untapped, otherwise each opponent loses 3" is one
`DB$ LoseLife` with a `SubAbility$ DB$ GainLife`, the negated condition split across the two, the exact reason Java's
own pairing is unconditional. Chaining into an API this port has not built an `Effect` for yet still fails with
`ErrUnimplemented` naming it, the identical contract a top-level ability already had, extended for free by the
recursion; the parts of a chain that already resolved stay resolved, CR's own sequential "this already happened" rather
than an all-or-nothing rollback. `SubAbility$` itself no longer blocks `GainLife`'s or `LoseLife`'s own resolution
(removed from both effects' own unresolved-param lists) — `Draw` never blocked it, so `Rousing Read`'s own real "draw
two cards, then discard a card" (`DB$ Draw`, chaining into `DB$ Discard`) is the first chain to actually run both
halves. `SubAbility$` no longer blocks any of the ten script-driven effects built so far —
`DealDamage`/`Pump`/`PumpAll`/`PutCounter`/`Discard`/`Scry`/`Surveil` had it removed from their own unresolved-param
lists too, the identical change `GainLife`/`LoseLife` already got. 170/253 (`Draw`), 18/253 (`GainLife`), 144/382
(`LoseLife`), 9/316 (`DealDamage`), 17/571 (`Pump`), 6/75 (`PumpAll`), 56/623 (`PutCounter`), 11/254 (`Discard`), 31/57
(`Scry`), and 2/15 (`Surveil`) of the corpus's own real SVar-defined lines naming `SubAbility$` now chain to an
already-built leaf ability and resolve end to end (a chain more than one hop deep, or one whose target is one of the 193
effects still unbuilt, is not counted). 191 script-driven effects past
`Draw`/`DealDamage`/`GainLife`/`Pump`/`PumpAll`/`LoseLife`/`PutCounter`/`Discard`/`Scry`/`Surveil`/`Sacrifice`/
`SacrificeAll` still report `ErrUnimplemented`. **Last-known-information landed too** (`Game.LKI`, `game.go`) — CR
603.6d's own "look back in time": `Move`'s own battlefield-leaving branch freezes a copy of the card before clearing its
own `Counters`/`PT`/`TypeMod`/`ColorMod`/`KeywordMod`, so `checkDiesTriggers`/`otherDiesTriggerMatches` (`trigger.go`)
still match a `ValidCard$` naming the dying card's own power, toughness, type, color, a keyword or a counter against
what it had the instant before it died, not the printed-only state `Move` has already reset it to by the time either
function runs — 116 of the corpus's own 7,574 real `Mode$ ChangesZone` lines whose `Destination$` permits Graveyard name
exactly that shape (Retched Wretch's own real "when CARDNAME dies, if it had a -1/-1 counter on it..."). `Card.Def`/
`Card.Controller()` never needed the lookup (`Move`'s own doc comment already covers why), so this is a plain struct
copy rather than Java's own `CardCopyService.getLKICopy()`'s field-by-field reconstruction — overwritten whole, never
merged, on every subsequent trip off the battlefield. `Game.Clone` (M7's own AI lookahead) gives its own copy an
independent snapshot, the identical "shares nothing writable" contract it already holds for every other per-card ledger.
The legend rule's own Corner Case 2 is real now too (`resolveLegendRule`, action.go) — two or more legendary permanents
that all carry `HasNonLegendaryCreatureNames` (card.go, a new Layer 3 continuous effect, `applyContinuousNames`,
continuous.go, resolving Spy Kit's own real `AddNames$ AllNonLegendaryCreatureNames` line — the corpus's only one — via
`Card.AttachedTo()`, this port's existing generic Aura/Equipment/Fortification attachment link, for
`AffectedDefined$ Equipped`) clash with each other even when their own printed names differ, grouped and asked about the
identical way an ordinary same-name duplicate already is, skipping any permanent the name-grouping above already sent to
its owner's graveyard so the controller is never asked about the same pair twice. Corner Case 1 stays unbuilt: whether
one of those borrowed names collides with some OTHER legendary's own literal printed name needs a lookup across every
creature card this game ever printed, and this port's `*Game` holds no `*carddb.DB` reference to ask — threading one
through every `*Game` constructor across the whole test suite is a disproportionately large refactor for the one corpus
card it would unlock (PORT-8). CR's own "unless a cost is paid" is real now too (`resolveUnlessCost`, `effect.go`,
ported from `AbilityUtils.handleUnlessCost`) — a new gate `Registry.Resolve` checks ahead of its own ordinary
`Effect.Resolve`/`resolveSubAbility` pairing whenever an ability names `UnlessCost$`: each of `UnlessPayer$`'s own
players (`definedPlayers`, reused; an absent value is Java's own "TargetedController" default, not resolved) is asked a
new `PlayerController` method, `ConfirmPayCost` (its twenty-seventh), and a yes actually charged through `PayManaCost`
(manapay.go) — the identical "decide, then pay" split every other mana decision on the interface already has. The
ability's own body runs when nobody paid (`UnlessSwitched$`'s own presence flips that), and `UnlessResolveSubs$` decides
whether its own chained `SubAbility$` still runs regardless (absent, "Always") or only on one particular outcome
("WhenPaid"/"WhenNotPaid"). Trimmed to the corpus's own one resolvable shape — a pure-mana `UnlessCost$` (a new
`cost.Cost.IsPureMana`, `internal/cost`, added once a direct `Tap`/`Untap` field read in effect.go collided with
phase.go's own `Untap` step constant under enginelint's plain-identifier matching) and an explicit `UnlessPayer$` naming
`You`/`Player`/`Opponent`/`Player.Opponent` — 56 of the corpus's 727 real `UnlessCost$` lines resolve past this gate and
are actually reachable by this port at all: `nicol_bolas.txt`'s own real "sacrifice CARDNAME unless you pay {U}{B}{R}"
shape dominates (51 of Sacrifice's own 155 real lines), plus 3 of DealDamage's own 31 (`force_of_nature.txt`'s own real
"deals 8 damage to you unless you pay {G}{G}{G}{G}") and 2 of Pump's own 15 (`spitting_slug.txt`'s own real "gains first
strike... unless you pay {1}{G}", chaining `UnlessResolveSubs$ WhenNotPaid` into `PumpAll` when the cost goes unpaid,
and `nakaya_shade.txt`'s own real activated `{B}:` ability, itself gated by its own nested "unless any player pays {2}"
— reachable once general activated-ability casting landed too, below). The other 671 real lines fail one hop up the call
chain rather than at this gate itself: an instant or sorcery's own top-level line (`CastSpell`'s own doc comment: "an
instant or sorcery resolves into a script effect this port does not build"), a line reached only through an unbuilt
API's own `SubAbility$`/`RepeatSubAbility$`/... chain link (`DB$ Effect`, `DB$ Repeat`, `DB$ GenericChoice`,
`DB$ DelayedTrigger`, none built), a `S:...AddTrigger$` line's own dynamically granted trigger (not a built
continuous-effect param), a non-mana cost part (`Sac<.../Discard<.../PayLife<...`), or an unresolvable `UnlessPayer$`
value (`TriggeredPlayer`, `EnchantedController`, ...) each account for the remainder.
`UnlessCost$`/`UnlessPayer$`/`UnlessResolveSubs$`/ `UnlessSwitched$` no longer block any of the eight already-built
effects that named them in their own unresolved-param lists
(`sacrificeEffect`/`sacrificeAllEffect`/`dealDamageEffect`/`pumpEffect`/`pumpAllEffect`/`gainLifeEffect`/
`loseLifeEffect`/`discardEffect`) — Sacrifice's own real corpus count rises from 465 to 516 of 792, DealDamage's from 62
to 65 of 2,219, and Pump's own Defined$-shape count from 1,147 to 1,148 of 1,335.

Activating an ability landed too (`ActivateAbility`, `activateability.go`), CR 602.2, the corpus's own single largest
still-unbuilt action by real line count: 10,879 real `A:AB$` lines exist, more than any one trigger mode past
`Mode$ ChangesZone` itself. Trimmed to its own two dominant real `Cost$` shapes, pure mana and pure mana plus a single
Tap-self token, through a new `cost.Cost.ActivationShape` (`internal/cost`) — `IsPureMana`'s own sibling, needed because
a bare `T` always also parses as its own named `Part` alongside setting the `Tap` flag (`namedParts`' own trailing `T`
entry), so `IsPureMana`'s flat "no `Parts` at all" contract cannot simply add `Tap` to its own allowed set. 6,246 of the
10,879 real lines carry that shape (2,515 bare `T`, 1,090 bare mana, 995 two mana symbols, 930 mana-plus-`T`, a long
tail past those four); excluding `AB$ Mana` itself (1,845 lines — CR 605.3a's own no-stack immediate resolution, a
wholly different mechanism this port only has for a basic land's own intrinsic ability, `TapLandForMana`,
manaability.go) leaves 4,401 real non-mana activated abilities reachable at the shape level, 1,987 of them already
naming one of the twelve already-built effects (`Pump` 993, `PutCounter` 293, `DealDamage` 221, `Draw` 196, `PumpAll`
119, `LoseLife` 41, `GainLife` 39, `Scry` 31, `Discard` 27, `Surveil` 27) — real lines none of those effects' own
previously-published resolved-line counts include yet, since every one was computed against cast/trigger reachability
alone; recomputing each against activated-ability reachability too is a further chunk's own work. Timing collapses to
`CastSpell`'s own CR 601.3a simplification (active player, a main phase, an empty stack), since this port has no real
priority window at all yet. A Tap-self cost checks CR 602.5b/302.6 first (`Card.SummonSick`/`HasKeyword`,
`DeclareCombatAttackers`'s own identical gate reused) with no side effect yet; the mana half pays through `PayManaCost`
exactly as `CastSpell`'s own does, and only once that succeeds does the tap itself actually happen. A successful
activation pushes through `pushTriggeredAbilities` (trigger.go) with the activating player as its own sole entry,
reusing every one of the twelve already-built effects and the general `Registry.Resolve` machinery (`UnlessCost$`,
`SubAbility$` chaining, `ConditionCheckSVar$`) with no new effect code at all.

`ActivateAbility` gained a third cost shape too — pure mana and/or a Tap-self token, plus a single self-sacrifice token
(`Sac<1/CARDNAME>`, "sacrifice this permanent," fetch lands' and sac-outlets' own dominant real shape) — folded into the
same `cost.Cost.ActivationShape` (`internal/cost`): 947 more of the corpus's own non-`AB$ Mana` real `A:AB$` lines are
reachable at the shape level this way (`ChangeZone` 164, `Draw` 145, `Destroy` 94, `DealDamage` 91, `Pump` 60,
`GainLife` 52 among the largest; 441 of the 947 already name one of the twelve already-built effects — recomputing each
effect's own resolved-line count against this shape too stays the same deferred further-chunk work `ActivationShape`'s
own landing already named). Paying it reuses `sacrificeCards` (sacrificeeffect.go) wholesale — CR 701.20's own "dies"
trigger, `RememberSacrificed$` and the batched `Mode$ ChangesZoneAll` firing all come free, exactly as they already do
for `Sacrifice`'s own "Self" branch — committed last, after mana and the tap, so a self-sac cost never sacrifices a
permanent whose own mana or tap half of the same cost went unpaid; the pushed ability then resolves normally even though
its own source has already left the battlefield (CR 112.7a), the identical "ability survives its source" contract
`SubAbility$` chaining into a just-sacrificed card's own `Defined$ Self` already relies on elsewhere.

CR 605.3's own general mana ability landed too (`ActivateManaAbility`, activatemanaability.go) — every real card
printing its own `A:AB$ Mana` line (rocks, dorks, Treasures), not only a basic land's synthesized intrinsic one
(`TapLandForMana`, manaability.go). 2,156 real lines exist corpus-wide, 1,946 already matching `ActivationShape` — the
identical predicate `ActivateAbility` uses, reused outright since the two APIs never compete for the same line.
`Produced$`'s own literal single-color/colorless shape (1,005 of the 1,946) resolves through a new `producedManaColor`;
a new positive allow-list, `manaAbilityAllowedParams`, admits only the five real keys this dispatch reads
(`compile.Ability.Params` is directly enumerable, so naming what it needs was shorter than naming everything it does
not) — 850 of the 1,005 clear it and resolve end to end. Payment order matches `ActivateAbility`'s own exactly (mana,
tap, self-sac), reusing `sacrificeCards` and `checkTapsForManaTriggers` (CR 603's own "taps for mana" trigger, fired
only when the cost actually taps something) wholesale.

`Produced$ Any` — CR 605.3b's own "choose a color," 334 more of the 1,946 — resolves too now, through a new
`PlayerController` method, `ChooseManaColor` (its 28th, taking an `options mana.Colors` set) — distinct from
`ChooseHybridManaColor`'s own always-exactly-two contract. A bad answer (not exactly one color) declines rather than
reaching `Pool.Add`'s own panic — caught by the regression-toggle check itself: disabling the guard turned the expected
test failure into an actual panic, proof the guard is load-bearing rather than defensive padding.

`Produced$ Combo <letters>` — a dual/tri-land's own real "Add W or U"/"Add G, U, or R," 367 more of the 1,946 — resolves
too, reusing `ChooseManaColor` with `options` narrowed to the listed colors instead of all five (`parseComboColors`)
rather than a second interface method: the answer is validated against that narrower set the identical way "Any"'s
answer is validated against all five. `Combo Any`/`Combo AnyDifferent` (24, "add two mana in any combination of colors,"
a per-unit independent choice this single-color-per-activation dispatch does not model), `ColorIdentity` (6, Commander's
own color-identity set, untracked) and `Chosen` (a color picked earlier in the same resolution) still are not built.

`ActivateAbility` gained a fourth cost primitive too — `Discard<N/Card>`, "discard N cards of your choice," 228 more
real non-`AB$ Mana` `A:AB$` lines — through `PlayerController.ChooseCardsToDiscard` (already built for `discardEffect`)
and a new shared `discardCards` (discardeffect.go). This landing also collapsed three growing near-identical `cost.Cost`
predicates (`IsPureManaOrTap`, `IsPureManaTapAndSelfSac`, `SelfSac`) into one `ActivationShape` decomposition — a
`Discard<N/Card>` count could not fit a bare `bool` the way `Tap`/`SelfSac` could, and three predicates was already the
sign a fourth should not be a fourth. `ActivateManaAbility` explicitly declines any `DiscardN > 0` rather than silently
ignoring it (0 real `AB$ Mana` lines carry `Discard<...>` at all, so there is nothing to execute, and letting the shape
through unhandled would claim the cost was paid while discarding nothing).

**P4 exit gate's fixture-count half met:** 342 scenarios (`testdata/scenarios/`) past the ≥300 floor; the qualitative
half ("every layer, every SBA," Plan Section 3.2) is not.
