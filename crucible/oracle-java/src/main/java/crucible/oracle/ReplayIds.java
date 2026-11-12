package crucible.oracle;

import java.util.List;
import java.util.Map;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonNull;
import com.google.gson.JsonObject;
import com.google.gson.JsonPrimitive;

import forge.game.CardTraitBase;
import forge.game.Game;
import forge.game.card.Card;
import forge.game.card.CardState;
import forge.game.combat.Combat;
import forge.game.keyword.KeywordInterface;
import forge.game.player.Player;
import forge.game.replacement.ReplacementEffect;
import forge.game.spellability.AbilitySub;
import forge.game.spellability.SpellAbility;
import forge.game.staticability.StaticAbility;
import forge.game.trigger.Trigger;
import forge.game.trigger.WrappedAbility;

/**
 * ADR-0059's identities: a card by Java's per-game id, a player by seat, an
 * ability, trigger, static or replacement by (host card, section, index),
 * never by a Java object id. An object it cannot name is written with
 * "unresolved" and counted, so a recording says how much of it R21c can map.
 */
final class ReplayIds {
    private final Game game;
    int unresolved;

    ReplayIds(Game game) {
        this.game = game;
    }

    JsonElement encode(Object o) {
        if (o == null) {
            return JsonNull.INSTANCE;
        }
        if (o instanceof Boolean b) {
            return new JsonPrimitive(b);
        }
        if (o instanceof Number n) {
            return new JsonPrimitive(n);
        }
        if (o instanceof String s) {
            return new JsonPrimitive(s);
        }
        if (o instanceof Enum<?> e) {
            return new JsonPrimitive(e.name());
        }
        if (o instanceof Card c) {
            return card(c);
        }
        if (o instanceof Player p) {
            return player(p);
        }
        if (o instanceof SpellAbility sa) {
            return ability(sa);
        }
        if (o instanceof Trigger t) {
            return trait(t, "trigger");
        }
        if (o instanceof StaticAbility s) {
            return trait(s, "static");
        }
        if (o instanceof ReplacementEffect r) {
            return trait(r, "replacement");
        }
        if (o instanceof Combat c) {
            return combat(c);
        }
        if (o instanceof forge.game.card.CardView cv) {
            JsonObject t = new JsonObject();
            t.addProperty("card", cv.getId());
            t.addProperty("name", cv.getName());
            return t;
        }
        if (o instanceof forge.game.player.PlayerView pv) {
            for (Player p : game.getRegisteredPlayers()) {
                if (p.getView() == pv) {
                    return player(p);
                }
            }
        }
        if (o instanceof forge.card.mana.ManaCost mc) {
            // Iterable over its colored shards only; the text keeps the generic.
            JsonObject t = new JsonObject();
            t.addProperty("manaCost", mc.getShortString());
            return t;
        }
        if (o instanceof forge.game.player.DelayedReveal d) {
            JsonObject t = new JsonObject();
            t.add("reveal", encode(d.getCards()));
            return t;
        }
        if (o instanceof Map<?, ?> m) {
            JsonArray a = new JsonArray();
            for (Map.Entry<?, ?> e : m.entrySet()) {
                JsonArray pair = new JsonArray();
                pair.add(encode(e.getKey()));
                pair.add(encode(e.getValue()));
                a.add(pair);
            }
            return a;
        }
        if (o instanceof org.apache.commons.lang3.tuple.Pair<?, ?> p) {
            JsonArray pair = new JsonArray();
            pair.add(encode(p.getLeft()));
            pair.add(encode(p.getRight()));
            return pair;
        }
        if (o instanceof Iterable<?> it) {
            JsonArray a = new JsonArray();
            for (Object x : it) {
                a.add(encode(x));
            }
            return a;
        }
        if (o instanceof Object[] arr) {
            JsonArray a = new JsonArray();
            for (Object x : arr) {
                a.add(encode(x));
            }
            return a;
        }
        // Costs, mana costs, views and the like: their text, which R21c
        // compares as a string. Not an identity, but not unresolved either.
        JsonObject t = new JsonObject();
        String text = o.toString();
        // Object's own toString is an address, which differs per run.
        if (!text.equals(o.getClass().getName() + "@" + Integer.toHexString(o.hashCode()))) {
            t.addProperty("text", text);
        }
        t.addProperty("class", o.getClass().getSimpleName());
        return t;
    }

