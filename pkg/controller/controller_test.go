package controller

import (
	"context"
	"encoding/json"
	"os"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/placement"
	"github.com/dcm-io/dcm/pkg/repository"
	kinestore "github.com/dcm-io/dcm/pkg/store/kine"
)

func setupTest(t *testing.T) (*ApplicationReconciler, *repository.Repository[*v1alpha1.Application], *repository.Repository[*v1alpha1.Environment], *repository.Repository[*v1alpha1.ResourceType], *repository.Repository[*v1alpha1.PlacementPolicy], *engine.MockDriver) {
	t.Helper()
	dir, err := os.MkdirTemp("", "dcm-ctrl-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s, err := kinestore.New(context.Background(), kinestore.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	appRepo := repository.New[*v1alpha1.Application](s, repository.ApplicationKey, repository.ApplicationPrefix())
	envRepo := repository.New[*v1alpha1.Environment](s, repository.EnvironmentKey, repository.EnvironmentPrefix())
	rtRepo := repository.New[*v1alpha1.ResourceType](s, repository.ResourceTypeKey, repository.ResourceTypePrefix())
	policyRepo := repository.New[*v1alpha1.PlacementPolicy](s, repository.PlacementPolicyKey, repository.PlacementPolicyPrefix())

	mock := engine.NewMockDriver()
	executor := engine.NewExecutor(map[string]engine.Driver{"mock": mock})
	placer, _ := placement.NewEngine()

	reconciler := NewApplicationReconciler(appRepo, envRepo, rtRepo, policyRepo, executor, placer, s)

	return reconciler, appRepo, envRepo, rtRepo, policyRepo, mock
}

func createResourceType(t *testing.T, rtRepo *repository.Repository[*v1alpha1.ResourceType]) {
	t.Helper()
	rt := &v1alpha1.ResourceType{}
	rt.APIVersion = v1alpha1.GroupVersion
	rt.Kind = v1alpha1.KindResourceType
	rt.Metadata = meta.ObjectMeta{Name: "database.postgresql"}
	rt.Spec = v1alpha1.ResourceTypeSpec{
		Version: "1.0.0", Lifecycle: "stable",
		Schema: map[string]any{
			"type": "object", "required": []any{"size"},
			"properties": map[string]any{
				"size": map[string]any{"type": "string", "enum": []any{"S", "M", "L"}, "default": "S"},
				"host": map[string]any{"type": "string", "readOnly": true},
				"port": map[string]any{"type": "integer", "readOnly": true},
			},
		},
	}
	if _, err := rtRepo.Create(context.Background(), rt); err != nil {
		t.Fatalf("create RT: %v", err)
	}
}

func createContainerType(t *testing.T, rtRepo *repository.Repository[*v1alpha1.ResourceType]) {
	t.Helper()
	rt := &v1alpha1.ResourceType{}
	rt.APIVersion = v1alpha1.GroupVersion
	rt.Kind = v1alpha1.KindResourceType
	rt.Metadata = meta.ObjectMeta{Name: "compute.container"}
	rt.Spec = v1alpha1.ResourceTypeSpec{
		Version: "1.0.0", Lifecycle: "stable",
		Schema: map[string]any{
			"type": "object", "required": []any{"image"},
			"properties": map[string]any{
				"image": map[string]any{"type": "string"},
				"host":  map[string]any{"type": "string", "readOnly": true},
			},
		},
	}
	if _, err := rtRepo.Create(context.Background(), rt); err != nil {
		t.Fatalf("create container RT: %v", err)
	}
}

func createEnvironment(t *testing.T, envRepo *repository.Repository[*v1alpha1.Environment], name string, resourceTypes []string) {
	t.Helper()
	// Build mock recipe bindings for all resource types
	recipes := make(map[string]map[string]v1alpha1.RecipeBinding)
	for _, rt := range resourceTypes {
		recipes[rt] = map[string]v1alpha1.RecipeBinding{
			"default": {Type: "mock", Source: map[string]string{}},
		}
	}
	env := &v1alpha1.Environment{}
	env.APIVersion = v1alpha1.GroupVersion
	env.Kind = v1alpha1.KindEnvironment
	env.Metadata = meta.ObjectMeta{Name: name}
	env.Spec = v1alpha1.EnvironmentSpec{
		Type: "kubernetes",
		Connection: v1alpha1.ConnectionSpec{
			Endpoint: "https://" + name + ".example.com", CredentialRef: "vault:" + name,
		},
		Capabilities: v1alpha1.CapabilitiesSpec{ResourceTypes: resourceTypes},
		Sovereignty: v1alpha1.SovereigntySpec{
			Country: "US", Region: "us-east-1", Jurisdiction: "US", DataClassification: "internal",
		},
		Recipes: recipes,
	}
	if _, err := envRepo.Create(context.Background(), env); err != nil {
		t.Fatalf("create env: %v", err)
	}
}

func createApp(t *testing.T, appRepo *repository.Repository[*v1alpha1.Application], name string, resources []v1alpha1.ResourceDecl) {
	t.Helper()
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: name}
	app.Spec = v1alpha1.ApplicationSpec{Resources: resources}
	if _, err := appRepo.Create(context.Background(), app); err != nil {
		t.Fatalf("create app: %v", err)
	}
}

