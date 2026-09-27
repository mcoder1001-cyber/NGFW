package pppoe

import (
	"fmt"
	"path/filepath"

	"ngfw/agent/internal/renderers"
)

// PppdBin is the PPPoE dialer; SystemctlBin controls the per-session units.
const (
	PppdBin      = "/usr/sbin/pppd"
	SystemctlBin = "/usr/bin/systemctl"
)

// Binaries is the renderer's allow-list (helpers_exec, ALLOWLIST.md).
func Binaries() []string { return []string{PppdBin, SystemctlBin} }

// Paths are the file locations the renderer writes. All absolute and clean.
type Paths struct {
	// PeersDir holds one pppd peer file per session ("<PeersDir>/vrx-<hostif>").
	PeersDir string
	// ChapSecrets / PapSecrets are the shared secret files pppd reads (one line per session).
	ChapSecrets string
	PapSecrets  string
	// IPUpDir / IPDownDir hold the per-session hook pppd runs on link up/down.
	IPUpDir   string
	IPDownDir string
	// UnitDir holds the per-session systemd unit ("<UnitDir>/vrx-pppoe-<hostif>.service").
	UnitDir string
	// StateDir is where the hook writes "<hostif>.state" (read by the state reader).
	StateDir string
}

// ProductPaths are the packaged locations.
func ProductPaths() Paths {
	return Paths{ //nolint:gosec // G101: file paths, not credentials
		PeersDir:    "/etc/ppp/peers",
		ChapSecrets: "/etc/ppp/chap-secrets", //nolint:gosec // G101: a config file path, not a credential
		PapSecrets:  "/etc/ppp/pap-secrets",
		IPUpDir:     "/etc/ppp/ip-up.d",
		IPDownDir:   "/etc/ppp/ip-down.d",
		UnitDir:     "/etc/systemd/system",
		StateDir:    "/run/vrx/pppoe",
	}
}

// PathsUnder roots every path at base (tests; a slot's own tree).
func PathsUnder(base string) Paths {
	return Paths{
		PeersDir:    filepath.Join(base, "etc/ppp/peers"),
		ChapSecrets: filepath.Join(base, "etc/ppp/chap-secrets"),
		PapSecrets:  filepath.Join(base, "etc/ppp/pap-secrets"),
		IPUpDir:     filepath.Join(base, "etc/ppp/ip-up.d"),
		IPDownDir:   filepath.Join(base, "etc/ppp/ip-down.d"),
		UnitDir:     filepath.Join(base, "etc/systemd/system"),
		StateDir:    filepath.Join(base, "run/vrx/pppoe"),
	}
}

// Validate checks the paths are absolute (WriteFiles needs clean absolute targets).
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"PeersDir": p.PeersDir, "ChapSecrets": p.ChapSecrets, "PapSecrets": p.PapSecrets,
		"IPUpDir": p.IPUpDir, "IPDownDir": p.IPDownDir, "UnitDir": p.UnitDir, "StateDir": p.StateDir,
	} {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("%w: %s %q is not absolute", renderers.ErrInvalidFiles, name, v)
		}
	}
	return nil
}
