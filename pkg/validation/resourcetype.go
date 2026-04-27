package validation

import (
	"regexp"
	"strings"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

var validLifecycles = map[string]bool{
	"draft":      true,
	"stable":     true,
	"deprecated": true,
}

// semverRegex is a simplified semver check: major.minor.patch
var semverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ValidateResourceType checks structural validity of a ResourceType.
func ValidateResourceType(rt *v1alpha1.ResourceType) *Result {
	r := &Result{}

	validateObjectMeta(rt.Metadata, r)

	// Name must follow category.technology dot notation
	if rt.Metadata.Name != "" && !strings.Contains(rt.Metadata.Name, ".") {
		r.AddErrorf("metadata.name", "%q should follow category.technology dot notation (e.g., database.postgresql)", rt.Metadata.Name)
	}

	// spec.version
	if rt.Spec.Version == "" {
		r.AddError("spec.version", "is required")
	} else if !semverRegex.MatchString(rt.Spec.Version) {
		r.AddErrorf("spec.version", "%q is not valid semver (expected major.minor.patch)", rt.Spec.Version)
	}

	// spec.lifecycle
	if rt.Spec.Lifecycle == "" {
		r.AddError("spec.lifecycle", "is required")
	} else if !validLifecycles[rt.Spec.Lifecycle] {
		r.AddErrorf("spec.lifecycle", "%q is not valid (valid: draft, stable, deprecated)", rt.Spec.Lifecycle)
	}

	// deprecation required when lifecycle is deprecated
	if rt.Spec.Lifecycle == "deprecated" && rt.Spec.Deprecation == nil {
		r.AddError("spec.deprecation", "is required when lifecycle is deprecated")
	}

	// spec.schema
	if rt.Spec.Schema == nil {
		r.AddError("spec.schema", "is required")
	} else {
		schemaType, _ := rt.Spec.Schema["type"].(string)
		if schemaType != "object" {
			r.AddError("spec.schema.type", "must be \"object\"")
		}
		if _, ok := rt.Spec.Schema["properties"]; !ok {
			r.AddError("spec.schema.properties", "is required")
		}
	}

	return r
}
