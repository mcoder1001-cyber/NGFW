package snmpd

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/rfkit"
)

// ErrInput is wrapped by every error about the desired state, so the commit engine reports it
// as a validation issue with the JSON path in the message.
var ErrInput = errors.New("snmpd: invalid desired state")

// DefaultView is the view every community and user gets unless it names one.
const DefaultView = "vrx_all"

// Model is the validated, fully resolved view of services.snmp: exactly what the template
// renders. Secret values are resolved (never logged: the renderer's Redactor knows them).
type Model struct {
	// Enabled false renders a config that answers nobody (see README "disabled").
	Enabled bool
	// Listen are the agentaddress transports ("udp:127.0.0.1:161", "udp6:[::1]:161").
	Listen []string
	// Warnings are rendered as "# WARNING:" lines (Warnings() reads them back): loopback-only
	// default, wildcard listen addresses (review M1).
	Warnings []string
	// EngineID is the SNMPv3 engine id in hex without 0x ("" = daemon default).
	EngineID                    string
	SysName, SysLoc, SysContact string
	// SysServices is sysServices (0 = not rendered).
	SysServices uint32
	Views       []View
	Communities []Community
	Users       []User
	Traps       []Trap
	AgentX      string // AgentX master socket path
	Disks       []Disk
	Load        *Load
}

// View is one `view <name> included|excluded <oid>` group.
type View struct {
	Name    string
	Include []string // numeric OIDs
	Exclude []string
}

// Community is one (ro|rw)community[6] line (one per source).
type Community struct {
	Name   string // document key (never rendered)
	Secret string // resolved community string
	RW     bool
	Source string // "default" or a canonical network prefix
	IPv6   bool
	View   string
}

// User is one SNMPv3 USM user: createUser + (ro|rw)user.
type User struct {
	Name      string
	Level     string // noauth | auth | priv
	AuthProto string // MD5 | SHA | SHA-256 | SHA-512
	PrivProto string // AES | DES
	Auth      string // resolved passphrases
	Priv      string
	RW        bool
	View      string
}

// Trap is one notification receiver.
type Trap struct {
	Version   string // v2c | v3
	Inform    bool
	Host      string // IP (IPv6 in brackets) or hostname
	Port      uint32
	Community string // v2c: resolved community string
	User      *User  // v3
}

// Disk is one `disk <path> <min>%` monitor.
type Disk struct {
	Path       string
	MinPercent uint32
}

// Load is the `load <1m> <5m> <15m>` monitor.
type Load struct{ Max1, Max5, Max15 uint32 }

var (
	tokenRe      = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	hostnameRe   = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$`)
	oidRe        = regexp.MustCompile(`^\.?[0-9]{1,10}(?:\.[0-9]{1,10}){0,63}$`)
	engineIDRe   = regexp.MustCompile(`^(?:[0-9a-fA-F]{2}){5,32}$`)
	passphraseRe = regexp.MustCompile(`^[A-Za-z0-9_.,:;@%+=/~^*!?-]{8,64}$`)
	diskPathRe   = regexp.MustCompile(`^/[A-Za-z0-9_./-]{0,127}$`)
)

// symbolicOIDs is the fixed allow-list of symbolic view subtrees (the daemon runs without MIB
// files in tests, so the renderer always writes the numeric form).
var symbolicOIDs = map[string]string{
	"all":         ".1",
	"internet":    ".1.3.6.1",
	"mib-2":       ".1.3.6.1.2.1",
	"system":      ".1.3.6.1.2.1.1",
	"interfaces":  ".1.3.6.1.2.1.2",
	"ip":          ".1.3.6.1.2.1.4",
	"host":        ".1.3.6.1.2.1.25",
	"ifMIB":       ".1.3.6.1.2.1.31",
	"enterprises": ".1.3.6.1.4.1",
	"ucdavis":     ".1.3.6.1.4.1.2021",
	"snmpV2":      ".1.3.6.1.6",
}

// OID validates a view subtree: numeric (dotted) or a name from the symbolic allow-list, and
// returns the numeric form with a leading dot.
func OID(s string) (string, error) {
	if n, ok := symbolicOIDs[s]; ok {
		return n, nil
	}
	if !oidRe.MatchString(s) {
		return "", fmt.Errorf("%w: OID %q must be numeric (1.3.6.1…) or one of the symbolic names %s", ErrInput, s, strings.Join(slices.Sorted(maps.Keys(symbolicOIDs)), ", "))
	}
	if !strings.HasPrefix(s, ".") {
		s = "." + s
	}
	return s, nil
}

// Token validates a name the daemon parses as one word (user, view): [A-Za-z0-9_.-]{1,64}.
func Token(s string) (string, error) {
	if !tokenRe.MatchString(s) {
		return "", fmt.Errorf("%w: %q must match %s", ErrInput, s, tokenRe)
	}
	return s, nil
}

// Text validates a rest-of-line value (sysLocation, sysContact): 1–255 printable ASCII
// characters, no leading or trailing blank (snmpd would trim them, so the value would drift).
func Text(s string) (string, error) {
	if s == "" || len(s) > 255 {
		return "", fmt.Errorf("%w: text must be 1–255 characters (got %d)", ErrInput, len(s))
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return "", fmt.Errorf("%w: text must be printable ASCII (byte 0x%02x at %d)", ErrInput, s[i], i)
		}
	}
	if strings.TrimSpace(s) != s {
		return "", fmt.Errorf("%w: text must not start or end with a blank", ErrInput)
	}
	return s, nil
}

// communityRe: one word, at least 8 characters (review L1: a short community is guessable and
// would make redaction of error texts ambiguous).
var communityRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{8,64}$`)

