package provider

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/terraform"
)

// The typed dashboard's schema, and its values, come from one shape that
// kubeapi's schema/terraform package derives from the dashboard contract. The
// functions here only translate that shape into the plugin framework's schema
// types and tftypes values; they know nothing about dashboards.

// shapeSchema returns a block's attributes and nested blocks.
func shapeSchema(n *terraform.Node) (map[string]schema.Attribute, map[string]schema.Block) {
	attrs := map[string]schema.Attribute{}
	blocks := map[string]schema.Block{}
	for _, f := range n.Fields {
		desc := f.Description
		switch f.Kind {
		case terraform.KindString:
			attrs[f.Name] = schema.StringAttribute{Optional: true, Description: desc}
		case terraform.KindNumber:
			attrs[f.Name] = schema.NumberAttribute{Optional: true, Description: desc}
		case terraform.KindBool:
			attrs[f.Name] = schema.BoolAttribute{Optional: true, Description: desc}
		case terraform.KindList:
			attrs[f.Name] = schema.ListAttribute{Optional: true, Description: desc, ElementType: scalarType(f.Elem)}
		case terraform.KindMap:
			attrs[f.Name] = schema.MapAttribute{Optional: true, Description: desc, ElementType: types.StringType}
		case terraform.KindJSON:
			if desc == "" {
				desc = "Set with jsonencode()."
			} else {
				desc += " Set with jsonencode()."
			}
			attrs[f.Name] = schema.StringAttribute{Optional: true, Description: desc, CustomType: jsonStringType{}}
		case terraform.KindBlock:
			a, b := shapeSchema(f)
			blocks[f.Name] = schema.SingleNestedBlock{Description: desc, Attributes: a, Blocks: b}
		case terraform.KindBlocks:
			a, b := shapeSchema(f)
			blocks[f.Name] = schema.ListNestedBlock{Description: desc,
				NestedObject: schema.NestedBlockObject{Attributes: a, Blocks: b}}
		}
	}
	return attrs, blocks
}

func scalarType(k terraform.Kind) attr.Type {
	switch k {
	case terraform.KindNumber:
		return types.NumberType
	case terraform.KindBool:
		return types.BoolType
	}
	return types.StringType
}

// valueToTree reads a block's value into the tree schema/terraform converts:
// scalars, []any and map[string]any, keyed by Terraform name. Members of the
// object that are not in the shape (the resource's own name, id, ...) are
// skipped. A value not yet known is an error: a dashboard cannot be written
// from one.
func valueToTree(n *terraform.Node, v tftypes.Value) (map[string]any, error) {
	if v.IsNull() {
		return nil, nil
	}
	if !v.IsKnown() {
		return nil, fmt.Errorf("value is not known until apply")
	}
	var obj map[string]tftypes.Value
	if err := v.As(&obj); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, f := range n.Fields {
		fv, ok := obj[f.Name]
		if !ok || fv.IsNull() {
			continue
		}
		if !fv.IsFullyKnown() {
			return nil, fmt.Errorf("%s is not known until apply", f.Name)
		}
		tv, err := fieldToTree(f, fv)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		out[f.Name] = tv
	}
	return out, nil
}

