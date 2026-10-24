package engine_test

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jczastkiewicz/crucible/internal/smoke"
)

// corpusSweepStride is the sampling step over the sorted corpus: one card in
// every stride is cast, resolved, activated and played three turns by the
// smoke prober (internal/smoke), so the engine's per-effect branches run on
// real scripts. The whole corpus is `crucible card-smoke -all`.
const corpusSweepStride = 2

// corpusSweepHangLimit is how long one card may take before it counts as hung.
// A card plays in seconds alone (Aang's Defense and Surge of Righteousness take
// under five), but the sweep runs eight workers beside the rest of the package
// under -race on a two-core CI runner, where 30 s was a false hang; a real loop
// still ends here and in go test's -timeout.
const corpusSweepHangLimit = 3 * time.Minute

// TestCorpusSampleNeverPanicsOrHangs is the batch guarantee behind GO-7: no
// sampled card panics the engine or loops it. A card the engine cannot play
// is refused with an error, which is not a failure here.
func TestCorpusSampleNeverPanicsOrHangs(t *testing.T) {
	t.Parallel()

	names := scenarioDB(t).Names()
	sort.Strings(names)
	var sample []string
	for i := 0; i < len(names); i += corpusSweepStride {
		sample = append(sample, names[i])
	}
	prober := smoke.NewProber(scenarioDB(t))
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bad []smoke.Result
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				res := prober.ProbeWithin(name, corpusSweepHangLimit)
				if res.Outcome == smoke.Panicked || res.Outcome == smoke.Hung {
					mu.Lock()
					bad = append(bad, res)
					mu.Unlock()
				}
			}
		}()
	}
	for _, name := range sample {
		jobs <- name
	}
	close(jobs)
	wg.Wait()
	for _, res := range bad {
		t.Errorf("%s: %v at %s: %s", res.Card, res.Outcome, res.Stage, res.Detail)
	}
}
