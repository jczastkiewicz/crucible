package engine_test

import (
	"testing"

	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
	"github.com/jczastkiewicz/crucible/internal/valid"
)

// Kicker (CR 702.33a): Burst Lightning ({R}, kicker {4}) deals 2 damage, or 4
// when kicked; the caster is asked and pays the extra cost only on yes.
func TestKickerAddsItsCostAndChangesTheEffect(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		kick   bool
		reds   int
		wantOK bool
		damage int
	}{
		{"unkicked", false, 1, true, 2},
		{"kicked", true, 5, true, 4},
		{"kicked without the mana", true, 1, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, other := newTwoPlayerGameOn(t, scenarioDB(t))
			bolt := g.NewCard(corpusCard(t, "Burst Lightning"), p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Red, tc.reds)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.kick)
			for range 4 {
				c.QueuePayGeneric(mana.ShardR)
			}
			c.QueueTargets([]engine.EntityID{engine.PlayerEntity(other)})
			if got := g.CastSpell(p, bolt, c); got != tc.wantOK {
				t.Fatalf("CastSpell = %v, want %v", got, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := 20 - g.Player(other).Life; got != tc.damage {
				t.Errorf("damage dealt = %d, want %d", got, tc.damage)
			}
		})
	}
}

// A permanent keeps its kicker choice: "if CARDNAME was kicked, it enters with
// +1/+1 counters" reads the `kicked` property and Count$Kicked on the battlefield.
func TestKickedPermanentRemembersItOnTheBattlefield(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Kicker Beast", "Creature Elf", "K:Kicker:G")
	def.Faces[0].ManaCost = mana.MustParse("G")
	card := g.NewCard(def, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 2)
	c := engine.NewScriptedController()
	c.QueueConfirmPayCost(true)
	if !g.CastSpell(p, card, c) {
		t.Fatal("CastSpell failed")
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if !engine.Matches(g, g.Card(card), valid.Parse("Card.Self+kicked"), p, card) {
		t.Error("the kicked creature on the battlefield does not match the kicked property")
	}
}

// Multikicker (GameActionUtil.java:667-685): the caster chooses how many times
// to pay the cost, each on top of the mana cost; Count$TimesKicked and the
// kicked property read the amount back, and a cast the pool cannot pay fails.
func TestMultikickerPaysTheChosenNumberOfTimes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		times    int
		greens   int
		wantOK   bool
		counters int
	}{
		{"unkicked", 0, 1, true, 0},
		{"kicked twice", 2, 3, true, 2},
		{"kicked without the mana", 2, 2, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := scriptDef(t, "Test Multikicker Beast", "Creature Elf",
				"K:Multikicker:G",
				"K:etbCounter:P1P1:XKicked:no Condition:CARDNAME enters with a +1/+1 counter on it for each time it was kicked.",
				"SVar:XKicked:Count$TimesKicked")
			def.Faces[0].ManaCost = mana.MustParse("G")
			card := g.NewCard(def, p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, tc.greens)
			c := engine.NewScriptedController()
			c.QueueNumberChoice(tc.times)
			for range 4 {
				c.QueuePayGeneric(mana.ShardG)
			}
			if got := g.CastSpell(p, card, c); got != tc.wantOK {
				t.Fatalf("CastSpell = %v, want %v", got, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := p1p1(g, card); got != tc.counters {
				t.Errorf("+1/+1 counters = %d, want %d", got, tc.counters)
			}
			if kicked := engine.Matches(g, g.Card(card), valid.Parse("Card.Self+kicked"), p, card); kicked != (tc.times > 0) {
				t.Errorf("kicked property = %v after %d payments", kicked, tc.times)
			}
		})
	}
}

// Squad and Offspring (CardFactoryUtil.java:1539-1556, :1777-1789): the cost
// is paid the chosen number of times as the spell is cast, and the permanent
// entering makes that many token copies (Offspring: one copy that is 1/1).
func TestSquadAndOffspringMakeTokenCopies(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		keyword string
		choice  int
		confirm bool
		want    int // creatures on the battlefield
	}{
		{"squad twice", "K:Squad:1", 2, false, 3},
		{"squad declined", "K:Squad:1", 0, false, 1},
		{"offspring paid", "K:Offspring:1", 0, true, 2},
		{"offspring declined", "K:Offspring:1", 0, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := scriptDef(t, "Test Copier", "Creature Elf", tc.keyword)
			def.Faces[0].ManaCost = mana.MustParse("G")
			card := g.NewCard(def, p, engine.Hand)
			g.Player(p).ManaPool.Add(mana.Green, 3)
			c := engine.NewScriptedController()
			if tc.keyword == "K:Offspring:1" {
				c.QueueConfirmPayCost(tc.confirm)
			} else {
				c.QueueNumberChoice(tc.choice)
			}
			for range 4 {
				c.QueuePayGeneric(mana.ShardG)
			}
			if !g.CastSpell(p, card, c) {
				t.Fatal("CastSpell failed")
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := len(g.Zone(engine.Battlefield, p).Cards()); got != tc.want {
				t.Errorf("creatures = %d, want %d", got, tc.want)
			}
		})
	}
}

