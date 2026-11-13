package crucible.oracle;

import java.io.IOException;
import java.io.Writer;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Random;
import java.util.Set;
import java.util.TreeMap;
import java.util.TreeSet;

import com.google.common.eventbus.Subscribe;
import com.google.common.collect.Multiset;
import com.google.common.collect.Table;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonNull;
import com.google.gson.JsonObject;

import forge.game.Game;
import forge.game.card.Card;
import forge.game.card.CardCollection;
import forge.game.card.CardView;
import forge.game.card.CounterEnumType;
import forge.game.card.CounterType;
import forge.game.event.GameEvent;
import forge.game.event.GameEventCardChangeZone;
import forge.game.event.GameEventCardCounters;
import forge.game.event.GameEventCardDamaged;
import forge.game.event.GameEventFlipCoin;
import forge.game.event.GameEventGameOutcome;
import forge.game.event.GameEventPlayerCounters;
import forge.game.event.GameEventPlayerDamaged;
import forge.game.event.GameEventPlayerLivesChanged;
import forge.game.event.GameEventRollDie;
import forge.game.event.GameEventSpellAbilityCast;
import forge.game.event.GameEventSpellResolved;
import forge.game.event.GameEventTurnBegan;
import forge.game.event.GameEventTurnPhase;
import forge.game.mana.Mana;
import forge.game.player.Player;
import forge.game.player.PlayerView;
import forge.game.spellability.AbilitySub;
import forge.game.spellability.OptionalCost;
import forge.game.spellability.SpellAbility;
import forge.game.spellability.SpellAbilityStackInstance;
import forge.game.spellability.TargetChoices;
import forge.game.zone.ZoneType;
import forge.game.zone.ZoneView;

/**
 * Collects one game's replay log (ADR-0059, ADR-0062, ADR-0063) and writes it
 * as JSON Lines: a header, then every controller call, play record, random
 * outcome and normalised game event in Java's order. Records are kept in memory and written when the game ends,
 * because the header holds each seat's starting library, known only after
 * the first shuffle.
 */
final class ReplayRecorder {
    static final int FORMAT_VERSION = 2;

    // Controller methods that answer nothing: notifications, UI plumbing and
    // factories. Their calls are not decisions and are not logged.
    private static final Set<String> NOT_DECISIONS = Set.of("reveal", "revealAnte", "revealAISkipCards",
            "revealUnsupported", "notifyOfValue", "autoPassCancel", "awaitNextInput", "cancelAwaitNextInput",
            "getCostDecisionMaker", "getAbilityToPlay");

    // The engine sites whose outcome is logged as a result, not as draws: the
    // shuffle's order (cheatShuffle), a die's face and a coin's side (events),
    // the starting toss.
    private static final Set<String> OUTCOME_SITES = Set.of("forge.game.player.Player.shuffle",
            "forge.game.ability.effects.RollDiceEffect", "forge.game.ability.effects.RollPlanarDiceEffect",
            "forge.game.ability.effects.FlipCoinEffect",
            // Who wins the starting toss: the decider of the chooseStartingPlayer
            // call that follows.
            "forge.game.GameAction.determineFirstTurnPlayer");

    private final Game game;
    final ReplayIds ids;
    private final List<JsonObject> records = new ArrayList<>();
    private final List<JsonArray> startingLibraries = new ArrayList<>();
    final List<JsonArray> seatCards = new ArrayList<>();
    // ADR-0063: a player's counters as the last PlayerCounters event left
    // them, because that event fires with no type on a clear.
    private final Map<Player, Map<String, Integer>> playerCounters = new HashMap<>();
    private int depth;
    int unattributedDraws;
    int eventFailures;

    ReplayRecorder(Game game) {
        this.game = game;
        this.ids = new ReplayIds(game);
        for (int i = 0; i < game.getRegisteredPlayers().size(); i++) {
            startingLibraries.add(null);
            seatCards.add(null);
        }
        game.subscribeToEvents(this);
    }

    /** The open record of a controller call, filled in by {@link #answer}. */
    JsonObject call(Player decider, String method, Object[] args) {
        depth++;
        if (NOT_DECISIONS.contains(method)) {
            return null;
        }
        JsonObject r = record("call");
        r.add("decider", ids.player(decider));
        r.addProperty("method", method);
        r.addProperty("depth", depth);
        r.add("offered", ids.encode(args));
        return r;
    }

