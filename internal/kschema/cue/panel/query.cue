// Package panel is an exact replica of the webapp's panel zod schemas.
//
// Source of truth, line references as of transliteration:
//   src/pages/explorer/v2/utils/zod-schemas.ts
//     fieldSchema                     :185
//     floatFieldSchema                :194
//     floatOrStringFieldSchema        :199
//     aggregationSchema               :210
//     panelMetricsQueryFormulaSchema  :236
//     labelOptionsSchema              :344
//     sortingSchema                   :357
//     explorerPaginationSchema        :369
//     baseLogsTracesQuerySchema       :597
//     explorerPanelQueryUnionSchema   :663
//   src/types/zod/filter.ts
//     uiStoreFilterSchema             :3
//
// EXACT REPLICA means exactly that: this is the panel FORM shape, field for
// field, including members that only the editor cares about (fieldConfig,
// filterMode, chart_type, pagination, the SPL xaxis/yaxis/source/list/value
// group). Nothing is added and nothing is dropped.
//
// Consequently, fields that exist only on the persisted ALERT document —
// raw_filters, selectedMeasurement, value_operation, the flattened fields/limit
// pair — are NOT here. They live in cue/alert, because the alert wire format
// flattens what the form nests (aggregation: {function, fields}) and carries a
// compiled filter alongside the raw one. Mixing them would make this schema a
// replica of nothing.
//
// zod `.catch(x)` becomes a CUE default `*x | …`. The two are not equivalent and
// the difference is deliberate: `.catch()` swallows a WRONG-TYPED value and
// substitutes; a CUE default applies only when the field is ABSENT. So a payload
// zod would silently repair is rejected here. That is correct for a wire
// contract — the repair is what let bad values reach the database — and it is
// why the form layer keeps its own zod rather than being replaced by this.
package panel

import "list"
// Kept OPEN (`...`). Two reasons, and the first is not optional:
//
//  1. A definition built by unification — `#Base & {…}` — exports as `$ref` plus
//     `additionalProperties` plus only the narrowing struct's own properties. In
//     draft 2020-12 `additionalProperties` is evaluated against ADJACENT
//     properties only, never those behind a `$ref`, so closing it rejects every
//     field the base declares. #LogsQuery exported with required:["selectedMode"]
//     and refused `label`, `columnFields` and `aggregation`.
//  2. Forward compatibility: a field kubeapi adds tomorrow must not fail
//     validation across four consumers at once. Missing REQUIRED keys still
//     error, so a typo'd key is still caught.
//
// Note zod `.object()` strips unknown keys rather than rejecting them, so neither
// open nor closed is a literal replica; open is the right call for a contract.


// ── enums, mirroring the exported const tuples ──────────────────────────────

// QUERY_MODES :163
#QueryMode: "builder" | "code"

// FILTER_MODES :164
#FilterMode: "MFD" | "ADVANCED_QUERY" | "SPL" | "SQL"

// NUMERIC_AGGREGATION_FUNCTIONS :151
#NumericAggregation: "avg" | "sum" | "max" | "min" |
	"p99" | "p95" | "p90" | "p75" | "p50"

// chart_type :639 — note this is the query-level chart_type, a narrower and
// differently-spelled set than PANEL_TYPES (:166). "timeseries" here vs
// "timeSeries" there. The comment in the zod records that `list` exists only to
// surface query-builder options and is otherwise unused.
#ChartType: "table" | "stat" | "bar" | "pie" | "topList" | "timeseries" | "list"

// The discriminator of the query union. `formula` is not a datasource — it
// composes other queries in the same panel by label.
#SelectedMode: "metrics" | "logs" | "traces" | "formula"

// ── fields ──────────────────────────────────────────────────────────────────

// #Field mirrors fieldSchema :185. `type` is a plain string, not an enum — the
// engine resolves names and types against its own catalog, so constraining it
// would reject spellings the engine accepts.
//
// This is the ONE field definition; everything below narrows it at the point of
// use, exactly as the zod does. The narrowing variants build on #FieldBase
// rather than on #Field, so they do not inherit #Field's lenient `type` default
// onto a member they then make required — required-and-defaulted is a
// contradiction, and scripts/check-export.mjs now fails the build on it.
#Field: {
	// The field name to query.
	field!: string

	// The data type of the field. Defaults to "string" rather than being
	// required, because stored rules genuinely arrive without it — a live traces
	// rule with 119 firing series carries
	// `groupBy: [{field: "workload", is_attribute: false}]` and no type at all.
	// Requiring it rejected a document the product itself wrote and is running.
	//
	// "string" is the safe fill: a group-by key is a string, and the two places a
	// NUMERIC type is load-bearing declare their own, so an aggregation cannot
	// silently acquire a string column.
	type: *"string" | string

	// Whether this is an attribute field.
	is_attribute!: bool

	...
}

