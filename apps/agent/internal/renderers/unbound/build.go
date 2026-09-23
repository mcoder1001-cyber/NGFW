package unbound

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// ErrInvalid is wrapped by every error about the desired state (a value the renderer refuses
// to put into unbound.conf). The schema checks first; the renderer checks again (D-049).
var ErrInvalid = errors.New("unbound: invalid desired state")

func invalid(path, format string, a ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalid, path, fmt.Sprintf(format, a...))
}

var (
	objectNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	// dnsName in packages/schema: ".", or labels of [A-Za-z0-9_-] with an optional "*." lead.
	dnsNameRe  = regexp.MustCompile(`^(?:\.|(?:\*\.)?(?:[A-Za-z0-9_][A-Za-z0-9_-]{0,62}\.)*[A-Za-z0-9_][A-Za-z0-9_-]{0,62}\.?)$`)
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	mxRe       = regexp.MustCompile(`^(\d{1,5}) (\S+)$`)
	srvRe      = regexp.MustCompile(`^(\d{1,5}) (\d{1,5}) (\d{1,5}) (\S+)$`)
)

var (
	zoneTypes   = []string{"static", "transparent", "redirect", "refuse", "deny", "nodefault"}
	aclActions  = []string{"allow", "deny", "refuse", "allow_snoop"}
	recordTypes = []string{"A", "AAAA", "CNAME", "MX", "NS", "PTR", "SRV", "TXT"}
)

const (
	maxDescription = 255
	maxTXT         = 1024
)

// fqdn validates a DNS name and returns it lower-cased and absolute ("example.test.").
func fqdn(path, s string) (string, error) {
	if len(s) == 0 || len(s) > 253 || !dnsNameRe.MatchString(s) {
		return "", invalid(path, "%q is not a DNS name", s)
	}
	s = strings.ToLower(s)
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s, nil
}

// txtData encodes TXT data as one or more zone-file character-strings ("…" "…", ≤ 255
// bytes each). Everything but letters, digits, space and a few inert punctuation marks is
// written as \DDD, so no quote, backslash, apostrophe, semicolon or colon reaches the file
// raw: the value can neither end the surrounding quoting nor spell `include:`.
func txtData(path, s string) (string, error) {
	if len(s) == 0 || len(s) > maxTXT {
		return "", invalid(path, "TXT data must be 1..%d characters", maxTXT)
	}
	var chunks []string
	var b strings.Builder
	n := 0
	flush := func() {
		chunks = append(chunks, `"`+b.String()+`"`)
		b.Reset()
		n = 0
	}
	lower := strings.ToLower(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < ' ' || c > '~' {
			return "", invalid(path, "TXT data must be printable ASCII")
		}
		// The word "include" is written with its first letter escaped (\105nclude), so the
		// token never appears in the file, not even inside a quoted record.
		spellsInclude := strings.HasPrefix(lower[i:], "include")
		if n == 255 {
			flush()
		}
		isPlain := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			strings.IndexByte(" .,-_=+/!?@#%&*()[]{}<>~^|", c) >= 0
		if isPlain && !spellsInclude {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, `\%03d`, c)
		}
		n++
	}
	flush()
	return strings.Join(chunks, " "), nil
}

// rrData validates and canonicalises the data of one record.
func rrData(path, typ, data string) (string, error) {
	switch typ {
	case "A", "AAAA":
		a, err := netip.ParseAddr(data)
		if err != nil || a.Zone() != "" || (typ == "A") != a.Is4() {
			return "", invalid(path, "%s record needs an IPv%s address, got %q", typ, map[bool]string{true: "4", false: "6"}[typ == "A"], data)
		}
		return a.String(), nil
	case "CNAME", "NS", "PTR":
		return fqdn(path, data)
	case "MX":
		m := mxRe.FindStringSubmatch(data)
		if m == nil {
			return "", invalid(path, `an MX record is "<priority> <mail host>"`)
		}
		prio, err := u16(path, m[1])
		if err != nil {
			return "", err
		}
		host, err := fqdn(path, m[2])
		if err != nil {
			return "", err
		}
		return prio + " " + host, nil
	case "SRV":
		m := srvRe.FindStringSubmatch(data)
		if m == nil {
			return "", invalid(path, `an SRV record is "<priority> <weight> <port> <target>"`)
		}
		var parts []string
		for _, f := range m[1:4] {
			v, err := u16(path, f)
			if err != nil {
				return "", err
			}
			parts = append(parts, v)
		}
		target, err := fqdn(path, m[4])
		if err != nil {
			return "", err
		}
		return strings.Join(append(parts, target), " "), nil
	case "TXT":
		return txtData(path, data)
	}
	return "", invalid(path, "record type %q is not one of %v", typ, recordTypes)
}

