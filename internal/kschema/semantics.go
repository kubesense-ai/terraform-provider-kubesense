package schema

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/presetwalk"
)

// This file holds every rule that spans more than one field.
//
// They are here rather than in CUE because CUE cannot express them, and the
// attempt to make it produced errors that named nothing: "a metrics query needs
// promql or selectedMetric" written as a disjunction rejected VALID payloads as
// incomplete — real queries carry both fields, so both arms matched — and when
// an arm did fail, the message dumped the entire struct instead of the field.
// Each rule below carries a written message and a JSON Pointer instead.
//
// Every rule is named, and its name lands in Error.Rule, so a caller can branch
// on which check failed without matching on message text.

// checkSemantics runs the cross-field rules for a format. It assumes the
// document is already shape-valid, which is why it can index and type-assert
// without reporting every miss.
func checkSemantics(f Format, document []byte) error {
	var doc any
	if err := json.Unmarshal(document, &doc); err != nil {
		return Errors{{Message: "document is not valid JSON: " + err.Error(), Rule: "json"}}
	}

	var errs Errors
	switch f {
	case AlertRule:
		errs = checkAlertRule(object(doc))
	case Panel:
		errs = checkPanel(object(doc), "")
	case PanelQuery:
		// A single query in isolation. Only the rules about one query alone apply;
		// the rest compare a query against siblings it does not have.
		q := object(doc)
		errs = checkInfrastructureQueries([]queryRef{{mode: text(q["selectedMode"]), raw: q}})
	case DashboardPreset:
		errs = checkPreset(object(doc))
	}
	return errs.orNil()
}

// ── alert rules ─────────────────────────────────────────────────────────────

// checkAlertRule applies every alert-level rule to one document.
func checkAlertRule(doc map[string]any) Errors {
	queries := alertQueries(doc)

	var errs Errors
	errs = append(errs, checkQueryTypeAgrees(doc, queries)...)
	errs = append(errs, checkMetricQueryLabel(doc, queries)...)
	errs = append(errs, checkBreachesCount(doc)...)
	errs = append(errs, checkMetricsQueryNamesAMetric(queries)...)
	errs = append(errs, checkAggregationHasFields(queries)...)
	errs = append(errs, checkFormulaGraph(queries)...)
	errs = append(errs, checkChangeHasComparedTo(doc)...)
	errs = append(errs, checkAggregationFieldType(queries)...)
	return errs
}

// checkChangeHasComparedTo requires a change-style condition to say what it
// changed from. change, change_percent and new_value all compare the current
// value against an earlier window; with no compared_to there is nothing to
// compare against and the rule evaluates a change of nothing.
func checkChangeHasComparedTo(doc map[string]any) Errors {
	switch text(doc["condition_type"]) {
	case "change", "change_percent", "new_value":
	default:
		return nil
	}
	if text(doc["compared_to"]) != "" {
		return nil
	}
	return Errors{{
		Path:    "/compared_to",
		Message: "required when condition_type is " + text(doc["condition_type"]) + " — the earlier window to compare the current value against, for example \"1h\"",
		Rule:    "compared_to",
	}}
}

// checkAggregationFieldType requires the operand of a numeric aggregation to be
// typed float. Anything else is read as unset by the engine, which silently
// turns a p95 into a row count. That includes an absent type: the contract
// defaults a missing #Field type to "string", but semantics run on the document
// as written, before that default, so a missing type has to be caught here.
// unique_count is exempt: it counts distinct values of a string or a number.
//
// Checked against every stored rule on the dev instance before tightening: all 19
// numeric-aggregation queries already carry type "float".
func checkAggregationFieldType(queries []queryRef) Errors {
	var errs Errors
	for _, q := range queries {
		if q.mode != "logs" && q.mode != "traces" {
			continue
		}
		operation := text(q.raw["value_operation"])
		if !aggregationsNeedingAField[operation] || operation == "unique_count" {
			continue
		}
		fields, _ := q.raw["fields"].([]any)
		for i, f := range fields {
			fm, ok := f.(map[string]any)
			if !ok {
				continue
			}
			typ := text(fm["type"])
			if typ == "float" {
				continue
			}
			got := `"` + typ + `"`
			if typ == "" {
				got = "missing"
			}
			errs = append(errs, Error{
				Path:    fmt.Sprintf("%s/fields/%d/type", q.path, i),
				Message: `value_operation ` + operation + ` needs a numeric field: type must be "float", got ` + got + ` — any other type degrades the rule to row_count`,
				Rule:    "aggregation_field_type",
			})
		}
	}
	return errs
}

