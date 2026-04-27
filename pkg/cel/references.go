package cel

import "strings"

// ResourceReference represents a cross-resource reference found in a CEL expression.
type ResourceReference struct {
	ResourceName string // The referenced resource name (e.g., "db")
	Field        string // The referenced field (e.g., "host")
	Expression   Expression
}

// ExtractReferences extracts cross-resource references from CEL expressions.
// A reference has the form "resourceName.fieldName" (e.g., "db.host").
// References to "params" or other known prefixes are excluded.
func ExtractReferences(expressions []Expression) []ResourceReference {
	var refs []ResourceReference

	for _, expr := range expressions {
		// Parse simple "resource.field" patterns from the CEL expression
		// This handles both standalone "${db.host}" and interpolated
		// "postgres://${db.host}:${db.port}/mydb"
		identifiers := extractIdentifiers(expr.CEL)
		for _, id := range identifiers {
			parts := strings.SplitN(id, ".", 2)
			if len(parts) != 2 {
				continue
			}
			resourceName := parts[0]
			field := parts[1]

			// Skip known non-resource prefixes
			if resourceName == "params" || resourceName == "env" {
				continue
			}

			refs = append(refs, ResourceReference{
				ResourceName: resourceName,
				Field:        field,
				Expression:   expr,
			})
		}
	}

	return refs
}

// ExtractReferencedResourceNames returns the unique set of resource names
// referenced by the given expressions.
func ExtractReferencedResourceNames(expressions []Expression) []string {
	refs := ExtractReferences(expressions)
	seen := make(map[string]bool)
	var names []string
	for _, ref := range refs {
		if !seen[ref.ResourceName] {
			seen[ref.ResourceName] = true
			names = append(names, ref.ResourceName)
		}
	}
	return names
}

// extractIdentifiers pulls out dotted identifier patterns from a CEL expression.
// It matches patterns like "db.host", "cache.port", etc.
func extractIdentifiers(cel string) []string {
	var results []string

	// Simple state machine to extract word.word patterns
	i := 0
	for i < len(cel) {
		// Find the start of an identifier
		if isIdentStart(cel[i]) {
			start := i
			for i < len(cel) && (isIdentChar(cel[i]) || cel[i] == '.') {
				i++
			}
			token := cel[start:i]
			// Only include if it contains a dot (resource.field)
			if strings.Contains(token, ".") {
				results = append(results, token)
			}
		} else {
			i++
		}
	}

	return results
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentChar(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}
