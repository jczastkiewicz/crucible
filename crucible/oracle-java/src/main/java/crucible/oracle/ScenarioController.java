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
import forge.card.CardStateName;
import forge.card.mana.ManaCost;
import forge.card.mana.ManaCostParser;
import forge.game.cost.Cost;
import forge.game.cost.CostDecisionMakerBase;
import forge.game.cost.CostPartMana;
import forge.game.cost.CostPayment;
import forge.ai.ComputerUtilCost;
import forge.game.spellability.OptionalCostValue;
import forge.game.cost.CostPartWithList;
import forge.game.mana.ManaConversionMatrix;
import forge.game.mana.ManaCostBeingPaid;
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
                if (!PlaySpellAbility.playSpellAbilityNoStack(this, player, sa, true)) {
                    throw new IllegalStateException(card + " did not activate " + sa);
                }
                continue;
            }
            boolean land = play.verb().equals("playland");
            if (play.refused()) {
                refuse(card, land);
                refused = true;
                continue;
            }
            for (SpellAbility sa : card.getAllPossibleAbilities(player, true)) {
                if (land ? sa.isLandAbility() : sa.isSpell() && castsFace(card, sa, play.arg())) {
                    return List.of(sa);
                }
            }
            throw new IllegalStateException(player + " cannot " + play.verb() + " " + card + " " + play.arg());
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
        return true;
    }

    // S17: a play Go refused. Java refuses it too when the card offers no
    // playable spell or land ability, or when playing it fails; a play that
    // goes through is an error, since Go's state assumes it never happened.
    private void refuse(Card card, boolean land) {
        for (SpellAbility sa : card.getAllPossibleAbilities(player, true)) {
            if (land ? sa.isLandAbility() : sa.isSpell()) {
                if (PlaySpellAbility.playSpellAbility(this, player, sa)) {
                    throw new IllegalStateException(player + " played " + sa + ", which Go refused");
                }
                return;
            }
        }
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
        if (currentAbility.isSpell() && currentAbility.getHostCard().isAura()) {
            List<GameEntity> candidates = tgt.getAllCandidates(currentAbility);
            if (candidates.size() == 1) {
                currentAbility.getTargets().add(candidates.get(0));
                return true;
            }
        }
        for (GameEntity e : answers.nextTargets()) {
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
            if (chosen.size() != 1 && chosen.size() != total) {
                throw new IllegalStateException("dividing " + total + " among " + chosen + " needs queue divide");
            }
            for (GameObject o : chosen) {
                currentAbility.addDividedAllocation(o, chosen.size() == 1 ? total : 1);
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
        while (!player.getManaPool().payManaCostFromPool(toPay, ability, false, ability.getPayingMana())) {
            ScenarioAnswers.Play tap = answers.nextTapNow();
            if (tap == null) {
                throw new IllegalStateException(player + "'s pool cannot pay " + toPay + " for " + ability);
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
    @Override
    public void declareAttackers(Player attacker, Combat combat) {
        List<Integer> ids = answers.nextAttackers();
        if (ids == null) {
            return;
        }
        for (int id : ids) {
            List<GameEntity> defenders = List.copyOf(combat.getDefenders());
            GameEntity defender = defenders.size() == 1 ? defenders.get(0) : answers.nextAttackTarget();
            combat.addAttacker(answers.card(id), defender);
        }
        if (!CombatUtil.validateAttackers(combat)) {
            throw new IllegalStateException("illegal attack declaration " + ids);
        }
    }

    // Go's DeclareCombatBlockers: the queued blocker=attacker pairs.
    @Override
    public void declareBlockers(Player defender, Combat combat) {
        List<int[]> pairs = answers.nextBlocks();
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
            throw new IllegalStateException("dividing " + attacker + "'s damage among " + blockers + " needs queue damage");
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
        throw new IllegalStateException("naming a card for " + sa + " needs queue cardname");
    }

    // A card choice for an effect pops queue cardchoice when the answer fits
    // the question; Go asks ChooseCardsForEffect where Java asks any of
    // these four. Otherwise the upstream default answers.
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
        Byte v = answers.bound() ? answers.nextColorChoice() : null;
        return v != null ? v : super.chooseColor(message, sa, colors);
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
