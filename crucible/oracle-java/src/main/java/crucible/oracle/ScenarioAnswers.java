package crucible.oracle;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.Set;

import forge.game.Game;
import forge.game.GameEntity;
import forge.game.card.Card;
import forge.game.card.CardCollection;
import forge.game.card.CardCollectionView;
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
    static final Set<String> KINDS = Set.of("attackers", "blocks", "attacktarget", "targets", "paygeneric",
            "cardorder", "cardchoice");

    /**
     * A tapformana, castspell or playland line, with the turn and phase Go
     * ran it in, or a queued action (pass, cast, activate, manaability) with
     * the turn and phase of the priority round that takes it. arg is the
     * color or the ability index. afterResolve when a resolvestack or
     * passpriority line came before a play since the previous one, so it
     * waits for the stack to empty. line is the actions.log line.
     */
    record Play(int line, String verb, String player, int turn, PhaseType phase, int id, String arg,
            boolean afterResolve) {
    }

    private final Deque<List<Integer>> attackers = new ArrayDeque<>();
    private final Deque<List<int[]>> blocks = new ArrayDeque<>();
    private final Deque<String> attackTargets = new ArrayDeque<>();
    private final Deque<List<String>> targets = new ArrayDeque<>();
    private final Deque<List<Integer>> cardOrders = new ArrayDeque<>();
    private final Deque<List<Integer>> cardChoices = new ArrayDeque<>();
    private final Deque<Play> plays = new ArrayDeque<>();
    // Go's ScriptedController keeps one action queue per player (TakeAction).
    private final Map<String, Deque<Play>> actions = new LinkedHashMap<>();
    private boolean resolved;
    private int lastRound = -1;
    private boolean resolvedForAction;
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
            case "cardorder" -> cardOrders.add(ids(value));
            case "cardchoice" -> cardChoices.add(value.strip().equals("none") ? List.of() : ids(value));
            default -> throw new IllegalStateException("queue kind " + kind);
        }
    }

    /** Queues a play line; args are the line's words after the verb. */
    void play(int line, String verb, String[] args, int turn, PhaseType phase) {
        if (verb.equals("castspell") || verb.equals("playland") ? args.length != 2 : args.length != 3) {
            throw new IllegalArgumentException(verb + " " + String.join(" ", args));
        }
        plays.add(new Play(line, verb, args[0], turn, phase, Integer.parseInt(args[1]),
                args.length > 2 ? args[2] : null, resolved));
        resolved = false;
    }

    /**
     * Queues a `queue action <player> <kind> ...` line (fixture
     * resolveAction), taken at that player's priority in the given turn and
     * phase; round is the line of the passpriority or step that takes it. A door, unlockdoor, foretell, plot, suspend and turnfaceup are
     * not mapped.
     */
    void action(int line, String[] args, int round, int turn, PhaseType phase) {
        if (args.length < 2) {
            throw new IllegalArgumentException(String.join(" ", args));
        }
        String kind = args[1];
        int want = switch (kind) {
            case "pass" -> 2;
            case "cast", "playland" -> 3;
            case "activate", "manaability", "tapformana" -> 4;
            default -> -1;
        };
        if (args.length != want) {
            throw new IllegalArgumentException(String.join(" ", args));
        }
        String verb = switch (kind) {
            case "cast" -> "castspell";
            default -> kind;
        };
        int id = want > 2 ? Integer.parseInt(args[2]) : 0;
        // An action of a later round than the one before waits for the
        // stack the earlier round left to empty.
        // So does one after a resolvestack or passpriority line.
        boolean later = lastRound >= 0 && round != lastRound || resolvedForAction;
        lastRound = round;
        resolvedForAction = false;
        actions.computeIfAbsent(args[0].toLowerCase(), k -> new ArrayDeque<>())
                .add(new Play(line, verb, args[0], turn, phase, id, want > 3 ? args[3] : null, later));
    }

    /** A resolvestack line: the next play waits for an empty stack. */
    void resolveStack() {
        resolved = true;
        resolvedForAction = true;
    }

    /**
     * The next play when it is p's, now, else p's next queued action when it
     * is due now; else null (p passes).
     */
    Play nextPlayFor(Player p) {
        Play next = plays.peek();
        // A tapformana is made at whoever's priority comes first: Go's
        // harness taps outside the priority loop.
        if (next != null && (next.player().equalsIgnoreCase(p.getName()) || next.verb().equals("tapformana"))
                && now(next)
                && (!next.afterResolve() || game.getStack().isEmpty())) {
            return plays.poll();
        }
        // An action waits for the first priority at or after its round: Go
        // keeps it queued through a step that gives none.
        Deque<Play> own = actions.get(p.getName().toLowerCase());
        // Nor before a play an earlier line made.
        if (own != null && own.peek() != null && due(own.peek())
                && (!own.peek().afterResolve() || game.getStack().isEmpty())
                && (next == null || next.line() > own.peek().line())) {
            return own.poll();
        }
        return null;
    }

    private boolean due(Play play) {
        int turn = game.getPhaseHandler().getTurn();
        return turn > play.turn()
                || turn == play.turn() && game.getPhaseHandler().getPhase().ordinal() >= play.phase().ordinal();
    }

    private boolean now(Play play) {
        return game.getPhaseHandler().getTurn() == play.turn() && game.getPhaseHandler().is(play.phase());
    }

    /** True once every play line was made: the checkpoint waits for it. */
    boolean playsDone() {
        return plays.isEmpty();
    }

    /**
     * True once every queued action was taken: a checkpoint at a priority
     * waits for it, one in a step without priority does not, as Go's queue
     * keeps what no round took.
     */
    boolean actionsDone() {
        return actions.values().stream().allMatch(Deque::isEmpty);
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

    /**
     * The next card order answer (Go's OrderCardsForZone, Java's
     * orderMoveToZoneList), as the cards asked about: it must name each of
     * them once, the way Go's checkChoice does.
     */
    CardCollection nextCardOrder(CardCollectionView cards) {
        List<Integer> order = cardOrders.poll();
        if (order == null) {
            throw new IllegalStateException("no card order queued for " + cards);
        }
        CardCollection out = new CardCollection();
        for (int id : order) {
            int javaId = card(id).getId();
            for (Card c : cards) {
                if (c.getId() == javaId && !out.contains(c)) {
                    out.add(c);
                }
            }
        }
        if (out.size() != cards.size() || order.size() != cards.size()) {
            throw new IllegalStateException("card order " + order + " for " + cards);
        }
        return out;
    }

    /**
     * The next card choice answer (Go's ChooseCardsForEffect) as cards of
     * options, popped only when it fits: each id one of options, between min
     * and max of them. Null when none is queued or the next one is for a
     * later question; the caller then answers the upstream default.
     */
    List<Card> nextCardChoice(Iterable<? extends GameEntity> options, int min, int max) {
        List<Integer> next = cardChoices.peek();
        if (next == null || next.size() < min || next.size() > max) {
            return null;
        }
        List<Card> out = new ArrayList<>();
        for (int id : next) {
            int javaId = card(id).getId();
            Card match = null;
            for (GameEntity e : options) {
                if (e instanceof Card c && c.getId() == javaId && !out.contains(c)) {
                    match = c;
                    break;
                }
            }
            if (match == null) {
                return null;
            }
            out.add(match);
        }
        cardChoices.poll();
        return out;
    }

    /** Throws when a queued card choice was never popped. */
    void checkCardChoicesUsed() {
        if (!cardChoices.isEmpty()) {
            throw new IllegalStateException("card choices never asked for: " + cardChoices);
        }
    }

    /** True once bind ran: the setup is applied. */
    boolean bound() {
        return game != null;
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
