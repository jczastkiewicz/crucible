package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// bendWatcherDef is a battlefield card carrying one T: line whose Execute$ is
// Trig, a GainLife of gain for its controller.
func bendWatcherDef(t *testing.T, trigger string, gain string) *compile.Card {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n"))
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	raw := &carddb.Card{Filename: "watcher"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Watcher"
	raw.Faces[0].Type = cardtype.Parse(reg, "Creature Elf")
	raw.Faces[0].Power, raw.Faces[0].Toughness = "1", "1"
	raw.Faces[0].Triggers = []string{trigger + " | Execute$ Trig"}
	raw.Faces[0].SVars.Set("Trig", "DB$ GainLife | Defined$ You | LifeAmount$ "+gain)
	c, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile watcher: %v", err)
	}
	return c
}

func newTwoPlayerGameOn(t *testing.T, db *compile.DB) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	g := engine.NewGame(db, javarand.New(1), []string{"a", "b"})
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	return g, p, other
}

func flowCardDef(t *testing.T, name, typeLine, cost string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: name}
	def.Faces[0].Name = name
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), typeLine)
	if cost != "" {
		def.Faces[0].ManaCost = mana.MustParse(cost)
	}
	if strings.Contains(typeLine, "Creature") {
		def.Faces[0].Power, def.Faces[0].Toughness = "1", "1"
	}
	return def
}

// TestEarthbendAnimatesAndReturnsTheLand proves CR 701.66: the chosen land
// becomes a 0/0 haste creature with Num$ +1/+1 counters, and each of its two
// delayed triggers brings it back tapped -- once after it dies, once after it
// is exiled. The ElementalBend trigger fires for the bender.
func TestEarthbendAnimatesAndReturnsTheLand(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	land := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	g.NewCard(bendWatcherDef(t, "Mode$ ElementalBend | ValidPlayer$ You | TriggerZones$ Battlefield", "3"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(land)})
	resolveLine(t, g, p, c, "DB$ Earthbend | Num$ 2")

	l := g.Card(land)
	if !l.Type().Has(cardtype.Creature) || !l.Type().Has(cardtype.Land) || !l.HasKeyword("Haste") {
		t.Fatalf("land type %v haste %v, want a hasty land creature", l.Type(), l.HasKeyword("Haste"))
	}
	if pw, _ := l.Power(); pw != 2 || l.Counters.Count(engine.P1P1) != 2 {
		t.Errorf("power %d counters %d, want 2 and 2", pw, l.Counters.Count(engine.P1P1))
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23 (ElementalBend trigger)", got)
	}

	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(land)},
		"DB$ Destroy | ValidTgts$ Land"); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if l.Zone != engine.Battlefield || !l.Tapped || l.Controller() != p || l.Type().Has(cardtype.Creature) {
		t.Fatalf("after dying: zone %v tapped %v creature %v, want back tapped as a plain land", l.Zone, l.Tapped, l.Type().Has(cardtype.Creature))
	}

	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), []engine.EntityID{engine.CardEntity(land)},
		"DB$ Airbend | ValidTgts$ Land"); err != nil {
		t.Fatalf("airbend: %v", err)
	}
	if l.Zone != engine.Battlefield || !l.Tapped {
		t.Errorf("after exile: zone %v tapped %v, want back on the battlefield tapped", l.Zone, l.Tapped)
	}
	if _, ok := g.MayPlayFromExile(p, land); ok {
		t.Error("a land kept an Airbend cast permission")
	}
}

