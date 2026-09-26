package rsyslog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Binaries this renderer invokes (internal/renderers/ALLOWLIST.md).
const (
	// RsyslogdBin validates a staged config (`rsyslogd -N1 -f <file>`).
	RsyslogdBin = "/usr/sbin/rsyslogd"
	// ModuleDirProduct is where rsyslog's loadable modules live; the renderer only checks for
	// the TLS netstream driver there (it executes nothing from it).
	ModuleDirProduct = "/usr/lib/x86_64-linux-gnu/rsyslog"
	// TLSDriver is the netstream driver the TLS targets use (package rsyslog-openssl).
	TLSDriver = "ossl"
)

// Binaries is the allowlist of the production SystemRunner (rsyslogd -N1, systemctl restart).
func Binaries() []string { return []string{RsyslogdBin, "/usr/bin/systemctl"} }

// Standalone makes the rendered file a complete rsyslogd configuration of its own (tests: a
// child rsyslogd reads only this file). In the product the file is an include of the host's
// rsyslog (/etc/rsyslog.d), whose main config owns inputs and the work directory.
type Standalone struct {
	// WorkDir is global(workDirectory).
	WorkDir string
	// Socket is the imuxsock input socket (never /dev/log).
	Socket string
	// TCPPort, when non-zero, adds an imtcp input on 127.0.0.1:TCPPort.
	TCPPort uint32
}

// Paths is every filesystem location the renderer uses; injected so tests never touch
// /etc/rsyslog*.
type Paths struct {
	// ConfFile is the rendered RainerScript file.
	ConfFile string
	// StatsFile is impstats' log.file (JSON lines, read by Retrieve).
	StatsFile string
	// TLSDir holds the per-target CA / certificate / key files of TLS targets.
	TLSDir string
	// ModuleDir is rsyslog's module directory (TLS driver availability check).
	ModuleDir string
	// Standalone is set in tests only.
	Standalone *Standalone
	// HostConfigs are the host rsyslog files (globs) that include ConfFile. The renderer reads
	// them — never writes — to find an impstats module the host already loads: rsyslog refuses a
	// second load ("module 'impstats' already in this config") and the whole config fails
	// (review M3). Empty in standalone tests.
	HostConfigs []string
	// FileOwner owns the config and certificate files; KeyOwner the private keys (rsyslog
	// drops privileges to user syslog before a TLS action opens its key: root:syslog 0640).
	FileOwner, KeyOwner string
	// FileMode of the config (0644: no secret in it; keys live in their own files).
	FileMode os.FileMode
}

// ProductPaths are the product locations (Ubuntu's rsyslog drops privileges to syslog, so the
// stats file lives in its spool directory).
func ProductPaths() Paths {
	return Paths{
		ConfFile:    "/etc/rsyslog.d/50-vrx-export.conf",
		StatsFile:   "/var/spool/rsyslog/vrx-impstats.json",
		TLSDir:      "/etc/vrx/rsyslog-tls",
		ModuleDir:   ModuleDirProduct,
		HostConfigs: []string{"/etc/rsyslog.conf", "/etc/rsyslog.d/*.conf"},
		FileOwner:   "root:root",
		KeyOwner:    "root:syslog",
		FileMode:    0o644,
	}
}

// TestPaths are the test-scoped paths for a slot prefix: everything under
// /run/vrx-test/<prefix>/rsyslog, a standalone config with imuxsock on <dir>/log.sock and
// (tcpPort > 0) imtcp on 127.0.0.1:tcpPort.
func TestPaths(prefix string, tcpPort uint32) Paths {
	return PathsUnder(filepath.Join("/run/vrx-test", prefix, "rsyslog"), tcpPort)
}

// PathsUnder are TestPaths rooted at base (an agent that is not the globals owner renders its slot-local instance
// there, F-unbound-chrony-syslog).
func PathsUnder(base string, tcpPort uint32) Paths {
	return Paths{
		ConfFile:   filepath.Join(base, "rsyslog.conf"),
		StatsFile:  filepath.Join(base, "impstats.json"),
		TLSDir:     filepath.Join(base, "tls"),
		ModuleDir:  ModuleDirProduct,
		Standalone: &Standalone{WorkDir: filepath.Join(base, "work"), Socket: filepath.Join(base, "log.sock"), TCPPort: tcpPort},
		FileMode:   0o644,
	}
}

// pathRe: paths are rendered inside RainerScript strings (escaped anyway) — keep them plain.
var pathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]{1,150}$`)

// Validate checks every path.
func (p Paths) Validate() error {
	check := map[string]string{"ConfFile": p.ConfFile, "StatsFile": p.StatsFile, "TLSDir": p.TLSDir, "ModuleDir": p.ModuleDir}
	if s := p.Standalone; s != nil {
		check["Standalone.WorkDir"], check["Standalone.Socket"] = s.WorkDir, s.Socket
		if s.TCPPort > 65535 {
			return fmt.Errorf("rsyslog: Paths.Standalone.TCPPort %d out of range", s.TCPPort)
		}
	}
	for name, v := range check {
		if !pathRe.MatchString(v) || filepath.Clean(v) != v {
			return fmt.Errorf("rsyslog: Paths.%s %q must be an absolute clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 {
		return fmt.Errorf("rsyslog: Paths.FileMode %v", p.FileMode)
	}
	return nil
}
