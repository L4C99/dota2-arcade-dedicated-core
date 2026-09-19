package records

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
)

func setup(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := config.Template{SchemaVersion: 1, Name: "original", Executable: exe, WorkingDirectory: dir, Arguments: []string{"-port", "{{game_port}}", "-con_logfile", "{{log_path}}", "+exec", "{{cfg_name}}"}, CFG: config.CFGConfig{Directory: dir, Lines: []string{`hostname "{{instance_id}}"`}}, Readiness: config.Readiness{SuccessAll: []string{"ready"}}}
	b, _ := json.Marshal(tmpl)
	path := filepath.Join(dir, "template.json")
	mustWrite(t, path, b)
	s, err := Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}
func mustWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func availablePort(t *testing.T, exclude ...int) int {
	t.Helper()
	var last error
	for attempt := 0; attempt < 20; attempt++ {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		l.Close()
		skip := false
		for _, v := range exclude {
			if p == v {
				skip = true
			}
		}
		if skip {
			continue
		}
		last = CheckPort(p)
		if last == nil {
			return p
		}
	}
	t.Fatalf("no dual-family TCP/UDP port found in 20 bounded attempts (IPv6 support is required by current implementation): %v", last)
	return 0
}
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *Failure
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}
func create(t *testing.T, s *Store, path string, p int, key string) *Instance {
	t.Helper()
	in, _, err := s.Create(path, p, key)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestReservationsSnapshotsAndRetry(t *testing.T) {
	s, path := setup(t)
	p := availablePort(t)
	a, op, err := s.Create(path, p, "key-a")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.Create(path, p, "new-intent")
	requireCode(t, err, "PORT_IN_USE")
	q := availablePort(t, p)
	b := create(t, s, path, q, "key-b")
	if a.ID == b.ID || len(s.State.Instances) != 2 {
		t.Fatal("new intentions not independent")
	}
	_, _, err = s.Create(path, q, "key-a")
	requireCode(t, err, "IDEMPOTENCY_CONFLICT")
	_, _, err = s.Create(filepath.Join(filepath.Dir(path), "different.json"), p, "key-a")
	requireCode(t, err, "IDEMPOTENCY_CONFLICT")
	mustWrite(t, path, []byte("invalid replacement"))
	again, againOp, err := s.Create(path, p, "key-a")
	if err != nil || again.ID != a.ID || againOp.ID != op.ID {
		t.Fatalf("retry reread template: %v", err)
	}
	if a.Snapshot.Name != "original" {
		t.Fatal("snapshot changed")
	}
	reopened, err := Open(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	again, againOp, err = reopened.Create(path, p, "key-a")
	if err != nil || again.ID != a.ID || againOp.ID != op.ID {
		t.Fatalf("persisted retry: %v", err)
	}
	r, err := reopened.PrepareRun(again)
	if err != nil || !r.Prepared {
		t.Fatalf("snapshot reuse after source changed: %v", err)
	}
}

func TestPortPreflightTransports(t *testing.T) {
	for _, network := range []string{"tcp4", "udp4"} {
		t.Run(network, func(t *testing.T) {
			var close func() error
			var p int
			if network == "tcp4" {
				l, err := net.Listen(network, "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				close = l.Close
				p = l.Addr().(*net.TCPAddr).Port
			} else {
				l, err := net.ListenPacket(network, "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				close = l.Close
				p = l.LocalAddr().(*net.UDPAddr).Port
			}
			defer close()
			requireCode(t, CheckPort(p), "PORT_IN_USE")
		})
	}
	for _, p := range []int{-1, 0, 65536} {
		requireCode(t, CheckPort(p), "PORT_REQUIRED")
	}
}

func TestRunOwnershipAndHistory(t *testing.T) {
	s, path := setup(t)
	a := create(t, s, path, availablePort(t), "a")
	r1, err := s.PrepareRun(a)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, r1.LogPath, []byte("first generation\n"))
	if err := s.CleanupRun(a, r1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r1.CFGPath); !os.IsNotExist(err) {
		t.Fatal("owned cfg remains")
	}
	r2, err := s.PrepareRun(a)
	if err != nil {
		t.Fatal(err)
	}
	if r1.CFGPath == r2.CFGPath || r1.LogPath == r2.LogPath || r2.Generation != 2 {
		t.Fatal("generation files overlap")
	}
	if err := s.CleanupRun(a, r2); err != nil {
		t.Fatal(err)
	}
	a.Lifecycle = "reclaimed"
	a.Cleanup = "complete"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State.Instances[a.ID] == nil || len(reopened.State.Operations) != 1 || len(reopened.State.Keys) != 1 {
		t.Fatal("history lost")
	}
	if b, err := os.ReadFile(r1.LogPath); err != nil || string(b) != "first generation\n" {
		t.Fatal("log lost", err)
	}
	for _, p := range []string{path, filepath.Join(r1.Directory, "generated.cfg"), filepath.Join(r2.Directory, "generated.cfg")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("source/archive removed", err)
		}
	}
	if _, err := reopened.PrepareRun(reopened.State.Instances[a.ID]); err == nil {
		t.Fatal("reclaimed instance revived")
	}
}

func TestCFGConflictAndChangedContentNeverDeleted(t *testing.T) {
	s, path := setup(t)
	a := create(t, s, path, availablePort(t), "a")
	expected := filepath.Join(a.Snapshot.CFG.Directory, fmt.Sprintf("d2core_%s_g1.cfg", a.ID))
	foreign := []byte("user data\n")
	mustWrite(t, expected, foreign)
	r, err := s.PrepareRun(a)
	if err == nil {
		t.Fatal("overwrote existing cfg")
	}
	if r == nil {
		t.Fatal("failed run intent absent")
	}
	if err := s.CleanupRun(a, r); err == nil {
		t.Fatal("conflicting cfg cleanup must report refusal")
	}
	b, _ := os.ReadFile(expected)
	if string(b) != string(foreign) {
		t.Fatal("removed unowned cfg")
	}
	// The test owner resolves its own deliberately introduced conflict. The
	// core must then complete the pending cleanup without creating a process.
	if err := os.Remove(expected); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupRun(a, r); err != nil {
		t.Fatal(err)
	}
	r2, err := s.PrepareRun(a)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(r2.CFGPath)
	mustWrite(t, r2.CFGPath, foreign)
	if err := s.CleanupRun(a, r2); err == nil {
		t.Fatal("removed changed cfg")
	}
	b, _ = os.ReadFile(r2.CFGPath)
	if string(b) != string(foreign) {
		t.Fatal("changed content lost")
	}
	mustWrite(t, r2.CFGPath, original)
	a.Process = "unknown"
	if err := s.CleanupRun(a, r2); err == nil {
		t.Fatal("cleanup with unknown process")
	}
	a.Process = "stopped"
	if err := s.CleanupRun(a, r2); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupRun(a, r2); err != nil {
		t.Fatal("cleanup not repeatable", err)
	}
}

func TestCorruptStateRejectedWithoutOverwrite(t *testing.T) {
	cases := []string{`{`, `null`, `{"formatVersion":2,"instances":{},"operations":{},"keys":{}}`, `{"formatVersion":1,"instances":{},"operations":{}}`, `{"formatVersion":1,"instances":{},"instances":{},"operations":{},"keys":{}}`, `{"formatVersion":1,"instances":{"bad":null},"operations":{},"keys":{}}`, `{"formatVersion":1,"instances":{},"operations":{},"keys":{"a":{"fingerprint":"x","instanceId":"absent","operationId":"absent"}}}`}
	for i, data := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			mustWrite(t, path, []byte(data))
			mustWrite(t, filepath.Join(dir, markerName), []byte(markerContent))
			if _, err := Open(dir); err == nil {
				t.Fatal("accepted corrupt state")
			}
			b, _ := os.ReadFile(path)
			if string(b) != data {
				t.Fatal("overwrote corrupt state")
			}
		})
	}
}

