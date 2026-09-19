//go:build linux

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The subprocess helper is bounded even if its controlling test is interrupted.
func TestLinuxHelper(t *testing.T) {
	args := os.Args
	idx := -1
	for i, a := range args {
		if a == "--engine-helper" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	mode := args[idx+1]
	if mode == "launcher" {
		run := args[idx+2]
		exe, _ := os.Executable()
		id, e := Start(Spec{Executable: exe, WorkingDirectory: run, RunDirectory: run, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", "normal"}})
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(3)
		}
		b, _ := json.Marshal(id)
		if os.WriteFile(filepath.Join(run, "identity.json"), b, 0600) != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
	tcp, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		os.Exit(5)
	}
	defer tcp.Close()
	udp, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		os.Exit(6)
	}
	defer udp.Close()
	fmt.Printf("sockets %s %s\n", tcp.Addr(), udp.LocalAddr())
	quit := make(chan struct{})
	if mode != "ignore" {
		go func() {
			s := bufio.NewScanner(os.Stdin)
			for s.Scan() {
				if s.Text() == "quit" {
					close(quit)
					return
				}
			}
		}()
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-quit:
			os.Exit(0)
		case <-deadline.C:
			os.Exit(0)
		case <-ticker.C:
			fmt.Println("heartbeat")
		}
	}
}
func startLinuxHelper(t *testing.T, mode string) (Identity, *Handle) {
	t.Helper()
	dir := t.TempDir()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	id, e := Start(Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", mode}})
	if e != nil {
		t.Fatalf("Start %+v: %v", id, e)
	}
	h, e := Open(id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = h.Kill(); _ = h.Close() })
	return id, h
}
func TestLinuxLifecycleAndIdentity(t *testing.T) {
	id, h := startLinuxHelper(t, "normal")
	if id.PID <= 0 || id.BootID == "" || id.StartTicks == 0 {
		t.Fatal(id)
	}
	wrong := id
	wrong.StartTicks++
	if bad, e := Open(wrong); !errors.Is(e, ErrIdentity) {
		if bad != nil {
			bad.Close()
		}
		t.Fatalf("identity mismatch accepted: %v", e)
	}
	var bindings []Binding
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var e error
		bindings, e = h.Bindings()
		if e != nil {
			t.Fatal(e)
		}
		if len(bindings) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	tcp, udp := false, false
	for _, b := range bindings {
		if b.PID != id.PID || b.Address != "127.0.0.1" || b.Port <= 0 || b.ObservedAt == "" {
			t.Fatal(b)
		}
		tcp = tcp || b.Protocol == "tcp4" || b.Protocol == "tcp6"
		udp = udp || b.Protocol == "udp4" || b.Protocol == "udp6"
	}
	if !tcp || !udp {
		t.Fatalf("missing owned listeners: %+v", bindings)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if e := h.Quit(ctx); e != nil {
		t.Fatal(e)
	}
	if alive, e := h.Alive(); alive || e != nil {
		t.Fatalf("not exited: %v %v", alive, e)
	}
	if e := h.Kill(); e != nil {
		t.Fatal("kill after exit", e)
	}
	if e := h.Quit(ctx); e != nil {
		t.Fatal("quit after exit", e)
	}
	if e := CleanupInput(id); e != nil {
		t.Fatal("cleanup input", e)
	}
	if e := CleanupInput(id); e != nil {
		t.Fatal("repeat cleanup", e)
	}
}
func TestLinuxQuitTimeoutAndKill(t *testing.T) {
	_, h := startLinuxHelper(t, "ignore")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if e := h.Quit(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", e)
	}
	if alive, e := h.Alive(); !alive || e != nil {
		t.Fatal("timeout implicitly killed", alive, e)
	}
	if e := h.Kill(); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		alive, e := h.Alive()
		if e != nil {
			t.Fatal(e)
		}
		if !alive {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pidfd kill exit unconfirmed")
}
func TestLinuxManagerExitLeavesLogs(t *testing.T) {
	dir := t.TempDir()
	exe, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestLinuxHelper$", "--", "--engine-helper", "launcher", dir)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("launcher: %v %s", e, out)
	}
	b, e := os.ReadFile(filepath.Join(dir, "identity.json"))
	if e != nil {
		t.Fatal(e)
	}
	var id Identity
	if e = json.Unmarshal(b, &id); e != nil {
		t.Fatal(e)
	}
	h, e := Open(id)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	defer h.Kill()
	before, e := os.Stat(filepath.Join(dir, "output.log"))
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(180 * time.Millisecond)
	after, e := os.Stat(filepath.Join(dir, "output.log"))
	if e != nil || after.Size() <= before.Size() {
		t.Fatalf("offline log not growing: %v", e)
	}
	quitCtx, quitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer quitCancel()
	if e := h.Quit(quitCtx); e != nil {
		t.Fatal(e)
	}
}
func TestLinuxQuitRejectsReplacedFIFO(t *testing.T) {
	id, h := startLinuxHelper(t, "ignore")
	fifo := filepath.Join(id.RunDirectory, "stdin.fifo")
	if e := os.Rename(fifo, fifo+".saved"); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(fifo, []byte("unrelated"), 0600); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := h.Quit(ctx); e == nil {
		t.Fatal("accepted replaced FIFO")
	}
	b, _ := os.ReadFile(fifo)
	if string(b) != "unrelated" {
		t.Fatal("wrote into unrelated file")
	}
	if e := CleanupInput(id); e == nil {
		t.Fatal("cleanup live process accepted")
	}
	if e := h.Kill(); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		alive, e := h.Alive()
		if e != nil {
			t.Fatal(e)
		}
		if !alive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e := CleanupInput(id); !errors.Is(e, ErrIdentity) {
		t.Fatalf("replaced input cleanup: %v", e)
	}
}
func TestLinuxAddressParsing(t *testing.T) {
	for raw, want := range map[string]string{"0100007F:6987": "127.0.0.1", "00000000000000000000000001000000:6987": "::1"} {
		addr, port, e := parseAddress(raw)
		if e != nil || addr != want || port != 27015 {
			t.Fatal(raw, addr, port, e)
		}
	}
	if _, _, e := parseAddress("bad"); e == nil {
		t.Fatal("accepted bad address")
	}
}

func TestLinuxUnstartedFIFOCleanup(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"output-conflict", "bad-working-directory"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			spec := Spec{Executable: exe, WorkingDirectory: dir, RunDirectory: dir}
			if mode == "output-conflict" {
				if e := os.WriteFile(filepath.Join(dir, "output.log"), []byte("keep"), 0600); e != nil {
					t.Fatal(e)
				}
			} else {
				spec.WorkingDirectory = filepath.Join(dir, "missing")
			}
			id, e := Start(spec)
			if e == nil || id.PID != 0 {
				t.Fatalf("unexpected start %+v %v", id, e)
			}
			if _, e := os.Lstat(filepath.Join(dir, "stdin.fifo")); !os.IsNotExist(e) {
				t.Fatalf("unstarted FIFO retained: %v", e)
			}
			if mode == "output-conflict" {
				b, _ := os.ReadFile(filepath.Join(dir, "output.log"))
				if string(b) != "keep" {
					t.Fatal("unrelated output changed")
				}
			}
		})
	}
}
