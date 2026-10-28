package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema"
)

// presetChecker is the provider's only seam onto kubeapi's schema package.
// Keeping it to one interface means the provider can move to its own
// repository by swapping this implementation, not by touching the resources.
type presetChecker interface {
	// Check migrates raw to the current schema version and returns the
	// migrated preset with every contract finding. err is reserved for input
	// that cannot be checked at all: not JSON, or from a future version.
	Check(raw []byte) (migrated []byte, findings []string, err error)
}

type schemaChecker struct{}

func (schemaChecker) Check(raw []byte) ([]byte, []string, error) {
	check, err := schema.CheckDashboardPreset(raw)
	if err != nil {
		return nil, nil, err
	}
	findings := make([]string, 0, len(check.Findings))
	for _, f := range check.Findings {
		findings = append(findings, f.Error())
	}
	// check.ContractErr means the contract itself could not be applied. The
	// server stores the preset anyway in that case, so it is not the user's
	// problem and is not reported.
	return check.Migrated, findings, nil
}

var checker presetChecker = schemaChecker{}

// normalisePreset turns a preset into the exact bytes the provider sends and
// compares: migrated by the server's own code, with a tabs key, and encoded
// with sorted keys and no whitespace.
//
// Sending the migrated form means the server stores what it was sent, so the
// next read matches. The tabs key is required on update: a preset without one
// over a tabbed dashboard is refused with a 409, because it would delete the
// tabs.
func normalisePreset(raw string) ([]byte, error) {
	migrated, _, err := checker.Check([]byte(raw))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(migrated))
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("dashboard JSON must be a JSON object: %w", err)
	}
	if _, ok := doc["tabs"]; !ok {
		doc["tabs"] = []any{}
	}
	return json.Marshal(doc)
}

// presetType is dashboard_json's type: a string whose equality is the
// normalised preset's, so key order, whitespace and the server's migration
// never show up as a diff.
type presetType struct{ basetypes.StringType }

var _ basetypes.StringTypable = presetType{}

func (t presetType) Equal(o attr.Type) bool {
	_, ok := o.(presetType)
	return ok
}

func (t presetType) String() string { return "presetType" }

func (t presetType) ValueType(context.Context) attr.Value { return presetValue{} }

func (t presetType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return presetValue{StringValue: in}, nil
}

func (t presetType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	s, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return presetValue{StringValue: s}, nil
}

type presetValue struct{ basetypes.StringValue }

var _ basetypes.StringValuableWithSemanticEquals = presetValue{}

func newPresetValue(s string) presetValue {
	return presetValue{StringValue: basetypes.NewStringValue(s)}
}

func (v presetValue) Equal(o attr.Value) bool {
	other, ok := o.(presetValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

func (v presetValue) Type(context.Context) attr.Type { return presetType{} }

// StringSemanticEquals keeps the configured string in state when the server
// returns the same preset in a different form.
func (v presetValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	other, ok := newValuable.(presetValue)
	if !ok {
		return false, nil
	}
	a, errA := normalisePreset(v.ValueString())
	b, errB := normalisePreset(other.ValueString())
	if errA != nil || errB != nil {
		return false, nil
	}
	return bytes.Equal(a, b), nil
}

// presetValidator refuses dashboard_json that is not a JSON object, the one
// thing that can be judged without a server.
//
// The contract itself is checked in ModifyPlan, against the server's own (see
// checkDashboard). Checking it here, against the contract embedded in this
// provider, would refuse what a newer server has since started accepting —
// a new panel type, say — and pinning the provider would then block users
// from features their server already has.
type presetValidator struct{}

func (presetValidator) Description(context.Context) string {
	return "must be a JSON object"
}

func (v presetValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (presetValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(req.ConfigValue.ValueString()), &doc); err != nil {
		var syntax *json.SyntaxError
		detail := err.Error()
		if errors.As(err, &syntax) {
			detail = fmt.Sprintf("%s (at byte %d)", detail, syntax.Offset)
		}
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid dashboard JSON", detail)
	}
}
