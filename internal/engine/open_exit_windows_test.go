package engine

import (
	"errors"
	"syscall"
	"testing"
)

func TestOpenExitDuringIdentityRead(t *testing.T) {
	id, _ := startChild(t, "heartbeat")
	h, err := openWithIdentity(id, func(handle syscall.Handle, pid int) (Identity, error) {
		// Open has already observed this process alive. Force exit at exactly
		// the native identity-read boundary; no scheduler timing or sleep.
		if err := syscall.TerminateProcess(handle, 19); err != nil {
			t.Fatal(err)
		}
		if state, err := syscall.WaitForSingleObject(handle, 5000); err != nil || state != syscall.WAIT_OBJECT_0 {
			t.Fatalf("exit not confirmed: %d %v", state, err)
		}
		_, nativeErr := nativeIdentity(handle, pid)
		t.Logf("identity query after confirmed exit: %v", nativeErr)
		if nativeErr != nil {
			return Identity{}, nativeErr
		}
		// Native image queries may fail during teardown. Inject that failure
		// even on Windows versions which retain image metadata after exit.
		return Identity{}, syscall.ERROR_ACCESS_DENIED
	})
	if h != nil {
		h.Close()
		t.Fatal("returned a control handle for an exited process")
	}
	if !errors.Is(err, ErrGone) {
		t.Fatalf("confirmed exit must be ErrGone, got %v", err)
	}
}

func TestOpenLiveIdentityFailuresRemainUnverified(t *testing.T) {
	id, original := startChild(t, "heartbeat")
	for _, mode := range []string{"read-error", "creation-mismatch", "image-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			h, err := openWithIdentity(id, func(handle syscall.Handle, pid int) (Identity, error) {
				if mode == "read-error" {
					return Identity{}, syscall.ERROR_ACCESS_DENIED
				}
				actual := id
				if mode == "creation-mismatch" {
					actual.CreationTime++
				} else {
					actual.Executable += ".other"
				}
				return actual, nil
			})
			if h != nil {
				h.Close()
				t.Fatal("unverified control handle returned")
			}
			if !errors.Is(err, ErrIdentity) {
				t.Fatalf("must fail closed, got %v", err)
			}
			if alive, err := original.Alive(); err != nil || !alive {
				t.Fatalf("identity failure affected child: %v %v", alive, err)
			}
		})
	}
}
