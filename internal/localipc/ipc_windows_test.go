//go:build windows

package localipc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsAnonymousPipeRejected(t *testing.T) {
	dir, _ := serving(t, func(b []byte) []byte { return b })
	if _, e := Call(context.Background(), dir, []byte(`{}`)); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "manager", "endpoint.json"))
	if e != nil {
		t.Fatal(e)
	}
	var ep Endpoint
	if e = json.Unmarshal(b, &ep); e != nil {
		t.Fatal(e)
	}
	name, e := windows.UTF16PtrFromString(ep.Address)
	if e != nil {
		t.Fatal(e)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread, e := windows.GetCurrentThread()
	if e != nil {
		t.Fatal(e)
	}
	r, _, callErr := windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateAnonymousToken").Call(uintptr(thread))
	if r == 0 {
		t.Fatal(callErr)
	}
	handle, openErr := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	revertErr := windows.RevertToSelf()
	if openErr == nil {
		windows.CloseHandle(handle)
	}
	if revertErr != nil {
		t.Fatal(revertErr)
	}
	if !errors.Is(openErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("anonymous pipe access must be denied, got %v", openErr)
	}
}

func TestWindowsUnsafeRootRejectedUnchanged(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if e := prepareDirectory(dir); e != nil {
		t.Fatal(e)
	}
	sid, e := currentSID()
	if e != nil {
		t.Fatal(e)
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FR;;;WD)")
	if e != nil {
		t.Fatal(e)
	}
	dacl, _, e := sd.DACL()
	if e != nil {
		t.Fatal(e)
	}
	if e = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); e != nil {
		t.Fatal(e)
	}
	before, e := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	if s, e := Listen(dir); e == nil {
		s.Close()
		t.Fatal("root granting Everyone read accepted")
	}
	after, e := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	if before.String() != after.String() {
		t.Fatal("existing ACL changed")
	}
	if _, e := Call(context.Background(), dir, []byte(`{}`)); e == nil {
		t.Fatal("Call accepted unsafe root")
	}
}
