package crucible.oracle;

import java.util.ArrayDeque;
import java.util.Arrays;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Deque;
import java.util.List;
import java.util.Map;
import java.util.Set;

import forge.card.MagicColor;
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
            "cardorder", "cardchoice", "confirmeffect", "optionaltrigger",
            "confirmpaycost", "discardchoice", "sacrificechoice", "modechoice", "scry", "surveil",
            "payphyrexian", "payx", "colorchoice", "numberchoice", "replacement", "confirmreplacement", "paysnow", "paymonocoloredhybrid",
            "paycolorlesshybrid", "payhybridphyrexian", "returnchoice", "enchanttarget", "divide");

    /**
     * A tapformana, castspell or playland line, with the turn and phase Go
     * ran it in, or a queued action (pass, cast, activate, manaability) with
     * the turn and phase of the priority round that takes it. arg is the
     * color or the ability index. afterResolve when a resolvestack or
     * passpriority line came before a play since the previous one, so it
     * waits for the stack to empty. line is the actions.log line.
     */
    record Play(int line, String verb, String player, int turn, PhaseType phase, int id, String arg,
            boolean afterResolve, boolean refused) {
    }

    private final Deque<List<Integer>> attackers = new ArrayDeque<>();
    private final Deque<List<int[]>> blocks = new ArrayDeque<>();
    private final Deque<String> attackTargets = new ArrayDeque<>();
    private final Deque<List<String>> targets = new ArrayDeque<>();
    private final Deque<List<Integer>> cardOrders = new ArrayDeque<>();
    private final Deque<List<Integer>> cardChoices = new ArrayDeque<>();
    private final Deque<Boolean> confirmEffects = new ArrayDeque<>();
    private final Deque<Boolean> optionalTriggers = new ArrayDeque<>();
    private final Deque<Boolean> confirmPayCosts = new ArrayDeque<>();
    private final Deque<List<Integer>> discardChoices = new ArrayDeque<>();
    private final Deque<List<Integer>> sacrificeChoices = new ArrayDeque<>();
    private final Deque<List<Integer>> modeChoices = new ArrayDeque<>();
    private final Deque<List<List<Integer>>> scries = new ArrayDeque<>();
    private final Deque<List<List<Integer>>> surveils = new ArrayDeque<>();
    private final Deque<Boolean> phyrexian = new ArrayDeque<>();
    private final Deque<Integer> xValues = new ArrayDeque<>();
    private final Deque<Byte> colorChoices = new ArrayDeque<>();
    private final Deque<Integer> numberChoices = new ArrayDeque<>();
    private final Deque<Integer> replacements = new ArrayDeque<>();
    private final Deque<Boolean> confirmReplacements = new ArrayDeque<>();
    private final Deque<String> hybridPhyrexian = new ArrayDeque<>();
    private final Deque<List<Integer>> returnChoices = new ArrayDeque<>();
    private final Deque<Integer> enchantTargets = new ArrayDeque<>();
    private final Deque<List<Integer>> divisions = new ArrayDeque<>();
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
            // Which snow mana pays {S}, and whether {2/W} or {C/W} takes its
            // color: the pool pays in Java's own order too.
            case "paysnow", "paymonocoloredhybrid", "paycolorlesshybrid" -> {
            }
            case "payhybridphyrexian" -> hybridPhyrexian.add(value.strip());
            case "returnchoice" -> returnChoices.add(ids(value));
            case "enchanttarget" -> enchantTargets.add(Integer.parseInt(value.strip()));
            case "divide" -> divisions.add(ids(value));
            case "cardorder" -> cardOrders.add(ids(value));
            case "confirmeffect" -> confirmEffects.add(bool(value));
            case "optionaltrigger" -> optionalTriggers.add(bool(value));
            case "confirmpaycost" -> confirmPayCosts.add(bool(value));
            case "cardchoice" -> cardChoices.add(value.strip().equals("none") ? List.of() : ids(value));
            case "discardchoice" -> discardChoices.add(ids(value));
            case "sacrificechoice" -> sacrificeChoices.add(ids(value));
            case "modechoice" -> modeChoices.add(ids(value));
            case "scry" -> scries.add(split(value));
            case "surveil" -> surveils.add(split(value));
            case "payphyrexian" -> phyrexian.add(bool(value));
            case "payx" -> xValues.add(Integer.parseInt(value.strip()));
            case "colorchoice" -> colorChoices.add(MagicColor.fromName(value.strip()));
            case "numberchoice" -> numberChoices.add(Integer.parseInt(value.strip()));
            case "replacement" -> replacements.add(Integer.parseInt(value.strip()));
            case "confirmreplacement" -> confirmReplacements.add(bool(value));
            default -> throw new IllegalStateException("queue kind " + kind);
        }
    }

    /** Queues a play line; args are the line's words after the verb. */
    void play(int line, String verb, String[] args, int turn, PhaseType phase) {
        boolean face = verb.equals("castspell") && args.length == 3 && (args[2].equals("face") || args[2].equals("fuse"));
        if (!face && (verb.equals("castspell") || verb.equals("playland") ? args.length != 2 : args.length != 3)) {
            throw new IllegalArgumentException(verb + " " + String.join(" ", args));
        }
        plays.add(new Play(line, verb, args[0], turn, phase, Integer.parseInt(args[1]),
                args.length > 2 ? args[2] : null, resolved, false));
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
                .add(new Play(line, verb, args[0], turn, phase, id, want > 3 ? args[3] : null, later, false));
    }

    /**
     * Queues an `expecterror refused castspell|playland <player> <id>` line:
     * a play Go refused, which Java must refuse too (S17).
     */
    void refusedPlay(int line, String verb, String[] args, int turn, PhaseType phase) {
        if (args.length != 2) {
            throw new IllegalArgumentException(verb + " " + String.join(" ", args));
        }
        plays.add(new Play(line, verb, args[0], turn, phase, Integer.parseInt(args[1]), null, resolved, true));
        resolved = false;
    }

    /**
     * Queues a `paymanacost <player> <cost...>` line, made at that player's
     * priority in the turn and phase Go ran it in (S19); arg is the cost.
     */
    void payManaCost(int line, String[] args, int turn, PhaseType phase) {
        if (args.length < 2) {
            throw new IllegalArgumentException("paymanacost " + String.join(" ", args));
        }
        plays.add(new Play(line, "paymanacost", args[0], turn, phase, 0,
                String.join(" ", Arrays.copyOfRange(args, 1, args.length)), resolved, false));
        resolved = false;
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
        // A refused play whose turn and phase passed with no priority for
        // its seat was refused by Java too: no one could have made it (S17).
        while (plays.peek() != null && plays.peek().refused() && due(plays.peek()) && !now(plays.peek())) {
            plays.poll();
        }
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

    /**
     * The next play when it is a tapformana of this turn and step: a cost
     * paid before the step's priority (an attack cost, CR 508.1g) activates
     * the mana abilities Go's harness ran in that step.
     */
    Play nextTapNow() {
        Play next = plays.peek();
        return next != null && next.verb().equals("tapformana") && now(next) ? plays.poll() : null;
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
        return nextCards(cardChoices, options, min, max);
    }

    /**
     * The next discardchoice answer (Go's ChooseCardsToDiscard), popped when
     * it fits as a card choice does; else the next card choice.
     */
    List<Card> nextDiscardChoice(Iterable<? extends GameEntity> options, int min, int max) {
        List<Card> out = nextCards(discardChoices, options, min, max);
        return out != null ? out : nextCardChoice(options, min, max);
    }

    /** The next returnchoice answer (Go's ChooseReturn for a Return cost), else the next card choice. */
    List<Card> nextReturnChoice(Iterable<? extends GameEntity> options, int min, int max) {
        List<Card> out = nextCards(returnChoices, options, min, max);
        return out != null ? out : nextCardChoice(options, min, max);
    }

    /** The next enchanttarget answer when it is one of options (Go's ChooseEnchantTarget), else null. */
    Card nextEnchantTarget(Iterable<? extends GameEntity> options) {
        Integer next = enchantTargets.peek();
        if (next == null) {
            return null;
        }
        int javaId = card(next).getId();
        for (GameEntity e : options) {
            if (e instanceof Card c && c.getId() == javaId) {
                enchantTargets.poll();
                return c;
            }
        }
        return null;
    }

    /** The next divide answer (Go's DivideAmongTargets shares, in target order), or null. */
    List<Integer> nextDivision() {
        return divisions.poll();
    }

    /** The next sacrificechoice answer (Go's ChoosePermanentsToSacrifice), else the next card choice. */
    List<Card> nextSacrificeChoice(Iterable<? extends GameEntity> options, int min, int max) {
        List<Card> out = nextCards(sacrificeChoices, options, min, max);
        return out != null ? out : nextCardChoice(options, min, max);
    }

    private List<Card> nextCards(Deque<List<Integer>> queue, Iterable<? extends GameEntity> options, int min,
            int max) {
        List<Integer> next = queue.peek();
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
        queue.poll();
        return out;
    }

    /** The next modechoice answer: indices into the modes offered, or null. */
    List<Integer> nextModeChoice() {
        return modeChoices.poll();
    }

    /**
     * The next scry answer as the cards to put on top (the first topmost)
     * and the cards to put on the bottom, or null. A card the answer leaves
     * out is not moved, as Go leaves it (scryeffect.go).
     */
    List<CardCollection> nextScry() {
        return cards(scries.poll());
    }

    /** The next surveil answer: the cards kept on top and those put into the graveyard, or null. */
    List<CardCollection> nextSurveil() {
        return cards(surveils.poll());
    }

    private List<CardCollection> cards(List<List<Integer>> halves) {
        if (halves == null) {
            return null;
        }
        List<CardCollection> out = new ArrayList<>();
        for (List<Integer> half : halves) {
            CardCollection cc = new CardCollection();
            for (int id : half) {
                cc.add(card(id));
            }
            out.add(cc);
        }
        return out;
    }

    /** The next payphyrexian answer (true pays the color, false 2 life), or null. */
    Boolean nextPayPhyrexian() {
        return phyrexian.poll();
    }

    /** The next payhybridphyrexian answer ("life" or a color letter), or null. */
    String nextPayHybridPhyrexian() {
        return hybridPhyrexian.poll();
    }

    /** The next payx answer, or null. */
    Integer nextPayX() {
        return xValues.poll();
    }

    /** The next colorchoice answer as a color mask, or null. */
    Byte nextColorChoice() {
        return colorChoices.poll();
    }

    /** The next numberchoice answer, or null. */
    Integer nextNumberChoice() {
        return numberChoices.poll();
    }

    /** The next confirmeffect answer (Go's ConfirmEffect), or null when none is queued. */
    Boolean nextConfirmEffect() {
        return confirmEffects.poll();
    }

    /** The next replacement answer (Go's ChooseReplacementEffect index), or null. */
    Integer nextReplacement() {
        return replacements.poll();
    }

    /** The next confirmreplacement answer (Go's ConfirmReplacementEffect), or null. */
    Boolean nextConfirmReplacement() {
        return confirmReplacements.poll();
    }

    /** The next confirmpaycost answer (Go's ConfirmPayCost), or null. */
    Boolean nextConfirmPayCost() {
        return confirmPayCosts.poll();
    }

    /** The next optionaltrigger answer (Go's ConfirmOptionalTrigger), or null. */
    Boolean nextOptionalTrigger() {
        return optionalTriggers.poll();
    }

    /**
     * Throws when a queued card choice or yes/no answer was never popped:
     * Java never asked what Go did.
     */
    void checkAnswersUsed() {
        if (!cardChoices.isEmpty()) {
            throw new IllegalStateException("card choices never asked for: " + cardChoices);
        }
        if (!targets.isEmpty()) {
            throw new IllegalStateException("targets never asked for: " + targets);
        }
        if (!confirmEffects.isEmpty()) {
            throw new IllegalStateException("confirmeffect answers never asked for: " + confirmEffects);
        }
        if (!confirmPayCosts.isEmpty()) {
            throw new IllegalStateException("confirmpaycost answers never asked for: " + confirmPayCosts);
        }
        if (!optionalTriggers.isEmpty()) {
            throw new IllegalStateException("optionaltrigger answers never asked for: " + optionalTriggers);
        }
        Map<String, Deque<?>> rest = new LinkedHashMap<>();
        rest.put("discardchoice", discardChoices);
        rest.put("sacrificechoice", sacrificeChoices);
        rest.put("modechoice", modeChoices);
        rest.put("scry", scries);
        rest.put("surveil", surveils);
        rest.put("payphyrexian", phyrexian);
        rest.put("payx", xValues);
        rest.put("colorchoice", colorChoices);
        rest.put("numberchoice", numberChoices);
        rest.put("replacement", replacements);
        rest.put("confirmreplacement", confirmReplacements);
        rest.put("payhybridphyrexian", hybridPhyrexian);
        rest.put("returnchoice", returnChoices);
        rest.put("enchanttarget", enchantTargets);
        rest.put("divide", divisions);
        for (Map.Entry<String, Deque<?>> e : rest.entrySet()) {
            if (!e.getValue().isEmpty()) {
                throw new IllegalStateException(e.getKey() + " answers never asked for: " + e.getValue());
            }
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

    // fixture/actions.go reads these with strconv.ParseBool.
    private static boolean bool(String value) {
        return switch (value.strip()) {
            case "true", "1", "t", "T", "TRUE", "True" -> true;
            case "false", "0", "f", "F", "FALSE", "False" -> false;
            default -> throw new IllegalArgumentException("not a bool: " + value);
        };
    }

    // Go's queue scry/surveil value: the top ids, then after a slash the
    // bottom (or graveyard) ids; either half may be empty.
    private static List<List<Integer>> split(String value) {
        String[] halves = value.strip().split("/", -1);
        if (halves.length > 2) {
            throw new IllegalArgumentException("scry/surveil " + value);
        }
        List<List<Integer>> out = new ArrayList<>();
        for (String h : halves) {
            out.add(h.isBlank() ? List.of() : ids(h));
        }
        if (out.size() == 1) {
            out.add(List.of());
        }
        return out;
    }

    private static List<Integer> ids(String value) {
        List<Integer> out = new ArrayList<>();
        for (String s : value.split(",")) {
            out.add(Integer.parseInt(s.strip()));
        }
        return out;
    }
}
