package catalog

// A rollup's window is optional: absent, MetricsQL picks one (max(step,
// scrape_interval)). A new instance still gets one, so what the builder shows
// is what runs. The legacy `rollup` arm has always defaulted to a fixed 30s;
// everything else defaults to $__rate_interval, which the API resolves against
// the step the query runs with.
_window: {name: "over", at: "window", optional: true, default: "$__rate_interval"}
_legacyWindow: {name: "over", at: "window", optional: true, default: "30s"}

// Legacy `range` arm. Stored type stays "range"; listed under Rollup.
_range: {
	type: "range", category: "rollup", legacy: "range"
	params: *[_window] | [...#Param]
}

// Legacy `rollup` arm.
_legacyRollup: {
	type: "rollup", category: "rollup", legacy: "rollup"
	params: *[_legacyWindow] | [...#Param]
}

_rollup: {
	type: "rollup", category: "rollup"
	params: *[_window] | [...#Param]
}

catalog: {
	rate: _range & {
		signature:   "rate(q[d])"
		description: "Average per-second increase rate of a counter over the window."
		example: {
			args: {over: "5m"}
			promql: "rate(m[5m])"
		}
	}
	irate: _range & {
		signature:   "irate(q[d])"
		description: "Per-second increase rate from the last two raw samples in the window."
		example: {
			args: {over: "1m"}
			promql: "irate(m[1m])"
		}
	}
	increase: _range & {
		signature:   "increase(q[d])"
		description: "Increase of a counter over the window."
		example: {
			args: {over: "1h"}
			promql: "increase(m[1h])"
		}
	}
	resets: _range & {
		signature:   "resets(q[d])"
		description: "Number of counter resets over the window."
		example: {
			args: {over: "1h"}
			promql: "resets(m[1h])"
		}
	}
	delta: _range & {
		signature:   "delta(q[d])"
		description: "Difference between the last sample and the last sample before the window."
		example: {
			args: {over: "5m"}
			promql: "delta(m[5m])"
		}
	}
	idelta: _range & {
		signature:   "idelta(q[d])"
		description: "Difference between the last two raw samples in the window."
		example: {
			args: {over: "5m"}
			promql: "idelta(m[5m])"
		}
	}
	deriv: _range & {
		signature:   "deriv(q[d])"
		description: "Per-second derivative over the window, by linear regression."
		example: {
			args: {over: "10m"}
			promql: "deriv(m[10m])"
		}
	}
	changes: _range & {
		signature:   "changes(q[d])"
		description: "Number of times the value changed over the window."
		example: {
			args: {over: "1h", keep_metric_names: true}
			promql: "changes(m[1h]) keep_metric_names"
		}
	}
	avg_over_time: _legacyRollup & {
		signature:   "avg_over_time(q[d])"
		description: "Average of raw samples over the window."
		example: {
			args: {over: "30s"}
			promql: "avg_over_time(m[30s])"
		}
	}
	sum_over_time: _legacyRollup & {
		signature:   "sum_over_time(q[d])"
		description: "Sum of raw samples over the window."
		example: {
			args: {over: "1h"}
			promql: "sum_over_time(m[1h])"
		}
	}
	max_over_time: _legacyRollup & {
		signature:   "max_over_time(q[d])"
		description: "Maximum raw sample over the window."
		example: {
			chain: [{name: "rate", args: {over: "5m"}}]
			args: {over: "1h", step: "1m"}
			promql: "max_over_time(rate(m[5m])[1h:1m])"
		}
	}
	min_over_time: _legacyRollup & {
		signature:   "min_over_time(q[d])"
		description: "Minimum raw sample over the window."
		example: {
			args: {over: "5m"}
			promql: "min_over_time(m[5m])"
		}
	}
	count_over_time: _legacyRollup & {
		signature:   "count_over_time(q[d])"
		description: "Number of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "count_over_time(m[5m])"
		}
	}
	last_over_time: _legacyRollup & {
		signature:   "last_over_time(q[d])"
		description: "Last raw sample in the window."
		example: {
			args: {over: "5m"}
			promql: "last_over_time(m[5m])"
		}
	}
	absent_over_time: _legacyRollup & {
		signature:   "absent_over_time(q[d])"
		description: "1 when the window holds no raw samples, empty otherwise."
		example: {
			args: {over: "10m"}
			promql: "absent_over_time(m[10m])"
		}
	}
	present_over_time: _legacyRollup & {
		signature:   "present_over_time(q[d])"
		description: "1 when the window holds at least one raw sample, empty otherwise."
		example: {
			args: {over: "10m"}
			promql: "present_over_time(m[10m])"
		}
	}
	increases_over_time: _legacyRollup & {
		signature:   "increases_over_time(q[d])"
		description: "Number of increases between consecutive raw samples over the window."
		example: {
			args: {over: "1h"}
			promql: "increases_over_time(m[1h])"
		}
	}
	range_over_time: _legacyRollup & {
		signature:   "range_over_time(q[d])"
		description: "Difference between the maximum and minimum raw samples over the window."
		example: {
			args: {over: "1h"}
			promql: "range_over_time(m[1h])"
		}
	}
	stddev_over_time: _legacyRollup & {
		signature:   "stddev_over_time(q[d])"
		description: "Standard deviation of raw samples over the window."
		example: {
			args: {over: "1h"}
			promql: "stddev_over_time(m[1h])"
		}
	}
	stdvar_over_time: _legacyRollup & {
		signature:   "stdvar_over_time(q[d])"
		description: "Standard variance of raw samples over the window."
		example: {
			args: {over: "1h"}
			promql: "stdvar_over_time(m[1h])"
		}
	}
	// phi BEFORE the windowed input. Array order is storage order, and render
	// order only applies within a position, so `over` leads to match the
	// legacy tuple [over, quantile] while phi still renders first.
	quantile_over_time: _legacyRollup & {
		params: [_legacyWindow, {name: "quantile", at: "before", default: 0.95}]
		signature:   "quantile_over_time(phi, q[d])"
		description: "phi-quantile of raw samples over the window, phi in [0, 1]."
		example: {
			args: {over: "5m", quantile: 0.99}
			promql: "quantile_over_time(0.99, m[5m])"
		}
	}
	ascent_over_time: _rollup & {
		signature:   "ascent_over_time(q[d])"
		description: "Sum of the increases between consecutive raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "ascent_over_time(m[5m])"
		}
	}
	changes_prometheus: _rollup & {
		signature:   "changes_prometheus(q[d])"
		description: "Number of value changes over the window, ignoring the sample before it, as Prometheus does."
		example: {
			args: {over: "5m"}
			promql: "changes_prometheus(m[5m])"
		}
	}
	decreases_over_time: _rollup & {
		signature:   "decreases_over_time(q[d])"
		description: "Number of decreases between consecutive raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "decreases_over_time(m[5m])"
		}
	}
	default_rollup: _rollup & {
		signature:   "default_rollup(q[d])"
		description: "Last raw sample in the window, honouring staleness markers."
		example: {
			args: {}
			promql: "default_rollup(m)"
		}
	}
	delta_prometheus: _rollup & {
		signature:   "delta_prometheus(q[d])"
		description: "Difference between the first and the last raw samples in the window, as Prometheus computes delta."
		example: {
			args: {over: "5m"}
			selectorModifiers: {offset: "1h"}
			promql: "delta_prometheus(m[5m] offset 1h)"
		}
	}
	deriv_fast: _rollup & {
		signature:   "deriv_fast(q[d])"
		description: "Per-second derivative from the first and the last raw samples in the window."
		example: {
			args: {over: "5m"}
			promql: "deriv_fast(m[5m])"
		}
	}
	descent_over_time: _rollup & {
		signature:   "descent_over_time(q[d])"
		description: "Sum of the decreases between consecutive raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "descent_over_time(m[5m])"
		}
	}
	distinct_over_time: _rollup & {
		signature:   "distinct_over_time(q[d])"
		description: "Number of distinct raw sample values over the window."
		example: {
			args: {over: "5m"}
			promql: "distinct_over_time(m[5m])"
		}
	}
	first_over_time: _rollup & {
		signature:   "first_over_time(q[d])"
		description: "First raw sample in the window."
		example: {
			args: {over: "5m"}
			promql: "first_over_time(m[5m])"
		}
	}
	geomean_over_time: _rollup & {
		signature:   "geomean_over_time(q[d])"
		description: "Geometric mean of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "geomean_over_time(m[5m])"
		}
	}
	histogram_over_time: _rollup & {
		signature:   "histogram_over_time(q[d])"
		description: "VictoriaMetrics histogram (vmrange buckets) of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "histogram_over_time(m[5m])"
		}
	}
	ideriv: _rollup & {
		signature:   "ideriv(q[d])"
		description: "Per-second derivative from the last two raw samples in the window."
		example: {
			args: {over: "5m"}
			promql: "ideriv(m[5m])"
		}
	}
	increase_prometheus: _rollup & {
		signature:   "increase_prometheus(q[d])"
		description: "Increase of a counter over the window, ignoring the sample before it, as Prometheus does."
		example: {
			args: {over: "1h", step: "5m"}
			promql: "increase_prometheus(m[1h:5m])"
		}
	}
	increase_pure: _rollup & {
		signature:   "increase_pure(q[d])"
		description: "Increase of a counter over the window, assuming every counter starts from 0."
		example: {
			args: {over: "5m"}
			promql: "increase_pure(m[5m])"
		}
	}
	integrate: _rollup & {
		signature:   "integrate(q[d])"
		description: "Integral of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "integrate(m[5m])"
		}
	}
	lag: _rollup & {
		signature:   "lag(q[d])"
		description: "Seconds between the last raw sample in the window and the evaluation time."
		example: {
			args: {}
			promql: "lag(m)"
		}
	}
	lifetime: _rollup & {
		signature:   "lifetime(q[d])"
		description: "Seconds between the first and the last raw samples in the window."
		example: {
			args: {over: "5m"}
			promql: "lifetime(m[5m])"
		}
	}
	mad_over_time: _rollup & {
		signature:   "mad_over_time(q[d])"
		description: "Median absolute deviation of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "mad_over_time(m[5m])"
		}
	}
	median_over_time: _rollup & {
		signature:   "median_over_time(q[d])"
		description: "Median of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "median_over_time(m[5m])"
		}
	}
	mode_over_time: _rollup & {
		signature:   "mode_over_time(q[d])"
		description: "Most frequent raw sample value over the window."
		example: {
			args: {over: "5m"}
			promql: "mode_over_time(m[5m])"
		}
	}
	outlier_iqr_over_time: _rollup & {
		signature:   "outlier_iqr_over_time(q[d])"
		description: "Last raw sample in the window when it falls outside the window's interquartile range bounds."
		example: {
			args: {over: "5m"}
			promql: "outlier_iqr_over_time(m[5m])"
		}
	}
	rate_prometheus: _rollup & {
		signature:   "rate_prometheus(q[d])"
		description: "Per-second increase rate of a counter over the window, ignoring the sample before it, as Prometheus does."
		example: {
			args: {over: "5m"}
			selectorModifiers: {offset: "1h", at: "end()"}
			promql: "rate_prometheus(m[5m] offset 1h @ end())"
		}
	}
	rate_over_sum: _rollup & {
		signature:   "rate_over_sum(q[d])"
		description: "Per-second rate over the sum of raw samples in the window."
		example: {
			args: {over: "5m"}
			promql: "rate_over_sum(m[5m])"
		}
	}
	scrape_interval: _rollup & {
		signature:   "scrape_interval(q[d])"
		description: "Average seconds between raw samples over the window."
		example: {
			args: {step: "1m"}
			promql: "scrape_interval(m[:1m])"
		}
	}
	stale_samples_over_time: _rollup & {
		signature:   "stale_samples_over_time(q[d])"
		description: "Number of staleness markers over the window."
		example: {
			args: {over: "5m"}
			promql: "stale_samples_over_time(m[5m])"
		}
	}
	sum2_over_time: _rollup & {
		signature:   "sum2_over_time(q[d])"
		description: "Sum of squares of raw samples over the window."
		example: {
			args: {over: "5m"}
			promql: "sum2_over_time(m[5m])"
		}
	}
	timestamp: _rollup & {
		signature:   "timestamp(q[d])"
		description: "Timestamp in seconds of the last raw sample in the window."
		example: {
			args: {over: "5m"}
			promql: "timestamp(m[5m])"
		}
	}
	timestamp_with_name: _rollup & {
		signature:   "timestamp_with_name(q[d])"
		description: "Timestamp in seconds of the last raw sample in the window, keeping the metric name."
		example: {
			args: {over: "5m"}
			promql: "timestamp_with_name(m[5m])"
		}
	}
	tfirst_over_time: _rollup & {
		signature:   "tfirst_over_time(q[d])"
		description: "Timestamp in seconds of the first raw sample in the window."
		example: {
			args: {over: "5m"}
			promql: "tfirst_over_time(m[5m])"
		}
	}
	tlast_change_over_time: _rollup & {
		signature:   "tlast_change_over_time(q[d])"
		description: "Timestamp in seconds of the last value change in the window."
		example: {
			args: {over: "5m"}
			promql: "tlast_change_over_time(m[5m])"
		}
	}
	tlast_over_time: _rollup & {
		signature:   "tlast_over_time(q[d])"
		description: "Timestamp in seconds of the last raw sample in the window; an alias for timestamp."
		example: {
			args: {over: "5m"}
			promql: "tlast_over_time(m[5m])"
		}
	}
	tmax_over_time: _rollup & {
		signature:   "tmax_over_time(q[d])"
		description: "Timestamp in seconds of the maximum raw sample in the window."
		example: {
			args: {over: "5m"}
			promql: "tmax_over_time(m[5m])"
		}
	}
	tmin_over_time: _rollup & {
		signature:   "tmin_over_time(q[d])"
		description: "Timestamp in seconds of the minimum raw sample in the window."
		example: {
			args: {over: "5m"}
			promql: "tmin_over_time(m[5m])"
		}
	}
	zscore_over_time: _rollup & {
		signature:   "zscore_over_time(q[d])"
		description: "z-score of raw samples over the window."
		example: {
			args: {over: "1d", keep_metric_names: true}
			promql: "zscore_over_time(m[1d]) keep_metric_names"
		}
	}
	count_eq_over_time: _rollup & {
		params: [_window, {name: "eq", at: "after", default: 0}]
		signature:   "count_eq_over_time(q[d], eq)"
		description: "Number of raw samples equal to eq over the window."
		example: {
			args: {over: "5m", eq: 0}
			promql: "count_eq_over_time(m[5m], 0)"
		}
	}
	count_gt_over_time: _rollup & {
		params: [_window, {name: "gt", at: "after", default: 0}]
		signature:   "count_gt_over_time(q[d], gt)"
		description: "Number of raw samples greater than gt over the window."
		example: {
			args: {over: "5m", gt: 100}
			promql: "count_gt_over_time(m[5m], 100)"
		}
	}
	count_le_over_time: _rollup & {
		params: [_window, {name: "le", at: "after", default: 0}]
		signature:   "count_le_over_time(q[d], le)"
		description: "Number of raw samples less than or equal to le over the window."
		example: {
			args: {over: "5m", le: 0}
			promql: "count_le_over_time(m[5m], 0)"
		}
	}
	count_ne_over_time: _rollup & {
		params: [_window, {name: "ne", at: "after", default: 0}]
		signature:   "count_ne_over_time(q[d], ne)"
		description: "Number of raw samples not equal to ne over the window."
		example: {
			args: {over: "5m", ne: 0}
			promql: "count_ne_over_time(m[5m], 0)"
		}
	}
	share_eq_over_time: _rollup & {
		params: [_window, {name: "eq", at: "after", default: 0}]
		signature:   "share_eq_over_time(q[d], eq)"
		description: "Share, in [0, 1], of raw samples equal to eq over the window."
		example: {
			args: {over: "5m", eq: 0}
			promql: "share_eq_over_time(m[5m], 0)"
		}
	}
	share_gt_over_time: _rollup & {
		params: [_window, {name: "gt", at: "after", default: 0}]
		signature:   "share_gt_over_time(q[d], gt)"
		description: "Share, in [0, 1], of raw samples greater than gt over the window."
		example: {
			args: {over: "1d", gt: 0}
			promql: "share_gt_over_time(m[1d], 0)"
		}
	}
	share_le_over_time: _rollup & {
		params: [_window, {name: "le", at: "after", default: 0}]
		signature:   "share_le_over_time(q[d], le)"
		description: "Share, in [0, 1], of raw samples less than or equal to le over the window."
		example: {
			args: {over: "1d", le: 0.5}
			promql: "share_le_over_time(m[1d], 0.5)"
		}
	}
	sum_eq_over_time: _rollup & {
		params: [_window, {name: "eq", at: "after", default: 0}]
		signature:   "sum_eq_over_time(q[d], eq)"
		description: "Sum of raw samples equal to eq over the window."
		example: {
			args: {over: "5m", eq: 0}
			promql: "sum_eq_over_time(m[5m], 0)"
		}
	}
	sum_gt_over_time: _rollup & {
		params: [_window, {name: "gt", at: "after", default: 0}]
		signature:   "sum_gt_over_time(q[d], gt)"
		description: "Sum of raw samples greater than gt over the window."
		example: {
			args: {over: "5m", gt: 0}
			promql: "sum_gt_over_time(m[5m], 0)"
		}
	}
	sum_le_over_time: _rollup & {
		params: [_window, {name: "le", at: "after", default: 0}]
		signature:   "sum_le_over_time(q[d], le)"
		description: "Sum of raw samples less than or equal to le over the window."
		example: {
			args: {over: "5m", le: 0}
			promql: "sum_le_over_time(m[5m], 0)"
		}
	}
	predict_linear: _rollup & {
		params: [_window, {name: "t", at: "after", default: 3600}]
		signature:   "predict_linear(q[d], t)"
		description: "Value t seconds ahead, by linear regression over the window."
		example: {
			args: {over: "1h", t: 14400}
			promql: "predict_linear(m[1h], 14400)"
		}
	}
	holt_winters: _rollup & {
		params: [_window, {name: "sf", at: "after", default: 0.5}, {name: "tf", at: "after", default: 0.5}]
		signature:   "holt_winters(q[d], sf, tf)"
		description: "Holt-Winters double exponential smoothing over the window, with smoothing factor sf and trend factor tf in [0, 1]."
		example: {
			args: {over: "1h", sf: 0.3, tf: 0.1}
			promql: "holt_winters(m[1h], 0.3, 0.1)"
		}
	}
	duration_over_time: _rollup & {
		params: [_window, {name: "max_interval", at: "after", default: "1m"}]
		signature:   "duration_over_time(q[d], max_interval)"
		description: "Seconds the series was present over the window, counting gaps longer than max_interval as absence."
		example: {
			args: {over: "1h", max_interval: "30s"}
			promql: "duration_over_time(m[1h], 30s)"
		}
	}
	count_values_over_time: _rollup & {
		params: [{name: "label", at: "before", default: "value"}, _window]
		signature:   "count_values_over_time(\"label\", q[d])"
		description: "Number of raw samples per distinct value over the window, the value stored in label."
		example: {
			args: {label: "value", over: "1h"}
			promql: "count_values_over_time(\"value\", m[1h])"
		}
	}
	hoeffding_bound_lower: _rollup & {
		params: [{name: "phi", at: "before", default: 0.95}, _window]
		signature:   "hoeffding_bound_lower(phi, q[d])"
		description: "Lower Hoeffding bound for phi in [0, 1] over the window."
		example: {
			args: {phi: 0.9, over: "1h"}
			promql: "hoeffding_bound_lower(0.9, m[1h])"
		}
	}
	hoeffding_bound_upper: _rollup & {
		params: [{name: "phi", at: "before", default: 0.95}, _window]
		signature:   "hoeffding_bound_upper(phi, q[d])"
		description: "Upper Hoeffding bound for phi in [0, 1] over the window."
		example: {
			args: {phi: 0.9, over: "1h"}
			promql: "hoeffding_bound_upper(0.9, m[1h])"
		}
	}
	quantiles_over_time: _rollup & {
		params: [{name: "phi_label", at: "before", default: "quantile"}, {name: "phis", at: "before", default: [0.5, 0.9, 0.99]}, _window]
		signature:   "quantiles_over_time(\"phiLabel\", phi1, ..., phiN, q[d])"
		description: "Several phi-quantiles of raw samples over the window, one series per phi told apart by the phiLabel label."
		example: {
			args: {phi_label: "phi", phis: [0.5, 0.99], over: "5m"}
			promql: "quantiles_over_time(\"phi\", 0.5, 0.99, m[5m])"
		}
	}
	aggr_over_time: _rollup & {
		params: [{name: "rollups", at: "before", default: ["min_over_time", "max_over_time"]}, _window]
		signature:   "aggr_over_time((\"rollup_func1\", ...), q[d])"
		description: "Every listed rollup function over the same window, one series per function."
		example: {
			args: {rollups: ["min_over_time", "max_over_time", "rate"], over: "5m"}
			promql: "aggr_over_time((\"min_over_time\", \"max_over_time\", \"rate\"), m[5m])"
		}
	}
	rollup: _rollup & {
		params: [_window, {name: "rollup", at: "after", optional: true}]
		signature:   "rollup(q[d], \"min\"|\"max\"|\"avg\")"
		description: "Minimum, maximum and average of raw samples over the window, labelled rollup=min|max|avg; the optional argument keeps only one."
		example: {
			args: {over: "5m", rollup: "max"}
			promql: "rollup(m[5m], \"max\")"
		}
	}
	rollup_candlestick: _rollup & {
		params: [_window, {name: "ohlc", at: "after", optional: true}]
		signature:   "rollup_candlestick(q[d], \"open\"|\"high\"|\"low\"|\"close\")"
		description: "Open, high, low and close of raw samples over the window, labelled rollup=open|high|low|close; the optional argument keeps only one."
		example: {
			args: {over: "1h", ohlc: "close"}
			promql: "rollup_candlestick(m[1h], \"close\")"
		}
	}
	rollup_delta: _rollup & {
		params: [_window, {name: "rollup", at: "after", optional: true}]
		signature:   "rollup_delta(q[d], \"min\"|\"max\"|\"avg\")"
		description: "Minimum, maximum and average of the differences between adjacent raw samples over the window, labelled rollup=min|max|avg; the optional argument keeps only one."
		example: {
			args: {over: "5m"}
			promql: "rollup_delta(m[5m])"
		}
	}
	rollup_deriv: _rollup & {
		params: [_window, {name: "rollup", at: "after", optional: true}]
		signature:   "rollup_deriv(q[d], \"min\"|\"max\"|\"avg\")"
		description: "Minimum, maximum and average of the per-second derivatives between adjacent raw samples over the window, labelled rollup=min|max|avg; the optional argument keeps only one."
		example: {
			args: {over: "5m"}
			promql: "rollup_deriv(m[5m])"
		}
	}
	rollup_increase: _rollup & {
		params: [_window, {name: "rollup", at: "after", optional: true}]
		signature:   "rollup_increase(q[d], \"min\"|\"max\"|\"avg\")"
		description: "Minimum, maximum and average of the increases between adjacent raw samples over the window, labelled rollup=min|max|avg; the optional argument keeps only one."
		example: {
			args: {over: "5m"}
			promql: "rollup_increase(m[5m])"
		}
	}
	rollup_rate: _rollup & {
		params: [_window, {name: "rollup", at: "after", optional: true}]
		signature:   "rollup_rate(q[d], \"min\"|\"max\"|\"avg\")"
		description: "Minimum, maximum and average of the per-second change rates between adjacent raw samples over the window, labelled rollup=min|max|avg; the optional argument keeps only one."
		example: {
			args: {over: "5m"}
			promql: "rollup_rate(m[5m])"
		}
	}
	rollup_scrape_interval: _rollup & {
		params: [_window, {name: "rollup", at: "after", optional: true}]
		signature:   "rollup_scrape_interval(q[d], \"min\"|\"max\"|\"avg\")"
		description: "Minimum, maximum and average of the seconds between adjacent raw samples over the window, labelled rollup=min|max|avg; the optional argument keeps only one."
		example: {
			args: {over: "5m"}
			promql: "rollup_scrape_interval(m[5m])"
		}
	}
}
