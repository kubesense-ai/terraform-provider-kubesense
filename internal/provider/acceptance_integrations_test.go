package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	tfstate "github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccIntegrationAndAWSAccount(t *testing.T) {
	api := newFakeAPI(t)

	config := func(name string, rdsCollect bool) string {
		return fmt.Sprintf(`
provider "kubesense" {
  api_url = %q
  api_key = "test-key"
}

resource "kubesense_integration" "jira" {
  integration_provider = "jira"
  name                 = %q
  api_key              = "jira-token"
  config = {
    deployment = "cloud"
    # The trailing slash is trimmed by the fake, as the server normalises it.
    base_url = "https://acme.atlassian.net/"
    username = "bot@acme.com"
  }
}

resource "kubesense_aws_account" "prod" {
  account_id = "123456789012"
  mode       = "credentials"
  secret_id  = "AKIAEXAMPLE"
  secret_key = "s3cret"
  resource_types = {
    ec2 = { collect = true, metric_enabled = true }
    rds = { collect = %t, metric_enabled = false }
  }
}
`, api.URL, name, rdsCollect)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: config("jira", false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("kubesense_integration.jira", "status", "healthy"),
					resource.TestCheckResourceAttr("kubesense_integration.jira", "auth_scheme", "basic"),
					resource.TestCheckResourceAttr("kubesense_aws_account.prod", "id", "kuid-aws-123456789012"),
					resource.TestCheckResourceAttr("kubesense_aws_account.prod", "status", "healthy"),
				),
			},
			{
				// The normalised base_url and the masked secret must not show
				// up as changes.
				Config:   config("jira", false),
				PlanOnly: true,
			},
			{
				Config: config("jira-prod", true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("kubesense_integration.jira", "name", "jira-prod"),
					resource.TestCheckResourceAttr("kubesense_aws_account.prod", "resource_types.rds.collect", "true"),
				),
			},
			{
				ResourceName:            "kubesense_integration.jira",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key", "config"},
			},
			{
				ResourceName:            "kubesense_aws_account.prod",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret_key"},
			},
		},
	})
}

func (f *fakeAPI) registerIntegrations(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/settings/integrations", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		id := f.id("int")
		cfg, _ := in["config"].(map[string]any)
		normaliseConfig(cfg)
		hint := "oken"
		it := map[string]any{
			"integration_id": id, "provider": in["provider"], "integration_type": "native", "name": in["name"],
			"api_key_hint": hint, "auth_scheme": "basic", "config": cfg, "enabled": true, "status": "healthy",
		}
		f.integrations[id] = it
		reply(w, 200, map[string]any{"integration": it, "probe": map[string]any{"connected": true}}, "Integration connected")
	})
	mux.HandleFunc("GET /api/settings/integrations/{id}", func(w http.ResponseWriter, r *http.Request) {
		it, ok := f.integrations[r.PathValue("id")]
		if !ok {
			reply(w, 404, nil, "integration not found")
			return
		}
		reply(w, 200, it, "Integration fetched")
	})
	mux.HandleFunc("PATCH /api/settings/integrations/{id}", func(w http.ResponseWriter, r *http.Request) {
		it, ok := f.integrations[r.PathValue("id")]
		if !ok {
			reply(w, 404, nil, "integration not found")
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		for _, k := range []string{"name", "enabled", "auth_scheme"} {
			if v, ok := in[k]; ok {
				it[k] = v
			}
		}
		if cfg, ok := in["config"].(map[string]any); ok {
			normaliseConfig(cfg)
			it["config"] = cfg
		}
		reply(w, 200, map[string]any{"integration": it}, "Integration updated")
	})
	mux.HandleFunc("DELETE /api/settings/integrations/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.integrations, r.PathValue("id"))
		reply(w, 200, nil, "Integration removed")
	})
}

func normaliseConfig(cfg map[string]any) {
	if u, ok := cfg["base_url"].(string); ok {
		cfg["base_url"] = strings.TrimRight(u, "/")
	}
}

// Listing resource_types for the first time takes control of collection in
// one apply: a type the UI turned on and the configuration does not list is
// switched off, not left running until a second apply.
func TestAccAWSAccountTakesOverResourceTypes(t *testing.T) {
	api := newFakeAPI(t)
	config := func(types string) string {
		return fmt.Sprintf(`
provider "kubesense" {
  api_url = %q
  api_key = "test-key"
}

resource "kubesense_aws_account" "prod" {
  account_id = "123456789012"
  mode       = "credentials"
  secret_id  = "AKIAEXAMPLE"
  secret_key = "s3cret"
%s}
`, api.URL, types)
	}
	enableS3InTheUI := func() {
		api.mu.Lock()
		defer api.mu.Unlock()
		api.domains["kuid-aws-123456789012"].flags["s3"] = map[string]any{
			"resource_type": "s3", "collect": true, "metric_enabled": false, "metric_tier": "core"}
	}
	s3IsOff := func(*tfstate.State) error {
		api.mu.Lock()
		defer api.mu.Unlock()
		if api.domains["kuid-aws-123456789012"].flags["s3"]["collect"] != false {
			return fmt.Errorf("s3 is still collected after taking control of resource_types")
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{Config: config("")},
			{
				PreConfig: enableS3InTheUI,
				Config:    config("  resource_types = {\n    ec2 = { collect = true, metric_enabled = true }\n  }\n"),
				Check:     s3IsOff,
			},
		},
	})
}

