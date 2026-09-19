package core

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

// Recovery never spawns a new game process. It resumes observation of a
// verified existing generation or completes an already-requested stop. An
// interrupted create/restart without its new generation becomes a queryable
// failure; replaying the same creation key cannot create a replacement.
func (m *Manager) recover() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	type resume struct {
		in *records.Instance
		r  *records.Run
		op *records.Operation
	}
	var pending []resume
	for _, in := range m.store.State.Instances {
		if in.Lifecycle == "reclaimed" {
			continue
		}
		op := m.store.State.Operations[in.CurrentOperationID]
		var r *records.Run
		if len(in.Runs) > 0 {
			r = in.Runs[len(in.Runs)-1]
			if r.Evidence != nil {
				r.Evidence.Valid = false
			}
		}
		in.Room = "unknown"
		in.Process = "stopped"
		var identityErr error
		if r != nil && !(r.StopResult != nil && r.StopResult.Confirmed) {
			if !completeIdentity(r.Identity) && r.SpawnAttempted {
				spec := engine.Spec{Executable: in.Snapshot.Executable, WorkingDirectory: in.Snapshot.WorkingDirectory, Arguments: r.Arguments, RunDirectory: r.Directory}
				// A lower time bound accommodates Linux boot-time/tick precision.
				// Ownership still requires exact executable/argv and owned input.
				candidates, err := engine.Discover(spec, r.CreatedAt.Truncate(time.Second).Add(-time.Second))
				if err != nil {
					identityErr = err
				} else if len(candidates) > 1 {
					identityErr = fmt.Errorf("multiple processes match the same run intent")
				} else if len(candidates) == 1 {
					r.Identity = &candidates[0]
				} else {
					// A complete bounded scan found no process belonging to the
					// unique intent. Keep any partial identity for file ownership.
					r.StopResult = &engine.StopResult{Confirmed: true}
				}
			}
			if identityErr == nil && r.Identity != nil && !(r.StopResult != nil && r.StopResult.Confirmed) {
				h, err := engine.Open(*r.Identity)
				if err == nil {
					h.Close()
					in.Process = "running"
					in.Room = "loading"
				} else if !errors.Is(err, engine.ErrGone) {
					identityErr = err
				}
			}
		}
		if identityErr != nil {
			in.Process = "unknown"
			m.recoveryFailure(in, op, "IDENTITY_UNVERIFIED", identityErr.Error())
			continue
		}
		if op.Status == "running" && op.Kind == "stop" {
			// Discovery/identity must be durably saved before any resumed control.
			pending = append(pending, resume{in, r, op})
			continue
		}
		if op.Status == "running" {
			if r != nil && in.Process == "running" && op.Generation == r.Generation && (op.Phase == "spawning" || op.Phase == "observing") {
				in.Lifecycle = "active"
				op.Phase = "observing"
				pending = append(pending, resume{in, r, op})
			} else {
				m.recoveryFailure(in, op, "INTERRUPTED", "operation interrupted before a recoverable new generation; no process was spawned during recovery")
			}
		} else if in.Process == "stopped" && in.Lifecycle != "failed" {
			// A historical successful operation stays successful, while current
			// room state reflects the exit and retains its allocation for stop.
			m.recoveryFailure(in, op, "START_FAILED", "recorded process is no longer running")
		}
	}
	if err := m.save(); err != nil {
		return err
	}
	for _, p := range pending {
		w := m.newWorker(p.in, p.op.Kind, p.op.ID)
		if p.op.Kind == "stop" {
			go m.stopOrRestart(p.in.ID, w, nil)
		} else {
			go func(p resume, w *worker) {
				defer m.finishWorker(p.in.ID, w)
				m.awaitReadiness(p.in, p.r, w)
			}(p, w)
		}
	}
	return nil
}

func completeIdentity(id *engine.Identity) bool {
	if id == nil || id.PID <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return id.CreationTime != 0
	}
	return id.BootID != "" && id.StartTicks != 0 && id.InputInode != 0
}

func (m *Manager) recoveryFailure(in *records.Instance, op *records.Operation, code, message string) {
	now := time.Now().UTC()
	f := &records.Failure{Code: code, Stage: "recover", Message: message, InstanceID: in.ID, OperationID: op.ID}
	in.Lifecycle = "failed"
	in.Error = f
	in.UpdatedAt = now
	if in.Process == "stopped" {
		in.Room = "failed"
	}
	if op.Status == "running" {
		op.Status = "failed"
		op.Phase = "done"
		op.Error = f
		op.FinishedAt = &now
	}
}
