# DCM Implementation Roadmap

**Status:** Draft
**Date:** 2026-04-27
**Authors:** DCM Team

---

## Overview

This document defines the step-by-step implementation order for the DCM control plane. Each phase builds on the previous one, is independently testable, and delivers incremental value. Phases are ordered by dependency — you cannot build placement without CEL, you cannot build the execution engine without placement, etc.

---

## Phase 0: Bootstrap (DONE)

**What exists today:**

| Component | Package | Description |
|-----------|---------|-------------|
| Domain types | `pkg/apis/v1alpha1/types.go` | All 5 resource types: Application, Environment, ResourceType, Recipe, PlacementPolicy |
| Meta types | `pkg/apis/meta/types.go` | TypeMeta, ObjectMeta, Object interface |
| Codec | `pkg/codec/codec.go` | YAML/JSON encode/decode with apiVersion+kind dispatch |
| Store interface | `pkg/store/store.go` | Generic CRUD + Watch interface |
| Kine store | `pkg/store/kine/kine.go` | SQLite-backed implementation via kine |
| Server skeleton | `internal/server/server.go` | Wires store, start/stop lifecycle |
| Entry point | `cmd/dcm-server/main.go` | Signal handling, flag parsing |

---

## Phase 1: Typed Repository Layer

**Goal:** Provide type-safe CRUD operations so every consumer works with Go structs, not raw bytes.

**Why first:** Every subsequent phase (API, validation, controllers) needs to read/write domain objects. A typed repository eliminates repeated codec boilerplate everywhere.

### 1.1 Generic Repository

```
pkg/
  repository/
    repository.go       # Generic typed repository
    repository_test.go
    keys.go             # Key-path helpers: /registry/{kind}/{name}
```

**`repository.go`** — A generic `Repository[T meta.Object]` that wraps `store.Store` + `codec`:

```go
type Repository[T meta.Object] struct { ... }

func (r *Repository[T]) Create(ctx, obj T) error
func (r *Repository[T]) Get(ctx, name string) (T, int64, error)
func (r *Repository[T]) List(ctx) ([]T, error)
func (r *Repository[T]) Update(ctx, obj T, revision int64) error
func (r *Repository[T]) Delete(ctx, name string, revision int64) error
func (r *Repository[T]) Watch(ctx) (<-chan WatchEvent[T], error)
```

**`keys.go`** — Centralizes the key schema:

```go
func ApplicationKey(name string) string    // /registry/applications/{name}
func EnvironmentKey(name string) string    // /registry/environments/{name}
func ResourceTypeKey(name string) string   // /registry/resourcetypes/{name}
func RecipeKey(name string) string         // /registry/recipes/{name}
func PlacementPolicyKey(name string) string // /registry/placementpolicies/{name}
```

**Tests:** Round-trip Create/Get/List/Update/Delete for each domain type through the typed repository.

**Deliverable:** All downstream code interacts with typed Go objects, never raw `[]byte`.

---

## Phase 2: REST API

**Goal:** Expose CRUD endpoints for all 5 resource types so that personas can interact with the control plane.

**Why now:** Without an API, nothing can create or query resources. This is the system boundary where all external interaction happens.

### 2.1 HTTP Server + Router

```
pkg/
  api/
    server.go           # HTTP server setup, router
    middleware.go        # Logging, content-type, error handling
    handler.go           # Generic CRUD handler factory
    routes.go            # Route registration for all 5 types
    errors.go            # Structured API error responses
    api_test.go          # Integration tests
```

