---
name: port-effect
description:
  Port one or more Forge ApiType effects (M6) to crucible/internal/engine - effect file, registry wiring, enginelint
  group, engine_test test, port-log section and API counts, in one commit. Use when asked to port, implement or land an
  effect such as "SetState", "Play", "CopySpellAbility", or "the next M6 effects".
---

# Port an M6 effect

Every step is enforced by a gate or has broken a commit before. Do them all, in order, for each API. Batch several APIs
per commit (memory: port in bigger packs), but every API gets every step.

## 0. Research first

Delegate to the `forge-oracle` subagent: "forge-oracle: <Api>". Its answer gives the params the Java reads, resolution
order, controller decisions, corpus shapes and PORT-8 bugs. Do not read `forge-game/` inline unless it left a gap.

Pick which params to resolve by corpus count. Everything else is rejected with an `error` (step 1), never silently
ignored.

## 1. Write `crucible/internal/engine/<api>effect.go`

File name is the API name lowercased plus `effect.go`. Shape, from `healdamageeffect.go`:

```go
package engine

import "fmt"

// <api>Effect is <Api>Effect.java: <one-line semantics, CR rule if any>.
//
// Ported from forge-game/src/main/java/forge/game/ability/effects/<Api>Effect.java's resolve.
type <api>Effect struct{}

func (<api>Effect) Resolve(g *Game, a *Ability, c PlayerController) error {
	// Reject every param this port does not resolve, before acting (PORT-8, GO-7).
	for _, key := range [...]string{"<Unresolved1>", "<Unresolved2>"} {
		if _, ok := a.Params.Param(key); ok {
			return fmt.Errorf("engine: <Api>: %s$ not resolvable yet", key)
		}
	}
	source := g.Card(a.Source)
	if !subAbilityConditionMet(g, source, a.Amounts, a.Params) {
		return nil
	}
	// ... resolve. Script-reachable failure -> error, never panic (GO-7).
	return nil
}
```

Rules that bite here:

| Rule   | Do                                                                                              |
| ------ | ----------------------------------------------------------------------------------------------- |
| GO-12  | Iterate `collect.OrderedSet`/`OrderedMap` or slices, never a bare `map`, where order is visible |
| GO-9   | Hold `CardID`/`PlayerID`, never `*Card`                                                         |
| GO-8   | No `any`; read params through `a.Params`                                                        |
| PORT-8 | Forge bug found -> stop, report file:line, do not compensate in Go                              |
| lint   | No `min`/`max` as identifiers (revive); blank line between a file comment and `package engine`  |

Reuse helpers before writing new ones: `targetedOrDefinedCards`, `targetedOrDefinedPlayers` (`defined.go`),
`resolveNamedAmount` (`amount.go`), `optionalAmount`, `checkChoice` (`effecthelpers.go`), `moveByEffect`
(`zonemove.go`).

## 2. Regenerate the registry

`NewRegistry()` is generated (ADR-0017): type `<api>Effect` with a value-receiver `Resolve` registers as `API<Api>`
automatically. Only a type serving several APIs, another API, or needing field values adds doc-comment lines:

```go
//crucible:register Manifest manifestEffect{api: "Manifest", remember: "RememberManifested"}
```

Then regenerate; never edit `registry_gen.go` by hand:

```bash
cd crucible && go generate -run genregistry ./internal/engine
```

On a merge conflict in `registry_gen.go`, take either side and rerun `go generate`.

## 3. `enginelint` group, in the effect file

The file declares its own group (named after the file) right after the package clause. Copy a similar effect's list,
e.g. `healdamageeffect.go`:

```go
package engine

//enginelint:allow card game ability defined condition control parts
```

Do not touch `enginelint.json` for an effect. Run until clean:

```bash
cd crucible && go run ./tools/enginelint -config internal/engine/enginelint.json
```

## 4. Test in `package engine_test`

At least two players: a one-player game ends at the first SBA check (CR 104.2a) and later stack items never resolve. Put
tests in a file named for behavior (TEST-3), e.g. an existing `pack*effects_test.go` or a new one. Check every returned
`error` (errcheck is a CI gate).

```go
func Test<Api>Effect<WhatItProves>(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ <Api> | <Param>$ <Value>")
	// assert on g / g.Card(host) / g.Player(other)
}
```

Helpers: `newTwoPlayerGame`, `resolveLine` (fails the test on error), `resolveNow` (returns the error; use it to assert
a rejected param), `creatureDefPT`, `libraryCards`, `etbChainDef` + `castETBChain` for a `SubAbility$` chain. Queue
controller answers on `engine.NewScriptedController()` (`QueueEntityChoice`, `QueueAbilityChoice`, ...).

Add one test per rejected param shape that matters: assert `resolveNow` returns the `not resolvable yet` error.

## 5. Docs, same commit (DOC-12)

| File                                                           | Change                                                                                                                   |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `docs/crucible/porting/port-log/game-state/effects-batches.md` | New `## <Apis> land` section at the end: what resolves, what is rejected and why, shared engine pieces, Java `path:line` |
| `docs/crucible/porting/port-log/game-state.md`                 | Index row for the new section; update the remaining-effects sentence in `## Not ported yet` (`M6's N remaining ...`)     |
| `docs/crucible/00-master-implementation-plan-in-progress.md`   | Resolved-API count                                                                                                       |
| `CLAUDE.md`                                                    | `M6 in progress: N of the corpus's 203` count and largest gaps                                                           |

Count = registered APIs minus the three M5 casting entries. `genregistry -check` (in `gates.sh` and CI) prints it and
fails when `CLAUDE.md` or the plan quotes a different number:

```bash
cd crucible && go run ./tools/genregistry -dir internal/engine -check \
    -docs ../CLAUDE.md,../docs/crucible/00-master-implementation-plan-in-progress.md
```

## 6. Verify, then commit

Delegate to `rules-reviewer` on the diff, fix findings, then commit. The commit hook runs `gates.sh full` and blocks on
red; to see failures without the context flood use the `gate-runner` subagent first.
