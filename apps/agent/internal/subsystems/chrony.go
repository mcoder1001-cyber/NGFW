package subsystems

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"ngfw/agent/internal/renderers/chrony"
)

// newChrony returns the chrony renderer of env: product paths for the globals owner, else the slot's
// (<slot dir>/chrony/agent, no clock control, loopback only) and the hook that prepares it. chronyd refuses a
// command-socket directory it does not own and drops to _chrony before reading the sources, so the slot directory is
// _chrony:_chrony 0750 (the RF-3 test layout).
func newChrony(env Env) (*chrony.Renderer, func() error) {
	if env.GlobalsOwner {
		return chrony.New(chrony.NewRunner()), nil
	}
	p := chrony.PathsUnder(filepath.Join(slotRunDir(env.Owner), "chrony", "agent"))
	prep := func() error {
		if err := mkdirShared(filepath.Dir(p.ConfDir)); err != nil {
			return fmt.Errorf("chrony slot dir: %w", err)
		}
		u, err := user.Lookup("_chrony")
		if err != nil {
			return fmt.Errorf("chrony slot dir: user _chrony: %w", err)
		}
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		for _, d := range []string{p.ConfDir, p.LogDir} {
			if err := os.MkdirAll(d, 0o750); err != nil {
				return fmt.Errorf("chrony slot dir: %w", err)
			}
			if err := os.Chown(d, uid, gid); err != nil {
				return fmt.Errorf("chrony slot dir: %w", err)
			}
			if err := os.Chmod(d, 0o750); err != nil { //nolint:gosec // _chrony must traverse it
				return fmt.Errorf("chrony slot dir: %w", err)
			}
		}
		if err := os.MkdirAll(p.SourceDir(), 0o755); err != nil { //nolint:gosec // chronyd reads sourcedir as _chrony
			return fmt.Errorf("chrony slot dir: %w", err)
		}
		return nil
	}
	return chrony.New(chrony.NewRunner(), chrony.WithPaths(p)), prep
}
