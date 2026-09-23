package kea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// input is what the renderer needs from the desired state.
type input struct {
	dhcp       *vrxv1.DhcpService
	interfaces map[string]*vrxv1.Interface // nil unless a DesiredState was given
}

// extract accepts the whole DesiredState, the services domain or the dhcp sub-tree.
func extract(desired proto.Message) (input, error) {
	switch d := desired.(type) {
	case nil:
		return input{}, nil
	case *vrxv1.DesiredState:
		return input{dhcp: d.GetServices().GetDhcp(), interfaces: d.GetInterfaces()}, nil
	case *vrxv1.ServicesConfig:
		return input{dhcp: d.GetDhcp()}, nil
	case *vrxv1.DhcpService:
		return input{dhcp: d}, nil
	default:
		return input{}, fmt.Errorf("%w: unsupported desired type %T", ErrInvalid, desired)
	}
}

// Kea's standard option definitions (Kea 3.0.3, probed with `kea-dhcp4 -t` / `kea-dhcp6 -t`:
// an option-def for these codes is rejected as "unable to override a standard option"). Any
// other code needs an option-def (text data) or csv-format false (0x… hex data).
var (
	std4 = codeSet("1-42 44-79 81 82 85-94 97-101 108 112-114 116-119 121 122 124 125 136-138 141 143 146 " +
		"151-157 159 162 212 213")
	std6 = codeSet("1-9 11-34 36-48 51-53 56-62 64-67 74 79 80 82 83 87 88 94-96 103 135 136 143 144 148")
)

func codeSet(spec string) map[uint32]bool {
	out := map[uint32]bool{}
	for _, f := range strings.Fields(spec) {
		lo, hi, _ := strings.Cut(f, "-")
		a, _ := strconv.Atoi(lo)
		b := a
		if hi != "" {
			b, _ = strconv.Atoi(hi)
		}
		for c := a; c <= b; c++ {
			out[uint32(c)] = true //nolint:gosec // small constants
		}
	}
	return out
}

// Typed options (schema fields that are not free option data), per family.
type typedOption struct {
	name string
	code uint32
}

var (
	optRouters4    = typedOption{"routers", 3}
	optDNS4        = typedOption{"domain-name-servers", 6}
	optDomainName4 = typedOption{"domain-name", 15}
	optNTP4        = typedOption{"ntp-servers", 42}
	optSearch4     = typedOption{"domain-search", 119}
	optDNS6        = typedOption{"dns-servers", 23}
	optSNTP6       = typedOption{"sntp-servers", 31}
	optSearch6     = typedOption{"domain-search", 24}
)

// famBuild accumulates one family's configuration.
type famBuild struct {
	family     int
	space      string
	interfaces []string
	subnets    []subnet
	optionDefs map[uint32]optionDef
	usedIDs    map[uint32]bool
	vrf        string
	vrfOwner   string
}

