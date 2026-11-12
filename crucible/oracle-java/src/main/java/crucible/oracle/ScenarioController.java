package crucible.oracle;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.function.BooleanSupplier;
import java.util.function.Predicate;

import forge.LobbyPlayer;
import forge.card.ColorSet;
import forge.card.ICardFace;
import forge.card.MagicColor;
import forge.card.mana.ManaCostShard;
import forge.game.spellability.AbilitySub;
import org.apache.commons.lang3.tuple.ImmutablePair;
import forge.game.Game;
import forge.game.GameEntity;
import forge.game.GameObject;
import forge.game.card.Card;
import forge.game.card.CardCollection;
import forge.game.card.CardCollectionView;
import forge.game.player.DelayedReveal;
import forge.game.replacement.ReplacementEffect;
import forge.game.player.PlayerActionConfirmMode;
import forge.game.combat.Combat;
import forge.game.combat.CombatUtil;
import forge.game.keyword.Keyword;
import forge.game.ability.ApiType;
import forge.card.CardStateName;
import forge.card.mana.ManaCost;
import forge.card.mana.ManaCostParser;
import forge.game.cost.Cost;
import forge.game.cost.CostDecisionMakerBase;
import forge.game.cost.CostExert;
import forge.game.cost.CostPartMana;
import forge.game.cost.CostPayment;
import forge.ai.ComputerUtilCost;
import forge.game.spellability.OptionalCostValue;
import forge.game.cost.CostPartWithList;
import forge.game.mana.ManaConversionMatrix;
import forge.game.mana.ManaCostBeingPaid;
import forge.game.mana.Mana;
import forge.game.player.PlaySpellAbility;
import forge.game.spellability.AbilityManaPart;
import forge.game.player.Player;
import forge.game.spellability.SpellAbility;
import forge.game.spellability.SpellPermanent;
import forge.game.spellability.TargetChoices;
import forge.game.spellability.TargetRestrictions;
import forge.game.trigger.WrappedAbility;
import forge.game.zone.ZoneType;
import forge.util.ITriggerEvent;
import forge.util.collect.FCollectionView;
import forge.gamesimulationtests.util.PlayerControllerForTests;

/**
 * ADR-0060's controller: upstream's PlayerControllerForTests, which answers
 * all of PlayerController, with the overrides the L2 runner needs.
 *
 * At priority it passes, unless the run has reached its checkpoint (the Go
 * run's final turn and phase): then it throws {@link Checkpoint} so the runner
 * captures the state right there, after Java's state-based actions and with
 * the triggers they raised on the stack -- the point Go's harness stops at.
 * Combat declarations pop the queued answers ({@link ScenarioAnswers}); with
 * none queued the seat declares nothing.
 */
final class ScenarioController extends PlayerControllerForTests {
    /** Thrown at the checkpoint's priority; the runner catches it and dumps. */
    static final class Checkpoint extends RuntimeException {
        private static final long serialVersionUID = 1L;

        Checkpoint() {
            super(null, null, false, false);
        }
    }

    private final BooleanSupplier atCheckpoint;
    private final ScenarioAnswers answers;

    ScenarioController(Game game, Player player, LobbyPlayer lobbyPlayer, BooleanSupplier atCheckpoint,
            ScenarioAnswers answers) {
        super(game, player, lobbyPlayer);
        this.atCheckpoint = atCheckpoint;
        this.answers = answers;
    }

    // At its priority the seat makes its queued plays of this turn and phase
    // in actions.log order: tapformana activates the land's mana ability for
    // the named color now, castspell and playland are chosen here and played
    // by playChosenSpellAbility. Another seat's play first, or none: pass.
    @Override
    public List<SpellAbility> chooseSpellAbilityToPlay() {
        if (atCheckpoint.getAsBoolean()) {
            throw new Checkpoint();
        }
        boolean refused = false;
        for (ScenarioAnswers.Play play; (play = answers.nextPlayFor(player)) != null;) {
            if (play.verb().equals("pass")) {
                return null;
            }
            if (play.verb().equals("concede")) {
                // S27: Go's Concede, at once; the game may be over after it.
                for (Player p : player.getGame().getPlayers()) {
                    if (p.getName().equalsIgnoreCase(play.player())) {
                        p.concede();
                    }
                }
                if (player.getGame().isGameOver()) {
                    return null;
                }
                continue;
            }
            if (play.verb().equals("paymanacost")) {
                payFreeManaCost(play.arg());
                continue;
            }
            Card card = answers.card(play.id());
            if (play.verb().equals("tapformana")) {
                tapForMana(card, play.arg());
                continue;
            }
            if (play.verb().equals("activate") || play.verb().equals("manaability")) {
                SpellAbility sa = printedAbility(card, Integer.parseInt(play.arg()));
                if (sa.isManaAbility() != play.verb().equals("manaability")) {
                    throw new IllegalStateException(play.verb() + " " + play.arg() + " of " + card + " is " + sa);
                }
                sa.setActivatingPlayer(player);
                if (!sa.isManaAbility()) {
                    return List.of(sa);
                }
                // Activated as a player activates it (MagicStack.add's mana
                // branch, MagicStack.java:222-241), so it counts as activated
                // and fires AbilityCast and AbilityResolves (Tyvar the
                // Bellicose), as Go's manaability does; a tapformana is the
                // harness's own payment and skips the stack.
                if (!PlaySpellAbility.playSpellAbility(this, player, sa)) {
                    throw new IllegalStateException(card + " did not activate " + sa);
                }
                continue;
            }
            if (play.verb().equals("unlockdoor")) {
                // S28: the Room's unlock special action for that door.
                SpellAbility sa = card.getUnlockAbility(CardStateName.smartValueOf(play.arg()));
                sa.setActivatingPlayer(player);
                return List.of(sa);
            }
            if (ScenarioRunner.SPECIAL.contains(play.verb())) {
                return List.of(specialAction(card, play.verb()));
            }
            boolean land = play.verb().equals("playland");
            if (play.refused()) {
                refuse(card, land);
                refused = true;
                continue;
            }
            List<SpellAbility> ways = new ArrayList<>();
            for (SpellAbility sa : card.getAllPossibleAbilities(player, true)) {
                if ((land ? sa.isLandAbility() : sa.isSpell()) && castsFace(card, sa, play.arg())) {
                    ways.add(sa);
                }
            }
            if (ways.isEmpty()) {
                throw new IllegalStateException(player + " cannot " + play.verb() + " " + card + " " + play.arg());
            }
            return List.of(castOption(card, ways));
        }
        // A refused play changed nothing: the checkpoint it held back may be
        // this priority, before the seat passes the step away (S17).
        if (refused && atCheckpoint.getAsBoolean()) {
            throw new Checkpoint();
        }
        return null;
    }

    // S19: Go's Game.PayManaCost, a mana cost paid outside any spell: X from
    // the payx queue, then the pool pays it as applyManaToCost pays a
    // spell's, Phyrexian answers included, on behalf of an empty ability.
    private void payFreeManaCost(String text) {
        ManaCostBeingPaid toPay = new ManaCostBeingPaid(new ManaCost(new ManaCostParser(text)));
        Card host = new Card(player.getGame().nextCardId(), player.getGame());
        host.setOwner(player);
        SpellAbility sa = new SpellAbility.EmptySa(host, player);
        if (toPay.getXcounter() > 0) {
            Integer x = answers.nextPayX();
            if (x == null) {
                throw new IllegalStateException("paymanacost " + text + " needs a payx answer");
            }
            toPay.setXManaCostPaid(x, null);
        }
        applyManaToCost(toPay, sa, "paymanacost", null, false);
    }

