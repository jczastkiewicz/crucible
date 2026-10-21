package engine

// package engine, not engine_test (TEST-2's "invariant not observable from
// outside" row): a Defined$ read outside the trigger or cost that records
// what it names fails loudly instead of answering empty; no card script
// reads one there, so no scenario reaches the error.

import "testing"

func TestDefinedWithoutItsRecordedContextFailsLoudly(t *testing.T) {
	t.Parallel()

	g := targetCandidatesTestGame(t, "a", "b")
	p := g.Players()[0]
	host := g.NewCard(targetCandidatesTestCreature(t), p, Battlefield)

	for _, defined := range []string{
		"ParentTargetedController", "TriggeredAttackerController", "TriggeredBlockerOwner",
		"TriggeredCardController", "TriggeredCardOpponent", "TriggeredDefendingPlayer",
		"TriggeredOriginalController", "TriggeredAttackingPlayer", "TriggeredActivator",
	} {
		if got, err := definedPlayers(g, p, host, defined, abilityRefs{}); err == nil {
			t.Errorf("definedPlayers(%q) = %v, want an error without the recorded context", defined, got)
		}
	}
	for _, defined := range []string{
		"Tapped", "Discarded", "Exiled", "TriggeredBlocker", "TriggeredAttacker", "TriggeredSources",
		"TriggeredCards", "ReplacedCard", "TopOfLibrary", "TriggeredCard",
	} {
		if got, err := definedCards(g.Card(host), defined, abilityRefs{}); err == nil && len(got) != 0 {
			t.Errorf("definedCards(%q) = %v, want an error or nothing without the recorded context", defined, got)
		}
	}
	if _, err := definedCards(g.Card(host), "TriggeredSources", abilityRefs{}); err == nil {
		t.Error("TriggeredSources without sources recorded resolved")
	}
	if _, err := definedCards(g.Card(host), "Discarded", abilityRefs{}); err == nil {
		t.Error("Discarded with no cost recorded resolved")
	}
	if _, err := definedCards(g.Card(host), "TopOfLibrary", abilityRefs{}); err == nil {
		t.Error("TopOfLibrary without the game resolved")
	}
}
