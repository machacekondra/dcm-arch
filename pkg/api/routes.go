package api

import (
	"context"
	"net/http"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/engine"
	"github.com/dcm-io/dcm/pkg/placement"
	"github.com/dcm-io/dcm/pkg/repository"
	"github.com/dcm-io/dcm/pkg/schema"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/validation"
)

const basePath = "/apis/dcm.io/v1alpha1"

// RegisterRoutes creates repositories for all resource types and registers
// their CRUD handlers on the given mux.
func RegisterRoutes(mux *http.ServeMux, s store.Store) {
	rtRepo := repository.New[*v1alpha1.ResourceType](s, repository.ResourceTypeKey, repository.ResourceTypePrefix())
	appRepo := repository.New[*v1alpha1.Application](s, repository.ApplicationKey, repository.ApplicationPrefix())
	envRepo := repository.New[*v1alpha1.Environment](s, repository.EnvironmentKey, repository.EnvironmentPrefix())
	policyRepo := repository.New[*v1alpha1.PlacementPolicy](s, repository.PlacementPolicyKey, repository.PlacementPolicyPrefix())

	registerHandlers(mux, "applications", NewHandler(
		appRepo,
		v1alpha1.KindApplication,
		applicationValidator(rtRepo),
	))
	registerHandlers(mux, "environments", NewHandler(
		envRepo,
		v1alpha1.KindEnvironment,
		func(env *v1alpha1.Environment) error { return validation.ValidateEnvironment(env).Error() },
	))
	registerHandlers(mux, "resourcetypes", NewHandler(
		rtRepo,
		v1alpha1.KindResourceType,
		func(rt *v1alpha1.ResourceType) error { return validation.ValidateResourceType(rt).Error() },
	))
	registerHandlers(mux, "recipes", NewHandler(
		repository.New[*v1alpha1.Recipe](s, repository.RecipeKey, repository.RecipePrefix()),
		v1alpha1.KindRecipe,
		func(r *v1alpha1.Recipe) error { return validation.ValidateRecipe(r).Error() },
	))
	registerHandlers(mux, "placementpolicies", NewHandler(
		policyRepo,
		v1alpha1.KindPlacementPolicy,
		func(pp *v1alpha1.PlacementPolicy) error { return validation.ValidatePlacementPolicy(pp).Error() },
	))

	// Deployments (read-only via CRUD — created by deploy handler)
	deployRepo := repository.New[*v1alpha1.Deployment](s, repository.DeploymentKey, repository.DeploymentPrefix())
	registerHandlers(mux, "deployments", NewHandler(deployRepo, v1alpha1.KindDeployment))

	// Placement simulation endpoint
	placer, _ := placement.NewEngine()
	ph := &PlacementHandler{appRepo: appRepo, envRepo: envRepo, policyRepo: policyRepo, engine: placer}
	mux.HandleFunc("GET "+basePath+"/placement/{name}", ph.simulate)

	// Deploy endpoint (placement + execution with mock driver)
	mockDriver := engine.NewMockDriver()
	executor := engine.NewExecutor(map[string]engine.Driver{"mock": mockDriver})
	dh := &DeployHandler{appRepo: appRepo, envRepo: envRepo, policyRepo: policyRepo, deployRepo: deployRepo, placer: placer, executor: executor}
	mux.HandleFunc("POST "+basePath+"/deploy/{name}", dh.deploy)
}

// applicationValidator returns a validator that performs both structural and
// schema validation for Applications. Schema validation loads registered
// ResourceTypes from the store to validate properties against their schemas.
func applicationValidator(rtRepo *repository.Repository[*v1alpha1.ResourceType]) ValidatorFunc[*v1alpha1.Application] {
	return func(app *v1alpha1.Application) error {
		// Structural validation first
		if err := validation.ValidateApplication(app).Error(); err != nil {
			return err
		}

		// Collect unique resource types needed
		typeNames := make(map[string]bool)
		for _, res := range app.Spec.Resources {
			typeNames[res.Type] = true
		}

		// Load ResourceTypes from store
		types := make(map[string]*v1alpha1.ResourceType)
		for name := range typeNames {
			rt, _, err := rtRepo.Get(context.Background(), name)
			if err != nil {
				return err
			}
			// rt may be nil (not found) — schema.ValidateApplication handles that
			if rt != nil {
				types[name] = rt
			}
		}

		// Schema validation
		return schema.ValidateApplication(app, types)
	}
}

type routeRegistrar interface {
	Create(http.ResponseWriter, *http.Request)
	Get(http.ResponseWriter, *http.Request)
	List(http.ResponseWriter, *http.Request)
	Update(http.ResponseWriter, *http.Request)
	Delete(http.ResponseWriter, *http.Request)
}

func registerHandlers(mux *http.ServeMux, resource string, h routeRegistrar) {
	path := basePath + "/" + resource
	mux.HandleFunc("POST "+path, h.Create)
	mux.HandleFunc("GET "+path, h.List)
	mux.HandleFunc("GET "+path+"/{name}", h.Get)
	mux.HandleFunc("PUT "+path+"/{name}", h.Update)
	mux.HandleFunc("DELETE "+path+"/{name}", h.Delete)
}