**Endpoints** (per resource type):

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/apis/dcm.io/v1alpha1/{resource}` | Create |
| `GET` | `/apis/dcm.io/v1alpha1/{resource}/{name}` | Get by name |
| `GET` | `/apis/dcm.io/v1alpha1/{resource}` | List all |
| `PUT` | `/apis/dcm.io/v1alpha1/{resource}/{name}` | Update (requires `X-DCM-Revision` header) |
| `DELETE` | `/apis/dcm.io/v1alpha1/{resource}/{name}` | Delete (requires `X-DCM-Revision` header) |

Where `{resource}` is one of: `applications`, `environments`, `resourcetypes`, `recipes`, `placementpolicies`.

**Design decisions:**
- Use `net/http` standard library (no framework needed for this scope).
- Request/response bodies are YAML or JSON (based on `Content-Type` / `Accept` headers).
- Revision passed via `X-DCM-Revision` header for optimistic concurrency.
- All errors return structured JSON: `{ "error": "message", "code": "CONFLICT" }`.

### 2.2 Update Server & Main

- Add `--listen` flag to `cmd/dcm-server/main.go` (default `:8080`).
- Wire HTTP server into `internal/server/server.go` lifecycle.

**Tests:** HTTP integration tests — create via POST, retrieve via GET, update via PUT, delete via DELETE, test conflict on stale revision.

**Deliverable:** `curl` or any HTTP client can manage all DCM resources. Personas can interact with the system.

---

## Phase 3: Structural Validation

**Goal:** Reject malformed resources at the API boundary before they reach storage.

**Why now:** Without validation, the store will accumulate garbage data that breaks every downstream component. Validation at the API layer is the first line of defense.

### 3.1 Validation Framework

```
pkg/
  validation/
    validation.go       # Validation result types, multi-error accumulator
    application.go      # Application structural validation
    environment.go      # Environment structural validation
    resourcetype.go     # ResourceType structural validation
    recipe.go           # Recipe structural validation
    placementpolicy.go  # PlacementPolicy structural validation
    validation_test.go
```

**Per-type structural validation:**

**Application:**
- `metadata.name` is required, valid DNS name
- `spec.resources` is non-empty
- Each resource has `type` and `name`
- Resource names are unique within the Application
- `requirements` reference existing resource names
- `includeWhen` is syntactically valid (defer full CEL validation to Phase 4)

**Environment:**
- `metadata.name` required
- `spec.type` is one of known types (kubernetes, openshift, vmware, aws, azure, gcp, bare-metal)
- `spec.connection.endpoint` is a valid URL
- `spec.connection.credentialRef` is non-empty
- `spec.capabilities.resourceTypes` is non-empty
- `spec.sovereignty` — required fields present, `country` is ISO 3166-1, `jurisdiction` is known
- `spec.sovereignty.dataClassification` is one of: public, internal, confidential, restricted

**ResourceType:**
- `metadata.name` follows `category.technology` dot notation
- `spec.version` is valid semver
- `spec.lifecycle` is one of: draft, stable, deprecated
- If `lifecycle == deprecated`, `spec.deprecation` must be present
- `spec.schema` is a valid structure (has `type: object` and `properties`)

**Recipe:**
- `spec.resourceType` is non-empty
- `spec.type` is one of: terraform, ansible, helm, kubernetes-operator, pulumi, custom
- `spec.source` is non-empty

**PlacementPolicy:**
- At least one match criterion is set, or `match.all: true`
- `spec.rule` and `spec.prefer` are non-empty strings if present (CEL syntax validation in Phase 4)
- `spec.weight` >= 0

### 3.2 Wire into API

- Validate on Create and Update in the API handlers.
- Return all validation errors at once (multi-error), not one at a time.

**Tests:** Table-driven tests with valid and invalid inputs for each resource type.

**Deliverable:** The API rejects malformed resources with clear error messages. The store only contains structurally valid data.

---

## Phase 4: OpenAPI Schema Validation

**Goal:** Parse ResourceType schemas (OpenAPI v3) and validate Application properties against them.

**Why now:** This is the critical compile-time safety gate from the design docs. Developers need to know immediately if their Application properties don't match the resource type contract.

### 4.1 Schema Parser

```
pkg/
  schema/
    parser.go           # Parse ResourceType spec.schema into structured form
    validator.go         # Validate Application properties against parsed schema
    schema_test.go