func checkCommunity(v string) error {
	if !communityRe.MatchString(v) {
		return fmt.Errorf("a community string must be 8–64 characters of [A-Za-z0-9_.-]")
	}
	return nil
}

func checkPassphrase(v string) error {
	if !passphraseRe.MatchString(v) {
		return fmt.Errorf("a USM passphrase must be 8–64 characters of [A-Za-z0-9_.,:;@%%+=/~^*!?-] (no blanks, quotes or backslash)")
	}
	return nil
}

var (
	authProtos = map[string]string{"md5": "MD5", "sha": "SHA", "sha256": "SHA-256", "sha512": "SHA-512"}
	privProtos = map[string]string{"aes": "AES", "des": "DES"}
	levels     = map[string]string{"noAuthNoPriv": "noauth", "authNoPriv": "auth", "authPriv": "priv"}
)

// BuildModel validates services.snmp and resolves its secrets. The former D-055 stand-ins (views,
// per-credential view, sysServices, monitors) are typed contract fields since F-snmp (D-086); ext is
// kept for the signature and no longer read.
func BuildModel(ds *vrxv1.DesiredState, _ *rfkit.Ext, sec *rfkit.Secrets, agentx string) (*Model, error) {
	snmp := ds.GetServices().GetSnmp()
	m := &Model{Enabled: snmp.GetEnabled(), AgentX: agentx}
	if !m.Enabled {
		return m, nil
	}
	if vrf := snmp.GetVrf(); vrf != "" && vrf != "default" {
		return nil, fmt.Errorf("%w: services.snmp.vrf %q: only the default VRF is supported until F-snmp binds snmpd to a VRF", ErrInput, vrf)
	}
	if err := buildSystem(m, snmp); err != nil {
		return nil, err
	}
	if err := buildListen(m, snmp); err != nil {
		return nil, err
	}
	views, err := buildViews(snmp.GetViews())
	if err != nil {
		return nil, err
	}
	m.Views = views
	viewOf := func(view *string, path string) (string, error) {
		if view == nil {
			return DefaultView, nil
		}
		v := *view
		if !slices.ContainsFunc(views, func(vw View) bool { return vw.Name == v }) {
			return "", fmt.Errorf("%w: %s.view %q is not defined in services.snmp.views", ErrInput, path, v)
		}
		return v, nil
	}
	secretOf := map[string]string{} // community name → resolved string
	for _, name := range slices.Sorted(maps.Keys(snmp.GetCommunities())) {
		c := snmp.GetCommunities()[name]
		path := "services.snmp.communities." + name
		if _, err := Token(name); err != nil {
			return nil, fmt.Errorf("%w: %s: community name: %w", ErrInput, path, err)
		}
		val, err := sec.Resolve(c.GetSecretRef(), checkCommunity, "password")
		if err != nil {
			return nil, fmt.Errorf("%w: %s.secretRef: %w", ErrInput, path, err)
		}
		secretOf[name] = val
		view, err := viewOf(c.View, path)
		if err != nil {
			return nil, err
		}
		rw, err := access(c.GetAccess(), path)
		if err != nil {
			return nil, err
		}
		sources := c.GetSources()
		if len(sources) == 0 {
			m.Communities = append(m.Communities, Community{Name: name, Secret: val, RW: rw, Source: "default", View: view})
			m.Communities = append(m.Communities, Community{Name: name, Secret: val, RW: rw, Source: "default", IPv6: true, View: view})
			continue
		}
		for i, s := range sources {
			p, err := netip.ParsePrefix(s)
			if err != nil || p.Addr().Zone() != "" {
				return nil, fmt.Errorf("%w: %s.sources[%d] %q is not a CIDR prefix", ErrInput, path, i, s)
			}
			m.Communities = append(m.Communities, Community{
				Name: name, Secret: val, RW: rw, Source: p.Masked().String(), IPv6: p.Addr().Is6() && !p.Addr().Is4In6(), View: view,
			})
		}
	}
	users := map[string]User{}
	for _, name := range slices.Sorted(maps.Keys(snmp.GetV3Users())) {
		u := snmp.GetV3Users()[name]
		path := "services.snmp.v3Users." + name
		if _, err := Token(name); err != nil {
			return nil, fmt.Errorf("%w: %s: user name: %w", ErrInput, path, err)
		}
		user, err := buildUser(name, u, sec, path)
		if err != nil {
			return nil, err
		}
		if user.View, err = viewOf(u.View, path); err != nil {
			return nil, err
		}
		m.Users = append(m.Users, *user)
		users[name] = *user
	}
	for i, t := range snmp.GetTrapReceivers() {
		tr, err := buildTrap(i, t, secretOf, users)
		if err != nil {
			return nil, err
		}
		m.Traps = append(m.Traps, *tr)
	}
	if err := buildMonitors(m, snmp.GetMonitors()); err != nil {
		return nil, err
	}
	return m, nil
}

