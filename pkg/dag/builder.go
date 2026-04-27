package dag

import (
	"fmt"

	dcmcel "github.com/dcm-io/dcm/pkg/cel"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

// Build creates a dependency DAG from an Application's resources.
// Dependencies come from:
//  1. Implicit: CEL cross-resource references (${db.host})
//  2. Explicit: requirements field
//
// Returns an error if resources reference nonexistent resources or form cycles.
func Build(app *v1alpha1.Application) (*DAG, error) {
	d := New()

	// Index resource names
	names := make(map[string]bool)
	for _, res := range app.Spec.Resources {
		if names[res.Name] {
			return nil, fmt.Errorf("duplicate resource name %q", res.Name)
		}
		names[res.Name] = true
		d.AddNode(res.Name)
	}

	var errors []string

	for _, res := range app.Spec.Resources {
		// Explicit dependencies from requirements
		for _, req := range res.Requirements {
			if !names[req] {
				errors = append(errors, fmt.Sprintf("resource %q requires unknown resource %q", res.Name, req))
				continue
			}
			if req == res.Name {
				errors = append(errors, fmt.Sprintf("resource %q cannot depend on itself", res.Name))
				continue
			}
			d.AddEdge(res.Name, req)
		}

		// Implicit dependencies from CEL expressions
		exprs := dcmcel.ExtractAllExpressions(res.Properties)
		refNames := dcmcel.ExtractReferencedResourceNames(exprs)
		for _, refName := range refNames {
			if !names[refName] {
				errors = append(errors, fmt.Sprintf("resource %q references unknown resource %q via CEL expression", res.Name, refName))
				continue
			}
			if refName == res.Name {
				continue // self-references in CEL are allowed (e.g., default values)
			}
			d.AddEdge(res.Name, refName)
		}
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("dependency errors:\n  - %s", joinErrors(errors))
	}

	// Check for cycles
	if cycle := d.DetectCycle(); cycle != nil {
		return nil, fmt.Errorf("circular dependency detected: %s", formatCycle(cycle))
	}

	return d, nil
}

func joinErrors(errors []string) string {
	result := errors[0]
	for _, e := range errors[1:] {
		result += "\n  - " + e
	}
	return result
}
