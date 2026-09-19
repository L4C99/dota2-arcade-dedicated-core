package a2s

import (
	"bytes"
	"os"
	"testing"
)

func TestNativeWritePermissionRefused(t *testing.T) {
	dir, path, before := fixture(t)
	if e := os.Chmod(path, 0400); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(path, 0600)
	if _, e := Enable(dir); e == nil {
		t.Fatal("replacement of readonly file succeeded")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, before) {
		t.Fatal("source changed")
	}
}
