package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Mode$ Clashed | ValidPlayer$ You | Won$ True (Sylvan Echoes): the clasher
// who revealed the higher mana value may draw; a lost clash fires nothing
// (TriggerClashed.java:49-62, ClashEffect.java:55-60).
func TestClashedFiresOnlyForTheWinningClasher(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		mine, theirs string
		wantHand     int
	}{
		{"won", "Colossal Dreadmaw", "Llanowar Elves", 1},
		{"lost", "Llanowar Elves", "Colossal Dreadmaw", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, "Sylvan Echoes"), p, engine.Battlefield)
			g.NewCard(corpusCard(t, tc.mine), p, engine.Library)
			g.NewCard(corpusCard(t, tc.theirs), other, engine.Library)
			c := engine.NewScriptedController()
			c.QueueCardOnTop(true)
			c.QueueCardOnTop(true)
			if tc.wantHand > 0 {
				c.QueueConfirmOptionalTrigger(true)
			}

			if err := resolveWith(t, g, p, c, "DB$ Clash"); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := len(g.Zone(engine.Hand, p).Cards()); got != tc.wantHand {
				t.Errorf("hand has %d cards, want %d", got, tc.wantHand)
			}
		})
	}
}

// Mode$ Proliferate fires once per Amount$ pass, with nothing to proliferate
// too (CountersProliferateEffect.java:56-75).
func TestProliferateFiresOncePerPassEvenWithNothingChosen(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(scriptDef(t, "Proliferate Watcher", "Enchantment",
		"T:Mode$ Proliferate | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain",
		"SVar:TrigGain:DB$ GainLife | LifeAmount$ 1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueEntityChoice(nil)
	c.QueueEntityChoice(nil)

	if err := resolveWith(t, g, p, c, "DB$ Proliferate | Amount$ 2"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Player(p).Life; got != 22 {
		t.Errorf("life %d, want 22 (two proliferate passes)", got)
	}
}

// Mode$ PayLife (Font of Agonies) counts life paid as a cost, the amount the
// cost named; life lost to an effect is no payment (Player.java:600, whose
// one caller is CostPayLife).
func TestPayLifeFiresForALifeCostNotForLifeLoss(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	font := g.NewCard(corpusCard(t, "Font of Agonies"), p, engine.Battlefield)
	blood := engine.CounterType("BLOOD")
	c := engine.NewScriptedController()

	if err := resolveWith(t, g, p, c, "DB$ LoseLife | Defined$ You | LifeAmount$ 2"); err != nil {
		t.Fatalf("resolve LoseLife: %v", err)
	}
	if got := g.Card(font).Counters.Count(blood); got != 0 {
		t.Fatalf("%d blood counters after losing life, want 0", got)
	}

	c.QueueConfirmPayCost(true)
	if err := resolveWith(t, g, p, c, "DB$ GainLife | LifeAmount$ 1 | UnlessCost$ PayLife<3> | UnlessPayer$ You | UnlessSwitched$ True"); err != nil {
		t.Fatalf("resolve the unless cost: %v", err)
	}
	if got := g.Card(font).Counters.Count(blood); got != 3 {
		t.Errorf("%d blood counters after paying 3 life, want 3", got)
	}
}

// Mode$ Foretell (Dream Devourer) fires once the card lies foretold in exile
// (Player.addForetoldThisTurn, Player.java:1494-1499).
func TestForetellFiresForTheForetellingPlayer(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	devourer := g.NewCard(corpusCard(t, "Dream Devourer"), p, engine.Battlefield)
	bolt := g.NewCard(corpusCard(t, "Demon Bolt"), p, engine.Hand)
	c := engine.NewScriptedController()
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c.QueuePayGeneric(mana.ShardR)
	c.QueuePayGeneric(mana.ShardR)

	if !g.Foretell(p, bolt, c) {
		t.Fatal("could not foretell Demon Bolt")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	sba(g)
	if got, _ := g.Card(devourer).Power(); got != 2 {
		t.Errorf("Dream Devourer has power %d, want 2", got)
	}
}

// Mode$ SearchedLibrary | ValidPlayer$ Player.Opponent | SearchOwnLibrary$
// True (Archivist of Oghma): an opponent searching their own library
// triggers it, found anything or not; its controller searching does not
// (TriggerSearchedLibrary.java:57-67, ChangeZoneEffect.java:1078-1081).
func TestSearchedLibraryFiresForAnOpponentsOwnSearch(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Archivist of Oghma"), p, engine.Battlefield)
	libraryCards(t, g, p, 2)
	libraryCards(t, g, other, 2)
	c := engine.NewScriptedController()
	search := "DB$ ChangeZone | Origin$ Library | Destination$ Hand | ChangeType$ Card | ChangeNum$ 1"

	c.QueueCardChoice(nil)
	if err := resolveWith(t, g, p, c, search); err != nil {
		t.Fatalf("own search: %v", err)
	}
	if got := g.Player(p).Life; got != 20 {
		t.Fatalf("life %d after searching own library, want 20", got)
	}

	c.QueueCardChoice(nil)
	if err := resolveWith(t, g, other, c, search); err != nil {
		t.Fatalf("opponent's search: %v", err)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life %d after the opponent searched, want 21", got)
	}
	if got := len(g.Zone(engine.Hand, p).Cards()); got != 1 {
		t.Errorf("hand has %d cards, want 1", got)
	}
}

// Mode$ ManifestDread (Paranormal Analyst) hands on the card put into the
// graveyard as TriggeredCards (ManifestDreadEffect.java:36-39).
func TestManifestDreadHandsOnTheGraveyardedCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Paranormal Analyst"), p, engine.Battlefield)
	lib := libraryCards(t, g, p, 2)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{lib[0]})
	c.QueueCardChoice([]engine.CardID{lib[1]})

	if err := resolveWith(t, g, p, c, "DB$ ManifestDread"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got := g.Card(lib[0]).Zone; got != engine.Battlefield {
		t.Errorf("manifested card in %v, want Battlefield", got)
	}
	if got := g.Card(lib[1]).Zone; got != engine.Hand {
		t.Errorf("graveyarded card in %v, want Hand (returned by the trigger)", got)
	}
}

// Mode$ Vote (Grudge Keeper): each opponent who voted for an option its
// controller did not loses 2 life; one who voted the same way does not
// (TriggerVote.java:66-121).
func TestVoteHandsOnTheOpponentsWhoVotedDifferently(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		theirVote int
		wantLife  int
	}{{"different", 1, 18}, {"same", 0, 20}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			g.SetTurnState(1, p, engine.Main1)
			g.NewCard(corpusCard(t, "Grudge Keeper"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			c.QueueAbilityChoice([]int{0})
			c.QueueAbilityChoice([]int{tc.theirVote})

			if err := resolveWith(t, g, p, c, "DB$ Vote | Defined$ Player | Choices$ DBA,DBB",
				"DBA", "DB$ GainLife | Defined$ You | LifeAmount$ 0",
				"DBB", "DB$ GainLife | Defined$ You | LifeAmount$ 0"); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := g.Player(other).Life; got != tc.wantLife {
				t.Errorf("opponent life %d, want %d", got, tc.wantLife)
			}
		})
	}
}

// Mode$ BecomesTargetOnce (Leyline of Combustion): a spell an opponent
// controls targeting you fires it once, its 2 damage going to that spell's
// controller (TriggerBecomesTargetOnce.java:56-80, MagicStack.java:474-486);
// your own spell does not.
func TestBecomesTargetOnceFiresForAnOpponentsSpell(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                 string
		casterIsOpponent     bool
		wantMine, wantTheirs int
	}{{"opponent", true, 18, 18}, {"own", false, 18, 20}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			caster := p
			if tc.casterIsOpponent {
				caster = other
			}
			g.SetTurnState(1, caster, engine.Main1)
			g.NewCard(corpusCard(t, "Leyline of Combustion"), p, engine.Battlefield)
			bolt := g.NewCard(corpusCard(t, "Shock"), caster, engine.Hand)
			g.Player(caster).ManaPool.Add(mana.Red, 1)
			c := engine.NewScriptedController()
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p)})
			if !g.CastSpell(caster, bolt, c) {
				t.Fatalf("cast failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got, want := [2]int{g.Player(p).Life, g.Player(other).Life}, [2]int{tc.wantMine, tc.wantTheirs}; got != want {
				t.Errorf("life %v, want %v", got, want)
			}
		})
	}
}

// Mode$ TapAll | ValidCards$ Merfolk.!token+YouCtrl (Deeproot Pilgrimage):
// one effect tapping two Merfolk fires once, not once per Merfolk
// (TriggerTapAll.java, TapAllEffect.java:57-62).
func TestTapAllFiresOncePerBatch(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, p, engine.Main1)
	g.NewCard(corpusCard(t, "Deeproot Pilgrimage"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Coral Merfolk"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Coral Merfolk"), p, engine.Battlefield)
	c := engine.NewScriptedController()

	if err := resolveWith(t, g, p, c, "DB$ TapAll | ValidCards$ Merfolk.YouCtrl"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	tokens := 0
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if g.Card(id).IsToken {
			tokens++
		}
	}
	if tokens != 1 {
		t.Errorf("%d tokens, want 1", tokens)
	}
}
