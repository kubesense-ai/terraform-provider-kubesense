// Panel configuration — an exact replica of panelConfigSchema.
//
// Source of truth:
//   src/pages/explorer/v2/utils/zod-schemas.ts
//     alignColumnMappingSchema  :120
//     alignColumnSchema         :127
//     thresholdOptionsSchema    :377
//     panelConfigSchema         :413
//     DEFAULT_PANEL_CONFIG      :60
//   src/pages/explorer/v2/color-palette/constant.ts
//     COLOR_PALETTE_KEYS        :115
//     SERIES_REDUCER_KEYS       :159
//     THRESHOLD_DISPLAY_MODE_KEYS :178
//   src/pages/explorer/v2/constants.tsx
//     PERCENTAGE_CHANGE_COLOR_MODE_OPTIONS :766
//     THRESHOLD_COLORS          :781
//     VALUE_CALCULATION_OPTIONS :796
//   src/shared/components/uplot-base/plugins/tooltip-plugin.tsx
//     TooltipMode               :44
//   src/pages/explorer/v2/utils/panel-time-frame.ts
//     PANEL_TIME_FRAME_KEYS
//
// This is the surface where the zod uses `.catch()` most heavily — every field
// has one, plus one on the whole object. Each becomes a CUE default. Remember the
// two are not the same: `.catch()` repairs a wrong-typed value, a CUE default
// only fills an absent one. A payload with `lineWidth: "thick"` is silently
// repaired by the form and rejected here, which is the intended difference.
package panel

// ── enum key sets, mirrored from their exported tuples ──────────────────────

// COLOR_PALETTE_KEYS
#ColorPaletteKey: "default" | "success" | "warning" | "error"

// SERIES_REDUCER_KEYS
#SeriesReducer: "last" | "min" | "max"

// THRESHOLD_DISPLAY_MODE_KEYS
#ThresholdDisplayMode: "off" | "lines" | "lines_dashed" |
	"filled_regions" | "filled_regions_and_lines" |
	"filled_regions_and_lines_dashed"

// PERCENTAGE_CHANGE_COLOR_MODE_OPTIONS — the zod builds this enum at runtime with
// Object.values(...).map(o => o.value), so the values are transcribed here.
#PercentageChangeColorMode: "standard" | "invert" | "same_as_value"

// VALUE_CALCULATION_OPTIONS — likewise built at runtime from the options map.
// These reduce the per-bucket values of a stat panel. "Whole range" is not a
// member: it is the absence of a calculation (see #ValueOptions), so there is
// one way to say it.
#ValueCalculation: "mean" | "standard_deviation" | "sum" | "max" | "min" |
	"median" | "last" | "first" | "range" | "min_above_zero"

// TooltipMode
#TooltipMode: "single" | "all"

// PANEL_TIME_FRAME_KEYS — a window relative to now that a panel queries in
// place of the dashboard's time. The rolling ones end at now. "week_to_date"
// starts Monday 00:00 of the current week and "month_to_date" the 1st 00:00 of
// the current month, both ending at now. "previous_week" is the previous Monday
// 00:00 to Sunday 23:59:59.999 and "previous_month" the 1st to the last day
// 23:59:59.999 of the previous month. Calendar boundaries fall in the viewer's
// timezone. Every value is relative; a panel cannot pin an absolute range.
#PanelTimeFrame: "last_5m" | "last_15m" | "last_30m" | "last_1h" | "last_3h" |
	"last_6h" | "last_12h" | "last_24h" | "last_2d" | "last_7d" | "last_30d" |
	"week_to_date" | "month_to_date" | "previous_week" | "previous_month"

// TOP_LIST_DISPLAY_MODES — top-list panels only. "stacked" splits each row's bar
// by the group-bys after the first, so it needs two or more of them. The webapp
// renders "flat" regardless when the data carries a single dimension, which
// makes a stored "stacked" on a one-group-by panel stale rather than invalid —
// hence no cross-field rule here.
#TopListDisplayMode: "flat" | "stacked"

