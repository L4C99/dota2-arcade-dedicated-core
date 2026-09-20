package records

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCFGOwnershipReplacementAndRace(t *testing.T) {
	for _, mode := range []string{"file", "parent", "during-file", "during-parent", "link"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			parent := filepath.Join(base, "cfg")
			if e := os.Mkdir(parent, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(parent, "own.cfg")
			content := []byte("identical\n")
			created, owner, e := createOwnedCFG(path, content)
			if e != nil || !created {
				t.Fatal(e)
			}
			var swapErr error
			replace := func() {
				if mode == "file" || mode == "during-file" {
					swapErr = os.Rename(path, path+".original")
					if swapErr == nil {
						swapErr = os.WriteFile(path, content, 0600)
					}
				} else {
					swapErr = os.Rename(parent, parent+".original")
					if swapErr != nil {
						return
					}
					if mode == "link" {
						foreign := filepath.Join(base, "foreign")
						if e := os.Mkdir(foreign, 0700); e != nil {
							t.Fatal(e)
						}
						mustWrite(t, filepath.Join(foreign, "own.cfg"), content)
						makeCFGDirectoryLink(t, parent, foreign)
					} else {
						swapErr = os.Mkdir(parent, 0700)
						if swapErr == nil {
							swapErr = os.WriteFile(path, content, 0600)
						}
					}
				}
			}
			var hook func()
			if mode == "during-file" || mode == "during-parent" {
				hook = replace
			} else {
				replace()
				if swapErr != nil {
					t.Fatal(swapErr)
				}
			}
			e = cleanupOwnedCFG(path, owner, digest(content), hook)
			if runtime.GOOS == "windows" && hook != nil {
				if swapErr == nil {
					t.Fatal("held handles allowed replacement")
				}
				if e != nil {
					t.Fatal(e)
				}
			} else {
				if e == nil {
					t.Fatal("replacement not refused")
				}
				got, re := os.ReadFile(path)
				if re != nil || string(got) != string(content) {
					t.Fatalf("foreign changed: %q %v", got, re)
				}
			}
		})
	}
}

func TestCFGUnownedSameContentAndLegacyFormat2(t *testing.T) {
	for _, mode := range []string{"not-created", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			s, path := setup(t)
			in := create(t, s, path, availablePort(t), "owned")
			r, e := s.PrepareRun(in)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "not-created" {
				r.Prepared = false
				r.CFGCreated = false
			}
			r.CFGOwnership = nil
			if e = s.Save(); e != nil {
				t.Fatal(e)
			}
			reopened, e := Open(s.Dir)
			if e != nil {
				t.Fatal(e)
			}
			in = reopened.State.Instances[in.ID]
			r = in.Runs[0]
			if e = reopened.CleanupRun(in, r); e == nil {
				t.Fatal("unproven file deleted")
			}
			if _, e = os.Stat(r.CFGPath); e != nil {
				t.Fatal(e)
			}
			// Operator removes the unproven object; retry only completes cleanup.
			if e = os.Remove(r.CFGPath); e != nil {
				t.Fatal(e)
			}
			if e = reopened.CleanupRun(in, r); e != nil {
				t.Fatal(e)
			}
			if in.Generation != 1 || len(in.Runs) != 1 {
				t.Fatal("cleanup spawned generation")
			}
		})
	}
}
