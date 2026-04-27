package v1alpha1

import "github.com/dcm-io/dcm/pkg/apis/meta"

// Application is a declarative YAML spec that describes an application's resources.
type Application struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          ApplicationSpec `json:"spec" yaml:"spec"`
}

func (a *Application) GetTypeMeta() meta.TypeMeta   { return a.TypeMeta }
func (a *Application) GetObjectMeta() meta.ObjectMeta { return a.Metadata }

type ApplicationSpec struct {
	Resources []ResourceDecl `json:"resources" yaml:"resources"`
}

type ResourceDecl struct {
	Type         string            `json:"type" yaml:"type"`
	Name         string            `json:"name" yaml:"name"`
	Properties   map[string]any    `json:"properties,omitempty" yaml:"properties,omitempty"`
	Requirements []string          `json:"requirements,omitempty" yaml:"requirements,omitempty"`
	IncludeWhen  string            `json:"includeWhen,omitempty" yaml:"includeWhen,omitempty"`
	Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Recipe       string            `json:"recipe,omitempty" yaml:"recipe,omitempty"`
}

// Environment represents a target infrastructure where resources can be provisioned.
type Environment struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          EnvironmentSpec `json:"spec" yaml:"spec"`
}

func (e *Environment) GetTypeMeta() meta.TypeMeta   { return e.TypeMeta }
func (e *Environment) GetObjectMeta() meta.ObjectMeta { return e.Metadata }

type EnvironmentSpec struct {
	Type         string           `json:"type" yaml:"type"`
	Description  string           `json:"description,omitempty" yaml:"description,omitempty"`
	Connection   ConnectionSpec   `json:"connection" yaml:"connection"`
	Capabilities CapabilitiesSpec `json:"capabilities" yaml:"capabilities"`
	Capacity     *CapacitySpec    `json:"capacity,omitempty" yaml:"capacity,omitempty"`
	Sovereignty  SovereigntySpec  `json:"sovereignty" yaml:"sovereignty"`
	Networking   *NetworkingSpec  `json:"networking,omitempty" yaml:"networking,omitempty"`
	Cost         *CostSpec        `json:"cost,omitempty" yaml:"cost,omitempty"`
	Recipes      map[string]map[string]RecipeBinding `json:"recipes,omitempty" yaml:"recipes,omitempty"`
}

type ConnectionSpec struct {
	Endpoint      string `json:"endpoint" yaml:"endpoint"`
	CredentialRef string `json:"credentialRef" yaml:"credentialRef"`
}

type CapabilitiesSpec struct {
	ResourceTypes []string `json:"resourceTypes" yaml:"resourceTypes"`
	Features      []string `json:"features,omitempty" yaml:"features,omitempty"`
}

type CapacitySpec struct {
	CPU     *ResourceCapacity            `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	Memory  *ResourceCapacity            `json:"memory,omitempty" yaml:"memory,omitempty"`
	Storage *ResourceCapacity            `json:"storage,omitempty" yaml:"storage,omitempty"`
	GPU     *ResourceCapacity            `json:"gpu,omitempty" yaml:"gpu,omitempty"`
	Custom  map[string]ResourceCapacity  `json:"custom,omitempty" yaml:"custom,omitempty"`
}

type ResourceCapacity struct {
	Total int    `json:"total" yaml:"total"`
	Unit  string `json:"unit" yaml:"unit"`
}

type SovereigntySpec struct {
	Country            string   `json:"country" yaml:"country"`
	Region             string   `json:"region" yaml:"region"`
	Zone               string   `json:"zone,omitempty" yaml:"zone,omitempty"`
	Jurisdiction       string   `json:"jurisdiction" yaml:"jurisdiction"`
	Compliance         []string `json:"compliance,omitempty" yaml:"compliance,omitempty"`
	DataClassification string   `json:"dataClassification" yaml:"dataClassification"`
}

type NetworkingSpec struct {
	Features []string      `json:"features,omitempty" yaml:"features,omitempty"`
	Zones    []NetworkZone `json:"zones,omitempty" yaml:"zones,omitempty"`
	Overlays []OverlaySpec `json:"overlays,omitempty" yaml:"overlays,omitempty"`
}

type NetworkZone struct {
	Name    string   `json:"name" yaml:"name"`
	Subnets []Subnet `json:"subnets,omitempty" yaml:"subnets,omitempty"`
}

type Subnet struct {
	Name         string `json:"name" yaml:"name"`
	CIDR         string `json:"cidr" yaml:"cidr"`
	Gateway      string `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	AvailableIPs int    `json:"availableIPs,omitempty" yaml:"availableIPs,omitempty"`
	VLAN         int    `json:"vlan,omitempty" yaml:"vlan,omitempty"`
}