func access(a, path string) (bool, error) {
	switch a {
	case "", "ro":
		return false, nil
	case "rw":
		return true, nil
	}
	return false, fmt.Errorf("%w: %s.access %q must be ro or rw", ErrInput, path, a)
}

func buildSystem(m *Model, snmp *vrxv1.SnmpService) error {
	if n := snmp.GetSysName(); n != "" {
		if len(n) > 253 || !hostnameRe.MatchString(n) {
			return fmt.Errorf("%w: services.snmp.sysName %q is not a hostname", ErrInput, n)
		}
		m.SysName = n
	}
	for _, f := range []struct {
		v    string
		dst  *string
		path string
	}{{snmp.GetSysLocation(), &m.SysLoc, "sysLocation"}, {snmp.GetSysContact(), &m.SysContact, "sysContact"}} {
		if f.v == "" {
			continue
		}
		t, err := Text(f.v)
		if err != nil {
			return fmt.Errorf("services.snmp.%s: %w", f.path, err)
		}
		*f.dst = t
	}
	if id := snmp.GetEngineId(); id != "" {
		if !engineIDRe.MatchString(id) {
			return fmt.Errorf("%w: services.snmp.engineId must be 5–32 hex bytes", ErrInput)
		}
		m.EngineID = strings.ToLower(id)
	}
	if svc := snmp.GetSysServices(); svc > 127 {
		return fmt.Errorf("%w: services.snmp.sysServices %d out of range 0..127", ErrInput, svc)
	}
	m.SysServices = snmp.GetSysServices()
	return nil
}