// aggregationsNeedingAField is every value_operation that aggregates OVER a
// column, as opposed to counting rows. Mirrors NUMERIC_AGGREGATION_FUNCTIONS
// plus unique_count (zod-schemas.ts:151, :215).
var aggregationsNeedingAField = map[string]bool{
	"unique_count": true,
	"avg":          true, "sum": true, "min": true, "max": true,
	"p99": true, "p95": true, "p90": true, "p75": true, "p50": true,
}

// checkAggregationHasFields requires a logs or traces query to name the column
// its aggregation runs over.
//
// row_count counts matching rows and takes no field; everything else needs one,
// or the engine has nothing to average and the rule evaluates nothing.
//
// This is a cross-field rule and therefore lives here rather than in the CUE. On
// the panel FORM shape the same constraint IS expressible, because the zod nests
// it — `aggregation: {function, fields}` is a discriminated union, so the arm
// requiring fields is selected by the function. The alert wire format flattens
// that pair into sibling members, and a disjunction over siblings inside an
// otherwise open struct rejects valid documents as "incomplete" (see the note on
// #QueryConfig in cue/alert/rule.cue).
//
// Both producers that actually write one of these enforce it already — the MCP
// tool ("fields is required when value_operation is …") and the editor's zod — so
// a document accepted here would have been rejected by whoever generated it.
func checkAggregationHasFields(queries []queryRef) Errors {
	var errs Errors
	for _, q := range queries {
		if q.mode != "logs" && q.mode != "traces" {
			continue
		}
		operation := text(q.raw["value_operation"])
		if !aggregationsNeedingAField[operation] {
			continue
		}
		if fields, ok := q.raw["fields"].([]any); ok && len(fields) > 0 {
			continue
		}
		errs = append(errs, Error{
			Path: q.path + "/fields",
			Message: "value_operation " + operation +
				" aggregates over a field but none is set — add fields, or use row_count to count rows",
			Rule: "aggregation_fields",
		})
	}
	return errs
}

// checkQueryTypeAgrees requires the rule's signal and its queries to describe
// the same thing.
//
// query_type is what the engine routes on and selectedMode is what each query
// runs against, so a rule declaring "metrics" over a logs query is routed to a
// backend that cannot answer it. Formula queries are exempt: they compose the
// others by label and belong to no signal of their own.
func checkQueryTypeAgrees(doc map[string]any, queries []queryRef) Errors {
	queryType := text(doc["query_type"])
	if queryType == "" {
		return nil
	}

	var errs Errors
	matched := false
	for _, q := range queries {
		if q.mode == "formula" {
			continue
		}
		if q.mode == queryType {
			matched = true
			continue
		}
		errs = append(errs, Error{
			Path: q.path + "/selectedMode",
			Message: fmt.Sprintf(
				"query is %q but the rule's query_type is %q — they must describe the same signal",
				q.mode, queryType),
			Rule: "query_type",
		})
	}
	if !matched && queryType != "formula" && len(queries) > 0 {
		errs = append(errs, Error{
			Path:    "/query_config",
			Message: fmt.Sprintf("rule's query_type is %q but no query runs against %s", queryType, queryType),
			Rule:    "query_type",
		})
	}
	return errs
}

