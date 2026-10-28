package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	tfstate "github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/terraform"
)

const alertProvider = `provider "kubesense" {
  api_url = %q
  api_key = "test-key"
}
`

// A hand-written rule leaves out what the server defaults (enabled,
// sample_limit, a query's visible). The plan after apply must still be empty,
// and a change to a member the configuration sets must apply.
func TestAccAlertRuleHandWritten(t *testing.T) {
	api := newFakeAPI(t)
	config := func(threshold int) string {
		return fmt.Sprintf(alertProvider+`
resource "kubesense_alert_rule" "cpu" {
  name                       = "cpu high"
  severity                   = "critical"
  metric_query_label         = "A"
  threshold_operator         = "greater_than"
  threshold_value            = %d
  time_window                = "5m"
  evaluation_interval        = "1m"
  notification_channel_names = ["slack-oncall"]

  query_config {
    label         = "A"
    query_mode    = "code"
    selected_mode = "metrics"
    promql        = "avg(rate(container_cpu_usage_seconds_total[5m]))"
  }
}
`, api.URL, threshold)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: config(80),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("kubesense_alert_rule.cpu", "id"),
					resource.TestCheckResourceAttr("kubesense_alert_rule.cpu", "threshold_value", "80"),
					resource.TestCheckResourceAttr("kubesense_alert_rule.cpu", "notification_channel_names.0", "slack-oncall"),
				),
			},
			{Config: config(80), PlanOnly: true},
			{
				Config: config(90),
				Check:  resource.TestCheckResourceAttr("kubesense_alert_rule.cpu", "threshold_value", "90"),
			},
		},
	})
}

// The configuration Export to Terraform writes, from the definition the
// server reads back, must apply, plan with no changes, and import to the same
// state.
func TestAccAlertRuleFromExport(t *testing.T) {
	api := newFakeAPI(t)
	def := map[string]any{}
	if err := json.Unmarshal([]byte(`{
	  "name": "ingestion band", "severity": "warning", "enabled": true, "sample_limit": 5,
	  "metric_query_label": "A", "threshold_operator": "greater_than", "threshold_value": 250000,
	  "time_window": "5m", "evaluation_interval": "1m", "labels": {"team": "platform"},
	  "conditions": [
	    {"id": "c1", "query": "A", "operator": "greater_than", "value": 250000},
	    {"id": "c2", "query": "A", "operator": "less_than", "value": 5000}
	  ],
	  "condition_expression": {"op": "or", "conditions": ["c1", "c2"]},
	  "query_config": [{
	    "label": "A", "queryMode": "builder", "selectedMode": "metrics", "visible": false,
	    "selectedMetric": "container_cpu_usage_seconds_total",
	    "functions": [{"name": "avg_over_time", "type": "rollup", "arguments": [{"arg_name": "over", "arg_value": "5m"}]}]
	  }],
	  "notification_channel_names": ["slack-oncall"]
	}`), &def); err != nil {
		t.Fatal(err)
	}
	def = fakeAlertDefaults(def)
	export, err := terraform.ExportAlertRule("", jsonNumbers(t, def))
	if err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(alertProvider, api.URL) + "\n" + export.HCL

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("kubesense_alert_rule.ingestion_band", "condition.#", "2"),
					resource.TestCheckResourceAttr("kubesense_alert_rule.ingestion_band", "query_config.0.function.0.name", "avg_over_time"),
				),
			},
			{Config: config, PlanOnly: true},
			{
				ResourceName:      "kubesense_alert_rule.ingestion_band",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Applying an export of an existing rule must edit that rule, not create a
// copy: the import block the export carries adopts it on the first apply, and
// an edit afterwards updates it in place.
func TestAccAlertRuleExportAdoptsTheOriginal(t *testing.T) {
	api := newFakeAPI(t)
	def := fakeAlertDefaults(map[string]any{
		"name": "disk full", "severity": "critical", "metric_query_label": "A",
		"threshold_operator": "greater_than", "threshold_value": 90,
		"time_window": "5m", "evaluation_interval": "1m",
		"query_config":               []any{map[string]any{"label": "A", "queryMode": "code", "selectedMode": "metrics", "promql": "disk_used_percent"}},
		"notification_channel_names": []any{"slack-oncall"},
	})
	api.alertRules["orig"] = def
	config := func(threshold int) string {
		edited := map[string]any{}
		for k, v := range def {
			edited[k] = v
		}
		edited["threshold_value"] = threshold
		export, err := terraform.ExportAlertRule("orig", jsonNumbers(t, edited))
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf(alertProvider, api.URL) + "\n" + export.HCL
	}
	onlyTheOriginal := func(threshold string) resource.TestCheckFunc {
		return func(*tfstate.State) error {
			api.mu.Lock()
			defer api.mu.Unlock()
			if len(api.alertRules) != 1 {
				return fmt.Errorf("%d rules on the server, want only the original", len(api.alertRules))
			}
			if got := fmt.Sprint(api.alertRules["orig"]["threshold_value"]); got != threshold {
				return fmt.Errorf("original threshold_value = %s, want %s", got, threshold)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: config(90),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("kubesense_alert_rule.disk_full", "id", "orig"),
					onlyTheOriginal("90"),
				),
			},
			{
				Config: config(95),
				Check:  onlyTheOriginal("95"),
			},
		},
	})
}

func jsonNumbers(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(v)
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// fakeAlertDefaults fills what kubeapi fills when it stores a rule.
func fakeAlertDefaults(def map[string]any) map[string]any {
	if _, ok := def["enabled"]; !ok {
		def["enabled"] = true
	}
	if _, ok := def["sample_limit"]; !ok {
		def["sample_limit"] = 5
	}
	if qs, ok := def["query_config"].([]any); ok {
		for _, q := range qs {
			if m, ok := q.(map[string]any); ok {
				if _, ok := m["visible"]; !ok {
					m["visible"] = false
				}
			}
		}
	}
	return def
}

func (f *fakeAPI) registerAlertRules(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/alerts/rules/definition", func(w http.ResponseWriter, r *http.Request) {
		var def map[string]any
		_ = json.NewDecoder(r.Body).Decode(&def)
		if def["name"] == nil || def["query_config"] == nil {
			reply(w, 400, nil, "name and query_config are required")
			return
		}
		id := f.id("rule")
		f.alertRules[id] = fakeAlertDefaults(def)
		reply(w, 201, map[string]any{"id": id}, "Alert rule created successfully")
	})
	mux.HandleFunc("GET /api/alerts/rules/{id}/definition", func(w http.ResponseWriter, r *http.Request) {
		def, ok := f.alertRules[r.PathValue("id")]
		if !ok {
			reply(w, 404, nil, "Alert rule not found")
			return
		}
		reply(w, 200, def, "Alert rule definition")
	})
	mux.HandleFunc("PUT /api/alerts/rules/{id}/definition", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := f.alertRules[r.PathValue("id")]; !ok {
			reply(w, 404, nil, "Alert rule not found")
			return
		}
		var def map[string]any
		_ = json.NewDecoder(r.Body).Decode(&def)
		f.alertRules[r.PathValue("id")] = fakeAlertDefaults(def)
		reply(w, 200, nil, "Alert rule updated successfully")
	})
	mux.HandleFunc("DELETE /api/alerts/rules/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.alertRules, r.PathValue("id"))
		reply(w, 200, nil, "Alert rule deleted")
	})
}
