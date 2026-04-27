package engine

import (
	"context"
	"fmt"
)

// MockDriver returns predefined outputs for testing. If no output is
// configured for a resource, it generates default outputs.
type MockDriver struct {
	// Results maps resourceName to the Result to return.
	Results map[string]*Result
	// Errors maps resourceName to an error to return.
	Errors map[string]error
	// Executed records the invocations in order.
	Executed []*Invocation
}

// NewMockDriver creates a MockDriver with empty result/error maps.
func NewMockDriver() *MockDriver {
	return &MockDriver{
		Results: make(map[string]*Result),
		Errors:  make(map[string]error),
	}
}

func (d *MockDriver) Execute(_ context.Context, inv *Invocation) (*Result, error) {
	d.Executed = append(d.Executed, inv)

	if err, ok := d.Errors[inv.ResourceName]; ok {
		return nil, err
	}

	if result, ok := d.Results[inv.ResourceName]; ok {
		return result, nil
	}

	// Generate default outputs
	return &Result{
		Values: map[string]any{
			"host": fmt.Sprintf("%s.default.svc.cluster.local", inv.ResourceName),
			"port": 5432,
		},
		Secrets: map[string]any{
			"username": "admin",
			"password": "generated-password",
		},
	}, nil
}

func (d *MockDriver) Destroy(_ context.Context, inv *Invocation) error {
	if err, ok := d.Errors[inv.ResourceName]; ok {
		return err
	}
	return nil
}
