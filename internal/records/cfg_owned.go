package records

import (
	"fmt"
	"os"
	"path/filepath"
)

// Optional format-2 evidence. Old records without it remain readable but cannot
// authorize deletion of an existing cfg. A checksum includes these fields.
type CFGOwnership struct {
	Parent string `json:"parent"`
	File   string `json:"file"`
}

func noLinkedParents(path string) error {
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("cfg parent is linked or not a directory: %s", p)
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}
