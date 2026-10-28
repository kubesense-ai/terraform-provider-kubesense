package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// legacySchemaVersions is what a server without GET /schema/versions writes.
// That endpoint arrived before any format moved past v1, so its absence
// proves v1 throughout.
var legacySchemaVersions = map[string]int{"alert-rule": 1, "dashboard-preset": 1}

// ServerSchemaVersions is the schema version the server writes for each
// format, keyed by format name ("dashboard-preset"). It is asked once per
// client and remembered.
//
// The provider needs it when it is NEWER than the server: a document sent in
// a version the server has not reached is refused as being from its future,
// so the provider converts down to this before it writes.
func (c *Client) ServerSchemaVersions(ctx context.Context) (map[string]int, error) {
	c.serverVersionsOnce.Do(func() {
		var out struct {
			SchemaVersions map[string]int `json:"schema_versions"`
		}
		_, err := c.do(ctx, http.MethodGet, "/schema/versions", nil, nil, &out)
		switch {
		case IsEndpointMissing(err):
			c.serverVersions = legacySchemaVersions
		case err != nil:
			c.serverVersionsErr = err
		default:
			c.serverVersions = out.SchemaVersions
		}
	})
	return c.serverVersions, c.serverVersionsErr
}

// Finding is one contract problem the server reports for a document, in the
// shape kubeapi's schema.Error serialises to.
type Finding struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Rule    string `json:"rule"`
}

func (f Finding) String() string {
	if f.Path == "" {
		return f.Message
	}
	return f.Path + ": " + f.Message
}

// ValidateDashboard asks the server whether it would accept preset, without
// writing anything. The answer is the SERVER's contract, which is the point:
// the one embedded in the provider is the contract as of the provider's
// release, and the server's may since have widened (a new panel type, a new
// option) in ways the provider would wrongly refuse.
//
// A server too old to have the endpoint answers with an error for which
// IsEndpointMissing is true.
func (c *Client) ValidateDashboard(ctx context.Context, preset []byte) (bool, []Finding, error) {
	var out struct {
		Valid    bool      `json:"valid"`
		Findings []Finding `json:"findings"`
	}
	if _, err := c.do(ctx, http.MethodPost, "/dashboard/validate", nil, json.RawMessage(preset), &out); err != nil {
		return false, nil, err
	}
	return out.Valid, out.Findings, nil
}
