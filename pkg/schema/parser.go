package schema

import "fmt"

// ParsedSchema is a structured representation of a ResourceType's OpenAPI v3 schema.
type ParsedSchema struct {
	Properties map[string]PropertySchema
	Required   []string
}

// PropertySchema describes a single property in a ResourceType schema.
type PropertySchema struct {
	Name        string
	Type        string // string, integer, number, boolean, array, object
	Description string
	ReadOnly    bool
	Sensitive   bool // x-dcm-sensitive
	Enum        []any
	Default     any
	Minimum     *float64
	Maximum     *float64
	Pattern     string
}

// InputProperties returns only the developer-settable properties (readOnly == false).
func (s *ParsedSchema) InputProperties() map[string]PropertySchema {
	result := make(map[string]PropertySchema)
	for name, prop := range s.Properties {
		if !prop.ReadOnly {
			result[name] = prop
		}
	}
	return result
}

// OutputProperties returns only the recipe-provided properties (readOnly == true).
func (s *ParsedSchema) OutputProperties() map[string]PropertySchema {
	result := make(map[string]PropertySchema)
	for name, prop := range s.Properties {
		if prop.ReadOnly {
			result[name] = prop
		}
	}
	return result
}

// RequiredInputs returns the subset of required fields that are input properties.
func (s *ParsedSchema) RequiredInputs() []string {
	inputs := s.InputProperties()
	var result []string
	for _, name := range s.Required {
		if _, ok := inputs[name]; ok {
			result = append(result, name)
		}
	}
	return result
}

// Parse converts a raw ResourceType spec.schema (map[string]any) into a ParsedSchema.
func Parse(raw map[string]any) (*ParsedSchema, error) {
	schemaType, _ := raw["type"].(string)
	if schemaType != "object" {
		return nil, fmt.Errorf("schema type must be \"object\", got %q", schemaType)
	}

	propsRaw, ok := raw["properties"]
	if !ok {
		return nil, fmt.Errorf("schema must have \"properties\"")
	}
	propsMap, ok := propsRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema.properties must be an object")
	}

	parsed := &ParsedSchema{
		Properties: make(map[string]PropertySchema),
	}

	// Parse required field
	if reqRaw, ok := raw["required"]; ok {
		if reqList, ok := reqRaw.([]any); ok {
			for _, r := range reqList {
				if s, ok := r.(string); ok {
					parsed.Required = append(parsed.Required, s)
				}
			}
		}
	}

	// Parse each property
	for name, propRaw := range propsMap {
		propMap, ok := propRaw.(map[string]any)
		if !ok {
			continue
		}

		prop := PropertySchema{Name: name}
		prop.Type, _ = propMap["type"].(string)
		prop.Description, _ = propMap["description"].(string)
		prop.ReadOnly, _ = propMap["readOnly"].(bool)
		prop.Sensitive, _ = propMap["x-dcm-sensitive"].(bool)
		prop.Default = propMap["default"]
		prop.Pattern, _ = propMap["pattern"].(string)

		if enumRaw, ok := propMap["enum"]; ok {
			if enumList, ok := enumRaw.([]any); ok {
				prop.Enum = enumList
			}
		}

		if minRaw, ok := propMap["minimum"]; ok {
			if v, ok := toFloat64(minRaw); ok {
				prop.Minimum = &v
			}
		}
		if maxRaw, ok := propMap["maximum"]; ok {
			if v, ok := toFloat64(maxRaw); ok {
				prop.Maximum = &v
			}
		}

		parsed.Properties[name] = prop
	}

	return parsed, nil
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
