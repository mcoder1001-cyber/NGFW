package keepalived

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/rfkit"
)

// ErrInput is wrapped by every error about the desired state.
var ErrInput = errors.New("keepalived: invalid desired state")

// InterfaceMapper maps a VPP interface name (the document's) to the Linux interface
// keepalived binds to (linux-cp host side). ok=false: the interface has no Linux side.
type InterfaceMapper func(vppName string) (linux string, ok bool)

// NoMapper is the product default until F-vrrp passes the linux-cp mapping: no interface
// has a Linux side, so every keepalived instance is rejected with a clear error instead of
// binding keepalived to a Linux interface that merely shares the VPP name (RF-1 review L4).
func NoMapper(string) (string, bool) { return "", false }

// PrefixMapper maps names that start with prefix to themselves (tests: rig veths "w8-a").
func PrefixMapper(prefix string) InterfaceMapper {
	return func(name string) (string, bool) {
		if prefix == "" || !strings.HasPrefix(name, prefix) {
			return "", false
		}
		return name, true
	}
}

// Model is the validated, resolved view of the keepalived part of ha.vrrp: exactly what the
// template renders.
type Model struct {
	RouterID          string
	GarpMasterRefresh uint32
	Scripts           []Script
	SyncGroups        []SyncGroup
	Instances         []Instance
	Notify            string // helper path
	StateDir          string
}

// Script is one vrrp_script (a shipped check executable only).
type Script struct {
	Name                        string
	Path                        string
	Interval, Fall, Rise        uint32
	Weight                      int
	HasWeight, HasFall, HasRise bool
}

// SyncGroup is one vrrp_sync_group.
type SyncGroup struct {
	Name      string
	Instances []string
}

// Instance is one vrrp_instance.
type Instance struct {
	Name         string
	State        string // MASTER (priority 255) | BACKUP
	Interface    string // Linux name
	VRID         uint32
	Priority     uint32
	AdvertInt    string // seconds, "1" / "0.5" / "1.23"
	IPv6         bool
	Version2     bool   // VRRPv2: only with PASS authentication
	AuthPass     string // resolved psk (≤ 8 characters)
	Preempt      bool
	PreemptDelay uint32
	Accept       bool
	UnicastSrc   string
	Peers        []string
	VIPs         []string // "<addr>/<len>"
	Routes       []Route
	Track        []Track
	TrackScripts []string
}

// Route is one virtual_routes entry.
type Route struct {
	Prefix, Via, Dev string
}

// Track is one track_interface entry.
type Track struct {
	Interface string
	Weight    int // negative: subtracted while down
}

var (
	nameRe     = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)
	ifnameRe   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)
	checkRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	authPassRe = regexp.MustCompile(`^[A-Za-z0-9_.,:;@%+=/~^*-]{1,8}$`)
)

// Name validates an instance/script/group name: [A-Za-z0-9_.-]{1,32}.
func Name(s string) (string, error) {
	if !nameRe.MatchString(s) {
		return "", fmt.Errorf("%w: name %q must match %s", ErrInput, s, nameRe)
	}
	return s, nil
}

// Options are the renderer-side inputs of BuildModel.
type Options struct {
	Paths  Paths
	Mapper InterfaceMapper
	// Checks is the set of shipped check executables a vrrp_script may name.
	Checks map[string]bool
}

