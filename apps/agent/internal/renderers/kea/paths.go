package kea

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"ngfw/agent/internal/renderers"
)

// Binaries the renderer invokes (listed in internal/renderers/ALLOWLIST.md). Fixed argv, no
// shell; rendered content travels through files (checkers) or the control socket (apply).
const (
	// Dhcp4Bin checks kea-dhcp4.conf (`-t <file>`).
	Dhcp4Bin = "/usr/sbin/kea-dhcp4"
	// Dhcp6Bin checks kea-dhcp6.conf (`-t <file>`).
	Dhcp6Bin = "/usr/sbin/kea-dhcp6"
	// IPBin runs the DHCP checkers inside Paths.Netns (`ip netns exec <ns> kea-dhcp4 -t
	// <file>`): Kea checks that a subnet's "interface" exists, so the checker must see the
	// namespace the server runs in. TEST-ONLY: `ip netns exec` can run any binary, so IPBin is
	// never part of Binaries()/NewRunner (the product runs Kea in the agent's namespace and
	// leaves Paths.Netns empty); the rig adds it to its own test runner.
	IPBin = "/usr/bin/ip"
)

// DefaultHooksDir is where Ubuntu 26.04 installs the Kea 3.0 hook libraries. It is not a
// binary: the renderer only names libdhcp_lease_cmds.so in the rendered config (Kea loads it).
const DefaultHooksDir = "/usr/lib/x86_64-linux-gnu/kea/hooks"

// LeaseCmdsHook is the hook library that provides lease4-get-page / lease6-get-page.
const LeaseCmdsHook = "libdhcp_lease_cmds.so"

// Binaries is the allowlist for the production SystemRunner of this renderer.
func Binaries() []string { return []string{Dhcp4Bin, Dhcp6Bin} }

// NewRunner returns the production runner: allow-listed binaries and the Kea 3.0 path
// environment (Env) so `kea-* -t` accepts the configured socket, lease and log directories.
func NewRunner(p Paths) *renderers.SystemRunner {
	r := renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...))
	r.Env = Env(p)
	return r
}

// Env is the environment for the Kea binaries. Kea ≥ 2.7.9 refuses control sockets, lease
// files and log files outside its compiled-in directories (/run/kea, /var/lib/kea,
// /var/log/kea) unless KEA_CONTROL_SOCKET_DIR / KEA_DHCP_DATA_DIR / KEA_LOG_FILE_DIR name
// others; the pidfile and lockfile directories follow KEA_PIDFILE_DIR / KEA_LOCKFILE_DIR.
func Env(p Paths) []string {
	return append(append([]string{}, renderers.DefaultEnv...),
		"KEA_CONTROL_SOCKET_DIR="+p.RunDir,
		"KEA_DHCP_DATA_DIR="+p.DataDir,
		"KEA_LOG_FILE_DIR="+p.LogDir,
		"KEA_PIDFILE_DIR="+p.RunDir,
		"KEA_LOCKFILE_DIR="+p.RunDir,
	)
}

// Paths is every filesystem location and listen parameter the renderer uses. It is injected
// so tests never touch the product paths (/etc/kea, /run/kea, /var/lib/kea).
type Paths struct {
	// ConfDir holds kea-dhcp4.conf, kea-dhcp6.conf and kea-ctrl-agent.conf.
	ConfDir string
	// RunDir holds the control sockets (Kea 3.0: must be mode 0750 or stricter).
	RunDir string
	// DataDir holds the memfile lease files.
	DataDir string
	// LogDir holds the daemon log files.
	LogDir string
	// HooksDir is searched for libdhcp_lease_cmds.so by New.
	HooksDir string
	// FileOwner owns the rendered files ("_kea:_kea" in the product, "" in tests).
	FileOwner string
	// FileMode of the rendered files (0640).
	FileMode os.FileMode
	// SocketType is interfaces-config.dhcp-socket-type for DHCPv4: "raw" (product: serves
	// clients without an address) or "udp".
	SocketType string
	// InterfacePrefix, when set, is required on every rendered interface name (tests: "w6-",
	// so a test rendering can never bind a host NIC such as ens192).
	InterfacePrefix string
	// LFCInterval is the memfile lease-file cleanup interval in seconds.
	LFCInterval uint32
	// Netns is the network namespace the DHCP servers run in ("" = the agent's own; always
	// empty in the product). The DHCP checkers run there too, through IPBin, which only a test
	// runner allows.
	Netns string
}

