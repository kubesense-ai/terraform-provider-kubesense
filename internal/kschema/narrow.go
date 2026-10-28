package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// narrow.go — makes a failed union report the arm the document claimed, instead
// of every arm it did not.
//
// A panel query is a union discriminated on `selectedMode`. Keeping only the
// leaves of the error tree, which is what flatten did on its own, reports all
// four arms at once: a traces query with a bad chart_type came back as
//
//	/queries/0: missing property 'expression'          (the formula arm)
//	/queries/0/chart_type: value must be 'table'
//	/queries/0/chart_type: value must be one of 'table', 'stat', …
//	/queries/0/selectedMode: value must be 'formula'
//	/queries/0/selectedMode: value must be 'logs'
//	/queries/0/selectedMode: value must be 'metrics'
//
// One of those six is about the document. The rest describe arms it never
// claimed — and `expression` in particular sends a reader to add a field that
// would be WRONG on a traces query.
//
// This matters most for the MCP tools. A model reading that list edits toward
// whichever line it believes, and four of the six lead it somewhere worse. The
// browser already narrows this way (webapp src/contract/discriminated.ts); the
// agent had been getting the unfiltered version, which is backwards given the
// agent is the consumer this contract exists to serve.
//
// The error tree makes the arms addressable: an `anyOf` failure carries one
// cause per arm, each rooted at #/$defs/Query/anyOf/N. So an arm that failed
// because its own discriminating constant did not match is one the document
// never claimed, and everything underneath it can go.

// claimedArms splits a failed anyOf into the arms the document plausibly
// claimed and the constants the rejected ones wanted.
//
// An arm is REJECTED when it failed on a `const` sitting directly on one of the
// union subject's own properties — that is the discriminator saying "this is not
// my arm". The walk deliberately stops at any nested branch: a `const` inside a
// property's own union (chart_type is one) says nothing about which arm applies,
// and counting it would reject every arm and lose the real finding.
func claimedArms(union *jsonschema.ValidationError) (claimed []*jsonschema.ValidationError, wanted []any, property string) {
	depth := len(union.InstanceLocation)

	for _, arm := range union.Causes {
		constant, prop, rejected := discriminatorOf(arm, depth)
		if !rejected {
			claimed = append(claimed, arm)
			continue
		}
		wanted = append(wanted, constant.Want)
		property = prop
	}
	return claimed, wanted, property
}

// discriminatorOf finds the constant an arm pins on the subject's own property,
// if that is why the arm failed.
//
// Descends only through $ref and sub-schema wrappers — the "validation failed"
// nodes that carry no verdict of their own — so the first real keyword decides.
func discriminatorOf(arm *jsonschema.ValidationError, depth int) (*kind.Const, string, bool) {
	var found *kind.Const
	var property string

	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if found != nil {
			return
		}
		switch e.ErrorKind.(type) {
		case *kind.Reference, *kind.Schema:
			for _, cause := range e.Causes {
				walk(cause)
			}
			return
		}
		if constant, ok := e.ErrorKind.(*kind.Const); ok && len(e.InstanceLocation) == depth+1 {
			found = constant
			property = e.InstanceLocation[depth]
		}
	}
	walk(arm)

	return found, property, found != nil
}

// wrongArgument is claimedArms for a function whose argument list has one
// definition, where there is no union to narrow. A builder argument that fails
// its own arg_name const is a different argument, so its arg_value failing too
// is noise: `abs` given `{"arg_name":"over","arg_value":5}` should hear which
// names it takes, not also that keep_metric_names wants a boolean. Scoped to
// the generated #Arg_* definitions, whose arg_name is a discriminator by
// construction; a const elsewhere may be an ordinary field.
func wrongArgument(e *jsonschema.ValidationError) (Error, bool) {
	ref, ok := e.ErrorKind.(*kind.Reference)
	if !ok || !strings.Contains(ref.URL, "/$defs/Arg_") {
		return Error{}, false
	}
	constant, property, rejected := discriminatorOf(e, len(e.InstanceLocation))
	if !rejected || property != "arg_name" {
		return Error{}, false
	}
	return Error{
		Path:    pointer(append(append([]string{}, e.InstanceLocation...), property)),
		Message: "value must be " + renderValue(constant.Want),
		Rule:    "shape",
	}, true
}