    void answer(JsonObject r, Object answer, Object[] args) {
        depth--;
        if (r == null) {
            return;
        }
        if (answer == null && args.length > 1 && args[1] instanceof forge.game.combat.Combat c) {
            // declareAttackers and declareBlockers answer by filling in Combat.
            r.add("answer", ids.combat(c));
        } else {
            r.add("answer", ids.encode(answer));
        }
    }

    /** ADR-0062: the choices the AI made for one play, read from the played ability. */
    void play(Player decider, SpellAbility sa, int call) {
        if (sa == null) {
            return;
        }
        JsonObject r = record("play");
        r.addProperty("call", call);
        r.add("decider", ids.player(decider));
        r.add("ability", ids.ability(sa));
        JsonArray chain = new JsonArray();
        for (SpellAbility s = sa; s != null; s = s.getSubAbility()) {
            chain.add(part(s));
        }
        r.add("chain", chain);
        if (sa.getXManaCostPaid() != null) {
            r.addProperty("x", sa.getXManaCostPaid());
        }
        JsonArray opt = new JsonArray();
        for (OptionalCost c : sa.getOptionalCosts()) {
            opt.add(c.name());
        }
        r.add("optionalCosts", opt);
        if (sa.getChosenList() != null) {
            JsonArray modes = new JsonArray();
            for (AbilitySub m : sa.getChosenList()) {
                modes.add(ids.ability(m));
            }
            r.add("modes", modes);
        }
        r.add("paid", paid(sa.getPaidHash()));
        JsonArray mana = new JsonArray();
        for (SpellAbility m : sa.getPayingManaAbilities()) {
            mana.add(ids.ability(m));
        }
        r.add("manaAbilities", mana);
        JsonArray paying = new JsonArray();
        for (Mana m : sa.getPayingMana()) {
            JsonObject o = new JsonObject();
            o.add("source", m.getSourceCard() == null ? null : ids.card(m.getSourceCard()));
            o.addProperty("color", m.getColor());
            paying.add(o);
        }
        r.add("payingMana", paying);
    }

    /**
     * ADR-0062: what one pay call paid with, read from the paying ability --
     * the mana abilities it added from mark on, and the cards each cost part
     * paid with.
     */
    void payment(Player payer, SpellAbility sa, int mark, int call) {
        JsonObject r = record("payment");
        r.addProperty("call", call);
        r.add("payer", ids.player(payer));
        if (sa == null) {
            return;
        }
        r.add("ability", ids.ability(sa));
        JsonArray mana = new JsonArray();
        List<SpellAbility> used = sa.getPayingManaAbilities();
        for (int i = Math.min(mark, used.size()); i < used.size(); i++) {
            mana.add(ids.ability(used.get(i)));
        }
        r.add("manaAbilities", mana);
        r.add("paid", paid(sa.getPaidHash()));
    }

    private JsonObject part(SpellAbility s) {
        JsonObject o = new JsonObject();
        o.add("ability", ids.ability(s));
        TargetChoices t = s.getTargets();
        o.add("targets", t == null ? new JsonArray() : ids.encode(new ArrayList<>(t)));
        return o;
    }

    private JsonArray paid(Table<String, Boolean, CardCollection> hash) {
        JsonArray a = new JsonArray();
        if (hash == null) {
            return a;
        }
        for (Table.Cell<String, Boolean, CardCollection> c : hash.cellSet()) {
            JsonObject o = new JsonObject();
            o.addProperty("part", c.getRowKey());
            o.addProperty("lki", c.getColumnKey());
            o.add("cards", ids.encode(c.getValue()));
            a.add(o);
        }
        return a;
    }

    /** A shuffle's outcome: the library order cheatShuffle returned. */
    void shuffle(Player p, Iterable<Card> order) {
        JsonObject r = record("shuffle");
        r.add("player", ids.player(p));
        JsonArray cards = new JsonArray();
        for (Card c : order) {
            cards.add(c.getId());
        }
        r.add("order", cards);
        int seat = game.getRegisteredPlayers().indexOf(p);
        if (seat >= 0 && startingLibraries.get(seat) == null) {
            startingLibraries.set(seat, cards);
            // ADR-0059 pairs a seat's deck cards with Go's by deck position:
            // Java creates them in deck order, so by id. Taken now, before a
            // player who loses leaves the game with their cards.
            List<Card> owned = new ArrayList<>();
            for (ZoneType z : List.of(ZoneType.Library, ZoneType.Hand, ZoneType.Sideboard, ZoneType.Command)) {
                owned.addAll(p.getCardsIn(z));
            }
            owned.sort((a, b) -> Integer.compare(a.getId(), b.getId()));
            JsonArray deck = new JsonArray();
            for (Card c : owned) {
                deck.add(ids.card(c));
            }
            seatCards.set(seat, deck);
        }
    }

