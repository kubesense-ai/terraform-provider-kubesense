// Package migrate runs a document through an ordered chain of n -> n+1 schema
// migrations. Each format (alert, dashboard) declares its own chain and wraps
// Run; this package holds the part that would otherwise be copy-pasted.
//
// It is internal on purpose: consumers call alert.Migrate or dashboard.Migrate,
// which know their own CurrentVersion and cannot be called with a mismatched
// pair.
//
// Documents are carried as map[string]any rather than typed structs, for one
// load-bearing reason: unknown fields survive. A document written by a newer
// client, read by an older binary, migrated and written back keeps every field
// the binary has never heard of. Unmarshalling into a struct would silently drop
// them, which is exactly the data loss this repo exists to prevent.
package migrate

import (
	"encoding/json"
	"fmt"
)

// VersionField is the member every versioned KubeSense document carries.
const VersionField = "schema_version"

// LegacyVersion is what a document with no version field is.
//
// Absence means LEGACY, not v1. Every document stored before this field existed
// predates the contract, so it is the one population guaranteed NOT to have been
// checked against it — a legacy dashboard carries `yaxis: "cpu"` from before that
// field became a list, and calling that v1 would assert a shape it does not have.
//
// Naming it 0 is what makes the first migration expressible: legacy -> v1 is a
// real step with a real transform, rather than a version the chain has to
// pretend already happened.
const LegacyVersion = 0

// Step is one n -> n+1 migration.
//
// Apply mutates the document in place and must be PURE — no I/O, no clock, no
// randomness — so it is reproducible and testable in isolation.
//
// A Step that has shipped is frozen. Never edit one: documents already at its
// target version will not run it again, so an edit leaves two populations of the
// same version with different shapes. Fix forward by appending a new Step.
type Step struct {
	// To is the version this step produces. Steps must ascend by exactly one.
	To int

	// Apply transforms a document of version To-1 into one of version To. It must
	// not set the version field — Run does that, so a step cannot forget to.
	Apply func(doc map[string]any) error

	// Down transforms a document of version To back into one of version To-1,
	// under the same rules as Apply: pure, and it must not set the version field.
	//
	// It is what lets a client that predates this step keep working against a
	// server that has it. The Terraform provider is pinned by its users and
	// upgraded on their schedule, not ours, so a server that bumps the version
	// still has to answer an old provider in the shape that provider was built
	// against — or every one of its plans shows a change it cannot explain.
	//
	// REQUIRED on every step from v2 on; Check refuses a chain without it. The
	// legacy -> v1 step is the one exception: v1 is the oldest version any
	// client has ever been sent, so there is nothing below it to answer in.
	//
	// Down may be lossy — a v1 document cannot carry what v2 added. That is
	// acceptable because the client asking for v1 could not have shown or kept
	// the addition anyway; what it must never do is produce a document v1
	// readers reject.
	Down func(doc map[string]any) error
}

// Oldest is the lowest version a chain can answer a client in: every Down
// from current back to it exists. Check guarantees that is v1 for any chain
// it accepts; the constant is named so callers do not hard-code the 1.
const Oldest = LegacyVersion + 1

// Run migrates doc up to current, applying every step whose target exceeds the
// document's own version, and returns the re-encoded document.
//
// A document already at current is returned re-encoded but otherwise untouched.
// A document from the FUTURE — a version above current, written by a newer
// binary — is an error rather than a silent pass: the caller cannot know which
// fields it fails to understand, and rendering it as if it were current is how a
// stale reader corrupts a document it then saves.
func Run(raw []byte, steps []Step, current int) ([]byte, error) {
	if err := Check(steps, current); err != nil {
		return nil, err
	}

	// Already current: hand back the bytes untouched.
	//
	// This is the STEADY STATE, not an edge case. Writes normalise, so once a
	// document has been through the chain once every later read finds it current
	// — and without this, each of those reads pays a full decode into
	// map[string]any, a walk, and a re-encode to produce the bytes it started
	// with. On a 30-panel preset that measured 328us and 359KB of garbage per
	// call, multiplied by the page size on the list endpoint.
	//
	// The peek reads one field into a narrow struct: the scanner still walks the
	// bytes but builds nothing for the fields it skips, which is roughly two
	// orders of magnitude cheaper than materialising the document.
	if alreadyAt(raw, current) {
		return raw, nil
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("migrate: decode document: %w", err)
	}

	from, err := versionOf(doc)
	if err != nil {
		return nil, err
	}
	if from > current {
		return nil, fmt.Errorf(
			"migrate: document is %s %d but this build only understands up to %d — upgrade before reading it",
			VersionField, from, current)
	}

	for _, step := range steps {
		if step.To <= from {
			continue
		}
		if err := step.Apply(doc); err != nil {
			return nil, fmt.Errorf("migrate: step v%d -> v%d: %w", step.To-1, step.To, err)
		}
		// Set the version here rather than inside the step, so a step physically
		// cannot leave the document mislabelled.
		doc[VersionField] = step.To
	}

	// A legacy document whose format needed no transform gets stamped anyway, so
	// what is written back is explicit about what it is.
	//
	// An explicit null counts as absent, not as present. versionOf already reads
	// it as legacy, so testing only for the KEY left `"schema_version": null`
	// in the output — a document the next reader has to make the same inference
	// about, from a field that is supposed to have ended the guessing.
	if v, ok := doc[VersionField]; !ok || v == nil {
		doc[VersionField] = current
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("migrate: encode document: %w", err)
	}
	return out, nil
}

