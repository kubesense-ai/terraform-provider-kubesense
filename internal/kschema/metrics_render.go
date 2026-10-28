package schema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// metrics_render.go — builder metrics query → PromQL, from the function catalog.
//
// A builder query stores a metric, label filters and a function chain, never
// PromQL; the webapp renders it at query time (explorer/v2/metrics-functions/
// render.ts and utils/generatePromQL.ts). This is the same renderer, driven by
// the same catalog data, so a server-side reader such as an MCP tool can say
// what a stored panel actually runs. The catalog examples are its goldens, as
// they are the webapp's; any change to the rules belongs in both renderers.
//
// RenderMetricsQuery leaves dashboard variables as written ("$cluster"), since
// resolving them needs a selection; RenderMetricsQueryResolved takes one and
// renders the PromQL a panel actually sends. Either way the renderer expands a
// field reference, a filter key "$$name" meaning "the variable's field = $name"
// (variable-field-reference.ts), which needs the dashboard's variable-to-field
// map.

//go:embed gen/metrics-functions.json
var metricsFunctionsJSON []byte

// The part of gen/metrics-functions.json the renderer reads. The webapp's
// metrics-functions/catalog.ts documents every field of the catalog.

type catalogParamKind struct {
	Kind   string `json:"kind"`
	Quoted bool   `json:"quoted"`
}

type catalogParam struct {
	Name     string           `json:"name"`
	At       string           `json:"at"`
	Kind     catalogParamKind `json:"kind"`
	Optional bool             `json:"optional"`
	Default  json.RawMessage  `json:"default"`
	Aliases  []string         `json:"aliases"`
}

type catalogFunction struct {
	Form   string         `json:"form"`
	Callee string         `json:"callee"`
	Legacy string         `json:"legacy"`
	Params []catalogParam `json:"params"`
}

// loadRenderCatalog parses the embedded catalog once, on first render.
var loadRenderCatalog = sync.OnceValues(func() (map[string]catalogFunction, error) {
	var c map[string]catalogFunction
	err := json.Unmarshal(metricsFunctionsJSON, &c)
	return c, err
})

// BuilderArgument is one stored {arg_name, arg_value}.
type BuilderArgument struct {
	Name  string          `json:"arg_name"`
	Value json.RawMessage `json:"arg_value"`
}

// BuilderFunction is one stored step of a builder chain.
type BuilderFunction struct {
	Name      string            `json:"name"`
	Arguments []BuilderArgument `json:"arguments"`
}

// SelectorModifiers are the offset and @ on a builder query's selector.
type SelectorModifiers struct {
	Offset string          `json:"offset,omitempty"`
	At     json.RawMessage `json:"at,omitempty"`
}

// LabelFilter is one label's stored filter values, in stored order.
type LabelFilter struct {
	Label  string
	Values []string
}

// RenderMetricsQuery renders a stored builder metrics query (a panel or alert
// query object) as PromQL. Filter order follows the object's key order, as the
// webapp's does.
//
// fields maps a dashboard variable's name to the label it lists (its
// fieldMeta.field); it may be nil. A filter key "$$name" becomes that label
// filtered by "$name", and is dropped when name has no entry.
func RenderMetricsQuery(query []byte, fields map[string]string) (string, error) {
	return renderMetricsQuery(query, fields, nil)
}

// RenderMetricsQueryResolved is RenderMetricsQuery with the dashboard's
// variable selection applied to the label filters, as generatePromQL does for
// a panel: a "$name" value becomes the selected values, "All" (a variable with
// no selection) becomes a match-all regex, and a name the map does not define
// drops out. Function arguments are not substituted, as in the webapp.
func RenderMetricsQueryResolved(query []byte, fields map[string]string, variables VariableValues) (string, error) {
	if variables == nil {
		variables = VariableValues{}
	}
	return renderMetricsQuery(query, fields, variables)
}

// renderMetricsQuery renders with variables resolved, or preserved as written
// when vars is nil.
func renderMetricsQuery(query []byte, fields map[string]string, vars VariableValues) (string, error) {
	var q struct {
		SelectedMetric    string             `json:"selectedMetric"`
		Functions         []BuilderFunction  `json:"functions"`
		SelectorModifiers *SelectorModifiers `json:"selectorModifiers"`
		Filters           json.RawMessage    `json:"filters"`
	}
	if err := json.Unmarshal(query, &q); err != nil {
		return "", err
	}
	if strings.TrimSpace(q.SelectedMetric) == "" {
		return "", errors.New("query has no selectedMetric")
	}
	filters, err := OrderedFilters(q.Filters)
	if err != nil {
		return "", fmt.Errorf("filters: %w", err)
	}
	filters = expandFieldReferences(filters, fields)
	selector := q.SelectedMetric + "{" + renderLabelFilters(filters, vars) + "}"
	return renderChain(selector, q.Functions, q.SelectorModifiers)
}

