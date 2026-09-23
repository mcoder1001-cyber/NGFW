package dhcp

import (
	"context"
	"fmt"
	"net/netip"
	"sort"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ProxyDescriptor manages dhcp.proxy objects: key dhcp.proxy/<rx_vrf>/<server_vrf>/<server>.
type ProxyDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*ProxyDescriptor)(nil)

// NewProxy returns the dhcp.proxy descriptor.
func NewProxy(client vpp.Client, opts ...Option) *ProxyDescriptor {
	return &ProxyDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ProxyDescriptor) Name() string { return NameProxy }

// ProxyKey returns the key of a relay server object.
func ProxyKey(rxVRF, serverVRF uint32, server string) scheduler.Key {
	return scheduler.Join(NameProxy, uitoa(rxVRF), uitoa(serverVRF), server)
}

// KeyOf implements scheduler.Descriptor.
func (d *ProxyDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[Proxy](obj)
	if err != nil {
		return scheduler.Join(NameProxy, "invalid")
	}
	return ProxyKey(s.RxVRF, s.ServerVRF, canonAddr(s.Server))
}

// Dependencies implements scheduler.Descriptor: the rx and server VRFs (optional: VPP creates a
// missing table itself, so they only order the plan).
func (d *ProxyDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[Proxy](obj)
	if err != nil {
		return nil
	}
	return d.o.vrfDeps(s.RxVRF, s.ServerVRF)
}

func (d *ProxyDescriptor) config(ctx context.Context, obj proto.Message, add bool) error {
	s, err := decode[Proxy](obj)
	if err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if !d.o.vrfScope(s.RxVRF) {
		return dfkit.Specf("dhcp proxy: rx vrf %d is outside this agent's VRF scope", s.RxVRF)
	}
	srv, _ := dfkit.ParseAddr(s.Server)
	src, _ := dfkit.ParseAddr(s.Src)
	_, err = dhcp.NewServiceClient(d.client).DHCPProxyConfig(ctx, &dhcp.DHCPProxyConfig{
		RxVrfID: s.RxVRF, ServerVrfID: s.ServerVRF, IsAdd: add,
		DHCPServer: dfkit.ToAPIAddress(srv), DHCPSrcAddress: dfkit.ToAPIAddress(src),
	})
	if err != nil {
		return fmt.Errorf("dhcp_proxy_config(is_add=%t, rx_vrf %d, server %s): %w", add, s.RxVRF, s.Server, err)
	}
	return nil
}

// Create implements scheduler.Descriptor (adding an existing server is a no-op in VPP).
func (d *ProxyDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.config(ctx, obj, true)
}

// Update implements scheduler.Descriptor: the only non-key field is the source address, which
// VPP keeps per rx VRF — recreate.
func (d *ProxyDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *ProxyDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	err := d.config(ctx, obj, false)
	if dfkit.IsVPPError(err, api.NO_SUCH_ENTRY) {
		return nil
	}
	return err
}

// dumpProxies returns the proxy details of both families.
func dumpProxies(ctx context.Context, c vpp.Client) ([]*dhcp.DHCPProxyDetails, error) {
	svc := dhcp.NewServiceClient(c)
	var out []*dhcp.DHCPProxyDetails
	for _, v6 := range []bool{false, true} {
		stream, err := svc.DHCPProxyDump(ctx, &dhcp.DHCPProxyDump{IsIP6: v6})
		if err != nil {
			return nil, fmt.Errorf("dhcp_proxy_dump(is_ip6=%t): %w", v6, err)
		}
		details, err := dfkit.Drain(stream, stream.Recv)
		if err != nil {
			return nil, fmt.Errorf("dhcp_proxy_dump(is_ip6=%t): %w", v6, err)
		}
		out = append(out, details...)
	}
	return out, nil
}

// detailAddr decodes an address of dhcp_proxy_details: VPP fills only the union (the af byte
// stays 0 = ip4), the family comes from is_ipv6.
func detailAddr(a ip_types.Address, v6 bool) netip.Addr {
	if v6 {
		return netip.AddrFrom16(a.Un.GetIP6())
	}
	return netip.AddrFrom4(a.Un.GetIP4())
}

// Retrieve implements scheduler.Descriptor: dhcp_proxy_dump (ip4 + ip6), one object per server
// of every rx VRF in scope.
func (d *ProxyDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	details, err := dumpProxies(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, p := range details {
		if !d.o.vrfScope(p.RxVrfID) {
			continue
		}
		src := detailAddr(p.DHCPSrcAddress, p.IsIPv6)
		for _, srv := range p.Servers {
			s := Proxy{
				RxVRF: p.RxVrfID, ServerVRF: srv.ServerVrfID,
				Server: detailAddr(srv.DHCPServer, p.IsIPv6).String(), Src: src.String(),
			}
			out = append(out, scheduler.KV{Key: ProxyKey(s.RxVRF, s.ServerVRF, s.Server), Value: s.Proto()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func canonAddr(s string) string {
	if a, err := dfkit.ParseAddr(s); err == nil {
		return a.String()
	}
	return s
}
