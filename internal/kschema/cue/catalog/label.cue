package catalog

// Label manipulation. Every string argument is quoted, and a list argument is
// spread into the call: label_set(q, "k1", "v1", "k2", "v2").
_label: {
	type: "label", category: "label"
	params: *[] | [...#Param]
}

catalog: {
	alias: _label & {
		params: [{name: "name", at: "after", default: ""}]
		signature:   "alias(q, \"name\")"
		description: "Every series renamed to name."
		example: {
			args: {name: "requests"}
			promql: "alias(m, \"requests\")"
		}
	}
	drop_common_labels: _label & {
		signature:   "drop_common_labels(q)"
		description: "Every label=value pair the series share dropped."
		example: {
			args: {}
			promql: "drop_common_labels(m)"
		}
	}
	label_copy: _label & {
		params: [{name: "pairs", at: "after", default: []}]
		signature:   "label_copy(q, \"src_label1\", \"dst_label1\", ...)"
		description: "Each src_label's value copied into its dst_label."
		example: {
			args: {pairs: [["pod", "instance"]]}
			promql: "label_copy(m, \"pod\", \"instance\")"
		}
	}
	label_del: _label & {
		params: [{name: "labels", at: "after", default: []}]
		signature:   "label_del(q, \"label1\", ...)"
		description: "The listed labels deleted."
		example: {
			args: {labels: ["pod", "instance"]}
			promql: "label_del(m, \"pod\", \"instance\")"
		}
	}
	label_graphite_group: _label & {
		params: [{name: "group_nums", at: "after", default: [0]}]
		signature:   "label_graphite_group(q, groupNum1, ...)"
		description: "Metric name replaced with the listed Graphite name groups joined by dots."
		example: {
			args: {group_nums: [0, 2]}
			promql: "label_graphite_group(m, 0, 2)"
		}
	}
	label_join: _label & {
		params: [{name: "dst_label", at: "after", default: ""}, {name: "separator", at: "after", default: "-"}, {name: "src_labels", at: "after", default: []}]
		signature:   "label_join(q, \"dst_label\", \"separator\", \"src_label1\", ...)"
		description: "The src_labels' values joined with separator into dst_label."
		example: {
			args: {dst_label: "target", separator: ":", src_labels: ["namespace", "pod"]}
			promql: "label_join(m, \"target\", \":\", \"namespace\", \"pod\")"
		}
	}
	label_keep: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "label_keep(q, \"label1\", ...)"
		description: "Every label but the listed ones deleted."
		example: {
			args: {labels: ["job"]}
			promql: "label_keep(m, \"job\")"
		}
	}
	label_lowercase: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "label_lowercase(q, \"label1\", ...)"
		description: "The listed labels' values lowercased."
		example: {
			args: {labels: ["method"]}
			promql: "label_lowercase(m, \"method\")"
		}
	}
	label_map: _label & {
		params: [{name: "label", at: "after", default: ""}, {name: "value_pairs", at: "after", default: []}]
		signature:   "label_map(q, \"label\", \"src_value1\", \"dst_value1\", ...)"
		description: "label's values mapped from each src_value to its dst_value."
		example: {
			args: {label: "code", value_pairs: [["200", "ok"], ["500", "error"]]}
			promql: "label_map(m, \"code\", \"200\", \"ok\", \"500\", \"error\")"
		}
	}
	label_match: _label & {
		params: [{name: "label", at: "after", default: ""}, {name: "regex", at: "after", default: ".*"}]
		signature:   "label_match(q, \"label\", \"regexp\")"
		description: "Only the series whose label matches regexp."
		example: {
			args: {label: "pod", regex: "api-.*"}
			promql: "label_match(m, \"pod\", \"api-.*\")"
		}
	}
	label_mismatch: _label & {
		params: [{name: "label", at: "after", default: ""}, {name: "regex", at: "after", default: ".*"}]
		signature:   "label_mismatch(q, \"label\", \"regexp\")"
		description: "Only the series whose label does not match regexp."
		example: {
			args: {label: "pod", regex: "api-.*"}
			promql: "label_mismatch(m, \"pod\", \"api-.*\")"
		}
	}
	label_move: _label & {
		params: [{name: "pairs", at: "after", default: []}]
		signature:   "label_move(q, \"src_label1\", \"dst_label1\", ...)"
		description: "Each src_label's value moved into its dst_label."
		example: {
			args: {pairs: [["pod", "instance"]]}
			promql: "label_move(m, \"pod\", \"instance\")"
		}
	}
	label_replace: _label & {
		params: [{name: "dst_label", at: "after", default: ""}, {name: "replacement", at: "after", default: "$1"}, {name: "src_label", at: "after", default: ""}, {name: "regex", at: "after", default: "(.*)"}]
		signature:   "label_replace(q, \"dst_label\", \"replacement\", \"src_label\", \"regex\")"
		description: "replacement, with regex captures substituted, stored in dst_label wherever regex matches src_label."
		example: {
			args: {dst_label: "app", replacement: "$1", src_label: "pod", regex: "(.*)-[a-z0-9]+"}
			promql: "label_replace(m, \"app\", \"$1\", \"pod\", \"(.*)-[a-z0-9]+\")"
		}
	}
	label_set: _label & {
		params: [{name: "label_values", at: "after", default: []}]
		signature:   "label_set(q, \"label1\", \"value1\", ...)"
		description: "The listed labels set to their values."
		example: {
			args: {label_values: [["env", "prod"], ["team", "core"]]}
			promql: "label_set(m, \"env\", \"prod\", \"team\", \"core\")"
		}
	}
	label_transform: _label & {
		params: [{name: "label", at: "after", default: ""}, {name: "regex", at: "after", default: "(.*)"}, {name: "replacement", at: "after", default: "$1"}]
		signature:   "label_transform(q, \"label\", \"regexp\", \"replacement\")"
		description: "Every regexp match in label's value replaced with replacement."
		example: {
			args: {label: "pod", regex: "-", replacement: "_"}
			promql: "label_transform(m, \"pod\", \"-\", \"_\")"
		}
	}
	label_uppercase: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "label_uppercase(q, \"label1\", ...)"
		description: "The listed labels' values uppercased."
		example: {
			args: {labels: ["method"]}
			promql: "label_uppercase(m, \"method\")"
		}
	}
	label_value: _label & {
		params: [{name: "label", at: "after", default: ""}]
		signature:   "label_value(q, \"label\")"
		description: "label's value parsed as the sample value."
		example: {
			args: {label: "le"}
			promql: "label_value(m, \"le\")"
		}
	}
	labels_equal: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 2}]
		signature:   "labels_equal(q, \"label1\", \"label2\", ...)"
		description: "Only the series whose listed labels all hold the same value."
		example: {
			args: {labels: ["src", "dst"]}
			promql: "labels_equal(m, \"src\", \"dst\")"
		}
	}
	sort_by_label: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "sort_by_label(q, \"label1\", ...)"
		description: "Series sorted by the listed labels' values, ascending."
		example: {
			args: {labels: ["pod"]}
			promql: "sort_by_label(m, \"pod\")"
		}
	}
	sort_by_label_desc: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "sort_by_label_desc(q, \"label1\", ...)"
		description: "Series sorted by the listed labels' values, descending."
		example: {
			args: {labels: ["pod"]}
			promql: "sort_by_label_desc(m, \"pod\")"
		}
	}
	sort_by_label_numeric: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "sort_by_label_numeric(q, \"label1\", ...)"
		description: "Series sorted by the listed labels' values, ascending, comparing numbers numerically."
		example: {
			args: {labels: ["code"]}
			promql: "sort_by_label_numeric(m, \"code\")"
		}
	}
	sort_by_label_numeric_desc: _label & {
		params: [{name: "labels", at: "after", default: [], minItems: 1}]
		signature:   "sort_by_label_numeric_desc(q, \"label1\", ...)"
		description: "Series sorted by the listed labels' values, descending, comparing numbers numerically."
		example: {
			args: {labels: ["code"]}
			promql: "sort_by_label_numeric_desc(m, \"code\")"
		}
	}
}
