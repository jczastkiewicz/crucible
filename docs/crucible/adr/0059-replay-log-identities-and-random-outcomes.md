# ADR-0059 — Replay Log: Structural Identities, Random Outcomes per Site, JSON Lines

- **Status:** Accepted
- **Date:** 2026-11-05
- **Deciders:** Crucible session (R21 preparation), accepted by `jc@archlab.pl`

## Context

ADR-0010 L3 replays a Java AI-vs-AI game in Go. A `RecordingPlayerController` logs every decision "as stable
identifiers, together with the resulting shuffle permutations". The `ReplayController` asserts that the option set
matches before it consumes an answer. ADR-0006 lets RNG consumption counts differ because outcomes are replayed, not
draws. Neither ADR says what a stable identifier is, which random outcomes beyond shuffles are recorded, or what the
file is. The Java facts settle most of it:

| Fact                                                                                                                                                                                                                         | Where                                                                           | Consequence                                                           |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `PlayerController` has 112 abstract methods: 30 choices from a list, 17 booleans, 10 orderings, 9 numbers, 12 strings/names/colors, 8 spell-ability choices, 6 combat, 4 mana, 3 targets, 3 dice, 9 notifications, 1 factory | `PlayerController.java:100-396`                                                 | Records are typed per answer kind, not per method                     |
| Card ids are a per-game counter; a token or copy takes the next one; a zone change keeps the id                                                                                                                              | `Game.java:1166-1168`; `TokenInfo.java:121`; `GameAction.java:207`              | Card ids are usable, but Go's `CardID` allocation order is not Java's |
| `SpellAbility`, `Trigger`, `ReplacementEffect` and `StaticAbility` ids come from JVM-global static counters                                                                                                                  | `SpellAbility.java:78-79`; `Trigger.java:55-56`; `ReplacementEffect.java:47-48` | Not stable across games in one JVM; never logged                      |
| The replacement-effect choice list comes from a `HashMap` keyset                                                                                                                                                             | `ReplacementHandler.java:676-703`                                               | List index is not stable; the answer must be logged by identity       |
| `MyRandom` is one unseeded `SecureRandom`, shared by the engine (shuffles, dice, coins, `AtRandom`, `Count$Random`: about 30 sites) and the AI (about 87 uses); `setRandom` replaces it                                      | `MyRandom.java:12`, `:46`; `forge-ai`                                           | Engine outcomes cannot be reproduced from a seed                      |

The Go engine reads one `*javarand.Rand` at 40 call sites in 25 files.

## Decision Drivers

- ADR-0006: a Go site that draws one extra number must not shift every later outcome.
- ADR-0010: a mismatched option set is reported as a legality bug, distinct from a diverging event stream.
- REV-1: no `forge-game` edit. The recorder lives in `crucible/oracle-java`, test-scoped.

## Considered Options

1. **Log raw `MyRandom` draws, and replay them in order through `javarand`.** Rejected: it requires every Go random site
   to draw exactly as Java's does. That is the coupling ADR-0006 removes, and one extra draw would make every later
   outcome look like a rules bug.
2. **Seed both engines identically.** Rejected: Java's AI draws from the same generator, so any AI difference moves the
   engine's draws, and `MyRandom` is unseeded.
3. **Log each random outcome at its site, keyed by site kind. Go's random sites ask a replay source for that kind's next
   outcome.** Chosen.

## Decision

**Identities.**

| Object                                | Identity in the log                                                                                                                                                                                                      |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Player                                | Seat index                                                                                                                                                                                                               |
| Card                                  | Java's per-game id. The replay keeps a Java-id-to-`CardID` map: deck cards are paired by seat and deck position at setup, every later card at its creation event, in order. A creation Go does not match is a divergence |
| Ability, trigger, static, replacement | `(host card, section, index)`: section is the definition's ability, trigger, static or replacement list, or a keyword trait (keyword index, trait index), or a grant (grantor, index). Never a Java object id            |
| Stack item                            | Its ability identity plus its stack position at the time of the decision                                                                                                                                                 |

**Decisions.** One record per controller call holds:

- the method name and the answer kind;
- the identities of the offered options, compared as a set where Java builds the list from a hash container
  (`chooseSingleReplacementEffect`) and as a sequence elsewhere;
- the answer, given as identities, never as an index into the list.

The option-set assertion of ADR-0010 runs on the offered field.

**Random outcomes.** The recorder installs a recording `Random` through `MyRandom.setRandom`. It attributes each draw
made outside a wrapped controller call to its engine site by the calling frame. It logs the site's outcome, not the
draw:

- a shuffle logs the resulting library order as card identities;
- a die or a coin logs its face;
- an `AtRandom` choice logs the chosen identities;
- `Count$Random` logs its value.

Draws made inside a controller call are the AI's and are not logged. In replay mode Go's random sites stop reading
`javarand` and ask the replay source for the next outcome of their kind. An outcome of the wrong kind, or none, is a
divergence report naming both sites.

**File.** JSON Lines read with `encoding/json`. The first line is a header: format version, Forge commit, deck lists,
seat order, and the game's starting library orders. Then one record per decision or random outcome, in Java's order. Any
field change bumps the version, and the replay refuses a version it does not know.

## Consequences

**Good:** Engine randomness and AI answers can no longer be mistaken for each other, and a Go site drawing differently
from Java changes nothing. Logging every answer by identity makes the hash-ordered lists harmless. The log is diffable
text.

**Bad:** Attributing a draw by calling frame ties the recorder to Forge's class and method names, so an upstream rename
breaks it. The upstream-sync runbook gains a recorder test run. Every Go random site needs a site kind, which adds a
small API across 25 files.

**Neutral:** The card-id map makes creation order observable. A token Go creates that Java did not is reported at
creation, earlier than any event diff would show it.

## Related

ADR-0006, ADR-0010, ADR-0012, ADR-0013