    // S26: a foretell, plot, suspend or turnfaceup line plays the card's
    // special action of that kind (CR 116.2).
    private SpellAbility specialAction(Card card, String verb) {
        for (SpellAbility sa : card.getAllPossibleAbilities(player, true)) {
            boolean match = switch (verb) {
                case "foretell" -> sa.isForetelling();
                case "plot" -> sa.isPlotting();
                case "suspend" -> !sa.isSpell() && sa.isKeyword(Keyword.SUSPEND);
                default -> sa.isTurnFaceUp();
            };
            if (match) {
                return sa;
            }
        }
        throw new IllegalStateException(player + " cannot " + verb + " " + card);
    }

    // S22: a spell with more than one way to cast it takes the next option
    // answer, an index into the ways Java offers (getAllPossibleAbilities
    // order: the printed spell, then keyword and alternative-cost spells), as
    // Go's chooseCastOption indexes its castOptions. None queued: the first.
    private SpellAbility castOption(Card card, List<SpellAbility> ways) {
        if (ways.size() == 1 || ways.get(0).isLandAbility()) {
            return ways.get(0);
        }
        Integer pick = answers.nextOption();
        if (pick == null) {
            return ways.get(0);
        }
        if (pick < 0 || pick >= ways.size()) {
            throw new IllegalStateException("option " + pick + " of " + card + "'s casts " + ways);
        }
        return ways.get(pick);
    }

    // S20: a castspell's face argument. None casts the card's first spell;
    // "face" its second face's (an Adventure, Omen, Aftermath or Disturb
    // spell, a Split card's right half: any state but Original and
    // LeftSplit); "fuse" a Split card's fused spell, which
    // AbilityFactory.buildFusedAbility puts on the Original state.
    private static boolean castsFace(Card card, SpellAbility sa, String arg) {
        CardStateName state = sa.getCardStateName();
        if ("fuse".equals(arg)) {
            return card.isSplitCard() && state == CardStateName.Original;
        }
        if ("face".equals(arg)) {
            return state != CardStateName.Original && state != CardStateName.LeftSplit;
        }
        // S28: a cast naming a Room door casts that door's spell.
        if ("LeftSplit".equals(arg) || "RightSplit".equals(arg)) {
            return state == CardStateName.smartValueOf(arg);
        }
        return true;
    }

    // S17: a play Go refused. Java refuses it too when the card offers no
    // playable spell or land ability, or when playing it fails; a play that
    // goes through is an error, since Go's state assumes it never happened.
    private void refuse(Card card, boolean land) {
        for (SpellAbility sa : card.getAllPossibleAbilities(player, true)) {
            if (land ? sa.isLandAbility() : sa.isSpell()) {
                refusing = true;
                try {
                    if (PlaySpellAbility.playSpellAbility(this, player, sa)) {
                        throw new IllegalStateException(player + " played " + sa + ", which Go refused");
                    }
                } finally {
                    refusing = false;
                }
                return;
            }
        }
    }

    // Set while refuse() tries a play Go refused: a cost the pool cannot pay
    // cancels the play (PlaySpellAbility refunds the mana, :417-418) instead
    // of taking a tapformana Go made after the refusal.
    private boolean refusing;

    // Go's ChooseAbilitiesForEffect: indices into the abilities offered, in
    // Choices$ order (GenericChoice, ChooseGenericEffect.java:102).
    @Override
    public List<SpellAbility> chooseSpellAbilitiesForEffect(List<SpellAbility> spells, SpellAbility sa, String title,
            int num, Map<String, Object> params) {
        List<Integer> picks = answers.bound() ? answers.nextAbilityChoice() : null;
        if (picks == null) {
            return super.chooseSpellAbilitiesForEffect(spells, sa, title, num, params);
        }
        List<SpellAbility> out = new ArrayList<>();
        for (int i : picks) {
            if (i < 0 || i >= spells.size()) {
                throw new IllegalStateException("abilitychoice " + i + " of " + spells.size() + " abilities");
            }
            out.add(spells.get(i));
        }
        return out;
    }

    @Override
    public boolean playChosenSpellAbility(SpellAbility sa) {
        if (!PlaySpellAbility.playSpellAbility(this, player, sa)) {
            throw new IllegalStateException(player + "'s play of " + sa + " was refused");
        }
        return true;
    }

    // Go's abilityAt: the index'th of the card's A: lines, then the
    // abilities its keywords and the statics granting it add, in that order.
    // Java keeps the same order after the basic spell or land ability
    // (CardState.getSpellAbilities, updateSpellAbilities).
    private static SpellAbility printedAbility(Card card, int index) {
        int i = 0;
        for (SpellAbility sa : card.getCurrentState().getSpellAbilities()) {
            if (sa instanceof SpellPermanent || sa.isLandAbility() || sa == card.getCurrentState().getAuraSpell()) {
                continue;
            }
            if (i++ == index) {
                return sa;
            }
        }
        throw new IllegalStateException("ability " + index + " of " + card + " among " + card.getCurrentState().getSpellAbilities());
    }

    // Go's TapLandForMana: the land's mana ability for one color, resolved
    // without the stack (a mana ability, CR 605.3), by its controller.
    private static void tapForMana(Card card, String color) {
        Player player = card.getController();
        for (SpellAbility ma : card.getManaAbilities()) {
            AbilityManaPart part = ma.getManaPart();
            if (!part.canProduce(color, ma)) {
                continue;
            }
            if (part.isAnyMana() || part.isComboMana()) {
                part.setExpressChoice(color);
            }
            ma.setActivatingPlayer(player);
            if (!PlaySpellAbility.playSpellAbilityNoStack(player.getController(), player, ma, true)) {
                throw new IllegalStateException(card + " did not tap for " + color);
            }
            return;
        }
        throw new IllegalStateException(card + " has no mana ability for " + color);
    }

    // A spell a Play effect casts is cast the way Go's castspell is, its
    // targets from the queue (PlayerControllerHuman.playSaFromPlayEffect).
    @Override
    public boolean playSaFromPlayEffect(SpellAbility tgtSA) {
        return PlaySpellAbility.playSpellAbility(this, player, tgtSA);
    }

    // Go asks ConfirmEffect whether to choose new targets for a copy
    // (copyspellabilityeffect.go), then the targets queue.
    @Override
    public TargetChoices chooseNewTargetsFor(SpellAbility ability, Predicate<GameObject> filter, boolean optional) {
        if (optional) {
            Boolean v = answers.nextConfirmEffect();
            if (v == null || !v) {
                return null;
            }
        }
        ability.resetTargets();
        return chooseTargetsFor(ability) ? ability.getTargets() : null;
    }

