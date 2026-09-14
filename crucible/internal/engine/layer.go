// Continuous-effect layers: CR 613's order for computing a permanent's
// current characteristics from its printed ones.

package engine

// StaticAbilityLayer is one of CR 613's continuous-effect layers, in the
// order they apply. Named and ordered after
// forge-game/src/main/java/forge/game/staticability/StaticAbilityLayer.java,
// including its 7a/7b/7c split of Layer 7 and the Layer 8 Forge adds for
// its own rule-changing bookkeeping, which carries no CR number.
type StaticAbilityLayer uint8

// The layers, in application order.
const (
	LayerCopy StaticAbilityLayer = iota
	LayerControl
	LayerText
	LayerType
	LayerColor
	LayerAbilities
	// LayerCharacteristic is 7a: a characteristic-defining ability's own
	// power/toughness, applied before anything else in Layer 7 (CR 613.4).
	LayerCharacteristic
	// LayerSetPT is 7b: an effect that sets power and/or toughness outright.
	LayerSetPT
	// LayerModifyPT is 7c: an effect that adds to whatever power and/or
	// toughness Layers 7a and 7b already produced.
	LayerModifyPT
	LayerRules

	numStaticAbilityLayers = int(LayerRules) + 1
)

var staticAbilityLayerNames = [numStaticAbilityLayers]string{
	LayerCopy: "1", LayerControl: "2", LayerText: "3", LayerType: "4",
	LayerColor: "5", LayerAbilities: "6", LayerCharacteristic: "7a",
	LayerSetPT: "7b", LayerModifyPT: "7c", LayerRules: "8",
}

// String returns the layer's number, the way Forge's own
// StaticAbilityLayer.num field does.
func (l StaticAbilityLayer) String() string {
	if int(l) >= numStaticAbilityLayers {
		return "?"
	}
	return staticAbilityLayerNames[l]
}
