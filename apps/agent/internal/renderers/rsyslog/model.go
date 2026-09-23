package rsyslog

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net/netip"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/rfkit"
)

// ErrInput is wrapped by every error about the desired state.
var ErrInput = errors.New("rsyslog: invalid desired state")

// Fixed template set (never a user-provided template string).
const (
	TemplateRFC5424 = "vrx_rfc5424"
	TemplateRFC3164 = "RSYSLOG_TraditionalForwardFormat"
)

// MaxTargets bounds management.syslog.
const MaxTargets = 16

// Model is the validated, resolved view of management.syslog.
type Model struct {
	Standalone *Standalone
	StatsFile  string
	Targets    []Target
}

// Target is one export: a ruleset with one omfwd action, called from the main flow under
// its facility/severity filter.
type Target struct {
	// Name is the ruleset and action name (the impstats key): vrx_export_<i>_<hash>, where the
	// hash covers the target's settings, so a changed target reports under a new name and the
	// post-restart convergence check can tell the new configuration from the old one.
	Name      string
	Host      string // canonical IP or lower-case hostname
	Port      uint32
	Protocol  string // udp | tcp
	Filter    string // prifilt() selector: "*.info", "kern,daemon.warning"
	Template  string
	Framing   string // TCP: "octet-counted" for RFC 5424, "traditional" otherwise
	QueueSize uint32
	TLS       *TLS
}

// TLS is a TLS target's driver settings and the files holding its material.
type TLS struct {
	AuthMode                  string // x509/name | x509/certvalid
	Peers                     []string
	CAFile, CertFile, KeyFile string // "" = not configured
}

