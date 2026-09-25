package engine

type (
	Game             struct{}
	Ability          struct{}
	PlayerController interface{}
	APIType          int
)

const APIDraw APIType = 0

// fooEffect has no APIFoo constant to register under.
type fooEffect struct{}

func (fooEffect) Resolve(g *Game, a *Ability, _ PlayerController) error { return nil }
