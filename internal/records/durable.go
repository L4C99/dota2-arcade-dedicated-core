package records

import (
	"fmt"
	"os"
	"path/filepath"
)

// DurabilityError means replacement became visible but its durability could not
// be confirmed. Callers must retain identities/reservations and stop mutating.
type DurabilityError struct{ Err error }

func (e *DurabilityError) Error() string {
	return "state replacement may already be committed: " + e.Err.Error()
}
func (e *DurabilityError) Unwrap() error { return e.Err }

func durableMkdirAll(path string) error {
	if st, e := os.Stat(path); e == nil {
		if !st.IsDir() {
			return fmt.Errorf("not a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	parent := filepath.Dir(path)
	if parent == path {
		return fmt.Errorf("missing filesystem root")
	}
	if e := durableMkdirAll(parent); e != nil {
		return e
	}
	if e := os.Mkdir(path, 0700); e != nil && !os.IsExist(e) {
		return e
	}
	return syncDirectory(parent)
}
