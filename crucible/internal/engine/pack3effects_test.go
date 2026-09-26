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

// newPackGame is newTwoPlayerGame over a DB holding cards, testTokens plus
// the Spirit and Human Soldier scripts, and the test type vocabulary.
func newPackGame(t *testing.T, cards ...*compile.Card) (*engine.Game, engine.PlayerID, engine.PlayerID) {
	t.Helper()
	byName := map[string]*compile.Card{}
	for _, c := range cards {
		byName[c.Name] = c
	}
	tokens := testTokens(t)
	tokens["w_x_x_spirit"] = tokenDefT(t, "Spirit Token", "Creature Elf", "0", "0")
	tokens["w_1_1_human_soldier"] = tokenDefT(t, "Human Soldier Token", "Creature Elf", "1", "1")
	db := compile.NewDB(byName).WithTokens(tokens).WithTypes(attachmentTypeRegistry(t))
	g := engine.NewGame(db, javarand.New(1), []string{"a", "b"})
	p, other := g.Players()[0], g.Players()[1]
	g.SetTurnState(1, p, engine.Main1)
	g.Player(p).Life, g.Player(other).Life = 20, 20
	return g, p, other
}

func namedCreature(t *testing.T, name, cost string) *compile.Card {
	t.Helper()
	def := creatureDefCost(t, name, cost)
	return def
}

func namedLand(t *testing.T, name string) *compile.Card {
	t.Helper()
	def := &compile.Card{Name: name}
	def.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Land")
	return def
}

func TestDetainStopsAttackBlockAndActivation(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	resolveLine(t, g, p, c, "DB$ Detain | ValidTgts$ Creature.OppCtrl")
	atk := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	if g.CanBlock(atk, victim) {
		t.Error("a detained creature can block")
	}
	g.SetTurnState(1, other, engine.Main1)
	ac := engine.NewScriptedController()
	ac.QueueAttackers(nil)
	if got := declareAttackers(t, g, ac); len(got) != 0 {
		t.Errorf("attackers = %v, want none (detained was the only creature)", got)
	}
	g.SetTurnState(1, other, engine.Cleanup)
	g.AdvancePhase(engine.NewScriptedController())
	g.SetTurnState(2, other, engine.Cleanup)
	g.AdvancePhase(engine.NewScriptedController())
	if !g.CanBlock(atk, victim) {
		t.Error("detain did not end at the detainer's next turn")
	}
}

func TestIntensifyAndBlight(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	host := resolveLine(t, g, p, c, "DB$ Intensify | Amount$ 2 | SubAbility$ DBAll",
		"DBAll", "DB$ Intensify | AllDefined$ Creature.YouCtrl")
	if got := g.Card(host).Intensity; got != 3 {
		t.Errorf("intensity = %d, want 3", got)
	}
	big := g.NewCard(creatureDefPT(t, "4", "4"), p, engine.Battlefield)
	c2 := engine.NewScriptedController()
	c2.QueueCardChoice([]engine.CardID{big})
	resolveLine(t, g, p, c2, "DB$ Blight | Defined$ You | Num$ 2")
	if got := g.Card(big).Counters.Count(engine.M1M1); got != 2 {
		t.Errorf("-1/-1 counters = %d, want 2", got)
	}
}

func TestTimeTravelAddsAndRemoves(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	clock := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	g.Card(clock).Counters.Add(engine.Time, 2)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{clock})
	c.QueueBinary(false)
	c.QueueCardChoice([]engine.CardID{clock})
	c.QueueBinary(false)
	resolveLine(t, g, p, c, "DB$ TimeTravel | Amount$ 2")
	if got := g.Card(clock).Counters.Count(engine.Time); got != 0 {
		t.Errorf("time counters = %d, want 0", got)
	}
}

func TestEndureCountersOrSpirit(t *testing.T) {
	t.Parallel()
	g, p, _ := newPackGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(true)
	c.QueueConfirmEffect(true)
	host := resolveLine(t, g, p, c, "DB$ Endure | Num$ 2 | SubAbility$ DBAgain",
		"DBAgain", "DB$ Endure | Num$ 3")
	if got := g.Card(host).Counters.Count(engine.P1P1); got != 5 {
		t.Errorf("+1/+1 = %d, want 5", got)
	}
}

