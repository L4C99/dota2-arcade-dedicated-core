package core

import "testing"

func TestRestartStateBeforeLowSpace(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	m.freeBytes = func(string) (uint64, error) { return 0, nil }
	m.workers[id] = &worker{kind: "restart"}
	m.mu.Unlock()
	if r := call(t, m, "restart", map[string]any{"instanceId": id}); r.OK || r.Error.Code != "BUSY" {
		t.Fatal(r)
	}
	m.mu.Lock()
	delete(m.workers, id)
	m.mu.Unlock()
	if r := call(t, m, "restart", map[string]any{"instanceId": id}); r.OK || r.Error.Code != "INSUFFICIENT_STORAGE" {
		t.Fatal(r)
	}
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if r := call(t, m, "restart", map[string]any{"instanceId": id}); r.OK || r.Error.Code != "RECLAIMED" {
		t.Fatal(r)
	}
}