// The two numeric variants repeat the shape rather than narrowing #Field with
// `#Field & {type!: …}`, and the repetition is load-bearing.
//
// A narrowing exports as `$ref` PLUS the narrowing struct's own keywords, and in
// draft 2020-12 `$ref` is evaluated in ADDITION to its siblings — so the base's
// `required` still applies and an adjacent override cannot take a field back out
// of it. Unification can add a constraint; it can never remove one. Building
// #Field on a stricter base therefore left `type` required no matter what the
// derived definition said, silently undoing this whole change. Declaring these
// standalone is the only form where each one means what it reads as.

// floatFieldSchema :194 — the numeric aggregations need a float column, so this
// one REQUIRES an explicit type rather than defaulting.
#FloatField: {
	field!:        string
	type!:         "float"
	is_attribute!: bool

	...
}

// floatOrStringFieldSchema :199 — unique_count counts distinct values, so a
// string column is valid too. Explicit, same reason as #FloatField.
#FloatOrStringField: {
	field!:        string
	type!:         "float" | "string"
	is_attribute!: bool

	...
}

// columnFields element :619 — fieldSchema extended with a non-empty field name
// plus optional display members. Only meaningful for the `list` panel type,
// where it is sent as required_fields. Lenient, like #Field.
#ColumnField: #Field & {
	field!: string & !=""
	alias?: string
	label?: string
	...
}

// uiStoreFilterSchema (types/zod/filter.ts:3) — the editor's filter store shape,
// key to list of values. The zod's .catch({}) default is applied at each use
// site, not here.
//
// See the EMPTY-CONTAINER DEFAULT rule at the bottom of this file: a constrained
// type must be a NAMED definition whenever an empty default sits beside it.
#Filters: {[string]: [...string]}

// ── aggregation ─────────────────────────────────────────────────────────────