// checkMetricQueryLabel requires the thresholded query to exist.
//
// metric_query_label selects WHICH query's series the threshold applies to. A
// rule naming a label no query carries evaluates nothing and can never fire — it
// does not error, it just sits there looking healthy, which is the specific
// failure this repo was built after: metric_query_label "A" against a query
// labelled "B".
//
// Only checked when at least one query is labelled. `label` is optional on the
// wire — kubeapi marks it omitempty and the webapp's own type omits it — and
// consumers fall back to labelOptions.label and then to the array index. Where
// there is nothing to match against, silence is better than rejecting documents
// the editor itself wrote.
func checkMetricQueryLabel(doc map[string]any, queries []queryRef) Errors {
	want := text(doc["metric_query_label"])
	if want == "" {
		return nil
	}

	labelled := make([]string, 0, len(queries))
	for _, q := range queries {
		if q.label != "" {
			labelled = append(labelled, q.label)
		}
	}
	if len(labelled) == 0 {
		return nil
	}
	for _, label := range labelled {
		if label == want {
			return nil
		}
	}
	return Errors{{
		Path: "/metric_query_label",
		Message: fmt.Sprintf(
			"names query %q, but the rule's queries are labelled %s — the threshold would apply to nothing and the rule could never fire",
			want, quoteList(labelled)),
		Rule: "metric_query_label",
	}}
}

// checkBreachesCount requires a breach count when the rule fires on repeats.
//
// frequency_type "more_than_once" means "fire after N breaches in the window",
// and N has nowhere to come from otherwise. The editor's minimum is 2, because
// "more than once" with a count of 1 is just "at_least_once" under another name.
func checkBreachesCount(doc map[string]any) Errors {
	if text(doc["frequency_type"]) != "more_than_once" {
		return nil
	}
	count, ok := number(doc["breaches_count"])
	if !ok {
		return Errors{{
			Path:    "/breaches_count",
			Message: `required when frequency_type is "more_than_once" — the rule has no breach count to fire on`,
			Rule:    "breaches_count",
		}}
	}
	if count < 2 {
		return Errors{{
			Path:    "/breaches_count",
			Message: fmt.Sprintf(`must be at least 2 when frequency_type is "more_than_once", got %v — a count of 1 is "at_least_once"`, count),
			Rule:    "breaches_count",
		}}
	}
	return nil
}

// A rule with no channels is deliberately NOT refused (KUBE-2852: import
// demanded a channel the editor leaves optional). It used to be, as "notifies
// nobody", but that was not knowable from the rule: an alert carrying no
// notification_channel_id falls through to every channel's label routes
// (route_match, route_all) in the generated Alertmanager config, whatever
// route_by_labels says — so whether it reaches anyone depends on the channels,
// not the rule. The editor saves such rules; import now accepts them too.

// ── panel and dashboard rules ───────────────────────────────────────────────

// checkPreset applies the tab rules, and the panel rules to every panel the
// dashboard holds, on every page and inside each page's sub-grids.
func checkPreset(doc map[string]any) Errors {
	errs := checkTabs(doc)
	for _, p := range presetwalk.Panels(doc) {
		errs = append(errs, checkPanel(p.Object, p.Pointer)...)
	}
	return errs
}

// checkPanel applies the cross-query rules to one panel's queries.
func checkPanel(doc map[string]any, prefix string) Errors {
	queries := panelQueries(doc, prefix)
	var errs Errors
	errs = append(errs, checkMetricsQueryNamesAMetric(queries)...)
	errs = append(errs, checkFormulaGraph(queries)...)
	errs = append(errs, checkHostMapQuery(doc, prefix, queries)...)
	errs = append(errs, checkInfrastructureQueries(queries)...)
	return errs
}

// checkMetricsQueryNamesAMetric requires a metrics query to say what it queries.
//
// Either a selectedMetric in builder mode or a promql expression in code mode;
// with neither there is no query at all. Expressed as "one of the two" rather
// than keyed off queryMode on purpose — a rule that arrived with queryMode
// "promql", which is not a legal value, carried a perfectly good promql string,
// and the point is to catch a query naming NOTHING rather than to police which
// field goes with which mode.
//
// Worth checking before defaults are applied and worthless after: an absent
// selectedMetric defaults to a real, unrelated metric name.
func checkMetricsQueryNamesAMetric(queries []queryRef) Errors {
	var errs Errors
	for _, q := range queries {
		if q.mode != "metrics" {
			continue
		}
		if text(q.raw["selectedMetric"]) != "" || text(q.raw["promql"]) != "" {
			continue
		}
		errs = append(errs, Error{
			Path:    q.path,
			Message: "metrics query names no metric: set selectedMetric, or promql for a code-mode query",
			Rule:    "metrics_query_source",
		})
	}
	return errs
}