func u16(path, s string) (string, error) {
	v, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return "", invalid(path, "%q is not 0..65535", s)
	}
	return strconv.FormatUint(v, 10), nil
}

// ----- the template model ---------------------------------------------------------------

type confData struct {
	Paths        Paths
	Resolvers    []resolverNote // comment lines: the resolvers merged into this instance
	Interfaces   []string
	Port         uint32
	Threads      uint32
	Access       []acl
	Cache        cache
	DNSSEC       bool
	AutoAnchor   bool
	QnameMin     bool
	HideIdentity bool
	HideVersion  bool
	LogQueries   bool
	NeedTLS      bool
	Zones        []localZone // global local zones (single resolver)
	Views        []view      // per-resolver local zones (several resolvers)
	Forwards     []forward
}

type acl struct{ Prefix, Action string }

type resolverNote struct{ Name, Description string }

type cache struct {
	MinTTL, MaxTTL, MsgMB, RRsetMB uint32
	Prefetch                       bool
}

type localZone struct {
	Zone, Type string
	Data       []string // complete RR text
}

type view struct {
	Name       string
	Interfaces []string
	Zones      []localZone
}

type forward struct {
	Zone  string
	Addrs []string // "<ip>@<port>[#<tls name>]"
	TLS   bool
	First bool
}

type input struct {
	dns *vrxv1.DnsService
}

func extract(desired proto.Message) (input, error) {
	switch d := desired.(type) {
	case nil:
		return input{}, nil
	case *vrxv1.DesiredState:
		return input{dns: d.GetServices().GetDns()}, nil
	case *vrxv1.ServicesConfig:
		return input{dns: d.GetDns()}, nil
	case *vrxv1.DnsService:
		return input{dns: d}, nil
	default:
		return input{}, fmt.Errorf("%w: unsupported desired type %T", ErrInvalid, desired)
	}
}

// settings are the per-instance options; merged resolvers must agree on them.
type settings struct {
	threads                                    uint32
	cache                                      cache
	dnssec, auto, qmin, hideID, hideVer, logQs bool
}

func resolverSettings(r *vrxv1.DnsResolver) settings {
	c, ds := r.GetCache(), r.GetDnssec()
	s := settings{
		threads: 1,
		cache:   cache{MinTTL: 0, MaxTTL: 86400, MsgMB: 4, RRsetMB: 8},
		dnssec:  ds == nil || ds.Enabled == nil || ds.GetEnabled(),
		auto:    ds == nil || ds.TrustAnchorAuto == nil || ds.GetTrustAnchorAuto(),
		qmin:    r.QnameMinimisation == nil || r.GetQnameMinimisation(),
		hideID:  r.HideIdentity == nil || r.GetHideIdentity(),
		hideVer: r.HideVersion == nil || r.GetHideVersion(),
		logQs:   r.GetLogQueries(),
	}
	if r.Threads != nil {
		s.threads = r.GetThreads()
	}
	if c != nil {
		if c.MinTtlSec != nil {
			s.cache.MinTTL = c.GetMinTtlSec()
		}
		if c.MaxTtlSec != nil {
			s.cache.MaxTTL = c.GetMaxTtlSec()
		}
		if c.MsgCacheMb != nil {
			s.cache.MsgMB = c.GetMsgCacheMb()
		}
		if c.RrsetCacheMb != nil {
			s.cache.RRsetMB = c.GetRrsetCacheMb()
		}
		s.cache.Prefetch = c.GetPrefetch()
	}
	return s
}

