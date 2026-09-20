package core

import (
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOrphanFIFOIdentityGapExpires(t *testing.T) {
	m, path, port := setupManager(t, "early")
	m.Close()
	in, op, e := m.store.Create(path, port, "fifo-gap")
	if e != nil {
		t.Fatal(e)
	}
	r, e := m.store.PrepareRun(in)
	if e != nil {
		t.Fatal(e)
	}
	r.SpawnAttempted = true
	r.StartupDeadline = time.Now().Add(time.Minute)
	op.Generation = r.Generation
	op.Phase = "spawning"
	if e = m.store.Save(); e != nil {
		t.Fatal(e)
	}
	id, e := engine.Start(engine.Spec{Executable: in.Snapshot.Executable, WorkingDirectory: in.Snapshot.WorkingDirectory, Arguments: r.Arguments, RunDirectory: r.Directory, InputPrepared: func(id engine.Identity) error { r.InputOwnership = &id; return m.store.Save() }})
	// A fast helper may exit during identity acquisition (rollback already handled).
	if e != nil && id.PID == 0 {
		t.Fatal(e)
	}
	time.Sleep(150 * time.Millisecond)
	// No process identity is written, exactly as when manager dies after spawn.
	next, e := Open(m.store.Dir)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	if result := stopRoom(t, next, in.ID); result["status"] != "succeeded" {
		t.Fatal(result)
	}
	if _, e = os.Lstat(filepath.Join(r.Directory, "stdin.fifo")); !os.IsNotExist(e) {
		t.Fatalf("FIFO remains: %v", e)
	}
	next.mu.Lock()
	n, e := next.store.PruneExpired(time.Now().Add(48*time.Hour), 24*time.Hour, 16)
	next.mu.Unlock()
	if e != nil || n != 1 {
		t.Fatalf("prune %d %v", n, e)
	}
}
