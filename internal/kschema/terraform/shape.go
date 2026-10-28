// Package terraform maps a dashboard preset to and from Terraform's shape:
// typed attributes and nested blocks, the way `datadog_dashboard` writes
// widgets, rather than one JSON string.
//
// The shape is derived at run time from the generated JSON Schema
// (schema/gen/dashboard-preset.schema.json), so the CUE contract stays the one
// source of truth: a panel option added there reaches Terraform without a code
// change here. kubeapi uses this package to export a dashboard as HCL, and the
// Terraform provider uses the same package for its resource schema and for
// converting in both directions, so the two can never disagree about a field.
package terraform

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema"
)

// Kind is how one preset member appears in Terraform.
type Kind int

const (
	KindString Kind = iota
	KindNumber
	KindBool
	// KindList is a list attribute of scalars; Node.Elem says which.
	KindList
	// KindMap is a map attribute of strings.
	KindMap
	// KindJSON carries a value Terraform's type system cannot express (a
	// union of a string and an object, say) as a JSON string, written with
	// jsonencode(). Its Terraform name ends in "_json".
	KindJSON
	// KindBlock is a single nested block for an object.
	KindBlock
	// KindBlocks is a repeated nested block for a list of objects.
	KindBlocks
)

// ExtraField is the attribute every block carries for members the schema does
// not describe. Preset objects are open (additionalProperties: true), and a
// conversion that dropped what it did not know would lose data on the next
// apply; this keeps the round trip lossless.
const ExtraField = "extra_json"

// Excluded preset members are kept off the Terraform surface:
// schema_version is the server's to set (the provider sends the current one),
// and publicDashboardPath is written when someone publishes the dashboard,
// which Terraform must not undo.
var Excluded = map[string]bool{"schema_version": true, "publicDashboardPath": true}

// AlertRuleOmitted are the stored rule's members that are not part of its
// definition: what the server computes (state, timestamps, ids, the resolved
// channel objects, query_type, which follows from the queries) and what a
// rule's create request cannot set. NotificationChannelNamesField stands in for
// the channel ids, which differ between installs.
var AlertRuleOmitted = map[string]bool{
	"id": true, "created_at": true, "updated_at": true, "created_by": true, "org_id": true,
	"current_state": true, "last_evaluation_time": true, "last_state_change_time": true,
	"firing_since": true, "consecutive_breaches": true, "last_successful_evaluation_at": true,
	"evaluation_failing_since": true, "evaluation_issue": true, "evaluation_issue_at": true,
	"evaluation_issue_kind": true, "notification_channel_ids": true, "notification_channels": true,
	"schema_version": true, "query_type": true, "raw_query": true, "team": true,
}

// NotificationChannelNamesField names a rule's notification channels in its
// definition. Channel names are unique and cannot change, so unlike the ids
// they mean the same thing in every install.
const NotificationChannelNamesField = "notification_channel_names"

// Node is one member of the Terraform shape. A block's Fields are its
// attributes and nested blocks, sorted by Terraform name. Omit, set on a
// root only, lists the document members deliberately kept off the Terraform
// surface, so a conversion neither carries them nor reports them as extra.
type Node struct {
	Omit        map[string]bool
	Kind        Kind
	Elem        Kind // element kind of a KindList
	Name        string
	JSONName    string
	Required    bool
	Description string
	Fields      []*Node
}