// aggregationSchema :210 — a discriminated union on `function`. row_count takes
// no field; the other two require at least one, of different types.
//
// This nesting is why the rule expresses cleanly here but not on the alert
// document: the alert format flattens it to sibling value_operation + fields,
// where the same constraint would need a cross-field check.
//
// The arms carry `...` for the same reason every other struct here does. The
// `*{function: "row_count"}` DEFAULT at each use site stays closed and exact —
// it is a concrete instance rather than a type, and a document with extra
// members matches these arms instead.
#Aggregation: {function!: "row_count", ...} |
	{function!: "unique_count", fields!: [#FloatOrStringField, ...#FloatOrStringField], ...} |
	{function!: #NumericAggregation, fields!: [#FloatField, ...#FloatField], ...}

// ── metrics builder functions ───────────────────────────────────────────────

// panelMetricsQueryFormulaSchema :236. #QueryFunction is GENERATED: one arm
// per entry of cue/catalog, written to functions_gen.cue by catalog_test.go.
// Every arm pins `name` with a const, so narrow.go reports one finding for an
// unknown name and only the claimed arm's findings otherwise; `type` is
// informational and optional on every arm, since the catalog keys on name.
//
// What stays hand-written here is the LEGACY shape: the seven argument tuples
// the builder wrote before the catalog existed. A legacy name's generated arm
// accepts its tuple as an alternative to the catalog list, which is the
// widening proof, arm by arm. These are frozen. A function's real parameters
// live in the catalog; add there, never here.
//
// Every element is a named definition, per the EMPTY-CONTAINER DEFAULT rule at
// the bottom of this file.

// Grafana-style interval macros. The API resolves these (observability/
// promqlmacros) before execution, the first two against the step the query
// actually runs with, $__time_filter against the selected time range, so they
// reach VictoriaMetrics as concrete durations and never appear in a stored
// query's meaning. Valid anywhere a window literal is.
#IntervalMacro: "$__interval" | "$__rate_interval" | "$__time_filter"

// A PromQL duration literal: one or more number-unit pairs, so `15m` and
// `1h30m` both match. `5 minutes` does not, and VictoriaMetrics would reject it.
#Duration: =~"^([0-9]+(ms|s|m|h|d|w|y))+$"

// A rollup window. Once an enum of six literals the builder offered; widened to
// any duration so a typed window like `15m` is accepted, and to #IntervalMacro
// because the macros are as valid a rollup window as any literal.
#RollupTimeRange: #Duration | #IntervalMacro

// `offset` shifts the lookbehind back by a duration; `at` pins evaluation to
// the query's start(), its end() or a unix timestamp in seconds.
#SelectorModifiers: {
	offset?: #Duration
	at?:     "start()" | "end()" | number

	...
}

// Every argument below carries `...`, like the rest of this package: a closed
// argument struct would make a member the webapp adds tomorrow fail validation
// in all four consumers at once, and on the write path. The tuple arities are
// what constrain these.

// Range's `over` has always been an open string and stays one; rollup's is
// constrained to a macro or a duration literal.
#RangeArg: {arg_name!: "over", arg_value!: string, ...}
#LegacyRangeArgs: [#RangeArg]

#AggregationsArg: {arg_name!: "by", arg_value!: [...string], ...}
#LegacyAggregationsArgs: [#AggregationsArg]

#TopBottomKArg: {arg_name!: "k", arg_value!: number, ...}
#TopBottomByArg: {arg_name!: "by", arg_value!: "max" | "min" | "avg" | "median" | "last", ...}
#LegacyTopBottomArgs: [#TopBottomKArg, #TopBottomByArg]

#RollupArg: {arg_name!: "over", arg_value!: #RollupTimeRange, ...}
// phi for quantile_over_time, a fraction between 0 and 1. Optional because
// instances stored before it existed carry only the window; the webapp's
// generator uses 0.95 for them.
#RollupQuantileArg: {arg_name!: "quantile", arg_value!: number & >=0 & <=1, ...}
#LegacyRollupArgs: [#RollupArg] | [#RollupArg, #RollupQuantileArg]

#ComparisonArg: {arg_name!: "than" | "to", arg_value!: number, ...}
#LegacyComparisonArgs: [#ComparisonArg]

// The only legacy shape that is a plain array rather than a fixed tuple: abs,
// sort and the math functions take none, clamp takes two, the rest take one,
// and the arm accepted any of the four names on any transform.
#TransformArg: {arg_name!: "min" | "max" | "to_nearest" | "quantile", arg_value!: number, ...}
#LegacyTransformArgs: [...#TransformArg]

// A binary operator against a scalar: `<expr> + value`, `-`, `*`, `/`, `%`, `^`.
#ArithmeticArg: {arg_name!: "value", arg_value!: number, ...}
#LegacyArithmeticArgs: [#ArithmeticArg]

// ── shared query members ────────────────────────────────────────────────────

// labelOptionsSchema :344 — discriminated on `type`, defaulting to auto
// (DEFAULT_LABEL_OPTIONS :133).
//
// Three arms for two cases. The first is the concrete default instance, and it
// has to be written with plain fields rather than `!` — a required field is
// never concrete, so `*#AutoLabelOptions` would leave nothing for the default to
// materialise. But a closed concrete arm rejects any member added later, and
// unlike `aggregation` or `sortBy` the auto case had no open sibling to fall
// through to: `{type: "auto", somethingNew: 1}` matched neither arm. #AutoLabel
// is that sibling.
#LabelOptions: *{type: "auto"} | #AutoLabel | #CustomLabel

#AutoLabel: {type!: "auto", ...}
#CustomLabel: {type!: "custom", format: *"" | string, ...}

// sortingSchema :357, defaulting to DEFAULT_SORTING :137.
#Sorting: {
	sortBy: *{field: "", type: "string", is_attribute: false} | #Field
	sortOrder: *"ASC" | "DESC"

	...
}

// explorerPaginationSchema :369, defaulting to DEFAULT_PAGINATION :146.
#Pagination: {
	page:      *1 | #Page
	page_size: *50 | #PageSize

	...
}

// Same reason as #Filters: an inline constraint arm beside a default is
// simplified away on export, taking the bounds with it. Named, they survive.
#Page:     number & >=1
#PageSize: number & >=1 & <=500

// fieldConfig — per-query display overrides, present on every arm of the
// union (queryFieldConfigSchema). Named after Grafana's fieldConfig: a
// "field" is a result column or series, and this is how that field displays,
// as opposed to the panel-wide `config`.
//
// `unit`: Y_AXIS_FORMATTERS is derived at runtime from the unit cascader, so
// its own zod resolves to plain string and so does this.
//
// `color`: the query's own colour, outranking the panel `colorScheme` for
// every series the query yields (threshold colours still outrank it). Two
// arms, sharing the panel scheme's vocabulary (queryColorOverrideSchema):
//   - single  — one hex colour; a grouped query that returns several series
//               is drawn in shades of it rather than all identical
//   - palette — a named palette cycled from `offset`, restarting at 0 for
//               this query rather than continuing the panel counter
#FieldConfig: {
	unit?:  string
	color?: #QueryColorOverride

	...
}

#QueryColorOverride: #SingleQueryColor | #PaletteQueryColor

#SingleQueryColor: {type!: "single", color!: #HexColor, ...}
#PaletteQueryColor: {
	type!:    "palette"
	palette!: #ColorPaletteKey
	offset?:  int & >=0

	...
}

// ── the query union ─────────────────────────────────────────────────────────

