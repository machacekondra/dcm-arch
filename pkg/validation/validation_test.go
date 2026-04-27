package validation

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

// --- Application ---

func validApp() *v1alpha1.Application {
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: "my-app"}
	app.Spec = v1alpha1.ApplicationSpec{
		Resources: []v1alpha1.ResourceDecl{
			{Type: "database.postgresql", Name: "db", Properties: map[string]any{"size": "S"}},
			{Type: "compute.container", Name: "fe", Requirements: []string{"db"}},
		},
	}
	return app
}

func TestApplicationValid(t *testing.T) {
	r := ValidateApplication(validApp())
	if !r.OK() {
		t.Fatalf("expected valid, got: %v", r.Error())
	}
}

func TestApplicationMissingName(t *testing.T) {
	app := validApp()
	app.Metadata.Name = ""
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for missing name")
	}
}

func TestApplicationInvalidName(t *testing.T) {
	app := validApp()
	app.Metadata.Name = "My_Invalid_Name!"
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for invalid name")
	}
}

func TestApplicationEmptyResources(t *testing.T) {
	app := validApp()
	app.Spec.Resources = nil
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for empty resources")
	}
}

func TestApplicationDuplicateResourceName(t *testing.T) {
	app := validApp()
	app.Spec.Resources = []v1alpha1.ResourceDecl{
		{Type: "database.postgresql", Name: "db"},
		{Type: "cache.redis", Name: "db"},
	}
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for duplicate resource name")
	}
}

func TestApplicationMissingResourceType(t *testing.T) {
	app := validApp()
	app.Spec.Resources[0].Type = ""
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for missing resource type")
	}
}

func TestApplicationMissingResourceName(t *testing.T) {
	app := validApp()
	app.Spec.Resources[0].Name = ""
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for missing resource name")
	}
}

func TestApplicationUnknownRequirement(t *testing.T) {
	app := validApp()
	app.Spec.Resources[1].Requirements = []string{"nonexistent"}
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for unknown requirement")
	}
}

func TestApplicationSelfDependency(t *testing.T) {
	app := validApp()
	app.Spec.Resources[0].Requirements = []string{"db"}
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected error for self-dependency")
	}
}

// --- Environment ---

func validEnv() *v1alpha1.Environment {
	env := &v1alpha1.Environment{}
	env.APIVersion = v1alpha1.GroupVersion
	env.Kind = v1alpha1.KindEnvironment
	env.Metadata = meta.ObjectMeta{Name: "prod-eu"}
	env.Spec = v1alpha1.EnvironmentSpec{
		Type: "kubernetes",
		Connection: v1alpha1.ConnectionSpec{
			Endpoint:      "https://k8s.example.com:6443",
			CredentialRef: "vault:secret/k8s",
		},
		Capabilities: v1alpha1.CapabilitiesSpec{
			ResourceTypes: []string{"compute.container"},
		},
		Sovereignty: v1alpha1.SovereigntySpec{
			Country:            "DE",
			Region:             "eu-central-1",
			Jurisdiction:       "EU",
			DataClassification: "confidential",
		},
	}
	return env
}

func TestEnvironmentValid(t *testing.T) {
	r := ValidateEnvironment(validEnv())
	if !r.OK() {
		t.Fatalf("expected valid, got: %v", r.Error())
	}
}

func TestEnvironmentInvalidType(t *testing.T) {
	env := validEnv()
	env.Spec.Type = "mainframe"
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for invalid type")
	}
}

func TestEnvironmentMissingEndpoint(t *testing.T) {
	env := validEnv()
	env.Spec.Connection.Endpoint = ""
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestEnvironmentInvalidEndpoint(t *testing.T) {
	env := validEnv()
	env.Spec.Connection.Endpoint = "not-a-url"
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for invalid endpoint")
	}
}

func TestEnvironmentMissingCredentialRef(t *testing.T) {
	env := validEnv()
	env.Spec.Connection.CredentialRef = ""
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for missing credentialRef")
	}
}

func TestEnvironmentEmptyResourceTypes(t *testing.T) {
	env := validEnv()
	env.Spec.Capabilities.ResourceTypes = nil
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for empty resourceTypes")
	}
}

