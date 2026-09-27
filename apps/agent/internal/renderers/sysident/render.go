// Package sysident is the system-identity renderer (F-system-identity, WBS D0.14, DEC-system-identity / D-152): the
// `system` domain — hostname, time zone, login/MOTD banners and the router's own DNS client — rendered into
// /etc/hostname (+ sethostname(2)), the /etc/localtime symlink, /etc/issue, /etc/issue.net, /etc/motd and a
// systemd-resolved drop-in. There is no daemon checker: the input is validated structurally (Check). See
// docs/agent/renderers/sysident.md.
package sysident

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// Defaults of the `system` schema domain (packages/schema/src/domains/system.ts).
const (
	DefaultHostname = "vrx"
	DefaultTimezone = "UTC"
	DefaultVRF      = "default"
	maxBanner       = 4096
	maxServers      = 8
	maxSearch       = 6
)

// ErrInvalid is wrapped by every input error.
var ErrInvalid = errors.New("sysident: invalid input")

// FieldError names the offending leaf (an RFC 6901 pointer into the configuration document).
type FieldError struct {
	Pointer string
	Msg     string
}

func (e *FieldError) Error() string { return e.Pointer + ": " + e.Msg }
func (e *FieldError) Unwrap() error { return ErrInvalid }

func bad(ptr, format string, a ...any) error {
	return &FieldError{Pointer: ptr, Msg: fmt.Sprintf(format, a...)}
}

// Input is the normalised render input of sys: defaults filled in, empty banners dropped, Banner and Dns always
// present. The scheduler Value is exactly this message, so Retrieve (which decodes it from the rendered drop-in)
// compares equal to a projection of the same document.
func Input(sys *vrxv1.SystemConfig) *vrxv1.SystemConfig {
	in := &vrxv1.SystemConfig{}
	if sys != nil {
		in = proto.Clone(sys).(*vrxv1.SystemConfig)
	}
	if in.GetHostname() == "" {
		in.Hostname = proto.String(DefaultHostname)
	}
	if in.GetTimezone() == "" {
		in.Timezone = proto.String(DefaultTimezone)
	}
	if in.Banner == nil {
		in.Banner = &vrxv1.SystemBanner{}
	}
	if in.Banner.GetLogin() == "" {
		in.Banner.Login = nil
	}
	if in.Banner.GetMotd() == "" {
		in.Banner.Motd = nil
	}
	if in.Dns == nil {
		in.Dns = &vrxv1.SystemDns{}
	}
	if in.Dns.GetVrf() == "" {
		in.Dns.Vrf = proto.String(DefaultVRF)
	}
	return in
}

var (
	// RFC 1123 host name (labels of letters, digits, hyphens; no leading/trailing hyphen), as the schema's primitive.
	hostnameRe = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$`)
	timezoneRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9_+-]*(?:/[A-Z0-9][A-Za-z0-9_+-]*)*$`)
)

// BannerOK reports whether s may be written to a terminal-facing banner file (D-049): printable characters, LF and
// TAB only — no C0/C1 control characters (ESC, CR, BEL, …), no DEL and no bidirectional-override/isolate marks that
// could forge or hide lines.
func BannerOK(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("banner is not valid UTF-8")
	}
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case r < 0x20 || (r >= 0x7f && r <= 0x9f):
			return fmt.Errorf("banner contains the control character U+%04X (D-049: printable, LF and TAB only)", r)
		case (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) || r == 0x200e || r == 0x200f || r == 0x061c:
			return fmt.Errorf("banner contains the bidirectional control U+%04X (D-049)", r)
		}
	}
	return nil
}

// Check validates in (normalised by Input) against the schema's rules and the host: the zone file must exist
// under zoneinfoDir. Errors are *FieldError.
func Check(in *vrxv1.SystemConfig, zoneinfoDir string) error {
	if err := CheckStructure(in); err != nil {
		return err
	}
	if fi, err := os.Stat(filepath.Join(zoneinfoDir, in.GetTimezone())); err != nil || !fi.Mode().IsRegular() {
		return bad("/system/timezone", "unknown time zone %q: %s has no such zone file (tzdata)", in.GetTimezone(), zoneinfoDir)
	}
	return nil
}

