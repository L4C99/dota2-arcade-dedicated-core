package core

import (
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
	"testing"
	"time"
)

func TestBackwardClockLifecycleAndReopen(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	previous := m.store.State.Instances[id].UpdatedAt
	m.clock = func() time.Time { return previous.Add(-24 * time.Hour) }
	m.mu.Unlock()
	r := success(t, call(t, m, "restart", map[string]any{"instanceId": id}))
	if r := waitOperation(t, m, r["operationId"].(string)); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.Close()
	saved, e := records.Open(m.store.Dir)
	if e != nil {
		t.Fatal(e)
	}
	if saved.State.Instances[id].UpdatedAt.Before(previous) {
		t.Fatal("time regressed")
	}
	next, e := Open(m.store.Dir)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	success(t, call(t, next, "status", map[string]any{"instanceId": id}))
	r = success(t, call(t, next, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "after-clock"}))
	if result := waitOperation(t, next, r["operationId"].(string)); result["status"] != "succeeded" {
		t.Fatal(result)
	}
	stopRoom(t, next, r["instanceId"].(string))
}

func TestBackwardClockRecoveryAndWatch(t *testing.T) {
	m, path, port := setupManager(t, "exit-later")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	future := time.Now().Add(24 * time.Hour).UTC()
	in := m.store.State.Instances[id]
	in.CreatedAt = future
	in.UpdatedAt = future
	m.clock = func() time.Time { return future.Add(-48 * time.Hour) }
	if e := m.store.Save(); e != nil {
		t.Fatal(e)
	}
	m.mu.Unlock()
	time.Sleep(1500 * time.Millisecond)
	m.mu.Lock()
	if m.writeError != nil {
		t.Fatal(m.writeError)
	}
	if in.UpdatedAt.Before(future) {
		t.Fatal("watch time regressed")
	}
	m.mu.Unlock()
	m.Close()
	next, e := Open(m.store.Dir)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	if r := stopRoom(t, next, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}

	// Interrupted intent has no child: recoveryFailure must complete an operation
	// whose creation time is ahead of the corrected current wall clock.
	next.Close()
	in2, op2, e := next.store.Create(path, port, "future-intent")
	if e != nil {
		t.Fatal(e)
	}
	in2.CreatedAt = future
	in2.UpdatedAt = future
	op2.CreatedAt = future
	if e = next.store.Save(); e != nil {
		t.Fatal(e)
	}
	last, e := Open(next.store.Dir)
	if e != nil {
		t.Fatal(e)
	}
	defer last.Close()
	if r := waitOperation(t, last, op2.ID); r["status"] != "failed" {
		t.Fatal(r)
	}
	if r := stopRoom(t, last, in2.ID); r["status"] != "succeeded" {
		t.Fatal(r)
	}
}
