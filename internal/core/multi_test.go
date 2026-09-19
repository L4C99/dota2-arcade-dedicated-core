package core

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

func TestMultipleAutomaticInstancesStayIndependent(t *testing.T) {
	for _, different := range []bool{false, true} {
		name := "same-template"
		if different {
			name = "different-templates"
		}
		t.Run(name, func(t *testing.T) {
			m, path, _ := setupManager(t, "ready")
			start := 0
			for n := 0; n < 30; n++ {
				l, e := net.Listen("tcp4", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				p := l.Addr().(*net.TCPAddr).Port
				l.Close()
				if p < 65535 && records.CheckPort(p) == nil && records.CheckPort(p+1) == nil {
					start = p
					break
				}
			}
			if start == 0 {
				t.Fatal("no two adjacent free test ports")
			}
			m.mu.Lock()
			m.ports = records.PortRange{Min: start, Max: start + 1}
			m.mu.Unlock()
			secondPath := path
			if different {
				b, e := os.ReadFile(path)
				if e != nil {
					t.Fatal(e)
				}
				var v map[string]any
				if e = json.Unmarshal(b, &v); e != nil {
					t.Fatal(e)
				}
				v["name"] = "second template"
				b, _ = json.Marshal(v)
				secondPath = filepath.Join(filepath.Dir(path), "second.json")
				if e = os.WriteFile(secondPath, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			results := make(chan response, 2)
			for i, p := range []string{path, secondPath} {
				key := []string{"one", "two"}[i]
				go func(p, key string) {
					results <- call(t, m, "create", map[string]any{"template": p, "idempotencyKey": key})
				}(p, key)
			}
			a, b := success(t, <-results), success(t, <-results)
			idA, idB := a["instanceId"].(string), b["instanceId"].(string)
			for _, v := range []map[string]any{a, b} {
				if got := waitOperation(t, m, v["operationId"].(string)); got["status"] != "succeeded" {
					t.Fatal(got)
				}
			}
			m.mu.Lock()
			ra, rb := m.store.State.Instances[idA].Runs[0], m.store.State.Instances[idB].Runs[0]
			identityA, identityB := *ra.Identity, *rb.Identity
			independent := ra.CFGPath != rb.CFGPath && ra.LogPath != rb.LogPath && m.store.State.Instances[idA].Port != m.store.State.Instances[idB].Port
			m.mu.Unlock()
			if !independent {
				t.Fatal("shared generated resources")
			}
			full := call(t, m, "create", map[string]any{"template": path, "idempotencyKey": "exhausted"})
			if full.OK || full.Error.Code != "NO_PORT_AVAILABLE" {
				t.Fatal(full)
			}
			// Real helper exit must not affect the other helper. No Dota process.
			h, e := engine.Open(identityA)
			if e != nil {
				t.Fatal(e)
			}
			e = h.Kill()
			h.Close()
			if e != nil {
				t.Fatal(e)
			}
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if success(t, call(t, m, "status", map[string]any{"instanceId": idA}))["lifecycle"] == "failed" {
					break
				}
				time.Sleep(30 * time.Millisecond)
			}
			if got := stopRoom(t, m, idA); got["status"] != "succeeded" {
				t.Fatal(got)
			}
			other := success(t, call(t, m, "status", map[string]any{"instanceId": idB}))
			if other["process"] != "running" || other["room"] != "ready" {
				t.Fatal(other)
			}
			h, e = engine.Open(identityB)
			if e != nil {
				t.Fatal("other identity changed", e)
			}
			h.Close()
			newRoom := success(t, call(t, m, "create", map[string]any{"template": path, "idempotencyKey": "after-reclaim"}))
			if got := waitOperation(t, m, newRoom["operationId"].(string)); got["status"] != "succeeded" {
				t.Fatal(got)
			}
			for _, id := range []string{idB, newRoom["instanceId"].(string)} {
				if got := stopRoom(t, m, id); got["status"] != "succeeded" {
					t.Fatal(got)
				}
			}
		})
	}
}
