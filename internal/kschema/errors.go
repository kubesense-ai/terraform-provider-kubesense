package schema

import (
	"fmt"
	"sort"
	"strings"
)

// Error is one problem with one document.
//
// Path is an RFC 6901 JSON Pointer — "/query_config/0/queryMode", or "" for the
// document itself. That is deliberately the same form ajv reports as
// `instancePath`, so a failure described by the Go module and one described by
// the npm package name the same location in the same syntax. A message a user
// sees on write and a message an agent sees while composing should not need
// translating between them.
// The JSON tags are part of the contract's surface, not an implementation
// detail: an alert validation endpoint, a dashboard one, and the MCP tools all
// report findings, and they must report them identically. Rendering this type
// separately in each place is how three subtly different error shapes appear.
type Error struct {
	// Path locates the offending value. Empty means the whole document.
	Path string `json:"path"`

	// Message says what is wrong, in a form that can be shown to a person.
	Message string `json:"message"`

	// Rule names the check that failed: "shape" for anything the JSON Schema
	// caught, otherwise the semantic rule's own name, e.g. "metric_query_label".
	// Callers branch on this rather than on message text.
	Rule string `json:"rule"`
}

// Error renders one finding as "path: message", omitting the path for a
// document-level problem.
func (e Error) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

// Errors is every problem found in one document.
//
// Validation collects rather than short-circuits: an agent repairing its own
// output, which is one of the four consumers here, fixes far more per round trip
// when it is told all six things at once instead of the first one six times.
type Errors []Error

// Error lists every finding, one per line, under a count. The count leads
// because it is the part a caller logging a single line most needs.
func (errs Errors) Error() string {
	if len(errs) == 0 {
		return "no validation errors"
	}
	if len(errs) == 1 {
		return errs[0].Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d validation errors:", len(errs))
	for _, e := range errs {
		b.WriteString("\n  - ")
		b.WriteString(e.Error())
	}
	return b.String()
}

// sorted orders findings by path, then rule, then message, so the same document
// always produces the same report. Schema validation walks maps, so without this
// the order would vary between runs and make output impossible to diff or test.
func (errs Errors) sorted() Errors {
	out := make(Errors, len(errs))
	copy(out, errs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].Message < out[j].Message
	})
	return out
}

// deduped drops repeats of the same finding at the same place.
//
// A failed `anyOf` reports once per arm, so a single wrong `selectedMode` on a
// query arrives four times — once from each arm of the union it failed to match.
// Reporting that four times is noise whichever consumer reads it.
func (errs Errors) deduped() Errors {
	seen := make(map[Error]bool, len(errs))
	out := make(Errors, 0, len(errs))
	for _, e := range errs {
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// orNil returns the findings as an error, or a nil error when there are none.
//
// Returning Errors directly from a function typed to return `error` would
// produce a non-nil error holding an empty slice — the classic typed-nil trap,
// where `err != nil` is true for a document that validated cleanly.
func (errs Errors) orNil() error {
	if len(errs) == 0 {
		return nil
	}
	return errs.deduped().sorted()
}
