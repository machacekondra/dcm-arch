package placement

import (
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/dag"
)

// --- Test helpers ---

func env(name, envType string, resourceTypes []string, opts ...func(*v1alpha1.Environment)) *v1alpha1.Environment {
	e := &v1alpha1.Environment{}
	e.APIVersion = v1alpha1.GroupVersion
	e.Kind = v1alpha1.KindEnvironment
	e.Metadata = meta.ObjectMeta{Name: name}
	e.Spec = v1alpha1.EnvironmentSpec{
		Type: envType,
		Connection: v1alpha1.ConnectionSpec{
			Endpoint:      "https://" + name + ".example.com",
			CredentialRef: "vault:" + name,
		},
		Capabilities: v1alpha1.CapabilitiesSpec{ResourceTypes: resourceTypes},
		Sovereignty: v1alpha1.SovereigntySpec{
			Country: "US", Region: "us-east-1", Jurisdiction: "US", DataClassification: "internal",
		},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func withSovereignty(country, jurisdiction string) func(*v1alpha1.Environment) {
	return func(e *v1alpha1.Environment) {
		e.Spec.Sovereignty.Country = country
		e.Spec.Sovereignty.Jurisdiction = jurisdiction
	}
}

func withLabels(labels map[string]string) func(*v1alpha1.Environment) {
	return func(e *v1alpha1.Environment) {
		e.Metadata.Labels = labels
	}
}

func withCapacity(cpuAvailable int) func(*v1alpha1.Environment) {
	return func(e *v1alpha1.Environment) {
		e.Spec.Capacity = &v1alpha1.CapacitySpec{
			CPU: &v1alpha1.ResourceCapacity{Total: cpuAvailable * 2, Unit: "cores"},
		}
	}
}

func withOverlays(overlays ...v1alpha1.OverlaySpec) func(*v1alpha1.Environment) {
	return func(e *v1alpha1.Environment) {
		if e.Spec.Networking == nil {
			e.Spec.Networking = &v1alpha1.NetworkingSpec{}
		}
		e.Spec.Networking.Overlays = overlays
	}
}

func policy(name string, opts ...func(*v1alpha1.PlacementPolicy)) *v1alpha1.PlacementPolicy {
	p := &v1alpha1.PlacementPolicy{}
	p.APIVersion = v1alpha1.GroupVersion
	p.Kind = v1alpha1.KindPlacementPolicy
	p.Metadata = meta.ObjectMeta{Name: name}
	p.Spec.Match = v1alpha1.MatchCriteria{All: true}
	p.Spec.Weight = 1.0
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func withRule(rule string) func(*v1alpha1.PlacementPolicy) {
	return func(p *v1alpha1.PlacementPolicy) { p.Spec.Rule = rule }
}

func withPrefer(prefer string) func(*v1alpha1.PlacementPolicy) {
	return func(p *v1alpha1.PlacementPolicy) { p.Spec.Prefer = prefer }
}

func withWeight(w float64) func(*v1alpha1.PlacementPolicy) {
	return func(p *v1alpha1.PlacementPolicy) { p.Spec.Weight = w }
}

func withMatchLabels(labels map[string]string) func(*v1alpha1.PlacementPolicy) {
	return func(p *v1alpha1.PlacementPolicy) {
		p.Spec.Match = v1alpha1.MatchCriteria{Labels: labels}
	}
}

func withMatchResourceTypes(types []string) func(*v1alpha1.PlacementPolicy) {
	return func(p *v1alpha1.PlacementPolicy) {
		p.Spec.Match = v1alpha1.MatchCriteria{ResourceTypes: types}
	}
}

func app(resources ...v1alpha1.ResourceDecl) *v1alpha1.Application {
	a := &v1alpha1.Application{}
	a.APIVersion = v1alpha1.GroupVersion
	a.Kind = v1alpha1.KindApplication
	a.Metadata = meta.ObjectMeta{Name: "test-app"}
	a.Spec.Resources = resources
	return a
}

func simpleDAG(resources []v1alpha1.ResourceDecl) *dag.DAG {
	d, _ := dag.Build(app(resources...))
	if d == nil {
		d = dag.New()
		for _, r := range resources {
			d.AddNode(r.Name)
		}
	}
	return d
}

// --- Prefilter tests ---

func TestPrefilterByResourceType(t *testing.T) {
	envs := []*v1alpha1.Environment{
		env("k8s", "kubernetes", []string{"compute.container", "database.postgresql"}),
		env("aws", "aws", []string{"compute.container"}),
		env("bare", "bare-metal", []string{"compute.virtual-machine"}),
	}

	result := PrefilterByResourceType(envs, "database.postgresql")
	if len(result) != 1 || result[0].Metadata.Name != "k8s" {
		t.Errorf("expected [k8s], got %v", envNames(result))
	}

	result = PrefilterByResourceType(envs, "compute.container")
	if len(result) != 2 {
		t.Errorf("expected 2 envs, got %d", len(result))
	}

	result = PrefilterByResourceType(envs, "cache.redis")
	if len(result) != 0 {
		t.Errorf("expected 0 envs, got %d", len(result))
	}
}

// --- Matcher tests ---

func TestMatchPolicyAll(t *testing.T) {
	p := policy("default")
	matched := MatchPolicies([]*v1alpha1.PlacementPolicy{p}, nil, "compute.container")
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchPolicyByLabels(t *testing.T) {
	p := policy("gdpr", withMatchLabels(map[string]string{"compliance": "gdpr"}))

	matched := MatchPolicies([]*v1alpha1.PlacementPolicy{p},
		map[string]string{"compliance": "gdpr", "team": "platform"}, "")
	if len(matched) != 1 {
		t.Error("expected match when labels are present")
	}

	matched = MatchPolicies([]*v1alpha1.PlacementPolicy{p},
		map[string]string{"team": "platform"}, "")
	if len(matched) != 0 {
		t.Error("expected no match when required label is missing")
	}
}

func TestMatchPolicyByResourceType(t *testing.T) {
	p := policy("db-policy", withMatchResourceTypes([]string{"database.postgresql"}))

	matched := MatchPolicies([]*v1alpha1.PlacementPolicy{p}, nil, "database.postgresql")
	if len(matched) != 1 {
		t.Error("expected match for database.postgresql")
	}

	matched = MatchPolicies([]*v1alpha1.PlacementPolicy{p}, nil, "compute.container")
	if len(matched) != 0 {
		t.Error("expected no match for compute.container")
	}
}

// --- Connectivity tests ---

func TestConnectivitySameEnv(t *testing.T) {
	g := BuildConnectivityGraph(nil)
	if !g.Connected("env-a", "env-a") {
		t.Error("same environment should always be connected")
	}
}

func TestConnectivitySharedOverlay(t *testing.T) {
	envs := []*v1alpha1.Environment{
		env("eu-1", "kubernetes", nil, withOverlays(v1alpha1.OverlaySpec{Name: "eu-mesh", Type: "submariner"})),
		env("eu-2", "kubernetes", nil, withOverlays(v1alpha1.OverlaySpec{Name: "eu-mesh", Type: "submariner"})),
		env("us-1", "kubernetes", nil, withOverlays(v1alpha1.OverlaySpec{Name: "us-mesh", Type: "submariner"})),
	}

	g := BuildConnectivityGraph(envs)

	if !g.Connected("eu-1", "eu-2") {
		t.Error("eu-1 and eu-2 share eu-mesh, should be connected")
	}
	if g.Connected("eu-1", "us-1") {
		t.Error("eu-1 and us-1 share no overlay, should not be connected")
	}
}

func TestConnectivityMultipleOverlays(t *testing.T) {
	envs := []*v1alpha1.Environment{
		env("a", "kubernetes", nil, withOverlays(
			v1alpha1.OverlaySpec{Name: "mesh-1", Type: "submariner"},
			v1alpha1.OverlaySpec{Name: "mesh-2", Type: "skupper"},
		)),
		env("b", "kubernetes", nil, withOverlays(
			v1alpha1.OverlaySpec{Name: "mesh-2", Type: "skupper"},
		)),
	}

	g := BuildConnectivityGraph(envs)
	if !g.Connected("a", "b") {
		t.Error("a and b share mesh-2, should be connected")
	}
}

// --- Engine tests ---

func TestPlaceSingleEnvSingleResource(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("prod", "kubernetes", []string{"database.postgresql"}),
	}
	a := app(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, nil, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["db"] != "prod" {
		t.Errorf("expected db -> prod, got %s", result.Assignments["db"])
	}
}

func TestPlaceNoEligibleEnv(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("prod", "kubernetes", []string{"compute.container"}),
	}
	a := app(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	d := simpleDAG(a.Spec.Resources)

	_, err := e.Place(a, envs, nil, d)
	if err == nil {
		t.Fatal("expected placement failure")
	}
	if !strings.Contains(err.Error(), "PlacementFailed") {
		t.Errorf("error should be PlacementFailed: %v", err)
	}
}

func TestPlaceWithRule(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("eu", "kubernetes", []string{"database.postgresql"}, withSovereignty("DE", "EU")),
		env("us", "kubernetes", []string{"database.postgresql"}, withSovereignty("US", "US")),
	}
	policies := []*v1alpha1.PlacementPolicy{
		policy("eu-only", withRule(`env.sovereignty.jurisdiction == "EU"`)),
	}
	a := app(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, policies, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["db"] != "eu" {
		t.Errorf("expected db -> eu, got %s", result.Assignments["db"])
	}
}

func TestPlaceWithPrefer(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("small", "kubernetes", []string{"compute.container"}, withCapacity(100)),
		env("large", "kubernetes", []string{"compute.container"}, withCapacity(500)),
	}
	policies := []*v1alpha1.PlacementPolicy{
		policy("prefer-capacity", withPrefer(`env.capacity.cpu.total`)),
	}
	a := app(v1alpha1.ResourceDecl{Name: "app", Type: "compute.container"})
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, policies, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["app"] != "large" {
		t.Errorf("expected app -> large (higher capacity), got %s", result.Assignments["app"])
	}
}

func TestPlaceTieBreakByName(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("beta", "kubernetes", []string{"compute.container"}),
		env("alpha", "kubernetes", []string{"compute.container"}),
	}
	a := app(v1alpha1.ResourceDecl{Name: "app", Type: "compute.container"})
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, nil, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["app"] != "alpha" {
		t.Errorf("expected tie-break to alpha, got %s", result.Assignments["app"])
	}
}

func TestPlaceMultiResourceSameEnv(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("prod", "kubernetes", []string{"database.postgresql", "compute.container"}),
	}
	resources := []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "app", Type: "compute.container", Requirements: []string{"db"}},
	}
	a := app(resources...)
	d := simpleDAG(resources)

	result, err := e.Place(a, envs, nil, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["db"] != "prod" || result.Assignments["app"] != "prod" {
		t.Errorf("expected both in prod, got %v", result.Assignments)
	}
}