func buildListen(m *Model, snmp *vrxv1.SnmpService) error {
	if len(snmp.GetListen()) == 0 {
		// Never all addresses by default (review M1): without an explicit listen address the
		// agent answers on loopback only.
		m.Listen = []string{"udp:127.0.0.1:161", "udp6:[::1]:161"}
		m.Warnings = append(m.Warnings, "services.snmp.listen is empty: snmpd listens on 127.0.0.1:161 and [::1]:161 only; set listen to reach it from the network")
		return nil
	}
	seen := map[string]bool{}
	for i, l := range snmp.GetListen() {
		a, err := netip.ParseAddr(l.GetAddress())
		if err != nil || a.Zone() != "" {
			return fmt.Errorf("%w: services.snmp.listen[%d].address %q is not an IP address", ErrInput, i, l.GetAddress())
		}
		port := l.GetPort()
		if l.Port == nil {
			port = 161
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("%w: services.snmp.listen[%d].port %d out of range", ErrInput, i, port)
		}
		t := fmt.Sprintf("udp:%s:%d", a.Unmap(), port)
		if a.Is6() && !a.Is4In6() {
			t = fmt.Sprintf("udp6:[%s]:%d", a, port)
		}
		if seen[t] {
			return fmt.Errorf("%w: services.snmp.listen[%d] duplicates %s", ErrInput, i, t)
		}
		seen[t] = true
		m.Listen = append(m.Listen, t)
		if a.IsUnspecified() {
			m.Warnings = append(m.Warnings, fmt.Sprintf("services.snmp.listen[%d] %s listens on every address of the host (management and data-plane sides)", i, t))
		}
	}
	return nil
}

func buildViews(in map[string]*vrxv1.SnmpView) ([]View, error) {
	views := []View{{Name: DefaultView, Include: []string{".1"}}}
	for _, name := range slices.Sorted(maps.Keys(in)) {
		path := "services.snmp.views." + name
		if _, err := Token(name); err != nil || name == DefaultView {
			return nil, fmt.Errorf("%w: %s: view name must match %s and not be %s", ErrInput, path, tokenRe, DefaultView)
		}
		x := in[name]
		v := View{Name: name}
		for _, part := range []struct {
			key  string
			list []string
			dst  *[]string
		}{{"include", x.GetInclude(), &v.Include}, {"exclude", x.GetExclude(), &v.Exclude}} {
			if len(part.list) > 32 {
				return nil, fmt.Errorf("%w: %s.%s has more than 32 subtrees", ErrInput, path, part.key)
			}
			for _, o := range part.list {
				n, err := OID(o)
				if err != nil {
					return nil, fmt.Errorf("%s.%s: %w", path, part.key, err)
				}
				*part.dst = append(*part.dst, n)
			}
		}
		if len(v.Include) == 0 {
			return nil, fmt.Errorf("%w: %s.include must name at least one subtree", ErrInput, path)
		}
		views = append(views, v)
	}
	return views, nil
}

func buildUser(name string, u *vrxv1.SnmpService_V3User, sec *rfkit.Secrets, path string) (*User, error) {
	levelName := u.GetSecurityLevel()
	if levelName == "" {
		levelName = "authPriv"
	}
	level, ok := levels[levelName]
	if !ok {
		return nil, fmt.Errorf("%w: %s.securityLevel %q is not noAuthNoPriv|authNoPriv|authPriv", ErrInput, path, levelName)
	}
	user := &User{Name: name, Level: level}
	var err error
	if user.RW, err = access(u.GetAccess(), path); err != nil {
		return nil, err
	}
	if level == "noauth" {
		if u.GetAuthRef() != "" || u.GetPrivRef() != "" {
			return nil, fmt.Errorf("%w: %s: noAuthNoPriv takes no authRef/privRef", ErrInput, path)
		}
		return user, nil
	}
	ap := cmp.Or(u.GetAuthProtocol(), "sha")
	if user.AuthProto, ok = authProtos[ap]; !ok {
		return nil, fmt.Errorf("%w: %s.authProtocol %q is not supported by net-snmp 5.9 here (md5|sha|sha256|sha512)", ErrInput, path, ap)
	}
	if user.Auth, err = sec.Resolve(u.GetAuthRef(), checkPassphrase, "password"); err != nil {
		return nil, fmt.Errorf("%w: %s.authRef: %w", ErrInput, path, err)
	}
	if level == "auth" {
		if u.GetPrivRef() != "" {
			return nil, fmt.Errorf("%w: %s: authNoPriv takes no privRef", ErrInput, path)
		}
		return user, nil
	}
	pp := cmp.Or(u.GetPrivProtocol(), "aes")
	if user.PrivProto, ok = privProtos[pp]; !ok {
		return nil, fmt.Errorf("%w: %s.privProtocol %q is not supported by the installed net-snmp (aes|des; no AES-256 in this build)", ErrInput, path, pp)
	}
	if user.Priv, err = sec.Resolve(u.GetPrivRef(), checkPassphrase, "password"); err != nil {
		return nil, fmt.Errorf("%w: %s.privRef: %w", ErrInput, path, err)
	}
	return user, nil
}

