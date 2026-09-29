# Porter plan: RestartGame (ADR-0034)

Scratch file for whoever resumes this worktree; deleted in the final commit.

## Assigned

- `RestartGame` (karn_liberated.txt:7), implementing ADR-0034.
- `ExiledWithSource` card state + valid property (ADR-0034 Decision, same unit).

## Step order (commit after each)

1. `ExiledWithSource`: `Card.exiledWith {card, stamp, listed}` cleared on every zone entry (put/putFront), set by the
   six Java `handleExiledWith` paths this port has (ChangeZone, ChangeZoneAll, Dig, DigUntil, Heist, Airbend);
   `valid.go` exact `ExiledWithSource` property (host CardID + zoneStamp, LKI stamp when host has left the battlefield,
   covers Karn moved to library mid-resolution and LTB triggers); lift from playeffect/cloneeffect reject lists. Tests.
2. Driver restart signal: `Game.restarted`/`restartedBy` + accessors, Clone; `resolveTop`/`ResolveStack`/
   `priorityRound`/`Step`/`Run` return at once; `Step`/`Run` entry with a pending restart -> error; `ResumeAfterRestart`
   (opening hands without coin flip/shuffle, mulligans with activator first, clear, StartTurn). Subgame + fixture
   `run`/`step` callers handle it.
3. `restartgameeffect.go`: reject-before-acting checks (variant Command cards, planechase, spell cards on the Stack
   zone), per-player Java-order move to owner's library top, shuffle, state clears. Tests. Registry regen.
4. Scenario fixture, port-log `effects-restartgame.md`, index row, counts. gates full.
