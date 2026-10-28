package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// minimalPreset is schema/fixtures/dashboard/pass/minimal.json.
const minimalPreset = `{
  "gridLayout": [{ "i": "p1", "x": 0, "y": 0, "w": 12, "h": 8 }],
  "panels": [
    { "name": "CPU by pod", "panelType": "timeSeries",
      "queries": [
        { "selectedMode": "metrics", "label": "A", "queryMode": "code",
          "promql": "sum(rate(container_cpu_usage_seconds_total[5m])) by (pod)" }
      ] }
  ]
}`

// The same preset, compact, with keys in another order.
const minimalPresetReordered = `{"panels":[{"queries":[{"promql":"sum(rate(container_cpu_usage_seconds_total[5m])) by (pod)","queryMode":"code","label":"A","selectedMode":"metrics"}],"panelType":"timeSeries","name":"CPU by pod"}],"gridLayout":[{"h":8,"w":12,"y":0,"x":0,"i":"p1"}]}`

// badVariablePreset is schema/fixtures/dashboard/fail/bad-variable-name.json.
const badVariablePreset = `{
  "gridLayout": [],
  "panels": [],
  "variables": [
    { "name": "9-bad name", "description": "starts with a digit and has a space",
      "meta": { "variableType": "textbox", "defaultValue": "", "value": "" } }
  ]
}`

// rumVariablePreset is schema/fixtures/dashboard/pass/rum-variables.json.
const rumVariablePreset = `{
  "gridLayout": [],
  "panels": [],
  "variables": [
    { "name": "app", "description": "RUM application",
      "meta": { "variableType": "rum", "rumEventType": "view", "fieldMeta": { "field": "application_id", "type": "string", "is_attribute": false } } }
  ]
}`

// rumVariableNoEventTypePreset is schema/fixtures/dashboard/fail/rum-variable-without-event-type.json.
const rumVariableNoEventTypePreset = `{
  "gridLayout": [],
  "panels": [],
  "variables": [
    { "name": "app", "description": "",
      "meta": { "variableType": "rum", "fieldMeta": { "field": "application_id", "type": "string", "is_attribute": false } } }
  ]
}`

func TestNormalisePresetIgnoresFormattingAndAddsTabs(t *testing.T) {
	a, err := normalisePreset(minimalPreset)
	if err != nil {
		t.Fatal(err)
	}
	b, err := normalisePreset(minimalPresetReordered)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Errorf("formatting changed the normalised preset:\n%s\n%s", a, b)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(a, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["tabs"]) != "[]" {
		t.Errorf("tabs = %s, want [] so an update never trips the stale-tabs guard", doc["tabs"])
	}
}

func TestNormalisePresetKeepsExistingTabs(t *testing.T) {
	in := `{"gridLayout":[],"panels":[],"tabs":[{"id":"t1","title":"One","panels":[],"gridLayout":[],"subGrids":[],"subGridLayout":[]}]}`
	out, err := normalisePreset(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"id":"t1"`) {
		t.Errorf("tabs were dropped: %s", out)
	}
}

func TestNormalisePresetRejectsNonObjects(t *testing.T) {
	for _, in := range []string{`not json`, `[1,2]`} {
		if _, err := normalisePreset(in); err == nil {
			t.Errorf("normalisePreset(%q) succeeded, want an error", in)
		}
	}
}

func TestPresetSemanticEquals(t *testing.T) {
	ctx := context.Background()
	prior := newPresetValue(minimalPreset)

	cases := []struct {
		name string
		next string
		want bool
	}{
		{"reformatted", minimalPresetReordered, true},
		{"panel renamed", strings.Replace(minimalPreset, "CPU by pod", "Memory by pod", 1), false},
		{"not JSON", "{", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := prior.StringSemanticEquals(ctx, newPresetValue(tc.next))
			if diags.HasError() {
				t.Fatal(diags)
			}
			if got != tc.want {
				t.Errorf("StringSemanticEquals = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPresetValidator(t *testing.T) {
	cases := []struct {
		name      string
		value     types.String
		wantError string
	}{
		{"valid", types.StringValue(minimalPreset), ""},
		{"unknown is skipped", types.StringUnknown(), ""},
		// Contract findings are not the validator's to judge: they are checked
		// at plan time against the server's contract (compat_test.go).
		{"contract finding is left to the plan", types.StringValue(badVariablePreset), ""},
		{"rum variable", types.StringValue(rumVariablePreset), ""},
		{"rum variable without event type is left to the plan", types.StringValue(rumVariableNoEventTypePreset), ""},
		{"not JSON", types.StringValue(`{"panels":`), "Invalid dashboard JSON"},
		{"not an object", types.StringValue(`[1, 2]`), "Invalid dashboard JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{Path: path.Root("dashboard_json"), ConfigValue: tc.value}
			var resp validator.StringResponse
			presetValidator{}.ValidateString(context.Background(), req, &resp)

			if tc.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("want an error containing %q", tc.wantError)
			}
			if got := resp.Diagnostics.Errors()[0].Summary(); !strings.Contains(got, tc.wantError) {
				t.Errorf("summary = %q, want it to contain %q", got, tc.wantError)
			}
		})
	}
}
