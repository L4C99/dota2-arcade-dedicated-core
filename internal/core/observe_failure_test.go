package core

import (
	"errors"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"os"
	"testing"
	"time"
)

func TestObservedFailureInvalidatesReadyOnce(t *testing.T) {
	for _, mode := range []string{"signal", "log", "bindings"} {
		t.Run(mode, func(t *testing.T) {
			m, path, port := setupManager(t, "ready")
			id, op := createRoom(t, m, path, port)
			if r := waitOperation(t, m, op); r["status"] != "succeeded" {
				t.Fatal(r)
			}
			m.mu.Lock()
			run := m.store.State.Instances[id].Runs[0]
			writes := 0
			base := m.persist
			m.persist = func() error { writes++; return base() }
			switch mode {
			case "signal":
				m.store.State.Instances[id].Snapshot.Readiness.FailureAny = []string{"FAILED"}
				if e := os.WriteFile(run.LogPath, []byte("loaded\nready\nFAILED\n"), 0600); e != nil {
					t.Fatal(e)
				}
			case "log":
				if e := os.Remove(run.LogPath); e != nil {
					t.Fatal(e)
				}
				if e := os.Mkdir(run.LogPath, 0700); e != nil {
					t.Fatal(e)
				}
			case "bindings":
				m.bindings = func(*engine.Handle) ([]engine.Binding, error) { return nil, errors.New("binding read failed") }
			}
			m.mu.Unlock()
			time.Sleep(2200 * time.Millisecond)
			m.mu.Lock()
			in := m.store.State.Instances[id]
			if in.Lifecycle != "failed" || in.Room != "failed" || in.Process != "running" {
				t.Fatalf("%s %s %s", in.Lifecycle, in.Room, in.Process)
			}
			if run.Evidence != nil && run.Evidence.Valid {
				t.Fatal("valid failure evidence")
			}
			if writes != 1 {
				t.Fatalf("stable failure persisted %d times", writes)
			}
			if m.store.State.Operations[op].Status != "succeeded" {
				t.Fatal("historical operation rewritten")
			}
			m.bindings = nil
			if mode == "log" {
				if e := os.Remove(run.LogPath); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(run.LogPath, nil, 0600); e != nil {
					t.Fatal(e)
				}
			}
			m.mu.Unlock()
			if r := stopRoom(t, m, id); r["status"] != "succeeded" {
				t.Fatal(r)
			}
		})
	}
}
