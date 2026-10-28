// Package schema holds the KubeSense wire-format contract and validates
// documents against it — alert rules, panels and dashboards.
//
// It is three things in one place, because they only make sense together:
//
//   - the SCHEMAS: ./cue is the source of truth, and ./gen holds the JSON Schema
//     and default instances generated from it (see schemagen_test.go)
//   - the VALIDATE functions: Validate, ValidateShape and ApplyDefaults, below
//   - the MIGRATE functions: ./migrate/alert and ./migrate/dashboard, which walk
//     a stored document up to the current version
//
// The write path is three steps, in this order:
//
//	raw, err := alert.Migrate(raw)        // bring an older document up to date
//	err = schema.Validate(schema.AlertRule, raw)
//	raw, err = schema.ApplyDefaults(schema.AlertRule, raw)
//
// Validate runs BEFORE ApplyDefaults, and the order is load-bearing rather than
// arbitrary. Several defaults are placeholders that stand in for a missing
// value — #MetricsQuery.selectedMetric is a real metric name that has nothing to
// do with the query being written — so filling defaults first would turn "this
// query names no metric", which is a rejectable document, into a valid query
// against an unrelated metric. Substituting that placeholder and then persisting
// it is the incident this repo was built after.
//
// Validation is two layers with different jobs:
//
//   - SHAPE — types, enums, required fields, patterns — is checked against the
//     generated JSON Schema in ./gen, the same bytes the npm package hands to
//     ajv. Sharing the artifact rather than the source is what makes a Go verdict
//     and a browser verdict the same verdict; a rule the API rejects on write is
//     one the editor rejected while it was being typed.
//   - SEMANTICS — anything spanning two fields — is checked in semantics.go.
//     CUE cannot express those, and the attempt to make it produced errors that
//     dumped the whole document instead of naming a field.
package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// englishPrinter renders the validator's messages. The library localises through
// a printer, and pinning one keeps a message stable regardless of the host
// process's locale — these strings end up in API responses and in fixtures.
var englishPrinter = message.NewPrinter(language.English)

// Format names one contract. The value is the artifact basename in ./gen, so a
// Format is also how a caller asks for the schema or defaults document.
type Format string

const (
	// AlertRule is the alert document as exported, imported and returned by GET.
	// It is NOT the POST body, which is a different shape.
	AlertRule Format = "alert-rule"

	// Panel is one panel: a name, a type, its queries and its config.
	Panel Format = "panel"

	// PanelQuery is a single query in isolation — the union of the metrics, logs,
	// traces and formula arms.
	PanelQuery Format = "panel-query"

	// DashboardPreset is the entire dashboard body: layout, panels, sub-grids and
	// variables. kubeapi stores it as an opaque string.
	DashboardPreset Format = "dashboard-preset"
)

// Formats lists every contract this module validates, in a stable order, so a
// caller can enumerate them without hardcoding the set.
var Formats = []Format{AlertRule, Panel, PanelQuery, DashboardPreset}

// gen holds the generated artifacts. Embedding them rather than reading from
// disk is what lets a consumer depend on this module and get the contract with
// it — there is no file to deploy alongside the binary and no way for the two to
// drift apart.
//
//go:embed gen/*.schema.json gen/*.defaults.json
var gen embed.FS

// valid reports whether f is a Format this module knows.
func (f Format) valid() bool {
	for _, known := range Formats {
		if f == known {
			return true
		}
	}
	return false
}

// String makes Format printable in error messages.
func (f Format) String() string { return string(f) }

// Schema returns the generated JSON Schema for a format.
//
// Exposed because kubeapi serves these at /api/schemas/… and an MCP tool uses
// one as its InputSchema — both want the bytes, not a validator.
func Schema(f Format) ([]byte, error) {
	return artifact(f, "schema")
}

// Defaults returns the generated default instance for a format: the value a
// caller receives having supplied nothing but the required fields.
//
// For a discriminated union — PanelQuery — this is keyed by discriminator value
// ("metrics", "logs", "traces", "formula"), because which defaults apply depends
// on which arm is being authored.
func Defaults(f Format) ([]byte, error) {
	return artifact(f, "defaults")
}

// artifact reads one embedded generated file.
func artifact(f Format, kind string) ([]byte, error) {
	if !f.valid() {
		return nil, fmt.Errorf("schemas: unknown format %q", f)
	}
	b, err := gen.ReadFile("gen/" + string(f) + "." + kind + ".json")
	if err != nil {
		return nil, fmt.Errorf("schemas: %s %s: %w", f, kind, err)
	}
	return b, nil
}

// compiled caches one compiled validator per format.
//
// Compiling parses a schema that reaches 57KB and resolves every $ref in it, so
// it is done once per format per process and never on the request path. sync.Map
// rather than a plain map because kubeapi validates concurrently.
var compiled sync.Map // Format -> *compileResult

type compileResult struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

// validator returns the compiled JSON Schema for a format, compiling on first
// use. A compile failure is cached too: it means the embedded artifact is
// malformed, which retrying cannot fix.
func validator(f Format) (*jsonschema.Schema, error) {
	if !f.valid() {
		return nil, fmt.Errorf("schemas: unknown format %q", f)
	}
	entry, _ := compiled.LoadOrStore(f, &compileResult{})
	result := entry.(*compileResult)
	result.once.Do(func() {
		raw, err := Schema(f)
		if err != nil {
			result.err = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			result.err = fmt.Errorf("schemas: decode %s schema: %w", f, err)
			return
		}
		url := string(f) + ".schema.json"
		c := jsonschema.NewCompiler()
		if err := c.AddResource(url, doc); err != nil {
			result.err = fmt.Errorf("schemas: add %s schema: %w", f, err)
			return
		}
		result.schema, result.err = c.Compile(url)
	})
	return result.schema, result.err
}