// BuildModel validates the keepalived instances of ha.vrrp (engine "keepalived", enabled)
// plus the stand-ins (D-055) and resolves their secrets.
func BuildModel(ds *vrxv1.DesiredState, ext *rfkit.Ext, sec *rfkit.Secrets, o Options) (*Model, error) {
	if o.Mapper == nil {
		o.Mapper = NoMapper
	}
	m := &Model{Notify: o.Paths.NotifyHelper, StateDir: o.Paths.StateDir, RouterID: "vrx"}
	if h := ds.GetSystem().GetHostname(); nameRe.MatchString(h) {
		m.RouterID = h
	}
	gx := ext.Get("ha", "keepalived")
	if err := gx.OnlyKeys("routerId", "garpMasterRefresh", "scripts", "syncGroups"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	if id, ok, err := gx.Get("routerId").String(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		m.RouterID = id
	}
	if _, err := Name(m.RouterID); err != nil {
		return nil, fmt.Errorf("ha.keepalived.routerId: %w", err)
	}
	var err error
	if m.GarpMasterRefresh, _, err = gx.Get("garpMasterRefresh").Uint(1, 86400); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	scripts, err := buildScripts(gx.Get("scripts"), o)
	if err != nil {
		return nil, err
	}
	m.Scripts = scripts
	vrrp := ds.GetHa().GetVrrp()
	for _, name := range slices.Sorted(maps.Keys(vrrp)) {
		v := vrrp[name]
		if v.GetEngine() != "keepalived" || (v.Enabled != nil && !v.GetEnabled()) {
			continue
		}
		inst, err := buildInstance(name, v, ext.Get("ha", "vrrp", name, "keepalived"), sec, o, scripts)
		if err != nil {
			return nil, err
		}
		m.Instances = append(m.Instances, *inst)
	}
	if err := checkUnique(m.Instances); err != nil {
		return nil, err
	}
	groups, err := buildGroups(gx.Get("syncGroups"), m.Instances)
	if err != nil {
		return nil, err
	}
	m.SyncGroups = groups
	return m, nil
}

func buildScripts(sx *rfkit.Ext, o Options) ([]Script, error) {
	names, err := sx.Keys()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	slices.Sort(names)
	var out []Script
	for _, name := range names {
		path := "ha.keepalived.scripts." + name
		if _, err := Name(name); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		x := sx.Get(name)
		if err := x.OnlyKeys("check", "interval", "weight", "fall", "rise"); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		check, _, err := x.Get("check").String()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		// Never user script text or paths: a name from the shipped allow-list only.
		if !checkRe.MatchString(check) || !o.Checks[check] {
			return nil, fmt.Errorf("%w: %s.check %q is not a shipped check (allowed: %s)", ErrInput, path, check, strings.Join(slices.Sorted(maps.Keys(o.Checks)), ", "))
		}
		s := Script{Name: name, Path: o.Paths.ChecksDir + "/" + check, Interval: 1}
		if n, ok, err := x.Get("interval").Uint(1, 3600); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		} else if ok {
			s.Interval = n
		}
		if s.Fall, s.HasFall, err = x.Get("fall").Uint(1, 255); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		if s.Rise, s.HasRise, err = x.Get("rise").Uint(1, 255); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		if s.Weight, s.HasWeight, err = x.Get("weight").Int(-253, 253); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		out = append(out, s)
	}
	return out, nil
}

