package placement

import "github.com/dcm-io/dcm/pkg/apis/v1alpha1"

// PrefilterByResourceType returns environments whose capabilities.resourceTypes
// contain the required resource type. This is the built-in hard constraint
// that always applies regardless of policies.
func PrefilterByResourceType(envs []*v1alpha1.Environment, resourceType string) []*v1alpha1.Environment {
	var result []*v1alpha1.Environment
	for _, env := range envs {
		if supportsResourceType(env, resourceType) {
			result = append(result, env)
		}
	}
	return result
}

func supportsResourceType(env *v1alpha1.Environment, resourceType string) bool {
	for _, rt := range env.Spec.Capabilities.ResourceTypes {
		if rt == resourceType {
			return true
		}
	}
	return false
}
