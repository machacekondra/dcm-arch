package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	kinestore "github.com/dcm-io/dcm/pkg/store/kine"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir, err := os.MkdirTemp("", "dcm-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s, err := kinestore.New(context.Background(), kinestore.Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	return httptest.NewServer(mux)
}

const pgResourceTypeYAML = `apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: database.postgresql
spec:
  version: "1.0.0"
  lifecycle: stable
  schema:
    type: object
    required:
      - size
    properties:
      size:
        type: string
        enum: ["XS", "S", "M", "L", "XL"]
        default: "S"
      storageGB:
        type: integer
        minimum: 10
        maximum: 10000
        default: 50
      multiAZ:
        type: boolean
        default: false
      host:
        type: string
        readOnly: true
      port:
        type: integer
        readOnly: true
      connectionString:
        type: string
        readOnly: true
`

// registerPostgresType registers the database.postgresql ResourceType for tests
// that need to create Applications referencing it.
func registerPostgresType(t *testing.T, ts *httptest.Server) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/resourcetypes", "application/yaml", strings.NewReader(pgResourceTypeYAML))
	if err != nil {
		t.Fatalf("register ResourceType: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register ResourceType: got %d", resp.StatusCode)
	}
}

const appYAML = `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: test-app
spec:
  resources:
    - type: database.postgresql
      name: my-db
      properties:
        size: S
`

const appJSON = `{
  "apiVersion": "dcm.io/v1alpha1",
  "kind": "Application",
  "metadata": {"name": "test-app"},
  "spec": {
    "resources": [
      {"type": "database.postgresql", "name": "my-db", "properties": {"size": "S"}}
    ]
  }
}`

func TestCreateAndGetApplication(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create via YAML
	resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST status: got %d, want %d, body: %s", resp.StatusCode, http.StatusCreated, body)
	}

	rev := resp.Header.Get("X-DCM-Revision")
	if rev == "" {
		t.Fatal("expected X-DCM-Revision header")
	}

	// Get
	resp, err = http.Get(ts.URL + "/apis/dcm.io/v1alpha1/applications/test-app")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	metadata := result["metadata"].(map[string]any)
	if metadata["name"] != "test-app" {
		t.Errorf("name: got %v, want test-app", metadata["name"])
	}
}

func TestCreateApplicationJSON(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/json", strings.NewReader(appJSON))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST status: got %d, want %d, body: %s", resp.StatusCode, http.StatusCreated, body)
	}
}

func TestCreateDuplicate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// First create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	resp.Body.Close()

	// Duplicate
	resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestGetNotFound(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/apis/dcm.io/v1alpha1/applications/nonexistent")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestListApplications(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create two apps
	for _, name := range []string{"app-a", "app-b"} {
		body := strings.Replace(appYAML, "test-app", name, 1)
		resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", name, err)
		}
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/apis/dcm.io/v1alpha1/applications")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result struct {
		Kind  string           `json:"kind"`
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Kind != "ApplicationList" {
		t.Errorf("kind: got %q, want ApplicationList", result.Kind)
	}
	if len(result.Items) != 2 {
		t.Errorf("items: got %d, want 2", len(result.Items))
	}
}

func TestUpdateApplication(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	rev := resp.Header.Get("X-DCM-Revision")
	resp.Body.Close()

	// Update
	updatedYAML := strings.Replace(appYAML, `size: S`, `size: M`, 1)
	req, _ := http.NewRequest("PUT", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", strings.NewReader(updatedYAML))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("X-DCM-Revision", rev)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status: got %d, want %d, body: %s", resp.StatusCode, http.StatusOK, body)
	}

	newRev := resp.Header.Get("X-DCM-Revision")
	if newRev == "" || newRev == rev {
		t.Error("expected new revision after update")
	}
}