func TestEndureDeclinedMakesSpirit(t *testing.T) {
	t.Parallel()
	g, p, _ := newPackGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(false)
	resolveLine(t, g, p, c, "DB$ Endure | Num$ 3")
	spirits := tokensOn(g, p, "Spirit Token")
	if len(spirits) != 1 {
		t.Fatalf("spirits = %d, want 1", len(spirits))
	}
	if pw, _ := g.Card(spirits[0]).Power(); pw != 3 {
		t.Errorf("spirit power = %d, want 3", pw)
	}
}

func TestAssignGroupAndVillainousChoice(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueAbilityChoice([]int{1})
	c.QueueAbilityChoice([]int{0})
	resolveLine(t, g, p, c, "DB$ AssignGroup | Defined$ Player | Choices$ DBGain,DBLose",
		"DBGain", "DB$ GainLife | Defined$ Remembered | LifeAmount$ 5",
		"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 3")
	if g.Player(p).Life != 17 || g.Player(other).Life != 25 {
		t.Errorf("life = %d/%d, want 17/25", g.Player(p).Life, g.Player(other).Life)
	}
	c2 := engine.NewScriptedController()
	c2.QueueAbilityChoice([]int{0})
	resolveLine(t, g, p, c2, "DB$ VillainousChoice | Defined$ Opponent | Choices$ DBA,DBB",
		"DBA", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 4",
		"DBB", "DB$ GainLife | Defined$ You | LifeAmount$ 9")
	if got := g.Player(other).Life; got != 21 {
		t.Errorf("opponent life = %d, want 21", got)
	}
}

func TestTwoPilesChosenPileResolves(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	x := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	y := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Graveyard)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{x})
	c.QueueBinary(false) // the other pile
	resolveLine(t, g, p, c, "DB$ TwoPiles | Defined$ You | Zone$ Graveyard | ChosenPile$ DBReturn",
		"DBReturn", "DB$ ChangeZone | Defined$ Remembered | Origin$ Graveyard | Destination$ Hand")
	if g.Card(y).Zone != engine.Hand || g.Card(x).Zone != engine.Graveyard {
		t.Errorf("zones x=%v y=%v, want y returned", g.Card(x).Zone, g.Card(y).Zone)
	}
}

