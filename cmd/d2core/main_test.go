package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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
