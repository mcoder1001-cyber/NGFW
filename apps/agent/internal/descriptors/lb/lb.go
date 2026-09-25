// Package lb holds the reconciler descriptors for the VPP load-balancer plugin (task DF-7, WBS
// D7.9, tier T3 — minimal): the global settings (lb_conf), virtual IPs (lb_add_del_vip_v2),
// their application servers (lb_add_del_as) and the NAT4/NAT6 in2out feature on an interface
// (lb_add_del_intf_nat4 / _nat6). lb_flush_vip is exposed as the FlushVIP action helper.
//
// All four are write-only (D-063). VPP 26.06 has no getter for lb_conf and no dump of the
// NAT interface feature; lb_vip_dump and lb_as_dump exist but corrupt the fields the diff
// needs (api.c: vip.protocol = htonl(protocol) into a u8 and flow_table_length =
// htonl(mask+1) into a u16 are always 0 on a little-endian host; src_ip_sticky and the
// new-flows table length are not reported; deleted VIPs/ASes stay listed until VPP's garbage
// collection). A Retrieve built on them could not tell a TCP from a UDP VIP. DumpVIPs returns
// the fields that do decode as read-only state and Create uses it to make re-application
// idempotent (VALUE_EXIST on an identical VIP is success, on a different one an error).
//
// Messages come only from apps/agent/binapi/{lb,lb_types}. docs/agent/descriptors/lb.md is the
// object ↔ message table.
package lb

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"net/netip"
	"strconv"
	"sync/atomic"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lb"
	"ngfw/agent/binapi/lb_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameConf    = "lb.conf"
	NameVIP     = "lb.vip"
	NameAS      = "lb.as"
	NameIntfNat = "lb.intf-nat"
)

// ConfID is the object id of the lb.conf singleton.
const ConfID = "global"

// VPP 26.06 start-up values (lb.c lb_init) that Conf.Delete restores.
const (
	DefaultIP4Src        = "255.255.255.255"
	DefaultIP6Src        = "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"
	DefaultStickyBuckets = 1 << 10
	DefaultFlowTimeout   = 40
)

// Protocols of a VIP.
const (
	ProtoAny = "any"
	ProtoTCP = "tcp"
	ProtoUDP = "udp"
)

var protocols = map[string]uint8{ProtoAny: 255, ProtoTCP: 6, ProtoUDP: 17}

// Encapsulations of a VIP.
const (
	EncapGRE4  = "gre4"
	EncapGRE6  = "gre6"
	EncapL3DSR = "l3dsr"
	EncapNAT4  = "nat4"
	EncapNAT6  = "nat6"
)

var encaps = map[string]lb_types.LbEncapType{
	EncapGRE4:  lb_types.LB_API_ENCAP_TYPE_GRE4,
	EncapGRE6:  lb_types.LB_API_ENCAP_TYPE_GRE6,
	EncapL3DSR: lb_types.LB_API_ENCAP_TYPE_L3DSR,
	EncapNAT4:  lb_types.LB_API_ENCAP_TYPE_NAT4,
	EncapNAT6:  lb_types.LB_API_ENCAP_TYPE_NAT6,
}

// Service types of a NAT VIP.
const (
	SrvClusterIP = "clusterip"
	SrvNodePort  = "nodeport"
)

var srvTypes = map[string]lb_types.LbSrvType{
	SrvClusterIP: lb_types.LB_API_SRV_TYPE_CLUSTERIP,
	SrvNodePort:  lb_types.LB_API_SRV_TYPE_NODEPORT,
}

// ---- specs ------------------------------------------------------------------------------------

// Conf is the desired state of the lb.conf singleton: the GRE source addresses and the
// sticky-table size / flow timeout (0 = keep VPP's current value).
type Conf struct {
	IP4Src               string `json:"ip4_src,omitempty"`
	IP6Src               string `json:"ip6_src,omitempty"`
	StickyBucketsPerCore uint32 `json:"sticky_buckets_per_core,omitempty"`
	FlowTimeout          uint32 `json:"flow_timeout,omitempty"`
}