// fieldReference is a whole filter key that is one "$$name".
var fieldReference = regexp.MustCompile(`^\$\$([a-zA-Z_][a-zA-Z0-9_]*)$`)

// expandFieldReferences is desugarFieldReferencesInFilters for metrics, whose
// labels have no attribute namespace. "$$ns" becomes label "$ns" at its own
// position; a label already filtered gains "$ns" once, and the filter values
// of a "$$" key itself are ignored.
func expandFieldReferences(filters []LabelFilter, fields map[string]string) []LabelFilter {
	hasReference := false
	for _, f := range filters {
		if fieldReference.MatchString(f.Label) {
			hasReference = true
			break
		}
	}
	if !hasReference {
		return filters
	}
	var out []LabelFilter
	at := map[string]int{}
	add := func(label string, values []string) {
		i, ok := at[label]
		if !ok {
			at[label] = len(out)
			out = append(out, LabelFilter{Label: label})
			i = len(out) - 1
		}
		for _, v := range values {
			if !slices.Contains(out[i].Values, v) {
				out[i].Values = append(out[i].Values, v)
			}
		}
	}
	for _, f := range filters {
		m := fieldReference.FindStringSubmatch(f.Label)
		if m == nil {
			add(f.Label, f.Values)
			continue
		}
		label := fields[m[1]]
		if label == "" {
			continue
		}
		// A label stored further down is merged at this position, as the webapp's.
		for _, g := range filters {
			if g.Label == label {
				add(label, g.Values)
			}
		}
		add(label, []string{"$" + m[1]})
	}
	return out
}

// RenderBuilderPromQL renders metric{filters} through the function chain.
func RenderBuilderPromQL(metric string, filters []LabelFilter, functions []BuilderFunction, mods *SelectorModifiers) (string, error) {
	return renderChain(metric+"{"+renderLabelFilters(filters, nil)+"}", functions, mods)
}

// OrderedFilters decodes Record<string, string[]> keeping key order. A label
// whose values are not a string list is skipped, as the webapp's would be.
func OrderedFilters(raw json.RawMessage) ([]LabelFilter, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("not an object")
	}
	var out []LabelFilter
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		label, _ := tok.(string)
		var values []string
		var any json.RawMessage
		if err := dec.Decode(&any); err != nil {
			return nil, err
		}
		if json.Unmarshal(any, &values) != nil {
			continue
		}
		out = append(out, LabelFilter{Label: label, Values: values})
	}
	return out, nil
}

// ---- label matchers (generatePromQL.ts buildLabelFilters, preserving $refs)

// variableReference is a whole "$var" or "${var}" value. It goes into a
// matcher verbatim, not escaped, so it still substitutes later.
var variableReference = regexp.MustCompile(`^\$(?:\{[a-zA-Z_][a-zA-Z0-9_]*\}|[a-zA-Z_][a-zA-Z0-9_]*)$`)

// regexMeta is what escapeRegexLiteral escapes: an exact value joined into a
// regex alternation must still match only itself.
var regexMeta = regexp.MustCompile(`[.*+?^${}()|\[\]\\]`)

func escapePromQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