```

**`parser.go`** — Parse the `map[string]any` schema into a structured representation:

```go
type ParsedSchema struct {
    Properties map[string]PropertySchema
    Required   []string
}

type PropertySchema struct {
    Type        string          // string, integer, boolean, number, array, object
    Description string
    ReadOnly    bool            // true = output property
    Sensitive   bool            // x-dcm-sensitive
    Enum        []any
    Default     any
    Minimum     *float64
    Maximum     *float64
    Pattern     string
}

func Parse(raw map[string]any) (*ParsedSchema, error)
func (s *ParsedSchema) InputProperties() map[string]PropertySchema   // readOnly == false
func (s *ParsedSchema) OutputProperties() map[string]PropertySchema  // readOnly == true
```

### 4.2 Application-to-Schema Validation

**`validator.go`** — Cross-validates Application resources against registered ResourceTypes:

```go
func ValidateApplicationAgainstSchemas(app *Application, types map[string]*ResourceType) []error
```

Checks (from appspec.md Section 8):
- Every `resource.type` references a registered ResourceType
- ResourceType lifecycle is `stable` or `deprecated` (with warning)
- Every key in `properties` exists in the ResourceType's non-readOnly properties
- Property types match (string vs int vs bool)
- Required fields are present
- Values satisfy constraints (min, max, enum, pattern)

### 4.3 Wire into API

- On Application Create/Update: load all referenced ResourceTypes from store, validate properties.

**Tests:** Valid applications pass; invalid ones produce precise error messages for each violation.

**Deliverable:** Developers get immediate feedback on property errors. The system enforces the resource type contract.

---

## Phase 5: CEL Engine

**Goal:** Parse, extract references from, and evaluate CEL expressions used in Application wiring and Placement policies.

**Why now:** CEL is the connective tissue of the entire system. The DAG builder needs CEL reference extraction. The placement engine needs CEL evaluation. This is the prerequisite for both.

**Dependency:** `github.com/google/cel-go`

### 5.1 Expression Parser

```
pkg/
  cel/
    parser.go           # Extract ${...} expressions from property values
    environment.go      # CEL environment setup with DCM-specific types
    evaluator.go        # Evaluate CEL expressions against data
    references.go       # Extract cross-resource references from expressions
    cel_test.go
```

**`parser.go`** — Extract `${...}` expressions from arbitrary values:

```go
// ExtractExpressions finds all ${...} patterns in a value (string or nested map).
func ExtractExpressions(value any) []Expression

type Expression struct {
    Raw      string   // The full ${...} string
    CEL      string   // The inner expression (without ${})
    Path     string   // Where in the properties tree this was found
}
```

**`references.go`** — Extract cross-resource references:

```go
// ExtractReferences returns the resource names referenced by CEL expressions.
// e.g., "${db.host}" -> references resource "db"
func ExtractReferences(expressions []Expression) []string
```

### 5.2 CEL Environment for Placement

**`environment.go`** — Define the CEL type environment for placement policies:

```go
// NewPlacementEnv creates a CEL environment with the `env` variable
// representing a DCM Environment object.
func NewPlacementEnv() (*cel.Env, error)
```

The `env` variable exposes the full Environment struct to CEL:
- `env.sovereignty.jurisdiction`, `env.sovereignty.country`, etc.
- `env.capacity.cpu.available`, `env.capacity.memory.total`, etc.
- `env.cost.rates.cpu.value`, etc.
- `env.networking.features`, `env.networking.zones`, etc.
- `env.status.staleness`

**`evaluator.go`** — Evaluate compiled CEL programs:

```go
// EvalBool evaluates a CEL expression that should return a boolean (for rules).
func EvalBool(program cel.Program, vars map[string]any) (bool, error)

