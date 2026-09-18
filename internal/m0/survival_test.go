package m0

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// This experiment only starts this test executable. It never discovers or kills
// external processes. Children have a bounded lifetime even if the test fails.
func TestProbeHelper(t *testing.T) {
	role := os.Getenv("D2CORE_M0_HELPER")
	if role == "" {
		return
	}
	if role == "child" {
		for i := 0; i < 20; i++ {
			fmt.Println("heartbeat", i)
			time.Sleep(100 * time.Millisecond)
		}
		os.Exit(0)
	}
	f, err := os.OpenFile(os.Getenv("D2CORE_M0_LOG"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(21)
	}
	c := exec.Command(os.Args[0], "-test.run=^TestProbeHelper$")
	c.Env = append(os.Environ(), "D2CORE_M0_HELPER=child")
	c.Stdout, c.Stderr = f, f
	if err := c.Start(); err != nil {
		os.Exit(22)
	}
	f.Close()
	c.Process.Release()
	if role == "abrupt" {
		os.Exit(23)
	}
	// Return normally through the Go test harness without waiting for the child.
}

func TestFileLoggingSurvivesParentExit(t *testing.T) {
	for _, mode := range []string{"normal", "abrupt"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "child.log")
			c := exec.Command(os.Args[0], "-test.run=^TestProbeHelper$")
			c.Env = append(os.Environ(), "D2CORE_M0_HELPER="+mode, "D2CORE_M0_LOG="+path)
			output, err := c.CombinedOutput()
			if mode == "normal" && err != nil {
				t.Fatalf("parent: %v %s", err, output)
			}
			if mode == "abrupt" {
				if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 23 {
					t.Fatalf("wrong abrupt exit: %v %s", err, output)
				}
			}
			initial, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Parent has exited; observe subsequent writes and wait past the
			// worker's bounded lifetime so Windows can remove the temporary file.
			time.Sleep(2500 * time.Millisecond)
			final, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(final) <= len(initial) {
				t.Fatalf("no log growth after parent exit: %d -> %d", len(initial), len(final))
			}
			t.Logf("parent=%s; bytes after exit=%d, later=%d; synthetic process only", mode, len(initial), len(final))
		})
	}
}
