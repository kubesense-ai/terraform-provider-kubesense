package terraform

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// Dashboard is what an export needs to know about one dashboard.
type Dashboard struct {
	ID          string
	Name        string
	Description string
	Preset      []byte
	// System dashboards are read-only, so they are exported as a data source.
	System bool
}

// Export is a dashboard rendered for the KubeSense Terraform provider.
type Export struct {
	HCL     string `json:"hcl"`
	Address string `json:"address"`
}

// ExportDashboard renders a dashboard as a kubesense_dashboard resource with
// its panels, variables and layout as typed blocks, formatted as
// `terraform fmt` would.
func ExportDashboard(d Dashboard) (Export, error) {
	label := Label(d.Name, "dashboard")
	f := hclwrite.NewEmptyFile()

	if d.System {
		block := f.Body().AppendNewBlock("data", []string{"kubesense_dashboard", label})
		block.Body().SetAttributeValue("name", cty.StringVal(d.Name))
		return Export{
			HCL: "# System dashboards are read-only. Read this one, and build your own\n" +
				"# from it to customise it.\n" + string(hclwrite.Format(f.Bytes())),
			Address: "data.kubesense_dashboard." + label,
		}, nil
	}

	shape, err := DashboardShape()
	if err != nil {
		return Export{}, err
	}
	preset, err := CanonicalPreset(d.Preset)
	if err != nil {
		return Export{}, err
	}

	block := f.Body().AppendNewBlock("resource", []string{"kubesense_dashboard", label})
	body := block.Body()
	body.SetAttributeValue("name", cty.StringVal(d.Name))
	if d.Description != "" {
		body.SetAttributeValue("description", cty.StringVal(d.Description))
	}
	if err := writeBody(body, shape, ToTerraform(shape, preset)); err != nil {
		return Export{}, err
	}

	address := "kubesense_dashboard." + label
	return Export{HCL: importBlock(address, d.ID) + string(hclwrite.Format(f.Bytes())), Address: address}, nil
}

// ExportAlertRule renders an alert rule's definition as a
// kubesense_alert_rule resource. The definition should be the server's
// canonical one (what the provider reads back), so the export imports with no
// changes.
func ExportAlertRule(id string, definition map[string]any) (Export, error) {
	shape, err := AlertRuleShape()
	if err != nil {
		return Export{}, err
	}
	name, _ := definition["name"].(string)
	label := Label(name, "alert")
	f := hclwrite.NewEmptyFile()
	block := f.Body().AppendNewBlock("resource", []string{"kubesense_alert_rule", label})
	if err := writeBody(block.Body(), shape, ToTerraform(shape, definition)); err != nil {
		return Export{}, err
	}
	address := "kubesense_alert_rule." + label
	return Export{HCL: importBlock(address, id) + string(hclwrite.Format(f.Bytes())), Address: address}, nil
}

// importBlock adopts the exported object on the first apply (Terraform 1.5+),
// so applying an export edits the original rather than creating a copy, with
// no separate terraform import to remember. Empty for an object with no id.
func importBlock(address, id string) string {
	if id == "" {
		return ""
	}
	return "# Manages the existing one: the first apply adopts it instead of creating a copy.\n" +
		"# Delete this block to create a copy instead.\n" +
		"import {\n" +
		"  to = " + address + "\n" +
		"  id = " + string(hclwrite.TokensForValue(cty.StringVal(id)).Bytes()) + "\n" +
		"}\n\n"
}

// writeBody writes a block's Terraform value: attributes first, then nested
// blocks, each in name order, which is how hand-written Terraform reads.
func writeBody(body *hclwrite.Body, n *Node, tf map[string]any) error {
	var blocks []*Node
	// A blank line separates each nested block from what precedes it, but
	// not from the opening brace.
	wrote := len(body.Attributes()) > 0
	for _, f := range n.Fields {
		v, ok := tf[f.Name]
		if !ok {
			continue
		}
		switch f.Kind {
		case KindBlock, KindBlocks:
			blocks = append(blocks, f)
		case KindJSON:
			wrote = true
			s, _ := v.(string)
			val, err := jsonToCty(s)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			body.SetAttributeRaw(f.Name, hclwrite.TokensForFunctionCall("jsonencode", hclwrite.TokensForValue(val)))
		default:
			wrote = true
			val, err := scalarToCty(v)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			body.SetAttributeValue(f.Name, val)
		}
	}
	for _, f := range blocks {
		v := tf[f.Name]
		var items []any
		if f.Kind == KindBlock {
			items = []any{v}
		} else {
			items, _ = v.([]any)
		}
		for _, item := range items {
			m, _ := item.(map[string]any)
			if wrote {
				body.AppendNewline()
			}
			wrote = true
			child := body.AppendNewBlock(f.Name, nil)
			if err := writeBody(child.Body(), f, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// scalarToCty converts a scalar, list or map attribute value.
func scalarToCty(v any) (cty.Value, error) {
	switch t := v.(type) {
	case string:
		return cty.StringVal(t), nil
	case bool:
		return cty.BoolVal(t), nil
	case json.Number:
		return cty.ParseNumberVal(t.String())
	case []any:
		if len(t) == 0 {
			return cty.EmptyTupleVal, nil
		}
		vals := make([]cty.Value, 0, len(t))
		for _, e := range t {
			c, err := scalarToCty(e)
			if err != nil {
				return cty.NilVal, err
			}
			vals = append(vals, c)
		}
		return cty.TupleVal(vals), nil
	case map[string]any:
		if len(t) == 0 {
			return cty.EmptyObjectVal, nil
		}
		vals := make(map[string]cty.Value, len(t))
		for k, e := range t {
			c, err := scalarToCty(e)
			if err != nil {
				return cty.NilVal, err
			}
			vals[k] = c
		}
		return cty.ObjectVal(vals), nil
	}
	return cty.NilVal, fmt.Errorf("unsupported value %T", v)
}

func jsonToCty(s string) (cty.Value, error) {
	ty, err := ctyjson.ImpliedType([]byte(s))
	if err != nil {
		return cty.NilVal, err
	}
	return ctyjson.Unmarshal([]byte(s), ty)
}

var notLabel = regexp.MustCompile(`[^a-z0-9]+`)

// Label is a Terraform resource label from a display name: lower case, runs of
// anything else collapsed to _. A label cannot start with a digit, so such a
// name is prefixed with the fallback, which also stands in for a name with
// nothing usable in it.
func Label(name, fallback string) string {
	slug := strings.Trim(notLabel.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if slug == "" {
		return fallback
	}
	if slug[0] >= '0' && slug[0] <= '9' {
		return fallback + "_" + slug
	}
	return slug
}
