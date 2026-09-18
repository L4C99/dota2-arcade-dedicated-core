// d2core currently exposes only M0 read-only validation helpers.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"d2core.local/core/internal/m0"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "m0-inspect" {
		return fmt.Errorf("M0 only: d2core m0-inspect [--executable ABS] [--working-directory ABS] [--cfg-directory ABS] [--vpk ABS] [--gameinfo ABS] [--version-file ABS]; does not launch Dota")
	}
	f := flag.NewFlagSet("m0-inspect", flag.ContinueOnError)
	var in m0.Input
	f.StringVar(&in.Executable, "executable", "", "absolute engine executable path")
	f.StringVar(&in.WorkingDirectory, "working-directory", "", "absolute engine working directory")
	f.StringVar(&in.CFGDirectory, "cfg-directory", "", "absolute engine cfg directory")
	f.StringVar(&in.VPK, "vpk", "", "absolute VPK path (reads entire file to hash)")
	f.StringVar(&in.GameInfo, "gameinfo", "", "absolute gameinfo.gi path (fingerprint only)")
	f.StringVar(&in.VersionFile, "version-file", "", "absolute engine version file (fingerprint only)")
	if err := f.Parse(args[1:]); err != nil {
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
