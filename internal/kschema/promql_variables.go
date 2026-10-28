package schema

import (
	"regexp"
	"strings"
)

// promql_variables.go — resolving dashboard "$variable" references in PromQL.
//
// A port of the webapp's explorer/v2/utils/promql-variables.ts and
// desugarFieldReferencesInPromQL (utils/variable-field-reference.ts), so a
// server-side reader runs the PromQL a dashboard panel sends. The webapp's
// tests are the goldens (promql_variables_test.go repeats them), except for
// three deliberate fixes: a negated matcher over an unselected variable matches
// everything, promoted exact matchers decode PromQL escapes before regex
// escaping, and the MCP caller keeps newlines.
//
// Go's regexp has no lookbehind, which the webapp's VARIABLE_REFERENCE uses so
// that the second "$" of "$$name" is never a value reference. The scanners
// below apply that rule by hand.

// VariableValues maps a dashboard variable's name (without "$") to its
// selected values. A variable present with no values means "All"; a variable
// absent from the map is unknown and its references are left as written.
type VariableValues map[string][]string

const (
	variableReferenceSource = `\$(?:\{([a-zA-Z_][a-zA-Z0-9_]*)\}|([a-zA-Z_][a-zA-Z0-9_]*))`
	labelMatcherSource      = `([a-zA-Z_][a-zA-Z0-9_]*)(\s*)(=~|!~|=|!=)(\s*)"((?:[^"\\]|\\.)*)"`
)

var (
	variableReferenceRe = regexp.MustCompile(variableReferenceSource)
	// Capture groups: 1-5 the label matcher, 6-7 a bare reference.
	substitutionScanRe = regexp.MustCompile(`^(?:` + labelMatcherSource + `|` + variableReferenceSource + `)`)
	inlineFlagsRe      = regexp.MustCompile(`^\(\?[a-zA-Z-]+\)`)
	// "$$name" at the cursor, with the separator that follows it.
	fieldReferenceCursorRe = regexp.MustCompile(`^\$\$([a-zA-Z_][a-zA-Z0-9_]*)\s*,?`)
	trailingCommaRe        = regexp.MustCompile(`,\s*$`)
)

// normaliseVariables drops the empty strings the webapp drops: an empty box is
// no selection, not the one value "".
func normaliseVariables(in VariableValues) VariableValues {
	out := make(VariableValues, len(in))
	for name, values := range in {
		kept := make([]string, 0, len(values))
		for _, v := range values {
			if v != "" {
				kept = append(kept, v)
			}
		}
		out[name] = kept
	}
	return out
}

type variableRef struct {
	start, end int
	name       string
}

// variableRefs finds the "$name" / "${name}" references in s, skipping a "$"
// that follows another "$".
func variableRefs(s string) []variableRef {
	var refs []variableRef
	for _, m := range variableReferenceRe.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > 0 && s[m[0]-1] == '$' {
			continue
		}
		name := ""
		if m[2] >= 0 {
			name = s[m[2]:m[3]]
		} else {
			name = s[m[4]:m[5]]
		}
		refs = append(refs, variableRef{start: m[0], end: m[1], name: name})
	}
	return refs
}

func renderRegexValues(values []string) string {
	if len(values) == 0 {
		return ".*"
	}
	escaped := make([]string, len(values))
	for i, v := range values {
		escaped[i] = escapeRegexLiteral(v)
	}
	if len(escaped) == 1 {
		return escaped[0]
	}
	return "(?:" + strings.Join(escaped, "|") + ")"
}

// renderTemplate renders a matcher value, writing literal text and variable
// references separately: they need different escaping. A reference the map
// cannot resolve is emitted as written rather than widened to ".*".
func renderTemplate(template string, vars VariableValues, renderLiteral func(string) string, renderValue func([]string) string) string {
	var out strings.Builder
	cursor := 0
	for _, ref := range variableRefs(template) {
		out.WriteString(renderLiteral(template[cursor:ref.start]))
		if values, ok := vars[ref.name]; ok {
			out.WriteString(renderValue(values))
		} else {
			out.WriteString(template[ref.start:ref.end])
		}
		cursor = ref.end
	}
	out.WriteString(renderLiteral(template[cursor:]))
	return out.String()
}

