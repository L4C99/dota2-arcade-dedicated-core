//go:build linux

package localipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLinuxPermissionsAndLongPath(t *testing.T) {
	dir, s := serving(t, func(b []byte) []byte { return b })
	st, e := os.Stat(filepath.Join(dir, "manager"))
	if e != nil || st.Mode().Perm() != 0700 {
		t.Fatal(st, e)
	}
	addr := s.listener.Addr().String()
	st, e = os.Stat(addr)
	if e != nil || st.Mode().Perm() != 0600 {
		t.Fatal(st, e)
	}
	s.Close()
	long := filepath.Join(t.TempDir(), strings.Repeat("x", 90))
	if srv, e := Listen(long); e == nil {
		srv.Close()
		t.Fatal("long path accepted")
	}
}

func TestLinuxOtherUserHelper(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--other-user-socket" {
		return
	}
	conn, e := net.DialTimeout("unix", os.Args[len(os.Args)-1], time.Second)
	if e == nil {
		conn.Close()
		os.Exit(7)
	}
	if !errors.Is(e, os.ErrPermission) {
		fmt.Fprintf(os.Stderr, "expected permission denial, got %T: %v\n", e, e)
		os.Exit(8)
	}
	os.Exit(0)
}
func TestLinuxOtherUserDenied(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("cross-UID process execution requires isolated root test invocation; no accounts are created")
	}
	dir, e := os.MkdirTemp("", "d2ipc-user-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	if e = os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	helper := filepath.Join(dir, "helper.test")
	if e = os.WriteFile(helper, b, 0755); e != nil {
		t.Fatal(e)
	}
	s, e := Listen(filepath.Join(dir, "data"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, "-test.run=^TestLinuxOtherUserHelper$", "--", "--other-user-socket", s.listener.Addr().String())
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534, NoSetGroups: true}}
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("cross-UID rejection helper: %v %s", e, out)
	}
}

func TestLinuxUnsafeRootRejectedUnchanged(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if s, e := Listen(dir); e == nil {
		s.Close()
		t.Fatal("nonprivate root accepted")
	}
	st, e := os.Stat(dir)
	if e != nil || st.Mode().Perm() != 0755 {
		t.Fatal("existing root permissions changed", e)
	}
	if _, e := Call(context.Background(), dir, []byte(`{}`)); e == nil {
		t.Fatal("Call accepted nonprivate root")
	}
}
func TestLinuxSocketReplacementPreserved(t *testing.T) {
	_, s := serving(t, func(b []byte) []byte { return b })
	p := s.listener.Addr().String()
	if e := os.Remove(p); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte("foreign"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.Close(); e == nil {
		t.Fatal("ownership loss not reported")
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "foreign" {
		t.Fatal("foreign endpoint removed", e)
	}
}
func TestLinuxPeerUID(t *testing.T) {
	dir, _ := serving(t, func(b []byte) []byte { return b })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := Call(ctx, dir, []byte(`{}`)); e != nil {
		t.Fatal(e)
	}
	if e := authorizePeer(&net.TCPConn{}); e == nil {
		t.Fatal("wrong peer transport accepted")
	}
}