// unknownArmError is the single finding for a document whose discriminator
// matches no arm at all.
//
// Naming every legal value once beats one "value must be 'metrics'" per arm: the
// reader needs the set, and four separate refusals are the set spelled out in
// the least usable form available.
func unknownArmError(union *jsonschema.ValidationError, wanted []any, property string) Error {
	rendered := make([]string, 0, len(wanted))
	for _, want := range wanted {
		rendered = append(rendered, renderValue(want))
	}
	return Error{
		Path:    pointer(append(append([]string{}, union.InstanceLocation...), property)),
		Message: "value must be one of " + strings.Join(rendered, ", "),
		Rule:    "shape",
	}
}

// renderValue writes a schema-supplied value the way it would appear in the
// document, so a reader can copy it.
//
// Worth doing rather than leaning on the library: its Const message degrades to
// "'const' failed" for anything that is not a scalar, which names neither the
// field's problem nor its remedy. `yaxis: ["auto"]` was reported that way.
func renderValue(value any) string {
	if text, ok := value.(string); ok {
		return "'" + text + "'"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

// describeLeaf renders one leaf error, preferring our own wording where the
// library's is unusable.
func describeLeaf(e *jsonschema.ValidationError) string {
	if constant, ok := e.ErrorKind.(*kind.Const); ok {
		return "value must be " + renderValue(constant.Want)
	}
	return e.ErrorKind.LocalizedString(englishPrinter)
}

// mergeAlternatives folds the branches of one surviving union into a single
// finding.
//
// The branches of an `anyOf` are alternatives, not a list of separate faults, so
// N of them against the same field is one problem described N ways. `chart_type`
// fails both its arms and would otherwise read as two defects in one field.
//
// Only merged when every finding addresses the SAME path; a union whose arms
// fail in different places is genuinely several problems.
func mergeAlternatives(findings Errors) Errors {
	if len(findings) < 2 {
		return findings
	}
	path := findings[0].Path
	for _, finding := range findings[1:] {
		if finding.Path != path {
			return findings
		}
	}

	options := make([]string, 0, len(findings))
	seen := make(map[string]bool, len(findings))
	for _, finding := range findings {
		option := optionPhrase(finding.Message)
		if seen[option] {
			continue
		}
		seen[option] = true
		options = append(options, option)
	}

	kept := make([]string, 0, len(options))
	for _, option := range options {
		if subsumed(option, options) {
			continue
		}
		kept = append(kept, option)
	}
	if len(kept) == 0 {
		kept = options
	}

	return Errors{{
		Path:    path,
		Message: "value must be " + strings.Join(kept, " or "),
		Rule:    findings[0].Rule,
	}}
}

// optionPhrase reduces one branch's message to the VALUE it wanted, so the
// branches read as a list of alternatives rather than as a stack of sentences.
//
// The library phrases a type failure as "got string, want array", which is a
// complete sentence about the document and unusable as the second half of
// "must be X or …".
func optionPhrase(message string) string {
	for _, lead := range []string{"value must be ", "must be "} {
		if trimmed, found := strings.CutPrefix(message, lead); found {
			return trimmed
		}
	}
	if i := strings.Index(message, ", want "); i >= 0 {
		return message[i+len(", want "):]
	}
	return message
}

// subsumed reports whether an option is already covered by another.
//
// A union of `const "table"` and an enum containing "table" is one the schema
// author wrote for a default, and reporting both yields
// "must be 'table' or one of 'table', 'stat', …" — which invites the reader to
// wonder what the difference is. There is none.
func subsumed(option string, all []string) bool {
	if !strings.HasPrefix(option, "'") {
		return false
	}
	for _, other := range all {
		if other == option || !strings.HasPrefix(other, "one of ") {
			continue
		}
		for _, listed := range strings.Split(strings.TrimPrefix(other, "one of "), ", ") {
			if listed == option {
				return true
			}
		}
	}
	return false
}