    // Go's targets queue: the next answer, every entity added as chosen.
    // Go asks nothing where there is no candidate (targeting.go
    // resolveTargets: no legal set unless TargetMin$ is 0), nor for an Aura
    // spell with one (castspell.go castAura).
    @Override
    public boolean chooseTargetsFor(SpellAbility currentAbility) {
        TargetRestrictions tgt = currentAbility.getTargetRestrictions();
        if (tgt.getNumCandidates(currentAbility) == 0) {
            return tgt.getMinTargets(currentAbility.getHostCard(), currentAbility) == 0;
        }
        // Nor where the target count is zero (a promised gift's X = 0, Jolly
        // Gerbils): targeting.go asks nothing when max and min are both 0.
        if (tgt.getMaxTargets(currentAbility.getHostCard(), currentAbility) == 0) {
            return tgt.getMinTargets(currentAbility.getHostCard(), currentAbility) == 0;
        }
        if (currentAbility.isSpell() && currentAbility.getHostCard().isAura()) {
            List<GameEntity> candidates = tgt.getAllCandidates(currentAbility);
            // Go's castAura asks ChooseEnchantTarget for the Aura's target.
            Card enchanted = answers.nextEnchantTarget(candidates);
            if (enchanted != null) {
                currentAbility.getTargets().add(enchanted);
                return true;
            }
            if (candidates.size() == 1) {
                currentAbility.getTargets().add(candidates.get(0));
                return true;
            }
        }
        for (GameObject e : answers.nextTargets()) {
            // A card id on the stack names the spell (Go's TargetStack).
            GameObject o = e instanceof Card c && c.isInZone(ZoneType.Stack)
                    ? c.getGame().getStack().getSpellMatchingHost(c) : e;
            // Java offers only what can still be chosen next to the targets
            // already picked (a MaxTotalTargetCMC$ sum); Go drops the rest
            // the same way (targeting.go trimTargetSet). A target illegal on
            // its own is kept, as Go keeps it: the spell fizzles.
            if (currentAbility.canTarget(o) || !alone(currentAbility, o)) {
                currentAbility.getTargets().add(o);
            }
        }
        // Fewer targets than the minimum is a selection that cannot finish,
        // as TargetSelection's (Go's resolveTargets refuses the cast).
        if (currentAbility.getTargets().size() < tgt.getMinTargets(currentAbility.getHostCard(), currentAbility)) {
            currentAbility.resetTargets();
            return false;
        }
        if (currentAbility.isDividedAsYouChoose()) {
            // Go's divideAmongTargets (dividedtargets.go) asks nothing for one
            // target (the total) or one per point (1 each); any other split is
            // queue divide, not mapped yet.
            List<GameObject> chosen = List.copyOf(currentAbility.getTargets());
            int total = currentAbility.getStillToDivide();
            List<Integer> shares = chosen.size() != 1 && chosen.size() != total ? answers.nextDivision() : null;
            if (chosen.size() != 1 && chosen.size() != total) {
                if (shares == null || shares.size() != chosen.size()
                        || shares.stream().mapToInt(Integer::intValue).sum() != total) {
                    throw new IllegalStateException("dividing " + total + " among " + chosen + " needs a fitting queue divide, got " + shares);
                }
            }
            for (int k = 0; k < chosen.size(); k++) {
                currentAbility.addDividedAllocation(chosen.get(k),
                        shares != null ? shares.get(k) : chosen.size() == 1 ? total : 1);
            }
        }
        return true;
    }

    // Whether sa could target o with nothing else chosen.
    private static boolean alone(SpellAbility sa, GameObject o) {
        List<GameObject> chosen = List.copyOf(sa.getTargets());
        sa.getTargets().clear();
        boolean ok = sa.canTarget(o);
        sa.getTargets().addAll(chosen);
        return ok;
    }

    // Mana is paid the way a human pays (PlayerControllerHuman.payManaCost):
    // PlaySpellAbility.payManaCost announces X and applies the cost
    // adjustments (a power-up's reduction, CostAdjustment.java:225), then
    // asks applyManaToCost.
    @Override
    public boolean payManaCost(ManaCost toPay, CostPartMana costPartMana, SpellAbility sa, String prompt,
            ManaConversionMatrix matrix, boolean effect) {
        return PlaySpellAbility.payManaCost(this, toPay, costPartMana, sa, player, prompt, matrix, effect);
    }

    // The cost is paid from the pool the queued tapformana lines filled; a
    // cost the pool cannot pay takes the tapformana plays Go made in this
    // step (a cost paid before priority), else it is an error, never an
    // automatic tap.
    //
    // Go asks ChoosePayPhyrexian once per one-color Phyrexian shard
    // (manapay.go); a false answer pays it with 2 life, deducted once the
    // mana is paid, the way InputPayManaOfCostPayment does.
    @Override
    public boolean applyManaToCost(ManaCostBeingPaid toPay, SpellAbility ability, String prompt,
            ManaConversionMatrix matrix, boolean effect) {
        int life = 0;
        if (answers.bound()) {
            for (ManaCostShard shard : toPay.getUnpaidShards()) {
                if (shard.isPhyrexian() && Integer.bitCount(shard.getColorMask() & MagicColor.ALL_COLORS) == 2) {
                    // Go's ChoosePayHybridPhyrexian: "life" pays the shard
                    // with 2 life, a color leaves it to the pool.
                    if ("life".equals(answers.nextPayHybridPhyrexian())) {
                        toPay.decreaseShard(shard, 1);
                        ability.setSpendPhyrexianMana(true);
                        life += 2;
                    }
                    continue;
                }
                if (!shard.isPhyrexian() || Integer.bitCount(shard.getColorMask() & MagicColor.ALL_COLORS) != 1) {
                    continue;
                }
                Boolean payColor = answers.nextPayPhyrexian();
                if (payColor == null) {
                    break;
                }
                if (!payColor) {
                    toPay.decreaseShard(shard, 1);
                    ability.setSpendPhyrexianMana(true);
                    life += 2;
                }
            }
        }
        if (answers.bound()) {
            payGenericAsAnswered(toPay, ability);
        }
        // payManaCostFromPool pays each unpaid shard once a call: a {2/W}
        // paid with colorless becomes {1}, paid by the next call.
        for (String before = toPay.toString();
                !player.getManaPool().payManaCostFromPool(toPay, ability, false, ability.getPayingMana());
                before = toPay.toString()) {
            if (!toPay.toString().equals(before)) {
                continue;
            }
            if (refusing) {
                return false;
            }
            ScenarioAnswers.Play tap = answers.nextTapNow();
            if (tap == null) {
                throw new IllegalStateException(player + "'s pool (" + player.getManaPool() + ") cannot pay " + toPay
                        + " for " + ability);
            }
            tapForMana(answers.card(tap.id()), tap.arg());
        }
        if (life > 0) {
            player.payLife(life, ability, effect);
        }
        return true;
    }

    // Go's DeclareCombatAttackers: each queued attacker attacks the only
    // defender there is, or the next queued attack target when there are
    // several (attack.go chooseAttackTargets).
    // S25: Go asks ChoosePayGeneric once per generic unit, a declined {2/W}
    // adding two (manapay.go); each answer spends a mana of that type from
    // the pool first, as Go spends the resolved shards. A mana not in the
    // pool yet (one tapped during the payment) is left to the pool.
    private void payGenericAsAnswered(ManaCostBeingPaid toPay, SpellAbility ability) {
        int generic = toPay.getGenericManaAmount();
        for (ManaCostShard shard : toPay.getUnpaidShards()) {
            if (shard.isOr2Generic()) {
                Boolean color = answers.nextPayMonocoloredHybrid();
                if (color != null && !color) {
                    generic += shard.getCmc();
                }
            }
        }
        for (int i = 0; i < generic; i++) {
            String kind = answers.nextPayGeneric();
            if (kind == null) {
                return;
            }
            for (Mana m : player.getManaPool()) {
                boolean match = kind.equalsIgnoreCase("C") ? m.isColorless()
                        : !m.isColorless() && m.getColor() == MagicColor.fromName(kind.charAt(0));
                if (match && player.getManaPool().tryPayCostWithMana(ability, toPay, m, false)) {
                    ability.getPayingMana().add(m);
                    break;
                }
            }
        }
    }

