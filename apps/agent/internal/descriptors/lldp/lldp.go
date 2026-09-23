// Package lldp holds the reconciler descriptors for the VPP LLDP plugin (task DF-7, WBS D1.7):
// the global settings (lldp_config: system name, tx hold, tx interval) and LLDP on an
// interface (sw_interface_set_lldp: port description, management addresses/OID).
//
// Both are write-only (D-063): VPP 26.06 has no dump of the LLDP configuration. lldp_dump
// reports the per-interface neighbour table (chassis/port id heard, ttl), which Neighbours
// returns as read-only state and the interface descriptor uses to verify a Create — it cannot
// rebuild port-desc or the management addresses, and D-063 forbids echoing desired state.
//
// Messages come only from apps/agent/binapi/lldp. docs/agent/descriptors/lldp.md is the
// object ↔ message table and lists the VPP quirks (sw_if_index used as hw_if_index).
package lldp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/lldp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names.
const (
	NameGlobal    = "lldp.global"
	NameInterface = "lldp.interface"
)

// GlobalID is the object id of the singleton (key lldp.global/global).
const GlobalID = "global"

// Defaults of VPP 26.06 (lldp_input.c, IEEE 802.1AB-2009) restored by Delete.
const (
	DefaultTxHold     = 4
	DefaultTxInterval = 30
)

// Global is the desired state of the lldp.global singleton. An empty SystemName leaves VPP's
// current system name unchanged (VPP ignores an empty name; it cannot be unset over the API).
type Global struct {
	SystemName string `json:"system_name,omitempty"`
	TxHold     uint32 `json:"tx_hold,omitempty"`
	TxInterval uint32 `json:"tx_interval,omitempty"`
}

// Validate checks g against VPP's bounds (tx hold 1–100, tx interval 1–3600; 0 = keep).
func (g Global) Validate() error {
	if g.TxHold > 100 {
		return df7.Specf("lldp tx_hold %d exceeds 100", g.TxHold)
	}
	if g.TxInterval > 3600 {
		return df7.Specf("lldp tx_interval %d exceeds 3600", g.TxInterval)
	}
	return nil
}

// Interface is the desired state of one lldp.interface object.
type Interface struct {
	Interface string `json:"interface,omitempty"`
	PortDesc  string `json:"port_desc,omitempty"`
	MgmtIP4   string `json:"mgmt_ip4,omitempty"`
	MgmtIP6   string `json:"mgmt_ip6,omitempty"`
	MgmtOID   string `json:"mgmt_oid,omitempty"`
}

// Validate checks i.
func (i Interface) Validate() error {
	if i.Interface == "" {
		return df7.Specf("lldp interface needs an interface")
	}
	if i.MgmtIP4 != "" {
		a, err := df7.ParseAddr(i.MgmtIP4)
		if err != nil || !a.Is4() || a.String() != i.MgmtIP4 {
			return df7.Specf("mgmt_ip4 %q is not a canonical IPv4 address", i.MgmtIP4)
		}
	}
	if i.MgmtIP6 != "" {
		a, err := df7.ParseAddr(i.MgmtIP6)
		if err != nil || !a.Is6() || a.String() != i.MgmtIP6 {
			return df7.Specf("mgmt_ip6 %q is not a canonical IPv6 address", i.MgmtIP6)
		}
	}
	if len(i.MgmtOID) > 127 {
		return df7.Specf("mgmt_oid longer than 127 bytes")
	}
	return nil
}

// ---- lldp.global ------------------------------------------------------------------------------

// GlobalDescriptor manages the lldp.global singleton with lldp_config. Write-only (D-063).
type GlobalDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*GlobalDescriptor)(nil)

// NewGlobal returns the lldp.global descriptor.
func NewGlobal(c vpp.Client, owner string, opts ...df7.Option) *GlobalDescriptor {
	return &GlobalDescriptor{df7.NewBase(NameGlobal, c, owner, opts)}
}

// KeyGlobal is "lldp.global/global".
func KeyGlobal() scheduler.Key { return scheduler.Join(NameGlobal, GlobalID) }

// KeyOf implements scheduler.Descriptor.
func (*GlobalDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyGlobal() }

// Dependencies implements scheduler.Descriptor: none.
func (*GlobalDescriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }

func (d *GlobalDescriptor) set(ctx context.Context, g Global) error {
	_, err := lldp.NewServiceClient(d.Client).LldpConfig(ctx, &lldp.LldpConfig{TxHold: g.TxHold, TxInterval: g.TxInterval, SystemName: g.SystemName})
	return d.Wrap("lldp_config", df7.PluginError("lldp", err))
}

