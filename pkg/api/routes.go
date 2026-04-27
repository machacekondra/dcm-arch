package api

import (
	"net/http"

	"github.com/dcm-io/dcm/pkg/apis/v1alpha1"
	"github.com/dcm-io/dcm/pkg/repository"
	"github.com/dcm-io/dcm/pkg/store"
)

const basePath = "/apis/dcm.io/v1alpha1"

// RegisterRoutes creates repositories for all resource types and registers
// their CRUD handlers on the given mux.
func RegisterRoutes(mux *http.ServeMux, s store.Store) {
	registerHandlers(mux, "applications", NewHandler(
		repository.New[*v1alpha1.Application](s, repository.ApplicationKey, repository.ApplicationPrefix()),
		v1alpha1.KindApplication,
	))
	registerHandlers(mux, "environments", NewHandler(
		repository.New[*v1alpha1.Environment](s, repository.EnvironmentKey, repository.EnvironmentPrefix()),
		v1alpha1.KindEnvironment,
	))
	registerHandlers(mux, "resourcetypes", NewHandler(
		repository.New[*v1alpha1.ResourceType](s, repository.ResourceTypeKey, repository.ResourceTypePrefix()),
		v1alpha1.KindResourceType,
	))
	registerHandlers(mux, "recipes", NewHandler(
		repository.New[*v1alpha1.Recipe](s, repository.RecipeKey, repository.RecipePrefix()),
		v1alpha1.KindRecipe,
	))
	registerHandlers(mux, "placementpolicies", NewHandler(
		repository.New[*v1alpha1.PlacementPolicy](s, repository.PlacementPolicyKey, repository.PlacementPolicyPrefix()),
		v1alpha1.KindPlacementPolicy,
	))
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