    @Override
    public void declareAttackers(Player attacker, Combat combat) {
        List<Integer> ids = answers.nextAttackers();
        if (ids != null && answers.refused(ids)) {
            // S23: Go refused this declaration; Java must find it illegal too.
            // It is undone and the next answer declared.
            List<Card> added = addAttackers(combat, ids);
            if (CombatUtil.validateAttackers(combat)) {
                throw new IllegalStateException("Java accepts the attack declaration Go refused: " + ids);
            }
            added.forEach(combat::removeFromCombat);
            ids = answers.nextAttackers();
        }
        if (ids == null) {
            return;
        }
        addAttackers(combat, ids);
        if (!CombatUtil.validateAttackers(combat)) {
            throw new IllegalStateException("illegal attack declaration " + ids);
        }
    }

    private List<Card> addAttackers(Combat combat, List<Integer> ids) {
        List<Card> added = new ArrayList<>();
        for (int id : ids) {
            List<GameEntity> defenders = List.copyOf(combat.getDefenders());
            GameEntity defender = defenders.size() == 1 ? defenders.get(0) : answers.nextAttackTarget();
            Card c = answers.card(id);
            combat.addAttacker(c, defender);
            added.add(c);
        }
        return added;
    }

    // Go's DeclareCombatBlockers: the queued blocker=attacker pairs.
    @Override
    public void declareBlockers(Player defender, Combat combat) {
        List<int[]> pairs = answers.nextBlocks();
        if (pairs != null && answers.refused(pairs)) {
            // S23: Go refused these blocks; Java must find them illegal too:
            // a blocker that can't block its attacker, or validateBlocks.
            boolean each = pairs.stream()
                    .allMatch(pair -> CombatUtil.canBlock(answers.card(pair[1]), answers.card(pair[0])));
            if (each) {
                List<Card> added = new ArrayList<>();
                for (int[] pair : pairs) {
                    Card blocker = answers.card(pair[0]);
                    combat.addBlocker(answers.card(pair[1]), blocker);
                    added.add(blocker);
                }
                String invalid = CombatUtil.validateBlocks(combat, defender);
                added.forEach(combat::undoBlockingAssignment);
                if (invalid == null) {
                    throw new IllegalStateException("Java accepts the blocks Go refused");
                }
            }
            pairs = answers.nextBlocks();
        }
        if (pairs == null) {
            return;
        }
        for (int[] pair : pairs) {
            Card blocker = answers.card(pair[0]);
            Card attacker = answers.card(pair[1]);
            if (!CombatUtil.canBlock(attacker, blocker)) {
                throw new IllegalStateException(blocker + " can't block " + attacker);
            }
            combat.addBlocker(attacker, blocker);
        }
        String invalid = CombatUtil.validateBlocks(combat, defender);
        if (invalid != null) {
            throw new IllegalStateException(invalid);
        }
    }

    // Go's dealAttackerDamage (combatdamage.go): a lone blocker takes all of
    // it, or with trample the lethal part and the rest goes to what is
    // attacked (the null key, Combat.assignAttackersDamage). Dividing among
    // several is a queued damage answer, not mapped yet.
    @Override
    public Map<Card, Integer> assignCombatDamage(Card attacker, CardCollectionView blockers, CardCollectionView remaining,
            int damageDealt, GameEntity defender, boolean overrideOrder) {
        if (blockers.size() != 1) {
            // Go's AssignCombatDamage, asked only with several recipients;
            // with trample what is left goes to what is attacked, as
            // combatdamage.go deals it, else it is not dealt.
            Map<Card, Integer> map = answers.nextDamage(blockers);
            if (map == null) {
                throw new IllegalStateException("dividing " + attacker + "'s damage among " + blockers + " needs queue damage");
            }
            int left = damageDealt - map.values().stream().mapToInt(Integer::intValue).sum();
            if (left > 0 && attacker.hasKeyword(Keyword.TRAMPLE) && attacker.isAttacking()) {
                map.put(null, left);
            }
            return map;
        }
        Card blocker = blockers.get(0);
        int toBlocker = damageDealt;
        // The blocker's half (Combat.assignBlockersDamage) has no trample.
        if (attacker.hasKeyword(Keyword.TRAMPLE) && attacker.isAttacking()) {
            int lethal = attacker.hasKeyword(Keyword.DEATHTOUCH) ? 1 : Math.max(0, blocker.getLethalDamage());
            toBlocker = Math.min(lethal, damageDealt);
        }
        Map<Card, Integer> map = new HashMap<>();
        map.put(blocker, toBlocker);
        if (damageDealt > toBlocker) {
            map.put(null, damageDealt - toBlocker);
        }
        return map;
    }

    // Upstream's version puts each trigger on the stack with no targets
    // (prepareSingleSa chooses them only for TargetingPlayer). A trigger is
    // played as a spell is, its targets from the queue
    // (PlayerControllerHuman.orderAndPlaySimultaneousSa); the order stays
    // upstream's, the list's.
    // Go's triggerorder lists the triggers in resolve order, first to resolve
    // first; the loop below puts the list on the stack front to back, so the
    // queued order is returned reversed. Unqueued, upstream's (the list's).
    @Override
    public List<SpellAbility> orderSimultaneousSa(List<SpellAbility> activePlayerSAs) {
        // Go asks only when two or more triggered together.
        if (activePlayerSAs.size() < 2) {
            return super.orderSimultaneousSa(activePlayerSAs);
        }
        List<Integer> order = answers.nextTriggerOrder(activePlayerSAs.size());
        if (order == null) {
            return super.orderSimultaneousSa(activePlayerSAs);
        }
        List<SpellAbility> out = new java.util.ArrayList<>();
        for (int i = order.size() - 1; i >= 0; i--) {
            out.add(activePlayerSAs.get(order.get(i)));
        }
        return out;
    }

    // ClashEffect.java:129-140 asks each clashing player through
    // clashMoveToTopOrBottom; Go's cardontop answers it. Unqueued, upstream's.
    @Override
    public boolean willPutCardOnTop(Card c) {
        Boolean top = answers.nextCardOnTop();
        return top == null ? super.willPutCardOnTop(c) : top;
    }

    @Override
    public void orderAndPlaySimultaneousSa(List<SpellAbility> activePlayerSAs) {
        for (SpellAbility sa : orderSimultaneousSa(activePlayerSAs)) {
            if (sa.isTrigger() && !sa.isCopied()) {
                PlaySpellAbility.playSpellAbility(this, player, sa);
            } else {
                // A copy that may choose new targets (MayChooseTarget$) asks
                // as PlayerControllerHuman.orderAndPlaySimultaneousSa does.
                if (sa.isCopied() && sa.isMayChooseNewTargets()) {
                    sa.setupNewTargets(player);
                }
                super.orderAndPlaySimultaneousSa(List.of(sa));
            }
        }
    }

    // Upstream's version calls PlayerController.getAbilityToPlay(Card, List),
    // which calls it back (PlayerControllerForTests.java:494-497,
    // PlayerController.java:99): every PlaySpellAbility.playSpellAbility
    // overflows the stack choosing an additional cost (PlaySpellAbility.java:146).
    // The first is the ability itself, the one Go's castspell plays.
    @Override
    public SpellAbility getAbilityToPlay(Card hostCard, List<SpellAbility> abilities, ITriggerEvent triggerEvent) {
        return abilities.isEmpty() ? null : abilities.get(0);
    }

