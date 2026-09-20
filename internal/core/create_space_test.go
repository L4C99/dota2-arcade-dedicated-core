package core

import (
	"os"
	"testing"
)

func TestCreateChecksCFGVolumeBeforeAcceptance(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	m.mu.Lock()
	base := m.freeBytes
	var checked []string
	m.freeBytes = func(p string) (uint64, error) {
		checked = append(checked, p)
		if p == m.store.Dir {
			return 2 << 30, nil
		}
		return 0, nil
	}
	m.mu.Unlock()
	r := call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "cfg-low"})
	if r.OK || r.Error.Code != "INSUFFICIENT_STORAGE" || r.Error.Stage != "storage" {
		t.Fatal(r)
	}
	m.mu.Lock()
	if len(m.store.State.Instances) != 0 || len(m.store.State.Keys) != 0 || len(m.store.State.Operations) != 0 {
		t.Fatal("new intent persisted")
	}
	if len(checked) != 2 || checked[0] == checked[1] {
		t.Fatal(checked)
	}
	m.freeBytes = base
	m.mu.Unlock()
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if e := os.WriteFile(path, []byte("invalid after acceptance"), 0600); e != nil {
		t.Fatal(e)
	}
	m.mu.Lock()
	m.freeBytes = func(string) (uint64, error) { return 0, nil }
	m.mu.Unlock()
	retry := success(t, call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "test-request"}))
	if retry["instanceId"] != id || retry["operationId"] != op {
		t.Fatal(retry)
	}
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
}
