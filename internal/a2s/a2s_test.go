package a2s

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMergePreservesOriginalAndIsIdempotent(t *testing.T) {
	for _, s := range []string{"\xef\xbb\xbf\"GameInfo\"\r\n{ // keep\r\n Game a\r\n Game b\r\n}\r\n", `GameInfo { GMS { Other "value" } }`, "GameInfo { /* Advertise 0 */ text \"a\\\"b\" }"} {
		b := []byte(s)
		got, changed, e := Merge(b)
		if e != nil || !changed {
			t.Fatalf("merge: %v", e)
		}
		// One insertion leaves original prefix and suffix unchanged.
		i := 0
		for i < len(b) && i < len(got) && b[i] == got[i] {
			i++
		}
		if !bytes.Equal(b[i:], got[i+len(got)-len(b):]) {
			t.Fatal("modified original content")
		}
		again, changed, e := Merge(got)
		if e != nil || changed || !bytes.Equal(got, again) {
			t.Fatalf("not idempotent: %v", e)
		}
	}
}

func TestMergeRejectsConflictAndMalformedStructure(t *testing.T) {
	for _, s := range []string{`GameInfo { GMS { Advertise 0 } }`, `GameInfo { GMS 1 }`, `GameInfo { GMS {} gms {} }`, `GameInfo {GMS {Advertise 1 Advertise 1}}`, `GameInfo {`, `GameInfo {} Other {}`, `#base "x" GameInfo {}`, "GameInfo { x \"unterminated }", "GameInfo { /*", "GameInfo {\x00}"} {
		if _, _, e := Merge([]byte(s)); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func fixture(t *testing.T) (string, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "game", "dota", "gameinfo.gi")
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		t.Fatal(e)
	}
	b := []byte("GameInfo\n{\n\t// keep this comment\n\tGame dota\n}\n")
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	return dir, p, b
}

func TestEnableBackupAndNoop(t *testing.T) {
	dir, path, before := fixture(t)
	r, e := Enable(dir)
	if e != nil || r.Status != "inserted" {
		t.Fatal(r, e)
	}
	b, e := os.ReadFile(r.Backup)
	if e != nil || !bytes.Equal(b, before) {
		t.Fatal("bad backup", e)
	}
	after, _ := os.ReadFile(path)
	again, e := Enable(dir)
	if e != nil || again.Status != "already_configured" || again.Backup != "" {
		t.Fatal(again, e)
	}
	last, _ := os.ReadFile(path)
	if !bytes.Equal(last, after) {
		t.Fatal("noop changed file")
	}
	matches, _ := filepath.Glob(path + ".d2core-*.bak")
	if len(matches) != 1 {
		t.Fatal(matches)
	}
}

func TestEnableDetectsConcurrentChange(t *testing.T) {
	dir, path, _ := fixture(t)
	foreign := []byte("GameInfo { Game changed }")
	r, e := enable(dir, func(stage string) error {
		if stage == "before-replace" {
			return os.WriteFile(path, foreign, 0600)
		}
		return nil
	})
	if e == nil || r.Status != "conflict" {
		t.Fatal(r, e)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, foreign) {
		t.Fatal("overwrote concurrent edit")
	}
}

func TestEnableFailureKeepsBackupAndNeverClaimsSuccess(t *testing.T) {
	for _, stage := range []string{"backup-saved", "before-replace", "after-replace"} {
		t.Run(stage, func(t *testing.T) {
			dir, path, before := fixture(t)
			r, e := enable(dir, func(at string) error {
				if at == stage {
					return errors.New("injected write failure")
				}
				return nil
			})
			if e == nil || r.Status != "error" {
				t.Fatal(r, e)
			}
			backup, e := os.ReadFile(r.Backup)
			if e != nil || !bytes.Equal(backup, before) {
				t.Fatal("backup not intact", e)
			}
			got, _ := os.ReadFile(path)
			if stage != "after-replace" && !bytes.Equal(got, before) {
				t.Fatal("original changed before publication")
			}
		})
	}
}

func TestEnableLockRefused(t *testing.T) {
	dir, path, before := fixture(t)
	lock := path + ".d2core-a2s.lock"
	if e := os.WriteFile(lock, []byte("foreign"), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := Enable(dir)
	if e == nil || r.Status != "conflict" {
		t.Fatal(r, e)
	}
	if b, _ := os.ReadFile(lock); string(b) != "foreign" {
		t.Fatal("removed foreign lock")
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, before) {
		t.Fatal("changed source")
	}
}

func TestEnableSymlinkRefused(t *testing.T) {
	dir, path, before := fixture(t)
	target := filepath.Join(t.TempDir(), "foreign.gi")
	if e := os.Rename(path, target); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, path); e != nil {
		t.Skipf("symlink creation not allowed: %v", e)
	}
	if _, e := Enable(dir); e == nil {
		t.Fatal("linked source accepted")
	}
	b, e := os.ReadFile(target)
	if e != nil || !bytes.Equal(b, before) {
		t.Fatal("target changed", e)
	}
}
