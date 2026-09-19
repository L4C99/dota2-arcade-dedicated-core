package core

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

func TestRecoveryKeepsTerminalFailureDiagnostic(t *testing.T) {
	for _, code := range []string{"START_TIMEOUT", "CLEANUP_FAILED"} {
		t.Run(code, func(t *testing.T) {
			mode := "never"
			if code == "CLEANUP_FAILED" {
				mode = "ready"
			}
			m, path, port := setupManager(t, mode)
			id, operationID := createRoom(t, m, path, port)
			first := waitOperation(t, m, operationID)
			var cfgPath string
			var original []byte
			if code == "CLEANUP_FAILED" {
				if first["status"] != "succeeded" {
					t.Fatal(first)
				}
				m.mu.Lock()
				cfgPath = m.store.State.Instances[id].Runs[0].CFGPath
				m.mu.Unlock()
				var e error
				original, e = os.ReadFile(cfgPath)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(cfgPath, []byte("foreign changed cfg"), 0600); e != nil {
					t.Fatal(e)
				}
				first = stopRoom(t, m, id)
				operationID = first["operationId"].(string)
			}
			if first["status"] != "failed" || first["error"].(map[string]any)["code"] != code {
				t.Fatal(first)
			}
			beforeState := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
			beforeError, _ := json.Marshal(beforeState["error"])
			next := reopen(t, m)
			after := waitOperation(t, next, operationID)
			if !reflect.DeepEqual(first, after) {
				t.Fatalf("historical failure changed: before=%+v after=%+v", first, after)
			}
			afterState := success(t, call(t, next, "status", map[string]any{"instanceId": id}))
			afterError, _ := json.Marshal(afterState["error"])
			if string(beforeError) != string(afterError) {
				t.Fatalf("original diagnostic replaced: before=%s after=%s", beforeError, afterError)
			}
			if code == "CLEANUP_FAILED" {
				if e := os.WriteFile(cfgPath, original, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if stopped := stopRoom(t, next, id); stopped["status"] != "succeeded" {
				t.Fatal(stopped)
			}
		})
	}
}

func TestRecoveryPendingRestartDoesNotAdoptOldReadyAsSuccess(t *testing.T) {
	for _, phase := range []string{"accepted", "stopping"} {
		t.Run(phase, func(t *testing.T) {
			m, path, port := setupManager(t, "ready")
			id, createOperation := createRoom(t, m, path, port)
			if got := waitOperation(t, m, createOperation); got["status"] != "succeeded" {
				t.Fatal(got)
			}
			m.Close()
			in := m.store.State.Instances[id]
			originalPID := in.Runs[0].Identity.PID
			opID, e := records.ID("o")
			if e != nil {
				t.Fatal(e)
			}
			m.store.State.Operations[opID] = &records.Operation{ID: opID, Kind: "restart", InstanceID: id, Generation: 1, Status: "running", Phase: phase, CreatedAt: time.Now().UTC()}
			in.CurrentOperationID = opID
			if e := m.store.Save(); e != nil {
				t.Fatal(e)
			}
			next := reopen(t, m)
			got := waitOperation(t, next, opID)
			if got["status"] != "failed" || got["error"].(map[string]any)["code"] != "INTERRUPTED" {
				t.Fatal(got)
			}
			time.Sleep(100 * time.Millisecond)
			state := success(t, call(t, next, "status", map[string]any{"instanceId": id}))
			if state["generation"] != float64(1) || state["process"] != "running" {
				t.Fatal("recovery spawned/stopped a generation", state)
			}
			next.mu.Lock()
			unchanged := len(next.store.State.Instances[id].Runs) == 1 && next.store.State.Instances[id].Runs[0].Identity.PID == originalPID
			next.mu.Unlock()
			if !unchanged {
				t.Fatal("recovery replaced old process")
			}
			if result := waitOperation(t, next, createOperation); result["status"] != "succeeded" {
				t.Fatal("historical create changed", result)
			}
			if stopped := stopRoom(t, next, id); stopped["status"] != "succeeded" {
				t.Fatal(stopped)
			}
		})
	}
}

func TestRecoveryLoadingRetainsOriginalStartupDeadline(t *testing.T) {
	m, path, port := setupManager(t, "never")
	id, op := createRoom(t, m, path, port)
	limit := time.Now().Add(time.Second)
	loading := false
	for time.Now().Before(limit) {
		state := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
		if state["process"] == "running" && state["room"] == "loading" {
			loading = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !loading {
		t.Fatal("helper did not reach loading")
	}
	m.Close()
	originalDeadline := m.store.State.Instances[id].Runs[0].StartupDeadline
	if originalDeadline.IsZero() {
		t.Fatal("missing durable startup deadline")
	}
	// Simulate management downtime extending beyond the original deadline. The
	// process remains alive but never emits readiness; reopening must not grant
	// a new full startup interval.
	if remaining := time.Until(originalDeadline.Add(30 * time.Millisecond)); remaining > 0 {
		time.Sleep(remaining)
	}
	next := reopen(t, m)
	next.mu.Lock()
	deadlineAfter := next.store.State.Instances[id].Runs[0].StartupDeadline
	next.mu.Unlock()
	if !deadlineAfter.Equal(originalDeadline) {
		t.Fatalf("startup deadline reset: %s -> %s", originalDeadline, deadlineAfter)
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	var result map[string]any
	for time.Now().Before(deadline) {
		result = success(t, call(t, next, "operation", map[string]any{"operationId": op}))
		if result["status"] != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if result["status"] != "failed" || result["error"].(map[string]any)["code"] != "START_TIMEOUT" {
		t.Fatalf("expired startup budget was not enforced: %+v", result)
	}
	if stopped := stopRoom(t, next, id); stopped["status"] != "succeeded" {
		t.Fatal(stopped)
	}
}