    // An "as it enters, choose a card name" replacement GameState's setup
    // runs names nothing, as Go's fixture.Load runs none: the setup's
    // NamedCard: is added after (GameState.java:1073-1076). In play a name
    // is a queued answer, not mapped yet.
    @Override
    public String chooseCardName(SpellAbility sa, Predicate<ICardFace> cpp, String valid, String message) {
        if (!answers.bound()) {
            return "";
        }
        // Go's ChooseOption by name (queue optionname), when cpp allows it.
        String name = answers.nextOptionName(n -> {
            forge.item.PaperCard pc = forge.StaticData.instance().getCommonCards().getCard(n);
            return pc != null && (cpp == null || cpp.test(pc.getRules().getMainPart()));
        });
        if (name == null) {
            throw new IllegalStateException("naming a card for " + sa + " needs queue optionname");
        }
        return name;
    }

    // One of several abilities (VentureEffect.chooseNextRoom, VentureEffect.
    // java:107): Go's ChooseAbilitiesForEffect for one (queue abilitychoice,
    // ventureeffect.go chooseNextRoom).
    @Override
    public SpellAbility chooseSingleSpellForEffect(List<SpellAbility> spells, SpellAbility sa, String title,
            Map<String, Object> params) {
        List<Integer> idx = answers.bound() && spells.size() > 1 ? answers.nextAbilityChoice() : null;
        if (idx == null) {
            return super.chooseSingleSpellForEffect(spells, sa, title, params);
        }
        if (idx.size() != 1 || idx.get(0) < 0 || idx.get(0) >= spells.size()) {
            throw new IllegalStateException("abilitychoice " + idx + " for one of " + spells);
        }
        return spells.get(idx.get(0));
    }

    // A vote among Choices$ abilities (VoteEffect.java:104): Go's castVote
    // asks ChooseAbilitiesForEffect (queue abilitychoice), one index a vote.
    @Override
    public Object vote(SpellAbility sa, String prompt, List<Object> options,
            com.google.common.collect.ListMultimap<Object, Player> votes, Player forPlayer, boolean optional) {
        if (answers.bound() && !options.isEmpty() && options.get(0) instanceof SpellAbility) {
            List<Integer> idx = answers.nextAbilityChoice();
            if (idx != null) {
                if (idx.size() != 1 || idx.get(0) < 0 || idx.get(0) >= options.size()) {
                    throw new IllegalStateException("vote answer " + idx + " for " + options);
                }
                return options.get(idx.get(0));
            }
        }
        return super.vote(sa, prompt, options, votes, forPlayer, optional);
    }

    // "You may have it assign its combat damage as though it weren't blocked"
    // and Cunning Giant's "to a creature defending player controls"
    // (Combat.java:845-866, :901-904): Go's ConfirmEffect (combatdamage.go).
    @Override
    public boolean confirmStaticApplication(Card hostCard, PlayerActionConfirmMode mode, String message, String logic) {
        if (answers.bound() && mode == PlayerActionConfirmMode.AlternativeDamageAssignment) {
            Boolean v = answers.nextConfirmEffect();
            if (v != null) {
                return v;
            }
        }
        return super.confirmStaticApplication(hostCard, mode, message, logic);
    }

    // Replicate's, Multikicker's, Squad's and Offspring's count
    // (GameActionUtil.java:673-733): Go's ChooseNumber (queue numberchoice).
    @Override
    public int chooseNumberForKeywordCost(SpellAbility sa, forge.game.cost.Cost cost,
            forge.game.keyword.KeywordInterface keyword, String prompt, int max) {
        Integer v = answers.bound() ? answers.nextNumberChoice() : null;
        return v != null ? Math.min(v, max) : super.chooseNumberForKeywordCost(sa, cost, keyword, prompt, max);
    }

    // A dungeon to venture into (VentureEffect.getDungeonCard,
    // VentureEffect.java:63): Go's ChooseOption by name (queue optionname).
    @Override
    public ICardFace chooseSingleCardFace(SpellAbility sa, List<ICardFace> faces, String message) {
        if (!answers.bound()) {
            return super.chooseSingleCardFace(sa, faces, message);
        }
        String name = answers.nextOptionName(n -> faces.stream().anyMatch(f -> f.getName().equals(n)));
        if (name == null) {
            throw new IllegalStateException("choosing one of " + faces + " for " + sa + " needs queue optionname");
        }
        return faces.stream().filter(f -> f.getName().equals(name)).findFirst().orElseThrow();
    }

    // Assist (CR 702.132a, CostAdjustment.adjustCostByAssist): the caster's
    // ChooseAssistant, then the assistant's AssistAmount, paid from the
    // assistant's pool as any cost is (applyManaToCost, S25).
    @Override
    public Player choosePlayerToAssistPayment(FCollectionView<Player> optionList, SpellAbility sa, String title,
            int max) {
        Player assistant = answers.bound() ? answers.nextAssistant(optionList) : null;
        return assistant != null ? assistant : super.choosePlayerToAssistPayment(optionList, sa, title, max);
    }

    @Override
    public boolean helpPayForAssistSpell(ManaCostBeingPaid cost, SpellAbility sa, int max, int requested) {
        Integer n = answers.bound() ? answers.nextAssistAmount() : null;
        if (n == null || n == 0) {
            return super.helpPayForAssistSpell(cost, sa, max, requested);
        }
        if (n > max) {
            throw new IllegalStateException("assistamount " + n + " over the " + max + " generic left");
        }
        applyManaToCost(new ManaCostBeingPaid(ManaCost.get(n)), sa, "assist", null, false);
        cost.decreaseGenericMana(n);
        return true;
    }

    // A Room door an effect locks or unlocks (UnlockDoorEffect's
    // LockOrUnlock): Go's ChooseRoomDoor.
    @Override
    public forge.game.card.CardState chooseSingleCardState(SpellAbility sa, List<forge.game.card.CardState> states,
            String message, Map<String, Object> params) {
        String door = answers.bound() ? answers.nextRoomDoor() : null;
        if (door == null) {
            return super.chooseSingleCardState(sa, states, message, params);
        }
        for (forge.game.card.CardState st : states) {
            if (st.getStateName() == CardStateName.smartValueOf(door)) {
                return st;
            }
        }
        throw new IllegalStateException("roomdoor " + door + " is not one of " + states);
    }

    // A type an effect chooses (ChooseType): Go's ChooseOption by name.
    @Override
    public String chooseSomeType(String kindOfType, SpellAbility sa, java.util.Collection<String> validTypes,
            boolean isOptional) {
        String name = answers.bound() ? answers.nextOptionName(validTypes::contains) : null;
        return name != null ? name : super.chooseSomeType(kindOfType, sa, validTypes, isOptional);
    }

    // The sector a creature is assigned as it enters (Space Beleren's
    // planet): Go's ChooseSector, asked from the setup's own pass too.
    @Override
    public String chooseSector(Card assignee, String ai, List<String> sectors) {
        String sector = answers.nextSector(sectors);
        return sector != null ? sector : super.chooseSector(assignee, ai, sectors);
    }

    // A card choice for an effect pops queue cardchoice when the answer fits
    // the question; Go asks ChooseCardsForEffect where Java asks any of
    // these four. Otherwise the upstream default answers.
    // Who goes first (GameAction.determineFirstTurnPlayer): Go's
    // ChooseStartingPlayer.
    @Override
    public Player chooseStartingPlayer(boolean isFirstGame) {
        Player first = answers.nextStartingPlayer(player.getGame().getPlayers());
        return first != null ? first : super.chooseStartingPlayer(isFirstGame);
    }

