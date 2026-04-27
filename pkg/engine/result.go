package engine

import (
	"fmt"
	"strings"

	"github.com/dcm-io/dcm/pkg/schema"
)

// ValidateResult checks that a Recipe result matches the ResourceType's
// readOnly properties. Every readOnly property must appear in either
// values or secrets. Sensitive properties must be in secrets.
func ValidateResult(result *Result, parsed *schema.ParsedSchema) error {
	var errors []string
	outputs := parsed.OutputProperties()

	for name, prop := range outputs {
		inValues := result.Values != nil && result.Values[name] != nil
		inSecrets := result.Secrets != nil && result.Secrets[name] != nil

		if !inValues && !inSecrets {
			errors = append(errors, fmt.Sprintf("missing output %q", name))
			continue
		}

		if prop.Sensitive && inValues && !inSecrets {
			errors = append(errors, fmt.Sprintf("sensitive output %q must be in secrets, not values", name))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("result validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}
	return nil
}