// PANEL_TYPES :166 — note "timeSeries" here, camel-cased, versus the query-level
// chart_type's "timeseries". Both spellings are real and they are not the same
// field.
//
// An "slo" panel names service level objectives rather than running a metrics
// query, but it still carries a query: #SloQuery, the `slo` arm of the query
// union, which holds the widget type and which objectives to draw.
//
// (An earlier revision kept that selection in #PanelConfig and left `queries`
// empty. It moved because the selection answers "what data", which is what a
// query is.)
//
// An "alert" panel works the same way and for the same reason: #AlertQuery
// names a rule and the widget to draw it as, and is served by the alert
// endpoints rather than /explore/query.
//
// A "hostMap" panel carries one #InfrastructureQuery, which /explore/hostmap
// compiles to PromQL server-side, so the panel never holds PromQL of its own.
//
// A "syntheticsMonitor" panel carries one #SyntheticsMonitorQuery and is
// served by the synthetics endpoints, as an "slo" panel is by the SLO ones.
#PanelType: "timeSeries" | "stat" | "table" | "list" | "bar" | "pie" |
	"topList" | "spl" | "sql" | "slo" | "treemap" | "alert" | "hostMap" |
	"syntheticsMonitor"

// ── thresholds ──────────────────────────────────────────────────────────────

// thresholdOptionsSchema :377. `threshold` is a union of a number with the two
// infinity SENTINEL STRINGS — JSON has no Infinity literal, so the zod encodes
// them as "-Infinity"/"Infinity" and the default threshold is the negative one,
// meaning "from the bottom of the range".
#ThresholdValue: number | "-Infinity" | "Infinity"

#Threshold: {
	threshold: *"-Infinity" | #ThresholdValue

	// THRESHOLD_COLORS supplies three presets (red #e24d42, green #56a64b,
	// orange #ef843c) but the field is a free string — the colour picker emits
	// arbitrary hex — so it is not enumerated.
	color: *"#56a64b" | string

	isDefault:     *false | bool
	isLabelShown?: bool
	label?:        string

	...
}

// ── merged-table column alignment ───────────────────────────────────────────

// alignColumnMappingSchema :120 — one query's source field for a merged
// dimension.
#AlignColumnMapping: {
	queryLabel!:  string
	field!:       string
	is_attribute: *false | bool

	...
}

// alignColumnSchema :127 — one merged dimension column, e.g. aligning
// `namespace` against `k8s_namespace_name` across two queries.
#AlignColumn: {
	id!:         string
	displayName: *"" | string
	mappings:    *[] | #AlignColumnMappings

	...
}

// ── column formatting (table panels) ───────────────────────────────────────
//
// Why the style list and the cell/colour modes are FLAT enums rather than
// discriminated unions. Kept out of the doc comments below because those are
// PUBLISHED as the schema's `description`, and cue's own failure modes are not
// something a consumer of the contract should have to read.
//
// Nine of the eleven `#ConditionStyle` values carry no payload, and a
// payload-free union arm does not survive this pipeline: `{type!: "x", ...}`
// exports as `{"const":{"type":"x"}}`, silently closing an arm that `...`
// promises is open. That is the EMPTY-CONTAINER class from query.cue with a
// non-empty const, which is why TestExportShape does not catch it. Pairing such
// an arm with a NAMED default — the construction `colorScheme` uses — also sent
// cue's subsumption check into unbounded recursion and killed the generator with
// a stack overflow. `colorScheme` survives only because every one of its arms
// carries a second member.

// How a cell renders its value. "number" writes the value, "bar" writes the
// value with an inline bar sized against the column's range.
//
// A "trend" arm and its #ColumnTrend settings were declared here before either
// side implemented them. Removed rather than left standing: a schema that
// accepts a value nothing renders invites a preset the API stores and the app
// then draws as an ordinary number. Re-add it with the renderer, not before.
#ColumnCellType: "number" | "bar"

// How a column picks its colours. "conditions" evaluates the rule list in
// order; "range" maps the column's values onto a continuous palette.
#ColumnColorMode: "conditions" | "range"