// ProductPaths are the paths of the packaged Kea 3.0 on Ubuntu 26.04.
func ProductPaths() Paths {
	return Paths{
		ConfDir:     "/etc/kea",
		RunDir:      "/run/kea",
		DataDir:     "/var/lib/kea",
		LogDir:      "/var/log/kea",
		HooksDir:    DefaultHooksDir,
		FileOwner:   "_kea:_kea",
		FileMode:    0o640,
		SocketType:  "raw",
		LFCInterval: 3600,
	}
}

// TestPaths are the test-scoped paths for slot prefix ("w6"): everything
// under /run/vrx-test/<prefix>/kea, interfaces "<prefix>-*", servers in ns-<prefix>-a.
func TestPaths(prefix string) Paths {
	base := filepath.Join("/run/vrx-test", prefix, "kea")
	return Paths{
		ConfDir:         filepath.Join(base, "etc"),
		RunDir:          filepath.Join(base, "run"),
		DataDir:         filepath.Join(base, "lib"),
		LogDir:          filepath.Join(base, "log"),
		HooksDir:        DefaultHooksDir,
		FileMode:        0o640,
		SocketType:      "raw",
		InterfacePrefix: prefix + "-",
		LFCInterval:     3600,
		Netns:           "ns-" + prefix + "-a",
	}
}

var (
	safePathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]*$`)
	netnsRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
)

// Validate checks that every path is absolute, clean and made of safe characters.
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"ConfDir": p.ConfDir, "RunDir": p.RunDir, "DataDir": p.DataDir, "LogDir": p.LogDir, "HooksDir": p.HooksDir,
	} {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v || !safePathRe.MatchString(v) {
			return fmt.Errorf("kea: Paths.%s %q must be an absolute, clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o007 != 0 {
		return fmt.Errorf("kea: Paths.FileMode %v must be permission bits and not world-accessible", p.FileMode)
	}
	if p.SocketType != "raw" && p.SocketType != "udp" {
		return fmt.Errorf("kea: Paths.SocketType %q must be raw or udp", p.SocketType)
	}
	if p.Netns != "" && !netnsRe.MatchString(p.Netns) {
		return fmt.Errorf("kea: Paths.Netns %q must match %s", p.Netns, netnsRe)
	}
	if p.InterfacePrefix != "" && !ifNameRe.MatchString(p.InterfacePrefix+"x") {
		return fmt.Errorf("kea: Paths.InterfacePrefix %q is not an interface-name prefix", p.InterfacePrefix)
	}
	return nil
}

// Dhcp4Conf is kea-dhcp4.conf.
func (p Paths) Dhcp4Conf() string { return filepath.Join(p.ConfDir, "kea-dhcp4.conf") }

// Dhcp6Conf is kea-dhcp6.conf.
func (p Paths) Dhcp6Conf() string { return filepath.Join(p.ConfDir, "kea-dhcp6.conf") }

// Socket4 is the DHCPv4 server's unix control socket.
func (p Paths) Socket4() string { return filepath.Join(p.RunDir, "kea4.sock") }

// Socket6 is the DHCPv6 server's unix control socket.
func (p Paths) Socket6() string { return filepath.Join(p.RunDir, "kea6.sock") }

// Leases4 is the DHCPv4 memfile.
func (p Paths) Leases4() string { return filepath.Join(p.DataDir, "leases4.csv") }

// Leases6 is the DHCPv6 memfile.
func (p Paths) Leases6() string { return filepath.Join(p.DataDir, "leases6.csv") }

// Log4 is kea-dhcp4's log file.
func (p Paths) Log4() string { return filepath.Join(p.LogDir, "kea-dhcp4.log") }

// Log6 is kea-dhcp6's log file.
func (p Paths) Log6() string { return filepath.Join(p.LogDir, "kea-dhcp6.log") }

// socket returns the control socket of family 4 or 6.
func (p Paths) socket(family int) string {
	if family == 6 {
		return p.Socket6()
	}
	return p.Socket4()
}

// conf returns the config file of family 4 or 6.
func (p Paths) conf(family int) string {
	if family == 6 {
		return p.Dhcp6Conf()
	}
	return p.Dhcp4Conf()
}
