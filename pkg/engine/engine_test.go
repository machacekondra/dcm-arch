package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/schema"
)

// --- Helpers ---

func testApp(resources ...v1alpha1.ResourceDecl) *v1alpha1.Application {
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: "test-app"}
	app.Spec.Resources = resources
	return app
}

func testEnv(name string) *v1alpha1.Environment {
	env := &v1alpha1.Environment{}
	env.APIVersion = v1alpha1.GroupVersion
	env.Kind = v1alpha1.KindEnvironment
	env.Metadata = meta.ObjectMeta{Name: name}
	mockRecipe := map[string]v1alpha1.RecipeBinding{
		"default": {Type: "mock", Source: map[string]string{}},
	}
	env.Spec = v1alpha1.EnvironmentSpec{
		Type: "kubernetes",
		Connection: v1alpha1.ConnectionSpec{
			Endpoint: "https://" + name, CredentialRef: "vault:" + name,
		},
		Capabilities: v1alpha1.CapabilitiesSpec{
			ResourceTypes: []string{"database.postgresql", "compute.container"},
		},
		Sovereignty: v1alpha1.SovereigntySpec{
			Country: "US", Region: "us-east-1", Jurisdiction: "US", DataClassification: "internal",
		},
		Recipes: map[string]map[string]v1alpha1.RecipeBinding{
			"database.postgresql": mockRecipe,
			"compute.container":   mockRecipe,
			"cache.redis":         mockRecipe,
			"queue.rabbitmq":      mockRecipe,
		},
	}
	return env
}

func makePlan(app *v1alpha1.Application, levels [][]string, assignments map[string]string) *ExecutionPlan {
	envs := make(map[string]*v1alpha1.Environment)
	for _, envName := range assignments {
		if envs[envName] == nil {
			envs[envName] = testEnv(envName)
		}
	}
	return &ExecutionPlan{
		Application:  app,
		Levels:       levels,
		Assignments:  assignments,
		Environments: envs,
	}
}

// --- Context tests ---

func TestBuildContext(t *testing.T) {
	app := testApp()
	env := testEnv("prod-eu")

	ctx := BuildContext("my-db", app, env)

	res := ctx["resource"].(map[string]any)
	if res["name"] != "my-db" {
		t.Errorf("resource.name: got %v", res["name"])
	}

	appMeta := ctx["application"].(map[string]any)
	if appMeta["name"] != "test-app" {
		t.Errorf("application.name: got %v", appMeta["name"])
	}

	envMeta := ctx["environment"].(map[string]any)
	if envMeta["name"] != "prod-eu" {
		t.Errorf("environment.name: got %v", envMeta["name"])
	}
	if envMeta["type"] != "kubernetes" {
		t.Errorf("environment.type: got %v", envMeta["type"])
	}
}

// --- State tests ---

func TestExecutionState(t *testing.T) {
	state := NewExecutionState()

	state.SetOutput("db", &Result{
		Values:  map[string]any{"host": "db.example.com", "port": 5432},
		Secrets: map[string]any{"password": "secret"},
	})

	val, err := state.ResolveOutputValue("db", "host")
	if err != nil {
		t.Fatalf("ResolveOutputValue: %v", err)
	}
	if val != "db.example.com" {
		t.Errorf("got %v, want db.example.com", val)
	}

	val, err = state.ResolveOutputValue("db", "password")
	if err != nil {
		t.Fatalf("resolve secret: %v", err)
	}
	if val != "secret" {
		t.Errorf("got %v, want secret", val)
	}

	_, err = state.ResolveOutputValue("cache", "host")
	if err == nil {
		t.Error("expected error for unprovisioned resource")
	}

	_, err = state.ResolveOutputValue("db", "nonexistent")
	if err == nil {
		t.Error("expected error for unknown field")
	}
}

// --- Result validation tests ---

