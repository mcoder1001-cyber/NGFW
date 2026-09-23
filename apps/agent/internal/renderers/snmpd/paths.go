package snmpd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// SnmpdBin is net-snmp's agent. The renderer runs it only for the Validate parse run (a
// daemonised check instance on a staged copy bound to a unix socket in the staging dir,
// killed right after). Listed in internal/renderers/ALLOWLIST.md.
const SnmpdBin = "/usr/sbin/snmpd"

// Binaries is the allowlist of the production SystemRunner of this renderer (snmpd for the
// parse run, systemctl for the product Controller).
func Binaries() []string { return []string{SnmpdBin, "/usr/bin/systemctl"} }

// Paths is every filesystem location the renderer uses; injected so tests never touch /etc/snmp.
type Paths struct {
	// ConfFile is the rendered snmpd.conf (holds communities and USM passphrases: 0600).
	ConfFile string
	// AgentXSocket is the AgentX master socket for the future private-MIB subagent (F-snmp).
	AgentXSocket string
	// FileOwner is the owner of snmpd.conf ("root:root" in the product, "" in tests).
	FileOwner string
	// FileMode is the mode of snmpd.conf (0600: it holds secrets).
	FileMode os.FileMode
}

// ProductPaths are the paths of the packaged net-snmp on Ubuntu 26.04.
func ProductPaths() Paths {
	return Paths{
		ConfFile:     "/etc/snmp/snmpd.conf",
		AgentXSocket: "/run/vrx/snmpd/agentx.sock",
		FileOwner:    "root:root",
		FileMode:     0o600,
	}
}

// TestPaths are the test-scoped paths for a slot prefix ("w8"): /run/vrx-test/<prefix>/snmpd.
func TestPaths(prefix string) Paths {
	base := filepath.Join("/run/vrx-test", prefix, "snmpd")
	return Paths{
		ConfFile:     filepath.Join(base, "snmpd.conf"),
		AgentXSocket: filepath.Join(base, "agentx.sock"),
		FileMode:     0o600,
	}
}

// pathRe is what a Paths entry may contain (rendered verbatim into snmpd.conf).
var pathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]{1,100}$`)

// Validate checks that every path is absolute, clean and made of safe characters, and that
// the file mode keeps the secrets private.
func (p Paths) Validate() error {
	for name, v := range map[string]string{"ConfFile": p.ConfFile, "AgentXSocket": p.AgentXSocket} {
		if !pathRe.MatchString(v) || filepath.Clean(v) != v {
			return fmt.Errorf("snmpd: Paths.%s %q must be an absolute clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o077 != 0 {
		return fmt.Errorf("snmpd: Paths.FileMode %v must be owner-only (the file holds secrets)", p.FileMode)
	}
	return nil
}
