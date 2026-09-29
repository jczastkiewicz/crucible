# Porter plan: ControlPlayer (ADR-0030)

Scratch scaffolding for whoever resumes this branch; deleted in the final commit.

## Scope

- `ControlPlayer` (11 corpus lines) per ADR-0030.
- Readers: Learn (`Player.java:3906`), Wish-family ChangeZone (`ChangeZoneEffect.java:989`).

## Findings vs ADR

- `changezoneeffect.go:68` rejects `Origin$ Sideboard` today, so the Wish reader is not live. Port it: allow Sideboard,
  route it hidden (`ZoneType.java:23`, `SpellAbility.isHidden`), per-fetcher exclusion for a controlled player.
- ADR's "turn.go:92-93 order already gives revoke-before-grant" is inverted (keyed runs first there). Own runner:
  unkeyed (revokes) snapshot first, then keyed-to-active (grants) snapshot.
- Cruel Entertainment unreachable (TargetUnique$, sub-ability targets, `Controller$ ParentTarget`): fails loudly.
- Secret of Bloodbending: `Condition$ OptionalCost` unported, rejected loudly; `Combat$` itself ported and tested.

## Steps / commits

1. Plan (this file).
2. Engine state: `Player.controlledBy` stack, `Game.scheduled`, runner at turn boundary / CombatBegin / endCombat,
   `ControllingPlayer`/`IsControlled`, loss frees mindslaves, Clone. Tests.
3. `controlplayereffect.go` + registry regen + tests.
4. Readers: Learn, ChangeZone Sideboard. Tests.
5. Docs: `effects-controlplayer.md`, index row, counts (184), plan list.
