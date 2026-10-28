package dashboard

import (
	"strings"

	"github.com/kubesense-ai/terraform-provider-kubesense/internal/kschema/presetwalk"
)

// legacy -> v1: everything the browser had been doing by sniffing shape.
//
// Four transforms, all ported from webapp's migrateToNewSchema
// (explorer/v2/utils/zod-schemas.ts:832) and coerceYaxisColumns (:578), which
// run on every preset the app decodes via presetStringToObjectCodec:
//
//	yaxis          a logs/traces query's y-axis became a list of columns
//	               (webapp 2c8d7ab1a, 2026-08-07)
//	columnFields   defaulted to [] on logs/traces when it is not an array
//	spl            selectedMode "spl" became "logs" + filterMode "SPL", and
//	               the panel it lives on became panelType "spl"
//	config         graphStyles/tableOptions/legendOptions at the panel root
//	               became a single `config` object, and inside it
//	               colorPalette -> colorScheme, enableThresholds -> thresholdDisplayMode
//
// All four exist because panels carried no version, so the browser had to guess
// from shape on every read. The CUE says as much at panel/query.cue:366: "with
// one, the migration would key off the version instead of guessing from shape."
// This is that migration, and it is why the browser's copy can eventually go.
//
// PORTED FAITHFULLY, quirks included — a document that changes shape depending
// on which side saw it last is the thing this replaces, so where the browser
// leaves a key behind (colorPalette at the panel root, which #Panel tolerates
// because it is open) this leaves it too.
//
// One deliberate difference: the browser migrates TOP-LEVEL panels only, because
// presetStringToObjectCodec maps over `panels` and sub-grid panels are parsed
// with the base schema. That is a gap rather than a decision — a sub-grid panel
// is a panel, validated identically — so this migrates those too.

// MigrateLegacyToV1 brings an unversioned dashboard preset up to v1.
//
// Never errors: a legacy document is by definition one nothing has validated, so
// it may hold anything at all. Refusing to migrate the readable parts because
// one panel is malformed would leave the caller with a document that is neither
// legacy nor v1. Whatever this cannot interpret it leaves alone, for the
// validator that runs next to report properly.
func MigrateLegacyToV1(doc map[string]any) error {
	forEachPanel(doc, func(panel map[string]any) {
		// Same order as the browser: the config restructure reads root keys that
		// the query passes do not touch, and the SPL pass rewrites selectedMode,
		// which the logs/traces pass then matches on. Reordering silently changes
		// what an "spl" query ends up with.
		migratePanelConfig(panel)
		migrateLegacySplQueries(panel)
		migrateLogsTracesQueries(panel)
	})
	return nil
}

// v1PanelConfig is DEFAULT_PANEL_CONFIG as it stood at v1, written out rather
// than read from the generated defaults artifact.
//
// A shipped step is frozen: it must produce the same output for the same input
// forever. Sourcing these from schema.Defaults would tie a frozen transform to a
// value that is free to change, so a document migrated today and one migrated
// after the next default tweak would disagree while both claiming v1. It would
// also be an import cycle — schema imports this package for the Migrate
// dispatcher.
//
// Returned by a function rather than held in a package var so each panel gets
// its own maps and slices; sharing them would alias config between panels.
func v1PanelConfig() map[string]any {
	return map[string]any{
		"lineWidth":                 1.5,
		"fillOpacity":               0,
		"stackSeries":               false,
		"showTrend":                 false,
		"showLabel":                 false,
		"showPercentageChange":      false,
		"percentageChangeColorMode": "standard",
		"mergeTables":               true,
		"legendVisibility":          true,
		"legendPlacement":           "bottom",
		"yAxisLabelFormatter":       "auto",
		"step":                      "auto",
		"thresholdDisplayMode":      "off",
		"valueOptions":              map[string]any{"show": "calculate", "calculation": "last"},
		"colorScheme":               map[string]any{"type": "palette", "palette": "default"},
		"thresholds": []any{map[string]any{
			"threshold":    "-Infinity",
			"color":        "#56a64b",
			"isDefault":    true,
			"isLabelShown": false,
			"label":        "",
		}},
	}
}

