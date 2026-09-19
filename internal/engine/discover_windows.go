//go:build windows

package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var queryProcessInfo = syscall.NewLazyDLL("ntdll.dll").NewProc("NtQueryInformationProcess")

// Discover only returns identity-verified candidates. The caller must reject
// ambiguity and must never interpret an error as an empty process set.
// Class 60 is a Windows 10 tested NT capability, not a stable Win32 promise:
// https://github.com/winsiderss/phnt/blob/master/ntpsapi.h
// Unsupported queries fail closed; there is no PID/name-only fallback.
func Discover(spec Spec, notBefore time.Time) ([]Identity, error) {
	if !filepath.IsAbs(spec.Executable) || !filepath.IsAbs(spec.WorkingDirectory) || !filepath.IsAbs(spec.RunDirectory) || notBefore.IsZero() {
		return nil, ErrIdentity
	}
	exe, err := filepath.EvalSymlinks(spec.Executable)
	if err != nil {
		return nil, err
	}
	if len(filepath.Base(exe)) >= 259 {
		return nil, fmt.Errorf("discovery executable name exceeds snapshot bound")
	}
	args := append([]string{exe}, spec.Arguments...)
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = syscall.EscapeArg(arg)
	}
	expected := strings.Join(escaped, " ")
	if len(expected) > 256<<10 {
		return nil, fmt.Errorf("discovery command exceeds bound")
	}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	err = syscall.Process32First(snapshot, &entry)
	out := []Identity{}
	deadline := time.Now().Add(5 * time.Second)
	for n := 0; err == nil; n++ {
		if n >= 65536 || time.Now().After(deadline) {
			return nil, fmt.Errorf("process discovery exceeded bound")
		}
		if strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), filepath.Base(exe)) {
			id, matched, e := discoverWindowsCandidate(int(entry.ProcessID), exe, expected, spec, notBefore)
			if e != nil {
				return nil, fmt.Errorf("candidate PID %d: %w", entry.ProcessID, e)
			}
			if matched {
				out = append(out, id)
				if len(out) > 64 {
					return nil, fmt.Errorf("too many matching processes")
				}
			}
		}
		err = syscall.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, syscall.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return out, nil
}

func discoverWindowsCandidate(pid int, exe, expected string, spec Spec, notBefore time.Time) (Identity, bool, error) {
	h, err := syscall.OpenProcess(0x1000|0x100000|1, false, uint32(pid))
	if errors.Is(err, syscall.Errno(87)) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, err
	}
	handle := &Handle{handle: h}
	defer handle.Close()
	alive, err := handle.Alive()
	if err != nil || !alive {
		return Identity{}, false, err
	}
	id, err := nativeIdentity(h, pid)
	if err != nil {
		return Identity{}, false, err
	}
	if !strings.EqualFold(filepath.Clean(id.Executable), filepath.Clean(exe)) {
		return Identity{}, false, nil
	}
	ft := syscall.Filetime{LowDateTime: uint32(id.CreationTime), HighDateTime: uint32(id.CreationTime >> 32)}
	if time.Unix(0, ft.Nanoseconds()).Before(notBefore) {
		return Identity{}, false, nil
	}
	raw, err := windowsCommandLine(h)
	if err != nil {
		return Identity{}, false, err
	}
	if raw != expected {
		return Identity{}, false, nil
	}
	id.Arguments = append([]string{}, spec.Arguments...)
	id.RunDirectory = spec.RunDirectory
	verified, err := Open(id)
	if errors.Is(err, ErrGone) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, err
	}
	defer verified.Close()
	// Re-read through the original held handle, so a mutable command line cannot
	// silently change between our observation and verification.
	again, err := windowsCommandLine(h)
	if err != nil {
		return Identity{}, false, err
	}
	if again != raw {
		return Identity{}, false, ErrIdentity
	}
	alive, err = handle.Alive()
	if err != nil || !alive {
		return Identity{}, false, err
	}
	return id, true, nil
}

func windowsCommandLine(h syscall.Handle) (string, error) {
	if err := queryProcessInfo.Find(); err != nil {
		return "", err
	}
	var size uint32
	queryProcessInfo.Call(uintptr(h), 60, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size < uint32(unsafe.Sizeof(ntUnicodeString{})) || size > 256<<10 {
		return "", fmt.Errorf("unsupported or oversized command-line query")
	}
	for attempt := 0; attempt < 3; attempt++ {
		buffer := make([]byte, size)
		status, _, _ := queryProcessInfo.Call(uintptr(h), 60, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&size)))
		if uint32(status) == 0xc0000004 || uint32(status) == 0xc0000023 {
			if size > 256<<10 {
				return "", fmt.Errorf("command-line query exceeds bound")
			}
			continue
		}
		if uint32(status) != 0 {
			return "", fmt.Errorf("command-line query NTSTATUS 0x%x", uint32(status))
		}
		return decodeCommandLine(buffer)
	}
	return "", fmt.Errorf("command line changed during bounded query")
}

type ntUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        uintptr
}

func decodeCommandLine(buffer []byte) (string, error) {
	if len(buffer) < int(unsafe.Sizeof(ntUnicodeString{})) {
		return "", ErrIdentity
	}
	header := *(*ntUnicodeString)(unsafe.Pointer(&buffer[0]))
	base := uintptr(unsafe.Pointer(&buffer[0]))
	if header.Length%2 != 0 || header.Length > header.MaximumLength || header.Buffer < base || header.Buffer-base > uintptr(len(buffer)) || uintptr(header.Length) > uintptr(len(buffer))-(header.Buffer-base) {
		return "", ErrIdentity
	}
	offset := int(header.Buffer - base)
	chars := make([]uint16, int(header.Length)/2)
	for i := range chars {
		chars[i] = uint16(buffer[offset+2*i]) | uint16(buffer[offset+2*i+1])<<8
	}
	return string(utf16.Decode(chars)), nil
}
