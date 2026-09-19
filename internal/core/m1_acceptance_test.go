package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestM1BusyRestartDoesNotCreateOperationOrGeneration(t *testing.T) {
	m, path, port := setupManager(t, "never")
	id, createOp := createRoom(t, m, path, port)
	deadline := time.Now().Add(time.Second)
	loading := false
	for time.Now().Before(deadline) {
		state := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
		if state["process"] == "running" && state["room"] == "loading" {
			loading = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loading {
		t.Fatal("helper never reached running/loading")
	}
	m.mu.Lock()
	operations := len(m.store.State.Operations)
	generation := m.store.State.Instances[id].Generation
	m.mu.Unlock()
	rejected := call(t, m, "restart", map[string]any{"instanceId": id})
	if rejected.OK || rejected.Error == nil || rejected.Error.Code != "BUSY" {
		t.Fatalf("restart while loading: %+v", rejected)
	}
	m.mu.Lock()
	unchanged := len(m.store.State.Operations) == operations && m.store.State.Instances[id].Generation == generation && m.store.State.Instances[id].CurrentOperationID == createOp
	m.mu.Unlock()
	if !unchanged {
		t.Fatal("rejected restart changed operations or generation")
	}
	if stopped := stopRoom(t, m, id); stopped["status"] != "succeeded" {
		t.Fatal(stopped)
	}
	if result := waitOperation(t, m, createOp); result["status"] != "cancelled" {
		t.Fatal(result)
	}
}

func TestM1EarlyExitRetainsDiagnosticAndReservation(t *testing.T) {
	m, path, port := setupManager(t, "early")
	id, operationID := createRoom(t, m, path, port)
	result := waitOperation(t, m, operationID)
	if result["status"] != "failed" || result["instanceId"] != id || result["operationId"] != operationID {
		t.Fatal(result)
	}
	failure, ok := result["error"].(map[string]any)
	if !ok || failure["code"] != "START_FAILED" || (failure["stage"] != "spawn" && failure["stage"] != "observe") || failure["instanceId"] != id || failure["operationId"] != operationID || failure["message"] == "" {
		t.Fatalf("missing stable diagnostic: %+v", result)
	}
	state := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if state["lifecycle"] != "failed" || state["port"] != float64(port) || state["error"] == nil {
		t.Fatal(state)
	}
	list := success(t, call(t, m, "list", map[string]any{}))
	instances := list["instances"].([]any)
	if len(instances) != 1 || instances[0].(map[string]any)["instanceId"] != id {
		t.Fatal("failed instance lost active diagnostic record", list)
	}
	m.mu.Lock()
	in := m.store.State.Instances[id]
	if len(in.Runs) != 1 {
		m.mu.Unlock()
		t.Fatal("early exit run intent lost")
	}
	logPath, runDir := in.Runs[0].LogPath, in.Runs[0].Directory
	snapshotName := in.Snapshot.Name
	m.mu.Unlock()
	if snapshotName != "early" {
		t.Fatal("failed snapshot lost")
	}
	for _, p := range []string{logPath, filepath.Join(runDir, "generated.cfg"), path} {
		if st, e := os.Stat(p); e != nil || !st.Mode().IsRegular() {
			t.Fatalf("diagnostic/source missing %s: %v", p, e)
		}
	}
	// The child has exited, but a new intent must still be denied by the core's
	// reservation until explicit stop, independently of current socket state.
	conflict := call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "different-intent"})
	if conflict.OK || conflict.Error == nil || conflict.Error.Code != "PORT_IN_USE" {
		t.Fatalf("failed instance reservation lost: %+v", conflict)
	}
	if stopped := stopRoom(t, m, id); stopped["status"] != "succeeded" {
		t.Fatal(stopped)
	}
}
