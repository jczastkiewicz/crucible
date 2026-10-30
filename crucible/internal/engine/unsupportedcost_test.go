package engine_test

// Cost parts no code reads yet, for tests that need a cost the engine must
// refuse. Porting a cost part breaks only these two lines, not every test that
// used it as its example: when one becomes supported, pick another part.
const (
	unsupportedCost1 = "Unattach<OriginalHost>"
	unsupportedCost4 = "Unattach<OriginalHost/Equipment>"
)
