package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/dag"
	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/placement"
	"github.com/dcm-io/dcm/pkg/repository"
	"github.com/dcm-io/dcm/pkg/schema"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/validation"
)

// ApplicationReconciler processes Application resources through the
// full pipeline: validate -> build DAG -> place -> execute.
type ApplicationReconciler struct {
	appRepo    *repository.Repository[*v1alpha1.Application]
	envRepo    *repository.Repository[*v1alpha1.Environment]
	rtRepo     *repository.Repository[*v1alpha1.ResourceType]
	policyRepo *repository.Repository[*v1alpha1.PlacementPolicy]
	executor   *engine.Executor
	placer     *placement.Engine
	statusStore store.Store
}

// NewApplicationReconciler creates a reconciler with all required dependencies.
func NewApplicationReconciler(
	appRepo *repository.Repository[*v1alpha1.Application],
	envRepo *repository.Repository[*v1alpha1.Environment],
	rtRepo *repository.Repository[*v1alpha1.ResourceType],
	policyRepo *repository.Repository[*v1alpha1.PlacementPolicy],
	executor *engine.Executor,
	placer *placement.Engine,
	statusStore store.Store,
) *ApplicationReconciler {
	return &ApplicationReconciler{
		appRepo:     appRepo,
		envRepo:     envRepo,
		rtRepo:      rtRepo,
		policyRepo:  policyRepo,
		executor:    executor,
		placer:      placer,
		statusStore: statusStore,
	}
}

// Reconcile runs the full Application pipeline.
func (r *ApplicationReconciler) Reconcile(ctx context.Context, name string) error {
	app, _, err := r.appRepo.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("get application: %w", err)
	}
	if app == nil {
		return nil // deleted
	}

	status := &ApplicationStatus{Phase: PhasePending, LastUpdate: time.Now()}

	// Step 1: Structural validation
	status.Phase = PhaseValidating
	r.saveStatus(ctx, name, status)

	if err := validation.ValidateApplication(app).Error(); err != nil {
		return r.failStatus(ctx, name, status, "validation failed: %v", err)
	}

	// Step 2: Schema validation
	types, err := r.loadResourceTypes(ctx, app)
	if err != nil {
		return r.failStatus(ctx, name, status, "load resource types: %v", err)
	}
	if err := schema.ValidateApplication(app, types); err != nil {
		return r.failStatus(ctx, name, status, "schema validation: %v", err)
	}

	// Step 3: Build DAG
	appDAG, err := dag.Build(app)
	if err != nil {
		return r.failStatus(ctx, name, status, "build DAG: %v", err)
	}

	levels, err := appDAG.TopologicalSort()
	if err != nil {
		return r.failStatus(ctx, name, status, "topological sort: %v", err)
	}

	// Step 4: Placement
	status.Phase = PhasePlacing
	r.saveStatus(ctx, name, status)

	envs, err := r.envRepo.List(ctx)
	if err != nil {
		return r.failStatus(ctx, name, status, "list environments: %v", err)
	}

	policies, err := r.policyRepo.List(ctx)
	if err != nil {
		return r.failStatus(ctx, name, status, "list policies: %v", err)
	}

	// Rebuild DAG for placement (TopologicalSort is destructive)
	placementDAG, _ := dag.Build(app)
	placementResult, err := r.placer.Place(app, envs, policies, placementDAG)
	if err != nil {
		return r.failStatus(ctx, name, status, "placement: %v", err)
	}

	// Step 5: Execute
	status.Phase = PhaseProvisioning
	r.saveStatus(ctx, name, status)

	envMap := make(map[string]*v1alpha1.Environment)
	for _, e := range envs {
		envMap[e.Metadata.Name] = e
	}

	plan := &engine.ExecutionPlan{
		Application:   app,
		Levels:        levels,
		Assignments:   placementResult.Assignments,
		Environments:  envMap,
		ResourceTypes: types,
	}

	execResult, err := r.executor.Execute(ctx, plan)
	if err != nil {
		// Partial failure — record what we have
		if execResult != nil {
			status.Resources = toResourceStatuses(execResult)
		}
		return r.failStatus(ctx, name, status, "execution: %v", err)
	}

	// Step 6: Success
	status.Phase = PhaseReady
	status.Resources = toResourceStatuses(execResult)
	status.LastUpdate = time.Now()
	r.saveStatus(ctx, name, status)

	log.Printf("Application %q reconciled successfully", name)
	return nil
}

func (r *ApplicationReconciler) loadResourceTypes(ctx context.Context, app *v1alpha1.Application) (map[string]*v1alpha1.ResourceType, error) {
	types := make(map[string]*v1alpha1.ResourceType)
	for _, res := range app.Spec.Resources {
		if types[res.Type] != nil {
			continue
		}
		rt, _, err := r.rtRepo.Get(ctx, res.Type)
		if err != nil {
			return nil, fmt.Errorf("get resource type %q: %w", res.Type, err)
		}
		if rt != nil {
			types[res.Type] = rt
		}
	}
	return types, nil
}

func (r *ApplicationReconciler) failStatus(ctx context.Context, name string, status *ApplicationStatus, format string, args ...any) error {
	status.Phase = PhaseFailed
	status.Message = fmt.Sprintf(format, args...)
	status.LastUpdate = time.Now()
	r.saveStatus(ctx, name, status)
	return fmt.Errorf(status.Message)
}

func (r *ApplicationReconciler) saveStatus(ctx context.Context, name string, status *ApplicationStatus) {
	key := fmt.Sprintf("/registry/applicationstatus/%s", name)
	data, err := json.Marshal(status)
	if err != nil {
		log.Printf("marshal status for %q: %v", name, err)
		return
	}

	// Try update, fall back to create
	existing, err := r.statusStore.Get(ctx, key)
	if err != nil {
		log.Printf("get status for %q: %v", name, err)
		return
	}
	if existing != nil {
		if _, err := r.statusStore.Update(ctx, key, data, existing.Revision); err != nil {
			log.Printf("update status for %q: %v", name, err)
		}
	} else {
		if _, err := r.statusStore.Create(ctx, key, data); err != nil {
			log.Printf("create status for %q: %v", name, err)
		}
	}
}

func toResourceStatuses(result *engine.ExecutionResult) []ResourceStatus {
	var statuses []ResourceStatus
	for name, s := range result.Statuses {
		rs := ResourceStatus{
			Name:        name,
			Phase:       s.Phase,
			Environment: s.Environment,
			Message:     s.Error,
		}
		if output, ok := result.State.GetOutput(name); ok {
			rs.Outputs = output.Values
		}
		statuses = append(statuses, rs)
	}
	return statuses
}