    @Subscribe
    public void receive(GameEvent ev) {
        // Game's EventBus logs a subscriber's exception and carries on, so a
        // throw here would drop the record silently: count it instead, and
        // GameRecorder refuses the log.
        try {
            recordEvent(ev);
        } catch (RuntimeException e) {
            eventFailures++;
            System.err.println("replay: event " + ev + " not recorded: " + e);
        }
    }

    private void recordEvent(GameEvent ev) {
        if (ev instanceof GameEventRollDie d && d.planar() == null) {
            JsonObject r = record("die");
            r.addProperty("sides", d.sides());
            r.addProperty("face", d.result());
        } else if (ev instanceof GameEventRollDie d) {
            JsonObject r = record("planarDie");
            r.addProperty("face", d.planar().name());
        } else if (ev instanceof GameEventFlipCoin f) {
            JsonObject r = record("coin");
            r.addProperty("heads", f.heads());
            r.addProperty("startingToss", f.startingToss());
        } else {
            normalised(ev);
        }
    }

    /** ADR-0063: the events the L3 diff compares, in the shape Go maps its own to. */
    private void normalised(GameEvent ev) {
        if (ev instanceof GameEventTurnBegan t) {
            JsonObject e = event("turn");
            e.addProperty("turn", t.turnNumber());
            e.addProperty("active", seat(t.turnOwner()));
        } else if (ev instanceof GameEventTurnPhase p) {
            JsonObject e = event("step");
            e.addProperty("step", p.phase().name());
            e.addProperty("active", seat(p.playerTurn()));
        } else if (ev instanceof GameEventSpellAbilityCast c) {
            JsonObject e = event("push");
            e.add("card", card(c.sa().getHostCard()));
            e.addProperty("controller", seat(c.si().getActivatingPlayer()));
            for (SpellAbilityStackInstance si : game.getStack()) {
                if (si.getId() == c.si().getId()) {
                    // For the report only: Go's event names the host alone.
                    e.add("ability", ids.ability(si.getSpellAbility()));
                }
            }
        } else if (ev instanceof GameEventSpellResolved r && !r.hasFizzled()) {
            // A fizzled ability has no resolve: Go emits none for it.
            event("resolve").add("card", card(r.spell().getHostCard()));
        } else if (ev instanceof GameEventGameOutcome) {
            int winner = -1;
            List<Player> players = game.getRegisteredPlayers();
            for (int i = 0; i < players.size(); i++) {
                if (players.get(i).getOutcome() != null && players.get(i).getOutcome().hasWon()) {
                    winner = i;
                }
            }
            event("end").addProperty("winner", winner);
        } else if (ev instanceof GameEventCardChangeZone z) {
            JsonObject e = event("move");
            e.add("card", card(z.card()));
            e.addProperty("from", zone(z.from()));
            e.addProperty("to", zone(z.to()));
        } else if (ev instanceof GameEventCardDamaged d) {
            damage(d.source(), card(d.card()), d.amount());
        } else if (ev instanceof GameEventPlayerDamaged d) {
            damage(d.source(), seatRef(d.target()), d.amount());
        } else if (ev instanceof GameEventPlayerLivesChanged l) {
            JsonObject e = event("life");
            e.addProperty("player", seat(l.player()));
            e.addProperty("delta", l.newLives() - l.oldLives());
        } else if (ev instanceof GameEventCardCounters k) {
            counter(card(k.card()), counterName(k.type()), k.newValue() - k.oldValue());
        } else if (ev instanceof GameEventPlayerCounters k) {
            playerCounters(k.receiver());
        }
    }

    private JsonObject event(String type) {
        JsonObject e = new JsonObject();
        e.addProperty("type", type);
        record("event").add("event", e);
        return e;
    }

    private void damage(CardView source, JsonElement target, int amount) {
        JsonObject e = event("damage");
        e.add("source", card(source));
        e.add("target", target);
        e.addProperty("amount", amount);
    }

    private void counter(JsonElement target, String type, int delta) {
        JsonObject e = event("counter");
        e.add("target", target);
        e.addProperty("counter", type);
        e.addProperty("delta", delta);
    }

