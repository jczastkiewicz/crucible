package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// Layer 7a amounts past the Count$Valid family (ExiledWith$, Remembered$,
// PlayerCountRemembered$, Count$YourTurns, Count$Party, the DifferentCardNames
// property), and characteristic-defining and EffectZone$ statics whose host is
// not on the battlefield (CR 604.3: a CDA functions in every zone).

func checkSBA(g *engine.Game) {
	engine.CheckStateBasedActions(g, engine.NewScriptedController())
}

func TestCDARememberedAmountAndManaCost(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p := g.Players()[0]
	wood := g.NewCard(cdaDef(t, "Remembered$Amount", "Remembered$Amount/Plus.1"), p, engine.Battlefield)
	checkSBA(g)
	wantCDAPT(t, g, wood, 0, 1, "nothing remembered")

	a := g.NewCard(creatureDefCost(t, "Sacrificed A", "G"), p, engine.Graveyard)
	b := g.NewCard(creatureDefCost(t, "Sacrificed B", "G"), p, engine.Graveyard)
	g.Card(wood).Memory.Remember(engine.CardEntity(a))
	g.Card(wood).Memory.Remember(engine.CardEntity(b))
	checkSBA(g)
	wantCDAPT(t, g, wood, 2, 3, "two remembered cards")

	lore := g.NewCard(cdaDef(t, "Remembered$CardManaCost", "Remembered$CardManaCost"), p, engine.Battlefield)
	exiled := g.NewCard(creatureDefCost(t, "Exiled Four", "3 G"), p, engine.Exile)
	g.Card(lore).Memory.Remember(engine.CardEntity(exiled))
	checkSBA(g)
	wantCDAPT(t, g, lore, 4, 4, "the remembered card's mana value")
}

func TestCDAPlayerCountRememberedLifeTotal(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p, opp := g.Players()[0], g.Players()[1]
	champ := g.NewCard(cdaDef(t, "PlayerCountRemembered$LifeTotal", "PlayerCountRemembered$LifeTotal"), p, engine.Battlefield)
	g.Card(champ).Memory.Remember(engine.PlayerEntity(opp))
	g.Player(opp).Life = 7
	checkSBA(g)
	wantCDAPT(t, g, champ, 7, 7, "the remembered player's life")
}

func TestCDAExiledWithCardPowerAndColors(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p := g.Players()[0]
	altar := g.NewCard(cdaDef(t, "ExiledWith$CardPower", "ExiledWith$CardPower"), p, engine.Battlefield)
	other := g.NewCard(cdaDef(t, "ExiledWith$CardPower", "ExiledWith$CardPower"), p, engine.Battlefield)
	checkSBA(g)
	wantCDAPT(t, g, altar, 0, 0, "nothing exiled with it")

	big := g.NewCard(creatureDefPT(t, "3", "2"), p, engine.Exile)
	small := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Exile)
	stranger := g.NewCard(creatureDefPT(t, "9", "9"), p, engine.Exile)
	g.SetExiledWith(big, altar)
	g.SetExiledWith(small, altar)
	g.SetExiledWith(stranger, other)
	checkSBA(g)
	wantCDAPT(t, g, altar, 4, 4, "powers 3 and 1 of the cards exiled with it")
	wantCDAPT(t, g, other, 9, 9, "its own exiled card only")

	sunbird := g.NewCard(cdaDef(t, "ExiledWith$Colors", "ExiledWith$Colors"), p, engine.Battlefield)
	green := g.NewCard(creatureDefCost(t, "Green One", "G"), p, engine.Exile)
	blue := g.NewCard(creatureDefCost(t, "Blue One", "U"), p, engine.Exile)
	blue2 := g.NewCard(creatureDefCost(t, "Blue Two", "U"), p, engine.Exile)
	for _, id := range []engine.CardID{green, blue, blue2} {
		g.SetExiledWith(id, sunbird)
	}
	checkSBA(g)
	wantCDAPT(t, g, sunbird, 2, 2, "green and blue, blue counted once")
}

// An unknown property over a non-empty list stays unresolved (an empty list is
// 0 whatever the property, handlePaid's first line), as do a remembered-card
// head with a lone-token exiled card and the LKI form.
func TestCDAListHeadsRefuseUnknownProperties(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p := g.Players()[0]
	for _, body := range []string{"ExiledWith$Bogus", "Imprinted$Bogus", "Remembered$Bogus"} {
		host := g.NewCard(cdaDef(t, body, body), p, engine.Battlefield)
		card := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Exile)
		g.SetExiledWith(card, host)
		g.Card(host).Memory.Imprint(card)
		g.Card(host).Memory.Remember(engine.CardEntity(card))
		checkSBA(g)
		if pw, ok := g.Card(host).Power(); ok {
			t.Errorf("%s: Power() = (%d, true), want unresolvable", body, pw)
		}
	}
	// A token exiled with the host is not on its list.
	host := g.NewCard(cdaDef(t, "ExiledWith$Amount", "ExiledWith$Amount"), p, engine.Battlefield)
	tok := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Exile)
	g.Card(tok).IsToken = true
	g.SetExiledWith(tok, host)
	checkSBA(g)
	wantCDAPT(t, g, host, 0, 0, "tokens are never on the exiled list")
}

