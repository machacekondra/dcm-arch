package store

import "context"

// ObjectWithRevision wraps a serialized object with its storage revision
// for optimistic concurrency control.
type ObjectWithRevision struct {
	Key      string
	Value    []byte
	Revision int64
}

// EventType represents the type of a watch event.
type EventType int

const (
	EventCreate EventType = iota
	EventUpdate
	EventDelete
)

// WatchEvent represents a change observed on a key.
type WatchEvent struct {
	Type   EventType
	Object ObjectWithRevision
}

// Store is the persistence interface for DCM objects.
// Objects are stored as serialized bytes keyed by a hierarchical path:
// /registry/{kind}/{name}
type Store interface {
	// Create stores a new object. Returns the assigned revision.
	// Fails if the key already exists.
	Create(ctx context.Context, key string, value []byte) (int64, error)

	// Get retrieves a single object by key.
	// Returns nil if the key does not exist.
	Get(ctx context.Context, key string) (*ObjectWithRevision, error)

	// List retrieves all objects under a key prefix.
	List(ctx context.Context, prefix string) ([]ObjectWithRevision, error)

	// Update replaces an object at the given revision (compare-and-swap).
	// Fails if the current revision does not match.
	Update(ctx context.Context, key string, value []byte, revision int64) (int64, error)

	// Delete removes an object at the given revision (compare-and-delete).
	// Fails if the current revision does not match.
	Delete(ctx context.Context, key string, revision int64) error

	// Watch returns a channel of events for keys matching the prefix.
	Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error)

	// Close releases all resources held by the store.
	Close() error
}
