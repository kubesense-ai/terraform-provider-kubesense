package schema

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	cuejson "cuelang.org/go/encoding/json"
)

// cueSource is the contract itself, embedded so defaults can be filled by CUE at
// runtime rather than by a table generated from it.
//
// ./cue holds the whole contract and nothing else, which is why this is one
// directory plus the module marker. The migration chains next door in ./alert
// and ./dashboard are Go, and are not part of the schema.
//
//go:embed cue.mod/module.cue cue/alert/*.cue cue/panel/*.cue cue/dashboard/*.cue
var cueSource embed.FS

// cueRoot is the path the embedded files are presented at. CUE's loader resolves
// packages against a directory containing cue.mod, and an overlay lets that
// directory be one that does not exist on disk — which is the point, since a
// consumer of this module has the .cue files inside their binary and nowhere
// else.
const cueRoot = "/kubesense-schemas"

// cueTarget maps a format to the CUE package and definition that describe it.
var cueTarget = map[Format]struct{ pkg, def string }{
	AlertRule:       {"alert", "#Rule"},
	Panel:           {"panel", "#Panel"},
	PanelQuery:      {"panel", "#Query"},
	DashboardPreset: {"dashboard", "#Preset"},
}

var (
	cueOnce sync.Once
	cueCtx  *cue.Context
	cuePkgs map[string]cue.Value
	cueErr  error
)

// loadCUE builds every embedded CUE package once.
//
// Building parses the whole contract and resolves its imports, so it happens on
// first use and is then shared. The resulting cue.Values are read-only, and the
// context is safe to use from several goroutines.
func loadCUE() {
	cueOnce.Do(func() {
		overlay := map[string]load.Source{}
		err := fs.WalkDir(cueSource, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := cueSource.ReadFile(p)
			if err != nil {
				return err
			}
			overlay[filepath.Join(cueRoot, p)] = load.FromBytes(b)
			return nil
		})
		if err != nil {
			cueErr = fmt.Errorf("schemas: read embedded CUE: %w", err)
			return
		}

		cueCtx = cuecontext.New()
		cuePkgs = map[string]cue.Value{}
		for _, dir := range []string{"alert", "panel", "dashboard"} {
			insts := load.Instances([]string{"./cue/" + dir}, &load.Config{
				Dir:     cueRoot,
				Overlay: overlay,
			})
			if len(insts) != 1 {
				cueErr = fmt.Errorf("schemas: expected 1 CUE instance for %s, got %d", dir, len(insts))
				return
			}
			if insts[0].Err != nil {
				cueErr = fmt.Errorf("schemas: load CUE %s: %w", dir, insts[0].Err)
				return
			}
			val := cueCtx.BuildInstance(insts[0])
			if err := val.Err(); err != nil {
				cueErr = fmt.Errorf("schemas: build CUE %s: %w", dir, err)
				return
			}
			cuePkgs[dir] = val
		}
	})
}

// ApplyDefaults fills in every default a document has left out and returns it
// re-encoded. It is the write path's job, done once.
//
// Call it AFTER Validate, never before. Some defaults are placeholders standing
// in for a value the author was supposed to supply — a metrics query with no
// selectedMetric acquires a real, unrelated metric name — so filling them first
// converts a document that should have been rejected into one that looks fine
// and queries the wrong thing. That substitution, followed by a save, is the
// incident this repo exists after.
//
// Never call it on READ. `gen/*.defaults.json` and the `default` annotations in
// the schema are documentation of what a writer would have got; applying them
// again when loading a stored document rewrites history, because the document
// that omitted a field did so at a time when the default may have been
// different.
//
// Defaults are filled by unifying the document with the CUE contract, rather
// than by merging the generated defaults file into it. CUE resolves a default
// wherever it appears — inside a list element, behind a discriminated union arm,
// nested three deep — and a flat merge cannot. Unknown fields survive, because
// every definition in the contract is open.
func ApplyDefaults(f Format, document []byte) ([]byte, error) {
	target, ok := cueTarget[f]
	if !ok {
		return nil, fmt.Errorf("schemas: unknown format %q", f)
	}
	loadCUE()
	if cueErr != nil {
		return nil, cueErr
	}

	schema := cuePkgs[target.pkg].LookupPath(cue.ParsePath(target.def))
	if err := schema.Err(); err != nil {
		return nil, fmt.Errorf("schemas: lookup %s: %w", target.def, err)
	}

	expr, err := cuejson.Extract("document.json", document)
	if err != nil {
		return nil, Errors{{Message: "document is not valid JSON: " + err.Error(), Rule: "json"}}
	}
	doc := cueCtx.BuildExpr(expr)
	if err := doc.Err(); err != nil {
		return nil, fmt.Errorf("schemas: decode document: %w", err)
	}

	unified := schema.Unify(doc)
	if err := unified.Err(); err != nil {
		return nil, Errors{{Message: "document does not fit the " + string(f) + " contract: " + err.Error(), Rule: "defaults"}}
	}

	out, err := unified.MarshalJSON()
	if err != nil {
		// Reached when a required field is missing, since the unified value then
		// has nothing concrete to encode there. Validate says which field; this
		// says why encoding stopped.
		return nil, Errors{{
			Message: "cannot fill defaults until the document is complete — run Validate first (" + err.Error() + ")",
			Rule:    "defaults",
		}}
	}
	return out, nil
}