// TestAirbendExilesWithACastForTwoPermission proves CR 701.65: the target is
// exiled, its owner may cast it for {2} while it stays there, a token gets
// no permission, and the permission ends when the card leaves exile.
func TestAirbendExilesWithACastForTwoPermission(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Battlefield)
	tok := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	g.Card(tok).IsToken = true
	g.NewCard(bendWatcherDef(t, "Mode$ Airbend | ValidPlayer$ You | TriggerZones$ Battlefield", "1"), p, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(),
		[]engine.EntityID{engine.CardEntity(victim), engine.CardEntity(tok)},
		"DB$ Airbend | ValidTgts$ Creature | TargetMax$ 2"); err != nil {
		t.Fatalf("airbend: %v", err)
	}
	if g.Card(victim).Zone != engine.Exile {
		t.Fatalf("victim zone = %v, want Exile", g.Card(victim).Zone)
	}
	grant, ok := g.MayPlayFromExile(other, victim)
	if !ok || !grant.HasAltCost || grant.AltGeneric != 2 {
		t.Errorf("grant = %+v %v, want its owner casting it for {2}", grant, ok)
	}
	if _, ok := g.MayPlayFromExile(p, victim); ok {
		t.Error("the airbender, not the owner, got the permission")
	}
	if _, ok := g.MayPlayFromExile(other, tok); ok {
		t.Error("a token got a cast permission")
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21 (Airbend trigger)", got)
	}
	g.Move(victim, engine.Hand, other)
	if _, ok := g.MayPlayFromExile(other, victim); ok {
		t.Error("permission outlived the card leaving exile")
	}
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ Airbend | ValidTgts$ Card | TgtZone$ Stack"); err == nil || !strings.Contains(err.Error(), "TgtZone$ not resolvable yet") {
		t.Errorf("TgtZone$ err = %v, want not resolvable yet", err)
	}
}

// TestEmpowerCreatesThenGrowsTheToken proves the first Empower creates the
// "<Type> Token" planeswalker from u_empower and the second grows it.
func TestEmpowerCreatesThenGrowsTheToken(t *testing.T) {
	t.Parallel()

	pw := flowCardDef(t, "Token", "Planeswalker", "")
	pw.Faces[0].Loyalty = "0"
	byName := map[string]*compile.Card{}
	db := compile.NewDB(byName).WithTokens(map[string]*compile.Card{"u_empower": pw}).WithTypes(attachmentTypeRegistry(t))
	g, p, _ := newTwoPlayerGameOn(t, db)
	f := &firstPicker{ScriptedController: engine.NewScriptedController()}
	g.Player(p).ManaPool.Add(mana.Green, 1)
	host := g.NewCard(etbChainDef(t, "Test Empower", "DB$ Empower | Type$ Jace | Num$ 2 | SubAbility$ DBMore",
		"DBMore", "DB$ Empower | Type$ Jace | Num$ 3"), p, engine.Hand)
	if !g.CastSpell(p, host, f) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), f); err != nil {
		t.Fatalf("Empower: %v", err)
	}
	var jaces []engine.CardID
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if c := g.Card(id); c.IsToken && c.Def.Name == "Jace Token" && c.Type().HasSubtype("Jace") {
			jaces = append(jaces, id)
		}
	}
	if len(jaces) != 1 {
		t.Fatalf("Jace tokens = %d, want 1", len(jaces))
	}
	if n := g.Card(jaces[0]).Counters.Count(engine.Loyalty); n != 5 {
		t.Errorf("loyalty = %d, want 5", n)
	}
}

