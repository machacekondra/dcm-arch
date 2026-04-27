package placement

import (
	"encoding/json"
	"fmt"
	"sort"

	dcmcel "github.com/dcm-io/dcm/pkg/cel"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/dag"
	celgo "github.com/google/cel-go/cel"
)

// Engine evaluates placement for Application resources.
type Engine struct {
	celEnv *celgo.Env
}

// NewEngine creates a placement engine.
func NewEngine() (*Engine, error) {
	celEnv, err := dcmcel.NewPlacementEnv()
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}
	return &Engine{celEnv: celEnv}, nil
}

// Place runs the placement pipeline for an Application:
//  1. For each resource, compute effective labels (app labels merged with resource labels)
//  2. Pre-filter environments by resource type
//  3. Match placement policies
//  4. Evaluate rule expressions (hard constraints)
//  5. Score prefer expressions (soft preferences)
//  6. Rank and select best environment
//  7. Validate cross-environment connectivity via DAG
func (e *Engine) Place(
	app *v1alpha1.Application,
	envs []*v1alpha1.Environment,
	policies []*v1alpha1.PlacementPolicy,
	appDAG *dag.DAG,
) (*PlacementResult, error) {

	result := &PlacementResult{
		Assignments: make(map[string]string),
	}

	// Sort policies by priority descending for rule short-circuit
	sortedPolicies := make([]*v1alpha1.PlacementPolicy, len(policies))
	copy(sortedPolicies, policies)
	sort.Slice(sortedPolicies, func(i, j int) bool {
		return sortedPolicies[i].Spec.Priority > sortedPolicies[j].Spec.Priority
	})

	// Place each resource independently
	for _, res := range app.Spec.Resources {
		decision, err := e.placeResource(app, res, envs, sortedPolicies)
		if err != nil {
			return nil, fmt.Errorf("placement failed for resource %q: %w", res.Name, err)
		}
		result.Decisions = append(result.Decisions, decision)

		if decision.Selected == "" {
			return nil, &PlacementError{
				Resource: res.Name,
				Decision: decision,
			}
		}
		result.Assignments[res.Name] = decision.Selected
	}

	// Validate cross-environment connectivity
	if err := e.validateConnectivity(appDAG, result.Assignments, envs); err != nil {
		return nil, err
	}

	return result, nil
}

func (e *Engine) placeResource(
	app *v1alpha1.Application,
	res v1alpha1.ResourceDecl,
	envs []*v1alpha1.Environment,
	policies []*v1alpha1.PlacementPolicy,
) (ResourceDecision, error) {
	decision := ResourceDecision{Resource: res.Name}

	// Step 1: Pre-filter by resource type
	candidates := PrefilterByResourceType(envs, res.Type)
	if len(candidates) == 0 {
		decision.FailedReason = fmt.Sprintf("no environment supports resource type %q", res.Type)
		return decision, nil
	}

	// Step 2: Compute effective labels (app labels + resource labels, resource wins)
	effectiveLabels := mergeLabels(app.Metadata.Labels, res.Labels)

	// Step 3: Match policies
	matched := MatchPolicies(policies, effectiveLabels, res.Type)

	// Step 4 & 5: Evaluate rules and preferences for each candidate
	for _, env := range candidates {
		cs := CandidateScore{Environment: env.Metadata.Name, Eligible: true}

		envMap := environmentToMap(env)
		vars := map[string]any{"env": envMap}

		// Evaluate rules (all must pass)
		for _, policy := range matched {
			if policy.Spec.Rule == "" {
				continue
			}
			program, err := dcmcel.CompileRule(e.celEnv, policy.Spec.Rule)
			if err != nil {
				cs.Eligible = false
				cs.Eliminations = append(cs.Eliminations, fmt.Sprintf("policy %q rule compile error: %v", policy.Metadata.Name, err))
				break
			}
			pass, err := dcmcel.EvalBool(program, vars)
			if err != nil {
				cs.Eligible = false
				cs.Eliminations = append(cs.Eliminations, fmt.Sprintf("policy %q rule eval error: %v", policy.Metadata.Name, err))
				break
			}
			if !pass {
				cs.Eligible = false
				cs.Eliminations = append(cs.Eliminations, fmt.Sprintf("policy %q rule not satisfied", policy.Metadata.Name))
				break
			}
		}

		// Score preferences (only if eligible)
		if cs.Eligible {
			for _, policy := range matched {
				if policy.Spec.Prefer == "" {
					continue
				}
				program, err := dcmcel.CompilePrefer(e.celEnv, policy.Spec.Prefer)
				if err != nil {
					continue
				}
				score, err := dcmcel.EvalFloat(program, vars)
				if err != nil {
					continue
				}
				weight := policy.Spec.Weight
				if weight == 0 {
					weight = 1.0
				}
				cs.Score += score * weight
			}
		}

		decision.Candidates = append(decision.Candidates, cs)
	}

	// Step 6: Rank eligible candidates
	var eligible []CandidateScore
	for _, cs := range decision.Candidates {
		if cs.Eligible {
			eligible = append(eligible, cs)
		}
	}

	if len(eligible) == 0 {
		decision.FailedReason = "no eligible environment after policy evaluation"
		return decision, nil
	}

	// Sort by score descending, then by name for deterministic tie-breaking
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Score != eligible[j].Score {
			return eligible[i].Score > eligible[j].Score
		}
		return eligible[i].Environment < eligible[j].Environment
	})

	decision.Selected = eligible[0].Environment
	return decision, nil
}

