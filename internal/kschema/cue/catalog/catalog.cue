// Package catalog is THE metrics builder function catalog: one entry per
// function the builder can apply to a selector, as pure data checked against
// #Function.
//
// Nothing here is exported as JSON Schema directly. CUE's JSON Schema encoder
// cannot export a definition built by a comprehension (it fails with "NamesFunc
// did not set Name field in all *CUERef values"), so the generator in
// catalog_test.go reads this package and writes LITERAL arms, one per entry, to
// cue/panel/functions_gen.cue, which the panel package then exports as usual.
// The same generator writes the resolved catalog to gen/metrics-functions.json
// for the webapp (contract:sync) and the skills repo.
//
// To add a function: add an entry in the file for its category, with an
// example whose promql is what the builder must render, then
//
//	go test ./schema -run TestGeneratedArtifactsAreCurrent -update
//	go test ./schema/...
//	cd ../webapp && npm run contract:sync
//
// The entry then has a contract arm, a zod arm, a menu entry, defaults and
// rendering. No other file changes.
//
// This package is test-only: it is not embedded in the binary, and the panel
// package never imports it. What ships is functions_gen.cue.
package catalog

import "list"

// The `type` a stored function carries. Informational since the generated arms
// discriminate on `name`; kept because every stored document has one and older
// webapps discriminate on it. The first seven are the legacy arms.
functionTypes: ["range", "aggregations", "top_bottom", "rollup", "comparison",
	"transform", "arithmetic", "label", "operator"]
#FunctionType: or(functionTypes)

// Menu group. Independent of `type`: rate is stored as "range" (legacy) but is
// listed under Rollup with the other rollups.
categories: ["rollup", "transform", "label", "aggregate", "operator"]
#Category: or(categories)

#LegacyType: "range" | "aggregations" | "top_bottom" | "rollup" | "comparison" |
	"transform" | "arithmetic"

// Where a parameter lands in the rendered text, relative to the input q.
//
//	before    f(P, q)                 phi, k, "label", a rollup-name list
//	after     f(q, P)                 gt, t, sf/tf, min/max, labels, pairs
//	window    q[P]                    lookbehind; absent = MetricsQL picks one
//	step      q[w:P]                  explicit subquery step
//	grouping  f by|without (P) (…)    aggregates; the param is `by` or `without`
//	limit     f(…) limit P            aggregates
//	suffix    … keep_metric_names     flag
//	bool      q > bool v              comparisons
//	rhs       q OP P                  infix form
//	callee    topk_P(…)               legacy top/bottom only: P picks the name
#Position: "before" | "after" | "window" | "step" | "grouping" | "limit" |
	"suffix" | "bool" | "rhs" | "callee"

#Role: "label" | "text"