    // MulliganService's keep question: Go's MulliganKeepHand.
    @Override
    public boolean mulliganKeepHand(Player firstPlayer, int cardsToReturn) {
        Boolean keep = answers.nextKeepHand();
        return keep != null ? keep : super.mulliganKeepHand(firstPlayer, cardsToReturn);
    }

    // The London mulligan's cards put on the bottom (LondonMulligan.
    // mulliganDraw): Go's ChooseCardsToTuck.
    @Override
    public CardCollectionView tuckCardsViaMulligan(CardCollectionView hand, int cardsToReturn) {
        List<Card> chosen = answers.nextTuck(hand, cardsToReturn);
        return chosen != null ? new CardCollection(chosen) : super.tuckCardsViaMulligan(hand, cardsToReturn);
    }

    // The cleanup step's discard to hand size (PhaseHandler.java:387): Go's
    // ChooseDiscardToHandSize.
    @Override
    public CardCollectionView chooseCardsToDiscardToMaximumHandSize(int numDiscard) {
        List<Card> chosen = answers.nextDiscard(player, numDiscard);
        return chosen != null ? new CardCollection(chosen) : super.chooseCardsToDiscardToMaximumHandSize(numDiscard);
    }

    // The attackers to exert (CR 508.1g, PhaseHandler.java:575): Go's
    // ExertAttackers. Upstream's default exerts every one offered.
    @Override
    public List<Card> exertAttackers(List<Card> attackers) {
        List<Card> chosen = answers.nextExert(attackers);
        return chosen != null ? chosen : super.exertAttackers(attackers);
    }

    // Go's enlistAttacker asks ChooseCardsForEffect, 0 or 1 of the creatures
    // that can be tapped, once per attacker with Enlist: a queued card choice
    // that fits enlists that attacker, none leaves it out. The picks are paid
    // by visit(CostEnlist).
    @Override
    public List<Card> enlistAttackers(List<Card> attackers) {
        if (!answers.bound()) {
            return super.enlistAttackers(attackers);
        }
        List<Card> out = new ArrayList<>();
        answers.enlistPicks.clear();
        for (Card attacker : attackers) {
            CardCollection options = forge.game.cost.CostEnlist.getCardsForEnlisting(player);
            options.removeAll(answers.enlistPicks);
            List<Card> pick = answers.nextCardChoice(options, 1, 1);
            if (pick != null) {
                out.add(attacker);
                answers.enlistPicks.add(pick.get(0));
            }
        }
        return out;
    }

    // A coin flip's call (FlipCoinEffect.flipCoin): Go's CallCoinFlip, true
    // for heads, as Java compares the call with nextBoolean's result.
    @Override
    public boolean chooseBinary(SpellAbility sa, String question, BinaryChoiceType kindOfChoice, Boolean defaultVal) {
        if (kindOfChoice == BinaryChoiceType.HeadsOrTails) {
            Boolean call = answers.nextCoinCall();
            if (call != null) {
                return call;
            }
        }
        return super.chooseBinary(sa, question, kindOfChoice, defaultVal);
    }

    @Override
    public CardCollectionView chooseCardsForEffect(CardCollectionView sourceList, SpellAbility sa, String title,
            int min, int max, boolean isOptional, Map<String, Object> params) {
        List<Card> chosen = answers.bound() ? answers.nextCardChoice(sourceList, isOptional ? 0 : min, max) : null;
        return chosen != null ? new CardCollection(chosen)
                : super.chooseCardsForEffect(sourceList, sa, title, min, max, isOptional, params);
    }

    @Override
    @SuppressWarnings("unchecked")
    public <T extends GameEntity> T chooseSingleEntityForEffect(FCollectionView<T> optionList,
            DelayedReveal delayedReveal, SpellAbility sa, String title, boolean isOptional, Player targetedPlayer,
            Map<String, Object> params) {
        // A battle's protector (CR 704.5w, GameAction.java:1711): Go's
        // ChooseBattleProtector, asked from the setup's own pass too.
        if (sa != null && sa.getApi() == ApiType.ChoosePlayer && sa.getHostCard() != null
                && sa.getHostCard().isBattle()) {
            Player protector = answers.nextBattleProtector(optionList);
            if (protector != null) {
                return (T) protector;
            }
        }
        // The legend rule's keep (GameAction.handleLegendRule): Go's
        // ChooseLegendaryToKeep, asked from the setup's own pass too.
        if (sa != null && sa.getApi() == ApiType.InternalLegendaryRule) {
            Card keep = answers.nextLegendaryKeep(optionList);
            if (keep != null) {
                return (T) keep;
            }
        }
        // Go's ChoosePlayerForEffect: a list of players (Captive Audience's
        // opponent).
        if (answers.bound()) {
            Player chosenPlayer = answers.nextPlayerChoice(optionList);
            if (chosenPlayer != null) {
                return (T) chosenPlayer;
            }
        }
        // An Aura entering without being cast picks what it enchants
        // (GameAction.attachAuraOnIndirectETB): Go's ChooseEnchantTarget.
        if (answers.bound() && sa != null && sa.getHostCard() != null
                && sa == sa.getHostCard().getCurrentState().getAuraSpell()) {
            Card aura = answers.nextEnchantTarget(optionList);
            if (aura != null) {
                return (T) aura;
            }
        }
        List<Card> chosen = answers.bound() ? answers.nextCardChoice(optionList, isOptional ? 0 : 1, 1) : null;
        if (chosen == null) {
            return super.chooseSingleEntityForEffect(optionList, delayedReveal, sa, title, isOptional, targetedPlayer,
                    params);
        }
        return chosen.isEmpty() ? null : (T) chosen.get(0);
    }

    @Override
    @SuppressWarnings("unchecked")
    public <T extends GameEntity> List<T> chooseEntitiesForEffect(FCollectionView<T> optionList, int min, int max,
            DelayedReveal delayedReveal, SpellAbility sa, String title, Player relatedPlayer,
            Map<String, Object> params) {
        // Go's ChooseEntitiesForEffect (queue entitychoice), else a card choice.
        List<GameEntity> entities = answers.bound() ? answers.nextEntityChoice(optionList, min, max) : null;
        if (entities != null) {
            return (List<T>) entities;
        }
        List<Card> chosen = answers.bound() ? answers.nextCardChoice(optionList, min, max) : null;
        return chosen != null ? (List<T>) chosen
                : super.chooseEntitiesForEffect(optionList, min, max, delayedReveal, sa, title, relatedPlayer, params);
    }

    @Override
    public Card chooseSingleCardForZoneChange(ZoneType destination, List<ZoneType> origin, SpellAbility sa,
            CardCollection fetchList, DelayedReveal delayedReveal, String selectPrompt, boolean isOptional,
            Player decider) {
        List<Card> chosen = answers.bound() ? answers.nextCardChoice(fetchList, isOptional ? 0 : 1, 1) : null;
        if (chosen == null) {
            return super.chooseSingleCardForZoneChange(destination, origin, sa, fetchList, delayedReveal,
                    selectPrompt, isOptional, decider);
        }
        return chosen.isEmpty() ? null : chosen.get(0);
    }

