package placement

import "github.com/dcm-io/dcm/pkg/apis/v1alpha1"

// ConnectivityGraph tracks which environments can reach each other
// via shared overlay networks.
type ConnectivityGraph struct {
	// overlays maps overlay name -> set of environment names
	overlays map[string]map[string]bool
}

// BuildConnectivityGraph builds a connectivity graph from environment overlay
// declarations. Two environments are connected if they share at least one
// overlay network name.
func BuildConnectivityGraph(envs []*v1alpha1.Environment) *ConnectivityGraph {
	g := &ConnectivityGraph{
		overlays: make(map[string]map[string]bool),
	}

	for _, env := range envs {
		if env.Spec.Networking == nil {
			continue
		}
		for _, overlay := range env.Spec.Networking.Overlays {
			if g.overlays[overlay.Name] == nil {
				g.overlays[overlay.Name] = make(map[string]bool)
			}
			g.overlays[overlay.Name][env.Metadata.Name] = true
		}
	}

	return g
}

// Connected returns true if envA and envB share at least one overlay network,
// or if they are the same environment.
func (g *ConnectivityGraph) Connected(envA, envB string) bool {
	if envA == envB {
		return true
	}
	for _, members := range g.overlays {
		if members[envA] && members[envB] {
			return true
		}
	}
	return false
}
