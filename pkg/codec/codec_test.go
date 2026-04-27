package codec

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

func TestRoundTripApplication(t *testing.T) {
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata.Name = "test-app"
	app.Spec.Resources = []v1alpha1.ResourceDecl{
		{
			Type: "database.postgresql",
			Name: "my-db",
			Properties: map[string]any{
				"size": "S",
			},
		},
		{
			Type: "compute.container",
			Name: "my-fe",
			Properties: map[string]any{
				"image": "quay.io/example/frontend",
			},
			Requirements: []string{"my-db"},
		},
	}

	data, err := Encode(app)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	obj, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	decoded, ok := obj.(*v1alpha1.Application)
	if !ok {
		t.Fatalf("expected *Application, got %T", obj)
	}

	if decoded.Metadata.Name != "test-app" {
		t.Errorf("name: got %q, want %q", decoded.Metadata.Name, "test-app")
	}
	if len(decoded.Spec.Resources) != 2 {
		t.Errorf("resources: got %d, want 2", len(decoded.Spec.Resources))
	}
	if decoded.Spec.Resources[0].Type != "database.postgresql" {
		t.Errorf("resource[0].type: got %q, want %q", decoded.Spec.Resources[0].Type, "database.postgresql")
	}
}

func TestRoundTripEnvironment(t *testing.T) {
	env := &v1alpha1.Environment{}
	env.APIVersion = v1alpha1.GroupVersion
	env.Kind = v1alpha1.KindEnvironment
	env.Metadata.Name = "prod-eu-k8s-01"
	env.Metadata.Labels = map[string]string{"tier": "production"}
	env.Spec = v1alpha1.EnvironmentSpec{
		Type: "kubernetes",
		Connection: v1alpha1.ConnectionSpec{
			Endpoint:      "https://k8s-prod-eu.example.com:6443",
			CredentialRef: "vault:secret/dcm/envs/prod-eu",
		},
		Capabilities: v1alpha1.CapabilitiesSpec{
			ResourceTypes: []string{"compute.container", "database.postgresql"},
			Features:      []string{"gpu", "ssd-storage"},
		},
		Sovereignty: v1alpha1.SovereigntySpec{
			Country:            "DE",
			Region:             "eu-central-1",
			Jurisdiction:       "EU",
			Compliance:         []string{"GDPR", "SOC2"},
			DataClassification: "confidential",
		},
	}

	data, err := Encode(env)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	obj, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	decoded, ok := obj.(*v1alpha1.Environment)
	if !ok {
		t.Fatalf("expected *Environment, got %T", obj)
	}

	if decoded.Metadata.Name != "prod-eu-k8s-01" {
		t.Errorf("name: got %q, want %q", decoded.Metadata.Name, "prod-eu-k8s-01")
	}
	if decoded.Spec.Type != "kubernetes" {
		t.Errorf("type: got %q, want %q", decoded.Spec.Type, "kubernetes")
	}
	if len(decoded.Spec.Capabilities.ResourceTypes) != 2 {
		t.Errorf("resourceTypes: got %d, want 2", len(decoded.Spec.Capabilities.ResourceTypes))
	}
}

func TestRoundTripResourceType(t *testing.T) {
	rt := &v1alpha1.ResourceType{}
	rt.APIVersion = v1alpha1.GroupVersion
	rt.Kind = v1alpha1.KindResourceType
	rt.Metadata.Name = "database.postgresql"
	rt.Spec = v1alpha1.ResourceTypeSpec{
		Version:   "1.0.0",
		Lifecycle: "stable",
		Schema: map[string]any{
			"type":     "object",
			"required": []any{"size"},
			"properties": map[string]any{
				"size": map[string]any{
					"type": "string",
					"enum": []any{"XS", "S", "M", "L", "XL"},
				},
			},
		},
	}

	data, err := Encode(rt)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	obj, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	decoded, ok := obj.(*v1alpha1.ResourceType)
	if !ok {
		t.Fatalf("expected *ResourceType, got %T", obj)
	}

	if decoded.Spec.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", decoded.Spec.Version, "1.0.0")
	}
	if decoded.Spec.Lifecycle != "stable" {
		t.Errorf("lifecycle: got %q, want %q", decoded.Spec.Lifecycle, "stable")
	}
}

func TestRoundTripPlacementPolicy(t *testing.T) {
	pp := &v1alpha1.PlacementPolicy{}
	pp.APIVersion = v1alpha1.GroupVersion
	pp.Kind = v1alpha1.KindPlacementPolicy
	pp.Metadata.Name = "gdpr-compliance"
	pp.Spec = v1alpha1.PlacementPolicySpec{
		Match: v1alpha1.MatchCriteria{
			Labels: map[string]string{"compliance": "gdpr"},
		},
		Rule:     `env.sovereignty.jurisdiction == "EU"`,
		Prefer:   "env.capacity.cpu.available",
		Weight:   1.0,
		Priority: 100,
	}

	data, err := Encode(pp)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	obj, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	decoded, ok := obj.(*v1alpha1.PlacementPolicy)
	if !ok {
		t.Fatalf("expected *PlacementPolicy, got %T", obj)
	}

	if decoded.Spec.Rule != `env.sovereignty.jurisdiction == "EU"` {
		t.Errorf("rule: got %q", decoded.Spec.Rule)
	}
	if decoded.Spec.Weight != 1.0 {
		t.Errorf("weight: got %f, want 1.0", decoded.Spec.Weight)
	}
}

func TestRoundTripRecipe(t *testing.T) {
	r := &v1alpha1.Recipe{}
	r.APIVersion = v1alpha1.GroupVersion
	r.Kind = v1alpha1.KindRecipe
	r.Metadata.Name = "database-postgresql-terraform-aws"
	r.Spec = v1alpha1.RecipeSpec{
		ResourceType:        "database.postgresql",
		ResourceTypeVersion: ">=1.0.0, <2.0.0",
		Type:                "terraform",
		Source: map[string]string{
			"registry": "registry.terraform.io",
			"module":   "dcm-modules/rds-postgresql/aws",
			"version":  "3.2.1",
		},
		EnvironmentMatch: &v1alpha1.EnvMatch{
			Types: []string{"aws"},
		},
		Parameters: map[string]any{
			"backup_retention_period": 7,
			"storage_encrypted":      true,
		},
	}

	data, err := Encode(r)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	obj, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	decoded, ok := obj.(*v1alpha1.Recipe)
	if !ok {
		t.Fatalf("expected *Recipe, got %T", obj)
	}

	if decoded.Spec.ResourceType != "database.postgresql" {
		t.Errorf("resourceType: got %q, want %q", decoded.Spec.ResourceType, "database.postgresql")
	}
	if decoded.Spec.Type != "terraform" {
		t.Errorf("type: got %q, want %q", decoded.Spec.Type, "terraform")
	}
}

func TestDecodeUnknownType(t *testing.T) {
	data := []byte(`apiVersion: dcm.io/v1alpha1
kind: Unknown
metadata:
  name: test
`)
	_, err := Decode(data)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestDecodeMissingKind(t *testing.T) {
	data := []byte(`apiVersion: dcm.io/v1alpha1
metadata:
  name: test
`)
	_, err := Decode(data)
	if err == nil {
		t.Fatal("expected error for missing kind")
	}
}
