package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/dag"
	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/placement"
	"github.com/dcm-io/dcm/pkg/repository"
)

// DeployHandler handles deploy requests for Applications.
type DeployHandler struct {
	appRepo    *repository.Repository[*v1alpha1.Application]
	envRepo    *repository.Repository[*v1alpha1.Environment]
	policyRepo *repository.Repository[*v1alpha1.PlacementPolicy]
	deployRepo *repository.Repository[*v1alpha1.Deployment]
	placer     *placement.Engine
	executor   *engine.Executor
}

// DeployResponse is the JSON response for a deploy operation.
type DeployResponse struct {
	DeploymentName string                       `json:"deploymentName"`
	Phase          string                       `json:"phase"`
	Assignments    map[string]string            `json:"assignments,omitempty"`
	Levels         [][]string                   `json:"levels,omitempty"`
	Decisions      []placement.ResourceDecision `json:"decisions,omitempty"`
	Resources      []DeployResourceStatus       `json:"resources,omitempty"`
	Error          string                       `json:"error,omitempty"`
}

// DeployResourceStatus records per-resource deploy outcome.
type DeployResourceStatus struct {
	Name        string         `json:"name"`
	Phase       string         `json:"phase"`
	Environment string         `json:"environment"`
	Outputs     map[string]any `json:"outputs,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func (h *DeployHandler) deploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		badRequest(w, "application name is required")
		return
	}

	ctx := r.Context()
	startedAt := time.Now().UTC()
	deploymentName := fmt.Sprintf("%s-%d", name, startedAt.Unix())

	app, rev, err := h.appRepo.Get(ctx, name)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if rev == 0 {
		notFound(w, fmt.Sprintf("application %q not found", name))
		return
	}

	// Build DAG
	appDAG, err := dag.Build(app)
	if err != nil {
		resp := DeployResponse{DeploymentName: deploymentName, Phase: "Failed", Error: fmt.Sprintf("DAG: %v", err)}
		h.saveDeployment(ctx, deploymentName, name, startedAt, resp)
		writeJSON(w, resp)
		return
	}

	levels, err := appDAG.TopologicalSort()
	if err != nil {
		resp := DeployResponse{DeploymentName: deploymentName, Phase: "Failed", Error: fmt.Sprintf("sort: %v", err)}
		h.saveDeployment(ctx, deploymentName, name, startedAt, resp)
		writeJSON(w, resp)
		return
	}

	// Placement
	envs, err := h.envRepo.List(ctx)
	if err != nil {
		resp := DeployResponse{DeploymentName: deploymentName, Phase: "Failed", Error: err.Error()}
		h.saveDeployment(ctx, deploymentName, name, startedAt, resp)
		writeJSON(w, resp)
		return
	}

	policies, err := h.policyRepo.List(ctx)
	if err != nil {
		resp := DeployResponse{DeploymentName: deploymentName, Phase: "Failed", Error: err.Error()}
		h.saveDeployment(ctx, deploymentName, name, startedAt, resp)
		writeJSON(w, resp)
		return
	}

	placementDAG, _ := dag.Build(app)
	placementResult, err := h.placer.Place(app, envs, policies, placementDAG)
	if err != nil {
		resp := DeployResponse{DeploymentName: deploymentName, Phase: "Failed", Error: err.Error(), Levels: levels}
		if placementResult != nil {
			resp.Assignments = placementResult.Assignments
			resp.Decisions = placementResult.Decisions
		}
		h.saveDeployment(ctx, deploymentName, name, startedAt, resp)
		writeJSON(w, resp)
		return
	}

	// Build environment map
	envMap := make(map[string]*v1alpha1.Environment)
	for _, e := range envs {
		envMap[e.Metadata.Name] = e
	}

	// Execute
	plan := &engine.ExecutionPlan{
		Application:  app,
		Levels:       levels,
		Assignments:  placementResult.Assignments,
		Environments: envMap,
	}

	execResult, execErr := h.executor.Execute(ctx, plan)

	resp := DeployResponse{
		DeploymentName: deploymentName,
		Phase:          "Provisioned",
		Assignments:    placementResult.Assignments,
		Levels:         levels,
		Decisions:      placementResult.Decisions,
	}

	if execErr != nil {
		resp.Phase = "Failed"
		resp.Error = execErr.Error()
	}

	if execResult != nil {
		for resName, status := range execResult.Statuses {
			rs := DeployResourceStatus{
				Name:        resName,
				Phase:       status.Phase,
				Environment: status.Environment,
				Error:       status.Error,
			}
			if output, ok := execResult.State.GetOutput(resName); ok {
				rs.Outputs = output.Values
			}
			resp.Resources = append(resp.Resources, rs)
		}
	}

	h.saveDeployment(ctx, deploymentName, name, startedAt, resp)
	writeJSON(w, resp)
}

func (h *DeployHandler) saveDeployment(ctx context.Context, deploymentName, appName string, startedAt time.Time, resp DeployResponse) {
	dep := &v1alpha1.Deployment{}
	dep.APIVersion = v1alpha1.GroupVersion
	dep.Kind = v1alpha1.KindDeployment
	dep.Metadata = meta.ObjectMeta{
		Name: deploymentName,
		Labels: map[string]string{
			"application": appName,
		},
	}
	dep.Spec = v1alpha1.DeploymentSpec{
		Application: appName,
		Phase:       resp.Phase,
		StartedAt:   startedAt.Format(time.RFC3339),
		FinishedAt:  time.Now().UTC().Format(time.RFC3339),
		Assignments: resp.Assignments,
		Levels:      resp.Levels,
		Error:       resp.Error,
	}

	for _, rs := range resp.Resources {
		dep.Spec.Resources = append(dep.Spec.Resources, v1alpha1.DeploymentResourceStatus{
			Name:        rs.Name,
			Phase:       rs.Phase,
			Environment: rs.Environment,
			Outputs:     rs.Outputs,
			Error:       rs.Error,
		})
	}

	h.deployRepo.Create(ctx, dep)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v)
}
