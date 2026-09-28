package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Guard the distribution boundary and ensure offline links survive packaging.
func TestRuntimeDistribution(t *testing.T) {
	root := filepath.Join("..", "..")
	files := map[string]string{}
	for _, name := range releaseFiles {
		if strings.HasPrefix(name, "tools/") || strings.HasPrefix(name, ".github/") || strings.Contains(name, "validation/") || strings.Contains(name, "review") || strings.HasSuffix(name, "_test.go") {
			t.Fatalf("development material in runtime package: %s", name)
		}
		if _, exists := files[name]; exists {
			t.Fatalf("duplicate: %s", name)
		}
		files[name] = filepath.Join(root, filepath.FromSlash(name))
		if _, err := os.Stat(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	links := regexp.MustCompile(`\]\(([^)]+)\)`)
	for name, source := range files {
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		body, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range links.FindAllSubmatch(body, -1) {
			link := strings.SplitN(string(match[1]), "#", 2)[0]
			if link == "" || strings.Contains(link, "://") {
				continue
			}
			target := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(name), link)))
			if _, ok := files[target]; !ok {
				t.Errorf("%s links to excluded file %s", name, target)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "runtime.zip")
	if err := archive(path, files, time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, required := range []string{"LICENSE", "LICENSING.md", "THIRD_PARTY_NOTICES.md", "CHANGELOG.md", "examples/README.md", "examples/template.windows.json", "examples/template.linux.json", "docs/delivery.md", "docs/operations.md", "docs/local-api.md", "docs/a2s.md", "README.md", "RELEASE_NOTES.md", "examples/launcher/main.go", "examples/launcher/README.md"} {
		if file, err := z.Open(required); err != nil {
			t.Errorf("missing required licensing file %s: %v", required, err)
		} else {
			file.Close()
		}
	}
	if len(z.File) != len(releaseFiles) {
		t.Fatal("unexpected archive entries")
	}
}

func TestArchiveDeterministicAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if e := os.WriteFile(source, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	files := map[string]string{"d2core": source, "docs/readme.md": source}
	stamp := time.Unix(1700000000, 0).UTC()
	a, b := filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")
	if e := archive(a, files, stamp); e != nil {
		t.Fatal(e)
	}
	if e := archive(b, files, stamp); e != nil {
		t.Fatal(e)
	}
	aa, _ := os.ReadFile(a)
	bb, _ := os.ReadFile(b)
	if !bytes.Equal(aa, bb) {
		t.Fatal("archive not deterministic")
	}
	if e := archive(a, files, stamp); e == nil {
		t.Fatal("overwrote archive")
	}
	z, e := zip.OpenReader(a)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 2 || z.File[0].Name != "d2core" || z.File[0].Mode().Perm() != 0755 {
		t.Fatal("archive layout or executable mode")
	}
}

func TestPublicTemplateSteamRule(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "examples", "template."+platform+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var template struct{ Readiness struct{ SuccessAll []string } }
		if err := json.Unmarshal(data, &template); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, rule := range template.Readiness.SuccessAll {
			if rule == "SV:  Connection to Steam servers successful." {
				found = true
			}
		}
		if !found || len(template.Readiness.SuccessAll) != 4 {
			t.Fatal("public template must retain Steam and three map evidence rules")
		}
	}
}
func TestReleasePaths(t *testing.T) {
	if err := validateReleaseFiles(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../escape", "/absolute", "C:/absolute", "docs/../README.md", "docs\\file", ""} {
		if validArchiveName(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
	old := releaseFiles
	defer func() { releaseFiles = old }()
	releaseFiles = append(append([]string{}, old...), "LICENSE")
	if validateReleaseFiles(filepath.Join("..", "..")) == nil {
		t.Fatal("duplicate accepted")
	}
	releaseFiles = []string{"nonexistent-release-file"}
	if validateReleaseFiles(filepath.Join("..", "..")) == nil {
		t.Fatal("missing file accepted")
	}
}
