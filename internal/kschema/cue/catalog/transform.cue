package catalog

// Legacy `transform` arm. Its tuple was an open list of any of min, max,
// to_nearest and quantile on any name, which the generator keeps as the
// legacy alternative; the params here are what each function actually takes.
_legacyTransform: {
	type: "transform", category: "transform", legacy: "transform"
	params: *[] | [...#Param]
}

_transform: {
	type: "transform", category: "transform"
	params: *[] | [...#Param]
}

catalog: {
	abs: _legacyTransform & {
		signature:   "abs(q)"
		description: "Absolute value of every sample."
		example: {
			args: {keep_metric_names: true}
			promql: "abs(m) keep_metric_names"
		}
	}
	clamp: _legacyTransform & {
		params: [{name: "min", at: "after", default: 0}, {name: "max", at: "after", default: 0}]
		signature:   "clamp(q, min, max)"
		description: "Clamps every sample to the [min, max] range."
		example: {
			args: {min: 0, max: 100}
			promql: "clamp(m, 0, 100)"
		}
	}
	clamp_max: _legacyTransform & {
		params: [{name: "max", at: "after", default: 0}]
		signature:   "clamp_max(q, max)"
		description: "Clamps every sample to at most max."
		example: {
			args: {max: 100}
			promql: "clamp_max(m, 100)"
		}
	}
	clamp_min: _legacyTransform & {
		params: [{name: "min", at: "after", default: 0}]
		signature:   "clamp_min(q, min)"
		description: "Clamps every sample to at least min."
		example: {
			args: {min: 0}
			promql: "clamp_min(m, 0)"
		}
	}
	// round(q, 0) rounds to a multiple of 0, which is NaN for every sample.
	round: _legacyTransform & {
		params: [{name: "to_nearest", at: "after", default: 1}]
		signature:   "round(q, nearest)"
		description: "Rounds every sample to the nearest multiple of nearest."
		example: {
			args: {to_nearest: 0.1}
			promql: "round(m, 0.1)"
		}
	}
	histogram_quantile: _legacyTransform & {
		params: [{name: "quantile", at: "before", default: 0.95}, {name: "bounds_label", at: "after", optional: true}]
		signature:   "histogram_quantile(phi, buckets, \"boundsLabel\")"
		description: "phi-quantile over histogram buckets (le or vmrange), phi in [0, 1]; with boundsLabel, also the lower and upper bounds of the estimate."
		example: {
			chain: [{name: "rate", args: {over: "5m"}}, {name: "sum", args: {by: ["le"]}}]
			args: {quantile: 0.95}
			promql: "histogram_quantile(0.95, sum by (le) (rate(m[5m])))"
		}
	}
	sort: _legacyTransform & {
		signature:   "sort(q)"
		description: "Series sorted by their last value, ascending."
		example: {
			args: {}
			promql: "sort(m)"
		}
	}
	sort_desc: _legacyTransform & {
		signature:   "sort_desc(q)"
		description: "Series sorted by their last value, descending."
		example: {
			args: {}
			promql: "sort_desc(m)"
		}
	}
	ceil: _legacyTransform & {
		signature:   "ceil(q)"
		description: "Every sample rounded up to the nearest integer."
		example: {
			args: {}
			promql: "ceil(m)"
		}
	}
	floor: _legacyTransform & {
		signature:   "floor(q)"
		description: "Every sample rounded down to the nearest integer."
		example: {
			args: {}
			promql: "floor(m)"
		}
	}
	sqrt: _legacyTransform & {
		signature:   "sqrt(q)"
		description: "Square root of every sample."
		example: {
			args: {keep_metric_names: true}
			promql: "sqrt(m) keep_metric_names"
		}
	}
	exp: _legacyTransform & {
		signature:   "exp(q)"
		description: "e raised to the power of every sample."
		example: {
			args: {}
			promql: "exp(m)"
		}
	}
	ln: _legacyTransform & {
		signature:   "ln(q)"
		description: "Natural logarithm of every sample."
		example: {
			args: {}
			promql: "ln(m)"
		}
	}
	log2: _legacyTransform & {
		signature:   "log2(q)"
		description: "Base-2 logarithm of every sample."
		example: {
			args: {}
			promql: "log2(m)"
		}
	}
	log10: _legacyTransform & {
		signature:   "log10(q)"
		description: "Base-10 logarithm of every sample."
		example: {
			args: {}
			promql: "log10(m)"
		}
	}
	absent: _transform & {
		signature:   "absent(q)"
		description: "1 when q has no points, empty otherwise."
		example: {
			args: {}
			promql: "absent(m)"
		}
	}
	acos: _transform & {
		signature:   "acos(q)"
		description: "Inverse cosine of every sample."
		example: {
			args: {}
			promql: "acos(m)"
		}
	}
	acosh: _transform & {
		signature:   "acosh(q)"
		description: "Inverse hyperbolic cosine of every sample."
		example: {
			args: {}
			promql: "acosh(m)"
		}
	}
	asin: _transform & {
		signature:   "asin(q)"
		description: "Inverse sine of every sample."
		example: {
			args: {}
			promql: "asin(m)"
		}
	}
	asinh: _transform & {
		signature:   "asinh(q)"
		description: "Inverse hyperbolic sine of every sample."
		example: {
			args: {}
			promql: "asinh(m)"
		}
	}
	atan: _transform & {
		signature:   "atan(q)"
		description: "Inverse tangent of every sample."
		example: {
			args: {}
			promql: "atan(m)"
		}
	}
	atanh: _transform & {
		signature:   "atanh(q)"
		description: "Inverse hyperbolic tangent of every sample."
		example: {
			args: {}
			promql: "atanh(m)"
		}
	}
	cos: _transform & {
		signature:   "cos(q)"
		description: "Cosine of every sample."
		example: {
			args: {}
			promql: "cos(m)"
		}
	}
	cosh: _transform & {
		signature:   "cosh(q)"
		description: "Hyperbolic cosine of every sample."
		example: {
			args: {}
			promql: "cosh(m)"
		}
	}
	day_of_month: _transform & {
		signature:   "day_of_month(q)"
		description: "Day of the month, 1 to 31, of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "day_of_month(m)"
		}
	}
	day_of_week: _transform & {
		signature:   "day_of_week(q)"
		description: "Day of the week, 0 (Sunday) to 6, of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "day_of_week(m)"
		}
	}
	day_of_year: _transform & {
		signature:   "day_of_year(q)"
		description: "Day of the year, 1 to 366, of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "day_of_year(m)"
		}
	}
	days_in_month: _transform & {
		signature:   "days_in_month(q)"
		description: "Number of days in the month of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "days_in_month(m)"
		}
	}
	deg: _transform & {
		signature:   "deg(q)"
		description: "Every sample converted from radians to degrees."
		example: {
			args: {}
			promql: "deg(m)"
		}
	}
	drop_empty_series: _transform & {
		signature:   "drop_empty_series(q)"
		description: "q without the series that have no points."
		example: {
			args: {}
			promql: "drop_empty_series(m)"
		}
	}
	histogram_avg: _transform & {
		signature:   "histogram_avg(buckets)"
		description: "Average value of the given histogram buckets."
		example: {
			args: {}
			promql: "histogram_avg(m)"
		}
	}
	histogram_stddev: _transform & {
		signature:   "histogram_stddev(buckets)"
		description: "Standard deviation of the given histogram buckets."
		example: {
			args: {}
			promql: "histogram_stddev(m)"
		}
	}
	histogram_stdvar: _transform & {
		signature:   "histogram_stdvar(buckets)"
		description: "Standard variance of the given histogram buckets."
		example: {
			args: {}
			promql: "histogram_stdvar(m)"
		}
	}
	hour: _transform & {
		signature:   "hour(q)"
		description: "Hour, 0 to 23, of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "hour(m)"
		}
	}
	interpolate: _transform & {
		signature:   "interpolate(q)"
		description: "Gaps filled by linear interpolation between the neighbouring points."
		example: {
			args: {}
			promql: "interpolate(m)"
		}
	}
	keep_last_value: _transform & {
		signature:   "keep_last_value(q)"
		description: "Gaps filled with the last non-empty point."
		example: {
			args: {}
			promql: "keep_last_value(m)"
		}
	}
	keep_next_value: _transform & {
		signature:   "keep_next_value(q)"
		description: "Gaps filled with the next non-empty point."
		example: {
			args: {}
			promql: "keep_next_value(m)"
		}
	}
	minute: _transform & {
		signature:   "minute(q)"
		description: "Minute, 0 to 59, of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "minute(m)"
		}
	}
	month: _transform & {
		signature:   "month(q)"
		description: "Month, 1 to 12, of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "month(m)"
		}
	}
	rad: _transform & {
		signature:   "rad(q)"
		description: "Every sample converted from degrees to radians."
		example: {
			args: {}
			promql: "rad(m)"
		}
	}
	prometheus_buckets: _transform & {
		signature:   "prometheus_buckets(buckets)"
		description: "VictoriaMetrics vmrange buckets converted to Prometheus le buckets."
		example: {
			args: {}
			promql: "prometheus_buckets(m)"
		}
	}
	range_avg: _transform & {
		signature:   "range_avg(q)"
		description: "Average of the points of each series over the selected time range."
		example: {
			args: {}
			promql: "range_avg(m)"
		}
	}
	range_first: _transform & {
		signature:   "range_first(q)"
		description: "Value of the first point of each series over the selected time range."
		example: {
			args: {}
			promql: "range_first(m)"
		}
	}
	range_last: _transform & {
		signature:   "range_last(q)"
		description: "Value of the last point of each series over the selected time range."
		example: {
			args: {}
			promql: "range_last(m)"
		}
	}
	range_linear_regression: _transform & {
		signature:   "range_linear_regression(q)"
		description: "Simple linear regression of each series over the selected time range."
		example: {
			args: {}
			promql: "range_linear_regression(m)"
		}
	}
	range_mad: _transform & {
		signature:   "range_mad(q)"
		description: "Median absolute deviation of the points of each series over the selected time range."
		example: {
			args: {}
			promql: "range_mad(m)"
		}
	}
	range_max: _transform & {
		signature:   "range_max(q)"
		description: "Maximum of the points of each series over the selected time range."
		example: {
			args: {}
			promql: "range_max(m)"
		}
	}
	range_median: _transform & {
		signature:   "range_median(q)"
		description: "Median of the points of each series over the selected time range."
		example: {
			args: {}
			promql: "range_median(m)"
		}
	}
	range_min: _transform & {
		signature:   "range_min(q)"
		description: "Minimum of the points of each series over the selected time range."
		example: {
			args: {}
			promql: "range_min(m)"
		}
	}
	range_normalize: _transform & {
		signature:   "range_normalize(q)"
		description: "Every series scaled into [0, 1] over the selected time range."
		example: {
			args: {}
			promql: "range_normalize(m)"
		}
	}
	range_stddev: _transform & {
		signature:   "range_stddev(q)"
		description: "Standard deviation of each series over the selected time range."
		example: {
			args: {}
			promql: "range_stddev(m)"
		}
	}
	range_stdvar: _transform & {
		signature:   "range_stdvar(q)"
		description: "Standard variance of each series over the selected time range."
		example: {
			args: {}
			promql: "range_stdvar(m)"
		}
	}
	range_sum: _transform & {
		signature:   "range_sum(q)"
		description: "Sum of the points of each series over the selected time range."
		example: {
			args: {}
			promql: "range_sum(m)"
		}
	}
	range_zscore: _transform & {
		signature:   "range_zscore(q)"
		description: "z-score of every point against its series over the selected time range."
		example: {
			args: {}
			promql: "range_zscore(m)"
		}
	}
	remove_resets: _transform & {
		signature:   "remove_resets(q)"
		description: "Counter resets removed from every series."
		example: {
			args: {}
			promql: "remove_resets(m)"
		}
	}
	running_avg: _transform & {
		signature:   "running_avg(q)"
		description: "Running average of every series."
		example: {
			args: {}
			promql: "running_avg(m)"
		}
	}
	running_max: _transform & {
		signature:   "running_max(q)"
		description: "Running maximum of every series."
		example: {
			args: {}
			promql: "running_max(m)"
		}
	}
	running_min: _transform & {
		signature:   "running_min(q)"
		description: "Running minimum of every series."
		example: {
			args: {}
			promql: "running_min(m)"
		}
	}
	running_sum: _transform & {
		signature:   "running_sum(q)"
		description: "Running sum of every series."
		example: {
			args: {}
			promql: "running_sum(m)"
		}
	}
	scalar: _transform & {
		signature:   "scalar(q)"
		description: "q as a scalar when it holds exactly one series, empty otherwise."
		example: {
			args: {}
			promql: "scalar(m)"
		}
	}
	sgn: _transform & {
		signature:   "sgn(q)"
		description: "1, -1 or 0 for every positive, negative or zero sample."
		example: {
			args: {}
			promql: "sgn(m)"
		}
	}
	sin: _transform & {
		signature:   "sin(q)"
		description: "Sine of every sample."
		example: {
			args: {}
			promql: "sin(m)"
		}
	}
	sinh: _transform & {
		signature:   "sinh(q)"
		description: "Hyperbolic sine of every sample."
		example: {
			args: {}
			promql: "sinh(m)"
		}
	}
	tan: _transform & {
		signature:   "tan(q)"
		description: "Tangent of every sample."
		example: {
			args: {}
			promql: "tan(m)"
		}
	}
	tanh: _transform & {
		signature:   "tanh(q)"
		description: "Hyperbolic tangent of every sample."
		example: {
			args: {}
			promql: "tanh(m)"
		}
	}
	ttf: _transform & {
		signature:   "ttf(free)"
		description: "Estimated seconds until the free resource q is exhausted."
		example: {
			args: {}
			promql: "ttf(m)"
		}
	}
	vector: _transform & {
		signature:   "vector(q)"
		description: "q unchanged."
		example: {
			args: {}
			promql: "vector(m)"
		}
	}
	year: _transform & {
		signature:   "year(q)"
		description: "Year of every sample read as a unix timestamp."
		example: {
			args: {}
			promql: "year(m)"
		}
	}
	bitmap_and: _transform & {
		params: [{name: "mask", at: "after", default: 1}]
		signature:   "bitmap_and(q, mask)"
		description: "Bitwise v & mask of every sample v."
		example: {
			args: {mask: 255}
			promql: "bitmap_and(m, 255)"
		}
	}
	bitmap_or: _transform & {
		params: [{name: "mask", at: "after", default: 1}]
		signature:   "bitmap_or(q, mask)"
		description: "Bitwise v | mask of every sample v."
		example: {
			args: {mask: 1}
			promql: "bitmap_or(m, 1)"
		}
	}
	bitmap_xor: _transform & {
		params: [{name: "mask", at: "after", default: 1}]
		signature:   "bitmap_xor(q, mask)"
		description: "Bitwise v ^ mask of every sample v."
		example: {
			args: {mask: 3}
			promql: "bitmap_xor(m, 3)"
		}
	}
	buckets_limit: _transform & {
		params: [{name: "limit", at: "before", default: 10}]
		signature:   "buckets_limit(limit, buckets)"
		description: "Histogram buckets reduced to at most limit, keeping the first and the last."
		example: {
			args: {limit: 10}
			promql: "buckets_limit(10, m)"
		}
	}
	histogram_fraction: _transform & {
		params: [{name: "lower_le", at: "before", default: 0}, {name: "upper_le", at: "before", default: 1}]
		signature:   "histogram_fraction(lowerLe, upperLe, buckets)"
		description: "Share, in [0, 1], of histogram buckets between lowerLe and upperLe."
		example: {
			args: {lower_le: 0.1, upper_le: 0.5}
			promql: "histogram_fraction(0.1, 0.5, m)"
		}
	}
	histogram_quantiles: _transform & {
		params: [{name: "phi_label", at: "before", default: "quantile"}, {name: "phis", at: "before", default: [0.5, 0.9, 0.99]}]
		signature:   "histogram_quantiles(\"phiLabel\", phi1, ..., phiN, buckets)"
		description: "Several phi-quantiles over histogram buckets, one series per phi told apart by the phiLabel label."
		example: {
			args: {phi_label: "phi", phis: [0.5, 0.9]}
			promql: "histogram_quantiles(\"phi\", 0.5, 0.9, m)"
		}
	}
	histogram_share: _transform & {
		params: [{name: "le", at: "before", default: 1}, {name: "bounds_label", at: "after", optional: true}]
		signature:   "histogram_share(le, buckets, \"boundsLabel\")"
		description: "Share, in [0, 1], of histogram buckets below le; with boundsLabel, also the lower and upper bounds of the estimate."
		example: {
			args: {le: 0.5, bounds_label: "bounds"}
			promql: "histogram_share(0.5, m, \"bounds\")"
		}
	}
	limit_offset: _transform & {
		params: [{name: "limit", at: "before", default: 10}, {name: "offset", at: "before", default: 0}]
		signature:   "limit_offset(limit, offset, q)"
		description: "Up to limit series of q after skipping the first offset, for paging."
		example: {
			args: {limit: 10, offset: 20}
			promql: "limit_offset(10, 20, m)"
		}
	}
	range_quantile: _transform & {
		params: [{name: "phi", at: "before", default: 0.95}]
		signature:   "range_quantile(phi, q)"
		description: "phi-quantile of the points of each series over the selected time range, phi in [0, 1]."
		example: {
			args: {phi: 0.5}
			promql: "range_quantile(0.5, m)"
		}
	}
	range_trim_outliers: _transform & {
		params: [{name: "k", at: "before", default: 3}]
		signature:   "range_trim_outliers(k, q)"
		description: "Points farther than k times range_mad from range_median dropped."
		example: {
			args: {k: 3}
			promql: "range_trim_outliers(3, m)"
		}
	}
	range_trim_spikes: _transform & {
		params: [{name: "phi", at: "before", default: 0.01}]
		signature:   "range_trim_spikes(phi, q)"
		description: "The phi share, in [0, 1], of the biggest spikes dropped from every series."
		example: {
			args: {phi: 0.05}
			promql: "range_trim_spikes(0.05, m)"
		}
	}
	range_trim_zscore: _transform & {
		params: [{name: "z", at: "before", default: 3}]
		signature:   "range_trim_zscore(z, q)"
		description: "Points farther than z times range_stddev from range_avg dropped."
		example: {
			args: {z: 2}
			promql: "range_trim_zscore(2, m)"
		}
	}
	smooth_exponential: _transform & {
		params: [{name: "sf", at: "after", default: 0.5}]
		signature:   "smooth_exponential(q, sf)"
		description: "Exponential moving average of every series with smoothing factor sf in [0, 1]."
		example: {
			args: {sf: 0.2}
			promql: "smooth_exponential(m, 0.2)"
		}
	}
}
