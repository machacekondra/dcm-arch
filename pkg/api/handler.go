package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/codec"
	"github.com/dcm-io/dcm/pkg/repository"
)

// Handler provides HTTP CRUD handlers for a specific DCM resource type.
type Handler[T meta.Object] struct {
	repo *repository.Repository[T]
	kind string
}

// NewHandler creates a Handler for the given resource type.
func NewHandler[T meta.Object](repo *repository.Repository[T], kind string) *Handler[T] {
	return &Handler[T]{repo: repo, kind: kind}
}

// Create handles POST requests to create a new resource.
func (h *Handler[T]) Create(w http.ResponseWriter, r *http.Request) {
	obj, err := h.decodeBody(r)
	if err != nil {
		badRequest(w, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	name := obj.GetObjectMeta().Name
	if name == "" {
		badRequest(w, "metadata.name is required")
		return
	}

	rev, err := h.repo.Create(r.Context(), obj)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			conflict(w, fmt.Sprintf("%s %q already exists", h.kind, name))
			return
		}
		internalError(w, err.Error())
		return
	}

	w.Header().Set("X-DCM-Revision", strconv.FormatInt(rev, 10))
	h.writeResponse(w, r, http.StatusCreated, obj)
}

// Get handles GET requests for a single resource by name.
func (h *Handler[T]) Get(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		badRequest(w, "name is required")
		return
	}

	obj, rev, err := h.repo.Get(r.Context(), name)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if rev == 0 {
		notFound(w, fmt.Sprintf("%s %q not found", h.kind, name))
		return
	}

	w.Header().Set("X-DCM-Revision", strconv.FormatInt(rev, 10))
	h.writeResponse(w, r, http.StatusOK, obj)
}

// List handles GET requests to list all resources of this type.
func (h *Handler[T]) List(w http.ResponseWriter, r *http.Request) {
	objects, err := h.repo.List(r.Context())
	if err != nil {
		internalError(w, err.Error())
		return
	}

	h.writeListResponse(w, r, objects)
}

// Update handles PUT requests to update an existing resource.
func (h *Handler[T]) Update(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		badRequest(w, "name is required")
		return
	}

	rev, err := parseRevisionHeader(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeRevisionNeeded, err.Error())
		return
	}

	obj, err := h.decodeBody(r)
	if err != nil {
		badRequest(w, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if obj.GetObjectMeta().Name != name {
		badRequest(w, fmt.Sprintf("metadata.name %q does not match URL path %q", obj.GetObjectMeta().Name, name))
		return
	}

	newRev, err := h.repo.Update(r.Context(), obj, rev)
	if err != nil {
		if strings.Contains(err.Error(), "revision conflict") {
			conflict(w, fmt.Sprintf("%s %q revision conflict", h.kind, name))
			return
		}
		internalError(w, err.Error())
		return
	}

	w.Header().Set("X-DCM-Revision", strconv.FormatInt(newRev, 10))
	h.writeResponse(w, r, http.StatusOK, obj)
}

// Delete handles DELETE requests to remove a resource.
func (h *Handler[T]) Delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		badRequest(w, "name is required")
		return
	}

	rev, err := parseRevisionHeader(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrCodeRevisionNeeded, err.Error())
		return
	}

	err = h.repo.Delete(r.Context(), name, rev)
	if err != nil {
		if strings.Contains(err.Error(), "revision conflict") {
			conflict(w, fmt.Sprintf("%s %q revision conflict", h.kind, name))
			return
		}
		internalError(w, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler[T]) decodeBody(r *http.Request) (T, error) {
	var zero T
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return zero, fmt.Errorf("read body: %w", err)
	}
	defer r.Body.Close()

	decoded, err := codec.Decode(body)
	if err != nil {
		return zero, err
	}

	typed, ok := decoded.(T)
	if !ok {
		return zero, fmt.Errorf("expected %s, got %T", h.kind, decoded)
	}
	return typed, nil
}

func (h *Handler[T]) writeResponse(w http.ResponseWriter, r *http.Request, status int, obj T) {
	if wantsYAML(r) {
		data, err := codec.Encode(obj)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(status)
		w.Write(data)
		return
	}

	data, err := codec.EncodeJSON(obj)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func (h *Handler[T]) writeListResponse(w http.ResponseWriter, r *http.Request, objects []T) {
	// List always returns JSON with an items wrapper
	type listResponse struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Items      []T    `json:"items"`
	}

	resp := listResponse{
		APIVersion: "dcm.io/v1alpha1",
		Kind:       h.kind + "List",
		Items:      objects,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("failed to encode list response: %v", err)
	}
}

func parseRevisionHeader(r *http.Request) (int64, error) {
	h := r.Header.Get("X-DCM-Revision")
	if h == "" {
		return 0, fmt.Errorf("X-DCM-Revision header is required for this operation")
	}
	rev, err := strconv.ParseInt(h, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid X-DCM-Revision: %w", err)
	}
	return rev, nil
}

func wantsYAML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/yaml") || strings.Contains(accept, "text/yaml")
}
