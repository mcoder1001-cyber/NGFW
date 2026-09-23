package renderers

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// WriteFileAtomic writes f to path atomically: the content goes to a temporary file in the
// same directory, is fsynced, gets its mode and owner, and is renamed over path; the
// directory is fsynced afterwards. Readers (and the daemon) see either the old or the new
// file, never a partial one. The parent directory must exist.
func WriteFileAtomic(path string, f File) error {
	if err := (Files{path: f}).Validate(); err != nil {
		return err
	}
	dir, base := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("renderers: create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(f.Content); err != nil {
		return cleanup(fmt.Errorf("renderers: write %s: %w", path, err))
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(fmt.Errorf("renderers: fsync %s: %w", path, err))
	}
	if err := tmp.Chmod(f.Mode); err != nil {
		return cleanup(fmt.Errorf("renderers: chmod %s: %w", path, err))
	}
	if err := chown(tmpName, f.Owner); err != nil {
		return cleanup(fmt.Errorf("renderers: chown %s: %w", path, err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renderers: close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("renderers: rename over %s: %w", path, err)
	}
	return syncDir(dir)
}

// WriteFiles writes every file atomically in Paths() order and stops at the first error.
// Take a Snapshot first so the caller can Restore on failure.
func WriteFiles(files Files) error {
	if err := files.Validate(); err != nil {
		return err
	}
	for _, p := range files.Paths() {
		if err := WriteFileAtomic(p, files[p]); err != nil {
			return err
		}
	}
	return nil
}

// syncDir fsyncs a directory so a rename is durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("renderers: open dir %s: %w", dir, err)
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		// Some filesystems (tmpfs on older kernels, overlays) refuse fsync on directories.
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) {
			return nil
		}
		return fmt.Errorf("renderers: fsync dir %s: %w", dir, err)
	}
	return d.Close()
}

// chown applies an "user[:group]" owner spec by name; "" is a no-op. Needs privileges to
// change to another user (the agent runs as root).
func chown(path, owner string) error {
	if owner == "" {
		return nil
	}
	uid, gid, err := lookupOwner(owner)
	if err != nil {
		return err
	}
	return os.Lchown(path, uid, gid)
}

// lookupOwner resolves "user[:group]" (names or numeric ids) to uid/gid. Without a group
// the user's primary group is used.
func lookupOwner(owner string) (uid, gid int, err error) {
	userName, groupName, hasGroup := strings.Cut(owner, ":")
	if userName == "" {
		return 0, 0, fmt.Errorf("owner %q has no user", owner)
	}
	u, err := user.Lookup(userName)
	if err != nil {
		if id, convErr := strconv.Atoi(userName); convErr == nil {
			u, err = user.LookupId(strconv.Itoa(id))
		}
		if err != nil {
			return 0, 0, fmt.Errorf("owner %q: %w", owner, err)
		}
	}
	if uid, err = strconv.Atoi(u.Uid); err != nil {
		return 0, 0, fmt.Errorf("owner %q: uid %q: %w", owner, u.Uid, err)
	}
	gidStr := u.Gid
	if hasGroup {
		if groupName == "" {
			return 0, 0, fmt.Errorf("owner %q has an empty group", owner)
		}
		g, gerr := user.LookupGroup(groupName)
		if gerr != nil {
			if _, convErr := strconv.Atoi(groupName); convErr != nil {
				return 0, 0, fmt.Errorf("owner %q: %w", owner, gerr)
			}
			gidStr = groupName
		} else {
			gidStr = g.Gid
		}
	}
	if gid, err = strconv.Atoi(gidStr); err != nil {
		return 0, 0, fmt.Errorf("owner %q: gid %q: %w", owner, gidStr, err)
	}
	return uid, gid, nil
}

// Snapshot remembers the state of a set of paths (content, mode, owner, or absence) so a
// failed Apply can put them back exactly (AD-4 step 3).
type Snapshot struct {
	entries map[string]snapEntry
}

type snapEntry struct {
	existed bool
	content []byte
	mode    os.FileMode
	uid     int
	gid     int
}

// TakeSnapshot records the current state of paths. Missing files are recorded as absent and
// removed again by Restore. Directories are rejected.
func TakeSnapshot(paths ...string) (*Snapshot, error) {
	s := &Snapshot{entries: make(map[string]snapEntry, len(paths))}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("%w: snapshot path %q is not absolute", ErrInvalidFiles, p)
		}
		info, err := os.Lstat(p)
		switch {
		case errors.Is(err, os.ErrNotExist):
			s.entries[p] = snapEntry{existed: false}
			continue
		case err != nil:
			return nil, fmt.Errorf("renderers: snapshot %s: %w", p, err)
		case !info.Mode().IsRegular():
			return nil, fmt.Errorf("renderers: snapshot %s: not a regular file (%v)", p, info.Mode())
		}
		content, err := os.ReadFile(p) //nolint:gosec // path comes from the renderer's own file set
		if err != nil {
			return nil, fmt.Errorf("renderers: snapshot %s: %w", p, err)
		}
		e := snapEntry{existed: true, content: content, mode: info.Mode().Perm(), uid: -1, gid: -1}
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			e.uid, e.gid = int(st.Uid), int(st.Gid) //nolint:gosec // uid/gid fit in int
		}
		s.entries[p] = e
	}
	return s, nil
}

// Paths returns the snapshotted paths in sorted order.
func (s *Snapshot) Paths() []string {
	out := make([]string, 0, len(s.entries))
	for p := range s.entries {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Restore puts every path back as it was: rewriting content, mode and owner atomically, or
// removing files that did not exist. It continues past errors and returns them joined.
func (s *Snapshot) Restore() error {
	var errs []error
	for _, p := range s.Paths() {
		e := s.entries[p]
		if !e.existed {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("renderers: restore remove %s: %w", p, err))
			}
			continue
		}
		if err := WriteFileAtomic(p, File{Mode: e.mode, Content: e.content}); err != nil {
			errs = append(errs, err)
			continue
		}
		if e.uid >= 0 && e.gid >= 0 {
			if err := os.Lchown(p, e.uid, e.gid); err != nil {
				errs = append(errs, fmt.Errorf("renderers: restore owner %s: %w", p, err))
			}
		}
	}
	return errors.Join(errs...)
}

// Staging is a temporary directory tree holding a copy of a Files set, for Validate:
// /etc/frr/frr.conf is staged at <Dir>/etc/frr/frr.conf so daemon checkers can be pointed at
// it without touching the live paths.
type Staging struct {
	Dir string
}

// Stage writes files under a fresh temporary directory (mode 0700) and returns the staging.
// Owners are not applied (the checker runs as the agent). Call Close to remove it.
func Stage(files Files) (*Staging, error) {
	if err := files.Validate(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "vrx-render-")
	if err != nil {
		return nil, fmt.Errorf("renderers: staging dir: %w", err)
	}
	st := &Staging{Dir: dir}
	for _, p := range files.Paths() {
		target := st.Path(p)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			_ = st.Close()
			return nil, fmt.Errorf("renderers: staging mkdir for %s: %w", p, err)
		}
		f := files[p]
		f.Owner = ""
		if err := WriteFileAtomic(target, f); err != nil {
			_ = st.Close()
			return nil, err
		}
	}
	return st, nil
}

// Path returns where the live path is staged.
func (s *Staging) Path(live string) string {
	return filepath.Join(s.Dir, filepath.Clean(live))
}

// Close removes the staging directory.
func (s *Staging) Close() error {
	if s == nil || s.Dir == "" {
		return nil
	}
	err := os.RemoveAll(s.Dir)
	s.Dir = ""
	return err
}
