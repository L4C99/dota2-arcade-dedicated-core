package a2s

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Result struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	Backup string `json:"backup,omitempty"`
}

func Enable(dotaDir string) (Result, error) { return enable(dotaDir, nil) }

func readBounded(path string) ([]byte, os.FileInfo, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > 4<<20 {
		return nil, nil, fmt.Errorf("expected regular gameinfo/backup no larger than 4 MiB")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if len(b) > 4<<20 {
		return nil, nil, fmt.Errorf("file grew past limit")
	}
	return b, st, e
}

func enable(dotaDir string, hook func(string) error) (r Result, err error) {
	r.Status = "error"
	if !filepath.IsAbs(dotaDir) {
		return r, fmt.Errorf("absolute dota-dir required")
	}
	dotaDir = filepath.Clean(dotaDir)
	r.Path = filepath.Join(dotaDir, "game", "dota", "gameinfo.gi")
	if e := plainInstallationPath(r.Path); e != nil {
		return r, e
	}
	lockPath := r.Path + ".d2core-a2s.lock"
	lock, e := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		if os.IsExist(e) {
			r.Status = "conflict"
		}
		return r, fmt.Errorf("configuration lock unavailable: %w", e)
	}
	lockInfo, e := lock.Stat()
	lock.Close()
	if e != nil {
		return r, e
	}
	defer func() {
		if st, e := os.Lstat(lockPath); e == nil && os.SameFile(st, lockInfo) {
			os.Remove(lockPath)
		}
	}()
	before, identity, e := readBounded(r.Path)
	if e != nil {
		return r, e
	}
	after, changed, e := Merge(before)
	if e != nil {
		r.Status = "conflict"
		return r, e
	}
	if !changed {
		r.Status = "already_configured"
		return r, nil
	}
	digest := sha256.Sum256(before)
	r.Backup = r.Path + ".d2core-" + hex.EncodeToString(digest[:]) + ".bak"
	backup, e := os.OpenFile(r.Backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		b, _, readErr := readBounded(r.Backup)
		if readErr != nil || !bytes.Equal(b, before) {
			r.Status = "conflict"
			return r, fmt.Errorf("existing backup does not match source")
		}
	} else if e != nil {
		return r, e
	} else {
		_, e = backup.Write(before)
		if e == nil {
			e = backup.Sync()
		}
		closeErr := backup.Close()
		if e != nil {
			return r, e
		}
		if closeErr != nil {
			return r, closeErr
		}
		if e = syncDir(filepath.Dir(r.Path)); e != nil {
			return r, e
		}
	}
	if hook != nil {
		if e = hook("backup-saved"); e != nil {
			return r, e
		}
	}
	f, e := os.CreateTemp(filepath.Dir(r.Path), ".d2core-a2s-*")
	if e != nil {
		return r, e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, e = f.Write(after)
	if e == nil {
		e = f.Chmod(identity.Mode().Perm())
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return r, e
	}
	if closeErr != nil {
		return r, closeErr
	}
	if hook != nil {
		if e = hook("before-replace"); e != nil {
			return r, e
		}
	}
	current, currentID, e := readBounded(r.Path)
	if e != nil || !os.SameFile(identity, currentID) || !bytes.Equal(current, before) {
		r.Status = "conflict"
		return r, fmt.Errorf("gameinfo changed during preparation; backup retained")
	}
	if e = replace(tmp, r.Path); e != nil {
		return r, e
	}
	if hook != nil {
		if e = hook("after-replace"); e != nil {
			return r, e
		}
	}
	verified, _, e := readBounded(r.Path)
	if e != nil {
		return r, e
	}
	if !bytes.Equal(verified, after) {
		r.Status = "conflict"
		return r, fmt.Errorf("post-write verification mismatch; backup retained")
	}
	r.Status = "inserted"
	return r, nil
}
