# Crucible — Implementation Plan: Open

Milestones M7-M9, not yet started. Roadmap overview and completed milestones (M0-M4):
[00-master-implementation-plan.md](00-master-implementation-plan.md). Currently in progress (M5-M6):
[00-master-implementation-plan-in-progress.md](00-master-implementation-plan-in-progress.md).

---

## Milestones

### M7 — AI port — 5–8 wks

33. `ComputerUtilMana` first (the runner cannot play a real game without mana planning), then `ComputerUtilCombat`,
    `ComputerUtilCard`, `CreatureEvaluator`.
34. `AiController` + per-API `SpellAbilityAi` decision logic; `.ai` profile loading.
35. Lookahead simulator on top of the cheap Go state clone. **Exit gate:** P6 gate — statistical parity within stated
    bounds vs. Java on the reference gauntlet.

### M8 — Simulation runner & telemetry — 2–3 wks

36. `internal/sim`: worker pool, seed management, gauntlet config, turn caps, crash isolation (a panicking game fails
    that game only).
37. `internal/telemetry`: recorder, castability probe, mana sampler, tenure tracking, aggregation.
38. `internal/store`: shard writers, `manifest.json`, and a streaming reader (ADR-0016). **Exit gate:** P7 gate — 100k
    games clean, deterministic re-run byte-identical.

### M9 — Reporting & the optimization loop — 2–4 wks

39. `internal/report`: matchup matrix w/ CIs, dead-card table, mana health, per-card impact (all four attribution modes,
    each labelled), opening-hand analysis, play/draw split.
40. Markdown + HTML + JSON output; `crucible report`.
41. **A/B swap optimizer**: propose candidate swaps from the dead-card and impact tables, run the confirmatory A/B
    gauntlet, rank by measured win-rate delta with CIs. This is the actual product. **Exit gate:** P8 gate — blind-test
    diagnosis matches expert assessment on known-good and known-bad decks.
