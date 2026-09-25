// Package igmp holds the reconciler descriptors for the VPP IGMP plugin (task DF-7, WBS D2.9):
// IGMP on an interface in host or router mode (igmp_enable_disable), static host joins
// (igmp_listen, retrieved with igmp_dump), SSM group ranges (igmp_group_prefix_set), IGMP
// proxy devices (igmp_proxy_device_add_del) and their downstream interfaces
// (igmp_proxy_device_add_del_interface). WatchEvents turns want_igmp_events + igmp_event into
// membership changes; ClearInterface is the igmp_clear_interface action helper.
//
// Only igmp.listen has a usable dump. The others are write-only (D-063): VPP 26.06 reports no
// interface mode, no proxy device and no downstream list, and igmp_group_prefix_dump answers
// with the igmp_details message id (igmp_api.c igmp_ssm_range_walk_dump), which govpp cannot
// decode as igmp_group_prefix_details.
//
// Messages come only from apps/agent/binapi/igmp. Ownership: everything belongs to the owner of
// its interface; group prefixes are VPP-wide and attributed by the desired state only.
// docs/agent/descriptors/igmp.md is the object ↔ message table.
package igmp

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"sync"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/igmp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameInterface   = "igmp.interface"
	NameListen      = "igmp.listen"
	NameGroupPrefix = "igmp.group-prefix"
	NameProxyDevice = "igmp.proxy-device"
	NameDownstream  = "igmp.proxy-downstream"
)

// Interface modes.
const (
	ModeHost   = "host"
	ModeRouter = "router"
)

// ---- specs ------------------------------------------------------------------------------------

// Interface is the desired state of one igmp.interface object.
type Interface struct {
	Interface string `json:"interface,omitempty"`
	Mode      string `json:"mode,omitempty"`
}

// Validate checks i.
func (i Interface) Validate() error {
	if i.Interface == "" {
		return df7.Specf("igmp interface needs an interface")
	}
	if i.Mode != ModeHost && i.Mode != ModeRouter {
		return df7.Specf("igmp mode %q: want host or router", i.Mode)
	}
	return nil
}

// Listen is the desired state of one igmp.listen object: a static (S,G) join in INCLUDE mode on
// a host-mode interface. Sources are canonical and sorted; at least one is required (an empty
// INCLUDE list is a leave; VPP 26.06 does not implement EXCLUDE-mode listens).
type Listen struct {
	Interface string   `json:"interface,omitempty"`
	Group     string   `json:"group,omitempty"`
	Sources   []string `json:"sources,omitempty"`
}

// Validate checks l.
func (l Listen) Validate() error {
	if l.Interface == "" {
		return df7.Specf("igmp listen needs an interface")
	}
	g, err := df7.ParseAddr(l.Group)
	if err != nil || !g.Is4() || !g.IsMulticast() || g.String() != l.Group {
		return df7.Specf("igmp group %q is not a canonical IPv4 multicast address", l.Group)
	}
	if len(l.Sources) == 0 || len(l.Sources) > 255 {
		return df7.Specf("igmp listen needs 1–255 sources (INCLUDE mode)")
	}
	sorted, err := df7.SortedAddrs(l.Sources)
	if err != nil {
		return err
	}
	for i, s := range sorted {
		a, _ := df7.ParseAddr(s)
		if !a.Is4() || s != l.Sources[i] {
			return df7.Specf("igmp sources must be canonical, sorted IPv4 addresses (%v)", sorted)
		}
	}
	return nil
}

// GroupPrefix is the desired state of one igmp.group-prefix object: an SSM group range.
type GroupPrefix struct {
	Prefix string `json:"prefix,omitempty"`
}

// Validate checks g.
func (g GroupPrefix) Validate() error {
	p, err := df7.ParsePrefix(g.Prefix)
	if err != nil {
		return err
	}
	if !p.Addr().Is4() || !p.Addr().IsMulticast() {
		return df7.Specf("igmp group prefix %s is not an IPv4 multicast range", p)
	}
	return nil
}

// ProxyDevice is the desired state of one igmp.proxy-device object: the proxy of VRF with its
// upstream (host-mode) interface.
type ProxyDevice struct {
	VRF      uint32 `json:"vrf,omitempty"`
	Upstream string `json:"upstream,omitempty"`
}

