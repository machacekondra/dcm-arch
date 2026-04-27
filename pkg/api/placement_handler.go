package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/dag"
	"github.com/dcm-io/dcm/pkg/placement"
	"github.com/dcm-io/dcm/pkg/repository"
)

// PlacementHandler handles placement simulation requests.
type PlacementHandler struct {
	appRepo    *repository.Repository[*v1alpha1.Application]
	envRepo    *repository.Repository[*v1alpha1.Environment]
	policyRepo *repository.Repository[*v1alpha1.PlacementPolicy]
	engine     *placement.Engine
}

// PlacementResponse is the JSON response for a placement simulation.
type PlacementResponse struct {
	Assignments map[string]string        `json:"assignments"`
	Decisions   []placement.ResourceDecision `json:"decisions"`
	Error       string                   `json:"error,omitempty"`
}

func (h *PlacementHandler) simulate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		badRequest(w, "application name is required")
		return
	}

	ctx := r.Context()

	app, rev, err := h.appRepo.Get(ctx, name)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if rev == 0 {
		notFound(w, fmt.Sprintf("application %q not found", name))
		return
	}

	envs, err := h.envRepo.List(ctx)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	policies, err := h.policyRepo.List(ctx)
	if err != nil {
		internalError(w, err.Error())
		return
	}

	appDAG, err := dag.Build(app)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(PlacementResponse{Error: fmt.Sprintf("DAG build failed: %v", err)})
		return
	}

	result, err := h.engine.Place(app, envs, policies, appDAG)

	resp := PlacementResponse{}
	if err != nil {
		resp.Error = err.Error()
		// Still try to include partial decisions if available
		if result != nil {
			resp.Assignments = result.Assignments
			resp.Decisions = result.Decisions
		}
	} else {
		resp.Assignments = result.Assignments
		resp.Decisions = result.Decisions
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
