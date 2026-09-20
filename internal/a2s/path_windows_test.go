package a2s

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsInstallationAliases(t *testing.T) {
	for _, mode := range []string{"case", "dots", "short"} {
		t.Run(mode, func(t *testing.T) {
			dir, _, _ := fixture(t)
			arg := dir
			switch mode {
			case "case":
				arg = strings.ToUpper(dir)
			case "dots":
				arg = dir + `\.\game\..`
			case "short":
				p, e := windows.UTF16PtrFromString(dir)
				if e != nil {
					t.Fatal(e)
				}
				b := make([]uint16, 32768)
				n, e := windows.GetShortPathName(p, &b[0], uint32(len(b)))
				if e != nil {
					t.Skipf("8.3 unavailable: %v", e)
				}
				arg = windows.UTF16ToString(b[:n])
				if strings.EqualFold(arg, dir) {
					t.Skip("volume has no distinct 8.3 alias")
				}
			}
			r, e := Enable(arg)
			if e != nil || r.Status != "inserted" {
				t.Fatal(r, e)
			}
			r, e = Enable(dir)
			if e != nil || r.Status != "already_configured" {
				t.Fatal(r, e)
			}
		})
	}
}
func TestWindowsInstallationJunctionRejected(t *testing.T) {
	dir, path, before := fixture(t)
	link := filepath.Join(t.TempDir(), "linked")
	if b, e := exec.Command("cmd", "/c", "mklink", "/J", filepath.Clean(link), filepath.Clean(dir)).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, b)
	}
	if _, e := Enable(link); e == nil {
		t.Fatal("junction accepted")
	}
	got, e := os.ReadFile(path)
	if e != nil || string(got) != string(before) {
		t.Fatal("source changed", e)
	}
}
