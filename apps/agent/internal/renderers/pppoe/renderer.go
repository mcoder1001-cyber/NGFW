// Package pppoe renders the PPPoE client (F-pppoe-client, mechanism = pppd rp-pppoe on a linux-cp tap of
// the WAN parent). Render is pure: given the resolved sessions (the password already resolved from its
// secret ref, the parent's Linux tap name resolved by the agent) it produces the pppd peer file, the
// chap/pap-secrets, the per-session ip-up/ip-down hooks and a systemd unit per session. pppd has no
// offline config checker, so Validate is structural (see README). Apply/state (starting pppd, mirroring
// the negotiated address into VPP) run on the box and are the agent side — this package is the config.
package pppoe

import (
	"embed"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"ngfw/agent/internal/renderers"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// ErrInput is wrapped by errors from a bad session (an empty user, a hostile string).
var ErrInput = errors.New("pppoe: invalid session")

// hostIfRe is a Linux interface name (IFNAMSIZ 15): the pppd nic and the file-name key.
var hostIfRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)

// Session is one resolved PPPoE client (the agent builds it from interfaces.<name>.pppoe).
type Session struct {
	// Iface is the configuration interface name (JSON pointer key); Remotename below is derived from it.
	Iface string
	// HostIf is the Linux ethernet interface pppd runs PPPoE discovery on (the parent's linux-cp tap).
	HostIf string
	// Password is the resolved secret; it lands only in the chap/pap-secrets (Secret files).
	Username, Password string
	ServiceName        string
	MTU                uint32
	MSSClamp           bool
	DefaultRoute       bool
	DNSFromPeer        bool
	// IPv6 is "off" | "slaac" | "dhcpv6".
	IPv6                string
	HoldoffSec, MaxFail uint32
}

// MinIPv6MTU is the IPv6 minimum link MTU (RFC 8200 §5): a session with IPv6 on must carry at least this.
const MinIPv6MTU = 1280

// Remotename is the pppd `remotename` / secrets server field / ip-param: stable per session, ties a
// secrets line to this peer and lets the shared hook tell our sessions apart.
func (s Session) Remotename() string { return "ngfw-" + s.HostIf }

// IPv6Enabled reports whether pppd should negotiate IPv6CP.
func (s Session) IPv6Enabled() bool { return s.IPv6 == "slaac" || s.IPv6 == "dhcpv6" }

func (s Session) validate() error {
	switch {
	case !hostIfRe.MatchString(s.HostIf):
		return fmt.Errorf("%w: host interface %q is not a Linux interface name", ErrInput, s.HostIf)
	case s.Username == "" || strings.ContainsAny(s.Username, "\"\\\n\r\t"):
		return fmt.Errorf("%w: username of %q is empty or has quotes/control characters", ErrInput, s.Iface)
	case strings.ContainsAny(s.Password, "\n\r"):
		return fmt.Errorf("%w: password of %q contains a newline", ErrInput, s.Iface)
	case strings.ContainsAny(s.ServiceName, "\"\\\n\r\t"):
		return fmt.Errorf("%w: service name of %q has quotes/control characters", ErrInput, s.Iface)
	case s.MTU < 128 || s.MTU > 1500:
		return fmt.Errorf("%w: MTU %d of %q is out of range (128..1500)", ErrInput, s.MTU, s.Iface)
	case s.IPv6 != "" && s.IPv6 != "off" && !s.IPv6Enabled():
		return fmt.Errorf("%w: IPv6 mode %q of %q is not off, slaac or dhcpv6", ErrInput, s.IPv6, s.Iface)
	case s.IPv6Enabled() && s.MTU < MinIPv6MTU:
		return fmt.Errorf("%w: MTU %d of %q is below the IPv6 minimum link MTU %d", ErrInput, s.MTU, s.Iface, MinIPv6MTU)
	}
	return nil
}

// Renderer implements the file half of renderers.Renderer for pppd.
type Renderer struct {
	paths Paths
	tmpl  *template.Template
}

// Option configures a Renderer.
type Option func(*Renderer)

// WithPaths overrides ProductPaths (tests: PathsUnder).
func WithPaths(p Paths) Option { return func(r *Renderer) { r.paths = p } }

// New returns a Renderer. It panics on a template bug (build-time, like the other renderers).
func New(opts ...Option) *Renderer {
	t := template.Must(renderers.NewTemplate("pppoe").ParseFS(templateFS, "templates/*.tmpl"))
	r := &Renderer{paths: ProductPaths(), tmpl: t}
	for _, o := range opts {
		o(r)
	}
	return r
}

// chapLine / papLine is one secrets line: `"<user>" <remotename> "<password>" *`.
func secretLine(user, remotename, pass string) string {
	return fmt.Sprintf("%q %s %q *\n", user, remotename, pass)
}

