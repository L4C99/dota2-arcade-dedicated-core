package records

import (
	"os"
	"testing"
)

func makeCFGDirectoryLink(t *testing.T, link, target string) {
	t.Helper()
	if e := os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
}
