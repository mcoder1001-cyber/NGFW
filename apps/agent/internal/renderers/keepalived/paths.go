package keepalived

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Binaries this renderer invokes (internal/renderers/ALLOWLIST.md).
const (
	// KeepalivedBin validates (`-t -f <staged> [-s <netns>]`) and reports its JSON signal
	// number (`--signum=JSON`).
	KeepalivedBin = "/usr/sbin/keepalived"
	// NotifyHelperProduct is the shipped notify target (P10 packages it); keepalived runs it,
	// the agent never does.
	NotifyHelperProduct = "/usr/libexec/vrx/vrx-keepalived-notify"
	// ChecksDirProduct holds the shipped track-script executables (the only scripts a
	// vrrp_script may name).
	ChecksDirProduct = "/usr/libexec/vrx/checks"
)

// Binaries is the allowlist of the production SystemRunner (keepalived for -t/--signum,
// systemctl for the product Controller).
func Binaries() []string { return []string{KeepalivedBin, "/usr/bin/systemctl"} }

// Paths is every filesystem location the renderer uses; injected so tests never touch
// /etc/keepalived.
type Paths struct {
	// ConfFile is keepalived.conf (0640 root: holds VRRPv2 PASS keys when configured).
	ConfFile string
	// StateDir is where the notify helper writes <instance>.state (the state/event channel).
	StateDir string
	// NotifyHelper is the absolute path of vrx-keepalived-notify.
	NotifyHelper string
	// ChecksDir holds the shipped track-script executables.
	ChecksDir string
	// DumpDir is keepalived's $TMPDIR: SIGJSON writes DumpDir/keepalived.json there.
	DumpDir string
	// NetNS is the network namespace keepalived runs in ("" = the agent's); Validate passes it
	// to `keepalived -t -s <ns>` because -t checks that the interfaces exist.
	NetNS string
	// FileOwner / FileMode of keepalived.conf.
	FileOwner string
	FileMode  os.FileMode
}

// ProductPaths are the product locations. DumpDir is keepalived's default ($TMPDIR unset in
// the Ubuntu unit): P10 should set TMPDIR to a private directory (README).
func ProductPaths() Paths {
	return Paths{
		ConfFile:     "/etc/keepalived/keepalived.conf",
		StateDir:     "/run/vrx/keepalived",
		NotifyHelper: NotifyHelperProduct,
		ChecksDir:    ChecksDirProduct,
		DumpDir:      "/tmp",
		FileOwner:    "root:root",
		FileMode:     0o640,
	}
}

// TestPaths are the test-scoped paths for a slot prefix: config and state under
// /run/vrx-test/<prefix>/keepalived, executables in binDir (it cannot be under /run, which is
// mounted noexec), the daemon inside netns.
func TestPaths(prefix, binDir, netns string) Paths {
	base := filepath.Join("/run/vrx-test", prefix, "keepalived")
	return Paths{
		ConfFile:     filepath.Join(base, "keepalived.conf"),
		StateDir:     filepath.Join(base, "state"),
		NotifyHelper: filepath.Join(binDir, "vrx-keepalived-notify"),
		ChecksDir:    filepath.Join(binDir, "checks"),
		DumpDir:      filepath.Join(base, "tmp"),
		NetNS:        netns,
		FileMode:     0o640,
	}
}

var (
	// pathRe: paths are rendered inside keepalived double-quoted script strings, so no blanks,
	// quotes, braces, '#', '!' or '$'.
	pathRe  = regexp.MustCompile(`^/[A-Za-z0-9_./-]{1,150}$`)
	netnsRe = regexp.MustCompile(`^ns-[a-z0-9-]{1,20}$`)
)

// Validate checks every path and the namespace name.
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"ConfFile": p.ConfFile, "StateDir": p.StateDir, "NotifyHelper": p.NotifyHelper, "ChecksDir": p.ChecksDir, "DumpDir": p.DumpDir,
	} {
		if !pathRe.MatchString(v) || filepath.Clean(v) != v {
			return fmt.Errorf("keepalived: Paths.%s %q must be an absolute clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.NetNS != "" && !netnsRe.MatchString(p.NetNS) {
		return fmt.Errorf("keepalived: Paths.NetNS %q must match %s", p.NetNS, netnsRe)
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o007 != 0 {
		return fmt.Errorf("keepalived: Paths.FileMode %v must not be world-accessible", p.FileMode)
	}
	return nil
}
