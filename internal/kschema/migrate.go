package schema

import (
	"fmt"

	alertmigrate "github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/migrate/alert"
	dashboardmigrate "github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/migrate/dashboard"
)

// Migrate brings a document up to the current version for its format.
//
// A thin dispatcher over the per-format chains, for callers that hold a Format
// rather than a concrete type — the MCP validate tools are one handler shared by
// both contracts, and without this they would have to switch on the format
// themselves, which is where the two drift apart.
//
// ALWAYS migrate before validating. The contract describes the CURRENT shape, so
// validating a stored document directly reports every field that has since moved
// as an error against a document that is merely old. That is not hypothetical:
// a legacy preset carrying `yaxis: "auto"` came back with six findings, none of
// them things a caller could act on.
func Migrate(f Format, document []byte) ([]byte, error) {
	switch f {
	case AlertRule:
		return alertmigrate.Migrate(document)
	case DashboardPreset:
		return dashboardmigrate.Migrate(document)
	case Panel, PanelQuery:
		// Panels are versioned by the preset that holds them, not on their own —
		// see the schema_version comment on #Preset. A panel validated in
		// isolation has no version to read and nothing to migrate, so it is
		// returned untouched rather than being stamped with one it cannot carry.
		return document, nil
	default:
		return nil, fmt.Errorf("schema: cannot migrate unknown format %q", f)
	}
}

// MigrateTo brings a document to version for its format, migrating down from
// the current version when the caller is older than this build. See
// migrate.Step.Down. Panels follow their preset, as in Migrate.
func MigrateTo(f Format, document []byte, version int) ([]byte, error) {
	switch f {
	case AlertRule:
		return alertmigrate.MigrateTo(document, version)
	case DashboardPreset:
		return dashboardmigrate.MigrateTo(document, version)
	case Panel, PanelQuery:
		return document, nil
	default:
		return nil, fmt.Errorf("schema: cannot migrate unknown format %q", f)
	}
}

// CurrentVersion is the schema_version this build writes for a versioned
// format, and false for one versioned by its container (a panel).
func CurrentVersion(f Format) (int, bool) {
	switch f {
	case AlertRule:
		return alertmigrate.CurrentVersion, true
	case DashboardPreset:
		return dashboardmigrate.CurrentVersion, true
	default:
		return 0, false
	}
}