func fieldToTree(f *terraform.Node, v tftypes.Value) (any, error) {
	switch f.Kind {
	case terraform.KindString, terraform.KindJSON, terraform.KindNumber, terraform.KindBool:
		return scalarToTree(v)
	case terraform.KindList:
		var elems []tftypes.Value
		if err := v.As(&elems); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(elems))
		for _, e := range elems {
			s, err := scalarToTree(e)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	case terraform.KindMap:
		var m map[string]tftypes.Value
		if err := v.As(&m); err != nil {
			return nil, err
		}
		out := make(map[string]any, len(m))
		for k, e := range m {
			s, err := scalarToTree(e)
			if err != nil {
				return nil, err
			}
			out[k] = s
		}
		return out, nil
	case terraform.KindBlock:
		return valueToTree(f, v)
	case terraform.KindBlocks:
		var elems []tftypes.Value
		if err := v.As(&elems); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(elems))
		for _, e := range elems {
			m, err := valueToTree(f, e)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown kind %d", f.Kind)
}

func scalarToTree(v tftypes.Value) (any, error) {
	switch {
	case v.Type().Is(tftypes.String):
		var s string
		err := v.As(&s)
		return s, err
	case v.Type().Is(tftypes.Bool):
		var b bool
		err := v.As(&b)
		return b, err
	case v.Type().Is(tftypes.Number):
		var f big.Float
		if err := v.As(&f); err != nil {
			return nil, err
		}
		return json.Number(f.Text('g', -1)), nil
	}
	return nil, fmt.Errorf("unexpected type %s", v.Type())
}

// treeToValue builds a block's value of type t from the tree, setting every
// attribute the shape does not know from extra (the resource's id, name, ...).
// Absent members are null; an absent repeated block is an empty list, which is
// how Terraform itself represents "no blocks".
func treeToValue(t tftypes.Type, n *terraform.Node, tree map[string]any, extra map[string]tftypes.Value) (tftypes.Value, error) {
	objType, ok := t.(tftypes.Object)
	if !ok {
		return tftypes.Value{}, fmt.Errorf("expected an object type, got %s", t)
	}
	vals := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, at := range objType.AttributeTypes {
		if v, ok := extra[name]; ok {
			vals[name] = v
			continue
		}
		f := n.Field(name)
		if f == nil {
			vals[name] = tftypes.NewValue(at, nil)
			continue
		}
		v, err := fieldToValue(at, f, tree[name])
		if err != nil {
			return tftypes.Value{}, fmt.Errorf("%s: %w", name, err)
		}
		vals[name] = v
	}
	return tftypes.NewValue(objType, vals), nil
}

func fieldToValue(t tftypes.Type, f *terraform.Node, v any) (tftypes.Value, error) {
	switch f.Kind {
	case terraform.KindBlock:
		m, ok := v.(map[string]any)
		if !ok {
			return tftypes.NewValue(t, nil), nil
		}
		return treeToValue(t, f, m, nil)
	case terraform.KindBlocks:
		lt, _ := t.(tftypes.List)
		list, _ := v.([]any)
		elems := make([]tftypes.Value, 0, len(list))
		for _, e := range list {
			m, _ := e.(map[string]any)
			ev, err := treeToValue(lt.ElementType, f, m, nil)
			if err != nil {
				return tftypes.Value{}, err
			}
			elems = append(elems, ev)
		}
		return tftypes.NewValue(t, elems), nil
	}
	if v == nil {
		return tftypes.NewValue(t, nil), nil
	}
	switch f.Kind {
	case terraform.KindList:
		lt, _ := t.(tftypes.List)
		list, _ := v.([]any)
		elems := make([]tftypes.Value, 0, len(list))
		for _, e := range list {
			ev, err := scalarToValue(lt.ElementType, e)
			if err != nil {
				return tftypes.Value{}, err
			}
			elems = append(elems, ev)
		}
		return tftypes.NewValue(t, elems), nil
	case terraform.KindMap:
		mt, _ := t.(tftypes.Map)
		m, _ := v.(map[string]any)
		elems := make(map[string]tftypes.Value, len(m))
		for k, e := range m {
			ev, err := scalarToValue(mt.ElementType, e)
			if err != nil {
				return tftypes.Value{}, err
			}
			elems[k] = ev
		}
		return tftypes.NewValue(t, elems), nil
	}
	return scalarToValue(t, v)
}

func scalarToValue(t tftypes.Type, v any) (tftypes.Value, error) {
	switch x := v.(type) {
	case string:
		return tftypes.NewValue(t, x), nil
	case bool:
		return tftypes.NewValue(t, x), nil
	case json.Number:
		f, _, err := big.ParseFloat(x.String(), 10, 512, big.ToNearestEven)
		if err != nil {
			return tftypes.Value{}, err
		}
		return tftypes.NewValue(t, f), nil
	}
	return tftypes.Value{}, fmt.Errorf("unexpected value %T", v)
}
