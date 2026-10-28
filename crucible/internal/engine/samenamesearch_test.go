package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// These tests cover ChangeZone's SameNameAsEach$ (ChangeZoneEffect.java, PR
// 12381): one library search that picks a card with the same name as each named
// card, replacing the RepeatEach + ChooseCard | QuasiLibrarySearch$ scripts of
// Doubling Chant, Dichotomancy and Clarion Ultimatum. Every library card and
// every battlefield creature made by creatureDefPT is named "Test Creature".

const sameNameSearch = "DB$ ChangeZone | Origin$ Library | Destination$ Hand | ChangeType$ Card | SameNameAsEach$ Valid Creature.Other+YouCtrl"

func TestSameNameAsEachPicksOneCardPerNamedCardInOneSearch(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
	lib := libraryCards(t, g, p, 3)
	rec := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueCardChoice(lib[1:2])
	rec.QueueCardChoice(lib[0:1])
	host := etbChainDef(t, "Test Shape", sameNameSearch)
	if _, err := castETBChain(t, g, p, host, rec.ScriptedController); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range lib[:2] {
		if z := g.Card(id).Zone; z != engine.Hand {
			t.Errorf("picked card %v zone = %v, want Hand", id, z)
		}
	}
	if z := g.Card(lib[2]).Zone; z != engine.Library {
		t.Errorf("unpicked card zone = %v, want Library", z)
	}
	if got := measure(t, g, p, "Count$SearchedLibrary.You"); got != 1 {
		t.Errorf("searches counted = %d, want 1: one search for both names", got)
	}
}

func TestSameNameAsEachDecline(t *testing.T) {
	t.Parallel()

	t.Run("decline", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		lib := libraryCards(t, g, p, 1)
		c := engine.NewScriptedController()
		c.QueueCardChoice(nil)
		resolveLine(t, g, p, c, sameNameSearch)
		if z := g.Card(lib[0]).Zone; z != engine.Library {
			t.Errorf("zone = %v, want Library: the search was declined", z)
		}
	})
}

func TestSameNameAsEachChosenCardsAndRefusal(t *testing.T) {
	t.Parallel()

	t.Run("ChosenCard", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		bear := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Battlefield)
		lib := libraryCards(t, g, p, 1)
		c := engine.NewScriptedController()
		c.QueueCardChoice([]engine.CardID{bear})
		c.QueueCardChoice(lib[:1])
		resolveLine(t, g, p, c, "DB$ ChooseCard | Choices$ Creature.Other | Mandatory$ True | SubAbility$ DBSearch",
			"DBSearch", strings.Replace(sameNameSearch, "Valid Creature.Other+YouCtrl", "ChosenCard", 1))
		if z := g.Card(lib[0]).Zone; z != engine.Hand {
			t.Errorf("zone = %v, want Hand", z)
		}
	})

	t.Run("with Defined$ is refused", func(t *testing.T) {
		t.Parallel()
		g, p, _ := newTwoPlayerGame(t)
		libraryCards(t, g, p, 1)
		_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, sameNameSearch+" | Defined$ Self")
		if err == nil || !strings.Contains(err.Error(), "SameNameAsEach$") {
			t.Errorf("err = %v, want one naming SameNameAsEach$", err)
		}
	})
}
