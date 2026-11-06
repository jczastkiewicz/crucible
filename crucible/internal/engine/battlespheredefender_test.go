package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// Myr Battlesphere's attack trigger deals X damage to the player or
// planeswalker it attacks: Defined$ TriggeredDefender.Opponent names a player
// defender, Valid Planeswalker.TriggeredDefender a planeswalker one, and the
// two halves never both name the same entity (a Java test on upstream/master
// shows a planeswalker taking X once).
func TestMyrBattlesphereDamagesTheAttackedDefenderOnce(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		planeswalker bool
	}{
		{"player", false},
		{"planeswalker", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.DeclareAttackers)
			sphere := g.NewCard(corpusCard(t, "Myr Battlesphere"), p, engine.Battlefield)
			myr := []engine.CardID{
				g.NewCard(corpusCard(t, "Alloy Myr"), p, engine.Battlefield),
				g.NewCard(corpusCard(t, "Alloy Myr"), p, engine.Battlefield),
			}
			pw := g.NewCard(corpusCard(t, "Ugin, the Spirit Dragon"), other, engine.Battlefield)
			g.Card(pw).Counters.Add(engine.Loyalty, 7)
			for _, id := range append(myr, sphere, pw) {
				g.Card(id).SummonSick = false
			}
			sba(g)
			target := engine.PlayerEntity(other)
			if tc.planeswalker {
				target = engine.CardEntity(pw)
			}
			c := engine.NewScriptedController()
			c.QueueAttackers([]engine.CardID{sphere})
			c.QueueAttackTarget(target)
			c.QueueConfirmOptionalTrigger(true)
			c.QueuePayX(2)
			c.QueueTapChoice(myr)
			declareAttackers(t, g, c)
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatal(err)
			}
			life, loyalty := g.Player(other).Life, g.Card(pw).Counters.Count(engine.Loyalty)
			wantLife, wantLoyalty := 18, 7
			if tc.planeswalker {
				wantLife, wantLoyalty = 20, 5
			}
			if life != wantLife || loyalty != wantLoyalty {
				t.Errorf("defender at %d life, planeswalker at %d loyalty; want %d and %d", life, loyalty, wantLife, wantLoyalty)
			}
		})
	}
}