// EvalFloat evaluates a CEL expression that should return a number (for preferences).
func EvalFloat(program cel.Program, vars map[string]any) (float64, error)
```

### 5.3 CEL Validation in Application

- On Application create/update: parse all `${}` expressions, validate CEL syntax.
- Validate cross-resource references against ResourceType output schemas (e.g., `${db.host}` — verify `host` is a readOnly property of the `database.postgresql` ResourceType).

**Tests:**
- Parse `${db.host}` -> resource "db", field "host"
- Parse `"postgres://${db.host}:${db.port}/mydb"` -> resource "db", fields "host" and "port"
- Evaluate placement rules against sample Environment objects
- Invalid CEL syntax produces clear errors

**Deliverable:** CEL expressions can be parsed, validated, and evaluated. The foundation for DAG building and placement.

---

## Phase 6: DAG Builder

**Goal:** Build and validate the dependency graph for Application resources.

**Why now:** The DAG determines provisioning order. It's needed before the execution engine can run anything.

### 6.1 DAG Package

```
pkg/
  dag/
    dag.go              # DAG data structure and operations
    builder.go          # Build DAG from Application resources
    dag_test.go
```

**`dag.go`** — Generic DAG with topological sort and cycle detection:

```go
type DAG struct { ... }

func New() *DAG
func (d *DAG) AddNode(id string)
func (d *DAG) AddEdge(from, to string)         // from depends on to
func (d *DAG) TopologicalSort() ([][]string, error)  // Returns levels for parallel execution
func (d *DAG) DetectCycle() ([]string, bool)    // Returns cycle path if found
func (d *DAG) Dependents(id string) []string    // What depends on this node
func (d *DAG) Dependencies(id string) []string  // What this node depends on
```

**`builder.go`** — Build DAG from an Application:

```go
// Build creates a dependency DAG from an Application's resources.
// Dependencies come from:
//   1. Implicit: CEL cross-resource references (${db.host})
//   2. Explicit: requirements field
func Build(app *v1alpha1.Application) (*DAG, error)
```

Returns errors for:
- Circular dependencies (with cycle path)
- References to nonexistent resources
- Self-references

### 6.2 TopologicalSort Returns Levels

The sort returns `[][]string` — groups of resources that can be provisioned in parallel:

```
Level 0: [db]           <- no dependencies, provision first
Level 1: [cache, api]   <- depend on db, can run in parallel
Level 2: [frontend]     <- depends on api
```

**Tests:**
- Linear chain: a -> b -> c
- Diamond: a -> b, a -> c, b -> d, c -> d
- Parallel: a, b, c with no dependencies
- Cycle detection: a -> b -> c -> a
- Mixed implicit (CEL) + explicit (requirements) dependencies

**Deliverable:** Any Application can be converted into a validated, topologically sorted execution plan.

---

## Phase 7: Placement Engine

**Goal:** Select the best Environment for each resource in an Application based on PlacementPolicies.

**Why now:** Placement must happen before execution. Resources need to know where they'll be provisioned.

**Depends on:** Phase 5 (CEL Engine)

### 7.1 Placement Package

```
pkg/
  placement/
    engine.go           # Main placement engine
    prefilter.go        # Resource type superset check
    matcher.go          # Match policies to applications/resources
    scorer.go           # Score environments using prefer expressions
    connectivity.go     # Overlay connectivity graph
    decision.go         # Placement decision log (audit)
    placement_test.go
```

### 7.2 Placement Pipeline (from placement.md Section 3)

**`engine.go`:**

```go
type Engine struct {
    envRepo    *Repository[*Environment]
    policyRepo *Repository[*PlacementPolicy]
    celEnv     *cel.Env
}

type PlacementResult struct {
    // resource name -> environment name
    Assignments map[string]string
    // Full decision log for auditability
    DecisionLog []DecisionEntry
}

