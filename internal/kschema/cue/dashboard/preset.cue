// Package dashboard is an exact replica of the dashboard preset zod schemas.
//
// Source of truth:
//   src/pages/dashboard/v2/types.ts
//     DASHBOARD_VARIABLE_TYPES        :8
//     DASHBOARD_VARIABLE_SELECT_TYPE  :16
//     dashboardVariableSelectTypeSchema :35
//     dashboardVariableSchema         :42
//     subGridLayoutItemSchema         :122
//     subGridSchema                   :130
//     dashboardPresetSchema           :140
//
// The preset is the ENTIRE dashboard body. kubeapi stores it as a string
// (observability/dashboard/pdata.go — `Preset string`) and, on every write,
// migrates it and validates it against this contract. It decodes the document
// into a generic map rather than a Go struct, so unknown fields survive a write
// untouched: nothing on the server side drops a member it does not recognise.
// The webapp parses it through presetStringToObjectCodec (:180), a
// string-to-object codec, so the persisted column holds JSON-encoded JSON.
package dashboard

import (
	"strings"

	"go.kubesense.ai/api/schema/cue/panel"
)

// ── layout ──────────────────────────────────────────────────────────────────

// subGridLayoutItemSchema :122 — one react-grid-layout cell. `i` is the item
// key that ties a cell to the thing it positions; x/y are grid coordinates and
// w/h are spans, all in grid units rather than pixels.
//
// Every member is required with no default: a cell missing its position is not
// something to guess at.
#LayoutItem: {
	// Identifier linking this cell to what it lays out.
	i!: string
	// Column offset from the left edge, in grid units.
	x!: number
	// Row offset from the top, in grid units.
	y!: number
	// Width in grid columns.
	w!: number
	// Height in grid rows.
	h!: number

	...
}

#Layout: [...#LayoutItem]

// ── sub-grids (collapsible rows) ────────────────────────────────────────────

// subGridSchema :130 — a collapsible group holding its own panels and its own
// layout. Note the nesting is one level only: a sub-grid contains panels, never
// further sub-grids.
#SubGrid: {
	id!:    string
	title!: string

	// Whether the group is folded shut in the UI.
	collapsed: *false | bool

	// Panels belonging to this group, laid out by gridLayout below rather than by
	// the dashboard's top-level layout.
	panels: *[] | #Panels

	// Positions for this group's own panels.
	gridLayout: *[] | #Layout

	...
}

#SubGrids: [...#SubGrid]

// ── tabs ────────────────────────────────────────────────────────────────────

// One tab: an id and a title, plus a page of its own. A page is the same four
// members the untabbed top level carries — panels, gridLayout, subGrids and
// subGridLayout — so a tab can hold rows.
//
// Every tab rule lives in schema/tabs.go, not here, because each is refused for
// every origin, including the UI's own save, where an ordinary contract finding
// is only logged. That covers the id (stable, used in the ?tab= URL: 1 to 40
// lowercase letters, digits or dashes, unique) and the title (not blank, at
// most 50 characters, unique ignoring case), as well as the top-level page
// staying empty, sub-grid ids unique across tabs, and one layout cell per panel.
#Tab: {
	panels:        *[] | #Panels
	gridLayout:    *[] | #Layout
	subGrids:      *[] | #SubGrids
	subGridLayout: *[] | #Layout

	...
}

#Tabs: [...#Tab]
#Panels: [...panel.#Panel]

// ── variables ───────────────────────────────────────────────────────────────

// DASHBOARD_VARIABLE_TYPES :8 — note the STATIC type's wire value is "custom",
// not "static". The constant is named STATIC; the persisted discriminator says
// custom.
#VariableType: "textbox" | "custom" | "traces" | "metrics" | "logs" | "rum"

// DASHBOARD_VARIABLE_SELECT_TYPE :16
#SelectType: "multiple" | "single"

// The field a datasource-backed variable draws its values from. Structurally the
// same three members as the panel package's #Field, but restated because the zod
// declares it inline per variant (:68, :85, :105) with a non-empty field name
// rather than reusing fieldSchema.
#VariableField: {
	field!:        string & !=""
	type!:         string
	is_attribute!: bool

	...
}

// dashboardVariableSchema :42. `meta` is a discriminated union on
// `variableType`, so the outer object carries only name/description.
#Variable: {
	// Referenced in queries as $name. The regex is the zod's own: must start with
	// a letter or underscore, then letters, digits and underscores only, max 20
	// characters — so it is safe to substitute into PromQL and filter values
	// without escaping.
	name!: string & !="" & =~"^[a-zA-Z_][a-zA-Z0-9_]*$" & strings.MaxRunes(20)

	// Required with no default in the zod, so an empty string is valid but the
	// key must be present.
	description!: string

	meta!: #VariableMeta

	...
}

#VariableMeta: #TextboxVariable | #CustomVariable | #TracesVariable |
	#LogsVariable | #MetricsVariable | #RumVariable

// A free-text value the viewer types.
#TextboxVariable: {
	variableType!: "textbox"
	defaultValue:  *"" | string
	value:         *"" | string

	...
}

// A fixed option list authored with the dashboard. At least one option is
// required — a picker with nothing in it cannot be resolved.
#CustomVariable: {
	variableType!: "custom"
	options!: [string, ...string]
	value:      *[] | #StringValues
	selectType: *"multiple" | #SelectType

	...
}

