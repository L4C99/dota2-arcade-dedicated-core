package core

import (
	"testing"
	"time"
)

func TestLowDiskRejectsNewIntentButAllowsRetryAndStop(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if got := waitOperation(t, m, op); got["status"] != "succeeded" {
		t.Fatal(got)
	}
	m.mu.Lock()
	m.freeBytes = func(string) (uint64, error) { return 0, nil }
	m.maintainStorage(time.Now().UTC())
	m.mu.Unlock()
	got := call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "new-low-space"})
	if got.OK || got.Error == nil || got.Error.Code != "INSUFFICIENT_STORAGE" {
		t.Fatal(got)
	}
	// Use the original durable key without assuming the fixture's spelling.
	m.mu.Lock()
	var key string
	for k := range m.store.State.Keys {
		key = k
	}
	m.mu.Unlock()
	retry := success(t, call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": key}))
	if retry["instanceId"] != id || retry["operationId"] != op {
		t.Fatal(retry)
	}
	got = call(t, m, "restart", map[string]any{"instanceId": id})
	if got.OK || got.Error == nil || got.Error.Code != "INSUFFICIENT_STORAGE" {
		t.Fatal(got)
	}
	if got := stopRoom(t, m, id); got["status"] != "succeeded" {
		t.Fatal(got)
	}
}