// build turns every enabled resolver into one Unbound instance. One resolver renders its
// local zones globally; several resolvers (same VRF, same instance settings) each get a
// `view:` holding their local zones, bound to their listen addresses with interface-view.
func (r *Renderer) build(in input) (*confData, error) {
	resolvers := in.dns.GetResolvers()
	var names []string
	for _, n := range sortedKeys(resolvers) {
		if res := resolvers[n]; res.Enabled == nil || res.GetEnabled() {
			names = append(names, n)
		}
	}
	d := &confData{Paths: r.paths, Port: 53, Threads: 1}
	if len(names) == 0 {
		return d, nil
	}
	var base settings
	vrf := ""
	aclSeen := map[string]string{}
	fwdSeen := map[string]string{}
	ports := map[uint32]bool{}
	for i, name := range names {
		res := resolvers[name]
		path := "resolvers/" + name
		if !objectNameRe.MatchString(name) {
			return nil, invalid(path, "name %q is not an object name", name)
		}
		desc, err := checkDescription(path+"/description", res.GetDescription())
		if err != nil {
			return nil, err
		}
		d.Resolvers = append(d.Resolvers, resolverNote{Name: name, Description: desc})
		v := res.GetVrf()
		if v == "" {
			v = "default"
		}
		if !objectNameRe.MatchString(v) {
			return nil, invalid(path+"/vrf", "VRF %q is not an object name", v)
		}
		st := resolverSettings(res)
		if i == 0 {
			base, vrf = st, v
		} else {
			if v != vrf {
				return nil, invalid(path+"/vrf", "VRF %q differs from %q: one Unbound instance serves one VRF", v, vrf)
			}
			if st != base {
				return nil, invalid(path, "instance settings (threads, cache, dnssec, qname minimisation, hide-*, log queries) differ from resolver %q: one Unbound instance has one set", names[0])
			}
		}
		// listen
		if len(res.GetListen()) == 0 {
			return nil, invalid(path+"/listen", "a resolver needs a listen address")
		}
		var ifaces []string
		for j, l := range res.GetListen() {
			lp := fmt.Sprintf("%s/listen/%d", path, j)
			a, err := netip.ParseAddr(l.GetAddress())
			if err != nil || a.Zone() != "" {
				return nil, invalid(lp+"/address", "%q is not an IP address", l.GetAddress())
			}
			if !r.paths.listenAllowed(a) {
				return nil, invalid(lp+"/address", "%s is outside the test scope (loopback only)", a)
			}
			port := l.GetPort()
			if l.Port == nil {
				port = 53
			}
			if port == 0 || port > 65535 {
				return nil, invalid(lp+"/port", "port %d outside 1..65535", port)
			}
			ports[port] = true
			ifaces = append(ifaces, fmt.Sprintf("%s@%d", a, port))
		}
		d.Interfaces = append(d.Interfaces, ifaces...)
		// access control
		for j, ac := range res.GetAccessControl() {
			ap := fmt.Sprintf("%s/accessControl/%d", path, j)
			pfx, err := renderers.Network(ac.GetPrefix())
			if err != nil {
				return nil, invalid(ap+"/prefix", "%q is not a CIDR prefix", ac.GetPrefix())
			}
			action := ac.GetAction()
			if action == "" {
				action = "allow"
			}
			if !slices.Contains(aclActions, action) {
				return nil, invalid(ap+"/action", "action %q is not one of %v", action, aclActions)
			}
			if prev, ok := aclSeen[pfx]; ok {
				if prev != action {
					return nil, invalid(ap, "prefix %s has conflicting actions %s / %s across resolvers", pfx, prev, action)
				}
				continue
			}
			aclSeen[pfx] = action
			d.Access = append(d.Access, acl{Prefix: pfx, Action: action})
		}
		// forwarders: the resolver's default forwarders are the "." forward zone
		zones := res.GetForwardZones()
		if len(res.GetForwarders()) > 0 {
			zones = append([]*vrxv1.DnsForwardZone{{Zone: proto.String("."), Forwarders: res.GetForwarders()}}, zones...)
		}
		ownZones := map[string]bool{}
		for j, fz := range zones {
			fp := fmt.Sprintf("%s/forwardZones/%d", path, j)
			f, err := buildForward(fp, fz)
			if err != nil {
				return nil, err
			}
			if ownZones[f.Zone] {
				return nil, invalid(fp+"/zone", "forward zone %s twice (the resolver forwarders are zone \".\")", f.Zone)
			}
			ownZones[f.Zone] = true
			key := fmt.Sprintf("%v", f)
			if prev, ok := fwdSeen[f.Zone]; ok {
				if prev != key {
					return nil, invalid(fp+"/zone", "forward zone %s is defined differently by several resolvers", f.Zone)
				}
				continue
			}
			fwdSeen[f.Zone] = key
			d.Forwards = append(d.Forwards, f)
			d.NeedTLS = d.NeedTLS || f.TLS
		}
		// local zones
		var zones2 []localZone
		zoneSeen := map[string]bool{}
		for j, lz := range res.GetLocalZones() {
			z, err := buildLocalZone(fmt.Sprintf("%s/localZones/%d", path, j), lz)
			if err != nil {
				return nil, err
			}
			if zoneSeen[z.Zone] {
				return nil, invalid(fmt.Sprintf("%s/localZones/%d", path, j), "local zone %s twice", z.Zone)
			}
			zoneSeen[z.Zone] = true
			zones2 = append(zones2, z)
		}
		sort.Slice(zones2, func(a, b int) bool { return zones2[a].Zone < zones2[b].Zone })
		if len(names) == 1 {
			d.Zones = zones2
		} else if len(zones2) > 0 {
			d.Views = append(d.Views, view{Name: name, Interfaces: ifaces, Zones: zones2})
		}
	}
	sort.Strings(d.Interfaces)
	if dup := firstDup(d.Interfaces); dup != "" {
		return nil, invalid("resolvers", "listen address %s used twice", dup)
	}
	sort.Slice(d.Access, func(a, b int) bool { return d.Access[a].Prefix < d.Access[b].Prefix })
	sort.Slice(d.Forwards, func(a, b int) bool { return d.Forwards[a].Zone < d.Forwards[b].Zone })
	d.Port = sortedKeys(ports)[0]
	d.Threads, d.Cache = base.threads, base.cache
	if d.Threads < 1 || d.Threads > 64 {
		return nil, invalid("resolvers", "threads %d outside 1..64", d.Threads)
	}
	if d.Cache.MinTTL > d.Cache.MaxTTL {
		return nil, invalid("resolvers", "cache minimum TTL exceeds the maximum")
	}
	if d.Cache.MsgMB < 1 || d.Cache.MsgMB > 4096 || d.Cache.RRsetMB < 1 || d.Cache.RRsetMB > 8192 {
		return nil, invalid("resolvers", "cache sizes outside the schema bounds")
	}
	d.DNSSEC, d.AutoAnchor = base.dnssec, base.auto
	d.QnameMin, d.HideIdentity, d.HideVersion, d.LogQueries = base.qmin, base.hideID, base.hideVer, base.logQs
	return d, nil
}