// ── host maps ───────────────────────────────────────────────────────────────

// infrastructureHierarchy is the host map's nesting, outermost first. A query's
// levels follow it, skipping any, never repeating or going back up.
var infrastructureHierarchy = []string{"host", "pod", "container"}

// infrastructureSignals is what each entity can be filled or sized by: the
// signals kubeapi's infrastructure catalog has a PromQL template for, in at
// least one metric family. A host has no restarts; only a host counts pods.
var infrastructureSignals = map[string][]string{
	"host":      {"cpu_utilization", "memory_utilization", "cpu_usage", "memory_usage", "pod_count", "ready"},
	"pod":       {"cpu_utilization", "memory_utilization", "cpu_usage", "memory_usage", "restarts", "ready"},
	"container": {"cpu_utilization", "memory_utilization", "cpu_usage", "memory_usage", "restarts", "ready"},
}

// infrastructureGroupBy is what a host map whose outermost level is each entity
// can be split into sections by: the dimensions that level's rows carry, less
// its own name. A host row has no namespace, and grouping hosts by host draws a
// section per tile.
var infrastructureGroupBy = map[string][]string{
	"host":      {"cluster", "env_type"},
	"pod":       {"cluster", "env_type", "host", "namespace", "workload"},
	"container": {"cluster", "env_type", "host", "namespace", "workload", "pod"},
}

// infrastructureFilters is what a host map's Basic filter can narrow by. A
// container is absent: no metric that places a container also names its host,
// so it could not narrow the levels around it. advanced_query holds the
// Advanced filter's text, which kubeapi's MCP tools parse and check.
var infrastructureFilters = []string{"cluster", "host", "namespace", "workload", "pod", "advanced_query"}

// checkHostMapQuery pairs the hostMap panel with the infrastructure query. A
// host map draws its first query and nothing else, and no other panel type can
// draw an infrastructure query, so either mismatch renders an empty panel.
func checkHostMapQuery(doc map[string]any, prefix string, queries []queryRef) Errors {
	if text(doc["panelType"]) != "hostMap" {
		var errs Errors
		for _, q := range queries {
			if q.mode == "infrastructure" {
				errs = append(errs, Error{
					Path:    q.path + "/selectedMode",
					Message: `an infrastructure query only draws in a "hostMap" panel — set panelType to "hostMap"`,
					Rule:    "host_map_query",
				})
			}
		}
		return errs
	}
	if len(queries) != 1 {
		return Errors{{
			Path:    prefix + "/queries",
			Message: fmt.Sprintf("a host map takes exactly one infrastructure query, got %d", len(queries)),
			Rule:    "host_map_query",
		}}
	}
	if queries[0].mode != "infrastructure" {
		return Errors{{
			Path:    queries[0].path + "/selectedMode",
			Message: `a host map's query must be "infrastructure", got "` + queries[0].mode + `"`,
			Rule:    "host_map_query",
		}}
	}
	return nil
}

