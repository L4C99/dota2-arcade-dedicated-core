//go:build linux

package engine

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Linux requires pidfd support; no PID-only signaling fallback is provided.
type Handle struct {
	mu       sync.Mutex
	fd       int
	identity Identity
}

func pidfdOpen(pid int) (int, error) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return -1, fmt.Errorf("pidfd adapter unsupported architecture %s", runtime.GOARCH)
	}
	fd, _, e := syscall.Syscall(434, uintptr(pid), 0, 0)
	if e != 0 {
		if e == syscall.ESRCH {
			return -1, ErrGone
		}
		return -1, fmt.Errorf("pidfd_open required: %w", e)
	}
	return int(fd), nil
}
func pidfdExited(fd int) (bool, error) {
	p := struct {
		FD      int32
		Events  int16
		Revents int16
	}{FD: int32(fd), Events: 1}
	timeout := syscall.Timespec{}
	_, _, e := syscall.Syscall6(syscall.SYS_PPOLL, uintptr(unsafe.Pointer(&p)), 1, uintptr(unsafe.Pointer(&timeout)), 0, 0, 0)
	if e != 0 {
		if e == syscall.EINTR {
			return false, nil
		}
		return false, e
	}
	if p.Revents&32 != 0 {
		return false, os.ErrClosed
	}
	return p.Revents&(1|8|16) != 0, nil
}
func readIdentity(pid int, run string) (Identity, error) {
	id := Identity{PID: pid, RunDirectory: run}
	base := fmt.Sprintf("/proc/%d", pid)
	stat, e := os.ReadFile(base + "/stat")
	if e != nil {
		return id, e
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return id, ErrIdentity
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) < 20 {
		return id, ErrIdentity
	}
	id.StartTicks, e = strconv.ParseUint(fields[19], 10, 64)
	if e != nil {
		return id, e
	}
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return id, e
	}
	id.BootID = strings.TrimSpace(string(boot))
	id.Executable, e = os.Readlink(base + "/exe")
	if e != nil {
		return id, e
	}
	args, e := os.ReadFile(base + "/cmdline")
	if e != nil {
		return id, e
	}
	if len(args) == 0 {
		return id, ErrGone
	}
	id.Arguments = strings.Split(strings.TrimSuffix(string(args), "\x00"), "\x00")
	var input syscall.Stat_t
	if e = syscall.Stat(base+"/fd/0", &input); e != nil {
		return id, e
	}
	if input.Mode&syscall.S_IFMT != syscall.S_IFIFO {
		return id, ErrIdentity
	}
	id.InputDevice = uint64(input.Dev)
	id.InputInode = input.Ino
	return id, nil
}

func Start(s Spec) (result Identity, resultErr error) {
	candidate := Identity{Executable: s.Executable, Arguments: append([]string{s.Executable}, s.Arguments...), RunDirectory: s.RunDirectory}
	if !filepath.IsAbs(s.Executable) || !filepath.IsAbs(s.WorkingDirectory) || !filepath.IsAbs(s.RunDirectory) {
		return candidate, fmt.Errorf("absolute executable, working and run directories required")
	}
	resolved, e := filepath.EvalSymlinks(s.Executable)
	if e != nil {
		return candidate, e
	}
	candidate.Executable = resolved
	fifo := filepath.Join(s.RunDirectory, "stdin.fifo")
	if e = syscall.Mkfifo(fifo, 0600); e != nil {
		return candidate, e
	}
	var createdInput syscall.Stat_t
	if e = syscall.Lstat(fifo, &createdInput); e != nil {
		return candidate, e
	}
	if createdInput.Mode&syscall.S_IFMT != syscall.S_IFIFO {
		return candidate, ErrIdentity
	}
	defer func() {
		if candidate.PID == 0 {
			if cleanupErr := removeInput(fifo, uint64(createdInput.Dev), createdInput.Ino); cleanupErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("unstarted FIFO cleanup: %w", cleanupErr))
			}
		}
	}()
	// O_EXCL above ensures an existing endpoint is never adopted. NOFOLLOW and
	// fstat below defend against replacement before descriptors are inherited.
	fd, e := syscall.Open(fifo, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return candidate, e
	}
	input := os.NewFile(uintptr(fd), fifo)
	defer input.Close()
	st, e := input.Stat()
	if e != nil || st.Mode()&os.ModeNamedPipe == 0 {
		return candidate, fmt.Errorf("stdin is not a FIFO")
	}
	inputStat := st.Sys().(*syscall.Stat_t)
	if inputStat.Dev != createdInput.Dev || inputStat.Ino != createdInput.Ino {
		return candidate, ErrIdentity
	}
	candidate.InputDevice = uint64(inputStat.Dev)
	candidate.InputInode = inputStat.Ino
	output, e := os.OpenFile(filepath.Join(s.RunDirectory, "output.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return candidate, e
	}
	defer output.Close()
	cmd := exec.Command(s.Executable, s.Arguments...)
	cmd.Dir = s.WorkingDirectory
	cmd.Stdin = input
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return candidate, e
	}
	candidate.PID = cmd.Process.Pid
	go func() { _ = cmd.Wait() }() // Reap while this manager lives; never kill on exit.
	pfd, e := pidfdOpen(candidate.PID)
	if e != nil {
		return candidate, e
	}
	defer syscall.Close(pfd)
	actual, e := readIdentity(candidate.PID, s.RunDirectory)
	if e != nil {
		return candidate, e
	}
	if actual.Executable != candidate.Executable || !reflect.DeepEqual(actual.Arguments, candidate.Arguments) || actual.InputDevice != candidate.InputDevice || actual.InputInode != candidate.InputInode {
		return candidate, ErrIdentity
	}
	exited, e := pidfdExited(pfd)
	if e != nil {
		return candidate, e
	}
	if exited {
		return actual, ErrGone
	}
	return actual, nil
}

