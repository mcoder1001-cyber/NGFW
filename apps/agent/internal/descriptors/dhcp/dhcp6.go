package dhcp

import (
	"context"
	"fmt"
	"net/netip"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/dhcp"
	"ngfw/agent/binapi/dhcp6_ia_na_client_cp"
	"ngfw/agent/binapi/dhcp6_pd_client_cp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// The DHCPv6 client objects have no dump in VPP 26.06 (dhcp6_ia_na_client_cp.api,
// dhcp6_pd_client_cp.api and dhcp.api define only enable/disable and event messages): their
// Retrieve returns ErrRetrieveUnsupported and the reconciler treats them as write-only (D-063).
// Every Create is idempotent: VPP accepts a repeated enable with the same arguments, and a
// duplicate prefix address (DUPLICATE_IF_ADDRESS) counts as success.

// IfaceMeta is the Meta of the interface-bound DHCPv6 objects.
type IfaceMeta struct {
	SwIfIndex uint32
}

func ifaceIndex(ctx context.Context, c vpp.Client, name, owner string, meta any) (uint32, error) {
	if m, ok := meta.(IfaceMeta); ok {
		return m.SwIfIndex, nil
	}
	return dfkit.ResolveInterface(ctx, c, name, owner)
}

// ---- dhcp.dhcp6-client ----------------------------------------------------------------------

// DHCP6ClientDescriptor manages dhcp.dhcp6-client objects: key dhcp.dhcp6-client/<interface>.
type DHCP6ClientDescriptor struct {
	client vpp.Client
	owner  string
	o      options
}

var _ scheduler.Descriptor = (*DHCP6ClientDescriptor)(nil)

// NewDHCP6Client returns the dhcp.dhcp6-client descriptor.
func NewDHCP6Client(client vpp.Client, owner string, opts ...Option) *DHCP6ClientDescriptor {
	return &DHCP6ClientDescriptor{client: client, owner: owner, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*DHCP6ClientDescriptor) Name() string { return NameDHCP6Client }

// KeyOf implements scheduler.Descriptor.
func (d *DHCP6ClientDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[DHCP6Client](obj)
	if err != nil {
		return scheduler.Join(NameDHCP6Client, "invalid")
	}
	return scheduler.Join(NameDHCP6Client, s.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *DHCP6ClientDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[DHCP6Client](obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{d.o.ifaceDep(s.Interface)}
}

func (d *DHCP6ClientDescriptor) set(ctx context.Context, obj proto.Message, meta any, enable bool) (any, error) {
	s, err := decode[DHCP6Client](obj)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	idx, err := ifaceIndex(ctx, d.client, s.Interface, d.owner, meta)
	if err != nil {
		return nil, err
	}
	_, err = dhcp6_ia_na_client_cp.NewServiceClient(d.client).DHCP6ClientEnableDisable(ctx,
		&dhcp6_ia_na_client_cp.DHCP6ClientEnableDisable{SwIfIndex: interface_types.InterfaceIndex(idx), Enable: enable})
	if err != nil {
		return nil, fmt.Errorf("dhcp6_client_enable_disable(%s, enable=%t): %w", s.Interface, enable, err)
	}
	return IfaceMeta{SwIfIndex: idx}, nil
}

// Create implements scheduler.Descriptor (idempotent).
func (d *DHCP6ClientDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return d.set(ctx, obj, nil, true)
}

// Update implements scheduler.Descriptor: the object has no field besides its key; re-apply.
func (d *DHCP6ClientDescriptor) Update(ctx context.Context, _, newObj proto.Message, meta any) (any, error) {
	return d.set(ctx, newObj, meta, true)
}

// Delete implements scheduler.Descriptor.
func (d *DHCP6ClientDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	_, err := d.set(ctx, obj, meta, false)
	if dfkit.IsVPPError(err, api.INVALID_SW_IF_INDEX) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: VPP has no dump (write-only, D-063).
func (d *DHCP6ClientDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameDHCP6Client)
}

// ---- dhcp.dhcp6-pd-client -------------------------------------------------------------------

// DHCP6PDClientDescriptor manages dhcp.dhcp6-pd-client objects: key
// dhcp.dhcp6-pd-client/<interface>.
type DHCP6PDClientDescriptor struct {
	client vpp.Client
	owner  string
	o      options
}

var _ scheduler.Descriptor = (*DHCP6PDClientDescriptor)(nil)

// NewDHCP6PDClient returns the dhcp.dhcp6-pd-client descriptor.
func NewDHCP6PDClient(client vpp.Client, owner string, opts ...Option) *DHCP6PDClientDescriptor {
	return &DHCP6PDClientDescriptor{client: client, owner: owner, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*DHCP6PDClientDescriptor) Name() string { return NameDHCP6PDClient }

// KeyOf implements scheduler.Descriptor.
func (d *DHCP6PDClientDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[DHCP6PDClient](obj)
	if err != nil {
		return scheduler.Join(NameDHCP6PDClient, "invalid")
	}
	return scheduler.Join(NameDHCP6PDClient, s.Interface)
}

// Dependencies implements scheduler.Descriptor: the interface.
func (d *DHCP6PDClientDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[DHCP6PDClient](obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{d.o.ifaceDep(s.Interface)}
}

func (d *DHCP6PDClientDescriptor) set(ctx context.Context, obj proto.Message, meta any, enable bool) (any, error) {
	s, err := decode[DHCP6PDClient](obj)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	idx, err := ifaceIndex(ctx, d.client, s.Interface, d.owner, meta)
	if err != nil {
		return nil, err
	}
	_, err = dhcp6_pd_client_cp.NewServiceClient(d.client).DHCP6PdClientEnableDisable(ctx,
		&dhcp6_pd_client_cp.DHCP6PdClientEnableDisable{
			SwIfIndex: interface_types.InterfaceIndex(idx), PrefixGroup: s.PrefixGroup, Enable: enable,
		})
	if err != nil {
		return nil, fmt.Errorf("dhcp6_pd_client_enable_disable(%s, group %q, enable=%t): %w", s.Interface, s.PrefixGroup, enable, err)
	}
	return IfaceMeta{SwIfIndex: idx}, nil
}

// Create implements scheduler.Descriptor (idempotent for the same prefix group; VPP rejects a
// different group on an enabled interface with INVALID_VALUE).
func (d *DHCP6PDClientDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return d.set(ctx, obj, nil, true)
}

// Update implements scheduler.Descriptor: a new prefix group needs disable + enable — recreate.
func (d *DHCP6PDClientDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *DHCP6PDClientDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	_, err := d.set(ctx, obj, meta, false)
	if dfkit.IsVPPError(err, api.INVALID_SW_IF_INDEX) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: VPP has no dump (write-only, D-063).
func (d *DHCP6PDClientDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameDHCP6PDClient)
}

// ---- dhcp.dhcp6-pd-address ------------------------------------------------------------------

// DHCP6PDAddressDescriptor manages dhcp.dhcp6-pd-address objects: key
// dhcp.dhcp6-pd-address/<interface>/<prefix group>/<address>/<len>.
type DHCP6PDAddressDescriptor struct {
	client vpp.Client
	owner  string
	o      options
}

var _ scheduler.Descriptor = (*DHCP6PDAddressDescriptor)(nil)

// NewDHCP6PDAddress returns the dhcp.dhcp6-pd-address descriptor.
func NewDHCP6PDAddress(client vpp.Client, owner string, opts ...Option) *DHCP6PDAddressDescriptor {
	return &DHCP6PDAddressDescriptor{client: client, owner: owner, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*DHCP6PDAddressDescriptor) Name() string { return NameDHCP6PDAddr }

// KeyOf implements scheduler.Descriptor.
func (d *DHCP6PDAddressDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s, err := decode[DHCP6PDAddress](obj)
	if err != nil {
		return scheduler.Join(NameDHCP6PDAddr, "invalid")
	}
	return scheduler.Join(NameDHCP6PDAddr, s.Interface, s.PrefixGroup, s.Address)
}

// Dependencies implements scheduler.Descriptor: the interface and a PD client that feeds the
// prefix group. The client is looked up on the same interface (the usual "delegate on WAN,
// number the WAN" case is covered; a group fed from another interface is ordered by the
// scheduler's registration order — dhcp6-pd-client registers first).
func (d *DHCP6PDAddressDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	s, err := decode[DHCP6PDAddress](obj)
	if err != nil {
		return nil
	}
	return []scheduler.Dependency{
		d.o.ifaceDep(s.Interface),
		{Key: scheduler.Join(NameDHCP6PDClient, s.Interface), Optional: true},
	}
}

func parseAddrLen(s string) (netip.Addr, int, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Addr{}, 0, dfkit.Specf("address %q: %v", s, err)
	}
	return p.Addr(), p.Bits(), nil
}

func (d *DHCP6PDAddressDescriptor) set(ctx context.Context, obj proto.Message, meta any, add bool) (any, error) {
	s, err := decode[DHCP6PDAddress](obj)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	idx, err := ifaceIndex(ctx, d.client, s.Interface, d.owner, meta)
	if err != nil {
		return nil, err
	}
	a, l, _ := parseAddrLen(s.Address)
	_, err = dhcp6_pd_client_cp.NewServiceClient(d.client).IP6AddDelAddressUsingPrefix(ctx,
		&dhcp6_pd_client_cp.IP6AddDelAddressUsingPrefix{
			SwIfIndex:         interface_types.InterfaceIndex(idx),
			PrefixGroup:       s.PrefixGroup,
			AddressWithPrefix: ip_types.IP6AddressWithPrefix{Address: ip_types.IP6Address(a.As16()), Len: uint8(l)}, //nolint:gosec // 0..128
			IsAdd:             add,
		})
	if err != nil {
		return nil, fmt.Errorf("ip6_add_del_address_using_prefix(%s, %q, %s, add=%t): %w", s.Interface, s.PrefixGroup, s.Address, add, err)
	}
	return IfaceMeta{SwIfIndex: idx}, nil
}

// Create implements scheduler.Descriptor; an existing identical entry (DUPLICATE_IF_ADDRESS) is success.
func (d *DHCP6PDAddressDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	meta, err := d.set(ctx, obj, nil, true)
	if dfkit.IsVPPError(err, api.DUPLICATE_IF_ADDRESS) {
		s, _ := decode[DHCP6PDAddress](obj)
		idx, rerr := dfkit.ResolveInterface(ctx, d.client, s.Interface, d.owner)
		if rerr != nil {
			return nil, rerr
		}
		return IfaceMeta{SwIfIndex: idx}, nil
	}
	return meta, err
}

// Update implements scheduler.Descriptor: every field is part of the key; never called with a
// different value, re-apply for safety.
func (d *DHCP6PDAddressDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor.
func (d *DHCP6PDAddressDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	_, err := d.set(ctx, obj, meta, false)
	if dfkit.IsVPPError(err, api.ADDRESS_NOT_FOUND_FOR_INTERFACE, api.INVALID_VALUE) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: VPP has no dump of prefix-derived addresses (the
// resulting address shows in ip_address_dump only once a prefix is delegated) — write-only.
func (d *DHCP6PDAddressDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameDHCP6PDAddr)
}

// ---- dhcp.dhcp6-duid ------------------------------------------------------------------------

// DHCP6DUIDDescriptor manages the dhcp.dhcp6-duid singleton (key dhcp.dhcp6-duid/global):
// the DUID-LL all DHCPv6 clients of this VPP send. VPP has no getter and no reset: Delete is a
// no-op (the DUID stays until VPP restarts, when VPP derives one from the first interface MAC).
// The DUID is VPP-global (D-071): an agent that is not the globals owner cannot set it and, VPP
// having no getter, cannot verify it either — its Create fails with ErrNotGlobalsOwner.
type DHCP6DUIDDescriptor struct {
	client  vpp.Client
	globals dfkit.Globals
}

var _ scheduler.Descriptor = (*DHCP6DUIDDescriptor)(nil)

// NewDHCP6DUID returns the dhcp.dhcp6-duid descriptor.
func NewDHCP6DUID(client vpp.Client, g dfkit.Globals) *DHCP6DUIDDescriptor {
	return &DHCP6DUIDDescriptor{client: client, globals: g}
}

// KeyDHCP6DUID is the key of the singleton.
var KeyDHCP6DUID = scheduler.Join(NameDHCP6DUID, DUIDID)

// Name implements scheduler.Descriptor.
func (*DHCP6DUIDDescriptor) Name() string { return NameDHCP6DUID }

// KeyOf implements scheduler.Descriptor.
func (*DHCP6DUIDDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyDHCP6DUID }

// Dependencies implements scheduler.Descriptor: none.
func (*DHCP6DUIDDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *DHCP6DUIDDescriptor) set(ctx context.Context, obj proto.Message) error {
	s, err := decode[DHCP6DUID](obj)
	if err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if !d.globals.Owner() {
		return d.globals.Require(ctx, NameDHCP6DUID, obj, nil)
	}
	b, _ := parseDUID(s.DUIDLL)
	if _, err := dhcp.NewServiceClient(d.client).DHCP6DuidLlSet(ctx, &dhcp.DHCP6DuidLlSet{DuidLl: b}); err != nil {
		return fmt.Errorf("dhcp6_duid_ll_set: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor (idempotent).
func (d *DHCP6DUIDDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.set(ctx, obj)
}

// Update implements scheduler.Descriptor.
func (d *DHCP6DUIDDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.set(ctx, newObj)
}

// Delete implements scheduler.Descriptor: no-op (VPP cannot reset the DUID).
func (*DHCP6DUIDDescriptor) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: VPP has no getter (write-only, D-063).
func (*DHCP6DUIDDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, dfkit.RetrieveUnsupported(NameDHCP6DUID)
}

// ---- actions and events ---------------------------------------------------------------------

// WatchDHCP6Replies subscribes this API client to dhcp6_reply_event (want_dhcp6_reply_events)
// and streams the events until ctx is cancelled; the subscription is removed then.
func WatchDHCP6Replies(ctx context.Context, c vpp.Client) (<-chan *dhcp.DHCP6ReplyEvent, error) {
	svc := dhcp.NewServiceClient(c)
	pid := pidSelf()
	w, err := c.WatchEvent(ctx, &dhcp.DHCP6ReplyEvent{})
	if err != nil {
		return nil, fmt.Errorf("watch dhcp6_reply_event: %w", err)
	}
	if _, err := svc.WantDHCP6ReplyEvents(ctx, &dhcp.WantDHCP6ReplyEvents{EnableDisable: 1, PID: pid}); err != nil {
		w.Close()
		return nil, fmt.Errorf("want_dhcp6_reply_events: %w", err)
	}
	out := make(chan *dhcp.DHCP6ReplyEvent, 16)
	go func() {
		defer close(out)
		defer func() {
			_, _ = svc.WantDHCP6ReplyEvents(context.WithoutCancel(ctx), &dhcp.WantDHCP6ReplyEvents{EnableDisable: 0, PID: pid})
		}()
		for m := range w.Events() {
			if ev, ok := m.(*dhcp.DHCP6ReplyEvent); ok {
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// WatchDHCP6PDReplies is WatchDHCP6Replies for dhcp6_pd_reply_event (want_dhcp6_pd_reply_events).
func WatchDHCP6PDReplies(ctx context.Context, c vpp.Client) (<-chan *dhcp.DHCP6PdReplyEvent, error) {
	svc := dhcp.NewServiceClient(c)
	pid := pidSelf()
	w, err := c.WatchEvent(ctx, &dhcp.DHCP6PdReplyEvent{})
	if err != nil {
		return nil, fmt.Errorf("watch dhcp6_pd_reply_event: %w", err)
	}
	if _, err := svc.WantDHCP6PdReplyEvents(ctx, &dhcp.WantDHCP6PdReplyEvents{EnableDisable: true, PID: pid}); err != nil {
		w.Close()
		return nil, fmt.Errorf("want_dhcp6_pd_reply_events: %w", err)
	}
	out := make(chan *dhcp.DHCP6PdReplyEvent, 16)
	go func() {
		defer close(out)
		defer func() {
			_, _ = svc.WantDHCP6PdReplyEvents(context.WithoutCancel(ctx), &dhcp.WantDHCP6PdReplyEvents{EnableDisable: false, PID: pid})
		}()
		for m := range w.Events() {
			if ev, ok := m.(*dhcp.DHCP6PdReplyEvent); ok {
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// SendDHCP6ClientMessage is the dhcp6_send_client_message action helper (manual solicit/renew/
// release — the IA_NA client control plane normally sends these itself).
func SendDHCP6ClientMessage(ctx context.Context, c vpp.Client, msg *dhcp.DHCP6SendClientMessage) error {
	if _, err := dhcp.NewServiceClient(c).DHCP6SendClientMessage(ctx, msg); err != nil {
		return fmt.Errorf("dhcp6_send_client_message: %w", err)
	}
	return nil
}

// SendDHCP6PDClientMessage is the dhcp6_pd_send_client_message action helper.
func SendDHCP6PDClientMessage(ctx context.Context, c vpp.Client, msg *dhcp.DHCP6PdSendClientMessage) error {
	if _, err := dhcp.NewServiceClient(c).DHCP6PdSendClientMessage(ctx, msg); err != nil {
		return fmt.Errorf("dhcp6_pd_send_client_message: %w", err)
	}
	return nil
}