// checkInfrastructureQueries checks each infrastructure query's levels against
// the hierarchy and the signals each entity has, and its grouping against what
// the outermost level carries. kubeapi's /explore/hostmap runs these same rules
// on every request, so a host map that saves is one that runs.
func checkInfrastructureQueries(queries []queryRef) Errors {
	var errs Errors
	for _, q := range queries {
		if q.mode != "infrastructure" {
			continue
		}
		levels := list(q.raw["levels"])
		deepest := -1
		for i, l := range levels {
			level := object(l)
			at := fmt.Sprintf("%s/levels/%d", q.path, i)
			entity := text(level["entity"])
			depth := slices.Index(infrastructureHierarchy, entity)
			if depth <= deepest {
				errs = append(errs, Error{
					Path:    at + "/entity",
					Message: fmt.Sprintf("%s cannot sit inside %s — levels nest host > pod > container, each deeper than the one before", entity, text(object(levels[i-1])["entity"])),
					Rule:    "infrastructure_levels",
				})
				continue
			}
			deepest = depth
			for _, role := range []string{"fill", "size"} {
				signal := text(level[role])
				if signal == "" || slices.Contains(infrastructureSignals[entity], signal) {
					continue
				}
				errs = append(errs, Error{
					Path:    at + "/" + role,
					Message: fmt.Sprintf("a %s has no %s; use one of %s", entity, signal, quoteList(infrastructureSignals[entity])),
					Rule:    "infrastructure_signal",
				})
			}
			if text(level["size"]) != "" && i < len(levels)-1 {
				errs = append(errs, Error{
					Path:    at + "/size",
					Message: "only the innermost level is sized; an outer level's tile is as big as what it holds",
					Rule:    "infrastructure_size",
				})
			}
		}
		for _, key := range slices.Sorted(maps.Keys(object(q.raw["filters"]))) {
			if slices.Contains(infrastructureFilters, key) {
				continue
			}
			errs = append(errs, Error{
				Path:    q.path + "/filters/" + key,
				Message: fmt.Sprintf("a host map cannot filter on %s; use %s", key, quoteList(infrastructureFilters[:5])),
				Rule:    "infrastructure_filter",
			})
		}
		if len(levels) == 0 {
			continue
		}
		outer := text(object(levels[0])["entity"])
		seen := map[string]bool{}
		for i, d := range list(q.raw["groupByDimensions"]) {
			dim := text(d)
			at := fmt.Sprintf("%s/groupByDimensions/%d", q.path, i)
			switch {
			case seen[dim]:
				errs = append(errs, Error{Path: at, Message: dim + " is already grouped by", Rule: "infrastructure_group_by"})
			case !slices.Contains(infrastructureGroupBy[outer], dim):
				errs = append(errs, Error{
					Path:    at,
					Message: fmt.Sprintf("a map of %ss cannot be grouped by %s; use %s", outer, dim, quoteList(infrastructureGroupBy[outer])),
					Rule:    "infrastructure_group_by",
				})
			}
			seen[dim] = true
		}
	}
	return errs
}

// ── the formula dependency graph ────────────────────────────────────────────

// checkFormulaGraph validates formula queries against the queries they compose.
//
// A transliteration of the webapp's superRefine (zod-schemas.ts:1036), quirks
// included, because a panel this rejects and one the editor rejects have to be
// the same set:
//
//   - the token pattern is /[A-Za-z]+/ — LETTERS ONLY. A query labelled "A1" is
//     referenced as "A", and "A1" never matches anything. Faithful, not a slip.
//   - the traversal is an ORDERING check, not a cycle check. Nodes are visited in
//     query order, and an edge to a node not yet visited is reported as
//     "references X before it is defined". So a formula may only reference
//     queries that appear EARLIER in the array — which forbids cycles, but also
//     forbids acyclic forward references the engine might have handled.
//
// The one rule not carried over is mathjs's parse, which needs an expression
// parser. Balanced parentheses is the subset of it worth having without one: it
// rejects nothing mathjs accepts, so the two cannot disagree in the direction
// that matters.
func checkFormulaGraph(queries []queryRef) Errors {
	if len(queries) == 0 {
		return nil
	}

	// Insertion-ordered, mirroring the JS object the original builds. A repeated
	// label overwrites its edges but keeps its original position, as it would
	// there.
	var order []string
	edges := map[string][]string{}
	byLabel := map[string]queryRef{}

	var errs Errors
	for _, q := range queries {
		if _, seen := edges[q.label]; !seen {
			order = append(order, q.label)
		}
		byLabel[q.label] = q

		if q.mode != "formula" {
			// A plain query is its own edge, so that referencing it resolves.
			edges[q.label] = []string{q.label}
			continue
		}

		if strings.TrimSpace(q.expression) == "" {
			errs = append(errs, Error{
				Path:    q.path + "/expression",
				Message: "formula query has no expression",
				Rule:    "formula_expression",
			})
		}
		if !balanced(q.expression) {
			errs = append(errs, Error{
				Path:    q.path + "/expression",
				Message: "expression has unbalanced parentheses",
				Rule:    "formula_expression",
			})
		}

		refs := references(q.expression)
		edges[q.label] = refs

		if len(refs) == 0 && strings.TrimSpace(q.expression) != "" {
			errs = append(errs, Error{
				Path:    q.path + "/expression",
				Message: "formula references no query — an expression of only numbers and operators computes nothing about this panel",
				Rule:    "formula_expression",
			})
		}
		for _, ref := range refs {
			if ref == q.label {
				errs = append(errs, Error{
					Path:    q.path + "/expression",
					Message: fmt.Sprintf("formula %q references itself", q.label),
					Rule:    "formula_cycle",
				})
			}
		}
	}

	visited := map[string]bool{}
	for _, node := range order {
		if visited[node] {
			continue
		}
		visited[node] = true
		for _, ref := range edges[node] {
			origin := byLabel[node]
			if _, known := edges[ref]; !known {
				errs = append(errs, Error{
					Path: origin.path + "/expression",
					Message: fmt.Sprintf(
						"formula %q references %q, which is not a query in this panel", node, ref),
					Rule: "formula_reference",
				})
				continue
			}
			if !visited[ref] {
				path := origin.path
				if origin.mode == "formula" {
					path += "/expression"
				}
				errs = append(errs, Error{
					Path:    path,
					Message: fmt.Sprintf("query %q references %q before it is defined", node, ref),
					Rule:    "formula_cycle",
				})
			}
		}
	}
	return errs
}