func (e *Engine) Place(ctx context.Context, app *Application) (*PlacementResult, error)
```

Pipeline steps:
1. Extract required resource types from Application
2. Load all Environments
3. **Pre-filter** (`prefilter.go`): discard environments whose `capabilities.resourceTypes` is not a superset of required types
4. **Match policies** (`matcher.go`): find PlacementPolicies matching the Application (by labels, resourceTypes, or `match.all`)
5. **Evaluate rules** (`engine.go`): for each candidate environment, evaluate all matching policies' `rule` CEL expressions (AND semantics)
6. **Score preferences** (`scorer.go`): for surviving environments, evaluate `prefer` expressions, compute weighted sum
7. **Rank and select**: sort by score descending, break ties by name

### 7.3 Per-Resource Placement (from multi-env.md Section 3)

Each resource is placed independently:
- Resource labels merge with Application labels (resource labels take precedence)
- The pipeline runs per-resource, not per-Application

### 7.4 Connectivity Validation (from multi-env.md Section 4)

**`connectivity.go`:**

```go
type ConnectivityGraph struct { ... }

func BuildConnectivityGraph(envs []*Environment) *ConnectivityGraph
func (g *ConnectivityGraph) Connected(envA, envB string) bool
```

After all resources are placed:
- For each DAG edge (A depends on B): verify the environments of A and B are connected (same environment, or share an overlay)
- If any edge lacks connectivity, placement fails with a detailed error

### 7.5 Decision Log

**`decision.go`:**

```go
type DecisionEntry struct {
    Resource    string
    Environment string
    Eligible    bool
    Scores      map[string]float64   // policy name -> score
    Eliminated  string               // reason if not eligible
}
```

Every placement decision is logged for auditability.

**Tests:**
- Single environment, all resources fit -> placed there
- Multiple environments, policies select the right one
- Pre-filter eliminates environments missing resource types
- Rule eliminates non-compliant environments
- Prefer selects highest-scoring environment
- Tie-breaking by name
- Cross-environment connectivity validated
- Connectivity failure produces clear error

**Deliverable:** Given an Application and the current state of Environments + Policies, the engine produces a deterministic, auditable placement decision.

---

## Phase 8: Execution Engine

**Goal:** Provision resources by invoking Recipes in the correct order, passing properties and collecting outputs.

**Why now:** This is where the system does real work. All prior phases were validation and decision-making.

**Depends on:** Phase 6 (DAG), Phase 7 (Placement)

### 8.1 Execution Package

```
pkg/
  engine/
    engine.go           # Orchestrates DAG execution
    context.go          # Builds the DCM context object injected into recipes
    result.go           # Result validation against ResourceType schema
    state.go            # Tracks execution state and resource outputs
    engine_test.go
  engine/drivers/
    driver.go           # Recipe driver interface
    terraform.go        # Terraform driver
    helm.go             # Helm driver
    ansible.go          # Ansible driver
    mock.go             # Mock driver for testing
```

### 8.2 Recipe Driver Interface

**`driver.go`:**

```go
type Driver interface {
    // Execute runs a recipe and returns the result.
    Execute(ctx context.Context, invocation *Invocation) (*Result, error)
    // Destroy tears down a previously provisioned resource.
    Destroy(ctx context.Context, invocation *Invocation) error
}

type Invocation struct {
    ResourceName string
    ResourceType string
    Recipe       RecipeBinding
    Properties   map[string]any      // merged: recipe params + developer properties
    Context      map[string]any      // DCM-injected context
}

type Result struct {
    Values  map[string]any   // non-sensitive outputs
    Secrets map[string]any   // sensitive outputs
}
```

### 8.3 Execution Orchestrator

**`engine.go`:**

```go
type Engine struct {
    drivers  map[string]Driver   // recipe type -> driver
    store    store.Store
}