func TestValidateResultValid(t *testing.T) {
	parsed := &schema.ParsedSchema{
		Properties: map[string]schema.PropertySchema{
			"host":     {Name: "host", Type: "string", ReadOnly: true},
			"port":     {Name: "port", Type: "integer", ReadOnly: true},
			"password": {Name: "password", Type: "string", ReadOnly: true, Sensitive: true},
			"size":     {Name: "size", Type: "string"}, // input, not readOnly
		},
	}
	result := &Result{
		Values:  map[string]any{"host": "db.example.com", "port": 5432},
		Secrets: map[string]any{"password": "secret"},
	}

	err := ValidateResult(result, parsed)
	if err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestValidateResultMissingOutput(t *testing.T) {
	parsed := &schema.ParsedSchema{
		Properties: map[string]schema.PropertySchema{
			"host": {Name: "host", Type: "string", ReadOnly: true},
			"port": {Name: "port", Type: "integer", ReadOnly: true},
		},
	}
	result := &Result{
		Values: map[string]any{"host": "db.example.com"},
		// port is missing
	}

	err := ValidateResult(result, parsed)
	if err == nil {
		t.Fatal("expected error for missing output")
	}
	if !strings.Contains(err.Error(), "missing output") {
		t.Errorf("error should mention missing output: %v", err)
	}
}

func TestValidateResultSensitiveInValues(t *testing.T) {
	parsed := &schema.ParsedSchema{
		Properties: map[string]schema.PropertySchema{
			"password": {Name: "password", Type: "string", ReadOnly: true, Sensitive: true},
		},
	}
	result := &Result{
		Values: map[string]any{"password": "oops"}, // should be in secrets
	}

	err := ValidateResult(result, parsed)
	if err == nil {
		t.Fatal("expected error for sensitive in values")
	}
	if !strings.Contains(err.Error(), "secrets") {
		t.Errorf("error should mention secrets: %v", err)
	}
}

// --- Execution tests ---

func TestExecuteSingleResource(t *testing.T) {
	mock := NewMockDriver()
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	plan := makePlan(app, [][]string{{"db"}}, map[string]string{"db": "prod"})

	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.Statuses["db"].Phase != "Provisioned" {
		t.Errorf("db phase: got %q, want Provisioned", result.Statuses["db"].Phase)
	}
	if len(mock.Executed) != 1 {
		t.Errorf("expected 1 invocation, got %d", len(mock.Executed))
	}
}

func TestExecuteLinearDAG(t *testing.T) {
	mock := NewMockDriver()
	mock.Results["db"] = &Result{
		Values:  map[string]any{"host": "db.prod.local", "port": 5432, "connectionString": "postgres://db.prod.local:5432/mydb"},
		Secrets: map[string]any{"username": "admin", "password": "secret"},
	}
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(
		v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"},
		v1alpha1.ResourceDecl{Name: "app", Type: "compute.container", Properties: map[string]any{
			"dbUrl": "${db.connectionString}",
		}},
	)
	plan := makePlan(app,
		[][]string{{"db"}, {"app"}},
		map[string]string{"db": "prod", "app": "prod"},
	)

	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// db should be executed first
	if mock.Executed[0].ResourceName != "db" {
		t.Errorf("first invocation should be db, got %s", mock.Executed[0].ResourceName)
	}
	if mock.Executed[1].ResourceName != "app" {
		t.Errorf("second invocation should be app, got %s", mock.Executed[1].ResourceName)
	}

	// app should have resolved ${db.connectionString}
	appProps := mock.Executed[1].Properties
	if appProps["dbUrl"] != "postgres://db.prod.local:5432/mydb" {
		t.Errorf("dbUrl should be resolved: got %v", appProps["dbUrl"])
	}

	if result.Statuses["db"].Phase != "Provisioned" {
		t.Errorf("db: got %q", result.Statuses["db"].Phase)
	}
	if result.Statuses["app"].Phase != "Provisioned" {
		t.Errorf("app: got %q", result.Statuses["app"].Phase)
	}
}

func TestExecuteParallelResources(t *testing.T) {
	mock := NewMockDriver()
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(
		v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"},
		v1alpha1.ResourceDecl{Name: "cache", Type: "cache.redis"},
		v1alpha1.ResourceDecl{Name: "queue", Type: "queue.rabbitmq"},
	)
	plan := makePlan(app,
		[][]string{{"cache", "db", "queue"}},
		map[string]string{"db": "prod", "cache": "prod", "queue": "prod"},
	)

	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(mock.Executed) != 3 {
		t.Errorf("expected 3 invocations, got %d", len(mock.Executed))
	}
	for _, name := range []string{"db", "cache", "queue"} {
		if result.Statuses[name].Phase != "Provisioned" {
			t.Errorf("%s: got %q", name, result.Statuses[name].Phase)
		}
	}
}

func TestExecuteMultiLevel(t *testing.T) {
	mock := NewMockDriver()
	mock.Results["db"] = &Result{
		Values: map[string]any{"host": "db.local", "connectionString": "postgres://db.local:5432/app"},
	}
	mock.Results["cache"] = &Result{
		Values: map[string]any{"host": "cache.local"},
	}
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(
		v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"},
		v1alpha1.ResourceDecl{Name: "cache", Type: "cache.redis"},
		v1alpha1.ResourceDecl{Name: "api", Type: "compute.container", Properties: map[string]any{
			"dbUrl":    "${db.connectionString}",
			"cacheUrl": "${cache.host}",
		}},
		v1alpha1.ResourceDecl{Name: "frontend", Type: "compute.container", Properties: map[string]any{
			"apiUrl": "${api.host}",
		}},
	)
	plan := makePlan(app,
		[][]string{{"cache", "db"}, {"api"}, {"frontend"}},
		map[string]string{"db": "prod", "cache": "prod", "api": "prod", "frontend": "prod"},
	)

	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for _, name := range []string{"db", "cache", "api", "frontend"} {
		if result.Statuses[name].Phase != "Provisioned" {
			t.Errorf("%s: got %q", name, result.Statuses[name].Phase)
		}
	}

	// Verify api got resolved references
	for _, inv := range mock.Executed {
		if inv.ResourceName == "api" {
			if inv.Properties["dbUrl"] != "postgres://db.local:5432/app" {
				t.Errorf("api.dbUrl not resolved: %v", inv.Properties["dbUrl"])
			}
			if inv.Properties["cacheUrl"] != "cache.local" {
				t.Errorf("api.cacheUrl not resolved: %v", inv.Properties["cacheUrl"])
			}
		}
	}
}

func TestExecuteDriverFailure(t *testing.T) {
	mock := NewMockDriver()
	mock.Errors["db"] = fmt.Errorf("connection refused")
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	plan := makePlan(app, [][]string{{"db"}}, map[string]string{"db": "prod"})

	result, err := executor.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error from driver failure")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error should mention driver error: %v", err)
	}
	if result.Statuses["db"].Phase != "Failed" {
		t.Errorf("db phase: got %q, want Failed", result.Statuses["db"].Phase)
	}
}