// Create implements scheduler.Descriptor (idempotent).
func (d *GlobalDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	g, err := df7.DecodeValid[Global](obj)
	if err != nil {
		return nil, err
	}
	return nil, d.set(ctx, g)
}

// Update implements scheduler.Descriptor: lldp_config in place.
func (d *GlobalDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: restores the timer defaults; the system name stays
// (the API cannot unset it).
func (d *GlobalDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	return d.set(ctx, Global{TxHold: DefaultTxHold, TxInterval: DefaultTxInterval})
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *GlobalDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameGlobal, "VPP 26.06 has no getter for lldp_config")
}

// ---- lldp.interface ---------------------------------------------------------------------------

// ErrIndexMismatch is returned when VPP enabled LLDP on another interface than asked:
// sw_interface_set_lldp passes the sw_if_index to lldp_cfg_intf_set, which treats it as a
// hw_if_index (VPP 26.06 lldp_api.c / lldp_cli.c). The two only coincide for interfaces whose
// software and hardware indexes are equal.
var ErrIndexMismatch = errors.New("lldp: VPP enabled LLDP on another interface (sw_if_index ≠ hw_if_index)")

// Meta of lldp.interface: the interface index.
type Meta struct{ SwIfIndex uint32 }

// KeyInterface is "lldp.interface/<interface>".
func KeyInterface(ifName string) scheduler.Key { return scheduler.Join(NameInterface, ifName) }

// InterfaceDescriptor manages lldp.interface objects with sw_interface_set_lldp. Write-only
// (D-063); Create verifies through lldp_dump that VPP enabled LLDP on the requested
// interface (ErrIndexMismatch otherwise).
type InterfaceDescriptor struct{ df7.Base }

var _ scheduler.Descriptor = (*InterfaceDescriptor)(nil)

// NewInterface returns the lldp.interface descriptor.
func NewInterface(c vpp.Client, owner string, opts ...df7.Option) *InterfaceDescriptor {
	return &InterfaceDescriptor{df7.NewBase(NameInterface, c, owner, opts)}
}

// KeyOf implements scheduler.Descriptor.
func (d *InterfaceDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	i, _ := df7.Decode[Interface](obj)
	return KeyInterface(i.Interface)
}

// Dependencies implements scheduler.Descriptor: the global settings (optional) and the interface.
func (d *InterfaceDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	i, _ := df7.Decode[Interface](obj)
	return []scheduler.Dependency{{Key: KeyGlobal(), Optional: true}, d.Opts.IfaceDep(i.Interface)}
}

func (d *InterfaceDescriptor) set(ctx context.Context, idx uint32, i Interface, enable bool) error {
	req := &lldp.SwInterfaceSetLldp{SwIfIndex: interface_types.InterfaceIndex(idx), Enable: enable, PortDesc: i.PortDesc, MgmtOid: make([]byte, 128)}
	if i.MgmtIP4 != "" {
		a := netip.MustParseAddr(i.MgmtIP4)
		req.MgmtIP4 = ip_types.IP4Address(a.As4())
	}
	if i.MgmtIP6 != "" {
		a := netip.MustParseAddr(i.MgmtIP6)
		req.MgmtIP6 = ip_types.IP6Address(a.As16())
	}
	copy(req.MgmtOid, i.MgmtOID)
	_, err := lldp.NewServiceClient(d.Client).SwInterfaceSetLldp(ctx, req)
	return d.Wrap(fmt.Sprintf("sw_interface_set_lldp %s (%d) enable=%v", i.Interface, idx, enable), df7.PluginError("lldp", err))
}

