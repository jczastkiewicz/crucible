package engine

type APIType int

const APIDraw APIType = 0

// orphanEffect registers but cannot resolve anything.
//
//crucible:register Draw
type orphanEffect struct{}
