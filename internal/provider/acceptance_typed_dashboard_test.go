package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/terraform"
)

// typedPreset exercises nested blocks, repeated blocks, a variable union and
// members that can only travel as JSON.
const typedPreset = `{
  "gridLayout": [
    { "i": "p1", "x": 0, "y": 0, "w": 12, "h": 8 },
    { "i": "p2", "x": 0, "y": 8, "w": 6, "h": 6 }
  ],
  "panels": [
    { "name": "CPU by pod", "panelType": "timeSeries",
      "queries": [
        { "selectedMode": "metrics", "label": "A", "queryMode": "code",
          "promql": "sum(rate(container_cpu_usage_seconds_total[5m])) by (pod)" }
      ] },
    { "name": "Requests", "panelType": "bar",
      "queries": [
        { "selectedMode": "traces", "label": "A", "aggregation": { "function": "row_count" }, "filters": {} }
      ],
      "config": { "eventOverlay": { "mode": "inherit" } } }
  ],
  "variables": [
    { "name": "cluster", "description": "Cluster to show",
      "meta": { "variableType": "textbox", "defaultValue": "prod", "value": "prod" } }
  ],
  "tabs": []
}`

// The configuration is what kubeapi's Export to Terraform writes, so this
// proves the export, the provider's conversion and the server round trip
// agree: apply, then a plan with no changes, then an import that matches.
func TestAccTypedDashboardFromExport(t *testing.T) {
	api := newFakeAPI(t)

	export, err := terraform.ExportDashboard(terraform.Dashboard{Name: "Checkout", Preset: []byte(typedPreset)})
	if err != nil {
		t.Fatal(err)
	}
	provider := fmt.Sprintf("provider \"kubesense\" {\n  api_url = %q\n  api_key = \"test-key\"\n}\n\n", api.URL)
	config := provider + export.HCL
	renamed := provider + strings.Replace(export.HCL, `"CPU by pod"`, `"CPU by namespace"`, 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("kubesense_dashboard.checkout", "id"),
					resource.TestCheckResourceAttr("kubesense_dashboard.checkout", "panel.#", "2"),
					resource.TestCheckResourceAttr("kubesense_dashboard.checkout", "panel.0.panel_type", "timeSeries"),
					resource.TestCheckResourceAttr("kubesense_dashboard.checkout", "variable.0.meta.variable_type", "textbox"),
				),
			},
			{Config: config, PlanOnly: true},
			{
				ResourceName:      "kubesense_dashboard.checkout",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: renamed,
				Check:  resource.TestCheckResourceAttr("kubesense_dashboard.checkout", "panel.0.name", "CPU by namespace"),
			},
		},
	})
}

// A *_json member written by hand, with spaces, rather than with jsonencode():
// the server reads it back compact, and that must not show as a change.
func TestAccTypedDashboardHandWrittenJSON(t *testing.T) {
	api := newFakeAPI(t)
	export, err := terraform.ExportDashboard(terraform.Dashboard{Name: "Checkout", Preset: []byte(typedPreset)})
	if err != nil {
		t.Fatal(err)
	}
	aggregation := regexp.MustCompile(`aggregation_json = jsonencode\(\{[^}]*\}\)`)
	if !aggregation.MatchString(export.HCL) {
		t.Fatalf("export has no aggregation_json to rewrite:\n%s", export.HCL)
	}
	hcl := aggregation.ReplaceAllLiteralString(export.HCL, `aggregation_json = "{\"function\": \"row_count\"}"`)
	config := fmt.Sprintf("provider \"kubesense\" {\n  api_url = %q\n  api_key = \"test-key\"\n}\n\n", api.URL) + hcl

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: config},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestTypedDashboardExportLooksLikeTerraform(t *testing.T) {
	export, err := terraform.ExportDashboard(terraform.Dashboard{ID: "d1", Name: "Checkout", Preset: []byte(typedPreset)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`resource "kubesense_dashboard" "checkout" {`,
		"  panel {",
		`    panel_type = "timeSeries"`,
		"    query {",
		"  variable {",
		"    meta {",
		"aggregation_json = jsonencode({",
	} {
		if !strings.Contains(export.HCL, want) {
			t.Errorf("export lacks %q:\n%s", want, export.HCL)
		}
	}
}
