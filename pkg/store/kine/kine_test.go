package kine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dcm-io/dcm/pkg/store"
)

func setupTestStore(t *testing.T) (*KineStore, context.Context) {
	t.Helper()
	dir, err := os.MkdirTemp("", "dcm-kine-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	ctx := context.Background()
	s, err := New(ctx, Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return s, ctx
}

func TestCreateAndGet(t *testing.T) {
	s, ctx := setupTestStore(t)

	key := "/registry/applications/test-app"
	value := []byte(`{"apiVersion":"dcm.io/v1alpha1","kind":"Application"}`)

	rev, err := s.Create(ctx, key, value)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rev == 0 {
		t.Fatal("expected non-zero revision")
	}

	obj, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if obj == nil {
		t.Fatal("expected object, got nil")
	}
	if string(obj.Value) != string(value) {
		t.Errorf("value: got %q, want %q", obj.Value, value)
	}
}

func TestCreateDuplicate(t *testing.T) {
	s, ctx := setupTestStore(t)

	key := "/registry/applications/dup"
	value := []byte(`test`)

	_, err := s.Create(ctx, key, value)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = s.Create(ctx, key, value)
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestGetNotFound(t *testing.T) {
	s, ctx := setupTestStore(t)

	obj, err := s.Get(ctx, "/registry/applications/nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if obj != nil {
		t.Fatalf("expected nil, got %+v", obj)
	}
}

func TestUpdate(t *testing.T) {
	s, ctx := setupTestStore(t)

	key := "/registry/applications/update-me"
	_, err := s.Create(ctx, key, []byte("v1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	obj, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	newRev, err := s.Update(ctx, key, []byte("v2"), obj.Revision)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if newRev == 0 {
		t.Fatal("expected non-zero revision after update")
	}

	obj, err = s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if string(obj.Value) != "v2" {
		t.Errorf("value after update: got %q, want %q", obj.Value, "v2")
	}
}

func TestUpdateConflict(t *testing.T) {
	s, ctx := setupTestStore(t)

	key := "/registry/applications/conflict"
	_, err := s.Create(ctx, key, []byte("v1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = s.Update(ctx, key, []byte("v2"), 9999)
	if err == nil {
		t.Fatal("expected error on revision conflict")
	}
}

func TestDelete(t *testing.T) {
	s, ctx := setupTestStore(t)

	key := "/registry/applications/delete-me"
	_, err := s.Create(ctx, key, []byte("data"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	obj, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	err = s.Delete(ctx, key, obj.Revision)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	obj, err = s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if obj != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestList(t *testing.T) {
	s, ctx := setupTestStore(t)

	prefix := "/registry/environments/"
	keys := []string{
		prefix + "env-a",
		prefix + "env-b",
		prefix + "env-c",
	}
	for _, k := range keys {
		if _, err := s.Create(ctx, k, []byte("data-"+k)); err != nil {
			t.Fatalf("Create %s: %v", k, err)
		}
	}

	results, err := s.List(ctx, prefix)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("List: got %d results, want 3", len(results))
	}
}

func TestWatch(t *testing.T) {
	s, ctx := setupTestStore(t)

	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	prefix := "/registry/applications/"
	ch, err := s.Watch(watchCtx, prefix)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Create an object to trigger a watch event
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Create(ctx, prefix+"watched", []byte("watch-data"))
	}()

	select {
	case ev := <-ch:
		if ev.Type != store.EventCreate {
			t.Errorf("event type: got %d, want EventCreate", ev.Type)
		}
		if string(ev.Object.Value) != "watch-data" {
			t.Errorf("event value: got %q, want %q", ev.Object.Value, "watch-data")
		}
	case <-watchCtx.Done():
		t.Fatal("timeout waiting for watch event")
	}
}
