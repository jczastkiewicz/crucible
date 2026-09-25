package engine_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jczastkiewicz/crucible/internal/carddb"
	"github.com/jczastkiewicz/crucible/internal/carddb/compile"
	"github.com/jczastkiewicz/crucible/internal/cardtype"
	"github.com/jczastkiewicz/crucible/internal/engine"
	"github.com/jczastkiewicz/crucible/pkg/javarand"
)

// dungeonTokens are the real res/tokenscripts files the venture tests
// read: the four dungeons and the tokens their rooms create.
var dungeonTokens = []string{
	"dungeon_of_the_mad_mage", "lost_mine_of_phandelver", "tomb_of_annihilation", "undercity",
	"r_1_1_goblin", "c_a_treasure_sac", "b_4_1_skeleton_menace",
}

// newDungeonGame is newTwoPlayerGame over a DB holding the real dungeon
// token scripts, compiled from the Forge tree.
func newDungeonGame(t *testing.T, players ...string) *engine.Game {
	t.Helper()
	root := scenarioRepoRoot(t)
	f, err := os.Open(filepath.Join(root, "forge-gui", "res", "lists", "TypeLists.txt"))
	if err != nil {
		t.Fatalf("open TypeLists: %v", err)
	}
	reg, err := cardtype.LoadRegistry(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	tokens := map[string]*compile.Card{}
	for _, script := range dungeonTokens {
		raw, err := os.ReadFile(filepath.Join(root, "forge-gui", "res", "tokenscripts", script+".txt"))
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		parsed, err := carddb.ParseScript(reg, script, raw)
		if err != nil {
			t.Fatalf("parse %s: %v", script, err)
		}
		if tokens[script], err = compile.Compile(parsed); err != nil {
			t.Fatalf("compile %s: %v", script, err)
		}
	}
	if len(players) == 0 {
		players = []string{"a", "b"}
	}
	g := engine.NewGame(compile.NewDB(nil).WithTokens(tokens), javarand.New(1), players)
	for _, pid := range g.Players() {
		g.Player(pid).Life = 20
	}
	g.SetTurnState(1, g.Players()[0], engine.Main1)
	return g
}

// resolveWith is resolveNow for any PlayerController: line resolves as
// the Execute$ of a battlefield host p controls, then the stack empties.
func resolveWith(t *testing.T, g *engine.Game, p engine.PlayerID, c engine.PlayerController, line string, svars ...string) error {
	t.Helper()
	def := etbChainDef(t, "Test Venture", line, svars...)
	host := g.NewCard(def, p, engine.Battlefield)
	sub := def.Faces[0].Triggers[0].Subs[0].Ability
	api, ok := engine.APIByName(sub.Name)
	if !ok {
		t.Fatalf("unknown API %q", sub.Name)
	}
	g.PushAbility(engine.Ability{API: api, Source: host, Controller: p, Params: sub, Amounts: def.Faces[0].Amounts})
	return g.ResolveStack(engine.NewRegistry(), c)
}

// optionRecorder records every ChooseOption offer.
type optionRecorder struct {
	*engine.ScriptedController
	offers [][]string
}

func (r *optionRecorder) ChooseOption(g *engine.Game, p engine.PlayerID, src engine.CardID, options []string) int {
	r.offers = append(r.offers, append([]string(nil), options...))
	return r.ScriptedController.ChooseOption(g, p, src, options)
}

// dungeonOf is p's dungeon in the Command zone, or NoCard.
func dungeonOf(g *engine.Game, p engine.PlayerID) engine.CardID {
	for _, id := range g.Zone(engine.Command, p).Cards() {
		if g.Card(id).Def.Faces[0].Type.Has(cardtype.Dungeon) {
			return id
		}
	}
	return engine.NoCard
}

// TestVentureWalksLostMineToCompletion proves the dominant shape (DB$
// Venture, 37 of 46 corpus lines with Defined$ You or none): the first
// venture enters a dungeon the player picks among the enterable ones
// (Undercity is not), sorted by name; each venture moves the marker to the
// next room -- the player picking where the room branches -- and runs that
// room's ability; once the marker sits on the last room and its ability
// has resolved, the dungeon is completed (CR 309.7) and leaves the game.
func TestVentureWalksLostMineToCompletion(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p, other := g.Players()[0], g.Players()[1]
	lib := libraryCards(t, g, p, 3)
	sc := engine.NewScriptedController()
	c := &optionRecorder{ScriptedController: sc}

	sc.QueueOption(1)
	sc.QueueScry([]engine.CardID{lib[0]}, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture | Defined$ You"); err != nil {
		t.Fatalf("venture 1: %v", err)
	}
	want := []string{"Dungeon of the Mad Mage", "Lost Mine of Phandelver", "Tomb of Annihilation"}
	if len(c.offers) != 1 || !reflect.DeepEqual(c.offers[0], want) {
		t.Fatalf("dungeon offers = %v, want one offer of %v", c.offers, want)
	}
	d := dungeonOf(g, p)
	if d == engine.NoCard || g.Card(d).Def.Name != "Lost Mine of Phandelver" || g.Card(d).CurrentRoom != "Cave Entrance" {
		t.Fatalf("dungeon %v, want Lost Mine on Cave Entrance", d)
	}

	sc.QueueAbilityChoice([]int{0})
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture 2: %v", err)
	}
	if g.Card(d).CurrentRoom != "Goblin Lair" || len(tokensOn(g, p, "Goblin Token")) != 1 {
		t.Fatalf("room %q goblins %d, want Goblin Lair and one Goblin", g.Card(d).CurrentRoom, len(tokensOn(g, p, "Goblin Token")))
	}

	sc.QueueAbilityChoice([]int{1})
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture 3: %v", err)
	}
	if g.Card(d).CurrentRoom != "Dark Pool" || g.Player(other).Life != 19 || g.Player(p).Life != 21 {
		t.Fatalf("room %q life %d/%d, want Dark Pool and 21/19", g.Card(d).CurrentRoom, g.Player(p).Life, g.Player(other).Life)
	}

	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture 4: %v", err)
	}
	if g.Card(lib[1]).Zone != engine.Hand && g.Card(lib[0]).Zone != engine.Hand {
		t.Error("Temple of Dumathoin drew no card")
	}
	if got := g.CompletedDungeons(p); len(got) != 1 || got[0] != d {
		t.Errorf("completed = %v, want [%v]", got, d)
	}
	if g.Card(d).Zone != engine.None || dungeonOf(g, p) != engine.NoCard {
		t.Errorf("dungeon zone %v, want None once completed", g.Card(d).Zone)
	}
	if n := g.Player(p).VenturedThisTurn; n != 4 {
		t.Errorf("VenturedThisTurn = %d, want 4", n)
	}
	endTurn(g, 1, p)
	if n := g.Player(p).VenturedThisTurn; n != 0 {
		t.Errorf("VenturedThisTurn after cleanup = %d, want 0", n)
	}
}