// buildFamily renders every enabled server of one family into one Kea config (one kea-dhcp4 /
// kea-dhcp6 process serves all of them). Server-level settings (lease time, timers,
// authoritative, global options) are pushed down to each subnet so servers stay independent.
func (r *Renderer) buildFamily(in input, family int) (serverConfig, error) {
	fb := &famBuild{family: family, space: fmt.Sprintf("dhcp%d", family), optionDefs: map[uint32]optionDef{}, usedIDs: map[uint32]bool{}}
	servers := in.dhcp.GetServers()
	for _, name := range sortedKeys(servers) {
		s := servers[name]
		if s.Enabled != nil && !s.GetEnabled() {
			continue
		}
		fam := 4
		switch s.GetFamily() {
		case "", "ipv4":
		case "ipv6":
			fam = 6
		default:
			return serverConfig{}, invalid("servers/"+name+"/family", "unknown family %q", s.GetFamily())
		}
		if fam != family {
			continue
		}
		if err := r.addServer(fb, name, s); err != nil {
			return serverConfig{}, err
		}
	}
	sort.Strings(fb.interfaces)
	fb.interfaces = slices.Compact(fb.interfaces)
	if len(fb.interfaces) > maxInterfaceCnt {
		return serverConfig{}, invalid("servers", "%d interfaces exceed %d", len(fb.interfaces), maxInterfaceCnt)
	}
	sort.Slice(fb.subnets, func(i, j int) bool { return fb.subnets[i].ID < fb.subnets[j].ID })

	cfg := serverConfig{
		InterfacesConfig: interfacesConfig{Interfaces: nonNil(fb.interfaces), ReDetect: true},
		ControlSockets:   []controlSocket{{SocketType: "unix", SocketName: r.paths.socket(family)}},
		LeaseDatabase: leaseDatabase{
			Type: "memfile", Persist: true, Name: r.paths.Leases4(), LFCInterval: r.paths.LFCInterval,
		},
		Loggers: []logger{{
			Name:          fmt.Sprintf("kea-dhcp%d", family),
			OutputOptions: []outputOption{{Output: r.paths.Log4(), MaxSize: 10 << 20, MaxVer: 4}},
			Severity:      "INFO",
		}},
	}
	if family == 6 {
		cfg.LeaseDatabase.Name = r.paths.Leases6()
		cfg.Loggers[0].OutputOptions[0].Output = r.paths.Log6()
	} else {
		cfg.InterfacesConfig.DHCPSocketType = r.paths.SocketType
	}
	if r.leaseCmdsHook != "" {
		cfg.HooksLibraries = []hookLibrary{{Library: r.leaseCmdsHook}}
	}
	for _, code := range sortedKeys(fb.optionDefs) {
		cfg.OptionDef = append(cfg.OptionDef, fb.optionDefs[code])
	}
	subnets := fb.subnets
	if subnets == nil {
		subnets = []subnet{}
	}
	if family == 6 {
		cfg.Subnet6 = &subnets
	} else {
		cfg.Subnet4 = &subnets
	}
	return cfg, nil
}

func (r *Renderer) addServer(fb *famBuild, name string, s *vrxv1.DhcpServer) error {
	path := "servers/" + name
	if err := checkName(path, name); err != nil {
		return err
	}
	vrf := s.GetVrf()
	if vrf == "" {
		vrf = "default"
	}
	if err := checkName(path+"/vrf", vrf); err != nil {
		return err
	}
	if fb.vrf == "" {
		fb.vrf, fb.vrfOwner = vrf, name
	} else if fb.vrf != vrf {
		// One kea-dhcp<N> process lives in one network namespace; per-VRF instances are a
		// later extension (vdom.md #5 keeps the document ready for it).
		return invalid(path+"/vrf", "VRF %q differs from %q of server %q: one Kea instance per family serves one VRF", vrf, fb.vrf, fb.vrfOwner)
	}
	desc, err := checkDescription(path+"/description", s.GetDescription())
	if err != nil {
		return err
	}
	if len(s.GetInterfaces()) == 0 {
		return invalid(path+"/interfaces", "a server needs at least one interface")
	}
	linux := make([]string, 0, len(s.GetInterfaces()))
	for i, vppName := range s.GetInterfaces() {
		ln, err := r.mapIf(vppName)
		if err != nil {
			return fmt.Errorf("%w: %s/interfaces/%d: %w", ErrInvalid, path, i, err)
		}
		if err := r.checkInterface(fmt.Sprintf("%s/interfaces/%d", path, i), ln); err != nil {
			return err
		}
		linux = append(linux, ln)
	}
	lease := s.GetLeaseTimeSec()
	if s.LeaseTimeSec == nil {
		lease = 3600
	}
	if lease < 60 {
		return invalid(path+"/leaseTimeSec", "lease time %d below 60 s", lease)
	}
	if s.RenewTimerSec != nil && s.GetRenewTimerSec() >= lease {
		return invalid(path+"/renewTimerSec", "renew timer must be shorter than the lease time")
	}
	if s.RebindTimerSec != nil && s.GetRebindTimerSec() >= lease {
		return invalid(path+"/rebindTimerSec", "rebind timer must be shorter than the lease time")
	}
	globalOpts, err := fb.options(path+"/options", s.GetOptions())
	if err != nil {
		return err
	}

	subnets := s.GetSubnets()
	for _, sn := range sortedKeys(subnets) {
		sub, err := r.buildSubnet(fb, name, sn, subnets[sn], linux)
		if err != nil {
			return err
		}
		if subnets[sn].LeaseTimeSec == nil {
			sub.ValidLifetime = lease
		}
		sub.RenewTimer, sub.RebindTimer = s.GetRenewTimerSec(), s.GetRebindTimerSec()
		if sub.RenewTimer >= sub.ValidLifetime || sub.RebindTimer >= sub.ValidLifetime {
			sub.RenewTimer, sub.RebindTimer = 0, 0 // subnet lease shorter than the server timers: let Kea derive them
		}
		if fb.family == 6 {
			sub.PreferredLifetime = sub.ValidLifetime
		} else {
			auth := s.Authoritative == nil || s.GetAuthoritative()
			sub.Authoritative = &auth
		}
		sub.OptionData = mergeOptions(sub.OptionData, globalOpts)
		sub.UserContext.VRX.ServerDescription = desc
		fb.subnets = append(fb.subnets, sub)
	}
	fb.interfaces = append(fb.interfaces, linux...)
	return nil
}