// explorerPanelQueryUnionSchema :663, metrics arm :665.
#MetricsQuery: {
	selectedMode!: "metrics"
	fieldConfig:   *{} | #FieldConfig
	label:         *"A" | string

	// Label matchers for builder mode, keyed by label. Each value's prefix picks
	// its PromQL operator: `v` is `label="v"`, `-v` is `label!="v"`, `~re` is
	// `label=~"re"` and `-~re` is `label!~"re"`. So `-` alone excludes the empty
	// value (`label!=""`). A whole `$var` or `-$var` value stands for the
	// variable's selected values.
	filters:   *{} | #Filters
	queryMode: *"builder" | #QueryMode

	// DEFAULT_METRIC :35. The editor substitutes this placeholder when the field
	// is missing and then PERSISTS it on the next save, so an omitted
	// selectedMetric is not neutral — it silently becomes a real query against an
	// unrelated metric.
	selectedMetric: *"aggregator_discovery_aggregation_count_total" | string

	functions: *[] | #QueryFunctions

	// Offset and @ on the series selector. The builder renders them once, where
	// the selector is first consumed: after its window, as in
	// `rate(m[5m] offset 1h @ end())`, or after the bare selector when nothing
	// windows it. Never between a selector and its window, which does not parse.
	selectorModifiers?: #SelectorModifiers

	promql:  *"" | string
	visible: *true | bool
	variables: *[] | #Variables
	labelOptions: #LabelOptions
	pagination:   #Pagination

	...
}

// baseLogsTracesQuerySchema :597, extended per arm at :699, :704 and :1148.
//
// The zod adds selectedMode via .extend() on each arm. A CUE definition is
// closed, so a field can be narrowed but not introduced by unification — hence
// the discriminator is declared here across every arm and narrowed to a literal
// below, which reaches the same place.
#LogsTracesQueryBase: {
	selectedMode!: "logs" | "traces" | "events" | "rum" | "synthetics"
	labelOptions:  #LabelOptions
	fieldConfig:  *{} | #FieldConfig
	label:        *"A" | string
	filters:      *{} | #Filters
	queryMode:    *"builder" | #QueryMode
	visible:      *true | bool
	variables:    *[] | #Variables
	groupBy:      *[] | #GroupByFields

	// No .catch() on this one in the zod — the only member of the base without a
	// fallback.
	columnFields!: [...#ColumnField]

	filterMode:  *"MFD" | #FilterMode
	aggregation: *{function: "row_count"} | #Aggregation
	pagination:  #Pagination
	sorting:     #Sorting
	query:       *"" | string
	chart_type:  *"table" | #ChartType

	// SPL/SQL-only visualisation hints (:642-656). "auto" means the formatter
	// infers the column.
	xaxis: *"auto" | string

	// yaxis is a LIST, not a string. It was a string until commit 2c8d7ab1a
	// (2026-08-07, "update y-axis handling to support multiple select"), which
	// changed the persisted type and added coerceYaxisColumns + yaxisColumnsSchema
	// (:571, :586) plus a coercion step in migrateToNewSchema (:854) so stored
	// documents holding `yaxis: "cpu"` keep that selection.
	//
	// Worth noting what that change cost: it is a breaking type change to a
	// persisted field, and it is detected by sniffing `!Array.isArray(next.yaxis)`
	// because panels carry no version marker. With one, the migration would key
	// off the version instead of guessing from shape.
	yaxis:  *["auto"] | #YaxisColumns
	source: *"auto" | string
	list:   *[] | #StringList
	value:  *"auto" | string

	...
}

// yaxisColumnsSchema :586 — a non-empty list of column names. Named rather than
// inlined so the min-length survives export alongside the default.
#YaxisColumns: [string, ...string]

#LogsQuery: #LogsTracesQueryBase & {selectedMode!: "logs", ...}

#TracesQuery: #LogsTracesQueryBase & {selectedMode!: "traces", ...}

// Custom events (deploys, CI runs, incidents) from kube_events.custom_events.
// Same query model as logs, so the same base. An advanced filter is stored as
// the WHERE text under `filters.advanced_query` with filterMode
// "ADVANCED_QUERY"; its fields are the events catalog and `@key` reads an
// attribute.
//
// filterMode keeps the base's full enum although the API serves no SPL or SQL
// over events: the zod arm accepts all four, and narrowing here alone would put
// this contract and the editor's schema in disagreement.
#EventsQuery: #LogsTracesQueryBase & {selectedMode!: "events", ...}

#RumEventType: "session" | "view" | "action" | "error" | "resource" | "long_task"

// Real user monitoring events from kube_rum. Same query model as logs, with the
// event type naming the table; a rum query without it has no table to read.
// Alert rules keep their own mode list and do not accept rum.
#RumQuery: #LogsTracesQueryBase & {selectedMode!: "rum", rumEventType!: #RumEventType, ...}

