package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// offerRecorder is a ScriptedController that also records every card pool
// ChooseCardsForEffect is offered, so a test can assert on the pool's own
// contents and order rather than only on the pick.
type offerRecorder struct {
	*engine.ScriptedController
	offers [][]engine.CardID
}

func (r *offerRecorder) ChooseCardsForEffect(g *engine.Game, p engine.PlayerID, src engine.CardID, choices []engine.CardID, lo, hi int) []engine.CardID {
	r.offers = append(r.offers, append([]engine.CardID(nil), choices...))
	return r.ScriptedController.ChooseCardsForEffect(g, p, src, choices, lo, hi)
}

// firstPicker is a ScriptedController that answers every
// ChooseCardsForEffect with the first lo cards offered and records them --
// for a pick among cards the test cannot name before the effect creates
// them.
type firstPicker struct {
	*engine.ScriptedController
	picked []engine.CardID
}

func (f *firstPicker) ChooseCardsForEffect(_ *engine.Game, _ engine.PlayerID, _ engine.CardID, choices []engine.CardID, lo, _ int) []engine.CardID {
	f.picked = append(f.picked, choices[:lo]...)
	return choices[:lo]
}

// pushStackAbility puts a harmless DB$ BlankLine ability for source on the
// stack, targeting targets -- a spell or ability waiting under whatever
// resolves next.
func pushStackAbility(t *testing.T, g *engine.Game, p engine.PlayerID, source engine.CardID, targets ...engine.EntityID) {
	t.Helper()
	def := etbChainDef(t, "Test Stack Item", "DB$ BlankLine")
	for _, sub := range def.Faces[0].Triggers[0].Subs {
		if strings.EqualFold(sub.Key, "Execute") {
			g.PushAbility(engine.Ability{API: engine.APIBlankLine, Source: source, Controller: p, Params: sub.Ability, Targets: targets})
			return
		}
	}
	t.Fatal("no Execute$")
}

// TestChooseSourceFiltersPermanentsByColorSource proves the dominant
// color-restricted shape (Choices$ Card.RedSource, circle_of_protection_
// red.txt): only red permanents are offered, the pick becomes the host's
// chosen card, and RememberChosen$ remembers it too.
func TestChooseSourceFiltersPermanentsByColorSource(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	red := g.NewCard(creatureDefManaCost(t, "1 R"), other, engine.Battlefield)
	g.NewCard(creatureDefManaCost(t, "1 B"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{red})

	host, err := resolveNow(t, g, p, c, nil, "DB$ ChooseSource | Choices$ Card.RedSource | RememberChosen$ True")
	if err != nil {
		t.Fatalf("ChooseSource: %v", err)
	}
	if got := g.Card(host).Memory.Chosen(); !slices.Equal(got, []engine.CardID{red}) {
		t.Errorf("chosen = %v, want [%d]", got, red)
	}
	if got := g.Card(host).Memory.Remembered(); !slices.Equal(got, []engine.EntityID{engine.CardEntity(red)}) {
		t.Errorf("remembered = %v, want the red creature", got)
	}
}

// TestChooseSourceRejectsPickOutsideChoices proves Choices$ bounds the
// pool: a black creature is not a red source, so picking it fails the
// resolution rather than being accepted.
func TestChooseSourceRejectsPickOutsideChoices(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(creatureDefManaCost(t, "1 R"), other, engine.Battlefield)
	black := g.NewCard(creatureDefManaCost(t, "1 B"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{black})

	if _, err := resolveNow(t, g, p, c, nil, "DB$ ChooseSource | Choices$ Card.RedSource"); err == nil {
		t.Error("picking a black creature for Choices$ Card.RedSource resolved, want an error")
	}
}

// TestChooseSourcePoolOrderAndStackSources proves ChooseSourceEffect.java's
// group order and its set semantics: permanents first, then the source of
// each spell or ability on the stack, then the cards those stack items
// target, each card listed once. The waiting stack item's source is an
// instant on the stack; it targets a graveyard card (a referenced object)
// and a permanent (already listed among the permanents).
func TestChooseSourcePoolOrderAndStackSources(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	bear := g.NewCard(creatureDefManaCost(t, "1 G"), other, engine.Battlefield)
	spell := g.NewCard(nonPermanentDef(t, "Test Instant"), other, engine.Stack)
	buried := g.NewCard(creatureDefManaCost(t, "1 B"), other, engine.Graveyard)
	pushStackAbility(t, g, other, spell, engine.CardEntity(buried), engine.CardEntity(bear))

	host := g.NewCard(etbChainDef(t, "Test Chooser", "DB$ BlankLine"), p, engine.Battlefield)
	def := etbChainDef(t, "Test Chooser", "DB$ ChooseSource | Choices$ Card,Emblem")
	var params *compile.Ability
	for _, sub := range def.Faces[0].Triggers[0].Subs {
		if strings.EqualFold(sub.Key, "Execute") {
			params = sub.Ability
		}
	}
	g.PushAbility(engine.Ability{API: engine.APIChooseSource, Source: host, Controller: p, Params: params})

	c := &offerRecorder{ScriptedController: engine.NewScriptedController()}
	c.QueueCardChoice([]engine.CardID{spell})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}

	want := []engine.CardID{host, bear, spell, buried}
	if len(c.offers) != 1 || !slices.Equal(c.offers[0], want) {
		t.Errorf("offered %v, want %v", c.offers, want)
	}
	if got := g.Card(host).Memory.Chosen(); !slices.Equal(got, []engine.CardID{spell}) {
		t.Errorf("chosen = %v, want the instant on the stack", got)
	}
}

// TestChooseSourceEmptyPoolIsANoOp proves Java's early return: nothing
// matches Choices$, so no one is asked and nothing is chosen.
func TestChooseSourceEmptyPoolIsANoOp(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	host, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ ChooseSource | Choices$ Card.WhiteSource")
	if err != nil {
		t.Fatalf("ChooseSource: %v", err)
	}
	if got := g.Card(host).Memory.Chosen(); len(got) != 0 {
		t.Errorf("chosen = %v, want nothing", got)
	}
}

// TestChooseSourceRejectsUnresolvableShapes proves every shape this port
// cannot evaluate fails loudly: Amount$/TargetControls$ (0 real lines) and
// the Choices$ properties Matches has no evaluator for, which would
// otherwise read false for every card and silently empty the pool.
func TestChooseSourceRejectsUnresolvableShapes(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"DB$ ChooseSource | Choices$ Card | Amount$ 2",
		"DB$ ChooseSource | Choices$ Card | TargetControls$ True",
		"DB$ ChooseSource | Choices$ Card.ChosenColorSource",
		"DB$ ChooseSource | Choices$ Card.SharesColorWith Imprinted",
	} {
		g, p, _ := newTwoPlayerGame(t)
		_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, line)
		if err == nil || !strings.Contains(err.Error(), "not resolvable yet") {
			t.Errorf("%q: err = %v, want a not resolvable yet error", line, err)
		}
	}
}