// TestDiscoverPutsTheHitInHandAndTheRestOnTheBottom proves CR 701.57's
// reveal loop: lands and cards over Num$ are passed, the first hit goes to
// hand when not cast, the rest go under the library, and the Discover
// trigger fires.
func TestDiscoverPutsTheHitInHandAndTheRestOnTheBottom(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	land := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Library)
	big := g.NewCard(creatureDefManaCost(t, "4 G"), p, engine.Library)
	hit := g.NewCard(creatureDefManaCost(t, "1 G"), p, engine.Library)
	stay := g.NewCard(creatureDefManaCost(t, "G"), p, engine.Library)
	g.NewCard(bendWatcherDef(t, "Mode$ Discover | ValidPlayer$ You | TriggerZones$ Battlefield", "2"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	host := resolveLine(t, g, p, c, "DB$ Discover | Num$ 3 | RememberDiscovered$ True")

	if g.Card(hit).Zone != engine.Hand {
		t.Errorf("hit zone = %v, want Hand", g.Card(hit).Zone)
	}
	lib := g.Zone(engine.Library, p).Cards()
	if len(lib) != 3 || lib[0] != stay {
		t.Fatalf("library = %v, want %d on top then the two passed cards", lib, stay)
	}
	if g.Card(land).Zone != engine.Library || g.Card(big).Zone != engine.Library {
		t.Error("passed cards did not go back to the library")
	}
	if r := g.Card(host).Memory.Remembered(); len(r) != 1 || r[0] != engine.CardEntity(hit) {
		t.Errorf("remembered = %v, want the hit", r)
	}
	if got := g.Player(p).Life; got != 22 {
		t.Errorf("life = %d, want 22 (Discover trigger)", got)
	}
}

// TestDiscoverCastsAPermanentWithoutPaying proves the cast branch: the hit
// goes to the stack with no mana paid and resolves onto the battlefield;
// choosing to cast a sorcery fails closed before anything moves.
func TestDiscoverCastsAPermanentWithoutPaying(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	hit := g.NewCard(creatureDefManaCost(t, "2 G"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(true)
	resolveLine(t, g, p, c, "DB$ Discover | Num$ 3")
	if g.Card(hit).Zone != engine.Battlefield {
		t.Errorf("hit zone = %v, want Battlefield", g.Card(hit).Zone)
	}
	if n := g.Player(p).SpellsCastThisTurn; n != 2 {
		t.Errorf("spells cast = %d, want 2 (host and the discovered card)", n)
	}

	g2, p2, _ := newTwoPlayerGame(t)
	sorcery := g2.NewCard(flowCardDef(t, "Sorcery", "Sorcery", "1 R"), p2, engine.Library)
	c2 := engine.NewScriptedController()
	c2.QueueConfirmEffect(true)
	_, err := resolveNow(t, g2, p2, c2, nil, "DB$ Discover | Num$ 2")
	if err == nil || !strings.Contains(err.Error(), "not resolvable yet") {
		t.Errorf("casting a sorcery err = %v, want not resolvable yet", err)
	}
	if g2.Card(sorcery).Zone != engine.Library {
		t.Errorf("sorcery zone = %v, want untouched in Library", g2.Card(sorcery).Zone)
	}
}

// TestDraftTakesOneOfThreeFromTheSpellbook proves the three offered cards
// are made from the spellbook (the "A-" rebalanced version when the DB has
// one), the pick goes to hand and is remembered, and the others stay out of
// the game.
func TestDraftTakesOneOfThreeFromTheSpellbook(t *testing.T) {
	t.Parallel()

	cards := []*compile.Card{
		namedCreature(t, "Bear", "1 G"), namedCreature(t, "A-Bear", "G"),
		namedCreature(t, "Elk", "1 G"), namedCreature(t, "Owl", "1 U"),
	}
	g, p, _ := newPackGame(t, cards...)
	first := engine.CardID(g.NumCards() + 2) // the host is made first
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{first})
	host := resolveLine(t, g, p, c, "DB$ Draft | Spellbook$ Bear,Elk,Owl | RememberDrafted$ True")

	if g.Card(first).Zone != engine.Hand {
		t.Errorf("pick zone = %v, want Hand", g.Card(first).Zone)
	}
	names := map[string]bool{g.Card(first).Def.Name: true}
	for _, id := range g.Zone(engine.None, p).Cards() {
		names[g.Card(id).Def.Name] = true
	}
	if len(names) != 3 || !names["A-Bear"] || names["Bear"] || !names["Elk"] || !names["Owl"] {
		t.Errorf("offered %v, want A-Bear, Elk and Owl", names)
	}
	if r := g.Card(host).Memory.Remembered(); len(r) != 1 || r[0] != engine.CardEntity(first) {
		t.Errorf("remembered = %v, want the pick", r)
	}
}

// TestHeistExilesFaceDownWithAPlayPermission proves the heister takes a
// nonland card of the target's library into exile face down, may play it,
// and the card turns face up and loses the permission once it leaves exile.
func TestHeistExilesFaceDownWithAPlayPermission(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(landDef(t, "Forest", "Basic Land Forest"), other, engine.Library)
	a := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Library)
	g.NewCard(landDef(t, "Island", "Basic Land Island"), other, engine.Library)
	b := g.NewCard(creatureDefPT(t, "3", "3"), other, engine.Library)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{a})
	c.QueueCardChoice([]engine.CardID{b})
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.PlayerEntity(other)},
		"DB$ Heist | ValidTgts$ Opponent | Num$ 2"); err != nil {
		t.Fatalf("heist: %v", err)
	}
	for _, id := range []engine.CardID{a, b} {
		card := g.Card(id)
		if card.Zone != engine.Exile || !card.IsFaceDown() {
			t.Errorf("card %d zone %v face down %v, want face down in exile", id, card.Zone, card.IsFaceDown())
		}
		if gr, ok := g.MayPlayFromExile(p, id); !ok || !gr.AnyManaType {
			t.Errorf("card %d grant = %+v %v, want the heister's any-mana permission", id, gr, ok)
		}
	}
	g.Move(a, engine.Graveyard, other)
	if g.Card(a).IsFaceDown() || g.Card(a).Def.Name != "Test Creature" {
		t.Error("heisted card did not turn face up leaving exile")
	}
	if _, ok := g.MayPlayFromExile(p, a); ok {
		t.Error("permission outlived the card leaving exile")
	}
}