func TestChooseTypeDrivesChosenType(t *testing.T) {
	t.Parallel()
	g, p, other := newPackGame(t)
	elf := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	land := g.NewCard(namedLand(t, "Plain Land"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueOption(0) // "Elf", the vocabulary's only creature type
	host := resolveLine(t, g, p, c, "DB$ ChooseType | Defined$ You | Type$ Creature | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ DestroyAll | ValidCards$ Permanent.ChosenType+OppCtrl")
	if got := g.Card(host).Memory.ChosenType(false); got != "Elf" {
		t.Errorf("chosen type = %q, want Elf", got)
	}
	if g.Card(elf).Zone != engine.Graveyard || g.Card(land).Zone != engine.Battlefield {
		t.Error("ChosenType did not select only the Elf")
	}
	c2 := engine.NewScriptedController()
	c2.QueueOption(1)
	host2 := resolveLine(t, g, p, c2, "DB$ ChooseType | Defined$ You | Type$ Card | InvalidTypes$ Kindred | ChooseType2$ True")
	if got := g.Card(host2).Memory.ChosenType(true); got != "Battle" {
		t.Errorf("chosen type2 = %q, want Battle", got)
	}
	c3 := engine.NewScriptedController()
	host3 := resolveLine(t, g, p, c3, "DB$ ChooseType | Defined$ You | Type$ Card | ValidTypes$ Artifact,Creature | AtRandom$ True")
	if got := g.Card(host3).Memory.ChosenType(false); got != "Artifact" && got != "Creature" {
		t.Errorf("random type = %q", got)
	}
}

func TestNameCardAndNamedCard(t *testing.T) {
	t.Parallel()
	bear := namedCreature(t, "Grizzly Bears", "1 G")
	g, p, other := newPackGame(t, bear, namedLand(t, "Forest Land"))
	target := g.NewCard(bear, other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueOption(0) // Grizzly Bears, the only nonland card
	host := resolveLine(t, g, p, c, "DB$ NameCard | Defined$ You | ValidCards$ Card.nonLand | SubAbility$ DBDestroy",
		"DBDestroy", "DB$ DestroyAll | ValidCards$ Creature.NamedCard")
	if got := g.Card(host).Memory.NamedCards(); len(got) != 1 || got[0] != "Grizzly Bears" {
		t.Errorf("named = %v", got)
	}
	if g.Player(p).NamedCard != "Grizzly Bears" {
		t.Errorf("player named %q", g.Player(p).NamedCard)
	}
	if g.Card(target).Zone != engine.Graveyard {
		t.Error("NamedCard did not match the bear")
	}
	c2 := engine.NewScriptedController()
	host2 := resolveLine(t, g, p, c2, "DB$ NameCard | Defined$ You | ChooseFromList$ Alpha,Beta | AtRandom$ True")
	if got := g.Card(host2).Memory.NamedCards(); len(got) != 1 {
		t.Errorf("random named = %v", got)
	}
}

func TestPreventDamageShield(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ PreventDamage | Defined$ You | Amount$ 3 | SubAbility$ DBBurn",
		"DBBurn", "DB$ DealDamage | Defined$ You | NumDmg$ 5 | SubAbility$ DBBurn2",
		"DBBurn2", "DB$ DealDamage | Defined$ You | NumDmg$ 2")
	if got := g.Player(p).Life; got != 16 {
		t.Errorf("life = %d, want 16 (3 of the first 5 prevented)", got)
	}
}

func TestDigMultipleOnePerCategory(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	cr := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	ld := g.NewCard(namedLand(t, "L"), p, engine.Library)
	other := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	g.MoveToLibraryTop(other, p)
	g.MoveToLibraryTop(ld, p)
	g.MoveToLibraryTop(cr, p)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{cr})
	c.QueueCardChoice([]engine.CardID{ld})
	resolveLine(t, g, p, c, "DB$ DigMultiple | DigNum$ 3 | ChangeValid$ Creature,Land | RememberChanged$ True")
	if g.Card(cr).Zone != engine.Hand || g.Card(ld).Zone != engine.Hand {
		t.Error("picks not in hand")
	}
	lib := g.Zone(engine.Library, p).Cards()
	if lib[len(lib)-1] != other {
		t.Error("rest not on the bottom")
	}
}

func TestRecruitMakesSoldierForNonland(t *testing.T) {
	t.Parallel()
	g, p, _ := newPackGame(t)
	spell := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueDiscardChoice([]engine.CardID{spell})
	resolveLine(t, g, p, c, "DB$ Recruit")
	if n := len(tokensOn(g, p, "Human Soldier Token")); n != 1 {
		t.Errorf("soldiers = %d, want 1", n)
	}
}

func TestBidLifeHighestBidWins(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueConfirmEffect(true) // p tops with 3
	c.QueueNumberChoice(3)
	c.QueueConfirmEffect(true) // other tops with 2 -> 5
	c.QueueNumberChoice(2)
	c.QueueConfirmEffect(false)
	c.QueueConfirmEffect(false)
	host := resolveLine(t, g, p, c, "DB$ BidLife | StartBidding$ 0 | BidSubAbility$ DBLose",
		"DBLose", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 5")
	if got := g.Player(other).Life; got != 15 {
		t.Errorf("winner life = %d, want 15", got)
	}
	if n, ok := g.Card(host).Memory.ChosenNumber(); !ok || n != 5 {
		t.Errorf("chosen number = %d, want the final bid 5", n)
	}
}

func TestExchangeControlVariantSwaps(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	mine := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	theirs := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(p), engine.PlayerEntity(other)})
	c.QueueCardChoice([]engine.CardID{mine})
	c.QueueCardChoice([]engine.CardID{theirs})
	resolveLine(t, g, p, c, "DB$ ExchangeControlVariant | ValidTgts$ Player | TargetMin$ 2 | TargetMax$ 2 | Type$ Creature.nonToken+toughnessLE2+powerGE1+cmcEQ0")
	if g.Card(mine).Controller() != other || g.Card(theirs).Controller() != p {
		t.Error("control not exchanged")
	}
}

func TestDayTimeSetAndUntapRule(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DayTime | Value$ Day")
	if g.DayTime() != engine.Day {
		t.Fatalf("daytime = %v, want Day", g.DayTime())
	}
	g.SetTurnState(1, p, engine.Cleanup)
	for _, pid := range g.Players() {
		g.Player(pid).SpellsCastThisTurn = 0
	}
	g.AdvancePhase(engine.NewScriptedController())
	if g.DayTime() != engine.Night {
		t.Errorf("after a turn with no spells daytime = %v, want Night", g.DayTime())
	}
	if _, err := resolveNow(t, g, g.ActivePlayer(), engine.NewScriptedController(), nil, "DB$ DayTime | Value$ Switch"); err != nil {
		t.Fatalf("DayTime: %v", err)
	}
	if g.DayTime() != engine.Day {
		t.Errorf("after Switch = %v, want Day", g.DayTime())
	}
}

