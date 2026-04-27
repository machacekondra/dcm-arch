package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/store"
	kinestore "github.com/dcm-io/dcm/pkg/store/kine"
)

func setupStore(t *testing.T) store.Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "dcm-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s, err := kinestore.New(context.Background(), kinestore.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newApp(name string) *v1alpha1.Application {
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: name}
	app.Spec = v1alpha1.ApplicationSpec{
		Resources: []v1alpha1.ResourceDecl{
			{Type: "database.postgresql", Name: "db", Properties: map[string]any{"size": "S"}},
		},
	}
	return app
}

func TestApplicationCRUD(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	// Create
	rev, err := repo.Create(ctx, newApp("test-app"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rev == 0 {
		t.Fatal("expected non-zero revision")
	}

	// Get
	app, gotRev, err := repo.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if app == nil {
		t.Fatal("expected object, got nil")
	}
	if app.Metadata.Name != "test-app" {
		t.Errorf("name: got %q, want %q", app.Metadata.Name, "test-app")
	}
	if gotRev == 0 {
		t.Fatal("expected non-zero revision from Get")
	}

	// Update
	app.Spec.Resources[0].Properties["size"] = "M"
	newRev, err := repo.Update(ctx, app, gotRev)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if newRev == 0 {
		t.Fatal("expected non-zero revision from Update")
	}

	// Verify update
	updated, _, err := repo.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if updated.Spec.Resources[0].Properties["size"] != "M" {
		t.Errorf("size after update: got %v, want M", updated.Spec.Resources[0].Properties["size"])
	}

	// Delete
	err = repo.Delete(ctx, "test-app", newRev)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify delete
	deleted, _, err := repo.Get(ctx, "test-app")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if deleted != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestApplicationCreateDuplicate(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	_, err := repo.Create(ctx, newApp("dup"))
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = repo.Create(ctx, newApp("dup"))
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestApplicationUpdateConflict(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	_, err := repo.Create(ctx, newApp("conflict"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	app, _, err := repo.Get(ctx, "conflict")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_, err = repo.Update(ctx, app, 9999)
	if err == nil {
		t.Fatal("expected error on stale revision")
	}
}

func TestApplicationGetNotFound(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	app, rev, err := repo.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if app != nil {
		t.Fatalf("expected nil, got %+v", app)
	}
	if rev != 0 {
		t.Fatalf("expected revision 0, got %d", rev)
	}
}

func TestApplicationList(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	for _, name := range []string{"app-a", "app-b", "app-c"} {
		if _, err := repo.Create(ctx, newApp(name)); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	apps, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 3 {
		t.Errorf("List: got %d, want 3", len(apps))
	}
}

func TestApplicationCreateMissingName(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	// No metadata.name set

	_, err := repo.Create(ctx, app)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestEnvironmentCRUD(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Environment](s, EnvironmentKey, EnvironmentPrefix())

	env := &v1alpha1.Environment{}
	env.APIVersion = v1alpha1.GroupVersion
	env.Kind = v1alpha1.KindEnvironment
	env.Metadata = meta.ObjectMeta{Name: "prod-eu", Labels: map[string]string{"tier": "production"}}
	env.Spec = v1alpha1.EnvironmentSpec{
		Type: "kubernetes",
		Connection: v1alpha1.ConnectionSpec{
			Endpoint:      "https://k8s.example.com:6443",
			CredentialRef: "vault:secret/k8s",
		},
		Capabilities: v1alpha1.CapabilitiesSpec{
			ResourceTypes: []string{"compute.container", "database.postgresql"},
		},
		Sovereignty: v1alpha1.SovereigntySpec{
			Country: "DE", Region: "eu-central-1", Jurisdiction: "EU", DataClassification: "confidential",
		},
	}

	rev, err := repo.Create(ctx, env)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, gotRev, err := repo.Get(ctx, "prod-eu")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Spec.Type != "kubernetes" {
		t.Errorf("type: got %q, want %q", got.Spec.Type, "kubernetes")
	}
	if got.Metadata.Labels["tier"] != "production" {
		t.Errorf("label tier: got %q, want %q", got.Metadata.Labels["tier"], "production")
	}

	err = repo.Delete(ctx, "prod-eu", gotRev)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_ = rev
}

func TestResourceTypeCRUD(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.ResourceType](s, ResourceTypeKey, ResourceTypePrefix())

	rt := &v1alpha1.ResourceType{}
	rt.APIVersion = v1alpha1.GroupVersion
	rt.Kind = v1alpha1.KindResourceType
	rt.Metadata = meta.ObjectMeta{Name: "database.postgresql"}
	rt.Spec = v1alpha1.ResourceTypeSpec{
		Version:   "1.0.0",
		Lifecycle: "stable",
		Schema:    map[string]any{"type": "object", "properties": map[string]any{}},
	}

	_, err := repo.Create(ctx, rt)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _, err := repo.Get(ctx, "database.postgresql")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Spec.Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", got.Spec.Version, "1.0.0")
	}
}

func TestPlacementPolicyCRUD(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.PlacementPolicy](s, PlacementPolicyKey, PlacementPolicyPrefix())

	pp := &v1alpha1.PlacementPolicy{}
	pp.APIVersion = v1alpha1.GroupVersion
	pp.Kind = v1alpha1.KindPlacementPolicy
	pp.Metadata = meta.ObjectMeta{Name: "gdpr"}
	pp.Spec = v1alpha1.PlacementPolicySpec{
		Match:  v1alpha1.MatchCriteria{All: true},
		Rule:   `env.sovereignty.jurisdiction == "EU"`,
		Weight: 1.0,
	}

	_, err := repo.Create(ctx, pp)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _, err := repo.Get(ctx, "gdpr")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Spec.Rule != `env.sovereignty.jurisdiction == "EU"` {
		t.Errorf("rule: got %q", got.Spec.Rule)
	}
}

func TestRecipeCRUD(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Recipe](s, RecipeKey, RecipePrefix())

	r := &v1alpha1.Recipe{}
	r.APIVersion = v1alpha1.GroupVersion
	r.Kind = v1alpha1.KindRecipe
	r.Metadata = meta.ObjectMeta{Name: "pg-terraform-aws"}
	r.Spec = v1alpha1.RecipeSpec{
		ResourceType: "database.postgresql",
		Type:         "terraform",
		Source:       map[string]string{"module": "dcm-modules/rds-postgresql/aws"},
	}

	_, err := repo.Create(ctx, r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _, err := repo.Get(ctx, "pg-terraform-aws")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Spec.ResourceType != "database.postgresql" {
		t.Errorf("resourceType: got %q", got.Spec.ResourceType)
	}
}

func TestApplicationWatch(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	repo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())

	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ch, err := repo.Watch(watchCtx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		repo.Create(ctx, newApp("watched-app"))
	}()

	select {
	case ev := <-ch:
		if ev.Type != store.EventCreate {
			t.Errorf("event type: got %d, want EventCreate", ev.Type)
		}
		if ev.Object == nil {
			t.Fatal("expected non-nil object in watch event")
		}
		if ev.Object.Metadata.Name != "watched-app" {
			t.Errorf("name: got %q, want %q", ev.Object.Metadata.Name, "watched-app")
		}
	case <-watchCtx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}

func TestIsolationBetweenTypes(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	appRepo := New[*v1alpha1.Application](s, ApplicationKey, ApplicationPrefix())
	envRepo := New[*v1alpha1.Environment](s, EnvironmentKey, EnvironmentPrefix())

	_, err := appRepo.Create(ctx, newApp("shared-name"))
	if err != nil {
		t.Fatalf("Create app: %v", err)
	}

	env := &v1alpha1.Environment{}
	env.APIVersion = v1alpha1.GroupVersion
	env.Kind = v1alpha1.KindEnvironment
	env.Metadata = meta.ObjectMeta{Name: "shared-name"}
	env.Spec = v1alpha1.EnvironmentSpec{
		Type:       "kubernetes",
		Connection: v1alpha1.ConnectionSpec{Endpoint: "https://k8s.example.com", CredentialRef: "vault:k8s"},
		Capabilities: v1alpha1.CapabilitiesSpec{ResourceTypes: []string{"compute.container"}},
		Sovereignty:  v1alpha1.SovereigntySpec{Country: "US", Region: "us-east-1", Jurisdiction: "US", DataClassification: "internal"},
	}
	_, err = envRepo.Create(ctx, env)
	if err != nil {
		t.Fatalf("Create env: %v", err)
	}

	// List should be isolated
	apps, err := appRepo.List(ctx)
	if err != nil {
		t.Fatalf("List apps: %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("apps: got %d, want 1", len(apps))
	}

	envs, err := envRepo.List(ctx)
	if err != nil {
		t.Fatalf("List envs: %v", err)
	}
	if len(envs) != 1 {
		t.Errorf("envs: got %d, want 1", len(envs))
	}
}