func (e *Engine) Execute(ctx context.Context, plan *ExecutionPlan) (*ExecutionResult, error)
```

Execution flow:
1. Receive DAG levels from the DAG builder + placement assignments
2. For each level (in order):
   a. For each resource in the level (in parallel):
      - Resolve Recipe for (resourceType, environment) pair
      - Build context object (resource name, app name, environment name/type)
      - Merge parameters: recipe defaults + developer properties (developer wins)
      - Invoke Recipe driver
      - Validate result against ResourceType readOnly schema
      - Store outputs in execution state
   b. Wait for all resources in level to complete
3. Make outputs available for CEL resolution in subsequent levels

### 8.4 Context Object (from resource-type.md Section 6)

**`context.go`:**

```go
func BuildContext(resource ResourceDecl, app *Application, env *Environment) map[string]any
```

Returns:
```json
{
  "resource": { "name": "my-db" },
  "application": { "name": "my-web-app" },
  "environment": { "name": "prod-eu-k8s-01", "type": "kubernetes" }
}
```

### 8.5 Result Validation

**`result.go`:**

```go
func ValidateResult(result *Result, schema *ParsedSchema) error
```

Checks:
- Every readOnly property has a value in either `values` or `secrets`
- Sensitive properties (`x-dcm-sensitive`) are in `secrets`, not `values`
- Types match the schema

### 8.6 Start with Mock Driver

Implement a `MockDriver` first that returns predefined outputs. Real drivers (Terraform, Helm, Ansible) are added incrementally.

**Tests:**
- Linear DAG executes in order
- Parallel resources execute concurrently
- Outputs from level N are available to level N+1
- Recipe parameter merging (operator defaults + developer overrides)
- Result validation catches missing or misplaced outputs
- Driver failure stops execution and reports error

**Deliverable:** An Application can be fully "provisioned" (with mock drivers) end-to-end: validate -> DAG -> place -> execute -> collect outputs.

---

## Phase 9: Application Controller

**Goal:** Watch for Application changes in the store and automatically trigger the validation -> DAG -> placement -> execution pipeline.

**Why now:** Until now, execution was manual. The controller closes the loop for automated processing.

**Depends on:** Phase 6, 7, 8

### 9.1 Controller Package

```
pkg/
  controller/
    application.go      # Application controller
    controller.go       # Base controller loop (watch + reconcile pattern)
    status.go           # Application status types and updates
    controller_test.go
```

### 9.2 Reconciliation Loop

**`controller.go`** — Generic watch-reconcile loop:

```go
type Reconciler interface {
    Reconcile(ctx context.Context, name string) error
}

type Controller struct {
    store      store.Store
    prefix     string
    reconciler Reconciler
}

func (c *Controller) Run(ctx context.Context) error
```

### 9.3 Application Reconciler

**`application.go`:**

```go
type ApplicationReconciler struct {
    appRepo    *Repository[*Application]
    engine     *engine.Engine
    placement  *placement.Engine
}

func (r *ApplicationReconciler) Reconcile(ctx context.Context, name string) error
```

Reconciliation steps:
1. Get Application from store
2. Validate (structural + schema)
3. Build DAG
4. Run placement
5. Execute (provision resources)
6. Update Application status

### 9.4 Application Status

Add status tracking to Applications:

```go
type ApplicationStatus struct {
    Phase      string            // Pending, Validating, Placing, Provisioning, Ready, Failed
    Resources  []ResourceStatus
    Message    string
    LastUpdate time.Time
}

type ResourceStatus struct {
    Name        string
    Phase       string            // Pending, Provisioning, Ready, Failed
    Environment string
    Outputs     map[string]any
    Message     string
}
```

**Tests:**
- Controller picks up new Application, runs full pipeline
- Controller handles updates (re-validate, re-place if needed)
- Failed validation sets status to Failed with errors
- Successful provisioning sets status to Ready with outputs

**Deliverable:** Creating an Application in the API automatically triggers provisioning. Status is visible via the API.

---

## Phase 10: Recipe Drivers (Terraform, Helm)

**Goal:** Replace the mock driver with real IaC execution.

**Why now:** The orchestration is solid and tested. Now wire it to real provisioners.

**Depends on:** Phase 8

### 10.1 Terraform Driver

```
pkg/
  engine/drivers/
    terraform.go        # Terraform init/plan/apply
    terraform_test.go
