package strongswan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// SwanctlBin is the only binary the renderer itself may run (listed in
// internal/renderers/ALLOWLIST.md): the optional integration checker in Validate
// (WithChecker) loads the staged files into a scratch charon. Apply, Retrieve and events use
// the VICI socket directly (govici), never a process.
const SwanctlBin = "/usr/sbin/swanctl"

// Binaries is the allowlist for the SystemRunner of this renderer.
func Binaries() []string { return []string{SwanctlBin} }

// Paths is every filesystem location the renderer uses. It is injected so tests never touch
// the product paths (/etc/strongswan.conf, /etc/swanctl, /var/run/charon.vici).
type Paths struct {
	// StrongswanConf is the daemon config (charon reads it at start; STRONGSWAN_CONF in tests).
	StrongswanConf string
	// SwanctlDir is the swanctl directory: conf.d/vrx.conf and conf.d/vrx-secrets.conf are
	// rendered below it; x509/, x509ca/ and pubkey/ hold certificate files (F-pki-basic).
	// The packaged <SwanctlDir>/swanctl.conf (`include conf.d/*.conf`) loads them at boot.
	SwanctlDir string
	// ViciSocket is charon's VICI socket path (charon.plugins.vici.socket and swanctl.socket).
	ViciSocket string
	// LogFile is charon's file log ("" = no filelog section; the product logs to the journal).
	LogFile string
	// FileOwner is the owner of the rendered files ("root:root" in the product, "" in tests).
	FileOwner string
	// ConfMode is the mode of strongswan.conf and vrx.conf (0640).
	ConfMode os.FileMode
	// SecretMode is the mode of vrx-secrets.conf (0600; never world- or group-readable).
	SecretMode os.FileMode
}

// ProductPaths are the paths of the packaged strongSwan on Ubuntu 26.04 (and of the P11
// vrx-strongswan package, which keeps them).
func ProductPaths() Paths {
	return Paths{
		StrongswanConf: "/etc/strongswan.conf",
		SwanctlDir:     "/etc/swanctl",
		ViciSocket:     "/var/run/charon.vici",
		FileOwner:      "root:root",
		ConfMode:       0o640,
		SecretMode:     0o600,
	}
}

// testInstanceRe bounds the instance name of TestPaths ("a", "b", "v", …).
var testInstanceRe = regexp.MustCompile(`^[a-z0-9]{1,8}$`)

// TestPaths are the test-scoped paths for slot prefix ("w3") and daemon instance ("a"):
// everything under /run/vrx-test/<prefix>/swan/<instance>, files owned by the test process.
func TestPaths(prefix, instance string) Paths {
	base := filepath.Join("/run/vrx-test", prefix, "swan", instance)
	return Paths{
		StrongswanConf: filepath.Join(base, "strongswan.conf"),
		SwanctlDir:     filepath.Join(base, "swanctl"),
		ViciSocket:     filepath.Join(base, "charon.vici"),
		LogFile:        filepath.Join(base, "charon.log"),
		ConfMode:       0o640,
		SecretMode:     0o600,
	}
}

// ConnsFile is the rendered connections file.
func (p Paths) ConnsFile() string { return filepath.Join(p.SwanctlDir, "conf.d", "vrx.conf") }

// SecretsFile is the rendered secrets file (mode SecretMode).
func (p Paths) SecretsFile() string { return filepath.Join(p.SwanctlDir, "conf.d", "vrx-secrets.conf") }

// SwanctlConf is the packaged top-level swanctl.conf (`include conf.d/*.conf`); not rendered.
func (p Paths) SwanctlConf() string { return filepath.Join(p.SwanctlDir, "swanctl.conf") }

// CertDir returns the swanctl directory holding files of a file-list key (certs → x509,
// cacerts/cacert → x509ca, pubkeys → pubkey), as swanctl resolves relative names.
func (p Paths) CertDir(key string) string {
	switch key {
	case "certs":
		return filepath.Join(p.SwanctlDir, "x509")
	case "cacerts", "cacert":
		return filepath.Join(p.SwanctlDir, "x509ca")
	case "pubkeys":
		return filepath.Join(p.SwanctlDir, "pubkey")
	}
	return ""
}

// pathRe is what a path may look like when it is rendered into strongswan.conf unquoted.
var pathRe = regexp.MustCompile(`^/[A-Za-z0-9_./+-]{0,254}$`)

// Validate checks the paths are absolute, clean and renderable.
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"StrongswanConf": p.StrongswanConf, "SwanctlDir": p.SwanctlDir, "ViciSocket": p.ViciSocket,
	} {
		if !pathRe.MatchString(v) || filepath.Clean(v) != v {
			return fmt.Errorf("strongswan: path %s %q must be absolute, clean and match %s", name, v, pathRe)
		}
	}
	if p.LogFile != "" && (!pathRe.MatchString(p.LogFile) || filepath.Clean(p.LogFile) != p.LogFile) {
		return fmt.Errorf("strongswan: path LogFile %q must be absolute, clean and match %s", p.LogFile, pathRe)
	}
	if p.ConfMode == 0 || p.ConfMode&0o007 != 0 {
		return fmt.Errorf("strongswan: ConfMode %v must be set and not world-accessible", p.ConfMode)
	}
	if p.SecretMode == 0 || p.SecretMode&0o077 != 0 {
		return fmt.Errorf("strongswan: SecretMode %v must be set and owner-only", p.SecretMode)
	}
	return nil
}
