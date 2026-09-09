// Fixture. One call site per shape apiscan has to recognise, plus the near
// misses that must not be recognised.
package forge.game.ability.effects;

public class ReadsEffect extends SpellAbilityEffect {
    public void resolve(SpellAbility sa) {
        // 1. the accessors on an ability
        sa.getParam("Accessor");
        sa.hasParam("Presence");
        sa.getParamOrDefault("Defaulted", "1");

        // 2. the raw map, before an ability object exists
        mapParams.containsKey("RawContains");
        params.get("RawGet");

        // 3. a helper taking the key as an argument, one and two at a time
        getDefinedPlayersOrTargeted(sa, "OneHelperKey");
        addToCombat(moved, sa, "FirstOfTwo", "SecondOfTwo");

        // 4. a key bound to a variable first
        final String key = "BoundToVariable";

        // 5. a trigger or replacement matching against the event
        matchesValidParam("ValidMatched", runParams.get(AbilityKey.Affected));

        // Near misses. A non-literal key cannot be recovered, and a string that
        // is not a param must not be collected.
        sa.getParam(variableKey);
        someOtherCall("NotAParam");
        String message = "AlsoNotAParam";
    }
}