func buildForward(path string, fz *vrxv1.DnsForwardZone) (forward, error) {
	zone, err := fqdn(path+"/zone", fz.GetZone())
	if err != nil {
		return forward{}, err
	}
	if len(fz.GetForwarders()) == 0 {
		return forward{}, invalid(path+"/forwarders", "a forward zone needs a forwarder")
	}
	f := forward{Zone: zone, First: fz.GetForwardFirst()}
	for k, u := range fz.GetForwarders() {
		up := fmt.Sprintf("%s/forwarders/%d", path, k)
		a, err := netip.ParseAddr(u.GetAddress())
		if err != nil || a.Zone() != "" {
			return forward{}, invalid(up+"/address", "%q is not an IP address", u.GetAddress())
		}
		port := u.GetPort()
		if u.Port == nil {
			port = 53
			if u.GetTls() {
				port = 853
			}
		}
		if port == 0 || port > 65535 {
			return forward{}, invalid(up+"/port", "port %d outside 1..65535", port)
		}
		addr := fmt.Sprintf("%s@%d", a, port)
		if u.TlsServerName != nil {
			if !u.GetTls() {
				return forward{}, invalid(up+"/tlsServerName", "tlsServerName applies to DNS-over-TLS upstreams only")
			}
			if len(u.GetTlsServerName()) > 253 || !hostnameRe.MatchString(u.GetTlsServerName()) {
				return forward{}, invalid(up+"/tlsServerName", "%q is not a host name", u.GetTlsServerName())
			}
			addr += "#" + strings.ToLower(u.GetTlsServerName())
		}
		if k > 0 && u.GetTls() != f.TLS {
			return forward{}, invalid(up+"/tls", "Unbound sets TLS per forward zone: all forwarders of a zone must agree")
		}
		f.TLS = u.GetTls()
		f.Addrs = append(f.Addrs, addr)
	}
	return f, nil
}

func buildLocalZone(path string, lz *vrxv1.DnsLocalZone) (localZone, error) {
	zone, err := fqdn(path+"/zone", lz.GetZone())
	if err != nil {
		return localZone{}, err
	}
	typ := lz.GetType()
	if typ == "" {
		typ = "static"
	}
	if !slices.Contains(zoneTypes, typ) {
		return localZone{}, invalid(path+"/type", "zone type %q is not one of %v", typ, zoneTypes)
	}
	z := localZone{Zone: zone, Type: typ}
	for k, rec := range lz.GetRecords() {
		rp := fmt.Sprintf("%s/records/%d", path, k)
		name, err := fqdn(rp+"/name", rec.GetName())
		if err != nil {
			return localZone{}, err
		}
		typ := rec.GetType()
		if !slices.Contains(recordTypes, typ) {
			return localZone{}, invalid(rp+"/type", "record type %q is not one of %v", typ, recordTypes)
		}
		ttl := rec.GetTtlSec()
		if rec.TtlSec == nil {
			ttl = 3600
		}
		if ttl > 604800 {
			return localZone{}, invalid(rp+"/ttlSec", "TTL %d above 604800", ttl)
		}
		data, err := rrData(rp+"/data", typ, rec.GetData())
		if err != nil {
			return localZone{}, err
		}
		z.Data = append(z.Data, fmt.Sprintf("%s %d IN %s %s", name, ttl, typ, data))
	}
	return z, nil
}

func checkDescription(path, s string) (string, error) {
	if utf8.RuneCountInString(s) > maxDescription {
		return "", invalid(path, "description longer than %d characters", maxDescription)
	}
	if _, err := renderers.Line(s); err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}
	return s, nil
}

func firstDup(sorted []string) string {
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			return sorted[i]
		}
	}
	return ""
}

func sortedKeys[K interface{ ~string | ~uint32 }, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
