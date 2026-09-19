package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "__engine-quit" {
		if err := engine.QuitHelper(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestManagedChild(t *testing.T) {
	values := map[string]string{}
	for i, a := range os.Args {
		if (a == "-port" || a == "-con_logfile" || a == "+exec") && i+1 < len(os.Args) {
			values[a] = os.Args[i+1]
		}
	}
	if values["-port"] == "" {
		return
	}
	go func() { time.Sleep(15 * time.Second); os.Exit(40) }()
	mode, err := os.ReadFile(values["+exec"])
	if err != nil {
		os.Exit(41)
	}
	if strings.Contains(string(mode), "early") {
		os.Exit(19)
	}
	port, _ := strconv.Atoi(values["-port"])
	tcp, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		os.Exit(42)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		os.Exit(43)
	}
	defer udp.Close()
	if !strings.Contains(string(mode), "never") {
		if err = os.WriteFile(values["-con_logfile"], []byte("loaded\nready 中文玩家\n"), 0600); err != nil {
			os.Exit(44)
		}
	}
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if scanner.Text() == "quit" {
				close(done)
				return
			}
		}
	}()
	if strings.Contains(string(mode), "exit-later") {
		time.Sleep(900 * time.Millisecond)
		return
	}
	select {
	case <-done:
	case <-time.After(12 * time.Second):
	}
}

