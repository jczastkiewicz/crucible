package engine

type (
	Game             struct{}
	Ability          struct{}
	PlayerController interface{}
	APIType          int
)

const (
	APIDraw APIType = iota
	APIManifest
	APICloak
	APIPermanentCreature
)

// drawEffect registers by name: APIDraw.
type drawEffect struct{}

func (drawEffect) Resolve(g *Game, a *Ability, _ PlayerController) error { return nil }

// manifestEffect serves two APIs with different field values.
//
//crucible:register Manifest manifestEffect{api: "Manifest"}
//crucible:register Cloak manifestEffect{api: "Cloak", cloak: true}
type manifestEffect struct {
	api   string
	cloak bool
}

func (e manifestEffect) Resolve(g *Game, a *Ability, c PlayerController) error { return nil }

// permanentEffect names its API explicitly with the zero value.
//
//crucible:register PermanentCreature
type permanentEffect struct{}

func (permanentEffect) Resolve(g *Game, a *Ability, c PlayerController) error { return nil }

// helperEffect has no Resolve method, so it is not an effect.
type helperEffect struct{}

// pointerEffect has a pointer receiver: a value in the array would not satisfy
// Effect, so it is skipped.
type pointerEffect struct{}

func (*pointerEffect) Resolve(g *Game, a *Ability, c PlayerController) error { return nil }