func buildInstance(name string, v *vrxv1.VrrpInstance, kx *rfkit.Ext, sec *rfkit.Secrets, o Options, scripts []Script) (*Instance, error) {
	path := "ha.vrrp." + name
	if _, err := Name(name); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := kx.OnlyKeys("authRef", "preemptDelay", "unicastSrcIp", "prefixLength", "virtualRoutes", "trackScripts"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	if vrf := v.GetVrf(); vrf != "" && vrf != "default" {
		return nil, fmt.Errorf("%w: %s.vrf %q: keepalived instances run in the default VRF only (F-vrrp)", ErrInput, path, vrf)
	}
	in := &Instance{Name: name, State: "BACKUP", Preempt: true, Accept: v.GetAcceptMode()}
	var err error
	if in.Interface, err = mapIf(v.GetInterface(), o.Mapper, path+".interface"); err != nil {
		return nil, err
	}
	in.VRID = v.GetVrId()
	if in.VRID < 1 || in.VRID > 255 {
		return nil, fmt.Errorf("%w: %s.vrId %d must be 1..255", ErrInput, path, in.VRID)
	}
	in.Priority = 100
	if v.Priority != nil {
		in.Priority = v.GetPriority()
	}
	if in.Priority < 1 || in.Priority > 255 {
		return nil, fmt.Errorf("%w: %s.priority %d must be 1..255", ErrInput, path, in.Priority)
	}
	if in.Priority == 255 {
		in.State = "MASTER"
	}
	adv := uint32(1000)
	if v.AdvertisementIntervalMs != nil {
		adv = v.GetAdvertisementIntervalMs()
	}
	if adv < 10 || adv > 40950 || adv%10 != 0 {
		return nil, fmt.Errorf("%w: %s.advertisementIntervalMs %d must be 10..40950 in steps of 10", ErrInput, path, adv)
	}
	in.AdvertInt = strings.TrimRight(strings.TrimRight(strconv.FormatFloat(float64(adv)/1000, 'f', 2, 64), "0"), ".")
	switch v.GetAddressFamily() {
	case "", "ipv4":
	case "ipv6":
		in.IPv6 = true
	default:
		return nil, fmt.Errorf("%w: %s.addressFamily %q", ErrInput, path, v.GetAddressFamily())
	}
	if v.Preempt != nil {
		in.Preempt = v.GetPreempt()
	}
	if d, ok, err := kx.Get("preemptDelay").Uint(0, 1000); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		if !in.Preempt {
			return nil, fmt.Errorf("%w: %s.keepalived.preemptDelay needs preempt", ErrInput, path)
		}
		in.PreemptDelay = d
	}
	if !in.Preempt && in.State == "MASTER" {
		return nil, fmt.Errorf("%w: %s: an address owner (priority 255) cannot be nopreempt", ErrInput, path)
	}
	family := func(a netip.Addr) bool { return a.Is6() && !a.Is4In6() }
	plen := 32
	if in.IPv6 {
		plen = 128
	}
	if n, ok, err := kx.Get("prefixLength").Uint(1, 128); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		if (!in.IPv6 && n > 32) || n < 1 {
			return nil, fmt.Errorf("%w: %s.keepalived.prefixLength %d out of range for the family", ErrInput, path, n)
		}
		plen = int(n)
	}
	if len(v.GetAddresses()) == 0 || len(v.GetAddresses()) > 32 {
		return nil, fmt.Errorf("%w: %s.addresses must hold 1..32 addresses", ErrInput, path)
	}
	for i, a := range v.GetAddresses() {
		ip, err := netip.ParseAddr(a)
		if err != nil || ip.Zone() != "" || family(ip) != in.IPv6 || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLoopback() {
			return nil, fmt.Errorf("%w: %s.addresses[%d] %q is not a usable %s address", ErrInput, path, i, a, fam(in.IPv6))
		}
		vip := netip.PrefixFrom(ip.Unmap(), plen).String()
		if slices.Contains(in.VIPs, vip) {
			return nil, fmt.Errorf("%w: %s.addresses[%d] duplicates %s", ErrInput, path, i, vip)
		}
		in.VIPs = append(in.VIPs, vip)
	}
	if u := v.GetUnicast(); u != nil {
		for i, p := range u.GetPeers() {
			ip, err := netip.ParseAddr(p)
			if err != nil || ip.Zone() != "" || family(ip) != in.IPv6 || ip.IsUnspecified() || ip.IsMulticast() {
				return nil, fmt.Errorf("%w: %s.unicast.peers[%d] %q is not a usable %s address", ErrInput, path, i, p, fam(in.IPv6))
			}
			in.Peers = append(in.Peers, ip.Unmap().String())
		}
		if len(in.Peers) == 0 {
			return nil, fmt.Errorf("%w: %s.unicast.peers must not be empty", ErrInput, path)
		}
	}
	if s, ok, err := kx.Get("unicastSrcIp").String(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		ip, err := netip.ParseAddr(s)
		if err != nil || ip.Zone() != "" || family(ip) != in.IPv6 || len(in.Peers) == 0 {
			return nil, fmt.Errorf("%w: %s.keepalived.unicastSrcIp %q must be a %s address of a unicast instance", ErrInput, path, s, fam(in.IPv6))
		}
		in.UnicastSrc = ip.Unmap().String()
	}
	for i, t := range v.GetTrack() {
		ifn, err := mapIf(t.GetInterface(), o.Mapper, fmt.Sprintf("%s.track[%d].interface", path, i))
		if err != nil {
			return nil, err
		}
		if ifn == in.Interface {
			return nil, fmt.Errorf("%w: %s.track[%d]: a virtual router cannot track its own interface", ErrInput, path, i)
		}
		dec := uint32(10)
		if t.PriorityDecrement != nil {
			dec = t.GetPriorityDecrement()
		}
		if dec < 1 || dec > 253 {
			return nil, fmt.Errorf("%w: %s.track[%d].priorityDecrement %d must be 1..253", ErrInput, path, i, dec)
		}
		in.Track = append(in.Track, Track{Interface: ifn, Weight: -int(dec)})
	}
	if err := buildRoutes(in, kx.Get("virtualRoutes"), o, path); err != nil {
		return nil, err
	}
	ts, err := kx.Get("trackScripts").Strings()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	for _, s := range ts {
		if !slices.ContainsFunc(scripts, func(sc Script) bool { return sc.Name == s }) {
			return nil, fmt.Errorf("%w: %s.keepalived.trackScripts: %q is not defined in ha.keepalived.scripts", ErrInput, path, s)
		}
		in.TrackScripts = append(in.TrackScripts, s)
	}
	if ref, ok, err := kx.Get("authRef").String(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	} else if ok {
		// VRRPv3 (RFC 5798) has no authentication; keepalived ignores it there ("does not
		// support authentication"), so an authenticated instance is VRRPv2: IPv4 and whole
		// seconds only, PASS keys of at most 8 characters.
		if in.IPv6 || adv%1000 != 0 {
			return nil, fmt.Errorf("%w: %s.keepalived.authRef needs VRRPv2: IPv4 and an advertisement interval in whole seconds", ErrInput, path)
		}
		pass, err := sec.Resolve(ref, func(v string) error {
			if !authPassRe.MatchString(v) {
				return fmt.Errorf("a VRRPv2 PASS key must be 1–8 characters of [A-Za-z0-9_.,:;@%%+=/~^*-]")
			}
			return nil
		}, "psk")
		if err != nil {
			return nil, fmt.Errorf("%w: %s.keepalived.authRef: %w", ErrInput, path, err)
		}
		in.Version2, in.AuthPass = true, pass
	}
	return in, nil
}

