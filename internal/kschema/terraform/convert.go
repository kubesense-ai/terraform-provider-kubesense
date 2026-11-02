package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema"
)

// The two trees this package converts between:
//
//   - a preset, as decoded JSON: map[string]any, []any, string, json.Number,
//     bool (always decoded with UseNumber, so numbers keep their text);
//   - a Terraform value, keyed by Terraform names: the same scalar types, []any
//     for a list attribute or a repeated block, map[string]any for a map
//     attribute or a block, and a JSON string for KindJSON and extra_json.
//
// Members that are absent, or null, are absent from the other tree too.

// CanonicalPreset decodes a preset in the form the server stores and the
// provider sends: migrated to the current schema version by the server's own
// code, with a tabs key (an update without one over a tabbed dashboard is
// refused), and numbers kept as written.
func CanonicalPreset(raw []byte) (map[string]any, error) {
	check, err := schema.CheckDashboardPreset(raw)
	if err != nil {
		return nil, err
	}
	doc, err := decode(check.Migrated)
	if err != nil {
		return nil, err
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dashboard preset must be a JSON object")
	}
	if _, ok := obj["tabs"]; !ok {
		obj["tabs"] = []any{}
	}
	return obj, nil
}

// ToTerraform converts a preset object to the Terraform value of a block.
func ToTerraform(n *Node, obj map[string]any) map[string]any {
	out := map[string]any{}
	extra := map[string]any{}
	known := map[string]bool{}
	for _, f := range n.Fields {
		if f.Name == ExtraField {
			continue
		}
		known[f.JSONName] = true
		v, ok := obj[f.JSONName]
		if !ok || v == nil {
			continue
		}
		if tv, ok := toTerraformValue(f, v); ok {
			out[f.Name] = tv
		} else {
			// A value that does not fit the schema's type still has to
			// survive the round trip.
			extra[f.JSONName] = v
		}
	}
	for k, v := range obj {
		if !known[k] && !n.Omit[k] {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		out[ExtraField] = encode(extra)
	}
	return out
}

// DropEmptyStrings removes string attributes whose value is "" from a block's
// Terraform value, nested blocks included. Terraform writes an unset optional
// string as null (terraform plan -generate-config-out does), so an import or
// an export that kept "" would plan a change to every such member. Use it only
// where "" and absent mean the same to the server, as they do for an alert
// rule's definition, which the server decodes into structs.
func DropEmptyStrings(n *Node, tf map[string]any) map[string]any {
	for _, f := range n.Fields {
		v, ok := tf[f.Name]
		if !ok {
			continue
		}
		switch f.Kind {
		case KindString:
			if s, _ := v.(string); s == "" {
				delete(tf, f.Name)
			}
		case KindBlock:
			if m, ok := v.(map[string]any); ok {
				DropEmptyStrings(f, m)
			}
		case KindBlocks:
			list, _ := v.([]any)
			for _, e := range list {
				if m, ok := e.(map[string]any); ok {
					DropEmptyStrings(f, m)
				}
			}
		}
	}
	return tf
}

// DropEmptyRequiredStrings removes required string members whose value is ""
// from a block's Terraform value where prior (the same block's previous
// value) has them unset. FromTerraform sends an unset required string as "",
// so a configuration that leaves one unset reads back unset, not as a change
// to "". Optional members are left alone: in a preset, "" and absent can
// differ.
func DropEmptyRequiredStrings(n *Node, tf, prior map[string]any) map[string]any {
	for _, f := range n.Fields {
		v, ok := tf[f.Name]
		if !ok {
			continue
		}
		pv := prior[f.Name]
		switch f.Kind {
		case KindString:
			if s, _ := v.(string); s == "" && f.Required && pv == nil {
				delete(tf, f.Name)
			}
		case KindBlock:
			m, _ := v.(map[string]any)
			pm, _ := pv.(map[string]any)
			if m != nil && pm != nil {
				DropEmptyRequiredStrings(f, m, pm)
			}
		case KindBlocks:
			list, _ := v.([]any)
			plist, _ := pv.([]any)
			// Entries beyond the prior ones are new; they keep what the
			// server says.
			for i := 0; i < len(list) && i < len(plist); i++ {
				m, _ := list[i].(map[string]any)
				pm, _ := plist[i].(map[string]any)
				if m != nil && pm != nil {
					DropEmptyRequiredStrings(f, m, pm)
				}
			}
		}
	}
	return tf
}

func toTerraformValue(f *Node, v any) (any, bool) {
	switch f.Kind {
	case KindString, KindNumber, KindBool:
		return v, scalarFits(f.Kind, v)
	case KindList:
		list, ok := v.([]any)
		if !ok {
			return nil, false
		}
		for _, e := range list {
			if !scalarFits(f.Elem, e) {
				return nil, false
			}
		}
		return list, true
	case KindMap:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		for _, e := range m {
			if _, ok := e.(string); !ok {
				return nil, false
			}
		}
		return m, true
	case KindJSON:
		return encode(v), true
	case KindBlock:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		return ToTerraform(f, m), true
	case KindBlocks:
		list, ok := v.([]any)
		if !ok {
			return nil, false
		}
		out := make([]any, 0, len(list))
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, ToTerraform(f, m))
		}
		return out, true
	}
	return nil, false
}

func scalarFits(k Kind, v any) bool {
	switch k {
	case KindString:
		_, ok := v.(string)
		return ok
	case KindNumber:
		_, ok := v.(json.Number)
		return ok
	case KindBool:
		_, ok := v.(bool)
		return ok
	}
	return false
}

// FromTerraform converts a block's Terraform value back to a preset object.
// A repeated block with no entries is written as [] only when the schema
// requires the member, since Terraform cannot tell an empty list from an
// absent one.
func FromTerraform(n *Node, tf map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for _, f := range n.Fields {
		v, ok := tf[f.Name]
		if !ok || v == nil {
			switch {
			case f.Kind == KindBlocks && f.Required:
				out[f.JSONName] = []any{}
			case f.Kind == KindString && f.Required:
				// Terraform writes an empty string as null; the schema
				// requires the member, so it was "" (see
				// DropEmptyRequiredStrings).
				out[f.JSONName] = ""
			}
			continue
		}
		if f.Name == ExtraField {
			s, _ := v.(string)
			if s == "" {
				continue
			}
			extra, err := decode([]byte(s))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ExtraField, err)
			}
			m, ok := extra.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s must be a JSON object", ExtraField)
			}
			for k, e := range m {
				out[k] = e
			}
			continue
		}
		jv, err := fromTerraformValue(f, v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if jv == nil {
			continue
		}
		out[f.JSONName] = jv
	}
	return out, nil
}

func fromTerraformValue(f *Node, v any) (any, error) {
	switch f.Kind {
	case KindJSON:
		s, _ := v.(string)
		if s == "" {
			return nil, nil
		}
		return decode([]byte(s))
	case KindBlock:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected a block")
		}
		return FromTerraform(f, m)
	case KindBlocks:
		list, _ := v.([]any)
		if len(list) == 0 && !f.Required {
			return nil, nil
		}
		out := make([]any, 0, len(list))
		for _, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected a block")
			}
			o, err := FromTerraform(f, m)
			if err != nil {
				return nil, err
			}
			out = append(out, o)
		}
		return out, nil
	}
	return v, nil
}

func decode(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// encode is compact JSON with sorted keys (encoding/json sorts map keys):
// what Terraform's jsonencode() produces, so a value written with jsonencode()
// compares equal.
func encode(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
