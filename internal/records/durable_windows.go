//go:build windows

package records

import "golang.org/x/sys/windows"

// File content is flushed before publication. Win32 has no general unprivileged
// directory-fsync equivalent; state publication requests WRITE_THROUGH instead.
// Real power-loss/filesystem guarantees remain subject to platform acceptance.
func syncDirectory(path string) error { return nil }
func replaceState(from, to string) (bool, error) {
	a, e := windows.UTF16PtrFromString(from)
	if e != nil {
		return false, e
	}
	b, e := windows.UTF16PtrFromString(to)
	if e != nil {
		return false, e
	}
	e = windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	return e == nil, e
}
