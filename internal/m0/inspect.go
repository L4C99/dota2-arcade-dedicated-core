// Package m0 contains experiments, not the production manager contract.
package m0

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type Input struct {
	Executable, WorkingDirectory, CFGDirectory, VPK, GameInfo, VersionFile string
}

type Check struct {
	Name         string `json:"name"`
	Path         string `json:"path,omitempty"`
	ResolvedPath string `json:"resolvedPath,omitempty"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	Size         int64  `json:"size,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
}

type Report struct {
	SchemaVersion    int       `json:"schemaVersion"`
	RecordedAt       time.Time `json:"recordedAt"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	GoVersion        string    `json:"goVersion"`
	InputsValid      bool      `json:"suppliedInputsValid"`
	EngineValidation string    `json:"engineValidation"`
	Checks           []Check   `json:"checks"`
}

func Inspect(in Input) Report {
	r := Report{SchemaVersion: 1, RecordedAt: time.Now().UTC(), OS: runtime.GOOS,
		Arch: runtime.GOARCH, GoVersion: runtime.Version(), InputsValid: true,
		EngineValidation: "not_run"}
	for _, item := range []struct {
		name, path string
		directory  bool
	}{
		{"executable", in.Executable, false}, {"workingDirectory", in.WorkingDirectory, true},
		{"cfgDirectory", in.CFGDirectory, true}, {"vpk", in.VPK, false},
		{"gameinfo", in.GameInfo, false}, {"versionFile", in.VersionFile, false},
	} {
		c := inspectPath(item.name, item.path, item.directory)
		r.Checks = append(r.Checks, c)
		if c.Status == "error" {
			r.InputsValid = false
		}
	}
	return r
}

func inspectPath(name, path string, directory bool) Check {
	c := Check{Name: name, Path: path, Status: "missing_input"}
	if path == "" {
		return c
	}
	c.Status = "error"
	if !filepath.IsAbs(path) {
		c.Error = "absolute path required"
		return c
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.ResolvedPath = resolved
	f, err := os.Open(path)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		c.Error = err.Error()
		return c
	}
	if directory {
		if !before.IsDir() {
			c.Error = "expected directory"
			return c
		}
	} else {
		if !before.Mode().IsRegular() {
			c.Error = "expected regular file"
			return c
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		if err != nil {
			c.Error = err.Error()
			return c
		}
		after, err := f.Stat()
		if err != nil {
			c.Error = err.Error()
			return c
		}
		current, err := os.Stat(path)
		if err != nil {
			c.Error = err.Error()
			return c
		}
		if !os.SameFile(before, current) || before.Size() != n || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			c.Error = "file changed during inspection; retry while resources are idle"
			return c
		}
		c.Size = n
		c.SHA256 = hex.EncodeToString(h.Sum(nil))
	}
	c.Status = "observed"
	return c
}