func Open(id Identity) (*Handle, error) {
	if id.PID <= 0 || id.StartTicks == 0 || id.BootID == "" || !filepath.IsAbs(id.Executable) || !filepath.IsAbs(id.RunDirectory) || len(id.Arguments) == 0 {
		return nil, ErrIdentity
	}
	fd, e := pidfdOpen(id.PID)
	if e != nil {
		return nil, e
	}
	h := &Handle{fd: fd, identity: id}
	h.identity.Arguments = append([]string(nil), id.Arguments...)
	alive, e := h.alive()
	if e != nil || !alive {
		syscall.Close(fd)
		if e != nil {
			return nil, e
		}
		return nil, ErrGone
	}
	return h, nil
}
func (h *Handle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fd < 0 {
		return nil
	}
	e := syscall.Close(h.fd)
	h.fd = -1
	return e
}
func (h *Handle) alive() (bool, error) {
	if h.fd < 0 {
		return false, os.ErrClosed
	}
	gone, e := pidfdExited(h.fd)
	if e != nil {
		return false, e
	}
	if gone {
		return false, nil
	}
	actual, e := readIdentity(h.identity.PID, h.identity.RunDirectory)
	if e != nil {
		// /proc/exe and fd entries can disappear before the same held pidfd
		// becomes readable during exit. Wait only for that pidfd, bounded;
		// permission errors or a real identity mismatch are never softened.
		deadline := time.Now()
		if errors.Is(e, os.ErrNotExist) || errors.Is(e, ErrGone) {
			deadline = deadline.Add(100 * time.Millisecond)
		}
		for {
			gone, pollErr := pidfdExited(h.fd)
			if pollErr != nil {
				return false, pollErr
			}
			if gone {
				return false, nil
			}
			if !time.Now().Before(deadline) {
				break
			}
			time.Sleep(time.Millisecond)
		}
		return false, fmt.Errorf("%w: %v", ErrIdentity, e)
	}
	if !reflect.DeepEqual(actual, h.identity) {
		return false, ErrIdentity
	}
	return true, nil
}
func (h *Handle) Alive() (bool, error) { h.mu.Lock(); defer h.mu.Unlock(); return h.alive() }
func (h *Handle) Kill() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	alive, e := h.alive()
	if e != nil {
		return e
	}
	if !alive {
		return nil
	}
	_, _, errno := syscall.Syscall6(424, uintptr(h.fd), uintptr(syscall.SIGKILL), 0, 0, 0, 0)
	if errno != 0 {
		if errno == syscall.ESRCH {
			return nil
		}
		return errno
	}
	return nil
}
func (h *Handle) Quit(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	h.mu.Lock()
	alive, e := h.alive()
	if e != nil || !alive {
		h.mu.Unlock()
		if e != nil {
			return e
		}
		return nil
	}
	fifo := filepath.Join(h.identity.RunDirectory, "stdin.fifo")
	fd, e := syscall.Open(fifo, syscall.O_WRONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e == nil {
		var a, b syscall.Stat_t
		e = syscall.Fstat(fd, &a)
		if e == nil {
			e = syscall.Stat(fmt.Sprintf("/proc/%d/fd/0", h.identity.PID), &b)
		}
		if e == nil && (a.Mode&syscall.S_IFMT != syscall.S_IFIFO || a.Dev != b.Dev || a.Ino != b.Ino || a.Mode&0777 != 0600 || a.Uid != uint32(os.Geteuid())) {
			e = ErrIdentity
		}
		if e == nil {
			alive, e = h.alive()
			if e == nil && !alive {
				e = ErrGone
			}
		}
		if e == nil {
			var n int
			n, e = syscall.Write(fd, []byte("quit\n"))
			if e == nil && n != 5 {
				e = fmt.Errorf("incomplete quit write")
			}
		}
		syscall.Close(fd)
	}
	h.mu.Unlock()
	if errors.Is(e, ErrGone) {
		return nil
	}
	if e != nil {
		return e
	}
	timer := time.NewTicker(20 * time.Millisecond)
	defer timer.Stop()
	for {
		alive, e := h.Alive()
		if e != nil {
			return e
		}
		if !alive {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// CleanupInput only removes the captured FIFO after exit has been established.
// A missing or reused process identity is never treated as permission to signal.
func CleanupInput(id Identity) error {
	if id.InputInode == 0 || !filepath.IsAbs(id.RunDirectory) {
		return ErrIdentity
	}
	h, e := Open(id)
	if e == nil {
		alive, checkErr := h.Alive()
		h.Close()
		if checkErr != nil {
			return checkErr
		}
		if alive {
			return fmt.Errorf("process exit not confirmed")
		}
	} else if !errors.Is(e, ErrGone) {
		return e
	}
	return removeInput(filepath.Join(id.RunDirectory, "stdin.fifo"), id.InputDevice, id.InputInode)
}

func removeInput(path string, device, inode uint64) error {
	var st syscall.Stat_t
	if e := syscall.Lstat(path, &st); e != nil {
		if errors.Is(e, syscall.ENOENT) {
			return nil
		}
		return e
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFIFO || uint64(st.Dev) != device || st.Ino != inode {
		return ErrIdentity
	}
	return os.Remove(path)
}

func parseAddress(raw string) (string, int, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid socket address")
	}
	port, e := strconv.ParseUint(parts[1], 16, 16)
	if e != nil {
		return "", 0, e
	}
	b, e := hex.DecodeString(parts[0])
	if e != nil || (len(b) != 4 && len(b) != 16) {
		return "", 0, fmt.Errorf("invalid socket IP")
	}
	for i := 0; i < len(b); i += 4 {
		word := binary.BigEndian.Uint32(b[i : i+4])
		binary.NativeEndian.PutUint32(b[i:i+4], word)
	}
	return net.IP(b).String(), int(port), nil
}
func (h *Handle) Bindings() ([]Binding, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	alive, e := h.alive()
	if e != nil {
		return nil, e
	}
	if !alive {
		return nil, ErrGone
	}
	base := fmt.Sprintf("/proc/%d", h.identity.PID)
	entries, e := os.ReadDir(base + "/fd")
	if e != nil {
		return nil, e
	}
	inodes := map[string]bool{}
	for _, entry := range entries {
		target, e := os.Readlink(base + "/fd/" + entry.Name())
		if e != nil {
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			return nil, e
		}
		if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
			inodes[target[8:len(target)-1]] = true
		}
	}
	out := []Binding{}
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	for _, name := range []string{"tcp", "tcp6", "udp", "udp6"} {
		f, e := os.Open(base + "/net/" + name)
		if e != nil {
			if errors.Is(e, os.ErrNotExist) && (name == "tcp6" || name == "udp6") {
				continue
			}
			return nil, e
		}
		scanner := bufio.NewScanner(f)
		scanner.Scan()
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}
			if !inodes[fields[9]] {
				continue
			}
			if strings.HasPrefix(name, "tcp") && fields[3] != "0A" {
				continue
			}
			addr, port, e := parseAddress(fields[1])
			if e != nil {
				f.Close()
				return nil, e
			}
			if port == 0 {
				continue
			}
			protocol := name
			if name == "tcp" || name == "udp" {
				protocol += "4"
			}
			out = append(out, Binding{Protocol: protocol, Address: addr, Port: port, PID: h.identity.PID, ObservedAt: observed})
		}
		e = scanner.Err()
		f.Close()
		if e != nil {
			return nil, e
		}
	}
	alive, e = h.alive()
	if e != nil {
		return nil, e
	}
	if !alive {
		return nil, ErrGone
	}
	return out, nil
}
