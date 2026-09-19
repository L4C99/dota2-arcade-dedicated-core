package client

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/localipc"
)

func TestPublicClientUsesLocalTransport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	s, e := localipc.Listen(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.Serve(ctx, func([]byte) []byte {
			return []byte(`{"protocolVersion":1,"ok":false,"error":{"code":"NOT_FOUND","stage":"validate","message":"missing"}}`)
		})
	}()
	c, e := New(dir)
	if e != nil {
		t.Fatal(e)
	}
	callCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	_, e = c.Call(callCtx, "status", map[string]string{"instanceId": "missing"})
	var rejection *Error
	if !errors.As(e, &rejection) || rejection.Code != "NOT_FOUND" {
		t.Fatal(e)
	}
	if _, e = c.Call(callCtx, "remote-command", nil); e == nil {
		t.Fatal("unknown method accepted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not close")
	}
}
