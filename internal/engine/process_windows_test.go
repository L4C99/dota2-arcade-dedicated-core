package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "__engine-quit" {
		if err := QuitHelper(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestEngineChild(t *testing.T) {
	if os.Getenv("D2CORE_ENGINE_CHILD") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if strings.HasPrefix(mode, "parent-") {
		exe, _ := os.Executable()
		dir := os.Getenv("D2CORE_ENGINE_RUN")
		id, err := Start(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestEngineChild$", "--", "heartbeat"}})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(41)
		}
		b, _ := json.Marshal(id)
		if err = os.WriteFile(filepath.Join(dir, "identity.json"), b, 0600); err != nil {
			os.Exit(42)
		}
		if mode == "parent-crash" {
			os.Exit(23)
		}
		return
	}
	go func() { time.Sleep(12 * time.Second); os.Exit(31) }()
	fmt.Fprintln(os.Stdout, "child started 中文")
	if mode == "heartbeat" {
		for n := 0; n < 50; n++ {
			fmt.Fprintln(os.Stdout, "heartbeat 中文")
			time.Sleep(100 * time.Millisecond)
		}
		return
	}
	if mode == "bindings" {
		tcp, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			os.Exit(35)
		}
		defer tcp.Close()
		udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			os.Exit(36)
		}
		defer udp.Close()
		fmt.Fprintln(os.Stdout, "bindings ready")
	}
	if mode == "console" {
		var attached [16]uint32
		count, _, _ := consoleProcesses.Call(uintptr(unsafe.Pointer(&attached[0])), uintptr(len(attached)))
		if count == 0 {
			ok, _, err := kernel.NewProc("AllocConsole").Call()
			if ok == 0 {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(32)
			}
		}
		// Match the engine's use of inherited standard input. Opening CONIN$
		// here hid the NUL handle that os/exec supplies for a nil Stdin.
		h, err := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
		if err != nil {
			os.Exit(33)
		}
		var mode uint32
		if err := syscall.GetConsoleMode(h, &mode); err != nil {
			fmt.Fprintln(os.Stderr, "standard input is not a console:", err)
			os.Exit(33)
		}
		f := os.NewFile(uintptr(h), "console input")
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "quit" {
				fmt.Fprintln(os.Stdout, "graceful quit received")
				return
			}
		}
		os.Exit(34)
	}
	if mode == "early" {
		os.Exit(17)
	}
	time.Sleep(10 * time.Second)
}

func TestWindowsBindingsOwnedByChild(t *testing.T) {
	id, h := startChild(t, "bindings")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		bindings, err := h.Bindings()
		if err != nil {
			t.Fatal(err)
		}
		tcp, udp := false, false
		for _, b := range bindings {
			if b.PID != id.PID || b.Port <= 0 {
				t.Fatal(b)
			}
			if b.Protocol == "tcp4" && b.Address == "127.0.0.1" {
				tcp = true
			}
			if b.Protocol == "udp4" && b.Address == "127.0.0.1" {
				udp = true
			}
		}
		if tcp && udp {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("child TCP/UDP bindings not observed")
}

func TestWindowsManagerExitPreservesChildAndLogs(t *testing.T) {
	for _, mode := range []string{"parent-normal", "parent-crash"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("D2CORE_ENGINE_CHILD", "1")
			dir := t.TempDir()
			t.Setenv("D2CORE_ENGINE_RUN", dir)
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "-test.run=^TestEngineChild$", "--", mode)
			output, err := cmd.CombinedOutput()
			if mode == "parent-normal" && err != nil {
				t.Fatalf("%v %s", err, output)
			}
			if mode == "parent-crash" && (cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 23) {
				t.Fatalf("unexpected parent exit: %v %s", err, output)
			}
			b, err := os.ReadFile(filepath.Join(dir, "identity.json"))
			if err != nil {
				t.Fatal(err)
			}
			var id Identity
			if err = json.Unmarshal(b, &id); err != nil {
				t.Fatal(err)
			}
			h, err := Open(id)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			defer h.Kill()
			path := filepath.Join(dir, "output.log")
			first, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				next, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if next.Size() > first.Size() {
					return
				}
				time.Sleep(30 * time.Millisecond)
			}
			t.Fatal("logs stopped after manager exit")
		})
	}
}

func startChild(t *testing.T, mode string) (Identity, *Handle) {
	t.Helper()
	t.Setenv("D2CORE_ENGINE_CHILD", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	id, err := Start(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestEngineChild$", "--", mode}})
	if err != nil {
		t.Fatal(err)
	}
	h, err := Open(id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = h.Kill()
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			alive, _ := h.Alive()
			if !alive {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		h.Close()
	})
	return id, h
}

func TestIdentityAndKill(t *testing.T) {
	id, h := startChild(t, "wait")
	bad := id
	bad.CreationTime++
	if other, err := Open(bad); err == nil {
		other.Close()
		t.Fatal("accepted changed creation time")
	}
	bad = id
	bad.Executable = filepath.Join(filepath.Dir(id.Executable), "wrong.exe")
	if other, err := Open(bad); err == nil {
		other.Close()
		t.Fatal("accepted wrong executable")
	}
	if alive, err := h.Alive(); err != nil || !alive {
		t.Fatal("mismatch test harmed process", err)
	}
	if _, err := h.Bindings(); err != nil {
		t.Fatal(err)
	}
	if err := h.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		alive, err := h.Alive()
		if err != nil {
			t.Fatal(err)
		}
		if !alive {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("kill did not confirm exit")
}

func TestConsoleQuit(t *testing.T) {
	id, h := startChild(t, "console")
	// Readiness here is the child-owned console, not an arbitrary sleep. Failed
	// attachment is retried before the deadline; no input is sent to other PIDs.
	deadline := time.Now().Add(5 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = h.Quit(ctx)
		cancel()
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(id.RunDirectory, "output.log"))
	if err != nil || !strings.Contains(string(b), "中文") {
		t.Fatalf("direct output missing: %q %v", b, err)
	}
	if !strings.Contains(string(b), "graceful quit received") {
		t.Fatalf("child exited without receiving quit through standard input: %s", b)
	}
}
