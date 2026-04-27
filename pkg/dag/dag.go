package dag

import "fmt"

// DAG is a directed acyclic graph where edges represent dependencies.
// An edge from A to B means "A depends on B" (B must come before A).
type DAG struct {
	nodes map[string]bool
	edges map[string]map[string]bool // from -> set of targets it depends on
}

// New creates an empty DAG.
func New() *DAG {
	return &DAG{
		nodes: make(map[string]bool),
		edges: make(map[string]map[string]bool),
	}
}

// AddNode adds a node to the graph.
func (d *DAG) AddNode(id string) {
	d.nodes[id] = true
	if d.edges[id] == nil {
		d.edges[id] = make(map[string]bool)
	}
}

// AddEdge adds a dependency: "from" depends on "to".
func (d *DAG) AddEdge(from, to string) {
	d.AddNode(from)
	d.AddNode(to)
	d.edges[from][to] = true
}

// Dependencies returns the nodes that id directly depends on.
func (d *DAG) Dependencies(id string) []string {
	var deps []string
	for dep := range d.edges[id] {
		deps = append(deps, dep)
	}
	return deps
}

// Dependents returns the nodes that directly depend on id.
func (d *DAG) Dependents(id string) []string {
	var deps []string
	for node, targets := range d.edges {
		if targets[id] {
			deps = append(deps, node)
		}
	}
	return deps
}

// Nodes returns all node IDs.
func (d *DAG) Nodes() []string {
	var result []string
	for id := range d.nodes {
		result = append(result, id)
	}
	return result
}

// DetectCycle returns the cycle path if the graph contains a cycle,
// or nil if it is acyclic.
func (d *DAG) DetectCycle() []string {
	visited := make(map[string]int) // 0=unvisited, 1=in-progress, 2=done
	var path []string

	var visit func(node string) bool
	visit = func(node string) bool {
		if visited[node] == 2 {
			return false
		}
		if visited[node] == 1 {
			// Found cycle — extract the cycle from path
			cycle := []string{node}
			for i := len(path) - 1; i >= 0; i-- {
				cycle = append(cycle, path[i])
				if path[i] == node {
					break
				}
			}
			// Reverse to get correct order
			for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
				cycle[i], cycle[j] = cycle[j], cycle[i]
			}
			path = cycle
			return true
		}

		visited[node] = 1
		path = append(path, node)

		for dep := range d.edges[node] {
			if visit(dep) {
				return true
			}
		}

		path = path[:len(path)-1]
		visited[node] = 2
		return false
	}

	for node := range d.nodes {
		if visited[node] == 0 {
			if visit(node) {
				return path
			}
		}
	}
	return nil
}

// TopologicalSort returns nodes grouped into levels for parallel execution.
// Level 0 contains nodes with no dependencies, level 1 contains nodes whose
// dependencies are all in level 0, etc. Returns an error if the graph has a cycle.
func (d *DAG) TopologicalSort() ([][]string, error) {
	cycle := d.DetectCycle()
	if cycle != nil {
		return nil, fmt.Errorf("circular dependency detected: %s", formatCycle(cycle))
	}

	// Kahn's algorithm
	inDegree := make(map[string]int)
	for id := range d.nodes {
		inDegree[id] = 0
	}
	for _, deps := range d.edges {
		for dep := range deps {
			inDegree[dep] += 0 // ensure dep exists
			_ = dep
		}
	}
	// Count actual in-degrees (how many nodes depend on each node)
	for _, deps := range d.edges {
		for dep := range deps {
			// dep is depended upon, but in-degree in Kahn's is about
			// "how many prerequisites does this node have"
			_ = dep
		}
	}

	// In our model, edges[A][B] means A depends on B.
	// For topological sort, we need "prerequisites count" per node.
	prereqs := make(map[string]int)
	for id := range d.nodes {
		prereqs[id] = len(d.edges[id])
	}

	var levels [][]string
	remaining := len(d.nodes)

	for remaining > 0 {
		var level []string
		for id := range d.nodes {
			if prereqs[id] == 0 {
				level = append(level, id)
			}
		}
		if len(level) == 0 {
			break // shouldn't happen since we checked for cycles
		}

		// Sort level for deterministic output
		sortStrings(level)
		levels = append(levels, level)

		for _, id := range level {
			delete(d.nodes, id)
			remaining--
			// Reduce prereq count for nodes that depend on id
			for node, deps := range d.edges {
				if deps[id] {
					delete(deps, id)
					prereqs[node] = len(deps)
				}
			}
		}

		// Recalculate prereqs for remaining
		for id := range d.nodes {
			prereqs[id] = len(d.edges[id])
		}
	}

	return levels, nil
}

func formatCycle(cycle []string) string {
	result := ""
	for i, node := range cycle {
		if i > 0 {
			result += " -> "
		}
		result += node
	}
	return result
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