type OverlaySpec struct {
	Name      string `json:"name" yaml:"name"`
	Type      string `json:"type" yaml:"type"`
	Latency   string `json:"latency,omitempty" yaml:"latency,omitempty"`
	Bandwidth string `json:"bandwidth,omitempty" yaml:"bandwidth,omitempty"`
}

type CostSpec struct {
	Currency string              `json:"currency" yaml:"currency"`
	Rates    CostRates           `json:"rates" yaml:"rates"`
	Custom   map[string]CostRate `json:"custom,omitempty" yaml:"custom,omitempty"`
}

type CostRates struct {
	CPU     *CostRate `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	Memory  *CostRate `json:"memory,omitempty" yaml:"memory,omitempty"`
	Storage *CostRate `json:"storage,omitempty" yaml:"storage,omitempty"`
	GPU     *CostRate `json:"gpu,omitempty" yaml:"gpu,omitempty"`
}

type CostRate struct {
	Unit  string  `json:"unit" yaml:"unit"`
	Value float64 `json:"value" yaml:"value"`
}

// RecipeBinding binds a resource type to a provisioner implementation on an environment.
type RecipeBinding struct {
	Type       string            `json:"type" yaml:"type"`
	Source     map[string]string `json:"source" yaml:"source"`
	Parameters map[string]any   `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// ResourceType defines a schema contract for a category of infrastructure resources.
type ResourceType struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta  `json:"metadata" yaml:"metadata"`
	Spec          ResourceTypeSpec `json:"spec" yaml:"spec"`
}

func (r *ResourceType) GetTypeMeta() meta.TypeMeta   { return r.TypeMeta }
func (r *ResourceType) GetObjectMeta() meta.ObjectMeta { return r.Metadata }

type ResourceTypeSpec struct {
	Version     string           `json:"version" yaml:"version"`
	Lifecycle   string           `json:"lifecycle" yaml:"lifecycle"`
	Deprecation *DeprecationSpec `json:"deprecation,omitempty" yaml:"deprecation,omitempty"`
	Schema      map[string]any   `json:"schema" yaml:"schema"`
}

type DeprecationSpec struct {
	Message  string `json:"message" yaml:"message"`
	Deadline string `json:"deadline" yaml:"deadline"`
}

// Recipe is a standalone resource that binds a ResourceType to an IaC implementation.
type Recipe struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          RecipeSpec      `json:"spec" yaml:"spec"`
}

func (r *Recipe) GetTypeMeta() meta.TypeMeta   { return r.TypeMeta }
func (r *Recipe) GetObjectMeta() meta.ObjectMeta { return r.Metadata }

type RecipeSpec struct {
	ResourceType        string            `json:"resourceType" yaml:"resourceType"`
	ResourceTypeVersion string            `json:"resourceTypeVersion,omitempty" yaml:"resourceTypeVersion,omitempty"`
	Type                string            `json:"type" yaml:"type"`
	Source              map[string]string `json:"source" yaml:"source"`
	EnvironmentMatch    *EnvMatch         `json:"environmentMatch,omitempty" yaml:"environmentMatch,omitempty"`
	Parameters          map[string]any    `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

type EnvMatch struct {
	Types []string `json:"types,omitempty" yaml:"types,omitempty"`
}

// PlacementPolicy defines rules and preferences for selecting environments.
type PlacementPolicy struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta     `json:"metadata" yaml:"metadata"`
	Spec          PlacementPolicySpec `json:"spec" yaml:"spec"`
}

func (p *PlacementPolicy) GetTypeMeta() meta.TypeMeta   { return p.TypeMeta }
func (p *PlacementPolicy) GetObjectMeta() meta.ObjectMeta { return p.Metadata }

type PlacementPolicySpec struct {
	Match    MatchCriteria `json:"match" yaml:"match"`
	Rule     string        `json:"rule,omitempty" yaml:"rule,omitempty"`
	Prefer   string        `json:"prefer,omitempty" yaml:"prefer,omitempty"`
	Weight   float64       `json:"weight,omitempty" yaml:"weight,omitempty"`
	Priority int           `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type MatchCriteria struct {
	Labels        map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	ResourceTypes []string          `json:"resourceTypes,omitempty" yaml:"resourceTypes,omitempty"`
	All           bool              `json:"all,omitempty" yaml:"all,omitempty"`
}
