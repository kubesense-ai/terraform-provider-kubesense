package catalog

// The family modifiers per category: parameters every entry of the category
// takes, appended after its own. An entry opts out of one with `except`, and
// may never add one ad hoc. Each list is what the MetricsQL parser accepts for
// the category; TestCatalogExamplesAreValidMetricsQL parses every entry's
// example to hold that true.
//
// Aggregates take `by` or `without` and `limit`, never keep_metric_names (the
// parser rejects it after an aggregate). `by` keeps its legacy default of an
// empty list, which renders no grouping at all.
families: {
	rollup: modifiers: [
		{name: "step", at: "step", optional: true},
		{name: "keep_metric_names", at: "suffix", optional: true},
	]
	transform: modifiers: [
		{name: "keep_metric_names", at: "suffix", optional: true},
	]
	label: modifiers: []
	aggregate: modifiers: [
		{name: "by", at: "grouping", default: []},
		{name: "without", at: "grouping", optional: true},
		{name: "limit", at: "limit", optional: true},
	]
	operator: modifiers: [
		{name: "bool", at: "bool", optional: true},
		{name: "keep_metric_names", at: "suffix", optional: true},
	]
}