// ValidateShape checks a document against the generated JSON Schema only:
// types, enums, required fields, patterns, list arities.
//
// Separate from Validate because the two have different audiences. A form
// checking a field as it is typed wants shape and nothing else — half-built
// documents fail cross-field rules for reasons that are not yet the user's
// fault — while a write path wants both.
//
// A nil error means the document is well-shaped, NOT that it is coherent. A rule
// whose metric_query_label names a query that does not exist passes here and
// fails Validate.
func ValidateShape(f Format, document []byte) error {
	schema, err := validator(f)
	if err != nil {
		return err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return Errors{{Message: "document is not valid JSON: " + err.Error(), Rule: "json"}}
	}
	if err := schema.Validate(instance); err != nil {
		var ve *jsonschema.ValidationError
		if !errors.As(err, &ve) {
			return err
		}
		return flatten(ve).orNil()
	}
	return nil
}

// Validate checks shape and then semantics, and is what a write path should
// call.
//
// Semantics run only if the shape is sound. A cross-field rule reading a field
// of the wrong type reports something misleading — "metric_query_label names no
// query" is the wrong thing to say about a document whose query_config is a
// string — so the shape errors are returned without them and the caller fixes
// those first. A preset's tab rules are the exception: they type-assert every
// member, and they must reach the UI's save path, which only logs shape
// findings, whatever else is wrong with the document.
func Validate(f Format, document []byte) error {
	err := ValidateShape(f, document)
	if err == nil {
		return checkSemantics(f, document)
	}
	var shape Errors
	if f != DashboardPreset || !errors.As(err, &shape) {
		return err
	}
	var doc map[string]any
	_ = json.Unmarshal(document, &doc)
	return append(shape, checkTabs(doc)...)
}

// flatten turns the validator's error tree into a flat list of findings.
//
// Only LEAVES are kept. A failed `anyOf` reports itself and then every arm it
// tried, so a query with a misspelled selectedMode yields one useless "does not
// match any arm" wrapper above four specific, useful ones. The wrappers restate
// the branch set without naming the offending field; the leaves name it. This is
// the same filter scripts/check-fixtures.mjs applies to ajv's output, for the
// same reason.
//
// A union gets one more pass before its leaves are taken: see narrow.go. Arms
// the document never claimed are dropped, and the alternatives that remain are
// folded into one sentence — so the leaves that survive are about the document
// in hand rather than about the three shapes it was never trying to be.
func flatten(ve *jsonschema.ValidationError) Errors {
	var out Errors
	var walk func(e *jsonschema.ValidationError)

	// collect runs a subtree in isolation, so a union can flatten its arms and
	// decide what to do with the result before anything is emitted.
	collect := func(e *jsonschema.ValidationError) Errors {
		saved := out
		out = nil
		walk(e)
		gathered := out
		out = saved
		return gathered
	}

	walk = func(e *jsonschema.ValidationError) {
		if _, isUnion := e.ErrorKind.(*kind.AnyOf); isUnion && len(e.Causes) > 1 {
			claimed, wanted, property := claimedArms(e)

			// Every arm refused the discriminator: the value names no arm at all,
			// and the one useful thing to say is which values would.
			if len(claimed) == 0 && property != "" {
				out = append(out, unknownArmError(e, wanted, property))
				return
			}

			// Some arm accepted the discriminator, so its failures are the real
			// ones. With none rejected this is an ordinary union and every arm
			// stands.
			arms := claimed
			if len(arms) == 0 {
				arms = e.Causes
			}

			var gathered Errors
			for _, arm := range arms {
				gathered = append(gathered, collect(arm)...)
			}
			out = append(out, mergeAlternatives(gathered)...)
			return
		}

		if finding, ok := wrongArgument(e); ok {
			out = append(out, finding)
			return
		}

		if len(e.Causes) > 0 {
			for _, cause := range e.Causes {
				walk(cause)
			}
			return
		}
		out = append(out, Error{
			Path:    pointer(e.InstanceLocation),
			Message: describeLeaf(e),
			Rule:    "shape",
		})
	}
	walk(ve)
	if len(out) == 0 {
		// A validation failure with no leaf causes: report the root rather than
		// returning success for a document that did not validate.
		out = append(out, Error{
			Path:    pointer(ve.InstanceLocation),
			Message: ve.ErrorKind.LocalizedString(englishPrinter),
			Rule:    "shape",
		})
	}
	return out
}

// pointer renders an instance location as an RFC 6901 JSON Pointer, matching the
// `instancePath` ajv reports for the same failure.
func pointer(location []string) string {
	if len(location) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range location {
		b.WriteByte('/')
		// RFC 6901: ~ is escaped first, then /, or unescaping is ambiguous.
		seg = strings.ReplaceAll(seg, "~", "~0")
		seg = strings.ReplaceAll(seg, "/", "~1")
		b.WriteString(seg)
	}
	return b.String()
}