type fakeDomain struct {
	row   map[string]any
	creds map[string]string
	flags map[string]map[string]any
}

var fakeSecretKeys = map[string]bool{"secret_key": true, "client_secret": true, "api_key": true, "api_secret": true}

func (f *fakeAPI) registerCloudDomains(mux *http.ServeMux) {
	get := func(w http.ResponseWriter, r *http.Request) (*fakeDomain, bool) {
		d, ok := f.domains[r.PathValue("kuid")]
		if !ok {
			reply(w, 404, nil, "Cloud domain not found")
		}
		return d, ok
	}
	mux.HandleFunc("POST /api/settings/cloud-domains", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Provider    string            `json:"provider"`
			AccountID   string            `json:"account_id"`
			Credentials map[string]string `json:"credentials"`
			Flags       []map[string]any  `json:"flags"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		kuid := "kuid-" + in.Provider + "-" + in.AccountID
		d := &fakeDomain{
			row: map[string]any{"kuid": kuid, "provider": in.Provider, "account_id": in.AccountID,
				"display_name": "Acme prod", "active": true, "status": "healthy"},
			creds: in.Credentials,
			flags: map[string]map[string]any{},
		}
		for _, rt := range []string{"ec2", "rds", "s3"} {
			d.flags[rt] = map[string]any{"resource_type": rt, "collect": false, "metric_enabled": false, "metric_tier": "core"}
		}
		for _, fl := range in.Flags {
			d.flags[fl["resource_type"].(string)] = fl
		}
		f.domains[kuid] = d
		reply(w, 200, map[string]any{"kuid": kuid}, "Cloud domain created")
	})
	mux.HandleFunc("GET /api/settings/cloud-domains/{kuid}", func(w http.ResponseWriter, r *http.Request) {
		if d, ok := get(w, r); ok {
			reply(w, 200, d.row, "Cloud domain fetched")
		}
	})
	mux.HandleFunc("GET /api/settings/cloud-domains/{kuid}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if d, ok := get(w, r); ok {
			out := map[string]string{}
			for k, v := range d.creds {
				if fakeSecretKeys[k] {
					v = "******"
				}
				out[k] = v
			}
			reply(w, 200, out, "Credentials listed")
		}
	})
	mux.HandleFunc("PATCH /api/settings/cloud-domains/{kuid}/credentials", func(w http.ResponseWriter, r *http.Request) {
		if d, ok := get(w, r); ok {
			var in struct {
				Credentials map[string]string `json:"credentials"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			for k, v := range in.Credentials {
				d.creds[k] = v
			}
			reply(w, 200, nil, "Credentials updated")
		}
	})
	mux.HandleFunc("GET /api/settings/cloud-domains/{kuid}/config", func(w http.ResponseWriter, r *http.Request) {
		if d, ok := get(w, r); ok {
			out := []map[string]any{}
			for _, rt := range []string{"ec2", "rds", "s3"} {
				out = append(out, d.flags[rt])
			}
			reply(w, 200, out, "Config listed")
		}
	})
	mux.HandleFunc("PATCH /api/settings/cloud-domains/{kuid}/config", func(w http.ResponseWriter, r *http.Request) {
		if d, ok := get(w, r); ok {
			var in struct {
				Flags []map[string]any `json:"flags"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			for _, fl := range in.Flags {
				d.flags[fl["resource_type"].(string)] = fl
			}
			reply(w, 200, nil, "Config updated")
		}
	})
	for action, active := range map[string]bool{"activate": true, "deactivate": false} {
		mux.HandleFunc("POST /api/settings/cloud-domains/{kuid}/"+action, func(w http.ResponseWriter, r *http.Request) {
			if d, ok := get(w, r); ok {
				d.row["active"] = active
				reply(w, 200, nil, "ok")
			}
		})
	}
	mux.HandleFunc("DELETE /api/settings/cloud-domains/{kuid}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := get(w, r); ok {
			delete(f.domains, r.PathValue("kuid"))
			reply(w, 200, nil, "Cloud domain deleted")
		}
	})
}