    JsonObject card(Card c) {
        JsonObject o = new JsonObject();
        o.addProperty("card", c.getId());
        o.addProperty("name", c.getName());
        return o;
    }

    JsonObject player(Player p) {
        JsonObject o = new JsonObject();
        o.addProperty("seat", game.getRegisteredPlayers().indexOf(p));
        return o;
    }

    /**
     * A spell ability: a trigger's wrapper names the trigger, a sub-ability
     * names its root and its depth in the chain, a copy names its original.
     */
    JsonObject ability(SpellAbility sa) {
        if (sa instanceof WrappedAbility w) {
            JsonObject o = trait(w.getTrigger(), "trigger");
            o.add("wrapped", ability(w.getWrappedAbility()));
            return o;
        }
        int depth = 0;
        SpellAbility root = sa;
        while (root instanceof AbilitySub sub && sub.getParent() != null) {
            root = sub.getParent();
            depth++;
        }
        SpellAbility def = root.getOriginalAbility() != null ? root.getOriginalAbility() : root;
        JsonObject o;
        if (def.isTrigger() && def.getTrigger() != null) {
            o = trait(def.getTrigger(), "trigger");
        } else if (def.isReplacementAbility() && def.getReplacementEffect() != null) {
            // A replacement's ReplaceWith ability: named by its replacement.
            o = trait(def.getReplacementEffect(), "replacement");
            o.addProperty("replaceWith", true);
        } else if (def.isLandAbility()) {
            o = new JsonObject();
            o.add("host", card(def.getHostCard()));
            o.addProperty("section", "land");
        } else {
            o = trait(def, "ability");
        }
        if (depth > 0) {
            o.addProperty("sub", depth);
        }
        if (sa.getApi() != null) {
            o.addProperty("api", sa.getApi().name());
        }
        return o;
    }

    /**
     * (host, section, index): the index among the host state's intrinsic
     * traits of that kind that no keyword made, or (keyword, trait) for a
     * keyword's trait. A trait the per-game id does not find (a copy made for
     * an alternative cost, a cloned or re-made state) is found by its script
     * params, and says so ("by": "params").
     */
    JsonObject trait(CardTraitBase t, String section) {
        JsonObject o = new JsonObject();
        Card host = t.getHostCard();
        if (host == null) {
            return unresolved(o, t);
        }
        o.add("host", card(host));
        o.addProperty("section", section);
        CardState state = t.getCardState() != null ? t.getCardState() : host.getCurrentState();
        o.addProperty("state", state.getStateName().name());
        KeywordInterface kw = t.getKeyword();
        if (kw != null) {
            // The keyword by its text and its index among the host's
            // keywords of that text; then the trait within the keyword.
            int ki = 0;
            for (KeywordInterface k : host.getKeywords()) {
                if (k == kw) {
                    break;
                }
                if (k.getOriginal().equals(kw.getOriginal())) {
                    ki++;
                }
            }
            o.addProperty("keyword", kw.getOriginal());
            o.addProperty("keywordIndex", ki);
            List<? extends CardTraitBase> traits = keywordTraits(kw, section);
            for (int j = 0; j < traits.size(); j++) {
                if (same(traits.get(j), t)) {
                    o.addProperty("index", j);
                    return o;
                }
            }
            // A copy of the keyword's own trait (bestow cast from a graveyard
            // under another static): by params, then by API.
            int k = keywordCopy(traits, t);
            if (k >= 0) {
                o.addProperty("index", k);
                o.addProperty("by", "params");
                return o;
            }
            // Flashback, evoke and the like cast a copy of the spell under the
            // keyword's cost: the spell it copies is the definition.
        } else if (!t.isIntrinsic()) {
            if (t instanceof SpellAbility sa && sa.getGrantorStatic() != null) {
                // Granted by a static: (grantor's static, ability).
                o.addProperty("section", "granted");
                o.add("grantor", trait(sa.getGrantorStatic(), "static"));
                return o;
            }
            // A rule-made ability with no definition (companion's special
            // action): named by its host and API.
            o.addProperty("section", "rule");
            return o;
        }
        int i = indexIn(stateTraits(state, section), t, false);
        if (i < 0 && state != host.getCurrentState()) {
            i = indexIn(stateTraits(host.getCurrentState(), section), t, false);
        }
        if (i < 0) {
            i = indexIn(stateTraits(state, section), t, true);
            if (i < 0) {
                i = indexIn(stateTraits(host.getCurrentState(), section), t, true);
            }
            if (i >= 0) {
                o.addProperty("by", "params");
            }
        }
        if (i < 0 && t instanceof SpellAbility sa && sa.getApi() != null) {
            // A copy whose params changed (Rebound casts the spell without
            // paying its mana cost): the one spell ability of that API.
            i = uniqueApi(stateTraits(host.getCurrentState(), section), sa);
            if (i >= 0) {
                o.addProperty("by", "api");
            }
        }
        if (i >= 0) {
            o.addProperty("index", i);
            return o;
        }
        if (t instanceof Trigger tr && tr.getSpawningAbility() != null) {
            // A delayed trigger: created at resolution, named by its host,
            // mode and the ability that created it.
            o.addProperty("section", "delayed");
            o.addProperty("mode", tr.getMode().name());
            o.add("spawnedBy", ability(tr.getSpawningAbility()));
            return o;
        }
        return unresolved(o, t);
    }

