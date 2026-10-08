package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// These tests cover Layer 3 word substitution (ADR-0039): ChangeText,
// ExchangeTextBox and ChangeColorWordsTo$, from the real corpus scripts where
// one exists and from one-line cards where a shape needs its own.

// textGame is a two-player game on the real corpus, whose DB carries the
// subtype vocabulary ChooseBasicLandType and ChooseCreatureType read.
func textGame(t *testing.T) (*engine.Game, engine.PlayerID) {
	t.Helper()
	g, p, _ := newTwoPlayerGameOn(t, scenarioDB(t))
	return g, p
}

// cardKeywords is the printed keyword lines c's definition holds right now.
func cardKeywords(g *engine.Game, id engine.CardID) []string {
	return g.Card(id).Def.Faces[0].Keywords
}

// hasKeywordLine reports whether id's definition has exactly line as a keyword.
func hasKeywordLine(g *engine.Game, id engine.CardID, line string) bool {
	return slices.Contains(cardKeywords(g, id), line)
}

// changeText resolves line, a ChangeText line aimed at Defined$ Targeted, on
// the given cards.
func changeText(t *testing.T, g *engine.Game, p engine.PlayerID, c engine.PlayerController, line string, cards ...engine.CardID) {
	t.Helper()
	var targets []engine.EntityID
	for _, id := range cards {
		targets = append(targets, engine.CardEntity(id))
	}
	pushAndResolve(t, g, p, c, line, targets...)
	sba(g)
}

// TestChangeTextRewritesAKeywordLine proves the dominant shape: Alter
// Reality's "replace one color word with another" on Black Knight changes
// "Protection from white" to the new color and leaves First Strike alone, and a
// second change of that new word chains into one flat map rather than leaving
// the first word behind.
func TestChangeTextRewritesAKeywordLine(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
	sba(g)
	c := engine.NewScriptedController()
	c.QueueColorChoice(mana.White)
	c.QueueColorChoice(mana.Blue)
	changeText(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ Choose Choose | Duration$ Permanent", knight)

	if !hasKeywordLine(g, knight, "Protection from blue") || hasKeywordLine(g, knight, "Protection from white") {
		t.Errorf("keywords = %v, want Protection from blue", cardKeywords(g, knight))
	}
	if !hasKeywordLine(g, knight, "First Strike") {
		t.Errorf("keywords = %v, lost First Strike", cardKeywords(g, knight))
	}
	if g.Card(knight).UncopiedDef().Faces[0].Keywords[1] != "Protection from white" {
		t.Error("the printed keyword changed: a text change is not a copiable value (CR 707.2)")
	}

	c.QueueColorChoice(mana.Blue)
	c.QueueColorChoice(mana.Green)
	changeText(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ Choose Choose | Duration$ Permanent", knight)
	if !hasKeywordLine(g, knight, "Protection from green") {
		t.Errorf("keywords = %v, want Protection from green after white->blue->green", cardKeywords(g, knight))
	}
}

// TestChangeTextEndsAtCleanupUnlessPermanent proves ChangeTextEffect.java:29:
// only Duration$ Permanent survives the turn; absent, and any other value,
// end at cleanup.
func TestChangeTextEndsAtCleanupUnlessPermanent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		duration string
		want     string
	}{
		{"absent", "", "Protection from white"},
		{"any other value", " | Duration$ UntilEndOfTurn", "Protection from white"},
		{"permanent", " | Duration$ Permanent", "Protection from blue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p := textGame(t)
			knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
			c := engine.NewScriptedController()
			changeText(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ White Blue"+tc.duration, knight)
			if !hasKeywordLine(g, knight, "Protection from blue") {
				t.Fatalf("keywords = %v, want the change to apply this turn", cardKeywords(g, knight))
			}
			advanceToCleanup(g, c)
			sba(g)
			if !hasKeywordLine(g, knight, tc.want) {
				t.Errorf("after cleanup keywords = %v, want %q", cardKeywords(g, knight), tc.want)
			}
		})
	}
}

