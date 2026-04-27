package repository

import (
	"context"
	"fmt"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/codec"
	"github.com/dcm-io/dcm/pkg/store"
)

// Repository provides type-safe CRUD operations for a specific DCM resource type.
// It wraps the raw store and codec to eliminate repeated serialization boilerplate.
type Repository[T meta.Object] struct {
	store   store.Store
	keyFunc func(name string) string
	prefix  string
}

// New creates a Repository for the given resource type.
// keyFunc maps a resource name to its store key (e.g., ApplicationKey).
// prefix is the key prefix for listing/watching (e.g., ApplicationPrefix()).
func New[T meta.Object](s store.Store, keyFunc func(string) string, prefix string) *Repository[T] {
	return &Repository[T]{
		store:   s,
		keyFunc: keyFunc,
		prefix:  prefix,
	}
}

// Create persists a new resource. Fails if a resource with the same name already exists.
func (r *Repository[T]) Create(ctx context.Context, obj T) (int64, error) {
	name := obj.GetObjectMeta().Name
	if name == "" {
		return 0, fmt.Errorf("metadata.name is required")
	}

	data, err := codec.Encode(obj)
	if err != nil {
		return 0, fmt.Errorf("encode %s: %w", name, err)
	}

	rev, err := r.store.Create(ctx, r.keyFunc(name), data)
	if err != nil {
		return 0, err
	}
	return rev, nil
}

// Get retrieves a resource by name. Returns the object, its revision, and any error.
// If the resource does not exist, returns a nil object and revision 0 with no error.
func (r *Repository[T]) Get(ctx context.Context, name string) (T, int64, error) {
	var zero T

	obj, err := r.store.Get(ctx, r.keyFunc(name))
	if err != nil {
		return zero, 0, err
	}
	if obj == nil {
		return zero, 0, nil
	}

	decoded, err := codec.Decode(obj.Value)
	if err != nil {
		return zero, 0, fmt.Errorf("decode %s: %w", name, err)
	}

	typed, ok := decoded.(T)
	if !ok {
		return zero, 0, fmt.Errorf("unexpected type for %s: got %T", name, decoded)
	}
	return typed, obj.Revision, nil
}

// List retrieves all resources of this type.
func (r *Repository[T]) List(ctx context.Context) ([]T, error) {
	objects, err := r.store.List(ctx, r.prefix)
	if err != nil {
		return nil, err
	}

	results := make([]T, 0, len(objects))
	for _, obj := range objects {
		decoded, err := codec.Decode(obj.Value)
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		typed, ok := decoded.(T)
		if !ok {
			return nil, fmt.Errorf("unexpected type: got %T", decoded)
		}
		results = append(results, typed)
	}
	return results, nil
}

// Update replaces a resource at the given revision (optimistic concurrency).
// Fails if the current revision does not match.
func (r *Repository[T]) Update(ctx context.Context, obj T, revision int64) (int64, error) {
	name := obj.GetObjectMeta().Name
	if name == "" {
		return 0, fmt.Errorf("metadata.name is required")
	}

	data, err := codec.Encode(obj)
	if err != nil {
		return 0, fmt.Errorf("encode %s: %w", name, err)
	}

	rev, err := r.store.Update(ctx, r.keyFunc(name), data, revision)
	if err != nil {
		return 0, err
	}
	return rev, nil
}

// Delete removes a resource at the given revision (optimistic concurrency).
// Fails if the current revision does not match.
func (r *Repository[T]) Delete(ctx context.Context, name string, revision int64) error {
	return r.store.Delete(ctx, r.keyFunc(name), revision)
}

// WatchEvent represents a typed change event.
type WatchEvent[T meta.Object] struct {
	Type     store.EventType
	Object   T
	Revision int64
}

// Watch returns a channel of typed events for resources of this type.
func (r *Repository[T]) Watch(ctx context.Context) (<-chan WatchEvent[T], error) {
	rawCh, err := r.store.Watch(ctx, r.prefix)
	if err != nil {
		return nil, err
	}

	typedCh := make(chan WatchEvent[T])
	go func() {
		defer close(typedCh)
		for raw := range rawCh {
			if raw.Type == store.EventDelete {
				// For deletes, value may be empty; send event with zero object
				var zero T
				typedCh <- WatchEvent[T]{
					Type:     raw.Type,
					Revision: raw.Object.Revision,
					Object:   zero,
				}
				continue
			}

			decoded, err := codec.Decode(raw.Object.Value)
			if err != nil {
				continue
			}
			typed, ok := decoded.(T)
			if !ok {
				continue
			}
			typedCh <- WatchEvent[T]{
				Type:     raw.Type,
				Object:   typed,
				Revision: raw.Object.Revision,
			}
		}
	}()

	return typedCh, nil
}
