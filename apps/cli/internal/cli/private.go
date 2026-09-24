package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Private files (session token, history, credential files) — review M1: never follow symlinks, never inherit a
// looser mode, never live in a directory someone else owns or can write, never in a shared temp directory.

// privateDir makes sure dir is a real directory owned by the current user with no group/other permissions,
// creating it (0700) when create is set.
func privateDir(dir string, create bool) error {
	if create {
		if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
			return err
		}
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory (symlink or file)", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is not owned by uid %d", dir, os.Getuid())
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s has mode %04o; it must be 0700", dir, fi.Mode().Perm())
	}
	return nil
}

// writePrivate replaces path atomically with data: a new 0600 temp file (O_EXCL) in the checked directory, then
// rename. An existing file, symlink or its mode is never reused.
func writePrivate(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := privateDir(dir, true); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// readPrivate reads a file that must be a regular file (no symlink) owned by the current user without group/other
// permissions; the checks and the read use the same open file.
func readPrivate(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //nolint:gosec // the user names the file; ownership and mode are checked on the open file
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("%s is a symlink; refused", path)
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		return nil, fmt.Errorf("%s is not owned by uid %d", path, os.Getuid())
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is accessible by group/others (mode %04o); chmod 600 it", path, fi.Mode().Perm())
	}
	return io.ReadAll(io.LimitReader(f, 1<<20))
}
