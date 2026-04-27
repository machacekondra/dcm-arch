package engine

import "context"

// Driver executes a Recipe to provision or destroy a resource.
type Driver interface {
	Execute(ctx context.Context, invocation *Invocation) (*Result, error)
	Destroy(ctx context.Context, invocation *Invocation) error
}

// Invocation holds all data needed to invoke a Recipe.
type Invocation struct {
	ResourceName string
	ResourceType string
	RecipeType   string            // terraform, helm, ansible, etc.
	Source       map[string]string  // recipe source location
	Properties   map[string]any    // merged: recipe params + developer properties
	Context      map[string]any    // DCM-injected context
}

// Result is the standardized output from a Recipe execution.
type Result struct {
	Values  map[string]any // non-sensitive outputs
	Secrets map[string]any // sensitive outputs (x-dcm-sensitive)
}
