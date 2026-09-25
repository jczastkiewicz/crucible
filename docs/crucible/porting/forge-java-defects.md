# Forge Java Defects

- **Status:** Active

Bugs in upstream Forge's Java that the port found: a method that cannot do what its name says, an unguarded index, a
loop that never ends. PORT-8: reported and fixed upstream, never compensated in Go. Reason: a workaround makes Crucible
disagree with the oracle for a reason no diff can explain. Card-script bugs go in
[card-script-defects.md](card-script-defects.md) instead.

A row lands in the same commit as the port that found it. What Crucible does meanwhile is fail closed — an `error` or a
rejected param — never a silent fix. "Upstream" tracks the PR filed against
[Card-Forge/forge](https://github.com/Card-Forge/forge); update it in place once one exists (DOC-16).

Rows start with the ChooseSource/Empower batch. Bugs noted before it are only in their own port-log sections.

## Status

| Site                              | Defect                                                                 | Crucible meanwhile                       | Upstream  |
| --------------------------------- | ---------------------------------------------------------------------- | ---------------------------------------- | --------- |
| `ChooseSourceEffect.java:84-89`   | `tgtPlayers.get(0)` unguarded; throws once the player list is empty    | `TargetControls$` rejected               | Not filed |
| `ChooseSourceEffect.java:131-133` | Pool exhausted before every chooser has picked hangs the game          | `error` for the chooser left empty       | Not filed |
| `Player.java:3435`                | `getMonarchSet` ternary condition inverted                             | No counterpart: no set codes in Crucible | Not filed |
| `GameAction.java:2568-2573`       | `takeInitiative` has no `return` after passing a lost player's take on | Reproduced (oracle parity)               | Not filed |

### `ChooseSourceEffect.java:84-89` — `TargetControls$` throws on an empty player list

```java
if (sa.hasParam("TargetControls")) {
    permanentSources = CardLists.filterControlledBy(permanentSources, tgtPlayers.get(0));
    stackSources = CardLists.filterControlledBy(stackSources, tgtPlayers.get(0));
    referencedSources = CardLists.filterControlledBy(referencedSources, tgtPlayers.get(0));
    commandZoneSources = CardLists.filterControlledBy(commandZoneSources, tgtPlayers.get(0));
}
```

`tgtPlayers` is `getTargetPlayers(sa)` (line 36): every `Target$`/`Defined$` player still in the game, in order. This
block reads only `tgtPlayers.get(0)`, so:

- **Crash.** If every player the ability targets or defines has already left the game by the time it resolves (last
  player standing concedes, a `Defined$ TargetedPlayer` reference to a player removed earlier in the chain),
  `sourcesToChooseFrom.isEmpty()` at line 117 is checked, but this line runs first and `tgtPlayers.get(0)` on an empty
  `List<Player>` throws `IndexOutOfBoundsException` before that guard is reached.
- **Wrong player on multi-target.** An ability that targets more than one player (`TargetsWithDefinedController` or
  similar, though no corpus card currently does this with `TargetControls$`) filters every player's source list by the
  _first_ target's controller, not each chooser's own reference point — `TargetControls$` reads "the player the
  ability's target controls," which for chooser N ≠ 0 should presumably still be `tgtPlayers.get(0)` by the param's own
  definition (it does not vary per chooser), so this half is arguably intended; only the empty-list crash is the bug.

**Proposed fix:** guard on `!tgtPlayers.isEmpty()` alongside the existing `hasParam("TargetControls")` check, matching
how `sourcesToChooseFrom.isEmpty()` is later treated as a no-op rather than an error:

```java
if (sa.hasParam("TargetControls") && !tgtPlayers.isEmpty()) {
```

**Crucible meanwhile:** `internal/engine/choosesourceeffect.go` rejects `TargetControls$` outright with an `error` — Go
has no card in the corpus exercising it, and reproducing an upstream crash as a Go `panic` would violate GO-7 (a bad
card script must not kill a batch).

### `ChooseSourceEffect.java:131-133` — exhausted pool hangs the game

```java
Card o = null;
do {
    o = p.getController().chooseSingleEntityForEffect(sourcesToChooseFrom, sa, choiceTitle, null);
} while (o == null || o.getName().startsWith("--"));
```

`sourcesToChooseFrom` is mutated by `sourcesToChooseFrom.remove(o)` (line 135) after every pick, once per chooser, up to
`validAmount` times per player. The loop's only valid exit is a controller returning a non-null, non-divider card. If
the pool empties before every chooser has picked their `validAmount` (a card with `Amount$` > 1 and few valid sources,
or several `tgtPlayers` competing for the same short pool), `chooseSingleEntityForEffect` has nothing left to return but
`null`, and the `do`/`while` spins forever — the controller is asked to choose from an empty list on every iteration
with no way to signal "no valid choice." This is a real hang, not a slow path: nothing bounds the iteration count or
breaks on an empty `sourcesToChooseFrom`.

**Proposed fix:** break out (and let the ability under-deliver, matching how `sourcesToChooseFrom.isEmpty()` at the top
of `resolve` is already treated as a no-op) when the pool is empty before a pick:

```java
do {
    if (sourcesToChooseFrom.isEmpty()) {
        break;
    }
    o = p.getController().chooseSingleEntityForEffect(sourcesToChooseFrom, sa, choiceTitle, null);
} while (o == null || o.getName().startsWith("--"));
if (o == null) {
    continue;
}
```

**Crucible meanwhile:** `choosesourceeffect.go` returns an `error` —
`engine: ChooseSource: no source left for chooser %d` — the moment a chooser's pool is empty, rather than reproducing
the hang.

### `Player.java:3435` — `getMonarchSet` ternary is inverted

```java
public String getMonarchSet() {
    return monarchEffect == null ? monarchEffect.getSetCode() : null;
}
```

Compare the sibling three lines down, which is correct:

```java
public String getInitiativeSet() {
    return initiativeEffect != null ? initiativeEffect.getSetCode() : null;
}
```

`getMonarchSet` has the condition and the branches both backwards from `getInitiativeSet`'s pattern:

- When `monarchEffect == null` (no one is the monarch, or `createMonarchEffect` hasn't run), the "true" branch calls
  `monarchEffect.getSetCode()` on a null reference — `NullPointerException`.
- When `monarchEffect != null` (there is a monarch and its set code is known), the "false" branch returns `null` instead
  of the set code — the caller (the monarch UI badge's set icon) silently gets nothing instead of the set that should
  never NPE.

Either path is wrong: it either throws whenever anyone would call it while there is no monarch, or (if some caller
happens to only invoke it after already checking `isMonarch()`/similar) returns `null` when a real value exists. There
is no code path where this method returns a useful non-null result.

**Proposed fix:** match `getInitiativeSet`'s pattern exactly:

```java
public String getMonarchSet() {
    return monarchEffect != null ? monarchEffect.getSetCode() : null;
}
```

The one game-rules caller is `Game.java:992/994`, passing the monarchy on when the monarch loses: `monarchEffect` is set
there, so it passes `null` rather than throwing.

**Crucible meanwhile:** no counterpart. `BecomeMonarch` is ported (`becomemonarcheffect.go`), but this port carries no
set codes and `becomeMonarch` takes none, so there is nothing for the defect to reach.

### `GameAction.java:2568-2573` — `takeInitiative` gives the initiative to a player who has lost

```java
if (!p.equals(previous)) {
    if (previous != null) {
        previous.removeInitiativeEffect();
    }

    if (p.hasLost()) { // the person who should take initiative is gone, it goes to next player
        takeInitiative(game.getNextPlayerAfter(p), set);
    }

    game.setHasInitiative(p);
    p.createInitiativeEffect(set);
}
```

The `hasLost` branch passes the take to the next player, as its comment says, and then falls through: `p` -- the player
who has lost -- becomes the holder and gets a new "The Initiative" card, while the next player's card, created by the
recursive call, is never removed. Both players then hold a card; only the lost one is `game.getHasInitiative()`.

Reached from `Game.onPlayerLost` (`Game.java:998-1006`) when the holder and the active player lose in the same
`checkGameOverCondition` pass: the holder's initiative passes to the active player, who has lost too.
`TakeInitiativeEffect` never reaches it (it skips players not in the game).

**Proposed fix:** return after the recursive call.

```java
if (p.hasLost()) {
    takeInitiative(game.getNextPlayerAfter(p), set);
    return;
}
```

**Crucible meanwhile:** reproduced. `takeinitiativeeffect.go`'s `takeInitiative` runs the same code, so the lost active
player ends holding the initiative (`TestInitiativeToALostActivePlayerReproducesJava`). A state bug, not a crash or a
hang, so matching the oracle wins over correcting it in Go alone (PORT-8).
