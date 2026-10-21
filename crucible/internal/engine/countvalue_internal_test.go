package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): a Count$ head read for no card at all (an amount evaluated
// outside any host) answers "not resolvable" per head; no card script puts a
// head in that position, so no scenario reaches it.

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/expr"
)

func TestCountValueWithoutAHostCardIsUnresolved(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p := g.Players()[0]
	for _, body := range []string{
		"Intensity", "CardStickers", "CardCounters.P1P1", "CardCounters.ALL", "xPaid", "TriggerRememberAmount",
		"ResolvedThisTurn", "CardManaCost", "CardPower", "CardToughness", "Kicked.2.1", "TimesKicked",
		"ChosenNumber", "ChromaSource.Red", "RememberedSize", "ImprintedSize", "ChosenSize", "RememberedNumber",
		"Converge", "CastTotalManaSpent", "TotalManaSpent", "LastStateBattlefield Creature",
		"TotalDamageDoneByThisTurn", "ExcessDamageReceivedThisTurn",
	} {
		count := expr.ParseCount(body)
		if n, ok := countValue(g, p, NoCard, count); ok {
			t.Errorf("Count$%s = (%d, true) with no host, want unresolved", body, n)
		}
	}
	if n, ok := countValue(g, NoPlayer, NoCard, expr.ParseCount("ThisTurnEntered_Battlefield_Creature")); ok {
		t.Errorf("ThisTurnEntered with no controller = (%d, true), want unresolved", n)
	}
}

// Count$LastState{Battlefield,Graveyard}WithFallback (AbilityUtils.java:
// 1804-1846): a source that was not cast counts 0; a cast one counts the
// matches in its cast-time snapshot, or in the zone now when that is empty.
func TestCountValueLastStateWithFallback(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p, other := g.Players()[0], g.Players()[1]
	def := targetCandidatesTestCreature(t)
	src := g.NewCard(def, p, Stack)
	g.NewCard(def, p, Battlefield)
	g.NewCard(def, other, Battlefield)
	g.NewCard(def, p, Graveyard)
	bf := expr.ParseCount("LastStateBattlefieldWithFallback Creature")
	gy := expr.ParseCount("LastStateGraveyardWithFallback Creature")

	if n, ok := countValue(g, p, src, bf); !ok || n != 0 {
		t.Errorf("uncast source = (%d, %v), want (0, true)", n, ok)
	}
	g.Card(src).wasCast = true
	if n, ok := countValue(g, p, src, bf); !ok || n != 2 {
		t.Errorf("empty snapshot falls back to the battlefield: (%d, %v), want (2, true)", n, ok)
	}
	if n, ok := countValue(g, p, src, gy); !ok || n != 1 {
		t.Errorf("empty snapshot falls back to the graveyards: (%d, %v), want (1, true)", n, ok)
	}
	only := g.Zone(Battlefield, p).Cards()[0]
	g.Card(src).castLast.battlefield = []CardID{only}
	if n, ok := countValue(g, p, src, bf); !ok || n != 1 {
		t.Errorf("snapshot of one card = (%d, %v), want (1, true)", n, ok)
	}
	if _, ok := countValue(g, p, src, expr.ParseCount("LastStateBattlefieldWithFallback")); ok {
		t.Error("a count without a valid string resolved")
	}
}