// Validate checks p.
func (p ProxyDevice) Validate() error {
	if p.Upstream == "" {
		return df7.Specf("igmp proxy device needs an upstream interface")
	}
	return nil
}

// Downstream is the desired state of one igmp.proxy-downstream object: a router-mode interface
// whose memberships the VRF's proxy device reports upstream.
type Downstream struct {
	VRF       uint32 `json:"vrf,omitempty"`
	Interface string `json:"interface,omitempty"`
}

// Validate checks d.
func (d Downstream) Validate() error {
	if d.Interface == "" {
		return df7.Specf("igmp proxy downstream needs an interface")
	}
	return nil
}

// ---- keys -------------------------------------------------------------------------------------

func u32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// KeyInterface is "igmp.interface/<interface>".
func KeyInterface(ifName string) scheduler.Key { return scheduler.Join(NameInterface, ifName) }

// KeyListen is "igmp.listen/<interface>/<group>".
func KeyListen(ifName, group string) scheduler.Key { return scheduler.Join(NameListen, ifName, group) }

// KeyGroupPrefix is "igmp.group-prefix/<prefix>".
func KeyGroupPrefix(prefix string) scheduler.Key { return scheduler.Join(NameGroupPrefix, prefix) }

// KeyProxyDevice is "igmp.proxy-device/<vrf>".
func KeyProxyDevice(vrf uint32) scheduler.Key { return scheduler.Join(NameProxyDevice, u32(vrf)) }

// KeyDownstream is "igmp.proxy-downstream/<vrf>/<interface>".
func KeyDownstream(vrf uint32, ifName string) scheduler.Key {
	return scheduler.Join(NameDownstream, u32(vrf), ifName)
}

// ---- shared -----------------------------------------------------------------------------------

// Meta is the interface index of an object.
type Meta struct{ SwIfIndex uint32 }

// Modes remembers the mode igmp.interface applied per interface in this process, so that
// igmp.listen's Retrieve can skip router-mode interfaces (igmp_dump also lists the groups a
// router learned from reports, which are not static joins). It is best effort: after an agent
// restart it fills again when the write-only igmp.interface objects are re-applied.
type Modes struct {
	mu sync.Mutex
	m  map[string]string
}

// NewModes returns an empty mode registry.
func NewModes() *Modes { return &Modes{m: map[string]string{}} }

func (r *Modes) set(ifName, mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mode == "" {
		delete(r.m, ifName)
		return
	}
	r.m[ifName] = mode
}

func (r *Modes) get(ifName string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[ifName]
}

// detach re-resolves an object's interface right before a delete (D-071), runs del when it
// still exists and releases the claim of holder.
func detach(ctx context.Context, b df7.Base, ifName, holder string, del func(idx uint32) error) error {
	tg, found, err := b.Detach(ctx, ifName, holder)
	if err != nil || !found {
		return err
	}
	if err := del(tg.Index); err != nil {
		return err
	}
	return tg.Release()
}

// ---- igmp.interface ---------------------------------------------------------------------------

// InterfaceDescriptor manages igmp.interface objects. Write-only (D-063).
type InterfaceDescriptor struct {
	df7.Base
	modes *Modes
}

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the igmp.interface descriptor; modes is shared with NewListen.
func NewInterface(c vpp.Client, owner string, modes *Modes, opts ...df7.Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{Base: df7.NewBase(NameInterface, c, owner, opts), modes: modes}
}