func fam(v6 bool) string {
	if v6 {
		return "IPv6"
	}
	return "IPv4"
}

func mapIf(vpp string, mapper InterfaceMapper, path string) (string, error) {
	if vpp == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInput, path)
	}
	linux, ok := mapper(vpp)
	if !ok {
		return "", fmt.Errorf("%w: %s %q has no Linux interface keepalived could use (linux-cp mapping: F-vrrp)", ErrInput, path, vpp)
	}
	if !ifnameRe.MatchString(linux) {
		return "", fmt.Errorf("%w: %s maps to %q, not a Linux interface name", ErrInput, path, linux)
	}
	return linux, nil
}

func buildRoutes(in *Instance, rx *rfkit.Ext, o Options, path string) error {
	n, err := rx.Len()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInput, err)
	}
	if n > 32 {
		return fmt.Errorf("%w: %s.keepalived.virtualRoutes has more than 32 entries", ErrInput, path)
	}
	for i := range n {
		x := rx.Index(i)
		if err := x.OnlyKeys("prefix", "via", "interface"); err != nil {
			return fmt.Errorf("%w: %w", ErrInput, err)
		}
		ps, _, err := x.Get("prefix").String()
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInput, err)
		}
		p, err := netip.ParsePrefix(ps)
		if err != nil || p.Addr().Zone() != "" || (p.Addr().Is6() && !p.Addr().Is4In6()) != in.IPv6 {
			return fmt.Errorf("%w: %s.prefix %q is not a %s prefix", ErrInput, x.Path(), ps, fam(in.IPv6))
		}
		r := Route{Prefix: p.Masked().String(), Dev: in.Interface}
		if via, ok, err := x.Get("via").String(); err != nil {
			return fmt.Errorf("%w: %w", ErrInput, err)
		} else if ok {
			ip, err := netip.ParseAddr(via)
			if err != nil || ip.Zone() != "" || (ip.Is6() && !ip.Is4In6()) != in.IPv6 || ip.IsUnspecified() || ip.IsMulticast() {
				return fmt.Errorf("%w: %s.via %q is not a usable gateway", ErrInput, x.Path(), via)
			}
			r.Via = ip.Unmap().String()
		}
		if ifn, ok, err := x.Get("interface").String(); err != nil {
			return fmt.Errorf("%w: %w", ErrInput, err)
		} else if ok {
			if r.Dev, err = mapIf(ifn, o.Mapper, x.Path()+".interface"); err != nil {
				return err
			}
		}
		in.Routes = append(in.Routes, r)
	}
	return nil
}

func checkUnique(insts []Instance) error {
	seen := map[string]string{}
	for _, in := range insts {
		k := fmt.Sprintf("%s/%v/%d", in.Interface, in.IPv6, in.VRID)
		if other, dup := seen[k]; dup {
			return fmt.Errorf("%w: ha.vrrp.%s and ha.vrrp.%s share interface %s, family and VRID %d", ErrInput, other, in.Name, in.Interface, in.VRID)
		}
		seen[k] = in.Name
	}
	return nil
}

func buildGroups(gx *rfkit.Ext, insts []Instance) ([]SyncGroup, error) {
	names, err := gx.Keys()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInput, err)
	}
	slices.Sort(names)
	member := map[string]string{}
	var out []SyncGroup
	for _, name := range names {
		path := "ha.keepalived.syncGroups." + name
		if _, err := Name(name); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		list, err := gx.Get(name).Strings()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInput, err)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%w: %s must name at least one instance", ErrInput, path)
		}
		for _, in := range list {
			if !slices.ContainsFunc(insts, func(i Instance) bool { return i.Name == in }) {
				return nil, fmt.Errorf("%w: %s: %q is not an enabled keepalived instance", ErrInput, path, in)
			}
			if g, dup := member[in]; dup {
				return nil, fmt.Errorf("%w: %s: %q is already in sync group %s", ErrInput, path, in, g)
			}
			member[in] = name
		}
		out = append(out, SyncGroup{Name: name, Instances: list})
	}
	return out, nil
}
