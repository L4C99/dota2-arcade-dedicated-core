package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpDoesNotRunCommands(t *testing.T) {
	for _, args := range [][]string{{"serve", "--help"}, {"create", "--help"}, {"help", "stop"}, {"help", "a2s", "enable"}, {"m0-inspect", "--help"}} {
		if err := run(args); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("%v: expected help without required parameters or side effects, got %v", args, err)
		}
	}
	for _, command := range []string{"version", "check", "serve", "create", "list", "status", "operation", "logs", "restart", "stop", "a2s enable", "m0-inspect"} {
		if !strings.Contains(commandHelp, command) {
			t.Fatalf("missing command %q in overview", command)
		}
	}
	if err := run([]string{"help", "__engine-quit"}); err == nil {
		t.Fatal("internal command must not be dispatched by public help")
	}
}

func TestInjectedVersion(t *testing.T) {
	previous := releaseVersion
	defer func() { releaseVersion = previous }()
	releaseVersion = "0.1.0-rc.2"
	v, err := capture(t, "version", "--json")
	if err != nil || v["result"].(map[string]any)["version"] != releaseVersion {
		t.Fatalf("version did not report build label: %v %v", v, err)
	}
}

func capture(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original; r.Close(); w.Close() }()
	err = run(args)
	w.Close()
	b, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var value map[string]any
	if len(b) > 0 && json.Unmarshal(b, &value) != nil {
		t.Fatalf("stdout is not one JSON object: %q", b)
	}
	return value, err
}

func TestVersionAndUsage(t *testing.T) {
	v, err := capture(t, "version", "--json")
	if err != nil || v["ok"] != true || v["protocolVersion"] != float64(1) {
		t.Fatalf("%v: %v", v, err)
	}
	for _, args := range [][]string{nil, {"start"}, {"check"}, {"version", "unexpected"}} {
		_, err := capture(t, args...)
		var u usageError
		if !errors.As(err, &u) {
			t.Fatalf("%v did not return usage error: %v", args, err)
		}
	}
}

func TestInvalidCheckIsStructuredAndDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	data := []byte(`{"schemaVersion":1,"name":"x","name":"y"}`)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	v, err := capture(t, "check", "--template", p, "--json")
	if err == nil || v["ok"] != false {
		t.Fatalf("%v: %v", v, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("check wrote files: %v %v", entries, err)
	}
	after, err := os.ReadFile(p)
	if err != nil || string(after) != string(data) {
		t.Fatal("check changed source template")
	}
}