// Field returns the child with the given Terraform name.
func (n *Node) Field(name string) *Node {
	for _, f := range n.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

var (
	dashboardOnce  sync.Once
	dashboardShape *Node
	dashboardErr   error
)

// DashboardShape is the Terraform shape of a dashboard preset: the root block,
// whose fields sit directly in the kubesense_dashboard resource.
func DashboardShape() (*Node, error) {
	dashboardOnce.Do(func() {
		raw, err := schema.Schema(schema.DashboardPreset)
		if err != nil {
			dashboardErr = err
			return
		}
		dashboardShape, dashboardErr = buildShape(raw, Excluded)
	})
	return dashboardShape, dashboardErr
}

var (
	alertOnce  sync.Once
	alertShape *Node
	alertErr   error
)

// AlertRuleShape is the Terraform shape of an alert rule's definition: the
// stored rule (the shape Export writes and Import reads) without what the
// server computes, and with its channels named rather than numbered.
func AlertRuleShape() (*Node, error) {
	alertOnce.Do(func() {
		raw, err := schema.Schema(schema.AlertRule)
		if err != nil {
			alertErr = err
			return
		}
		alertShape, alertErr = buildShape(raw, AlertRuleOmitted)
		if alertErr != nil {
			return
		}
		alertShape.Fields = append(alertShape.Fields, &Node{
			Kind: KindList, Elem: KindString, Name: NotificationChannelNamesField,
			JSONName:    NotificationChannelNamesField,
			Description: "Notification channels to send this rule's alerts to, by name.",
		})
		sortFields(alertShape)
	})
	return alertShape, alertErr
}

type jsonSchema = map[string]any

// builder walks the JSON Schema. active holds the $defs being classified on
// the current path: a Terraform schema cannot be recursive, so a definition
// met again inside itself (a condition expression holding expressions, say)
// is carried as JSON from that point down.
type builder struct {
	defs   map[string]jsonSchema
	active map[string]bool
}

func buildShape(raw []byte, omit map[string]bool) (*Node, error) {
	var root jsonSchema
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("terraform shape: %w", err)
	}
	b := builder{defs: map[string]jsonSchema{}, active: map[string]bool{}}
	if defs, ok := root["$defs"].(map[string]any); ok {
		for k, v := range defs {
			if d, ok := v.(map[string]any); ok {
				b.defs[k] = d
			}
		}
	}
	node := b.object("", root)
	kept := node.Fields[:0]
	for _, f := range node.Fields {
		if !omit[f.JSONName] {
			kept = append(kept, f)
		}
	}
	node.Fields = kept
	node.Omit = omit
	if err := checkNames(node, "preset"); err != nil {
		return nil, err
	}
	return node, nil
}

// deref follows $ref, keeping the referring schema's description.
func (b builder) deref(s jsonSchema) jsonSchema {
	for {
		ref, ok := s["$ref"].(string)
		if !ok {
			return s
		}
		target := b.defs[strings.TrimPrefix(ref, "#/$defs/")]
		if target == nil {
			return jsonSchema{}
		}
		if desc, ok := s["description"]; ok {
			merged := jsonSchema{}
			for k, v := range target {
				merged[k] = v
			}
			merged["description"] = desc
			s = merged
		} else {
			s = target
		}
	}
}

// node classifies one member schema.
func (b builder) node(jsonName string, s jsonSchema, required bool) *Node {
	n := b.classify(s)
	n.JSONName = jsonName
	n.Required = required
	if d, ok := b.deref(s)["description"].(string); ok {
		n.Description = d
	}
	n.Name = tfName(jsonName, n.Kind)
	return n
}

func (b builder) classify(s jsonSchema) *Node {
	if ref, ok := s["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		if b.active[name] {
			return &Node{Kind: KindJSON}
		}
		target := b.defs[name]
		if target == nil {
			return &Node{Kind: KindJSON}
		}
		b.active[name] = true
		defer delete(b.active, name)
		return b.classify(target)
	}
	if all, ok := s["allOf"].([]any); ok {
		merged := jsonSchema{"type": "object", "properties": map[string]any{}}
		props := merged["properties"].(map[string]any)
		var req []any
		for _, part := range all {
			p := b.deref(asSchema(part))
			if pp, ok := p["properties"].(map[string]any); ok {
				for k, v := range pp {
					props[k] = v
				}
			}
			if r, ok := p["required"].([]any); ok {
				req = append(req, r...)
			}
		}
		merged["required"] = req
		return b.classify(merged)
	}
	if branches, ok := s["anyOf"].([]any); ok {
		return b.union(branches)
	}
	if c, ok := s["const"]; ok {
		return scalarNode(c)
	}
	if e, ok := s["enum"].([]any); ok {
		kind := KindJSON
		for i, v := range e {
			k := scalarNode(v).Kind
			if i == 0 {
				kind = k
			} else if k != kind {
				kind = KindJSON
			}
		}
		return &Node{Kind: kind}
	}
	switch t := s["type"].(type) {
	case string:
		switch t {
		case "string":
			return &Node{Kind: KindString}
		case "integer", "number":
			return &Node{Kind: KindNumber}
		case "boolean":
			return &Node{Kind: KindBool}
		case "array":
			return b.array(s)
		case "object":
			return b.objectOrMap(s)
		}
	case nil:
		if _, ok := s["properties"]; ok {
			return b.objectOrMap(s)
		}
	}
	return &Node{Kind: KindJSON}
}

