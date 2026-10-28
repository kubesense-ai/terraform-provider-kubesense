package provider

import (
	"context"
	"maps"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/client"
)

func TestReconcile(t *testing.T) {
	prior := map[string]string{"base_url": "https://acme.atlassian.net/", "username": "bot", "deployment": "cloud"}
	applied := map[string]string{"base_url": "https://acme.atlassian.net", "username": "bot", "deployment": "cloud"}

	cases := []struct {
		name   string
		server map[string]string
		want   map[string]string
	}{
		{
			name:   "server normalised the URL at apply: no drift",
			server: map[string]string{"base_url": "https://acme.atlassian.net", "username": "bot", "deployment": "cloud", "mcp": "{}"},
			want:   prior,
		},
		{
			name:   "edited in the UI since: drift",
			server: map[string]string{"base_url": "https://other.atlassian.net", "username": "bot", "deployment": "cloud"},
			want:   map[string]string{"base_url": "https://other.atlassian.net", "username": "bot", "deployment": "cloud"},
		},
		{
			name:   "key removed on the server: dropped so the plan sets it again",
			server: map[string]string{"base_url": "https://acme.atlassian.net", "deployment": "cloud"},
			want:   map[string]string{"base_url": "https://acme.atlassian.net/", "deployment": "cloud"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconcile(prior, tc.server, applied); !maps.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadCredential(t *testing.T) {
	server := map[string]string{"role_arn": "arn:aws:iam::1:role/b", "secret_key": client.MaskedSecret, "mode": "role_access"}
	applied := map[string]string{"role_arn": "arn:aws:iam::1:role/a", "mode": "role_access"}

	cases := []struct {
		name      string
		field     string
		prior     types.String
		importing bool
		want      types.String
	}{
		{"masked secret keeps config", "secret_key", types.StringValue("s3cret"), false, types.StringValue("s3cret")},
		{"unchanged value keeps config", "mode", types.StringValue("role_access"), false, types.StringValue("role_access")},
		{"changed since apply is drift", "role_arn", types.StringValue("arn:aws:iam::1:role/a"), false, types.StringValue("arn:aws:iam::1:role/b")},
		{"never configured stays unset", "mode", types.StringNull(), false, types.StringNull()},
		{"missing on server is unset", "secret_id", types.StringValue("AKIA"), false, types.StringNull()},
		{"import adopts readable values", "mode", types.StringNull(), true, types.StringValue("role_access")},
		{"import leaves secrets unset", "secret_key", types.StringNull(), true, types.StringNull()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readCredential(tc.prior, server, applied, tc.field, tc.importing); !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadFlags(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	server := []client.CloudFlag{
		{ResourceType: "ec2", Collect: true, MetricEnabled: true, MetricTier: "core"},
		{ResourceType: "rds", Collect: true, MetricTier: "core"}, // enabled in the UI
		{ResourceType: "s3", MetricTier: "core"},                 // seeded, off
	}
	prior := map[string]cloudFlagModel{
		"ec2": {Collect: types.BoolValue(true), MetricEnabled: types.BoolValue(true), MetricTier: types.StringNull(), MetricExtras: types.SetNull(types.StringType)},
	}

	got := readFlags(ctx, prior, server, false, &diags)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if _, ok := got["s3"]; ok {
		t.Error("a type that is off and unlisted should not appear")
	}
	if _, ok := got["rds"]; !ok {
		t.Error("a type enabled outside Terraform should appear, as drift")
	}
	if !got["ec2"].MetricTier.IsNull() {
		t.Errorf("unset metric_tier should stay unset while the server holds the default, got %v", got["ec2"].MetricTier)
	}

	if readFlags(ctx, nil, server, false, &diags) != nil {
		t.Error("unmanaged resource_types should stay unset")
	}
	if imported := readFlags(ctx, nil, server, true, &diags); len(imported) != 2 {
		t.Errorf("import should adopt the 2 enabled types, got %v", imported)
	}
}

func TestFlagsToAPIDisablesDroppedTypes(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	on := cloudFlagModel{Collect: types.BoolValue(true), MetricEnabled: types.BoolValue(false), MetricTier: types.StringNull(), MetricExtras: types.SetNull(types.StringType)}

	got := flagsToAPI(ctx, map[string]cloudFlagModel{"ec2": on}, map[string]cloudFlagModel{"ec2": on, "rds": on}, &diags)
	want := []client.CloudFlag{
		{ResourceType: "ec2", Collect: true, MetricTier: "core"},
		{ResourceType: "rds", MetricTier: "core"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].ResourceType != want[i].ResourceType || got[i].Collect != want[i].Collect ||
			got[i].MetricEnabled != want[i].MetricEnabled || got[i].MetricTier != want[i].MetricTier {
			t.Errorf("flag %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestIntegrationPatchSendsOnlyChanges(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics
	cfg, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"deployment": "cloud"})
	state := integrationModel{
		Name: types.StringValue("jira"), APIKey: types.StringValue("k1"), AuthScheme: types.StringValue("basic"),
		AuthHeader: types.StringNull(), Config: cfg, Enabled: types.BoolValue(true),
	}

	renamed := state
	renamed.Name = types.StringValue("jira-prod")
	in := integrationPatch(ctx, renamed, state, &diags)
	if in.Name == nil || *in.Name != "jira-prod" || in.APIKey != nil || in.Config != nil || in.AuthScheme != nil || in.Enabled != nil {
		t.Errorf("a rename should send only the name, got %+v", in)
	}

	keyDropped := state
	keyDropped.APIKey = types.StringNull()
	if in := integrationPatch(ctx, keyDropped, state, &diags); in.APIKey != nil {
		t.Error("removing api_key from the configuration must not clear the stored key")
	}
}
