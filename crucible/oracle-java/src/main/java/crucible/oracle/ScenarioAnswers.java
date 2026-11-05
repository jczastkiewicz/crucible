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
import forge.game.player.Player;

/**
 * The actions.log queue lines the L2 runner maps (l2-verbs.md, queue kinds),
 * shared by every seat's {@link ScenarioController} the way Go's one
 * ScriptedController answers for every player: each decision pops the next
 * answer of its kind. Card ids are setup.state's Id: handles.
 */
final class ScenarioAnswers {
    /** The queue kinds mapped so far; a scenario queueing any other is unmapped. */
    static final Set<String> KINDS = Set.of("attackers", "blocks", "attacktarget");

    private final Deque<List<Integer>> attackers = new ArrayDeque<>();
    private final Deque<List<int[]>> blocks = new ArrayDeque<>();
    private final Deque<String> attackTargets = new ArrayDeque<>();
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
            default -> throw new IllegalStateException("queue kind " + kind);
        }
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
