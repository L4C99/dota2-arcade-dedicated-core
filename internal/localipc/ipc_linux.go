//go:build linux

package localipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func verifyDirectory(path string) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	native, ok := st.Sys().(*syscall.Stat_t)
	if !ok || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm() != 0700 || native.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("manager directory must be owned by current user with mode 0700")
	}
	return nil
}
func prepareDirectory(path string) error {
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	return verifyDirectory(path)
}
func acquireLock(path string) (func() error, error) {
	fd, e := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	var st syscall.Stat_t
	if e = syscall.Fstat(fd, &st); e != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Uid != uint32(os.Geteuid()) {
		syscall.Close(fd)
		return nil, fmt.Errorf("invalid lock file")
	}
	if e = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		syscall.Close(fd)
		return nil, ErrLocked
	}
	return func() error { return syscall.Close(fd) }, nil
}
func listenPlatform(manager, token string) (net.Listener, Endpoint, func() error, error) {
	ep := Endpoint{ProtocolVersion: 1, Type: "unix", Address: filepath.Join(manager, "ipc-"+token+".sock")}
	if len(ep.Address) >= 108 {
		return nil, ep, nil, fmt.Errorf("Unix socket path exceeds 107 bytes; select shorter data-dir")
	}
	l, e := net.ListenUnix("unix", &net.UnixAddr{Name: ep.Address, Net: "unix"})
	if e != nil {
		return nil, ep, nil, e
	}
	l.SetUnlinkOnClose(false)
	st, e := os.Lstat(ep.Address)
	if e != nil {
		l.Close()
		return nil, ep, nil, e
	}
	if e = os.Chmod(ep.Address, 0600); e != nil {
		l.Close()
		os.Remove(ep.Address)
		return nil, ep, nil, e
	}
	cleanup := func() error {
		now, e := os.Lstat(ep.Address)
		if errors.Is(e, os.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		if !os.SameFile(st, now) || now.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("socket ownership changed; not removed")
		}
		return os.Remove(ep.Address)
	}
	return l, ep, cleanup, nil
}
func authorizePeer(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("unexpected connection type")
	}
	raw, e := uc.SyscallConn()
	if e != nil {
		return e
	}
	var credential *syscall.Ucred
	var checkErr error
	e = raw.Control(func(fd uintptr) {
		credential, checkErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if e != nil {
		return e
	}
	if checkErr != nil {
		return checkErr
	}
	if credential.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("peer UID rejected")
	}
	return nil
}
func dialPlatform(ctx context.Context, manager string, ep Endpoint) (net.Conn, error) {
	if ep.Type != "unix" || filepath.Dir(ep.Address) != manager || !strings.HasPrefix(filepath.Base(ep.Address), "ipc-") || !strings.HasSuffix(ep.Address, ".sock") {
		return nil, fmt.Errorf("endpoint outside private manager directory")
	}
	st, e := os.Lstat(ep.Address)
	if e != nil {
		return nil, e
	}
	native, ok := st.Sys().(*syscall.Stat_t)
	if !ok || st.Mode()&os.ModeSocket == 0 || st.Mode().Perm() != 0600 || native.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("unsafe socket")
	}
	var d net.Dialer
	c, e := d.DialContext(ctx, "unix", ep.Address)
	if e != nil {
		return nil, e
	}
	if e = authorizePeer(c); e != nil {
		c.Close()
		return nil, e
	}
	return c, nil
}
