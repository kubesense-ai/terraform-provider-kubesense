package schema

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/internal/migrate"
)

// VersionsHeader carries the schema versions a client was built against, so
// the API can answer it in those versions rather than in its own.
//
//	KubeSense-Schema-Versions: alert-rule=1, dashboard-preset=1
//
// It exists for the Terraform provider. Users pin the provider and upgrade it
// on their own schedule, independently of the server, so a server that bumps
// a format's version will be read by providers built against the old one. A
// provider handed a document from its future cannot compare it with what the
// user wrote, and every plan shows a change; a provider handed its own version
// sees exactly what it wrote. See migrate.Step.Down.
//
// Absent means "whatever this build writes": the browser, the MCP tools and
// every other client ship with the server and are never behind it.
const VersionsHeader = "KubeSense-Schema-Versions"

// OldestVersion is the lowest version any format can be answered in. See
// migrate.Oldest: every step from v2 on must be reversible, so it is v1.
const OldestVersion = migrate.Oldest

// Versions is the version a client understands for each format it names.
type Versions map[Format]int

// Header renders versions for VersionsHeader, in a stable order.
func (v Versions) Header() string {
	parts := make([]string, 0, len(v))
	for f, n := range v {
		parts = append(parts, fmt.Sprintf("%s=%d", f, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// ParseVersions reads a VersionsHeader value. Formats this build does not
// version are skipped, not refused: a newer client may name a format that did
// not exist when this server was built, and that is no reason to fail a read
// of one it does know.
func ParseVersions(header string) (Versions, error) {
	out := Versions{}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, num, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("%s: %q is not format=version", VersionsHeader, part)
		}
		n, err := strconv.Atoi(strings.TrimSpace(num))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%s: %q is not a positive version", VersionsHeader, part)
		}
		f := Format(strings.TrimSpace(name))
		if _, versioned := CurrentVersion(f); versioned {
			out[f] = n
		}
	}
	return out, nil
}

// CurrentVersions is every versioned format at the version this build writes:
// what a client sends in VersionsHeader, and what the API reports it serves.
func CurrentVersions() Versions {
	out := Versions{}
	for _, f := range []Format{AlertRule, DashboardPreset} {
		n, _ := CurrentVersion(f)
		out[f] = n
	}
	return out
}

// ForClient renders a document of format f for a client that sent header: in
// the client's version when it is older than this build, otherwise unchanged
// apart from the up-migration every read already gets.
//
// An empty header, or one that does not name f, is answered at the current
// version, as every read was before the header existed.
func ForClient(f Format, document []byte, header string) ([]byte, error) {
	if strings.TrimSpace(header) == "" {
		return Migrate(f, document)
	}
	versions, err := ParseVersions(header)
	if err != nil {
		return nil, err
	}
	target, named := versions[f]
	if !named {
		return Migrate(f, document)
	}
	return MigrateTo(f, document, target)
}
