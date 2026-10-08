package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// The hidden keyword strings AddHiddenKeyword$ grants that the engine now
// reads beyond block legality: the attack-alone pair, the untap-step lock,
// and "count as <name>.".

func TestHiddenKeywordCantAttackAlone(t *testing.T) {
	t.Parallel()
	g, a, _ := combatGame(t)
	loner := g.NewCard(copyTestDef(t, "Shy Elf", "Creature Elf", "2", "2",
		"S:Mode$ Continuous | AffectedDefined$ Self | AddHiddenKeyword$ CARDNAME can't attack alone."),
		a, engine.Battlefield)
	friend := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	checkSBA(g)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{loner})
	_, err := g.DeclareCombatAttackers(ac)
	wantIllegal(t, err, "CR 508.1c")

	g2, a2, _ := combatGame(t)
	loner2 := g2.NewCard(copyTestDef(t, "Shy Elf", "Creature Elf", "2", "2",
		"S:Mode$ Continuous | AffectedDefined$ Self | AddHiddenKeyword$ CARDNAME can't attack alone."),
		a2, engine.Battlefield)
	friend2 := g2.NewCard(creatureDefPT(t, "2", "2"), a2, engine.Battlefield)
	checkSBA(g2)
	declareAttacking(t, g2, loner2, friend2)
	_ = friend
}

func TestHiddenKeywordCanOnlyAttackAlone(t *testing.T) {
	t.Parallel()
	g, a, _ := combatGame(t)
	solo := g.NewCard(copyTestDef(t, "Lone Elf", "Creature Elf", "2", "2",
		"S:Mode$ Continuous | AffectedDefined$ Self | AddHiddenKeyword$ CARDNAME can only attack alone."),
		a, engine.Battlefield)
	friend := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	checkSBA(g)

	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{solo, friend})
	_, err := g.DeclareCombatAttackers(ac)
	wantIllegal(t, err, "CR 508.1c")

	g2, a2, _ := combatGame(t)
	solo2 := g2.NewCard(copyTestDef(t, "Lone Elf", "Creature Elf", "2", "2",
		"S:Mode$ Continuous | AffectedDefined$ Self | AddHiddenKeyword$ CARDNAME can only attack alone."),
		a2, engine.Battlefield)
	g2.NewCard(creatureDefPT(t, "2", "2"), a2, engine.Battlefield)
	checkSBA(g2)
	declareAttacking(t, g2, solo2)
}

// "This card doesn't untap during your next untap step." holds the tapped
// permanent through its controller's untap step for as long as the grant
// stands, and a permanent without it untaps.
func TestHiddenKeywordDoesntUntap(t *testing.T) {
	t.Parallel()
	g, a, b := combatGame(t)
	locked := g.NewCard(copyTestDef(t, "Frozen Elf", "Creature Elf", "2", "2",
		"S:Mode$ Continuous | AffectedDefined$ Self | AddHiddenKeyword$ This card doesn't untap during your next untap step."),
		a, engine.Battlefield)
	free := g.NewCard(creatureDefPT(t, "2", "2"), a, engine.Battlefield)
	g.Card(locked).Tapped, g.Card(free).Tapped = true, true
	checkSBA(g)

	c := engine.NewScriptedController()
	g.SetTurnState(1, b, engine.Cleanup)
	g.AdvancePhase(c)
	if g.ActivePlayer() != a || g.ActivePhase() != engine.Untap {
		t.Fatalf("setup: turn is %v in %v, want a's untap step", g.ActivePlayer(), g.ActivePhase())
	}
	if !g.Card(locked).Tapped {
		t.Error("the locked permanent untapped")
	}
	if g.Card(free).Tapped {
		t.Error("the free permanent stayed tapped")
	}
}

// Pardic Firecat in a graveyard "counts as a card named Flame Burst": Flame
// Burst's damage reads the hasKeyword property over every graveyard.
func TestHiddenKeywordCountAsNameInAGraveyard(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	counter := g.NewCard(cdaDef(t, "Count$ValidGraveyard Card.hasKeywordCARDNAME count as Flame Burst.", "Count$ValidGraveyard Card.hasKeywordCARDNAME count as Flame Burst."), p, engine.Battlefield)
	firecat := g.NewCard(corpusCard(t, "Pardic Firecat"), p, engine.Hand)
	checkSBA(g)
	wantCDAPT(t, g, counter, 0, 0, "Pardic Firecat in hand counts as nothing")
	g.Move(firecat, engine.Graveyard, p)
	// Layer 8 builds the hidden keyword after Layer 7 reads this CDA, so the
	// count follows on the next pass (a Flame Burst resolving reads it later).
	checkSBA(g)
	checkSBA(g)
	wantCDAPT(t, g, counter, 1, 1, "Pardic Firecat in a graveyard counts as Flame Burst")
}

// The CheckSVar$-style amounts the layers can now evaluate: Imprinted$Valid,
// PlayerCountOpponents$HighestCardsInGraveyard and ...Counters.Poison.
func TestAmountImprintedValidAndOpponentGraveyardAndPoison(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p, opp := g.Players()[0], g.Players()[1]

	imp := g.NewCard(cdaDef(t, "Imprinted$Valid Creature", "Imprinted$Valid Land"), p, engine.Battlefield)
	creature := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Exile)
	land := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Exile)
	g.Card(imp).Memory.Imprint(creature)
	g.Card(imp).Memory.Imprint(land)
	checkSBA(g)
	wantCDAPT(t, g, imp, 1, 1, "one imprinted creature, one imprinted land")

	yard := g.NewCard(cdaDef(t, "PlayerCountOpponents$HighestCardsInGraveyard", "PlayerCountOpponents$HighestCounters.Poison"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Graveyard)
	g.NewCard(creatureDefPT(t, "1", "1"), opp, engine.Graveyard)
	g.Player(opp).Counters.Add(engine.Poison, 3)
	checkSBA(g)
	wantCDAPT(t, g, yard, 2, 3, "two cards in the opponent's graveyard, three poison counters")
}
