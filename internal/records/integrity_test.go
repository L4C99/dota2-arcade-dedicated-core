package records

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
)

func writeChecksummedState(t *testing.T, s *Store) {
	t.Helper()
	sum, e := stateChecksum(s.State)
	if e != nil {
		t.Fatal(e)
	}
	s.State.Checksum = sum
	b, e := json.MarshalIndent(s.State, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	mustWrite(t, filepath.Join(s.Dir, "state.json"), b)
}
func TestChecksumAndVersionRefuseWithoutOverwrite(t *testing.T) {
	for _, kind := range []string{"checksum", "old-version"} {
		t.Run(kind, func(t *testing.T) {
			s, path := setup(t)
			in := create(t, s, path, availablePort(t), "a")
			p := filepath.Join(s.Dir, "state.json")
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			if kind == "checksum" {
				b = bytes.Replace(b, []byte(`"name": "original"`), []byte(`"name": "corrupted"`), 1)
			} else {
				b = bytes.Replace(b, []byte(`"formatVersion": 2`), []byte(`"formatVersion": 1`), 1)
			}
			mustWrite(t, p, b)
			if _, e := Open(s.Dir); e == nil {
				t.Fatal("accepted", kind, in.ID)
			}
			after, _ := os.ReadFile(p)
			if !bytes.Equal(after, b) {
				t.Fatal("rewrote rejected state")
			}
		})
	}
}
func TestSemanticCorruptionWithRecomputedChecksum(t *testing.T) {
	mutations := map[string]func(*Store, *Instance){
		"enum":       func(s *Store, in *Instance) { in.Process = "maybe" },
		"current-op": func(s *Store, in *Instance) { in.CurrentOperationID = "o_00000000000000000000000000000000" },
		"fingerprint": func(s *Store, in *Instance) {
			k := s.State.Keys["a"]
			k.Fingerprint = strings.Repeat("0", 64)
			s.State.Keys["a"] = k
		},
		"phase":                  func(s *Store, in *Instance) { s.State.Operations[in.CurrentOperationID].Phase = "bogus" },
		"run-path":               func(s *Store, in *Instance) { in.Runs[0].LogPath = filepath.Join(s.Dir, "other.log") },
		"args":                   func(s *Store, in *Instance) { in.Runs[0].Arguments[1] = "1" },
		"cfg-digest":             func(s *Store, in *Instance) { in.Runs[0].CFGDigest = strings.Repeat("0", 64) },
		"spawn-without-deadline": func(s *Store, in *Instance) { in.Runs[0].SpawnAttempted = true },
		"duplicate-port": func(s *Store, in *Instance) {
			copy := *in
			copy.ID, _ = ID("i")
			copy.Runs = []*Run{}
			copy.Generation = 0
			s.State.Instances[copy.ID] = &copy
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s, path := setup(t)
			in := create(t, s, path, availablePort(t), "a")
			if _, e := s.PrepareRun(in); e != nil {
				t.Fatal(e)
			}
			mutate(s, in)
			if e := s.Save(); e == nil {
				t.Fatal("Save accepted invalid state")
			}
			writeChecksummedState(t, s)
			before, _ := os.ReadFile(filepath.Join(s.Dir, "state.json"))
			if _, e := Open(s.Dir); e == nil {
				t.Fatal("Open accepted invalid state")
			}
			after, _ := os.ReadFile(filepath.Join(s.Dir, "state.json"))
			if !bytes.Equal(before, after) {
				t.Fatal("damaged state overwritten")
			}
		})
	}
}
func TestHistoryLoadDoesNotRequireSourceResources(t *testing.T) {
	s, path := setup(t)
	in := create(t, s, path, availablePort(t), "a")
	if _, e := s.PrepareRun(in); e != nil {
		t.Fatal(e)
	}
	// Simulate moved/uninstalled external resources in the stored snapshot and
	// regenerate its pure derived fields before committing a valid history.
	in.Snapshot.Executable = filepath.Join(filepath.Dir(path), "removed.exe")
	in.Snapshot.WorkingDirectory = filepath.Join(filepath.Dir(path), "removed-work")
	if e := s.Save(); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(s.Dir); e != nil {
		t.Fatalf("history coupled to runtime resources: %v", e)
	}
}
func TestSaveBoundAndAtomicFailure(t *testing.T) {
	s, path := setup(t)
	in := create(t, s, path, availablePort(t), "a")
	p := filepath.Join(s.Dir, "state.json")
	original, _ := os.ReadFile(p)
	for _, stage := range []string{"temp-written", "before-replace"} {
		s.saveHook = func(current string) error {
			if current == stage {
				return errors.New("injected write failure")
			}
			return nil
		}
		in.Snapshot.Name = "not published"
		if e := s.Save(); e == nil {
			t.Fatal("failure ignored")
		}
		after, _ := os.ReadFile(p)
		if !bytes.Equal(original, after) {
			t.Fatal("precommit failure replaced old record")
		}
		if _, e := Open(s.Dir); e != nil {
			t.Fatal(e)
		}
	}
	s.saveHook = nil
	in.Snapshot.Name = strings.Repeat("x", MaxStateBytes)
	if e := s.Save(); e == nil {
		t.Fatal("oversized state saved")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(original, after) {
		t.Fatal("oversize replaced old record")
	}
}
func TestAmbiguousCreateRetainsIdempotency(t *testing.T) {
	s, path := setup(t)
	s.saveHook = func(stage string) error {
		if stage == "after-replace" {
			return errors.New("injected post-publication failure")
		}
		return nil
	}
	port := availablePort(t)
	in, op, e := s.Create(path, port, "a")
	if e == nil || in == nil || op == nil {
		t.Fatalf("lost committed intent: %v", e)
	}
	var uncertain *DurabilityError
	if !errors.As(e, &uncertain) {
		t.Fatal("published error lost durability classification", e)
	}
	if s.State.Keys["a"].InstanceID != in.ID {
		t.Fatal("rolled back uncertain intent")
	}
	s.saveHook = nil
	again, againOp, e := s.Create(path, port, "a")
	if e != nil || again.ID != in.ID || againOp.ID != op.ID {
		t.Fatal("retry duplicated uncertain intent", e)
	}
	if _, e := Open(s.Dir); e != nil {
		t.Fatal(e)
	}
}
func TestCleanupUnrecordedCFGCreation(t *testing.T) {
	s, path := setup(t)
	in := create(t, s, path, availablePort(t), "a")
	r, e := s.PrepareRun(in)
	if e != nil {
		t.Fatal(e)
	}
	r.CFGCreated = false
	r.Prepared = false
	if e = s.Save(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(s.Dir)
	if e != nil {
		t.Fatal(e)
	}
	in = reopened.State.Instances[in.ID]
	r = in.Runs[0]
	if e = reopened.CleanupRun(in, r); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(r.CFGPath); !os.IsNotExist(e) {
		t.Fatal("crash-window cfg not removed")
	}
	if _, e = os.Stat(r.LogPath); e != nil {
		t.Fatal("history log removed")
	}
}

func TestRecordsCrashHelper(t *testing.T) {
	stage := os.Getenv("D2CORE_RECORDS_FAULT")
	if stage == "" {
		return
	}
	s, e := Open(os.Getenv("D2CORE_RECORDS_DIR"))
	if e != nil {
		os.Exit(71)
	}
	for _, in := range s.State.Instances {
		in.Snapshot.Name = "interrupted-update"
	}
	s.saveHook = func(current string) error {
		if current == stage {
			os.Exit(73)
		}
		return nil
	}
	if e = s.Save(); e != nil {
		os.Exit(72)
	}
	os.Exit(74)
}

func TestProcessInterruptionLeavesWholeState(t *testing.T) {
	for _, stage := range []string{"temp-written", "before-replace", "after-replace"} {
		t.Run(stage, func(t *testing.T) {
			s, path := setup(t)
			in := create(t, s, path, availablePort(t), "a")
			exe, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRecordsCrashHelper$")
			cmd.Env = append(os.Environ(), "D2CORE_RECORDS_FAULT="+stage, "D2CORE_RECORDS_DIR="+s.Dir)
			if out, e := cmd.CombinedOutput(); e == nil {
				t.Fatal("helper did not interrupt", string(out))
			} else {
				var exit *exec.ExitError
				if !errors.As(e, &exit) || exit.ExitCode() != 73 {
					t.Fatalf("unexpected helper exit: %v %s", e, out)
				}
			}
			reopened, e := Open(s.Dir)
			if e != nil {
				t.Fatal("interrupted save left invalid state", e)
			}
			want := "original"
			if stage == "after-replace" {
				want = "interrupted-update"
			}
			if reopened.State.Instances[in.ID].Snapshot.Name != want {
				t.Fatal("wrong publication state")
			}
			if reopened.State.Keys["a"].InstanceID != in.ID {
				t.Fatal("interruption lost idempotency identity")
			}
		})
	}
}

func TestNativeBindingEnumsAndOwnership(t *testing.T) {
	s, path := setup(t)
	in := create(t, s, path, availablePort(t), "a")
	r, e := s.PrepareRun(in)
	if e != nil {
		t.Fatal(e)
	}
	r.SpawnAttempted = true
	r.StartupDeadline = time.Now().UTC().Add(time.Minute)
	args := append([]string{}, r.Arguments...)
	if runtime.GOOS == "linux" {
		args = append([]string{in.Snapshot.Executable}, args...)
	}
	r.Identity = &engine.Identity{PID: 123, Executable: in.Snapshot.Executable, RunDirectory: r.Directory, Arguments: args}
	for _, protocol := range []string{"tcp4", "tcp6", "udp4", "udp6"} {
		r.Bindings = []engine.Binding{{Protocol: protocol, Address: "127.0.0.1", Port: 12345, PID: 123, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
		if e := s.Save(); e != nil {
			t.Fatal(protocol, e)
		}
	}
	r.Bindings[0].PID = 124
	if e := s.Save(); e == nil {
		t.Fatal("foreign process binding accepted")
	}
	r.Bindings[0].PID = 123
	r.Bindings[0].Protocol = "tcp"
	if e := s.Save(); e == nil {
		t.Fatal("unknown binding enum accepted")
	}
}

func TestInitializationMarkerFailClosed(t *testing.T) {
	for _, kind := range []string{"state-missing", "marker-missing", "marker-invalid", "marker-directory"} {
		t.Run(kind, func(t *testing.T) {
			s, path := setup(t)
			create(t, s, path, availablePort(t), "a")
			state := filepath.Join(s.Dir, "state.json")
			marker := filepath.Join(s.Dir, markerName)
			switch kind {
			case "state-missing":
				if e := os.Remove(state); e != nil {
					t.Fatal(e)
				}
			case "marker-missing":
				if e := os.Remove(marker); e != nil {
					t.Fatal(e)
				}
			case "marker-invalid":
				mustWrite(t, marker, []byte("not a store marker"))
			case "marker-directory":
				if e := os.Remove(marker); e != nil {
					t.Fatal(e)
				}
				if e := os.Mkdir(marker, 0700); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := Open(s.Dir); e == nil {
				t.Fatal("accepted damaged initialization", kind)
			}
			if e := s.Save(); e == nil {
				t.Fatal("Save silently repaired damaged initialization", kind)
			}
		})
	}
	dir := t.TempDir()
	if e := os.Mkdir(filepath.Join(dir, "instances"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(dir); e == nil {
		t.Fatal("forgot existing instances")
	}
}

func TestInitialSaveFailureRequiresDiagnosis(t *testing.T) {
	s, path := setup(t)
	s.saveHook = func(stage string) error {
		if stage == "before-replace" {
			return errors.New("injected first publication failure")
		}
		return nil
	}
	if _, _, e := s.Create(path, availablePort(t), "a"); e == nil {
		t.Fatal("first save unexpectedly succeeded")
	}
	if ok, e := verifyMarker(s.Dir); !ok || e != nil {
		t.Fatal("initialization marker not durable", e)
	}
	if _, e := os.Stat(filepath.Join(s.Dir, "state.json")); !os.IsNotExist(e) {
		t.Fatal("unexpected committed state")
	}
	if _, e := Open(s.Dir); e == nil {
		t.Fatal("silently initialized after interrupted first save")
	}
	s.saveHook = nil
	if e := s.Save(); e == nil {
		t.Fatal("same writer silently repaired interrupted first save")
	}
}