// Synthetic check runs from kube_synthetics.synthetics_results, one row per
// run. Same query model as logs, so the same base, and served by
// /explore/query and /explore/query-range like events. Fields are the
// synthetics results catalog; `@key` reads a run attribute and
// `labels.<key>` a monitor label.
//
// Not "synthetics_monitor", the arm that names monitors and is served by the
// synthetics endpoints rather than /explore.
//
// No narrowing beyond the base, for the reason #EventsQuery gives: the zod
// arm is the base extended with this literal, and narrowing here alone would
// put this contract and the editor's schema in disagreement.
#SyntheticsQuery: #LogsTracesQueryBase & {selectedMode!: "synthetics", ...}

// formula arm :709. The zod also carries a .refine() restricting the expression
// to query labels, digits, the four operators and parentheses; replicated here
// as a regex, with whitespace stripped being the one thing a pattern cannot do
// (the zod compacts before testing, so " A + B " is valid there and here).
#FormulaQuery: {
	selectedMode!: "formula"
	expression!:   string & =~"^[A-Z0-9+/*() \t-]*$"
	label:         *"A" | string
	visible:       *true | bool
	labelOptions:  #LabelOptions
	fieldConfig:   *{} | #FieldConfig

	...
}

// ── slo queries ─────────────────────────────────────────────────────────────

// SLO filter selections, keyed by filter field — the shape the webapp's filter
// store holds, not the converted request body. The conversion expands dashboard
// variables and drops unresolved ones, so it is lossy and depends on where it
// ran; the panel stores what was picked and converts when it queries.
//
// Not enumerated. The SLO filter API decides which fields exist, and an unknown
// key is inert on both sides, so closing this would reject a document the
// browser round-trips happily.
#SloFilters: {[string]: [...string]}

// Which widget an slo query draws.
#SloWidgetType: "summary" | "list" | "single"

// Which layout the list widget draws.
//
// Only "primary" reads from the row the list already returned. The other three
// fetch a timeline per row through a queue shared by every SLO table in the
// browser, so a dashboard of them would contend with each other and with the
// SLO page. All four are legal so enabling them later is a renderer change
// rather than a contract change.
#SloTableView: "primary" | "daily" | "weekly" | "monthly"

// The fields an SLO list can be ordered by. Only these two sort: /slo/list
// builds its ORDER BY from this, and an SLO's compliance and error budget are
// computed after that query returns, so there is no column for them. "" is
// unsorted.
#SloSortField: "" | "name" | "created"
#SloSortOrder: "" | "ASC" | "DESC"

// What a single-SLO panel draws.
#SloViewMode: "overall" | "groups" | "both"

// The rollup windows the API computes for every SLO.
#SloTimeWindow: "last_2_hour" | "last_24_hour" | "last_7_days" | "last_28_days"

// What a single-SLO panel shows when nothing was chosen. Named rather than
// inline: see the EMPTY-CONTAINER DEFAULT rule — a list default sitting beside
// an inline constrained type exports as a bare array and drops the constraint.
#DefaultSloTimeWindows: ["last_24_hour", "last_7_days", "last_28_days"]

// An slo query names service level objectives and the widget to draw them as.
//
// It is a query rather than a member of #PanelConfig because it answers "what
// data" — which is what every other arm of this union answers too. #PanelConfig
// is presentation, shared by all ten panel types.
//
// Unlike every other arm this one is NOT sent to /explore/query. The webapp
// excludes it and fetches from /slo/list and /slo/stats instead.
//
// Flat, discriminated only by selectedMode like its siblings. `widgetType` is a
// plain enum rather than a nested union because CUE exports a union-of-unions
// as an intersection of open structs, losing the narrowing that makes the
// discriminant worth having. The cost is that `objectiveId` is meaningless
// outside "single" and the table fields outside "list"; each renderer reads
// only what its own widget defines.
#SloQuery: {
	selectedMode!: "slo"
	label:         *"A" | string
	visible:       *true | bool

	// "single", not "summary": an unconfigured panel waits for an objective
	// rather than fetching a fleet-wide rollup nobody asked for. The webapp
	// moved to that default in #2388 and this side was missed, so a panel saved
	// without the field meant one widget to the browser and another to
	// ApplyDefaults. The two must agree — this is the number the published
	// schema hands every other consumer.
	widgetType: *"single" | #SloWidgetType

	// Scopes "summary" and "list". Ignored by "single", which names one SLO.
	filters: *{} | #SloFilters

	// The objective "single" draws. "" means none picked yet, which the webapp
	// renders as an empty panel rather than fetching.
	objectiveId: *"" | string

	// The objective's name AS IT WAS when the panel was saved. A cache, not an
	// identifier — objectiveId is what names the SLO.
	//
	// Persisted so an editor can label its picker before the lookup returns, and
	// so a DELETED objective can still be named rather than shown as a bare
	// uuid. Readers must prefer the live name: a rename would otherwise leave
	// every dashboard asserting the old one forever.
	objectiveName: *"" | string

	// Which rollup windows "single" draws.
	//
	// These four and no others, because these are the four the API computes.
	// getSLOMetricsForSLOs builds one CTE per window with a literal INTERVAL, so
	// the set is fixed server-side rather than parameterised — a document naming
	// "90 days" would describe a column no response carries. Widen this only
	// together with the query and the columns.
	//
	// "last_28_days" is four whole weeks, deliberately not 30 days.
	timeWindows: *#DefaultSloTimeWindows | [...#SloTimeWindow]

	// Show each window's remaining error budget beside its compliance. Both
	// numbers are already on every SLO row, so this costs no extra request.
	showErrorBudget: *true | bool

	// What "single" draws: the objective's own rollups, a per-group breakdown,
	// or both. Only meaningful for a GROUPED objective — the webapp falls back
	// to the rollups when a panel names an SLO with no groups, so a saved
	// document does not blank itself if its SLO changes shape.
	viewMode: *"overall" | #SloViewMode

	sloTableView: *"primary" | #SloTableView
	sortBy:       *"" | #SloSortField
	sortOrder:    *"" | #SloSortOrder

	...
}