func getStatus(t *testing.T, s *ApplicationReconciler, name string) *ApplicationStatus {
	t.Helper()
	key := "/registry/applicationstatus/" + name
	obj, err := s.statusStore.Get(context.Background(), key)
	if err != nil || obj == nil {
		return nil
	}
	var status ApplicationStatus
	json.Unmarshal(obj.Value, &status)
	return &status
}

// --- Tests ---

func TestReconcileSuccess(t *testing.T) {
	reconciler, appRepo, envRepo, rtRepo, _, mock := setupTest(t)
	ctx := context.Background()

	createResourceType(t, rtRepo)
	createEnvironment(t, envRepo, "prod", []string{"database.postgresql"})

	mock.Results["db"] = &engine.Result{
		Values: map[string]any{"host": "db.prod.local", "port": 5432},
	}

	createApp(t, appRepo, "my-app", []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql", Properties: map[string]any{"size": "M"}},
	})

	err := reconciler.Reconcile(ctx, "my-app")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	status := getStatus(t, reconciler, "my-app")
	if status == nil {
		t.Fatal("expected status to be saved")
	}
	if status.Phase != PhaseReady {
		t.Errorf("phase: got %q, want Ready", status.Phase)
	}
	if len(status.Resources) != 1 {
		t.Fatalf("expected 1 resource status, got %d", len(status.Resources))
	}
	if status.Resources[0].Phase != "Provisioned" {
		t.Errorf("resource phase: got %q", status.Resources[0].Phase)
	}

	// Verify mock was called
	if len(mock.Executed) != 1 {
		t.Errorf("expected 1 execution, got %d", len(mock.Executed))
	}
}

func TestReconcileMultiResource(t *testing.T) {
	reconciler, appRepo, envRepo, rtRepo, _, mock := setupTest(t)
	ctx := context.Background()

	createResourceType(t, rtRepo)
	createContainerType(t, rtRepo)
	createEnvironment(t, envRepo, "prod", []string{"database.postgresql", "compute.container"})

	mock.Results["db"] = &engine.Result{
		Values: map[string]any{"host": "db.prod.local", "port": 5432},
	}

	createApp(t, appRepo, "web-app", []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql", Properties: map[string]any{"size": "L"}},
		{Name: "fe", Type: "compute.container", Properties: map[string]any{
			"image": "quay.io/app/frontend",
		}, Requirements: []string{"db"}},
	})

	err := reconciler.Reconcile(ctx, "web-app")
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	status := getStatus(t, reconciler, "web-app")
	if status.Phase != PhaseReady {
		t.Errorf("phase: got %q, want Ready", status.Phase)
	}
	if len(status.Resources) != 2 {
		t.Errorf("expected 2 resource statuses, got %d", len(status.Resources))
	}

	// Verify execution order: db before fe
	if mock.Executed[0].ResourceName != "db" {
		t.Errorf("first execution should be db, got %s", mock.Executed[0].ResourceName)
	}
	if mock.Executed[1].ResourceName != "fe" {
		t.Errorf("second execution should be fe, got %s", mock.Executed[1].ResourceName)
	}
}