// KeyOf implements scheduler.Descriptor.
func (d *InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	i, _ := df7.Decode[Interface](obj)
	return KeyInterface(i.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	i, _ := df7.Decode[Interface](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(i.Interface)}
}

func (d *InterfaceDescriptor) set(ctx context.Context, idx uint32, i Interface, enable bool) error {
	var mode uint8
	if i.Mode == ModeHost {
		mode = 1
	}
	_, err := igmp.NewServiceClient(d.Client).IgmpEnableDisable(ctx, &igmp.IgmpEnableDisable{Enable: enable, Mode: mode, SwIfIndex: interface_types.InterfaceIndex(idx)})
	return d.Wrap(fmt.Sprintf("igmp_enable_disable %s %s enable=%v", i.Interface, i.Mode, enable), df7.PluginError("igmp", err))
}

// Create implements scheduler.Descriptor. Idempotent: VPP answers an enable of an enabled
// interface with UNSPECIFIED (-1), which is success here (the mode cannot be read back).
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	i, err := df7.DecodeValid[Interface](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, i.Interface, string(KeyInterface(i.Interface)))
	if err != nil {
		return nil, err
	}
	idx := tg.Index
	err = d.set(ctx, idx, i, true)
	switch {
	case df7.IsVPPError(err, api.UNSPECIFIED):
		// -1: IGMP is already on — success only when it is ours (a resync of our own object);
		// another owner's / an operator's config is never adopted (review M1). The mode cannot
		// be read back, so a mode drift is not detected (igmp.md, review L4).
		if err := tg.Adopt(); err != nil {
			return nil, fmt.Errorf("%s: %w", NameInterface, err)
		}
	case err != nil:
		return nil, err
	default:
		if err := tg.Claim(); err != nil {
			return nil, err
		}
	}
	if d.modes != nil {
		d.modes.set(i.Interface, i.Mode)
	}
	return Meta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: a mode change needs disable + enable (ErrRecreate).
func (*InterfaceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: disable (VPP also removes the interface from its
// proxy device and deletes a proxy device whose upstream it is).
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	i, err := df7.Decode[Interface](obj)
	if err != nil {
		return err
	}
	err = detach(ctx, d.Base, i.Interface, string(KeyInterface(i.Interface)), func(idx uint32) error {
		if err := d.set(ctx, idx, i, false); err != nil && !df7.IsVPPError(err, api.UNSPECIFIED) {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if d.modes != nil {
		d.modes.set(i.Interface, "")
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *InterfaceDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameInterface, "VPP 26.06 reports no IGMP interface configuration")
}

// ---- igmp.listen ------------------------------------------------------------------------------

// ListenDescriptor manages igmp.listen objects.
type ListenDescriptor struct {
	df7.Base
	modes *Modes
}

var _ scheduler.Descriptor = (*ListenDescriptor)(nil)

// NewListen returns the igmp.listen descriptor; modes is shared with NewInterface.
func NewListen(c vpp.Client, owner string, modes *Modes, opts ...df7.Option) *ListenDescriptor {
	return &ListenDescriptor{Base: df7.NewBase(NameListen, c, owner, opts), modes: modes}
}

// KeyOf implements scheduler.Descriptor.
func (d *ListenDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	l, _ := df7.Decode[Listen](obj)
	return KeyListen(l.Interface, l.Group)
}

// Dependencies implements scheduler.Descriptor: the interface's IGMP config (host mode).
func (d *ListenDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	l, _ := df7.Decode[Listen](obj)
	return []scheduler.Dependency{{Key: KeyInterface(l.Interface)}, d.Opts.IfaceDep(l.Interface)}
}

func (d *ListenDescriptor) listen(ctx context.Context, idx uint32, l Listen, sources []string) error {
	g := netip.MustParseAddr(l.Group)
	req := &igmp.IgmpListen{Group: igmp.IgmpGroup{Filter: igmp.INCLUDE, SwIfIndex: interface_types.InterfaceIndex(idx), Gaddr: ip_types.IP4Address(g.As4())}}
	for _, s := range sources {
		req.Group.Saddrs = append(req.Group.Saddrs, ip_types.IP4Address(netip.MustParseAddr(s).As4()))
	}
	req.Group.NSrcs = uint8(len(req.Group.Saddrs)) //nolint:gosec // ≤ 255
	_, err := igmp.NewServiceClient(d.Client).IgmpListen(ctx, req)
	return d.Wrap(fmt.Sprintf("igmp_listen %s %s %v", l.Interface, l.Group, sources), err)
}

// Create implements scheduler.Descriptor: igmp_listen INCLUDE with the sources (the interface
// must be admin-up and in host mode).
func (d *ListenDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	l, err := df7.DecodeValid[Listen](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, l.Interface, string(KeyListen(l.Interface, l.Group)))
	if err != nil {
		return nil, err
	}
	if err := d.listen(ctx, tg.Index, l, l.Sources); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: tg.Index}, tg.Claim()
}

// Update implements scheduler.Descriptor: a new source list replaces the old one in place
// (RFC 3376 §2: each listen request replaces the previous one).
func (d *ListenDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	o, err := df7.Decode[Listen](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Listen](newObj)
	if err != nil {
		return nil, err
	}
	if o.Interface != n.Interface || o.Group != n.Group {
		return nil, scheduler.ErrRecreate
	}
	tg, found, err := d.Detach(ctx, n.Interface, string(KeyListen(n.Interface, n.Group)))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w: %q", NameListen, df7.ErrNoSuchInterface, n.Interface)
	}
	return Meta{SwIfIndex: tg.Index}, d.listen(ctx, tg.Index, n, n.Sources)
}

// Delete implements scheduler.Descriptor: an INCLUDE listen with no sources (leave), on the
// re-resolved interface (D-071).
func (d *ListenDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	l, err := df7.Decode[Listen](obj)
	if err != nil {
		return err
	}
	return detach(ctx, d.Base, l.Interface, string(KeyListen(l.Interface, l.Group)), func(idx uint32) error {
		return d.listen(ctx, idx, l, nil)
	})
}

// Retrieve implements scheduler.Descriptor: igmp_dump (INCLUDE sources per group) on owned
// interfaces, skipping interfaces known to be in router mode (see Modes).
func (d *ListenDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := igmp.NewServiceClient(d.Client).IgmpDump(ctx, &igmp.IgmpDump{SwIfIndex: interface_types.InterfaceIndex(df7.NoIndex)})
	if err != nil {
		return nil, d.Wrap("igmp_dump", df7.PluginError("igmp", err))
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, d.Wrap("igmp_dump", err)
	}
	type gk struct {
		idx   uint32
		group string
	}
	groups := map[gk]map[string]bool{}
	names := map[uint32]string{}
	for _, det := range dets {
		g := netip.AddrFrom4(det.Gaddr).String()
		name, ok := ifs.Owned(uint32(det.SwIfIndex), func(n string) string { return string(KeyListen(n, g)) })
		if !ok || (d.modes != nil && d.modes.get(name) == ModeRouter) {
			continue
		}
		names[uint32(det.SwIfIndex)] = name
		k := gk{uint32(det.SwIfIndex), g}
		if groups[k] == nil {
			groups[k] = map[string]bool{}
		}
		groups[k][netip.AddrFrom4(det.Saddr).String()] = true // VPP sends every source twice: dedupe
	}
	out := make([]scheduler.KV, 0, len(groups))
	for k, srcs := range groups {
		l := Listen{Interface: names[k.idx], Group: k.group}
		for s := range srcs {
			l.Sources = append(l.Sources, s)
		}
		df7.SortAddrStrings(l.Sources)
		out = append(out, df7.KV(KeyListen(l.Interface, l.Group), l, Meta{SwIfIndex: k.idx}))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ---- igmp.group-prefix ------------------------------------------------------------------------

// GroupPrefixDescriptor manages igmp.group-prefix objects. Write-only (D-063, see the package
// doc).
type GroupPrefixDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*GroupPrefixDescriptor)(nil)

// NewGroupPrefix returns the igmp.group-prefix descriptor.
func NewGroupPrefix(c vpp.Client, owner string, opts ...df7.Option) *GroupPrefixDescriptor {
	return &GroupPrefixDescriptor{df7.NewBase(NameGroupPrefix, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *GroupPrefixDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	g, _ := df7.Decode[GroupPrefix](obj)
	return KeyGroupPrefix(g.Prefix)
}

// Dependencies implements scheduler.Descriptor: none.
func (*GroupPrefixDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *GroupPrefixDescriptor) set(ctx context.Context, g GroupPrefix, t igmp.GroupPrefixType) error {
	p := netip.MustParsePrefix(g.Prefix)
	_, err := igmp.NewServiceClient(d.Client).IgmpGroupPrefixSet(ctx, &igmp.IgmpGroupPrefixSet{Gp: igmp.GroupPrefix{Type: t, Prefix: df7.ToPrefix(p)}})
	return d.Wrap(fmt.Sprintf("igmp_group_prefix_set %s type %d", g.Prefix, t), df7.PluginError("igmp", err))
}

// Create implements scheduler.Descriptor: set the range to SSM (idempotent).
func (d *GroupPrefixDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	g, err := df7.DecodeValid[GroupPrefix](obj)
	if err != nil {
		return nil, err
	}
	return nil, d.set(ctx, g, igmp.SSM)
}

// Update implements scheduler.Descriptor: the prefix is the key.
func (*GroupPrefixDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: set the range back to ASM (VPP removes the entry).
func (d *GroupPrefixDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	g, err := df7.Decode[GroupPrefix](obj)
	if err != nil {
		return err
	}
	return d.set(ctx, g, igmp.ASM)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *GroupPrefixDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameGroupPrefix, "igmp_group_prefix_dump replies with the igmp_details message id in VPP 26.06")
}

// ---- igmp.proxy-device ------------------------------------------------------------------------

// ProxyDeviceDescriptor manages igmp.proxy-device objects. Write-only (D-063).
type ProxyDeviceDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*ProxyDeviceDescriptor)(nil)

// NewProxyDevice returns the igmp.proxy-device descriptor.
func NewProxyDevice(c vpp.Client, owner string, opts ...df7.Option) *ProxyDeviceDescriptor {
	return &ProxyDeviceDescriptor{df7.NewBase(NameProxyDevice, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *ProxyDeviceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	p, _ := df7.Decode[ProxyDevice](obj)
	return KeyProxyDevice(p.VRF)
}

// Dependencies implements scheduler.Descriptor: the VRF, the upstream interface and its IGMP
// config (host mode).
func (d *ProxyDeviceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	p, _ := df7.Decode[ProxyDevice](obj)
	return []scheduler.Dependency{{Key: df7.VRFKey(p.VRF)}, d.Opts.IfaceDep(p.Upstream), {Key: KeyInterface(p.Upstream)}}
}

func (d *ProxyDeviceDescriptor) set(ctx context.Context, idx uint32, p ProxyDevice, add bool) error {
	var a uint8
	if add {
		a = 1
	}
	_, err := igmp.NewServiceClient(d.Client).IgmpProxyDeviceAddDel(ctx, &igmp.IgmpProxyDeviceAddDel{Add: a, VrfID: p.VRF, SwIfIndex: interface_types.InterfaceIndex(idx)})
	return d.Wrap(fmt.Sprintf("igmp_proxy_device_add_del vrf %d %s add=%v", p.VRF, p.Upstream, add), df7.PluginError("igmp", err))
}

// Create implements scheduler.Descriptor (idempotent: VPP keeps an existing device).
func (d *ProxyDeviceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p, err := df7.DecodeValid[ProxyDevice](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, p.Upstream, string(KeyProxyDevice(p.VRF)))
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, tg.Index, p, true); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: tg.Index}, tg.Claim()
}

// Update implements scheduler.Descriptor: another upstream is a new device (ErrRecreate).
func (*ProxyDeviceDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor on the re-resolved upstream interface (D-071); VPP
// also drops the downstream list, and ignores a delete of a device that does not exist.
func (d *ProxyDeviceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	p, err := df7.Decode[ProxyDevice](obj)
	if err != nil {
		return err
	}
	return detach(ctx, d.Base, p.Upstream, string(KeyProxyDevice(p.VRF)), func(idx uint32) error {
		// INVALID_INTERFACE: the upstream left the VRF or IGMP — the device is gone with it
		if err := d.set(ctx, idx, p, false); err != nil && !df7.IsVPPError(err, api.INVALID_INTERFACE) {
			return err
		}
		return nil
	})
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *ProxyDeviceDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameProxyDevice, "VPP 26.06 has no dump of IGMP proxy devices")
}

// ---- igmp.proxy-downstream --------------------------------------------------------------------

// DownstreamDescriptor manages igmp.proxy-downstream objects. Write-only (D-063).
type DownstreamDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*DownstreamDescriptor)(nil)

// NewDownstream returns the igmp.proxy-downstream descriptor.
func NewDownstream(c vpp.Client, owner string, opts ...df7.Option) *DownstreamDescriptor {
	return &DownstreamDescriptor{df7.NewBase(NameDownstream, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *DownstreamDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, _ := df7.Decode[Downstream](obj)
	return KeyDownstream(s.VRF, s.Interface)
}

// Dependencies implements scheduler.Descriptor: the proxy device, the interface and its IGMP
// config (router mode).
func (d *DownstreamDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, _ := df7.Decode[Downstream](obj)
	return []scheduler.Dependency{{Key: KeyProxyDevice(s.VRF)}, d.Opts.IfaceDep(s.Interface), {Key: KeyInterface(s.Interface)}}
}

func (d *DownstreamDescriptor) set(ctx context.Context, idx uint32, s Downstream, add bool) error {
	_, err := igmp.NewServiceClient(d.Client).IgmpProxyDeviceAddDelInterface(ctx, &igmp.IgmpProxyDeviceAddDelInterface{Add: add, VrfID: s.VRF, SwIfIndex: interface_types.InterfaceIndex(idx)})
	return d.Wrap(fmt.Sprintf("igmp_proxy_device_add_del_interface vrf %d %s add=%v", s.VRF, s.Interface, add), df7.PluginError("igmp", err))
}

// Create implements scheduler.Descriptor. Idempotent: VPP answers -1 (UNSPECIFIED) both for
// "already downstream" and "no proxy device"; the mandatory dependency on the device makes the
// second impossible through the scheduler, so -1 is taken as "already there".
func (d *DownstreamDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := df7.DecodeValid[Downstream](obj)
	if err != nil {
		return nil, err
	}
	tg, err := d.Target(ctx, s.Interface, string(KeyDownstream(s.VRF, s.Interface)))
	if err != nil {
		return nil, err
	}
	err = d.set(ctx, tg.Index, s, true)
	switch {
	case df7.IsVPPError(err, api.UNSPECIFIED): // -1: already a downstream — only ours is accepted
		if err := tg.Adopt(); err != nil {
			return nil, fmt.Errorf("%s: %w", NameDownstream, err)
		}
	case err != nil:
		return nil, err
	default:
		if err := tg.Claim(); err != nil {
			return nil, err
		}
	}
	return Meta{SwIfIndex: tg.Index}, nil
}

// Update implements scheduler.Descriptor: everything is in the key.
func (*DownstreamDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. "Not a downstream" (-2/-3, e.g. the device was
// already deleted with its list) is success.
func (d *DownstreamDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	s, err := df7.Decode[Downstream](obj)
	if err != nil {
		return err
	}
	return detach(ctx, d.Base, s.Interface, string(KeyDownstream(s.VRF, s.Interface)), func(idx uint32) error {
		if err := d.set(ctx, idx, s, false); err != nil &&
			!df7.IsVPPError(err, api.UNSPECIFIED, api.INVALID_SW_IF_INDEX, api.NO_SUCH_FIB, api.INVALID_INTERFACE) {
			return err
		}
		return nil
	})
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *DownstreamDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameDownstream, "VPP 26.06 has no dump of IGMP proxy downstream interfaces")
}

// ClearInterface is the igmp_clear_interface action helper: drop all learned state of an
// interface.
func ClearInterface(ctx context.Context, c vpp.Client, swIfIndex uint32) error {
	_, err := igmp.NewServiceClient(c).IgmpClearInterface(ctx, &igmp.IgmpClearInterface{SwIfIndex: interface_types.InterfaceIndex(swIfIndex)})
	return err
}

// Register constructs the per-interface igmp descriptors; interface and listen share one Modes
// registry.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	modes := NewModes()
	r.Register(NewInterface(c, owner, modes, opts...))
	r.Register(NewListen(c, owner, modes, opts...))
	r.Register(NewProxyDevice(c, owner, opts...))
	r.Register(NewDownstream(c, owner, opts...))
}

// RegisterGlobals constructs the VPP-global igmp.group-prefix descriptor (the SSM range list is
// one list per VPP). Only the globals owner calls it (D-071).
func RegisterGlobals(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewGroupPrefix(c, owner, opts...))
}
