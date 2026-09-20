package records

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUnknownFIFORetentionRefused(t *testing.T) {
	s, in := historyFixture(t)
	path := filepath.Join(in.Runs[0].Directory, "stdin.fifo")
	if e := syscall.Mkfifo(path, 0600); e != nil {
		t.Fatal(e)
	}
	if n, e := s.PruneExpired(time.Now().Add(48*time.Hour), 24*time.Hour, 16); e == nil || n != 0 {
		t.Fatalf("foreign FIFO accepted %d %v", n, e)
	}
	if _, e := os.Lstat(path); e != nil {
		t.Fatal(e)
	}
}