// ghostlyFlameDef is ghostly_flame.txt: black and red permanents and spells
// are colorless sources of damage.
func ghostlyFlameDef(t *testing.T) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: "ghostly_flame"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Ghostly Flame"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Statics = []string{"Mode$ ColorlessDamageSource | ValidCard$ Permanent.Black+inZoneBattlefield,Permanent.Red+inZoneBattlefield,Spell.Black+inZoneStack,Spell.Red+inZoneStack"}
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile ghostly_flame: %v", err)
	}
	return c
}

// TestMatchesColorSourceReadsColorUnlessColorlessDamageSource proves
// CardStateProperty's withSource color form: RedSource matches a red card
// and not a white one, and a ColorlessDamageSource static (Ghostly Flame)
// turns the red card into a colorless source -- no longer RedSource, now
// ColorlessSource -- while the bare Red property still reads its color.
func TestMatchesColorSourceReadsColorUnlessColorlessDamageSource(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	red := g.Card(g.NewCard(creatureDefManaCost(t, "1 R"), p, engine.Battlefield))
	white := g.Card(g.NewCard(creatureDefManaCost(t, "1 W"), p, engine.Battlefield))
	matches := func(c *engine.Card, spec string) bool {
		return engine.Matches(g, c, valid.Parse(spec), p, engine.NoCard)
	}

	if !matches(red, "Card.RedSource") || matches(white, "Card.RedSource") {
		t.Error("RedSource: want the red creature only")
	}
	if !matches(white, "Card.nonRedSource") || matches(red, "Card.ColorlessSource") {
		t.Error("nonRedSource/ColorlessSource misread a colored source")
	}

	g.NewCard(ghostlyFlameDef(t), p, engine.Battlefield)
	if matches(red, "Card.RedSource") || !matches(red, "Card.ColorlessSource") {
		t.Error("under Ghostly Flame the red creature should be a colorless source")
	}
	if !matches(red, "Card.Red") {
		t.Error("under Ghostly Flame the red creature should still be Red")
	}
	if !matches(white, "Card.WhiteSource") {
		t.Error("Ghostly Flame should not touch a white source")
	}
}

