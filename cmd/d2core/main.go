// d2core manages local dedicated servers; implemented commands are listed in usage.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/core"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/localipc"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/m0"
)

var buildTime = "unknown" // Set with -ldflags at packaging time, not the commit timestamp.

type usageError struct{ error }

func usage(format string, args ...any) error { return usageError{fmt.Errorf(format, args...)} }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var u usageError
		if errors.As(err, &u) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage("usage: d2core check --template ABS [--json] | version [--json] | m0-inspect [resource options]")
	}
	switch args[0] {
	case "serve", "create", "list", "status", "operation", "logs", "restart", "stop":
		return manage(args[0], args[1:])
	case "__engine-quit":
		if len(args) != 2 {
			return usage("internal console helper requires identity")
		}
		return engine.QuitHelper(args[1])
	case "check":
		f := flag.NewFlagSet("check", flag.ContinueOnError)
		path := f.String("template", "", "absolute template path")
		f.Bool("json", false, "structured output (also the default)")
		if err := f.Parse(args[1:]); err != nil {
			return usageError{err}
		}
		if f.NArg() != 0 || *path == "" {
			return usage("check requires --template ABS and no positional arguments")
		}
		t, err := config.Load(*path)
		if err != nil {
			code := "INVALID_TEMPLATE"
			var ce *config.Error
			if errors.As(err, &ce) {
				code = ce.Code
			}
			_ = writeResult(nil, map[string]string{"code": code, "stage": "validate", "message": err.Error()})
			return err
		}
		return writeResult(map[string]any{"template": t, "staticOnly": true}, nil)
	case "version":
		f := flag.NewFlagSet("version", flag.ContinueOnError)
		f.Bool("json", false, "structured output (also the default)")
		if err := f.Parse(args[1:]); err != nil {
			return usageError{err}
		}
		if f.NArg() != 0 {
			return usage("version accepts no positional arguments")
		}
		v := map[string]any{"version": "0.1.0-dev", "gitCommit": "unknown", "buildTime": buildTime, "schemaVersion": 1, "formatVersion": 1, "protocolVersion": 1}
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					v["gitCommit"] = s.Value
				}
			}
		}
		return writeResult(v, nil)
	case "m0-inspect":
		return inspect(args[1:])
	default:
		return usage("unknown command %q", args[0])
	}
}

// Keep flag parsing predictable while allowing documented `status ID --json`.
func flagsBeforePositionals(args []string) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && (a == "--data-dir" || a == "--template" || a == "--port" || a == "--idempotency-key" || a == "--tail" || a == "--generation") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positionals = append(positionals, a)
		}
	}
	return append(flags, positionals...)
}
func manage(command string, args []string) error {
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := f.String("data-dir", filepath.Join(filepath.Dir(self), "data"), "absolute data directory")
	f.Bool("json", false, "structured output (default)")
	var template, key string
	var port, tail, generation int
	if command == "create" {
		f.StringVar(&template, "template", "", "absolute template path")
		f.StringVar(&key, "idempotency-key", "", "stable caller request key")
		f.IntVar(&port, "port", 0, "explicit game port")
	}
	if command == "logs" {
		f.IntVar(&tail, "tail", 100, "lines 1..1000")
		f.IntVar(&generation, "generation", 0, "generation, defaults to latest")
	}
	if err = f.Parse(flagsBeforePositionals(args)); err != nil {
		return usageError{err}
	}
	if !filepath.IsAbs(*dir) {
		return usage("--data-dir must be absolute")
	}
	needID := command == "status" || command == "operation" || command == "logs" || command == "restart" || command == "stop"
	if needID && f.NArg() != 1 || !needID && f.NArg() != 0 {
		return usage("%s: unexpected or missing positional ID", command)
	}
	if command == "serve" {
		s, err := localipc.Listen(*dir)
		if err != nil {
			code := "IO_ERROR"
			if errors.Is(err, localipc.ErrLocked) {
				code = "MANAGER_LOCKED"
			}
			_ = writeResult(nil, map[string]string{"code": code, "stage": "persist", "message": err.Error()})
			return err
		}
		defer s.Close()
		m, err := core.Open(*dir)
		if err != nil {
			_ = writeResult(nil, map[string]string{"code": "IO_ERROR", "stage": "persist", "message": err.Error()})
			return err
		}
		defer m.Close() // Retain the IPC lock until every manager worker finishes.
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		fmt.Fprintln(os.Stderr, "d2core local manager ready")
		if err = writeResult(map[string]any{"serving": true, "dataDir": *dir}, nil); err != nil {
			return err
		}
		return s.Serve(ctx, m.Respond)
	}
	params := map[string]any{}
	switch command {
	case "create":
		if template == "" || key == "" {
			return usage("create requires --template ABS and --idempotency-key KEY")
		}
		params["template"] = template
		params["port"] = port
		params["idempotencyKey"] = key
	case "operation":
		params["operationId"] = f.Arg(0)
	case "status", "restart", "stop", "logs":
		params["instanceId"] = f.Arg(0)
		if command == "logs" {
			params["tail"] = tail
			params["generation"] = generation
		}
	}
	b, err := json.Marshal(map[string]any{"protocolVersion": 1, "method": command, "params": params})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err = localipc.Call(ctx, *dir, b)
	if err != nil {
		code := "MANAGER_UNAVAILABLE"
		if errors.Is(err, localipc.ErrVersion) {
			code = "UNSUPPORTED_VERSION"
		}
		_ = writeResult(nil, map[string]string{"code": code, "stage": "transport", "message": err.Error()})
		return err
	}
	var envelope struct {
		ProtocolVersion int             `json:"protocolVersion"`
		OK              bool            `json:"ok"`
		Result          json.RawMessage `json:"result"`
		Error           json.RawMessage `json:"error"`
	}
	if err = config.DecodeStrict(b, &envelope); err != nil || envelope.ProtocolVersion != 1 {
		return fmt.Errorf("invalid manager response: %v", err)
	}
	if _, err = os.Stdout.Write(append(b, '\n')); err != nil {
		return err
	}
	if !envelope.OK {
		return fmt.Errorf("manager rejected request; see JSON error")
	}
	return nil
}

func writeResult(result any, failure any) error {
	v := map[string]any{"protocolVersion": 1, "ok": failure == nil}
	if failure != nil {
		v["error"] = failure
	} else {
		v["result"] = result
	}
	return json.NewEncoder(os.Stdout).Encode(v)
}

func inspect(args []string) error {
	f := flag.NewFlagSet("m0-inspect", flag.ContinueOnError)
	var in m0.Input
	f.StringVar(&in.Executable, "executable", "", "absolute engine executable path")
	f.StringVar(&in.WorkingDirectory, "working-directory", "", "absolute engine working directory")
	f.StringVar(&in.CFGDirectory, "cfg-directory", "", "absolute engine cfg directory")
	f.StringVar(&in.VPK, "vpk", "", "absolute VPK path (reads entire file to hash)")
	f.StringVar(&in.GameInfo, "gameinfo", "", "absolute gameinfo.gi path (fingerprint only)")
	f.StringVar(&in.VersionFile, "version-file", "", "absolute engine version file (fingerprint only)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	r := m0.Inspect(in)
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if err := e.Encode(r); err != nil {
		return err
	}
	if !r.InputsValid {
		return fmt.Errorf("M0 input inspection failed; see JSON checks")
	}
	return nil
}
