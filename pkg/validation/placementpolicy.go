package validation

import "github.com/dcm-io/dcm/pkg/apis/v1alpha1"

// ValidatePlacementPolicy checks structural validity of a PlacementPolicy.
func ValidatePlacementPolicy(pp *v1alpha1.PlacementPolicy) *Result {
	r := &Result{}

	validateObjectMeta(pp.Metadata, r)

	// At least one match criterion must be set
	m := pp.Spec.Match
	if !m.All && len(m.Labels) == 0 && len(m.ResourceTypes) == 0 {
		r.AddError("spec.match", "at least one match criterion is required, or set match.all: true")
	}

	if pp.Spec.Weight < 0 {
		r.AddErrorf("spec.weight", "must be >= 0, got %f", pp.Spec.Weight)
	}

	return r
}
