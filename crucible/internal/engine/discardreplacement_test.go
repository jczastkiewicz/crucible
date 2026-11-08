package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// The discard replacement walk (ReplaceMoved's Discard$ True, Player.discard)
// refuses what it cannot order or check rather than guessing (GO-7).

// A discard paid as a spell's cost is not an effect (PlaySpellAbility.java:683
// builds the cost's decision maker with ability.isTrigger()), so Loxodon
// Smiter's EffectOnly$/ValidCause$ line does not apply and the Smiter goes to
// the graveyard.
func TestDiscardPaidAsASpellCostIsNotAnEffect(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	voice := g.NewCard(corpusCard(t, "Tormenting Voice"), p, engine.Hand)
	smiter := g.NewCard(corpusCard(t, "Loxodon Smiter"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	g.NewCard(corpusCard(t, "Island"), p, engine.Library)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c := engine.NewScriptedController()
	c.QueuePayGeneric(mana.ShardR)
	c.QueueDiscardChoice([]engine.CardID{smiter})
	if !g.CastSpell(p, voice, c) {
		t.Fatal("could not cast Tormenting Voice")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(smiter).Zone; got != engine.Graveyard {
		t.Errorf("Loxodon Smiter is in %v, want the graveyard", got)
	}
}

// Madness and Rest in Peace both apply to the discard: they are two candidates
// of one CR 616.1 choice for the discarding player (ReplacementHandler.java:
// 256-279). Choosing Rest in Peace exiles the card with no madness trigger;
// choosing Madness exiles it and the madness trigger asks to cast it.
func TestMadnessBesideAGraveyardReplacementIsOneChoice(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (*engine.Game, engine.PlayerID, engine.CardID) {
		g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
		g.NewCard(corpusCard(t, "Rest in Peace"), p, engine.Battlefield)
		temper := g.NewCard(corpusCard(t, "Fiery Temper"), p, engine.Hand)
		return g, p, temper
	}

	t.Run("rest in peace", func(t *testing.T) {
		t.Parallel()
		g, p, temper := setup(t)
		c := engine.NewScriptedController()
		c.QueueReplacementEffect(1)
		if _, err := resolveNow(t, g, p, c, nil, "DB$ Discard | Defined$ You | Mode$ Hand"); err != nil {
			t.Fatal(err)
		}
		if got := g.Card(temper).Zone; got != engine.Exile {
			t.Errorf("Fiery Temper is in %v, want exile", got)
		}
	})

	t.Run("madness", func(t *testing.T) {
		t.Parallel()
		g, p, temper := setup(t)
		c := engine.NewScriptedController()
		c.QueueReplacementEffect(0)
		c.QueueConfirmEffect(false)
		if _, err := resolveNow(t, g, p, c, nil, "DB$ Discard | Defined$ You | Mode$ Hand"); err != nil {
			t.Fatal(err)
		}
		if got := g.Card(temper).Zone; got != engine.Exile {
			t.Errorf("Fiery Temper is in %v, want exile (Madness, then Rest in Peace)", got)
		}
	})
}

// A ValidCause$ the walk does not read is refused, and so is a line whose
// ReplaceWith$ chain does not move the replaced card.
func TestDiscardReplacementShapesRefused(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"chain moving nothing", []string{
			"R:Event$ Moved | ActiveZones$ Hand | ValidCard$ Card.Self | Discard$ True | ReplaceWith$ DBGain",
			"SVar:DBGain:DB$ GainLife | Defined$ You | LifeAmount$ 1",
		}, "Discard$ shape"},
		{"unread ValidCause", []string{
			"R:Event$ Moved | ActiveZones$ Hand | ValidCard$ Card.Self | ValidCause$ SpellAbility.YouCtrl | Discard$ True | ReplaceWith$ DBExile",
			"SVar:DBExile:DB$ ChangeZone | Defined$ ReplacedCard | Origin$ Hand | Destination$ Exile",
		}, "ValidCause$"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.NewCard(scriptDef(t, "Test Discardee", "Sorcery", tc.lines...), p, engine.Hand)
			_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Discard | Defined$ You | Mode$ Hand")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// Loxodon Smiter: discarded by an opponent's effect, its own hand line's
// chain puts it onto the battlefield instead (ReplaceMoved's overriding
// ability); discarded by its owner's own effect, it goes to the graveyard.
func TestLoxodonSmiterDiscardedByAnOpponentEntersTheBattlefield(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	smiter := g.NewCard(corpusCard(t, "Loxodon Smiter"), p, engine.Hand)
	if _, err := resolveNow(t, g, other, engine.NewScriptedController(), nil, "DB$ Discard | Defined$ Opponent | Mode$ Hand"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(smiter).Zone; got != engine.Battlefield {
		t.Errorf("Loxodon Smiter is in %v, want the battlefield", got)
	}

	g2, p2, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	own := g2.NewCard(corpusCard(t, "Loxodon Smiter"), p2, engine.Hand)
	if _, err := resolveNow(t, g2, p2, engine.NewScriptedController(), nil, "DB$ Discard | Defined$ You | Mode$ Hand"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g2.Card(own).Zone; got != engine.Graveyard {
		t.Errorf("self-discarded Loxodon Smiter is in %v, want the graveyard", got)
	}
}

// Library of Leng's Optional$ line asks the discarding player: yes puts the
// card on top of the library, no lets it go to the graveyard.
func TestLibraryOfLengAsksAndPutsTheCardOnTop(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		confirm bool
		want    engine.ZoneType
	}{
		{"accept", true, engine.Library},
		{"decline", false, engine.Graveyard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.NewCard(corpusCard(t, "Library of Leng"), p, engine.Battlefield)
			g.NewCard(corpusCard(t, "Island"), p, engine.Library)
			bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Hand)
			c := engine.NewScriptedController()
			c.QueueConfirmReplacementEffect(tc.confirm)
			if _, err := resolveNow(t, g, p, c, nil, "DB$ Discard | Defined$ You | Mode$ Hand"); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Card(bears).Zone; got != tc.want {
				t.Fatalf("Grizzly Bears is in %v, want %v", got, tc.want)
			}
			if tc.confirm {
				if top := g.Zone(engine.Library, p).Cards()[0]; top != bears {
					t.Errorf("top of the library is %d, want Grizzly Bears", top)
				}
			}
		})
	}
}

// Two Madness keywords: two discard lines on one card; the walk applies one
// (CR 616.1: the same description, so no question) and the card is exiled.
func TestTwoMadnessLinesExileOnce(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	card := g.NewCard(scriptDef(t, "Test Discardee", "Sorcery", "K:Madness:R", "K:Madness:B"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	if _, err := resolveNow(t, g, p, c, nil, "DB$ Discard | Defined$ You | Mode$ Hand"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	// The one madness trigger declined to cast: the card goes on to the
	// graveyard.
	if got := g.Card(card).Zone; got != engine.Graveyard {
		t.Errorf("the card is in %v, want the graveyard after one declined madness trigger", got)
	}
}

// Wilt-Leaf Liege's chain no longer names DefinedPlayer$ ReplacedPlayer (PR
// 12471 dropped it, the Moved replacement never records a player): an
// opponent's discard puts it onto the battlefield instead (CR 614.1).
func TestWiltLeafLiegeEntersWhenAnOpponentMakesYouDiscardIt(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	liege := g.NewCard(corpusCard(t, "Wilt-Leaf Liege"), p, engine.Hand)
	if _, err := resolveNow(t, g, other, engine.NewScriptedController(), nil, "DB$ Discard | Defined$ Opponent | Mode$ Hand"); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Card(liege).Zone; got != engine.Battlefield {
		t.Errorf("Wilt-Leaf Liege is in %v, want the battlefield", got)
	}
}

// Play's PlayCost$ ManaCost is refused before a card is
// chosen.
func TestPlayCostPastManaIsRefused(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"ManaCost"} {
		g, p, _ := newTwoPlayerGame(t)
		_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Play | Defined$ Self | PlayCost$ "+raw)
		if err == nil || !strings.Contains(err.Error(), "PlayCost$") {
			t.Errorf("PlayCost$ %s: err = %v, want refused", raw, err)
		}
	}
}

// CR 616.1 for a move to the graveyard (ReplacementHandler.run): Rest in
// Peace and a line returning the card to its owner's hand both apply to the
// dying Bears; its controller chooses which, and the chosen one replaces the
// move.
func TestTwoGraveyardReplacementsAreOneChoiceForTheDyingCardsController(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		pick int
		want engine.ZoneType
	}{
		{"rest in peace", 0, engine.Exile},
		{"return to hand", 1, engine.Hand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
			g.NewCard(corpusCard(t, "Rest in Peace"), p, engine.Battlefield)
			g.NewCard(scriptDef(t, "Test Recall Field", "Enchantment",
				"R:Event$ Moved | ActiveZones$ Battlefield | Destination$ Graveyard | ValidCard$ Creature | ReplaceWith$ DBHand | Description$ Return it to its owner's hand instead.",
				"SVar:DBHand:DB$ ChangeZone | Hidden$ True | Origin$ All | Destination$ Hand | Defined$ ReplacedCard"), p, engine.Battlefield)
			bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			c.QueueReplacementEffect(tc.pick)
			if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(bears)}, "DB$ Destroy | Defined$ Targeted"); err != nil {
				t.Fatal(err)
			}
			if got := g.Card(bears).Zone; got != tc.want {
				t.Errorf("Grizzly Bears is in %v, want %v", got, tc.want)
			}
		})
	}
}
