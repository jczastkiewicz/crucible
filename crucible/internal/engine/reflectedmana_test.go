package engine_test

import (
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/internal/mana"
)

// These tests cover ManaReflected's ReflectProperty$ Produced shape: a
// Static$ True Mode$ TapsForMana trigger adds one more mana of the type the
// tapped permanent produced (CardUtil.java:295-307), resolved at the trigger
// site (ADR-0020).

// scriptDef compiles a one-face card from script lines: "T:" a trigger,
// "A:" an ability, "K:" a keyword, "SVar:Name:body" an SVar.
func scriptDef(t *testing.T, name, typeLine string, lines ...string) *compile.Card {
	t.Helper()
	raw := &carddb.Card{Filename: name}
	f := &raw.Faces[0]
	f.Present = true
	f.Name = name
	f.Type = cardtype.Parse(attachmentTypeRegistry(t), typeLine)
	if strings.Contains(typeLine, "Creature") {
		f.Power, f.Toughness = "1", "1"
	}
	for _, l := range lines {
		head, body, _ := strings.Cut(l, ":")
		switch head {
		case "T":
			f.Triggers = append(f.Triggers, body)
		case "A":
			f.Abilities = append(f.Abilities, body)
		case "K":
			f.Keywords = append(f.Keywords, body)
		case "SVar":
			k, v, _ := strings.Cut(body, ":")
			f.SVars.Set(k, v)
		default:
			t.Fatalf("scriptDef: unknown line %q", l)
		}
	}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile %q: %v", name, err)
	}
	return def
}

// manaFlareLikeDef is mana_flare.txt with the reflected line's params
// swapped for reflect: "Whenever a player taps a land for mana, that player
// adds one mana of any type that land produced."
func manaFlareLikeDef(t *testing.T, trigger, reflect string, svars ...string) *compile.Card {
	lines := []string{
		"T:Mode$ TapsForMana | " + trigger + " | Execute$ TrigMana | TriggerZones$ Battlefield | Static$ True",
		"SVar:TrigMana:DB$ ManaReflected | " + reflect,
	}
	return scriptDef(t, "Test Flare", "Enchantment", append(lines, svars...)...)
}

// Mana Flare: each player's tapped land doubles for that player.
func TestManaFlareReflectsEachTappersColor(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(manaFlareLikeDef(t, "ValidCard$ Land",
		"ColorOrType$ Type | ReflectProperty$ Produced | Defined$ TriggeredActivator"), p, engine.Battlefield)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	island := g.NewCard(landDef(t, "Island", "Basic Land Island"), other, engine.Battlefield)
	c := engine.NewScriptedController()

	if !g.TapLandForMana(p, forest, mana.Green, c) || !g.TapLandForMana(other, island, mana.Blue, c) {
		t.Fatal("TapLandForMana failed")
	}
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("TakePendingError: %v", err)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 0, 2, 0} {
		t.Errorf("tapper pool = %v, want two green", got)
	}
	if got := g.Player(other).ManaPool.Breakdown(); got != [6]int{0, 2, 0, 0, 0, 0} {
		t.Errorf("opponent pool = %v, want two blue", got)
	}
	if got := g.StackLen(); got != 0 {
		t.Errorf("StackLen() = %d, want 0", got)
	}
}

// ColorOrType$ Type reflects colorless too (maxChoices 6); Color does not.
// A mana ability's own Produced$ C tap is the colorless case.
func TestManaReflectedColorlessOnlyUnderType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		colorOrType string
		want        [6]int
	}{
		{"Type", [6]int{0, 0, 0, 0, 0, 2}},
		{"Color", [6]int{0, 0, 0, 0, 0, 1}},
	} {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(manaFlareLikeDef(t, "ValidCard$ Land",
			"ColorOrType$ "+tc.colorOrType+" | ReflectProperty$ Produced | Defined$ TriggeredActivator"), p, engine.Battlefield)
		wastes := g.NewCard(scriptDef(t, "Test Wastes", "Land", "A:AB$ Mana | Cost$ T | Produced$ C"), p, engine.Battlefield)
		if !g.ActivateManaAbility(p, wastes, 0, engine.NewScriptedController()) {
			t.Fatal("ActivateManaAbility failed")
		}
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("%s: TakePendingError: %v", tc.colorOrType, err)
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != tc.want {
			t.Errorf("%s: pool = %v, want %v", tc.colorOrType, got, tc.want)
		}
	}
}

