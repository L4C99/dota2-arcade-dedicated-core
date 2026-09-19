package records

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func historyFixture(t *testing.T) (*Store, *Instance) {
	t.Helper()
	s, path := setup(t)
	in := create(t, s, path, availablePort(t), "retained")
	r, err := s.PrepareRun(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CleanupRun(in, r); err != nil {
		t.Fatal(err)
	}
	r.InputRemoved = true
	in.Lifecycle = "reclaimed"
	in.Cleanup = "complete"
	op := s.State.Operations[in.CurrentOperationID]
	op.Generation = 1
	op.Status = "succeeded"
	op.Phase = "done"
	now := time.Now().UTC()
	op.FinishedAt = &now
	in.UpdatedAt = now
	if err = s.Save(); err != nil {
		t.Fatal(err)
	}
	return s, in
}

func TestHistoryRetentionRemovesFilesAndIntentTogether(t *testing.T) {
	s, in := historyFixture(t)
	path := in.TemplatePath
	if n, e := s.PruneExpired(in.UpdatedAt.Add(23*time.Hour), 24*time.Hour, 16); e != nil || n != 0 {
		t.Fatalf("early prune %d %v", n, e)
	}
	if n, e := s.PruneExpired(in.UpdatedAt.Add(24*time.Hour), 24*time.Hour, 16); e != nil || n != 1 {
		t.Fatalf("prune %d %v", n, e)
	}
	if _, e := os.Stat(path); e != nil {
		t.Fatal("source template removed", e)
	}
	if _, e := os.Stat(filepath.Join(s.Dir, "instances", in.ID)); !os.IsNotExist(e) {
		t.Fatal("history directory remains", e)
	}
	loaded, e := Open(s.Dir)
	if e != nil {
		t.Fatal(e)
	}
	if len(loaded.State.Keys) != 0 || len(loaded.State.Operations) != 0 || len(loaded.State.Instances) != 0 {
		t.Fatal("partial metadata expiry")
	}
}

func TestHistoryRefusesForeignFilesAndRetries(t *testing.T) {
	s, in := historyFixture(t)
	foreign := filepath.Join(in.Runs[0].Directory, "operator-notes.txt")
	mustWrite(t, foreign, []byte("must keep"))
	when := in.UpdatedAt.Add(25 * time.Hour)
	_, e := s.PruneExpired(when, 24*time.Hour, 16)
	requireCode(t, e, "CLEANUP_FAILED")
	if _, e = os.Stat(in.Runs[0].LogPath); e != nil {
		t.Fatal("partial removal before ownership scan", e)
	}
	if len(s.State.Keys) != 1 {
		t.Fatal("retry key removed after cleanup failure")
	}
	if e = os.Remove(foreign); e != nil {
		t.Fatal(e)
	}
	if n, e := s.PruneExpired(when, 24*time.Hour, 16); e != nil || n != 1 {
		t.Fatalf("retry %d %v", n, e)
	}
}

func TestHistoryInterruptedMetadataSaveKeepsRetryOwnership(t *testing.T) {
	s, in := historyFixture(t)
	s.saveHook = func(stage string) error {
		if stage == "before-replace" {
			return errors.New("injected publication failure")
		}
		return nil
	}
	when := in.UpdatedAt.Add(25 * time.Hour)
	_, err := s.PruneExpired(when, 24*time.Hour, 16)
	requireCode(t, err, "IO_ERROR")
	if len(s.State.Keys) != 1 {
		t.Fatal("in-memory key lost")
	}
	next, e := Open(s.Dir)
	if e != nil {
		t.Fatal(e)
	}
	if len(next.State.Instances) != 1 {
		t.Fatal("old record lost")
	}
	if n, e := next.PruneExpired(when, 24*time.Hour, 16); e != nil || n != 1 {
		t.Fatalf("resume %d %v", n, e)
	}
}

func TestHistoryActiveAndChangedConfigAreNeverDeleted(t *testing.T) {
	s, in := historyFixture(t)
	in.Lifecycle = "failed"
	in.Cleanup = "pending"
	when := in.UpdatedAt.Add(25 * time.Hour)
	if n, e := s.PruneExpired(when, 24*time.Hour, 16); n != 0 || e != nil {
		t.Fatal(n, e)
	}
	in.Lifecycle = "reclaimed"
	in.Cleanup = "complete"
	mustWrite(t, filepath.Join(in.Runs[0].Directory, "generated.cfg"), []byte("changed"))
	_, err := s.PruneExpired(when, 24*time.Hour, 16)
	requireCode(t, err, "CLEANUP_FAILED")
}

func TestNativeDiskFreeBytes(t *testing.T) {
	n, e := FreeBytes(t.TempDir())
	if e != nil || n == 0 {
		t.Fatalf("native disk space: %d %v", n, e)
	}
}

func TestHistoryRejectsLinkedGeneration(t *testing.T) {
	s, in := historyFixture(t)
	dir := in.Runs[0].Directory
	backup := filepath.Join(t.TempDir(), "original")
	if err := os.Rename(dir, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, dir); err != nil {
		t.Skipf("OS did not permit creating test symlink: %v", err)
	}
	_, err := s.PruneExpired(in.UpdatedAt.Add(25*time.Hour), 24*time.Hour, 16)
	requireCode(t, err, "CLEANUP_FAILED")
	if _, err = os.Stat(filepath.Join(backup, "engine.log")); err != nil {
		t.Fatal("linked target touched", err)
	}
}
