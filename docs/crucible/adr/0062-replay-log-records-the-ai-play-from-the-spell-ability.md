# ADR-0062 — Replay Log Records the AI's Own Choices for a Play from the Cast Spell Ability

- **Status:** Accepted
- **Date:** 2026-11-12
- **Deciders:** Crucible session (R21a), accepted by the owner

## Context

ADR-0059 logs one record per `PlayerController` call. The Go replay asks its controller separately for each choice of a
play: `TakeAction`, then `ChooseTargets`, `ChoosePayX`, `ChooseModesForAbility`, the cost-part card choices and
`PayManaCost` (`internal/engine/control.go`). Java's AI makes most of those choices without a controller call:

| Choice                                                 | Java                                                                                                                                    | Controller call                                                |
| ------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| Targets of a spell or ability the AI plays             | Written by the `forge-ai` ability classes while `chooseSpellAbilityToPlay` evaluates the play                                           | None                                                           |
| Targets of a trigger the AI controls                   | `PlayerControllerAi.chooseTargetsFor` → `brains.doTrigger` (`PlayerControllerAi.java:1600-1601`)                                        | Wrapped, but the answer is on the `SpellAbility`, not returned |
| X                                                      | `ComputerUtilCost.java:762, 805`; `ComputerUtilMana.java:1577, 1583, 1987`                                                              | None                                                           |
| Cards for a cost part (discard, sacrifice, exile, tap) | `CostPayment` with `AiCostDecision` (`ComputerUtil.java:148`); `chooseCardsForCost` is `assert(false)` (`PlayerControllerAi.java:1436`) | None                                                           |
| Mana sources                                           | `PlayerControllerAi.payManaCost` → `ComputerUtilMana.payManaCost` (`:1421`)                                                             | Wrapped, answer is a boolean                                   |
| Charm modes, optional costs, any-color mana            | `chooseModeForAbility` (`PlayerController.java:316`), `chooseOptionalCosts` (`:368`), `chooseColors` (`:338`)                           | Yes                                                            |

After `playChosenSpellAbility` returns, the cast `SpellAbility` and its sub-ability chain still hold every one of them:
`getTargets` (`SpellAbility.java:1802`), `getXManaCostPaid` (`:2549`), `getChosenList` (`:2294`), `getOptionalCosts`
(`:566`), `getPaidHash` (`:943`, reset before payment at `ComputerUtil.java:123`, not after), `getPayingManaAbilities`
(`:938`) and `getPayingMana` (`:866`).

## Decision Drivers

- REV-1: no `forge-ai` or `forge-game` edit; the recorder stays in `crucible/oracle-java`.
- ADR-0059: answers are logged as identities, and a mismatched option set is a legality report.
- The Go replay must answer each question when Go asks it, whatever order Go asks in.

## Considered Options

1. **Route the AI's choices through controller calls.** Rejected: it edits every `forge-ai` ability class that targets
   or pays, and every upstream sync would conflict there.
2. **Infer the choices from the state change around the play** (cards that moved, permanents that tapped). Rejected: a
   tapped land does not say which of its mana abilities paid, and a target that left play leaves no trace.
3. **Read the choices from the cast `SpellAbility` once the play is done, and log them as one play record.** Chosen.

## Decision

The recorder overrides the controller methods that make the AI play something: `playChosenSpellAbility`,
`playSpellAbilityNoStack`, `playTrigger`, `orderAndPlaySimultaneousSa` and `chooseTargetsFor`. When one returns, it
writes a **play record** after that call's own record. The play record walks the root `SpellAbility` and each
sub-ability and logs, by ADR-0059 identity:

- the ability played and the host card, and the targets per sub-ability;
- X, the chosen modes and the optional costs;
- the cards each cost part paid with, per cost part, from `getPaidHash`;
- the mana abilities that paid, in order, each with its source card and the mana it produced.

In replay, `TakeAction` reads the play record, and Go's follow-up questions for the same play (`ChooseTargets`,
`ChoosePayX`, `ChooseModesForAbility`, the cost-part choices, `PayManaCost`) read the matching field of the same record.
They are matched by question kind and sub-ability, not by the order of the calls. A question with no matching field, or
a field Go never asks for, is a divergence report naming the play.

A random outcome inside a play is logged as its result. `Discard<1/Random>` is decided by `AiCostDecision` with
`MyRandom`, inside the controller call, and appears as the play record's paid cards. ADR-0059's rule that draws inside a
controller call are the AI's therefore loses no engine outcome of a play.

The AI runs without game simulation, because a simulated game's engine frames would look like the real game's. The
recorder leaves `Game.AI_FLAT_MONTE_CARLO` false (`Game.java:106`); a desktop match sets it from
`MATCH_AI_FLAT_MONTE_CARLO`, true by default, and then runs rollouts on copied games. It refuses a seat with
`AIOption.USE_HYBRID_SIMULATION` or `USE_FULL_SIMULATION`.

## Consequences

**Good:** The log answers every question Go asks for an AI play without touching Forge. A Go targeting or payment rule
that offers different options from Java's is reported at the play that differs.

**Bad:** The recorder reads `SpellAbility` getters whose meaning is defined by the cast path, and an upstream change to
when one is cleared breaks the play record silently. The upstream-sync runbook's recorder run covers it. A mana ability
that pays without activating (`getPayingMana` with no matching ability, such as floating mana) is logged with the `Mana`
object's source card alone.

**Neutral:** The recorded AI is the desktop AI without flat Monte Carlo, a weaker player. Replay parity does not depend
on how well the AI plays. Choices that are already controller calls (modes, optional costs, colors) appear twice, as the
call record and inside the play record. The replay reads the play record.

## Related

ADR-0006, ADR-0010, ADR-0059