// Validate checks c.
func (c Conf) Validate() error {
	if c.IP4Src != "" {
		if a, err := df7.ParseAddr(c.IP4Src); err != nil || !a.Is4() {
			return df7.Specf("ip4_src %q is not an IPv4 address", c.IP4Src)
		}
	}
	if c.IP6Src != "" {
		if a, err := df7.ParseAddr(c.IP6Src); err != nil || !a.Is6() {
			return df7.Specf("ip6_src %q is not an IPv6 address", c.IP6Src)
		}
	}
	if b := c.StickyBucketsPerCore; b != 0 && b&(b-1) != 0 {
		return df7.Specf("sticky_buckets_per_core %d is not a power of two", b)
	}
	return nil
}

// VIP identifies a virtual IP: prefix, protocol (any/tcp/udp) and port (0 = all ports, which
// forces protocol any).
type VIP struct {
	Prefix   string `json:"prefix,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Port     uint16 `json:"port,omitempty"`
}

func (v VIP) validate() (netip.Prefix, error) {
	p, err := df7.ParsePrefix(v.Prefix)
	if err != nil {
		return p, err
	}
	if _, ok := protocols[v.Protocol]; !ok {
		return p, df7.Specf("vip protocol %q: want any, tcp or udp", v.Protocol)
	}
	if (v.Port == 0) != (v.Protocol == ProtoAny) {
		return p, df7.Specf("vip port 0 means all ports with protocol any; port %d with protocol %q is ambiguous", v.Port, v.Protocol)
	}
	return p, nil
}

func (v VIP) id() []string {
	return []string{v.Prefix, v.Protocol, strconv.FormatUint(uint64(v.Port), 10)}
}

// VIPSpec is the desired state of one lb.vip object.
type VIPSpec struct {
	VIP
	Encap               string `json:"encap,omitempty"`
	DSCP                uint8  `json:"dscp,omitempty"`        // l3dsr
	SrvType             string `json:"srv_type,omitempty"`    // nat4/nat6
	TargetPort          uint16 `json:"target_port,omitempty"` // nat4/nat6
	NodePort            uint16 `json:"node_port,omitempty"`   // nat4/nat6 nodeport
	NewFlowsTableLength uint32 `json:"new_flows_table_length,omitempty"`
	SrcIPSticky         bool   `json:"src_ip_sticky,omitempty"`
}

// Validate checks v.
func (v VIPSpec) Validate() error {
	p, err := v.validate()
	if err != nil {
		return err
	}
	if _, ok := encaps[v.Encap]; !ok {
		return df7.Specf("vip encap %q: want gre4, gre6, l3dsr, nat4 or nat6", v.Encap)
	}
	switch {
	case p.Addr().Is4() && v.Encap == EncapNAT6, p.Addr().Is6() && (v.Encap == EncapNAT4 || v.Encap == EncapL3DSR):
		return df7.Specf("vip %s cannot use encap %s", v.Prefix, v.Encap)
	}
	nat := v.Encap == EncapNAT4 || v.Encap == EncapNAT6
	if nat {
		if _, ok := srvTypes[v.SrvType]; !ok {
			return df7.Specf("nat vip needs srv_type clusterip or nodeport")
		}
	} else if v.SrvType != "" || v.TargetPort != 0 || v.NodePort != 0 {
		return df7.Specf("srv_type/target_port/node_port apply to nat4/nat6 VIPs only")
	}
	if v.Encap != EncapL3DSR && v.DSCP != 0 {
		return df7.Specf("dscp applies to l3dsr VIPs only")
	}
	if v.DSCP > 63 {
		return df7.Specf("dscp %d exceeds 63", v.DSCP)
	}
	if n := v.NewFlowsTableLength; n != 0 && n&(n-1) != 0 {
		return df7.Specf("new_flows_table_length %d is not a power of two", n)
	}
	return nil
}

// AS is the desired state of one lb.as object: an application server of a VIP.
type AS struct {
	VIP     VIP    `json:"vip"`
	Address string `json:"address,omitempty"`
	// FlushOnDelete also flushes the VIP's flow table entries of this AS when it is removed.
	FlushOnDelete bool `json:"flush_on_delete,omitempty"`
}

// Validate checks a.
func (a AS) Validate() error {
	if _, err := a.VIP.validate(); err != nil {
		return err
	}
	if _, err := df7.ParseAddr(a.Address); err != nil {
		return err
	}
	return nil
}

// Families of the NAT in2out feature.
const (
	FamilyIP4 = "ip4"
	FamilyIP6 = "ip6"
)

// IntfNat is the desired state of one lb.intf-nat object.
type IntfNat struct {
	Interface string `json:"interface,omitempty"`
	Family    string `json:"family,omitempty"`
}

// Validate checks n.
func (n IntfNat) Validate() error {
	if n.Interface == "" {
		return df7.Specf("lb intf-nat needs an interface")
	}
	if n.Family != FamilyIP4 && n.Family != FamilyIP6 {
		return df7.Specf("lb intf-nat family %q: want ip4 or ip6", n.Family)
	}
	return nil
}

// ---- keys -------------------------------------------------------------------------------------

// KeyConf is "lb.conf/global".
func KeyConf() scheduler.Key { return scheduler.Join(NameConf, ConfID) }

// KeyVIP is "lb.vip/<prefix>/<protocol>/<port>".
func KeyVIP(v VIP) scheduler.Key { return scheduler.Join(NameVIP, v.id()...) }

// KeyAS is "lb.as/<vip prefix>/<protocol>/<port>/<as address>".
func KeyAS(v VIP, addr string) scheduler.Key {
	return scheduler.Join(NameAS, append(v.id(), addr)...)
}

// KeyIntfNat is "lb.intf-nat/<interface>/<family>".
func KeyIntfNat(ifName, family string) scheduler.Key {
	return scheduler.Join(NameIntfNat, ifName, family)
}

// ---- lb.conf ----------------------------------------------------------------------------------

// ConfDescriptor manages the lb.conf singleton (lb_conf). Write-only (D-063).
type ConfDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*ConfDescriptor)(nil)

// NewConf returns the lb.conf descriptor.
func NewConf(c vpp.Client, owner string, opts ...df7.Option) *ConfDescriptor {
	return &ConfDescriptor{df7.NewBase(NameConf, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (*ConfDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyConf() }

// Dependencies implements scheduler.Descriptor: none.
func (*ConfDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *ConfDescriptor) set(ctx context.Context, c Conf) error {
	req := &lb.LbConf{StickyBucketsPerCore: ^uint32(0), FlowTimeout: ^uint32(0)}
	if c.StickyBucketsPerCore != 0 {
		req.StickyBucketsPerCore = c.StickyBucketsPerCore
	}
	if c.FlowTimeout != 0 {
		req.FlowTimeout = c.FlowTimeout
	}
	ip4, ip6 := c.IP4Src, c.IP6Src
	if ip4 == "" {
		ip4 = DefaultIP4Src
	}
	if ip6 == "" {
		ip6 = DefaultIP6Src
	}
	req.IP4SrcAddress = ip_types.IP4Address(netip.MustParseAddr(ip4).As4())
	req.IP6SrcAddress = ip_types.IP6Address(netip.MustParseAddr(ip6).As16())
	_, err := lb.NewServiceClient(d.Client).LbConf(ctx, req)
	return d.Wrap("lb_conf", df7.PluginError("lb", err))
}

// Create implements scheduler.Descriptor (idempotent). Unset source addresses are sent as
// VPP's start-up value (all-ones): lb_conf always writes both.
func (d *ConfDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	c, err := df7.DecodeValid[Conf](obj)
	if err != nil {
		return nil, err
	}
	return nil, d.set(ctx, c)
}

// Update implements scheduler.Descriptor: lb_conf in place.
func (d *ConfDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: restores VPP's start-up values.
func (d *ConfDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.set(ctx, Conf{StickyBucketsPerCore: DefaultStickyBuckets, FlowTimeout: DefaultFlowTimeout})
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *ConfDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameConf, "VPP 26.06 has no getter for lb_conf")
}

// ---- lb.vip -----------------------------------------------------------------------------------

// VIPDescriptor manages lb.vip objects (lb_add_del_vip_v2). Write-only (D-063, see the
// package doc).
type VIPDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*VIPDescriptor)(nil)

// NewVIP returns the lb.vip descriptor.
func NewVIP(c vpp.Client, owner string, opts ...df7.Option) *VIPDescriptor {
	return &VIPDescriptor{df7.NewBase(NameVIP, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *VIPDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	v, _ := df7.Decode[VIPSpec](obj)
	return KeyVIP(v.VIP)
}

// Dependencies implements scheduler.Descriptor: lb.conf, optional.
func (d *VIPDescriptor) Dependencies(proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: KeyConf(), Optional: true}}
}

// rawEnum works around VPP 26.06 lb api.c, which compares the u32 enums encap and type of
// lb_add_del_vip(_v2) without ntohl: govpp sends them big-endian, so every value but 0 (gre4,
// clusterip) would be misread and the VIP rejected (INVALID_ADDRESS_FAMILY). Byte-swapping
// here makes the wire bytes equal the host-order value VPP expects on a little-endian host
// (x86_64/arm64, the only VRX targets). docs/agent/descriptors/lb.md, DF-7-questions.md.
func rawEnum(v uint32) uint32 {
	if enumNative.Load() {
		return v // VPP converts with ntohl (V20 fixed): send the plain value
	}
	return bits.ReverseBytes32(v)
}

// enumNative is set when a runtime check found that the VPP in use converts the enums itself
// (review M2): the first non-zero encap whose VIP VPP rejects or reports with another type
// flips it and the add is retried once.
var enumNative atomic.Bool

// ErrEnumOrder is returned when neither byte order of the lb enums gives the requested VIP type.
var ErrEnumOrder = errors.New("lb: VPP did not create the requested VIP type in either enum byte order — lb API changed? (V20)")

// vipPrefix encodes a VIP prefix the way the lb plugin expects: ip46 prefix lengths, i.e. an
// IPv4 /n is sent as /(96+n) (util.h ip46_prefix_is_ip4 needs len ≥ 96).
func vipPrefix(v VIP) ip_types.AddressWithPrefix {
	p := netip.MustParsePrefix(v.Prefix)
	out := df7.ToAddressWithPrefix(p)
	if p.Addr().Is4() {
		out.Len += 96
	}
	return out
}

func (d *VIPDescriptor) addDel(ctx context.Context, v VIPSpec, del bool) error {
	req := &lb.LbAddDelVipV2{
		Pfx:                 vipPrefix(v.VIP),
		Protocol:            protocols[v.Protocol],
		Port:                v.Port,
		Encap:               lb_types.LbEncapType(rawEnum(uint32(encaps[v.Encap]))),
		Dscp:                v.DSCP,
		Type:                lb_types.LbSrvType(rawEnum(uint32(srvTypes[v.SrvType]))),
		TargetPort:          v.TargetPort,
		NodePort:            v.NodePort,
		NewFlowsTableLength: v.NewFlowsTableLength,
		SrcIPSticky:         v.SrcIPSticky,
		IsDel:               del,
	}
	if req.NewFlowsTableLength == 0 {
		req.NewFlowsTableLength = 1024
	}
	_, err := lb.NewServiceClient(d.Client).LbAddDelVipV2(ctx, req)
	return d.Wrap(fmt.Sprintf("lb_add_del_vip_v2 %s/%s/%d del=%v", v.Prefix, v.Protocol, v.Port, del), df7.PluginError("lb", err))
}

// Create implements scheduler.Descriptor. VIPs are untagged global objects: an existing VIP
// (VALUE_EXIST) is ours only with this owner's record, written after our own successful add on
// this VPP instance (review M4) — never adopted. After the add, lb_vip_dump must report the
// requested encapsulation (it reports the VIP type correctly); if VPP rejected the type or made
// another one, the byte order of the enums is switched once (VPP with the V20 ntohl fix) and the
// add retried; if that fails too the VIP is removed and ErrEnumOrder returned (review M2).
func (d *VIPDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	v, err := df7.DecodeValid[VIPSpec](obj)
	if err != nil {
		return nil, err
	}
	key := string(KeyVIP(v.VIP))
	for attempt := 0; attempt < 2; attempt++ {
		err = d.addDel(ctx, v, false)
		switch {
		case df7.IsVPPError(err, api.VALUE_EXIST):
			ours, rerr := d.Recorded(ctx, key)
			if rerr != nil {
				return nil, rerr
			}
			if !ours {
				return nil, fmt.Errorf("%s: %w: VIP %s exists and was not created by %s", NameVIP, dfkit.ErrNotOurs, key, d.Owner)
			}
			return nil, nil
		case df7.IsVPPError(err, api.INVALID_ADDRESS_FAMILY) && encaps[v.Encap] != 0:
			enumNative.Store(!enumNative.Load()) // VPP misread the encap: other byte order
			continue
		case err != nil:
			return nil, err
		}
		ok, cerr := d.typeMatches(ctx, v)
		if cerr != nil {
			return nil, cerr
		}
		if ok {
			return nil, d.RecordNow(ctx, key, "vip")
		}
		if derr := d.addDel(ctx, v, true); derr != nil {
			return nil, fmt.Errorf("%w; removing the wrong VIP failed: %v", ErrEnumOrder, derr)
		}
		enumNative.Store(!enumNative.Load())
	}
	return nil, fmt.Errorf("%s %s: %w (last error: %v)", NameVIP, key, ErrEnumOrder, err)
}

// typeMatches checks with lb_vip_dump that VPP created v with the requested encapsulation (the
// last entry of that prefix/port: VPP lists deleted-but-not-collected VIPs too).
func (d *VIPDescriptor) typeMatches(ctx context.Context, v VIPSpec) (bool, error) {
	if encaps[v.Encap] == 0 {
		return true, nil // 0 reads the same in both byte orders
	}
	vips, err := DumpVIPs(ctx, d.Client)
	if err != nil {
		return false, err
	}
	enc := ""
	for _, s := range vips {
		if s.Prefix == v.Prefix && s.Port == v.Port {
			enc = s.Encap
		}
	}
	return enc == v.Encap, nil
}

// Update implements scheduler.Descriptor: VPP cannot modify a VIP.
func (*VIPDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor (ASes depend on the VIP and are removed first).
// Only a VIP this owner recorded on this VPP instance is deleted; NO_SUCH_ENTRY (already gone,
// e.g. after a VPP restart) is success (D-074, review M4).
func (d *VIPDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	v, err := df7.Decode[VIPSpec](obj)
	if err != nil {
		return err
	}
	key := string(KeyVIP(v.VIP))
	ours, err := d.Recorded(ctx, key)
	if err != nil {
		return err
	}
	if ours {
		if err := d.addDel(ctx, v, true); err != nil && !df7.IsVPPError(err, api.NO_SUCH_ENTRY) {
			return err
		}
	}
	return d.ForgetApplied(key)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *VIPDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameVIP, "lb_vip_details corrupts protocol and flow-table length in VPP 26.06")
}

// VIPState is what lb_vip_dump reports correctly (read-only state).
type VIPState struct {
	Prefix     string
	Port       uint16
	Encap      string
	DSCP       uint8
	TargetPort uint16
	// RawProtocol and RawFlowTableLength are shown as VPP sends them (always 0 on this host).
	RawProtocol        uint8
	RawFlowTableLength uint16
}

var vipTypeEncap = map[lb_types.LbVipType]string{
	lb_types.LB_API_VIP_TYPE_IP6_GRE6: EncapGRE6, lb_types.LB_API_VIP_TYPE_IP6_GRE4: EncapGRE4,
	lb_types.LB_API_VIP_TYPE_IP4_GRE6: EncapGRE6, lb_types.LB_API_VIP_TYPE_IP4_GRE4: EncapGRE4,
	lb_types.LB_API_VIP_TYPE_IP4_L3DSR: EncapL3DSR, lb_types.LB_API_VIP_TYPE_IP4_NAT4: EncapNAT4,
	lb_types.LB_API_VIP_TYPE_IP6_NAT6: EncapNAT6,
}

// DumpVIPs runs lb_vip_dump. VPP puts the VIP *type* into the encap field, which is mapped
// back to the encapsulation here.
func DumpVIPs(ctx context.Context, c vpp.Client) ([]VIPState, error) {
	stream, err := lb.NewServiceClient(c).LbVipDump(ctx, &lb.LbVipDump{Protocol: 255})
	if err != nil {
		return nil, fmt.Errorf("lb_vip_dump: %w", df7.PluginError("lb", err))
	}
	dets, err := df7.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("lb_vip_dump: %w", err)
	}
	out := make([]VIPState, 0, len(dets))
	for _, det := range dets {
		enc, ok := vipTypeEncap[lb_types.LbVipType(det.Encap)]
		if !ok {
			enc = fmt.Sprintf("#%d", det.Encap)
		}
		pfx := det.Vip.Pfx
		if pfx.Address.Af == ip_types.ADDRESS_IP4 && pfx.Len >= 96 {
			pfx.Len -= 96 // ip46 length of an IPv4 VIP
		}
		p := df7.FromAddressWithPrefix(pfx)
		out = append(out, VIPState{Prefix: p.Masked().String(), Port: det.Vip.Port, Encap: enc, DSCP: uint8(det.Dscp),
			TargetPort: det.TargetPort, RawProtocol: uint8(det.Vip.Protocol), RawFlowTableLength: det.FlowTableLength})
	}
	return out, nil
}

// FlushVIP is the lb_flush_vip action helper: flush the VIP's flow table. Call it only for a
// VIP that exists (VPP 26.06 flushes an uninitialised index when the lookup fails).
func FlushVIP(ctx context.Context, c vpp.Client, v VIP) error {
	if _, err := v.validate(); err != nil {
		return err
	}
	_, err := lb.NewServiceClient(c).LbFlushVip(ctx, &lb.LbFlushVip{Pfx: vipPrefix(v), Protocol: protocols[v.Protocol], Port: v.Port})
	return err
}

// ---- lb.as ------------------------------------------------------------------------------------

// ASDescriptor manages lb.as objects (lb_add_del_as). Write-only (D-063, see the package doc).
type ASDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*ASDescriptor)(nil)

// NewAS returns the lb.as descriptor.
func NewAS(c vpp.Client, owner string, opts ...df7.Option) *ASDescriptor {
	return &ASDescriptor{df7.NewBase(NameAS, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *ASDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	a, _ := df7.Decode[AS](obj)
	return KeyAS(a.VIP, a.Address)
}

// Dependencies implements scheduler.Descriptor: the VIP.
func (d *ASDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	a, _ := df7.Decode[AS](obj)
	return []scheduler.Dependency{{Key: KeyVIP(a.VIP)}}
}

func (d *ASDescriptor) addDel(ctx context.Context, a AS, del bool) error {
	addr, err := df7.ParseAddr(a.Address)
	if err != nil {
		return err
	}
	_, err = lb.NewServiceClient(d.Client).LbAddDelAs(ctx, &lb.LbAddDelAs{Pfx: vipPrefix(a.VIP), Protocol: protocols[a.VIP.Protocol], Port: a.VIP.Port,
		AsAddress: df7.ToAddress(addr), IsDel: del, IsFlush: del && a.FlushOnDelete})
	return d.Wrap(fmt.Sprintf("lb_add_del_as %s → %s del=%v", a.VIP.Prefix, a.Address, del), df7.PluginError("lb", err))
}

// Create implements scheduler.Descriptor (idempotent: VALUE_EXIST is success).
func (d *ASDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	a, err := df7.DecodeValid[AS](obj)
	if err != nil {
		return nil, err
	}
	key := string(KeyAS(a.VIP, a.Address))
	err = d.addDel(ctx, a, false)
	switch {
	case df7.IsVPPError(err, api.VALUE_EXIST): // ours only with our record (review M4)
		ours, rerr := d.Recorded(ctx, key)
		if rerr != nil {
			return nil, rerr
		}
		if !ours {
			return nil, fmt.Errorf("%s: %w: AS %s exists and was not created by %s", NameAS, dfkit.ErrNotOurs, key, d.Owner)
		}
		return nil, nil
	case err != nil:
		return nil, err
	}
	return nil, d.RecordNow(ctx, key, "as")
}

// Update implements scheduler.Descriptor: only FlushOnDelete can change, which VPP does not
// store; nothing to do.
func (d *ASDescriptor) Update(_ context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, err := df7.Decode[AS](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[AS](newObj)
	if err != nil {
		return nil, err
	}
	if o.VIP != n.VIP || o.Address != n.Address {
		return nil, scheduler.ErrRecreate
	}
	return meta, nil
}

// Delete implements scheduler.Descriptor.
// Only an AS this owner recorded is removed; NO_SUCH_ENTRY (VIP or AS already gone) is success.
func (d *ASDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	a, err := df7.Decode[AS](obj)
	if err != nil {
		return err
	}
	key := string(KeyAS(a.VIP, a.Address))
	ours, err := d.Recorded(ctx, key)
	if err != nil {
		return err
	}
	if ours {
		if err := d.addDel(ctx, a, true); err != nil && !df7.IsVPPError(err, api.NO_SUCH_ENTRY) {
			return err
		}
	}
	return d.ForgetApplied(key)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *ASDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameAS, "lb_as_details corrupts the VIP protocol in VPP 26.06")
}

// ---- lb.intf-nat ------------------------------------------------------------------------------

// IntfNatDescriptor manages lb.intf-nat objects (lb_add_del_intf_nat4 / _nat6). Write-only
// (D-063): VPP has no dump of the feature.
type IntfNatDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*IntfNatDescriptor)(nil)

// NewIntfNat returns the lb.intf-nat descriptor.
func NewIntfNat(c vpp.Client, owner string, opts ...df7.Option) *IntfNatDescriptor {
	return &IntfNatDescriptor{df7.NewBase(NameIntfNat, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *IntfNatDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	n, _ := df7.Decode[IntfNat](obj)
	return KeyIntfNat(n.Interface, n.Family)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *IntfNatDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	n, _ := df7.Decode[IntfNat](obj)
	return []scheduler.Dependency{d.Opts.IfaceDep(n.Interface)}
}

// NatMeta is the interface index.
type NatMeta struct{ SwIfIndex uint32 }

func (d *IntfNatDescriptor) set(ctx context.Context, idx uint32, family string, add bool) error {
	svc := lb.NewServiceClient(d.Client)
	var err error
	if family == FamilyIP6 {
		_, err = svc.LbAddDelIntfNat6(ctx, &lb.LbAddDelIntfNat6{IsAdd: add, SwIfIndex: interface_types.InterfaceIndex(idx)})
	} else {
		_, err = svc.LbAddDelIntfNat4(ctx, &lb.LbAddDelIntfNat4{IsAdd: add, SwIfIndex: interface_types.InterfaceIndex(idx)})
	}
	return d.Wrap(fmt.Sprintf("lb_add_del_intf_nat %s %d add=%v", family, idx, add), df7.PluginError("lb", err))
}

// Create implements scheduler.Descriptor: enable once per VPP lifetime (D-076) — every
// lb_add_del_intf_nat4 enables the lb-nat4-in2out feature again, which stacks a second instance
// of the node, so a re-application with an unchanged boot identity is skipped.
func (d *IntfNatDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	n, err := df7.DecodeValid[IntfNat](obj)
	if err != nil {
		return nil, err
	}
	key := string(KeyIntfNat(n.Interface, n.Family))
	tg, err := d.Target(ctx, n.Interface, key)
	if err != nil {
		return nil, err
	}
	skipped, err := d.ApplyOnce(ctx, key, df7.IfaceValue(tg.Index, n.Interface), func() error { return d.set(ctx, tg.Index, n.Family, true) })
	if err != nil {
		return nil, err
	}
	if !skipped {
		if err := tg.Claim(); err != nil {
			return nil, err
		}
	}
	return NatMeta{SwIfIndex: tg.Index}, nil
}

// Update implements scheduler.Descriptor: every field is in the key.
func (*IntfNatDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor: re-resolve the interface (D-071) and disable — only
// when enabled in this VPP lifetime (after a VPP restart the feature is gone).
func (d *IntfNatDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	n, err := df7.Decode[IntfNat](obj)
	if err != nil {
		return err
	}
	key := string(KeyIntfNat(n.Interface, n.Family))
	tg, found, err := d.Detach(ctx, n.Interface, key)
	if err != nil {
		return err
	}
	if found {
		applied, err := d.AppliedNow(ctx, key, df7.IfaceValue(tg.Index, n.Interface))
		if err != nil {
			return err
		}
		if applied {
			if err := d.set(ctx, tg.Index, n.Family, false); err != nil {
				return err
			}
		}
		if err := tg.Release(); err != nil {
			return err
		}
	}
	return d.ForgetApplied(key)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *IntfNatDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameIntfNat, "VPP 26.06 has no dump of the lb nat in2out feature")
}

// Register constructs the per-object lb descriptors (VIPs, ASes, NAT interfaces). lb.vip
// depends on lb.conf only optionally, so it works whether or not this agent is the globals
// owner.
// Not on kit.Register: takes per-family options or extra dependencies beyond kit.Env (TD-16).
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewVIP(c, owner, opts...))
	r.Register(NewAS(c, owner, opts...))
	r.Register(NewIntfNat(c, owner, opts...))
}

// RegisterGlobals constructs the VPP-global lb.conf descriptor. Only the globals owner calls
// it (D-071).
func RegisterGlobals(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewConf(c, owner, opts...))
}
