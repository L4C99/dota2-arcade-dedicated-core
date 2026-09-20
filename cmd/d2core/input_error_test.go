package main

import (
	"context"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/core"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/localipc"
	"os"
	"testing"
)

func TestCLICallerErrorCodes(t *testing.T) {
	dir, e := os.MkdirTemp("", "d2cli-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	// Let localipc create its private directory rather than adopting temp ACLs.
	dir += "/data"
	server, e := localipc.Listen(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	m, e := core.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { server.Serve(ctx, m.Respond); close(done) }()
	defer func() { cancel(); server.Close(); <-done }()
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"create", "--template", "relative.json", "--idempotency-key", "invalid"}, "INVALID_PATH"},
		{[]string{"logs", "--tail", "-1", "unused"}, "INVALID_REQUEST"},
		{[]string{"logs", "--tail", "1001", "unused"}, "INVALID_REQUEST"},
	} {
		args := append([]string{tc.args[0], "--data-dir", dir}, tc.args[1:]...)
		result, e := capture(t, args...)
		if e == nil {
			t.Fatal("accepted invalid input")
		}
		failure, ok := result["error"].(map[string]any)
		if !ok || failure["code"] != tc.code || failure["stage"] != "validate" {
			t.Fatalf("%v: %v %v", args, result, e)
		}
	}
}