// RunTo migrates doc to target: up to current first, exactly as Run does, then
// back down one Down at a time until it reaches target.
//
// target is the newest version the CLIENT understands, so it can be above
// current — a provider newer than the server it talks to. That is answered at
// current, not refused: the client knows every version up to its own, and
// down-converting is the server's job only when the client is the older one.
//
// A target below Oldest is refused rather than clamped. No client has ever
// been sent a legacy document, so a request for one is a bug in the caller,
// and answering it at v1 would hide that.
func RunTo(raw []byte, steps []Step, current, target int) ([]byte, error) {
	if target < Oldest {
		return nil, fmt.Errorf("migrate: cannot answer in %s %d — the oldest is %d", VersionField, target, Oldest)
	}
	up, err := Run(raw, steps, current)
	if err != nil {
		return nil, err
	}
	if target >= current {
		return up, nil
	}

	var doc map[string]any
	if err := json.Unmarshal(up, &doc); err != nil {
		return nil, fmt.Errorf("migrate: decode document: %w", err)
	}
	// Steps are indexed by position, and Check has proved position i produces
	// v(i+1), so the step that produced version v sits at v-1.
	for v := current; v > target; v-- {
		step := steps[v-1]
		if err := step.Down(doc); err != nil {
			return nil, fmt.Errorf("migrate: step v%d -> v%d: %w", v, v-1, err)
		}
		doc[VersionField] = v - 1
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("migrate: encode document: %w", err)
	}
	return out, nil
}

// Check asserts the chain is well-formed. It runs on every Run rather than only
// in tests, because a malformed chain silently skipping a step is worse than a
// failed read: the document would be stamped with a version whose shape it does
// not have.
func Check(steps []Step, current int) error {
	if current < 1 {
		return fmt.Errorf("migrate: current version must be at least 1, got %d", current)
	}
	// The first possible step produces v1, migrating a legacy document.
	want := LegacyVersion + 1
	for i, step := range steps {
		if step.Apply == nil {
			return fmt.Errorf("migrate: step %d (to v%d) has no Apply", i, step.To)
		}
		if step.To != want {
			return fmt.Errorf(
				"migrate: steps must ascend by one — step %d targets v%d, expected v%d",
				i, step.To, want)
		}
		if step.To > Oldest && step.Down == nil {
			return fmt.Errorf(
				"migrate: step %d (to v%d) has no Down — every step from v%d on must be reversible, "+
					"or clients built against v%d break when this ships",
				i, step.To, Oldest+1, step.To-1)
		}
		want++
	}
	// An EMPTY chain is legal only at v1, and it is a claim rather than an
	// omission: that legacy documents of this format are already v1-shaped and
	// need no transform. Any other shortfall is a missing step.
	if len(steps) == 0 {
		if current != 1 {
			return fmt.Errorf(
				"migrate: CurrentVersion is %d but the chain is empty — add the missing steps",
				current)
		}
		return nil
	}

	// want-1 is the version the last step produces.
	if got := want - 1; got != current {
		return fmt.Errorf(
			"migrate: chain produces v%d but CurrentVersion is %d — add the missing step or bump the constant",
			got, current)
	}
	return nil
}

// versionOf reads the document's version, treating absence as LEGACY.
//
// JSON numbers decode as float64, so an integral float is accepted and anything
// else — a string "2", a fraction — is rejected rather than coerced. A version
// that has to be guessed at is not a version.
func versionOf(doc map[string]any) (int, error) {
	v, ok := doc[VersionField]
	if !ok || v == nil {
		return LegacyVersion, nil
	}
	f, ok := v.(float64)
	if !ok || f != float64(int(f)) || f < 1 {
		return 0, fmt.Errorf("migrate: %s must be a positive integer, got %v", VersionField, v)
	}
	return int(f), nil
}

// alreadyAt reports whether a document is already stamped at exactly version.
//
// Deliberately conservative: anything it cannot read confidently — malformed
// JSON, a missing or non-integral version, a version from the future — returns
// false and falls through to the full path, where the existing decode reports
// the problem properly. This is a fast path, so it is never the thing that
// decides a document is bad.
func alreadyAt(raw []byte, version int) bool {
	var probe struct {
		Version *float64 `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	if probe.Version == nil {
		return false
	}
	return *probe.Version == float64(version)
}
