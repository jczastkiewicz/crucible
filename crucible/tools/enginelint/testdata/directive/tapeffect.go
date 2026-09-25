package fake

//enginelint:allow

// Violation: the directive allows nothing, so state is off limits.
func tapEffect(id uint32) uint32 { return newCard(id).ID }
