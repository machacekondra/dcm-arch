package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/k3s-io/kine/pkg/endpoint"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Kine implements a subset of etcd API used by Kubernetes.
// All writes go through Txn — plain Put/Delete are not supported.
// These helpers match the exact transaction patterns kine recognizes.

// kineCreate creates a new key (fails if key already exists).
// Pattern: If ModRevision == 0 (key doesn't exist), Then Put.
func kineCreate(ctx context.Context, client *clientv3.Client, key, value string) (*clientv3.TxnResponse, error) {
	return client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", 0)).
		Then(clientv3.OpPut(key, value)).
		Commit()
}

// kineUpdate updates an existing key at a specific revision (compare-and-swap).
// Pattern: If ModRevision == rev, Then Put, Else Get (to return current value).
func kineUpdate(ctx context.Context, client *clientv3.Client, key, value string, rev int64) (*clientv3.TxnResponse, error) {
	return client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", rev)).
		Then(clientv3.OpPut(key, value)).
		Else(clientv3.OpGet(key)).
		Commit()
}

// kineDelete deletes a key at a specific revision (compare-and-delete).
// Pattern: If ModRevision == rev, Then Delete, Else Get (to return current value).
func kineDelete(ctx context.Context, client *clientv3.Client, key string, rev int64) (*clientv3.TxnResponse, error) {
	return client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", rev)).
		Then(clientv3.OpDelete(key)).
		Else(clientv3.OpGet(key)).
		Commit()
}