// ── alert queries ───────────────────────────────────────────────────────────

// Rule selections for the summary widget, keyed by facet group — the same shape
// the alerts page's filter store holds and the same wire encoding it sends to
// /alerts/rules/facets.
//
// Not enumerated, for the reason #SloFilters is not: the facet groups are the
// API's to decide, an unknown key is inert on both sides, and closing this
// would reject a document the browser round-trips happily.
#AlertFilters: {[string]: [...string]}

// Which widget an alert query draws.
//
// Datadog's fourth, Check Status, is deliberately absent. It reports OK, Warn,
// Alert and No Data; the rule engine persists "normal" and "firing", carries no
// warning threshold, and treats no-data as a policy for what to do rather than
// a state it observed. Declaring the arm before the data exists is how the
// "trend" column cell type came to be accepted by the contract and drawn by
// nothing — see #ColumnCellType in config.cue.
#AlertWidgetType: "graph" | "value" | "summary"

// How the graph widget draws the rule's own query. Mirrors Datadog's viz_type.
#AlertVizType: "timeseries" | "topList"

// What the summary widget shows: the status counts, the rules behind them, or
// both.
#AlertDisplayFormat: "count" | "list" | "both"

// Whether a status count is coloured by its text or by a filled background.
#AlertColorPreference: "text" | "background"

// What one row of the summary widget stands for. Mirrors Datadog's
// summary_type, with one deliberate difference in what "group" can show.
//
//   monitor  — one row per RULE, carrying that rule's overall state.
//   group    — one row per firing INSTANCE of a rule. A rule that groups by
//              `pod` fires once per breaching pod, and each is its own row.
//   combined — one row per rule, as "monitor", plus a per-state count of that
//              rule's instances beside it.
//
// The difference: Datadog's "group" also lists groups that are OK, because a
// multi-alert monitor enumerates its groups whether or not they breach. Our
// engine materialises an instance only while it is firing, so a healthy series
// has no row to show. "group" here therefore means "firing instances", which is
// what a dashboard is asking about anyway.
#AlertSummaryType: "monitor" | "group" | "combined"

// An alert query names an alert rule and the widget to draw it as.
//
// A query rather than a member of #PanelConfig for the reason #SloQuery is: it
// answers "what data", and #PanelConfig is presentation shared by every panel
// type.
//
// Like the slo arm, this one is NOT sent to /explore/query. The webapp fetches
// the alert endpoints directly — the rule and its instances for "graph" and
// "value", /alerts/rules/stats and /alerts/rules/facets for "summary".
//
// Flat and discriminated only by selectedMode, like its siblings. `ruleId` is
// meaningless to "summary" and `filters` to the two widgets that name one rule;
// each renderer reads only what its own widget defines.
#AlertQuery: {
	selectedMode!: "alert"
	label:         *"A" | string
	visible:       *true | bool

	widgetType: *"graph" | #AlertWidgetType

	// The rule "graph" and "value" draw. "" means none picked yet, which the
	// webapp renders as an empty panel rather than fetching.
	ruleId: *"" | string

	// The rule's name AS IT WAS when the panel was saved. A cache, not an
	// identifier — ruleId is what names the rule.
	//
	// Persisted so an editor can label its picker before the lookup returns, and
	// so a DELETED rule can still be named rather than shown as a bare uuid.
	// Readers must prefer the live name: a rename would otherwise leave every
	// dashboard asserting the old one forever.
	ruleName: *"" | string

	vizType: *"timeseries" | #AlertVizType

	// Scopes "summary". Ignored by the widgets that name one rule.
	filters: *{} | #AlertFilters

	displayFormat:   *"both" | #AlertDisplayFormat
	colorPreference: *"text" | #AlertColorPreference

	// Scopes "summary" like `filters`. Defaults to "monitor", which is what the
	// widget did before this field existed — an older saved panel keeps its
	// behaviour rather than silently changing what each row means.
	summaryType: *"monitor" | #AlertSummaryType

	...
}

