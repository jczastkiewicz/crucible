# ADR-0021 — Phasing: the Battlefield Enumeration Excludes Phased-Out Permanents

- **Status:** Proposed
- **Date:** 2026-09-26
- **Deciders:** `mc@archlab.pl`

## Context

CR 702.26b: a phased-out permanent is treated as though it does not exist. It stays in the battlefield zone — phasing is
not a zone change, so no leaves-the-battlefield or enters trigger fires (CR 702.26d) — and it keeps its counters, damage
and attachments.

`Phases` is M6's largest unported API: 72 corpus lines (`DB$` 54, `AB$` 11, `SP$` 7; `Defined$` 42, `ValidTgts$` 22,
`AllValid$` 11, `PhaseInOrOut$` 8, `WontPhaseInNormal$` 5), plus `K:Phasing` on 13 cards. Examples: Teferi's Protection,
Slip Out the Back, Oubliette, Guardian of Faith.

Java gets "does not exist" from one place. `PlayerZoneBattlefield.getCards(filter)` drops phased-out cards by default,
returning the original list untouched when none is phased out (`PlayerZoneBattlefield.java:76-100`); `Game.getCardsIn`
and `Player.getCardsIn` route through it (`Game.java:611`, `Player.java:1313`, `:1320`). A handful of callers opt in to
see them: the untap-step phasing action (`Untap.java:202`), `PhasesEffect`'s phase-in branch
(`PhasesEffect.java:47-48`), the GameState dump (`GameState.java:182`, `:223`), a full-battlefield visitor
(`Game.java:772`). Per-card `isInPlay()` is only a zone check (`Card.java:7123-7125`) and does not exclude phased-out
cards; callers that must also check `isPhasedOut()` do so explicitly (`GameAction.java:977`).

This port reads the battlefield two ways: `g.Zone(Battlefield, pid).Cards()` at 73 non-test sites (targeting, combat,
SBAs, statics, `traitHosts` at `game.go:341`), and `c.Zone == Battlefield` at 52. `Zone.Cards()` returns the zone's own
ordered slice (`zone.go:87`). No site knows phasing; `destroyeffect.go:103` names it as not ported.

## Decision Drivers

- PORT-7 parity: where Java filters and where it does not is observable (an SBA, a trigger host, a target list).
- GO-12: battlefield order breaks ties between simultaneous triggers, so phasing must not reorder the zone.
- Auditing 73 enumeration sites by hand is how one gets missed silently; the default must be the safe one.
- Zero cost when nothing is phased out, which is nearly every game.

## Considered Options

1. **A `PhasedOut` flag each caller checks.** Rejected: 73 sites, and every future one, must remember it.
2. **Move a phased-out card out of the zone's ordered set.** Rejected: phasing back in re-appends it, changing
   battlefield order (GO-12), and every zone-membership check starts lying about CR 702.26d.
3. **The battlefield enumeration excludes phased-out cards by default, order-preserving, with a named opt-in; per-card
   zone checks stay unfiltered, as in Java.** Chosen.

## Decision

1. **`Zone(Battlefield, pid).Cards()` excludes phased-out permanents**, keeping the relative order of the rest, and
   returns the backing slice unchanged when none is phased out. Every existing enumeration site gets CR 702.26b for
   free. Other zones are unaffected.
2. **One opt-in accessor returns the full battlefield, phased-out included.** Only these may call it: the untap-step
   phasing action, `Phases`' phase-in branch, the fixture dump and load (`PhasedOut:` round-trip, TEST-5), `Game.Clone`
   and LKI snapshots, and a static that governs phasing itself (`StaticAbilityCantPhase`). A new caller is a review
   item, cited against the Java opt-in it mirrors.
3. **`c.Zone == Battlefield` keeps meaning "in the battlefield zone"**, exactly Java's `isInPlay()`. A per-card site
   that Java guards with `isPhasedOut()` gets the same guard when its mode is ported, cited to the Java line. No blanket
   change to the 52 sites.
4. **Phasing is its own operation, never `Move`.** It emits no zone-change event and no ETB/LTB trigger; it fires the
   `PhaseIn`/`PhaseOut` trigger modes, removes a phasing-out permanent from combat (`Card.java:5647-5650`), phases
   attachments indirectly (CR 702.26g, `Card.java:5602-5609`), and records the player on whose untap step it phases back
   in (`Card.java:5645`, read back by `isPhasedOut(Player)`, `:5575`). It is a new event kind — an ADR-0013 schema
   increment in the implementing commit.
5. **Phased-out state lives with the zone and the card, and `Game.Clone` copies both.** Which structure carries it is
   the implementing PR's choice, provided decision 1's order and zero-cost properties hold.

## Consequences

**Good:** unblocks `Phases` (72 lines) and `K:Phasing` (13 cards). All 73 enumeration sites, and every future one, get
the rule without an audit. Battlefield order survives phasing.

**Bad:** the two read paths now disagree for a phased-out card — the enumeration hides it, `c.Zone` still says
Battlefield. That is Java's own split, reproduced on purpose, but a per-card site that forgets its guard is wrong
silently. Mitigation: `Phases`' own implementing PR lists every `c.Zone == Battlefield` site Java guards and ports those
guards with it.

**Neutral:** a scenario with nothing phased out takes the unchanged-slice path; no fixture moves.

## Related

ADR-0009 (game state representation), ADR-0013 (event schema), GO-12, [04-adr-process](../guidelines/04-adr-process.md)