// A parameter's value kind. Decides the stored arg_value schema (see
// valueSchema in catalog_test.go and kindSchema in the webapp's stored.ts),
// the UI cell, and with the position the rendered spelling.
#Kind: {kind: "number", min?: number, max?: number, integer?: bool} |
	{kind: "numbers", min?: number, max?: number, integer?: bool} |
	{kind: "window"} |
	{kind: "duration"} |
	{kind: "timestamp"} |
	{kind: "string", role: "label" | "text" | "regex"} |
	{kind: "enum", values: [...string], quoted: bool} |
	{kind: "labels"} |
	{kind: "pairs", roles: [#Role, #Role]} |
	{kind: "rollups"} |
	{kind: "flag"}

// The argument vocabulary: one kind per argument name, using the MetricsQL
// documentation's own names. An entry's parameter takes its kind from here by
// name, so two functions cannot spell the same argument differently. Legacy
// names keep their legacy kinds. The one legacy exception is `by`, a label list
// on aggregations and a ranking statistic on top/bottom; only a legacy entry
// may override the vocabulary (the generator checks this).
vocab: [=~"^[a-z_]+$"]: #Kind
vocab: {
	// Legacy argument names.
	over:       {kind: "window"}
	by:         {kind: "labels"}
	k:          {kind: "number"}
	quantile:   {kind: "number", min: 0, max: 1}
	than:       {kind: "number"}
	to:         {kind: "number"}
	min:        {kind: "number"}
	max:        {kind: "number"}
	to_nearest: {kind: "number"}
	value:      {kind: "number"}

	// Family modifiers.
	without:           {kind: "labels"}
	limit:             {kind: "number", integer: true, min: 1}
	step:              {kind: "duration"}
	keep_metric_names: {kind: "flag"}
	bool:              {kind: "flag"}

	// Scalars.
	phi:          {kind: "number", min: 0, max: 1}
	phis:         {kind: "numbers", min: 0, max: 1}
	sf:           {kind: "number", min: 0, max: 1}
	tf:           {kind: "number", min: 0, max: 1}
	t:            {kind: "number"}
	gt:           {kind: "number"}
	le:           {kind: "number"}
	eq:           {kind: "number"}
	ne:           {kind: "number"}
	lower_le:     {kind: "number"}
	upper_le:     {kind: "number"}
	z:            {kind: "number"}
	tolerance:    {kind: "number"}
	mask:         {kind: "number", integer: true, min: 0}
	offset:       {kind: "number", integer: true, min: 0}
	group_nums:   {kind: "numbers", integer: true, min: 0}
	max_interval: {kind: "duration"}

	// Strings.
	label:        {kind: "string", role: "label"}
	phi_label:    {kind: "string", role: "label"}
	bounds_label: {kind: "string", role: "label"}
	dst_label:    {kind: "string", role: "label"}
	src_label:    {kind: "string", role: "label"}
	name:         {kind: "string", role: "text"}
	replacement:  {kind: "string", role: "text"}
	separator:    {kind: "string", role: "text"}
	other:        {kind: "string", role: "text"}
	regex:        {kind: "string", role: "regex"}
	rollup:       {kind: "enum", values: ["min", "max", "avg"], quoted: true}
	ohlc:         {kind: "enum", values: ["open", "high", "low", "close"], quoted: true}

	// Lists.
	labels:       {kind: "labels"}
	src_labels:   {kind: "labels"}
	pairs:        {kind: "pairs", roles: ["label", "label"]}
	label_values: {kind: "pairs", roles: ["label", "text"]}
	value_pairs:  {kind: "pairs", roles: ["text", "text"]}
	rollups:      {kind: "rollups"}
}

#Param: {
	name!: =~"^[a-z_]+$"
	at!:   #Position
	kind:  *vocab[name] | #Kind
	// A non-optional parameter always renders: absent in storage means
	// `default`, which is how a quantile_over_time stored before phi existed
	// still renders 0.95. An optional one is omitted when absent, which is how
	// "no window" means "let MetricsQL pick one".
	optional: *false | bool
	if !optional {
		default!: _
	}
	default?: _
	// A list the function fails without, as labels_equal does with fewer than
	// two labels: the argument must be stored, holding at least this many
	// items. The default stays empty, so a function just added from the menu
	// is invalid until the user fills it in.
	minItems?: int & >=1
	// Other arg_names this parameter answers to. Legacy comparison accepted
	// `than` and `to` for every operator.
	aliases?: [...string]
	description?: string
}

// Modifier parameters every entry of a category takes unless it lists them in
// `except`. The generator appends them after the entry's own params. The lists
// live in families.cue.
families: [#Category]: modifiers: [...#Param]

#Function: {
	type!:     #FunctionType
	category!: #Category
	// call      name(before…, q[window], after…)
	// infix     q callee rhs
	// identity  q
	form: *"call" | "infix" | "identity"
	// The rendered name when it is not the entry name: ">" for greater,
	// "topk_{by}" for legacy top.
	callee?: string
	params!: [...#Param]
	// Family modifiers this entry does not take.
	except: *[] | [...string]
	// Whether the menu offers the function. A legacy spelling that stays
	// valid and rendered but should no longer be added gets false.
	menu: *true | false
	// Which legacy arm this name came from. The generator adds that arm's
	// argument tuple as an alternative, so every stored legacy document stays
	// valid, arm by arm. A legacy entry lists its tuple's params first, in
	// tuple order, so a function that gains no new argument serialises to
	// exactly its legacy tuple.
	legacy?:      #LegacyType
	signature!:   string
	description!: string
	example!:     #Example

	_paramNames: [for p in params {p.name}] & list.UniqueItems()
}

// One use of the function, rendered on the selector `m`: its arguments by name,
// optionally the functions applied before it and the query's selector
// modifiers, and the exact PromQL the builder must produce. kubeapi parses
// every example's promql with the MetricsQL parser and validates its arguments
// against the contract; the webapp renders the same arguments and compares.
#Example: {
	chain?: [...{name!: string, args!: {[string]: _}}]
	args!: {[string]: _}
	selectorModifiers?: {offset?: string, at?: "start()" | "end()" | number}
	promql!: string
}

#Catalog: [=~"^[A-Za-z_][A-Za-z0-9_]*$"]: #Function

catalog: #Catalog