// mergeOptions appends server-wide options that the subnet does not set itself.
func mergeOptions(subnetOpts, globalOpts []optionData) []optionData {
	have := map[uint32]bool{}
	for _, o := range subnetOpts {
		have[o.Code] = true
	}
	out := subnetOpts
	for _, o := range globalOpts {
		if !have[o.Code] {
			out = append(out, o)
		}
	}
	sortOptions(out)
	return out
}

func sortOptions(opts []optionData) {
	sort.SliceStable(opts, func(i, j int) bool { return opts[i].Code < opts[j].Code })
}

// subnetID derives a stable Kea subnet id from the document names (leases reference it, so it
// must not change when other subnets are added): FNV-32a of "<server>/<subnet>" in
// 1..2^32-2, linear probing on the (rare) collision.
func (fb *famBuild) subnetID(server, subnetName string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(server + "/" + subnetName))
	id := h.Sum32()%4294967294 + 1
	for fb.usedIDs[id] {
		id = id%4294967294 + 1
	}
	fb.usedIDs[id] = true
	return id
}

func (r *Renderer) buildSubnet(fb *famBuild, server, name string, s *vrxv1.DhcpSubnet, linux []string) (subnet, error) {
	path := "servers/" + server + "/subnets/" + name
	if err := checkName(path, name); err != nil {
		return subnet{}, err
	}
	pfx, err := netip.ParsePrefix(s.GetSubnet())
	if err != nil {
		return subnet{}, invalid(path+"/subnet", "%q is not a CIDR prefix", s.GetSubnet())
	}
	pfx = pfx.Masked()
	if (fb.family == 4) != pfx.Addr().Is4() {
		return subnet{}, invalid(path+"/subnet", "%s is not an IPv%d prefix", pfx, fb.family)
	}
	desc, err := checkDescription(path+"/description", s.GetDescription())
	if err != nil {
		return subnet{}, err
	}
	sub := subnet{
		ID:          fb.subnetID(server, name),
		Subnet:      pfx.String(),
		UserContext: &userContext{VRX: vrxContext{Server: server, Subnet: name, Description: desc}},
	}
	if s.LeaseTimeSec != nil {
		if s.GetLeaseTimeSec() < 60 {
			return subnet{}, invalid(path+"/leaseTimeSec", "lease time %d below 60 s", s.GetLeaseTimeSec())
		}
		sub.ValidLifetime = s.GetLeaseTimeSec()
	}
	if len(s.GetPools()) == 0 {
		return subnet{}, invalid(path+"/pools", "a subnet needs at least one pool")
	}
	for i, p := range s.GetPools() {
		pp := fmt.Sprintf("%s/pools/%d", path, i)
		a, err := checkAddr(pp+"/start", p.GetStart(), fb.family)
		if err != nil {
			return subnet{}, err
		}
		b, err := checkAddr(pp+"/end", p.GetEnd(), fb.family)
		if err != nil {
			return subnet{}, err
		}
		if !pfx.Contains(a) || !pfx.Contains(b) || b.Less(a) {
			return subnet{}, invalid(pp, "pool %s-%s is not an ordered range inside %s", a, b, pfx)
		}
		sub.Pools = append(sub.Pools, pool{Pool: a.String() + "-" + b.String()})
	}
	// The subnet is bound to the server's interface when there is exactly one, and Kea's
	// interfaces-config gets "<if>/<addr>" when the desired interface has an address in it.
	if len(linux) == 1 {
		sub.Interface = linux[0]
	}

	opts, err := fb.typedOptions(path, s)
	if err != nil {
		return subnet{}, err
	}
	custom, err := fb.options(path+"/options", s.GetOptions())
	if err != nil {
		return subnet{}, err
	}
	for _, c := range custom {
		for _, o := range opts {
			if o.Code == c.Code {
				return subnet{}, invalid(path+"/options", "option %d is also set by a typed field", c.Code)
			}
		}
	}
	sub.OptionData = append(opts, custom...)
	sortOptions(sub.OptionData)

	reservations := s.GetReservations()
	for _, rn := range sortedKeys(reservations) {
		res, err := fb.reservation(path+"/reservations/"+rn, rn, reservations[rn], pfx)
		if err != nil {
			return subnet{}, err
		}
		sub.Reservations = append(sub.Reservations, res)
	}
	return sub, nil
}

