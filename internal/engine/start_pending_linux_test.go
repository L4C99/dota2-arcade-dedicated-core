package engine

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestStartupEmptyCommandLineStillRequiresIdentity(t *testing.T) {
	for _, mode := range []string{"recovers", "timeout", "mismatch", "exits", "poll-error"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			exe, _ := os.Executable()
			calls, fd := 0, -1
			open := func(pid int) (int, error) {
				var err error
				fd, err = pidfdOpen(pid)
				return fd, err
			}
			read := func(pid int, run string) (Identity, error) {
				calls++
				if mode == "exits" && calls == 1 {
					_, _, err := syscall.Syscall6(424, uintptr(fd), uintptr(syscall.SIGKILL), 0, 0, 0, 0)
					if err != 0 {
						return Identity{}, err
					}
				}
				if calls <= 2 || mode == "timeout" || mode == "exits" {
					return Identity{}, ErrGone
				}
				id, err := readIdentity(pid, run)
				if mode == "mismatch" {
					id.InputInode++
				}
				return id, err
			}
			poll := pidfdExited
			injected := errors.New("pidfd polling failed")
			if mode == "poll-error" {
				poll = func(int) (bool, error) { return false, injected }
			}
			before := time.Now()
			id, err := startWithIdentity(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", "ignore"}}, open, read, poll)
			if mode == "recovers" {
				if err != nil {
					t.Fatal(err)
				}
				result, err := Stop(id, 0, time.Second)
				if !result.Confirmed || err != nil {
					t.Fatalf("cleanup: %+v %v", result, err)
				}
				if calls < 3 || id.StartTicks == 0 {
					t.Fatal("full identity not established")
				}
				return
			}
			var rollback *StartError
			if !errors.As(err, &rollback) || !rollback.Exited {
				t.Fatalf("not safely rolled back: %v", err)
			}
			if mode == "timeout" && (time.Since(before) < 100*time.Millisecond || !errors.Is(err, ErrIdentity)) {
				t.Fatalf("unbounded/incorrect timeout: %v", err)
			}
			if mode == "mismatch" && !errors.Is(err, ErrIdentity) {
				t.Fatalf("mismatch softened: %v", err)
			}
			if mode == "poll-error" && !errors.Is(err, injected) {
				t.Fatalf("poll error hidden: %v", err)
			}
			if e := syscall.Kill(id.PID, 0); !errors.Is(e, syscall.ESRCH) {
				t.Fatalf("child remains: %v", e)
			}
			if _, e := os.Lstat(filepath.Join(dir, "stdin.fifo")); !errors.Is(e, os.ErrNotExist) {
				t.Fatalf("FIFO remains: %v", e)
			}
		})
	}
}

func TestStartupIdentityNativeRepeat(t *testing.T) {
	exe, _ := os.Executable()
	for n := 0; n < 100; n++ {
		dir := t.TempDir()
		id, err := Start(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", "ignore"}})
		if err != nil {
			t.Fatalf("start %d: %v", n, err)
		}
		result, err := Stop(id, 0, time.Second)
		if !result.Confirmed || err != nil {
			t.Fatalf("stop %d: %+v %v", n, result, err)
		}
	}
}