// Values discovered by querying the traces store for a field's distinct values.
#TracesVariable: {
	variableType!: "traces"
	fieldMeta!:    #VariableField
	value:         *[] | #StringValues
	selectType:    *"multiple" | #SelectType

	// Narrows which rows the distinct values are drawn from.
	filters: *{} | #VariableFilters

	// Only MFD and ADVANCED_QUERY here — a variable cannot be driven by an SPL or
	// SQL query, unlike a panel query's filterMode.
	filterMode: *"MFD" | "MFD" | "ADVANCED_QUERY"

	...
}

// Same as traces, against the logs store.
#LogsVariable: {
	variableType!: "logs"
	fieldMeta!:    #VariableField
	value:         *[] | #StringValues
	selectType:    *"multiple" | #SelectType
	filters:       *{} | #VariableFilters
	filterMode:    *"MFD" | "MFD" | "ADVANCED_QUERY"

	...
}

// Values drawn from a metric's label. Carries `metric` in addition to the
// field, and has NO filterMode — the metrics path has no advanced-query mode.
#MetricsVariable: {
	variableType!: "metrics"
	metric!:       string & !=""
	fieldMeta!:    #VariableField
	value:         *[] | #StringValues
	selectType:    *"multiple" | #SelectType
	filters:       *{} | #VariableFilters

	...
}

// Top values of a RUM field for one event type, scoped by the variable's own
// RUM filter. Applying it is an ordinary field filter. rumEventType is required
// with no default, like #RumQuery: without it there is no table to read (the UI
// picks "view" when it authors one, but a stored variable must say so).
#RumVariable: {
	variableType!: "rum"
	rumEventType!: panel.#RumEventType
	fieldMeta!:    #VariableField
	value:         *[] | #StringValues
	selectType:    *"multiple" | #SelectType
	filters:       *{} | #VariableFilters
	filterMode:    *"MFD" | "MFD" | "ADVANCED_QUERY"

	...
}

#StringValues: [...string]
#VariableFilters: {[string]: [...string]}
#DashboardVariables: [...#Variable]

// ── the preset ──────────────────────────────────────────────────────────────

// dashboardPresetSchema :140.
//
// gridLayout and panels are required with no .catch(); everything else defaults
// to empty. On a tabbed dashboard both are empty and each tab carries its own
// page instead. Panels at the top level are positioned by gridLayout; panels inside
// a sub-grid are positioned by that sub-grid's own gridLayout, and subGridLayout
// positions the sub-grids themselves.
#Preset: {
	// The document's schema version, and the entry point for migration. A reader
	// migrates from whatever version the document was saved at up to
	// CurrentVersion, one step at a time, then renders — which is what lets ONE
	// renderer display dashboards written years apart.
	//
	// Versioned at the DOCUMENT level, not per panel. A nested panel carries no
	// version of its own; the preset's chain migrates its panels as part of
	// migrating the preset. Grafana does the same: schemaVersion sits on the
	// dashboard and DashboardMigrator reaches into panels from there. Versioning
	// panels independently would mean a document whose parts are at different
	// versions, and no single number to key a migration off.
	//
	// Absent means LEGACY, which is version 0 — NOT 1. Every dashboard already
	// stored predates this field, so it is the one population guaranteed never
	// to have been checked against this contract; calling it v1 would assert a
	// shape it does not have. `legacy -> v1` is a real step with a real
	// transform (migrate/dashboard), not a version the chain pretends happened.
	//
	// The default below is what a document gets STAMPED with, and it must equal
	// the Go chain's CurrentVersion — this is the second place that number is
	// written, and TestGeneratedDefaultVersionMatchesTheChain is what stops the
	// two drifting. Bump both together.
	schema_version: *1 | (int & >=1)

	// Positions for the top-level panels.
	gridLayout!: #Layout

	// Top-level panels, outside any sub-grid.
	panels!: #Panels

	variables: *[] | #DashboardVariables

	// Collapsible groups, each with its own panels and layout.
	subGrids: *[] | #SubGrids

	// Positions for the sub-grids within the dashboard.
	subGridLayout: *[] | #Layout

	// Set when the dashboard has been shared publicly; the path the public view
	// is served at.
	publicDashboardPath?: string

	// Tabs, each with a page of its own. Empty or absent means the dashboard is
	// untabbed and its page is the top level above. When non-empty, the
	// top-level panels, gridLayout, subGrids and subGridLayout must all be
	// empty; they stay present so an older reader still parses the document.
	// Variables, event overlays and publicDashboardPath stay at the top level
	// and apply to every tab.
	tabs: *[] | #Tabs

	// Event markers drawn on every timeseries and bar panel that inherits them.
	// Absent means none.
	eventOverlays?: panel.#EventOverlays

	...
}

// dashboardFileUploadSchema (:157) is deliberately NOT modelled here. It is the
// import/export envelope — a name, a description and the preset — which makes it
// a REQUEST BODY, not an entity. This module describes the entities that get
// persisted; how a handler wraps one in transit is that endpoint's business and
// changes on a different clock.
//
// A caller validating an upload should validate its `preset` member against
// #Preset and check the envelope itself however it checks its other request
// bodies.
