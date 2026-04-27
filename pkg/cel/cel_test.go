package cel

import (
	"sort"
	"testing"
)

// --- Parser tests ---

func TestExtractStandaloneExpression(t *testing.T) {
	exprs := ExtractExpressions("targetPort", "${db.appPort}")
	if len(exprs) != 1 {
		t.Fatalf("expected 1 expression, got %d", len(exprs))
	}
	if exprs[0].CEL != "db.appPort" {
		t.Errorf("CEL: got %q, want %q", exprs[0].CEL, "db.appPort")
	}
	if exprs[0].Path != "targetPort" {
		t.Errorf("Path: got %q, want %q", exprs[0].Path, "targetPort")
	}
}

func TestExtractInterpolatedExpression(t *testing.T) {
	exprs := ExtractExpressions("connectionUrl", "postgres://${db.host}:${db.port}/${params.dbName}")
	if len(exprs) != 3 {
		t.Fatalf("expected 3 expressions, got %d", len(exprs))
	}

	cels := make([]string, len(exprs))
	for i, e := range exprs {
		cels[i] = e.CEL
	}
	sort.Strings(cels)

	expected := []string{"db.host", "db.port", "params.dbName"}
	sort.Strings(expected)
	for i, want := range expected {
		if cels[i] != want {
			t.Errorf("expr[%d]: got %q, want %q", i, cels[i], want)
		}
	}
}

func TestExtractFromMap(t *testing.T) {
	props := map[string]any{
		"image":         "quay.io/example/app",
		"dbUrl":         "${db.connectionString}",
		"cacheHost":     "${cache.host}",
		"staticSetting": "literal-value",
	}
	exprs := ExtractAllExpressions(props)
	if len(exprs) != 2 {
		t.Fatalf("expected 2 expressions, got %d", len(exprs))
	}
}

func TestExtractFromNestedMap(t *testing.T) {
	props := map[string]any{
		"config": map[string]any{
			"host": "${db.host}",
			"port": "${db.port}",
		},
	}
	exprs := ExtractAllExpressions(props)
	if len(exprs) != 2 {
		t.Fatalf("expected 2 expressions, got %d", len(exprs))
	}
	// Check paths include nesting
	paths := map[string]bool{}
	for _, e := range exprs {
		paths[e.Path] = true
	}
	if !paths["config.host"] {
		t.Error("expected path config.host")
	}
	if !paths["config.port"] {
		t.Error("expected path config.port")
	}
}

func TestExtractFromSlice(t *testing.T) {
	props := map[string]any{
		"hosts": []any{"${db.host}", "${cache.host}"},
	}
	exprs := ExtractAllExpressions(props)
	if len(exprs) != 2 {
		t.Fatalf("expected 2 expressions, got %d", len(exprs))
	}
}

func TestExtractNoExpressions(t *testing.T) {
	props := map[string]any{
		"image": "quay.io/example/app",
		"port":  8080,
	}
	exprs := ExtractAllExpressions(props)
	if len(exprs) != 0 {
		t.Fatalf("expected 0 expressions, got %d", len(exprs))
	}
}

func TestContainsCEL(t *testing.T) {
	if !ContainsCEL("${db.host}") {
		t.Error("should detect CEL in ${db.host}")
	}
	if ContainsCEL("just a string") {
		t.Error("should not detect CEL in plain string")
	}
	if ContainsCEL(42) {
		t.Error("should not detect CEL in non-string")
	}
}

// --- Reference extraction tests ---

func TestExtractReferences(t *testing.T) {
	exprs := []Expression{
		{CEL: "db.host", Path: "dbHost"},
		{CEL: "db.port", Path: "dbPort"},
		{CEL: "cache.host", Path: "cacheHost"},
	}
	refs := ExtractReferences(exprs)
	if len(refs) != 3 {
		t.Fatalf("expected 3 references, got %d", len(refs))
	}
	if refs[0].ResourceName != "db" || refs[0].Field != "host" {
		t.Errorf("ref[0]: got %s.%s, want db.host", refs[0].ResourceName, refs[0].Field)
	}
}

func TestExtractReferencesSkipsParams(t *testing.T) {
	exprs := []Expression{
		{CEL: "params.dbName", Path: "dbName"},
		{CEL: "db.host", Path: "dbHost"},
	}
	refs := ExtractReferences(exprs)
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference (params skipped), got %d", len(refs))
	}
	if refs[0].ResourceName != "db" {
		t.Errorf("expected db, got %s", refs[0].ResourceName)
	}
}

func TestExtractReferencesSkipsEnv(t *testing.T) {
	exprs := []Expression{
		{CEL: `env.sovereignty.jurisdiction == "EU"`, Path: "rule"},
	}
	refs := ExtractReferences(exprs)
	if len(refs) != 0 {
		t.Fatalf("expected 0 references (env skipped), got %d", len(refs))
	}
}