// Render builds the files for the given sessions (sorted by HostIf for determinism). No sessions →
// empty Files (the caller removes the whole tree). Duplicate HostIf is an error.
func (r *Renderer) Render(sessions []Session) (renderers.Files, error) {
	if err := r.paths.Validate(); err != nil {
		return nil, err
	}
	ss := append([]Session(nil), sessions...)
	sort.Slice(ss, func(i, j int) bool { return ss[i].HostIf < ss[j].HostIf })
	seen := map[string]bool{}
	files := renderers.Files{}
	var chap, pap strings.Builder
	chap.WriteString("# ngfw-agent PPPoE client secrets (F-pppoe-client). Rendered; do not edit.\n")
	pap.WriteString("# ngfw-agent PPPoE client secrets (F-pppoe-client). Rendered; do not edit.\n")
	for _, s := range ss {
		if err := s.validate(); err != nil {
			return nil, err
		}
		if seen[s.HostIf] {
			return nil, fmt.Errorf("%w: two sessions on host interface %q", ErrInput, s.HostIf)
		}
		seen[s.HostIf] = true

		peer, err := renderers.ExecuteTemplate(r.tmpl, "peer.tmpl", peerData{Session: s})
		if err != nil {
			return nil, err
		}
		files[r.paths.PeersDir+"/ngfw-"+s.HostIf] = renderers.File{Mode: 0o644, Content: peer}

		unit, err := renderers.ExecuteTemplate(r.tmpl, "unit.tmpl", unitData{Session: s, PppdBin: PppdBin})
		if err != nil {
			return nil, err
		}
		files[r.paths.UnitDir+"/ngfw-pppoe-"+s.HostIf+".service"] = renderers.File{Mode: 0o644, Content: unit}

		for _, h := range []struct {
			dir, kind, phase string
		}{{r.paths.IPUpDir, "ip-up", "up"}, {r.paths.IPDownDir, "ip-down", "down"}} {
			body, err := renderers.ExecuteTemplate(r.tmpl, "hook.tmpl", hookData{Session: s, Kind: h.kind, Phase: h.phase, StateDir: r.paths.StateDir})
			if err != nil {
				return nil, err
			}
			files[h.dir+"/ngfw-"+s.HostIf] = renderers.File{Mode: 0o755, Content: body}
		}

		if s.IPv6Enabled() {
			if err := r.renderIPv6(files, s); err != nil {
				return nil, err
			}
		}

		line := secretLine(s.Username, s.Remotename(), s.Password)
		chap.WriteString(line)
		pap.WriteString(line)
	}
	if len(ss) == 0 {
		return files, nil
	}
	files[r.paths.ChapSecrets] = renderers.File{Mode: 0o600, Content: []byte(chap.String()), Secret: true}
	files[r.paths.PapSecrets] = renderers.File{Mode: 0o600, Content: []byte(pap.String()), Secret: true}
	if err := files.Validate(); err != nil {
		return nil, err
	}
	return files, nil
}

// Validate is the structural dry run: pppd has no offline checker (README), so re-render and re-check.
func (r *Renderer) Validate(sessions []Session) error {
	_, err := r.Render(sessions)
	return err
}

type peerData struct{ Session }
type unitData struct {
	Session
	PppdBin string
}
type hookData struct {
	Session
	Kind, Phase, StateDir string
}
type hook6Data struct {
	Session
	Kind, Phase, StateDir, DhcpcdBin, DhcpcdConf, IPv6Helper, IPv6UpHook string
}
type dhcpcdData struct {
	Session
	Script string
}

// renderIPv6 adds a session's IPv6 files: the ipv6-up/ipv6-down hooks (kernel SLAAC on the PPP link and the
// "<hostif>.state6" refresher) and, for "dhcpv6", the dhcpcd configuration and event script (IA_NA + IA_PD).
func (r *Renderer) renderIPv6(files renderers.Files, s Session) error {
	data := hook6Data{Session: s, StateDir: r.paths.StateDir, DhcpcdBin: DhcpcdBin,
		DhcpcdConf: r.paths.dhcpcdConf(s.HostIf), IPv6Helper: r.paths.ipv6Helper(s.HostIf),
		IPv6UpHook: r.paths.IPv6UpDir + "/ngfw-" + s.HostIf}
	helper, err := renderers.ExecuteTemplate(r.tmpl, "ipv6.tmpl", data)
	if err != nil {
		return err
	}
	files[data.IPv6Helper] = renderers.File{Mode: 0o755, Content: helper}
	for _, h := range []struct {
		dir, kind, phase string
	}{{r.paths.IPv6UpDir, "ipv6-up", "up"}, {r.paths.IPv6DownDir, "ipv6-down", "down"}} {
		data.Kind, data.Phase = h.kind, h.phase
		body, err := renderers.ExecuteTemplate(r.tmpl, "hook6.tmpl", data)
		if err != nil {
			return err
		}
		files[h.dir+"/ngfw-"+s.HostIf] = renderers.File{Mode: 0o755, Content: body}
	}
	if s.IPv6 != "dhcpv6" {
		return nil
	}
	conf, err := renderers.ExecuteTemplate(r.tmpl, "dhcpcd.tmpl", dhcpcdData{Session: s, Script: r.paths.dhcp6Script(s.HostIf)})
	if err != nil {
		return err
	}
	files[r.paths.dhcpcdConf(s.HostIf)] = renderers.File{Mode: 0o644, Content: conf}
	script, err := renderers.ExecuteTemplate(r.tmpl, "dhcp6.tmpl", data)
	if err != nil {
		return err
	}
	files[r.paths.dhcp6Script(s.HostIf)] = renderers.File{Mode: 0o755, Content: script}
	return nil
}
