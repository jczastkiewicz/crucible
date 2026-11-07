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
import com.google.common.eventbus.Subscribe;
import forge.game.GameObject;
import forge.game.event.GameEventSpellAbilityCast;
import forge.game.spellability.SpellAbility;
import forge.game.spellability.SpellAbilityStackInstance;
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
            "paycolorlesshybrid", "payhybridphyrexian", "returnchoice", "enchanttarget", "divide", "abilitychoice", "playerchoice",
            "discard", "exertattackers", "legendarykeep", "option", "hybridmanacolor", "coincall", "damage",
            "startingplayer", "keephand", "tuck", "battleprotector", "sector", "tapchoice", "entitychoice",
            "optionname");

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
    // The attackers and blocks answers an expecterror line says Go refused,
    // by identity (S23).
    private final Set<Object> refusedDeclarations = java.util.Collections.newSetFromMap(new java.util.IdentityHashMap<>());
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
    private final Deque<List<Integer>> abilityChoices = new ArrayDeque<>();
    private final Deque<String> playerChoices = new ArrayDeque<>();
    private final Deque<List<Integer>> discards = new ArrayDeque<>();
    private final Deque<List<Integer>> exerts = new ArrayDeque<>();
    private final Deque<Integer> legendaryKeeps = new ArrayDeque<>();
    private final Deque<Integer> options = new ArrayDeque<>();
    private final Deque<Boolean> coinCalls = new ArrayDeque<>();
    private final Deque<List<int[]>> damages = new ArrayDeque<>();
    private final Deque<String> generics = new ArrayDeque<>();
    private final Deque<String> protectors = new ArrayDeque<>();
    private final Deque<String> sectors = new ArrayDeque<>();
    private final Deque<List<Integer>> tapChoices = new ArrayDeque<>();
    private final Deque<List<String>> entityChoices = new ArrayDeque<>();
    private final Deque<String> optionNames = new ArrayDeque<>();
    private final Deque<Boolean> monoHybrids = new ArrayDeque<>();
    private final Deque<String> startingPlayers = new ArrayDeque<>();
    private final Deque<Boolean> keepHands = new ArrayDeque<>();
    private final Deque<List<Integer>> tucks = new ArrayDeque<>();
    private final Deque<Play> plays = new ArrayDeque<>();
    // Go's ScriptedController keeps one action queue per player (TakeAction).
    private final Map<String, Deque<Play>> actions = new LinkedHashMap<>();
    private boolean resolved;
    private int lastRound = -1;
    private boolean resolvedForAction;
    private boolean applied;
    private Game game;
    private Map<Integer, Card> idToCard;
    private final List<Integer> pushes = new ArrayList<>();

    /** Queues one `queue <kind> <value>` line (fixture/actions.go's grammar). */
    void queue(String kind, String value) {
        switch (kind) {
            // A fresh list for none: S23 marks answers by identity.
            case "attackers" -> attackers.add(value.equals("none") ? new ArrayList<>() : ids(value));
            case "blocks" -> blocks.add(value.equals("none") ? new ArrayList<>() : pairs(value));
            case "attacktarget" -> attackTargets.add(value.strip());
            case "damage" -> damages.add(pairs(value));
            case "startingplayer" -> startingPlayers.add(value.strip());
            case "battleprotector" -> protectors.add(value.strip());
            case "sector" -> sectors.add(value.strip());
            case "tapchoice" -> tapChoices.add(ids(value));
            case "entitychoice" -> entityChoices.add(value.strip().equals("none") ? List.of() : List.of(value.strip().split(",")));
            // Spaces in a name are written "_" (fixture/actions.go).
            case "optionname" -> optionNames.add(value.strip().replace('_', ' '));
            case "keephand" -> keepHands.add(bool(value));
            case "tuck" -> tucks.add(ids(value));
            case "targets" -> {
                targets.add(value.strip().equals("none") ? List.of() : List.of(value.strip().split(",")));
            }
            // The mana that pays each generic unit (S25), and whether {2/W}
            // takes its color or two generic units.
            case "paygeneric" -> generics.add(value.strip());
            case "paymonocoloredhybrid" -> monoHybrids.add(bool(value));
            // Which snow mana pays {S}, whether {C/W} takes its color, and the
            // color a hybrid shard takes (Go's ChooseHybridManaColor): the
            // pool pays in Java's own order.
            case "paysnow", "paycolorlesshybrid", "hybridmanacolor" -> {
            }
            case "discard" -> discards.add(ids(value));
            case "exertattackers" -> exerts.add(value.strip().equals("none") ? List.of() : ids(value));
            case "legendarykeep" -> legendaryKeeps.add(Integer.parseInt(value.strip()));
            case "option" -> options.add(Integer.parseInt(value.strip()));
            case "coincall" -> coinCalls.add(switch (value.strip()) {
                case "heads" -> true;
                case "tails" -> false;
                default -> throw new IllegalArgumentException("coincall " + value);
            });
            case "payhybridphyrexian" -> hybridPhyrexian.add(value.strip());
            case "returnchoice" -> returnChoices.add(ids(value));
            case "enchanttarget" -> enchantTargets.add(Integer.parseInt(value.strip()));
            case "divide" -> divisions.add(ids(value));
            case "abilitychoice" -> abilityChoices.add(ids(value));
            case "playerchoice" -> playerChoices.add(value.strip());
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
        boolean twoArgs = verb.equals("castspell") || verb.equals("playland") || ScenarioRunner.SPECIAL.contains(verb);
        if (!face && (twoArgs ? args.length != 2 : args.length != 3)) {
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

    /**
     * The next targets answer: cards and players, and `stack:N`, the Nth push
     * onto the stack this game (Go's StackItemID, assigned in push order from
     * 1) as the spell or ability it put there (S21).
     */
    List<GameObject> nextTargets() {
        List<String> next = targets.poll();
        if (next == null) {
            throw new IllegalStateException("no targets queued");
        }
        List<GameObject> out = new ArrayList<>();
        for (String v : next) {
            v = v.strip();
            out.add(v.startsWith("stack:") ? stackItem(Integer.parseInt(v.substring("stack:".length()))) : entity(v));
        }
        return out;
    }

    // S21: the spell or ability the nth push put on the stack, still there.
    private SpellAbility stackItem(int n) {
        if (n < 1 || n > pushes.size()) {
            throw new IllegalStateException("stack:" + n + " of " + pushes.size() + " pushes");
        }
        int id = pushes.get(n - 1);
        for (SpellAbilityStackInstance si : game.getStack()) {
            if (si.getId() == id) {
                return si.getSpellAbility();
            }
        }
        throw new IllegalStateException("stack:" + n + " is no longer on the stack");
    }

    // Every push onto the stack fires GameEventSpellAbilityCast with its
    // stack instance (MagicStack.java:379), on the posting thread.
    @Subscribe
    public void onPush(GameEventSpellAbilityCast e) {
        pushes.add(e.si().getId());
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

    /** The next abilitychoice answer (Go's ChooseAbilitiesForEffect indices), or null. */
    List<Integer> nextAbilityChoice() {
        return abilityChoices.poll();
    }

    /** The next playerchoice answer when it is one of options (Go's ChoosePlayerForEffect), else null. */
    Player nextPlayerChoice(Iterable<? extends GameEntity> options) {
        String next = playerChoices.peek();
        if (next == null) {
            return null;
        }
        for (GameEntity e : options) {
            if (e instanceof Player p && p.getName().equalsIgnoreCase(next)) {
                playerChoices.poll();
                return p;
            }
        }
        return null;
    }

    /**
     * The cleanup step's discard (Go's ChooseDiscardToHandSize) as cards in
     * p's hand, or null when none is queued; exactly n of them.
     */
    List<Card> nextDiscard(Player p, int n) {
        List<Integer> next = discards.poll();
        if (next == null) {
            return null;
        }
        List<Card> out = new ArrayList<>();
        for (int id : next) {
            Card c = card(id);
            if (!p.getCardsIn(forge.game.zone.ZoneType.Hand).contains(c) || out.contains(c)) {
                throw new IllegalStateException("discard " + next + ": " + c + " is not in " + p + "'s hand");
            }
            out.add(c);
        }
        if (out.size() != n) {
            throw new IllegalStateException("discard " + next + " for " + n + " cards");
        }
        return out;
    }

    /**
     * The next exertattackers answer (Go's ExertAttackers) as cards of the
     * attackers offered, or null when none is queued.
     */
    List<Card> nextExert(List<Card> offered) {
        List<Integer> next = exerts.poll();
        if (next == null) {
            return null;
        }
        List<Card> out = new ArrayList<>();
        for (int id : next) {
            Card c = card(id);
            if (!offered.contains(c)) {
                throw new IllegalStateException("exert " + c + ": not one of " + offered);
            }
            out.add(c);
        }
        return out;
    }

    /**
     * The next legendarykeep answer (Go's ChooseLegendaryToKeep) when it is
     * one of options, else null. Asked from the setup's own state-based pass
     * too, so it needs only attach.
     */
    Card nextLegendaryKeep(Iterable<? extends GameEntity> optionList) {
        Integer next = legendaryKeeps.peek();
        if (next == null || game == null) {
            return null;
        }
        int javaId = card(next).getId();
        for (GameEntity e : optionList) {
            if (e instanceof Card c && c.getId() == javaId) {
                legendaryKeeps.poll();
                return c;
            }
        }
        return null;
    }

    /**
     * The next damage answer (Go's AssignCombatDamage) as the amount each of
     * recipients gets, in the answer's order, or null when none is queued.
     */
    Map<Card, Integer> nextDamage(CardCollectionView recipients) {
        List<int[]> next = damages.poll();
        if (next == null) {
            return null;
        }
        Map<Card, Integer> out = new LinkedHashMap<>();
        for (int[] pair : next) {
            Card c = card(pair[0]);
            if (!recipients.contains(c) || out.containsKey(c)) {
                throw new IllegalStateException("damage to " + c + ": not one of " + recipients);
            }
            out.put(c, pair[1]);
        }
        return out;
    }

    /**
     * The next startingplayer answer (Go's ChooseStartingPlayer) as one of
     * players, found by name, or null. A subgame asks with its own players.
     */
    Player nextStartingPlayer(Iterable<Player> players) {
        String next = startingPlayers.poll();
        if (next == null) {
            return null;
        }
        for (Player p : players) {
            if (p.getName().equalsIgnoreCase(next)) {
                return p;
            }
        }
        throw new IllegalStateException("startingplayer " + next + " is not one of " + players);
    }

    /** The next keephand answer (Go's MulliganKeepHand), or null. */
    Boolean nextKeepHand() {
        return keepHands.poll();
    }

    /**
     * The next tuck answer (Go's ChooseCardsToTuck) as n cards of hand, or
     * null when none is queued.
     */
    List<Card> nextTuck(CardCollectionView hand, int n) {
        List<Integer> next = tucks.poll();
        if (next == null) {
            return null;
        }
        List<Card> out = new ArrayList<>();
        for (int id : next) {
            Card c = card(id);
            if (!hand.contains(c) || out.contains(c)) {
                throw new IllegalStateException("tuck " + next + ": " + c + " is not in hand " + hand);
            }
            out.add(c);
        }
        if (out.size() != n) {
            throw new IllegalStateException("tuck " + next + " for " + n + " cards");
        }
        return out;
    }

    /** The next paygeneric answer, a mana letter (Go's ChoosePayGeneric), or null. */
    String nextPayGeneric() {
        return generics.poll();
    }

    /** The next paymonocoloredhybrid answer (Go's ChoosePayMonocoloredHybrid), or null. */
    Boolean nextPayMonocoloredHybrid() {
        return monoHybrids.poll();
    }

    /**
     * The next battleprotector answer (Go's ChooseBattleProtector) when it
     * names one of options, else null. Asked from the setup's own pass too.
     */
    Player nextBattleProtector(Iterable<? extends GameEntity> options) {
        String next = protectors.peek();
        if (next == null) {
            return null;
        }
        for (GameEntity e : options) {
            if (e instanceof Player p && p.getName().equalsIgnoreCase(next)) {
                protectors.poll();
                return p;
            }
        }
        return null;
    }

    /** The next sector answer (Go's ChooseSector) as one of sectors, by name, or null. */
    String nextSector(List<String> offered) {
        String next = sectors.poll();
        if (next == null) {
            return null;
        }
        for (String s : offered) {
            if (s.equalsIgnoreCase(next)) {
                return s;
            }
        }
        throw new IllegalStateException("sector " + next + " is not one of " + offered);
    }

    /** The next tapchoice answer (Go's ChoosePermanentsToTap) when it fits, as cards of options, or null. */
    List<Card> nextTapChoice(Iterable<? extends GameEntity> options, int min, int max) {
        return nextCards(tapChoices, options, min, max);
    }

    /**
     * The next entitychoice answer (Go's ChooseEntitiesForEffect): cards and
     * players of options, popped only when each is one of them and the count
     * is within min and max; else null.
     */
    List<GameEntity> nextEntityChoice(Iterable<? extends GameEntity> options, int min, int max) {
        List<String> next = entityChoices.peek();
        if (next == null || next.size() < min || next.size() > max) {
            return null;
        }
        List<GameEntity> out = new ArrayList<>();
        for (String v : next) {
            GameEntity want = entity(v.strip());
            GameEntity match = null;
            for (GameEntity e : options) {
                if (e.getId() == want.getId() && e.getClass() == want.getClass() && !out.contains(e)) {
                    match = e;
                    break;
                }
            }
            if (match == null) {
                return null;
            }
            out.add(match);
        }
        entityChoices.poll();
        return out;
    }

    /** The next optionname answer (Go's ChooseOption by name) when it is one of offered, else null. */
    String nextOptionName(java.util.function.Predicate<String> offered) {
        String next = optionNames.peek();
        if (next == null || !offered.test(next)) {
            return null;
        }
        return optionNames.poll();
    }

    /** The next option answer (Go's ChooseOption index), or null. */
    Integer nextOption() {
        return options.poll();
    }

    /** The next coincall answer, true for heads (Go's CallCoinFlip), or null. */
    Boolean nextCoinCall() {
        return coinCalls.poll();
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
        rest.put("abilitychoice", abilityChoices);
        rest.put("playerchoice", playerChoices);
        rest.put("discard", discards);
        rest.put("exertattackers", exerts);
        rest.put("legendarykeep", legendaryKeeps);
        rest.put("option", options);
        rest.put("coincall", coinCalls);
        rest.put("damage", damages);
        rest.put("startingplayer", startingPlayers);
        rest.put("keephand", keepHands);
        rest.put("tuck", tucks);
        rest.put("battleprotector", protectors);
        rest.put("sector", sectors);
        rest.put("tapchoice", tapChoices);
        rest.put("entitychoice", entityChoices);
        rest.put("optionname", optionNames);
        for (Map.Entry<String, Deque<?>> e : rest.entrySet()) {
            if (!e.getValue().isEmpty()) {
                throw new IllegalStateException(e.getKey() + " answers never asked for: " + e.getValue());
            }
        }
    }

    /** True once bind ran: the setup is applied. */
    boolean bound() {
        return applied;
    }

    /**
     * Attaches the game and GameState's own id map before applyToGame, which
     * fills the map before its state-based pass: a legend rule there asks for
     * its legendarykeep answer.
     */
    void attach(Game game0, Map<Integer, Card> idToCard0) {
        game = game0;
        idToCard = idToCard0;
    }

    /** Binds the answers to the game GameState set up, once applyToGame ran. */
    void bind() {
        applied = true;
        game.subscribeToEvents(this);
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
     * Marks the last attackers (declareattackers) or blocks
     * (declareblockers) answer queued as one Go refused: an `expecterror
     * <text> declareattackers|declareblockers` line (S23).
     */
    void refuseDeclaration(String verb) {
        Object last = switch (verb) {
            case "declareattackers" -> attackers.peekLast();
            case "declareblockers" -> blocks.peekLast();
            default -> null;
        };
        if (last == null || !refusedDeclarations.add(last)) {
            throw new IllegalArgumentException("expecterror " + verb + " with no answer queued for it");
        }
    }

    /** True when an expecterror line says Go refused this declaration (S23). */
    boolean refused(Object declaration) {
        return refusedDeclarations.contains(declaration);
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

    // id=number pairs, comma-separated: a blocks or damage value.
    private static List<int[]> pairs(String value) {
        List<int[]> out = new ArrayList<>();
        for (String pair : value.split(",")) {
            String[] half = pair.split("=", 2);
            if (half.length != 2) {
                throw new IllegalArgumentException("pair " + pair);
            }
            out.add(new int[] {Integer.parseInt(half[0].strip()), Integer.parseInt(half[1].strip())});
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