// references extracts the query labels an expression mentions, using the
// original's letters-only pattern. Duplicates are dropped, order preserved.
func references(expression string) []string {
	var out []string
	seen := map[string]bool{}
	start := -1
	for i := 0; i <= len(expression); i++ {
		isLetter := i < len(expression) &&
			(expression[i] >= 'a' && expression[i] <= 'z' || expression[i] >= 'A' && expression[i] <= 'Z')
		switch {
		case isLetter && start < 0:
			start = i
		case !isLetter && start >= 0:
			if token := expression[start:i]; !seen[token] {
				seen[token] = true
				out = append(out, token)
			}
			start = -1
		}
	}
	return out
}

// balanced reports whether an expression's parentheses match.
func balanced(expression string) bool {
	depth := 0
	for i := 0; i < len(expression); i++ {
		switch expression[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// ── reading queries out of a document ───────────────────────────────────────

// queryRef is one query, flattened to what the cross-field rules need: where it
// is, what it is, and how it is referred to.
type queryRef struct {
	// path is the JSON Pointer to the query object itself.
	path string
	// label is what a formula uses to reference it.
	label string
	// mode is selectedMode: metrics, logs, traces or formula.
	mode string
	// expression is the formula body, empty for every other mode.
	expression string
	// raw is the query object, for rules needing a field not lifted out here.
	raw map[string]any
}

// alertQueries reads an alert document's query_config[].
//
// The label falls back to labelOptions.label, matching what consumers do when
// kubeapi's omitempty leaves `label` off a stored rule.
func alertQueries(doc map[string]any) []queryRef {
	entries := list(doc["query_config"])
	out := make([]queryRef, 0, len(entries))
	for i, entry := range entries {
		q := object(entry)
		label := text(q["label"])
		if label == "" {
			label = text(object(q["labelOptions"])["label"])
		}
		out = append(out, queryRef{
			path:       fmt.Sprintf("/query_config/%d", i),
			label:      label,
			mode:       text(q["selectedMode"]),
			expression: text(q["expression"]),
			raw:        q,
		})
	}
	return out
}

// panelQueries reads a panel's queries[].
func panelQueries(doc map[string]any, prefix string) []queryRef {
	entries := list(doc["queries"])
	out := make([]queryRef, 0, len(entries))
	for i, entry := range entries {
		q := object(entry)
		out = append(out, queryRef{
			path:       fmt.Sprintf("%s/queries/%d", prefix, i),
			label:      text(q["label"]),
			mode:       text(q["selectedMode"]),
			expression: text(q["expression"]),
			raw:        q,
		})
	}
	return out
}

// ── typed access to a decoded document ──────────────────────────────────────
//
// The document arrives as map[string]any because unknown fields must survive,
// the same reason the migration chain carries one. These readers return the zero
// value for a missing or wrong-typed member rather than reporting: shape
// validation has already run and said so, and repeating it here would report
// every problem twice.

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func text(v any) string {
	s, _ := v.(string)
	return s
}

func number(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func truth(v any) bool {
	b, _ := v.(bool)
	return b
}

// quoteList renders labels for a message: "A", "B" and "C".
func quoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = `"` + item + `"`
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}