// migratePanelConfig folds a panel's loose styling keys into `config`.
//
// Two eras, and they are distinguished the way the browser distinguishes them:
// a panel that already has a `config` object only needs the two renames inside
// it; one that does not, but carries graphStyles/tableOptions/legendOptions at
// its root, predates `config` entirely and has one built.
func migratePanelConfig(panel map[string]any) {
	if config, ok := panel["config"].(map[string]any); ok {
		// colorPalette was a bare string; colorScheme is a tagged object, so
		// other scheme types (thresholds, single colour) became expressible.
		if palette, ok := config["colorPalette"].(string); ok {
			delete(config, "colorPalette")
			config["colorScheme"] = map[string]any{"type": "palette", "palette": palette}
		}
		// A boolean became a mode, so "on" could say HOW thresholds are drawn.
		if enabled, ok := config["enableThresholds"].(bool); ok {
			delete(config, "enableThresholds")
			if enabled {
				config["thresholdDisplayMode"] = "lines_dashed"
			} else {
				config["thresholdDisplayMode"] = "off"
			}
		}
		return
	}

	// yAxisLabelFormatter alone is only evidence of the old format when there is
	// no config to have held it.
	_, hasGraphStyles := panel["graphStyles"]
	_, hasTableOptions := panel["tableOptions"]
	_, hasLegendOptions := panel["legendOptions"]
	_, hasFormatter := panel["yAxisLabelFormatter"]
	_, hasConfig := panel["config"]
	if !hasGraphStyles && !hasTableOptions && !hasLegendOptions && !(hasFormatter && !hasConfig) {
		return
	}

	graphStyles, _ := panel["graphStyles"].(map[string]any)
	legendOptions, _ := panel["legendOptions"].(map[string]any)

	config := v1PanelConfig()
	for key, from := range map[string]string{
		"lineWidth":                 "lineWidth",
		"fillOpacity":               "fillOpacity",
		"stackSeries":               "stackSeries",
		"showTrend":                 "showTrend",
		"showLabel":                 "showLabel",
		"showPercentageChange":      "showPercentageChange",
		"percentageChangeColorMode": "percentageChangeColorMode",
	} {
		if value, ok := graphStyles[from]; ok {
			config[key] = value
		}
	}
	// legendOptions kept its keys under shorter names.
	if value, ok := legendOptions["visibility"]; ok {
		config["legendVisibility"] = value
	}
	if value, ok := legendOptions["placement"]; ok {
		config["legendPlacement"] = value
	}
	for _, key := range []string{"yAxisLabelFormatter", "step", "thresholds", "valueOptions"} {
		if value, ok := panel[key]; ok {
			config[key] = value
		}
	}
	if enabled, ok := panel["enableThresholds"].(bool); ok {
		if enabled {
			config["thresholdDisplayMode"] = "lines_dashed"
		} else {
			config["thresholdDisplayMode"] = "off"
		}
	}
	if palette, ok := panel["colorPalette"].(string); ok {
		config["colorScheme"] = map[string]any{"type": "palette", "palette": palette}
	}

	// The keys the browser removes — and only those. It leaves colorPalette at
	// the root, which #Panel tolerates because it is open; dropping it here
	// would be this side quietly doing more than the other.
	for _, key := range []string{
		"graphStyles", "tableOptions", "legendOptions",
		"yAxisLabelFormatter", "step", "thresholds", "enableThresholds", "valueOptions",
	} {
		delete(panel, key)
	}
	panel["config"] = config
}

// migrateLegacySplQueries turns the retired `spl` query mode back into a logs
// query that happens to be written in SPL.
//
// SPL stopped being its own datasource and became a FILTER MODE on logs, so the
// mode moved from the query's selectedMode to its filterMode. The panel is
// promoted too: panelType "spl" is what makes the editor open the SPL view for a
// dashboard whose queries are now, structurally, logs queries.
func migrateLegacySplQueries(panel map[string]any) {
	queries, ok := panel["queries"].([]any)
	if !ok {
		return
	}

	found := false
	for _, item := range queries {
		query, ok := item.(map[string]any)
		if !ok || query["selectedMode"] != "spl" {
			continue
		}
		found = true

		query["selectedMode"] = "logs"
		if _, ok := query["filterMode"]; !ok {
			query["filterMode"] = "SPL"
		}
		if _, ok := query["aggregation"]; !ok {
			query["aggregation"] = map[string]any{"function": "row_count"}
		}
		if _, ok := query["chart_type"]; !ok {
			query["chart_type"] = "table"
		}
		// `query` holds the SPL text. Anything that is not a string is replaced
		// rather than preserved: the editor renders this into a text area, and a
		// non-string there is a crash rather than a bad query.
		if text, ok := query["query"].(string); ok {
			query["query"] = text
		} else {
			query["query"] = ""
		}
	}

	if found {
		panel["panelType"] = "spl"
	}
}

// migrateLogsTracesQueries fixes the two fields that moved on the shared
// logs/traces query base.
func migrateLogsTracesQueries(panel map[string]any) {
	queries, ok := panel["queries"].([]any)
	if !ok {
		return
	}

	for _, item := range queries {
		query, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch query["selectedMode"] {
		case "logs", "traces":
		default:
			continue
		}

		// columnFields became a required list. A null or absent one renders an
		// empty table rather than throwing, but the contract wants the array.
		if _, isList := query["columnFields"].([]any); !isList {
			query["columnFields"] = []any{}
		}

		// Absent stays absent. The field defaults to ["auto"] in the contract, and
		// writing that in here would turn "unset" into an explicit choice.
		if raw, present := query["yaxis"]; present {
			query["yaxis"] = coerceYaxisColumns(raw)
		}
	}
}

// coerceYaxisColumns mirrors the browser's function of the same name.
//
// A blank result falls back to ["auto"] rather than to an empty list, because
// #YaxisColumns is `[string, ...string]` — non-empty — so [] would be a document
// this migration had itself made invalid.
func coerceYaxisColumns(value any) []any {
	if list, ok := value.([]any); ok {
		columns := make([]any, 0, len(list))
		for _, item := range list {
			if column, ok := item.(string); ok && strings.TrimSpace(column) != "" {
				columns = append(columns, column)
			}
		}
		if len(columns) == 0 {
			return []any{"auto"}
		}
		return columns
	}
	if column, ok := value.(string); ok && strings.TrimSpace(column) != "" {
		return []any{column}
	}
	return []any{"auto"}
}

// forEachPanel visits every panel in a preset, wherever it is nested: on the
// top level, in a tab, or in a sub-grid of either. See presetwalk for the walk.
//
// Tabs postdate this step, and no legacy document has them, so reaching into
// tabs changes nothing for any document this step already migrated. It has to
// reach them anyway: the webapp's decoder drops schema_version, so every save
// from the UI arrives unversioned and runs this step, tabs included.
func forEachPanel(doc map[string]any, visit func(panel map[string]any)) {
	for _, p := range presetwalk.Panels(doc) {
		visit(p.Object)
	}
}
