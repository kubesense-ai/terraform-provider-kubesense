package catalog

// Legacy `comparison` arm: infix against a scalar. The legacy tuple accepted
// `than` or `to` on every operator; the builder wrote `than` for greater and
// lesser and `to` for the rest, which is what each entry's canonical name and
// alias preserve.
_comparison: {
	type: "comparison", category: "operator", legacy: "comparison", form: "infix"
	params!: [...#Param]
}
_than: {name: "than", at: "rhs", default: 0, aliases: ["to"]}
_to: {name: "to", at: "rhs", default: 0, aliases: ["than"]}

// Legacy `arithmetic` arm: infix against a scalar. `bool` only means something
// on a comparison.
_arithmetic: {
	type: "arithmetic", category: "operator", legacy: "arithmetic", form: "infix"
	except: ["bool"]
	params: *[{name: "value", at: "rhs", default: 1}] | [...#Param]
}

// MetricsQL's own binary operators against a scalar.
_operator: {
	type: "operator", category: "operator", form: "infix"
	except: ["bool"]
	params: *[{name: "value", at: "rhs", default: 0}] | [...#Param]
}

catalog: {
	greater: _comparison & {
		callee: ">"
		params: [_than]
		signature:   "q > v"
		description: "Keeps samples greater than v."
		example: {args: {than: 0.5}, promql: "m > 0.5"}
	}
	lesser: _comparison & {
		callee: "<"
		params: [_than]
		signature:   "q < v"
		description: "Keeps samples less than v."
		example: {args: {than: 100, bool: true}, promql: "m < bool 100"}
	}
	greater_than_or_equal: _comparison & {
		callee: ">="
		params: [_to]
		signature:   "q >= v"
		description: "Keeps samples greater than or equal to v."
		example: {args: {to: 1}, promql: "m >= 1"}
	}
	less_than_or_equal: _comparison & {
		callee: "<="
		params: [_to]
		signature:   "q <= v"
		description: "Keeps samples less than or equal to v."
		example: {args: {to: 1, bool: true, keep_metric_names: true}, promql: "(m <= bool 1) keep_metric_names"}
	}
	equal: _comparison & {
		callee: "=="
		params: [_to]
		signature:   "q == v"
		description: "Keeps samples equal to v."
		example: {args: {to: 0}, promql: "m == 0"}
	}
	not_equal: _comparison & {
		callee: "!="
		params: [_to]
		signature:   "q != v"
		description: "Keeps samples not equal to v."
		example: {args: {to: 0}, promql: "m != 0"}
	}

	add: _arithmetic & {
		callee:      "+"
		signature:   "q + v"
		description: "Adds v to every sample."
		example: {args: {value: 1}, promql: "m + 1"}
	}
	subtract: _arithmetic & {
		callee:      "-"
		signature:   "q - v"
		description: "Subtracts v from every sample."
		example: {args: {value: 1}, promql: "m - 1"}
	}
	multiply: _arithmetic & {
		callee: "*"
		// 100 because the common use is a ratio shown as a percentage.
		params: [{name: "value", at: "rhs", default: 100}]
		signature:   "q * v"
		description: "Multiplies every sample by v."
		example: {chain: [{name: "greater", args: {than: 5}}], args: {value: 100}, promql: "(m > 5) * 100"}
	}
	divide: _arithmetic & {
		callee:      "/"
		signature:   "q / v"
		description: "Divides every sample by v."
		example: {args: {value: 1024, keep_metric_names: true}, promql: "(m / 1024) keep_metric_names"}
	}
	modulo: _arithmetic & {
		callee:      "%"
		signature:   "q % v"
		description: "Remainder of every sample divided by v."
		example: {args: {value: 60}, promql: "m % 60"}
	}
	power: _arithmetic & {
		callee:      "^"
		signature:   "q ^ v"
		description: "Raises every sample to the power v."
		example: {args: {value: 2}, promql: "m ^ 2"}
	}

	default: _operator & {
		callee:      "default"
		signature:   "q default v"
		description: "Gaps in q filled with v."
		example: {
			selectorModifiers: {offset: "1d"}
			args: {value: 0}
			promql: "m offset 1d default 0"
		}
	}
	"if": _operator & {
		callee: "if"
		params: [{name: "value", at: "rhs", default: 1}]
		signature:   "q if v"
		description: "q wherever the scalar v is not empty."
		example: {args: {value: 1}, promql: "m if 1"}
	}
	ifnot: _operator & {
		callee: "ifnot"
		params: [{name: "value", at: "rhs", default: 1}]
		signature:   "q ifnot v"
		description: "q wherever the scalar v is empty."
		example: {args: {value: 1}, promql: "m ifnot 1"}
	}
}
