package schema

import (
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

func postgresSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []any{"size"},
		"properties": map[string]any{
			"size": map[string]any{
				"type":    "string",
				"enum":    []any{"XS", "S", "M", "L", "XL"},
				"default": "S",
			},
			"version": map[string]any{
				"type":    "string",
				"enum":    []any{"14", "15", "16", "17"},
				"default": "16",
			},
			"storageGB": map[string]any{
				"type":    "integer",
				"minimum": float64(10),
				"maximum": float64(10000),
				"default": float64(50),
			},
			"multiAZ": map[string]any{
				"type":    "boolean",
				"default": false,
			},
			"host": map[string]any{
				"type":     "string",
				"readOnly": true,
			},
			"port": map[string]any{
				"type":     "integer",
				"readOnly": true,
			},
			"connectionString": map[string]any{
				"type":     "string",
				"readOnly": true,
			},
			"username": map[string]any{
				"type":           "string",
				"readOnly":       true,
				"x-dcm-sensitive": true,
			},
			"password": map[string]any{
				"type":           "string",
				"readOnly":       true,
				"x-dcm-sensitive": true,
			},
		},
	}
}

func makeRT(name string, schema map[string]any) *v1alpha1.ResourceType {
	rt := &v1alpha1.ResourceType{}
	rt.APIVersion = v1alpha1.GroupVersion
	rt.Kind = v1alpha1.KindResourceType
	rt.Metadata = meta.ObjectMeta{Name: name}
	rt.Spec = v1alpha1.ResourceTypeSpec{
		Version:   "1.0.0",
		Lifecycle: "stable",
		Schema:    schema,
	}
	return rt
}

func makeApp(resources []v1alpha1.ResourceDecl) *v1alpha1.Application {
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: "test-app"}
	app.Spec = v1alpha1.ApplicationSpec{Resources: resources}
	return app
}

// --- Parser tests ---

func TestParseSchema(t *testing.T) {
	parsed, err := Parse(postgresSchema())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(parsed.Properties) != 9 {
		t.Errorf("properties: got %d, want 9", len(parsed.Properties))
	}

	inputs := parsed.InputProperties()
	if len(inputs) != 4 {
		t.Errorf("input properties: got %d, want 4 (size, version, storageGB, multiAZ)", len(inputs))
	}

	outputs := parsed.OutputProperties()
	if len(outputs) != 5 {
		t.Errorf("output properties: got %d, want 5", len(outputs))
	}

	// Check sensitive
	if !outputs["username"].Sensitive {
		t.Error("username should be sensitive")
	}
	if !outputs["password"].Sensitive {
		t.Error("password should be sensitive")
	}
	if outputs["host"].Sensitive {
		t.Error("host should not be sensitive")
	}
}

func TestParseRequired(t *testing.T) {
	parsed, err := Parse(postgresSchema())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	reqInputs := parsed.RequiredInputs()
	if len(reqInputs) != 1 || reqInputs[0] != "size" {
		t.Errorf("required inputs: got %v, want [size]", reqInputs)
	}
}

func TestParseMinMax(t *testing.T) {
	parsed, _ := Parse(postgresSchema())
	storage := parsed.Properties["storageGB"]
	if storage.Minimum == nil || *storage.Minimum != 10 {
		t.Errorf("storageGB minimum: got %v, want 10", storage.Minimum)
	}
	if storage.Maximum == nil || *storage.Maximum != 10000 {
		t.Errorf("storageGB maximum: got %v, want 10000", storage.Maximum)
	}
}

func TestParseEnum(t *testing.T) {
	parsed, _ := Parse(postgresSchema())
	size := parsed.Properties["size"]
	if len(size.Enum) != 5 {
		t.Errorf("size enum: got %d values, want 5", len(size.Enum))
	}
}

func TestParseInvalidSchema(t *testing.T) {
	_, err := Parse(map[string]any{"type": "array"})
	if err == nil {
		t.Fatal("expected error for non-object schema")
	}
}

// --- Validator tests ---

func TestValidApplicationProperties(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type: "database.postgresql",
			Name: "db",
			Properties: map[string]any{
				"size":      "M",
				"storageGB": float64(100),
				"multiAZ":   true,
			},
		},
	})

	err := ValidateApplication(app, types)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestUnknownResourceType(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{}
	app := makeApp([]v1alpha1.ResourceDecl{
		{Type: "cache.redis", Name: "cache", Properties: map[string]any{}},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for unknown resource type")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error should mention 'not registered': %v", err)
	}
}

