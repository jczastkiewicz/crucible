// A scenario's actions.log: the ordered, explicit decisions TEST-5 fixtures
// script against a loaded game.

package fixture

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jczastkiewicz/crucible/internal/engine"
)

// RunActions executes a TEST-5 actions.log against a loaded game.
//
// There is no Java counterpart: Plan Section 3.3's Layer 2 names
// setup.state/actions.log/expected as the scenario shape, but the line
// format inside actions.log is Crucible's own -- Forge's differential
// tooling reads setup.state and dumps expected/ independently, and never
// needs a scripted-action format of its own because it drives a real
// PlayerController from Java code, not from text.
//
// Because no AI is involved, this is what TEST-5 promises: any divergence
// between the resulting game and expect.state is a rules bug, never an AI
// one, the same reasoning Layer 3's replay parity applies to a full game
// (Plan Section 3.3).
//
// A blank line or one starting with # is skipped, matching setup.state's own
// convention. Each other line is one action:
//
//	startturn <player>            Game.StartTurn(player, controller)
//	advance [n]                   Game.AdvancePhase(controller), n times (default 1)
//	mulligan <firstplayer>        PerformMulligans(game, controller, firstplayer)
//	declareattackers              Game.DeclareCombatAttackers(controller)
//	declareblockers               Game.DeclareCombatBlockers(controller)
//	firststrikedamage             Game.DealFirstStrikeDamage(controller)
//	combatdamage                  Game.DealCombatDamage(controller)
//	queue keephand <bool>         ScriptedController.QueueKeepHand
//	queue tuck <id>[,<id>...]     ScriptedController.QueueTuck, ids from CardByFixtureID
//	queue startingplayer <p>      ScriptedController.QueueStartingPlayer
//	queue startinghand <n>        ScriptedController.QueueStartingHand
//	queue legendarykeep <id>      ScriptedController.QueueLegendaryToKeep, id from CardByFixtureID
//	queue attackers [<id>,...]    ScriptedController.QueueAttackers, ids from CardByFixtureID (no ids declines)
//	queue blocks [<b>=<a>,...]    ScriptedController.QueueBlocks, blocker=attacker pairs from CardByFixtureID (no pairs declines)
//	queue damage <b>=<n>[,...]    ScriptedController.QueueDamageAssignment, blocker=amount pairs from CardByFixtureID
//
// A scenario that needs a decision point no verb here reaches -- casting
// anything -- cannot be written yet, because nothing downstream of
// ScriptedController can answer it either (M5, later).
func RunActions(r io.Reader, l *Loaded, controller *engine.ScriptedController) error {
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if err := runAction(text, l, controller); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
	return sc.Err()
}

func runAction(line string, l *Loaded, c *engine.ScriptedController) error {
	fields := strings.Fields(line)
	verb, args := fields[0], fields[1:]

	switch verb {
	case "startturn":
		pid, err := resolveActionPlayer(l, args, 1)
		if err != nil {
			return err
		}
		l.Game.StartTurn(pid, c)

	case "advance":
		n := 1
		if len(args) > 0 {
			v, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("advance %q: %w", args[0], err)
			}
			n = v
		}
		for i := 0; i < n; i++ {
			l.Game.AdvancePhase(c)
		}

	case "mulligan":
		pid, err := resolveActionPlayer(l, args, 1)
		if err != nil {
			return err
		}
		engine.PerformMulligans(l.Game, c, pid)

	case "declareattackers":
		l.Game.DeclareCombatAttackers(c)

	case "declareblockers":
		l.Game.DeclareCombatBlockers(c)

	case "firststrikedamage":
		l.Game.DealFirstStrikeDamage(c)

	case "combatdamage":
		l.Game.DealCombatDamage(c)

	case "queue":
		return runQueue(args, l, c)

	default:
		return fmt.Errorf("unknown action %q", verb)
	}
	return nil
}

