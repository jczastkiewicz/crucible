package replay

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/cost"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/smoke"
)

// Question is the first question the Go game asked that the log could not
// answer: Java made no such call there, or its answer names something Go
// cannot find or does not offer.
type Question struct {
	// Seq is the Java record the answer came from, -1 when there was none.
	Seq    int
	Turn   int
	Step   string
	Method string
	Detail string
}

func (q *Question) String() string {
	return fmt.Sprintf("turn %d %s: %s (java record %d): %s", q.Turn, q.Step, q.Method, q.Seq, q.Detail)
}

// Controller answers a replayed Go game's questions from a Java game's log
// (ADR-0059, ADR-0062): each question reads the next unread Java call of the
// matching kind made by the same seat in the same turn and step. A question
// the log does not cover yet is answered by the smoke probe's defaults and,
// when the log has nothing to say for it, recorded as the replay's first
// unanswered question.
type Controller struct {
	smoke.Controller
	g   *engine.Game
	s   *script
	ids *IDMap
	// cur is the play TakeAction last started, targets its sub-abilities'
	// target lists still to answer, paid whether its mana is paid.
	cur     *play
	targets [][]json.RawMessage
	paid    bool
	// paidUsed is which of cur's paid parts a cost card choice has spent.
	paidUsed map[int]bool
	// castWay is how the play TakeAction started casts its card: "" for
	// the spell's own way, else the alternative-cost keyword ("Warp"),
	// for ChooseOption's cast-option question.
	castWay  string
	castCard engine.CardID
	// optional is what Java's one chooseOptionalCosts answer for the cast
	// of castCard says of each cost it offered, in order: Go asks
	// ConfirmPayCost once per optional cost (kicker.go), in the same
	// order. asked is whether that record was read for this cast.
	optional []bool
	asked    bool
	// pending is the rest of a chooseSpellAbilityToPlay answer naming more
	// than one play, after the call it came from.
	pending     []abilityID
	pendingCall *call
	// attacks is the defender each attacker of the last declareAttackers
	// answer attacked.
	attacks map[engine.CardID]Ref
	// triggerPlays is how many trigger play records ChooseTargets has read.
	triggerPlays map[*play]bool
	// picked is the call chooseCards last answered from.
	picked *call
	first  *Question
	// last is the Java answer TakeAction last played, for a Go error's
	// report.
	last string
	// paying is the cost PayManaCost was last asked for, generic the pool
	// buckets ChoosePayGeneric has spent on its generic part so far.
	paying  mana.Cost
	generic [6]int
	// hybrid is the pool buckets ChooseHybridManaColor has spent on the
	// cost's two-colour hybrid symbols so far.
	hybrid [6]int
}

func newController(g *engine.Game, s *script, ids *IDMap) *Controller {
	return &Controller{g: g, s: s, ids: ids, triggerPlays: map[*play]bool{}}
}

// Unanswered is the first question the log could not answer, nil if none.
func (c *Controller) Unanswered() *Question { return c.first }

func (c *Controller) now() when {
	if c.g.Turn() == 0 {
		return when{}
	}
	return when{c.g.Turn(), javaSteps[stepName(c.g.ActivePhase())]}
}

func (c *Controller) fail(seq int, method, format string, args ...any) {
	if c.first != nil {
		return
	}
	step := ""
	if c.g.Turn() > 0 {
		step = stepName(c.g.ActivePhase())
	}
	c.first = &Question{Seq: seq, Turn: c.g.Turn(), Step: step, Method: method, Detail: fmt.Sprintf(format, args...)}
}

// next is seat's first unread call to one of methods made at the current
// turn and step, marked read.
func (c *Controller) next(pid engine.PlayerID, methods ...string) *call {
	now := c.now()
	for _, k := range c.s.calls {
		if !k.used && k.Decider.Seat == seat(pid) && k.when() == now && slices.Contains(methods, k.Method) {
			k.used = true
			if k.Method == "chooseEntitiesForEffect" || k.Method == "chooseCardsForEffect" {
				c.readNested(k, "chooseSingleEntityForEffect")
			}
			return k
		}
	}
	return nil
}

func (c *Controller) goCard(r Ref) (engine.CardID, bool) {
	if r.Card == 0 {
		return engine.NoCard, false
	}
	return c.ids.GoCard(r.Card)
}

func (c *Controller) entity(r Ref) (engine.EntityID, bool) {
	if r.Card != 0 {
		id, ok := c.goCard(r)
		return engine.CardEntity(id), ok
	}
	if r.Seat >= 0 && r.Seat < len(c.g.Players()) {
		return engine.PlayerEntity(c.g.Players()[r.Seat]), true
	}
	return 0, false
}