func setupManager(t *testing.T, mode string) (*Manager, string, int) {
	t.Helper()
	dir := t.TempDir()
	exe, _ := os.Executable()
	template := config.Template{SchemaVersion: 1, Name: mode, Executable: exe, WorkingDirectory: dir, Arguments: []string{"-test.run=^TestManagedChild$", "--", "-port", "{{game_port}}", "-con_logfile", "{{log_path}}", "+exec", "{{cfg_name}}"}, CFG: config.CFGConfig{Directory: dir, Lines: []string{"mode " + mode}}, Readiness: config.Readiness{SuccessAll: []string{"loaded", "ready"}}, Timeouts: config.Timeouts{StartupSeconds: 2, StopSeconds: 1, ForceSeconds: 1}}
	b, _ := json.Marshal(template)
	path := filepath.Join(dir, "template.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		m.Close()
		for _, in := range m.store.State.Instances {
			for _, r := range in.Runs {
				if r.Identity != nil {
					_, _ = engine.Stop(*r.Identity, 100*time.Millisecond, time.Second)
				}
			}
		}
	})
	for n := 0; n < 20; n++ {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if records.CheckPort(port) == nil {
			return m, path, port
		}
	}
	t.Fatal("no test port")
	return nil, "", 0
}
func call(t *testing.T, m *Manager, method string, params any) response {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"protocolVersion": 1, "method": method, "params": params})
	var r response
	if err := json.Unmarshal(m.Respond(b), &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func success(t *testing.T, r response) map[string]any {
	t.Helper()
	if !r.OK {
		t.Fatal(r.Error)
	}
	return r.Result.(map[string]any)
}
func createRoom(t *testing.T, m *Manager, path string, port int) (string, string) {
	r := success(t, call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "test-request"}))
	return r["instanceId"].(string), r["operationId"].(string)
}
func waitOperation(t *testing.T, m *Manager, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		r := success(t, call(t, m, "operation", map[string]any{"operationId": id}))
		if r["status"] != "running" {
			return r
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("operation did not terminate", id)
	return nil
}
func stopRoom(t *testing.T, m *Manager, id string) map[string]any {
	t.Helper()
	r := success(t, call(t, m, "stop", map[string]any{"instanceId": id}))
	return waitOperation(t, m, r["operationId"].(string))
}

func TestLifecycleSnapshotRestartStopAndRetry(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if err := os.WriteFile(path, []byte("now invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	retry := success(t, call(t, m, "create", map[string]any{"template": path, "port": port, "idempotencyKey": "test-request"}))
	if retry["instanceId"] != id || retry["operationId"] != op {
		t.Fatal(retry)
	}
	r := success(t, call(t, m, "restart", map[string]any{"instanceId": id}))
	if result := waitOperation(t, m, r["operationId"].(string)); result["status"] != "succeeded" {
		t.Fatal(result)
	}
	status := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if status["generation"] != float64(2) || status["room"] != "ready" || status["port"] != float64(port) {
		t.Fatal(status)
	}
	logs := success(t, call(t, m, "logs", map[string]any{"instanceId": id, "tail": 2}))
	if !strings.Contains(logs["text"].(string), "中文玩家") {
		t.Fatal(logs)
	}
	stopped := stopRoom(t, m, id)
	if stopped["status"] != "succeeded" {
		t.Fatal(stopped)
	}
	again := success(t, call(t, m, "stop", map[string]any{"instanceId": id}))
	if again["operationId"] != stopped["operationId"] {
		t.Fatal("duplicate stop changed operation")
	}
	if r := call(t, m, "restart", map[string]any{"instanceId": id}); r.OK || r.Error.Code != "RECLAIMED" {
		t.Fatal(r)
	}
	list := success(t, call(t, m, "list", map[string]any{}))
	if len(list["instances"].([]any)) != 0 {
		t.Fatal(list)
	}
	if err := records.CheckPort(port); err != nil {
		t.Fatal(err)
	}
	if r := call(t, m, "status", map[string]any{"instanceId": "missing"}); r.OK || r.Error.Code != "NOT_FOUND" {
		t.Fatal(r)
	}
}

func TestEarlyExitTimeoutAndCancel(t *testing.T) {
	for _, mode := range []string{"early", "never", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			actual := mode
			if mode == "cancel" {
				actual = "never"
			}
			m, path, port := setupManager(t, actual)
			id, op := createRoom(t, m, path, port)
			if mode == "cancel" {
				if r := stopRoom(t, m, id); r["status"] != "succeeded" {
					t.Fatal(r)
				}
				if r := waitOperation(t, m, op); r["status"] != "cancelled" {
					t.Fatal(r)
				}
				return
			}
			r := waitOperation(t, m, op)
			if r["status"] != "failed" {
				t.Fatal(r)
			}
			if mode == "never" && r["error"].(map[string]any)["code"] != "START_TIMEOUT" {
				t.Fatal(r)
			}
			if r = stopRoom(t, m, id); r["status"] != "succeeded" {
				t.Fatal(r)
			}
		})
	}
}

func TestCleanupRetryDoesNotSpawn(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	run := m.store.State.Instances[id].Runs[0]
	cfg := run.CFGPath
	m.mu.Unlock()
	original, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(cfg, []byte("foreign replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	r := stopRoom(t, m, id)
	if r["status"] != "failed" || r["error"].(map[string]any)["code"] != "CLEANUP_FAILED" {
		t.Fatal(r)
	}
	status := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if status["process"] != "stopped" || status["cleanup"] != "failed" {
		t.Fatal(status)
	}
	if err = os.WriteFile(cfg, original, 0600); err != nil {
		t.Fatal(err)
	}
	r = stopRoom(t, m, id)
	if r["status"] != "succeeded" {
		t.Fatal(r)
	}
	status = success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if status["generation"] != float64(1) || status["lifecycle"] != "reclaimed" {
		t.Fatal(status)
	}
}

func TestHistoricalSuccessDoesNotHideExit(t *testing.T) {
	m, path, port := setupManager(t, "exit-later")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
		if r["process"] == "stopped" {
			if r["lifecycle"] != "failed" || r["room"] == "ready" {
				t.Fatal(r)
			}
			if r := waitOperation(t, m, op); r["status"] != "succeeded" {
				t.Fatal(r)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("exited process remained ready")
}

func TestPersistenceFailureNotReportedAsSuccess(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	original := m.persist
	m.persist = func() error {
		if m.store.State.Instances[id].Lifecycle == "reclaimed" {
			return errors.New("injected save failure")
		}
		return original()
	}
	m.mu.Unlock()
	r := stopRoom(t, m, id)
	if r["status"] != "failed" || r["error"].(map[string]any)["stage"] != "persist" {
		t.Fatal(r)
	}
	status := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if status["lifecycle"] == "reclaimed" || status["error"] == nil {
		t.Fatal(status)
	}
}