func TestPlaceCrossEnvWithConnectivity(t *testing.T) {
	e, _ := NewEngine()
	overlay := v1alpha1.OverlaySpec{Name: "global-mesh", Type: "submariner"}
	envs := []*v1alpha1.Environment{
		env("eu", "kubernetes", []string{"database.postgresql"}, withSovereignty("DE", "EU"), withOverlays(overlay)),
		env("us", "kubernetes", []string{"compute.container"}, withSovereignty("US", "US"), withOverlays(overlay)),
	}
	policies := []*v1alpha1.PlacementPolicy{
		policy("db-eu", withMatchResourceTypes([]string{"database.postgresql"}), withRule(`env.sovereignty.jurisdiction == "EU"`)),
		policy("app-us", withMatchResourceTypes([]string{"compute.container"}), withRule(`env.sovereignty.jurisdiction == "US"`)),
	}
	resources := []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "app", Type: "compute.container", Requirements: []string{"db"}},
	}
	a := app(resources...)
	d := simpleDAG(resources)

	result, err := e.Place(a, envs, policies, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["db"] != "eu" {
		t.Errorf("expected db -> eu, got %s", result.Assignments["db"])
	}
	if result.Assignments["app"] != "us" {
		t.Errorf("expected app -> us, got %s", result.Assignments["app"])
	}
}

