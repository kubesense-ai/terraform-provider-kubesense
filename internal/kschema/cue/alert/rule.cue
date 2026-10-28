// Package alert defines the alert-rule document as exchanged on the wire: what
// the Export button writes, what bulk import and the kubesense-alerts skill
// produce, what GET /api/alerts/rules/{id} returns, and what the alert editor
// hydrates from.
//
// This is NOT the POST body. That is a different shape — `query` instead of
// `query_config`, a nested `threshold` object, camelCase `notificationChannels`,
// durations as `*_prometheus_format` — related to this one by
// convertImportedRuleToPosting in the webapp. Modelling it is separate work.
//
// An alert's query_config[] is NOT a panel query, despite being derived from one
// by the editor. The wire form flattens what the panel form nests and carries
// members the form has no concept of:
//
//	panel form (cue/panel)          alert wire (here)
//	aggregation: {function, fields} value_operation + fields, as siblings
//	filters (editor store shape)    unified_filter (compiled tree) + raw_filters
//	—                               selectedMeasurement, limit, label
//	pagination, filterMode,         —
//	chart_type, xaxis/yaxis/…
//
// So the shared primitives are imported from the panel package — #Field,
// #QueryFunction, the aggregation function set — and the query container is
// defined here against the two authorities for this shape:
//
//	webapp   src/pages/alert-config/type.ts:548  (TAlertConfig)
//	kubeapi  observability/alertmanager/rule-engine/pdata.go:180  (QueryConfig)
//	         observability/alertmanager/rule-engine/pdata.go:135  (AlertCreationRequest)
//
// Where the two disagree, kubeapi wins on presence (it is what actually persists
// and returns the document) and the webapp wins on legal values (it is what
// renders them). Each such case is commented at the field.
package alert

import "go.kubesense.ai/api/schema/cue/panel"

