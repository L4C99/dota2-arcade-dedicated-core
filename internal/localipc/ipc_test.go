package localipc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func serving(t *testing.T, handler func([]byte) []byte) (string, *Server) {
	t.Helper()
	base, e := os.MkdirTemp("", "d2ipc-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := os.RemoveAll(base); e != nil {
			t.Error(e)
		}
	})
	dir := filepath.Join(base, "data")
	s, e := Listen(dir)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, handler) }()
	t.Cleanup(func() {
		cancel()
		s.Close()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(time.Second):
			t.Error("serve did not stop")
		}
	})
	return dir, s
}
func TestRoundtripAndExclusiveManager(t *testing.T) {
	dir, s := serving(t, func(b []byte) []byte { return append([]byte(nil), b...) })
	if other, e := Listen(dir); !errors.Is(e, ErrLocked) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("second manager: %v", e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, e := Call(ctx, dir, []byte(`{"hello":"中文"}`))
	if e != nil || string(got) != `{"hello":"中文"}` {
		t.Fatalf("roundtrip %q %v", got, e)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	next, e := Listen(dir)
	if e != nil {
		t.Fatal("lock not released", e)
	}
	next.Close()
}
func TestCallCancellationDoesNotCancelAcceptedHandler(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	defer close(release)
	dir, _ := serving(t, func(b []byte) []byte { close(entered); <-release; close(finished); return []byte(`{}`) })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, e := Call(ctx, dir, []byte(`{}`)); result <- e }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	select {
	case e := <-result:
		if e == nil {
			t.Fatal("missing timeout")
		}
	case <-time.After(time.Second):
		t.Fatal("call cancellation unbounded")
	}
	select {
	case <-finished:
		t.Fatal("handler cancelled by disconnect")
	default:
	}
}
func TestBounds(t *testing.T) {
	dir, _ := serving(t, func(b []byte) []byte { return bytes.Repeat([]byte("x"), MaxResponse+1) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := Call(ctx, dir, bytes.Repeat([]byte("x"), MaxRequest+1)); e == nil {
		t.Fatal("oversize request accepted")
	}
	if _, e := Call(ctx, dir, []byte("{}\n{}")); e == nil {
		t.Fatal("multiple request lines accepted")
	}
	if _, e := Call(ctx, dir, []byte(`{}`)); e == nil {
		t.Fatal("oversize response accepted")
	}
	if _, e := readLine(bytes.NewReader(append(bytes.Repeat([]byte("x"), MaxRequest+1), '\n')), MaxRequest); e == nil {
		t.Fatal("server reader accepted oversize")
	}
}
func TestClosePreservesChangedEndpoint(t *testing.T) {
	dir, s := serving(t, func(b []byte) []byte { return b })
	p := filepath.Join(dir, "manager", "endpoint.json")
	if e := os.WriteFile(p, []byte("foreign"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "foreign" {
		t.Fatalf("unowned endpoint removed: %s %v", b, e)
	}
}
func TestEndpointVersionAndRelativeDirectory(t *testing.T) {
	dir, s := serving(t, func(b []byte) []byte { return b })
	if e := os.WriteFile(filepath.Join(dir, "manager", "endpoint.json"), []byte(`{"protocolVersion":2,"type":"bad","address":"bad"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Call(context.Background(), dir, []byte(`{}`)); e == nil {
		t.Fatal("version accepted")
	}
	s.Close()
	if x, e := Listen("relative"); e == nil {
		x.Close()
		t.Fatal("relative accepted")
	}
}

func TestListenPreservesUnrecognizedEndpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if e := prepareDirectory(dir); e != nil {
		t.Fatal(e)
	}
	manager := filepath.Join(dir, "manager")
	if e := prepareDirectory(manager); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(manager, "endpoint.json")
	if e := os.WriteFile(p, []byte("foreign"), 0600); e != nil {
		t.Fatal(e)
	}
	if s, e := Listen(dir); e == nil {
		s.Close()
		t.Fatal("overwrote unrecognized endpoint")
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "foreign" {
		t.Fatal("foreign endpoint changed", e)
	}
}

func TestServeCancellationRetainsWriterLock(t *testing.T) {
	base, e := os.MkdirTemp("", "d2ipc-lock-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(base)
	dir := filepath.Join(base, "data")
	s, e := Listen(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	defer close(release)
	go func() {
		done <- s.Serve(ctx, func(b []byte) []byte { close(entered); <-release; close(finished); return []byte(`{}`) })
	}()
	callCtx, callCancel := context.WithTimeout(context.Background(), time.Second)
	defer callCancel()
	go func() { _, _ = Call(callCtx, dir, []byte(`{}`)) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve cancellation did not return")
	}
	if next, e := Listen(dir); !errors.Is(e, ErrLocked) {
		if next != nil {
			next.Close()
		}
		t.Fatalf("writer lock released before explicit Close: %v", e)
	}
	select {
	case <-finished:
		t.Fatal("cancel unexpectedly finished handler")
	default:
	}
	release <- struct{}{}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("accepted work did not drain")
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	next, e := Listen(dir)
	if e != nil {
		t.Fatal("explicit Close did not release lock", e)
	}
	next.Close()
}