```

- Download module from source
- Write `terraform.tfvars.json` with properties + context
- Run `terraform init`, `terraform apply -auto-approve`
- Parse `terraform output -json` to extract the `result` object
- Wrap result into `Result{Values, Secrets}`

### 10.2 Helm Driver

```
pkg/
  engine/drivers/
    helm.go             # Helm install/upgrade
    helm_test.go
```

- Build `values.yaml` from properties + context
- Run `helm install` / `helm upgrade`
- Read result from a designated ConfigMap/Secret in the target namespace

### 10.3 Ansible Driver

```
pkg/
  engine/drivers/
    ansible.go          # Ansible playbook runner
    ansible_test.go
```

- Write variables file with properties + context
- Run `ansible-playbook` with the extra vars
- Parse the `result` fact from Ansible output

**Tests:** Integration tests with real tools (can be skipped in CI if tools not installed, use build tags).

**Deliverable:** Real infrastructure can be provisioned.

---

## Phase 11: Git Controller

**Goal:** Watch Git repositories for Application YAML files and sync them to the store.

**Why now:** GitOps is a primary interface for developers. This enables the "commit to deploy" workflow.

**Depends on:** Phase 9 (Application Controller handles the downstream work)

### 11.1 Git Package

```
pkg/
  gitops/
    controller.go       # Git polling/webhook controller
    parser.go           # Discover and parse Application YAMLs from repo
    sync.go             # Diff and sync to store
    gitops_test.go
```

### 11.2 Git Sync Flow

1. Clone/pull repository on interval or webhook trigger
2. Walk the repo for `*.yaml` / `*.yml` files
3. Decode each file with the codec (skip non-DCM files)
4. For Applications: compare with store, create/update/delete as needed
5. For other types (Environments, ResourceTypes, etc.): same sync logic

### 11.3 Configuration

```yaml
apiVersion: dcm.io/v1alpha1
kind: GitRepository
metadata:
  name: my-app-repo
spec:
  url: "https://github.com/org/app-configs.git"
  branch: main
  path: "dcm/"
  interval: 60s
```

**Deliverable:** Committing an Application YAML to Git triggers automatic provisioning.

---

## Phase 12: Capacity Agent

**Goal:** Report dynamic capacity from environments to the control plane.

**Why now:** Placement policies reference `env.capacity.cpu.available` and `env.status.staleness`. Without live data, placement decisions are based on stale or missing capacity info.

### 12.1 Agent Binary

```
cmd/
  dcm-agent/
    main.go             # Agent entry point
```

### 12.2 Agent API Endpoint

Add to the control plane API:

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/apis/dcm.io/v1alpha1/environments/{name}/capacity` | Agent pushes capacity report |

### 12.3 Capacity Report

```go
type CapacityReport struct {
    CPU     DynamicCapacity `json:"cpu"`
    Memory  DynamicCapacity `json:"memory"`
    Storage DynamicCapacity `json:"storage"`
    GPU     DynamicCapacity `json:"gpu"`
}

type DynamicCapacity struct {
    Total     int    `json:"total"`
    Allocated int    `json:"allocated"`
    Available int    `json:"available"`
    Unit      string `json:"unit"`
}
```

### 12.4 Staleness Tracking

- Store `status.lastReported` timestamp on each capacity push
- Background goroutine checks staleness: if `now - lastReported > threshold`, set `status.staleness = "stale"`

**Deliverable:** Placement policies can make decisions based on real-time capacity data.

---

## Phase 13: Status, Events & Observability

**Goal:** Provide visibility into what the system is doing.

### 13.1 Event System

```
pkg/
  events/
    events.go           # Event types and emitter
    recorder.go         # Persists events to store
```

