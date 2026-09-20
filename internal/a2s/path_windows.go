package a2s

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
)

func plainInstallationPath(path string) error {
	// Case and DOS aliases are names of the same object, not reparse points.
	// Inspect native attributes on every component rather than comparing names.
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		name, e := windows.UTF16PtrFromString(p)
		if e != nil {
			return e
		}
		attrs, e := windows.GetFileAttributes(name)
		if e != nil {
			return e
		}
		if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("linked gameinfo path rejected; select the resolved installation")
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}
