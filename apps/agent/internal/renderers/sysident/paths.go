package sysident

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Paths is every filesystem location the renderer touches. It is injected so tests (and every agent that is not the
// globals owner, D-071) never touch the host's identity: /etc/hostname, /etc/localtime, the banners and the resolver
// configuration are box-wide singletons (docs/lab/shared-host-rules.md).
type Paths struct {
	// Hostname is /etc/hostname.
	Hostname string
	// Localtime is the /etc/localtime symlink; ZoneinfoDir is the tzdata directory its target lives in (read only).
	Localtime   string
	ZoneinfoDir string
	// Issue and IssueNet receive banner.login (console getty and telnet/SSH issue files); Motd receives banner.motd.
	Issue    string
	IssueNet string
	Motd     string
	// ResolvedDropIn is the systemd-resolved drop-in holding DNS= and Domains= (and the embedded render input).
	ResolvedDropIn string
	// SetKernelHostname calls sethostname(2) after /etc/hostname changed. Only the globals owner sets it.
	SetKernelHostname bool
	// FileMode of every rendered file (0644: /etc/issue, /etc/motd and /etc/hostname are world-readable by design).
	FileMode os.FileMode
}

// ProductPaths are the paths of the Ubuntu 26.04 image.
func ProductPaths() Paths {
	return Paths{
		Hostname:          "/etc/hostname",
		Localtime:         "/etc/localtime",
		ZoneinfoDir:       "/usr/share/zoneinfo",
		Issue:             "/etc/issue",
		IssueNet:          "/etc/issue.net",
		Motd:              "/etc/motd",
		ResolvedDropIn:    "/etc/systemd/resolved.conf.d/vrx.conf",
		SetKernelHostname: true,
		FileMode:          0o644,
	}
}

// PathsUnder mirrors ProductPaths under base (<base>/etc/…): a test slot's agent and unit tests. The zoneinfo
// directory stays the host's (read only); the kernel hostname is never set.
func PathsUnder(base string) Paths {
	p := ProductPaths()
	for _, f := range []*string{&p.Hostname, &p.Localtime, &p.Issue, &p.IssueNet, &p.Motd, &p.ResolvedDropIn} {
		*f = filepath.Join(base, *f)
	}
	p.SetKernelHostname = false
	return p
}

var safePathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]*$`)

// Validate checks that every path is absolute, clean and made of safe characters, and the mode.
func (p Paths) Validate() error {
	for name, v := range map[string]string{
		"Hostname": p.Hostname, "Localtime": p.Localtime, "ZoneinfoDir": p.ZoneinfoDir, "Issue": p.Issue,
		"IssueNet": p.IssueNet, "Motd": p.Motd, "ResolvedDropIn": p.ResolvedDropIn,
	} {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v || !safePathRe.MatchString(v) {
			return fmt.Errorf("sysident: Paths.%s %q must be an absolute, clean path of [A-Za-z0-9_./-]", name, v)
		}
	}
	if p.FileMode == 0 || p.FileMode&^os.ModePerm != 0 || p.FileMode&0o022 != 0 {
		return fmt.Errorf("sysident: Paths.FileMode %v must be permission bits, not group/world-writable", p.FileMode)
	}
	return nil
}

// Dirs are the parent directories Apply creates when missing.
func (p Paths) Dirs() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range []string{p.Hostname, p.Localtime, p.Issue, p.IssueNet, p.Motd, p.ResolvedDropIn} {
		if d := filepath.Dir(f); !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}