#ConditionOperator: ">" | ">=" | "<" | "<=" | "=" | "!="

// The "show with" choices. The three families (solid background, light
// background, coloured text) are spelled out rather than composed from
// colour × treatment, because that is how they are presented and because a
// composed pair would allow combinations the picker never offers.
//
// The two `custom-*` values are the only ones that read `color`.
// The middle severity here is YELLOW, while the range palettes below use
// ORANGE. That split is deliberate and matches Datadog: a condition paints a
// discrete severity, where yellow reads as "warning" between red and green; a
// palette is a continuous ramp, where orange sits better beside red.
#ConditionStyle: "red-background" | "yellow-background" | "green-background" |
	"light-red-background" | "light-yellow-background" | "light-green-background" |
	"red-text" | "yellow-text" | "green-text" |
	"custom-background" | "custom-text"

// The styles a visual formatting rule may carry: the whole #ConditionStyle
// set. A stat panel paints its block solid or light, or colours its value
// text, so every family applies there. A top-list block only has a background
// and sits beside blocks in their series colour, where a solid fill reads as
// another series — so the top-list PICKER offers the light backgrounds and
// custom only. That narrowing lives in the editor, not here: the contract
// accepts every style on both surfaces, and a solid style on a top list is
// simply drawn as the fill it names.
#VisualFormattingStyle: #ConditionStyle

// One "if the value is X, paint the block Y" rule, for top-list and stat panels.
//
// Deliberately the same shape as #Condition, down to LAST-MATCH-WINS: the two
// are the same idea on different surfaces, and one vocabulary means one picker
// and one thing to explain. Kept as its own definition rather than a reuse of
// #Condition because the style set is narrower and because `id` is required
// here — the form uses it as a field-array key.
//
// Every block is judged by ITS OWN value, so on a stacked row the rule marks
// the block that breached rather than the whole bar.
//
// Supersedes `thresholds` for top-list panels, which no longer offer the
// Thresholds section, and for stat panels, which no longer offer it either. A
// panel saved before this existed has an empty list and keeps being coloured
// by its thresholds; the first rule added moves it over.
#VisualFormattingRule: {
	id!:       string
	operator!: #ConditionOperator
	value!:    number

	style: *"light-red-background" | #VisualFormattingStyle

	// Read only when `style` is "custom-background", and OPTIONAL even then: a
	// custom rule with no colour is INERT, painting nothing, rather than an
	// error. Deliberate, and the same stance #Condition takes.
	//
	// Requiring it here was considered and rejected. The zod cannot mirror a
	// conditional requirement without a .refine, which would cost the rule
	// schema its TOTALITY — and `visualFormattingRules` carries a `.catch([])`,
	// so one refusable entry would take every valid sibling rule with it on
	// read. Requiring it on this side alone is worse still: the browser would
	// accept a document the API rejects, which is the drift
	// `conformance.test.ts` exists to catch, and the editor would build panels
	// that cannot be saved.
	//
	// The editor closes the gap where it opens: choosing "Custom background"
	// seeds a colour immediately, so the half-configured state is not reachable
	// from the UI. What remains is a hand- or agent-written preset saying
	// "custom" and meaning nothing, which renders as though the rule were absent.
	color?: string

	...
}

// One colouring rule. The LAST condition a value satisfies wins, in list
// order — not the most specific and not the first. Two rules can overlap
// (`> 10` and `> 15`), the user controls the order, so the tie-break has to be
// stated rather than discovered.
//
// Last rather than first because a threshold list reads as escalating: rules
// get added as the bar rises, so `> 10 green` above `> 15 red` paints 58 red.
// This said first-wins until 2026-08-14, under which the general rule shadowed
// every more specific rule below it and the severe one could never fire.
//
// A cell whose value is not a number never satisfies a condition.
#Condition: {
	operator!: #ConditionOperator
	value!:    number

	style: *"red-background" | #ConditionStyle

	// Read only when `style` is "custom-background" or "custom-text". Free-form
	// like #Threshold.color — the picker emits arbitrary hex.
	color?: string

	...
}