func main() {
	ctx := context.Background()

	// Start kine with SQLite backend.
	// This starts a gRPC server that speaks a subset of etcd v3 API,
	// backed by a SQLite database file.
	config, err := endpoint.Listen(ctx, endpoint.Config{
		Endpoint:       "sqlite:///tmp/kine-demo.db",
		Listener:       "unix://kine.sock",
		NotifyInterval: 5 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to start kine: %v", err)
	}

	// Create an etcd client pointing at the kine gRPC endpoint
	client, err := clientv3.New(clientv3.Config{
		Endpoints: config.Endpoints,
	})
	if err != nil {
		log.Fatalf("Failed to create etcd client: %v", err)
	}
	defer client.Close()

	fmt.Println("=== Kine SQLite Demo ===")
	fmt.Println()

	// CREATE: store new key-value pairs using kine's transaction pattern
	fmt.Println("--- CREATE keys ---")
	entries := []struct{ key, value string }{
		{"/registry/services/default/nginx", `{"name":"nginx","port":80}`},
		{"/registry/services/default/redis", `{"name":"redis","port":6379}`},
		{"/registry/services/kube-system/dns", `{"name":"dns","port":53}`},
	}
	for _, e := range entries {
		resp, err := kineCreate(ctx, client, e.key, e.value)
		if err != nil {
			log.Fatalf("CREATE %s failed: %v", e.key, err)
		}
		fmt.Printf("  CREATE %s succeeded=%v\n", e.key, resp.Succeeded)
	}
	fmt.Println()

	// CREATE duplicate — should fail (Succeeded=false)
	fmt.Println("--- CREATE duplicate (should fail) ---")
	resp, err := kineCreate(ctx, client, "/registry/services/default/nginx", `{"name":"nginx","port":99}`)
	if err != nil {
		log.Fatalf("CREATE duplicate failed: %v", err)
	}
	fmt.Printf("  CREATE duplicate succeeded=%v (expected false)\n", resp.Succeeded)
	fmt.Println()

	// GET: retrieve a single key (Range still works normally)
	fmt.Println("--- GET single key ---")
	getResp, err := client.Get(ctx, "/registry/services/default/nginx")
	if err != nil {
		log.Fatalf("GET failed: %v", err)
	}
	for _, kv := range getResp.Kvs {
		fmt.Printf("  Key=%s Value=%s Rev=%d\n", kv.Key, kv.Value, kv.ModRevision)
	}
	fmt.Println()

	// GET with prefix: list all keys under a prefix
	fmt.Println("--- LIST with prefix /registry/services/default/ ---")
	getResp, err = client.Get(ctx, "/registry/services/default/", clientv3.WithPrefix())
	if err != nil {
		log.Fatalf("LIST failed: %v", err)
	}
	for _, kv := range getResp.Kvs {
		fmt.Printf("  Key=%s Value=%s\n", kv.Key, kv.Value)
	}
	fmt.Println()

	// UPDATE: modify a key using compare-and-swap on its current revision
	fmt.Println("--- UPDATE key (compare-and-swap) ---")
	currentRev := getResp.Kvs[0].ModRevision
	txnResp, err := kineUpdate(ctx, client, "/registry/services/default/nginx", `{"name":"nginx","port":8080}`, currentRev)
	if err != nil {
		log.Fatalf("UPDATE failed: %v", err)
	}
	fmt.Printf("  UPDATE succeeded=%v (new header rev=%d)\n", txnResp.Succeeded, txnResp.Header.Revision)

	// Verify the update
	getResp, err = client.Get(ctx, "/registry/services/default/nginx")
	if err != nil {
		log.Fatalf("GET after update failed: %v", err)
	}
	for _, kv := range getResp.Kvs {
		fmt.Printf("  After update: Key=%s Value=%s Rev=%d\n", kv.Key, kv.Value, kv.ModRevision)
	}
	fmt.Println()

	// UPDATE with stale revision — should fail (conflict detection)
	fmt.Println("--- UPDATE with stale revision (should fail) ---")
	txnResp, err = kineUpdate(ctx, client, "/registry/services/default/nginx", `{"name":"nginx","port":9999}`, currentRev)
	if err != nil {
		log.Fatalf("UPDATE stale failed: %v", err)
	}
	fmt.Printf("  UPDATE with stale rev succeeded=%v (expected false)\n", txnResp.Succeeded)
	if !txnResp.Succeeded && len(txnResp.Responses) > 0 {
		rangeResp := txnResp.Responses[0].GetResponseRange()
		if rangeResp != nil && len(rangeResp.Kvs) > 0 {
			fmt.Printf("  Current value: %s (rev=%d)\n", rangeResp.Kvs[0].Value, rangeResp.Kvs[0].ModRevision)
		}
	}
	fmt.Println()

	// DELETE: remove a key using compare-and-delete
	fmt.Println("--- DELETE key ---")
	getResp, err = client.Get(ctx, "/registry/services/kube-system/dns")
	if err != nil {
		log.Fatalf("GET before delete failed: %v", err)
	}
	delRev := getResp.Kvs[0].ModRevision
	txnResp, err = kineDelete(ctx, client, "/registry/services/kube-system/dns", delRev)
	if err != nil {
		log.Fatalf("DELETE failed: %v", err)
	}
	fmt.Printf("  DELETE succeeded=%v\n", txnResp.Succeeded)
	fmt.Println()

	// LIST all remaining keys
	fmt.Println("--- LIST all remaining keys ---")
	getResp, err = client.Get(ctx, "/registry/", clientv3.WithPrefix())
	if err != nil {
		log.Fatalf("LIST all failed: %v", err)
	}
	for _, kv := range getResp.Kvs {
		fmt.Printf("  Key=%s Value=%s\n", kv.Key, kv.Value)
	}
	fmt.Println()

	// WATCH: demonstrate watching for changes
	fmt.Println("--- WATCH demo ---")
	watchCh := client.Watch(ctx, "/registry/services/", clientv3.WithPrefix())

	// Make a change in a goroutine so the watch picks it up
	go func() {
		_, err := kineCreate(ctx, client, "/registry/services/default/postgres", `{"name":"postgres","port":5432}`)
		if err != nil {
			log.Printf("CREATE in goroutine failed: %v", err)
		}
	}()

	// Read one watch event
	watchResp := <-watchCh
	for _, ev := range watchResp.Events {
		fmt.Printf("  Watch event: Type=%s Key=%s Value=%s\n",
			strings.ToUpper(ev.Type.String()), ev.Kv.Key, ev.Kv.Value)
	}

	fmt.Println()
	fmt.Println("=== Demo complete ===")
}
