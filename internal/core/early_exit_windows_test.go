package core

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
)

// The acceptance test asserts a confirmed exit, not Windows' transient state
// where an image query is denied while the process handle is still unsignaled.
// Synchronize the fixture on the OS event; never retry a failed core operation.
func confirmEarlyExitBeforeObservation(m *Manager) func() error {
	var fixtureErr error
	m.mu.Lock()
	spawn := m.spawn
	m.spawn = func(spec engine.Spec) (engine.Identity, error) {
		id, err := spawn(spec)
		if err == nil {
			fixtureErr = waitEarlyChildExit(id)
		}
		return id, err
	}
	m.mu.Unlock()
	return func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		return fixtureErr
	}
}

func waitEarlyChildExit(id engine.Identity) error {
	h, err := syscall.OpenProcess(0x1000|0x100000, false, uint32(id.PID))
	if errors.Is(err, syscall.Errno(87)) {
		return nil // Process object already gone; no PID control is performed.
	}
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	var creation, exit, kernelTime, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creation, &exit, &kernelTime, &user); err != nil {
		return err
	}
	if uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime) != id.CreationTime {
		return fmt.Errorf("fixture process creation identity changed")
	}
	state, err := syscall.WaitForSingleObject(h, 5000)
	if err != nil || state != syscall.WAIT_OBJECT_0 {
		return fmt.Errorf("fixture exit wait: state=%d error=%v", state, err)
	}
	return nil
}