// How a string rule compares the cell's text to `value`. Case-sensitive: the
// text a rule names is the text the cell shows, and "is" should mean is.
//
// "is-empty" and "is-not-empty" read NO `value` at all, and exist because the
// other six cannot express emptiness. An empty `value` is reserved to mean "a
// rule not written yet" (see #StringCondition.value), so `is-not` with a blank
// cannot be "the cell has text" — the two readings are indistinguishable on
// screen, and the unfinished one is far commoner. Naming the intent settles it:
// the operator says emptiness is the subject, and the value input disappears.
#StringConditionOperator: "is" | "is-not" | "contains" | "does-not-contain" |
	"starts-with" | "ends-with" | "is-empty" | "is-not-empty"

// What a string rule writes into the cell in place of its text. Two arms on
// `mode`, like #ColorScheme on `type`, because they take different inputs:
// "cell" swaps the WHOLE text for `with`; "substring" swaps every occurrence
// of `search` for `with`. A union rather than one struct with an optional
// `search` so that a "cell" replace cannot carry a search string that
// nothing reads. A "substring" arm whose `search` is empty replaces nothing,
// there being no occurrence of nothing to find, rather than salting `with`
// between every character.
//
// An empty `with` means different things in the two arms, and the split is
// deliberate. Under "substring" it DELETES the match, which is how a prefix is
// stripped. Under "cell" it is INERT rather than blanking the cell: the editor
// seeds an empty `with` the moment "Replace" is ticked, so writing it through
// would empty every matching cell before anything had been typed, which reads
// as data loss. Blanking a cell outright is the rarer intent and a single
// space expresses it.
#StringReplace: #CellReplace | #SubstringReplace
#CellReplace: {mode!: "cell", with!: string, ...}
#SubstringReplace: {mode!: "substring", search!: string, with!: string, ...}

// One "if the text is X, then ..." rule, for STRING columns. The counterpart
// of #Condition, kept as its own definition for the reason #VisualFormattingRule
// is: the two share the "show with" vocabulary and nothing else. A threshold
// compares numbers and always paints; a string rule compares text and has TWO
// optional effects, so it may paint, replace, or both. Widening
// #Condition.value to `number | string` was considered and rejected: it would
// let a "contains" rule carry a number and a ">" rule carry text, and the
// editor would have to guess which picker to show.
//
// `style` is absent when "Show with" is unchecked, and `replace` when "Replace"
// is. A rule with neither is legal and inert, for the reason
// #VisualFormattingRule.color gives: a conditional requirement costs the zod
// its totality.
//
// The two effects resolve DIFFERENTLY, and the asymmetry is deliberate.
//
// The fill takes the LAST matching rule that has a style, in list order, the
// same tie-break as #Condition: a cell has one colour, so one rule has to win,
// and last-wins lets the list read as escalating.
//
// The replacement is a PIPELINE: EVERY matching rule's replace applies, in
// list order, each on the output of the rule before it. Last-wins was
// considered here too and rejected because text composes where colour does
// not. Under last-wins a `starts-with "/api"` prefix-strip written below a
// `contains "v1"` rewrite would silently cancel the rule above it, with
// nothing in the editor saying so. Chaining means two substring rules
// compose and a later "cell" replace overwrites whatever the earlier rules
// produced, which is what the order on screen suggests. Each rule is still
// JUDGED against the cell's original text, not the text the rules before it
// produced; only the replacement input chains. Judging the rewritten text
// would let a rule's own output stop it matching.
//
// Only a cell whose raw value is a string is judged. A number is left to
// #Condition, and a cell that is not text is never painted or rewritten by
// these rules. When a column carries both lists they evaluate as one
// sequence, `conditions` then `stringConditions`; the two can only both
// match a numeric-looking string such as "404", and the string list sitting
// later means it wins there, which is the more specific statement about
// that column.
#StringCondition: {
	operator!: #StringConditionOperator

	// An EMPTY value matches NOTHING under the six operators that read it. It is
	// a rule that has not been written yet, not a rule about the empty string:
	// four of them are otherwise true for every cell, since every string starts
	// with, ends with and contains "", and "is-not" holds for any non-empty one.
	// Left alone, adding a rule would paint the whole column before stating a
	// predicate. #Condition has no such state to guard, being seeded with a real
	// number.
	//
	// "is-empty" and "is-not-empty" ignore this field, which is what lets
	// emptiness be asked about without colliding with the unfinished state.
	value!: string

	replace?: #StringReplace

	// Any of the eleven #ConditionStyle values. OPTIONAL, unlike
	// #Condition.style, because painting is one of two effects here and the
	// checkbox that enables it can be off.
	style?: #ConditionStyle

	// Read only when `style` is "custom-background" or "custom-text", and
	// optional even then, for the reason #VisualFormattingRule.color gives.
	color?: string

	...
}