func TestUpdateConflict(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	resp.Body.Close()

	// Update with stale revision
	req, _ := http.NewRequest("PUT", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", strings.NewReader(appYAML))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("X-DCM-Revision", "9999")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestUpdateMissingRevision(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", strings.NewReader(appYAML))
	req.Header.Set("Content-Type", "application/yaml")
	// No X-DCM-Revision header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestUpdateNameMismatch(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	rev := resp.Header.Get("X-DCM-Revision")
	resp.Body.Close()

	// Update with mismatched name
	req, _ := http.NewRequest("PUT", ts.URL+"/apis/dcm.io/v1alpha1/applications/wrong-name", strings.NewReader(appYAML))
	req.Header.Set("Content-Type", "application/yaml")
	req.Header.Set("X-DCM-Revision", rev)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestDeleteApplication(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	rev := resp.Header.Get("X-DCM-Revision")
	resp.Body.Close()

	// Delete
	req, _ := http.NewRequest("DELETE", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", nil)
	req.Header.Set("X-DCM-Revision", rev)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Verify gone
	resp, _ = http.Get(ts.URL + "/apis/dcm.io/v1alpha1/applications/test-app")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestDeleteConflict(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	resp.Body.Close()

	// Delete with stale revision
	req, _ := http.NewRequest("DELETE", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", nil)
	req.Header.Set("X-DCM-Revision", "9999")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestGetYAMLResponse(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	resp.Body.Close()

	// Get with Accept: application/yaml
	req, _ := http.NewRequest("GET", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", nil)
	req.Header.Set("Accept", "application/yaml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "application/yaml" {
		t.Errorf("content-type: got %q, want application/yaml", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "apiVersion:") {
		t.Error("expected YAML output with apiVersion key")
	}
}

func TestEnvironmentCRUD(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	envYAML := `apiVersion: dcm.io/v1alpha1
kind: Environment
metadata:
  name: prod-eu
  labels:
    tier: production
spec:
  type: kubernetes
  connection:
    endpoint: "https://k8s.example.com:6443"
    credentialRef: "vault:secret/k8s"
  capabilities:
    resourceTypes:
      - compute.container
  sovereignty:
    country: DE
    region: eu-central-1
    jurisdiction: EU
    dataClassification: confidential
`
	// Create
	resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/environments", "application/yaml", strings.NewReader(envYAML))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	// Get
	resp, err = http.Get(ts.URL + "/apis/dcm.io/v1alpha1/environments/prod-eu")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestResourceTypeCRUD(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	rtYAML := `apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: database.postgresql
spec:
  version: "1.0.0"
  lifecycle: stable
  schema:
    type: object
    properties:
      size:
        type: string
`
	resp, err := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/resourcetypes", "application/yaml", strings.NewReader(rtYAML))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	resp, _ = http.Get(ts.URL + "/apis/dcm.io/v1alpha1/resourcetypes/database.postgresql")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestRecipeCRUD(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	recipeYAML := `apiVersion: dcm.io/v1alpha1
kind: Recipe
metadata:
  name: pg-terraform
spec:
  resourceType: database.postgresql
  type: terraform
  source:
    module: dcm-modules/rds-postgresql/aws
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/recipes", "application/yaml", strings.NewReader(recipeYAML))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestPlacementPolicyCRUD(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	ppYAML := `apiVersion: dcm.io/v1alpha1
kind: PlacementPolicy
metadata:
  name: default
spec:
  match:
    all: true
  rule: 'env.status.staleness == "fresh"'
  weight: 0.5
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/placementpolicies", "application/yaml", strings.NewReader(ppYAML))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestValidationRejectsInvalidApplication(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Missing resources
	invalidApp := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: bad-app
spec:
  resources: []
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(invalidApp))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want %d, body: %s", resp.StatusCode, http.StatusBadRequest, body)
	}

	var apiErr struct{ Error string }
	json.NewDecoder(resp.Body).Decode(&apiErr)
	if !strings.Contains(apiErr.Error, "at least one resource") {
		t.Errorf("error should mention resources, got: %s", apiErr.Error)
	}
}

func TestValidationRejectsInvalidEnvironment(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	invalidEnv := `apiVersion: dcm.io/v1alpha1
kind: Environment
metadata:
  name: bad-env
spec:
  type: mainframe
  connection:
    endpoint: "not-a-url"
    credentialRef: ""
  capabilities:
    resourceTypes: []
  sovereignty:
    country: GERMANY
    region: ""
    jurisdiction: ""
    dataClassification: top-secret
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/environments", "application/yaml", strings.NewReader(invalidEnv))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want %d, body: %s", resp.StatusCode, http.StatusBadRequest, body)
	}
}

func TestValidationRejectsInvalidResourceType(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	invalidRT := `apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: nodot
spec:
  version: "bad"
  lifecycle: "beta"
  schema:
    type: array
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/resourcetypes", "application/yaml", strings.NewReader(invalidRT))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestValidationOnUpdate(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	// Create valid app
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	rev := resp.Header.Get("X-DCM-Revision")
	resp.Body.Close()

	// Update with invalid data (empty resources)
	invalidUpdate := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: test-app
spec:
  resources: []
`
	req, _ := http.NewRequest("PUT", ts.URL+"/apis/dcm.io/v1alpha1/applications/test-app", strings.NewReader(invalidUpdate))
	req.Header.Set("X-DCM-Revision", rev)
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestInvalidBody(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader("not valid yaml: [[["))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSchemaValidationEndToEnd(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// 1. Register a ResourceType
	rtYAML := `apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: database.postgresql
spec:
  version: "1.0.0"
  lifecycle: stable
  schema:
    type: object
    required:
      - size
    properties:
      size:
        type: string
        enum: ["S", "M", "L"]
      storageGB:
        type: integer
        minimum: 10
        maximum: 10000
      host:
        type: string
        readOnly: true
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/resourcetypes", "application/yaml", strings.NewReader(rtYAML))
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create RT: got %d", resp.StatusCode)
	}

	// 2. Create Application with valid properties — should succeed
	validAppYAML := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: good-app
spec:
  resources:
    - type: database.postgresql
      name: db
      properties:
        size: M
        storageGB: 100
`
	resp, _ = http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(validAppYAML))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create valid app: got %d, body: %s", resp.StatusCode, body)
	}

	// 3. Create Application with invalid enum — should fail
	badEnumYAML := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: bad-enum-app
spec:
  resources:
    - type: database.postgresql
      name: db
      properties:
        size: XXL
`
	resp, _ = http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(badEnumYAML))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad enum: got %d, want 400", resp.StatusCode)
	}

	// 4. Create Application setting readOnly property — should fail
	readOnlyYAML := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: readonly-app
spec:
  resources:
    - type: database.postgresql
      name: db
      properties:
        size: S
        host: "hacker.example.com"
`
	resp, _ = http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(readOnlyYAML))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("readOnly: got %d, want 400", resp.StatusCode)
	}

	// 5. Create Application with storageGB below minimum — should fail
	belowMinYAML := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: belowmin-app
spec:
  resources:
    - type: database.postgresql
      name: db
      properties:
        size: S
        storageGB: 5
`
	resp, _ = http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(belowMinYAML))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("below min: got %d, want 400", resp.StatusCode)
	}
}

func TestSchemaValidationUnknownResourceType(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	// Create app referencing unregistered resource type
	appYAML := `apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: unknown-type-app
spec:
  resources:
    - type: cache.redis
      name: cache
      properties:
        memoryGB: 4
`
	resp, _ := http.Post(ts.URL+"/apis/dcm.io/v1alpha1/applications", "application/yaml", strings.NewReader(appYAML))
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown type: got %d, want 400, body: %s", resp.StatusCode, body)
	}
}

