package ansible

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dcm-io/dcm/pkg/engine"
)

// Driver executes Ansible playbook recipes.
type Driver struct {
	// WorkDir is the base directory for temporary execution files.
	// Defaults to os.TempDir() if empty.
	WorkDir string
}

// New creates an Ansible driver.
func New(workDir string) *Driver {
	if workDir == "" {
		workDir = os.TempDir()
	}
	return &Driver{WorkDir: workDir}
}

func (d *Driver) Execute(ctx context.Context, inv *engine.Invocation) (*engine.Result, error) {
	runDir, err := os.MkdirTemp(d.WorkDir, "dcm-ansible-*")
	if err != nil {
		return nil, fmt.Errorf("create run directory: %w", err)
	}
	defer os.RemoveAll(runDir)

	// Resolve playbook source
	playbookPath, err := d.resolvePlaybook(ctx, inv.Source, runDir)
	if err != nil {
		return nil, fmt.Errorf("resolve playbook: %w", err)
	}

	// Build extra vars: properties + dcm_context
	extraVars := make(map[string]any)
	for k, v := range inv.Properties {
		extraVars[k] = v
	}
	extraVars["dcm_context"] = inv.Context

	// Write extra vars file
	varsFile := filepath.Join(runDir, "extra_vars.json")
	varsData, err := json.Marshal(extraVars)
	if err != nil {
		return nil, fmt.Errorf("marshal extra vars: %w", err)
	}
	if err := os.WriteFile(varsFile, varsData, 0o600); err != nil {
		return nil, fmt.Errorf("write extra vars: %w", err)
	}

	// Write callback plugin to capture the result fact
	callbackDir := filepath.Join(runDir, "callback_plugins")
	if err := os.MkdirAll(callbackDir, 0o755); err != nil {
		return nil, fmt.Errorf("create callback dir: %w", err)
	}
	resultFile := filepath.Join(runDir, "dcm_result.json")
	if err := writeResultCallback(callbackDir, resultFile); err != nil {
		return nil, fmt.Errorf("write callback plugin: %w", err)
	}

	// Build ansible-playbook command
	args := []string{
		playbookPath,
		"--extra-vars", "@" + varsFile,
		"--connection", "local",
	}

	// If inventory is specified in source, use it
	if inv.Source["inventory"] != "" {
		args = append(args, "-i", inv.Source["inventory"])
	} else {
		// Default: localhost inventory
		inventoryFile := filepath.Join(runDir, "inventory")
		if err := os.WriteFile(inventoryFile, []byte("localhost ansible_connection=local\n"), 0o600); err != nil {
			return nil, fmt.Errorf("write inventory: %w", err)
		}
		args = append(args, "-i", inventoryFile)
	}

	cmd := exec.CommandContext(ctx, "ansible-playbook", args...)
	cmd.Dir = runDir
	cmd.Env = append(os.Environ(),
		"ANSIBLE_CALLBACK_PLUGINS="+callbackDir,
		"ANSIBLE_STDOUT_CALLBACK=default",
		"DCM_RESULT_FILE="+resultFile,
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_RETRY_FILES_ENABLED=False",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ansible-playbook failed: %w\noutput:\n%s", err, truncate(string(output), 2000))
	}

	// Read result from callback output
	result, err := readResult(resultFile)
	if err != nil {
		return nil, fmt.Errorf("read result: %w\nplaybook output:\n%s", err, truncate(string(output), 1000))
	}

	return result, nil
}

func (d *Driver) Destroy(ctx context.Context, inv *engine.Invocation) error {
	// For destroy, look for a destroy playbook in the source
	destroyPlaybook := inv.Source["destroyPlaybook"]
	if destroyPlaybook == "" {
		// Convention: if playbook is "provision.yml", try "destroy.yml" in same dir
		playbook := inv.Source["playbook"]
		if playbook != "" {
			dir := filepath.Dir(playbook)
			destroyPlaybook = filepath.Join(dir, "destroy.yml")
		}
	}
	if destroyPlaybook == "" {
		return fmt.Errorf("no destroy playbook configured")
	}

	// Clone the source and swap the playbook
	destroySource := make(map[string]string)
	for k, v := range inv.Source {
		destroySource[k] = v
	}
	destroySource["playbook"] = destroyPlaybook

	destroyInv := &engine.Invocation{
		ResourceName: inv.ResourceName,
		ResourceType: inv.ResourceType,
		RecipeType:   inv.RecipeType,
		Source:        destroySource,
		Properties:    inv.Properties,
		Context:       inv.Context,
	}

	_, err := d.Execute(ctx, destroyInv)
	return err
}

// resolvePlaybook gets the playbook file path. If source has a repository,
// it clones it first. If it's a local path, it uses it directly.
func (d *Driver) resolvePlaybook(ctx context.Context, source map[string]string, runDir string) (string, error) {
	playbook := source["playbook"]
	if playbook == "" {
		return "", fmt.Errorf("source.playbook is required")
	}

	repo := source["repository"]
	if repo == "" {
		// Local playbook path — resolve relative paths against working directory
		if !filepath.IsAbs(playbook) {
			wd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("get working directory: %w", err)
			}
			playbook = filepath.Join(wd, playbook)
		}
		if _, err := os.Stat(playbook); err != nil {
			return "", fmt.Errorf("playbook not found: %s", playbook)
		}
		return playbook, nil
	}

	// Clone repository
	cloneDir := filepath.Join(runDir, "repo")
	cloneArgs := []string{"clone", "--depth", "1"}
	if version := source["version"]; version != "" {
		cloneArgs = append(cloneArgs, "--branch", version)
	}
	cloneArgs = append(cloneArgs, repo, cloneDir)

	cmd := exec.CommandContext(ctx, "git", cloneArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone failed: %w\n%s", err, truncate(string(output), 500))
	}

	fullPath := filepath.Join(cloneDir, playbook)
	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("playbook %q not found in repository", playbook)
	}

	return fullPath, nil
}

// readResult reads the DCM result JSON written by the callback plugin.
func readResult(path string) (*engine.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("result file not found — ensure the playbook sets the 'result' fact via set_fact")
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid result JSON: %w", err)
	}

	result := &engine.Result{
		Values:  make(map[string]any),
		Secrets: make(map[string]any),
	}

	if values, ok := raw["values"].(map[string]any); ok {
		result.Values = values
	}
	if secrets, ok := raw["secrets"].(map[string]any); ok {
		result.Secrets = secrets
	}

	return result, nil
}

// writeResultCallback writes a minimal Ansible callback plugin that
// captures the 'result' fact set by set_fact and writes it to a JSON file.
func writeResultCallback(dir, resultFile string) error {
	plugin := fmt.Sprintf(`import json
import os

from ansible.plugins.callback import CallbackBase


class CallbackModule(CallbackBase):
    CALLBACK_VERSION = 2.0
    CALLBACK_TYPE = 'aggregate'
    CALLBACK_NAME = 'dcm_result'

    def __init__(self):
        super().__init__()
        self.result_file = os.environ.get('DCM_RESULT_FILE', '%s')

    def v2_runner_on_ok(self, result):
        task_action = result._task.action
        if task_action in ('set_fact', 'ansible.builtin.set_fact'):
            facts = result._result.get('ansible_facts', {})
            if 'result' in facts:
                with open(self.result_file, 'w') as f:
                    json.dump(facts['result'], f)
`, strings.ReplaceAll(resultFile, `\`, `\\`))

	return os.WriteFile(filepath.Join(dir, "dcm_result.py"), []byte(plugin), 0o644)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}
