// The panel — a named group of queries plus one config object.
//
// Source of truth: panelEditorFormValuesBaseSchema
//   src/pages/explorer/v2/utils/zod-schemas.ts:1017
//
// Deliberately the BASE schema, not panelEditorFormValuesSchema (:1144). The
// latter wraps this in z.preprocess(migrateToNewSchema, …), and migration is not
// part of the shape — it is the step that produces a document OF this shape from
// an older one. Migration belongs in the Go module (see the repo README), which
// is also why the zod itself keeps the base separate: a codec cannot use
// preprocess, because preprocess is unidirectional.
package panel

#Panel: {
	// Optional, and blank is meaningful rather than missing. The zod is a bare
	// z.string() (no .min(1)), and its own description says so: blank means
	// automatic, with the display title derived from the query at render time
	// (panel-name.ts) instead of stored. So "" is a real value here, not a
	// placeholder for one — and a panel that omits the key entirely means the
	// same thing. Neither is an error; both render a derived title.
	name?: string

	description?: string

	panelType: *"timeSeries" | #PanelType

	// No .catch() here either: `queries` must be present. An empty array is
	// permitted by the shape, though a panel with no queries renders nothing.
	queries!: #Queries

	config: *#DefaultPanelConfig | #PanelConfig

	...
}

#Queries: [...#Query]

// The zod's superRefine (:1036) additionally validates the formula dependency
// graph across queries:
//
//   - a formula's expression must not be empty
//   - it must parse as an expression (mathjs)
//   - it must not reference its own label
//   - it must reference at least one query
//   - referenced labels must exist among the panel's queries
//   - and the reference graph must be acyclic, reported as
//     `Query "B" references "A" before it is defined`
//
// None of that is expressible here. Every rule spans sibling array elements, and
// two of them require parsing an expression language. They belong in the Go
// validator alongside the alert document's `metric_query_label` check — same
// reason, same place. Cycle detection in particular is an algorithm, not a
// constraint.
//
// #FormulaQuery's regex still catches the character-level part (labels, digits,
// the four operators, parentheses), which is the zod's own .refine() at :713.

// DEFAULT_PANEL_CONFIG :60, as one concrete instance. Named so the default on
// `config` above keeps the #PanelConfig arm intact on export.
#DefaultPanelConfig: {
	lineWidth:                 1.5
	fillOpacity:               0
	stackSeries:               false
	showTrend:                 false
	showLabel:                 false
	showPercentageChange:      false
	percentageChangeColorMode: "standard"
	mergeTables:               true
	legendVisibility:          true
	legendPlacement:           "bottom"
	tooltipMode:               "single"
	yAxisLabelFormatter:       "auto"
	yAxisMin:                  null
	yAxisMax:                  null
	yAxisIncludeZero:          false
	yAxisDecimals:             null
	step:                      "auto"
	thresholds: [#DefaultThreshold]
	thresholdDisplayMode: "off"
	valueOptions: {show: "calculate"}
	colorScheme: #DefaultColorScheme
	tableSettings: {columnWidths: {}, columnPinning: {}, columnVisibility: {}, sorting: []}
	alignColumns:   []
	columnFormatting: []
	topListDisplayMode:    "flat"
	visualFormattingRules: []
}