// ── synthetics_monitor queries ──────────────────────────────────────────────

// Which widget a synthetics_monitor query draws.
//
//   uptime      — one row per selected monitor: its uptime over the window,
//                 coloured against display.thresholds, with apdex beside it
//                 when display.showApdex.
//   status_list — one row per selected monitor carrying its current state
//                 (UP, DOWN, …) beside its uptime.
//   single      — one figure for the whole selection: its uptime (runs
//                 succeeded over runs made) with the selection's state counts.
#SyntheticsWidgetType: "uptime" | "status_list" | "single"

// The synthetics check types, as synthetics-api spells them.
#SyntheticsCheckType: "HTTP" | "TCP" | "DNS" | "GRPC" | "PING" | "SSL" |
	"K8S" | "WEBSOCKET" | "UDP" | "MULTISTEP" | "MCP" | "BROWSER" | "MOBILE" |
	"NETWORK_PATH"

// A monitor's presented state. PAUSED is a disabled monitor.
#SyntheticsMonitorState: "UP" | "DOWN" | "DEGRADED" | "NO_DATA" | "UNKNOWN" | "PAUSED"

// The uptime windows synthetics-api computes, each ending now.
#SyntheticsUptimeWindow: "1h" | "24h" | "7d" | "30d"

// At most 100 monitors: the cap POST /api/synthetics/uptimes enforces, which
// answers a longer list with a 422 rather than truncating it.
#SyntheticsMonitorIds: list.MaxItems(100) & [...string]

// Monitor labels to match, key to value. Every pair must match.
#SyntheticsMonitorLabels: {[string]: string}

#SyntheticsCheckTypes: [...#SyntheticsCheckType]
#SyntheticsMonitorStates: [...#SyntheticsMonitorState]

// Which monitors a panel draws when it does not name them, or how it narrows
// the ones it names. Every key present must match; within types and within
// states any one value matches.
//
// Labels are a map here and a list of "key:value" on the wire: the renderer
// sends them as `labels` to POST /api/synthetics/uptimes. Types go as `types`.
// That endpoint does not filter by state, so a states filter is resolved
// through GET /api/synthetics/monitors?state=…, which carries each monitor's
// state, and the uptimes are then fetched for the ids it returns.
#SyntheticsMonitorFilter: {
	labels?: #SyntheticsMonitorLabels
	types?:  #SyntheticsCheckTypes
	states?: #SyntheticsMonitorStates

	...
}

// Uptime percentages a monitor is coloured by: at or above `good` is healthy,
// at or above `warn` a warning, below it failing.
#SyntheticsUptimeThresholds: {
	good: *99.9 | #UptimePercent
	warn: *99.0 | #UptimePercent

	...
}

// Named rather than inline beside its default: an inline bound next to a
// default exports as a bare number and loses the bound. (Kept apart from the
// doc comment below, which is published.)

// A percentage, 0 to 100.
#UptimePercent: number & >=0 & <=100

// How rows are ordered. Unset, they keep the order the API returned: the
// order of monitorIds when the panel names monitors, by name otherwise.
#SyntheticsSortField: "uptime" | "name" | "state"

#SyntheticsDisplay: {
	thresholds?: #SyntheticsUptimeThresholds

	// Show each monitor's apdex (0 to 1) beside its uptime. Apdex is null for
	// a monitor whose runs in the range carry no apdex threshold.
	showApdex: *false | bool

	sort?: #SyntheticsSortField

	...
}

// A synthetics_monitor query names synthetics monitors and the widget to draw
// them as. Not "synthetics": that selectedMode is the Explore source over the
// synthetics results, a log-like query run through /explore/query.
//
// A query rather than a member of #PanelConfig for the reason #SloQuery is: it
// answers "what data".
//
// Like the slo and alert arms, this one is NOT sent to /explore/query. The
// webapp posts the selection to /api/synthetics/uptimes and, for "single",
// reads the state counts from /api/synthetics/monitors/summary (or, when the
// panel selects monitors, from the states of the monitors it selects).
//
// The panel names monitors by `monitorIds`, selects them by `filter`, or
// both, in which case the filter narrows the named ones. With neither it draws
// every monitor, the first 100 by name.
//
// Flat, discriminated only by selectedMode, with `widgetType` a plain enum,
// for the reason #SloQuery gives.
#SyntheticsMonitorQuery: {
	selectedMode!: "synthetics_monitor"
	label:         *"A" | string
	visible:       *true | bool

	// Required: this arm has no stored documents that predate it, so there
	// is no older panel for a default to keep drawing.
	widgetType!: #SyntheticsWidgetType

	// The monitors to draw, in display order unless display.sort says
	// otherwise. An id that no longer exists is reported by the API and
	// skipped.
	monitorIds?: #SyntheticsMonitorIds

	filter?: #SyntheticsMonitorFilter

	// The window the uptime covers, ending now. Absent, the panel's time range
	// applies: its config.timeFrame, else the dashboard's.
	window?: #SyntheticsUptimeWindow

	display?: #SyntheticsDisplay

	...
}

