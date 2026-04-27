package engine

import "fmt"

// ExecutionState tracks resource outputs during DAG execution. Outputs from
// provisioned resources are stored here so that dependent resources can
// resolve CEL cross-references.
type ExecutionState struct {
	outputs map[string]*Result // resourceName -> result
}

// NewExecutionState creates an empty execution state.
func NewExecutionState() *ExecutionState {
	return &ExecutionState{outputs: make(map[string]*Result)}
}

// SetOutput records the result of provisioning a resource.
func (s *ExecutionState) SetOutput(resourceName string, result *Result) {
	s.outputs[resourceName] = result
}

// GetOutput retrieves the provisioning result for a resource.
func (s *ExecutionState) GetOutput(resourceName string) (*Result, bool) {
	r, ok := s.outputs[resourceName]
	return r, ok
}

// ResolveOutputValue looks up a specific output field from a resource's result.
// It searches both values and secrets.
func (s *ExecutionState) ResolveOutputValue(resourceName, field string) (any, error) {
	result, ok := s.outputs[resourceName]
	if !ok {
		return nil, fmt.Errorf("resource %q has not been provisioned yet", resourceName)
	}

	if v, ok := result.Values[field]; ok {
		return v, nil
	}
	if v, ok := result.Secrets[field]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("resource %q has no output field %q", resourceName, field)
}