func TestAlterAttributeSuspected(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	atk := g.NewCard(creatureDefPT(t, "2", "2"), other, engine.Battlefield)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ AlterAttribute | Defined$ Self | Attributes$ Suspected,Solved")
	c := g.Card(host)
	if !c.Suspected || !c.Solved || !c.HasKeyword("Menace") {
		t.Errorf("suspected=%v solved=%v menace=%v", c.Suspected, c.Solved, c.HasKeyword("Menace"))
	}
	if g.CanBlock(atk, host) {
		t.Error("a suspected creature can block")
	}
}

func TestAlterAttributeUnsuspect(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ AlterAttribute | Defined$ Self | Attributes$ Suspected | SubAbility$ DBOff",
		"DBOff", "DB$ AlterAttribute | Defined$ Self | Attributes$ Suspected | Activate$ False")
	if c := g.Card(host); c.Suspected || c.HasKeyword("Menace") {
		t.Error("unsuspected card kept suspect or menace")
	}
}

func TestVoteChoicesMostVotesWin(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	c := engine.NewScriptedController()
	c.QueueAbilityChoice([]int{1})
	c.QueueAbilityChoice([]int{1})
	resolveLine(t, g, p, c, "DB$ Vote | Defined$ Player | Choices$ DBA,DBB",
		"DBA", "DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"DBB", "DB$ LoseLife | Defined$ Opponent | LifeAmount$ 2")
	if got := g.Player(other).Life; got != 18 {
		t.Errorf("opponent life = %d, want 18", got)
	}
	c2 := engine.NewScriptedController()
	c2.QueueAbilityChoice([]int{0})
	c2.QueueAbilityChoice([]int{1})
	resolveLine(t, g, p, c2, "DB$ Vote | Defined$ Player | Choices$ DBA,DBB | VoteTiedAbility$ DBTie",
		"DBA", "DB$ GainLife | Defined$ You | LifeAmount$ 1",
		"DBB", "DB$ GainLife | Defined$ You | LifeAmount$ 2",
		"DBTie", "DB$ GainLife | Defined$ You | LifeAmount$ 7")
	if got := g.Player(p).Life; got != 27 {
		t.Errorf("life after tie = %d, want 27", got)
	}
	c3 := engine.NewScriptedController()
	c3.QueueAbilityChoice([]int{0})
	c3.QueueAbilityChoice([]int{0})
	resolveLine(t, g, p, c3, "DB$ Vote | Defined$ Player | Choices$ DBA | StoreVoteNum$ True",
		"DBA", "DB$ GainLife | Defined$ You | LifeAmount$ VoteNum")
	if got := g.Player(p).Life; got != 29 {
		t.Errorf("life after vote count = %d, want 29", got)
	}
	c4 := engine.NewScriptedController()
	c4.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(other)})
	c4.QueueEntityChoice([]engine.EntityID{engine.PlayerEntity(other)})
	resolveLine(t, g, p, c4, "DB$ Vote | Defined$ Player | VotePlayer$ Player | VoteSubAbility$ DBHit",
		"DBHit", "DB$ LoseLife | Defined$ Remembered | LifeAmount$ 1")
	if got := g.Player(other).Life; got != 17 {
		t.Errorf("voted player life = %d, want 17", got)
	}
}

func TestMakeCardConjuresIntoZones(t *testing.T) {
	t.Parallel()
	bear := namedCreature(t, "Grizzly Bears", "1 G")
	g, p, _ := newPackGame(t, bear)
	host := resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ MakeCard | Name$ Grizzly Bears | Zone$ Hand | Amount$ 2 | RememberMade$ True | SubAbility$ DBBf",
		"DBBf", "DB$ MakeCard | Name$ Grizzly Bears | Zone$ Battlefield | WithCounter$ P1P1 | WithCounterNum$ 2")
	if n := len(g.Card(host).Memory.Remembered()); n != 2 {
		t.Errorf("remembered %d made cards, want 2", n)
	}
	var found bool
	for _, id := range g.Zone(engine.Battlefield, p).Cards() {
		if c := g.Card(id); c.Def != nil && c.Def.Name == "Grizzly Bears" && c.Counters.Count(engine.P1P1) == 2 {
			found = true
		}
	}
	if !found {
		t.Error("no conjured bear with counters on the battlefield")
	}
	c := engine.NewScriptedController()
	c.QueueOption(0)
	resolveLine(t, g, p, c, "DB$ MakeCard | Spellbook$ Grizzly Bears | Zone$ Library")
}

