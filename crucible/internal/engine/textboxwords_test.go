package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// TestChangeTextEndsWhenTheCardChangesZone proves CR 400.7: a permanent word
// change is the old object's, so a card that leaves and returns is printed.
func TestChangeTextEndsWhenTheCardChangesZone(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
	changeText(t, g, p, engine.NewScriptedController(),
		"DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ White Red | Duration$ Permanent", knight)
	if !hasKeywordLine(g, knight, "Protection from red") {
		t.Fatalf("keywords = %v, want the change to apply", cardKeywords(g, knight))
	}
	g.Move(knight, engine.Graveyard, p)
	g.Move(knight, engine.Battlefield, p)
	sba(g)
	if !hasKeywordLine(g, knight, "Protection from white") {
		t.Errorf("keywords = %v, want the printed Protection from white", cardKeywords(g, knight))
	}
}

// TestChangeTextSurvivesGameClone proves the records copy with the game and
// the clone's rewritten definitions are its own: the original's card keeps its
// change when the clone's is ended.
func TestChangeTextSurvivesGameClone(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
	changeText(t, g, p, engine.NewScriptedController(),
		"DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ White Green | Duration$ Permanent", knight)

	clone := g.Clone()
	sba(clone)
	if !hasKeywordLine(clone, knight, "Protection from green") {
		t.Errorf("clone keywords = %v, want Protection from green", cardKeywords(clone, knight))
	}
	clone.Move(knight, engine.Graveyard, p)
	sba(clone)
	sba(g)
	if !hasKeywordLine(g, knight, "Protection from green") {
		t.Errorf("original keywords = %v, want its change untouched by the clone", cardKeywords(g, knight))
	}
}

// exchangeLine pushes line as an ability of host, aimed at cards.
func exchangeLine(t *testing.T, g *engine.Game, p engine.PlayerID, host engine.CardID, line string, cards ...engine.CardID) error {
	t.Helper()
	def := scriptDef(t, "Exchange Source", "Enchantment", "A:"+line)
	var targets []engine.EntityID
	for _, id := range cards {
		targets = append(targets, engine.CardEntity(id))
	}
	g.PushAbility(engine.Ability{
		API: mustAPI(t, "ExchangeTextBox"), Source: host, Controller: p,
		Params: def.Faces[0].Abilities[0], Amounts: def.Faces[0].Amounts, Targets: targets,
	})
	err := g.ResolveStack(engine.NewRegistry(), engine.NewScriptedController())
	sba(g)
	return err
}

// TestExchangeTextBoxSwapsAbilitiesAndKeywords proves Exchange of Words:
// Serra Angel and Grizzly Bears trade their text boxes -- keywords here -- and
// keep name, types and power/toughness; with AsLongAsInPlay the swap ends when
// the host leaves the battlefield.
func TestExchangeTextBoxSwapsAbilitiesAndKeywords(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	angel := g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	host := g.NewCard(scriptDef(t, "Exchanger", "Enchantment"), p, engine.Battlefield)
	if err := exchangeLine(t, g, p, host, "DB$ ExchangeTextBox | Defined$ Targeted | Duration$ AsLongAsInPlay", angel, bears); err != nil {
		t.Fatal(err)
	}

	if g.Card(angel).HasKeyword("Flying") || !g.Card(bears).HasKeyword("Flying") || !g.Card(bears).HasKeyword("Vigilance") {
		t.Errorf("angel keywords %v, bears keywords %v: want the boxes swapped", cardKeywords(g, angel), cardKeywords(g, bears))
	}
	if g.Card(angel).Name() != "Serra Angel" || !g.Card(angel).Type().HasSubtype("Angel") {
		t.Error("the exchange changed more than the text box")
	}
	wantPT(t, g, angel, 4, 4)
	wantPT(t, g, bears, 2, 2)

	g.Move(host, engine.Graveyard, p)
	sba(g)
	if !g.Card(angel).HasKeyword("Flying") || g.Card(bears).HasKeyword("Flying") {
		t.Errorf("angel keywords %v, bears keywords %v: want the swap ended with its host", cardKeywords(g, angel), cardKeywords(g, bears))
	}
}

