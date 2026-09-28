package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Keep raw Git stdout separate from diagnostics; never normalize source bytes.
func gitBytes(root string, args ...string) ([]byte, error) {
	c := exec.Command("git", append([]string{"--no-replace-objects", "-C", root}, args...)...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	data, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w: %s", args, err, stderr.Bytes())
	}
	return data, nil
}

// Deliberately cover the entire tracked tree, a superset of both build targets'
// sources and the distribution inputs. Git status alone applies clean filters.
func verifyTrackedInputs(root, commit string) error {
	head, err := gitBytes(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(head)) != commit {
		return fmt.Errorf("HEAD changed during packaging")
	}
	tree, err := gitBytes(root, "ls-tree", "-rz", "--full-tree", commit)
	if err != nil {
		return err
	}
	for _, entry := range bytes.Split(tree, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		fields := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(fields) != 2 {
			return fmt.Errorf("invalid Git tree entry")
		}
		meta := strings.Fields(string(fields[0]))
		name := string(fields[1])
		if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") || !validArchiveName(name) {
			return fmt.Errorf("unsupported tracked input: %s", name)
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular tracked input: %s", name)
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		expected, err := gitBytes(root, "cat-file", "blob", meta[2])
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("source provenance mismatch: %s differs byte-for-byte from %s (no line-ending normalization permitted)", name, commit)
		}
	}
	return nil
}

func verifyArchiveSources(path, root, commit string) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, name := range releaseFiles {
		f, err := z.Open(name)
		if err != nil {
			return err
		}
		actual, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		expected, err := gitBytes(root, "show", commit+":"+name)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("archive provenance mismatch: %s", name)
		}
	}
	return nil
}