// CheckStructure is Check without the host lookup (the zone file): the projection's DryRun check.
func CheckStructure(in *vrxv1.SystemConfig) error {
	h := in.GetHostname()
	if len(h) > 253 || !hostnameRe.MatchString(h) {
		return bad("/system/hostname", "%q is not an RFC 1123 host name", h)
	}
	tz := in.GetTimezone()
	if len(tz) > 64 || !timezoneRe.MatchString(tz) || strings.Contains(tz, "..") {
		return bad("/system/timezone", "%q is not an IANA time zone name", tz)
	}
	for name, v := range map[string]string{"login": in.GetBanner().GetLogin(), "motd": in.GetBanner().GetMotd()} {
		if len([]rune(v)) > maxBanner {
			return bad("/system/banner/"+name, "banner is longer than %d characters", maxBanner)
		}
		if err := BannerOK(v); err != nil {
			return bad("/system/banner/"+name, "%v", err)
		}
	}
	d := in.GetDns()
	if len(d.GetServers()) > maxServers {
		return bad("/system/dns/servers", "at most %d name servers", maxServers)
	}
	for i, s := range d.GetServers() {
		a, err := netip.ParseAddr(s)
		if err != nil || a.Zone() != "" {
			return bad(fmt.Sprintf("/system/dns/servers/%d", i), "%q is not an IP address", s)
		}
	}
	if len(d.GetSearchDomains()) > maxSearch {
		return bad("/system/dns/searchDomains", "at most %d search domains", maxSearch)
	}
	for i, s := range d.GetSearchDomains() {
		if len(s) > 253 || !hostnameRe.MatchString(s) {
			return bad(fmt.Sprintf("/system/dns/searchDomains/%d", i), "%q is not a domain name", s)
		}
	}
	if d.GetVrf() != DefaultVRF {
		return bad("/system/dns/vrf", "the system resolver (systemd-resolved) reaches name servers in the default VRF only; VRF %q is not supported by this agent build", d.GetVrf())
	}
	return nil
}

const inputPrefix = "# vrx-input: "

var inputLineRe = regexp.MustCompile(`(?m)^# vrx-input: ([A-Za-z0-9+/]*={0,2})$`)

// Rendering is the output of Render: the files plus the /etc/localtime symlink target.
type Rendering struct {
	Files    renderers.Files
	Zonefile string
}

// Render renders in (normalised, checked) for p. Pure: no I/O.
func Render(in *vrxv1.SystemConfig, p Paths) (Rendering, error) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	if err != nil {
		return Rendering{}, fmt.Errorf("%w: encode render input: %w", ErrInvalid, err)
	}
	var rc bytes.Buffer
	rc.WriteString("# Rendered by vrx-agent from the `system.dns` configuration (F-system-identity). Do not edit.\n")
	rc.WriteString(inputPrefix + base64.StdEncoding.EncodeToString(raw) + "\n")
	rc.WriteString("[Resolve]\n")
	if s := in.GetDns().GetServers(); len(s) > 0 {
		rc.WriteString("DNS=" + strings.Join(s, " ") + "\n")
	}
	if s := in.GetDns().GetSearchDomains(); len(s) > 0 {
		rc.WriteString("Domains=" + strings.Join(s, " ") + "\n")
	}
	f := func(b []byte) renderers.File { return renderers.File{Mode: p.FileMode, Content: b} }
	login := banner(in.GetBanner().GetLogin())
	files := renderers.Files{
		p.Hostname:       f([]byte(in.GetHostname() + "\n")),
		p.Issue:          f(login),
		p.IssueNet:       f(login),
		p.Motd:           f(banner(in.GetBanner().GetMotd())),
		p.ResolvedDropIn: f(rc.Bytes()),
	}
	return Rendering{Files: files, Zonefile: filepath.Join(p.ZoneinfoDir, in.GetTimezone())}, files.Validate()
}

// banner is the file content of a banner: the text with exactly one trailing newline (empty for no banner).
func banner(s string) []byte {
	if s == "" {
		return []byte{}
	}
	return []byte(strings.TrimRight(s, "\n") + "\n")
}

// EmbeddedInput reads the render input back from a rendered drop-in. ok is false for a file without one.
func EmbeddedInput(dropIn []byte) (*vrxv1.SystemConfig, bool, error) {
	m := inputLineRe.FindAllSubmatch(dropIn, 2)
	switch len(m) {
	case 0:
		return nil, false, nil
	case 1:
	default:
		return nil, false, fmt.Errorf("sysident: %q appears more than once", inputPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(string(m[0][1]))
	if err != nil {
		return nil, false, fmt.Errorf("sysident: render input: %w", err)
	}
	in := &vrxv1.SystemConfig{}
	if err := proto.Unmarshal(raw, in); err != nil {
		return nil, false, fmt.Errorf("sysident: render input: %w", err)
	}
	return in, true, nil
}
