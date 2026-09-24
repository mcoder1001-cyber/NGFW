package chrony

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"ngfw/agent/internal/renderers"
)

// Binaries the renderer invokes (listed in internal/renderers/ALLOWLIST.md). Fixed argv, no
// shell; rendered content travels through files.
const (
	// ChronydBin checks chrony.conf and the sources file (`chronyd -p -f <file>`: parse,
	// print, exit; chrony ≥ 4.0).
	ChronydBin = "/usr/sbin/chronyd"
	// ChronycBin reloads sources / keys and reads state over the unix command socket
	// (`chronyc -h <socket> …`).
	ChronycBin = "/usr/bin/chronyc"
)

// Binaries is the allowlist for the production SystemRunner of this renderer.
func Binaries() []string { return []string{ChronydBin, ChronycBin} }

// NewRunner returns the production runner (allow-listed binaries, minimal environment).
func NewRunner() *renderers.SystemRunner {
	return renderers.NewSystemRunner(renderers.NewAllowlist(Binaries()...))
}

// Paths is every filesystem location the renderer uses. It is injected so tests never touch
// the product paths (/etc/chrony, /run/chrony, /var/lib/chrony).
type Paths struct {
	// ConfDir holds chrony.conf, chrony.keys and sources.d/.
	ConfDir string
	// RunDir holds the pidfile and the unix command socket.
	RunDir string
	// StateDir holds the drift file and the NTS dump directory.
	StateDir string
	// LogDir is chrony's logdir.
	LogDir string
	// FileOwner / FileMode of chrony.conf and the sources file ("root:_chrony" 0640 in the
	// product); KeyOwner / KeyMode of chrony.keys ("_chrony:_chrony" 0600: secret, and chronyd
	// re-reads it on `rekey` after dropping privileges to _chrony).
	FileOwner string
	FileMode  os.FileMode
	KeyOwner  string
	KeyMode   os.FileMode
	// ClockControl is false for test instances (chronyd -x): rtcsync is not rendered.
	ClockControl bool
	// LoopbackOnly restricts bind addresses to loopback and requires them in server mode
	// (tests: a test instance can never serve on a host NIC such as ens192).
	LoopbackOnly bool
	// SourcePort, when non-zero, is appended as `port N` to every server line (tests: the
	// client reaches the test server on 3<slot>23; the document has no per-server port).
	SourcePort uint16
	// Unit is the systemd unit a restart is requested for ("chrony").
	Unit string
	// PendingFile persists a restart request until chronyd runs the new chrony.conf (D-079,
	// review M2); in /run so that a reboot, which restarts chronyd, clears it.
	PendingFile string
}

// ProductPaths are the paths of the packaged chrony 4.8 on Ubuntu 26.04.
func ProductPaths() Paths {
	return Paths{
		ConfDir:      "/etc/chrony",
		RunDir:       "/run/chrony",
		StateDir:     "/var/lib/chrony",
		LogDir:       "/var/log/chrony",
		FileOwner:    "root:_chrony",
		FileMode:     0o640,
		KeyOwner:     "_chrony:_chrony",
		KeyMode:      0o600,
		ClockControl: true,
		Unit:         "chrony",
		PendingFile:  "/run/vrx/renderers/chrony.pending",
	}
}

// TestPaths are the test-scoped paths for slot prefix ("w6") and an instance name ("server",
// "client"): everything under /run/vrx-test/<prefix>/chrony/<instance>, no clock control,
// loopback only. Owners are the product's: chronyd drops to _chrony after reading its config
// and chronyc drops to _chrony before talking to the socket, so the instance directory is
// _chrony:_chrony 0750 (chronyd refuses a command-socket directory it does not own).
func TestPaths(prefix, instance string) Paths {
	return PathsUnder(filepath.Join("/run/vrx-test", prefix, "chrony", instance))
}

// PathsUnder are TestPaths rooted at base (an agent that is not the globals owner renders its slot-local instance
// there, F-unbound-chrony-syslog).
func PathsUnder(base string) Paths {
	return Paths{
		ConfDir:      base,
		RunDir:       base,
		StateDir:     base,
		LogDir:       filepath.Join(base, "log"),
		FileOwner:    "root:_chrony",
		FileMode:     0o640,
		KeyOwner:     "_chrony:_chrony",
		KeyMode:      0o600,
		ClockControl: false,
		LoopbackOnly: true,
		Unit:         "chrony",
		PendingFile:  filepath.Join(base, "vrx.pending"),
	}
}

var (
	safePathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]*$`)
	unitRe     = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,64}$`)
)

// Validate checks the paths (absolute, clean, safe characters: chrony directives take
// unquoted tokens) and the modes.
func (p Paths) Validate() error {
	for name, v := range map[string]string{"ConfDir": p.ConfDir, "RunDir": p.RunDir, "StateDir": p.StateDir, "LogDir": p.LogDir, "PendingFile": p.PendingFile} {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v || !safePathRe.MatchString(v) {
			return fmt.Errorf("chrony: Paths.%s %q must be an absolute, clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o007 != 0 {
		return fmt.Errorf("chrony: Paths.FileMode %v must be permission bits and not world-accessible", p.FileMode)
	}
	if p.KeyMode == 0 || p.KeyMode&^os.ModePerm != 0 || p.KeyMode&0o027 != 0 {
		return fmt.Errorf("chrony: Paths.KeyMode %v must be 0640 or stricter", p.KeyMode)
	}
	if !unitRe.MatchString(p.Unit) {
		return fmt.Errorf("chrony: Paths.Unit %q is not a unit name", p.Unit)
	}
	return nil
}

// Conf is chrony.conf.
func (p Paths) Conf() string { return filepath.Join(p.ConfDir, "chrony.conf") }

// Keys is chrony.keys.
func (p Paths) Keys() string { return filepath.Join(p.ConfDir, "chrony.keys") }

// SourceDir is the sourcedir; Sources is the file the renderer owns in it.
func (p Paths) SourceDir() string { return filepath.Join(p.ConfDir, "sources.d") }

// Sources is sources.d/vrx.sources.
func (p Paths) Sources() string { return filepath.Join(p.SourceDir(), "vrx.sources") }

// Socket is the unix command socket (bindcmdaddress).
func (p Paths) Socket() string { return filepath.Join(p.RunDir, "chronyd.sock") }

// PidFile is chronyd's pidfile.
func (p Paths) PidFile() string { return filepath.Join(p.RunDir, "chronyd.pid") }

// DriftFile is the driftfile.
func (p Paths) DriftFile() string { return filepath.Join(p.StateDir, "chrony.drift") }

// NTSDumpDir is the ntsdumpdir.
func (p Paths) NTSDumpDir() string { return filepath.Join(p.StateDir, "nts") }