func (fb *famBuild) typedOptions(path string, s *vrxv1.DhcpSubnet) ([]optionData, error) {
	var out []optionData
	addrList := func(field string, list []string) (string, error) {
		parts := make([]string, 0, len(list))
		for i, v := range list {
			a, err := checkAddr(fmt.Sprintf("%s/%s/%d", path, field, i), v, fb.family)
			if err != nil {
				return "", err
			}
			parts = append(parts, a.String())
		}
		return strings.Join(parts, ", "), nil
	}
	hostList := func(field string, list []string) (string, error) {
		parts := make([]string, 0, len(list))
		for i, v := range list {
			h, err := checkHostname(fmt.Sprintf("%s/%s/%d", path, field, i), v)
			if err != nil {
				return "", err
			}
			parts = append(parts, h)
		}
		return strings.Join(parts, ", "), nil
	}
	add := func(o typedOption, data string) {
		if data != "" {
			out = append(out, optionData{Name: o.name, Code: o.code, Space: fb.space, Data: data})
		}
	}
	dns, err := addrList("dnsServers", s.GetDnsServers())
	if err != nil {
		return nil, err
	}
	ntp, err := addrList("ntpServers", s.GetNtpServers())
	if err != nil {
		return nil, err
	}
	search := s.GetDomainSearch()
	domain := ""
	if s.DomainName != nil {
		if domain, err = checkHostname(path+"/domainName", s.GetDomainName()); err != nil {
			return nil, err
		}
	}
	if fb.family == 4 {
		if s.Gateway != nil {
			gw, err := checkAddr(path+"/gateway", s.GetGateway(), 4)
			if err != nil {
				return nil, err
			}
			add(optRouters4, gw.String())
		}
		add(optDNS4, dns)
		add(optDomainName4, domain)
		add(optNTP4, ntp)
		sl, err := hostList("domainSearch", search)
		if err != nil {
			return nil, err
		}
		add(optSearch4, sl)
		return out, nil
	}
	if s.Gateway != nil {
		return nil, invalid(path+"/gateway", "a gateway applies to DHCPv4 only (IPv6 uses router advertisements)")
	}
	add(optDNS6, dns)
	add(optSNTP6, ntp)
	// DHCPv6 has no domain-name option: the domain name leads the search list.
	if domain != "" && !slices.Contains(search, domain) {
		search = append([]string{domain}, search...)
	}
	sl, err := hostList("domainSearch", search)
	if err != nil {
		return nil, err
	}
	add(optSearch6, sl)
	return out, nil
}

// options renders free option data. Standard codes use Kea's definition (csv-format text);
// other codes carry either 0x… hex (csv-format false, no definition needed) or text, for which
// a string option-def "vrx-<code>" is added.
func (fb *famBuild) options(path string, in []*vrxv1.DhcpOption) ([]optionData, error) {
	std, maxCode := std4, uint32(254)
	if fb.family == 6 {
		std, maxCode = std6, 65535
	}
	seen := map[uint32]bool{}
	out := make([]optionData, 0, len(in))
	for i, o := range in {
		op := fmt.Sprintf("%s/%d", path, i)
		code := o.GetCode()
		if code < 1 || code > maxCode {
			return nil, invalid(op+"/code", "option code %d outside 1..%d", code, maxCode)
		}
		if seen[code] {
			return nil, invalid(op+"/code", "option code %d set twice", code)
		}
		seen[code] = true
		data := o.GetData()
		if err := checkOptionData(op+"/data", data, fb.family); err != nil {
			return nil, err
		}
		od := optionData{Code: code, Space: fb.space, Data: data, AlwaysSend: o.GetAlwaysSend()}
		switch {
		case std[code]:
		case hexDataRe.MatchString(data):
			f := false
			od.CSVFormat = &f
		default:
			name := fmt.Sprintf("vrx-%d", code)
			fb.optionDefs[code] = optionDef{Name: name, Code: code, Type: "string", Space: fb.space}
			od.Name = name
		}
		out = append(out, od)
	}
	return out, nil
}

