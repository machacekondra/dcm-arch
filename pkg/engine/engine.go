package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
)

// ExecutionPlan contains everything needed to execute an Application.
type ExecutionPlan struct {
	Application *v1alpha1.Application
	// Levels from topological sort — resources in each level can run in parallel.
	Levels [][]string
	// Assignments maps resource name -> environment name (from placement).
	Assignments map[string]string
	// Environments maps environment name -> Environment object.
	Environments map[string]*v1alpha1.Environment
	// ResourceTypes maps resource type name -> ResourceType object.
	ResourceTypes map[string]*v1alpha1.ResourceType
}

// ExecutionResult holds the outcome of executing an Application.
type ExecutionResult struct {
	State    *ExecutionState
	Statuses map[string]ResourceStatus
}

// ResourceStatus records the execution outcome for a single resource.
type ResourceStatus struct {
	Name        string
	Environment string
	Phase       string // Provisioned, Failed
	Error       string
}

// Executor orchestrates resource provisioning in DAG order.
type Executor struct {
	drivers map[string]Driver // recipe type -> driver
}

// NewExecutor creates an Executor with the given drivers.
func NewExecutor(drivers map[string]Driver) *Executor {
	return &Executor{drivers: drivers}
}

// Execute provisions all resources in the plan, level by level.
// Resources within a level are provisioned in parallel.
func (e *Executor) Execute(ctx context.Context, plan *ExecutionPlan) (*ExecutionResult, error) {
	state := NewExecutionState()
	result := &ExecutionResult{
		State:    state,
		Statuses: make(map[string]ResourceStatus),
	}

	// Index resources by name
	resourceIndex := make(map[string]v1alpha1.ResourceDecl)
	for _, res := range plan.Application.Spec.Resources {
		resourceIndex[res.Name] = res
	}

	for levelIdx, level := range plan.Levels {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("execution cancelled at level %d: %w", levelIdx, err)
		}

		if len(level) == 1 {
			// Single resource — execute directly
			res := resourceIndex[level[0]]
			if err := e.executeResource(ctx, plan, res, state, result); err != nil {
				return result, err
			}
		} else {
			// Multiple resources — execute in parallel
			var wg sync.WaitGroup
			errs := make([]error, len(level))

			for i, name := range level {
				wg.Add(1)
				go func(idx int, resName string) {
					defer wg.Done()
					res := resourceIndex[resName]
					errs[idx] = e.executeResource(ctx, plan, res, state, result)
				}(i, name)
			}
			wg.Wait()

			// Check for errors
			for _, err := range errs {
				if err != nil {
					return result, err
				}
			}
		}
	}

	return result, nil
}

func (e *Executor) executeResource(
	ctx context.Context,
	plan *ExecutionPlan,
	res v1alpha1.ResourceDecl,
	state *ExecutionState,
	result *ExecutionResult,
) error {
	envName := plan.Assignments[res.Name]
	env := plan.Environments[envName]

	// Resolve recipe for this resource type on this environment
	recipeType, source, recipeParams, recipeErr := resolveRecipe(env, res)
	if recipeErr != nil {
		result.Statuses[res.Name] = ResourceStatus{
			Name: res.Name, Environment: envName, Phase: "Failed", Error: recipeErr.Error(),
		}
		return recipeErr
	}

	// Merge parameters: recipe defaults + developer properties (developer wins)
	properties := mergeProperties(recipeParams, res.Properties)

	// Resolve CEL references from prior outputs
	properties = resolveReferences(properties, state)

	// Build context
	dcmCtx := BuildContext(res.Name, plan.Application, env)

	// Find driver
	driver, ok := e.drivers[recipeType]
	if !ok {
		err := fmt.Errorf("no driver registered for recipe type %q", recipeType)
		result.Statuses[res.Name] = ResourceStatus{
			Name: res.Name, Environment: envName, Phase: "Failed", Error: err.Error(),
		}
		return err
	}

	// Execute
	inv := &Invocation{
		ResourceName: res.Name,
		ResourceType: res.Type,
		RecipeType:   recipeType,
		Source:        source,
		Properties:   properties,
		Context:      dcmCtx,
	}

	execResult, err := driver.Execute(ctx, inv)
	if err != nil {
		result.Statuses[res.Name] = ResourceStatus{
			Name: res.Name, Environment: envName, Phase: "Failed", Error: err.Error(),
		}
		return fmt.Errorf("resource %q execution failed: %w", res.Name, err)
	}

	// Store outputs for dependent resources
	state.SetOutput(res.Name, execResult)
	result.Statuses[res.Name] = ResourceStatus{
		Name: res.Name, Environment: envName, Phase: "Provisioned",
	}

	return nil
}

// resolveRecipe finds the recipe binding for a resource type on an environment.
// Returns the recipe type, source, default parameters, and an error if no recipe is found.
func resolveRecipe(env *v1alpha1.Environment, res v1alpha1.ResourceDecl) (string, map[string]string, map[string]any, error) {
	if env == nil {
		return "", nil, nil, fmt.Errorf("no environment assigned for resource %q", res.Name)
	}

	if env.Spec.Recipes == nil {
		return "", nil, nil, fmt.Errorf("environment %q has no recipes configured for resource type %q — add a recipe binding to the environment's spec.recipes", env.Metadata.Name, res.Type)
	}

	recipes, ok := env.Spec.Recipes[res.Type]
	if !ok {
		return "", nil, nil, fmt.Errorf("environment %q has no recipe for resource type %q — register a recipe binding in spec.recipes[%q]", env.Metadata.Name, res.Type, res.Type)
	}

	// Use named recipe if specified, otherwise use "default"
	recipeName := res.Recipe
	if recipeName == "" {
		recipeName = "default"
	}

	binding, ok := recipes[recipeName]
	if !ok {
		available := make([]string, 0, len(recipes))
		for name := range recipes {
			available = append(available, name)
		}
		return "", nil, nil, fmt.Errorf("recipe %q not found for resource type %q on environment %q (available: %v)", recipeName, res.Type, env.Metadata.Name, available)
	}

	return binding.Type, binding.Source, binding.Parameters, nil
}

func mergeProperties(base map[string]any, overlay map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// resolveReferences replaces ${resource.field} patterns in property values
// with actual outputs from previously provisioned resources.
func resolveReferences(properties map[string]any, state *ExecutionState) map[string]any {
	result := make(map[string]any)
	for k, v := range properties {
		result[k] = resolveValue(v, state)
	}
	return result
}

func resolveValue(value any, state *ExecutionState) any {
	s, ok := value.(string)
	if !ok {
		return value
	}

	if !strings.Contains(s, "${") {
		return s
	}

	// Replace all ${resource.field} patterns
	resolved := s
	for {
		start := strings.Index(resolved, "${")
		if start == -1 {
			break
		}
		end := strings.Index(resolved[start:], "}")
		if end == -1 {
			break
		}
		end += start

		expr := resolved[start+2 : end]
		parts := strings.SplitN(expr, ".", 2)
		if len(parts) == 2 {
			val, err := state.ResolveOutputValue(parts[0], parts[1])
			if err == nil {
				replacement := fmt.Sprintf("%v", val)
				resolved = resolved[:start] + replacement + resolved[end+1:]
				continue
			}
		}
		// Can't resolve — skip this expression
		break
	}

	return resolved
}
