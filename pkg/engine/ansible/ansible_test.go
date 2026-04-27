package ansible

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dcm-io/dcm/pkg/engine"
)

func hasAnsible() bool {
	_, err := exec.LookPath("ansible-playbook")
	return err == nil
}

func TestReadResult(t *testing.T) {
	dir := t.TempDir()
	resultFile := filepath.Join(dir, "result.json")

	data := map[string]any{
		"values": map[string]any{
			"host": "db.example.com",
			"port": 5432,
		},
		"secrets": map[string]any{
			"username": "admin",
			"password": "secret123",
		},
	}
	raw, _ := json.Marshal(data)
	os.WriteFile(resultFile, raw, 0o644)

	result, err := readResult(resultFile)
	if err != nil {
		t.Fatalf("readResult: %v", err)
	}
	if result.Values["host"] != "db.example.com" {
		t.Errorf("host: got %v", result.Values["host"])
	}
	if result.Secrets["password"] != "secret123" {
		t.Errorf("password: got %v", result.Secrets["password"])
	}
}

func TestReadResultNotFound(t *testing.T) {
	_, err := readResult("/nonexistent/result.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "result file not found") {
		t.Errorf("error should mention result file: %v", err)
	}
}

func TestReadResultInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "result.json")
	os.WriteFile(f, []byte("not json"), 0o644)

	_, err := readResult(f)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWriteResultCallback(t *testing.T) {
	dir := t.TempDir()
	callbackDir := filepath.Join(dir, "callback_plugins")
	os.MkdirAll(callbackDir, 0o755)

	err := writeResultCallback(callbackDir, "/tmp/result.json")
	if err != nil {
		t.Fatalf("writeResultCallback: %v", err)
	}

	pluginFile := filepath.Join(callbackDir, "dcm_result.py")
	data, err := os.ReadFile(pluginFile)
	if err != nil {
		t.Fatalf("read plugin: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "CallbackModule") {
		t.Error("plugin should contain CallbackModule class")
	}
	if !strings.Contains(content, "set_fact") {
		t.Error("plugin should handle set_fact")
	}
	if !strings.Contains(content, "result") {
		t.Error("plugin should look for 'result' fact")
	}
}

func TestResolvePlaybookLocal(t *testing.T) {
	dir := t.TempDir()

	// Create a local playbook
	playbook := filepath.Join(dir, "provision.yml")
	os.WriteFile(playbook, []byte("---\n- hosts: localhost\n"), 0o644)

	d := New("")
	path, err := d.resolvePlaybook(context.Background(), map[string]string{
		"playbook": playbook,
	}, dir)
	if err != nil {
		t.Fatalf("resolvePlaybook: %v", err)
	}
	if path != playbook {
		t.Errorf("path: got %q, want %q", path, playbook)
	}
}

func TestResolvePlaybookMissing(t *testing.T) {
	d := New("")
	_, err := d.resolvePlaybook(context.Background(), map[string]string{
		"playbook": "/nonexistent/playbook.yml",
	}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing playbook")
	}
}

func TestResolvePlaybookNoPlaybookKey(t *testing.T) {
	d := New("")
	_, err := d.resolvePlaybook(context.Background(), map[string]string{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing playbook key")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 100) != "short" {
		t.Error("should not truncate short string")
	}
	result := truncate("a very long string here", 10)
	if !strings.Contains(result, "truncated") {
		t.Error("should truncate long string")
	}
}

// --- Integration test (requires ansible-playbook) ---

func TestExecuteLocalPlaybook(t *testing.T) {
	if !hasAnsible() {
		t.Skip("ansible-playbook not installed")
	}

	dir := t.TempDir()

	// Write a minimal playbook that sets the result fact
	playbook := filepath.Join(dir, "provision.yml")
	playbookContent := `---
- name: DCM Test Provision
  hosts: localhost
  gather_facts: false
  tasks:
    - name: Set result
      ansible.builtin.set_fact:
        result:
          values:
            host: "test-db.local"
            port: 5432
            connectionString: "postgres://test-db.local:5432/mydb"
          secrets:
            username: "admin"
            password: "test-password-123"
`
	os.WriteFile(playbook, []byte(playbookContent), 0o644)

	d := New(dir)
	inv := &engine.Invocation{
		ResourceName: "my-db",
		ResourceType: "database.postgresql",
		RecipeType:   "ansible",
		Source: map[string]string{
			"playbook": playbook,
		},
		Properties: map[string]any{
			"size":    "M",
			"version": "16",
		},
		Context: map[string]any{
			"resource":    map[string]any{"name": "my-db"},
			"application": map[string]any{"name": "test-app"},
			"environment": map[string]any{"name": "prod-eu", "type": "bare-metal"},
		},
	}

	result, err := d.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify outputs
	if result.Values["host"] != "test-db.local" {
		t.Errorf("host: got %v, want test-db.local", result.Values["host"])
	}
	if result.Values["port"] != float64(5432) { // JSON numbers are float64
		t.Errorf("port: got %v (%T)", result.Values["port"], result.Values["port"])
	}
	if result.Values["connectionString"] != "postgres://test-db.local:5432/mydb" {
		t.Errorf("connectionString: got %v", result.Values["connectionString"])
	}
	if result.Secrets["username"] != "admin" {
		t.Errorf("username: got %v", result.Secrets["username"])
	}
	if result.Secrets["password"] != "test-password-123" {
		t.Errorf("password: got %v", result.Secrets["password"])
	}
}

func TestExecutePlaybookWithProperties(t *testing.T) {
	if !hasAnsible() {
		t.Skip("ansible-playbook not installed")
	}

	dir := t.TempDir()

	// Playbook that uses properties passed via extra vars
	playbook := filepath.Join(dir, "provision.yml")
	playbookContent := `---
- name: DCM Test with Properties
  hosts: localhost
  gather_facts: false
  tasks:
    - name: Set result using properties
      ansible.builtin.set_fact:
        result:
          values:
            host: "db-{{ size }}.local"
            port: 5432
          secrets:
            username: "{{ dcm_context.resource.name }}_admin"
            password: "generated"
`
	os.WriteFile(playbook, []byte(playbookContent), 0o644)

	d := New(dir)
	inv := &engine.Invocation{
		ResourceName: "my-db",
		ResourceType: "database.postgresql",
		RecipeType:   "ansible",
		Source:       map[string]string{"playbook": playbook},
		Properties:   map[string]any{"size": "L"},
		Context: map[string]any{
			"resource":    map[string]any{"name": "my-db"},
			"application": map[string]any{"name": "test-app"},
			"environment": map[string]any{"name": "prod", "type": "bare-metal"},
		},
	}

	result, err := d.Execute(context.Background(), inv)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if result.Values["host"] != "db-L.local" {
		t.Errorf("host: got %v, want db-L.local", result.Values["host"])
	}
	if result.Secrets["username"] != "my-db_admin" {
		t.Errorf("username: got %v, want my-db_admin", result.Secrets["username"])
	}
}

func TestExecutePlaybookFailure(t *testing.T) {
	if !hasAnsible() {
		t.Skip("ansible-playbook not installed")
	}

	dir := t.TempDir()

	// Playbook that fails
	playbook := filepath.Join(dir, "bad.yml")
	os.WriteFile(playbook, []byte(`---
- name: Fail
  hosts: localhost
  gather_facts: false
  tasks:
    - name: This will fail
      ansible.builtin.fail:
        msg: "intentional failure"
`), 0o644)

	d := New(dir)
	inv := &engine.Invocation{
		ResourceName: "fail-resource",
		RecipeType:   "ansible",
		Source:       map[string]string{"playbook": playbook},
		Properties:   map[string]any{},
		Context:      map[string]any{},
	}

	_, err := d.Execute(context.Background(), inv)
	if err == nil {
		t.Fatal("expected error from failing playbook")
	}
	if !strings.Contains(err.Error(), "ansible-playbook failed") {
		t.Errorf("error should mention ansible-playbook: %v", err)
	}
}

func TestExecuteNoResult(t *testing.T) {
	if !hasAnsible() {
		t.Skip("ansible-playbook not installed")
	}

	dir := t.TempDir()

	// Playbook that doesn't set the result fact
	playbook := filepath.Join(dir, "noresult.yml")
	os.WriteFile(playbook, []byte(`---
- name: No result
  hosts: localhost
  gather_facts: false
  tasks:
    - name: Do nothing useful
      ansible.builtin.debug:
        msg: "hello"
`), 0o644)

	d := New(dir)
	inv := &engine.Invocation{
		ResourceName: "no-result",
		RecipeType:   "ansible",
		Source:       map[string]string{"playbook": playbook},
		Properties:   map[string]any{},
		Context:      map[string]any{},
	}

	_, err := d.Execute(context.Background(), inv)
	if err == nil {
		t.Fatal("expected error when result fact is not set")
	}
	if !strings.Contains(err.Error(), "result file not found") {
		t.Errorf("error should mention result: %v", err)
	}
}