// Replicate (CardFactoryUtil.java:1698-1706): the cost is paid the chosen
// number of times and the cast trigger copies the spell once for each.
func TestReplicateCopiesTheSpellForEachPayment(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	def := scriptDef(t, "Test Replicator", "Sorcery",
		"K:Replicate:G",
		"A:SP$ GainLife | Defined$ You | LifeAmount$ 1")
	def.Faces[0].ManaCost = mana.MustParse("G")
	card := g.NewCard(def, p, engine.Hand)
	g.Player(p).ManaPool.Add(mana.Green, 3)
	c := engine.NewScriptedController()
	c.QueueNumberChoice(2)
	for range 4 {
		c.QueuePayGeneric(mana.ShardG)
	}
	if !g.CastSpell(p, card, c) {
		t.Fatalf("CastSpell failed: %v", g.TakePendingError())
	}
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 23 {
		t.Errorf("life = %d, want 23: the spell and two copies each gain 1", got)
	}
}

// Casualty and Conspire (GameActionUtil.java:630-662): the optional cost
// (sacrifice a creature with enough power, tap two creatures that share a
// color with the spell) is offered as the spell is cast, and paying it copies
// the spell.
func TestCasualtyAndConspireCopyTheSpell(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		keyword string
		pay     bool
		life    int
	}{
		{"casualty paid", "K:Casualty:1", true, 22},
		{"casualty declined", "K:Casualty:1", false, 21},
		{"conspire paid", "K:Conspire", true, 22},
		{"conspire declined", "K:Conspire", false, 21},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := scriptDef(t, "Test Copier", "Sorcery", tc.keyword, "A:SP$ GainLife | Defined$ You | LifeAmount$ 1")
			def.Faces[0].ManaCost = mana.MustParse("G")
			card := g.NewCard(def, p, engine.Hand)
			var bears []engine.CardID
			for range 2 {
				bear := scriptDef(t, "Test Grizzly", "Creature Bear")
				bear.Faces[0].ManaCost = mana.MustParse("G") // shares green with the spell
				bears = append(bears, g.NewCard(bear, p, engine.Battlefield))
			}
			g.Player(p).ManaPool.Add(mana.Green, 1)
			c := engine.NewScriptedController()
			c.QueueConfirmPayCost(tc.pay)
			if tc.pay && tc.keyword == "K:Casualty:1" {
				c.QueueSacrificeChoice(bears[:1])
			} else if tc.pay {
				c.QueueTapChoice(bears)
			}
			c.QueuePayGeneric(mana.ShardG)
			if !g.CastSpell(p, card, c) {
				t.Fatalf("CastSpell failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := g.Player(p).Life; got != tc.life {
				t.Errorf("life = %d, want %d", got, tc.life)
			}
		})
	}
}

// Squad and Replicate with a cost past mana (GameActionUtil.java:705-733): the
// whole cost is added to the spell once per repetition, so the discard or
// energy part is paid that many times too.
func TestKeywordRepeatsPayNonManaPartsEachTime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, keyword string
		hand, energy  int
		choice        int
		wantHand      int
		wantEnergy    int
		wantCreatures int
	}{
		{"squad discards twice", "K:Squad:1 Discard<1/Card>", 3, 0, 2, 1, 0, 3},
		{"squad capped by the hand", "K:Squad:1 Discard<1/Card>", 1, 0, 1, 0, 0, 2},
		{"squad pays energy twice", "K:Squad:PayEnergy<2>", 0, 5, 2, 0, 1, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, p, _ := newTwoPlayerGame(t)
			def := scriptDef(t, "Test Costly Copier", "Creature Elf", tc.keyword)
			def.Faces[0].ManaCost = mana.MustParse("G")
			card := g.NewCard(def, p, engine.Hand)
			var others []engine.CardID
			for range tc.hand {
				others = append(others, g.NewCard(creatureDefPT(t, "1", "1"), p, engine.Hand))
			}
			g.Player(p).Counters.Add(engine.Energy, tc.energy)
			g.Player(p).ManaPool.Add(mana.Green, 3)
			c := engine.NewScriptedController()
			c.QueueNumberChoice(tc.choice)
			if tc.hand > 0 {
				c.QueueDiscardChoice(others[:tc.choice])
			}
			for range 4 {
				c.QueuePayGeneric(mana.ShardG)
			}
			if !g.CastSpell(p, card, c) {
				t.Fatalf("CastSpell failed: %v", g.TakePendingError())
			}
			if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
				t.Fatalf("ResolveStack: %v", err)
			}
			if got := len(g.Zone(engine.Hand, p).Cards()); got != tc.wantHand {
				t.Errorf("hand = %d, want %d", got, tc.wantHand)
			}
			if got := g.Player(p).Counters.Count(engine.Energy); got != tc.wantEnergy {
				t.Errorf("energy = %d, want %d", got, tc.wantEnergy)
			}
			if got := len(g.Zone(engine.Battlefield, p).Cards()); got != tc.wantCreatures {
				t.Errorf("creatures = %d, want %d", got, tc.wantCreatures)
			}
		})
	}
}