// resolvedValuesIn is the values of every reference in a template that vars
// can resolve.
func resolvedValuesIn(template string, vars VariableValues) [][]string {
	var resolved [][]string
	for _, ref := range variableRefs(template) {
		if values, ok := vars[ref.name]; ok {
			resolved = append(resolved, values)
		}
	}
	return resolved
}

func identity(s string) string { return s }

func substituteVariablesInRegex(pattern string, vars VariableValues) string {
	return renderTemplate(pattern, vars, identity, renderRegexValues)
}

func referencesEmptyVariable(template string, vars VariableValues) bool {
	for _, values := range resolvedValuesIn(template, vars) {
		if len(values) == 0 {
			return true
		}
	}
	return false
}

type regexAlternative struct {
	text string
	// flagsBefore is the top-level flag groups set before this alternative.
	flagsBefore string
}

// topLevelAlternatives splits a regex at its top-level "|", leaving escapes,
// groups and classes whole.
func topLevelAlternatives(pattern string) []regexAlternative {
	var alternatives []regexAlternative
	depth, start := 0, 0
	inClass := false
	flags, flagsAtStart := "", ""
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		flagGroup := ""
		if c == '(' && depth == 0 && !inClass {
			flagGroup = inlineFlagsRe.FindString(pattern[i:])
		}
		switch {
		case c == '\\':
			i++
		case inClass:
			inClass = c != ']'
		case flagGroup != "":
			flags += flagGroup
			i += len(flagGroup) - 1
		case c == '[':
			inClass = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == '|' && depth == 0:
			alternatives = append(alternatives, regexAlternative{text: pattern[start:i], flagsBefore: flagsAtStart})
			start = i + 1
			flagsAtStart = flags
		}
	}
	return append(alternatives, regexAlternative{text: pattern[min(start, len(pattern)):], flagsBefore: flagsAtStart})
}

// withoutEmptyVariableAlternatives is the pattern without the top-level
// alternatives that reference a variable with nothing selected; ok is false
// when none are left. For an exclusion, where an unselected variable should
// constrain nothing. A flag set in a dropped alternative still reaches the
// kept ones.
func withoutEmptyVariableAlternatives(pattern string, vars VariableValues) (string, bool) {
	alternatives := topLevelAlternatives(pattern)
	var kept []string
	for _, alt := range alternatives {
		if !referencesEmptyVariable(alt.text, vars) {
			kept = append(kept, alt.flagsBefore+alt.text)
		}
	}
	switch len(kept) {
	case len(alternatives):
		return pattern, true
	case 0:
		return "", false
	}
	return strings.Join(kept, "|"), true
}

// isOnlyEmptyVariables reports whether the template is nothing but references
// to variables with no selection.
func isOnlyEmptyVariables(template string, vars VariableValues) bool {
	refs := variableRefs(template)
	if len(refs) == 0 {
		return false
	}
	cursor := 0
	for _, ref := range refs {
		if values, ok := vars[ref.name]; !ok || len(values) != 0 || ref.start != cursor {
			return false
		}
		cursor = ref.end
	}
	return cursor == len(template)
}

