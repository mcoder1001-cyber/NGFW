package ip6nd

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip6_nd"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ProxyNdName is the descriptor name; keys are "ip6-nd.proxy/<interface>/<address>".
const ProxyNdName = "ip6-nd.proxy"

// ProxyNdDescriptor manages proxied ND addresses: ip6nd_proxy_enable_disable on the interface
// plus ip6nd_proxy_add_del per address. The interface feature is switched off again when the
// last proxied address of the interface is deleted.
type ProxyNdDescriptor struct {
	client vpp.Client
	owner  string
}

// NewProxyNd returns the descriptor for the given owner.
func NewProxyNd(c vpp.Client, owner string) *ProxyNdDescriptor {
	return &ProxyNdDescriptor{client: c, owner: owner}
}

// ProxyNdMeta is the runtime handle.
type ProxyNdMeta struct{ SwIfIndex uint32 }

// Name implements scheduler.Descriptor.
func (*ProxyNdDescriptor) Name() string { return ProxyNdName }

// KeyOf implements scheduler.Descriptor.
func (*ProxyNdDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	p := obj.(*ProxyNd)
	a := p.GetAddress()
	if parsed, err := df2.ParseAddr(a); err == nil {
		a = parsed.String()
	}
	return scheduler.Join(ProxyNdName, p.GetInterface(), a)
}

// Dependencies implements scheduler.Descriptor.
func (*ProxyNdDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{df2.InterfaceDep(obj.(*ProxyNd).GetInterface())}
}

func (d *ProxyNdDescriptor) addDel(ctx context.Context, p *ProxyNd, idx interface_types.InterfaceIndex, isAdd bool) error {
	a, err := df2.ParseAddr(p.GetAddress())
	if err != nil {
		return err
	}
	ip6, err := df2.ToIP6(a)
	if err != nil {
		return fmt.Errorf("%s: %w", ProxyNdName, err)
	}
	svc := ip6_nd.NewServiceClient(d.client)
	if isAdd {
		if _, err := svc.IP6ndProxyEnableDisable(ctx, &ip6_nd.IP6ndProxyEnableDisable{SwIfIndex: idx, IsEnable: true}); err != nil {
			return fmt.Errorf("ip6nd_proxy_enable_disable: %w", err)
		}
	}
	if _, err := svc.IP6ndProxyAddDel(ctx, &ip6_nd.IP6ndProxyAddDel{SwIfIndex: idx, IsAdd: isAdd, IP: ip6}); err != nil {
		return fmt.Errorf("ip6nd_proxy_add_del: %w", err)
	}
	if !isAdd {
		remaining, err := d.dump(ctx)
		if err != nil {
			return err
		}
		for _, r := range remaining {
			if r.SwIfIndex == idx {
				return nil // other proxied addresses keep the feature on
			}
		}
		if _, err := svc.IP6ndProxyEnableDisable(ctx, &ip6_nd.IP6ndProxyEnableDisable{SwIfIndex: idx, IsEnable: false}); err != nil {
			return fmt.Errorf("ip6nd_proxy_enable_disable: %w", err)
		}
	}
	return nil
}

func (d *ProxyNdDescriptor) dump(ctx context.Context) ([]*ip6_nd.IP6ndProxyDetails, error) {
	stream, err := ip6_nd.NewServiceClient(d.client).IP6ndProxyDump(ctx, &ip6_nd.IP6ndProxyDump{})
	if err != nil {
		return nil, fmt.Errorf("ip6nd_proxy_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("ip6nd_proxy_dump: %w", err)
	}
	return details, nil
}

// Create implements scheduler.Descriptor.
func (d *ProxyNdDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	p := obj.(*ProxyNd)
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Index(p.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, p, idx, true); err != nil {
		return nil, err
	}
	return ProxyNdMeta{SwIfIndex: uint32(idx)}, nil
}

// Update implements scheduler.Descriptor: both fields are the key.
func (*ProxyNdDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *ProxyNdDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(ProxyNdMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", ProxyNdName, df2.ErrBadMeta, meta)
	}
	return d.addDel(ctx, obj.(*ProxyNd), interface_types.InterfaceIndex(m.SwIfIndex), false)
}

// Retrieve lists proxied addresses (ip6nd_proxy_dump) on owned interfaces.
func (d *ProxyNdDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	details, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, det := range details {
		name, ok := ifs.OwnedName(uint32(det.SwIfIndex))
		if !ok {
			continue
		}
		v := &ProxyNd{Interface: name, Address: det.IP.ToIP().String()}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: ProxyNdMeta{SwIfIndex: uint32(det.SwIfIndex)}})
	}
	return out, nil
}
