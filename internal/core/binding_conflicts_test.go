package core

import (
	"fmt"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"testing"
	"time"
)

func TestBindingAddressAndUDPClassification(t *testing.T) {
	for _, tc := range []struct {
		a, b engine.Binding
		want bool
	}{
		{engine.Binding{Protocol: "tcp4", Address: "0.0.0.0", Port: 9}, engine.Binding{Protocol: "tcp4", Address: "127.0.0.1", Port: 9}, true},
		{engine.Binding{Protocol: "udp4", Address: "127.0.0.1", Port: 9}, engine.Binding{Protocol: "udp4", Address: "127.0.0.2", Port: 9}, false},
		{engine.Binding{Protocol: "tcp6", Address: "::", Port: 9}, engine.Binding{Protocol: "tcp6", Address: "::1", Port: 9}, true},
		{engine.Binding{Protocol: "tcp4", Address: "0.0.0.0", Port: 9}, engine.Binding{Protocol: "udp4", Address: "127.0.0.1", Port: 9}, false},
	} {
		if got := overlap(tc.a, tc.b); got != tc.want {
			t.Fatalf("%+v %+v = %v", tc.a, tc.b, got)
		}
	}
	udp := engine.Binding{Protocol: "udp4", Address: "127.0.0.1", Port: 50001}
	if serviceBinding(udp, []engine.Binding{udp}, 27015) {
		t.Fatal("UDP ephemeral guessed service")
	}
	tcp := udp
	tcp.Protocol = "tcp4"
	if !serviceBinding(udp, []engine.Binding{tcp, udp}, 27015) {
		t.Fatal("paired UDP not checked")
	}
}

func TestAdditionalMainAndAdditionalConflicts(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	second := success(t, call(t, m, "create", map[string]any{"template": path, "idempotencyKey": "peer"}))
	sid := second["instanceId"].(string)
	if r := waitOperation(t, m, second["operationId"].(string)); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	m.mu.Lock()
	a, b := m.store.State.Instances[id], m.store.State.Instances[sid]
	bind := engine.Binding{Protocol: "tcp4", Address: "127.0.0.1", Port: b.Port, PID: a.Runs[0].Identity.PID}
	if p := m.checkBindingConflicts(a, []engine.Binding{bind}); p.Status != "conflict" {
		t.Fatal(p)
	}
	// Inject only the native table read, retaining actual identity/liveness checks.
	// Pure overlap cases complement the unmodified real socket helper test below.
	extra := []engine.Binding{{Protocol: "tcp4", Address: "0.0.0.0", Port: 40099}, {Protocol: "udp4", Address: "0.0.0.0", Port: 40099}}
	m.bindings = func(*engine.Handle) ([]engine.Binding, error) { return extra, nil }
	own := []engine.Binding{{Protocol: "tcp4", Address: "127.0.0.1", Port: 40099}, {Protocol: "udp4", Address: "127.0.0.1", Port: 40099}}
	p := m.checkBindingConflicts(a, own)
	if p.Status != "conflict" || len(p.Conflicts) != 2 {
		t.Fatal(p)
	}
	unknown := []engine.Binding{{Protocol: "udp4", Address: "127.0.0.1", Port: 40099}}
	if p := m.checkBindingConflicts(a, unknown); p.Status != "partial" || len(p.Conflicts) != 0 {
		t.Fatal(p)
	}
	m.bindings = nil
	m.mu.Unlock()
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	if s := success(t, call(t, m, "status", map[string]any{"instanceId": sid})); s["process"] != "running" || s["room"] != "ready" {
		t.Fatal(s)
	}
	stopRoom(t, m, sid)
}

func TestNativeExtraSocketsDoNotFalseConflict(t *testing.T) {
	m, path, port := setupManager(t, "extra-sockets")
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "succeeded" {
		t.Fatal(r)
	}
	time.Sleep(550 * time.Millisecond)
	m.mu.Lock()
	in := m.store.State.Instances[id]
	r := in.Runs[0]
	extras := 0
	for _, b := range r.Bindings {
		if b.Port != port {
			extras++
		}
	}
	if extras < 3 || r.PortCheck == nil || r.PortCheck.Status != "partial" || len(r.PortCheck.Conflicts) != 0 {
		t.Fatalf("%s %+v", fmt.Sprint(r.Bindings), r.PortCheck)
	}
	if in.Room != "ready" {
		t.Fatal(in.Room)
	}
	m.mu.Unlock()
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
}
