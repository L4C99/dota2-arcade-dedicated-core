package records

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
)

func cfgOpen(path string, access, create uint32) (*os.File, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(p, access, windows.FILE_SHARE_READ, nil, create, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(h), path), nil
}
func cfgKey(f *os.File) (string, error) {
	var info windows.ByHandleFileInformation
	e := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info)
	if e != nil {
		return "", e
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", fmt.Errorf("cfg reparse object")
	}
	return fmt.Sprintf("windows:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
func createOwnedCFG(path string, b []byte) (bool, *CFGOwnership, error) {
	if e := noLinkedParents(filepath.Dir(path)); e != nil {
		return false, nil, e
	}
	parent, e := cfgOpen(filepath.Dir(path), windows.GENERIC_READ, windows.OPEN_EXISTING)
	if e != nil {
		return false, nil, e
	}
	defer parent.Close()
	pk, e := cfgKey(parent)
	if e != nil {
		return false, nil, e
	}
	f, e := cfgOpen(path, windows.GENERIC_WRITE|windows.GENERIC_READ, windows.CREATE_NEW)
	if e != nil {
		return false, nil, e
	}
	defer f.Close()
	fk, e := cfgKey(f)
	if e != nil {
		return true, nil, e
	}
	owner := &CFGOwnership{Parent: pk, File: fk}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	return true, owner, e
}
func cleanupOwnedCFG(path string, owner *CFGOwnership, want string, hook func()) error {
	if e := noLinkedParents(filepath.Dir(path)); e != nil {
		return e
	}
	// Disallow delete sharing: the parent and opened file cannot be renamed or
	// replaced during verification. Delete by this file handle, never by path.
	parent, e := cfgOpen(filepath.Dir(path), windows.GENERIC_READ, windows.OPEN_EXISTING)
	if e != nil {
		return e
	}
	defer parent.Close()
	pk, e := cfgKey(parent)
	if e != nil {
		return e
	}
	if pk != owner.Parent {
		return fmt.Errorf("cfg parent identity changed")
	}
	f, e := cfgOpen(path, windows.GENERIC_READ|windows.DELETE, windows.OPEN_EXISTING)
	if errors.Is(e, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if e != nil {
		return e
	}
	defer f.Close()
	fk, e := cfgKey(f)
	if e != nil {
		return e
	}
	st, e := f.Stat()
	if e != nil {
		return e
	}
	if fk != owner.File || !st.Mode().IsRegular() {
		return fmt.Errorf("cfg file identity changed")
	}
	b, e := io.ReadAll(io.LimitReader(f, 1<<20))
	if e != nil {
		return e
	}
	if digest(b) != want {
		return fmt.Errorf("cfg changed; refusing removal")
	}
	if hook != nil {
		hook()
	}
	disposition := byte(1)
	return windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo, &disposition, 1)
}