// TestExchangeZoneSwapsHostAndHandCard proves the host on the battlefield
// trades places with the chosen card from hand; Type$ fails closed.
func TestExchangeZoneSwapsHostAndHandCard(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	inHand := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Hand)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{inHand})
	host := resolveLine(t, g, p, c, "DB$ ExchangeZone | ValidExchange$ Creature | Mandatory$ True")
	if g.Card(host).Zone != engine.Hand || g.Card(inHand).Zone != engine.Battlefield {
		t.Errorf("host %v, hand card %v, want Hand and Battlefield", g.Card(host).Zone, g.Card(inHand).Zone)
	}
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ ExchangeZone | Type$ Aura")
	if err == nil || !strings.Contains(err.Error(), "Type$ not resolvable yet") {
		t.Errorf("Type$ err = %v, want not resolvable yet", err)
	}
}

// TestEarthbendIgnoresATargetNoLongerALandYouControl proves CR 608.2b: a
// target that changed zones or controllers by resolution does nothing.
func TestEarthbendIgnoresATargetNoLongerALandYouControl(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	notYours := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), other, engine.Battlefield)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(),
		[]engine.EntityID{engine.CardEntity(notYours)}, "DB$ Earthbend"); err != nil {
		t.Fatalf("earthbend: %v", err)
	}
	if g.Card(notYours).Type().Has(cardtype.Creature) {
		t.Error("earthbend animated an opponent's land")
	}
}

// TestEmpowerAsksAChoiceAmongSeveralTokens proves the second Empower call,
// with more than one matching token already out, asks ChooseCardsForEffect
// rather than assuming the first.
func TestEmpowerAsksAChoiceAmongSeveralTokens(t *testing.T) {
	t.Parallel()

	pw := flowCardDef(t, "Token", "Planeswalker", "")
	byName := map[string]*compile.Card{}
	db := compile.NewDB(byName).WithTokens(map[string]*compile.Card{"u_empower": pw}).WithTypes(attachmentTypeRegistry(t))
	g, p, _ := newTwoPlayerGameOn(t, db)
	jace1 := g.NewCard(flowCardDef(t, "Jace Token", "Planeswalker Jace", ""), p, engine.Battlefield)
	g.Card(jace1).IsToken = true
	g.Card(jace1).Counters.Add(engine.Loyalty, 3)
	jace2 := g.NewCard(flowCardDef(t, "Jace Token", "Planeswalker Jace", ""), p, engine.Battlefield)
	g.Card(jace2).IsToken = true
	g.Card(jace2).Counters.Add(engine.Loyalty, 3)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{jace2})
	resolveLine(t, g, p, c, "DB$ Empower | Type$ Jace | Num$ 1")
	if n := g.Card(jace2).Counters.Count(engine.Loyalty); n != 4 {
		t.Errorf("jace2 loyalty = %d, want 4 (3 plus the chosen Empower)", n)
	}
	if n := g.Card(jace1).Counters.Count(engine.Loyalty); n != 3 {
		t.Errorf("jace1 loyalty = %d, want 3 (untouched, not chosen)", n)
	}
}