func runQueue(args []string, l *Loaded, c *engine.ScriptedController) error {
	if len(args) < 2 {
		return fmt.Errorf("queue: want a kind and a value, got %q", strings.Join(args, " "))
	}
	kind, value := args[0], args[1]

	switch kind {
	case "keephand":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("queue keephand %q: %w", value, err)
		}
		c.QueueKeepHand(v)

	case "tuck":
		ids, err := resolveCardIDs(l, value)
		if err != nil {
			return fmt.Errorf("queue tuck: %w", err)
		}
		c.QueueTuck(ids)

	case "startingplayer":
		pid, err := resolveActionPlayer(l, args[1:], 1)
		if err != nil {
			return err
		}
		c.QueueStartingPlayer(pid)

	case "startinghand":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("queue startinghand %q: %w", value, err)
		}
		c.QueueStartingHand(n)

	case "legendarykeep":
		ids, err := resolveCardIDs(l, value)
		if err != nil {
			return fmt.Errorf("queue legendarykeep: %w", err)
		}
		if len(ids) != 1 {
			return fmt.Errorf("queue legendarykeep: want exactly one id, got %q", value)
		}
		c.QueueLegendaryToKeep(ids[0])

	case "attackers":
		// "none" is written explicitly, not an empty value, because every
		// other queue kind requires a value too (the len(args) < 2 check
		// above) -- declining to attack with anything is still an answer
		// that has to be queued, not an absent one.
		if value == "none" {
			c.QueueAttackers(nil)
			break
		}
		ids, err := resolveCardIDs(l, value)
		if err != nil {
			return fmt.Errorf("queue attackers: %w", err)
		}
		c.QueueAttackers(ids)

	case "blocks":
		// "none" mirrors "attackers none" above: declining to block is a
		// queued answer too, not an absent one.
		if value == "none" {
			c.QueueBlocks(nil)
			break
		}
		blocks, err := resolveBlocks(l, value)
		if err != nil {
			return fmt.Errorf("queue blocks: %w", err)
		}
		c.QueueBlocks(blocks)

	case "damage":
		assignment, err := resolveDamageAssignment(l, value)
		if err != nil {
			return fmt.Errorf("queue damage: %w", err)
		}
		c.QueueDamageAssignment(assignment)

	default:
		return fmt.Errorf("unknown queue kind %q", kind)
	}
	return nil
}

// resolveActionPlayer looks args[i] up as a seated player's name -- human,
// ai, p2..p9, the same vocabulary setup.state itself uses.
func resolveActionPlayer(l *Loaded, args []string, want int) (engine.PlayerID, error) {
	if len(args) < want {
		return engine.NoPlayer, fmt.Errorf("want a player name, got %q", strings.Join(args, " "))
	}
	name := strings.ToLower(args[want-1])
	for _, pid := range l.Game.Players() {
		if l.Game.Player(pid).Name == name {
			return pid, nil
		}
	}
	return engine.NoPlayer, fmt.Errorf("player %q is not seated in this game", name)
}

// resolveCardIDs turns a comma-separated list of setup.state Id: numbers
// into the CardIDs Load assigned them, in the order written.
func resolveCardIDs(l *Loaded, value string) ([]engine.CardID, error) {
	parts := strings.Split(value, ",")
	ids := make([]engine.CardID, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("id %q: %w", p, err)
		}
		id, ok := l.CardByFixtureID[n]
		if !ok {
			return nil, fmt.Errorf("id %d: no card in setup.state has that Id:", n)
		}
		ids[i] = id
	}
	return ids, nil
}

// resolveBlocks turns a comma-separated list of blocker=attacker fixture-ID
// pairs into the Blocks Load's ids resolve to.
func resolveBlocks(l *Loaded, value string) ([]engine.Block, error) {
	pairs := strings.Split(value, ",")
	blocks := make([]engine.Block, len(pairs))
	for i, p := range pairs {
		halves := strings.SplitN(p, "=", 2)
		if len(halves) != 2 {
			return nil, fmt.Errorf("pair %q: want blocker=attacker", p)
		}
		ids, err := resolveCardIDs(l, halves[0]+","+halves[1])
		if err != nil {
			return nil, err
		}
		blocks[i] = engine.Block{Blocker: ids[0], Attacker: ids[1]}
	}
	return blocks, nil
}

// resolveDamageAssignment turns a comma-separated list of blocker=amount
// pairs into the []engine.DamageAssignment AssignCombatDamage expects, in
// the order written -- that order is the order the attacking player assigns
// in (CR 510.1c), so unlike resolveBlocks this cannot reorder its pairs.
func resolveDamageAssignment(l *Loaded, value string) ([]engine.DamageAssignment, error) {
	pairs := strings.Split(value, ",")
	assignment := make([]engine.DamageAssignment, len(pairs))
	for i, p := range pairs {
		halves := strings.SplitN(p, "=", 2)
		if len(halves) != 2 {
			return nil, fmt.Errorf("pair %q: want blocker=amount", p)
		}
		ids, err := resolveCardIDs(l, halves[0])
		if err != nil {
			return nil, err
		}
		amount, err := strconv.Atoi(strings.TrimSpace(halves[1]))
		if err != nil {
			return nil, fmt.Errorf("amount %q: %w", halves[1], err)
		}
		assignment[i] = engine.DamageAssignment{Blocker: ids[0], Amount: amount}
	}
	return assignment, nil
}
