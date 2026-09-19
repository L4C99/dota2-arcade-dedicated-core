package core

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

func TestCoreCrashWindowHelper(t *testing.T) {
	if len(os.Args) < 6 || os.Args[len(os.Args)-5] != "--recovery-crash-child" {
		return
	}
	args := os.Args[len(os.Args)-4:]
	mode, dir, path := args[0], args[1], args[2]
	port, _ := strconv.Atoi(args[3])
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	baseSave := m.persist
	m.persist = func() error {
		for _, in := range m.store.State.Instances {
			if len(in.Runs) == 0 {
				continue
			}
			r := in.Runs[len(in.Runs)-1]
			op := m.store.State.Operations[in.CurrentOperationID]
			if mode == "after-spawn" && op.Phase == "observing" && r.Identity != nil {
				os.Exit(73)
			}
			if mode == "before-spawn" && op.Phase == "spawning" && r.Identity == nil {
				if err := baseSave(); err != nil {
					return err
				}
				os.Exit(73)
			}
			if mode == "after-stop" && op.Kind == "stop" && op.Phase == "stopping" && r.StopResult != nil && r.StopResult.Confirmed {
				if err := baseSave(); err != nil {
					return err
				}
				os.Exit(73)
			}
		}
		return baseSave()
	}
	id, op := createRoom(t, m, path, port)
	if waitOperation(t, m, op)["status"] != "succeeded" {
		t.Fatal("create failed")
	}
	if mode == "after-stop" {
		stopRoom(t, m, id)
	}
	t.Fatal("crash injection did not execute")
}

func TestRecoveryActualManagerCrashBoundaries(t *testing.T) {
	for _, mode := range []string{"before-spawn", "after-spawn", "after-stop"} {
		t.Run(mode, func(t *testing.T) {
			m, path, port := setupManager(t, "ready")
			m.Close()
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "-test.run=^TestCoreCrashWindowHelper$", "--", "--recovery-crash-child", mode, m.store.Dir, path, strconv.Itoa(port))
			out, err := cmd.CombinedOutput()
			if err == nil || cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 73 {
				t.Fatalf("expected crash: %v %s", err, out)
			}
			next := reopen(t, m)
			var id string
			for candidate := range next.store.State.Instances {
				id = candidate
			}
			if id == "" {
				t.Fatal("lost intent")
			}
			op := next.store.State.Instances[id].CurrentOperationID
			result := waitOperation(t, next, op)
			if mode == "before-spawn" {
				if result["status"] != "failed" {
					t.Fatal(result)
				}
			} else if result["status"] != "succeeded" {
				t.Fatal(result)
			}
			state := success(t, call(t, next, "status", map[string]any{"instanceId": id}))
			if state["generation"] != float64(1) {
				t.Fatal("duplicate generation", state)
			}
			if mode == "after-stop" {
				if state["lifecycle"] != "reclaimed" {
					t.Fatal(state)
				}
			} else if stopRoom(t, next, id)["status"] != "succeeded" {
				t.Fatal("cleanup failed")
			}
		})
	}
}

