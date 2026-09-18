package m0

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingInputIsNotEngineSuccess(t *testing.T) {
	r := Inspect(Input{})
	if r.EngineValidation != "not_run" || len(r.Checks) != 6 {
		t.Fatal(r)
	}
	for _, c := range r.Checks {
		if c.Status != "missing_input" {
			t.Fatal(c)
		}
	}
}

func TestInspectionHashesWithoutChangingResources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "地图 with spaces.vpk")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	r := Inspect(Input{VPK: path, WorkingDirectory: dir})
	c := r.Checks[3]
	if !r.InputsValid || c.Status != "observed" || c.SHA256 != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal(r)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "abc" {
		t.Fatalf("resource modified: %q %v", b, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected writes: %v %v", entries, err)
	}
}

func TestInvalidPaths(t *testing.T) {
	dir := t.TempDir()
	for _, in := range []Input{
		{VPK: "relative.vpk"}, {VPK: dir}, {VPK: filepath.Join(dir, "missing.vpk")},
	} {
		if Inspect(in).InputsValid {
			t.Fatalf("accepted invalid input: %+v", in)
		}
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if Inspect(Input{WorkingDirectory: file}).InputsValid {
		t.Fatal("accepted file as directory")
	}
}
