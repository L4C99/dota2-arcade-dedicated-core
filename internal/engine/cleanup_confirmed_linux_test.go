//go:build linux

package engine

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Confirmed-exit cleanup must never reopen or signal an old PID. Using this
// test process as that stale PID exercises the rule without another process.
func TestCleanupConfirmedInputIgnoresReusedPID(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "stdin.fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	var st syscall.Stat_t
	if err := syscall.Lstat(fifo, &st); err != nil {
		t.Fatal(err)
	}
	id := Identity{PID: os.Getpid(), RunDirectory: dir, InputDevice: uint64(st.Dev), InputInode: st.Ino}
	if err := CleanupConfirmedInput(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(fifo); !os.IsNotExist(err) {
		t.Fatalf("owned FIFO remained: %v", err)
	}
	if err := CleanupConfirmedInput(id); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	// A second operation proves the current process remains alive after cleanup.
	if err := os.WriteFile(filepath.Join(dir, "still-alive"), []byte("alive"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupConfirmedInputRejectsReplacement(t *testing.T) {
	for _, kind := range []string{"fifo", "regular", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			fifo := filepath.Join(dir, "stdin.fifo")
			if err := syscall.Mkfifo(fifo, 0600); err != nil {
				t.Fatal(err)
			}
			var st syscall.Stat_t
			if err := syscall.Lstat(fifo, &st); err != nil {
				t.Fatal(err)
			}
			id := Identity{PID: os.Getpid(), RunDirectory: dir, InputDevice: uint64(st.Dev), InputInode: st.Ino}
			// Retaining the original inode prevents accidental inode reuse in this test.
			retained := filepath.Join(dir, "original.fifo")
			if err := os.Rename(fifo, retained); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "fifo":
				err = syscall.Mkfifo(fifo, 0600)
			case "regular":
				err = os.WriteFile(fifo, []byte("unrelated content"), 0600)
			case "symlink":
				err = os.Symlink(retained, fifo)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = CleanupConfirmedInput(id); err == nil {
				t.Fatal("replacement was accepted")
			}
			if _, err = os.Lstat(fifo); err != nil {
				t.Fatalf("replacement altered: %v", err)
			}
			var original syscall.Stat_t
			if err = syscall.Lstat(retained, &original); err != nil {
				t.Fatal(err)
			}
			if original.Ino != st.Ino || original.Dev != st.Dev {
				t.Fatal("original FIFO changed")
			}
			if kind == "regular" {
				b, err := os.ReadFile(fifo)
				if err != nil || string(b) != "unrelated content" {
					t.Fatalf("replacement content changed: %q %v", b, err)
				}
			}
		})
	}
}