// TestVentureIntoUndercityOffersOnlyUndercity proves Dungeon$ (the
// initiative's "venture into Undercity"): only dungeons of that subtype
// are offered, and the chooser is still asked, as Java asks
// chooseSingleCardFace over a one-face list.
func TestVentureIntoUndercityOffersOnlyUndercity(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	sc := engine.NewScriptedController()
	c := &optionRecorder{ScriptedController: sc}
	sc.QueueOption(0)
	sc.QueueCardChoice(nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture | Dungeon$ Undercity"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	if len(c.offers) != 1 || !reflect.DeepEqual(c.offers[0], []string{"Undercity"}) {
		t.Errorf("offers = %v, want [[Undercity]]", c.offers)
	}
	if d := dungeonOf(g, p); d == engine.NoCard || g.Card(d).CurrentRoom != "Secret Entrance" {
		t.Errorf("dungeon %v, want Undercity on Secret Entrance", d)
	}
}

// TestVentureFromTheLastRoomCompletesFirst proves getDungeonCard's other
// completion path: a venture while the marker already sits on the last
// room (its ability gone from the stack but no SBA run since) completes
// that dungeon, then enters a new one.
func TestVentureFromTheLastRoomCompletesFirst(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	first := dungeonOf(g, p)
	g.Card(first).CurrentRoom = "Temple of Dumathoin"

	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	second := dungeonOf(g, p)
	if got := g.CompletedDungeons(p); len(got) != 1 || got[0] != first || second == first || second == engine.NoCard {
		t.Errorf("completed %v, dungeon %v, want [%v] completed and a new one", got, second, first)
	}
}