var (
	hostnameRe = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$`)
	severities = map[string]string{
		"emergency": "emerg", "alert": "alert", "critical": "crit", "error": "err",
		"warning": "warning", "notice": "notice", "info": "info", "debug": "debug",
	}
	facilities = []string{"kern", "user", "mail", "daemon", "auth", "syslog", "lpr", "news", "uucp", "cron", "authpriv", "ftp",
		"local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7"}
	// pemRe: a PEM document (printable ASCII lines) as the secret store returns it.
	pemRe = regexp.MustCompile(`^-----BEGIN [A-Z0-9 ]{1,40}-----\n(?:[ -~]*\n)+-----END [A-Z0-9 ]{1,40}-----\n?$`)
)

func checkPEM(v string) error {
	if len(v) > 64<<10 || !pemRe.MatchString(v) {
		return errors.New("expected one or more PEM blocks (printable ASCII, ≤ 64 KiB)")
	}
	return nil
}

// Files the model needs besides the config: TLS material resolved from secret references.
type tlsFile struct {
	Path    string
	Content string
	Secret  bool
}

// BuildModel validates management.syslog (+ stand-ins, D-055) and resolves TLS material.
func BuildModel(ds *vrxv1.DesiredState, ext *rfkit.Ext, sec *rfkit.Secrets, p Paths) (*Model, []tlsFile, error) {
	m := &Model{Standalone: p.Standalone, StatsFile: p.StatsFile}
	list := ds.GetManagement().GetSyslog()
	if len(list) > MaxTargets {
		return nil, nil, fmt.Errorf("%w: management.syslog holds %d targets (max %d)", ErrInput, len(list), MaxTargets)
	}
	var files []tlsFile
	seen := map[string]int{}
	for i, s := range list {
		path := fmt.Sprintf("management.syslog[%d]", i)
		x := ext.Get("management", "syslog")
		if x != nil {
			x = x.Index(i)
		}
		t, tf, err := buildTarget(i, s, x, sec, p, path)
		if err != nil {
			return nil, nil, err
		}
		key := fmt.Sprintf("%s|%d|%s", t.Host, t.Port, t.Protocol)
		if j, dup := seen[key]; dup {
			return nil, nil, fmt.Errorf("%w: %s duplicates management.syslog[%d]", ErrInput, path, j)
		}
		seen[key] = i
		t.Name = actionName(i, t)
		m.Targets = append(m.Targets, *t)
		files = append(files, tf...)
	}
	return m, files, nil
}

func buildTarget(i int, s *vrxv1.SyslogTarget, x *rfkit.Ext, sec *rfkit.Secrets, p Paths, path string) (*Target, []tlsFile, error) {
	if vrf := s.GetVrf(); vrf != "" && vrf != "default" {
		return nil, nil, fmt.Errorf("%w: %s.vrf %q: syslog export runs in the default VRF only (F-logging)", ErrInput, path, vrf)
	}
	t := &Target{Port: 514, Protocol: "udp", QueueSize: 10000, Template: TemplateRFC5424, Framing: "octet-counted"}
	addr := s.GetAddress()
	if a, err := netip.ParseAddr(addr); err == nil && a.Zone() == "" {
		if a.IsUnspecified() || a.IsMulticast() {
			return nil, nil, fmt.Errorf("%w: %s.address %s is not a collector address", ErrInput, path, a)
		}
		t.Host = a.Unmap().String()
	} else if len(addr) <= 253 && hostnameRe.MatchString(addr) {
		t.Host = strings.ToLower(addr)
	} else {
		return nil, nil, fmt.Errorf("%w: %s.address %q is neither an IP address nor a hostname", ErrInput, path, addr)
	}
	if s.Port != nil {
		t.Port = s.GetPort()
	}
	if t.Port < 1 || t.Port > 65535 {
		return nil, nil, fmt.Errorf("%w: %s.port %d out of range", ErrInput, path, t.Port)
	}
	proto := s.GetProtocol()
	switch proto {
	case "", "udp":
	case "tcp", "tls":
		t.Protocol = "tcp"
	default:
		return nil, nil, fmt.Errorf("%w: %s.protocol %q must be udp, tcp or tls", ErrInput, path, proto)
	}
	sev := s.GetSeverity()
	if sev == "" {
		sev = "info"
	}
	rsSev, ok := severities[sev]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s.severity %q", ErrInput, path, sev)
	}
	if x != nil {
		if err := checkExtKeys(x); err != nil {
			return nil, nil, err
		}
	}
	facs, err := x.Get("facilities").Strings()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	for _, f := range facs {
		if !slices.Contains(facilities, f) {
			return nil, nil, fmt.Errorf("%w: %s.facilities: %q is not a syslog facility (%s)", ErrInput, path, f, strings.Join(facilities, ", "))
		}
	}
	slices.Sort(facs)
	facs = slices.Compact(facs)
	sel := "*"
	if len(facs) > 0 {
		sel = strings.Join(facs, ",")
	}
	t.Filter = sel + "." + rsSev
	if f, ok, err := x.Get("format").String(); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		switch f {
		case "rfc5424":
		case "rfc3164":
			t.Template, t.Framing = TemplateRFC3164, "traditional"
		default:
			return nil, nil, fmt.Errorf("%w: %s.format %q must be rfc5424 or rfc3164", ErrInput, path, f)
		}
	}
	if q, ok, err := x.Get("queueSize").Uint(100, 1000000); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		t.QueueSize = q
	}
	tx := x.Get("tls")
	if proto != "tls" {
		if tx != nil {
			return nil, nil, fmt.Errorf("%w: %s.tls needs protocol tls", ErrInput, path)
		}
		return t, nil, nil
	}
	return buildTLS(t, i, tx, sec, p, path)
}

// actionName is vrx_export_<i>_<fnv32a of the rendered settings> (TLS material is not part of
// the hash: it is secret; a rotated key keeps the name).
func actionName(i int, t *Target) string {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s|%d|%s|%s|%s|%s|%d", t.Host, t.Port, t.Protocol, t.Filter, t.Template, t.Framing, t.QueueSize)
	if t.TLS != nil {
		fmt.Fprintf(h, "|%s|%s|%s|%s|%s", t.TLS.AuthMode, strings.Join(t.TLS.Peers, ","), t.TLS.CAFile, t.TLS.CertFile, t.TLS.KeyFile)
	}
	return fmt.Sprintf("vrx_export_%d_%08x", i, h.Sum32())
}

func checkExtKeys(x *rfkit.Ext) error {
	keys, err := x.Keys()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInput, err)
	}
	for _, k := range keys {
		switch k {
		case "address", "port", "protocol", "severity", "vrf", // schema fields
			"facilities", "format", "queueSize", "tls": // stand-ins
		default:
			return fmt.Errorf("%w: %s has unknown key %q", ErrInput, x.Path(), k)
		}
	}
	return nil
}

func buildTLS(t *Target, i int, tx *rfkit.Ext, sec *rfkit.Secrets, p Paths, path string) (*Target, []tlsFile, error) {
	if err := tx.OnlyKeys("caRef", "certRef", "keyRef", "authMode", "permittedPeers"); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	tls := &TLS{AuthMode: "x509/name"}
	if m, ok, err := tx.Get("authMode").String(); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		if m != "x509/name" && m != "x509/certvalid" {
			return nil, nil, fmt.Errorf("%w: %s.tls.authMode %q must be x509/name or x509/certvalid (anonymous TLS is refused)", ErrInput, path, m)
		}
		tls.AuthMode = m
	}
	peers, err := tx.Get("permittedPeers").Strings()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	for _, pe := range peers {
		if len(pe) > 253 || !hostnameRe.MatchString(pe) {
			return nil, nil, fmt.Errorf("%w: %s.tls.permittedPeers: %q is not a host name", ErrInput, path, pe)
		}
		tls.Peers = append(tls.Peers, strings.ToLower(pe))
	}
	if tls.AuthMode == "x509/name" && len(tls.Peers) == 0 && !hostnameRe.MatchString(t.Host) {
		return nil, nil, fmt.Errorf("%w: %s.tls: x509/name needs permittedPeers", ErrInput, path)
	}
	var files []tlsFile
	for _, f := range []struct {
		key, kind, suffix string
		dst               *string
		secret, required  bool
	}{
		{"caRef", "cert", "ca.pem", &tls.CAFile, false, true},
		{"certRef", "cert", "cert.pem", &tls.CertFile, false, false},
		{"keyRef", "key", "key.pem", &tls.KeyFile, true, false},
	} {
		ref, ok, err := tx.Get(f.key).String()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		if !ok {
			if f.required {
				return nil, nil, fmt.Errorf("%w: %s.tls.%s is required", ErrInput, path, f.key)
			}
			continue
		}
		v, err := sec.Resolve(ref, checkPEM, f.kind)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s.tls.%s: %w", ErrInput, path, f.key, err)
		}
		*f.dst = filepath.Join(p.TLSDir, fmt.Sprintf("export-%d-%s", i, f.suffix))
		files = append(files, tlsFile{Path: *f.dst, Content: v, Secret: f.secret})
	}
	if (tls.CertFile == "") != (tls.KeyFile == "") {
		return nil, nil, fmt.Errorf("%w: %s.tls: certRef and keyRef go together", ErrInput, path)
	}
	t.TLS = tls
	return t, files, nil
}
