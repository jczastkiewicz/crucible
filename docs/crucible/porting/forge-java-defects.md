# Forge Java Defects

- **Status:** Active

Bugs in upstream Forge's Java that the port found: a method that cannot do what its name says, an unguarded index, a
loop that never ends. PORT-8: reported and fixed upstream, never compensated in Go. Reason: a workaround makes Crucible
disagree with the oracle for a reason no diff can explain. Card-script bugs go in
[card-script-defects.md](card-script-defects.md) instead.

A row lands in the same commit as the port that found it. What Crucible does meanwhile is fail closed — an `error` or a
rejected param — never a silent fix.

Rows start with the ChooseSource/Empower batch. Bugs noted before it are only in their own port-log sections.

## Status

| Site                              | Defect                                                                                                                                  | Crucible meanwhile                    | Upstream     |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- | ------------ |
| `ChooseSourceEffect.java:84-89`   | `TargetControls$` read by presence only; `tgtPlayers.get(0)` unguarded, `IndexOutOfBoundsException` once the chooser has left           | `TargetControls$` rejected            | Not reported |
| `ChooseSourceEffect.java:131-133` | Pool exhausted before every chooser has picked: the do/while rejects the divider cards forever, the game hangs                          | `error` for the chooser left empty    | Not reported |
| `Player.java:3434-3436`           | `getMonarchSet` condition inverted (`monarchEffect == null ? monarchEffect.getSetCode() : null`); sibling `getInitiativeSet` is correct | Not reached: `BecomeMonarch` unported | Not reported |
