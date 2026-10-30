package engine_test

// Cost parts no code reads yet, for tests that need a cost the engine must
// refuse. Porting a cost part breaks only these two lines, not every test that
// used it as its example: when one becomes supported, pick another part.
const (
	unsupportedCost1 = "RevealFromExile<1>"
	unsupportedCost4 = "RevealFromExile<4>"
)
