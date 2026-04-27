package controller

import (
	"context"
	"log"

	"github.com/dcm-io/dcm/pkg/apis/meta"
	"github.com/dcm-io/dcm/pkg/repository"
	"github.com/dcm-io/dcm/pkg/store"
)

// Reconciler processes a single resource by name.
type Reconciler interface {
	Reconcile(ctx context.Context, name string) error
}

// Controller watches a store prefix for changes and calls the reconciler.
type Controller struct {
	repo       watchLister
	reconciler Reconciler
	kind       string
}

type watchLister interface {
	Watch(ctx context.Context) (<-chan repository.WatchEvent[meta.Object], error)
}

// GenericController watches a typed repository and reconciles on changes.
type GenericController[T meta.Object] struct {
	repo       *repository.Repository[T]
	reconciler Reconciler
	kind       string
}

// NewGenericController creates a controller that watches the given repository.
func NewGenericController[T meta.Object](
	repo *repository.Repository[T],
	reconciler Reconciler,
	kind string,
) *GenericController[T] {
	return &GenericController[T]{
		repo:       repo,
		reconciler: reconciler,
		kind:       kind,
	}
}

// Run starts the watch loop. It blocks until the context is cancelled.
func (c *GenericController[T]) Run(ctx context.Context) error {
	ch, err := c.repo.Watch(ctx)
	if err != nil {
		return err
	}

	log.Printf("%s controller started", c.kind)

	for {
		select {
		case <-ctx.Done():
			log.Printf("%s controller stopped", c.kind)
			return ctx.Err()
		case event, ok := <-ch:
			if !ok {
				log.Printf("%s controller: watch channel closed", c.kind)
				return nil
			}
			if event.Type == store.EventDelete {
				continue
			}
			name := event.Object.GetObjectMeta().Name
			log.Printf("%s controller: reconciling %q", c.kind, name)
			if err := c.reconciler.Reconcile(ctx, name); err != nil {
				log.Printf("%s controller: reconcile %q failed: %v", c.kind, name, err)
			}
		}
	}
}