// ── infrastructure queries ──────────────────────────────────────────────────

// The fixed nesting a host map draws, outermost first. A level list follows
// this order; kubeapi rejects one that does not.
#InfrastructureEntity: "host" | "pod" | "container"

// What colours (fill) or sizes (size) a tile. Each entity supports a subset,
// checked by kubeapi against its PromQL catalog.
#InfrastructureSignal: "cpu_utilization" | "memory_utilization" | "cpu_usage" |
	"memory_usage" | "restarts" | "ready" | "pod_count"

// Canonical dimensions every result row is labelled with, whichever metric
// family (k8s or legacy) answered it.
#InfrastructureDimension: "cluster" | "env_type" | "host" | "namespace" |
	"workload" | "pod" | "container"

#InfrastructureLevel: {
	entity!: #InfrastructureEntity
	fill!:   #InfrastructureSignal
	size?:   #InfrastructureSignal
	...
}

#InfrastructureLevels: list.MinItems(1) & list.MaxItems(3) & [...#InfrastructureLevel]

// Values per dimension, matched exactly. Container is absent: no metric that
// places a container also names its host, so it could not narrow the levels
// around it.
#InfrastructureFilters: {
	cluster?:   #StringList
	host?:      #StringList
	namespace?: #StringList
	workload?:  #StringList
	pod?:       #StringList
	// The Advanced filter's expression, one entry, as logs queries store it.
	advanced_query?: #StringList
	...
}

#InfrastructureGroupBy: [...#InfrastructureDimension]

#InfrastructureFilterMode: "MFD" | "ADVANCED_QUERY"

// A host map's query: up to three nested levels sharing one filter and one
// grouping. Like the slo and alert arms it is NOT sent to /explore/query; the
// webapp posts it to /explore/hostmap, which compiles it to PromQL per level
// and per metric family. A filter value with a leading "-" excludes.
#InfrastructureQuery: {
	selectedMode!: "infrastructure"
	label:         *"A" | string
	visible:       *true | bool

	levels!: #InfrastructureLevels

	filters: *{} | #InfrastructureFilters

	// "ADVANCED_QUERY" when the filter is a WHERE-style expression, held in
	// filters.advanced_query as logs queries hold theirs, e.g.
	//   namespace = "payments" AND (cluster = "prod" OR cluster = "staging")
	// Fields are the filter dimensions; operators are =, != and IN, with AND,
	// OR, NOT and parentheses. NOT is a prefix: exclude a list as
	// NOT namespace IN (a, b).
	filterMode: *"MFD" | #InfrastructureFilterMode

	// Sections around the outermost level's tiles. Only dimensions that level
	// carries are accepted by kubeapi; a host has no namespace. Not `groupBy`,
	// which the logs and traces arms use for a list of field objects.
	groupByDimensions: *[] | #InfrastructureGroupBy

	...
}

// #Query is the discriminated union. Discriminating on selectedMode is what
// makes the generated TypeScript a union that narrows, rather than an
// intersection of open structs.
#Query: #MetricsQuery | #LogsQuery | #TracesQuery | #EventsQuery | #RumQuery | #SyntheticsQuery | #FormulaQuery | #SloQuery | #AlertQuery | #InfrastructureQuery | #SyntheticsMonitorQuery

// ── the EMPTY-CONTAINER DEFAULT rule ────────────────────────────────────────
//
// `*{} | <inline constrained type>` and `*[] | <inline constrained type>` both
// LOSE the constraint on export. The empty container subsumes the other arm, CUE
// reduces the disjunction to its default, and the encoder emits `const: {}` or a
// bare `{"type": "array"}` — silently, with no error and no $defs entry.
//
// This is not cosmetic. `functions: *[] | [...#QueryFunction]` exported as
// `{"type": "array"}`, so the entire six-variant builder-function catalog —
// argument names, tuple arities, every enum — validated nothing at all. Naming
// the arm and referencing it keeps both arms as an anyOf.
//
// So: any type that appears beside an empty default gets a name below.
#QueryFunctions: [...#QueryFunction]
#Variables: [...string]
#GroupByFields: [...#Field]
#StringList: [...string]
