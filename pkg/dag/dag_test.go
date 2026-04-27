package dag

import (
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

// --- DAG data structure tests ---

func TestEmptyDAG(t *testing.T) {
	d := New()
	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 0 {
		t.Errorf("expected 0 levels, got %d", len(levels))
	}
}

func TestSingleNode(t *testing.T) {
	d := New()
	d.AddNode("a")
	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 1 || levels[0][0] != "a" {
		t.Errorf("expected [[a]], got %v", levels)
	}
}

func TestLinearChain(t *testing.T) {
	d := New()
	d.AddEdge("c", "b") // c depends on b
	d.AddEdge("b", "a") // b depends on a

	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}
	if levels[0][0] != "a" {
		t.Errorf("level 0: got %v, want [a]", levels[0])
	}
	if levels[1][0] != "b" {
		t.Errorf("level 1: got %v, want [b]", levels[1])
	}
	if levels[2][0] != "c" {
		t.Errorf("level 2: got %v, want [c]", levels[2])
	}
}

func TestDiamond(t *testing.T) {
	d := New()
	d.AddEdge("d", "b") // d depends on b
	d.AddEdge("d", "c") // d depends on c
	d.AddEdge("b", "a") // b depends on a
	d.AddEdge("c", "a") // c depends on a

	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}
	if levels[0][0] != "a" {
		t.Errorf("level 0: got %v, want [a]", levels[0])
	}
	// b and c should be in the same level (parallel)
	if len(levels[1]) != 2 {
		t.Errorf("level 1: expected 2 parallel nodes, got %v", levels[1])
	}
	if levels[2][0] != "d" {
		t.Errorf("level 2: got %v, want [d]", levels[2])
	}
}

func TestParallelNodes(t *testing.T) {
	d := New()
	d.AddNode("a")
	d.AddNode("b")
	d.AddNode("c")

	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(levels))
	}
	if len(levels[0]) != 3 {
		t.Errorf("expected 3 parallel nodes, got %d", len(levels[0]))
	}
}

func TestCycleDetection(t *testing.T) {
	d := New()
	d.AddEdge("a", "b")
	d.AddEdge("b", "c")
	d.AddEdge("c", "a")

	cycle := d.DetectCycle()
	if cycle == nil {
		t.Fatal("expected cycle to be detected")
	}
	// Cycle should contain a, b, c
	cycleStr := strings.Join(cycle, " -> ")
	if !strings.Contains(cycleStr, "a") || !strings.Contains(cycleStr, "b") || !strings.Contains(cycleStr, "c") {
		t.Errorf("cycle should contain a, b, c: got %s", cycleStr)
	}
}

func TestCycleInTopologicalSort(t *testing.T) {
	d := New()
	d.AddEdge("a", "b")
	d.AddEdge("b", "a")

	_, err := d.TopologicalSort()
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error should mention circular dependency: %v", err)
	}
}

func TestNoCycle(t *testing.T) {
	d := New()
	d.AddEdge("b", "a")
	d.AddEdge("c", "a")

	cycle := d.DetectCycle()
	if cycle != nil {
		t.Errorf("no cycle expected, got %v", cycle)
	}
}

func TestDependencies(t *testing.T) {
	d := New()
	d.AddEdge("app", "db")
	d.AddEdge("app", "cache")

	deps := d.Dependencies("app")
	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies, got %d", len(deps))
	}
}

func TestDependents(t *testing.T) {
	d := New()
	d.AddEdge("app", "db")
	d.AddEdge("frontend", "db")

	dependents := d.Dependents("db")
	if len(dependents) != 2 {
		t.Errorf("expected 2 dependents, got %d", len(dependents))
	}
}

// --- Builder tests ---

func makeApp(resources []v1alpha1.ResourceDecl) *v1alpha1.Application {
	app := &v1alpha1.Application{}
	app.APIVersion = v1alpha1.GroupVersion
	app.Kind = v1alpha1.KindApplication
	app.Metadata = meta.ObjectMeta{Name: "test-app"}
	app.Spec = v1alpha1.ApplicationSpec{Resources: resources}
	return app
}

