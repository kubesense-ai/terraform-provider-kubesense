package schema

import (
	"encoding/json"
	"errors"
)

// ErrPresetNotJSON and ErrPresetNotMigratable classify the two hard failures of
// CheckDashboardPreset, so a caller can word them without parsing messages. The
// error returned wraps one of them (errors.Is) and the underlying cause
// (errors.Unwrap), and its text is "<kind>: <cause>".
var (
	ErrPresetNotJSON       = errors.New("preset is not valid JSON")
	ErrPresetNotMigratable = errors.New("preset could not be migrated")
)

type presetError struct {
	kind  error
	cause error
}

func (e *presetError) Error() string        { return e.kind.Error() + ": " + e.cause.Error() }
func (e *presetError) Is(target error) bool { return target == e.kind }
func (e *presetError) Unwrap() error        { return e.cause }

// PresetCheck is the verdict on one dashboard preset, before any origin policy.
type PresetCheck struct {
	// Migrated is the form to store: always the migrated bytes, whatever the
	// verdict, so what was checked is what gets written.
	Migrated []byte

	// Findings is every contract finding on the migrated form; nil when clean.
	Findings Errors

	// ContractErr is set when Validate failed for a reason other than findings,
	// i.e. the contract could not be applied. That is our problem, not the
	// document's, so it is never a refusal.
	ContractErr error
}

// CheckDashboardPreset migrates a dashboard preset to the current version and
// holds the RESULT against the wire contract.
//
// Migrating first is load-bearing, not tidiness. The contract describes the
// current shape; a legacy document is by definition one written before it. A
// preset carrying `yaxis: "auto"` — the form that field had before webapp commit
// 2c8d7ab1a — was reported as six contract violations when it was really one
// document that nobody had brought forward.
//
// The migrated preset is RETURNED rather than validated and dropped, because the
// caller must persist the same bytes that were checked. Validating one document
// and storing another is worse than not checking at all: it certifies a shape
// that never reaches the database, and leaves the legacy row in place to fail
// the same way on the next read.
//
// Input that is not JSON, or that comes from a FUTURE version, returns an error
// wrapping ErrPresetNotJSON or ErrPresetNotMigratable. Both are refused for
// every origin: storing the first yields a dashboard that cannot render for
// anyone, and this build cannot know which fields of the second it fails to
// understand, so storing it after a partial read is how a stale writer corrupts
// a document it did not author.
//
// Everything else is a verdict, not an error. Whether a finding refuses the
// write is the caller's origin policy; see Refused.
func CheckDashboardPreset(raw []byte) (PresetCheck, error) {
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return PresetCheck{}, &presetError{kind: ErrPresetNotJSON, cause: err}
	}

	migrated, err := Migrate(DashboardPreset, raw)
	if err != nil {
		return PresetCheck{}, &presetError{kind: ErrPresetNotMigratable, cause: err}
	}

	check := PresetCheck{Migrated: migrated}
	verr := Validate(DashboardPreset, migrated)
	if verr == nil {
		return check, nil
	}
	if !errors.As(verr, &check.Findings) {
		check.ContractErr = verr
	}
	return check, nil
}

// Refused applies the origin policy to the findings and returns those that
// refuse the write.
//
// A programmatic producer (strict) is refused every finding and handed them,
// because it can act on them and retry. The UI's own path (not strict) is
// refused only RefusedForEveryOrigin, the tab rules, and logs the rest until the
// stored dashboards have been swept: a dashboard that fails to import is an
// annoyance, but one someone cannot save because the contract is wrong about a
// shape we have not seen yet is an outage. Same asymmetry as the alert gate (see
// ruleengine.checkContract).
func (c PresetCheck) Refused(strict bool) Errors {
	if strict {
		return c.Findings
	}
	return c.Findings.RefusedForEveryOrigin()
}
