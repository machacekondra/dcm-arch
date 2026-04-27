package validation

import (
	"fmt"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

// ValidateApplication checks structural validity of an Application.
func ValidateApplication(app *v1alpha1.Application) *Result {
	r := &Result{}

	validateObjectMeta(app.Metadata, r)

	if len(app.Spec.Resources) == 0 {
		r.AddError("spec.resources", "must contain at least one resource")
		return r
	}

	names := make(map[string]bool)
	for i, res := range app.Spec.Resources {
		field := func(f string) string {
			return fmt.Sprintf("spec.resources[%d].%s", i, f)
		}

		if res.Name == "" {
			r.AddError(field("name"), "is required")
		} else if names[res.Name] {
			r.AddErrorf(field("name"), "duplicate resource name %q", res.Name)
		} else {
			names[res.Name] = true
		}

		if res.Type == "" {
			r.AddError(field("type"), "is required")
		}
	}

	// Validate requirements reference existing resource names
	for i, res := range app.Spec.Resources {
		reqField := fmt.Sprintf("spec.resources[%d].requirements", i)
		for _, req := range res.Requirements {
			if !names[req] {
				r.AddErrorf(reqField, "references unknown resource %q", req)
			}
			if req == res.Name {
				r.AddErrorf(reqField, "resource %q cannot depend on itself", req)
			}
		}
	}

	return r
}
