package core

import (
	"testing"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

func TestTerminalWorkerAllowsRestartBeforeTeardown(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	// Drain actual teardown, then deterministically hold an equivalent worker
	// between finish's terminal publication and deferred finishWorker.
	m.mu.Lock()
	prior := m.workers[id]
	m.mu.Unlock()
	if prior != nil {
		<-prior.done
	}
	m.mu.Lock()
	in := m.store.State.Instances[id]
	w := m.newWorker(in, "create", op)
	m.store.State.Operations[op].Status = "running"
	m.finish(in, w, "succeeded", nil)
	m.mu.Unlock()
	released := false
	defer func() {
		if !released {
			m.finishWorker(id, w)
		}
	}()
	if r := success(t, call(t, m, "operation", map[string]any{"operationId": op})); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	r := success(t, call(t, m, "restart", map[string]any{"instanceId": id}))
	restartID := r["operationId"].(string)
	m.mu.Lock()
	next := m.workers[id]
	generation := in.Generation
	phase := m.store.State.Operations[restartID].Phase
	m.mu.Unlock()
	if next == w || generation != 1 || phase != "accepted" {
		t.Fatalf("restart did not wait for old teardown: generation=%d phase=%s", generation, phase)
	}
	m.finishWorker(id, w)
	released = true
	if r := waitOperation(t, m, restartID); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	state := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if state["generation"] != float64(2) || state["room"] != "ready" {
		t.Fatal(state)
	}
	// Replaying the creation key still returns the original operation.
	if replayID, replayOp := createRoom(t, m, path, port); replayID != id || replayOp != op {
		t.Fatal("terminal handoff changed idempotency")
	}
	if stopped := stopRoom(t, m, id); stopped["status"] != "succeeded" {
		t.Fatal(stopped)
	}
}

func TestRunningWorkerStillRejectsRestart(t *testing.T) {
	// No process needed: the busy guard must precede running-state validation.
	m := &Manager{workers: map[string]*worker{"instance": {operationID: "op"}}, store: &records.Store{}}
	m.store.State.Operations = map[string]*records.Operation{"op": {Status: "running"}}
	_, failure := m.change(&records.Instance{ID: "instance", Lifecycle: "active"}, "restart")
	if failure == nil || failure.Code != "BUSY" {
		t.Fatalf("running operation must remain busy: %v", failure)
	}
}