// TestEmpowerCreatesTheTokenThenAddsLoyalty proves the corpus's own shape
// (DB$ Empower | Type$ Jace | Num$ 2, all 32 lines Type$ Jace): with no Jace
// token yet, one is created from u_empower -- a Planeswalker Jace named
// "Jace Token" -- and gets the loyalty counters before state-based actions
// would put a 0-loyalty planeswalker into the graveyard.
func TestEmpowerCreatesTheTokenThenAddsLoyalty(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	f := &firstPicker{ScriptedController: engine.NewScriptedController()}
	tokensBefore := len(g.Zone(engine.Battlefield, p).Cards())
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbChainDef(t, "Test Empower", "DB$ Empower | Type$ Jace | Num$ 2"), p, engine.Hand)
	if !g.CastSpell(p, host, f) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), f); err != nil {
		t.Fatalf("Empower: %v", err)
	}

	jaces := tokensOn(g, p, "Jace Token")
	if len(jaces) != 1 || !slices.Equal(f.picked, jaces) {
		t.Fatalf("Jace tokens = %v, picked %v, want the one token chosen", jaces, f.picked)
	}
	tok := g.Card(jaces[0])
	if !tok.Type().Has(cardtype.Planeswalker) || !tok.Type().HasSubtype("Jace") {
		t.Errorf("type = %v, want Planeswalker Jace", tok.Type())
	}
	if got := tok.Counters.Count(engine.Loyalty); got != 2 {
		t.Errorf("loyalty = %d, want 2", got)
	}
	if got := len(g.Zone(engine.Battlefield, p).Cards()); got != tokensBefore+2 {
		t.Errorf("battlefield grew by %d, want 2 (the host and one token)", got-tokensBefore)
	}
}

// TestEmpowerReusesAnExistingToken proves the second half on its own: a
// Jace token already there means no new token, and the counters land on the
// one the player chooses among several.
func TestEmpowerReusesAnExistingToken(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	first := g.NewCard(tokenDefT(t, "Jace Token", "Planeswalker Jace", "", ""), p, engine.Battlefield)
	second := g.NewCard(tokenDefT(t, "Jace Token", "Planeswalker Jace", "", ""), p, engine.Battlefield)
	for _, id := range []engine.CardID{first, second} {
		g.Card(id).IsToken = true
		g.Card(id).Counters.Add(engine.Loyalty, 1)
	}
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{second})
	resolveLine(t, g, p, c, "DB$ Empower | Type$ Jace | Num$ 5")

	if n := len(tokensOn(g, p, "Jace Token")); n != 2 {
		t.Errorf("Jace tokens = %d, want 2 (no new one)", n)
	}
	if got := g.Card(second).Counters.Count(engine.Loyalty); got != 6 {
		t.Errorf("chosen token loyalty = %d, want 6", got)
	}
	if got := g.Card(first).Counters.Count(engine.Loyalty); got != 1 {
		t.Errorf("other token loyalty = %d, want 1", got)
	}
}

// TestChooseSourceRefusesSpellUnderColorlessDamageSource proves the gap
// fails loudly: Ghostly Flame makes a red spell on the stack a colorless
// source, a clause (Spell.Red+inZoneStack) Matches cannot evaluate yet, so
// a color-Source Choices$ with a spell on the stack is an error rather than
// offering the spell as red.
func TestChooseSourceRefusesSpellUnderColorlessDamageSource(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(ghostlyFlameDef(t), other, engine.Battlefield)
	spell := g.NewCard(nonPermanentDef(t, "Test Instant"), other, engine.Stack)
	pushStackAbility(t, g, other, spell)

	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ ChooseSource | Choices$ Card.RedSource")
	if err == nil || !strings.Contains(err.Error(), "not resolvable yet") {
		t.Errorf("err = %v, want a not resolvable yet error", err)
	}
}

// TestChooseSourceFailsWhenThePoolRunsDry proves the Forge hang is an
// error instead: two choosers share a one-card pool, and Java's do/while
// would reject the dividers forever once the first took it.
func TestChooseSourceFailsWhenThePoolRunsDry(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	c := &firstPicker{ScriptedController: engine.NewScriptedController()}
	host := g.NewCard(etbChainDef(t, "Test Chooser", "DB$ BlankLine"), p, engine.Battlefield)
	def := etbChainDef(t, "Test Chooser", "DB$ ChooseSource | Defined$ Player | Choices$ Card.Self")
	for _, sub := range def.Faces[0].Triggers[0].Subs {
		if strings.EqualFold(sub.Key, "Execute") {
			g.PushAbility(engine.Ability{API: engine.APIChooseSource, Source: host, Controller: p, Params: sub.Ability})
		}
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err == nil {
		t.Error("a second chooser facing an empty pool resolved, want an error")
	}
	if len(c.picked) != 1 {
		t.Errorf("picked %v, want the first chooser's one pick", c.picked)
	}
}
