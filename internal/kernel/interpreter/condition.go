package interpreter

import (
	"fmt"
	"strconv"
	"strings"
)

// Fact is a service's emitted value — a decoded JSON object the interpreter
// evaluates branch and successor conditions against.
type Fact map[string]any

// evalCondition evaluates a simple condition of the form `<key> == <literal>` or
// `<key> != <literal>` against a fact. Literals are true/false, a quoted string,
// a number, or a bare word (treated as a string). A missing key compares unequal.
func evalCondition(cond string, fact Fact) (bool, error) {
	op := "=="
	if strings.Contains(cond, "!=") {
		op = "!="
	} else if !strings.Contains(cond, "==") {
		return false, fmt.Errorf("interpreter: unsupported condition %q", cond)
	}

	parts := strings.SplitN(cond, op, 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("interpreter: malformed condition %q", cond)
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return false, fmt.Errorf("interpreter: condition %q has no key", cond)
	}
	expected := parseLiteral(strings.TrimSpace(parts[1]))

	eq := valuesEqual(fact[key], expected)
	if op == "!=" {
		return !eq, nil
	}
	return eq, nil
}

func parseLiteral(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

// valuesEqual compares a fact value (as decoded from JSON: bool, float64, string,
// or nil) against a parsed literal.
func valuesEqual(actual, expected any) bool {
	switch a := actual.(type) {
	case bool:
		b, ok := expected.(bool)
		return ok && a == b
	case float64:
		b, ok := expected.(float64)
		return ok && a == b
	case string:
		b, ok := expected.(string)
		return ok && a == b
	case nil:
		return expected == nil
	default:
		return false
	}
}