// TestDraftRejectsATooShortSpellbook and TestDraftDefaultsToYou prove the
// Spellbook$ length check and the Defined$-absent default.
func TestDraftRejectsATooShortSpellbook(t *testing.T) {
	t.Parallel()
	g, p, _ := newPackGame(t, namedCreature(t, "Bear", "1 G"), namedCreature(t, "Elk", "1 G"))
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Draft | Spellbook$ Bear,Elk")
	if err == nil || !strings.Contains(err.Error(), "need 3") {
		t.Errorf("err = %v, want a Spellbook$ too-short error", err)
	}
}

func TestDraftDefaultsToYou(t *testing.T) {
	t.Parallel()
	cards := []*compile.Card{namedCreature(t, "Bear", "1 G"), namedCreature(t, "Elk", "1 G"), namedCreature(t, "Owl", "1 U")}
	g, p, _ := newPackGame(t, cards...)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{engine.CardID(g.NumCards() + 2)})
	host := resolveLine(t, g, p, c, "DB$ Draft | Spellbook$ Bear,Elk,Owl")
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("host zone = %v, want Battlefield (unaffected)", z)
	}
}

// TestHeistSkipsAnEmptyLibrary proves an empty (or all-land) library round
// is skipped rather than erroring, and no targeted player means nothing
// happens.
func TestHeistSkipsAnEmptyLibrary(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.NewCard(landDef(t, "Forest", "Basic Land Forest"), other, engine.Library)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(),
		[]engine.EntityID{engine.PlayerEntity(other)}, "DB$ Heist"); err != nil {
		t.Fatalf("heist: %v", err)
	}
	if len(g.Zone(engine.Exile, other).Cards()) != 0 {
		t.Error("heist exiled a land-only library")
	}
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Heist | ValidTgts$ Opponent"); err != nil {
		t.Fatalf("heist with no target: %v", err)
	}
}

// TestExchangeZoneParamRejectsAnUnknownZone proves Zone1$/Zone2$ resolution
// errors on an unresolvable zone name.
func TestExchangeZoneParamRejectsAnUnknownZone(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ ExchangeZone | Zone1$ Bogus")
	if err == nil || !strings.Contains(err.Error(), "Zone1$") {
		t.Errorf("err = %v, want a Zone1$ error", err)
	}
}

// TestExchangeZoneNothingToExchangeDoesNothing proves the "nothing in Zone2$
// matches" no-op.
func TestExchangeZoneNothingToExchangeDoesNothing(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ ExchangeZone")
	if z := g.Card(host).Zone; z != engine.Battlefield {
		t.Errorf("host zone = %v, want Battlefield (nothing in hand to trade for)", z)
	}
}

// TestAirbendSkipsACardNotOnTheBattlefield proves a target already moved by
// resolution (Java's equalsWithGameTimestamp skip) is silently passed over.
func TestAirbendSkipsACardNotOnTheBattlefield(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	inHand := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Hand)
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(),
		[]engine.EntityID{engine.CardEntity(inHand)}, "DB$ Airbend | Defined$ Self"); err != nil {
		t.Fatalf("airbend: %v", err)
	}
	if g.Card(inHand).Zone != engine.Hand {
		t.Error("airbend moved a card the target list did not really name")
	}
}

