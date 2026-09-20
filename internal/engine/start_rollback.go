package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// StartError retains whether the exact spawned child was confirmed exited.
// A rollback failure must remain unknown; callers must not release resources.
type StartError struct {
	Cause  error
	Exited bool
}

func (e *StartError) Error() string { return e.Cause.Error() }
func (e *StartError) Unwrap() error { return e.Cause }

// No waiter is started before this call. On Linux this also keeps an exited
// child unreaped, preventing PID reuse even on kernels without Go pidfd support.
// Windows Process retains the original creation handle. Never reopen by PID.
func rollbackStart(cmd *exec.Cmd, cause error) error {
	killErr := cmd.Process.Kill()
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		go func() { _ = cmd.Wait() }()
		return &StartError{Cause: errors.Join(cause, fmt.Errorf("spawn rollback kill: %w", killErr))}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		if cmd.ProcessState == nil {
			return &StartError{Cause: errors.Join(cause, fmt.Errorf("spawn rollback wait did not confirm exit: %v", waitErr))}
		}
		return &StartError{Cause: cause, Exited: true}
	case <-time.After(5 * time.Second):
		return &StartError{Cause: errors.Join(cause, errors.New("spawn rollback exit not confirmed"))}
	}
}