func TestPlaceCrossEnvWithoutConnectivity(t *testing.T) {
	e, _ := NewEngine()
	// No shared overlays between eu and us
	envs := []*v1alpha1.Environment{
		env("eu", "kubernetes", []string{"database.postgresql"}, withSovereignty("DE", "EU"),
			withOverlays(v1alpha1.OverlaySpec{Name: "eu-mesh", Type: "submariner"})),
		env("us", "kubernetes", []string{"compute.container"}, withSovereignty("US", "US"),
			withOverlays(v1alpha1.OverlaySpec{Name: "us-mesh", Type: "submariner"})),
	}
	policies := []*v1alpha1.PlacementPolicy{
		policy("db-eu", withMatchResourceTypes([]string{"database.postgresql"}), withRule(`env.sovereignty.jurisdiction == "EU"`)),
		policy("app-us", withMatchResourceTypes([]string{"compute.container"}), withRule(`env.sovereignty.jurisdiction == "US"`)),
	}
	resources := []v1alpha1.ResourceDecl{
		{Name: "db", Type: "database.postgresql"},
		{Name: "app", Type: "compute.container", Requirements: []string{"db"}},
	}
	a := app(resources...)
	d := simpleDAG(resources)

	_, err := e.Place(a, envs, policies, d)
	if err == nil {
		t.Fatal("expected connectivity error")
	}
	if !strings.Contains(err.Error(), "no connectivity") {
		t.Errorf("error should mention connectivity: %v", err)
	}
}

