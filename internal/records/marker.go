package records

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const markerName = "store.marker"
const markerContent = "d2core state format 2\n"

func verifyMarker(dir string) (bool, error) {
	path := filepath.Join(dir, markerName)
	st, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if !st.Mode().IsRegular() || st.Size() != int64(len(markerContent)) {
		return false, fmt.Errorf("invalid store initialization marker")
	}
	f, e := os.Open(path)
	if e != nil {
		return false, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(len(markerContent))+1))
	if e != nil {
		return false, e
	}
	if !bytes.Equal(b, []byte(markerContent)) {
		return false, fmt.Errorf("unrecognized store initialization marker")
	}
	return true, nil
}

func ensureInitialized(dir string) error {
	exists, e := verifyMarker(dir)
	if e != nil {
		return e
	}
	state, e := os.Lstat(filepath.Join(dir, "state.json"))
	if e == nil {
		if !state.Mode().IsRegular() {
			return fmt.Errorf("state is not a regular file")
		}
		if !exists {
			return fmt.Errorf("state exists without initialization marker")
		}
		return nil
	}
	if !os.IsNotExist(e) {
		return e
	}
	if exists {
		return fmt.Errorf("initialized store lost state; refusing reinitialization")
	}
	if _, e = os.Lstat(filepath.Join(dir, "instances")); e == nil || !os.IsNotExist(e) {
		return fmt.Errorf("instance files exist without state; refusing initialization")
	}
	_, e = exclusive(filepath.Join(dir, markerName), []byte(markerContent))
	return e
}
