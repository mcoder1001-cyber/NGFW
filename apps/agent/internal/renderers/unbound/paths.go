package unbound

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"

	"ngfw/agent/internal/renderers"
)

// Binaries the renderer invokes (listed in internal/renderers/ALLOWLIST.md). Fixed argv, no
// shell; the rendered unbound.conf travels through a file.
const (
	// CheckconfBin validates unbound.conf (`unbound-checkconf <file>`).
	CheckconfBin = "/usr/sbin/unbound-checkconf"
	// ControlBin reloads and reads state (`unbound-control -c <conf> <command>`).
	ControlBin = "/usr/sbin/unbound-control"
)

// Binaries is the allowlist for the production SystemRunner of this renderer.
func Binaries() []string { return []string{CheckconfBin, ControlBin} }

// NewRunner returns the production runner (allow-listed binaries, minimal environment).
func NewRunner() *renderers.SystemRunner {
	return renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...))
}

// Paths is every filesystem location the renderer uses. It is injected so tests never touch
// the product paths (/etc/unbound, /run/unbound, /var/lib/unbound).
type Paths struct {
	// ConfDir holds unbound.conf; it is also unbound's `directory:`.
	ConfDir string
	// ControlSocketPath is the unix remote-control socket and PidFilePath the pidfile. The
	// product uses Debian's locations directly in /run (/run/unbound.ctl, /run/unbound.pid):
	// the packaged unbound.service has no RuntimeDirectory, so /run/unbound would not exist
	// (review M4). Apply creates missing parent directories anyway (0755, root).
	ControlSocketPath string
	PidFilePath       string
	// PendingFile persists a restart request until unbound runs the new configuration
	// (D-079, review M2); /run so that a reboot — which restarts unbound — clears it.
	PendingFile string
	// IdlePort is the loopback port of an instance without enabled resolvers (product 53,
	// tests the slot port: never 127.0.0.1:53 on the shared host, review L5).
	IdlePort uint32
	// TrustAnchor is the RFC 5011 auto-trust-anchor-file (writable by unbound; must exist —
	// Debian's unbound-anchor seeds it from dns-root-data).
	TrustAnchor string
	// RootKey is the static root trust anchor used when automatic updates are off.
	RootKey string
	// TLSCertBundle verifies DNS-over-TLS upstreams.
	TLSCertBundle string
	// Username unbound drops privileges to ("unbound" in the product, "" in tests).
	Username string
	// LogFile, when set, replaces syslog (tests).
	LogFile string
	// FileOwner / FileMode of unbound.conf ("root:unbound" 0640 in the product).
	FileOwner string
	FileMode  os.FileMode
	// LoopbackOnly restricts every listen address to loopback (tests: a test rendering can
	// never bind a host NIC such as ens192).
	LoopbackOnly bool
}

// ProductPaths are the paths of the packaged Unbound 1.24 on Ubuntu 26.04.
func ProductPaths() Paths {
	return Paths{
		ConfDir:           "/etc/unbound",
		ControlSocketPath: "/run/unbound.ctl",
		PidFilePath:       "/run/unbound.pid",
		PendingFile:       "/run/vrx/renderers/unbound.pending",
		IdlePort:          53,
		TrustAnchor:       "/var/lib/unbound/root.key",
		RootKey:           "/usr/share/dns/root.key",
		TLSCertBundle:     "/etc/ssl/certs/ca-certificates.crt",
		Username:          "unbound",
		FileOwner:         "root:unbound",
		FileMode:          0o640,
	}
}

// TestPaths are the test-scoped paths for slot prefix ("w6") and slot number: everything
// under /run/vrx-test/<prefix>/unbound, loopback listeners only, idle port 3<slot>53.
func TestPaths(prefix string, slot int) Paths {
	return PathsUnder(filepath.Join("/run/vrx-test", prefix, "unbound"), uint32(3000+slot*100+53)) //nolint:gosec // slots 1–12
}

// PathsUnder are TestPaths rooted at base (an agent that is not the globals owner renders its slot-local instance
// there, F-unbound-chrony-syslog): everything under base, loopback listeners only, the given idle port.
func PathsUnder(base string, idlePort uint32) Paths {
	return Paths{
		ConfDir:           base,
		ControlSocketPath: filepath.Join(base, "unbound.ctl"),
		PidFilePath:       filepath.Join(base, "unbound.pid"),
		PendingFile:       filepath.Join(base, "vrx.pending"),
		IdlePort:          idlePort,
		TrustAnchor:       filepath.Join(base, "root.key"),
		RootKey:           "/usr/share/dns/root.key",
		TLSCertBundle:     "/etc/ssl/certs/ca-certificates.crt",
		LogFile:           filepath.Join(base, "unbound.log"),
		FileMode:          0o640,
		LoopbackOnly:      true,
	}
}

var (
	safePathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]*$`)
	userRe     = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

// Validate checks every path (absolute, clean, safe characters: they are rendered into the
// config inside quotes) and the user name.
func (p Paths) Validate() error {
	paths := map[string]string{
		"ConfDir": p.ConfDir, "ControlSocketPath": p.ControlSocketPath, "PidFilePath": p.PidFilePath,
		"PendingFile": p.PendingFile, "TrustAnchor": p.TrustAnchor, "RootKey": p.RootKey,
		"TLSCertBundle": p.TLSCertBundle,
	}
	if p.LogFile != "" {
		paths["LogFile"] = p.LogFile
	}
	for name, v := range paths {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v || !safePathRe.MatchString(v) {
			return fmt.Errorf("unbound: Paths.%s %q must be an absolute, clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.IdlePort == 0 || p.IdlePort > 65535 {
		return fmt.Errorf("unbound: Paths.IdlePort %d outside 1..65535", p.IdlePort)
	}
	if p.Username != "" && !userRe.MatchString(p.Username) {
		return fmt.Errorf("unbound: Paths.Username %q is not a user name", p.Username)
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o007 != 0 {
		return fmt.Errorf("unbound: Paths.FileMode %v must be permission bits and not world-accessible", p.FileMode)
	}
	return nil
}

// Conf is unbound.conf.
func (p Paths) Conf() string { return filepath.Join(p.ConfDir, "unbound.conf") }

// PidFile is unbound's pidfile.
func (p Paths) PidFile() string { return p.PidFilePath }

// ControlSocket is the unix remote-control socket.
func (p Paths) ControlSocket() string { return p.ControlSocketPath }

// listenAllowed applies LoopbackOnly.
func (p Paths) listenAllowed(a netip.Addr) bool { return !p.LoopbackOnly || a.IsLoopback() }
