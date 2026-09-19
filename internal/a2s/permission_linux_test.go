package a2s

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeWritePermissionRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires ordinary user for native permission denial")
	}
	dir, path, before := fixture(t)
	parent := filepath.Dir(path)
	if e := os.Chmod(parent, 0500); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(parent, 0700)
	if _, e := Enable(dir); e == nil {
		t.Fatal("write to readonly directory succeeded")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, before) {
		t.Fatal("source changed")
	}
}