// Continuous colouring across the column's values. `min`/`max` are null for
// "derive it from the data", which is the default and what the UI shows as
// "auto".
#ColumnRange: {
	palette: *"green" | #ColumnPalette
	scale:   *"logarithmic" | #ColumnScale
	min:     *null | number
	max:     *null | number

	...
}

// How a value's position in [min, max] is computed.
//
// "linear" spreads the palette evenly, which is right for bounded values like a
// percentage. It is a poor fit for the counts this product mostly shows: those
// are power-law distributed, so with a max three orders of magnitude above the
// median every row but the largest lands within a few percent of the minimum and
// the column reads as blank.
//
// "logarithmic" spreads by order of magnitude instead, and is the DEFAULT for
// that reason: a count column is the common case here, and on a linear ramp it
// renders as blank cells and one coloured row.
//
// It is computed over log(1 + v), so a range starting at zero — which any count
// column with an idle series has — still spreads properly instead of silently
// behaving linearly. Only a NEGATIVE minimum falls back to linear, log being
// genuinely undefined there.
#ColumnScale: "linear" | "logarithmic"

// One host map level's colours: a table column's range, plus `reverse`, which
// flips the palette end for end. Every two-hue palette is red at the minimum,
// so without it "high CPU is red" could not be said.
#HostMapColor: #ColumnRange & {
	reverse: *false | bool
}

#HostMapColors: {
	host?:      #HostMapColor
	pod?:       #HostMapColor
	container?: #HostMapColor
	...
}

// Two families of six. The GRADIENT family ramps from transparent at the
// minimum to full colour at the maximum, so a low cell reads as an ordinary row
// and only high cells draw the eye. The SOLID family carries full colour across
// the whole range, for when every cell should be legible as a band.
//
// For the two-hue ramps the name states the direction: "red-green" is red at
// the MINIMUM and green at the maximum. Reversing it is a matter of swapping
// min/max, so there is no separate "green-red".
#ColumnPalette:
	"green" | "orange" | "red" | "blue" | "red-green" | "red-blue" |
	"solid-green" | "solid-orange" | "solid-red" | "solid-blue" |
	"solid-red-green" | "solid-red-blue"

// Formatting for ONE table column.
//
// `id` is a uuid the form uses as its field-array key; `columnId` is the column
// it applies to. A columnId matching nothing currently rendered is not an error
// — the entry simply does not apply, which is what lets a saved panel survive a
// query being edited.
//
// `displayName` is the outermost of four ways to name a column and the one that
// wins, over `alignColumns[].displayName` for a merged dimension,
// `queries[].columnFields[].alias` for a list column, and
// `queries[].labelOptions.format` for a value column, then the field catalog's
// label, then the raw key. An EMPTY or whitespace-only value counts as absent
// and falls through, so clearing the input restores the derived name instead of
// rendering a blank header.
//
// `conditions` judges numbers and `stringConditions` text. Two lists rather
// than one mixed list because the rules have different shapes and different
// pickers, and because a column carries whichever list its kind offers: both
// survive a query edit that changes the kind, and the list that no longer
// applies matches nothing and stays stored, so switching the query back
// restores the rules.
#ColumnFormat: {
	id!:       string
	columnId!: string

	cellType:  *"number" | #ColumnCellType
	colorMode: *"conditions" | #ColumnColorMode

	conditions:       *[] | #Conditions
	stringConditions: *[] | #StringConditions
	range?:           #ColumnRange

	displayName?: string
	hidden?:      bool
	width?:       number & >0

	...
}