func TestPlaceWithWeightedPreferences(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("cheap", "kubernetes", []string{"compute.container"}, withCapacity(100)),
		env("big", "kubernetes", []string{"compute.container"}, withCapacity(500)),
	}
	// Two competing preferences: capacity (weight 1) vs negative capacity (weight 3)
	// big has capacity 1000 (total), cheap has 200
	// With inverted preference (weight 3), cheap should win
	policies := []*v1alpha1.PlacementPolicy{
		policy("prefer-capacity", withPrefer(`env.capacity.cpu.total`), withWeight(1.0)),
		policy("prefer-cheap", withPrefer(`-env.capacity.cpu.total`), withWeight(3.0)),
	}
	a := app(v1alpha1.ResourceDecl{Name: "app", Type: "compute.container"})
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, policies, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	// cheap: (200 * 1) + (-200 * 3) = -400
	// big:   (1000 * 1) + (-1000 * 3) = -2000
	// cheap wins (higher score)
	if result.Assignments["app"] != "cheap" {
		t.Errorf("expected app -> cheap (weighted preference), got %s", result.Assignments["app"])
	}
}

func TestPlaceDecisionLog(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("eu", "kubernetes", []string{"database.postgresql"}, withSovereignty("DE", "EU")),
		env("us", "kubernetes", []string{"database.postgresql"}, withSovereignty("US", "US")),
	}
	policies := []*v1alpha1.PlacementPolicy{
		policy("eu-only", withRule(`env.sovereignty.jurisdiction == "EU"`)),
	}
	a := app(v1alpha1.ResourceDecl{Name: "db", Type: "database.postgresql"})
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, policies, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if len(result.Decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(result.Decisions))
	}

	dec := result.Decisions[0]
	if dec.Selected != "eu" {
		t.Errorf("selected: got %q, want eu", dec.Selected)
	}
	if len(dec.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(dec.Candidates))
	}

	// Find the US candidate — it should be eliminated
	for _, c := range dec.Candidates {
		if c.Environment == "us" {
			if c.Eligible {
				t.Error("us should not be eligible")
			}
			if len(c.Eliminations) == 0 {
				t.Error("us should have elimination reasons")
			}
		}
	}
}

func TestPlaceResourceLabelsOverrideAppLabels(t *testing.T) {
	e, _ := NewEngine()
	envs := []*v1alpha1.Environment{
		env("eu", "kubernetes", []string{"database.postgresql"}, withSovereignty("DE", "EU")),
		env("us", "kubernetes", []string{"database.postgresql"}, withSovereignty("US", "US")),
	}
	policies := []*v1alpha1.PlacementPolicy{
		policy("region-eu", withMatchLabels(map[string]string{"region": "eu"}), withRule(`env.sovereignty.jurisdiction == "EU"`)),
		policy("region-us", withMatchLabels(map[string]string{"region": "us"}), withRule(`env.sovereignty.jurisdiction == "US"`)),
	}
	// App has region=us, but resource overrides with region=eu
	a := app(v1alpha1.ResourceDecl{
		Name:   "db",
		Type:   "database.postgresql",
		Labels: map[string]string{"region": "eu"},
	})
	a.Metadata.Labels = map[string]string{"region": "us"}
	d := simpleDAG(a.Spec.Resources)

	result, err := e.Place(a, envs, policies, d)
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if result.Assignments["db"] != "eu" {
		t.Errorf("resource labels should override app labels: expected eu, got %s", result.Assignments["db"])
	}
}

// --- helpers ---

func envNames(envs []*v1alpha1.Environment) []string {
	var names []string
	for _, e := range envs {
		names = append(names, e.Metadata.Name)
	}
	return names
}