func (fb *famBuild) reservation(path, name string, rs *vrxv1.DhcpReservation, pfx netip.Prefix) (reservation, error) {
	if err := checkName(path, name); err != nil {
		return reservation{}, err
	}
	res := reservation{UserContext: &userContext{VRX: vrxContext{Reservation: name}}}
	switch {
	case rs.Mac != nil && rs.Duid != nil, rs.Mac == nil && rs.Duid == nil:
		return reservation{}, invalid(path, "a reservation identifies the client by exactly one of mac or duid")
	case rs.Mac != nil:
		mac, err := checkMAC(path+"/mac", rs.GetMac())
		if err != nil {
			return reservation{}, err
		}
		res.HWAddress = mac
	default:
		if fb.family == 4 {
			return reservation{}, invalid(path+"/duid", "DUID reservations apply to DHCPv6 only")
		}
		duid, err := checkDUID(path+"/duid", rs.GetDuid())
		if err != nil {
			return reservation{}, err
		}
		res.DUID = duid
	}
	ip, err := checkAddr(path+"/ip", rs.GetIp(), fb.family)
	if err != nil {
		return reservation{}, err
	}
	if !pfx.Contains(ip) {
		return reservation{}, invalid(path+"/ip", "%s is outside %s", ip, pfx)
	}
	if fb.family == 4 {
		res.IPAddress = ip.String()
	} else {
		res.IPAddresses = []string{ip.String()}
	}
	if rs.Hostname != nil {
		if res.Hostname, err = checkHostname(path+"/hostname", rs.GetHostname()); err != nil {
			return reservation{}, err
		}
	}
	if res.OptionData, err = fb.options(path+"/options", rs.GetOptions()); err != nil {
		return reservation{}, err
	}
	sortOptions(res.OptionData)
	return res, nil
}

// interfaceBindings turns plain interface names into "<if>/<addr>" where the desired
// interface (DesiredState given) has an IPv4 address inside one of the family's subnets.
func (r *Renderer) interfaceBindings(in input, cfg *serverConfig, family int) {
	if family != 4 || in.interfaces == nil || cfg.Subnet4 == nil {
		return
	}
	// linux name → VPP name
	byLinux := map[string]string{}
	for vpp := range in.interfaces {
		if ln, err := r.mapIf(vpp); err == nil {
			byLinux[ln] = vpp
		}
	}
	for i, ifn := range cfg.InterfacesConfig.Interfaces {
		vpp, ok := byLinux[ifn]
		if !ok {
			continue
		}
		for _, cidr := range in.interfaces[vpp].GetIpv4() {
			p, err := netip.ParsePrefix(cidr)
			if err != nil {
				continue
			}
			for _, sub := range *cfg.Subnet4 {
				sp, _ := netip.ParsePrefix(sub.Subnet)
				if sp.Contains(p.Addr()) {
					cfg.InterfacesConfig.Interfaces[i] = ifn + "/" + p.Addr().String()
					break
				}
			}
			if strings.Contains(cfg.InterfacesConfig.Interfaces[i], "/") {
				break
			}
		}
	}
}

func (r *Renderer) buildCtrlAgent() ctrlAgentRoot {
	return ctrlAgentRoot{ControlAgent: ctrlAgent{
		HTTPHost: r.paths.CtrlAgentHost,
		HTTPPort: r.paths.CtrlAgentPort,
		ControlSockets: map[string]controlSocket{
			"dhcp4": {SocketType: "unix", SocketName: r.paths.Socket4()},
			"dhcp6": {SocketType: "unix", SocketName: r.paths.Socket6()},
		},
		Loggers: []logger{{
			Name:          "kea-ctrl-agent",
			OutputOptions: []outputOption{{Output: r.paths.LogCtrlAgent(), MaxSize: 10 << 20, MaxVer: 4}},
			Severity:      "INFO",
		}},
	}}
}

// marshal encodes v as indented JSON without HTML escaping and with a trailing newline.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("kea: marshal: %w", err)
	}
	return buf.Bytes(), nil
}

func sortedKeys[K interface{ ~string | ~uint32 }, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