// TestExchangeTextBoxEdges proves the cases around Deadpool's shape: one card
// exchanges nothing; an unknown duration is an error rather than a guess; a
// host already gone applies nothing for AsLongAsInPlay; no Duration$ never
// ends, even when an unrelated host leaves.
func TestExchangeTextBoxEdges(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	angel := g.NewCard(corpusCard(t, "Serra Angel"), p, engine.Battlefield)
	bears := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	host := g.NewCard(scriptDef(t, "Exchanger", "Enchantment"), p, engine.Battlefield)

	if err := exchangeLine(t, g, p, host, "DB$ ExchangeTextBox | Defined$ Targeted", angel); err != nil {
		t.Fatal(err)
	}
	if !g.Card(angel).HasKeyword("Flying") {
		t.Error("one card exchanged with nothing")
	}
	err := exchangeLine(t, g, p, host, "DB$ ExchangeTextBox | Defined$ Targeted | Duration$ UntilYourNextTurn", angel, bears)
	if err == nil || !strings.Contains(err.Error(), "not resolvable") {
		t.Errorf("unknown Duration$: error = %v", err)
	}
	gone := g.NewCard(scriptDef(t, "Gone", "Enchantment"), p, engine.Graveyard)
	if err := exchangeLine(t, g, p, gone, "DB$ ExchangeTextBox | Defined$ Targeted | Duration$ AsLongAsInPlay", angel, bears); err != nil {
		t.Fatal(err)
	}
	if !g.Card(angel).HasKeyword("Flying") {
		t.Error("a host already gone still exchanged the boxes")
	}
	if err := exchangeLine(t, g, p, host, "DB$ ExchangeTextBox | Defined$ Targeted", angel, bears); err != nil {
		t.Fatal(err)
	}
	g.Move(host, engine.Graveyard, p)
	sba(g)
	if g.Card(angel).HasKeyword("Flying") {
		t.Error("a permanent exchange ended with an unrelated host")
	}
}

// TestChangeColorWordsToStatic proves Layer 3's static form with the real
// Swirl the Mists: its "ChosenColor" reads the host's chosen color, does
// nothing while none is chosen, and ends when the host leaves.
func TestChangeColorWordsToStatic(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
	swirl := g.NewCard(corpusCard(t, "Swirl the Mists"), p, engine.Battlefield)
	sba(g)
	if !hasKeywordLine(g, knight, "Protection from white") {
		t.Fatalf("keywords = %v, want printed text while no color is chosen", cardKeywords(g, knight))
	}
	g.Card(swirl).Memory.SetChosenColors(mana.Green)
	sba(g)
	if !hasKeywordLine(g, knight, "Protection from green") {
		t.Errorf("keywords = %v, want Protection from green under Swirl the Mists", cardKeywords(g, knight))
	}
	g.Move(swirl, engine.Graveyard, p)
	sba(g)
	if !hasKeywordLine(g, knight, "Protection from white") {
		t.Errorf("keywords = %v, want the text back when Swirl leaves", cardKeywords(g, knight))
	}
}

// TestChangeColorWordsToLiteral proves a literal color and ChangeColorWordsFrom$
// narrow the static to one word, and a word that is no color does nothing.
func TestChangeColorWordsToLiteral(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
	g.NewCard(scriptDef(t, "Tinter", "Enchantment",
		"S:Mode$ Continuous | Affected$ Permanent | ChangeColorWordsTo$ Blue | ChangeColorWordsFrom$ White"), p, engine.Battlefield)
	sba(g)
	if !hasKeywordLine(g, knight, "Protection from blue") {
		t.Errorf("keywords = %v, want Protection from blue", cardKeywords(g, knight))
	}
	for _, bad := range []string{"ChangeColorWordsTo$ Purple", "ChangeColorWordsTo$ Blue | ChangeColorWordsFrom$ Purple"} {
		g2, p2 := textGame(t)
		k2 := g2.NewCard(corpusCard(t, "Black Knight"), p2, engine.Battlefield)
		g2.NewCard(scriptDef(t, "Tinter", "Enchantment", "S:Mode$ Continuous | Affected$ Permanent | "+bad), p2, engine.Battlefield)
		sba(g2)
		if !hasKeywordLine(g2, k2, "Protection from white") {
			t.Errorf("%s: keywords = %v, want the printed text", bad, cardKeywords(g2, k2))
		}
	}
}