    // A multi-card hidden search (ChangeZoneEffect.java:1160, Surgical
    // Extraction's "any number"): upstream's test controller answers null,
    // picking nothing; Go asks ChooseCardsForEffect once for the list.
    @Override
    public List<Card> chooseCardsForZoneChange(ZoneType destination, List<ZoneType> origin, SpellAbility sa,
            CardCollection fetchList, int min, int max, DelayedReveal delayedReveal, String selectPrompt,
            Player decider) {
        List<Card> chosen = answers.bound() ? answers.nextCardChoice(fetchList, min, max) : null;
        return chosen != null ? chosen
                : super.chooseCardsForZoneChange(destination, origin, sa, fetchList, min, max, delayedReveal,
                        selectPrompt, decider);
    }

    // Go's discard and sacrifice picks are ChooseCardsToDiscard and
    // ChoosePermanentsToSacrifice (queue discardchoice, sacrificechoice);
    // its reveal, delve, cost, devour and exploit picks are
    // ChooseCardsForEffect (costassist.go, chosencosts.go), so a hook falls
    // back to the card choice queue.
    @Override
    public CardCollectionView choosePermanentsToSacrifice(SpellAbility sa, int min, int max,
            CardCollectionView validTargets, String message) {
        List<Card> chosen = answers.nextSacrificeChoice(validTargets, min, max);
        return chosen != null ? new CardCollection(chosen)
                : super.choosePermanentsToSacrifice(sa, min, max, validTargets, message);
    }

    @Override
    public CardCollection chooseCardsToDiscardFrom(Player playerDiscard, SpellAbility sa, CardCollection validCards,
            int min, int max, CardCollectionView visibleToChooser) {
        List<Card> chosen = answers.nextDiscardChoice(validCards, min, max);
        return chosen != null ? new CardCollection(chosen)
                : super.chooseCardsToDiscardFrom(playerDiscard, sa, validCards, min, max, visibleToChooser);
    }

    @Override
    public CardCollectionView chooseCardsToRevealFromHand(int min, int max, CardCollectionView valid) {
        List<Card> chosen = answers.nextCardChoice(valid, min, max);
        return chosen != null ? new CardCollection(chosen) : super.chooseCardsToRevealFromHand(min, max, valid);
    }

    @Override
    public CardCollectionView chooseCardsToDiscardUnlessType(int min, CardCollectionView hand, String[] unlessTypes,
            SpellAbility sa) {
        List<Card> chosen = answers.nextCardChoice(hand, 1, min);
        return chosen != null ? new CardCollection(chosen)
                : super.chooseCardsToDiscardUnlessType(min, hand, unlessTypes, sa);
    }

    @Override
    public CardCollectionView chooseCardsToDelve(int genericAmount, CardCollection grave) {
        List<Card> chosen = answers.nextCardChoice(grave, 0, Math.min(genericAmount, grave.size()));
        return chosen != null ? new CardCollection(chosen) : super.chooseCardsToDelve(genericAmount, grave);
    }

    @Override
    public CardCollectionView chooseCardsForCost(CardCollectionView optionList, SpellAbility sa,
            CostPartWithList cpl, int amount, boolean isOptional, String prompt) {
        List<Card> chosen = answers.nextCardChoice(optionList, isOptional ? 0 : amount, amount);
        return chosen != null ? new CardCollection(chosen)
                : super.chooseCardsForCost(optionList, sa, cpl, amount, isOptional, prompt);
    }

    // Go's ConfirmEffect: an effect's own yes/no prompt.
    @Override
    public boolean confirmAction(SpellAbility sa, PlayerActionConfirmMode mode, String message, List<String> options,
            Card cardToShow, Map<String, Object> params) {
        Boolean v = answers.bound() ? answers.nextConfirmEffect() : null;
        return v != null ? v : super.confirmAction(sa, mode, message, options, cardToShow, params);
    }

    // An optional replacement: an Optional$ line's "you may" (Fasting) is
    // Go's ConfirmReplacementEffect, an ETBReplacement's Optional (Clone's
    // "you may have it enter as a copy") Go's ConfirmEffect; Java asks both
    // here, the first from the confirmreplacement queue.
    @Override
    public boolean confirmReplacementEffect(ReplacementEffect replacementEffect, SpellAbility effectSA,
            GameEntity affected, String question) {
        Boolean v = answers.bound() ? answers.nextConfirmReplacement() : null;
        if (v == null && answers.bound()) {
            v = answers.nextConfirmEffect();
        }
        return v != null ? v : super.confirmReplacementEffect(replacementEffect, effectSA, affected, question);
    }

    // CR 616.1: which of several replacement effects applies first, Go's
    // ChooseReplacementEffect index into its candidates. Java's damage path
    // offers a HashMap's key order, which changes from run to run
    // (ReplacementHandler.java:702), so the index reads the candidates in
    // Go's order (eachReplacementRule, entryCandidates): an entering card's
    // own lines first, then by seat, zone (battlefield, command, graveyard)
    // and card, then the line's place on its card.
    @Override
    public ReplacementEffect chooseSingleReplacementEffect(List<ReplacementEffect> possibleReplacers) {
        Integer i = answers.bound() ? answers.nextReplacement() : null;
        if (i == null) {
            return super.chooseSingleReplacementEffect(possibleReplacers);
        }
        if (i < 0 || i >= possibleReplacers.size()) {
            throw new IllegalStateException("replacement " + i + " of " + possibleReplacers.size() + " candidates");
        }
        List<ReplacementEffect> ordered = new ArrayList<>(possibleReplacers);
        ordered.sort(Comparator.comparingInt(ScenarioController::replacementRank)
                .thenComparingInt(re -> re.getHostCard().getId())
                .thenComparingInt(re -> re.getHostCard().getReplacementEffects().indexOf(re)));
        return ordered.get(i);
    }

    // Go's walk order of a replacement's host: a card outside the zones
    // eachReplacementRule walks is the one entering, whose own lines come
    // first; then seat, then battlefield, command, graveyard.
    private static int replacementRank(ReplacementEffect re) {
        Card host = re.getHostCard();
        int zone = host.isInZone(ZoneType.Battlefield) ? 0 : host.isInZone(ZoneType.Command) ? 1
                : host.isInZone(ZoneType.Graveyard) ? 2 : -1;
        if (zone < 0) {
            return -1;
        }
        return host.getGame().getRegisteredPlayers().indexOf(host.getController()) * 3 + zone;
    }

    // Go's ConfirmPayCost for an unless cost (ward, echo, a shock land's
    // replacement): the caller trusts a true answer as paid
    // (AbilityUtils.java:1435), so it pays here, from the pool, the way
    // PlayerControllerAi.payCostToPreventEffect does.
    @Override
    public boolean payCostToPreventEffect(Cost cost, SpellAbility sa, boolean alreadyPaid,
            FCollectionView<Player> allPayers) {
        Boolean v = answers.bound() ? answers.nextConfirmPayCost() : null;
        if (v == null) {
            return super.payCostToPreventEffect(cost, sa, alreadyPaid, allPayers);
        }
        return v && pay(cost, sa);
    }

    // Go's ConfirmPayCost for an attack cost (Propaganda, Ghostly Prison).
    @Override
    public boolean payCombatCost(Card c, Cost cost, SpellAbility sa, String prompt) {
        // An exert the player already chose (exertAttackers) is the whole
        // cost (getAttackCost starts from a zero mana part): Go's ExertAttackers answer exerts it, with no confirmation.
        // An enlist enlistAttackers already picked is paid the same way.
        if (cost.getCostParts().stream().allMatch(part -> part instanceof CostExert
                || part instanceof forge.game.cost.CostEnlist
                || part instanceof CostPartMana m && m.getMana().isZero())) {
            return pay(cost, sa);
        }
        Boolean v = answers.nextConfirmPayCost();
        if (v == null) {
            throw new IllegalStateException("an attack cost for " + c + " needs queue confirmpaycost");
        }
        return v && pay(cost, sa);
    }

