package validation

import (
	"fmt"
	"net/url"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

var validEnvTypes = map[string]bool{
	"kubernetes": true,
	"openshift":  true,
	"vmware":     true,
	"aws":        true,
	"azure":      true,
	"gcp":        true,
	"bare-metal": true,
}

var validDataClassifications = map[string]bool{
	"public":       true,
	"internal":     true,
	"confidential": true,
	"restricted":   true,
}

// ValidateEnvironment checks structural validity of an Environment.
func ValidateEnvironment(env *v1alpha1.Environment) *Result {
	r := &Result{}

	validateObjectMeta(env.Metadata, r)

	// spec.type
	if env.Spec.Type == "" {
		r.AddError("spec.type", "is required")
	} else if !validEnvTypes[env.Spec.Type] {
		r.AddErrorf("spec.type", "%q is not a valid environment type (valid: kubernetes, openshift, vmware, aws, azure, gcp, bare-metal)", env.Spec.Type)
	}

	// spec.connection
	if env.Spec.Connection.Endpoint == "" {
		r.AddError("spec.connection.endpoint", "is required")
	} else if _, err := url.ParseRequestURI(env.Spec.Connection.Endpoint); err != nil {
		r.AddErrorf("spec.connection.endpoint", "%q is not a valid URL", env.Spec.Connection.Endpoint)
	}
	if env.Spec.Connection.CredentialRef == "" {
		r.AddError("spec.connection.credentialRef", "is required")
	}

	// spec.capabilities
	if len(env.Spec.Capabilities.ResourceTypes) == 0 {
		r.AddError("spec.capabilities.resourceTypes", "must contain at least one resource type")
	}

	// spec.sovereignty
	validateSovereignty(env.Spec.Sovereignty, r)

	// spec.networking.overlays
	if env.Spec.Networking != nil {
		for i, overlay := range env.Spec.Networking.Overlays {
			if overlay.Name == "" {
				r.AddError(fmt.Sprintf("spec.networking.overlays[%d].name", i), "is required")
			}
			if overlay.Type == "" {
				r.AddError(fmt.Sprintf("spec.networking.overlays[%d].type", i), "is required")
			}
		}
	}

	// spec.cost
	if env.Spec.Cost != nil && env.Spec.Cost.Currency == "" {
		r.AddError("spec.cost.currency", "is required when cost is specified")
	}

	return r
}

func validateSovereignty(s v1alpha1.SovereigntySpec, r *Result) {
	if s.Country == "" {
		r.AddError("spec.sovereignty.country", "is required")
	} else if len(s.Country) != 2 {
		r.AddErrorf("spec.sovereignty.country", "%q must be a 2-letter ISO 3166-1 alpha-2 code", s.Country)
	}
	if s.Region == "" {
		r.AddError("spec.sovereignty.region", "is required")
	}
	if s.Jurisdiction == "" {
		r.AddError("spec.sovereignty.jurisdiction", "is required")
	}
	if s.DataClassification == "" {
		r.AddError("spec.sovereignty.dataClassification", "is required")
	} else if !validDataClassifications[s.DataClassification] {
		r.AddErrorf("spec.sovereignty.dataClassification", "%q is not valid (valid: public, internal, confidential, restricted)", s.DataClassification)
	}
}