// cards maps an answer naming cards, one or a list, to Go cards.
func (c *Controller) cards(raw json.RawMessage) ([]engine.CardID, bool) {
	var refs []Ref
	if err := json.Unmarshal(raw, &refs); err != nil {
		var one Ref
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, false
		}
		refs = []Ref{one}
	}
	out := make([]engine.CardID, 0, len(refs))
	for _, r := range refs {
		if r.Card == 0 && r.Seat < 0 {
			continue
		}
		id, ok := c.goCard(r)
		if !ok {
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}

// ChooseStartingPlayer is Java's chooseStartingPlayer answer.
func (c *Controller) ChooseStartingPlayer(g *engine.Game, decider engine.PlayerID, first bool) engine.PlayerID {
	for _, k := range c.s.calls {
		if !k.used && k.Method == "chooseStartingPlayer" {
			k.used = true
			var r Ref
			if json.Unmarshal(k.Answer, &r) == nil && r.Seat >= 0 && r.Seat < len(g.Players()) {
				return g.Players()[r.Seat]
			}
		}
	}
	c.fail(-1, "chooseStartingPlayer", "no answer")
	return c.Controller.ChooseStartingPlayer(g, decider, first)
}

// MulliganKeepHand is Java's mulliganKeepHand answer.
func (c *Controller) MulliganKeepHand(_ *engine.Game, decider, _ engine.PlayerID, _ int) bool {
	if k := c.next(decider, "mulliganKeepHand"); k != nil {
		var keep bool
		if json.Unmarshal(k.Answer, &keep) == nil {
			return keep
		}
	}
	c.fail(-1, "mulliganKeepHand", "no answer for seat %d", seat(decider))
	return true
}

// TuckCardsViaMulligan is Java's tuckCardsViaMulligan answer.
func (c *Controller) TuckCardsViaMulligan(g *engine.Game, decider engine.PlayerID, hand []engine.CardID, n int) []engine.CardID {
	if k := c.next(decider, "tuckCardsViaMulligan"); k != nil {
		if ids, ok := c.cards(k.Answer); ok && len(ids) == n {
			return ids
		}
		c.fail(k.Seq, k.Method, "answer %s is not %d cards of the hand", k.Answer, n)
	} else {
		c.fail(-1, "tuckCardsViaMulligan", "no answer for seat %d", seat(decider))
	}
	return c.Controller.TuckCardsViaMulligan(g, decider, hand, n)
}

// TakeAction is the next play of Java's chooseSpellAbilityToPlay answers
// at this priority, a pass when Java answered none or was not asked.
func (c *Controller) TakeAction(_ *engine.Game, pid engine.PlayerID) engine.Action {
	c.cur, c.targets = nil, nil
	if len(c.pending) > 0 {
		a := c.pending[0]
		c.pending = c.pending[1:]
		return c.start(pid, a, c.pendingCall)
	}
	c.skipMissedPriority(pid)
	k := c.next(pid, "chooseSpellAbilityToPlay")
	if k == nil {
		return engine.Action{}
	}
	var answer []abilityID
	if err := json.Unmarshal(k.Answer, &answer); err != nil || len(answer) == 0 {
		return engine.Action{}
	}
	c.pending, c.pendingCall = answer[1:], k
	return c.start(pid, answer[0], k)
}

// skipMissedPriority marks pid's priority calls from earlier steps read: Go
// gave no priority there. One that played something is the replay's first
// unanswered question.
func (c *Controller) skipMissedPriority(pid engine.PlayerID) {
	now := c.now()
	for _, k := range c.s.calls {
		if k.used || k.Decider.Seat != seat(pid) || k.Method != "chooseSpellAbilityToPlay" || !k.when().before(now) {
			continue
		}
		k.used = true
		if string(k.Answer) != "null" && string(k.Answer) != "[]" {
			c.fail(k.Seq, k.Method, "Java played %s at turn %d %s, where Go gave no priority", k.Answer, k.Turn, k.Phase)
		}
	}
}

// start begins the play Java answered with a: its play record holds the
// rest of its choices.
func (c *Controller) start(pid engine.PlayerID, a abilityID, k *call) engine.Action {
	var pc *call
	for _, x := range c.s.calls {
		if !x.used && x.Seq > k.Seq && x.Decider.Seat == seat(pid) && x.Method == "playChosenSpellAbility" {
			x.used = true
			pc = x
			break
		}
	}
	if pc != nil {
		for _, p := range c.s.plays {
			if p.Call == pc.Seq {
				c.cur = p
			}
		}
	}
	c.paid, c.paidUsed = false, map[int]bool{}
	if c.cur != nil {
		for _, part := range c.cur.Chain {
			if len(part.Targets) > 0 {
				c.targets = append(c.targets, part.Targets)
			}
		}
	}
	c.last = fmt.Sprintf("record %d, turn %d %s: %s", k.Seq, k.Turn, k.Phase, a)
	act, err := c.action(a)
	if err != nil {
		c.fail(k.Seq, k.Method, "%s: %v", a, err)
		return engine.Action{}
	}
	return act
}

// LastPlay is the Java answer TakeAction last played, "" if none.
func (c *Controller) LastPlay() string { return c.last }

// action is the Go action that plays the Java ability a.
func (c *Controller) action(a abilityID) (engine.Action, error) {
	id, ok := c.goCard(a.Host)
	if !ok {
		return engine.Action{}, fmt.Errorf("no Go card for %s", a.Host)
	}
	section := a.Section
	if a.Keyword != "" && section == "ability" {
		section = "keyword"
	}
	switch section {
	case "land":
		// Java's land play names no face: a modal double-faced card whose
		// front is no land is played as its back (Sink into Stupor). One
		// with a land on both faces (a Pathway) is a face the log does not
		// name, never guessed.
		act := engine.Action{Kind: engine.ActionPlayLand, Card: id}
		if card := c.g.Card(id); card.Def.SplitType == carddb.SplitModal {
			front := card.Def.Faces[0].Type.Has(cardtype.Land)
			back := card.Def.Faces[carddb.FaceAlternate].Type.Has(cardtype.Land)
			switch {
			case front && back:
				return engine.Action{}, fmt.Errorf("land play of %s names no face", a.Host)
			case back:
				act.Door = carddb.FaceAlternate
			}
		}
		return act, nil
	case "keyword":
		// An alternative cast a keyword makes (Warp:W): the keyword's spell,
		// picked among Go's cast options by its label.
		if !strings.HasPrefix(a.API, "Permanent") && !c.isSpellAPI(id, a.API) {
			return c.keywordAbility(id, a)
		}
		name, _, _ := strings.Cut(a.Keyword, ":")
		c.castWay, c.castCard, c.asked = name, id, false
		return engine.Action{Kind: engine.ActionCast, Card: id}, nil
	case "ability":
		ab, index, cast, err := c.printed(id, a)
		switch {
		case err != nil:
			if a.API == "Mana" && c.g.Card(id).Type().Has(cardtype.Land) {
				return engine.Action{Kind: engine.ActionTapForMana, Card: id, Color: c.landColor(id, 0)}, nil
			}
			return engine.Action{}, err
		case cast:
			// A spell Java found by its params is the copy an
			// AlternativeCost static casts (Force of Negation's exile).
			way := ""
			if a.By == "params" {
				way = "alternative"
			}
			c.castWay, c.castCard, c.asked = way, id, false
			return engine.Action{Kind: engine.ActionCast, Card: id}, nil
		case ab.Name == "Mana":
			return engine.Action{Kind: engine.ActionManaAbility, Card: id, AbilityIndex: index}, nil
		default:
			return engine.Action{Kind: engine.ActionActivate, Card: id, AbilityIndex: index}, nil
		}
	case "rule", "granted":
		// An ability an effect or a static gave the card (Urza's Saga's
		// chapter): Go numbers them after the printed lines; Java names the
		// API alone, so the first one with that API.
		card := c.g.Card(id)
		for i := len(card.Def.Faces[0].Abilities); i < card.AbilityCount(); i++ {
			if ab, ok := card.AbilityAt(i); ok && ab.Name == a.API {
				kind := engine.ActionActivate
				if ab.Name == "Mana" {
					kind = engine.ActionManaAbility
				}
				return engine.Action{Kind: kind, Card: id, AbilityIndex: i}, nil
			}
		}
		return engine.Action{}, fmt.Errorf("the Go card has no gained %s ability", a.API)
	}
	return engine.Action{}, fmt.Errorf("section %q is not mapped to a Go action yet", a.Section)
}

// keywordAbility is the activated ability a keyword stands for (Equip:1 R):
// Go appends a keyword's lines after the printed ones (ADR-0038), each
// naming its K: line, and Java's index is the trait's place among that
// keyword's.
func (c *Controller) keywordAbility(id engine.CardID, a abilityID) (engine.Action, error) {
	nth := 0
	if a.Index != nil {
		nth = *a.Index
	}
	for i, ab := range c.g.Card(id).Def.Faces[0].Abilities {
		if ab.Keyword != a.Keyword || ab.Record != compile.Activated {
			continue
		}
		if nth > 0 {
			nth--
			continue
		}
		if ab.Name != a.API {
			return engine.Action{}, fmt.Errorf("the Go %s ability is %s, the Java one %s", a.Keyword, ab.Name, a.API)
		}
		return engine.Action{Kind: engine.ActionActivate, Card: id, AbilityIndex: i}, nil
	}
	return engine.Action{}, fmt.Errorf("keyword ability %s is not mapped to a Go action yet", a.Keyword)
}

// printed is the Go ability line Java's ability a names, and its index for
// an Action, or cast true when a is the card's own permanent spell. Java
// numbers a state's intrinsic spell abilities in script order, a permanent
// card's spell first (it has no SP line); Go numbers its printed lines. The
// API must agree.
func (c *Controller) printed(id engine.CardID, a abilityID) (ab *compile.Ability, index int, cast bool, err error) {
	// A modal card played as its back face (Witch-Blessed Meadow) is that
	// face in Go: its lines are the definition's first face, as Java's
	// Backside state's are numbered.
	if a.Index == nil || a.State != "Original" && (a.State != "Backside" || c.g.Card(id).Name() != a.Host.Name) {
		return nil, 0, false, fmt.Errorf("state %q index %v is not mapped yet", a.State, a.Index)
	}
	def := c.g.Card(id).Def
	if def == nil || len(def.Faces) == 0 {
		return nil, 0, false, fmt.Errorf("the Go card has no definition")
	}
	lines := def.Faces[0].Abilities
	index = *a.Index
	// A back face with no mana cost (a transformed planeswalker) has no
	// spell; a token's state, with none either, has one.
	transformed := a.State == "Backside" && def.Faces[0].ManaCost.IsNoCost()
	if !c.g.Card(id).Type().Has(cardtype.Land) && !transformed && !slices.ContainsFunc(lines, func(l *compile.Ability) bool {
		return l.Record == compile.Spell
	}) {
		if index == 0 {
			if !strings.HasPrefix(a.API, "Permanent") {
				return nil, 0, false, fmt.Errorf("ability 0 of a Java permanent is %s, not its spell", a.API)
			}
			return nil, 0, true, nil
		}
		index--
	}
	if index >= len(lines) {
		return nil, 0, false, fmt.Errorf("the Go card has no ability %d", index)
	}
	ab = lines[index]
	if ab.Name != a.API {
		return nil, 0, false, fmt.Errorf("the Go ability %d is %s, the Java one %s", index, ab.Name, a.API)
	}
	return ab, index, ab.Record == compile.Spell, nil
}

// landColor is the color a land without a printed mana ability taps for:
// the color Java's mana paid with, else its first basic land type's.
func (c *Controller) landColor(id engine.CardID, javaColor int) mana.Colors {
	if javaColor != 0 {
		return mana.Colors(javaColor)
	}
	t := c.g.Card(id).Type()
	for _, b := range []struct {
		sub   string
		color mana.Colors
	}{{"Plains", mana.White}, {"Island", mana.Blue}, {"Swamp", mana.Black}, {"Mountain", mana.Red}, {"Forest", mana.Green}} {
		if t.HasSubtype(b.sub) {
			return b.color
		}
	}
	return 0
}

// PayManaCost activates the mana abilities Java paid with: the current
// play's for its own cost, else the payment record of the next pay call.
func (c *Controller) PayManaCost(_ *engine.Game, payer engine.PlayerID, toPay mana.Cost, paidFor engine.CardID) []engine.Action {
	c.paying, c.generic, c.hybrid = toPay, [6]int{}, [6]int{}
	var abilities []abilityID
	colors := map[int][]int{}
	if c.cur != nil && !c.paid && paidFor != engine.NoCard {
		if host, ok := c.goCard(c.cur.Ability.Host); ok && host == paidFor {
			c.paid = true
			c.paymentCall(payer, paidFor)
			abilities = c.cur.ManaAbilities
			for _, m := range c.cur.PayingMana {
				colors[m.Source.Card] = append(colors[m.Source.Card], m.Color)
			}
		}
	}
	if abilities == nil {
		k, before := c.paymentCall(payer, paidFor)
		abilities = before
		if k == nil {
			if toPay.IsZero() {
				return nil
			}
			c.fail(-1, "payManaCost", "Java paid nothing here; Go pays %s for %v", toPay, paidFor)
			return nil
		}
		if p := c.s.paymentFor(k.Seq); p != nil {
			abilities = append(abilities, p.ManaAbilities...)
		}
	}
	var out []engine.Action
	for _, a := range abilities {
		color := 0
		if q := colors[a.Host.Card]; len(q) > 0 {
			color, colors[a.Host.Card] = q[0], q[1:]
		}
		act, err := c.action(a)
		if err != nil {
			c.fail(-1, "payManaCost", "%s: %v", a, err)
			continue
		}
		if act.Kind == engine.ActionTapForMana {
			act.Color = c.landColor(act.Card, color)
		}
		out = append(out, act)
	}
	return out
}

// paymentCall is payer's first unread payManaCost call paying for paidFor,
// else the first unread one, marked read. The AI can activate a mana ability for
// another one's cost before it asks to pay that cost (an Island tapped,
// then Mystic Gate's {W/U} paid from the pool): the Mana abilities whose
// own payManaCost calls come between are returned with it, left unread
// for their own activations' payments.
func (c *Controller) paymentCall(payer engine.PlayerID, paidFor engine.CardID) (*call, []abilityID) {
	now := c.now()
	var first *call
	var before []abilityID
	for _, k := range c.s.calls {
		if k.used || k.Decider.Seat != seat(payer) || k.when() != now || k.Method != "payManaCost" {
			continue
		}
		if first == nil {
			first = k
		}
		var offered []json.RawMessage
		var a abilityID
		if json.Unmarshal(k.Offered, &offered) != nil || len(offered) < 3 || json.Unmarshal(offered[2], &a) != nil {
			before = nil
			continue
		}
		if host, ok := c.goCard(a.Host); ok && host == paidFor {
			k.used = true
			return k, before
		}
		if a.API != "Mana" {
			before = nil
			continue
		}
		before = append(before, a)
	}
	if first != nil {
		first.used = true
	}
	return first, nil
}

// ChoosePayX is the X of the current play: Java's AI announces it without a
// controller call (ComputerUtilMana), and the play record names it. A play
// that recorded none, or no play under way, is the replay's unanswered
// question.
func (c *Controller) ChoosePayX(g *engine.Game, decider engine.PlayerID, cost mana.Cost) int {
	if c.cur != nil && c.cur.X != nil {
		return *c.cur.X
	}
	c.fail(-1, "announceX", "Java's play names no X for %s", cost)
	return c.Controller.ChoosePayX(g, decider, cost)
}

// ChooseTargets is the next target list of the current play, else of the
// next trigger Java put on the stack for decider.
func (c *Controller) ChooseTargets(g *engine.Game, decider engine.PlayerID, valid []engine.EntityID, lo, hi int) []engine.EntityID {
	if len(c.targets) == 0 {
		// A spell a Play effect casts (rebound, cascade) has its own play
		// record, linked to Java's playSaFromPlayEffect call.
		if k := c.next(decider, "playSaFromPlayEffect"); k != nil {
			for _, p := range c.s.plays {
				if p.Call != k.Seq {
					continue
				}
				for _, part := range p.Chain {
					if len(part.Targets) > 0 {
						c.targets = append(c.targets, part.Targets)
					}
				}
			}
		}
	}
	if len(c.targets) == 0 {
		for _, p := range c.s.plays {
			// A trigger's targets are chosen as it is put on the stack: the play
			// of its wrapper, a delayed or reflexive one's too (Guide of
			// Souls), never the play of its resolution, which repeats them
			// unwrapped (Soul-Guide Lantern).
			pushed := (p.Ability.Section == "trigger" || p.Ability.Section == "delayed") && p.Ability.Wrapped != nil
			if c.triggerPlays[p] || p.Decider.Seat != seat(decider) || !pushed {
				continue
			}
			c.triggerPlays[p] = true
			for _, part := range p.Chain {
				if len(part.Targets) > 0 {
					c.targets = append(c.targets, part.Targets)
				}
			}
			if len(c.targets) > 0 {
				break
			}
		}
	}
	if len(c.targets) == 0 {
		c.fail(-1, "chooseTargets", "Java chose no targets here")
		return c.Controller.ChooseTargets(g, decider, valid, lo, hi)
	}
	raw := c.targets[0]
	c.targets = c.targets[1:]
	out := make([]engine.EntityID, 0, len(raw))
	for _, t := range raw {
		var r Ref
		e, ok := engine.EntityID(0), json.Unmarshal(t, &r) == nil
		if ok {
			e, ok = c.entity(r)
		}
		if !ok {
			e, ok = c.stackTarget(t, valid)
		}
		if !ok || !slices.Contains(valid, e) {
			c.fail(-1, "chooseTargets", "Java's target %s is not one Go offers", t)
			return c.Controller.ChooseTargets(g, decider, valid, lo, hi)
		}
		out = append(out, e)
	}
	return out
}

// stackTarget is the item among valid that Java's target names when it is
// on the stack, recorded by its identity (ADR-0059). Go targets a spell (an
// "ability" section: the cast spell's own ability) as its card, any other
// item (a trigger, an activated ability) as that stack item.
func (c *Controller) stackTarget(raw json.RawMessage, valid []engine.EntityID) (engine.EntityID, bool) {
	var a abilityID
	if json.Unmarshal(raw, &a) != nil || a.Host.Card == 0 {
		return 0, false
	}
	host, ok := c.goCard(a.Host)
	if !ok {
		return 0, false
	}
	if a.Section == "ability" {
		if slices.Contains(valid, engine.CardEntity(host)) {
			return engine.CardEntity(host), true
		}
		return 0, false
	}
	for _, e := range valid {
		if id, isItem := e.AsStackItem(); isItem {
			if src, spell, ok := c.g.StackItemOf(id); ok && src == host && !spell {
				return e, true
			}
		}
	}
	return 0, false
}

// AssignCombatDamage is Java's assignCombatDamage answer for this attacker
// (its first argument): Java asks for a lone recipient too, Go only for two
// or more, so the record is found by attacker, not taken in turn.
func (c *Controller) AssignCombatDamage(g *engine.Game, decider engine.PlayerID, attacker engine.CardID, blockers []engine.CardID) []engine.DamageAssignment {
	now := c.now()
	for _, k := range c.s.calls {
		if k.used || k.Method != "assignCombatDamage" || k.Decider.Seat != seat(decider) || k.when() != now {
			continue
		}
		var offered []json.RawMessage
		var first Ref
		if json.Unmarshal(k.Offered, &offered) != nil || len(offered) == 0 || json.Unmarshal(offered[0], &first) != nil {
			continue
		}
		if id, ok := c.goCard(first); !ok || id != attacker {
			continue
		}
		k.used = true
		var answer [][2]json.RawMessage
		if json.Unmarshal(k.Answer, &answer) != nil {
			break
		}
		out := make([]engine.DamageAssignment, 0, len(answer))
		for _, a := range answer {
			var r Ref
			var n int
			if json.Unmarshal(a[0], &r) != nil || json.Unmarshal(a[1], &n) != nil {
				break
			}
			id, ok := c.goCard(r)
			if !ok || !slices.Contains(blockers, id) {
				break
			}
			out = append(out, engine.DamageAssignment{Blocker: id, Amount: n})
		}
		if len(out) == len(answer) {
			return out
		}
		c.fail(k.Seq, k.Method, "answer %s is not among Go's recipients", k.Answer)
		return c.Controller.AssignCombatDamage(g, decider, attacker, blockers)
	}
	c.fail(-1, "assignCombatDamage", "Java assigned no damage for this attacker here")
	return c.Controller.AssignCombatDamage(g, decider, attacker, blockers)
}

type combatAnswer struct {
	Attacker Ref   `json:"attacker"`
	Defender Ref   `json:"defender"`
	Blockers []Ref `json:"blockers"`
}

// DeclareCombatAttackers is Java's declareAttackers answer; no attack when
// Java was not asked.
func (c *Controller) DeclareCombatAttackers(_ *engine.Game, decider engine.PlayerID, eligible []engine.CardID) []engine.CardID {
	k := c.next(decider, "declareAttackers")
	if k == nil {
		return nil
	}
	var answer []combatAnswer
	if err := json.Unmarshal(k.Answer, &answer); err != nil {
		c.fail(k.Seq, k.Method, "answer %s: %v", k.Answer, err)
		return nil
	}
	c.attacks = map[engine.CardID]Ref{}
	var out []engine.CardID
	for _, a := range answer {
		id, ok := c.goCard(a.Attacker)
		if !ok || !slices.Contains(eligible, id) {
			c.fail(k.Seq, k.Method, "attacker %s is not one Go offers", a.Attacker)
			continue
		}
		c.attacks[id] = a.Defender
		out = append(out, id)
	}
	return out
}

// ChooseAttackTarget is the defender Java's declareAttackers gave attacker.
func (c *Controller) ChooseAttackTarget(g *engine.Game, decider engine.PlayerID, attacker engine.CardID, eligible []engine.EntityID) engine.EntityID {
	if d, ok := c.attacks[attacker]; ok {
		if e, ok := c.entity(d); ok && slices.Contains(eligible, e) {
			return e
		}
	}
	// A creature put onto the battlefield attacking (Mobilize's tokens):
	// Java asks the activator for its defender with
	// chooseSingleEntityForEffect (SpellAbilityEffect.java:797).
	if k := c.next(decider, "chooseSingleEntityForEffect"); k != nil {
		var r Ref
		if json.Unmarshal(k.Answer, &r) == nil {
			if e, ok := c.entity(r); ok && slices.Contains(eligible, e) {
				return e
			}
		}
		c.fail(k.Seq, k.Method, "defender %s is not one Go offers for %d", k.Answer, attacker)
		return c.Controller.ChooseAttackTarget(g, decider, attacker, eligible)
	}
	c.fail(-1, "chooseAttackTarget", "no recorded defender Go offers for %d", attacker)
	return c.Controller.ChooseAttackTarget(g, decider, attacker, eligible)
}

// DeclareCombatBlockers is Java's declareBlockers answer; no block when Java
// was not asked.
func (c *Controller) DeclareCombatBlockers(_ *engine.Game, decider engine.PlayerID, _, eligible []engine.CardID) []engine.Block {
	k := c.next(decider, "declareBlockers")
	if k == nil {
		return nil
	}
	var answer []combatAnswer
	if err := json.Unmarshal(k.Answer, &answer); err != nil {
		c.fail(k.Seq, k.Method, "answer %s: %v", k.Answer, err)
		return nil
	}
	var out []engine.Block
	for _, a := range answer {
		att, ok := c.goCard(a.Attacker)
		if !ok {
			continue
		}
		for _, b := range a.Blockers {
			if id, ok := c.goCard(b); ok && slices.Contains(eligible, id) {
				out = append(out, engine.Block{Blocker: id, Attacker: att})
			} else {
				c.fail(k.Seq, k.Method, "blocker %s is not one Go offers", b)
			}
		}
	}
	return out
}

// OrderSimultaneousTriggers is Java's orderSimultaneousSa answer, which
// lists the triggers in the order they go on the stack: Go wants the
// reverse, first to resolve first.
func (c *Controller) OrderSimultaneousTriggers(g *engine.Game, decider engine.PlayerID, items []engine.TriggerOrderItem) []int {
	// Java asks about a lone trigger too (MagicStack's
	// chooseOrderOfSimultaneousStackEntry calls orderAndPlaySimultaneousSa
	// for any), Go only about two or more: a one-trigger answer was a push
	// Go made without asking, never this question.
	var k *call
	var answer []abilityID
	for {
		k = c.next(decider, "orderSimultaneousSa")
		if k == nil {
			c.fail(-1, "orderSimultaneousSa", "Java ordered no triggers here")
			return c.Controller.OrderSimultaneousTriggers(g, decider, items)
		}
		answer = nil
		_ = json.Unmarshal(k.Answer, &answer)
		if len(answer) > 1 {
			break
		}
	}
	used := make([]bool, len(items))
	var order []int
	for _, a := range answer {
		host, _ := c.goCard(a.Host)
		api := a.API
		if a.Wrapped != nil {
			api = a.Wrapped.API
		}
		// The trigger of the host with the answer's effect; failing that, the
		// host's first: two triggers of one card differ by their effect
		// (Haliya's draw and its warp exile).
		pick := -1
		for i, it := range items {
			if used[i] || it.Source != host {
				continue
			}
			if it.API == api {
				pick = i
				break
			}
			if pick < 0 {
				pick = i
			}
		}
		if pick >= 0 {
			used[pick] = true
			order = append(order, pick)
		}
	}
	if len(order) != len(items) {
		c.fail(k.Seq, k.Method, "answer %s does not name Go's %d triggers", k.Answer, len(items))
		return c.Controller.OrderSimultaneousTriggers(g, decider, items)
	}
	slices.Reverse(order)
	return order
}

// ArrangeForSurveil is Java's arrangeForSurveil answer: [top, graveyard].
func (c *Controller) ArrangeForSurveil(g *engine.Game, decider engine.PlayerID, topN []engine.CardID) ([]engine.CardID, []engine.CardID) {
	if top, rest, ok := c.twoPiles(decider, "arrangeForSurveil"); ok {
		return top, rest
	}
	return c.Controller.ArrangeForSurveil(g, decider, topN)
}

// ArrangeForScry is Java's arrangeForScry answer: [top, bottom].
func (c *Controller) ArrangeForScry(g *engine.Game, decider engine.PlayerID, topN []engine.CardID) ([]engine.CardID, []engine.CardID) {
	if top, rest, ok := c.twoPiles(decider, "arrangeForScry"); ok {
		return top, rest
	}
	return c.Controller.ArrangeForScry(g, decider, topN)
}

func (c *Controller) twoPiles(decider engine.PlayerID, method string) ([]engine.CardID, []engine.CardID, bool) {
	k := c.next(decider, method)
	if k == nil {
		c.fail(-1, method, "Java made no such call here")
		return nil, nil, false
	}
	var piles []json.RawMessage
	if json.Unmarshal(k.Answer, &piles) == nil && len(piles) == 2 {
		top, ok1 := c.cards(piles[0])
		rest, ok2 := c.cards(piles[1])
		if ok1 && ok2 {
			return top, rest, true
		}
	}
	c.fail(k.Seq, k.Method, "answer %s", k.Answer)
	return nil, nil, false
}

// cardChoices are the Java calls that answer with cards chosen from a list.
var cardChoices = []string{
	"chooseSingleEntityForEffect", "chooseEntitiesForEffect", "chooseCardsForEffect",
	"chooseSingleCardForZoneChange", "chooseCardsForZoneChange", "choosePermanentsToSacrifice",
	"choosePermanentsToDestroy", "chooseCardsToDiscardFrom", "chooseCardsToDiscardToMaximumHandSize",
	"chooseCardsForCost",
}

// chooseCards is the next card-choice answer of decider's, kept to options.
func (c *Controller) chooseCards(decider engine.PlayerID, method string, options []engine.CardID) ([]engine.CardID, bool) {
	for {
		k := c.next(decider, cardChoices...)
		if k == nil {
			c.fail(-1, method, "Java chose no cards here")
			return nil, false
		}
		ids, ok := c.cards(k.Answer)
		if !ok {
			c.fail(k.Seq, k.Method, "answer %s names a card Go does not have", k.Answer)
			return nil, false
		}
		if !slices.ContainsFunc(ids, func(id engine.CardID) bool { return !slices.Contains(options, id) }) {
			c.picked = k
			return ids, true
		}
		// A one-option single-entity record outside Go's options is a
		// question Go never asked (ChooseEntitiesForEffect).
		if k.Method != "chooseSingleEntityForEffect" || !loneOffer(k) {
			c.fail(k.Seq, k.Method, "answer %s is not among Go's options", k.Answer)
			return nil, false
		}
	}
}

// ChooseCardsForEffect is Java's card choice.
func (c *Controller) ChooseCardsForEffect(g *engine.Game, decider engine.PlayerID, source engine.CardID, options []engine.CardID, lo, hi int) []engine.CardID {
	if source == engine.NoCard {
		// RunOpeningHandActions asks with no source: Java's
		// chooseSaToActivateFromOpeningHand, answered with abilities.
		if k := c.next(decider, "chooseSaToActivateFromOpeningHand"); k != nil {
			var answer []abilityID
			_ = json.Unmarshal(k.Answer, &answer)
			var out []engine.CardID
			for _, a := range answer {
				if id, ok := c.goCard(a.Host); ok && slices.Contains(options, id) {
					out = append(out, id)
				}
			}
			if len(out) == len(answer) {
				return out
			}
			c.fail(k.Seq, k.Method, "answer %s is not among Go's options", k.Answer)
			return nil
		}
	}
	if ids, ok := c.costCards(source, options, lo, hi); ok {
		return ids
	}
	if ids, ok := c.convokeCards(decider, source, options, hi); ok {
		return ids
	}
	if ids, ok := c.chooseCards(decider, "chooseCardsForEffect", options); ok {
		return c.laterPicks(decider, ids, options, hi)
	}
	return c.Controller.ChooseCardsForEffect(g, decider, source, options, lo, hi)
}

// pickTitle is the "(i / n)" of a search's one-card prompt ("Select a card
// from your library (2 / 2)").
var pickTitle = regexp.MustCompile(`\((\d+) / \d+\)$`)

// pickIndex is the i of a chooseSingleCardForZoneChange call's prompt, 0
// for none.
func pickIndex(k *call) int {
	var offered []json.RawMessage
	_ = json.Unmarshal(k.Offered, &offered)
	for _, o := range offered {
		var title string
		if json.Unmarshal(o, &title) != nil {
			continue
		}
		if m := pickTitle.FindStringSubmatch(title); m != nil {
			i, _ := strconv.Atoi(m[1])
			return i
		}
	}
	return 0
}

// laterPicks completes a search Java answers one card per call
// (ChangeZoneEffect.changeHiddenOriginResolve asks chooseSingleCardForZoneChange
// once per card, its prompt numbered "(i / n)") where Go asks for up to hi
// cards at once: the calls numbered on from the first are read until hi
// cards or a null answer, Java's "up to" stop.
func (c *Controller) laterPicks(decider engine.PlayerID, ids, options []engine.CardID, hi int) []engine.CardID {
	if c.picked == nil || c.picked.Method != "chooseSingleCardForZoneChange" || pickIndex(c.picked) != 1 {
		return ids
	}
	now := c.now()
	for _, k := range c.s.calls {
		if len(ids) >= hi {
			break
		}
		if k.used || k.Decider.Seat != seat(decider) || k.when() != now || k.Method != "chooseSingleCardForZoneChange" {
			continue
		}
		if pickIndex(k) != len(ids)+1 {
			break
		}
		k.used = true
		if string(k.Answer) == "null" {
			break
		}
		more, ok := c.cards(k.Answer)
		if !ok || slices.ContainsFunc(more, func(id engine.CardID) bool { return !slices.Contains(options, id) || slices.Contains(ids, id) }) {
			c.fail(k.Seq, k.Method, "answer %s is not among Go's options", k.Answer)
			break
		}
		ids = append(ids, more...)
	}
	return ids
}

// convokeCards answers Convoke's or Improvise's choice of permanents to tap
// for the play under way from Java's chooseCardsForConvokeOrImprovise, whose
// answer pairs each card with the colour it paid. Java asks once while the
// AI chooses the play and again while it pays: every such call of the play
// is read, the last one answers. An answer naming a card Go does not offer
// is none: Go asks Convoke and Improvise apart, as Java does.
func (c *Controller) convokeCards(decider engine.PlayerID, source engine.CardID, options []engine.CardID, hi int) ([]engine.CardID, bool) {
	if c.cur == nil {
		return nil, false
	}
	if host, ok := c.goCard(c.cur.Ability.Host); !ok || host != source {
		return nil, false
	}
	var last *call
	for _, k := range c.s.calls {
		if k.used || k.Decider.Seat != seat(decider) || k.Method != "chooseCardsForConvokeOrImprovise" || k.Seq > c.cur.Seq {
			continue
		}
		if k.when() == c.now() {
			last = k
		}
	}
	if last == nil {
		return nil, false
	}
	var pairs [][]json.RawMessage
	if err := json.Unmarshal(last.Answer, &pairs); err != nil {
		return nil, false
	}
	var out []engine.CardID
	for _, p := range pairs {
		var r Ref
		if len(p) == 0 || json.Unmarshal(p[0], &r) != nil {
			return nil, false
		}
		id, ok := c.goCard(r)
		if !ok || !slices.Contains(options, id) {
			return nil, false
		}
		out = append(out, id)
	}
	if len(out) > hi {
		return nil, false
	}
	for _, k := range c.s.calls {
		if !k.used && k.Decider.Seat == seat(decider) && k.Method == "chooseCardsForConvokeOrImprovise" && k.Seq <= last.Seq && k.when() == c.now() {
			k.used = true
		}
	}
	return out, true
}

// costCards answers a card choice that pays a cost of the play under way
// (Solitude's evoke exile) from that play's paid parts: Java's AI chooses
// cost cards without a controller call (AiCostDecision), and the play
// record names what it paid. A part whose cards Go does not offer, or whose
// count is outside lo..hi, is no answer.
func (c *Controller) costCards(source engine.CardID, options []engine.CardID, lo, hi int) ([]engine.CardID, bool) {
	if c.cur == nil || source == engine.NoCard {
		return nil, false
	}
	if host, ok := c.goCard(c.cur.Ability.Host); !ok || host != source {
		return nil, false
	}
	for i, part := range c.cur.Paid {
		if c.paidUsed[i] || len(part.Cards) < lo || len(part.Cards) > hi {
			continue
		}
		ids := make([]engine.CardID, 0, len(part.Cards))
		for _, r := range part.Cards {
			if id, ok := c.goCard(r); ok && slices.Contains(options, id) {
				ids = append(ids, id)
			}
		}
		if len(ids) != len(part.Cards) {
			continue
		}
		// Java records one payment under several part names (Exiled,
		// ExiledCards): every part naming the same cards is spent with it.
		for j, other := range c.cur.Paid {
			if slices.Equal(other.Cards, part.Cards) {
				c.paidUsed[j] = true
			}
		}
		return ids, true
	}
	return nil, false
}

// playCostCards is costCards for a question that names no source: a
// Discard<> or Sac<> cost part of the play under way (Psychic Frog's
// discard), whose cards Java's AI chose without a call.
func (c *Controller) playCostCards(options []engine.CardID, n int) ([]engine.CardID, bool) {
	if c.cur == nil {
		return nil, false
	}
	host, ok := c.goCard(c.cur.Ability.Host)
	if !ok {
		return nil, false
	}
	return c.costCards(host, options, n, n)
}

// ChoosePermanentsToSacrifice is a sacrifice cost's cards from the play
// under way, else Java's card choice.
func (c *Controller) ChoosePermanentsToSacrifice(g *engine.Game, decider engine.PlayerID, options []engine.CardID, n int) []engine.CardID {
	if ids, ok := c.playCostCards(options, n); ok {
		return ids
	}
	if ids, ok := c.chooseCards(decider, "choosePermanentsToSacrifice", options); ok {
		return ids
	}
	return c.Controller.ChoosePermanentsToSacrifice(g, decider, options, n)
}

// ChooseCardsToDiscard is a discard cost's cards from the play under way,
// else Java's card choice.
func (c *Controller) ChooseCardsToDiscard(g *engine.Game, decider engine.PlayerID, hand []engine.CardID, n int) []engine.CardID {
	if ids, ok := c.playCostCards(hand, n); ok {
		return ids
	}
	if ids, ok := c.chooseCards(decider, "chooseCardsToDiscard", hand); ok {
		return ids
	}
	return c.Controller.ChooseCardsToDiscard(g, decider, hand, n)
}

// DiscardToHandSize is Java's card choice.
func (c *Controller) DiscardToHandSize(g *engine.Game, decider engine.PlayerID, hand []engine.CardID, n int) []engine.CardID {
	if ids, ok := c.chooseCards(decider, "discardToHandSize", hand); ok {
		return ids
	}
	return c.Controller.DiscardToHandSize(g, decider, hand, n)
}

// ChooseEntitiesForEffect is Java's choice, cards or players.
func (c *Controller) ChooseEntitiesForEffect(g *engine.Game, decider engine.PlayerID, source engine.CardID, options []engine.EntityID, lo, hi int) []engine.EntityID {
	k := c.next(decider, cardChoices...)
	for k != nil {
		var refs []Ref
		if json.Unmarshal(k.Answer, &refs) != nil {
			var one Ref
			if json.Unmarshal(k.Answer, &one) == nil {
				refs = []Ref{one}
			}
		}
		var out []engine.EntityID
		for _, r := range refs {
			if e, ok := c.entity(r); ok && slices.Contains(options, e) {
				out = append(out, e)
			}
		}
		if len(out) == len(refs) {
			return out
		}
		// Java asks a single-entity choice of one option too (PlayEffect's
		// card to play), Go does not: such a record outside Go's options
		// was a question Go never asked.
		if k.Method == "chooseSingleEntityForEffect" && loneOffer(k) {
			k = c.next(decider, cardChoices...)
			continue
		}
		c.fail(k.Seq, k.Method, "answer %s is not among Go's options", k.Answer)
		break
	}
	if k == nil {
		c.fail(-1, "chooseEntitiesForEffect", "Java chose nothing here")
	}
	return c.Controller.ChooseEntitiesForEffect(g, decider, source, options, lo, hi)
}

// loneOffer reports whether k offered exactly one option (its first
// argument, a list).
func loneOffer(k *call) bool {
	var offered []json.RawMessage
	var options []json.RawMessage
	return json.Unmarshal(k.Offered, &offered) == nil && len(offered) > 0 &&
		json.Unmarshal(offered[0], &options) == nil && len(options) == 1
}

// readNested marks read the method calls nested in k: Java's AI answers
// chooseEntitiesForEffect by asking itself chooseSingleEntityForEffect once
// per pick (PlayerControllerAi.java:422-439), and the outer answer holds
// them all.
func (c *Controller) readNested(k *call, method string) {
	after := false
	for _, x := range c.s.calls {
		if x == k {
			after = true
			continue
		}
		if !after {
			continue
		}
		if x.Depth <= k.Depth {
			return
		}
		if x.Method == method && x.Decider == k.Decider {
			x.used = true
		}
	}
}

// OrderCardsForZone is Java's orderMoveToZoneList answer.
func (c *Controller) OrderCardsForZone(g *engine.Game, decider engine.PlayerID, cards []engine.CardID, dest engine.ZoneType) []engine.CardID {
	if k := c.next(decider, "orderMoveToZoneList"); k != nil {
		if ids, ok := c.cards(k.Answer); ok && len(ids) == len(cards) {
			return ids
		}
		c.fail(k.Seq, k.Method, "answer %s does not order Go's %d cards", k.Answer, len(cards))
	}
	return c.Controller.OrderCardsForZone(g, decider, cards, dest)
}

// ChooseNumber is Java's chooseNumber answer.
func (c *Controller) ChooseNumber(g *engine.Game, decider engine.PlayerID, source engine.CardID, lo, hi int) int {
	// A keyword's repeat count (Replicate, Multikicker) is
	// chooseNumberForKeywordCost.
	if k := c.next(decider, "chooseNumber", "chooseNumberForKeywordCost"); k != nil {
		var n int
		if json.Unmarshal(k.Answer, &n) == nil && n >= lo && n <= hi {
			return n
		}
		c.fail(k.Seq, k.Method, "answer %s outside %d..%d", k.Answer, lo, hi)
	} else {
		c.fail(-1, "chooseNumber", "Java chose no number here")
	}
	return c.Controller.ChooseNumber(g, decider, source, lo, hi)
}

// ConfirmOptionalTrigger is Java's confirmTrigger answer: an optional
// trigger's "you may" (OptionalDecider$) and a triggered ability's cost
// (Artist's Talent's Discard<1/Card>) both ask it as the trigger resolves.
func (c *Controller) ConfirmOptionalTrigger(g *engine.Game, decider engine.PlayerID, source engine.CardID) bool {
	if k := c.next(decider, "confirmTrigger"); k != nil {
		var yes bool
		if json.Unmarshal(k.Answer, &yes) == nil {
			return yes
		}
		c.fail(k.Seq, k.Method, "answer %s", k.Answer)
	} else {
		c.fail(-1, "confirmTrigger", "Java was not asked to confirm a trigger here")
	}
	return c.Controller.ConfirmOptionalTrigger(g, decider, source)
}

// ConfirmPayCost is Java's payCostToPreventEffect answer, or, for an
// optional cost of the card being cast, its chooseOptionalCosts answer.
func (c *Controller) ConfirmPayCost(g *engine.Game, decider engine.PlayerID, price cost.Cost, source engine.CardID) bool {
	if source != engine.NoCard && source == c.castCard {
		if !c.asked {
			c.asked, c.optional = true, nil
			if k := c.next(decider, "chooseOptionalCosts"); k != nil {
				c.optional = optionalAnswers(k)
			}
		}
		if len(c.optional) > 0 {
			yes := c.optional[0]
			c.optional = c.optional[1:]
			return yes
		}
	}
	if k := c.next(decider, "payCostToPreventEffect", "payCombatCost", "confirmPayment"); k != nil {
		var yes bool
		if json.Unmarshal(k.Answer, &yes) == nil {
			return yes
		}
	}
	c.fail(-1, "confirmPayCost", "Java was not asked to pay here")
	return c.Controller.ConfirmPayCost(g, decider, price, source)
}

// optionalAnswers is, for each optional cost a chooseOptionalCosts record
// offered (its second argument), whether its answer chose it.
func optionalAnswers(k *call) []bool {
	var offered []json.RawMessage
	var costs, chosen []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(k.Offered, &offered) != nil || len(offered) < 2 ||
		json.Unmarshal(offered[1], &costs) != nil || json.Unmarshal(k.Answer, &chosen) != nil {
		return nil
	}
	out := make([]bool, len(costs))
	for i, o := range costs {
		out[i] = slices.ContainsFunc(chosen, func(ch struct {
			Text string `json:"text"`
		}) bool {
			return ch.Text == o.Text
		})
	}
	return out
}

// ChooseReplacementEffect is Java's chooseSingleReplacementEffect answer.
//
// Java asks for every entry with several replacements; Go asks only where
// its choice matters (two "enters tapped" or two "enters untapped" lines
// agree, an entry counter is applied apart): a Java call is read only when
// it offered the hosts Go offers, and the calls of a question Go did not
// ask are passed over.
func (c *Controller) ChooseReplacementEffect(g *engine.Game, decider engine.PlayerID, options []engine.ReplacementOption) int {
	var hosts []engine.CardID
	for _, o := range options {
		hosts = append(hosts, o.Host)
	}
	slices.Sort(hosts)
	for {
		k := c.next(decider, "chooseSingleReplacementEffect")
		if k == nil {
			c.fail(-1, "chooseSingleReplacementEffect", "Java chose no replacement here")
			break
		}
		if !slices.Equal(c.replacementHosts(k), hosts) {
			continue
		}
		var a abilityID
		if json.Unmarshal(k.Answer, &a) == nil {
			if host, ok := c.goCard(a.Host); ok {
				for i, o := range options {
					if o.Host == host {
						return i
					}
				}
			}
		}
		c.fail(k.Seq, k.Method, "answer %s is not among Go's options", k.Answer)
		break
	}
	return c.Controller.ChooseReplacementEffect(g, decider, options)
}

// replacementHosts are the Go cards whose replacements a
// chooseSingleReplacementEffect call offered, sorted.
func (c *Controller) replacementHosts(k *call) []engine.CardID {
	var offered []json.RawMessage
	var list []abilityID
	if json.Unmarshal(k.Offered, &offered) != nil || len(offered) == 0 || json.Unmarshal(offered[0], &list) != nil {
		return nil
	}
	var hosts []engine.CardID
	for _, a := range list {
		id, _ := c.goCard(a.Host)
		hosts = append(hosts, id)
	}
	slices.Sort(hosts)
	return hosts
}

// ChooseModesForAbility is Java's chooseModeForAbility answer, by the
// modes' positions in what Java offered.
func (c *Controller) ChooseModesForAbility(g *engine.Game, decider engine.PlayerID, source engine.CardID, options []string, lo, hi int) []int {
	if k := c.next(decider, "chooseModeForAbility"); k != nil {
		var offered []json.RawMessage
		var answer []json.RawMessage
		if json.Unmarshal(k.Offered, &offered) == nil && len(offered) > 1 && json.Unmarshal(k.Answer, &answer) == nil {
			var modes []json.RawMessage
			_ = json.Unmarshal(offered[1], &modes)
			var out []int
			for _, a := range answer {
				for i, m := range modes {
					if string(m) == string(a) && i < len(options) {
						out = append(out, i)
						break
					}
				}
			}
			if len(out) == len(answer) {
				return out
			}
		}
		c.fail(k.Seq, k.Method, "answer %s", k.Answer)
	}
	return c.Controller.ChooseModesForAbility(g, decider, source, options, lo, hi)
}

// isSpellAPI reports whether id's printed spell has api.
func (c *Controller) isSpellAPI(id engine.CardID, api string) bool {
	def := c.g.Card(id).Def
	if def == nil || len(def.Faces) == 0 {
		return false
	}
	for _, ab := range def.Faces[0].Abilities {
		if ab.Record == compile.Spell && ab.Name == api {
			return true
		}
	}
	return false
}

// ChooseOption picks the cast option of the way Java cast the current play
// (castoptions.go's labels); any other option question is not read from
// the log yet.
func (c *Controller) ChooseOption(g *engine.Game, decider engine.PlayerID, source engine.CardID, options []string) int {
	if c.castCard != engine.NoCard && source == c.castCard {
		want := "Cast " + g.Card(source).Def.Name
		for i, o := range options {
			if c.castWay == "" && o == want || c.castWay != "" &&
				strings.Contains(o, " for its "+strings.ToLower(c.castWay)+" cost ") {
				c.castCard = engine.NoCard
				return i
			}
		}
		c.fail(-1, "chooseCastOption", "no Go cast option %q for way %q among %q", want, c.castWay, options)
	}
	// Java's chooseCardName and chooseSomeType answer with the name or type
	// itself (Pithing Needle's named card).
	if k := c.next(decider, "chooseCardName", "chooseSomeType"); k != nil {
		var name string
		if json.Unmarshal(k.Answer, &name) == nil {
			if i := slices.Index(options, name); i >= 0 {
				return i
			}
		}
		c.fail(k.Seq, k.Method, "answer %s is not among Go's %d options", k.Answer, len(options))
	}
	return c.Controller.ChooseOption(g, decider, source, options)
}

// ChooseManaColor is Java's chooseColor answer, a MagicColor bit, the same
// bit Go's Colors uses.
func (c *Controller) ChooseManaColor(g *engine.Game, decider engine.PlayerID, source engine.CardID, options mana.Colors) mana.Colors {
	if k := c.next(decider, "chooseColor", "chooseColorAllowColorless"); k != nil {
		var bit int
		if json.Unmarshal(k.Answer, &bit) == nil && options.Has(mana.Colors(bit)) {
			return mana.Colors(bit)
		}
		c.fail(k.Seq, k.Method, "answer %s is not one of %v", k.Answer, options)
	} else {
		c.fail(-1, "chooseColor", "no answer for seat %d", seat(decider))
	}
	return c.Controller.ChooseManaColor(g, decider, source, options)
}

// ChooseHybridManaColor pays a two-colour hybrid symbol with whichever of
// its colours the pool holds more of beyond the cost's one-colour symbols
// and the hybrid symbols paid so far: Java records no such choice, it pays
// from the mana its payment record made (Kaheera's {G/W} with Plains mana).
func (c *Controller) ChooseHybridManaColor(g *engine.Game, decider engine.PlayerID, options mana.Colors) mana.Colors {
	colors := [...]mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green}
	shards := [...]mana.Shard{mana.ShardW, mana.ShardU, mana.ShardB, mana.ShardR, mana.ShardG}
	need := [5]int{}
	for _, s := range c.paying.Shards() {
		for i, sh := range shards {
			if s == sh {
				need[i]++
			}
		}
	}
	have := g.UsableMana(decider)
	best, spare := -1, 0
	for i, col := range colors {
		if options&col == 0 {
			continue
		}
		if left := have[i] - need[i] - c.hybrid[i]; best < 0 || left > spare {
			best, spare = i, left
		}
	}
	if best < 0 {
		return c.Controller.ChooseHybridManaColor(g, decider, options)
	}
	c.hybrid[best]++
	return colors[best]
}

