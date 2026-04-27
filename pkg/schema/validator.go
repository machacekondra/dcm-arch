package schema

import (
	"fmt"
	"strings"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

// ValidationError holds all validation errors found during schema validation.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("schema validation failed:\n  - %s", strings.Join(e.Errors, "\n  - "))
}

// ValidateApplication cross-validates an Application's resources against their
// registered ResourceType schemas. The types map is keyed by ResourceType metadata.name.
func ValidateApplication(app *v1alpha1.Application, types map[string]*v1alpha1.ResourceType) error {
	var errors []string

	for i, res := range app.Spec.Resources {
		prefix := fmt.Sprintf("spec.resources[%d](%s)", i, res.Name)

		rt, ok := types[res.Type]
		if !ok {
			errors = append(errors, fmt.Sprintf("%s: resource type %q is not registered", prefix, res.Type))
			continue
		}

		// Warn on deprecated
		if rt.Spec.Lifecycle == "deprecated" {
			msg := "deprecated"
			if rt.Spec.Deprecation != nil {
				msg = fmt.Sprintf("deprecated: %s (deadline: %s)", rt.Spec.Deprecation.Message, rt.Spec.Deprecation.Deadline)
			}
			errors = append(errors, fmt.Sprintf("%s: resource type %q is %s", prefix, res.Type, msg))
		}

		parsed, err := Parse(rt.Spec.Schema)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: failed to parse schema for %q: %v", prefix, res.Type, err))
			continue
		}

		errs := validateProperties(prefix, res.Properties, parsed)
		errors = append(errors, errs...)
	}

	if len(errors) == 0 {
		return nil
	}
	return &ValidationError{Errors: errors}
}

func validateProperties(prefix string, properties map[string]any, schema *ParsedSchema) []string {
	var errors []string
	inputs := schema.InputProperties()

	// Check that all provided properties exist and are input properties
	for name, value := range properties {
		prop, exists := schema.Properties[name]
		if !exists {
			errors = append(errors, fmt.Sprintf("%s.properties.%s: unknown property", prefix, name))
			continue
		}
		if prop.ReadOnly {
			errors = append(errors, fmt.Sprintf("%s.properties.%s: is a read-only output property, cannot be set by developer", prefix, name))
			continue
		}

		// Type check
		if err := checkType(name, value, prop); err != nil {
			errors = append(errors, fmt.Sprintf("%s.properties.%s: %s", prefix, name, err))
			continue
		}

		// Enum check
		if len(prop.Enum) > 0 {
			if !enumContains(prop.Enum, value) {
				errors = append(errors, fmt.Sprintf("%s.properties.%s: value %v is not in allowed values %v", prefix, name, value, prop.Enum))
			}
		}

		// Min/Max check (for numeric types)
		if numVal, ok := toFloat64(value); ok {
			if prop.Minimum != nil && numVal < *prop.Minimum {
				errors = append(errors, fmt.Sprintf("%s.properties.%s: value %v is less than minimum %v", prefix, name, value, *prop.Minimum))
			}
			if prop.Maximum != nil && numVal > *prop.Maximum {
				errors = append(errors, fmt.Sprintf("%s.properties.%s: value %v is greater than maximum %v", prefix, name, value, *prop.Maximum))
			}
		}
	}

	// Check required input fields are present
	for _, reqName := range schema.RequiredInputs() {
		if _, provided := properties[reqName]; !provided {
			// Check if there's a default
			if prop, ok := inputs[reqName]; ok && prop.Default != nil {
				continue
			}
			errors = append(errors, fmt.Sprintf("%s.properties: missing required property %q", prefix, reqName))
		}
	}

	return errors
}

func checkType(name string, value any, prop PropertySchema) error {
	if prop.Type == "" {
		return nil // no type constraint
	}

	// CEL expressions (${...}) are validated later in Phase 5, skip type checking here
	if s, ok := value.(string); ok && strings.Contains(s, "${") {
		return nil
	}

	switch prop.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "integer":
		switch value.(type) {
		case int, int64, float64:
			// float64 is how JSON numbers are decoded; accept if it's a whole number
			if f, ok := value.(float64); ok && f != float64(int64(f)) {
				return fmt.Errorf("expected integer, got float %v", value)
			}
		default:
			return fmt.Errorf("expected integer, got %T", value)
		}
	case "number":
		switch value.(type) {
		case int, int64, float64:
			// all numeric types are fine
		default:
			return fmt.Errorf("expected number, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	}

	return nil
}

func enumContains(enum []any, value any) bool {
	for _, e := range enum {
		if fmt.Sprintf("%v", e) == fmt.Sprintf("%v", value) {
			return true
		}
	}
	return false
}