func buildTrap(i int, t *vrxv1.SnmpService_TrapReceiver, secretOf map[string]string, users map[string]User) (*Trap, error) {
	path := fmt.Sprintf("services.snmp.trapReceivers[%d]", i)
	tr := &Trap{Version: cmp.Or(t.GetVersion(), "v2c"), Inform: t.GetInform(), Port: 162}
	if t.Port != nil {
		tr.Port = t.GetPort()
	}
	if tr.Port < 1 || tr.Port > 65535 {
		return nil, fmt.Errorf("%w: %s.port %d out of range", ErrInput, path, tr.Port)
	}
	addr := t.GetAddress()
	if a, err := netip.ParseAddr(addr); err == nil && a.Zone() == "" {
		tr.Host = a.Unmap().String()
		if a.Is6() && !a.Is4In6() {
			tr.Host = "[" + a.String() + "]"
		}
	} else if len(addr) <= 253 && hostnameRe.MatchString(addr) {
		tr.Host = strings.ToLower(addr)
	} else {
		return nil, fmt.Errorf("%w: %s.address %q is neither an IP address nor a hostname", ErrInput, path, addr)
	}
	switch tr.Version {
	case "v2c":
		val, ok := secretOf[t.GetCommunity()]
		if !ok || t.GetUser() != "" {
			return nil, fmt.Errorf("%w: %s: a v2c receiver needs a community defined in services.snmp.communities and no user", ErrInput, path)
		}
		tr.Community = val
	case "v3":
		u, ok := users[t.GetUser()]
		if !ok || t.GetCommunity() != "" {
			return nil, fmt.Errorf("%w: %s: a v3 receiver needs a user defined in services.snmp.v3Users and no community", ErrInput, path)
		}
		tr.User = &u
	default:
		return nil, fmt.Errorf("%w: %s.version %q must be v2c or v3", ErrInput, path, tr.Version)
	}
	return tr, nil
}

func buildMonitors(m *Model, mon *vrxv1.SnmpMonitors) error {
	if mon == nil {
		return nil
	}
	if len(mon.GetDisks()) > 16 {
		return fmt.Errorf("%w: services.snmp.monitors.disks has more than 16 entries", ErrInput)
	}
	for i, d := range mon.GetDisks() {
		path := fmt.Sprintf("services.snmp.monitors.disks[%d]", i)
		p := d.GetPath()
		if !diskPathRe.MatchString(p) || strings.Contains(p, "//") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") {
			return fmt.Errorf("%w: %s.path %q must be an absolute clean path of [A-Za-z0-9_./-]", ErrInput, path, p)
		}
		pct := uint32(10)
		if d.MinPercent != nil {
			pct = d.GetMinPercent()
			if pct < 1 || pct > 99 {
				return fmt.Errorf("%w: %s.minPercent %d out of range 1..99", ErrInput, path, pct)
			}
		}
		m.Disks = append(m.Disks, Disk{Path: p, MinPercent: pct})
	}
	if l := mon.GetLoad(); l != nil {
		out := &Load{}
		for _, f := range []struct {
			k   string
			v   *uint32
			dst *uint32
		}{{"max1", l.Max1, &out.Max1}, {"max5", l.Max5, &out.Max5}, {"max15", l.Max15, &out.Max15}} {
			if f.v == nil {
				return fmt.Errorf("%w: services.snmp.monitors.load.%s is required", ErrInput, f.k)
			}
			if *f.v < 1 || *f.v > 1000 {
				return fmt.Errorf("%w: services.snmp.monitors.load.%s %d out of range 1..1000", ErrInput, f.k, *f.v)
			}
			*f.dst = *f.v
		}
		m.Load = out
	}
	return nil
}
