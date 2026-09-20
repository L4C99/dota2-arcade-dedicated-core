package records

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func cfgKey(f *os.File) (string, error) {
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	s := st.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("linux:%d:%d", s.Dev, s.Ino), nil
}
func cfgParent(path string) (*os.File, error) {
	p := filepath.Dir(path)
	if e := noLinkedParents(p); e != nil {
		return nil, e
	}
	fd, e := unix.Open(p, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), p), nil
}
func createOwnedCFG(path string, b []byte) (bool, *CFGOwnership, error) {
	parent, e := cfgParent(path)
	if e != nil {
		return false, nil, e
	}
	defer parent.Close()
	pk, e := cfgKey(parent)
	if e != nil {
		return false, nil, e
	}
	fd, e := unix.Openat(int(parent.Fd()), filepath.Base(path), unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return false, nil, e
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	fk, e := cfgKey(f)
	if e != nil {
		return true, nil, e
	}
	owner := &CFGOwnership{Parent: pk, File: fk}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if e == nil {
		e = parent.Sync()
	}
	return true, owner, e
}
func cleanupOwnedCFG(path string, owner *CFGOwnership, want string, hook func()) error {
	parent, e := cfgParent(path)
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
	fd := int(parent.Fd())
	name := filepath.Base(path)
	ffd, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(e, unix.ENOENT) {
		return parent.Sync()
	}
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(ffd), path)
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
	current, e := cfgParent(path)
	if e != nil {
		return e
	}
	currentKey, e := cfgKey(current)
	current.Close()
	if e != nil {
		return e
	}
	if currentKey != owner.Parent {
		return fmt.Errorf("cfg parent changed before deletion")
	}
	// Capture the directory entry before destructive deletion, then verify the
	// captured object again. A raced foreign entry is restored without overwriting
	// anything. The random private directory is never exposed to other processes.
	token, e := ID("cleanup")
	if e != nil {
		return e
	}
	q := ".d2core-" + token
	if e = unix.Mkdirat(fd, q, 0700); e != nil {
		return e
	}
	qfd, e := unix.Openat(fd, q, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(qfd)
	if e = unix.Renameat2(fd, name, qfd, "cfg", unix.RENAME_NOREPLACE); e != nil {
		_ = unix.Unlinkat(fd, q, unix.AT_REMOVEDIR)
		return e
	}
	captured, e := unix.Openat(qfd, "cfg", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	valid := false
	if e == nil {
		cf := os.NewFile(uintptr(captured), "captured cfg")
		key, ke := cfgKey(cf)
		cs, se := cf.Stat()
		bytes, re := io.ReadAll(io.LimitReader(cf, 1<<20))
		cf.Close()
		valid = ke == nil && se == nil && re == nil && cs.Mode().IsRegular() && key == owner.File && digest(bytes) == want
	}
	if !valid {
		restore := unix.Renameat2(qfd, "cfg", fd, name, unix.RENAME_NOREPLACE)
		if restore == nil {
			_ = unix.Unlinkat(fd, q, unix.AT_REMOVEDIR)
		}
		return errors.Join(fmt.Errorf("cfg entry changed before deletion; retained at %s/%s/cfg if restore failed", parent.Name(), q), restore)
	}
	if e = unix.Unlinkat(qfd, "cfg", 0); e != nil {
		return e
	}
	if e = unix.Unlinkat(fd, q, unix.AT_REMOVEDIR); e != nil {
		return e
	}
	return parent.Sync()
}
