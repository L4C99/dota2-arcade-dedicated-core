package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func provenanceFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		b, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	run("init", "-q")
	run("config", "core.autocrlf", "true")
	run("config", "user.name", "Package test")
	run("config", "user.email", "package@example.invalid")
	run("config", "commit.gpgsign", "false")
	for name, data := range map[string]string{".gitattributes": "* text=auto\n", "README.md": "line one\nline two\n", "source.go": "package fixture\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("commit", "-qm", "fixture")
	return root, run("rev-parse", "HEAD")
}

func TestTrackedInputProvenance(t *testing.T) {
	root, commit := provenanceFixture(t)
	if err := verifyTrackedInputs(root, commit); err != nil {
		t.Fatal(err)
	}
	// Both distributed text and compiled source must fail, even if Git's clean
	// conversion considers the content unchanged under core.autocrlf=true.
	for _, name := range []string{"README.md", "source.go"} {
		path := filepath.Join(root, name)
		original, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(original), "\n", "\r\n")), 0600); err != nil {
			t.Fatal(err)
		}
		// Refresh the index stat through Git's clean filter without changing HEAD.
		if _, err := gitBytes(root, "add", "--", name); err != nil {
			t.Fatal(err)
		}
		status, err := gitBytes(root, "status", "--porcelain")
		if err != nil {
			t.Fatal(err)
		}
		if len(status) != 0 {
			t.Fatalf("fixture must reproduce Git-clean transcription, got %s", status)
		}
		if err := verifyTrackedInputs(root, commit); err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("undetected transcription: %s: %v", name, err)
		}
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := gitBytes(root, "add", "--", name); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyTrackedInputs(root, commit); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveBlobProvenance(t *testing.T) {
	root, commit := provenanceFixture(t)
	old := releaseFiles
	releaseFiles = []string{"README.md"}
	defer func() { releaseFiles = old }()
	source := filepath.Join(root, "README.md")
	good := filepath.Join(t.TempDir(), "good.zip")
	if err := archive(good, map[string]string{"README.md": source}, time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchiveSources(good, root, commit); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("line one\r\nline two\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	if err := archive(bad, map[string]string{"README.md": source}, time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchiveSources(bad, root, commit); err == nil {
		t.Fatal("accepted rewritten static ZIP entry")
	}
}
