package replay

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// Questions the recorded games do not reach often enough to pin through
// Run (TEST-2: a Go question needs a board no fixture holds): a scry, a
// sacrifice, a multi-defender attack, a replacement choice. Each is answered
// from a call record, and refused with an unanswered question when the
// record is missing or names something Go did not offer.

// rareSetup is a game with three cards seated for seat 0, Java ids 101-103,
// and a controller over calls.
func rareSetup(t *testing.T, calls ...*call) (*Controller, *engine.Game, []engine.CardID) {
	t.Helper()
	g := engine.NewGame(nil, javarand.New(1), []string{"a", "b"})
	ids := newIDMap(g, &Log{})
	var cards []engine.CardID
	for i := range 3 {
		id := g.NewCard(&compile.Card{Name: "Card"}, g.Players()[0], engine.Battlefield)
		ids.pair(101+i, id)
		cards = append(cards, id)
	}
	return newController(g, &script{calls: calls}, ids), g, cards
}

func answer(method, raw string) *call {
	return &call{Seq: 1, Method: method, Decider: SeatRef(0), Answer: json.RawMessage(raw)}
}

func TestControllerAnswersRareQuestionsFromTheLog(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		call *call
		// silent is a question Java may skip (one order is the only order),
		// so a missing record is no unanswered question.
		silent bool
		ask    func(c *Controller, g *engine.Game, cards []engine.CardID) bool
	}{
		{"scry", answer("arrangeForScry", `[[{"card":102}],[{"card":101}]]`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			top, bottom := c.ArrangeForScry(g, g.Players()[0], cards[:2])
			return slices.Equal(top, cards[1:2]) && slices.Equal(bottom, cards[:1])
		}},
		{"sacrifice", answer("choosePermanentsToSacrifice", `[{"card":103}]`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			return slices.Equal(c.ChoosePermanentsToSacrifice(g, g.Players()[0], cards, 1), cards[2:])
		}},
		{"discard", answer("chooseCardsToDiscardFrom", `[{"card":101}]`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			return slices.Equal(c.ChooseCardsToDiscard(g, g.Players()[0], cards, 1), cards[:1])
		}},
		{"hand size", answer("chooseCardsToDiscardToMaximumHandSize", `[{"card":102}]`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			return slices.Equal(c.DiscardToHandSize(g, g.Players()[0], cards, 1), cards[1:2])
		}},
		{"entities", answer("chooseEntitiesForEffect", `[{"card":101},{"seat":1}]`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			options := []engine.EntityID{engine.CardEntity(cards[0]), engine.PlayerEntity(g.Players()[1])}
			return slices.Equal(c.ChooseEntitiesForEffect(g, g.Players()[0], engine.NoCard, options, 0, 2), options)
		}},
		{"one entity", answer("chooseSingleEntityForEffect", `{"card":101}`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			options := []engine.EntityID{engine.CardEntity(cards[0])}
			return slices.Equal(c.ChooseEntitiesForEffect(g, g.Players()[0], engine.NoCard, options, 0, 1), options)
		}},
		{"number", answer("chooseNumber", `2`), false, func(c *Controller, g *engine.Game, _ []engine.CardID) bool {
			return c.ChooseNumber(g, g.Players()[0], engine.NoCard, 0, 3) == 2
		}},
		{"replacement", answer("chooseSingleReplacementEffect", `{"host":{"card":102}}`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			return c.ChooseReplacementEffect(g, g.Players()[0], []engine.ReplacementOption{{Host: cards[0]}, {Host: cards[1]}}) == 1
		}},
		{"order", answer("orderMoveToZoneList", `[{"card":102},{"card":101}]`), true, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			return slices.Equal(c.OrderCardsForZone(g, g.Players()[0], cards[:2], engine.Library), []engine.CardID{cards[1], cards[0]})
		}},
		{"mana color", answer("chooseColor", `8`), false, func(c *Controller, g *engine.Game, _ []engine.CardID) bool {
			return c.ChooseManaColor(g, g.Players()[0], engine.NoCard, 31) == 8
		}},
		{"attack target", nil, false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			if c.attacks == nil {
				c.attacks = map[engine.CardID]Ref{cards[0]: SeatRef(1)}
			}
			defender := engine.PlayerEntity(g.Players()[1])
			return c.ChooseAttackTarget(g, g.Players()[0], cards[0], []engine.EntityID{defender}) == defender
		}},
	} {
		var calls []*call
		if tc.call != nil {
			calls = append(calls, tc.call)
		}
		c, g, cards := rareSetup(t, calls...)
		if !tc.ask(c, g, cards) {
			t.Errorf("%s: not Java's answer", tc.name)
		}
		if c.Unanswered() != nil {
			t.Errorf("%s: unanswered %s", tc.name, c.Unanswered())
		}

		// The same question with no record, then with a record naming a
		// card Go did not offer: both are the replay's unanswered question.
		for _, calls := range [][]*call{nil, {answer(methodOf(tc.call), `[{"card":999}]`)}} {
			c, g, cards := rareSetup(t, calls...)
			if tc.call == nil {
				c.attacks = map[engine.CardID]Ref{cards[0]: CardRef(999, "")}
			}
			tc.ask(c, g, cards)
			if c.Unanswered() == nil && (!tc.silent || calls != nil) {
				t.Errorf("%s with %d calls: no unanswered question", tc.name, len(calls))
			}
		}
	}
}

func methodOf(k *call) string {
	if k == nil {
		return "none"
	}
	return k.Method
}
