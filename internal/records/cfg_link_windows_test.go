package records

import (
	"os/exec"
	"testing"
)

func makeCFGDirectoryLink(t *testing.T, link, target string) {
	t.Helper()
	if b, e := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); e != nil {
		t.Fatalf("junction: %v %s", e, b)
	}
}
