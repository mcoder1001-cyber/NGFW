package frr

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Binaries the renderer invokes (listed in internal/renderers/ALLOWLIST.md). Fixed argv, no
// shell; rendered content travels through files only.
const (
	// VtyshBin validates (-C -f) and reads state (-c "show … json").
	VtyshBin = "/usr/bin/vtysh"
	// ReloadBin applies a rendered frr.conf as a diff against the running config.
	ReloadBin = "/usr/lib/frr/frr-reload.py"
)

// Binaries is the allowlist for the production SystemRunner of this renderer.
func Binaries() []string { return []string{VtyshBin, ReloadBin} }

// Paths is every filesystem location the renderer uses. It is injected so tests never touch
// the product paths (/etc/frr, /var/run/frr).
//
// FRR's pathspace (-N <ns>) appends "/<ns>" to the config and socket directories of vtysh,
// frr-reload.py and the daemons, so with Namespace "w12" the integrated config is
// <ConfDir>/w12/frr.conf and the vty sockets live in <RunDir>/w12/.
type Paths struct {
	// ConfDir is the FRR config directory (vtysh --config_dir, frr-reload --confdir).
	ConfDir string
	// RunDir is the FRR state directory holding the vty sockets (vtysh --vty_socket).
	RunDir string
	// Namespace is the FRR pathspace (-N); "" in the product.
	Namespace string
	// BinDir is the directory holding vtysh, handed to frr-reload.py --bindir.
	BinDir string
	// ReloadLog is frr-reload.py's --logfile (its default is /var/log/frr/frr-reload.log).
	ReloadLog string
	// FileOwner is the owner of the rendered files ("frr:frr" in the product, "" in tests).
	FileOwner string
	// FileMode is the mode of the rendered files (0640: frr.conf may later hold BGP passwords).
	FileMode os.FileMode
}

// ProductPaths are the paths of the packaged FRR on Ubuntu 26.04.
func ProductPaths() Paths {
	return Paths{
		ConfDir:   "/etc/frr",
		RunDir:    "/var/run/frr",
		BinDir:    "/usr/bin",
		ReloadLog: "/var/log/frr/frr-reload.log",
		FileOwner: "frr:frr",
		FileMode:  0o640,
	}
}

// TestPaths are the test-scoped paths for slot prefix ("w12"): everything under
// /run/vrx-test/<prefix>/frr, pathspace <prefix>, files owned by the test process.
func TestPaths(prefix string) Paths {
	base := filepath.Join("/run/vrx-test", prefix, "frr")
	return Paths{
		ConfDir:   filepath.Join(base, "etc"),
		RunDir:    filepath.Join(base, "run"),
		Namespace: prefix,
		BinDir:    "/usr/bin",
		ReloadLog: filepath.Join(base, "frr-reload.log"),
		FileMode:  0o640,
	}
}

var namespaceRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,5}$`)

// Validate checks that every path is absolute and clean and the namespace is a slot prefix.
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"ConfDir": p.ConfDir, "RunDir": p.RunDir, "BinDir": p.BinDir, "ReloadLog": p.ReloadLog,
	} {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v {
			return fmt.Errorf("frr: Paths.%s %q must be absolute and clean", name, v)
		}
	}
	if p.Namespace != "" && !namespaceRe.MatchString(p.Namespace) {
		return fmt.Errorf("frr: Paths.Namespace %q must match %s", p.Namespace, namespaceRe)
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o007 != 0 {
		return fmt.Errorf("frr: Paths.FileMode %v must be permission bits and not world-accessible", p.FileMode)
	}
	return nil
}

// ConfSubdir is the directory FRR actually reads for this pathspace (<ConfDir>[/<ns>]).
func (p Paths) ConfSubdir() string { return filepath.Join(p.ConfDir, p.Namespace) }

// SocketDir is the directory holding the daemons' vty sockets and pidfiles (<RunDir>[/<ns>]).
func (p Paths) SocketDir() string { return filepath.Join(p.RunDir, p.Namespace) }

// ConfFile is the integrated configuration file.
func (p Paths) ConfFile() string { return filepath.Join(p.ConfSubdir(), "frr.conf") }

// VtyshConf is vtysh.conf.
func (p Paths) VtyshConf() string { return filepath.Join(p.ConfSubdir(), "vtysh.conf") }

// vtyshArgs are the common vtysh arguments selecting config dir, sockets and pathspace.
func (p Paths) vtyshArgs(confDir string) []string {
	args := []string{"--config_dir", confDir, "--vty_socket", p.RunDir}
	if p.Namespace != "" {
		args = append(args, "-N", p.Namespace)
	}
	return args
}

// reloadArgs is the fixed argv of frr-reload.py in mode ("--reload" or "--test") for file.
func (p Paths) reloadArgs(mode, file string) []string {
	// critical: at info/warning/error frr-reload.py logs the config lines it applies or failed
	// to apply — including secrets — to the log file (RF-1 review M2). Errors reach the caller
	// through the exit status and the (redacted) convergence diff instead.
	args := []string{mode, "--log-level", "critical", "--logfile", p.ReloadLog,
		"--bindir", p.BinDir, "--confdir", p.ConfDir, "--rundir", p.SocketDir(), "--vty_socket", p.RunDir}
	if p.Namespace != "" {
		args = append(args, "--pathspace", p.Namespace)
	}
	return append(args, file)
}
