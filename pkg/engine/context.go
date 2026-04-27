package engine

import "github.com/dcm-io/dcm/pkg/apis/v1alpha1"

// BuildContext creates the DCM context object injected into every Recipe
// invocation. It provides metadata about the resource, application, and
// target environment.
func BuildContext(resourceName string, app *v1alpha1.Application, env *v1alpha1.Environment) map[string]any {
	ctx := map[string]any{
		"resource": map[string]any{
			"name": resourceName,
		},
		"application": map[string]any{
			"name": app.Metadata.Name,
		},
	}

	if env != nil {
		ctx["environment"] = map[string]any{
			"name": env.Metadata.Name,
			"type": env.Spec.Type,
		}
	}

	return ctx
}
