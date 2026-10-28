package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	kschema "github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema"
)

// Version skew: users upgrade the provider and the server independently, and
// neither upgrade may break a configuration that worked before it. Each test
// here applies a configuration, changes the server's age underneath it, and
// asserts what the user sees. compat.go explains the mechanisms.

// registerSchemaVersions mounts the two endpoints a current server has and an
// old one does not. They answer only while f.current is set.
func (f *fakeAPI) registerSchemaVersions(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/schema/versions", func(w http.ResponseWriter, r *http.Request) {
		if !f.current {
			reply(w, 404, nil, "Endpoint doesn't exists")
			return
		}
		versions := map[string]int{}
		for format, n := range kschema.CurrentVersions() {
			versions[string(format)] = n
		}
		reply(w, 200, map[string]any{"schema_versions": versions, "oldest": kschema.OldestVersion}, "ok")
	})
	mux.HandleFunc("POST /api/dashboard/validate", func(w http.ResponseWriter, r *http.Request) {
		if !f.current {
			reply(w, 404, nil, "Endpoint doesn't exists")
			return
		}
		body, _ := io.ReadAll(r.Body)
		if f.validate != nil {
			valid, findings := f.validate(body)
			reply(w, 200, map[string]any{"valid": valid, "findings": findings}, "ok")
			return
		}
		// Without an override the fake judges by the contract it was built
		// with, as a server of the same release would.
		_, findings, err := checker.Check(body)
		if err != nil {
			reply(w, 400, nil, err.Error())
			return
		}
		out := make([]map[string]string, 0, len(findings))
		for _, f := range findings {
			out = append(out, map[string]string{"message": f, "rule": "shape"})
		}
		reply(w, 200, map[string]any{"valid": len(out) == 0, "findings": out}, "ok")
	})
}

func dashboardJSONConfig(url, preset string) string {
	return fmt.Sprintf(`
provider "kubesense" {
  api_url = %q
  api_key = "test-key"
}

resource "kubesense_dashboard_json" "checkout" {
  name           = "Checkout"
  dashboard_json = <<EOT
%s
EOT
}
`, url, preset)
}

// The server is upgraded under an existing configuration: the plan after the
// upgrade must be empty. This is the test the release process runs for every
// supported server version; here the "upgrade" is the fake gaining the
// endpoints this release added.
func TestAccServerUpgradeLeavesThePlanEmpty(t *testing.T) {
	api := newFakeAPI(t)
	config := dashboardJSONConfig(api.URL, minimalPreset)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig: func() { api.current = true },
				Config:    config,
				PlanOnly:  true,
			},
		},
	})

	want := kschema.CurrentVersions().Header()
	if api.versionsHeader != want {
		t.Errorf("provider sent %s %q, want %q", "KubeSense-Schema-Versions", api.versionsHeader, want)
	}
}

// The server has widened its contract since this provider was built: it
// accepts a preset the provider's embedded contract refuses (a new panel
// type, a new option). A pinned provider must not stand in the way.
func TestAccServerWithAWiderContractIsBelieved(t *testing.T) {
	api := newFakeAPI(t)
	api.current = true
	api.validate = func([]byte) (bool, []map[string]string) { return true, nil }

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: dashboardJSONConfig(api.URL, badVariablePreset)},
		},
	})
}

// The server's contract refuses what the configuration says: the plan fails
// with the server's own finding, before anything is written.
func TestAccServerRefusalFailsThePlan(t *testing.T) {
	api := newFakeAPI(t)
	api.current = true
	api.validate = func([]byte) (bool, []map[string]string) {
		return false, []map[string]string{{"path": "/panels/0/panelType", "message": "not a panel type this server knows", "rule": "shape"}}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config:      dashboardJSONConfig(api.URL, minimalPreset),
				ExpectError: regexp.MustCompile(`not a panel type this server knows`),
			},
		},
	})
	if len(api.dashboards) != 0 {
		t.Errorf("%d dashboard(s) written despite the refusal", len(api.dashboards))
	}
}

// A server too old for the dry run: the provider falls back to the contract
// it embeds, exactly as every provider before this one behaved.
func TestAccOldServerIsCheckedByTheEmbeddedContract(t *testing.T) {
	api := newFakeAPI(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config:      dashboardJSONConfig(api.URL, badVariablePreset),
				ExpectError: regexp.MustCompile(`does not match the dashboard schema`),
			},
		},
	})
}

// The provider is newer than the server — or the server was rolled back — and
// the server has no endpoint for a resource in state. The plan must FAIL and
// say why. Before this, Read took the 404 for "deleted outside Terraform",
// dropped the resource from state, and planned to create it again.
func TestAccMissingEndpointFailsInsteadOfForgettingTheResource(t *testing.T) {
	api := newFakeAPI(t)
	config := dashboardJSONConfig(api.URL, minimalPreset)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:   func() { api.missing = []string{"/api/dashboard"} },
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`older\s+than\s+this\s+provider\s+version\s+supports`),
			},
			{
				// Restored, so the framework's destroy can clean up.
				PreConfig: func() { api.missing = nil },
				Config:    config,
				PlanOnly:  true,
			},
		},
	})
}

// minimalPresetIsCurrent guards the assumption the tests above rest on: the
// preset they write is already at this build's version, so "plan is empty"
// is about skew and not about a migration the fake happens to skip.
func TestMinimalPresetIsCurrent(t *testing.T) {
	migrated, err := kschema.Migrate(kschema.DashboardPreset, []byte(minimalPreset))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(migrated, &doc); err != nil {
		t.Fatal(err)
	}
	if current, _ := kschema.CurrentVersion(kschema.DashboardPreset); doc["schema_version"] != float64(current) {
		t.Fatalf("schema_version = %v after migration", doc["schema_version"])
	}
}