func TestKeysAndFingerprint(t *testing.T) {
	for _, s := range []string{"", strings.Repeat("a", 129), "a b", "中文", "a/b"} {
		if ValidateKey(s) == nil {
			t.Fatal("accepted key", s)
		}
	}
	for _, s := range []string{"abc_123-.XYZ", strings.Repeat("a", 128)} {
		if err := ValidateKey(s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Fingerprint("relative", 123); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestCleanupRejectsUnownedRecord(t *testing.T) {
	s, path := setup(t)
	a := create(t, s, path, availablePort(t), "a")
	r, err := s.PrepareRun(a)
	if err != nil {
		t.Fatal(err)
	}
	copyOfInstance := *a
	if err := s.CleanupRun(&copyOfInstance, r); err == nil {
		t.Fatal("cleanup accepted instance not owned by store")
	}
	if _, err := os.Stat(r.CFGPath); err != nil {
		t.Fatal("unowned cleanup removed cfg", err)
	}
	copyOfRun := *r
	if err := s.CleanupRun(a, &copyOfRun); err == nil {
		t.Fatal("cleanup accepted run not attached to instance")
	}
}

func TestInvalidRunRecordRejected(t *testing.T) {
	s, path := setup(t)
	a := create(t, s, path, availablePort(t), "a")
	a.Runs = []*Run{nil}
	a.Generation = 1
	if err := s.Save(); err == nil {
		t.Fatal("Save accepted null run")
	}
	writeChecksummedState(t, s)
	if _, err := Open(s.Dir); err == nil {
		t.Fatal("accepted null run record")
	}
}