// #QueryConfig is one entry of query_config[].
//
// Almost everything is optional because kubeapi's struct tags are almost all
// `omitempty` (pdata.go:180-206) — a stored rule genuinely arrives without
// variables, filters, raw_filters or visible, so requiring them would reject
// documents the editor itself wrote.
#QueryConfig: {
	// Names the query so metric_query_label and formula expressions can reference
	// it. Present on kubeapi's struct (`label,omitempty`) but ABSENT from the
	// webapp's TAlertConfig.query_config type entirely — a gap in the TS type, not
	// in the data. Consumers that miss it fall back to labelOptions.label and then
	// to the array index.
	label?: string

	selectedMode!: panel.#SelectedMode | "slo"

	// ── metrics ──
	// queryMode defaults to builder because that is what consumers assume when it
	// is absent (helper.ts:701 `query?.queryMode || "builder"`, mirroring the
	// zod's .catch("builder")). A producer wrote "promql" here — not a legal value
	// — and the editor therefore fell into builder mode, ignored the PromQL, and
	// rendered its placeholder metric.
	queryMode?:      panel.#QueryMode
	promql?:         string
	selectedMetric?: string

	// The optional lists on this document accept `null` as well as an array,
	// because that is what a Go producer actually sends. A nil slice marshals to
	// `null`, not to `[]` or to nothing, unless every struct tag carries
	// omitempty — and kubeapi's QueryConfig.Fields does not (pdata.go), so every
	// rule the MCP create-alert tool builds carries `"fields": null`.
	//
	// Rejecting that meant the tool could not produce a single valid rule, and
	// because shape is checked before semantics, the one shape error masked every
	// cross-field finding behind it. null and absent mean the same thing here —
	// no fields — so accepting both costs nothing and is what the producers on
	// this contract emit.
	functions?: null | [...panel.#QueryFunction]

	// ── logs / traces ──
	// The flattened form of the panel's nested aggregation. TAlertConfig types
	// this as a bare `string`; the legal set is the aggregation function names,
	// per the editor's own mapping (helper.ts:167-198) and the MCP tool's
	// validation (tool_create_alert.go:52).
	value_operation?: "row_count" | "unique_count" | panel.#NumericAggregation

	// What the engine thresholds. Derived from value_operation for logs/traces —
	// row_count and unique_count both collapse to "count" — and written as "sum"
	// for metrics.
	selectedMeasurement?: "count" | "sum" | "avg" | "min" | "max"

	fields?: null | [...panel.#Field]
	groupBy?: null | [...panel.#Field]

	// Named and OPEN rather than inline. An inline struct inherits nothing from
	// #QueryConfig's `...`, so it exported with additionalProperties:false — the
	// one closed node in an otherwise open document, which would reject any member
	// kubeapi adds here later.
	sortBy?: #SortBy

	limit?: int & >0

	// ── filters, in both forms ──
	//
	// The store shape and the compiled shape are NOT two encodings of one thing.
	// The store shape is a flat map — an implicit AND of equals/not-equals, with
	// a "-" prefix for exclusion — and cannot express grouping or OR. The
	// compiled shape is a tree, and can. So a rule authored in the advanced query
	// editor has ONLY the tree: there is no store form to derive from it, which
	// is why both keys exist and why either may be absent.
	//
	// raw_filters is the editor's store shape (key to list of values).
	raw_filters?: panel.#Filters

	// unified_filter is the key kubeapi actually STORES the compiled form under.
	// convertQueryConfig renames `filters` to `unified_filter` on the way in
	// (alertmanager/rule-engine/controller.go:952), so every stored rule carrying
	// a real predicate uses this name and none of them use the other one. The
	// contract named only `filters` for a while, which meant it validated a key
	// that never arrives and ignored the one that does.
	unified_filter?: null | #UnifiedFilter

	// filters is the same compiled object under the name it travels by on the
	// POST body, kept because bulk import and older stored rows may carry it.
	// Deliberately left open rather than #UnifiedFilter: on the panel side this
	// same key means the STORE shape instead, and this contract has to accept a
	// document from either producer.
	filters?: null | {[string]: _}

	// ── formula ──
	expression?: string

	// ── native SLO ──
	// A native SLO alert reads durable evaluation buckets from ClickHouse. Its
	// current version is resolved by the engine, never persisted on the rule.
	slo?: {
		sloId!: string & !=""
		sloName?: string
		signal!: "burn_rate" | "error_budget_balance"
		burnRateWindow?: "1h" | "2h" | "6h"
		burnRateLongWindowHours?: int & >0 & <=48
		groupMode?: "overall" | "per_group"
		...
	}

	// ── shared ──
	// On the wire this is a flat string map, not the panel form's discriminated
	// {type: auto|custom} union — the editor flattens it on save.
	labelOptions?: {[string]: string}

	variables?: null | [...string]
	visible?: bool

	is_free_search?: bool

	// Persisted by kubeapi and set by the panel editor; no alert consumer reads
	// them, but they round-trip through the document (pdata.go:186-201).
	uniqueBy?:    string
	timeFrame?:   int
	displayAs?:   string
	visualiseAs?: string
	unit?:        string
	page?:        int
	pageSize?:    int

	// The query's chart colour, set by the editor's per-query colour picker
	// (QueryColorControl) and otherwise identical to the panel form's own
	// fieldConfig — see panel.#FieldConfig for the single/palette shapes.
	fieldConfig?: panel.#FieldConfig

	...
}

// #SortBy orders the rows a logs or traces rule evaluates. The doubled name is
// the wire format's, not a transcription slip: the outer key is the member on
// #QueryConfig, the inner one is the field to sort on.
#SortBy: {
	sortBy!:   panel.#Field
	sortOrder: *"ASC" | "DESC"

	...
}

// #FilterValue is one value inside a filter predicate. Go carries these as
// []any, and they arrive as whatever the field's type is — a label string, a
// numeric threshold, a boolean flag. Spelled out rather than left as `_`
// because a list of `_` exports as a bare {type:array}, which is byte-identical
// to an item constraint that collapsed on export and so cannot be told apart
// from the bug the export gate exists to catch.
#FilterValue: string | number | bool | null

// #UnifiedFilter is kubeapi's compiled filter object (observability/common
// /pdata.go:168). Which member carries the predicate depends on `type`:
// "advanced" fills adv_filters with a tree, "common" fills common_filter with a
// flat list.
//
// Every member is optional and nullable because Go marshals a zero struct rather
// than omitting it, so a document routinely carries the arm it is not using as
// an empty object or a null. Constraining what IS present is still worth doing:
// it catches the third shape webapp can emit — a bare array — which fails to
// decode into this struct entirely rather than degrading quietly.
#UnifiedFilter: {
	type?:          string
	adv_filters?:   null | #AdvancedFilter
	common_filter?: null | [...#ChFilter]

	...
}

// #AdvancedFilter is one node of the compiled filter tree.
//
// It is recursive: `children` is what lets the tree express grouping and OR,
// which the editor's flat store shape cannot. That difference in expressive
// power is the whole reason two filter representations exist.
#AdvancedFilter: {
	operation?:  string
	field?:      string
	field_type?: string
	values?:     null | [...#FilterValue]
	type?:       string
	children?:   null | [...#AdvancedFilter]

	...
}

// #ChFilter is one entry of the flat compiled list — the degenerate case of the
// tree, used when no advanced query is involved.
#ChFilter: {
	field?:            string
	operation?:        string
	values?:           null | [...#FilterValue]
	field_type?:       string
	type?:             string
	expression_type?:  string
	composite_fields?: null | [...string]
	is_rbac_filter?:   bool

	...
}

// #PromDuration is a Prometheus duration: one or more <number><unit> pairs.
// Worth constraining because the plausible mistakes an agent makes here — "5",
// "5 minutes", "300s" meaning five minutes — all parse as strings and only
// surface as a rule evaluating over the wrong window.
#PromDuration: string & =~"^([0-9]+(ms|s|m|h|d|w|y))+$"

// Several duration fields are legitimately blank on stored rules; the editor
// substitutes its own fallback when hydrating. Blank has to stay valid.
#PromDurationOrEmpty: "" | #PromDuration

// #ThresholdOperator is the comparison applied to each series.
//
// "above" and "below" are deliberately absent. In this product they are display
// labels the editor renders for greater_than and less_than
// (alert-config-form.tsx:415) — never stored values. A producer sent "above"
// anyway, and the editor coerced it: `operatorMap[op] || "greater_than"`
// (helper.ts:863). That coercion is silent, and it means a rule authored as
// "below" would have been persisted as greater_than — an inverted alert.
#ThresholdOperator: "greater_than" | "greater_than_or_equal" |
	"less_than" | "less_than_or_equal" |
	"equals" | "not_equals"

// #ConditionType is the condition's shape. Empty means an absolute threshold,
// which is the common case and what the editor's "Threshold Alert" tab writes,
// so "" has to be accepted rather than treated as a bad value.
#ConditionType: "" | "threshold" | "change" | "change_percent" | "new_value"

// #ThresholdInputUnit is the duration unit a threshold may be entered in.
//
// The empty string is included because that is what the wire actually carries.
// The Go model marshals `threshold_input_unit` without omitempty, so EVERY rule
// serialises the field, and every rule that has no input unit — which is most of
// them — sends "". A closed set of just the four units described the app rather
// than the data, and rejected four MCP contract fixtures that are valid rules.
// Empty means the same thing as absent: threshold_value is already in native
// units.
//
// Otherwise the set is closed deliberately: the API validates against exactly
// these and DROPS anything else, so a wider contract would accept a value the
// API then discards silently. Mirrors durationInputUnits in
// observability/alertmanager/rule-engine/threshold_unit.go and
// DURATION_UNIT_NS in webapp src/pages/alert-config/threshold-unit.ts.
#ThresholdInputUnit: "" | "ns" | "us" | "ms" | "s"

// #Frequency is how many breaches inside the window are needed to fire.
#Frequency: "" | "at_least_once" | "always" | "more_than_once"

#NoDataState: "normal" | "firing" | "previous"

// What a rule does once its evaluation has been FAILING for five minutes —
// the counterpart of #NoDataState for a failed evaluation rather than an
// empty one. previous holds the last state; error holds it and raises a
// separate evaluation-error alert; firing treats the failure as a breach;
// normal resolves.
#ErrorState: "previous" | "error" | "firing" | "normal"

// #LabelRouting: which channels a label-routed rule's alerts reach — the first
// one whose route_match matches, or every one of them.
#LabelRouting: "first" | "all"

// #RuleCondition is one threshold of a rule that states several, applied to one of
// the rule's queries by LABEL. The id is what #RuleConditionExpression refers to and
// is never an alert label (labels feed the fingerprint).
#RuleCondition: {
	id!:       string & !=""
	query!:    string & !=""
	operator!: #ThresholdOperator
	value!:    number
	unit?:     string
}

// #RuleConditionExpression says how conditions combine. v1 writes one flat level;
// `children` is accepted, not constrained further, so nesting can arrive later
// without narrowing this contract.
#RuleConditionExpression: {
	op!:         "and" | "or"
	conditions?: null | [...string]
	children?:   null | [...{[string]: _}]
}

