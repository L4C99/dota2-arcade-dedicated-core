// package is a maintainer-only tool that builds delivery archives from a clean checkout. It does
// not publish a GitHub release or include games, maps, credentials or local data.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Explicit runtime distribution allowlist. Development evidence stays in source.
var releaseFiles = []string{
	"README.md", "RELEASE_NOTES.md", "docs/delivery.md", "docs/operations.md",
	"docs/local-api.md", "docs/a2s.md", "examples/README.md",
	"examples/template.windows.json", "examples/template.linux.json",
	"examples/launcher/README.md", "examples/launcher/main.go",
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func command(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	b, e := c.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("%s: %w: %s", name, e, b)
	}
	return strings.TrimSpace(string(b)), nil
}
func run() error {
	goTool := flag.String("go", "go", "Go 1.27.1 binary")
	out := flag.String("output", "dist", "archive output directory")
	buildStamp := flag.String("build-time", "", "RFC3339 build timestamp; supply the same value for reproducible rebuilds")
	release := flag.String("version", "0.1.1-dev", "release version without v, e.g. 0.1.1")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-dev|-rc\.[1-9][0-9]*)?$`).MatchString(*release) {
		return fmt.Errorf("version must be X.Y.Z, X.Y.Z-dev or X.Y.Z-rc.N")
	}
	root, e := command("git", "rev-parse", "--show-toplevel")
	if e != nil {
		return e
	}
	cwd, e := os.Getwd()
	if e != nil {
		return e
	}
	if filepath.Clean(root) != filepath.Clean(cwd) {
		return fmt.Errorf("run from repository root")
	}
	status, e := command("git", "status", "--porcelain")
	if e != nil {
		return e
	}
	if status != "" {
		return fmt.Errorf("refusing package from dirty checkout")
	}
	commit, e := command("git", "rev-parse", "HEAD")
	if e != nil {
		return e
	}
	stamp, e := command("git", "show", "-s", "--format=%cI", "HEAD")
	if e != nil {
		return e
	}
	when, e := time.Parse(time.RFC3339, stamp)
	if e != nil {
		return e
	}
	when = when.UTC()
	builtAt := time.Now().UTC().Truncate(time.Second)
	if *buildStamp != "" {
		builtAt, e = time.Parse(time.RFC3339, *buildStamp)
		if e != nil {
			return e
		}
		builtAt = builtAt.UTC()
	}
	version, e := command(*goTool, "version")
	if e != nil {
		return e
	}
	if !strings.Contains(version, " go1.27.1 ") {
		return fmt.Errorf("expected pinned Go 1.27.1, got %s", version)
	}
	abs, e := filepath.Abs(*out)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(abs, 0755); e != nil {
		return e
	}
	stage, e := os.MkdirTemp(abs, ".package-")
	if e != nil {
		return e
	}
	defer func() {
		rel, e := filepath.Rel(abs, stage)
		if e == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			os.RemoveAll(stage)
		}
	}()
	common := releaseFiles
	for _, platform := range []string{"windows", "linux"} {
		suffix := ""
		if platform == "windows" {
			suffix = ".exe"
		}
		files := map[string]string{}
		for _, name := range common {
			files[name] = filepath.Join(root, filepath.FromSlash(name))
		}
		for _, target := range []struct{ name, pkg string }{{"d2core", "./cmd/d2core"}, {"launcher-example", "./examples/launcher"}} {
			path := filepath.Join(stage, platform+"-"+target.name+suffix)
			c := exec.Command(*goTool, "build", "-trimpath", "-buildvcs=true", "-ldflags=-X main.buildTime="+builtAt.Format(time.RFC3339)+" -X main.releaseVersion="+*release, "-o", path, target.pkg)
			c.Env = append(os.Environ(), "GOOS="+platform, "GOARCH=amd64", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
			if b, e := c.CombinedOutput(); e != nil {
				return fmt.Errorf("build %s/%s: %w %s", platform, target.name, e, b)
			}
			files[target.name+suffix] = path
		}
		manifest, _ := json.MarshalIndent(map[string]any{"version": *release, "gitCommit": commit, "sourceTime": when.Format(time.RFC3339), "buildTime": builtAt.Format(time.RFC3339), "goVersion": "1.27.1", "os": platform, "arch": "amd64", "status": "built; artifact validation recorded separately"}, "", "  ")
		mp := filepath.Join(stage, platform+"-BUILD.json")
		if e = os.WriteFile(mp, manifest, 0600); e != nil {
			return e
		}
		files["BUILD.json"] = mp
		name := "d2core-" + commit[:12] + "-" + platform + "-amd64.zip"
		if *release != "0.1.1-dev" {
			name = "d2core-v" + *release + "-" + platform + "-amd64.zip"
		}
		path := filepath.Join(abs, name)
		if e = archive(path, files, builtAt); e != nil {
			return e
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		hash := sha256.New()
		_, e = io.Copy(hash, f)
		f.Close()
		if e != nil {
			return e
		}
		checksum := hex.EncodeToString(hash.Sum(nil)) + "  " + name + "\n"
		cf, e := os.OpenFile(path+".sha256", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			return e
		}
		_, e = cf.WriteString(checksum)
		closeErr := cf.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Print(checksum)
	}
	return nil
}
func archive(path string, files map[string]string, when time.Time) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	z := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		mode := os.FileMode(0644)
		if !strings.Contains(name, "/") && (strings.HasPrefix(name, "d2core") || strings.HasPrefix(name, "launcher-example")) {
			mode = 0755
		}
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetModTime(when)
		h.SetMode(mode)
		w, e := z.CreateHeader(h)
		if e != nil {
			z.Close()
			f.Close()
			return e
		}
		source, e := os.Open(files[name])
		if e != nil {
			z.Close()
			f.Close()
			return e
		}
		_, e = io.Copy(w, source)
		source.Close()
		if e != nil {
			z.Close()
			f.Close()
			return e
		}
	}
	if e = z.Close(); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}
