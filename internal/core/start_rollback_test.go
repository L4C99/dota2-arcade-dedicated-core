package core

import (
	"errors"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"testing"
	"time"
)

func TestStartRollbackRemainsStoppable(t *testing.T) {
	m, path, port := setupManager(t, "ready")
	m.spawn = func(s engine.Spec) (engine.Identity, error) {
		id, e := engine.Start(s)
		if e != nil {
			return id, e
		}
		result, e := engine.Stop(id, 0, time.Second)
		if !result.Confirmed {
			return id, e
		}
		return id, &engine.StartError{Cause: errors.New("post-spawn identity failure"), Exited: true}
	}
	id, op := createRoom(t, m, path, port)
	if r := waitOperation(t, m, op); r["status"] != "failed" {
		t.Fatal(r)
	}
	state := success(t, call(t, m, "status", map[string]any{"instanceId": id}))
	if state["process"] != "stopped" {
		t.Fatal(state)
	}
	m.mu.Lock()
	identity := *m.store.State.Instances[id].Runs[0].Identity
	m.mu.Unlock()
	h, e := engine.Open(identity)
	if h != nil {
		h.Close()
		t.Fatal("child still active")
	}
	if !errors.Is(e, engine.ErrGone) {
		t.Fatal(e)
	}
	if r := stopRoom(t, m, id); r["status"] != "succeeded" {
		t.Fatal(r)
	}
}
