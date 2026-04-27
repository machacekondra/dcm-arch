package placement

import "github.com/dcm-io/dcm/pkg/apis/v1alpha1"

// MatchPolicies returns the PlacementPolicies that apply to a resource
// given its effective labels and resource type.
func MatchPolicies(policies []*v1alpha1.PlacementPolicy, labels map[string]string, resourceType string) []*v1alpha1.PlacementPolicy {
	var matched []*v1alpha1.PlacementPolicy
	for _, p := range policies {
		if matchesPolicy(p, labels, resourceType) {
			matched = append(matched, p)
		}
	}
	return matched
}

func matchesPolicy(policy *v1alpha1.PlacementPolicy, labels map[string]string, resourceType string) bool {
	m := policy.Spec.Match

	// match.all: true matches everything
	if m.All {
		return true
	}

	// All specified criteria must be satisfied
	labelMatch := true
	rtMatch := true

	// Check labels: application must have all policy labels
	if len(m.Labels) > 0 {
		labelMatch = labelsMatch(m.Labels, labels)
	}

	// Check resourceTypes: resource type must be in the policy's list
	if len(m.ResourceTypes) > 0 {
		rtMatch = resourceTypeMatches(m.ResourceTypes, resourceType)
	}

	// Both must be true if both are specified
	if len(m.Labels) > 0 && len(m.ResourceTypes) > 0 {
		return labelMatch && rtMatch
	}
	if len(m.Labels) > 0 {
		return labelMatch
	}
	if len(m.ResourceTypes) > 0 {
		return rtMatch
	}

	return false
}

func labelsMatch(required, actual map[string]string) bool {
	for k, v := range required {
		if actual[k] != v {
			return false
		}
	}
	return true
}

func resourceTypeMatches(policyTypes []string, resourceType string) bool {
	for _, rt := range policyTypes {
		if rt == resourceType {
			return true
		}
	}
	return false
}