// TestEmpowerRequiresType proves Empower without Type$ fails closed.
func TestEmpowerRequiresType(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil, "DB$ Empower | Num$ 1")
	if err == nil || !strings.Contains(err.Error(), "Type$ missing") {
		t.Errorf("err = %v, want Type$ missing", err)
	}
}

// TestExchangeZoneSkipsACardOwnedByAnotherPlayer proves the object1 owner
// check: a host owned by another player exchanges nothing.
func TestExchangeZoneSkipsACardOwnedByAnotherPlayer(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	notMine := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	// The Ability's own Controller is p, but Object$ points at a card
	// another player owns.
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(),
		[]engine.EntityID{engine.CardEntity(notMine)}, "DB$ ExchangeZone | Object$ Targeted"); err != nil {
		t.Fatalf("exchangezone: %v", err)
	}
	if g.Card(notMine).Zone != engine.Battlefield {
		t.Error("exchange moved a card another player owns")
	}
}

// TestExchangeZoneOnTheBattlefieldFiltersByController proves Zone2$
// Battlefield restricts candidates to the activator's own permanents, not an
// opponent's: Object$ names a card already in exile (Zone1$ Exile) so the
// swap does not touch the resolving host itself.
func TestExchangeZoneOnTheBattlefieldFiltersByController(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	exiled := g.NewCard(creatureDefPT(t, "2", "2"), p, engine.Exile)
	g.NewCard(creatureDefPT(t, "9", "9"), other, engine.Battlefield)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{mine})
	if _, err := resolveNow(t, g, p, c, []engine.EntityID{engine.CardEntity(exiled)},
		"DB$ ExchangeZone | Object$ Targeted | Zone1$ Exile | Zone2$ Battlefield | Mandatory$ True"); err != nil {
		t.Fatalf("exchangezone: %v", err)
	}
	if g.Card(exiled).Zone != engine.Battlefield || g.Card(mine).Zone != engine.Exile {
		t.Fatalf("exiled %v mine %v, want them swapped", g.Card(exiled).Zone, g.Card(mine).Zone)
	}
}

// TestElementalBendTriggersSkipWrongModeLimitAndPlayer proves
// checkPlayerActionTriggers' filters: a differently-named trigger, an
// ActivationLimit$ line and a ValidPlayer$ that does not match the bender
// are all passed over, leaving only the real ElementalBend watcher to fire.
func TestElementalBendTriggersSkipWrongModeLimitAndPlayer(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(bendWatcherDef(t, "Mode$ Discover | ValidPlayer$ You | TriggerZones$ Battlefield", "5"), p, engine.Battlefield)
	g.NewCard(bendWatcherDef(t, "Mode$ ElementalBend | ValidPlayer$ You | TriggerZones$ Battlefield | ActivationLimit$ 1", "5"), p, engine.Battlefield)
	g.NewCard(bendWatcherDef(t, "Mode$ ElementalBend | ValidPlayer$ You | TriggerZones$ Battlefield", "5"), other, engine.Battlefield)
	land := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(land)})
	resolveLine(t, g, p, c, "DB$ Earthbend")
	if got := g.Player(p).Life; got != 20 {
		t.Errorf("bender life = %d, want 20 (Discover mode, the limit and the other player's watcher all skipped)", got)
	}
	if got := g.Player(other).Life; got != 20 {
		t.Errorf("other player life = %d, want 20 (their watcher's ValidPlayer$ You does not match the bender)", got)
	}
}

// TestDiscoverStopsForALostPlayer proves the game-over guard on Discover's
// per-player loop.
func TestDiscoverStopsForALostPlayer(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	g.Player(other).Lost = true
	if _, err := resolveNow(t, g, p, engine.NewScriptedController(),
		[]engine.EntityID{engine.PlayerEntity(other)}, "DB$ Discover | ValidTgts$ Player"); err != nil {
		t.Fatalf("discover: %v", err)
	}
}
