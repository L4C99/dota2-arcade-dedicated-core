package engine

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestStartIdentityRollback(t *testing.T) {
	t.Setenv("D2CORE_ENGINE_CHILD", "1")
	for _, stage := range []string{"open", "read"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			exe, _ := os.Executable()
			open := syscall.OpenProcess
			read := nativeIdentity
			var original syscall.Handle
			injected := errors.New("injected " + stage)
			open = func(access uint32, inherit bool, pid uint32) (syscall.Handle, error) {
				var e error
				original, e = syscall.OpenProcess(0x100000, false, pid)
				if e != nil {
					t.Fatal(e)
				}
				if stage == "open" {
					return 0, injected
				}
				return syscall.OpenProcess(access, inherit, pid)
			}
			if stage == "read" {
				read = func(syscall.Handle, int) (Identity, error) { return Identity{}, injected }
			}
			id, e := startWithIdentity(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestEngineChild$", "--", "heartbeat"}}, open, read)
			defer syscall.CloseHandle(original)
			var rollback *StartError
			if id.PID <= 0 || !errors.As(e, &rollback) || !rollback.Exited {
				t.Fatalf("id=%+v err=%v", id, e)
			}
			state, e := syscall.WaitForSingleObject(original, 0)
			if e != nil || state != syscall.WAIT_OBJECT_0 {
				t.Fatalf("child remains %v %v", state, e)
			}
		})
	}
}