func TestEnvironmentInvalidCountryCode(t *testing.T) {
	env := validEnv()
	env.Spec.Sovereignty.Country = "DEU"
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for 3-letter country code")
	}
}

func TestEnvironmentInvalidDataClassification(t *testing.T) {
	env := validEnv()
	env.Spec.Sovereignty.DataClassification = "top-secret"
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for invalid data classification")
	}
}

func TestEnvironmentMissingSovereigntyFields(t *testing.T) {
	env := validEnv()
	env.Spec.Sovereignty = v1alpha1.SovereigntySpec{}
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected errors for missing sovereignty fields")
	}
	// Should have multiple errors
	if len(r.Messages()) < 3 {
		t.Errorf("expected at least 3 sovereignty errors, got %d", len(r.Messages()))
	}
}

func TestEnvironmentCostMissingCurrency(t *testing.T) {
	env := validEnv()
	env.Spec.Cost = &v1alpha1.CostSpec{Rates: v1alpha1.CostRates{}}
	r := ValidateEnvironment(env)
	if r.OK() {
		t.Fatal("expected error for cost without currency")
	}
}

// --- ResourceType ---

func validResourceType() *v1alpha1.ResourceType {
	rt := &v1alpha1.ResourceType{}
	rt.APIVersion = v1alpha1.GroupVersion
	rt.Kind = v1alpha1.KindResourceType
	rt.Metadata = meta.ObjectMeta{Name: "database.postgresql"}
	rt.Spec = v1alpha1.ResourceTypeSpec{
		Version:   "1.0.0",
		Lifecycle: "stable",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"size": map[string]any{"type": "string"}},
		},
	}
	return rt
}

func TestResourceTypeValid(t *testing.T) {
	r := ValidateResourceType(validResourceType())
	if !r.OK() {
		t.Fatalf("expected valid, got: %v", r.Error())
	}
}

func TestResourceTypeNameNoDot(t *testing.T) {
	rt := validResourceType()
	rt.Metadata.Name = "postgresql"
	r := ValidateResourceType(rt)
	if r.OK() {
		t.Fatal("expected error for name without dot notation")
	}
}

func TestResourceTypeInvalidVersion(t *testing.T) {
	rt := validResourceType()
	rt.Spec.Version = "v1"
	r := ValidateResourceType(rt)
	if r.OK() {
		t.Fatal("expected error for invalid semver")
	}
}

func TestResourceTypeInvalidLifecycle(t *testing.T) {
	rt := validResourceType()
	rt.Spec.Lifecycle = "beta"
	r := ValidateResourceType(rt)
	if r.OK() {
		t.Fatal("expected error for invalid lifecycle")
	}
}

func TestResourceTypeDeprecatedWithoutDeprecation(t *testing.T) {
	rt := validResourceType()
	rt.Spec.Lifecycle = "deprecated"
	r := ValidateResourceType(rt)
	if r.OK() {
		t.Fatal("expected error for deprecated without deprecation spec")
	}
}

func TestResourceTypeDeprecatedWithDeprecation(t *testing.T) {
	rt := validResourceType()
	rt.Spec.Lifecycle = "deprecated"
	rt.Spec.Deprecation = &v1alpha1.DeprecationSpec{
		Message:  "Use database.postgresql v2",
		Deadline: "2026-12-31",
	}
	r := ValidateResourceType(rt)
	if !r.OK() {
		t.Fatalf("expected valid, got: %v", r.Error())
	}
}

func TestResourceTypeMissingSchema(t *testing.T) {
	rt := validResourceType()
	rt.Spec.Schema = nil
	r := ValidateResourceType(rt)
	if r.OK() {
		t.Fatal("expected error for missing schema")
	}
}

func TestResourceTypeSchemaNotObject(t *testing.T) {
	rt := validResourceType()
	rt.Spec.Schema = map[string]any{"type": "array"}
	r := ValidateResourceType(rt)
	if r.OK() {
		t.Fatal("expected error for schema type != object")
	}
}

// --- Recipe ---

