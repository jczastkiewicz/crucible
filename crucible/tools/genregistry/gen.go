package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// registerDirective names an API a type serves when its name alone does not:
// several APIs, another API, or a value with fields.
//
//	//crucible:register Manifest manifestEffect{api: "Manifest"}
const registerDirective = "//crucible:register"

// m5Casting are the registry entries M5 wired for casting permanents and
// Auras, before M6's first script-driven effect. The resolved-API count the
// docs quote has never included them.
var m5Casting = map[string]bool{"PermanentCreature": true, "PermanentNoncreature": true, "Attach": true}

// entry is one registry line: r[API<API>] = <Expr>.
type entry struct {
	API  string
	Expr string
}

// Registry is the parsed result: every entry, sorted by API name.
type Registry struct {
	Entries []entry
}

// M6Count is the number the docs quote as resolved script-driven APIs.
func (r Registry) M6Count() int {
	n := 0
	for _, e := range r.Entries {
		if !m5Casting[e.API] {
			n++
		}
	}
	return n
}

// Scan reads the package in dir, skipping the generated output file, and
// returns its registry.
func Scan(dir, outName string) (Registry, error) {
	fset := token.NewFileSet()
	files, err := parsePackage(fset, dir, outName)
	if err != nil {
		return Registry{}, err
	}

	apis := map[string]bool{}
	resolvers := map[string]bool{}
	directives := map[string][]entry{}
	directiveAt := map[string]token.Pos{}

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		f := files[name]
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if api, ok := strings.CutPrefix(id.Name, "API"); ok && api != "" {
								apis[api] = true
							}
						}
					case *ast.TypeSpec:
						doc := s.Doc
						if doc == nil && len(d.Specs) == 1 {
							doc = d.Doc
						}
						es, err := parseDirectives(doc, s.Name.Name)
						if err != nil {
							return Registry{}, fmt.Errorf("%s: %w", fset.Position(s.Pos()), err)
						}
						if len(es) > 0 {
							directives[s.Name.Name] = es
							directiveAt[s.Name.Name] = s.Pos()
						}
					}
				}
			case *ast.FuncDecl:
				if t, ok := resolveReceiver(d); ok {
					resolvers[t] = true
				}
			}
		}
	}

	for t, pos := range directiveAt {
		if !resolvers[t] {
			return Registry{}, fmt.Errorf("%s: %s on %s, which has no Resolve(*Game, *Ability, PlayerController) error method",
				fset.Position(pos), registerDirective, t)
		}
	}

	var out []entry
	for t := range resolvers {
		if es, ok := directives[t]; ok {
			out = append(out, es...)
			continue
		}
		base, ok := strings.CutSuffix(t, "Effect")
		if !ok || base == "" {
			continue // a Resolve method on a type that is not an effect (Registry itself)
		}
		out = append(out, entry{API: strings.ToUpper(base[:1]) + base[1:], Expr: t + "{}"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].API < out[j].API })

	for i, e := range out {
		if !apis[e.API] {
			return Registry{}, fmt.Errorf("%s registers API %q, which has no API%s constant", e.Expr, e.API, e.API)
		}
		if i > 0 && out[i-1].API == e.API {
			return Registry{}, fmt.Errorf("API %q registered twice: %s and %s", e.API, out[i-1].Expr, e.Expr)
		}
	}
	return Registry{Entries: out}, nil
}

// parseDirectives reads the register lines in a type's doc comment. A bare
// API name registers the zero value of the type.
func parseDirectives(doc *ast.CommentGroup, typeName string) ([]entry, error) {
	if doc == nil {
		return nil, nil
	}
	var out []entry
	for _, c := range doc.List {
		rest, ok := strings.CutPrefix(c.Text, registerDirective)
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		api, expr, _ := strings.Cut(rest, " ")
		if api == "" {
			return nil, fmt.Errorf("%s with no API name", registerDirective)
		}
		expr = strings.TrimSpace(expr)
		if expr == "" {
			expr = typeName + "{}"
		}
		if !strings.HasPrefix(expr, typeName+"{") {
			return nil, fmt.Errorf("%s %s: value %q is not a %s literal", registerDirective, api, expr, typeName)
		}
		out = append(out, entry{API: api, Expr: expr})
	}
	return out, nil
}

// resolveReceiver reports the value-receiver type of an Effect.Resolve
// implementation: Resolve(*Game, *Ability, PlayerController) error.
func resolveReceiver(d *ast.FuncDecl) (string, bool) {
	if d.Recv == nil || len(d.Recv.List) != 1 || d.Name.Name != "Resolve" {
		return "", false
	}
	id, ok := d.Recv.List[0].Type.(*ast.Ident)
	if !ok {
		return "", false // pointer receiver: a value in the array would not satisfy Effect
	}
	var params []string
	for _, p := range d.Type.Params.List {
		n := len(p.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			params = append(params, types.ExprString(p.Type))
		}
	}
	if strings.Join(params, ",") != "*Game,*Ability,PlayerController" {
		return "", false
	}
	if d.Type.Results == nil || len(d.Type.Results.List) != 1 || types.ExprString(d.Type.Results.List[0].Type) != "error" {
		return "", false
	}
	return id.Name, true
}

// Render returns the formatted registry_gen.go for the registry.
func Render(r Registry) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by tools/genregistry. DO NOT EDIT.

package engine

// NewRegistry returns a Registry with every effect in this package
// registered: each type named *Effect with a value-receiver
// Resolve(*Game, *Ability, PlayerController) error, under the API its name
// gives or the API its //crucible:register lines name (ADR-0017).
//
// %d APIs. Explicit construction, not an init() populating a package-level
// Registry: a caller that wants fewer APIs builds its own Registry.
//
// Regenerate with "go generate -run genregistry ./internal/engine" after adding an effect.
func NewRegistry() *Registry {
	var r Registry
`, len(r.Entries))
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "\tr[API%s] = %s\n", e.API, e.Expr)
	}
	b.WriteString("\treturn &r\n}\n")
	return format.Source(b.Bytes())
}

// docCount matches the resolved-API count sentence the docs share.
var docCount = regexp.MustCompile(`(\d+) of the corpus's \d+ script-driven`)

// CheckDocs verifies each doc's first "N of the corpus's M script-driven"
// count equals want.
func CheckDocs(paths []string, want int) []string {
	var problems []string
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		m := docCount.FindSubmatch(raw)
		if m == nil {
			problems = append(problems, fmt.Sprintf("%s: no %q count sentence", p, "N of the corpus's M script-driven"))
			continue
		}
		if got := string(m[1]); got != fmt.Sprint(want) {
			problems = append(problems, fmt.Sprintf("%s: says %s of the corpus's APIs resolve, registry has %d", p, got, want))
		}
	}
	return problems
}

// parsePackage returns the non-test Go files of dir, honouring build
// constraints the way the compiler does, minus the generated output.
func parsePackage(fset *token.FileSet, dir, skip string) (map[string]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	ctx := build.Default
	out := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == skip {
			continue
		}
		ok, err := ctx.MatchFile(dir, name)
		if err != nil {
			return nil, fmt.Errorf("build constraints for %s: %w", name, err)
		}
		if !ok {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		out[name] = f
	}
	return out, nil
}