func TestLearnRummages(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	junk := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand)
	g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{junk})
	resolveLine(t, g, p, c, "DB$ Learn")
	if g.Card(junk).Zone != engine.Graveyard || len(g.Zone(engine.Hand, p).Cards()) != 1 {
		t.Error("learn did not discard then draw")
	}
}

func TestLearnFetchesLesson(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	lesson := &compile.Card{Name: "Lesson"}
	lesson.Faces[0].Type = cardtype.Parse(lessonRegistry(t), "Sorcery Lesson")
	id := g.NewCard(lesson, p, engine.Sideboard)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{id})
	resolveLine(t, g, p, c, "DB$ Learn")
	if g.Card(id).Zone != engine.Hand {
		t.Error("lesson not fetched")
	}
}

func lessonRegistry(t *testing.T) *cardtype.Registry {
	t.Helper()
	reg, err := cardtype.LoadRegistry(strings.NewReader("[CreatureTypes]\nElf\n[SpellTypes]\nLesson\n"))
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestCopyPermanentMakesTokenCopies(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	orig := g.NewCard(creatureDefPT(t, "4", "4"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(orig)})
	host := resolveLine(t, g, p, c, "DB$ CopyPermanent | ValidTgts$ Creature | NumCopies$ 2 | SetPower$ 1 | RememberTokens$ True | TokenTapped$ True")
	rem := g.Card(host).Memory.Remembered()
	if len(rem) != 2 {
		t.Fatalf("copies = %d, want 2", len(rem))
	}
	cid, _ := rem[0].AsCard()
	cp := g.Card(cid)
	pw, _ := cp.Power()
	tg, _ := cp.Toughness()
	if !cp.IsToken || cp.Controller() != p || pw != 1 || tg != 4 || !cp.Tapped {
		t.Errorf("copy token=%v ctrl=%v pt=%d/%d tapped=%v", cp.IsToken, cp.Controller(), pw, tg, cp.Tapped)
	}
}

func TestCopyPermanentChoicesAndDefinedName(t *testing.T) {
	t.Parallel()
	bear := namedCreature(t, "Grizzly Bears", "1 G")
	g, p, _ := newPackGame(t, bear)
	src := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{src})
	host := resolveLine(t, g, p, c, "DB$ CopyPermanent | Choices$ Creature.powerEQ3 | RememberTokens$ True | SubAbility$ DBName",
		"DBName", "DB$ CopyPermanent | DefinedName$ Grizzly Bears | RememberTokens$ True")
	if n := len(g.Card(host).Memory.Remembered()); n != 2 {
		t.Errorf("tokens = %d, want 2", n)
	}
}

func TestCounterSpellOnStack(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	spell := g.NewCard(creatureDefCost(t, "Spell", "G"), other, engine.Hand)
	g.SetTurnState(1, other, engine.Main1)
	g.Player(other).ManaPool.Add(mana.Green, 1)
	if !g.CastSpell(other, spell, engine.NewScriptedController()) {
		t.Fatal("cast failed")
	}
	def := etbChainDef(t, "Counterer", "DB$ Counter | TargetType$ Spell | ValidTgts$ Card | Destination$ Exile | RememberCountered$ True")
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	c := engine.NewScriptedController()
	g.PushAbility(engine.Ability{API: engine.APICounter, Source: host, Controller: p, Params: sub,
		Targets: []engine.EntityID{engine.CardEntity(spell)}})
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatal(err)
	}
	if g.Card(spell).Zone != engine.Exile || g.StackLen() != 0 {
		t.Errorf("spell zone = %v, stack %d; want exiled, empty", g.Card(spell).Zone, g.StackLen())
	}
}