func reopen(t *testing.T, m *Manager) *Manager {
	t.Helper()
	m.Close()
	next, err := Open(m.store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(next.Close)
	return next
}

func TestRecoveryReadyProcessAndHistoricalRetry(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if waitOperation(t, m, op)["status"] != "succeeded" {
		t.Fatal("create failed")
	}
	m.mu.Lock()
	identity := *m.store.State.Instances[id].Runs[0].Identity
	m.mu.Unlock()
	next := reopen(t, m)
	if got := success(t, call(t, next, "status", map[string]any{"instanceId": id})); got["process"] != "running" || got["generation"] != float64(1) {
		t.Fatal(got)
	}
	if got := waitOperation(t, next, op); got["status"] != "succeeded" {
		t.Fatal(got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got := success(t, call(t, next, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "test-request"}))
	if got["instanceId"] != id || got["operationId"] != op {
		t.Fatal(got)
	}
	if next.store.State.Instances[id].Runs[0].Identity.PID != identity.PID {
		t.Fatal("recovery replaced process")
	}
	if stopRoom(t, next, id)["status"] != "succeeded" {
		t.Fatal("stop failed")
	}
}

func TestRecoveryPreSpawnIntentFailsWithoutRespawn(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	m.Close()
	in, op, err := m.store.Create(path, port, "pre-spawn")
	if err != nil {
		t.Fatal(err)
	}
	next := reopen(t, m)
	r := waitOperation(t, next, op.ID)
	if r["status"] != "failed" || r["error"].(map[string]any)["code"] != "INTERRUPTED" {
		t.Fatal(r)
	}
	got := success(t, call(t, next, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "pre-spawn"}))
	if got["instanceId"] != in.ID || next.store.State.Instances[in.ID].Generation != 0 {
		t.Fatal(got)
	}
	if stopRoom(t, next, in.ID)["status"] != "succeeded" {
		t.Fatal("stop failed")
	}
}

func TestRecoverySpawnIdentityGapAdoptsOnlyExistingGeneration(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	m.Close()
	in, op, err := m.store.Create(path, port, "spawn-gap")
	if err != nil {
		t.Fatal(err)
	}
	op.Phase = "preparing"
	r, err := m.store.PrepareRun(in)
	if err != nil {
		t.Fatal(err)
	}
	op.Generation, op.Phase = r.Generation, "spawning"
	r.SpawnAttempted = true
	r.StartupDeadline = time.Now().UTC().Add(5 * time.Second)
	if err = m.store.Save(); err != nil {
		t.Fatal(err)
	}
	id, err := engine.Start(engine.Spec{Executable: in.Snapshot.Executable, WorkingDirectory: in.Snapshot.WorkingDirectory, Arguments: r.Arguments, RunDirectory: r.Directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = engine.Stop(id, 100*time.Millisecond, time.Second) })
	// The live identity is deliberately absent from the durable state.
	next := reopen(t, m)
	if got := waitOperation(t, next, op.ID); got["status"] != "succeeded" {
		t.Fatal(got)
	}
	recovered := next.store.State.Instances[in.ID]
	if recovered.Generation != 1 || recovered.Runs[0].Identity.PID != id.PID {
		t.Fatal("did not adopt original generation")
	}
	if stopRoom(t, next, in.ID)["status"] != "succeeded" {
		t.Fatal("stop failed")
	}
}

func TestRecoveryConfirmedExitCompletesPendingCleanup(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	// A durable intent with generated files is sufficient to exercise the
	// exit-before-reclaim window; no unrelated process is signalled.
	m.Close()
	in, _, err := m.store.Create(path, port, "cleanup-gap")
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.store.PrepareRun(in)
	if err != nil {
		t.Fatal(err)
	}
	stopID, _ := records.ID("o")
	old := m.store.State.Operations[in.CurrentOperationID]
	now := time.Now().UTC()
	old.Status, old.Phase, old.FinishedAt = "cancelled", "done", &now
	op := &records.Operation{ID: stopID, InstanceID: in.ID, Kind: "stop", Status: "running", Phase: "cleanup", Generation: 1, CreatedAt: now}
	m.store.State.Operations[stopID] = op
	in.CurrentOperationID = stopID
	r.StopResult = &engine.StopResult{Confirmed: true}
	if err = m.store.Save(); err != nil {
		t.Fatal(err)
	}
	next := reopen(t, m)
	if got := waitOperation(t, next, stopID); got["status"] != "succeeded" {
		t.Fatal(got)
	}
	if _, err = os.Stat(r.CFGPath); !os.IsNotExist(err) {
		t.Fatal("generated cfg not reclaimed", err)
	}
}

func TestRecoveryMismatchedIdentityNeverControlsProcess(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if waitOperation(t, m, op)["status"] != "succeeded" {
		t.Fatal("create failed")
	}
	m.Close()
	r := m.store.State.Instances[id].Runs[0]
	original := *r.Identity
	t.Cleanup(func() { _, _ = engine.Stop(original, 100*time.Millisecond, time.Second) })
	if runtime.GOOS == "windows" {
		r.Identity.CreationTime++
	} else {
		r.Identity.StartTicks++
	}
	if err := m.store.Save(); err != nil {
		t.Fatal(err)
	}
	next := reopen(t, m)
	state := success(t, call(t, next, "status", map[string]any{"instanceId": id}))
	if state["process"] != "unknown" || state["lifecycle"] != "failed" {
		t.Fatal(state)
	}
	if result := stopRoom(t, next, id); result["status"] != "failed" {
		t.Fatal(result)
	}
	h, err := engine.Open(original)
	if err != nil {
		t.Fatal("unrelated identity harmed", err)
	}
	h.Close()
}

func TestConcurrentCreateRetriesProduceOneIntent(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	var wg sync.WaitGroup
	results := make(chan response, 12)
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "concurrent"})
		}()
	}
	wg.Wait()
	close(results)
	var id, op string
	for r := range results {
		got := success(t, r)
		if id == "" {
			id = got["instanceId"].(string)
			op = got["operationId"].(string)
		}
		if got["instanceId"] != id || got["operationId"] != op {
			t.Fatal("duplicate intent", got)
		}
	}
	if waitOperation(t, m, op)["status"] != "succeeded" {
		t.Fatal("create failed")
	}
	m.mu.Lock()
	count := len(m.store.State.Instances)
	generation := m.store.State.Instances[id].Generation
	m.mu.Unlock()
	if count != 1 || generation != 1 {
		t.Fatal(count, generation)
	}
	if stopRoom(t, m, id)["status"] != "succeeded" {
		t.Fatal("stop failed")
	}
}
