package engine

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStartIdentityRollback(t *testing.T) {
	for _, stage := range []string{"pidfd", "read", "mismatch", "poll"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			exe, _ := os.Executable()
			open := pidfdOpen
			read := readIdentity
			poll := pidfdExited
			injected := errors.New("injected " + stage)
			switch stage {
			case "pidfd":
				open = func(int) (int, error) { return -1, injected }
			case "read":
				read = func(int, string) (Identity, error) { return Identity{}, injected }
			case "mismatch":
				read = func(pid int, run string) (Identity, error) {
					id, e := readIdentity(pid, run)
					id.Executable += "-wrong"
					return id, e
				}
			case "poll":
				poll = func(int) (bool, error) { return false, injected }
			}
			id, err := startWithIdentity(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", "ignore"}}, open, read, poll)
			var rollback *StartError
			if id.PID <= 0 || !errors.As(err, &rollback) || !rollback.Exited {
				t.Fatalf("id=%+v err=%v", id, err)
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

func TestStartResolvedExecutable(t *testing.T) {
	dir := t.TempDir()
	exe, _ := os.Executable()
	link := filepath.Join(dir, "alias")
	if e := os.Symlink(exe, link); e != nil {
		t.Fatal(e)
	}
	id, e := Start(Spec{Executable: link, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", "normal"}})
	if e != nil {
		t.Fatal(e)
	}
	defer Stop(id, 0, 1000000000)
	resolved, _ := filepath.EvalSymlinks(exe)
	if id.Executable != resolved || id.Arguments[0] != link {
		t.Fatalf("not resolved: %+v", id)
	}
}
