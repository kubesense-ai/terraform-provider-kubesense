package catalog

// Legacy `aggregations` arm. Its `by` is the family modifier, which leads the
// params so the entry still serialises to exactly the legacy [by] tuple.
_aggregation: {
	type: "aggregations", category: "aggregate", legacy: "aggregations"
	params: *[] | [...#Param]
}

_aggregate: {
	type: "aggregations", category: "aggregate"
	params: *[] | [...#Param]
}

// Legacy `top_bottom` arm: `by` names the function rather than being an
// argument, so it sits at the callee position and is spelled unquoted. It
// replaces the family's grouping, which the rendered topk_<by> call never had.
// Still rendered; no longer offered, since topk_max and friends are entries.
_topBottom: {
	type: "top_bottom", category: "aggregate", legacy: "top_bottom", menu: false
	except: ["by", "without", "limit"]
	params: [
		{name: "k", at: "before", default: 5},
		{name: "by", at: "callee", default: "max", kind: {
			kind: "enum", values: ["max", "min", "avg", "median", "last"], quoted: false
		}},
	]
}

catalog: {
	sum: _aggregation & {
		signature:   "sum(q) by (labels)"
		description: "Sum across series in each group."
		example: {
			args: {by: ["job"]}
			promql: "sum by (job) (m)"
		}
	}
	avg: _aggregation & {
		signature:   "avg(q) by (labels)"
		description: "Average across series in each group."
		example: {
			args: {by: [], without: ["instance"]}
			promql: "avg without (instance) (m)"
		}
	}
	max: _aggregation & {
		signature:   "max(q) by (labels)"
		description: "Maximum across series in each group."
		example: {
			args: {by: ["job"], limit: 3}
			promql: "max by (job) (m) limit 3"
		}
	}
	min: _aggregation & {
		signature:   "min(q) by (labels)"
		description: "Minimum across series in each group."
		example: {
			args: {by: []}
			promql: "min(m)"
		}
	}
	count: _aggregation & {
		signature:   "count(q) by (labels)"
		description: "Number of series in each group."
		example: {
			args: {by: ["namespace"]}
			promql: "count by (namespace) (m)"
		}
	}
	stddev: _aggregation & {
		signature:   "stddev(q) by (labels)"
		description: "Standard deviation across series in each group."
		example: {
			args: {by: ["job"]}
			promql: "stddev by (job) (m)"
		}
	}
	stdvar: _aggregation & {
		signature:   "stdvar(q) by (labels)"
		description: "Standard variance across series in each group."
		example: {
			args: {by: ["job"]}
			promql: "stdvar by (job) (m)"
		}
	}
	group: _aggregation & {
		signature:   "group(q) by (labels)"
		description: "1 for each group of series."
		example: {
			args: {by: ["pod"]}
			promql: "group by (pod) (m)"
		}
	}
	any: _aggregate & {
		signature:   "any(q) by (labels)"
		description: "One series per group."
		example: {
			args: {}
			promql: "any(m)"
		}
	}
	distinct: _aggregate & {
		signature:   "distinct(q) by (labels)"
		description: "Number of distinct values across series in each group."
		example: {
			args: {}
			promql: "distinct(m)"
		}
	}
	geomean: _aggregate & {
		signature:   "geomean(q) by (labels)"
		description: "Geometric mean across series in each group."
		example: {
			args: {}
			promql: "geomean(m)"
		}
	}
	histogram: _aggregate & {
		signature:   "histogram(q) by (labels)"
		description: "VictoriaMetrics histogram (vmrange buckets) of the values across series in each group."
		example: {
			args: {}
			promql: "histogram(m)"
		}
	}
	mad: _aggregate & {
		signature:   "mad(q) by (labels)"
		description: "Median absolute deviation across series in each group."
		example: {
			args: {}
			promql: "mad(m)"
		}
	}
	median: _aggregate & {
		signature:   "median(q) by (labels)"
		description: "Median across series in each group."
		example: {
			args: {}
			promql: "median(m)"
		}
	}
	mode: _aggregate & {
		signature:   "mode(q) by (labels)"
		description: "Most frequent value across series in each group."
		example: {
			args: {}
			promql: "mode(m)"
		}
	}
	outliers_iqr: _aggregate & {
		signature:   "outliers_iqr(q) by (labels)"
		description: "Series with a point outside the group's interquartile range bounds."
		example: {
			args: {}
			promql: "outliers_iqr(m)"
		}
	}
	share: _aggregate & {
		signature:   "share(q) by (labels)"
		description: "Each series' share, in [0, 1], of its group's sum."
		example: {
			args: {by: ["le"]}
			promql: "share by (le) (m)"
		}
	}
	sum2: _aggregate & {
		signature:   "sum2(q) by (labels)"
		description: "Sum of squares across series in each group."
		example: {
			args: {by: []}
			selectorModifiers: {at: "end()"}
			promql: "sum2(m @ end())"
		}
	}
	zscore: _aggregate & {
		signature:   "zscore(q) by (labels)"
		description: "z-score of each series within its group."
		example: {
			args: {}
			promql: "zscore(m)"
		}
	}
	topk: _aggregate & {
		params: [{name: "k", at: "before", default: 5}]
		signature:   "topk(k, q)"
		description: "The k biggest points across series in each group, per timestamp."
		example: {
			args: {k: 3}
			promql: "topk(3, m)"
		}
	}
	bottomk: _aggregate & {
		params: [{name: "k", at: "before", default: 5}]
		signature:   "bottomk(k, q)"
		description: "The k smallest points across series in each group, per timestamp."
		example: {
			args: {k: 3}
			promql: "bottomk(3, m)"
		}
	}
	limitk: _aggregate & {
		params: [{name: "k", at: "before", default: 5}]
		signature:   "limitk(k, q) by (labels)"
		description: "Up to k series per group, the same ones on every call."
		example: {
			args: {k: 2, by: ["job"]}
			promql: "limitk by (job) (2, m)"
		}
	}
	outliersk: _aggregate & {
		params: [{name: "k", at: "before", default: 5}]
		signature:   "outliersk(k, q)"
		description: "The k series with the biggest standard deviation in each group."
		example: {
			args: {k: 2}
			promql: "outliersk(2, m)"
		}
	}
	outliers_mad: _aggregate & {
		params: [{name: "tolerance", at: "before", default: 3}]
		signature:   "outliers_mad(tolerance, q)"
		description: "Series with a point farther than tolerance times the group's median absolute deviation from its median."
		example: {
			args: {tolerance: 2.5}
			promql: "outliers_mad(2.5, m)"
		}
	}
	count_values: _aggregate & {
		params: [{name: "label", at: "before", default: "value"}]
		signature:   "count_values(\"label\", q)"
		description: "Number of points per distinct value in each group, the value stored in label."
		example: {
			args: {label: "version", by: []}
			promql: "count_values(\"version\", m)"
		}
	}
	quantile: _aggregate & {
		params: [{name: "phi", at: "before", default: 0.95}]
		signature:   "quantile(phi, q) by (labels)"
		description: "phi-quantile across series in each group, phi in [0, 1]."
		example: {
			args: {phi: 0.9, by: ["job"]}
			promql: "quantile by (job) (0.9, m)"
		}
	}
	quantiles: _aggregate & {
		params: [{name: "phi_label", at: "before", default: "quantile"}, {name: "phis", at: "before", default: [0.5, 0.9, 0.99]}]
		signature:   "quantiles(\"phiLabel\", phi1, ..., phiN, q)"
		description: "Several phi-quantiles across series in each group, one series per phi told apart by the phiLabel label."
		example: {
			args: {phi_label: "phi", phis: [0.5, 0.9]}
			promql: "quantiles(\"phi\", 0.5, 0.9, m)"
		}
	}
	topk_avg: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "topk_avg(k, q, \"other_label=other_value\")"
		description: "The k series with the biggest averages; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "topk_avg(5, m)"
		}
	}
	bottomk_avg: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "bottomk_avg(k, q, \"other_label=other_value\")"
		description: "The k series with the smallest averages; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "bottomk_avg(5, m)"
		}
	}
	topk_last: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "topk_last(k, q, \"other_label=other_value\")"
		description: "The k series with the biggest last values; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "topk_last(5, m)"
		}
	}
	bottomk_last: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "bottomk_last(k, q, \"other_label=other_value\")"
		description: "The k series with the smallest last values; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "bottomk_last(5, m)"
		}
	}
	topk_max: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "topk_max(k, q, \"other_label=other_value\")"
		description: "The k series with the biggest maximums; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5, other: "pod=other"}
			promql: "topk_max(5, m, \"pod=other\")"
		}
	}
	bottomk_max: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "bottomk_max(k, q, \"other_label=other_value\")"
		description: "The k series with the smallest maximums; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "bottomk_max(5, m)"
		}
	}
	topk_median: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "topk_median(k, q, \"other_label=other_value\")"
		description: "The k series with the biggest medians; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "topk_median(5, m)"
		}
	}
	bottomk_median: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "bottomk_median(k, q, \"other_label=other_value\")"
		description: "The k series with the smallest medians; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "bottomk_median(5, m)"
		}
	}
	topk_min: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "topk_min(k, q, \"other_label=other_value\")"
		description: "The k series with the biggest minimums; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5}
			promql: "topk_min(5, m)"
		}
	}
	bottomk_min: _aggregate & {
		params: [{name: "k", at: "before", default: 5}, {name: "other", at: "after", optional: true}]
		signature:   "bottomk_min(k, q, \"other_label=other_value\")"
		description: "The k series with the smallest minimums; with other, the rest summed into one series carrying that label=value."
		example: {
			args: {k: 5, by: ["namespace"]}
			promql: "bottomk_min by (namespace) (5, m)"
		}
	}

	No_Aggregations: _aggregation & {
		form: "identity"
		menu: false
		except: ["without", "limit"]
		signature:   "q"
		description: "Legacy placeholder that renders nothing."
		example: {
			args: {by: []}
			promql: "m"
		}
	}

	top: _topBottom & {
		callee:      "topk_{by}"
		signature:   "topk_<by>(k, q)"
		description: "Legacy spelling of topk_max, topk_min, topk_avg, topk_median and topk_last: the k series with the biggest value of that statistic."
		example: {
			args: {k: 5, by: "max"}
			promql: "topk_max(5, m)"
		}
	}
	bottom: _topBottom & {
		callee:      "bottomk_{by}"
		signature:   "bottomk_<by>(k, q)"
		description: "Legacy spelling of bottomk_max, bottomk_min, bottomk_avg, bottomk_median and bottomk_last: the k series with the smallest value of that statistic."
		example: {
			args: {k: 3, by: "avg"}
			promql: "bottomk_avg(3, m)"
		}
	}
}
