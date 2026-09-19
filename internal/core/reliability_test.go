package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReliabilityIdentitySaveFailure(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	m.mu.Lock()
	original := m.persist
	m.persist = func() error {
		for _, in := range m.store.State.Instances {
			if len(in.Runs) > 0 && in.Runs[0].Identity != nil {
				return errors.New("injected identity persistence failure")
			}
		}
		return original()
	}
	m.mu.Unlock()
	id, op := createRoom(t, m, path, port)
	result := waitOperation(t, m, op)
	if result["status"] != "failed" || result["error"].(map[string]any)["stage"] != "persist" {
		t.Fatal(result)
	}
	status := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if status["lifecycle"] != "failed" || status["process"] != "running" {
		t.Fatal(status)
	}
	if r := call(t, m, "restart", map[string]any{"instanceId": id}); r.OK || r.Error.Code != "IO_ERROR" {
		t.Fatal(r)
	}
}

func TestReliabilityStoppedSaveFailure(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	original := m.persist
	m.persist = func() error {
		in := m.store.State.Instances[id]
		if in.Runs[0].StopResult != nil && in.Runs[0].StopResult.Confirmed {
			return errors.New("injected exit persistence failure")
		}
		return original()
	}
	m.mu.Unlock()
	result := stopRoom(t, m, id)
	if result["status"] != "failed" || result["error"].(map[string]any)["stage"] != "persist" {
		t.Fatal(result)
	}
	status := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if status["lifecycle"] == "reclaimed" || status["process"] != "stopped" || status["cleanup"] == "complete" {
		t.Fatal(status)
	}
}

func TestReliabilityStopLoadingNoNewGeneration(t *testing.T) {
	m, path, port := setupManager(t, "never")
	id, op := createRoom(t, m, path, port)
	deadline := time.Now().Add(time.Second)
	found := false
	for time.Now().Before(deadline) {
		r := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
		if r["process"] == "running" && r["room"] == "loading" {
			found = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatal("test did not reach a running loading process")
	}
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if r := waitOperation(t, m, op); r["status"] != "cancelled" {
		t.Fatal(r)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	in := m.store.State.Instances[id]
	if in.Generation != 1 || in.Lifecycle != "reclaimed" || in.Process != "stopped" {
		t.Fatalf("unexpected state: %+v", in)
	}
}

func TestReliabilityStopQueuedRestartNoNewGeneration(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	// Keep the same serialization lock used by Respond so stop wins before the
	// restart worker reaches its first process-control boundary.
	m.mu.Lock()
	in := m.store.State.Instances[id]
	accepted, failure := m.change(in, "restart")
	if failure != nil {
		m.mu.Unlock()
		t.Fatal(failure)
	}
	restartID := accepted.(map[string]any)["operationId"].(string)
	accepted, failure = m.change(in, "stop")
	m.mu.Unlock()
	if failure != nil {
		t.Fatal(failure)
	}
	stopID := accepted.(map[string]any)["operationId"].(string)
	if r := waitOperation(t, m, stopID); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if r := waitOperation(t, m, restartID); r["status"] != "cancelled" {
		t.Fatal(r)
	}
	r := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if r["generation"] != float64(1) || r["lifecycle"] != "reclaimed" {
		t.Fatal(r)
	}
}

func TestReliabilityCloseWaitsForStopWrites(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	m.mu.Lock()
	original := m.persist
	blocked := false
	m.persist = func() error {
		in := m.store.State.Instances[id]
		if !blocked && in.Runs[0].StopResult != nil {
			blocked = true
			close(entered)
			<-release
		}
		return original()
	}
	m.mu.Unlock()
	success(t, call(t, m, "stop", map[string]any{"instanceId": id}))
	select {
	case <-entered:
	case <-time.After(4 * time.Second):
		t.Fatal("stop did not reach persistence boundary")
	}
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while worker was writing")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	released = true
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not drain stop worker")
	}
	m.mu.Lock()
	remaining := len(m.workers)
	m.mu.Unlock()
	if remaining != 0 {
		t.Fatal("Close left a live worker", remaining)
	}
	statePath := filepath.Join(m.store.Dir, "state.json")
	first, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	second, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("state changed after Close returned")
	}
}
