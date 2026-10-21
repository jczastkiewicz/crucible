package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Primal Adversary: "you may pay {1}{G} any number of times. When you pay this
// cost one or more times, put that many +1/+1 counters on it": the announced
// count is the SVar the reflexive trigger's condition and counters read.
func TestNumTimesCostPutsThatManyCounters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		times int
	}{{"once", 1}, {"twice", 2}, {"never", 0}} {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.SetTurnState(1, p, engine.Main1)
		wolf := g.NewCard(corpusCard(t, "Primal Adversary"), p, engine.Hand)
		g.Player(p).ManaPool.Add(mana.Green, 1+tc.times)
		g.Player(p).ManaPool.AddColorless(2 + tc.times)
		c := engine.NewScriptedController()
		for range 2 + tc.times {
			c.QueuePayGeneric(mana.ShardC)
		}
		c.QueueConfirmOptionalTrigger(true)
		c.QueuePayX(tc.times)
		castThenResolve(t, g, p, wolf, c)
		if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := g.Card(wolf).Counters.Count("P1P1"); got != tc.times {
			t.Errorf("%s: +1/+1 counters = %d, want %d", tc.name, got, tc.times)
		}
	}
}

// The announced count honours AnnounceMax$, the confirmation and a decline,
// and an unreadable shape fails loudly (nothing is asked).
func TestNumTimesCostBoundsAndRefusals(t *testing.T) {
	t.Parallel()

	const body = "AB$ GainLife | Cost$ Mana<COST\\NumTimes> | Announce$ NumTimes | AnnounceMax$ MAX | ConditionCheckSVar$ NumTimes | Defined$ You | LifeAmount$ 1"
	for _, tc := range []struct {
		name, cost, max string
		confirm         bool
		x               int
		life            int
		wantErr         string
	}{
		{"capped by AnnounceMax", "W", "2", true, 5, 21, ""},
		{"declined trigger", "W", "2", false, 1, 20, ""},
		{"declined count", "W", "2", true, -1, 20, ""},
		{"bad mana", "ZZ", "2", true, 1, 20, "Cost$"},
		{"unreadable max", "W", "Nope", true, 1, 20, "AnnounceMax$"},
		{"not a mana cost", "", "2", true, 1, 20, "not resolvable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			text := strings.NewReplacer("COST", tc.cost, "MAX", tc.max).Replace(body)
			if tc.cost == "" {
				text = strings.Replace(text, "Mana<\\NumTimes>", "Discard<1/Card>", 1)
			}
			placeWatcher(t, g, p, text)
			g.Player(p).ManaPool.Add(mana.White, 2)
			c := engine.NewScriptedController()
			c.QueueConfirmOptionalTrigger(tc.confirm)
			c.QueuePayX(tc.x)
			c.QueuePayGeneric(mana.ShardW)
			err := fireWatcher(t, g, p, c)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ResolveStack = %v, want an error naming %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := g.Player(p).Life; got != tc.life {
				t.Errorf("life = %d, want %d", got, tc.life)
			}
		})
	}
}

func TestNumberEntityIsNeitherACardNorAPlayer(t *testing.T) {
	t.Parallel()

	n := engine.NumberEntity(7)
	if got, ok := n.AsNumber(); !ok || got != 7 {
		t.Errorf("AsNumber = %d, %v, want 7, true", got, ok)
	}
	if n.IsCard() || n.IsPlayer() {
		t.Error("a number entity reads as a card or a player")
	}
	if _, ok := engine.CardEntity(3).AsNumber(); ok {
		t.Error("a card reads as a number")
	}
	if _, ok := engine.PlayerEntity(1).AsNumber(); ok {
		t.Error("a player reads as a number")
	}
}
