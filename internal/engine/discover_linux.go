//go:build linux

package engine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Discover requires an explicit conservative wall-clock lower bound. Linux
// start ticks are quantized; an overlapping time interval is unknown, never a
// guessed match. Exact argv and this generation's FIFO are also mandatory.
func Discover(spec Spec, notBefore time.Time) ([]Identity, error) {
	if !filepath.IsAbs(spec.Executable) || !filepath.IsAbs(spec.WorkingDirectory) || !filepath.IsAbs(spec.RunDirectory) || notBefore.IsZero() {
		return nil, ErrIdentity
	}
	exe, err := filepath.EvalSymlinks(spec.Executable)
	if err != nil {
		return nil, err
	}
	hz, err := clockTickRate()
	if err != nil {
		return nil, err
	}
	before := time.Now()
	var boot unix.Timespec
	if err = unix.ClockGettime(unix.CLOCK_BOOTTIME, &boot); err != nil {
		return nil, err
	}
	after := time.Now()
	bootLow := before.Add(-time.Duration(boot.Nano()))
	bootHigh := after.Add(-time.Duration(boot.Nano()))
	f, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(65537)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 65536 {
		return nil, fmt.Errorf("process enumeration exceeds bound")
	}
	expected := append([]string{spec.Executable}, spec.Arguments...)
	var fifo syscall.Stat_t
	fifoErr := syscall.Lstat(filepath.Join(spec.RunDirectory, "stdin.fifo"), &fifo)
	if fifoErr != nil && !errors.Is(fifoErr, syscall.ENOENT) {
		return nil, fifoErr
	}
	if fifoErr == nil && (fifo.Mode&syscall.S_IFMT != syscall.S_IFIFO || fifo.Mode&0777 != 0600 || fifo.Uid != uint32(os.Geteuid())) {
		return nil, ErrIdentity
	}
	out := []Identity{}
	deadline := time.Now().Add(5 * time.Second)
	for _, entry := range entries {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("process discovery exceeded time bound")
		}
		pid, e := strconv.Atoi(entry.Name())
		if e != nil || pid <= 0 {
			continue
		}
		base := "/proc/" + entry.Name()
		var st syscall.Stat_t
		if e = syscall.Stat(base, &st); errors.Is(e, syscall.ENOENT) {
			continue
		} else if e != nil {
			return nil, e
		}
		if st.Uid != uint32(os.Geteuid()) {
			continue
		}
		target, e := os.Readlink(base + "/exe")
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return nil, fmt.Errorf("cannot inspect same-user PID %d: %w", pid, e)
		}
		if target != exe {
			continue
		}
		raw, e := readDiscoveryFile(base+"/cmdline", 256<<10)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if len(raw) == 0 {
			continue
		}
		if !reflect.DeepEqual(strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00"), expected) {
			continue
		}
		if fifoErr != nil {
			return nil, fmt.Errorf("matching process has missing generation FIFO: %w", ErrIdentity)
		}
		pfd, e := pidfdOpen(pid)
		if errors.Is(e, ErrGone) {
			continue
		}
		if e != nil {
			return nil, e
		}
		id, e := readIdentity(pid, spec.RunDirectory)
		gone, pollErr := pidfdExited(pfd)
		syscall.Close(pfd)
		if pollErr != nil {
			return nil, pollErr
		}
		if gone {
			continue
		}
		if e != nil {
			return nil, fmt.Errorf("matching PID %d: %w", pid, e)
		}
		if id.Executable != exe || !reflect.DeepEqual(id.Arguments, expected) {
			return nil, ErrIdentity
		}
		if id.InputDevice != uint64(fifo.Dev) || id.InputInode != fifo.Ino {
			return nil, fmt.Errorf("matching argv with different stdin ownership: %w", ErrIdentity)
		}
		if id.StartTicks > uint64((1<<63-1)/int64(time.Second)) {
			return nil, ErrIdentity
		}
		lower := bootLow.Add(time.Duration(id.StartTicks) * time.Second / time.Duration(hz))
		upper := bootHigh.Add(time.Duration(id.StartTicks+1) * time.Second / time.Duration(hz))
		if upper.Before(notBefore) {
			continue
		}
		if lower.Before(notBefore) {
			return nil, fmt.Errorf("process start-time interval overlaps intent lower bound: %w", ErrIdentity)
		}
		verified, e := Open(id)
		if errors.Is(e, ErrGone) {
			continue
		}
		if e != nil {
			return nil, e
		}
		alive, e := verified.Alive()
		verified.Close()
		if e != nil {
			return nil, e
		}
		if !alive {
			continue
		}
		out = append(out, id)
		if len(out) > 64 {
			return nil, fmt.Errorf("too many matching processes")
		}
	}
	return out, nil
}
func readDiscoveryFile(path string, limit int64) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("process metadata exceeds bound")
	}
	return b, nil
}
func clockTickRate() (uint64, error) {
	b, e := readDiscoveryFile("/proc/self/auxv", 64<<10)
	if e != nil {
		return 0, e
	}
	// Supported Linux architectures are 64-bit; AT_CLKTCK avoids assuming HZ=100.
	if strconv.IntSize != 64 || len(b)%16 != 0 {
		return 0, ErrIdentity
	}
	for len(b) >= 16 {
		tag, value := binary.NativeEndian.Uint64(b), binary.NativeEndian.Uint64(b[8:])
		if tag == 17 {
			if value == 0 || value > 1e9 {
				return 0, ErrIdentity
			}
			return value, nil
		}
		b = b[16:]
	}
	return 0, fmt.Errorf("AT_CLKTCK unavailable")
}
