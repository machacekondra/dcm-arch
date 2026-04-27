package cel

import (
	"fmt"
	"regexp"
	"strings"
)

// Expression represents a single ${...} expression found in a property value.
type Expression struct {
	Raw  string // The full ${...} string including delimiters
	CEL  string // The inner expression without ${ and }
	Path string // Where in the properties tree this was found (e.g., "dbUrl")
}

var exprPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ExtractExpressions finds all ${...} patterns in an arbitrary value.
// It recursively walks strings, maps, and slices.
func ExtractExpressions(path string, value any) []Expression {
	var results []Expression

	switch v := value.(type) {
	case string:
		matches := exprPattern.FindAllStringSubmatch(v, -1)
		for _, match := range matches {
			results = append(results, Expression{
				Raw:  match[0],
				CEL:  strings.TrimSpace(match[1]),
				Path: path,
			})
		}
	case map[string]any:
		for key, val := range v {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			results = append(results, ExtractExpressions(childPath, val)...)
		}
	case []any:
		for i, val := range v {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			results = append(results, ExtractExpressions(childPath, val)...)
		}
	}

	return results
}

// ExtractAllExpressions extracts expressions from a resource's properties map.
func ExtractAllExpressions(properties map[string]any) []Expression {
	return ExtractExpressions("", properties)
}

// ContainsCEL returns true if the value contains any ${...} expression.
func ContainsCEL(value any) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	return exprPattern.MatchString(s)
}
