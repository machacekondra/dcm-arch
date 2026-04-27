package placement

// PlacementResult holds the final placement assignments and audit trail.
type PlacementResult struct {
	// Assignments maps resource name to the selected environment name.
	Assignments map[string]string
	// Decisions contains the full audit log of placement evaluations.
	Decisions []ResourceDecision
}

// ResourceDecision records the placement evaluation for a single resource.
type ResourceDecision struct {
	Resource     string
	Candidates   []CandidateScore
	Selected     string
	FailedReason string // non-empty if placement failed for this resource
}

// CandidateScore records an environment's evaluation result for a resource.
type CandidateScore struct {
	Environment string
	Eligible    bool
	Score       float64
	Eliminations []string // reasons why this env was eliminated (if not eligible)
}