func TestReconcileValidationFailure(t *testing.T) {
	reconciler, appRepo, _, _, _, _ := setupTest(t)
	ctx := context.Background()

	// Create app with no resources (structural validation will fail)
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: "bad-app"}
	app.Spec = v1alpha1.ApplicationSpec{Resources: nil}
	appRepo.Create(ctx, app)

	err := reconciler.Reconcile(ctx, "bad-app")
	if err == nil {
		t.Fatal("expected validation error")
	}

	status := getStatus(t, reconciler, "bad-app")
	if status == nil {
		t.Fatal("expected status")
	}
	if status.Phase != PhaseFailed {
		t.Errorf("phase: got %q, want Failed", status.Phase)
	}
	if !strings.Contains(status.Message, "validation") {
		t.Errorf("message should mention validation: %q", status.Message)
	}
}

func TestReconcilePlacementFailure(t *testing.T) {
	reconciler, appRepo, envRepo, rtRepo, _, _ := setupTest(t)
	ctx := context.Background()

	createResourceType(t, rtRepo)
	// Environment doesn't support database.postgresql
	createEnvironment(t, envRepo, "prod", []string{"compute.container"})

	createApp(t, appRepo, "no-place-app", []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql", Properties: map[string]any{"size": "S"}},
	})

	err := reconciler.Reconcile(ctx, "no-place-app")
	if err == nil {
		t.Fatal("expected placement error")
	}

	status := getStatus(t, reconciler, "no-place-app")
	if status.Phase != PhaseFailed {
		t.Errorf("phase: got %q, want Failed", status.Phase)
	}
	if !strings.Contains(status.Message, "placement") {
		t.Errorf("message should mention placement: %q", status.Message)
	}
}

func TestReconcileExecutionFailure(t *testing.T) {
	reconciler, appRepo, envRepo, rtRepo, _, mock := setupTest(t)
	ctx := context.Background()

	createResourceType(t, rtRepo)
	createEnvironment(t, envRepo, "prod", []string{"database.postgresql"})

	mock.Errors["db"] = fmt.Errorf("connection timeout")

	createApp(t, appRepo, "exec-fail-app", []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql", Properties: map[string]any{"size": "S"}},
	})

	err := reconciler.Reconcile(ctx, "exec-fail-app")
	if err == nil {
		t.Fatal("expected execution error")
	}

	status := getStatus(t, reconciler, "exec-fail-app")
	if status.Phase != PhaseFailed {
		t.Errorf("phase: got %q, want Failed", status.Phase)
	}
}

func TestReconcileDeletedApp(t *testing.T) {
	reconciler, _, _, _, _, _ := setupTest(t)
	ctx := context.Background()

	err := reconciler.Reconcile(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("reconciling deleted app should not error: %v", err)
	}
}

func TestGenericControllerWatchLoop(t *testing.T) {
	dir, _ := os.MkdirTemp("", "dcm-ctrl-watch-*")
	t.Cleanup(func() { os.RemoveAll(dir) })

	s, _ := kinestore.New(context.Background(), kinestore.Config{DataDir: dir})
	t.Cleanup(func() { s.Close() })

	appRepo := repository.New[*v1alpha1.Application](s, repository.ApplicationKey, repository.ApplicationPrefix())

	reconciled := make(chan string, 10)
	reconciler := &testReconciler{ch: reconciled}

	ctrl := NewGenericController(appRepo, reconciler, "Application")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go ctrl.Run(ctx)

	// Give watch time to start
	time.Sleep(100 * time.Millisecond)

	// Create an application — should trigger reconciliation
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: "watched-app"}
	app.Spec = v1alpha1.ApplicationSpec{
		Resources: []v1alpha1.ResourceDecl{{Name: "db", Type: "database.postgresql"}},
	}
	appRepo.Create(context.Background(), app)

	select {
	case name := <-reconciled:
		if name != "watched-app" {
			t.Errorf("reconciled: got %q, want watched-app", name)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for reconciliation")
	}
}

type testReconciler struct {
	ch chan string
}

func (r *testReconciler) Reconcile(_ context.Context, name string) error {
	r.ch <- name
	return nil
}