func (b builder) array(s jsonSchema) *Node {
	items, ok := s["items"].(map[string]any)
	if !ok {
		return &Node{Kind: KindJSON}
	}
	item := b.classify(items)
	switch item.Kind {
	case KindString, KindNumber, KindBool:
		return &Node{Kind: KindList, Elem: item.Kind}
	case KindBlock:
		item.Kind = KindBlocks
		return item
	}
	return &Node{Kind: KindJSON}
}

func (b builder) objectOrMap(s jsonSchema) *Node {
	if _, ok := s["properties"]; ok {
		return b.object("", s)
	}
	if ap, ok := s["additionalProperties"].(map[string]any); ok {
		if b.classify(ap).Kind == KindString {
			return &Node{Kind: KindMap}
		}
	}
	return &Node{Kind: KindJSON}
}

func (b builder) object(jsonName string, s jsonSchema) *Node {
	required := map[string]bool{}
	if r, ok := s["required"].([]any); ok {
		for _, v := range r {
			if name, ok := v.(string); ok {
				required[name] = true
			}
		}
	}
	n := &Node{Kind: KindBlock, JSONName: jsonName}
	props, _ := s["properties"].(map[string]any)
	for name, p := range props {
		n.Fields = append(n.Fields, b.node(name, asSchema(p), required[name]))
	}
	n.Fields = append(n.Fields, &Node{Kind: KindJSON, Name: ExtraField,
		Description: "Members the schema does not describe, as JSON. Kept so nothing is lost."})
	sortFields(n)
	return n
}

// union merges anyOf branches into one Terraform type. Branches that only say
// "empty is allowed" (const [] beside an array, const {} beside an object) and
// null add nothing. Object branches merge into one block holding every
// branch's members; anything else that does not agree becomes KindJSON.
func (b builder) union(branches []any) *Node {
	var nodes []*Node
	var emptyArray, emptyObject bool
	for _, br := range branches {
		s := b.deref(asSchema(br))
		if t, _ := s["type"].(string); t == "null" {
			continue
		}
		if c, ok := s["const"]; ok {
			switch v := c.(type) {
			case []any:
				if len(v) == 0 {
					emptyArray = true
					continue
				}
			case map[string]any:
				if len(v) == 0 {
					emptyObject = true
					continue
				}
			case nil:
				continue
			}
		}
		nodes = append(nodes, b.classify(asSchema(br)))
	}
	if len(nodes) == 0 {
		return &Node{Kind: KindJSON}
	}
	out := nodes[0]
	for _, n := range nodes[1:] {
		out = merge(out, n)
	}
	if emptyArray && out.Kind != KindList && out.Kind != KindBlocks && out.Kind != KindJSON {
		return &Node{Kind: KindJSON}
	}
	if emptyObject && out.Kind != KindBlock && out.Kind != KindMap && out.Kind != KindJSON {
		return &Node{Kind: KindJSON}
	}
	return out
}

// merge combines two shapes for the same member, or gives up to KindJSON.
func merge(a, b *Node) *Node {
	if a.Kind != b.Kind {
		return &Node{Kind: KindJSON}
	}
	switch a.Kind {
	case KindList:
		if a.Elem != b.Elem {
			return &Node{Kind: KindJSON}
		}
		return a
	case KindBlock, KindBlocks:
		out := &Node{Kind: a.Kind, JSONName: a.JSONName}
		byJSON := map[string]*Node{}
		for _, f := range a.Fields {
			byJSON[f.JSONName+"\x00"+f.Name] = f
		}
		for _, f := range b.Fields {
			key := f.JSONName + "\x00" + f.Name
			if g, ok := byJSON[key]; ok {
				isExtra := g.Name == ExtraField
				jsonName, desc, req := g.JSONName, g.Description, mergedRequired(g, f.Required, true)
				m := merge(g, f)
				m.JSONName, m.Description = jsonName, desc
				// A member required in only one branch is optional in the merge.
				m.Required = req
				if isExtra {
					m.Name = ExtraField
				} else {
					m.Name = tfName(jsonName, m.Kind)
				}
				byJSON[key] = m
				continue
			}
			// The same member under another Terraform name means the branches
			// disagree on its type: carry it as JSON.
			if clash := findJSON(byJSON, f.JSONName); clash != "" && f.JSONName != "" {
				old := byJSON[clash]
				delete(byJSON, clash)
				byJSON[f.JSONName+"\x00"+tfName(f.JSONName, KindJSON)] = &Node{
					Kind: KindJSON, JSONName: f.JSONName, Description: old.Description,
					Name: tfName(f.JSONName, KindJSON),
				}
				continue
			}
			c := *f
			c.Required = mergedRequired(&c, false, false)
			byJSON[key] = &c
		}
		for _, f := range byJSON {
			if !inBoth(a, b, f.JSONName) {
				f.Required = mergedRequired(f, false, false)
			}
			out.Fields = append(out.Fields, f)
		}
		sortFields(out)
		return out
	}
	return a
}