// TestChangeTextTypeWordChangesTheSubtype proves WordChangedType: Magical
// Hack's basic land type swap turns a Swamp into an Island for real, the
// resolving player picks both words, and the new word's list leaves the
// original out.
func TestChangeTextTypeWordChangesTheSubtype(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	reg := scenarioDB(t).Types()
	basics := reg.Members(cardtype.CategoryBasic)
	swamp := g.NewCard(corpusCard(t, "Swamp"), p, engine.Battlefield)
	rec := &optionRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueOption(slices.Index(basics, "Swamp"))
	rec.QueueOption(slices.Index(slices.DeleteFunc(slices.Clone(basics), func(s string) bool { return s == "Swamp" }), "Island"))

	hack := corpusCard(t, "Magical Hack")
	g.PushAbility(engine.Ability{
		API: mustAPI(t, "ChangeText"), Source: swamp, Controller: p,
		Params: hack.Faces[0].Abilities[0], Amounts: hack.Faces[0].Amounts,
		Targets: []engine.EntityID{engine.CardEntity(swamp)},
	})
	if err := g.ResolveStack(engine.NewRegistry(), rec); err != nil {
		t.Fatal(err)
	}
	sba(g)

	if ty := g.Card(swamp).Type(); !ty.HasSubtype("Island") || ty.HasSubtype("Swamp") {
		t.Errorf("type = %v, want an Island and no Swamp", ty)
	}
	if len(rec.offers) != 2 || slices.Contains(rec.offers[1], "Swamp") || !slices.Contains(rec.offers[0], "Swamp") {
		t.Errorf("option lists = %v, want all basics then all but Swamp", rec.offers)
	}
}

func mustAPI(t *testing.T, name string) engine.APIType {
	t.Helper()
	api, ok := engine.APIByName(name)
	if !ok {
		t.Fatalf("no API %q", name)
	}
	return api
}

// TestChangeTextForbiddenNewTypes proves ForbiddenNewTypes$ (Artificial
// Evolution's Wall): the new creature type is chosen from every type but the
// original and the forbidden ones.
func TestChangeTextForbiddenNewTypes(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	creatures := scenarioDB(t).Types().Members(cardtype.CategoryCreature)
	elves := g.NewCard(corpusCard(t, "Llanowar Elves"), p, engine.Battlefield)
	rec := &optionRecorder{ScriptedController: engine.NewScriptedController()}
	rec.QueueOption(slices.Index(creatures, "Elf"))
	rec.QueueOption(0)
	changeText(t, g, p, rec,
		"DB$ ChangeText | Defined$ Targeted | ChangeTypeWord$ ChooseCreatureType ChooseCreatureType | ForbiddenNewTypes$ Wall | Duration$ Permanent", elves)

	if len(rec.offers) != 2 {
		t.Fatalf("asked %d option lists, want 2", len(rec.offers))
	}
	for _, banned := range []string{"Wall", "Elf"} {
		if slices.Contains(rec.offers[1], banned) {
			t.Errorf("new type options contain %q", banned)
		}
	}
	if g.Card(elves).Type().HasSubtype("Elf") {
		t.Errorf("type = %v, want no Elf left", g.Card(elves).Type())
	}
}

