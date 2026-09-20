package engine

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var queryImage = kernel.NewProc("QueryFullProcessImageNameW")
var freeConsole = kernel.NewProc("FreeConsole")
var attachConsole = kernel.NewProc("AttachConsole")
var consoleProcesses = kernel.NewProc("GetConsoleProcessList")
var writeConsoleInput = kernel.NewProc("WriteConsoleInputW")
var iphelper = syscall.NewLazyDLL("iphlpapi.dll")
var tcpTable = iphelper.NewProc("GetExtendedTcpTable")
var udpTable = iphelper.NewProc("GetExtendedUdpTable")

type Handle struct {
	handle   syscall.Handle
	identity Identity
}

func CleanupInput(id Identity) error { return nil }

func nativeIdentity(handle syscall.Handle, pid int) (Identity, error) {
	var creation, exit, kernelTime, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &creation, &exit, &kernelTime, &user); err != nil {
		return Identity{}, err
	}
	buf := make([]uint16, 32768)
	count := uint32(len(buf))
	ok, _, err := queryImage.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&count)))
	if ok == 0 {
		return Identity{}, err
	}
	return Identity{PID: pid, Executable: syscall.UTF16ToString(buf[:count]), CreationTime: uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)}, nil
}

func Start(spec Spec) (Identity, error) {
	return startWithIdentity(spec, syscall.OpenProcess, nativeIdentity)
}
func startWithIdentity(spec Spec, openProcess func(uint32, bool, uint32) (syscall.Handle, error), readID func(syscall.Handle, int) (Identity, error)) (Identity, error) {
	executable, err := filepath.EvalSymlinks(spec.Executable)
	if err != nil {
		return Identity{}, err
	}
	output, err := os.OpenFile(filepath.Join(spec.RunDirectory, "output.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Identity{}, err
	}
	defer output.Close()
	cmd := exec.Command(executable, spec.Arguments...)
	cmd.Dir = spec.WorkingDirectory
	// A nil os/exec Stdin is an inherited NUL file, which prevents AllocConsole
	// from installing the engine's console input handle. Pass a NULL handle:
	// Windows can initialize it when the dedicated engine allocates its console.
	// stdout/stderr still go directly to the owned file while the manager is offline.
	nullInput := os.NewFile(0, "NULL standard input")
	// Closing this wrapper only attempts CloseHandle(0); it cannot close a
	// parent's input or the console handle later installed in the child.
	defer nullInput.Close()
	cmd.Stdin = nullInput
	cmd.Stdout = output
	cmd.Stderr = output
	// The dedicated engine allocates its own console, as in the M0 probe.
	// No job object ties the child lifetime to this process.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
	if err = cmd.Start(); err != nil {
		return Identity{}, err
	}
	candidate := Identity{PID: cmd.Process.Pid, Executable: executable, Arguments: append([]string{}, spec.Arguments...), RunDirectory: spec.RunDirectory}
	h, err := openProcess(0x1000|0x100000|1, false, uint32(candidate.PID))
	if err == nil {
		var id Identity
		id, err = readID(h, candidate.PID)
		syscall.CloseHandle(h)
		if err == nil {
			candidate.CreationTime = id.CreationTime
			candidate.Executable = id.Executable
		}
	}
	if err != nil {
		return candidate, rollbackStart(cmd, err)
	}
	go func() { _ = cmd.Wait() }()
	return candidate, nil
}

func Open(id Identity) (*Handle, error) {
	if id.PID <= 0 || id.CreationTime == 0 || !filepath.IsAbs(id.Executable) {
		return nil, ErrIdentity
	}
	h, err := syscall.OpenProcess(0x1000|0x100000|1, false, uint32(id.PID))
	if errors.Is(err, syscall.Errno(87)) {
		return nil, ErrGone
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIdentity, err)
	}
	result := &Handle{h, id}
	alive, err := result.Alive()
	if err != nil || !alive {
		result.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrGone
	}
	actual, err := nativeIdentity(h, id.PID)
	if err != nil || actual.CreationTime != id.CreationTime || !strings.EqualFold(filepath.Clean(actual.Executable), filepath.Clean(id.Executable)) {
		result.Close()
		return nil, ErrIdentity
	}
	return result, nil
}
func (h *Handle) Close() error {
	if h.handle == 0 {
		return nil
	}
	err := syscall.CloseHandle(h.handle)
	h.handle = 0
	return err
}
func (h *Handle) Alive() (bool, error) {
	if h.handle == 0 {
		return false, ErrIdentity
	}
	r, err := syscall.WaitForSingleObject(h.handle, 0)
	if err != nil {
		return false, err
	}
	if r == syscall.WAIT_OBJECT_0 {
		return false, nil
	}
	if r == syscall.WAIT_TIMEOUT {
		return true, nil
	}
	return false, fmt.Errorf("unexpected process wait result %d", r)
}
func (h *Handle) Kill() error {
	alive, err := h.Alive()
	if err != nil || !alive {
		return err
	}
	return syscall.TerminateProcess(h.handle, 1)
}
func (h *Handle) Quit(ctx context.Context) error {
	alive, err := h.Alive()
	if err != nil || !alive {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	b, err := json.Marshal(h.identity)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, self, "__engine-quit", base64.RawURLEncoding.EncodeToString(b))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
	if b, err = cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("console quit helper: %w: %s", err, b)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		alive, err = h.Alive()
		if err != nil || !alive {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// QuitHelper runs in a disposable process so console attachment never changes
// the manager's console. The target's original process handle stays open.
func QuitHelper(encoded string) error {
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	var id Identity
	if err = json.Unmarshal(b, &id); err != nil {
		return err
	}
	h, err := Open(id)
	if err != nil {
		return err
	}
	defer h.Close()
	freeConsole.Call()
	ok, _, err := attachConsole.Call(uintptr(id.PID))
	if ok == 0 {
		return fmt.Errorf("attach console: %v", err)
	}
	defer freeConsole.Call()
	var ids [16]uint32
	n, _, err := consoleProcesses.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	if n == 0 || n > uintptr(len(ids)) {
		return fmt.Errorf("cannot verify console ownership: %v", err)
	}
	found := false
	for _, pid := range ids[:n] {
		if int(pid) == id.PID {
			found = true
		} else if int(pid) != os.Getpid() {
			return fmt.Errorf("shared console: refusing quit")
		}
	}
	if !found {
		return ErrIdentity
	}
	name, _ := syscall.UTF16PtrFromString("CONIN$")
	input, err := syscall.CreateFile(name, 0xC0000000, 3, nil, 3, 0, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(input)
	// INPUT_RECORD has a 4-byte header followed by a 16-byte KEY_EVENT_RECORD.
	records := make([]byte, 20*2*len("quit\r"))
	for i, c := range []byte("quit\r") {
		for k := 0; k < 2; k++ {
			r := records[(i*2+k)*20:]
			binary.LittleEndian.PutUint16(r, 1)
			if k == 0 {
				binary.LittleEndian.PutUint32(r[4:], 1)
			}
			binary.LittleEndian.PutUint16(r[8:], 1)
			vk := c
			if c >= 'a' && c <= 'z' {
				vk = c - 32
			}
			binary.LittleEndian.PutUint16(r[10:], uint16(vk))
			binary.LittleEndian.PutUint16(r[14:], uint16(c))
		}
	}
	alive, err := h.Alive()
	if err != nil || !alive {
		return ErrGone
	}
	var written uint32
	ok, _, err = writeConsoleInput.Call(uintptr(input), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)/20), uintptr(unsafe.Pointer(&written)))
	if ok == 0 || int(written) != len(records)/20 {
		return fmt.Errorf("incomplete console input: %v", err)
	}
	return nil
}

func (h *Handle) Bindings() ([]Binding, error) {
	alive, err := h.Alive()
	if err != nil {
		return nil, err
	}
	if !alive {
		return []Binding{}, nil
	}
	var result []Binding
	for _, entry := range []struct {
		proc                           *syscall.LazyProc
		family, class                  uint32
		network                        string
		rowSize, pidOffset, portOffset int
	}{{tcpTable, 2, 3, "tcp4", 24, 20, 8}, {tcpTable, 23, 3, "tcp6", 56, 52, 20}, {udpTable, 2, 1, "udp4", 12, 8, 4}, {udpTable, 23, 1, "udp6", 28, 24, 20}} {
		var size uint32
		entry.proc.Call(0, uintptr(unsafe.Pointer(&size)), 0, uintptr(entry.family), uintptr(entry.class), 0)
		if size < 4 || size > 16<<20 {
			return nil, fmt.Errorf("invalid socket table size %d", size)
		}
		var table []byte
		for attempt := 0; attempt < 3; attempt++ {
			table = make([]byte, size)
			code, _, _ := entry.proc.Call(uintptr(unsafe.Pointer(&table[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(entry.family), uintptr(entry.class), 0)
			if code == 0 {
				break
			}
			if code != 122 || attempt == 2 {
				return nil, fmt.Errorf("socket table error %d", code)
			}
			if size > 16<<20 {
				return nil, fmt.Errorf("socket table too large")
			}
		}
		count := int(binary.LittleEndian.Uint32(table))
		if count > (len(table)-4)/entry.rowSize {
			return nil, fmt.Errorf("truncated socket table")
		}
		for i := 0; i < count; i++ {
			row := table[4+i*entry.rowSize:]
			if int(binary.LittleEndian.Uint32(row[entry.pidOffset:])) != h.identity.PID {
				continue
			}
			offset := 0
			if entry.network == "tcp4" {
				offset = 4
			}
			ipLen := 4
			if entry.family == 23 {
				ipLen = 16
			}
			address := net.IP(row[offset : offset+ipLen]).String()
			port := int(binary.BigEndian.Uint16(row[entry.portOffset:]))
			result = append(result, Binding{entry.network, address, port, h.identity.PID, time.Now().UTC().Format(time.RFC3339Nano)})
		}
	}
	alive, err = h.Alive()
	if err != nil {
		return nil, err
	}
	if !alive {
		return []Binding{}, nil
	}
	return result, nil
}