// Named for the same reason as the panel package's list types: an empty default
// beside an inline constrained type silently discards the constraint on export.
#Labels: {[string]: string}
#ChannelIDs: [...int]

// #NotificationChannel is server-echoed channel detail, read-only on this
// payload. Per-type connection settings are a wide, server-owned union, so
// `config` stays unconstrained rather than enumerated here.
#NotificationChannel: {
	id!:          string | int
	name!:        string
	type!:        string
	template_id?: int | null
	// An open map, not `{...}` — the latter exports as `const: {}`, i.e. "must be
	// exactly the empty object". See the EMPTY-CONTAINER DEFAULT rule in
	// panel/query.cue.
	config?: {[string]: _}
	enabled?:    bool
	created_at?: string
	updated_at?: string
	created_by?: string
	...
}

#Rule: {
	// The document's schema version, and the entry point for migration.
	//
	// A reader migrates any document from the version it was saved at up to
	// current, one step at a time, then renders. That is what keeps ONE renderer
	// able to display documents written years apart, and it is why Grafana runs a
	// single dashboard renderer against schemaVersion 42.
	//
	// It exists from v1 deliberately. The webapp's existing panel migration
	// (migrateToNewSchema in explorer/v2/utils/zod-schemas.ts) has to identify
	// legacy payloads by SNIFFING STRUCTURE — "has graphStyles at root", "queries
	// missing columnFields" — precisely because there was no version marker to
	// read. That cost is already paid once; adding a new format without a marker
	// pays it again.
	//
	// Documents already in the database predate this field, and absence means
	// LEGACY — version 0, not 1. They are the one population guaranteed never to
	// have been checked against this contract, so calling them v1 would assert a
	// shape they may not have. That is why the field defaults rather than being
	// required, and why `legacy -> v1` is expressible as a step at all.
	//
	// The default below is what a document gets STAMPED with, and it must equal
	// the Go chain's CurrentVersion — this is the second place that number is
	// written, and TestGeneratedDefaultVersionMatchesTheChain is what stops the
	// two drifting. Bump both together.
	schema_version: *1 | (int & >=1)

	name!: string & !=""

	// The signal this rule queries. Must agree with query_config[].selectedMode.
	query_type!: panel.#SelectedMode | "slo"

	// One entry per query; at least one. Multiple entries appear when a formula
	// query references others by label.
	//
	// Written as [#Query, ...#Query] rather than the shorter [_, ...#Query].
	// Both mean "at least one element" to CUE, but the `_` exports to JSON
	// Schema as `prefixItems: [true]` — and in draft 2020-12 `items` only
	// constrains elements AFTER prefixItems, so the first query would be
	// validated against nothing at all. For a single-query rule, which is the
	// common case, that silently disables the entire query check. Repeating the
	// type keeps the positional entry constrained.
	query_config!: [#QueryConfig, ...#QueryConfig]

	// Selects WHICH query's series the threshold applies to, by label.
	//
	// Must match one of query_config[].label. It is not checked here — CUE cannot
	// express a constraint between a scalar and a sibling list's contents — and
	// it is not cosmetic: the rule from the incident carried
	// metric_query_label "A" against a query labelled "B", which means it
	// evaluated nothing and could never fire. Enforced in the Go validator.
	metric_query_label!: string

	threshold_operator!: #ThresholdOperator
	threshold_value!:    number

	// The LOWER threshold of a two-level rule. Crossing threshold_value is
	// critical; crossing only this is a warning. Datadog's model: one rule, one
	// condition, two levels, one incident whose level moves.
	//
	// Optional, and absent on every rule written before it existed — which is
	// the entire compatibility story. `severity` is part of an alert's identity
	// (it is in the outgoing label map and the fingerprint is an md5 of that
	// map), so a single-level rule must keep emitting exactly what it emits
	// today or every stored alert resolves and re-fires at once.
	//
	// The API refuses a rule that sets this unless: severity is "critical" (the
	// upper level; the lower one is always "warning"), threshold_operator is an
	// ORDERED comparison, the value sits on the reachable side of the critical
	// one, and threshold_frequency is at_least_once. Those are semantic rules
	// the contract cannot express — a number is a number — so they live in
	// observability/alertmanager/rule-engine/warning_threshold.go and are
	// enforced on the write path, not here.
	warning_threshold_value?: number

	// Several thresholds combined with AND / OR. All three absent (or null) is a
	// single-threshold rule. threshold_operator / threshold_value still carry
	// the first condition. Cross-field rules — both-or-neither, ids resolving,
	// join_by required across queries — are enforced by the API validator
	// (composite_conditions.go), not here.
	conditions?:           null | [...#RuleCondition]
	condition_expression?: null | #RuleConditionExpression

	// join_by is tri-state and null is NOT []: null = no cross-query join,
	// [] = every query ungrouped, [labels] = pair series on those labels.
	join_by?: null | [...string]

	// Server-echoed, read-only: the rule engine's latest evaluation problem
	// (its error, or the warnings of a partial evaluation). Absent when the
	// latest evaluation was clean. Never read from a write.
	evaluation_issue?:    null | string
	evaluation_issue_at?: null | string
	// "error" = the latest evaluation failed (current_state is stale);
	// "warning" = it ran only partly.
	evaluation_issue_kind?: null | "error" | "warning"
	// When the rule last COMPLETED an evaluation (queries ran without error;
	// a no-data result counts, so it does not mean fresh measurements exist).
	// null / absent = none recorded yet. Engine-written; never read from a write.
	last_successful_evaluation_at?: null | string

	// Optional, with no default here: absent on create means the server's
	// default for new rules ("error"); absent on update keeps the stored
	// value. "" is what a database without the column reads back as.
	error_state?: null | "" | #ErrorState
	// Start of the current spell of failed evaluations; null when the rule is
	// not failing. Engine-written; never read from a write.
	evaluation_failing_since?: null | string

	// "Remind me every": how often a still-firing alert of THIS rule
	// re-notifies, in Alertmanager duration syntax ("4h"). null / absent / ""
	// inherits each notification channel's repeat_interval. Must fit every
	// channel the rule notifies through (a multiple of its group interval) —
	// enforced by the API (cadence.ValidateRuleRepeat), not here.
	repeat_interval?: null | string

	// How long changes to THIS rule's alert batch (a series starting or
	// recovering, warning → critical) wait before they are sent, in
	// Alertmanager duration syntax ("5m"). null / absent / "" inherits each
	// notification channel's group_interval. At least 1m; when set, the rule's
	// repeat_interval must be a multiple of it — enforced by the API
	// (cadence.ValidateRuleCadence), not here.
	group_interval?: null | string

	// The unit the operator TYPED the threshold in — metadata for round-tripping
	// the form, never read by the evaluator.
	//
	// threshold_value is always in the query's NATIVE storage units, and trace
	// `duration` is nanoseconds. Nothing recorded what the author meant, so "500"
	// for "500 ms" was stored and compared as 500 ns and the rule breached on
	// every evaluation (KUBE-2435). The client converts before sending; this only
	// records the spelling so the form can show the number back as entered.
	//
	// Absent OR EMPTY means the value is already in native units — which is what
	// every rule written before this field existed means, so both must stay
	// valid. The wire always carries the key (no omitempty on the Go model), so
	// "" is the common case, not an edge one.
	//
	// Only set when the value the ENGINE compares is a trace duration: with a
	// formula that is the query named by metric_query_label (a formula OUTPUT is
	// unitless), without one it is the sum of every query, so all of them must be
	// duration-valued. Dimensionless conditions (change_percent, new_value) never
	// carry it. The API drops a unit that does not meet those conditions rather
	// than rejecting the rule, so an import naming one here may find it absent on
	// read-back — that is normalisation, not data loss.
	//
	// NOT the same thing as `unit`, which is the chart's y-axis display formatter
	// (auto, raw, percentage, acres, …) and must never be read as an input unit.
	threshold_input_unit?: #ThresholdInputUnit

	condition_type: *"threshold" | #ConditionType

	// For change / change_percent / new_value: the earlier window the current
	// value is compared against. Empty for an absolute threshold.
	compared_to?: #PromDurationOrEmpty

	time_window:         *"5m" | #PromDurationOrEmpty
	evaluation_interval: *"1m" | #PromDurationOrEmpty

	frequency_type: *"at_least_once" | #Frequency

	// Required when frequency_type is "more_than_once" — a cross-field rule, so
	// it lives in the Go validator. The editor enforces a minimum of 2.
	breaches_count?:         int & >=0
	breach_counting_window?: #PromDurationOrEmpty

	// Empty is accepted alongside the three named states because the entity's
	// field is a bare string with no omitempty, so a producer that never sets it
	// — the create-alert tool, bulk import — stores "". The engine and the editor
	// both read an empty value as "normal", so this is a real synonym rather than
	// a gap being papered over.
	no_data_state: *"normal" | "" | #NoDataState

	// Not enumerated: the API accepts and stores any string here, and rejecting
	// an unknown severity would fail rules that already exist. The editor offers
	// critical, error, warning, info.
	//
	// NOT required, despite every rule having one. A required field and a default
	// are contradictory — `!` means the value must be supplied, so the default
	// could never apply, yet it still exported into gen/alert-rule.defaults.json.
	// A consumer initialising a form from that file and then omitting the field —
	// which is the entire purpose of a default — failed validation. The default is
	// the thing that guarantees a severity, so the default is what stays.
	severity: *"warning" | string

	description?: string
	enabled:      *true | bool

	// Extra labels attached to the rule and every alert it fires. Also the input
	// to label-based routing when route_by_labels is true.
	labels: *{} | #Labels

	// Channel IDs this rule routes to directly. Every listed channel is
	// notified (a rule with several is routed to all of them by rule_id). An
	// empty list is allowed, as in the editor: the rule's alerts then reach the
	// channels whose label routes (route_match, route_all) match them, or nobody
	// if none do.
	// null as well as a list, for the same reason as the optional lists on
	// #QueryConfig: model.AlertRule's field carries no omitempty, so a rule with
	// no channels marshals it as `null` rather than omitting it.
	notification_channel_ids: *[] | null | #ChannelIDs

	route_by_labels: *false | bool

	// Only meaningful when route_by_labels is true: "first" notifies the first
	// channel whose route_match matches (the newest such channel), "all" every
	// one of them. Optional, with no default here: absent on create means
	// "first"; absent on update keeps the stored value. "" is what a database
	// without the column reads back as.
	label_routing?: null | "" | #LabelRouting

	// Y-axis value formatter for the rule's chart. Cosmetic — it does not change
	// what the rule does. Not enumerated because the valid list is derived at
	// runtime from the unit cascader, so the webapp's own zod for it also
	// resolves to plain string.
	unit: *"auto" | string

	// Attach the rows behind the alert: log lines or spans from each condition
	// that fired (logs and traces queries; a metrics-only rule has none).
	include_samples: *false | bool
	sample_limit:    *5 | (int & >=1 & <=100)

	// Attach a graph of the evaluation that fired the alert to its
	// notifications. Opt-in. Only a rule with one metrics, logs or traces
	// query, or a formula over queries, and an absolute above/below threshold
	// can honour it; the API refuses it on any other (rule-engine
	// graphEligibility), which is a semantic check and not expressed here. Optional, with no default: absent on create means off,
	// absent on update keeps the stored value.
	include_graph?: null | bool

	// Keep a grouped rule's series labels from overriding its own labels. The
	// rule engine folds them into one kubesense_series label instead, so a
	// rule labelled workload=log keeps workload=log on every alert (silences
	// and maintenance windows on it match) and each series is still its own
	// alert. Optional, with no default: absent on create means off, absent on
	// update keeps the stored value.
	exclude_group_by_labels?: null | bool

	// Set when the rule was created from a dashboard panel.
	dashboard_id?: string
	panel_id?:     string

	// Legacy free-form query. Empty on every rule the current editor writes;
	// retained because existing rows carry it.
	raw_query: *"" | string

	// ── server-owned; present on GET, blank or absent on an authored rule ──

	id?:     string
	org_id?: string
	team?:   string

	// Username of the creator; a filter facet on the rules list.
	created_by?: string
	created_at?: string
	updated_at?: string

	// Server-echoed resolution of notification_channel_ids. Frequently null —
	// alert-rule-detail-page.tsx resolves ids to names itself rather than
	// relying on it.
	notification_channels?: null | [...#NotificationChannel]

	// Evaluation state, owned by the engine rather than by whoever wrote the
	// rule. All three timestamps are *time.Time on the entity with no omitempty
	// (dao/model/alert_rule.go), so a rule that has never been evaluated — every
	// rule, at the moment it is created — marshals them as `null` rather than
	// omitting them. Accepting null is what lets the contract be applied on the
	// write path at all; requiring a string would reject every new rule.
	current_state?:          string
	last_evaluation_time?:   null | string
	last_state_change_time?: null | string
	firing_since?:           null | string
	consecutive_breaches?:   int

	...
}