func TestCDAYourTurnsCountsTurnsTaken(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p := g.Players()[0]
	for _, pid := range g.Players() {
		for range 8 {
			g.NewCard(creatureDefPT(t, "1", "1"), pid, engine.Library)
		}
	}
	whale := g.NewCard(cdaDef(t, "Count$YourTurns", "Count$YourTurns"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	g.StartTurn(p, c)
	checkSBA(g)
	wantCDAPT(t, g, whale, 1, 1, "the first turn counts from its start")
	for g.Turn() < 3 {
		g.AdvancePhase(c)
	}
	checkSBA(g)
	wantCDAPT(t, g, whale, 2, 2, "the controller's second turn is turn three")
}

func TestCDAPartyCountsDistinctPartyRoles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		types []string
		want  int
	}{
		{"none", []string{"Creature Human"}, 0},
		{"one role twice", []string{"Creature Human Cleric", "Creature Human Cleric"}, 1},
		{"four roles", []string{"Creature Cleric", "Creature Rogue", "Creature Warrior", "Creature Wizard"}, 4},
		{"a two-role creature fills the missing one", []string{"Creature Cleric", "Creature Cleric Rogue"}, 2},
		{"a two-role creature cannot fill two roles", []string{"Creature Cleric Rogue"}, 1},
		{"all four roles on one creature is a wildcard", []string{"Creature Cleric Rogue Warrior Wizard", "Creature Cleric"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := newLiveGame(t)
			p := g.Players()[0]
			host := g.NewCard(cdaDef(t, "Count$Party", "Count$Party"), p, engine.Hand)
			g.Move(host, engine.Battlefield, p)
			for i, tl := range tc.types {
				g.NewCard(amountDef(t, "Party Member "+string(rune('A'+i)), tl, "", "1", "1", nil), p, engine.Battlefield)
			}
			checkSBA(g)
			wantCDAPT(t, g, host, tc.want, tc.want, tc.name)
		})
	}
}

func TestCDADifferentCardNames(t *testing.T) {
	t.Parallel()
	g := newLiveGame(t)
	p := g.Players()[0]
	amalgam := g.NewCard(cdaDef(t, "Count$Valid Land.YouCtrl$DifferentCardNames", "Count$Valid Land.YouCtrl$DifferentCardNames"), p, engine.Battlefield)
	checkSBA(g)
	wantCDAPT(t, g, amalgam, 0, 0, "no lands")
	for _, n := range []string{"Forest", "Forest", "Island"} {
		g.NewCard(landDef(t, n, "Basic Land "+n), p, engine.Battlefield)
	}
	checkSBA(g)
	wantCDAPT(t, g, amalgam, 2, 2, "two names among three lands")
}

// A CDA functions in every zone: Colossal Rattlewurm "has flash as long as you
// control a Desert" while it sits in the hand.
func TestCDAKeywordFunctionsInHand(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	wurm := g.NewCard(corpusCard(t, "Colossal Rattlewurm"), p, engine.Hand)
	checkSBA(g)
	if g.Card(wurm).HasKeyword("Flash") {
		t.Fatal("flash in hand without a Desert")
	}
	g.NewCard(landDef(t, "Test Desert", "Land Desert"), p, engine.Battlefield)
	checkSBA(g)
	if !g.Card(wurm).HasKeyword("Flash") {
		t.Error("no flash in hand with a Desert, want the CDA applied")
	}
	// ...and it is castable at instant speed on the opponent's turn.
	g.SetTurnState(1, g.Players()[1], engine.Main1)
	checkSBA(g)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	g.Player(p).ManaPool.AddColorless(2)
	if !g.CastSpell(p, wurm, payWith(mana.ShardC, mana.ShardC)) {
		t.Error("CastSpell on the opponent's turn with the flash CDA = false")
	}
}

// Sphinx of the Guildpact is all colors in every zone; Grist, the Hunger
// Tide is a 1/1 Insect creature everywhere but the battlefield (ExcludeZone$).
func TestCDAColorTypeAndPTOffTheBattlefield(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	sphinx := g.NewCard(corpusCard(t, "Sphinx of the Guildpact"), p, engine.Hand)
	grist := g.NewCard(corpusCard(t, "Grist, the Hunger Tide"), p, engine.Graveyard)
	checkSBA(g)
	if got := g.Card(sphinx).Colors().Count(); got != 5 {
		t.Errorf("Sphinx in hand has %d colors, want 5", got)
	}
	if !g.Card(grist).Type().HasSubtype("Insect") {
		t.Error("Grist in the graveyard is not an Insect")
	}
	if pw, ok := g.Card(grist).Power(); !ok || pw != 1 {
		t.Errorf("Grist in the graveyard has power %d (resolved %v), want 1", pw, ok)
	}
	g.Move(sphinx, engine.Graveyard, p)
	g.Move(grist, engine.Exile, p)
	checkSBA(g)
	if got := g.Card(sphinx).Colors().Count(); got != 5 {
		t.Errorf("Sphinx in the graveyard has %d colors, want 5", got)
	}
	if pw, ok := g.Card(grist).Power(); !ok || pw != 1 {
		t.Errorf("Grist in exile has power %d (resolved %v), want 1", pw, ok)
	}
}

// Brawn works from the graveyard (EffectZone$ Graveyard, a host traitHosts
// does not walk): creatures its controller controls have trample while a
// Forest is also under that controller's control.
func TestEffectZoneGraveyardHostGrantsKeyword(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	bear := g.NewCard(corpusCard(t, "Grizzly Bears"), p, engine.Battlefield)
	brawn := g.NewCard(corpusCard(t, "Brawn"), p, engine.Hand)
	g.NewCard(corpusCard(t, "Forest"), p, engine.Battlefield)
	checkSBA(g)
	if g.Card(bear).HasKeyword("Trample") {
		t.Fatal("trample with Brawn in the hand")
	}
	g.Move(brawn, engine.Graveyard, p)
	checkSBA(g)
	if !g.Card(bear).HasKeyword("Trample") {
		t.Error("no trample with Brawn in the graveyard and a Forest")
	}
	g.Move(brawn, engine.Exile, p)
	checkSBA(g)
	if g.Card(bear).HasKeyword("Trample") {
		t.Error("trample outlived Brawn leaving the graveyard")
	}
}