// Zendikar Resurgent: Activator$ You, Defined$ You -- an opponent's tap
// fires nothing.
func TestZendikarResurgentReflectsOnlyItsControllersLands(t *testing.T) {
	t.Parallel()

	g, p, other := newTwoPlayerGame(t)
	g.NewCard(manaFlareLikeDef(t, "ValidCard$ Land | Activator$ You",
		"ColorOrType$ Type | ReflectProperty$ Produced | Defined$ You"), p, engine.Battlefield)
	mountain := g.NewCard(landDef(t, "Mountain", "Basic Land Mountain"), p, engine.Battlefield)
	island := g.NewCard(landDef(t, "Island", "Basic Land Island"), other, engine.Battlefield)
	c := engine.NewScriptedController()
	g.TapLandForMana(p, mountain, mana.Red, c)
	g.TapLandForMana(other, island, mana.Blue, c)
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("TakePendingError: %v", err)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 0, 2, 0, 0} {
		t.Errorf("controller pool = %v, want two red", got)
	}
	if got := g.Player(other).ManaPool.Breakdown(); got != [6]int{0, 1, 0, 0, 0, 0} {
		t.Errorf("opponent pool = %v, want one blue", got)
	}
}

// Overabundance chains ManaReflected under DealDamage: the sub-ability
// still reads the triggering mana, since a chained ability inherits its
// parent's triggered objects.
func TestOverabundanceChainedReflectionReadsTheTrigger(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(scriptDef(t, "Test Overabundance", "Enchantment",
		"T:Mode$ TapsForMana | ValidCard$ Land | Execute$ TrigDmg | TriggerZones$ Battlefield | Static$ True",
		"SVar:TrigDmg:DB$ DealDamage | Defined$ TriggeredActivator | NumDmg$ 1 | SubAbility$ DBMana",
		"SVar:DBMana:DB$ ManaReflected | ColorOrType$ Type | ReflectProperty$ Produced | Defined$ TriggeredActivator"), p, engine.Battlefield)
	swamp := g.NewCard(landDef(t, "Swamp", "Basic Land Swamp"), p, engine.Battlefield)
	g.TapLandForMana(p, swamp, mana.Black, engine.NewScriptedController())
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("TakePendingError: %v", err)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{0, 0, 2, 0, 0, 0} {
		t.Errorf("pool = %v, want two black", got)
	}
	if got := g.Player(p).Life; got != 19 {
		t.Errorf("life = %d, want 19", got)
	}
}

// Amount$ multiplies the reflected type; zero adds nothing.
func TestManaReflectedAmount(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		amount string
		want   [6]int
	}{
		{"2", [6]int{0, 0, 0, 0, 3, 0}},
		{"0", [6]int{0, 0, 0, 0, 1, 0}},
	} {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(manaFlareLikeDef(t, "ValidCard$ Land",
			"ColorOrType$ Type | ReflectProperty$ Produced | Defined$ TriggeredActivator | Amount$ "+tc.amount), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController())
		if err := g.TakePendingError(); err != nil {
			t.Fatalf("Amount$ %s: TakePendingError: %v", tc.amount, err)
		}
		if got := g.Player(p).ManaPool.Breakdown(); got != tc.want {
			t.Errorf("Amount$ %s: pool = %v, want %v", tc.amount, got, tc.want)
		}
	}
}

