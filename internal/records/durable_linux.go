//go:build linux

package records

import (
	"os"
	"path/filepath"
)

func syncDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func replaceState(from, to string) (bool, error) {
	if e := os.Rename(from, to); e != nil {
		return false, e
	}
	return true, syncDirectory(filepath.Dir(to))
}