// unescapePromQLString decodes the escapes of a double-quoted PromQL string
// body; an unrecognised escape is kept as written.
func unescapePromQLString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\', '"', '\'':
			b.WriteByte(s[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// substituteInMatcher renders one label matcher's value, promoting the
// operator when the values cannot be expressed as an exact match.
func substituteInMatcher(label, spaceBefore, operator, spaceAfter, template string, vars VariableValues) string {
	isRegexMatcher := operator == "=~" || operator == "!~"
	if operator == "!=" || operator == "!~" {
		// Excluding "everything selected" is excluding nothing, so a negation whose
		// whole value is an unselected variable flips to a match-all. A webapp
		// bug (`ns!~".*"` matches no series) is deliberately not ported.
		if isRegexMatcher {
			pattern, ok := withoutEmptyVariableAlternatives(template, vars)
			if !ok {
				return label + spaceBefore + "=~" + spaceAfter + `".*"`
			}
			template = pattern
		} else if isOnlyEmptyVariables(template, vars) {
			return label + spaceBefore + "=~" + spaceAfter + `".*"`
		}
	}

	resolved := resolvedValuesIn(template, vars)
	if len(resolved) == 0 {
		return label + spaceBefore + operator + spaceAfter + `"` + template + `"`
	}

	stays := !isRegexMatcher
	for _, values := range resolved {
		if len(values) != 1 {
			stays = false
		}
	}
	if stays {
		value := renderTemplate(template, vars, identity, func(values []string) string { return escapePromQLString(values[0]) })
		return label + spaceBefore + operator + spaceAfter + `"` + value + `"`
	}

	// In a regex matcher the literal text is already a regex; in an exact matcher
	// being promoted it is a plain string, so `ns="v1.2-$ns"` keeps its `.`.
	renderLiteral := identity
	if !isRegexMatcher {
		// The template is still PromQL-escaped source text: decode it to the
		// label value before regex-escaping, or `\"` would be escaped twice.
		renderLiteral = func(s string) string {
			return escapePromQLString(escapeRegexLiteral(unescapePromQLString(s)))
		}
	}
	body := renderTemplate(template, vars, renderLiteral, func(values []string) string {
		return escapePromQLString(renderRegexValues(values))
	})
	next := operator
	if !isRegexMatcher {
		next = "=~"
		if operator == "!=" {
			next = "!~"
		}
	}
	return label + spaceBefore + next + spaceAfter + `"` + body + `"`
}

// SubstituteVariablesInPromQL resolves "$variable" / "${variable}" references
// in a raw PromQL query. A reference inside a label matcher is rendered for
// that matcher (several values or none promote "=" to "=~"); one elsewhere,
// such as rate(up[$window]), is inserted raw. References the map does not
// define stay as written.
func SubstituteVariablesInPromQL(query string, variables VariableValues) string {
	if query == "" {
		return query
	}
	vars := normaliseVariables(variables)
	if len(vars) == 0 {
		return query
	}

	var out strings.Builder
	for i := 0; i < len(query); {
		c := query[i]
		var m []string
		if c == '$' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			m = substitutionScanRe.FindStringSubmatch(query[i:])
		}
		// The reference alternative never starts right after a "$": that is a
		// "$$name" field reference, not a value.
		if m != nil && m[1] == "" && i > 0 && query[i-1] == '$' {
			m = nil
		}
		if m == nil {
			out.WriteByte(c)
			i++
			continue
		}
		out.WriteString(substituteScanMatch(m, vars))
		i += len(m[0])
	}
	return out.String()
}

func substituteScanMatch(m []string, vars VariableValues) string {
	whole := m[0]
	if m[1] != "" {
		template := m[5]
		if !strings.Contains(template, "$") {
			return whole
		}
		return substituteInMatcher(m[1], m[2], m[3], m[4], template, vars)
	}
	// Outside a matcher: a duration, a label name, a scalar. There is no
	// matcher semantics to apply, so the value goes in as written.
	name := m[6]
	if name == "" {
		name = m[7]
	}
	values, ok := vars[name]
	if !ok {
		return whole
	}
	if len(values) == 0 {
		return ".*"
	}
	return strings.Join(values, "|")
}

// DesugarFieldReferencesInPromQL rewrites "$$pod" inside a selector {...} as
// pod="$pod", which SubstituteVariablesInPromQL then promotes to "=~" for
// several or no values. fields maps a variable's name to the label it lists;
// a reference with no entry is dropped.
func DesugarFieldReferencesInPromQL(query string, fields map[string]string) string {
	if !strings.Contains(query, "$$") {
		return query
	}
	var out strings.Builder
	depth := 0
	var quote byte
	rewritten := false
	for i := 0; i < len(query); i++ {
		c := query[i]
		if quote != 0 {
			out.WriteByte(c)
			if c == '\\' && quote != '`' {
				i++
				if i < len(query) {
					out.WriteByte(query[i])
				}
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch {
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '{':
			depth++
		case c == '}' && depth > 0:
			depth--
			if rewritten {
				trimmed := trailingCommaRe.ReplaceAllString(out.String(), "")
				out.Reset()
				out.WriteString(trimmed)
			}
			if depth == 0 {
				rewritten = false
			}
		case c == '$' && depth > 0:
			if m := fieldReferenceCursorRe.FindStringSubmatch(query[i:]); m != nil {
				if label, ok := fields[m[1]]; ok && label != "" {
					out.WriteString(label + `="$` + m[1] + `",`)
				}
				rewritten = true
				i += len(m[0]) - 1
				continue
			}
		}
		out.WriteByte(c)
	}
	return out.String()
}