    private void playerCounters(PlayerView pv) {
        int seat = seat(pv);
        if (seat < 0) {
            return;
        }
        Player p = game.getRegisteredPlayers().get(seat);
        Map<String, Integer> now = new TreeMap<>();
        for (Multiset.Entry<CounterType> c : p.getCounters().entrySet()) {
            now.merge(counterName(c.getElement()), c.getCount(), Integer::sum);
        }
        Map<String, Integer> was = playerCounters.getOrDefault(p, Map.of());
        TreeSet<String> types = new TreeSet<>(now.keySet());
        types.addAll(was.keySet());
        for (String t : types) {
            int delta = now.getOrDefault(t, 0) - was.getOrDefault(t, 0);
            if (delta != 0) {
                counter(seatRef(pv), t, delta);
            }
        }
        playerCounters.put(p, now);
    }

    // Go's CounterType is the enum constant's name ("P1P1", "LOYALTY"), not
    // CounterEnumType.getName's display form.
    private static String counterName(CounterType t) {
        return t instanceof CounterEnumType e ? e.name() : t.getName();
    }

    private static JsonElement card(CardView c) {
        if (c == null) {
            return JsonNull.INSTANCE;
        }
        JsonObject o = new JsonObject();
        o.addProperty("card", c.getId());
        o.addProperty("name", c.getName());
        return o;
    }

    private JsonElement seatRef(PlayerView pv) {
        JsonObject o = new JsonObject();
        o.addProperty("seat", seat(pv));
        return o;
    }

    private int seat(PlayerView pv) {
        List<Player> players = game.getRegisteredPlayers();
        for (int i = 0; i < players.size(); i++) {
            if (players.get(i).getView() == pv) {
                return i;
            }
        }
        return -1;
    }

    // A token's creation comes from no zone; Go's comes from None.
    private static String zone(ZoneView z) {
        return z == null || z.zoneType() == null ? "None" : z.zoneType().name();
    }

    /**
     * A draw from MyRandom. Inside a controller call it is the AI's and is not
     * logged (ADR-0059; ADR-0062 logs a play's random cost as its paid cards).
     * Outside, a site with a logged outcome is skipped, and any other site --
     * AtRandom, Count$Random -- is logged as an unattributed draw, which R21c
     * reports until that site's outcome is recorded.
     */
    void draw(int bits, int value) {
        if (depth > 0) {
            return;
        }
        String site = StackWalker.getInstance().walk(s -> s
                .map(f -> f.getClassName() + "." + f.getMethodName() + ":" + f.getLineNumber())
                .filter(n -> n.startsWith("forge.") && !n.startsWith("forge.util."))
                .findFirst().orElse("?"));
        if (site.startsWith("forge.token.TokenDb.")) {
            // A token's print: art and set, not a game outcome.
            return;
        }
        for (String known : OUTCOME_SITES) {
            if (site.startsWith(known)) {
                return;
            }
        }
        unattributedDraws++;
        JsonObject r = record("draw");
        r.addProperty("site", site);
        r.addProperty("bits", bits);
        r.addProperty("value", value);
    }

    private JsonObject record(String kind) {
        JsonObject r = new JsonObject();
        r.addProperty("seq", records.size());
        r.addProperty("kind", kind);
        r.addProperty("turn", game.getPhaseHandler().getTurn());
        if (game.getPhaseHandler().getPhase() != null) {
            r.addProperty("phase", game.getPhaseHandler().getPhase().name());
        }
        records.add(r);
        return r;
    }

    void write(Path out, JsonObject header) throws IOException {
        JsonArray libs = new JsonArray();
        startingLibraries.forEach(libs::add);
        header.add("startingLibraries", libs);
        header.addProperty("unresolvedIdentities", ids.unresolved);
        header.addProperty("unattributedDraws", unattributedDraws);
        header.addProperty("eventFailures", eventFailures);
        header.addProperty("records", records.size());
        Gson gson = new GsonBuilder().serializeNulls().create();
        try (Writer w = Files.newBufferedWriter(out, StandardCharsets.UTF_8)) {
            w.write(gson.toJson(header));
            w.write('\n');
            for (JsonObject r : records) {
                w.write(gson.toJson(r));
                w.write('\n');
            }
        }
    }

    /** The Random installed through MyRandom.setRandom: seeded, and every draw reported. */
    static final class RecordingRandom extends Random {
        private static final long serialVersionUID = 1L;
        private transient ReplayRecorder recorder;

        RecordingRandom(long seed) {
            super(seed);
        }

        void attach(ReplayRecorder r) {
            recorder = r;
        }

        @Override
        protected int next(int bits) {
            int v = super.next(bits);
            if (recorder != null) {
                recorder.draw(bits, v);
            }
            return v;
        }
    }
}