func validRecipe() *v1alpha1.Recipe {
	r := &v1alpha1.Recipe{}
	r.APIVersion = v1alpha1.GroupVersion
	r.Kind = v1alpha1.KindRecipe
	r.Metadata = meta.ObjectMeta{Name: "pg-terraform-aws"}
	r.Spec = v1alpha1.RecipeSpec{
		ResourceType: "database.postgresql",
		Type:         "terraform",
		Source:       map[string]string{"module": "dcm-modules/rds-postgresql/aws"},
	}
	return r
}

func TestRecipeValid(t *testing.T) {
	r := ValidateRecipe(validRecipe())
	if !r.OK() {
		t.Fatalf("expected valid, got: %v", r.Error())
	}
}

func TestRecipeMissingResourceType(t *testing.T) {
	recipe := validRecipe()
	recipe.Spec.ResourceType = ""
	r := ValidateRecipe(recipe)
	if r.OK() {
		t.Fatal("expected error for missing resourceType")
	}
}

func TestRecipeInvalidType(t *testing.T) {
	recipe := validRecipe()
	recipe.Spec.Type = "chef"
	r := ValidateRecipe(recipe)
	if r.OK() {
		t.Fatal("expected error for invalid recipe type")
	}
}

func TestRecipeMissingSource(t *testing.T) {
	recipe := validRecipe()
	recipe.Spec.Source = nil
	r := ValidateRecipe(recipe)
	if r.OK() {
		t.Fatal("expected error for missing source")
	}
}

// --- PlacementPolicy ---

func validPlacementPolicy() *v1alpha1.PlacementPolicy {
	pp := &v1alpha1.PlacementPolicy{}
	pp.APIVersion = v1alpha1.GroupVersion
	pp.Kind = v1alpha1.KindPlacementPolicy
	pp.Metadata = meta.ObjectMeta{Name: "default"}
	pp.Spec = v1alpha1.PlacementPolicySpec{
		Match:  v1alpha1.MatchCriteria{All: true},
		Rule:   `env.status.staleness == "fresh"`,
		Weight: 1.0,
	}
	return pp
}

func TestPlacementPolicyValid(t *testing.T) {
	r := ValidatePlacementPolicy(validPlacementPolicy())
	if !r.OK() {
		t.Fatalf("expected valid, got: %v", r.Error())
	}
}

func TestPlacementPolicyNoMatchCriteria(t *testing.T) {
	pp := validPlacementPolicy()
	pp.Spec.Match = v1alpha1.MatchCriteria{}
	r := ValidatePlacementPolicy(pp)
	if r.OK() {
		t.Fatal("expected error for no match criteria")
	}
}

func TestPlacementPolicyMatchByLabels(t *testing.T) {
	pp := validPlacementPolicy()
	pp.Spec.Match = v1alpha1.MatchCriteria{Labels: map[string]string{"tier": "production"}}
	r := ValidatePlacementPolicy(pp)
	if !r.OK() {
		t.Fatalf("expected valid with labels match, got: %v", r.Error())
	}
}

func TestPlacementPolicyMatchByResourceTypes(t *testing.T) {
	pp := validPlacementPolicy()
	pp.Spec.Match = v1alpha1.MatchCriteria{ResourceTypes: []string{"database.postgresql"}}
	r := ValidatePlacementPolicy(pp)
	if !r.OK() {
		t.Fatalf("expected valid with resourceTypes match, got: %v", r.Error())
	}
}

func TestPlacementPolicyNegativeWeight(t *testing.T) {
	pp := validPlacementPolicy()
	pp.Spec.Weight = -1.0
	r := ValidatePlacementPolicy(pp)
	if r.OK() {
		t.Fatal("expected error for negative weight")
	}
}

// --- Multi-error accumulation ---

func TestMultipleErrors(t *testing.T) {
	app := &v1alpha1.Application{}
	app.Metadata.Name = "My_Bad_Name!"
	// Missing resources too
	r := ValidateApplication(app)
	if r.OK() {
		t.Fatal("expected errors")
	}
	if len(r.Messages()) < 2 {
		t.Errorf("expected at least 2 errors, got %d: %v", len(r.Messages()), r.Messages())
	}
}
