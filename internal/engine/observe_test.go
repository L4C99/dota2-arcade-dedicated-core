package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
)

func TestEvidenceCurrentGenerationAndExit(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.log")
	path := filepath.Join(dir, "current.log")
	if err := os.WriteFile(old, []byte("loaded ready"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("loa"), 0600); err != nil {
		t.Fatal(err)
	}
	o := NewObserver(2, path, config.Readiness{SuccessAll: []string{"loaded", "ready"}, FailureAny: []string{"fatal"}})
	got, err := o.Scan(true)
	if err != nil || got.Ready {
		t.Fatal("old log or partial signal counted", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("ded ready 中文玩家\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err = o.Scan(true)
	if err != nil || !got.Ready || got.Evidence.Generation != 2 || got.Evidence.Source != path {
		t.Fatal(got, err)
	}
	got, err = o.Scan(false)
	if err != nil || got.Ready || got.Evidence.Valid {
		t.Fatal("exit kept readiness", err)
	}
	if err = os.WriteFile(path, []byte("fatal loaded ready"), 0600); err != nil {
		t.Fatal(err)
	}
	// This is a deliberate replacement of the same generation file, not a
	// restart. A new generation always has a distinct path.
	o = NewObserver(2, path, o.rules)
	got, err = o.Scan(true)
	if err != nil || got.Ready || got.Failure != "fatal" {
		t.Fatal("failure not prioritized", err)
	}
	if err = os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got, err = o.Scan(true)
	if err != nil || got.Ready || len(got.Evidence.Matched) != 0 {
		t.Fatal("truncation retained old evidence", err)
	}
}

func TestTailBoundAndUnicode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "engine.log")
	if err := os.WriteFile(p, []byte(strings.Repeat("old\n", 70000)+"中文玩家\nlast\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, truncated, err := ReadTail(p, 2)
	if err != nil || !truncated || got != "中文玩家\nlast\n" {
		t.Fatalf("%q %v %v", got, truncated, err)
	}
}
