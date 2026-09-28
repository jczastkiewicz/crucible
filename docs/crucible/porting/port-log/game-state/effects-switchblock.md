# Effects: SwitchBlock

`SwitchBlockEffect.java`'s two real corpus lines (`general_jarkeld.txt`, `sorrows_path.txt`) each need pieces outside
the effect before either can activate, target or resolve. Each lands as a shared primitive first.

## Combat and activation pieces SwitchBlock needs

| Piece                        | Where                                                          | Java                                                    | Why SwitchBlock needs it                                                                                                  |
| ---------------------------- | -------------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `blocked` valid property     | `valid.go` `propertyMatches`                                   | `CardProperty.java:1591-1592`                           | Jarkeld's `ValidTgts$ Creature.attacking+blocked` matched nothing: the tap was paid, then no target, ability never pushed |
| `ActivationPhases$` enforced | `activateability.go` `inActivationPhases`                      | `SpellAbilityRestriction.java:131-132,294-297`          | Jarkeld is "declare blockers step only"; unenforced it could switch blocks after first-strike damage, changing outcomes   |
| `TargetsWithSameController$` | `targeting.go` `withSameControllerPartner`, `targetStillLegal` | `CardLists.java:201-217`, `SpellAbility.java:1543-1549` | Sorrow's Path's two targets must share a controller; 35 corpus files named it, all silently ignored                       |

- `blocked` is exact-matched: `combat.isBlocked(card)`, an attacker with a blocker or one an effect made blocked
  (`Combat.ForcedBlocked`). `blockedBySource*`/`blockedThisTurn`/`blockedValidThisTurn` are distinct Java branches, stay
  unported and fall through to "matches nothing".
- `ActivationPhases$` is a general activation restriction, not SwitchBlock's own: every `AB$` naming it (150 corpus
  files) now activates only in its phase set. Parsed by `parsePhaseRange` (`PhaseType.parseRange`) against the current
  step; all 17 distinct corpus values parse. An unreadable value declines the activation (GO-7) rather than dropping the
  restriction.
- `destroyalleffect.go`, `damagealleffect.go`, `removecountereffect.go`, `untapalleffect.go` still reject
  `ActivationPhases$` at resolve. Now enforced at activation, those rejects are stale; left for their own owners to
  lift.
- `TargetsWithSameController$`: two of Java's three halves. Candidate pre-filter (`targetChoiceFor`, min targets >= 2):
  a card whose controller controls no other candidate is not offered. Fizzle check (`targetStillLegal`): a card target
  whose controller differs from any other chosen card target is illegal; every target is checked against the full chosen
  list before any is dropped (`MagicStack.hasFizzled` removes after its loop), so a split pair fizzles whole. Not
  ported: re-validating the one `ChooseTargets` answer (Java checks each click); no other `ChooseTargets` answer is
  re-validated either, and a split answer fizzles at resolution anyway. A copy's retargeting
  (`copyspellabilityeffect.go`) gets the pre-filter through `targetChoiceFor`.

| Test                                                    | Proves                                                                  |
| ------------------------------------------------------- | ----------------------------------------------------------------------- |
| `TestMatchesBlocked`                                    | Blocked attacker matches, unblocked attacker and the blocker do not     |
| `TestActivateAbilityActivationPhasesRestrictsTiming`    | Declines in Main1 and Combat Damage, cost unpaid; activates in step     |
| `TestActivateAbilityActivationPhasesRange`              | `A->B` range inclusive both ends                                        |
| `TestActivateAbilityActivationPhasesUnreadableDeclines` | Unreadable phase list declines (GO-7)                                   |
| `TestTargetsWithSameControllerDropsLoneCandidate`       | Three players: lone creature of third player never offered; pair pumped |
| `TestTargetsWithSameControllerMixedAnswerFizzles`       | Split answer fizzles whole at resolution                                |
| `TestTargetsWithSameControllerFizzlesOnControlChange`   | Control change on stack makes both targets illegal                      |