// Create implements scheduler.Descriptor: enable, then verify with lldp_dump. Idempotent: VPP
// ignores an enable of an interface that already has LLDP (configuration changes go through
// Update).
func (d *InterfaceDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	i, err := df7.DecodeValid[Interface](obj)
	if err != nil {
		return nil, err
	}
	ifs, err := d.Ifaces(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := ifs.Attach(i.Interface, string(KeyInterface(i.Interface)))
	if err != nil {
		return nil, err
	}
	before, err := Neighbours(ctx, d.Client)
	if err != nil {
		return nil, err
	}
	if err := d.set(ctx, idx, i, true); err != nil {
		return nil, err
	}
	after, err := Neighbours(ctx, d.Client)
	if err != nil {
		return nil, err
	}
	if _, ok := after[idx]; !ok {
		var others []uint32
		for k := range after {
			if _, was := before[k]; !was {
				others = append(others, k)
			}
		}
		return nil, fmt.Errorf("%s: %w: asked for %s (sw_if_index %d), newly enabled sw_if_index %v — disable it by hand",
			NameInterface, ErrIndexMismatch, i.Interface, idx, others)
	}
	return Meta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: VPP ignores new parameters on an enabled interface,
// so the interface is disabled and enabled again with the new ones.
func (d *InterfaceDescriptor) Update(ctx context.Context, oldObj, newObj proto.Message, _ any) (any, error) {
	o, err := df7.Decode[Interface](oldObj)
	if err != nil {
		return nil, err
	}
	n, err := df7.DecodeValid[Interface](newObj)
	if err != nil {
		return nil, err
	}
	if o.Interface != n.Interface {
		return nil, scheduler.ErrRecreate
	}
	idx, found, err := d.Detach(ctx, n.Interface, string(KeyInterface(n.Interface)))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s: %w: %q", NameInterface, df7.ErrNoSuchInterface, n.Interface)
	}
	if err := d.set(ctx, idx, o, false); err != nil {
		return nil, err
	}
	return Meta{SwIfIndex: idx}, d.set(ctx, idx, n, true)
}

// Delete implements scheduler.Descriptor: re-resolve the interface (D-071) and disable (VPP
// ignores a disable of an interface without LLDP); a vanished interface is success.
func (d *InterfaceDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	i, err := df7.Decode[Interface](obj)
	if err != nil {
		return err
	}
	key := string(KeyInterface(i.Interface))
	idx, found, err := d.Detach(ctx, i.Interface, key)
	if err != nil {
		return err
	}
	if found {
		if err := d.set(ctx, idx, i, false); err != nil {
			return err
		}
	}
	return d.Release(i.Interface, key)
}

// Retrieve implements scheduler.Descriptor: write-only (D-063).
func (d *InterfaceDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, df7.Unsupported(NameInterface, "lldp_dump reports neighbours, not the port-desc/management configuration")
}

// Neighbour is one entry of lldp_dump: an interface with LLDP enabled and what was heard on it.
type Neighbour struct {
	SwIfIndex   uint32
	ChassisID   []byte
	PortID      []byte
	TTL         uint16
	LastHeard   float64
	LastSent    float64
	ChassisType lldp.ChassisIDSubtype
	PortType    lldp.PortIDSubtype
}

// Neighbours runs lldp_dump (cursor-paginated) and returns one entry per LLDP-enabled
// interface, keyed by sw_if_index. Read-only state for the state API and Create's check.
func Neighbours(ctx context.Context, c vpp.Client) (map[uint32]Neighbour, error) {
	out := map[uint32]Neighbour{}
	cursor := uint32(0)
	for page := 0; page < 1024; page++ {
		stream, err := lldp.NewServiceClient(c).LldpDump(ctx, &lldp.LldpDump{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("lldp_dump: %w", df7.PluginError("lldp", err))
		}
		var reply *lldp.LldpDumpReply
		for {
			det, rep, err := stream.Recv()
			if rep != nil {
				reply = rep
			}
			if det != nil {
				n := Neighbour{SwIfIndex: uint32(det.SwIfIndex), TTL: det.TTL, LastHeard: det.LastHeard, LastSent: det.LastSent,
					ChassisType: det.ChassisIDSubtype, PortType: det.PortIDSubtype}
				n.ChassisID = append(n.ChassisID, det.ChassisID[:min(int(det.ChassisIDLen), len(det.ChassisID))]...)
				n.PortID = append(n.PortID, det.PortID[:min(int(det.PortIDLen), len(det.PortID))]...)
				out[n.SwIfIndex] = n
			}
			if err == nil {
				continue
			}
			if errors.Is(err, api.EAGAIN) && reply != nil {
				break
			}
			if reply != nil && errors.Is(err, io.EOF) {
				return out, nil
			}
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, fmt.Errorf("lldp_dump: %w", err)
		}
		cursor = reply.Cursor
	}
	return out, nil
}

// Register constructs the per-interface lldp descriptor. lldp.interface depends on lldp.global
// only optionally, so it works whether or not this agent is the globals owner.
func Register(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewInterface(c, owner, opts...))
}

// RegisterGlobals constructs the VPP-global lldp.global descriptor. Only the globals owner
// (agent config globalsOwner: true, never a test slot on the shared host) calls it (D-071).
func RegisterGlobals(r scheduler.Registry, c vpp.Client, owner string, opts ...df7.Option) {
	r.Register(NewGlobal(c, owner, opts...))
}
