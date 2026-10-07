package pppoe

import (
	"fmt"
	"path/filepath"

	"ngfw/agent/internal/renderers"
)

// PppdBin is the PPPoE dialer; SystemctlBin controls the per-session units. DhcpcdBin is the DHCPv6 client the
// rendered ipv6-up hook starts for ipv6 "dhcpv6" (dhcpcd-base, Ubuntu priority important; the agent never execs it).
const (
	PppdBin      = "/usr/sbin/pppd"
	SystemctlBin = "/usr/bin/systemctl"
	DhcpcdBin    = "/usr/sbin/dhcpcd"
)

// Binaries is the renderer's allow-list (helpers_exec, ALLOWLIST.md).
func Binaries() []string { return []string{PppdBin, SystemctlBin} }

// Paths are the file locations the renderer writes. All absolute and clean.
type Paths struct {
	// PeersDir holds one pppd peer file per session ("<PeersDir>/ngfw-<hostif>").
	PeersDir string
	// ChapSecrets / PapSecrets are the shared secret files pppd reads (one line per session).
	ChapSecrets string
	PapSecrets  string
	// IPUpDir / IPDownDir hold the per-session hook pppd runs on link up/down.
	IPUpDir   string
	IPDownDir string
	// IPv6UpDir / IPv6DownDir hold the per-session IPv6CP hook (rendered only when the session's IPv6 is on).
	IPv6UpDir   string
	IPv6DownDir string
	// HelperDir holds per-session helpers that run-parts must not execute: the DHCPv6 client configuration
	// ("<HelperDir>/ngfw-dhcpcd-<hostif>.conf") and its event script ("<HelperDir>/ngfw-dhcp6-<hostif>").
	HelperDir string
	// UnitDir holds the per-session systemd unit ("<UnitDir>/ngfw-pppoe-<hostif>.service").
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
		IPv6UpDir:   "/etc/ppp/ipv6-up.d",
		IPv6DownDir: "/etc/ppp/ipv6-down.d",
		HelperDir:   "/etc/ppp",
		UnitDir:     "/etc/systemd/system",
		StateDir:    "/run/ngfw/pppoe",
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
		IPv6UpDir:   filepath.Join(base, "etc/ppp/ipv6-up.d"),
		IPv6DownDir: filepath.Join(base, "etc/ppp/ipv6-down.d"),
		HelperDir:   filepath.Join(base, "etc/ppp"),
		UnitDir:     filepath.Join(base, "etc/systemd/system"),
		StateDir:    filepath.Join(base, "run/ngfw/pppoe"),
	}
}

// Validate checks the paths are absolute (WriteFiles needs clean absolute targets).
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"PeersDir": p.PeersDir, "ChapSecrets": p.ChapSecrets, "PapSecrets": p.PapSecrets,
		"IPUpDir": p.IPUpDir, "IPDownDir": p.IPDownDir, "IPv6UpDir": p.IPv6UpDir, "IPv6DownDir": p.IPv6DownDir,
		"HelperDir": p.HelperDir, "UnitDir": p.UnitDir, "StateDir": p.StateDir,
	} {
		if !filepath.IsAbs(v) {
			return fmt.Errorf("%w: %s %q is not absolute", renderers.ErrInvalidFiles, name, v)
		}
	}
	return nil
}

// Dirs are the directories the rendered files and the hooks' state live in (created before writing).
func (p Paths) Dirs() []string {
	return []string{p.PeersDir, p.IPUpDir, p.IPDownDir, p.IPv6UpDir, p.IPv6DownDir, p.HelperDir, p.UnitDir, p.StateDir}
}

// dhcpcdConf / dhcp6Script are a session's DHCPv6 client configuration and event script.
func (p Paths) dhcpcdConf(hostIf string) string {
	return p.HelperDir + "/ngfw-dhcpcd-" + hostIf + ".conf"
}
func (p Paths) dhcp6Script(hostIf string) string { return p.HelperDir + "/ngfw-dhcp6-" + hostIf }