func escapeRegexLiteral(s string) string {
	return regexMeta.ReplaceAllStringFunc(s, func(m string) string { return `\` + m })
}

// parseFilterValue reads the stored prefixes: "-" excludes, "~" is a regex,
// and a quoted regex keeps its quotes out of the pattern.
func parseFilterValue(stored string) (negated, regex bool, text string) {
	negated = strings.HasPrefix(stored, "-")
	body := strings.TrimPrefix(stored, "-")
	if !negated {
		body = stored
	}
	regex = strings.HasPrefix(body, "~")
	text = body
	if regex {
		text = body[1:]
		if len(text) > 1 && strings.HasPrefix(text, `"`) && strings.HasSuffix(text, `"`) {
			text = text[1 : len(text)-1]
		}
	}
	return negated, regex, text
}

// matchAllWildcard marks a variable with nothing selected inside a label's
// exact values, so the matcher becomes a match-all regex rather than an exact
// match on ".*".
const matchAllWildcard = "$$__MATCH_ALL__$$"

// expandVariableValues replaces each "$name" value with the variable's
// selection. A variable with no selection becomes the match-all marker, and a
// name the map does not define drops out.
func expandVariableValues(values []string, vars VariableValues) []string {
	var out []string
	for _, v := range values {
		if !strings.HasPrefix(v, "$") {
			out = append(out, v)
			continue
		}
		selected, known := vars[v[1:]]
		switch {
		case len(selected) > 0:
			out = append(out, selected...)
		case known:
			out = append(out, matchAllWildcard)
		}
	}
	return out
}

// renderLabelFilters gives each label at most one including and one excluding
// matcher, including first, in filter order. With vars nil the "$name" values
// stay as written; otherwise they are resolved against vars.
func renderLabelFilters(filters []LabelFilter, vars VariableValues) string {
	resolve := vars != nil
	if resolve {
		vars = normaliseVariables(vars)
	}
	var matchers []string
	for _, f := range filters {
		if len(f.Values) == 0 {
			continue
		}
		for _, negated := range []bool{false, true} {
			var exact, regex []string
			for _, v := range f.Values {
				n, r, text := parseFilterValue(v)
				if n != negated {
					continue
				}
				if r {
					regex = append(regex, text)
				} else {
					exact = append(exact, text)
				}
			}
			matchAll := false
			if resolve {
				// A variable with nothing selected means "any value": a regex
				// match-all, never an exact match on the two characters ".*".
				// Excluding any value is never the intent, so there it drops out.
				expanded := expandVariableValues(exact, vars)
				exact = exact[:0:0]
				for _, v := range expanded {
					if v == matchAllWildcard {
						matchAll = !negated
					} else {
						exact = append(exact, v)
					}
				}
				var resolved []string
				for _, pattern := range regex {
					if negated {
						var ok bool
						if pattern, ok = withoutEmptyVariableAlternatives(pattern, vars); !ok {
							continue
						}
					}
					resolved = append(resolved, substituteVariablesInRegex(pattern, vars))
				}
				regex = resolved
			}
			if m := renderLabelMatcher(f.Label, exact, regex, negated, matchAll, !resolve); m != "" {
				matchers = append(matchers, m)
			}
		}
	}
	return strings.Join(matchers, ", ")
}

// renderLabelMatcher picks the operator from what the values need: with
// keepReferences a whole "$var" value goes in unescaped so it still
// substitutes later. One exact
// value stays an exact match, one pattern is a regex match, and anything more
// becomes one alternation. The (?:…) group keeps the alternation from binding
// past it; VictoriaMetrics anchors matcher regexes, so no ^…$ is needed.
func renderLabelMatcher(label string, exact, regex []string, negated, matchAll, keepReferences bool) string {
	isKept := func(v string) bool { return keepReferences && variableReference.MatchString(v) }
	exactOp, regexOp := "=", "=~"
	if negated {
		exactOp, regexOp = "!=", "!~"
	}
	if matchAll {
		return label + regexOp + `".*"`
	}
	switch {
	case len(exact) == 1 && len(regex) == 0:
		v := exact[0]
		if !isKept(v) {
			v = escapePromQLString(v)
		}
		return label + exactOp + `"` + v + `"`
	case len(exact) == 0 && len(regex) == 1:
		return label + regexOp + `"` + escapePromQLString(regex[0]) + `"`
	case len(exact) == 0 && len(regex) == 0:
		return ""
	}
	alts := make([]string, 0, len(exact)+len(regex))
	for _, v := range exact {
		if isKept(v) {
			alts = append(alts, v)
		} else {
			alts = append(alts, escapePromQLString(escapeRegexLiteral(v)))
		}
	}
	for _, v := range regex {
		alts = append(alts, escapePromQLString(v))
	}
	return label + regexOp + `"(?:` + strings.Join(alts, "|") + `)"`
}

// ---- the chain (render.ts)

// defaultRangeTime is the window a stored `over: ""` renders. Panels saved
// before windows had a default carry the empty string.
const defaultRangeTime = "$__rate_interval"

// exprShape decides whether the next step parenthesises the text so far. Only
// a binary expression does, and only before an operator or a window: the
// builder means (m > 5) * 2, and rate(m + 1[5m]) would put the window on the 1.
type exprShape int

const (
	shapeSelector exprShape = iota
	shapeAtom
	shapeBinary
)

// expr is the text so far. A bare selector carries its offset/@ unrendered
// until a step consumes it, because they follow the window: rate(m[5m] offset 1h).
type expr struct {
	shape     exprShape
	text      string
	modifiers string
}

func (e expr) consume() string {
	if e.shape == shapeSelector {
		return e.text + e.modifiers
	}
	return e.text
}

func (e expr) operand() string {
	if e.shape == shapeBinary {
		return "(" + e.text + ")"
	}
	return e.consume()
}

func renderSelectorModifiers(m *SelectorModifiers) string {
	if m == nil {
		return ""
	}
	out := ""
	if m.Offset != "" {
		out += " offset " + m.Offset
	}
	if len(m.At) > 0 && string(m.At) != "null" {
		out += " @ " + scalarText(m.At)
	}
	return out
}

func renderChain(selector string, functions []BuilderFunction, mods *SelectorModifiers) (string, error) {
	cat, err := loadRenderCatalog()
	if err != nil {
		return "", fmt.Errorf("function catalog: %w", err)
	}
	e := expr{shape: shapeSelector, text: selector, modifiers: renderSelectorModifiers(mods)}
	for _, fn := range functions {
		spec, ok := cat[fn.Name]
		if !ok {
			return "", fmt.Errorf("unknown function %q", fn.Name)
		}
		e = renderStep(e, fn, spec)
	}
	return e.consume(), nil
}

// step is a stored function resolved against its spec: values keyed by
// canonical param name, the first stored value winning, non-optional params
// filled from their defaults.
type step struct {
	name   string
	spec   catalogFunction
	values map[string]json.RawMessage
}

// paramOf finds the param an arg_name addresses, by name or alias. Aliases are
// older stored names for the same param, such as "to" for greater's "than".
func paramOf(spec catalogFunction, argName string) *catalogParam {
	for i := range spec.Params {
		p := &spec.Params[i]
		if p.Name == argName {
			return p
		}
		for _, a := range p.Aliases {
			if a == argName {
				return p
			}
		}
	}
	return nil
}

func toStep(fn BuilderFunction, spec catalogFunction) step {
	values := map[string]json.RawMessage{}
	for _, arg := range fn.Arguments {
		if p := paramOf(spec, arg.Name); p != nil {
			if _, seen := values[p.Name]; !seen {
				values[p.Name] = arg.Value
			}
		}
	}
	for _, p := range spec.Params {
		if _, ok := values[p.Name]; !ok && !p.Optional && len(p.Default) > 0 {
			values[p.Name] = p.Default
		}
	}
	return step{name: fn.Name, spec: spec, values: values}
}

// placed is one param with the value the step holds for it.
type placed struct {
	param catalogParam
	value json.RawMessage
}

// at lists the params that render at one position, in catalog order. Within a
// position, catalog order is render order.
func (s step) at(position string) []placed {
	var out []placed
	for _, p := range s.spec.Params {
		if v, ok := s.values[p.Name]; ok && p.At == position {
			out = append(out, placed{p, v})
		}
	}
	return out
}

// flag reports whether a flag param at position is set to true. Flags render
// as keywords (bool, keep_metric_names), not as argument values.
func (s step) flag(position string) bool {
	for _, v := range s.at(position) {
		if string(bytes.TrimSpace(v.value)) == "true" {
			return true
		}
	}
	return false
}

// legacyTransformArgs are the argument names the pre-catalog transform tuple
// allowed.
var legacyTransformArgs = map[string]bool{"min": true, "max": true, "to_nearest": true, "quantile": true}

// matchesLegacyTransform: the pre-catalog transform tuple, any number of
// numeric {min|max|to_nearest|quantile} arguments (empty included).
func matchesLegacyTransform(fn BuilderFunction) bool {
	for _, a := range fn.Arguments {
		var f float64
		if !legacyTransformArgs[a.Name] || json.Unmarshal(a.Value, &f) != nil {
			return false
		}
	}
	return true
}

// renderStep applies one function to the text so far. A transform stored in
// the pre-catalog shape keeps rendering the way it always did; everything else
// renders by the catalog's form: call, infix operator, or identity (no-op).
func renderStep(e expr, fn BuilderFunction, spec catalogFunction) expr {
	if spec.Legacy == "transform" && matchesLegacyTransform(fn) {
		return expr{shape: shapeAtom, text: renderLegacyTransform(e, fn, spec)}
	}
	s := toStep(fn, spec)
	switch spec.Form {
	case "infix":
		return expr{shape: shapeBinary, text: renderInfix(e, s)}
	case "call":
		return expr{shape: shapeAtom, text: renderCall(e, s)}
	}
	return e // identity
}

// renderLegacyTransform renders a legacy document in stored argument order, so
// byte identity never depends on catalog order.
func renderLegacyTransform(e expr, fn BuilderFunction, spec catalogFunction) string {
	if fn.Name == "histogram_quantile" {
		var phi json.RawMessage
		for _, a := range fn.Arguments {
			if a.Name == "quantile" {
				phi = a.Value
				break
			}
		}
		if phi == nil {
			if p := paramOf(spec, "quantile"); p != nil {
				phi = p.Default
			}
		}
		return "histogram_quantile(" + scalarText(phi) + ", " + e.consume() + ")"
	}
	args := []string{e.consume()}
	for _, a := range fn.Arguments {
		args = append(args, scalarText(a.Value))
	}
	return fn.Name + "(" + strings.Join(args, ", ") + ")"
}

// renderInfix renders `q op v`, with "bool" after the operator when set. With
// keep_metric_names the whole expression is parenthesised first.
func renderInfix(e expr, s step) string {
	op := s.spec.Callee
	if op == "" {
		op = s.name
	}
	if s.flag("bool") {
		op += " bool"
	}
	rhs := ""
	if v := s.at("rhs"); len(v) > 0 {
		rhs = renderArgValue(v[0].param, v[0].value)
	}
	text := e.operand() + " " + op + " " + rhs
	if s.flag("suffix") {
		return "(" + text + ") keep_metric_names"
	}
	return text
}

// rendered spells the values at position, dropping empty ones so an empty
// list never leaves a dangling comma.
func (s step) rendered(position string) []string {
	var out []string
	for _, v := range s.at(position) {
		if t := renderArgValue(v.param, v.value); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// renderCall renders name(before…, series, after…). A callee template such as
// "topk_{by}" takes its value from the callee param, a non-empty by/without list
// becomes the grouping clause, and limit and keep_metric_names trail the call.
func renderCall(e expr, s step) string {
	args := append(append(s.rendered("before"), seriesArgument(e, s)), s.rendered("after")...)

	callee := s.spec.Callee
	if callee == "" {
		callee = s.name
	}
	for _, v := range s.at("callee") {
		callee = strings.Replace(callee, "{"+v.param.Name+"}", renderArgValue(v.param, v.value), 1)
	}

	text := callee + "(" + strings.Join(args, ", ") + ")"
	for _, v := range s.at("grouping") {
		var labels []string
		if json.Unmarshal(v.value, &labels) == nil && len(labels) > 0 {
			text = callee + " " + v.param.Name + " (" + strings.Join(labels, ", ") + ") (" + strings.Join(args, ", ") + ")"
			break
		}
	}
	for _, v := range s.at("limit") {
		text += " limit " + scalarText(v.value)
	}
	if s.flag("suffix") {
		text += " keep_metric_names"
	}
	return text
}

// seriesArgument is the call's series argument with its window, or with a
// subquery step alone (q[:1m]). A stored window of "" renders the default.
func seriesArgument(e expr, s step) string {
	window, subStep := s.at("window"), s.at("step")
	if len(window) == 0 && len(subStep) == 0 {
		return e.consume()
	}
	rng := ""
	if len(window) > 0 {
		rng = scalarText(window[0].value)
		if rng == "" {
			rng = defaultRangeTime
		}
	}
	if len(subStep) > 0 {
		rng += ":" + scalarText(subStep[0].value)
	}
	if e.shape == shapeSelector {
		return e.text + "[" + rng + "]" + e.modifiers
	}
	return e.operand() + "[" + rng + "]"
}

// scalarText is JavaScript's String(value) for a JSON string, number or bool.
func scalarText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.TrimSpace(string(raw))
}

// quote is JSON.stringify for a string: no HTML escaping.
func quote(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// renderArgValue spells one value by its param kind: strings and label names
// are JSON-quoted, numbers and windows bare. Flags render nothing here; flag()
// reads them.
func renderArgValue(p catalogParam, raw json.RawMessage) string {
	switch p.Kind.Kind {
	case "number", "window", "duration", "timestamp":
		return scalarText(raw)
	case "numbers":
		var ns []json.RawMessage
		_ = json.Unmarshal(raw, &ns)
		out := make([]string, len(ns))
		for i, n := range ns {
			out[i] = scalarText(n)
		}
		return strings.Join(out, ", ")
	case "string":
		return quote(scalarText(raw))
	case "enum":
		if p.Kind.Quoted {
			return quote(scalarText(raw))
		}
		return scalarText(raw)
	case "labels":
		var ls []string
		_ = json.Unmarshal(raw, &ls)
		return joinQuoted(ls)
	case "pairs":
		var pairs [][]string
		_ = json.Unmarshal(raw, &pairs)
		var flat []string
		for _, pr := range pairs {
			flat = append(flat, pr...)
		}
		return joinQuoted(flat)
	case "rollups":
		var rs []string
		_ = json.Unmarshal(raw, &rs)
		return "(" + joinQuoted(rs) + ")"
	}
	return "" // flag
}

func joinQuoted(items []string) string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = quote(s)
	}
	return strings.Join(out, ", ")
}