// TestChangeTextLiteralWords proves a literal original and a literal new word
// ask nobody (New Blood's Vampire, "Elf Giant"), and a prefix match reaches a
// longer word: a Landwalk line's "Swamp" becomes "Island" inside "Swampwalk"
// text, with the optional "non" kept.
func TestChangeTextLiteralWords(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	def := scriptDef(t, "Walker", "Creature Elf", "K:Landwalk:Card.nonSwamp", "K:Flying",
		"A:AB$ Pump | Cost$ T | ValidTgts$ Creature.Elf | NumAtt$ 1")
	walker := g.NewCard(def, p, engine.Battlefield)
	c := engine.NewScriptedController()
	changeText(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeTypeWord$ Elf Giant | Duration$ Permanent", walker)

	if ty := g.Card(walker).Type(); !ty.HasSubtype("Giant") || ty.HasSubtype("Elf") {
		t.Errorf("type = %v, want Giant, no Elf", ty)
	}
	if got, _ := g.Card(walker).Def.Faces[0].Abilities[0].Param("ValidTgts"); got != "Creature.Giant" {
		t.Errorf("ValidTgts = %q, want Creature.Giant", got)
	}
	if !hasKeywordLine(g, walker, "Landwalk:Card.nonSwamp") || !hasKeywordLine(g, walker, "Flying") {
		t.Errorf("keywords = %v, want both unchanged", cardKeywords(g, walker))
	}

	changeText(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeTypeWord$ Swamp Island | Duration$ Permanent", walker)
	if !hasKeywordLine(g, walker, "Landwalk:Card.nonIsland") {
		t.Errorf("keywords = %v, want Landwalk:Card.nonIsland", cardKeywords(g, walker))
	}
}

// TestChangeTextErrors proves the lines a script can get wrong fail the
// resolution, naming the word, and never write a half change (GO-7).
func TestChangeTextErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{"one color word", "ChangeColorWord$ White", "not two words"},
		{"one type word", "ChangeTypeWord$ Elf", "not two words"},
		{"original is no color", "ChangeColorWord$ Purple Blue", "not a color"},
		{"new is no color", "ChangeColorWord$ White Purple", "not a color"},
		{"Any is no new color", "ChangeColorWord$ White Any", "not a color"},
		{"unknown choose token", "ChangeTypeWord$ ChooseLandType Island", "not resolvable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, p := textGame(t)
			knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
			err := pushAndResolveErr(t, g, p, engine.NewScriptedController(),
				"DB$ ChangeText | Defined$ Targeted | "+tc.line, engine.CardEntity(knight))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", err, tc.want)
			}
			sba(g)
			if !hasKeywordLine(g, knight, "Protection from white") {
				t.Errorf("keywords = %v, a failed change wrote something", cardKeywords(g, knight))
			}
		})
	}
}

// TestChangeTextChoiceErrors proves a controller answer outside the offered
// list fails the resolution: a second color equal to the first, and a type
// index past the list.
func TestChangeTextChoiceErrors(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	knight := g.NewCard(corpusCard(t, "Black Knight"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueColorChoice(mana.White)
	c.QueueColorChoice(mana.White)
	err := pushAndResolveErr(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeColorWord$ Choose Choose", engine.CardEntity(knight))
	if err == nil || !strings.Contains(err.Error(), "invalid new color") {
		t.Errorf("same color twice: error = %v", err)
	}

	c = engine.NewScriptedController()
	c.QueueOption(1 << 20)
	err = pushAndResolveErr(t, g, p, c, "DB$ ChangeText | Defined$ Targeted | ChangeTypeWord$ ChooseBasicLandType Island", engine.CardEntity(knight))
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("type index past the list: error = %v", err)
	}
}

// TestChangeTextOnASpellOnTheStack proves the stack half: a spell resolves
// from the ability built when it was cast, so a word change that reaches it
// while it waits rewrites that ability (MagicStack.java:580). Here a "deals 2
// damage to each red creature" spell is changed to blue before it resolves.
func TestChangeTextOnASpellOnTheStack(t *testing.T) {
	t.Parallel()

	g, p := textGame(t)
	redDef := creatureDefPT(t, "3", "3")
	redDef.Faces[0].ManaCost = mana.MustParse("R")
	blueDef := creatureDefPT(t, "3", "3")
	blueDef.Faces[0].ManaCost = mana.MustParse("U")
	red := g.NewCard(redDef, p, engine.Battlefield)
	blue := g.NewCard(blueDef, p, engine.Battlefield)

	burn := g.NewCard(spellDefWith(t, "Red Burn", "Sorcery", "R",
		"SP$ DamageAll | NumDmg$ 2 | ValidCards$ Creature.Red"), p, engine.Hand)
	alter := g.NewCard(corpusCard(t, "Alter Reality"), p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Red, 1)
	g.Player(p).ManaPool.Add(mana.Blue, 2)
	c := engine.NewScriptedController()
	if !g.CastSpell(p, burn, c) {
		t.Fatal("CastSpell(burn) failed")
	}
	c.QueueTargets([]engine.EntityID{engine.CardEntity(burn)})
	c.QueueColorChoice(mana.Red)
	c.QueueColorChoice(mana.Blue)
	c.QueuePayGeneric(mana.ShardU)
	if !g.CastSpell(p, alter, c) {
		t.Fatal("CastSpell(Alter Reality) failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if g.Card(red).Damage.Marked != 0 || g.Card(blue).Damage.Marked != 2 {
		t.Errorf("damage red %d blue %d, want 0 and 2: the spell now hits blue creatures",
			g.Card(red).Damage.Marked, g.Card(blue).Damage.Marked)
	}
}