// A land whose mana was replaced away produced nothing, so there is no type
// to reflect.
func TestManaReflectedWithNothingProduced(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	g.NewCard(replacementEnchantmentDefWithSVar(t, "Test Mana Void",
		"Event$ ProduceMana | ActiveZones$ Battlefield | ValidCard$ Land | ReplaceWith$ ProduceNone | Description$ Nothing.",
		"ProduceNone", "DB$ ReplaceMana | ReplaceAmount$ 0"), p, engine.Battlefield)
	g.NewCard(manaFlareLikeDef(t, "ValidCard$ Land",
		"ColorOrType$ Type | ReflectProperty$ Produced | Defined$ TriggeredActivator"), p, engine.Battlefield)
	forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
	g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController())
	if err := g.TakePendingError(); err != nil {
		t.Fatalf("TakePendingError: %v", err)
	}
	if got := g.Player(p).ManaPool.Breakdown(); got != [6]int{} {
		t.Errorf("pool = %v, want empty", got)
	}
}

// Every shape past Produced fails loudly through the static trigger's
// pending error.
func TestManaReflectedRejectsUnportedShapes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		reflect string
		want    string
	}{
		{"ColorOrType$ Color | ReflectProperty$ Produce | Valid$ Land.YouCtrl", "Valid$ not resolvable yet"},
		{"ColorOrType$ Color | ReflectProperty$ Produce", `ReflectProperty$ "Produce" not resolvable yet`},
		{"ColorOrType$ Color | ReflectProperty$ Is", `ReflectProperty$ "Is" not resolvable yet`},
		{"ColorOrType$ Combo | ReflectProperty$ Produced", `ColorOrType$ "Combo" not resolvable`},
		{"ColorOrType$ Type | ReflectProperty$ Produced | Produced$ Combo", "Produced$ not resolvable yet"},
		{"ColorOrType$ Type | ReflectProperty$ Produced | Amount$ Bogus", `Amount$ "Bogus" is not resolvable`},
		{"ColorOrType$ Type | ReflectProperty$ Produced | Defined$ Nobody", `Defined$ "Nobody" not resolvable yet`},
	} {
		g, p, _ := newTwoPlayerGame(t)
		g.NewCard(manaFlareLikeDef(t, "ValidCard$ Land", tc.reflect), p, engine.Battlefield)
		forest := g.NewCard(landDef(t, "Forest", "Basic Land Forest"), p, engine.Battlefield)
		g.TapLandForMana(p, forest, mana.Green, engine.NewScriptedController())
		err := g.TakePendingError()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: TakePendingError = %v, want one containing %q", tc.reflect, err, tc.want)
		}
	}
}

// Outside Mode$ TapsForMana there is no triggering Produced to read; Java
// throws there, this port errors.
func TestManaReflectedOutsideTapsForManaErrors(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	_, err := resolveNow(t, g, p, engine.NewScriptedController(), nil,
		"DB$ ManaReflected | ColorOrType$ Type | ReflectProperty$ Produced | Defined$ You")
	if err == nil || !strings.Contains(err.Error(), "the trigger recorded no produced mana") {
		t.Fatalf("resolve = %v, want the no-produced error", err)
	}
}

// Reflecting Pool's A:AB$ ManaReflected is a mana ability: ActivateAbility
// never puts it on the stack, and ActivateManaAbility declines it until the
// Produce walk is ported.
func TestReflectingPoolIsNotActivatable(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	pool := g.NewCard(scriptDef(t, "Test Reflecting Pool", "Land",
		"A:AB$ ManaReflected | Cost$ T | Valid$ Land.YouCtrl | ColorOrType$ Type | ReflectProperty$ Produce"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	if g.ActivateAbility(p, pool, 0, c) {
		t.Error("ActivateAbility(Reflecting Pool) = true, want false -- a mana ability never uses the stack")
	}
	if g.ActivateManaAbility(p, pool, 0, c) {
		t.Error("ActivateManaAbility(Reflecting Pool) = true, want false -- the Produce walk is not ported")
	}
	if g.StackLen() != 0 || g.Card(pool).Tapped {
		t.Errorf("stack = %d, tapped = %v; want nothing done", g.StackLen(), g.Card(pool).Tapped)
	}
}
