// Package ipneighbor implements the descriptors of VPP's ip_neighbor API: static ARP/ND
// entries and the per-family neighbour database configuration. Messages come from
// apps/agent/binapi/ip_neighbor only.
package ipneighbor

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_neighbor"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NeighborName is the descriptor name; keys are "ip-neighbor.neighbor/<interface>/<ip>".
const NeighborName = "ip-neighbor.neighbor"

// NeighborDescriptor manages static neighbours (ip_neighbor_add_del) on interfaces owned by
// this agent. Learned (dynamic) entries are never retrieved.
type NeighborDescriptor struct {
	client vpp.Client
	owner  string
	opts   df2.Options
}

// NewNeighbor returns the descriptor for the given owner; df2.WithClaims sets the store that
// attributes neighbours on untagged (physical) interfaces.
func NewNeighbor(c vpp.Client, owner string, opts ...df2.Option) *NeighborDescriptor {
	return &NeighborDescriptor{client: c, owner: owner, opts: df2.BuildOptions(opts...)}
}

// NeighborMeta is the runtime handle: the sw_if_index the entry lives on.
type NeighborMeta struct{ SwIfIndex uint32 }

// Name implements scheduler.Descriptor.
func (*NeighborDescriptor) Name() string { return NeighborName }

// KeyOf implements scheduler.Descriptor.
func (*NeighborDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	n := obj.(*Neighbor)
	ip := n.GetIpAddress()
	if a, err := df2.ParseAddr(ip); err == nil {
		ip = a.String()
	}
	return scheduler.Join(NeighborName, n.GetInterface(), ip)
}

// Dependencies implements scheduler.Descriptor: the interface must exist.
func (*NeighborDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{df2.InterfaceDep(obj.(*Neighbor).GetInterface())}
}

func (d *NeighborDescriptor) encode(n *Neighbor, swIfIndex interface_types.InterfaceIndex) (ip_neighbor.IPNeighbor, error) {
	a, err := df2.ParseAddr(n.GetIpAddress())
	if err != nil {
		return ip_neighbor.IPNeighbor{}, err
	}
	mac, err := df2.ParseMAC(n.GetMacAddress())
	if err != nil {
		return ip_neighbor.IPNeighbor{}, err
	}
	flags := ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC
	if n.GetNoFibEntry() {
		flags |= ip_neighbor.IP_API_NEIGHBOR_FLAG_NO_FIB_ENTRY
	}
	return ip_neighbor.IPNeighbor{SwIfIndex: swIfIndex, Flags: flags, MacAddress: mac, IPAddress: df2.ToAddress(a)}, nil
}

func (d *NeighborDescriptor) addDel(ctx context.Context, n *Neighbor, swIfIndex interface_types.InterfaceIndex, isAdd bool) error {
	nb, err := d.encode(n, swIfIndex)
	if err != nil {
		return err
	}
	if _, err := ip_neighbor.NewServiceClient(d.client).IPNeighborAddDel(ctx, &ip_neighbor.IPNeighborAddDel{IsAdd: isAdd, Neighbor: nb}); err != nil {
		return fmt.Errorf("ip_neighbor_add_del: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *NeighborDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	n := obj.(*Neighbor)
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	idx, untagged, err := ifs.Resolve(n.GetInterface())
	if err != nil {
		return nil, err
	}
	if err := d.addDel(ctx, n, interface_types.InterfaceIndex(idx), true); err != nil {
		return nil, err
	}
	if err := df2.Claim(d.opts.Claims, untagged, d.KeyOf(n)); err != nil {
		return nil, err
	}
	return NeighborMeta{SwIfIndex: idx}, nil
}

// Update changes the MAC in place (VPP replaces the entry for the same interface + address);
// a different interface, address or flag set is a different entry.
func (d *NeighborDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	o, n := oldObj.(*Neighbor), newObj.(*Neighbor)
	if d.KeyOf(o) != d.KeyOf(n) || o.GetNoFibEntry() != n.GetNoFibEntry() {
		return nil, scheduler.ErrRecreate
	}
	m, ok := meta.(NeighborMeta)
	if !ok {
		return nil, fmt.Errorf("%s: %w %T", NeighborName, df2.ErrBadMeta, meta)
	}
	if err := d.addDel(ctx, n, interface_types.InterfaceIndex(m.SwIfIndex), true); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete implements scheduler.Descriptor.
func (d *NeighborDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	m, ok := meta.(NeighborMeta)
	if !ok {
		return fmt.Errorf("%s: %w %T", NeighborName, df2.ErrBadMeta, meta)
	}
	if err := d.addDel(ctx, obj.(*Neighbor), interface_types.InterfaceIndex(m.SwIfIndex), false); err != nil {
		return err
	}
	return df2.Release(d.opts.Claims, d.KeyOf(obj))
}

// Retrieve dumps both address families and keeps the static entries on interfaces tagged by
// this owner and claimed entries on untagged interfaces.
func (d *NeighborDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	ifs, err := df2.DumpInterfaces(ctx, d.client, d.owner)
	if err != nil {
		return nil, err
	}
	svc := ip_neighbor.NewServiceClient(d.client)
	var out []scheduler.KV
	for _, af := range []ip_types.AddressFamily{ip_types.ADDRESS_IP4, ip_types.ADDRESS_IP6} {
		stream, err := svc.IPNeighborDump(ctx, &ip_neighbor.IPNeighborDump{SwIfIndex: interface_types.InterfaceIndex(df2.NoInterface), Af: af})
		if err != nil {
			return nil, fmt.Errorf("ip_neighbor_dump: %w", err)
		}
		details, err := df2.Collect(stream.Recv)
		if err != nil {
			return nil, fmt.Errorf("ip_neighbor_dump: %w", err)
		}
		for _, det := range details {
			nb := det.Neighbor
			if nb.Flags&ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC == 0 {
				continue // learned entry: never desired state
			}
			name, ok := ifs.Name(uint32(nb.SwIfIndex))
			if !ok {
				continue
			}
			v := &Neighbor{
				Interface:  name,
				IpAddress:  df2.FromAddress(nb.IPAddress).String(),
				MacAddress: df2.MACString(nb.MacAddress),
				NoFibEntry: nb.Flags&ip_neighbor.IP_API_NEIGHBOR_FLAG_NO_FIB_ENTRY != 0,
			}
			if !ifs.OwnsObject(uint32(nb.SwIfIndex), d.KeyOf(v), d.opts.Claims) {
				continue // another owner's interface, or an untagged one we did not configure
			}
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: NeighborMeta{SwIfIndex: uint32(nb.SwIfIndex)}})
		}
	}
	return out, nil
}
