package engine

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestExitPermissionRequiresHeldPidfdEvidence(t *testing.T) {
	for _, exits := range []bool{false, true} {
		t.Run(map[bool]string{false: "permission-persists", true: "exact-child-exits"}[exits], func(t *testing.T) {
			_, h := startLinuxHelper(t, "ignore")
			polls := 0
			poll := func(fd int) (bool, error) {
				polls++
				if exits && polls == 2 {
					// Terminate only the original held pidfd. The production
					// observation path itself never sends this signal.
					_, _, e := syscall.Syscall6(424, uintptr(fd), uintptr(syscall.SIGKILL), 0, 0, 0, 0)
					if e != 0 {
						return false, e
					}
				}
				return pidfdExited(fd)
			}
			before := time.Now()
			alive, err := h.aliveWith(func(int, string) (Identity, error) {
				return Identity{}, &os.PathError{Op: "stat", Path: "/proc/test/fd/0", Err: syscall.EACCES}
			}, poll)
			if exits {
				if alive || err != nil {
					t.Fatalf("confirmed exit: alive=%v err=%v", alive, err)
				}
			} else {
				if alive || !errors.Is(err, ErrIdentity) {
					t.Fatalf("permission was softened: %v %v", alive, err)
				}
				if time.Since(before) < 100*time.Millisecond {
					t.Fatal("exit observation window not exercised")
				}
				if live, err := h.Alive(); !live || err != nil {
					t.Fatalf("live child changed: %v %v", live, err)
				}
			}
		})
	}
}

func TestExitPermissionDoesNotSoftenMismatchOrPollError(t *testing.T) {
	id, h := startLinuxHelper(t, "ignore")
	id.StartTicks++
	alive, err := h.aliveWith(func(int, string) (Identity, error) { return id, nil }, pidfdExited)
	if alive || !errors.Is(err, ErrIdentity) {
		t.Fatalf("mismatch accepted: %v %v", alive, err)
	}
	want := errors.New("pidfd observation unavailable")
	alive, err = h.aliveWith(readIdentity, func(int) (bool, error) { return false, want })
	if alive || !errors.Is(err, want) {
		t.Fatalf("poll failure hidden: %v %v", alive, err)
	}
}