// ChoosePayPhyrexian pays a Phyrexian symbol with mana when the pool holds
// that colour beyond the cost's one-colour symbols and the hybrid and
// Phyrexian symbols paid so far, else with life: Java records the mana it
// paid with, not which symbol took life (Dismember paid with one {B} from
// Talisman of Resilience and 2 life).
func (c *Controller) ChoosePayPhyrexian(g *engine.Game, decider engine.PlayerID, color mana.Colors) bool {
	colors := [...]mana.Colors{mana.White, mana.Blue, mana.Black, mana.Red, mana.Green}
	shards := [...]mana.Shard{mana.ShardW, mana.ShardU, mana.ShardB, mana.ShardR, mana.ShardG}
	i := slices.Index(colors[:], color)
	if i < 0 {
		return c.Controller.ChoosePayPhyrexian(g, decider, color)
	}
	need := 0
	for _, s := range c.paying.Shards() {
		if s == shards[i] {
			need++
		}
	}
	if g.UsableMana(decider)[i]-need-c.hybrid[i] <= 0 {
		return false
	}
	c.hybrid[i]++
	return true
}

// ChoosePayGeneric pays a generic unit with mana the cost's colored and {C}
// pips do not need, colorless first. Java's log names the mana paid, not
// which pip each paid; any choice that leaves the pips payable is Java's
// payment.
func (c *Controller) ChoosePayGeneric(g *engine.Game, decider engine.PlayerID) mana.Shard {
	shards := [...]mana.Shard{mana.ShardW, mana.ShardU, mana.ShardB, mana.ShardR, mana.ShardG, mana.ShardC}
	need := [6]int{}
	for _, s := range c.paying.Shards() {
		for i, sh := range shards {
			if s == sh {
				need[i]++
			}
		}
	}
	have := g.UsableMana(decider)
	for _, i := range []int{5, 0, 1, 2, 3, 4} {
		if have[i]-need[i]-c.hybrid[i]-c.generic[i] > 0 {
			c.generic[i]++
			return shards[i]
		}
	}
	return c.Controller.ChoosePayGeneric(g, decider)
}