// ── colour scheme ───────────────────────────────────────────────────────────

// colorScheme :511 — a discriminated union on `type`.
#ColorScheme: #PaletteScheme | #SingleColorScheme | #ShadesScheme |
	#ThresholdsScheme | #CustomColorScheme

// Open like everything else. `colorScheme`'s default arm (#DefaultColorScheme)
// stays closed and exact — it is a concrete instance, not a type — and these
// arms are what a real document with an extra member matches instead.
#PaletteScheme: {type!: "palette", palette!: #ColorPaletteKey, ...}
#SingleColorScheme: {type!: "single", color!: string, ...}
#ShadesScheme: {type!: "shades", color!: string, ...}
#ThresholdsScheme: {type!: "thresholds", seriesReducer!: #SeriesReducer, ...}

// Explicit per-series colours applied in order. The zod restricts these to hex
// codes (#rgb, #rgba, #rrggbb, #rrggbbaa) so callers can drive semantic colours
// such as alert severity, and requires at least one.
#CustomColorScheme: {
	type!: "custom"
	colors!: [#HexColor, ...#HexColor]

	...
}

#HexColor: string & =~"^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$"

// ── the config object ───────────────────────────────────────────────────────

#PanelConfig: {
	// From graphStyles, flattened into config by the migration at
	// migrateToNewSchema :944.
	lineWidth:   *1.5 | (number & >=0 & <=10)
	fillOpacity: *0 | (number & >=0 & <=100)
	stackSeries: *false | bool

	// stat-panel display toggles
	showTrend:                 *false | bool
	showLabel:                 *false | bool
	showPercentageChange:      *false | bool
	percentageChangeColorMode: *"standard" | #PercentageChangeColorMode

	// From tableOptions. The zod is z.literal(true).catch(true) — merging is no
	// longer optional, so the only legal value is true. Kept because stored
	// documents carry the key.
	mergeTables: *true | true

	// From legendOptions, with renamed keys.
	legendVisibility: *true | bool
	legendPlacement:  *"bottom" | "bottom" | "right"
	tooltipMode:      *"single" | #TooltipMode

	// Y axis. yAxisMin/Max are nullable — null means "auto", a hard number
	// clamps that end of the axis.
	yAxisLabelFormatter: *"auto" | string
	yAxisMin:            *null | number
	yAxisMax:            *null | number
	yAxisIncludeZero:    *false | bool
	yAxisDecimals:       *null | #YAxisDecimals

	// Query resolution. "auto" lets the range decide; a positive integer forces
	// a step in seconds.
	step: *"auto" | #Step

	thresholds:           *[#DefaultThreshold] | #Thresholds
	thresholdDisplayMode: *"off" | #ThresholdDisplayMode

	valueOptions: #ValueOptions
	colorScheme:  *#DefaultColorScheme | #ColorScheme
	tableSettings: #TableSettings
	alignColumns:  *[] | #AlignColumns

	// Per-column formatting. Table and list panels.
	columnFormatting: *[] | #ColumnFormatting

	// Flat or stacked bars. Top-list panels only.
	topListDisplayMode: *"flat" | #TopListDisplayMode

	// Value-driven block painting. Top-list and stat panels; supersedes
	// `thresholds` on both.
	visualFormattingRules: *[] | #VisualFormattingRules

	// Event markers on this panel. Absent means inherit the dashboard's.
	// Timeseries and bar panels only.
	eventOverlay?: #PanelEventOverlay

	// How each host map level is coloured, keyed by entity so removing or
	// re-adding a level never moves one level's colours onto another. Host map
	// panels only. A level with no entry gets a neutral ramp.
	hostMapColors?: #HostMapColors

	// The window this panel queries instead of the dashboard's time. Absent
	// means the dashboard's time. Every query the panel makes uses it, and the
	// panel does not drag-to-zoom.
	timeFrame?: #PanelTimeFrame

	...
}