    // t's index among the intrinsic, keyword-free traits, matched by id or,
    // with byParams, by equal script params.
    private static int indexIn(Iterable<? extends CardTraitBase> traits, CardTraitBase t, boolean byParams) {
        int i = 0;
        for (CardTraitBase x : traits) {
            if (x.getKeyword() != null || !x.isIntrinsic()) {
                continue;
            }
            if (byParams ? x.getClass() == t.getClass() && x.getMapParams().equals(t.getMapParams()) : same(x, t)) {
                return i;
            }
            i++;
        }
        return -1;
    }

    // One definition: the same object, or a copy of it (an LKI or a moving
    // card's trait keeps the per-game id of the trait it copies).
    private static boolean same(CardTraitBase a, CardTraitBase b) {
        if (a == b) {
            return true;
        }
        if (a instanceof SpellAbility x && b instanceof SpellAbility y) {
            return x.getId() == y.getId();
        }
        if (a instanceof Trigger x && b instanceof Trigger y) {
            return x.getId() == y.getId();
        }
        if (a instanceof ReplacementEffect x && b instanceof ReplacementEffect y) {
            return x.getId() == y.getId();
        }
        if (a instanceof StaticAbility x && b instanceof StaticAbility y) {
            return x.getId() == y.getId();
        }
        return false;
    }

    private static int keywordCopy(List<? extends CardTraitBase> traits, CardTraitBase t) {
        int api = -1;
        for (int j = 0; j < traits.size(); j++) {
            CardTraitBase x = traits.get(j);
            if (x.getClass() == t.getClass() && x.getMapParams().equals(t.getMapParams())) {
                return j;
            }
            if (x instanceof SpellAbility y && t instanceof SpellAbility sa && y.getApi() == sa.getApi()) {
                api = api == -1 ? j : -2;
            }
        }
        return api >= 0 ? api : -1;
    }

    private static int uniqueApi(Iterable<? extends CardTraitBase> traits, SpellAbility sa) {
        int i = 0;
        int found = -1;
        for (CardTraitBase x : traits) {
            if (x.getKeyword() != null || !x.isIntrinsic()) {
                continue;
            }
            if (x instanceof SpellAbility y && y.getApi() == sa.getApi()) {
                if (found >= 0) {
                    return -1;
                }
                found = i;
            }
            i++;
        }
        return found;
    }

    private static List<? extends CardTraitBase> keywordTraits(KeywordInterface kw, String section) {
        return List.copyOf(switch (section) {
            case "trigger" -> kw.getTriggers();
            case "static" -> kw.getStaticAbilities();
            case "replacement" -> kw.getReplacements();
            default -> kw.getAbilities();
        });
    }

    private static Iterable<? extends CardTraitBase> stateTraits(CardState state, String section) {
        return switch (section) {
            case "trigger" -> state.getTriggers();
            case "static" -> state.getStaticAbilities();
            case "replacement" -> state.getReplacementEffects();
            default -> state.getSpellAbilities();
        };
    }

    private JsonObject unresolved(JsonObject o, Object t) {
        unresolved++;
        o.addProperty("unresolved", true);
        o.addProperty("text", String.valueOf(t));
        return o;
    }

    JsonArray combat(Combat c) {
        JsonArray a = new JsonArray();
        for (Card att : c.getAttackers()) {
            JsonObject o = new JsonObject();
            o.add("attacker", card(att));
            o.add("defender", encode(c.getDefenderByAttacker(att)));
            o.add("blockers", encode(c.getBlockers(att)));
            a.add(o);
        }
        return a;
    }
}
