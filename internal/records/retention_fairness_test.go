package records

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestRetentionFailedPrefixCannotStarve(t *testing.T) {
	s, path := setup(t)
	port := availablePort(t)
	now := time.Now().UTC()
	var ids []string
	for n := 0; n < 17; n++ {
		in, op, e := s.Create(path, port, fmt.Sprintf("fair-%d", n))
		if e != nil {
			t.Fatal(e)
		}
		in.Lifecycle = "reclaimed"
		in.Cleanup = "complete"
		op.Status = "succeeded"
		op.Phase = "done"
		finished := time.Now().UTC()
		op.FinishedAt = &finished
		ids = append(ids, in.ID)
	}
	sort.Strings(ids)
	for _, id := range ids[:16] {
		p := filepath.Join(s.Dir, "instances", id)
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
		mustWrite(t, filepath.Join(p, "foreign"), []byte("retain"))
	}
	if e := s.Save(); e != nil {
		t.Fatal(e)
	}
	when := now.Add(48 * time.Hour)
	if n, e := s.PruneExpired(when, 24*time.Hour, 16); n != 0 || e == nil {
		t.Fatalf("first tick %d %v", n, e)
	}
	if n, e := s.PruneExpired(when, 24*time.Hour, 16); n != 1 || e == nil {
		t.Fatalf("second tick %d %v", n, e)
	}
	if s.State.Instances[ids[16]] != nil {
		t.Fatal("seventeenth starved")
	}
	if len(s.State.Instances) != 16 {
		t.Fatal("failed records lost")
	}
	for _, id := range ids[:16] {
		if e := os.Remove(filepath.Join(s.Dir, "instances", id, "foreign")); e != nil {
			t.Fatal(e)
		}
	}
	if n, e := s.PruneExpired(when, 24*time.Hour, 16); n != 16 || e != nil {
		t.Fatalf("retry %d %v", n, e)
	}
	if len(s.State.Keys) != 0 || len(s.State.Operations) != 0 {
		t.Fatal("metadata not retired together")
	}
}
