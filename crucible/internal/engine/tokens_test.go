package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// tokenDefT is a token script as compile would build it: name, type line
// and printed power/toughness ("" for a noncreature).
func tokenDefT(t *testing.T, name, typeLine, power, toughness string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: name}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), typeLine)
	def.Faces[0].Power, def.Faces[0].Toughness = power, toughness
	return def
}

// testTokens is the token table these tests run against, keyed by script
// name the way res/tokenscripts is.
func testTokens(t *testing.T) map[string]*compile.Card {
	t.Helper()
	return map[string]*compile.Card{
		"w_1_1_soldier":               tokenDefT(t, "Soldier Token", "Creature Soldier", "1", "1"),
		"c_a_clue_draw":               tokenDefT(t, "Clue Token", "Artifact Clue", "", ""),
		"b_0_0_zombie_army":           tokenDefT(t, "Zombie Army Token", "Creature Zombie Army", "0", "0"),
		"b_0_0_orc_army":              tokenDefT(t, "Orc Army Token", "Creature Orc Army", "0", "0"),
		"incubator_c_0_0_a_phyrexian": tokenDefT(t, "Incubator Token", "Artifact Incubator", "", ""),
	}
}

// newTokenGame is newTwoPlayerGame over a DB holding testTokens.
func newTokenGame(t *testing.T) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	g := engine.NewGame(compile.NewDB(nil).WithTokens(testTokens(t)), javarand.New(1), []string{"a", "b"})
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	return g, p, other
}

// tokensOn is every token on p's battlefield named name.
func tokensOn(g *engine.Game, p engine.PlayerID, name string) []engine.CardID {
	var out []engine.CardID
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if c := g.Card(id); c.IsToken && c.Def.Name == name {
			out = append(out, id)
		}
	}
	return out
}

// TestTokenEffectCreatesAmountForOwner proves the dominant shape (2,226 of
// 3,606 lines name TokenOwner$ You): TokenAmount$ tokens of the script
// enter under the owner's control, tapped with TokenTapped$, summoning
// sick, and RememberTokens$ records each on the host.
func TestTokenEffectCreatesAmountForOwner(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Token Maker",
		"DB$ Token | TokenAmount$ 2 | TokenScript$ w_1_1_soldier | TokenOwner$ You | TokenTapped$ True | RememberTokens$ True")
	host, err := castETBChain(t, g, p, def, c)
	if err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	soldiers := tokensOn(g, p, "Soldier Token")
	if len(soldiers) != 2 {
		t.Fatalf("soldiers on p's battlefield = %d, want 2", len(soldiers))
	}
	for _, id := range soldiers {
		sc := g.Card(id)
		if !sc.Tapped || !sc.SummonSick || sc.Controller() != p || sc.Owner != p {
			t.Errorf("soldier tapped=%v sick=%v controller=%v owner=%v, want tapped, sick, p's", sc.Tapped, sc.SummonSick, sc.Controller(), sc.Owner)
		}
	}
	if got := len(g.Card(host).Memory.Remembered()); got != 2 {
		t.Errorf("host remembers %d, want 2 tokens", got)
	}
}

// TestTokenEffectOwnerOpponentAndBasePT proves TokenOwner$ Opponent puts the
// token under the opponent, and TokenPower$/TokenToughness$ replace the
// script's printed power and toughness (TokenInfo's setBasePower).
func TestTokenEffectOwnerOpponentAndBasePT(t *testing.T) {
	t.Parallel()

	g, p, other := newTokenGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Token Gift",
		"DB$ Token | TokenScript$ w_1_1_soldier | TokenOwner$ Opponent | TokenPower$ 4 | TokenToughness$ 5")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	soldiers := tokensOn(g, other, "Soldier Token")
	if len(soldiers) != 1 {
		t.Fatalf("soldiers on other's battlefield = %d, want 1", len(soldiers))
	}
	pw, _ := g.Card(soldiers[0]).Power()
	tg, _ := g.Card(soldiers[0]).Toughness()
	if pw != 4 || tg != 5 {
		t.Errorf("soldier = %d/%d, want 4/5", pw, tg)
	}
}