func (e *Engine) validateConnectivity(
	appDAG *dag.DAG,
	assignments map[string]string,
	envs []*v1alpha1.Environment,
) error {
	graph := BuildConnectivityGraph(envs)

	// Check each DAG edge: if resources are in different environments,
	// they must be connected via an overlay.
	for _, node := range appDAG.Nodes() {
		nodeEnv := assignments[node]
		for _, dep := range appDAG.Dependencies(node) {
			depEnv := assignments[dep]
			if !graph.Connected(nodeEnv, depEnv) {
				return &ConnectivityError{
					ResourceA:    node,
					EnvironmentA: nodeEnv,
					ResourceB:    dep,
					EnvironmentB: depEnv,
				}
			}
		}
	}

	return nil
}

func mergeLabels(base, overlay map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// environmentToMap converts an Environment to a nested map for CEL evaluation.
func environmentToMap(env *v1alpha1.Environment) map[string]any {
	data, err := json.Marshal(env.Spec)
	if err != nil {
		return map[string]any{}
	}
	var result map[string]any
	json.Unmarshal(data, &result)

	// Add metadata labels
	result["name"] = env.Metadata.Name
	if env.Metadata.Labels != nil {
		labels := make(map[string]any)
		for k, v := range env.Metadata.Labels {
			labels[k] = v
		}
		result["labels"] = labels
	}

	return result
}

// PlacementError is returned when no eligible environment is found.
type PlacementError struct {
	Resource string
	Decision ResourceDecision
}

func (e *PlacementError) Error() string {
	msg := fmt.Sprintf("PlacementFailed: no eligible environment for resource %q", e.Resource)
	if e.Decision.FailedReason != "" {
		msg += ": " + e.Decision.FailedReason
	}
	for _, cs := range e.Decision.Candidates {
		if !cs.Eligible {
			for _, elim := range cs.Eliminations {
				msg += fmt.Sprintf("\n  %s: %s", cs.Environment, elim)
			}
		}
	}
	return msg
}

// ConnectivityError is returned when cross-environment dependencies lack connectivity.
type ConnectivityError struct {
	ResourceA, EnvironmentA string
	ResourceB, EnvironmentB string
}

func (e *ConnectivityError) Error() string {
	return fmt.Sprintf(
		"PlacementFailed: no connectivity between environments for dependent resources.\n"+
			"  Resource %q (%s) depends on %q (%s)\n"+
			"  Environments %s and %s share no overlay network.",
		e.ResourceA, e.EnvironmentA, e.ResourceB, e.EnvironmentB,
		e.EnvironmentA, e.EnvironmentB,
	)
}
