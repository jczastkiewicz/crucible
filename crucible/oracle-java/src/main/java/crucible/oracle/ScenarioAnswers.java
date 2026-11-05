package crucible.oracle;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.Set;

import forge.game.Game;
import forge.game.GameEntity;
import forge.game.card.Card;
import forge.game.phase.PhaseType;
import forge.game.player.Player;

/**
 * The actions.log queue lines the L2 runner maps (l2-verbs.md, queue kinds),
 * shared by every seat's {@link ScenarioController} the way Go's one
 * ScriptedController answers for every player: each decision pops the next
 * answer of its kind. Card ids are setup.state's Id: handles.
 */
final class ScenarioAnswers {
    /** The queue kinds mapped so far; a scenario queueing any other is unmapped. */
    static final Set<String> KINDS = Set.of("attackers", "blocks", "attacktarget", "targets", "paygeneric");

    /**
     * A tapformana, castspell or playland line, with the turn and phase Go
     * ran it in; afterResolve when a resolvestack line came before it since
     * the previous play, so it waits for the stack to empty.
     */
    record Play(String verb, String player, int turn, PhaseType phase, int id, String color, boolean afterResolve) {
    }

    private final Deque<List<Integer>> attackers = new ArrayDeque<>();
    private final Deque<List<int[]>> blocks = new ArrayDeque<>();
    private final Deque<String> attackTargets = new ArrayDeque<>();
    private final Deque<List<String>> targets = new ArrayDeque<>();
    private final Deque<Play> plays = new ArrayDeque<>();
    private boolean resolved;
    private Game game;
    private Map<Integer, Card> idToCard;

    /** Queues one `queue <kind> <value>` line (fixture/actions.go's grammar). */
    void queue(String kind, String value) {
        switch (kind) {
            case "attackers" -> attackers.add(value.equals("none") ? List.of() : ids(value));
            case "blocks" -> {
                List<int[]> pairs = new ArrayList<>();
                if (!value.equals("none")) {
                    for (String pair : value.split(",")) {
                        String[] half = pair.split("=", 2);
                        pairs.add(new int[] {Integer.parseInt(half[0].strip()), Integer.parseInt(half[1].strip())});
                    }
                }
                blocks.add(pairs);
            }
            case "attacktarget" -> attackTargets.add(value.strip());
            case "targets" -> {
                if (value.contains("stack:")) {
                    throw new IllegalArgumentException("a stack-item target");
                }
                targets.add(value.strip().equals("none") ? List.of() : List.of(value.strip().split(",")));
            }
            // Which pool mana pays a generic shard: payManaCost pays from
            // the pool in Java's own order (ManaPool.payManaCostFromPool).
            case "paygeneric" -> {
            }
            default -> throw new IllegalStateException("queue kind " + kind);
        }
    }

    /** Queues a play line; args are the line's words after the verb. */
    void play(String verb, String[] args, int turn, PhaseType phase) {
        if (verb.equals("castspell") || verb.equals("playland") ? args.length != 2 : args.length != 3) {
            throw new IllegalArgumentException(verb + " " + String.join(" ", args));
        }
        plays.add(new Play(verb, args[0], turn, phase, Integer.parseInt(args[1]), args.length > 2 ? args[2] : null,
                resolved));
        resolved = false;
    }

    /** A resolvestack line: the next play waits for an empty stack. */
    void resolveStack() {
        resolved = true;
    }

    /** The next play when it is p's, now; else null (p passes). */
    Play nextPlayFor(Player p) {
        Play next = plays.peek();
        if (next == null || !next.player().equalsIgnoreCase(p.getName())
                || game.getPhaseHandler().getTurn() != next.turn() || !game.getPhaseHandler().is(next.phase())
                || next.afterResolve() && !game.getStack().isEmpty()) {
            return null;
        }
        return plays.poll();
    }

    /** True once every play line was made: the checkpoint waits for it. */
    boolean playsDone() {
        return plays.isEmpty();
    }

    /** The next targets answer as entities. */
    List<GameEntity> nextTargets() {
        List<String> next = targets.poll();
        if (next == null) {
            throw new IllegalStateException("no targets queued");
        }
        List<GameEntity> out = new ArrayList<>();
        for (String v : next) {
            out.add(entity(v.strip()));
        }
        return out;
    }

    /** Binds the answers to the game GameState set up, once applyToGame ran. */
    void bind(Game game0, Map<Integer, Card> idToCard0) {
        game = game0;
        idToCard = idToCard0;
    }

    /** The next attackers answer, or null when none is queued (declare none). */
    List<Integer> nextAttackers() {
        return attackers.poll();
    }

    /** The next blocks answer as blocker/attacker id pairs, or null. */
    List<int[]> nextBlocks() {
        return blocks.poll();
    }

    /**
     * The next attack target: a setup card id or a player name. Go asks only
     * when an attacker has more than one defender (attack.go
     * chooseAttackTargets), and so does the controller.
     */
    GameEntity nextAttackTarget() {
        String v = attackTargets.poll();
        if (v == null) {
            throw new IllegalStateException("no attack target queued");
        }
        return entity(v);
    }

    // A setup card id or a seated player's name (fixture resolveAttackTarget).
    private GameEntity entity(String v) {
        if (v.chars().allMatch(Character::isDigit)) {
            return card(Integer.parseInt(v));
        }
        for (Player p : game.getRegisteredPlayers()) {
            if (p.getName().equalsIgnoreCase(v)) {
                return p;
            }
        }
        throw new IllegalStateException("attack target " + v);
    }

    /** The card a setup Id: names, as it is now (Game.findById). */
    Card card(int id) {
        Card c = idToCard.get(id);
        if (c == null) {
            throw new IllegalStateException("no setup card with Id:" + id);
        }
        Card now = game.findById(c.getId());
        return now != null ? now : c;
    }

    private static List<Integer> ids(String value) {
        List<Integer> out = new ArrayList<>();
        for (String s : value.split(",")) {
            out.add(Integer.parseInt(s.strip()));
        }
        return out;
    }
}
