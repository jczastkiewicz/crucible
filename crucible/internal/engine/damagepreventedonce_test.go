package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Mode$ DamagePreventedOnce (Selfless Squire's second ability): damage that
// would be dealt to you and is prevented puts that many +1/+1 counters on the
// Squire (CardDamageTable.triggerPreventDamage sums the prevented amount per
// target). The prevention is a replacement effect, as the Squire's own enter
// effect makes it.
func TestDamagePreventedOnceCountsThePreventedAmount(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	squire := g.NewCard(copyTestDef(t, "Squire", "Creature Human Soldier", "1", "1",
		"R:Event$ DamageDone | Prevent$ True | ActiveZones$ Battlefield | ValidTarget$ You",
		"T:Mode$ DamagePreventedOnce | ValidTarget$ You | TriggerZones$ Battlefield | Execute$ TrigPut",
		"SVar:TrigPut:DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ X",
		"SVar:X:TriggerCount$DamageAmount"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()
	if err := resolveWith(t, g, p, c, "DB$ DealDamage | Defined$ You | NumDmg$ 3"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Card(squire).Counters.Count(engine.P1P1); got != 3 {
		t.Errorf("Squire has %d +1/+1 counters, want 3", got)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("life = %d, want 20", got)
	}
}