// ── event overlays ──────────────────────────────────────────────────────────

// A set of events drawn as vertical markers on a timeseries or bar panel, one
// marker per event matching the filter.
#EventOverlay: {
	// Label shown in the legend and on the marker's hover card.
	name?: string
	// Colour of the markers, a name from a fixed palette rather than a CSS
	// value.
	color: *"red" | #EventOverlayColor
	// Which events to mark, written like an events query's filters.
	filters: *{} | #Filters
	// "ADVANCED_QUERY" reads the WHERE text from `filters.advanced_query`.
	filterMode: *"MFD" | #EventOverlayFilterMode

	...
}

#EventOverlayFilterMode: #FilterMode & ("MFD" | "ADVANCED_QUERY")
#EventOverlays: [...#EventOverlay]

#EventOverlayColor: "red" | "purple" | "orange" | "gray" | "green" | "blue"

// How a panel picks its event overlays. "inherit" uses the dashboard's,
// "custom" uses `overlays` instead (it replaces, never merges), and "off" draws
// none.
#PanelEventOverlay: {
	mode!: "inherit" | "custom" | "off"
	// Read only when mode is "custom".
	overlays?: #EventOverlays

	...
}

// `show` has a single legal value; the field exists so a future "hide" mode is
// additive rather than a shape change.
#ValueOptions: {
	show: *"calculate" | "calculate"

	// Optional on purpose. Absent means the number is computed over the whole
	// range in one aggregate for ClickHouse-backed sources (rum, logs, traces,
	// events, synthetics), so a unique count or percentile is exact; metrics
	// panels use the last value. Any value set here instead reduces the
	// per-bucket values.
	calculation?: #ValueCalculation

	...
}

// Table-level state that survives a panel save, for the table and list panels.
// Every key after `columnWidths` is a TanStack Table state slice under its own
// name and shape, so the persisted value is handed to `useReactTable` as-is.
#TableSettings: {
	// Per-column pixel widths, keyed by column id. (TanStack calls this
	// `columnSizing`; the name predates the rule above and renaming needs a
	// migration.)
	columnWidths: *{} | #ColumnWidths
	// `ColumnPinningState`: column ids pinned to each edge, in order.
	columnPinning: *{} | #ColumnPinning
	// `VisibilityState`: column id → shown. Absent ids are visible.
	columnVisibility: *{} | #ColumnVisibility
	// `SortingState`: applied in order, first entry primary.
	sorting: *[] | #TableSorting

	...
}

// ── named types for every empty/struct default above ────────────────────────
//
// See the EMPTY-CONTAINER DEFAULT rule in query.cue: a constrained type sitting
// beside a default must be named, or the constraint is silently dropped on
// export.
#AlignColumnMappings: [...#AlignColumnMapping]
#AlignColumns: [...#AlignColumn]
#ColumnFormatting: [...#ColumnFormat]
#Conditions: [...#Condition]
#StringConditions: [...#StringCondition]
#VisualFormattingRules: [...#VisualFormattingRule]
#Thresholds: [...#Threshold]
#ColumnWidths: {[string]: number}
#ColumnPinning: {
	left?:  [...string]
	right?: [...string]
	...
}
#ColumnVisibility: {[string]: bool}
#TableSorting: [...#TableSortingEntry]
#TableSortingEntry: {
	id:   string
	desc: bool
	...
}
#Step: int & >0
#YAxisDecimals: int & >=0 & <=4

// DEFAULT_PANEL_CONFIG.thresholds :81 — a single green threshold from the
// bottom of the range, label hidden.
#DefaultThreshold: {
	threshold:    "-Infinity"
	color:        "#56a64b"
	isDefault:    true
	isLabelShown: false
	label:        ""
}

// DEFAULT_PANEL_CONFIG.colorScheme :101
#DefaultColorScheme: {type: "palette", palette: "default"}
