package provider

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestProviderSchemas has the framework build and check every schema, which is
// where mistakes like a Default on a non-Computed attribute surface.
func TestProviderSchemas(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("%s: %s", d.Summary, d.Detail)
	}
	for _, name := range []string{
		"kubesense_dashboard", "kubesense_dashboard_json", "kubesense_dashboard_list", "kubesense_integration",
		"kubesense_alert_rule",
		"kubesense_aws_account", "kubesense_gcp_project", "kubesense_azure_subscription",
		"kubesense_mongodbatlas_org", "kubesense_confluent_org", "kubesense_kong_control_plane",
	} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %s is not registered", name)
		}
	}
	for _, name := range []string{"kubesense_dashboard", "kubesense_integration"} {
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("data source %s is not registered", name)
		}
	}
}

func TestDiffIDs(t *testing.T) {
	add, remove := diffIDs([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	if !slices.Equal(add, []string{"d"}) || !slices.Equal(remove, []string{"a"}) {
		t.Errorf("add=%v remove=%v, want add=[d] remove=[a]", add, remove)
	}
	if add, remove := diffIDs([]string{"a"}, []string{"a"}); add != nil || remove != nil {
		t.Errorf("no change should be a no-op, got add=%v remove=%v", add, remove)
	}
}