// mergedRequired is whether a member stays required when union branches
// merge. Required only means something for a repeated block, which is written
// as [] when it has no entries (Terraform cannot tell an empty list of blocks
// from none). For those it is required if ANY branch requires it, so a branch
// that needs the empty list gets one; the others are open objects and accept
// it. Every other member is required only if every branch requires it.
func mergedRequired(n *Node, other, otherHasIt bool) bool {
	if n.Kind == KindBlocks {
		return n.Required || other
	}
	return n.Required && other && otherHasIt
}

func findJSON(m map[string]*Node, jsonName string) string {
	for k, v := range m {
		if v.JSONName == jsonName {
			return k
		}
	}
	return ""
}

func inBoth(a, b *Node, jsonName string) bool {
	has := func(n *Node) bool {
		for _, f := range n.Fields {
			if f.JSONName == jsonName {
				return true
			}
		}
		return false
	}
	return has(a) && has(b)
}

func scalarNode(v any) *Node {
	switch v.(type) {
	case string:
		return &Node{Kind: KindString}
	case float64, json.Number:
		return &Node{Kind: KindNumber}
	case bool:
		return &Node{Kind: KindBool}
	}
	return &Node{Kind: KindJSON}
}

func sortFields(n *Node) {
	sort.Slice(n.Fields, func(i, j int) bool { return n.Fields[i].Name < n.Fields[j].Name })
}

func asSchema(v any) jsonSchema {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return jsonSchema{}
}

// tfName is the Terraform name for a preset member: snake_case, singular for
// a repeated block (panels → panel), and suffixed _json for a JSON carrier.
func tfName(jsonName string, kind Kind) string {
	name := snake(jsonName)
	switch kind {
	case KindBlocks:
		name = singular(name)
	case KindJSON:
		name += "_json"
	}
	return name
}

// snake turns camelCase into snake_case, keeping acronyms whole
// (publicURL → public_url), and replaces anything Terraform does not allow in a
// name with an underscore.
func snake(s string) string {
	rs := []rune(s)
	var out strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) {
			prevLower := i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1]))
			acronymEnd := i > 0 && unicode.IsUpper(rs[i-1]) && i+1 < len(rs) && unicode.IsLower(rs[i+1])
			if prevLower || acronymEnd {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(r))
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	name := out.String()
	if name == "" || unicode.IsDigit(rune(name[0])) {
		name = "f_" + name
	}
	return name
}

func singular(name string) string {
	switch {
	case strings.HasSuffix(name, "ies"):
		return strings.TrimSuffix(name, "ies") + "y"
	case strings.HasSuffix(name, "sses"), strings.HasSuffix(name, "xes"):
		return strings.TrimSuffix(name, "es")
	case strings.HasSuffix(name, "s") && !strings.HasSuffix(name, "ss"):
		return strings.TrimSuffix(name, "s")
	}
	return name
}

// checkNames refuses a shape in which two members of one block map to the same
// Terraform name: one would silently shadow the other.
func checkNames(n *Node, path string) error {
	if n.Kind != KindBlock && n.Kind != KindBlocks {
		return nil
	}
	seen := map[string]string{}
	for _, f := range n.Fields {
		if other, ok := seen[f.Name]; ok {
			return fmt.Errorf("terraform shape: %s: %q and %q both map to %q", path, other, f.JSONName, f.Name)
		}
		seen[f.Name] = f.JSONName
		if err := checkNames(f, path+"."+f.Name); err != nil {
			return err
		}
	}
	return nil
}
