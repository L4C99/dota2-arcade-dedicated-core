// d2core manages local dedicated servers; implemented commands are listed in usage.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/L4C99/dota2-arcade-dedicated-core/internal/config"
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