func TestExtractReferencedResourceNames(t *testing.T) {
	exprs := []Expression{
		{CEL: "db.host"},
		{CEL: "db.port"},
		{CEL: "cache.host"},
	}
	names := ExtractReferencedResourceNames(exprs)
	sort.Strings(names)
	if len(names) != 2 {
		t.Fatalf("expected 2 unique names, got %d: %v", len(names), names)
	}
	if names[0] != "cache" || names[1] != "db" {
		t.Errorf("got %v, want [cache, db]", names)
	}
}

func TestExtractFromComplexExpression(t *testing.T) {
	exprs := []Expression{
		{CEL: `db.host + ":" + db.port`},
	}
	refs := ExtractReferences(exprs)
	// Should find db.host and db.port
	if len(refs) < 2 {
		t.Fatalf("expected at least 2 references, got %d", len(refs))
	}
}

// --- CEL environment and evaluation tests ---

func TestNewPlacementEnv(t *testing.T) {
	env, err := NewPlacementEnv()
	if err != nil {
		t.Fatalf("NewPlacementEnv: %v", err)
	}
	if env == nil {
		t.Fatal("expected non-nil environment")
	}
}

func TestCompileAndEvalRule(t *testing.T) {
	celEnv, _ := NewPlacementEnv()

	program, err := CompileRule(celEnv, `env.sovereignty.jurisdiction == "EU"`)
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}

	vars := map[string]any{
		"env": map[string]any{
			"sovereignty": map[string]any{
				"jurisdiction": "EU",
			},
		},
	}

	result, err := EvalBool(program, vars)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for EU jurisdiction")
	}
}

func TestCompileAndEvalRuleFalse(t *testing.T) {
	celEnv, _ := NewPlacementEnv()

	program, _ := CompileRule(celEnv, `env.sovereignty.jurisdiction == "EU"`)
	vars := map[string]any{
		"env": map[string]any{
			"sovereignty": map[string]any{
				"jurisdiction": "US",
			},
		},
	}

	result, _ := EvalBool(program, vars)
	if result {
		t.Error("expected false for US jurisdiction")
	}
}

func TestCompileAndEvalPrefer(t *testing.T) {
	celEnv, _ := NewPlacementEnv()

	program, err := CompilePrefer(celEnv, `env.capacity.cpu.available`)
	if err != nil {
		t.Fatalf("CompilePrefer: %v", err)
	}

	vars := map[string]any{
		"env": map[string]any{
			"capacity": map[string]any{
				"cpu": map[string]any{
					"available": int64(500),
				},
			},
		},
	}

	score, err := EvalFloat(program, vars)
	if err != nil {
		t.Fatalf("EvalFloat: %v", err)
	}
	if score != 500.0 {
		t.Errorf("score: got %f, want 500.0", score)
	}
}

func TestCompileRuleComplexExpression(t *testing.T) {
	celEnv, _ := NewPlacementEnv()

	program, err := CompileRule(celEnv, `
		env.sovereignty.jurisdiction == "EU"
		&& env.sovereignty.country == "DE"
	`)
	if err != nil {
		t.Fatalf("CompileRule: %v", err)
	}

	vars := map[string]any{
		"env": map[string]any{
			"sovereignty": map[string]any{
				"jurisdiction": "EU",
				"country":      "DE",
			},
		},
	}

	result, err := EvalBool(program, vars)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true")
	}
}

func TestCompilePreferCostExpression(t *testing.T) {
	celEnv, _ := NewPlacementEnv()

	program, err := CompilePrefer(celEnv, `-(env.cost.rates.cpu.value * 100.0)`)
	if err != nil {
		t.Fatalf("CompilePrefer: %v", err)
	}

	vars := map[string]any{
		"env": map[string]any{
			"cost": map[string]any{
				"rates": map[string]any{
					"cpu": map[string]any{
						"value": 0.048,
					},
				},
			},
		},
	}

	score, err := EvalFloat(program, vars)
	if err != nil {
		t.Fatalf("EvalFloat: %v", err)
	}
	if score > -4.7 || score < -4.9 {
		t.Errorf("score: got %f, want ~-4.8", score)
	}
}

func TestCheckSyntaxValid(t *testing.T) {
	celEnv, _ := NewPlacementEnv()
	err := CheckSyntax(celEnv, `env.sovereignty.jurisdiction == "EU"`)
	if err != nil {
		t.Fatalf("expected valid syntax: %v", err)
	}
}

func TestCheckSyntaxInvalid(t *testing.T) {
	celEnv, _ := NewPlacementEnv()
	err := CheckSyntax(celEnv, `env.sovereignty.jurisdiction ==== "EU"`)
	if err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestCompileRuleInvalidSyntax(t *testing.T) {
	celEnv, _ := NewPlacementEnv()
	_, err := CompileRule(celEnv, `this is not valid cel !!!`)
	if err == nil {
		t.Fatal("expected compile error")
	}
}
