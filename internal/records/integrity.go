package records

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
)

// Checksum detects accidental corruption, not malicious edits by the OS owner.
func stateChecksum(state State) (string, error) {
	state.Checksum = ""
	b, e := json.Marshal(state)
	if e != nil {
		return "", e
	}
	if len(b) > MaxStateBytes {
		return "", fmt.Errorf("state exceeds 64 MiB")
	}
	return digest(b), nil
}
func oneOf(value string, choices ...string) bool {
	for _, x := range choices {
		if value == x {
			return true
		}
	}
	return false
}
func cleanAbsolute(p string) bool {
	return filepath.IsAbs(p) && filepath.Clean(p) == p && !strings.ContainsAny(p, "\x00\r\n")
}

func (s *Store) validateState() error {
	state := s.State
	if state.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported data formatVersion")
	}
	if state.Instances == nil || state.Operations == nil || state.Keys == nil {
		return fmt.Errorf("incomplete state")
	}
	ports := map[int]string{}
	for id, in := range state.Instances {
		if in == nil || !validID(id, "i") || in.ID != id || in.Port < 1 || in.Port > 65535 || !cleanAbsolute(in.TemplatePath) {
			return fmt.Errorf("invalid instance record %q", id)
		}
		if !oneOf(in.Lifecycle, "active", "failed", "reclaimed") || !oneOf(in.Process, "stopped", "running", "unknown") || !oneOf(in.Room, "unknown", "loading", "ready", "failed") || !oneOf(in.Cleanup, "pending", "complete", "failed") {
			return fmt.Errorf("invalid instance state %s", id)
		}
		if in.CreatedAt.IsZero() || in.UpdatedAt.Before(in.CreatedAt) {
			return fmt.Errorf("invalid instance timestamps %s", id)
		}
		if in.Lifecycle != "reclaimed" {
			if prior := ports[in.Port]; prior != "" {
				return fmt.Errorf("duplicate reserved port for %s and %s", prior, id)
			}
			ports[in.Port] = id
		}
		if in.Lifecycle == "reclaimed" && (in.Process != "stopped" || in.Cleanup != "complete") {
			return fmt.Errorf("invalid reclaimed instance %s", id)
		}
		if in.Room == "ready" && in.Process != "running" {
			return fmt.Errorf("ready instance without running process %s", id)
		}
		current := state.Operations[in.CurrentOperationID]
		if current == nil || current.InstanceID != id {
			return fmt.Errorf("invalid current operation %s", id)
		}
		if in.Error != nil {
			referenced := state.Operations[in.Error.OperationID]
			if in.Error.InstanceID != id || referenced == nil || referenced.InstanceID != id || in.Error.Code == "" || in.Error.Stage == "" || in.Error.Message == "" {
				return fmt.Errorf("invalid instance error relationship")
			}
		}
		if in.Runs == nil || in.Generation != len(in.Runs) {
			return fmt.Errorf("invalid generation count %s", id)
		}
		// Validate every snapshot even before generation 1, without filesystem IO.
		sampleDir := filepath.Join(s.Dir, "instances", id, "runs", "000001")
		if _, e := config.ExpandSnapshot(in.Snapshot, config.Values{InstanceID: id, CfgName: fmt.Sprintf("d2core_%s_g1.cfg", id), LogPath: filepath.Join(sampleDir, "engine.log"), GamePort: in.Port}); e != nil {
			return fmt.Errorf("invalid snapshot %s: %w", id, e)
		}
		for n, r := range in.Runs {
			if r == nil || r.Generation != n+1 || r.CreatedAt.IsZero() {
				return fmt.Errorf("invalid run record %s", id)
			}
			dir := filepath.Join(s.Dir, "instances", id, "runs", fmt.Sprintf("%06d", r.Generation))
			name := fmt.Sprintf("d2core_%s_g%d.cfg", id, r.Generation)
			if r.Directory != dir || r.LogPath != filepath.Join(dir, "engine.log") || r.CFGPath != filepath.Join(in.Snapshot.CFG.Directory, name) {
				return fmt.Errorf("run path ownership mismatch %s", id)
			}
			expanded, e := config.ExpandSnapshot(in.Snapshot, config.Values{InstanceID: id, CfgName: name, LogPath: r.LogPath, GamePort: in.Port})
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(r.Arguments, expanded.Arguments) || r.CFGDigest != digest([]byte(expanded.CFG)) {
				return fmt.Errorf("run expansion mismatch %s generation %d", id, r.Generation)
			}
			if r.Prepared && !r.CFGCreated {
				return fmt.Errorf("prepared run without cfg ownership")
			}
			if r.SpawnAttempted && (!r.Prepared || r.StartupDeadline.IsZero()) {
				return fmt.Errorf("spawn intent lacks preparation/deadline")
			}
			if r.Identity != nil {
				actual := r.Identity
				if !r.SpawnAttempted || actual.PID <= 0 || !cleanAbsolute(actual.Executable) || actual.RunDirectory != dir {
					return fmt.Errorf("invalid process candidate")
				}
				args := r.Arguments
				if runtime.GOOS == "linux" {
					args = append([]string{in.Snapshot.Executable}, args...)
				}
				if !reflect.DeepEqual(args, actual.Arguments) {
					return fmt.Errorf("process argv mismatch")
				}
			}
			if r.Evidence != nil {
				ev := r.Evidence
				if ev.Generation != r.Generation || ev.Source != r.LogPath || ev.ObservedAt.IsZero() {
					return fmt.Errorf("invalid evidence ownership")
				}
				seen := map[string]bool{}
				for _, match := range ev.Matched {
					if seen[match.Rule] || match.ObservedAt.IsZero() || !oneOf(match.Rule, in.Snapshot.Readiness.SuccessAll...) {
						return fmt.Errorf("invalid evidence rule")
					}
					seen[match.Rule] = true
				}
			}
			for _, binding := range r.Bindings {
				if r.Identity == nil || binding.PID != r.Identity.PID || binding.Port < 1 || binding.Port > 65535 || !oneOf(binding.Protocol, "tcp4", "tcp6", "udp4", "udp6") {
					return fmt.Errorf("invalid binding ownership")
				}
				if _, e := time.Parse(time.RFC3339Nano, binding.ObservedAt); e != nil {
					return fmt.Errorf("invalid binding timestamp")
				}
			}
		}
	}
	creates := map[string]int{}
	for id, op := range state.Operations {
		in := (*Instance)(nil)
		if op != nil {
			in = state.Instances[op.InstanceID]
		}
		if op == nil || !validID(id, "o") || op.ID != id || in == nil || op.Generation < 0 || op.Generation > in.Generation {
			return fmt.Errorf("invalid operation record")
		}
		if !oneOf(op.Kind, "create", "restart", "stop") || !oneOf(op.Status, "running", "succeeded", "failed", "cancelled") || !oneOf(op.Phase, "accepted", "preparing", "spawning", "observing", "stopping", "cleanup", "done") {
			return fmt.Errorf("invalid operation state")
		}
		if op.Kind == "create" {
			creates[in.ID]++
		}
		if op.Status == "failed" && op.Error == nil {
			return fmt.Errorf("failed operation lacks diagnostic")
		}
		if op.CreatedAt.IsZero() {
			return fmt.Errorf("invalid operation timestamp")
		}
		if op.Status == "running" {
			if op.Phase == "done" || op.FinishedAt != nil {
				return fmt.Errorf("running operation has terminal phase")
			}
		} else if op.Phase != "done" || op.FinishedAt == nil || op.FinishedAt.Before(op.CreatedAt) {
			return fmt.Errorf("terminal operation lacks completion")
		}
		if op.Error != nil && (op.Error.Code == "" || op.Error.Stage == "" || op.Error.Message == "" || op.Error.InstanceID != in.ID || op.Error.OperationID != id) {
			return fmt.Errorf("invalid operation error identity")
		}
	}
	keysByInstance := map[string]int{}
	for name, key := range state.Keys {
		if e := ValidateKey(name); e != nil {
			return e
		}
		in := state.Instances[key.InstanceID]
		op := state.Operations[key.OperationID]
		if in == nil || op == nil || op.InstanceID != in.ID || op.Kind != "create" {
			return fmt.Errorf("orphaned or mismatched idempotency record")
		}
		fp, e := Fingerprint(in.TemplatePath, in.Port)
		autoFP, _ := Fingerprint(in.TemplatePath, 0)
		if e != nil || (key.Fingerprint != fp && key.Fingerprint != autoFP) {
			return fmt.Errorf("idempotency fingerprint mismatch")
		}
		keysByInstance[in.ID]++
	}
	for id := range state.Instances {
		if keysByInstance[id] != 1 || creates[id] != 1 {
			return fmt.Errorf("instance must have exactly one creation intent")
		}
	}
	return nil
}
