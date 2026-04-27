package controller

import "time"

// ApplicationStatus tracks the provisioning state of an Application.
type ApplicationStatus struct {
	Phase      string           `json:"phase"`
	Resources  []ResourceStatus `json:"resources,omitempty"`
	Message    string           `json:"message,omitempty"`
	LastUpdate time.Time        `json:"lastUpdate"`
}

// ResourceStatus records the execution outcome for a single resource.
type ResourceStatus struct {
	Name        string         `json:"name"`
	Phase       string         `json:"phase"`
	Environment string         `json:"environment,omitempty"`
	Outputs     map[string]any `json:"outputs,omitempty"`
	Message     string         `json:"message,omitempty"`
}

// Phase constants for Application status.
const (
	PhasePending      = "Pending"
	PhaseValidating   = "Validating"
	PhasePlacing      = "Placing"
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseFailed       = "Failed"
)