func TestUnknownProperty(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": "S", "nonexistent": "value"},
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for unknown property")
	}
	if !strings.Contains(err.Error(), "unknown property") {
		t.Errorf("error should mention 'unknown property': %v", err)
	}
}

func TestReadOnlyPropertySet(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": "S", "host": "myhost.example.com"},
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for setting readOnly property")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error should mention 'read-only': %v", err)
	}
}

func TestWrongType(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": 123}, // should be string
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	if !strings.Contains(err.Error(), "expected string") {
		t.Errorf("error should mention type mismatch: %v", err)
	}
}

func TestEnumViolation(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": "XXL"},
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for enum violation")
	}
	if !strings.Contains(err.Error(), "not in allowed values") {
		t.Errorf("error should mention enum: %v", err)
	}
}

func TestMinimumViolation(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": "S", "storageGB": float64(5)},
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for minimum violation")
	}
	if !strings.Contains(err.Error(), "less than minimum") {
		t.Errorf("error should mention minimum: %v", err)
	}
}

func TestMaximumViolation(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": "S", "storageGB": float64(99999)},
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for maximum violation")
	}
	if !strings.Contains(err.Error(), "greater than maximum") {
		t.Errorf("error should mention maximum: %v", err)
	}
}

func TestMissingRequiredProperty(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	// "size" is required but has a default, so it should pass
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"storageGB": float64(50)},
		},
	})

	err := ValidateApplication(app, types)
	if err != nil {
		t.Fatalf("size has a default, should pass: %v", err)
	}
}

func TestMissingRequiredNoDefault(t *testing.T) {
	// Create a schema where "size" is required and has NO default
	schemaNoDefault := map[string]any{
		"type":     "object",
		"required": []any{"size"},
		"properties": map[string]any{
			"size": map[string]any{
				"type": "string",
				"enum": []any{"S", "M", "L"},
			},
		},
	}
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", schemaNoDefault),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{}, // size not provided, no default
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected error for missing required property without default")
	}
	if !strings.Contains(err.Error(), "missing required") {
		t.Errorf("error should mention 'missing required': %v", err)
	}
}

func TestCELExpressionSkipsTypeCheck(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type: "database.postgresql",
			Name: "db",
			Properties: map[string]any{
				"size":    "S",
				"multiAZ": "${params.enableHA}", // CEL expression in boolean field
			},
		},
	})

	err := ValidateApplication(app, types)
	if err != nil {
		t.Fatalf("CEL expression should skip type check: %v", err)
	}
}

func TestDeprecatedResourceType(t *testing.T) {
	rt := makeRT("database.postgresql", postgresSchema())
	rt.Spec.Lifecycle = "deprecated"
	rt.Spec.Deprecation = &v1alpha1.DeprecationSpec{
		Message:  "Use database.postgresql v2",
		Deadline: "2026-12-31",
	}
	types := map[string]*v1alpha1.ResourceType{"database.postgresql": rt}
	app := makeApp([]v1alpha1.ResourceDecl{
		{Type: "database.postgresql", Name: "db", Properties: map[string]any{"size": "S"}},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected warning for deprecated resource type")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("error should mention 'deprecated': %v", err)
	}
}

func TestMultipleResources(t *testing.T) {
	containerSchema := map[string]any{
		"type":     "object",
		"required": []any{"image"},
		"properties": map[string]any{
			"image": map[string]any{"type": "string"},
			"ports": map[string]any{"type": "array"},
		},
	}
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
		"compute.container":   makeRT("compute.container", containerSchema),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type:       "database.postgresql",
			Name:       "db",
			Properties: map[string]any{"size": "M"},
		},
		{
			Type:       "compute.container",
			Name:       "fe",
			Properties: map[string]any{"image": "quay.io/example/app"},
		},
	})

	err := ValidateApplication(app, types)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestMultipleErrors(t *testing.T) {
	types := map[string]*v1alpha1.ResourceType{
		"database.postgresql": makeRT("database.postgresql", postgresSchema()),
	}
	app := makeApp([]v1alpha1.ResourceDecl{
		{
			Type: "database.postgresql",
			Name: "db",
			Properties: map[string]any{
				"size":      123,            // wrong type
				"storageGB": float64(5),     // below minimum
				"host":      "should.fail",  // readOnly
				"unknown":   "prop",         // unknown
			},
		},
	})

	err := ValidateApplication(app, types)
	if err == nil {
		t.Fatal("expected multiple errors")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(ve.Errors), ve.Errors)
	}
}
