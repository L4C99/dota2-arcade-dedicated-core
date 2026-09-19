//go:build windows

package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

func TestDiscoverWindowsExactArgumentsAndTime(t *testing.T) {
	t.Setenv("D2CORE_ENGINE_CHILD", "1")
	exe, _ := os.Executable()
	run := t.TempDir()
	spec := Spec{Executable: exe, WorkingDirectory: run, RunDirectory: run, Arguments: []string{"-test.run=^TestEngineChild$", "--", run, "space and 中文", `quote"slash\`, "heartbeat"}}
	before := time.Now().Add(-time.Second)
	id, e := Start(spec)
	if e != nil {
		t.Fatal(e)
	}
	h, e := Open(id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Kill(); h.Close() })
	found, e := Discover(spec, before)
	if e != nil || len(found) != 1 || found[0].PID != id.PID {
		t.Fatalf("discovery: %+v %v", found, e)
	}
	altered := spec
	altered.Arguments = append([]string{}, spec.Arguments...)
	altered.Arguments[2] = "different unique generation"
	if found, e = Discover(altered, before); e != nil || len(found) != 0 {
		t.Fatalf("wrong args: %+v %v", found, e)
	}
	if found, e = Discover(spec, time.Now().Add(time.Second)); e != nil || len(found) != 0 {
		t.Fatalf("old process: %+v %v", found, e)
	}
	second := spec
	second.RunDirectory = filepath.Join(run, "second")
	if e = os.Mkdir(second.RunDirectory, 0700); e != nil {
		t.Fatal(e)
	}
	other, e := Start(second)
	if e != nil {
		t.Fatal(e)
	}
	otherHandle, e := Open(other)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { otherHandle.Kill(); otherHandle.Close() })
	found, e = Discover(spec, before)
	if e != nil || len(found) != 2 {
		t.Fatalf("ambiguity must be returned: %+v %v", found, e)
	}
}
func TestDecodeCommandLineRejectsInvalidPointer(t *testing.T) {
	b := make([]byte, 64)
	header := (*ntUnicodeString)(unsafe.Pointer(&b[0]))
	header.Length = 4
	header.MaximumLength = 4
	header.Buffer = uintptr(unsafe.Pointer(&b[0])) + 63
	if _, e := decodeCommandLine(b); e == nil {
		t.Fatal("accepted pointer outside bounded buffer")
	}
	header.Buffer = uintptr(unsafe.Pointer(&b[0])) + 32
	header.Length = 3
	if _, e := decodeCommandLine(b); e == nil {
		t.Fatal("accepted odd UTF16 length")
	}
}
