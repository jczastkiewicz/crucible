package replay

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
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
		{"card name", answer("chooseCardName", `"Card"`), true, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			return c.ChooseOption(g, g.Players()[0], cards[0], []string{"Other", "Card"}) == 1
		}},
		{"attacking token", answer("chooseSingleEntityForEffect", `{"seat":1}`), false, func(c *Controller, g *engine.Game, cards []engine.CardID) bool {
			defender := engine.PlayerEntity(g.Players()[1])
			return c.ChooseAttackTarget(g, g.Players()[0], cards[0], []engine.EntityID{defender}) == defender
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

// Java asks orderSimultaneousSa about a lone trigger too, Go only about two
// or more; and it records Improvise's choice twice, as the AI picks the play
// and as it pays. Both are read past to the answer Go's question needs.
func TestControllerReadsPastJavaOnlyQuestions(t *testing.T) {
	t.Parallel()

	lone := &call{Seq: 1, Method: "orderSimultaneousSa", Decider: SeatRef(0), Answer: json.RawMessage(`[{"host":{"card":101}}]`)}
	pair := &call{Seq: 2, Method: "orderSimultaneousSa", Decider: SeatRef(0), Answer: json.RawMessage(`[{"host":{"card":102}},{"host":{"card":103}}]`)}
	c, g, cards := rareSetup(t, lone, pair)
	items := []engine.TriggerOrderItem{{Source: cards[2]}, {Source: cards[1]}}
	if got := c.OrderSimultaneousTriggers(g, g.Players()[0], items); !slices.Equal(got, []int{0, 1}) || c.Unanswered() != nil {
		t.Errorf("trigger order %v, unanswered %v; want [0 1] from the two-trigger answer", got, c.Unanswered())
	}

	// Java's AI answers chooseEntitiesForEffect through one nested
	// chooseSingleEntityForEffect per pick: the nested record is no question
	// of its own.
	entities := &call{Seq: 1, Depth: 1, Method: "chooseEntitiesForEffect", Decider: SeatRef(0), Answer: json.RawMessage(`[{"card":101}]`)}
	nested := &call{Seq: 2, Depth: 2, Method: "chooseSingleEntityForEffect", Decider: SeatRef(0), Answer: json.RawMessage(`{"card":101}`)}
	next := &call{Seq: 3, Depth: 1, Method: "chooseSingleEntityForEffect", Decider: SeatRef(0), Answer: json.RawMessage(`{"card":102}`)}
	c, g, cards = rareSetup(t, entities, nested, next)
	options := []engine.EntityID{engine.CardEntity(cards[0]), engine.CardEntity(cards[1])}
	first := c.ChooseEntitiesForEffect(g, g.Players()[0], engine.NoCard, options, 0, 1)
	second := c.ChooseEntitiesForEffect(g, g.Players()[0], engine.NoCard, options, 0, 1)
	if !slices.Equal(first, options[:1]) || !slices.Equal(second, options[1:]) || c.Unanswered() != nil {
		t.Errorf("choices %v then %v, unanswered %v; want the outer answer, then the next question's", first, second, c.Unanswered())
	}

	improvise := func(seq int, raw string) *call {
		return &call{Seq: seq, Method: "chooseCardsForConvokeOrImprovise", Decider: SeatRef(0), Answer: json.RawMessage(raw)}
	}
	for _, tc := range []struct{ name, raw string }{
		{"offered", `[[{"card":102},"GENERIC"],[{"card":103},"GENERIC"]]`},
		{"not offered", `[[{"card":101},"GENERIC"]]`},
	} {
		c, g, cards := rareSetup(t, improvise(1, tc.raw), improvise(3, tc.raw))
		c.cur = &play{Seq: 5, Ability: abilityID{Host: CardRef(101, "")}}
		got, ok := c.convokeCards(g.Players()[0], cards[0], cards[1:], 2)
		switch tc.name {
		case "offered":
			if !ok || !slices.Equal(got, cards[1:]) || !c.s.calls[0].used || !c.s.calls[1].used {
				t.Errorf("%s: %v, %v; want both artifacts and both records read", tc.name, got, ok)
			}
		default:
			if ok {
				t.Errorf("%s: answered %v", tc.name, got)
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

// A recorded land play names no face: a modal double-faced card is played as
// its land face, and one with a land on both faces is refused, not guessed.
func TestControllerPlaysTheLandFaceOfAModalCard(t *testing.T) {
	t.Parallel()
	land, spell := cardtype.ParseToken("Land"), cardtype.ParseToken("Sorcery")
	for _, tc := range []struct {
		name        string
		front, back cardtype.Line
		door        engine.Door
		refused     bool
	}{
		{"spell // land", spell, land, carddb.FaceAlternate, false},
		{"land // spell", land, spell, engine.DoorLeft, false},
		{"land // land", land, land, 0, true},
	} {
		c, g, _ := rareSetup(t)
		def := &compile.Card{Name: "Modal", SplitType: carddb.SplitModal}
		def.Faces[0].Type, def.Faces[carddb.FaceAlternate].Type = tc.front, tc.back
		id := g.NewCard(def, g.Players()[0], engine.Hand)
		c.ids.pair(201, id)
		act, err := c.action(abilityID{Host: CardRef(201, "Modal"), Section: "land"})
		switch {
		case tc.refused && err == nil:
			t.Errorf("%s: played %v, want it refused", tc.name, act)
		case !tc.refused && (err != nil || act.Kind != engine.ActionPlayLand || act.Door != tc.door):
			t.Errorf("%s: action %v, %v; want the land play of face %v", tc.name, act, err, tc.door)
		}
	}
}

// Answers Java records no call for: an optional cost from the one
// chooseOptionalCosts record of the cast, a hybrid symbol's colour from the
// pool, a cost's cards from the play's paid parts.
func TestControllerAnswersCastingQuestionsFromThePlay(t *testing.T) {
	t.Parallel()

	optional := &call{Method: "chooseOptionalCosts",
		Offered: json.RawMessage(`[{"host":{"card":101}},[{"text":"Kicker – {W}"},{"text":"Kicker – {1}"}]]`),
		Answer:  json.RawMessage(`[{"text":"Kicker – {1}"}]`)}
	if got := optionalAnswers(optional); !slices.Equal(got, []bool{false, true}) {
		t.Errorf("optional answers %v, want [false true]", got)
	}
	if got := optionalAnswers(&call{Offered: json.RawMessage(`[]`)}); got != nil {
		t.Errorf("a record without costs: %v", got)
	}
	kicked := answer("chooseOptionalCosts", `[{"text":"Kicker – {1}"}]`)
	kicked.Offered = optional.Offered
	c, g, cards := rareSetup(t, kicked)
	c.castCard = cards[0]
	pid := g.Players()[0]
	if c.ConfirmPayCost(g, pid, cost.Parse("W"), cards[0]) || !c.ConfirmPayCost(g, pid, cost.Parse("1"), cards[0]) {
		t.Error("the kickers are not Java's choice")
	}

	g.Player(pid).ManaPool.Add(mana.White, 2)
	g.Player(pid).ManaPool.Add(mana.Red, 1)
	c.paying = mana.MustParse("1 GW GW")
	for i := range 2 {
		if got := c.ChooseHybridManaColor(g, pid, mana.Green|mana.White); got != mana.White {
			t.Errorf("hybrid %d paid with %v, want white: the pool holds no green", i, got)
		}
	}

	// Kozilek's Command's {X}{C}{C} with X 3 from a pool of four {C} and a
	// {G}: the {C} pips keep two, so the third generic unit is the {G}.
	g.Player(pid).ManaPool = engine.Pool{}
	g.Player(pid).ManaPool.AddColorless(4)
	g.Player(pid).ManaPool.Add(mana.Green, 1)
	c.paying, c.generic = mana.MustParse("3 C C"), [6]int{}
	var generic []mana.Shard
	for range 3 {
		generic = append(generic, c.ChoosePayGeneric(g, pid))
	}
	if want := []mana.Shard{mana.ShardC, mana.ShardC, mana.ShardG}; !slices.Equal(generic, want) {
		t.Errorf("generic paid with %v, want %v", generic, want)
	}

	var p play
	if err := json.Unmarshal([]byte(`{"ability":{"host":{"card":101}},"paid":[{"part":"Exiled","cards":[{"card":102}]},{"part":"ExiledCards","cards":[{"card":102}]}]}`), &p); err != nil {
		t.Fatal(err)
	}
	c.cur, c.paidUsed = &p, map[int]bool{}
	if c.ChoosePayX(g, pid, mana.MustParse("X C C")); c.Unanswered() == nil {
		t.Error("a play naming no X answered the announcement")
	}
	three := 3
	p.X = &three
	if got := c.ChoosePayX(g, pid, mana.MustParse("X C C")); got != 3 {
		t.Errorf("X %d, want the play's 3", got)
	}
	if got, ok := c.costCards(cards[0], cards, 1, 1); !ok || !slices.Equal(got, cards[1:2]) {
		t.Errorf("cost cards %v %v, want the exiled card", got, ok)
	}
	if _, ok := c.costCards(cards[0], cards, 1, 1); ok {
		t.Error("the one payment answered twice")
	}
	if _, ok := c.costCards(cards[1], cards, 1, 1); ok {
		t.Error("a choice for another card answered from this play")
	}
}

// Two triggers of one card are told apart by their effect, and a trigger's
// targets are read from the play that put it on the stack (its wrapper),
// never from the play that resolved it and repeats them unwrapped.
func TestControllerReadsTriggersAsJavaPushedThem(t *testing.T) {
	t.Parallel()

	order := &call{Seq: 1, Method: "orderSimultaneousSa", Decider: SeatRef(0), Answer: json.RawMessage(
		`[{"host":{"card":101},"wrapped":{"host":{"card":101},"api":"Draw"}},{"host":{"card":101},"wrapped":{"host":{"card":101},"api":"ChangeZone"}}]`)}
	c, g, cards := rareSetup(t, order)
	items := []engine.TriggerOrderItem{{Source: cards[0], API: "ChangeZone"}, {Source: cards[0], API: "Draw"}}
	if got := c.OrderSimultaneousTriggers(g, g.Players()[0], items); !slices.Equal(got, []int{0, 1}) || c.Unanswered() != nil {
		// The answer's Draw is item 1, not the host's first item; the order is
		// Java's reversed into Go's.
		t.Errorf("trigger order %v, unanswered %v; want [0 1]", got, c.Unanswered())
	}

	c, g, cards = rareSetup(t)
	for _, raw := range []string{
		`{"seq":1,"decider":{"seat":0},"ability":{"host":{"card":101},"section":"trigger","wrapped":{"host":{"card":101}}},"chain":[{"targets":[{"card":102}]}]}`,
		`{"seq":2,"decider":{"seat":0},"ability":{"host":{"card":101},"section":"trigger"},"chain":[{"targets":[{"card":102}]}]}`,
		`{"seq":3,"decider":{"seat":0},"ability":{"host":{"card":101},"section":"trigger","wrapped":{"host":{"card":101}}},"chain":[{"targets":[{"card":103}]}]}`,
	} {
		p := &play{}
		if err := json.Unmarshal([]byte(raw), p); err != nil {
			t.Fatal(err)
		}
		c.s.plays = append(c.s.plays, p)
	}
	valid := []engine.EntityID{engine.CardEntity(cards[1]), engine.CardEntity(cards[2])}
	first := c.ChooseTargets(g, g.Players()[0], valid, 1, 1)
	second := c.ChooseTargets(g, g.Players()[0], valid, 1, 1)
	if !slices.Equal(first, valid[:1]) || !slices.Equal(second, valid[1:]) || c.Unanswered() != nil {
		t.Errorf("targets %v then %v, unanswered %v; want each pushed trigger's own", first, second, c.Unanswered())
	}
}

// Coins and dice are fed in Java's order, each kind its own queue; a roll of
// another die than Java's takes nothing from it, and an empty queue leaves
// the flip or roll to Go's stream.
func TestChanceFeedsRecordedCoinsAndDice(t *testing.T) {
	t.Parallel()

	ch := &chance{s: &script{coins: []bool{true}, dice: []die{{Sides: 6, Face: 3}}}}
	if _, ok := ch.Die(0, 20); ok {
		t.Error("a d20 took Java's d6 roll")
	}
	if face, ok := ch.Die(0, 6); !ok || face != 3 {
		t.Errorf("d6 rolled %d, %v; want Java's 3", face, ok)
	}
	if _, ok := ch.Die(0, 6); ok {
		t.Error("a roll past the log was answered")
	}
	if heads, ok := ch.Coin(0); !ok || !heads {
		t.Errorf("coin %v, %v; want Java's heads", heads, ok)
	}
	if _, ok := ch.Coin(0); ok {
		t.Error("a flip past the log was answered")
	}
}

// Java names a spell on the stack by its identity (ADR-0059); Go targets it
// as its card. A cast Java found by its params is the copy an
// AlternativeCost static casts, picked among Go's options by its label.
func TestControllerReadsSpellTargetsAndAlternativeCasts(t *testing.T) {
	t.Parallel()

	c, _, cards := rareSetup(t)
	valid := []engine.EntityID{engine.CardEntity(cards[1])}
	spell := json.RawMessage(`{"host":{"card":102},"section":"ability","state":"Original","index":0,"api":"Mana"}`)
	if got, ok := c.stackTarget(spell, valid); !ok || got != valid[0] {
		t.Errorf("spell target %v, %v; want card 102's spell", got, ok)
	}
	trigger := json.RawMessage(`{"host":{"card":102},"section":"trigger","index":0}`)
	if got, ok := c.stackTarget(trigger, valid); ok {
		t.Errorf("trigger target read as the spell %v", got)
	}

	c, g, _ := rareSetup(t)
	def := &compile.Card{Name: "Negator"}
	def.Faces[0].Type = cardtype.ParseToken("Creature")
	id := g.NewCard(def, g.Players()[0], engine.Hand)
	c.ids.pair(201, id)
	zero := 0
	if _, err := c.action(abilityID{Host: CardRef(201, "Negator"), Section: "ability", State: "Original", Index: &zero, API: "PermanentCreature", By: "params"}); err != nil {
		t.Fatal(err)
	}
	options := []string{"Cast Negator", "Cast Negator for its alternative cost Exile a blue card"}
	if got := c.ChooseOption(g, g.Players()[0], id, options); got != 1 || c.Unanswered() != nil {
		t.Errorf("cast option %d, unanswered %v; want the alternative cost", got, c.Unanswered())
	}
}

// A keyword's repeat count (Consign to Memory's Replicate) is Java's
// chooseNumberForKeywordCost answer.
func TestControllerReadsAKeywordCostCount(t *testing.T) {
	t.Parallel()

	c, g, _ := rareSetup(t, answer("chooseNumberForKeywordCost", "2"))
	if got := c.ChooseNumber(g, g.Players()[0], engine.NoCard, 0, 5); got != 2 || c.Unanswered() != nil {
		t.Errorf("count %d, unanswered %v; want Java's 2", got, c.Unanswered())
	}
}