func TestManifestAndCloakAndTurnFaceUp(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	top := g.NewCard(creatureDefPT(t, "5", "5"), p, engine.Library)
	host := resolveLine(t, g, p, engine.NewScriptedController(), "DB$ Manifest | RememberManifested$ True")
	c := g.Card(top)
	if c.Zone != engine.Battlefield || !c.IsFaceDown() || !c.Manifested {
		t.Fatalf("zone=%v facedown=%v", c.Zone, c.IsFaceDown())
	}
	if pw, _ := c.Power(); pw != 2 {
		t.Errorf("face-down power = %d, want 2", pw)
	}
	if len(g.Card(host).Memory.Remembered()) != 1 {
		t.Error("manifested card not remembered")
	}
	cc := engine.NewScriptedController()
	cc.QueueTargets([]engine.EntityID{engine.CardEntity(top)})
	resolveLine(t, g, p, cc, "DB$ SetState | ValidTgts$ Creature | Mode$ TurnFaceUp")
	if c.IsFaceDown() {
		t.Fatal("still face down")
	}
	if pw, _ := c.Power(); pw != 5 {
		t.Errorf("face-up power = %d, want 5", pw)
	}
	hand := g.NewCard(creatureDefPT(t, "3", "3"), p, engine.Hand)
	ch := engine.NewScriptedController()
	ch.QueueCardChoice([]engine.CardID{hand})
	resolveLine(t, g, p, ch, "DB$ Cloak | ChoiceZone$ Hand")
	if !g.Card(hand).Cloaked || !g.Card(hand).HasKeyword("Ward") {
		t.Error("cloaked card lacks ward")
	}
	g.Move(hand, engine.Graveyard, p)
	if g.Card(hand).IsFaceDown() {
		t.Error("left the battlefield face down")
	}
}

func TestManifestDreadManifestsOneMillsOther(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	b := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Library)
	c := engine.NewScriptedController()
	c.QueueCardChoice([]engine.CardID{b})
	resolveLine(t, g, p, c, "DB$ ManifestDread")
	if !g.Card(b).IsFaceDown() || g.Card(a).Zone != engine.Graveyard {
		t.Errorf("b facedown=%v a zone=%v", g.Card(b).IsFaceDown(), g.Card(a).Zone)
	}
}

func transformDef(t *testing.T) *compile.Card {
	t.Helper()
	reg := attachmentTypeRegistry(t)
	def := &compile.Card{Name: "Front", SplitType: carddb.SplitTransform}
	def.Faces[0].Name, def.Faces[0].Type = "Front", cardtype.Parse(reg, "Creature Elf")
	def.Faces[0].Power, def.Faces[0].Toughness = "1", "1"
	def.Faces[1].Name, def.Faces[1].Type = "Back", cardtype.Parse(reg, "Creature Elf")
	def.Faces[1].Power, def.Faces[1].Toughness = "4", "4"
	return def
}

func TestSetStateTransformsBothWays(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	card := g.NewCard(transformDef(t), p, engine.Battlefield)
	for i, want := range []int{4, 1} {
		c := engine.NewScriptedController()
		c.QueueTargets([]engine.EntityID{engine.CardEntity(card)})
		resolveLine(t, g, p, c, "DB$ SetState | ValidTgts$ Creature.powerGE0+toughnessGE1+cmcEQ0+nonToken+YouCtrl | Mode$ Transform")
		if pw, _ := g.Card(card).Power(); pw != want {
			t.Errorf("after transform %d power = %d, want %d", i+1, pw, want)
		}
	}
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(card)})
	resolveLine(t, g, p, c, "DB$ SetState | ValidTgts$ Creature | Mode$ Transform")
	if g.Card(card).Def.Name != "Back" {
		t.Fatalf("name = %q, want Back", g.Card(card).Def.Name)
	}
	g.Move(card, engine.Graveyard, p)
	if g.Card(card).Def.Name != "Front" {
		t.Error("left the battlefield transformed")
	}
}

// goadedGame is a three-player game on ps[1]'s turn whose one creature ps[0]
// goaded during its own turn.
func goadedGame(t *testing.T) (*engine.Game, []engine.PlayerID, engine.CardID) {
	t.Helper()
	g := newGame(t, "a", "b", "c")
	ps := g.Players()
	for _, p := range ps {
		g.Player(p).Life = 20
	}
	g.SetTurnState(1, ps[0], engine.Main1)
	victim := g.NewCard(creatureDefPT(t, "2", "2"), ps[1], engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.CardEntity(victim)})
	resolveLine(t, g, ps[0], c, "DB$ Goad | ValidTgts$ Creature")
	if !g.Card(victim).IsGoaded() {
		t.Fatal("not goaded")
	}
	g.SetTurnState(2, ps[1], engine.DeclareAttackers)
	return g, ps, victim
}

