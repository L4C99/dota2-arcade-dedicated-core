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

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/a2s"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/core"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/engine"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/localipc"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/m0"
	"github.com/L4C99/dota2-arcade-dedicated-core/internal/records"
)

var buildTime = "unknown"        // Set with -ldflags at packaging time, not the commit timestamp.
var releaseVersion = "0.1.0-dev" // Set by the release packager; source builds retain the development marker.

const commandHelp = `Usage: d2core <command> [options]

Commands:
  version                         Build, commit and protocol versions
  check --template ABS            Static template validation only
  serve                           Run the local manager in the foreground
  create --template ABS --idempotency-key KEY [--port N]
                                  Create a room; poll the returned operation ID
  list                            Active and failed instances, storage status
  status INSTANCE                 Current instance state
  operation OPERATION             Asynchronous operation result
  logs INSTANCE [--tail N] [--generation N]
                                  Engine log, optionally from a prior generation
  restart INSTANCE                Restart using the saved template and port
  stop INSTANCE                   Stop and reclaim, including failed instances
  a2s enable --dota-dir ABS        Explicitly configure gameinfo Advertise
  m0-inspect [resource options]    Optional read-only development diagnostics

Manager commands accept --data-dir ABS and --json (JSON is the default).
serve options: --port-min 27015 --port-max 27064 --history-days 7 --min-free-mib 1024
create: port 0 or omitted selects automatically; a new intent needs a new key.
logs: --tail defaults to 100 (1..1000); --generation 0 selects the latest.
Use d2core <command> --help or d2core help <command> for parameter details.
For A2S details use d2core a2s enable --help. m0-inspect does not accept --json.
Ctrl+C exits the manager without stopping games. Use stop to reclaim a room.
See docs/operations.md for environment setup, resource layout and examples.`

type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

func usage(format string, args ...any) error { return usageError{fmt.Errorf(format, args...)} }

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
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
		return usage("%s", commandHelp)
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) == 1 {
			fmt.Fprintln(os.Stdout, commandHelp)
			return nil
		}
		if args[0] == "help" {
			switch args[1] {
			case "version", "check", "serve", "create", "list", "status", "operation", "logs", "restart", "stop", "m0-inspect":
				if len(args) == 2 {
					return run([]string{args[1], "--help"})
				}
			case "a2s":
				if len(args) == 2 || len(args) == 3 && args[2] == "enable" {
					return run([]string{"a2s", "enable", "--help"})
				}
			}
		}
		return usage("use d2core help or d2core help <command>")
	}
	switch args[0] {
	case "a2s":
		if len(args) < 2 || args[1] != "enable" {
			return usage("a2s enable --dota-dir ABS [--json]")
		}
		f := flag.NewFlagSet("a2s enable", flag.ContinueOnError)
		dir := f.String("dota-dir", "", "absolute Dota installation root")
		f.Bool("json", false, "structured output")
		if err := f.Parse(args[2:]); err != nil {
			return usageError{err}
		}
		if *dir == "" || f.NArg() != 0 {
			return usage("a2s enable requires --dota-dir ABS")
		}
		result, err := a2s.Enable(*dir)
		if err != nil {
			code := "IO_ERROR"
			if result.Status == "conflict" {
				code = "CONFIG_CONFLICT"
			}
			_ = writeResult(result, map[string]string{"code": code, "stage": "configure", "message": err.Error()})
			return err
		}
		return writeResult(result, nil)
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
		v := map[string]any{"version": releaseVersion, "gitCommit": "unknown", "buildTime": buildTime, "schemaVersion": 1, "formatVersion": records.FormatVersion, "protocolVersion": 1}
		if info, ok := debug.ReadBuildInfo(); ok {
			v["goVersion"] = info.GoVersion
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					v["gitCommit"] = s.Value
				}
				if s.Key == "vcs.modified" {
					v["gitDirty"] = s.Value == "true"
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
			if !strings.Contains(a, "=") && (a == "--data-dir" || a == "--template" || a == "--port" || a == "--port-min" || a == "--port-max" || a == "--history-days" || a == "--min-free-mib" || a == "--idempotency-key" || a == "--tail" || a == "--generation") && i+1 < len(args) {
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
	options := core.DefaultOptions()
	if command == "serve" {
		f.IntVar(&options.Ports.Min, "port-min", options.Ports.Min, "automatic game port range start")
		f.IntVar(&options.Ports.Max, "port-max", options.Ports.Max, "automatic game port range end")
		f.IntVar(&options.HistoryDays, "history-days", options.HistoryDays, "reclaimed history and request-key retention days")
		f.IntVar(&options.MinFreeMiB, "min-free-mib", options.MinFreeMiB, "minimum free disk space before new starts")
	}
	if command == "create" {
		f.StringVar(&template, "template", "", "absolute template path")
		f.StringVar(&key, "idempotency-key", "", "stable caller request key")
		f.IntVar(&port, "port", 0, "game port; 0 or omitted selects automatically")
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
		if err = options.Validate(); err != nil {
			return usage("%v", err)
		}
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
		m, err := core.OpenWithOptions(*dir, options)
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
		if result != nil {
			v["result"] = result
		}
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
