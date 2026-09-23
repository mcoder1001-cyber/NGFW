package dhcp

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ClientMeta is the Meta of a dhcp.client object.
type ClientMeta struct {
	SwIfIndex uint32
}

// ClientDescriptor manages dhcp.client objects: key dhcp.client/<interface> (logical name, D-069).
// The interface is this owner's tagged interface or an untagged one (claimed on Create, D-071).
type ClientDescriptor struct {
	client vpp.Client
	owner  string
	o      options
}

var _ scheduler.Descriptor = (*ClientDescriptor)(nil)

// NewClient returns the dhcp.client descriptor.
func NewClient(client vpp.Client, owner string, opts ...Option) *ClientDescriptor {
	return &ClientDescriptor{client: client, owner: owner, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ClientDescriptor) Name() string { return NameClient }

// KeyOf implements scheduler.Descriptor.
func (d *ClientDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[Client](obj)
	if err != nil {
		return scheduler.Join(NameClient, "invalid")
	}
	return scheduler.Join(NameClient, s.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface (D-065 alias key).
func (d *ClientDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[Client](obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{d.o.ifaceDep(s.Interface)}
}

func (d *ClientDescriptor) config(ctx context.Context, s Client, swIfIndex uint32, add bool) error {
	_, err := dhcp.NewServiceClient(d.client).DHCPClientConfig(ctx, &dhcp.DHCPClientConfig{
		IsAdd: add,
		Client: dhcp.DHCPClient{
			SwIfIndex:        interface_types.InterfaceIndex(swIfIndex),
			Hostname:         s.Hostname,
			ID:               []byte(s.ClientID),
			WantDHCPEvent:    s.WantEvents,
			SetBroadcastFlag: s.SetBroadcastFlag,
			Dscp:             ip_types.IPDscp(s.DSCP),
			PID:              pidSelf(),
		},
	})
	if err != nil {
		return fmt.Errorf("dhcp_client_config(is_add=%t, %s): %w", add, s.Interface, err)
	}
	return nil
}

// Create implements scheduler.Descriptor. VPP answers INVALID_VALUE when a client already exists
// on the interface; Create then compares with the dump and succeeds only if it is identical.
func (d *ClientDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s, err := decode[Client](obj)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	idx, err := dfkit.ResolveAndClaim(ctx, d.client, s.Interface, d.owner, NameClient)
	if err != nil {
		return nil, err
	}
	if err := d.config(ctx, s, idx, true); err != nil {
		if dfkit.IsVPPError(err, api.INVALID_VALUE) {
			if cur, ok, rerr := d.retrieveOne(ctx, idx); rerr == nil && ok && proto.Equal(cur.Proto(), s.Proto()) {
				return ClientMeta{SwIfIndex: idx}, nil
			}
		}
		return nil, err
	}
	return ClientMeta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: VPP has no in-place change of a client — recreate.
func (d *ClientDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. The hostname is sent again because VPP copies it with
// strlen on delete too.
func (d *ClientDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	s, err := decode[Client](obj)
	if err != nil {
		return err
	}
	m, ok := meta.(ClientMeta)
	if !ok {
		idx, rerr := dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner)
		if errors.Is(rerr, dfkit.ErrNoInterface) { // the interface and with it the client are gone
			return dfkit.Claims(d.owner).Release(s.Interface, NameClient)
		}
		if rerr != nil {
			return fmt.Errorf("%w: %T (%v)", dfkit.ErrBadMeta, meta, rerr)
		}
		m = ClientMeta{SwIfIndex: idx}
	}
	if s.Hostname == "" {
		s.Hostname = "vrx"
	}
	// D-074: delete only what still exists (and is still on the same interface)
	if _, exists, rerr := d.retrieveOne(ctx, m.SwIfIndex); rerr != nil {
		return rerr
	} else if !exists {
		return dfkit.Claims(d.owner).Release(s.Interface, NameClient)
	}
	err = d.config(ctx, s, m.SwIfIndex, false)
	if err != nil && !dfkit.IsVPPError(err, api.INVALID_VALUE, api.INVALID_SW_IF_INDEX) { // not enabled / interface gone
		return err
	}
	return dfkit.Claims(d.owner).Release(s.Interface, NameClient)
}

func clientFromDetails(c dhcp.DHCPClient, ifName string) Client {
	id := string(c.ID)
	if i := strings.IndexByte(id, 0); i >= 0 {
		id = id[:i]
	}
	return Client{
		Interface:        ifName,
		Hostname:         strings.TrimRight(c.Hostname, "\x00"),
		ClientID:         id,
		SetBroadcastFlag: c.SetBroadcastFlag,
		DSCP:             uint8(c.Dscp),
		WantEvents:       c.WantDHCPEvent,
	}
}

func (d *ClientDescriptor) dump(ctx context.Context) ([]*dhcp.DHCPClientDetails, error) {
	stream, err := dhcp.NewServiceClient(d.client).DHCPClientDump(ctx, &dhcp.DHCPClientDump{})
	if err != nil {
		return nil, fmt.Errorf("dhcp_client_dump: %w", err)
	}
	details, err := dfkit.Drain(stream, stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("dhcp_client_dump: %w", err)
	}
	return details, nil
}

func (d *ClientDescriptor) retrieveOne(ctx context.Context, idx uint32) (Client, bool, error) {
	details, err := d.dump(ctx)
	if err != nil {
		return Client{}, false, err
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client)
	if err != nil {
		return Client{}, false, err
	}
	for _, det := range details {
		if uint32(det.Client.SwIfIndex) == idx {
			name, _, _ := ifaces.Logical(idx, d.owner)
			return clientFromDetails(det.Client, name), true, nil
		}
	}
	return Client{}, false, nil
}

// Retrieve implements scheduler.Descriptor: dhcp_client_dump, clients on owned interfaces.
func (d *ClientDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	details, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	if len(details) == 0 {
		return nil, nil
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client)
	if err != nil {
		return nil, err
	}
	var out []scheduler.KV
	for _, det := range details {
		idx := uint32(det.Client.SwIfIndex)
		name, ok := ifaces.Reportable(idx, d.owner, NameClient)
		if !ok {
			continue
		}
		s := clientFromDetails(det.Client, name)
		out = append(out, scheduler.KV{Key: scheduler.Join(NameClient, name), Value: s.Proto(), Meta: ClientMeta{SwIfIndex: idx}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Lease is the read-only DHCPv4 lease state of a client (dhcp_client_details.lease and
// dhcp_compl_event). It is status, never part of the desired Value.
type Lease struct {
	SwIfIndex   uint32
	State       string // DISCOVER | REQUEST | BOUND
	Hostname    string
	Address     netip.Prefix // leased address / mask width (invalid until bound)
	Router      netip.Addr
	DNSServers  []netip.Addr
	HostMAC     string
	FromEventOf uint32 // PID the event was addressed to (0 for dump-derived leases)
}

func leaseFrom(l dhcp.DHCPLease, pid uint32) Lease {
	st := strings.TrimPrefix(dhcp.DHCPClientState(l.State).String(), "DHCP_CLIENT_STATE_API_")
	out := Lease{
		SwIfIndex:   uint32(l.SwIfIndex),
		State:       st,
		Hostname:    strings.TrimRight(l.Hostname, "\x00"),
		Router:      netip.AddrFrom4(l.RouterAddress.Un.GetIP4()),
		HostMAC:     l.HostMac.String(),
		FromEventOf: pid,
	}
	if a := netip.AddrFrom4(l.HostAddress.Un.GetIP4()); !a.IsUnspecified() && l.MaskWidth <= 32 {
		out.Address = netip.PrefixFrom(a, int(l.MaskWidth))
	}
	for _, s := range l.DomainServer {
		out.DNSServers = append(out.DNSServers, netip.AddrFrom4(s.Address.Un.GetIP4()))
	}
	return out
}

// Leases returns the current lease state of every client on an owned interface, keyed by
// interface name (state RPCs).
func (d *ClientDescriptor) Leases(ctx context.Context) (map[string]Lease, error) {
	details, err := d.dump(ctx)
	if err != nil {
		return nil, err
	}
	ifaces, err := dfkit.DumpInterfaces(ctx, d.client)
	if err != nil {
		return nil, err
	}
	out := map[string]Lease{}
	for _, det := range details {
		if name, ok := ifaces.Reportable(uint32(det.Client.SwIfIndex), d.owner, NameClient); ok {
			out[name] = leaseFrom(det.Lease, 0)
		}
	}
	return out, nil
}

// WatchLeases streams dhcp_compl_event (sent by VPP to the API client that configured a client
// with WantEvents) as Lease values until ctx is cancelled. It is the StreamEvents source for
// DHCPv4 lease changes.
func WatchLeases(ctx context.Context, c vpp.Client) (<-chan Lease, error) {
	w, err := c.WatchEvent(ctx, &dhcp.DHCPComplEvent{})
	if err != nil {
		return nil, fmt.Errorf("watch dhcp_compl_event: %w", err)
	}
	out := make(chan Lease, 16)
	go func() {
		defer close(out)
		for m := range w.Events() {
			ev, ok := m.(*dhcp.DHCPComplEvent)
			if !ok {
				continue
			}
			select {
			case out <- leaseFrom(ev.Lease, ev.PID):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