// A goaded creature declared attacking goes at the player who did not goad
// it (CR 701.15b): the only option offered, so no target is asked for.
func TestGoadForcesAttackAwayFromGoader(t *testing.T) {
	t.Parallel()
	g, ps, victim := goadedGame(t)
	ac := engine.NewScriptedController()
	ac.QueueAttackers([]engine.CardID{victim})
	got := declareAttackers(t, g, ac)
	if len(got) != 1 || got[0] != victim {
		t.Fatalf("attackers = %v, want the goaded creature", got)
	}
	if tgt := g.AttackTarget(victim); tgt != engine.PlayerEntity(ps[2]) {
		t.Errorf("attack target = %v, want the non-goading player", tgt)
	}
}

// Leaving a goaded creature that can attack at home is an illegal
// declaration (CR 508.1d, ADR-0024), not one the engine repairs by adding it.
func TestGoadedCreatureLeftHomeIsIllegal(t *testing.T) {
	t.Parallel()
	g, _, victim := goadedGame(t)
	ac := engine.NewScriptedController()
	ac.QueueAttackers(nil)
	_, err := g.DeclareCombatAttackers(ac)
	ill := wantIllegal(t, err, "CR 508.1d")
	if len(ill.Cards) != 1 || ill.Cards[0] != victim {
		t.Errorf("cards = %v, want the goaded creature", ill.Cards)
	}
	if len(g.Attackers()) != 0 || g.Card(victim).Tapped {
		t.Errorf("attackers = %v, tapped = %v: the illegal declaration was applied", g.Attackers(), g.Card(victim).Tapped)
	}
}

func TestRemoveFromMatchByType(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	x := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Hand)
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ RemoveFromMatch | RemoveType$ Creature.OppOwn | IncludeSideboard$ True | RemoveFromInventory$ True")
	if g.Card(x).Zone != engine.None {
		t.Errorf("zone = %v, want None", g.Card(x).Zone)
	}
}

func TestActivateAbilityTapsLandsForMana(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
	resolveLine(t, g, p, c, "DB$ ActivateAbility | ValidTgts$ Player | Type$ Land | ManaAbility$ True")
	if !g.Card(forest).Tapped {
		t.Error("forest not tapped")
	}
}

func TestMultiplePilesRandomChosen(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	a := g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Battlefield)
	b := g.NewCard(creatureDefPT(t, "1", "1"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueCardChoice(nil)
	c.QueueCardChoice(nil)
	resolveLine(t, g, p, c, "DB$ MultiplePiles | Defined$ Player | Zone$ Battlefield | ValidCards$ Permanent.powerEQ1 | RandomChosen$ True | Piles$ 2 | ChosenPile$ DBSac",
		"DBSac", "DB$ SacrificeAll | ValidCards$ Permanent.IsRemembered")
	_ = a
	_ = b
}

func TestDamageMapDealsOnResolve(t *testing.T) {
	t.Parallel()
	g, p, other := newTwoPlayerGame(t)
	resolveLine(t, g, p, engine.NewScriptedController(),
		"DB$ DealDamage | Defined$ Opponent | NumDmg$ 2 | DamageMap$ True | SubAbility$ DBMore",
		"DBMore", "DB$ DealDamage | Defined$ Opponent | NumDmg$ 3 | SubAbility$ DBCheck",
		"DBCheck", "DB$ GainLife | Defined$ You | LifeAmount$ 1 | SubAbility$ DBResolve",
		"DBResolve", "DB$ DamageResolve")
	if got := g.Player(other).Life; got != 15 {
		t.Errorf("life = %d, want 15 (both hits dealt at resolve)", got)
	}
	resolveLine(t, g, p, engine.NewScriptedController(), "DB$ DamageResolve")
}

func TestDigMultipleMustChoose(t *testing.T) {
	t.Parallel()
	g, p, _ := newTwoPlayerGame(t)
	libraryCards(t, g, p, 3)
	c := engine.NewScriptedController()
	c.QueueCardChoice(nil)
	if _, err := castETBChain(t, g, p, etbChainDef(t, "Dig", "DB$ DigMultiple | DigNum$ 2 | ChangeValid$ Card"), c); err == nil {
		t.Error("an empty pick without Optional$ resolved")
	}
}
