package repository

import "fmt"

const registryPrefix = "/registry"

func ApplicationKey(name string) string     { return fmt.Sprintf("%s/applications/%s", registryPrefix, name) }
func ApplicationPrefix() string             { return fmt.Sprintf("%s/applications/", registryPrefix) }

func EnvironmentKey(name string) string     { return fmt.Sprintf("%s/environments/%s", registryPrefix, name) }
func EnvironmentPrefix() string             { return fmt.Sprintf("%s/environments/", registryPrefix) }

func ResourceTypeKey(name string) string    { return fmt.Sprintf("%s/resourcetypes/%s", registryPrefix, name) }
func ResourceTypePrefix() string            { return fmt.Sprintf("%s/resourcetypes/", registryPrefix) }

func RecipeKey(name string) string          { return fmt.Sprintf("%s/recipes/%s", registryPrefix, name) }
func RecipePrefix() string                  { return fmt.Sprintf("%s/recipes/", registryPrefix) }

func PlacementPolicyKey(name string) string { return fmt.Sprintf("%s/placementpolicies/%s", registryPrefix, name) }
func PlacementPolicyPrefix() string         { return fmt.Sprintf("%s/placementpolicies/", registryPrefix) }