// TestCantVentureMoreThanOnceEachTurn proves Keen-Eared Sentry's static
// (Mode$ CantVenture | ValidPlayer$ Opponent.VenturedThisTurn): an
// opponent's first venture goes through, the second does nothing; the
// static's controller is not limited.
func TestCantVentureMoreThanOnceEachTurn(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p, other := g.Players()[0], g.Players()[1]
	raw := &carddb.Card{Filename: "sentry"}
	raw.Faces[0].Present = true
	raw.Faces[0].Name = "Sentry"
	raw.Faces[0].Type = cardtype.Parse(attachmentTypeRegistry(t), "Enchantment")
	raw.Faces[0].Statics = []string{"Mode$ CantVenture | ValidPlayer$ Opponent.VenturedThisTurn | Description$ x"}
	def, err := compile.Compile(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	g.NewCard(def, p, engine.Battlefield)

	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, other, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	if err := resolveWith(t, g, other, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	if n := g.Player(other).VenturedThisTurn; n != 1 {
		t.Errorf("opponent ventured %d times, want 1", n)
	}
	c.QueueOption(1)
	c.QueueScry(nil, nil)
	c.QueueAbilityChoice([]int{0})
	for i := 0; i < 2; i++ {
		if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
			t.Fatalf("venture: %v", err)
		}
	}
	if n := g.Player(p).VenturedThisTurn; n != 2 {
		t.Errorf("static's controller ventured %d times, want 2", n)
	}
}

// TestDungeonCompletedTriggerMode proves Mode$ DungeonCompleted
// (TriggerCompletedDungeon): ValidPlayer$ against who completed it, and
// Defined$ TriggeredPlayer naming them.
func TestDungeonCompletedTriggerMode(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	g.NewCard(triggerWatcherDef(t, "Watcher",
		"Mode$ DungeonCompleted | ValidPlayer$ You | TriggerZones$ Battlefield | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ TriggeredPlayer | LifeAmount$ 5"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	g.Card(dungeonOf(g, p)).CurrentRoom = "Temple of Dumathoin"
	engine.CheckStateBasedActions(g, c)
	if err := g.ResolveStack(engine.NewRegistry(), c); err != nil {
		t.Fatalf("ResolveStack: %v", err)
	}
	if got := g.Player(p).Life; got != 25 {
		t.Errorf("life = %d, want 25", got)
	}
}

// TestRoomEnteredTriggerOnAnotherCard proves Mode$ RoomEntered on a
// battlefield card: ValidCard$ is read against the dungeon (Card matches,
// Card.Self -- the watcher itself -- does not), and a line without
// ValidRoom$ fires for every room.
func TestRoomEnteredTriggerOnAnotherCard(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	g.NewCard(triggerWatcherDef(t, "Any Room",
		"Mode$ RoomEntered | ValidCard$ Card | TriggerZones$ Battlefield | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 1"), p, engine.Battlefield)
	g.NewCard(triggerWatcherDef(t, "Self Only",
		"Mode$ RoomEntered | ValidCard$ Card.Self | TriggerZones$ Battlefield | Execute$ TrigGain",
		"TrigGain", "DB$ GainLife | Defined$ You | LifeAmount$ 100"), p, engine.Battlefield)
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	if got := g.Player(p).Life; got != 21 {
		t.Errorf("life = %d, want 21", got)
	}
}

// TestVentureSkipsUnmetConditionAndLostTarget proves the two ways a
// venture does nothing: an unmet sub-ability condition, and a targeted
// player who has lost (VentureEffect's isInGame check).
func TestVentureSkipsUnmetConditionAndLostTarget(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t, "a", "b", "c")
	p, lost := g.Players()[0], g.Players()[1]
	c := engine.NewScriptedController()
	if err := resolveWith(t, g, p, c, "DB$ Venture | ConditionPlayerTurn$ False"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	g.Player(lost).Lost = true
	c.QueueTargets([]engine.EntityID{engine.PlayerEntity(lost)})
	if err := resolveWith(t, g, p, c, "DB$ Venture | ValidTgts$ Player"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	if dungeonOf(g, p) != engine.NoCard || dungeonOf(g, lost) != engine.NoCard {
		t.Error("a dungeon was entered")
	}
	if err := resolveWith(t, g, p, c, "DB$ Venture | Defined$ TriggeredPlayer"); err == nil {
		t.Error("Defined$ TriggeredPlayer outside a trigger: err = nil")
	}
}

// TestVentureStateSurvivesClone proves Game.Clone copies the marker and
// the completed list, independently.
func TestVentureStateSurvivesClone(t *testing.T) {
	t.Parallel()

	g := newDungeonGame(t)
	p := g.Players()[0]
	c := engine.NewScriptedController()
	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	d := dungeonOf(g, p)
	g.Card(d).CurrentRoom = "Temple of Dumathoin"
	engine.CheckStateBasedActions(g, c)
	clone := g.Clone()
	if len(clone.CompletedDungeons(p)) != 1 {
		t.Fatalf("clone completed = %v, want one", clone.CompletedDungeons(p))
	}
	clone.Card(d).CurrentRoom = "elsewhere"
	if g.Card(d).CurrentRoom != "Temple of Dumathoin" {
		t.Error("the clone's marker moved the original's")
	}
}

// TestVentureFailsClosed proves the shapes that error: no dungeon in the
// database, a dungeon choice out of range, a room choice out of range, and
// ConditionDefined$.
func TestVentureFailsClosed(t *testing.T) {
	t.Parallel()

	g, p, _ := newTwoPlayerGame(t)
	if err := resolveWith(t, g, p, engine.NewScriptedController(), "DB$ Venture"); err == nil ||
		!strings.Contains(err.Error(), "no dungeon") {
		t.Errorf("nil DB: err = %v, want no dungeon", err)
	}

	g = newDungeonGame(t)
	p = g.Players()[0]
	c := engine.NewScriptedController()
	c.QueueOption(7)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err == nil {
		t.Error("dungeon choice 7: err = nil")
	}

	c.QueueOption(1)
	c.QueueScry(nil, nil)
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err != nil {
		t.Fatalf("venture: %v", err)
	}
	c.QueueAbilityChoice([]int{5})
	if err := resolveWith(t, g, p, c, "DB$ Venture"); err == nil {
		t.Error("room choice 5: err = nil")
	}

	if err := resolveWith(t, g, p, c, "DB$ Venture | ConditionDefined$ Remembered | ConditionPresent$ Card"); err == nil {
		t.Error("ConditionDefined$: err = nil")
	}
}
