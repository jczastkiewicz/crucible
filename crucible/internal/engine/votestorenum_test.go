package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// TestVoteStoreVoteNumFeedsRepeatEachAmountFromVotes proves a Vote without
// Choices$ stores each option's count (VoteEffect.java:164-167) and RepeatEach
// reads it per player as the Votes SVar (RepeatEachEffect.java:216-229):
// Cirdan the Shipwright's "draws a card for each vote they received".
func TestVoteStoreVoteNumFeedsRepeatEachAmountFromVotes(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	libraryCards(t, g, p, 5)
	libraryCards(t, g, other, 5)
	c := engine.NewScriptedController()
	c.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(other)})
	c.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(other)})
	def := etbChainDef(t, "Test Council",
		"DB$ Vote | Defined$ Player | Secretly$ True | VotePlayer$ Player | StoreVoteNum$ True | SubAbility$ Rep",
		"Rep", "DB$ RepeatEach | RepeatPlayers$ Player | RepeatSubAbility$ Draw | AmountFromVotes$ True",
		"Draw", "DB$ Draw | Defined$ Remembered | NumCards$ Votes")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := len(g.Zone(engine.Hand, other).Cards()); got != 2 {
		t.Errorf("voted-for player's hand = %d, want 2", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 0 {
		t.Errorf("unvoted player's hand = %d, want 0", got)
	}
}
