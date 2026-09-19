//go:build linux

package engine

import (
	"os"
	"testing"
	"time"
)

func TestDiscoverLinuxExactOwnershipAndTime(t *testing.T) {
	exe, _ := os.Executable()
	run := t.TempDir()
	spec := Spec{Executable: exe, WorkingDirectory: run, RunDirectory: run, Arguments: []string{"-test.run=^TestLinuxHelper$", "--", "--engine-helper", "normal", run, "space and 中文"}}
	before := time.Now().Add(-2 * time.Second)
	id, e := Start(spec)
	if e != nil {
		t.Fatal(e)
	}
	h, e := Open(id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Kill(); h.Close() })
	found, e := Discover(spec, before)
	if e != nil || len(found) != 1 || found[0].PID != id.PID {
		t.Fatalf("discovery: %+v %v", found, e)
	}
	altered := spec
	altered.Arguments = append([]string{}, spec.Arguments...)
	altered.Arguments[len(altered.Arguments)-1] = "other generation"
	if found, e = Discover(altered, before); e != nil || len(found) != 0 {
		t.Fatalf("wrong args: %+v %v", found, e)
	}
	if found, e = Discover(spec, time.Now().Add(time.Second)); e != nil || len(found) != 0 {
		t.Fatalf("old process: %+v %v", found, e)
	}
	altered = spec
	altered.RunDirectory = t.TempDir()
	if _, e = Discover(altered, before); e == nil {
		t.Fatal("accepted matching argv with absent generation FIFO")
	}
}
func TestDiscoveryClockRate(t *testing.T) {
	if hz, e := clockTickRate(); e != nil || hz == 0 {
		t.Fatalf("AT_CLKTCK: %d %v", hz, e)
	}
}