func TestExecuteNoDriverRegistered(t *testing.T) {
	executor := NewExecutor(map[string]Driver{}) // no drivers

	app := testApp(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	plan := makePlan(app, [][]string{{"db"}}, map[string]string{"db": "prod"})

	_, err := executor.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error for missing driver")
	}
	if !strings.Contains(err.Error(), "no driver") {
		t.Errorf("error should mention missing driver: %v", err)
	}
}

func TestExecuteContextInjection(t *testing.T) {
	mock := NewMockDriver()
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	plan := makePlan(app, [][]string{{"db"}}, map[string]string{"db": "prod"})

	_, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	inv := mock.Executed[0]
	ctx := inv.Context
	res := ctx["resource"].(map[string]any)
	if res["name"] != "db" {
		t.Errorf("context.resource.name: got %v", res["name"])
	}
	appCtx := ctx["application"].(map[string]any)
	if appCtx["name"] != "test-app" {
		t.Errorf("context.application.name: got %v", appCtx["name"])
	}
	envCtx := ctx["environment"].(map[string]any)
	if envCtx["name"] != "prod" {
		t.Errorf("context.environment.name: got %v", envCtx["name"])
	}
}

func TestExecuteInterpolatedReferences(t *testing.T) {
	mock := NewMockDriver()
	mock.Results["db"] = &Result{
		Values: map[string]any{"host": "db.local", "port": 5432},
	}
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(
		v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"},
		v1alpha1.ResourceDecl{Name: "app", Type: "compute.container", Properties: map[string]any{
			"connStr": "postgres://${db.host}:${db.port}/mydb",
		}},
	)
	plan := makePlan(app,
		[][]string{{"db"}, {"app"}},
		map[string]string{"db": "prod", "app": "prod"},
	)

	_, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for _, inv := range mock.Executed {
		if inv.ResourceName == "app" {
			if inv.Properties["connStr"] != "postgres://db.local:5432/mydb" {
				t.Errorf("interpolation failed: got %v", inv.Properties["connStr"])
			}
		}
	}
}

func TestExecuteRecipeParameterMerging(t *testing.T) {
	mock := NewMockDriver()
	executor := NewExecutor(map[string]Driver{"helm": mock})

	env := testEnv("prod")
	env.Spec.Recipes = map[string]map[string]v1alpha1.RecipeBinding{
		"database.postgresql": {
			"default": {
				Type:       "helm",
				Source:     map[string]string{"chart": "cnpg-postgresql"},
				Parameters: map[string]any{"storageClass": "ssd", "backups": true},
			},
		},
	}

	app := testApp(v1alpha1.ResourceDecl{
		Name: "db", Type: "database.postgresql",
		Properties: map[string]any{"size": "M", "storageClass": "nvme"}, // developer overrides storageClass
	})
	plan := &ExecutionPlan{
		Application:  app,
		Levels:       [][]string{{"db"}},
		Assignments:  map[string]string{"db": "prod"},
		Environments: map[string]*v1alpha1.Environment{"prod": env},
	}

	_, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	inv := mock.Executed[0]
	// Developer's storageClass should override recipe default
	if inv.Properties["storageClass"] != "nvme" {
		t.Errorf("storageClass: got %v, want nvme (developer override)", inv.Properties["storageClass"])
	}
	// Recipe default should be present for non-overridden params
	if inv.Properties["backups"] != true {
		t.Errorf("backups: got %v, want true (recipe default)", inv.Properties["backups"])
	}
	// Developer property should be present
	if inv.Properties["size"] != "M" {
		t.Errorf("size: got %v, want M", inv.Properties["size"])
	}
	// Source should come from recipe
	if inv.Source["chart"] != "cnpg-postgresql" {
		t.Errorf("source.chart: got %v", inv.Source["chart"])
	}
}

func TestExecuteCancellation(t *testing.T) {
	mock := NewMockDriver()
	executor := NewExecutor(map[string]Driver{"mock": mock})

	app := testApp(
		v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"},
		v1alpha1.ResourceDecl{Name: "app", Type: "compute.container"},
	)
	plan := makePlan(app, [][]string{{"db"}, {"app"}}, map[string]string{"db": "prod", "app": "prod"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := executor.Execute(ctx, plan)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error should mention cancelled: %v", err)
	}
}
