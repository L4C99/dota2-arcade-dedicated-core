package a2s

import (
	"fmt"
	"path/filepath"
)

func plainInstallationPath(path string) error {
	real, e := filepath.EvalSymlinks(path)
	if e != nil {
		return e
	}
	if real != path {
		return fmt.Errorf("linked gameinfo path rejected; select the resolved installation")
	}
	return nil
}
