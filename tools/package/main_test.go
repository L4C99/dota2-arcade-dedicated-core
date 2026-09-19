package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveDeterministicAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if e := os.WriteFile(source, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	files := map[string]string{"d2core": source, "docs/readme.md": source}
	stamp := time.Unix(1700000000, 0).UTC()
	a, b := filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")
	if e := archive(a, files, stamp); e != nil {
		t.Fatal(e)
	}
	if e := archive(b, files, stamp); e != nil {
		t.Fatal(e)
	}
	aa, _ := os.ReadFile(a)
	bb, _ := os.ReadFile(b)
	if !bytes.Equal(aa, bb) {
		t.Fatal("archive not deterministic")
	}
	if e := archive(a, files, stamp); e == nil {
		t.Fatal("overwrote archive")
	}
	z, e := zip.OpenReader(a)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 2 || z.File[0].Name != "d2core" || z.File[0].Mode().Perm() != 0755 {
		t.Fatal("archive layout or executable mode")
	}
}