// TestTokenLeavingBattlefieldCeasesToExist proves CR 704.5d: a token that
// dies goes to the graveyard (dies triggers see it) and the next
// state-based action check removes it from there.
func TestTokenLeavingBattlefieldCeasesToExist(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Token Then Destroy",
		"DB$ Token | TokenScript$ w_1_1_soldier | RememberTokens$ True | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ Destroy | Defined$ Remembered")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	for _, id := range g.Zone(engine.Graveyard, p).Cards() {
		if g.Card(id).IsToken {
			t.Errorf("token %v still in the graveyard after state-based actions", id)
		}
	}
	if n := len(tokensOn(g, p, "Soldier Token")); n != 0 {
		t.Errorf("soldiers on battlefield = %d, want 0", n)
	}
}

// TestTokenEffectPumpKeywordsUntilEndOfTurn proves PumpKeywords$ with a
// PumpDuration$ lasts until cleanup.
func TestTokenEffectPumpKeywordsUntilEndOfTurn(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Token Haste",
		"DB$ Token | TokenScript$ w_1_1_soldier | PumpKeywords$ Haste | PumpDuration$ EOT")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	soldier := tokensOn(g, p, "Soldier Token")[0]
	if !g.Card(soldier).HasKeyword("Haste") {
		t.Fatal("token lacks Haste right after it was created")
	}
	for g.ActivePhase() != engine.Cleanup {
		g.AdvancePhase(c)
	}
	if g.Card(soldier).HasKeyword("Haste") {
		t.Error("token still has Haste after cleanup")
	}
}

// TestTokenEffectUnknownScriptFails proves a TokenScript$ the DB lacks is an
// error, not a silent no-op (PORT-8).
func TestTokenEffectUnknownScriptFails(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	def := etbChainDef(t, "Test Token Missing", "DB$ Token | TokenScript$ no_such_token")
	_, err := castETBChain(t, g, p, def, engine.NewScriptedController())
	if err == nil || !strings.Contains(err.Error(), "no_such_token") {
		t.Errorf("ResolveStack error = %v, want one naming no_such_token", err)
	}
}

// TestInvestigateEffectCreatesClues proves Num$ Clues for the investigator.
func TestInvestigateEffectCreatesClues(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	def := etbChainDef(t, "Test Investigator", "DB$ Investigate | Num$ 2")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if n := len(tokensOn(g, p, "Clue Token")); n != 2 {
		t.Errorf("clues = %d, want 2", n)
	}
}

// TestAmassEffectCreatesArmyThenGrowsIt proves CR 701.47: with no Army the
// amasser creates a 0/0 one and puts the counters on it; a second amass
// reuses it, and an Army of another type gains the amassed type.
func TestAmassEffectCreatesArmyThenGrowsIt(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	c := engine.NewScriptedController()
	def := etbChainDef(t, "Test Amasser", "DB$ Amass | Type$ Zombie | Num$ 2 | SubAbility$ DBAmass",
		"DBAmass", "DB$ Amass | Type$ Orc | Num$ 1")
	if _, err := castETBChain(t, g, p, def, c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	armies := tokensOn(g, p, "Zombie Army Token")
	if len(armies) != 1 {
		t.Fatalf("zombie armies = %d, want 1 (the second amass reuses it)", len(armies))
	}
	army := g.Card(armies[0])
	if pw, _ := army.Power(); pw != 3 {
		t.Errorf("army power = %d, want 3 (2 + 1 counters)", pw)
	}
	if !army.Type().HasSubtype("Orc") {
		t.Error("army did not become an Orc when amassed as Orcs")
	}
}

// TestIncubateEffectEntersWithCounters proves the Incubator enters with
// Amount$ +1/+1 counters.
func TestIncubateEffectEntersWithCounters(t *testing.T) {
	t.Parallel()

	g, p, _ := newTokenGame(t)
	def := etbChainDef(t, "Test Incubate", "DB$ Incubate | Amount$ 3")
	if _, err := castETBChain(t, g, p, def, engine.NewScriptedController()); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	incubators := tokensOn(g, p, "Incubator Token")
	if len(incubators) != 1 {
		t.Fatalf("incubators = %d, want 1", len(incubators))
	}
	if n := g.Card(incubators[0]).Counters.Count(engine.P1P1); n != 3 {
		t.Errorf("incubator +1/+1 counters = %d, want 3", n)
	}
}
