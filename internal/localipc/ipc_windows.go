//go:build windows

package localipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func currentSID() (string, error) {
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", e
	}
	return u.User.Sid.String(), nil
}
func directorySDDL() (string, error) {
	sid, e := currentSID()
	if e != nil {
		return "", e
	}
	return "D:P(A;OICI;FA;;;" + sid + ")", nil
}
func verifyDirectory(path string) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("manager path is not a private directory")
	}
	ownerSD, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	owner, _, e := ownerSD.Owner()
	if e != nil {
		return e
	}
	sid, e := currentSID()
	if e != nil {
		return e
	}
	if owner == nil || owner.String() != sid {
		return fmt.Errorf("private directory owner must be current user")
	}
	actual, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	sddl, e := directorySDDL()
	if e != nil {
		return e
	}
	expected, e := windows.SecurityDescriptorFromString(sddl)
	if e != nil {
		return e
	}
	if actual.String() != expected.String() {
		return fmt.Errorf("manager directory ACL is not restricted to current user")
	}
	return nil
}
func prepareDirectory(path string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	if _, e := os.Lstat(path); e == nil {
		return verifyDirectory(path)
	} else if !os.IsNotExist(e) {
		return e
	}
	text, e := directorySDDL()
	if e != nil {
		return e
	}
	sid, e := currentSID()
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("O:" + sid + text)
	if e != nil {
		return e
	}
	ptr, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if e = windows.CreateDirectory(ptr, &sa); e != nil && e != windows.ERROR_ALREADY_EXISTS {
		return e
	}
	return verifyDirectory(path)
}
func acquireLock(path string) (func() error, error) {
	ptr, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("invalid lock file")
	}
	var overlap windows.Overlapped
	if e = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap); e != nil {
		windows.CloseHandle(h)
		return nil, ErrLocked
	}
	return func() error { return windows.CloseHandle(h) }, nil
}
func listenPlatform(manager, token string) (net.Listener, Endpoint, func() error, error) {
	ep := Endpoint{ProtocolVersion: 1, Type: "named-pipe", Address: `\\.\pipe\d2core-` + token}
	sid, e := currentSID()
	if e != nil {
		return nil, ep, nil, e
	}
	// go-winio v0.6.2 pipe.go makeServerPipeHandle always sets
	// FILE_PIPE_REJECT_REMOTE_CLIENTS, independently of this user-only DACL.
	l, e := winio.ListenPipe(ep.Address, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")", InputBufferSize: 65536, OutputBufferSize: 65536})
	if e != nil {
		return nil, ep, nil, e
	}
	return l, ep, func() error { return nil }, nil
}
func authorizePeer(conn net.Conn) error { return nil } // Kernel enforces the user SID DACL and rejects remote pipe clients.
func dialPlatform(ctx context.Context, manager string, ep Endpoint) (net.Conn, error) {
	if ep.Type != "named-pipe" || !strings.HasPrefix(ep.Address, `\\.\pipe\d2core-`) || len(strings.TrimPrefix(ep.Address, `\\.\pipe\d2core-`)) != 32 {
		return nil, fmt.Errorf("invalid local named pipe endpoint")
	}
	return winio.DialPipeContext(ctx, ep.Address)
}
