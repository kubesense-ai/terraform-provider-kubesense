package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema"
)

// Users pin this provider and upgrade it on their own schedule, independently
// of the KubeSense server it talks to, so either can be the newer of the two.
// Neither may break a configuration that worked before the other moved:
//
//   - an OLDER provider tells the server the schema versions it was built
//     against (client.SchemaVersionsHeader, set in Configure) and the server
//     answers in them, so reads compare cleanly with what the user wrote;
//   - a NEWER provider converts what it writes down to the version the server
//     writes (forServer), because the server refuses a document from its
//     future;
//   - the contract a dashboard is checked against at plan time is the
//     SERVER's (checkDashboard), because the one embedded here is frozen at
//     this release and the server's may since have widened.
//
// DEVELOPERS.md has the rules this depends on and the tests that hold them.

// forServer converts a preset built by this provider, at this provider's
// schema version, down to the version the server writes. A server at or above
// this provider's version gets it unchanged.
func forServer(ctx context.Context, c *client.Client, preset []byte) ([]byte, error) {
	versions, err := c.ServerSchemaVersions(ctx)
	if err != nil {
		return nil, err
	}
	mine, _ := schema.CurrentVersion(schema.DashboardPreset)
	theirs, known := versions[string(schema.DashboardPreset)]
	if !known || theirs >= mine {
		return preset, nil
	}
	return schema.MigrateTo(schema.DashboardPreset, preset, theirs)
}

// checkDashboard reports every contract finding on preset as an error,
// judged by the server's contract when it can say, and by the one embedded in
// this provider when it is too old to have the dry-run endpoint.
//
// It runs at plan time (ModifyPlan) rather than in a validator because only
// then is the provider configured: `terraform validate` has no server to ask.
// The server applies the same strictness to a Terraform write (see
// OriginTerraform in kubeapi), so a clean check here is a clean apply.
func checkDashboard(ctx context.Context, c *client.Client, preset []byte, report func(summary, detail string), diags *diag.Diagnostics) {
	toSend, err := forServer(ctx, c, preset)
	if err != nil {
		diags.AddError("Reading the server's schema versions failed", err.Error())
		return
	}
	_, findings, err := c.ValidateDashboard(ctx, toSend)
	if client.IsEndpointMissing(err) {
		embedded(preset, report, diags)
		return
	}
	if err != nil {
		diags.AddError("Checking the dashboard against the server failed", err.Error())
		return
	}
	for _, f := range findings {
		report("Dashboard does not match the dashboard schema", f.String())
	}
}

// embedded checks preset against the contract this provider was built with.
func embedded(preset []byte, report func(summary, detail string), diags *diag.Diagnostics) {
	_, findings, err := checker.Check(preset)
	if err != nil {
		diags.AddError("Invalid dashboard", err.Error())
		return
	}
	for _, f := range findings {
		report("Dashboard does not match the dashboard schema", f)
	}
}
