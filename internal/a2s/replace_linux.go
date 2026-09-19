//go:build linux

package a2s

import (
	"os"
	"path/filepath"
)

func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func replace(from, to string) error {
	if e := os.Rename(from, to); e != nil {
		return e
	}
	return syncDir(filepath.Dir(to))
}
