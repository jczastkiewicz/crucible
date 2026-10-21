package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// A Mode$ CantPutCounter host entering in the same ChangeZoneAll as a
// planeswalker does not stop its loyalty counters, whichever of the two
// moves first (StaticAbilityCantPutCounter.java:22-25; upstream 6df5eaa3b98's
// Solemnity and Dark Depths); a host already on the battlefield still does.
func TestCantPutCounterIgnoresHostEnteringTogether(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		hostFirst    bool
		hostInPlay   bool
		wantLoyalty  int
		wantHostZone engine.ZoneType
	}{
		{"host moves before the walker", true, false, 4, engine.Battlefield},
		{"host moves after the walker", false, false, 4, engine.Battlefield},
		{"host already in play", true, true, 0, engine.Battlefield},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			var host, walker engine.CardID
			addHost := func() {
				zone := engine.Graveyard
				if tc.hostInPlay {
					zone = engine.Battlefield
				}
				host = g.NewCard(scriptDef(t, "Test Solemnity", "Enchantment", solemnityLine), p, zone)
			}
			if tc.hostFirst {
				addHost()
			}
			walker = g.NewCard(planeswalkerDefLoyalty(t, "4"), p, engine.Graveyard)
			if !tc.hostFirst {
				addHost()
			}
			c := engine.NewScriptedController()
			if tc.hostFirst {
				c.QueueCardOrder([]engine.CardID{host, walker})
			} else {
				c.QueueCardOrder([]engine.CardID{walker, host})
			}
			resolveLine(t, g, p, c,
				"DB$ ChangeZoneAll | ChangeType$ Card.YouOwn | Origin$ Graveyard | Destination$ Battlefield")
			if z := g.Card(host).Zone; z != tc.wantHostZone {
				t.Fatalf("host zone = %v, want %v", z, tc.wantHostZone)
			}
			if got := g.Card(walker).Counters.Count(engine.Loyalty); got != tc.wantLoyalty {
				t.Errorf("loyalty = %d, want %d", got, tc.wantLoyalty)
			}
		})
	}
}
