package kine

import (
	"context"
	"fmt"
	"time"

	"github.com/dcm-io/dcm/pkg/store"
	"github.com/k3s-io/kine/pkg/endpoint"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// KineStore implements store.Store using kine with a SQLite backend.
type KineStore struct {
	client *clientv3.Client
}

// Config holds configuration for the kine store.
type Config struct {
	// DataDir is the directory where the SQLite database file is stored.
	DataDir string
	// Listener is the kine gRPC listener address (e.g., "unix://kine.sock").
	Listener string
}

// New creates a new KineStore backed by SQLite via kine.
func New(ctx context.Context, cfg Config) (*KineStore, error) {
	dbPath := fmt.Sprintf("sqlite://%s/dcm.db", cfg.DataDir)
	listener := cfg.Listener
	if listener == "" {
		listener = fmt.Sprintf("unix://%s/kine.sock", cfg.DataDir)
	}

	etcdCfg, err := endpoint.Listen(ctx, endpoint.Config{
		Endpoint:       dbPath,
		Listener:       listener,
		NotifyInterval: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start kine: %w", err)
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints: etcdCfg.Endpoints,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &KineStore{client: client}, nil
}

func (s *KineStore) Create(ctx context.Context, key string, value []byte) (int64, error) {
	resp, err := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", 0)).
		Then(clientv3.OpPut(key, string(value))).
		Commit()
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", key, err)
	}
	if !resp.Succeeded {
		return 0, fmt.Errorf("create %s: key already exists", key)
	}
	return resp.Header.Revision, nil
}

func (s *KineStore) Get(ctx context.Context, key string) (*store.ObjectWithRevision, error) {
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, nil
	}
	kv := resp.Kvs[0]
	return &store.ObjectWithRevision{
		Key:      string(kv.Key),
		Value:    kv.Value,
		Revision: kv.ModRevision,
	}, nil
}

func (s *KineStore) List(ctx context.Context, prefix string) ([]store.ObjectWithRevision, error) {
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", prefix, err)
	}
	results := make([]store.ObjectWithRevision, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		results = append(results, store.ObjectWithRevision{
			Key:      string(kv.Key),
			Value:    kv.Value,
			Revision: kv.ModRevision,
		})
	}
	return results, nil
}

func (s *KineStore) Update(ctx context.Context, key string, value []byte, revision int64) (int64, error) {
	resp, err := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", revision)).
		Then(clientv3.OpPut(key, string(value))).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return 0, fmt.Errorf("update %s: %w", key, err)
	}
	if !resp.Succeeded {
		return 0, fmt.Errorf("update %s: revision conflict (expected %d)", key, revision)
	}
	return resp.Header.Revision, nil
}

func (s *KineStore) Delete(ctx context.Context, key string, revision int64) error {
	resp, err := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", revision)).
		Then(clientv3.OpDelete(key)).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	if !resp.Succeeded {
		return fmt.Errorf("delete %s: revision conflict (expected %d)", key, revision)
	}
	return nil
}

func (s *KineStore) Watch(ctx context.Context, prefix string) (<-chan store.WatchEvent, error) {
	watchCh := s.client.Watch(ctx, prefix, clientv3.WithPrefix())
	eventCh := make(chan store.WatchEvent)

	go func() {
		defer close(eventCh)
		for resp := range watchCh {
			for _, ev := range resp.Events {
				var eventType store.EventType
				switch ev.Type {
				case clientv3.EventTypePut:
					if ev.Kv.CreateRevision == ev.Kv.ModRevision {
						eventType = store.EventCreate
					} else {
						eventType = store.EventUpdate
					}
				case clientv3.EventTypeDelete:
					eventType = store.EventDelete
				}
				eventCh <- store.WatchEvent{
					Type: eventType,
					Object: store.ObjectWithRevision{
						Key:      string(ev.Kv.Key),
						Value:    ev.Kv.Value,
						Revision: ev.Kv.ModRevision,
					},
				}
			}
		}
	}()

	return eventCh, nil
}

func (s *KineStore) Close() error {
	return s.client.Close()
}
