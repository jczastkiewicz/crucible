package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// This file covers card properties that read where a card sits in its
// owner's library or graveyard, its printed cost, its creature and land
// types against other cards, and the source's remembered cards
// (CardProperty.java: TopLibrary/BottomLibrary, TopGraveyard/BottomGraveyard,
// hasXCost, cmcEven, AdventureCard, sharesCreatureTypeWith,
// sharesLandTypeWith, greatestRememberedCMC, lowestCMC, NotedName).

func TestCardPropertiesReadPositionCostAndSharedTypes(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	forest := g.NewCard(corpusCard(t, "Forest"), p, engine.Library)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Library)
	island := g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	lib := g.Zone(engine.Library, p).Cards()
	top, bottom := lib[0], lib[len(lib)-1]

	elves := g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Graveyard)
	bolt := g.NewCard(corpusCard(t, "Lightning Bolt"), p, engine.Graveyard)
	fireball := g.NewCard(corpusCard(t, "Fireball"), p, engine.Graveyard)

	giant := g.NewCard(corpusCard(t, "Bonecrusher Giant"), other, engine.Battlefield)
	hill := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	ogre := g.NewCard(corpusCard(t, "Gray Ogre"), other, engine.Battlefield)
	g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	sliver := g.NewCard(corpusCard(t, "Ornithopter"), p, engine.Battlefield)
	g.Card(hill).Memory.Remember(engine.CardEntity(giant))
	g.Card(hill).Memory.Remember(engine.CardEntity(ogre))
	g.Card(hill).Memory.Remember(engine.CardEntity(elves))

	firstLand := forest
	if top == island {
		firstLand = island
	}
	cases := []struct {
		spec   string
		card   engine.CardID
		source engine.CardID
		want   bool
	}{
		{"Card.TopLibrary", top, engine.NoCard, true},
		{"Card.TopLibrary", bottom, engine.NoCard, false},
		{"Card.BottomLibrary", bottom, engine.NoCard, true},
		{"Card.TopLibrary_Land", firstLand, engine.NoCard, true},
		{"Card.TopLibrary_Creature", bears, engine.NoCard, true},
		{"Card.TopGraveyard", fireball, engine.NoCard, true},
		{"Card.TopGraveyard", bolt, engine.NoCard, false},
		{"Card.TopGraveyard2", bolt, engine.NoCard, true},
		{"Card.TopGraveyard2", elves, engine.NoCard, false},
		{"Card.TopGraveyardCreature", elves, engine.NoCard, true},
		{"Card.BottomGraveyard", elves, engine.NoCard, true},
		{"Card.hasXCost", fireball, engine.NoCard, true},
		{"Card.hasXCost", bolt, engine.NoCard, false},
		{"Card.cmcEven", bears, engine.NoCard, true},
		{"Card.cmcOdd", bolt, engine.NoCard, true},
		{"Card.AdventureCard", giant, engine.NoCard, true},
		{"Card.AdventureCard", hill, engine.NoCard, false},
		{"Creature.sharesCreatureTypeWith", giant, hill, true},
		{"Creature.sharesCreatureTypeWith", ogre, hill, false},
		{"Creature.sharesCreatureTypeWith Remembered", ogre, hill, true},
		{"Land.sharesLandTypeWith Valid Land.YouCtrl", forest, hill, true},
		{"Land.sharesLandTypeWith Valid Land.YouCtrl", island, hill, false},
		{"Card.greatestRememberedCMC", ogre, hill, true},
		{"Card.greatestRememberedCMC", elves, hill, false},
		{"Card.greatestRememberedCMC", sliver, hill, false},
		{"Card.lowestRememberedCMC", elves, hill, true},
		{"Card.lowestCMC", sliver, engine.NoCard, true},
		{"Card.lowestCMC", hill, engine.NoCard, false},
		{"Card.NotedNameAetherSearcher", hill, hill, false},
	}
	for _, tc := range cases {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, tc.source); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
		if err := g.TakePendingError(); err != nil {
			t.Errorf("%s: %v", tc.spec, err)
		}
	}
}

// blockedBySource reads the combat in progress, blockedByRemembered the
// blocks of this turn, attackedThisTurn the attacks declared this turn
// (CardProperty.java:1185-1188, 1612-1679).
func TestCardPropertiesReadThisTurnsAttacksAndBlocks(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	attacker := g.NewCard(corpusCard(t, "Hill Giant"), p, engine.Battlefield)
	idle := g.NewCard(corpusCard(t, "Gray Ogre"), p, engine.Battlefield)
	blocker := g.NewCard(corpusCard(t, "Grizzly Bears"), other, engine.Battlefield)
	watcher := g.NewCard(corpusCard(t, "Ornithopter"), other, engine.Battlefield)
	g.Card(watcher).Memory.Remember(engine.CardEntity(blocker))
	declareLoneAttacker(t, g, attacker)
	g.SetTurnState(2, p, engine.DeclareBlockers)
	if _, err := declareBlocks(g, engine.Block{Blocker: blocker, Attacker: attacker}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		spec   string
		card   engine.CardID
		source engine.CardID
		want   bool
	}{
		{"Creature.attackedThisTurn", attacker, engine.NoCard, true},
		{"Creature.attackedThisTurn", idle, engine.NoCard, false},
		{"Creature.blockedBySource", attacker, blocker, true},
		{"Creature.blockedBySource", attacker, watcher, false},
		{"Creature.blockedByRemembered", attacker, watcher, true},
		{"Creature.blockedRemembered", attacker, watcher, false},
		{"Creature.isBlockedByRemembered", attacker, watcher, true},
	}
	for _, tc := range cases {
		if got := engine.Matches(g, g.Card(tc.card), valid.Parse(tc.spec), p, tc.source); got != tc.want {
			t.Errorf("%s on %s = %v, want %v", tc.spec, g.Card(tc.card).Name(), got, tc.want)
		}
	}
}
