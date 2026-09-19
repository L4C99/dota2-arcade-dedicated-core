package core

import "testing"

func TestUnverifiedStopKeepsOwnership(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	run := m.store.State.Instances[id].Runs[0]
	original := *run.Identity
	run.Identity.CreationTime++
	run.Identity.StartTicks++
	m.mu.Unlock()
	r := stopRoom(t, m, id)
	if r["status"] != "failed" {
		t.Fatal(r)
	}
	s := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if s["process"] != "unknown" || s["lifecycle"] == "reclaimed" {
		t.Fatal(s)
	}
	list := success(t, call(t, m, "list", map[string]any{}))
	if len(list["instances"].([]any)) != 1 {
		t.Fatal("unverified instance lost reservation")
	}
	m.mu.Lock()
	*run.Identity = original
	m.mu.Unlock()
	if r = stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
}

func TestInvalidTemplateDoesNotCreateRecord(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	r := call(t, m, "create", map[string]any{"template": path + ".missing", "port": port, "idempotencyKey": "invalid"})
	if r.OK || r.Error == nil {
		t.Fatal(r)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.store.State.Instances) != 0 || len(m.workers) != 0 {
		t.Fatal("invalid template created an instance")
	}
}