Events for:
- Application created/updated/deleted
- Validation succeeded/failed
- Placement succeeded/failed (with decision log)
- Provisioning started/succeeded/failed per resource
- Capacity report received
- Staleness threshold crossed

### 13.2 Status Sub-resource

Add `GET /apis/dcm.io/v1alpha1/applications/{name}/status` endpoint that returns provisioning state without the full spec.

### 13.3 Structured Logging

Replace `log.Printf` with structured logging (`log/slog`):
- Request logging middleware
- Controller reconciliation logging
- Placement decision logging

**Deliverable:** Operators can understand system behavior, debug placement decisions, and monitor provisioning progress.

---

## Phase 14: Policies (Mutation & Validation)

**Goal:** Enable Platform Engineers to enforce organizational standards via mutation and validation policies.

**Why now:** Deferred because the core loop works without them, but they're essential for production governance.

### 14.1 Policy Types

Add two new resource types:

```yaml
apiVersion: dcm.io/v1alpha1
kind: MutationPolicy
metadata:
  name: enforce-min-size
spec:
  match:
    labels:
      tier: production
  mutations:
    - resource:
        type: database.postgresql
      set:
        properties:
          multiAZ: true
```

```yaml
apiVersion: dcm.io/v1alpha1
kind: ValidationPolicy
metadata:
  name: require-multi-az-prod
spec:
  match:
    labels:
      tier: production
  rule: >-
    resources.filter(r, r.type == "database.postgresql")
      .all(r, r.properties.multiAZ == true)
  message: "Production databases must have multiAZ enabled"
```

### 14.2 Policy Engine

```
pkg/
  policy/
    mutation.go         # Apply mutation policies to Applications
    validation.go       # Evaluate validation policies against Applications
    policy_test.go
```

- Mutations run before validation in the Application pipeline
- Multiple mutation policies compose (applied in priority order)
- Validation rules are CEL expressions evaluated against the Application

**Deliverable:** Platform Engineers can enforce standards declaratively.

---

## Dependency Graph

```
Phase 0: Bootstrap (DONE)
    |
Phase 1: Typed Repository
    |
Phase 2: REST API
    |
Phase 3: Structural Validation
    |
Phase 4: Schema Validation ─────────────────────┐
    |                                            |
Phase 5: CEL Engine                              |
    |                                            |
    ├──────────────────┐                         |
    |                  |                         |
Phase 6: DAG Builder   Phase 7: Placement Engine |
    |                  |                         |
    └──────────────────┘                         |
              |                                  |
Phase 8: Execution Engine ──────────────────────┘
    |
    ├───────────────────────────────┐
    |                               |
Phase 9: Application Controller     Phase 10: Recipe Drivers
    |
    ├──────────────────┐
    |                  |
Phase 11: Git Controller   Phase 12: Capacity Agent
    |
Phase 13: Observability
    |
Phase 14: Policies
```

---

## Summary

| Phase | Scope | Key Deliverable |
|-------|-------|-----------------|
| 0 | Bootstrap | Types, store, codec, server skeleton |
| 1 | Typed Repository | Type-safe CRUD for all resources |
| 2 | REST API | HTTP endpoints for all personas |
| 3 | Structural Validation | Reject malformed resources at API boundary |
| 4 | Schema Validation | Validate Application properties against ResourceType schemas |
| 5 | CEL Engine | Parse, validate, evaluate CEL expressions |
| 6 | DAG Builder | Dependency graph with topological sort and cycle detection |
| 7 | Placement Engine | Environment selection via policies, connectivity validation |
| 8 | Execution Engine | Orchestrate resource provisioning in DAG order |
| 9 | Application Controller | Automated reconciliation loop |
| 10 | Recipe Drivers | Real Terraform/Helm/Ansible execution |
| 11 | Git Controller | GitOps-driven Application sync |
| 12 | Capacity Agent | Dynamic capacity reporting from environments |
| 13 | Observability | Events, status, structured logging |
| 14 | Policies | Mutation and validation policy enforcement |
