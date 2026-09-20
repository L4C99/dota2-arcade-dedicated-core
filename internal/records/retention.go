package records

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PruneExpired removes at most limit reclaimed histories per call. Files are
// removed before publishing metadata removal: interruptions keep retry ownership.
// Nothing with a live/unknown process or incomplete cleanup is eligible.
func (s *Store) PruneExpired(now time.Time, age time.Duration, limit int) (int, error) {
	if age <= 0 || limit < 1 {
		return 0, fmt.Errorf("invalid history policy")
	}
	ids := []string{}
	for id, in := range s.State.Instances {
		if in.Lifecycle == "reclaimed" && in.Process == "stopped" && in.Cleanup == "complete" && !now.Before(in.UpdatedAt.Add(age)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	start := sort.Search(len(ids), func(i int) bool { return ids[i] > s.pruneAfter })
	ids = append(ids[start:], ids[:start]...)
	count := 0
	var cleanupErr error
	for index, id := range ids {
		if index == limit {
			break
		}
		s.pruneAfter = id
		in := s.State.Instances[id]
		if err := s.removeHistoryFiles(in); err != nil {
			cleanupErr = errors.Join(cleanupErr, &Failure{Code: "CLEANUP_FAILED", Stage: "cleanup", Message: err.Error(), InstanceID: id})
			continue
		}
		ops := map[string]*Operation{}
		keys := map[string]Key{}
		for k, v := range s.State.Operations {
			if v.InstanceID == id {
				ops[k] = v
				delete(s.State.Operations, k)
			}
		}
		for k, v := range s.State.Keys {
			if v.InstanceID == id {
				keys[k] = v
				delete(s.State.Keys, k)
			}
		}
		delete(s.State.Instances, id)
		if err := s.Save(); err != nil {
			// Preserve the in-memory identity on uncertain publication too. The
			// manager must stop mutations and reload rather than reuse the key.
			s.State.Instances[id] = in
			for k, v := range ops {
				s.State.Operations[k] = v
			}
			for k, v := range keys {
				s.State.Keys[k] = v
			}
			return count, &Failure{Code: "IO_ERROR", Stage: "persist", Message: err.Error(), cause: err}
		}
		count++
	}
	return count, cleanupErr
}

func (s *Store) removeHistoryFiles(in *Instance) error {
	if !validID(in.ID, "i") || s.State.Instances[in.ID] != in {
		return fmt.Errorf("unowned history")
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	base := filepath.Join("instances", in.ID)
	// Refuse symlinks and unexpected entries, including parent junctions.
	for _, dir := range []string{"instances", base, filepath.Join(base, "runs")} {
		st, e := root.Lstat(dir)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("history directory replaced: %s", dir)
		}
	}
	allowed := map[string]bool{base: true, filepath.Join(base, "runs"): true}
	files := []string{}
	dirs := []string{}
	for _, r := range in.Runs {
		dir := filepath.Join(base, "runs", fmt.Sprintf("%06d", r.Generation))
		if filepath.Join(s.Dir, dir) != r.Directory {
			return fmt.Errorf("history generation path mismatch")
		}
		allowed[dir] = true
		dirs = append(dirs, dir)
		for _, name := range []string{"engine.log", "output.log", "generated.cfg"} {
			p := filepath.Join(dir, name)
			allowed[p] = true
			files = append(files, p)
		}
	}
	// The private rooted filesystem prevents deletion outside data-dir even if
	// paths race. No recursive deletion and no following links during inspection.
	err = inspectHistory(root, base, allowed)
	if err != nil {
		return err
	}
	for _, r := range in.Runs {
		p := filepath.Join(base, "runs", fmt.Sprintf("%06d", r.Generation), "generated.cfg")
		f, e := root.Open(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(f, 1<<20))
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if digest(b) != r.CFGDigest {
			return fmt.Errorf("history cfg changed; refusing removal")
		}
	}
	for _, p := range files {
		if e := root.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	for _, p := range append(dirs, filepath.Join(base, "runs"), base) {
		if e := root.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	if _, e := root.Stat("instances"); errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return syncDirectory(filepath.Join(s.Dir, "instances"))
}

func inspectHistory(root *os.Root, path string, allowed map[string]bool) error {
	st, e := root.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	if !allowed[path] || st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unexpected history entry: %s", path)
	}
	if st.IsDir() {
		f, e := root.Open(path)
		if e != nil {
			return e
		}
		entries, e := f.ReadDir(4097)
		f.Close()
		if e != nil && !errors.Is(e, io.EOF) {
			return e
		}
		if len(entries) > 4096 {
			return fmt.Errorf("history directory exceeds bounded inspection")
		}
		for _, entry := range entries {
			if e = inspectHistory(root, filepath.Join(path, entry.Name()), allowed); e != nil {
				return e
			}
		}
		return nil
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("non-regular history entry: %s", path)
	}
	return nil
}
