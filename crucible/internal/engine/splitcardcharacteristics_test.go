package engine_test

import (
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Off the stack a Split card has both halves' name, mana cost, color and type
// (CR 709.4; Java's Original state, CardFactory.java:330-339): Fire // Ice in
// a hand is a blue and red instant of mana value 4.
func TestSplitCardOffTheStackCombinesBothHalves(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	fire := g.Card(g.NewCard(corpusCard(t, "Fire"), p, engine.Hand))
	if got := fire.Name(); got != "Fire // Ice" {
		t.Errorf("name %q, want Fire // Ice", got)
	}
	if got := fire.CMC(); got != 4 {
		t.Errorf("mana value %d, want 4", got)
	}
	if got := fire.Colors(); got != mana.Red|mana.Blue {
		t.Errorf("colors %v, want blue and red", got)
	}
	commit := g.Card(g.NewCard(corpusCard(t, "Commit"), p, engine.Graveyard))
	if typ := commit.Type(); !typ.Has(cardtype.Instant) || !typ.Has(cardtype.Sorcery) {
		t.Errorf("Commit // Memory's type %v, want Instant and Sorcery", typ)
	}
}

// A Room card in a hand is a Split card too: Bottomless Pool // Locker Room
// has mana value 6 ({U} and {4}{U}).
func TestRoomCardInHandHasBothDoorsManaValue(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	room := g.Card(g.NewCard(corpusCard(t, "Bottomless Pool"), p, engine.Hand))
	if got := room.CMC(); got != 6 {
		t.Errorf("mana value %d, want 6", got)
	}
	if got := room.Name(); got != "Bottomless Pool // Locker Room" {
		t.Errorf("name %q, want Bottomless Pool // Locker Room", got)
	}
}

// Tarmogoyf counts both of Commit // Memory's types in a graveyard: 2/3.
func TestTarmogoyfCountsBothHalvesTypes(t *testing.T) {
	t.Parallel()
	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	goyf := g.NewCard(corpusCard(t, "Tarmogoyf"), p, engine.Battlefield)
	g.NewCard(corpusCard(t, "Commit"), opp, engine.Graveyard)
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
	wantCDAPT(t, g, goyf, 2, 3, "Commit // Memory: an instant and a sorcery")
}

// Dark Confidant reveals Fire // Ice and its controller loses 4 life, the
// combined mana value.
func TestDarkConfidantLosesBothHalvesManaValue(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
	libraryCards(t, g, p, 8)
	libraryCards(t, g, other, 8)
	g.NewCard(corpusCard(t, "Dark Confidant"), p, engine.Battlefield)
	fire := g.NewCard(corpusCard(t, "Fire"), p, engine.Library)
	g.MoveToLibraryTop(fire, p)
	g.SetTurnState(2, p, engine.Untap)
	life := g.Player(p).Life
	toStep(t, g, engine.NewScriptedController(), p, engine.Upkeep, nil)
	if got := g.Card(fire).Zone; got != engine.Hand {
		t.Fatalf("Fire // Ice in %v, want the hand", got)
	}
	if got := life - g.Player(p).Life; got != 4 {
		t.Errorf("lost %d life, want 4", got)
	}
}

// The left half is cast with its own characteristics, not the combined ones:
// Incubation, a sorcery, is not cast on an opponent's turn though Incubation
// // Incongruity in hand is also an instant; Incongruity is. On the stack Fire
// has Fire's mana value, and back in the graveyard the card has both halves'.
func TestSplitHalfIsCastWithItsOwnCharacteristics(t *testing.T) {
	t.Parallel()
	g, p, opp := newTwoPlayerGameOn(t, scenarioDB(t))
	g.SetTurnState(1, opp, engine.Main1)
	inc := g.NewCard(corpusCard(t, "Incubation"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 1)
	c := engine.NewScriptedController()
	if g.CastSpell(p, inc, c) {
		t.Fatal("Incubation, a sorcery, was cast on the opponent's turn")
	}
	_ = g.TakePendingError()
	if got := g.Card(inc).CMC(); got != 4 {
		t.Errorf("after the refused cast Incubation // Incongruity has mana value %d, want 4", got)
	}

	g.SetTurnState(1, p, engine.Main1)
	fire := g.NewCard(corpusCard(t, "Fire"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 2)
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(opp)})
	c.QueuePayGeneric(mana.ShardR)
	if !g.CastSpell(p, fire, c) {
		t.Fatalf("Fire not cast: %v", g.TakePendingError())
	}
	if got, name := g.Card(fire).CMC(), g.Card(fire).Name(); got != 2 || name != "Fire" {
		t.Errorf("on the stack %q has mana value %d, want Fire with 2", name, got)
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if got := g.Card(fire).Zone; got != engine.Graveyard {
		t.Fatalf("Fire // Ice in %v, want the graveyard", got)
	}
	if got := g.Card(fire).CMC(); got != 4 {
		t.Errorf("in the graveyard Fire // Ice has mana value %d, want 4", got)
	}
}

// nameAtTargets records the name card has when its targets are chosen.
type nameAtTargets struct {
	*optionRecorder
	card  engine.CardID
	names []string
}

func (r *nameAtTargets) ChooseTargets(g *engine.Game, p engine.PlayerID, eligible []engine.EntityID, lo, hi int) []engine.EntityID {
	r.names = append(r.names, g.Card(r.card).Name())
	return r.optionRecorder.ChooseTargets(g, p, eligible, lo, hi)
}

// Play judges and casts a Split card's plain option as its left half: under
// ValidSA$ Spell.cmcLE2, Wear ({1}{R}) and Tear ({W}) qualify though Wear //
// Tear has mana value 3, the fused spell does not, and Wear is cast as Wear.
func TestPlayCastsASplitCardsLeftHalfAsThatHalf(t *testing.T) {
	t.Parallel()
	g, p, card, artifact, enchantment := wearTearSetup(t, engine.Hand)
	c := &nameAtTargets{optionRecorder: &optionRecorder{ScriptedController: engine.NewScriptedController()}, card: card}
	c.QueueConfirmEffect(true)
	c.QueueCardChoice([]engine.CardID{card})
	c.QueueOptionNamed("Cast Wear")
	c.QueueTargets([]engine.EntityID{engine.CardEntity(artifact)})
	pushPlayLine(t, g, p, nil, "DB$ Play | Valid$ Card.nonLand+YouOwn | ValidZone$ Hand | ValidSA$ Spell.cmcLE2 | WithoutManaCost$ True | Optional$ True")
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if want := []string{"Cast Wear", "Cast Tear"}; len(c.offers) != 1 || !slices.Equal(c.offers[0], want) {
		t.Fatalf("offered %v, want [%v]", c.offers, want)
	}
	if !slices.Equal(c.names, []string{"Wear"}) {
		t.Errorf("targets chosen for %v, want [Wear]", c.names)
	}
	if g.Card(artifact).Zone != engine.Graveyard || g.Card(enchantment).Zone != engine.Battlefield {
		t.Errorf("Ornithopter in %v, Glorious Anthem in %v, want only the artifact destroyed", g.Card(artifact).Zone, g.Card(enchantment).Zone)
	}
}