func TestFullLifecycle(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()
	registerPostgresType(t, ts)

	url := ts.URL + "/apis/dcm.io/v1alpha1/applications"

	// 1. List empty
	resp, _ := http.Get(url)
	var list struct{ Items []any }
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(list.Items))
	}

	// 2. Create
	resp, _ = http.Post(url, "application/yaml", strings.NewReader(appYAML))
	rev1, _ := strconv.ParseInt(resp.Header.Get("X-DCM-Revision"), 10, 64)
	resp.Body.Close()

	// 3. List has 1
	resp, _ = http.Get(url)
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}

	// 4. Update
	updated := strings.Replace(appYAML, `size: S`, `size: L`, 1)
	req, _ := http.NewRequest("PUT", url+"/test-app", strings.NewReader(updated))
	req.Header.Set("X-DCM-Revision", strconv.FormatInt(rev1, 10))
	resp, _ = http.DefaultClient.Do(req)
	rev2, _ := strconv.ParseInt(resp.Header.Get("X-DCM-Revision"), 10, 64)
	resp.Body.Close()

	if rev2 <= rev1 {
		t.Errorf("expected rev2 > rev1, got %d <= %d", rev2, rev1)
	}

	// 5. Delete
	req, _ = http.NewRequest("DELETE", url+"/test-app", nil)
	req.Header.Set("X-DCM-Revision", strconv.FormatInt(rev2, 10))
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()

	// 6. List empty again
	resp, _ = http.Get(url)
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Items) != 0 {
		t.Fatalf("expected 0 items after delete, got %d", len(list.Items))
	}
}
