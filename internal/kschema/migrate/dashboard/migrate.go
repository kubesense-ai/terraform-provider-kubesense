// Migration chain for the dashboard preset document.
//
// Go only — deliberately. The npm package ships no migration code, because the
// API migrates on read: the UI therefore never receives an unmigrated document
// and needs none. The `files` whitelist in package.json enforces that
// structurally, since it ships only ./gen.
//
// Presets are versioned at the document level, so a step here migrates nested
// panels too. There is no separate panel chain — see the schema_version comment
// on #Preset for why.
package dashboard

import "github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/internal/migrate"

// CurrentVersion is the schema_version this build writes and understands. It
// must equal the version the last step in `steps` produces; migrate.Check
// enforces that on every call.
const CurrentVersion = 1

// steps is the ordered n -> n+1 chain. Append only; never edit or reorder a step
// that has shipped (see migrate.Step for why).
//
// Adding one means three things, in this order:
//
//  1. a new file `migrate_v<n>_v<n+1>.go` holding the transform, named
//     Migrate<from>To<to>
//  2. an entry appended here, and CurrentVersion bumped to match
//  3. a fixture pair under ./testdata/migrate/<from>_<to>/ — {input,want}.json
//
// From v2 on a step also needs Down, its inverse; migrate.Check refuses the
// chain without it. See migrate.Step.Down for why, and the fixture pair doubles
// as its test: Down(want.json) must give back input.json.
var steps = []migrate.Step{
	{To: 1, Apply: MigrateLegacyToV1},
}

// StepCount is how many migrations the chain runs to reach CurrentVersion from
// legacy. The test harness needs it to assert one fixture pair per step, and it
// cannot be derived from CurrentVersion: a format whose legacy documents were
// already v1-shaped reaches v1 with no steps at all.
func StepCount() int { return len(steps) }

// Migrate brings a dashboard preset up to CurrentVersion, applying each n -> n+1
// step the document has not already been through, and returns it re-encoded.
//
// Unknown fields survive: the document is carried as a map, so a field this
// build has never heard of round-trips untouched rather than being dropped.
// That matters because the caller usually writes the result back — and it is
// how a dashboard saved by a newer client survives being opened by an older one.
func Migrate(document []byte) ([]byte, error) {
	return migrate.Run(document, steps, CurrentVersion)
}

// MigrateTo brings a dashboard preset to version: up to CurrentVersion first, then back
// down through each step's Down. It is how the API answers a client — the
// Terraform provider — that was built against an older version than this one.
// A version above CurrentVersion is answered at CurrentVersion.
func MigrateTo(document []byte, version int) ([]byte, error) {
	return migrate.RunTo(document, steps, CurrentVersion, version)
}