    // A cost part's cards come from queue cardchoice (ScenarioCostDecision).
    @Override
    public CostDecisionMakerBase getCostDecisionMaker(Player player, SpellAbility ability, boolean effect,
            String prompt) {
        return answers.bound() ? new ScenarioCostDecision(player, ability, effect, answers)
                : super.getCostDecisionMaker(player, ability, effect, prompt);
    }

    // Go's ChooseModesForAbility (charmeffect.go): indices into the modes
    // CharmEffect.makePossibleOptions offers, the list Go builds the same way.
    @Override
    public List<AbilitySub> chooseModeForAbility(SpellAbility sa, List<AbilitySub> possible, int min, int num,
            boolean allowRepeat) {
        List<Integer> idx = answers.bound() ? answers.nextModeChoice() : null;
        if (idx == null) {
            return super.chooseModeForAbility(sa, possible, min, num, allowRepeat);
        }
        List<AbilitySub> out = new ArrayList<>();
        for (int i : idx) {
            out.add(possible.get(i));
        }
        return out;
    }

    // Go's ArrangeForScry and ArrangeForSurveil: the queued top cards, the
    // first topmost (GameAction.scry and Player.surveil reverse the list
    // themselves), and the queued bottom or graveyard cards.
    @Override
    public ImmutablePair<CardCollection, CardCollection> arrangeForScry(CardCollection topN) {
        List<CardCollection> v = answers.bound() ? answers.nextScry() : null;
        return v != null ? ImmutablePair.of(v.get(0), v.get(1)) : super.arrangeForScry(topN);
    }

    @Override
    public ImmutablePair<CardCollection, CardCollection> arrangeForSurveil(CardCollection topN) {
        List<CardCollection> v = answers.bound() ? answers.nextSurveil() : null;
        return v != null ? ImmutablePair.of(v.get(0), v.get(1)) : super.arrangeForSurveil(topN);
    }

    // Go's ChoosePayX (castspell.go, manapay.go): the X PlaySpellAbility
    // announces (PlaySpellAbility.java:751, :773). Go asks ChooseNumber for
    // any other announced value (ChooseNumberEffect.java:72) and for the X
    // of a cost a replacement pays (replacementcost.go: Minion of the
    // Wastes), so a numberchoice answers when no payx is queued.
    @Override
    public Integer announceRequirements(SpellAbility ability, int min, int max, String announce) {
        Integer v = null;
        if (answers.bound()) {
            v = "X".equals(announce) ? answers.nextPayX() : null;
            if (v == null) {
                v = answers.nextNumberChoice();
            }
        }
        return v != null ? v : super.announceRequirements(ability, min, max, announce);
    }

    // Go's ChooseColors: one queued color, for a one-color or a set choice.
    @Override
    public byte chooseColor(String message, SpellAbility sa, ColorSet colors) {
        // One mana of a Mana effect (ManaEffect.java:118, :161): Go's
        // ChooseManaColor (S29).
        if (answers.bound() && sa != null && sa.getApi() == ApiType.Mana && answers.hasManaColor()) {
            return answers.nextManaColor();
        }
        Byte v = answers.bound() ? answers.nextColorChoice() : null;
        return v != null ? v : super.chooseColor(message, sa, colors);
    }

    // S29: a Mana effect's Amount$ above 1 in any combination of colors
    // (ManaEffect.java:70-91), asked at once; Go asks ChooseManaColor once per
    // mana, so the answers are counted by color.
    @Override
    public Map<Byte, Integer> specifyManaCombo(SpellAbility sa, ColorSet colorSet, int manaAmount, boolean different) {
        Map<Byte, Integer> out = new java.util.LinkedHashMap<>();
        for (int i = 0; i < manaAmount; i++) {
            Byte color = answers.bound() ? answers.nextManaColor() : null;
            if (color == null) {
                return super.specifyManaCombo(sa, colorSet, manaAmount, different);
            }
            out.merge(color, 1, Integer::sum);
        }
        return out;
    }

    @Override
    public ColorSet chooseColors(String message, SpellAbility sa, int min, int max, ColorSet options) {
        Byte v = answers.bound() ? answers.nextColorChoice() : null;
        return v != null ? ColorSet.fromMask(v) : super.chooseColors(message, sa, min, max, options);
    }

    // Go's ChooseNumber.
    @Override
    public int chooseNumber(SpellAbility sa, String title, int min, int max) {
        Integer v = answers.bound() ? answers.nextNumberChoice() : null;
        return v != null ? v : super.chooseNumber(sa, title, min, max);
    }

    @Override
    public int chooseNumber(SpellAbility sa, String title, List<Integer> values, Player relatedPlayer) {
        Integer v = answers.bound() ? answers.nextNumberChoice() : null;
        return v != null ? v : super.chooseNumber(sa, title, values, relatedPlayer);
    }

    private boolean pay(Cost cost, SpellAbility sa) {
        if (!ComputerUtilCost.canPayCost(cost, sa, player, true)) {
            return false;
        }
        return new CostPayment(cost, sa).payComputerCosts(getCostDecisionMaker(player, sa, true, null));
    }

    // Go asks ConfirmPayCost once per optional cost a spell offers (kicker,
    // bargain, behold, blight, a flash cost), in the offered order.
    @Override
    public List<OptionalCostValue> chooseOptionalCosts(SpellAbility chosen, List<OptionalCostValue> optionalCostValues) {
        List<OptionalCostValue> out = new ArrayList<>();
        for (OptionalCostValue o : optionalCostValues) {
            Boolean v = answers.nextConfirmPayCost();
            if (v == null) {
                throw new IllegalStateException("optional cost " + o + " needs queue confirmpaycost");
            }
            if (v) {
                out.add(o);
            }
        }
        return out;
    }

    // Go's ConfirmOptionalTrigger: an OptionalDecider$ trigger's "you may".
    @Override
    public boolean confirmTrigger(WrappedAbility wrapper) {
        Boolean v = answers.bound() ? answers.nextOptionalTrigger() : null;
        return v != null ? v : super.confirmTrigger(wrapper);
    }

    // Cards moving into one zone together go in the queued order (CR
    // 613.7m); Go asks at the same points (zonemove.go
    // orderCardsByTheirOwners and the effects that order). The state-based
    // check GameState.applyToGame runs during setup keeps the given order.
    @Override
    public CardCollectionView orderMoveToZoneList(CardCollectionView cards, ZoneType destinationZone,
            SpellAbility source) {
        if (!answers.bound()) {
            return cards;
        }
        return answers.nextCardOrder(cards);
    }

    // A trigger that never uses the stack takes its targets the same way.
    @Override
    public boolean playTrigger(Card host, WrappedAbility wrapperAbility, boolean isMandatory) {
        return PlaySpellAbility.playSpellAbilityNoStack(this, player, wrapperAbility, false);
    }

    // Upstream's version plays only the abilities of a list of named cards
    // and throws for any other (a planeswalker's or a Saga's state trigger,
    // F6). Every one is played, as the game's own controllers do.
    @Override
    public void playSpellAbilityNoStack(SpellAbility effectSA, boolean mayChoseNewTargets) {
        PlaySpellAbility.playSpellAbilityNoStack(this, player, effectSA, !mayChoseNewTargets);
    }
}
