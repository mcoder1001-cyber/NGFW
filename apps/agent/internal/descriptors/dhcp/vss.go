package dhcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ProxyVSSDescriptor manages dhcp.proxy-vss objects: key dhcp.proxy-vss/<ip4|ip6>/<vrf>.
//
// VPP reports the VSS of a VRF only inside dhcp_proxy_details, i.e. only while that VRF relays
// to at least one server: a VSS on a VRF without a dhcp.proxy object is never retrieved (and
// would be re-created on every reconcile). Configure it only on relaying VRFs.
type ProxyVSSDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*ProxyVSSDescriptor)(nil)

// NewProxyVSS returns the dhcp.proxy-vss descriptor.
func NewProxyVSS(client vpp.Client, opts ...Option) *ProxyVSSDescriptor {
	return &ProxyVSSDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ProxyVSSDescriptor) Name() string { return NameProxyVSS }

// ProxyVSSKey returns the key of the VSS of a VRF.
func ProxyVSSKey(family string, vrf uint32) scheduler.Key {
	return scheduler.Join(NameProxyVSS, family, uitoa(vrf))
}

// KeyOf implements scheduler.Descriptor.
func (d *ProxyVSSDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[ProxyVSS](obj)
	if err != nil {
		return scheduler.Join(NameProxyVSS, "invalid")
	}
	return ProxyVSSKey(s.Family, s.VRF)
}

// Dependencies implements scheduler.Descriptor: the VRF (optional).
func (d *ProxyVSSDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[ProxyVSS](obj)
	if err != nil {
		return nil
	}
	return d.o.vrfDeps(s.VRF)
}

func vssTypeToAPI(t string) dhcp.VssType {
	switch t {
	case VSSASCII:
		return dhcp.VSS_TYPE_API_ASCII
	case VSSVPNID:
		return dhcp.VSS_TYPE_API_VPN_ID
	default:
		return dhcp.VSS_TYPE_API_DEFAULT
	}
}

func (d *ProxyVSSDescriptor) set(ctx context.Context, obj proto.Message, add bool) error {
	s, err := decode[ProxyVSS](obj)
	if err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if !d.o.vrfScope(s.VRF) {
		return dfkit.Specf("dhcp proxy-vss: vrf %d is outside this agent's VRF scope", s.VRF)
	}
	_, err = dhcp.NewServiceClient(d.client).DHCPProxySetVss(ctx, &dhcp.DHCPProxySetVss{
		TblID: s.VRF, VssType: vssTypeToAPI(s.Type), VPNAsciiID: s.VPNASCIIID,
		Oui: s.OUI, VPNIndex: s.VPNIndex, IsIPv6: s.Family == "ip6", IsAdd: add,
	})
	if err != nil {
		return fmt.Errorf("dhcp_proxy_set_vss(is_add=%t, %s vrf %d): %w", add, s.Family, s.VRF, err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *ProxyVSSDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.set(ctx, obj, true)
}

// Update implements scheduler.Descriptor: dhcp_proxy_set_vss replaces the VSS of the VRF in place.
func (d *ProxyVSSDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.set(ctx, newObj, true)
}

// Delete implements scheduler.Descriptor. VPP reports a VSS only on relaying VRFs, so the D-074
// existence check cannot see every VSS; the delete itself answers NO_SUCH_ENTRY for a missing one
// (host-verified harmless).
func (d *ProxyVSSDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	err := d.set(ctx, obj, false)
	if dfkit.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: the VSS fields of dhcp_proxy_details (ip4 + ip6) of
// the rx VRFs in scope.
func (d *ProxyVSSDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	details, err := dumpProxies(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, p := range details {
		if !d.o.vrfScope(p.RxVrfID) {
			continue
		}
		s := ProxyVSS{Family: "ip4", VRF: p.RxVrfID}
		if p.IsIPv6 {
			s.Family = "ip6"
		}
		switch p.VssType {
		case dhcp.VSS_TYPE_API_ASCII:
			s.Type, s.VPNASCIIID = VSSASCII, strings.TrimRight(p.VssVPNAsciiID, "\x00")
		case dhcp.VSS_TYPE_API_VPN_ID:
			s.Type, s.OUI, s.VPNIndex = VSSVPNID, p.VssOui, p.VssFibID
		case dhcp.VSS_TYPE_API_DEFAULT:
			s.Type = VSSDefault
		default: // VSS_TYPE_API_INVALID: no VSS on this VRF
			continue
		}
		out = append(out, scheduler.KV{Key: ProxyVSSKey(s.Family, s.VRF), Value: s.Proto()})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return dfkit.Dedupe(out), nil
}