func TestBuildSimpleDependency(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql", Properties: map[string]any{"size": "S"}},
		{Name: "app", Type: "compute.container", Properties: map[string]any{
			"dbUrl": "${db.connectionString}",
		}},
	})

	d, err := Build(app)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %v", len(levels), levels)
	}
	if levels[0][0] != "db" {
		t.Errorf("level 0: got %v, want [db]", levels[0])
	}
	if levels[1][0] != "app" {
		t.Errorf("level 1: got %v, want [app]", levels[1])
	}
}

func TestBuildExplicitRequirements(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "app", Type: "compute.container", Requirements: []string{"db"}},
	})

	d, err := Build(app)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	deps := d.Dependencies("app")
	if len(deps) != 1 || deps[0] != "db" {
		t.Errorf("app dependencies: got %v, want [db]", deps)
	}
}

func TestBuildMixedDependencies(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "cache", Type: "cache.redis"},
		{Name: "app", Type: "compute.container",
			Requirements: []string{"cache"},
			Properties: map[string]any{
				"dbUrl": "${db.connectionString}",
			},
		},
	})

	d, err := Build(app)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	deps := d.Dependencies("app")
	if len(deps) != 2 {
		t.Errorf("app should depend on db and cache, got %v", deps)
	}
}

func TestBuildParallelResources(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "cache", Type: "cache.redis"},
		{Name: "queue", Type: "queue.rabbitmq"},
	})

	d, err := Build(app)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 1 {
		t.Fatalf("expected 1 level (all parallel), got %d", len(levels))
	}
	if len(levels[0]) != 3 {
		t.Errorf("expected 3 parallel nodes, got %d", len(levels[0]))
	}
}

func TestBuildMultiLevelDiamond(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "cache", Type: "cache.redis"},
		{Name: "api", Type: "compute.container", Properties: map[string]any{
			"dbUrl":    "${db.connectionString}",
			"cacheUrl": "${cache.host}",
		}},
		{Name: "frontend", Type: "compute.container", Properties: map[string]any{
			"apiUrl": "${api.host}",
		}},
	})

	d, err := Build(app)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	levels, err := d.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}
	// Level 0: db, cache (parallel)
	if len(levels[0]) != 2 {
		t.Errorf("level 0: expected 2 nodes, got %v", levels[0])
	}
	// Level 1: api
	if len(levels[1]) != 1 || levels[1][0] != "api" {
		t.Errorf("level 1: got %v, want [api]", levels[1])
	}
	// Level 2: frontend
	if len(levels[2]) != 1 || levels[2][0] != "frontend" {
		t.Errorf("level 2: got %v, want [frontend]", levels[2])
	}
}

func TestBuildInterpolatedExpression(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "app", Type: "compute.container", Properties: map[string]any{
			"connStr": "postgres://${db.host}:${db.port}/mydb",
		}},
	})

	d, err := Build(app)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	deps := d.Dependencies("app")
	if len(deps) != 1 || deps[0] != "db" {
		t.Errorf("expected [db], got %v", deps)
	}
}

func TestBuildUnknownRequirement(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "app", Type: "compute.container", Requirements: []string{"nonexistent"}},
	})

	_, err := Build(app)
	if err == nil {
		t.Fatal("expected error for unknown requirement")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Errorf("error should mention unknown resource: %v", err)
	}
}

func TestBuildUnknownCELReference(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "app", Type: "compute.container", Properties: map[string]any{
			"url": "${nonexistent.host}",
		}},
	})

	_, err := Build(app)
	if err == nil {
		t.Fatal("expected error for unknown CEL reference")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Errorf("error should mention unknown resource: %v", err)
	}
}

func TestBuildSelfDependency(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "app", Type: "compute.container", Requirements: []string{"app"}},
	})

	_, err := Build(app)
	if err == nil {
		t.Fatal("expected error for self-dependency")
	}
	if !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("error should mention self-dependency: %v", err)
	}
}

func TestBuildCyclicDependency(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "a", Type: "compute.container", Requirements: []string{"b"}},
		{Name: "b", Type: "compute.container", Requirements: []string{"c"}},
		{Name: "c", Type: "compute.container", Requirements: []string{"a"}},
	})

	_, err := Build(app)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error should mention circular dependency: %v", err)
	}
}

func TestBuildDuplicateResourceName(t *testing.T) {
	app := makeApp([]v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "db", Type: "cache.redis"},
	})

	_, err := Build(app)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate: %v", err)
	}
}
