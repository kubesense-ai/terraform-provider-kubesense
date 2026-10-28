package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// jsonStringType is the type of the typed resources' *_json and extra_json
// attributes: a string whose equality is the JSON it holds. The server reads
// those members back as compact JSON, so a value written by hand with spaces
// (rather than with jsonencode()) would otherwise show a change on every plan.
type jsonStringType struct{ basetypes.StringType }

var _ basetypes.StringTypable = jsonStringType{}

func (t jsonStringType) Equal(o attr.Type) bool {
	_, ok := o.(jsonStringType)
	return ok
}

func (t jsonStringType) String() string { return "jsonStringType" }

func (t jsonStringType) ValueType(context.Context) attr.Value { return jsonStringValue{} }

func (t jsonStringType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return jsonStringValue{StringValue: in}, nil
}

func (t jsonStringType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	s, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return jsonStringValue{StringValue: s}, nil
}

type jsonStringValue struct{ basetypes.StringValue }

var _ basetypes.StringValuableWithSemanticEquals = jsonStringValue{}

func (v jsonStringValue) Equal(o attr.Value) bool {
	other, ok := o.(jsonStringValue)
	return ok && v.StringValue.Equal(other.StringValue)
}

func (v jsonStringValue) Type(context.Context) attr.Type { return jsonStringType{} }

// StringSemanticEquals keeps the configured string in state when the server
// returns the same JSON formatted differently.
func (v jsonStringValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	other, ok := newValuable.(jsonStringValue)
	if !ok {
		return false, nil
	}
	return sameJSON(v.ValueString(), other.ValueString()), nil
}

// sameJSON reports whether two strings hold the same JSON value. Anything
// that is not JSON is compared as written.
func sameJSON(a, b string) bool {
	if a == b {
		return true
	}
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
