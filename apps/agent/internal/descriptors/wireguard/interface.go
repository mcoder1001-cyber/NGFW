package wireguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// Interface manages WireGuard interfaces (wireguard_interface_create / _delete; dump
// wireguard_interface_dump with show_private_key=false — never true). VPP names the interface
// wg<instance> and refuses an existing instance, so an interface that is not ours is never
// adopted; the owner tag "<owner>:wg<instance>" makes the logical name equal VPP's (D-069). The private key is an
// "x25519:<base64 public key>" reference: Create resolves the 32-byte private key, Retrieve
// rebuilds the reference from the public key VPP reports, so the private key never leaves the
// secret store after Create. Every field is immutable (no update message): Update is ErrRecreate.
// generate_key is not used: a VPP-generated key could not be referenced by desired state.
type Interface struct{ cfg Config }

// InterfaceMeta is the runtime handle of a WireGuard interface.
type InterfaceMeta struct{ SwIfIndex uint32 }

// NewInterface returns the descriptor.
func NewInterface(cfg Config) *Interface { return &Interface{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*Interface) Name() string { return InterfaceName }

// RecordsNoOwnership declares the TD-11b ownership protocol (dfkit/persist): the interface is ours by
// its owner tag "<owner>:wg<instance>", which VPP itself carries; nothing is recorded in a store.
func (*Interface) RecordsNoOwnership() {}

// KeyOf implements scheduler.Descriptor: wireguard.interface/wg<instance>.
func (*Interface) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.WireguardInterface)
	return scheduler.Join(InterfaceName, ItfName(o.GetInstance()))
}

// Dependencies implements scheduler.Descriptor. The source address is not tied to a key: DF-1's
// address key needs the interface and prefix length, which a WireGuard src_ip does not carry (see
// docs/agent/descriptors/wireguard.md); VPP accepts a src_ip that is not (yet) configured.
func (*Interface) Dependencies(proto.Message) []scheduler.Dependency { return nil }

// ProvidedKeys lets the WireGuard interface satisfy the cross-plugin alias interface/wg<instance>
// (D-065; P05's optional KeyProvider extension), so peers and anything else referencing it by
// name order after it.
func (*Interface) ProvidedKeys(obj proto.Message) []scheduler.Key {
	o, _ := obj.(*vpnpb.WireguardInterface)
	return []scheduler.Key{vpn.InterfaceKey(ItfName(o.GetInstance()))}
}

// Create implements scheduler.Descriptor.
func (d *Interface) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.WireguardInterface)
	if !ok {
		return nil, typeErr(InterfaceName, obj)
	}
	if o.GetInstance() == noInterface {
		return nil, errors.New("wireguard: interface needs an explicit instance")
	}
	if o.GetPort() == 0 || o.GetPort() > 65535 {
		return nil, fmt.Errorf("wireguard: %s port %d out of range 1–65535", ItfName(o.GetInstance()), o.GetPort())
	}
	if d051, ok := unavailable(o.GetPrivateKey()); ok {
		return nil, fmt.Errorf("wireguard: %s private key %s: %w", ItfName(o.GetInstance()), d051, ErrSecretUnavailable)
	}
	if !strings.HasPrefix(o.GetPrivateKey(), vpn.RefX25519) {
		return nil, fmt.Errorf("wireguard: %s needs private_key as an x25519 reference", ItfName(o.GetInstance()))
	}
	src, err := vpn.ParseAddress(o.GetSrcIp())
	if err != nil {
		return nil, err
	}
	priv, err := vpn.Resolve(ctx, d.cfg.Secrets, d.cfg.Keys, o.GetPrivateKey())
	if err != nil {
		return nil, fmt.Errorf("wireguard: %s private_key: %w", ItfName(o.GetInstance()), err)
	}
	defer vpn.Zero(priv)
	if len(priv) != vpn.X25519KeyLen {
		return nil, fmt.Errorf("wireguard: %s private_key must be %d bytes", ItfName(o.GetInstance()), vpn.X25519KeyLen)
	}
	req := &wireguard.WireguardInterfaceCreate{Interface: wireguard.WireguardInterface{
		UserInstance: o.GetInstance(), PrivateKey: priv, Port: uint16(o.GetPort()), SrcIP: src, //nolint:gosec // checked
	}}
	svc := wireguard.NewServiceClient(d.cfg.Client)
	name := ItfName(o.GetInstance())
	del := func(i uint32) error {
		_, err := svc.WireguardInterfaceDelete(ctx, &wireguard.WireguardInterfaceDelete{SwIfIndex: interface_types.InterfaceIndex(i)})
		return err
	}
	// D-095 / VPP V19 (TD-3 re-review H1): the new sw_if_index is cleaned of what its previous
	// holder left behind (or quarantined and a fresh one taken) before it is tagged and reported
	// created — an addressed wg interface on an index with a stale ip classify binding is the
	// 04:50 crash path
	idx, err := ifsanitize.Acquire(ctx, d.cfg.Client, d.cfg.Owner, name, func() (uint32, error) {
		rep, err := svc.WireguardInterfaceCreate(ctx, req)
		if err != nil {
			return 0, fmt.Errorf("wireguard_interface_create (%s): %w", name, err)
		}
		return uint32(rep.SwIfIndex), nil
	}, del)
	if err != nil {
		return nil, err
	}
	if err := vpn.TagInterface(ctx, d.cfg.Client, interface_types.InterfaceIndex(idx), d.cfg.Owner, name); err != nil {
		_ = del(idx)
		return nil, err
	}
	return InterfaceMeta{SwIfIndex: idx}, nil
}

// Update implements scheduler.Descriptor: nothing can change in place.
func (*Interface) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor. Right before the delete it re-verifies that the index
// still carries our tag for this instance (D-071: indexes are reused after a VPP restart); an
// interface that is gone needs nothing (D-074).
func (d *Interface) Delete(ctx context.Context, obj proto.Message, meta any) error {
	o, ok := obj.(*vpnpb.WireguardInterface)
	if !ok {
		return typeErr(InterfaceName, obj)
	}
	m, ok := meta.(InterfaceMeta)
	if !ok {
		return metaErr(InterfaceName, meta)
	}
	present, err := vpn.OwnedAt(ctx, d.cfg.Client, d.cfg.Owner, m.SwIfIndex, ItfName(o.GetInstance()))
	if err != nil || !present {
		return err
	}
	// D-095 c / TD-3 re-review H1: bindings go while their tables still exist (VPP keeps them on the
	// freed index, V19)
	if err := ifsanitize.BeforeDelete(ctx, d.cfg.Client, m.SwIfIndex, ItfName(o.GetInstance())); err != nil {
		return err
	}
	if _, err := wireguard.NewServiceClient(d.cfg.Client).WireguardInterfaceDelete(ctx, &wireguard.WireguardInterfaceDelete{
		SwIfIndex: interface_types.InterfaceIndex(m.SwIfIndex),
	}); err != nil {
		return fmt.Errorf("wireguard_interface_delete (%s): %w", ItfName(o.GetInstance()), err)
	}
	return nil
}

// Retrieve implements scheduler.Descriptor: WireGuard interfaces tagged by this owner with the tag
// id of their instance.
func (d *Interface) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
	if err != nil {
		return nil, err
	}
	stream, err := wireguard.NewServiceClient(d.cfg.Client).WireguardInterfaceDump(ctx, &wireguard.WireguardInterfaceDump{
		ShowPrivateKey: false, SwIfIndex: interface_types.InterfaceIndex(noInterface),
	})
	if err != nil {
		return nil, fmt.Errorf("wireguard_interface_dump: %w", err)
	}
	var out []scheduler.KV
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("wireguard_interface_dump: %w", err)
		}
		w := det.Interface
		vpn.Zero(w.PrivateKey) // empty with show_private_key=false; zeroed regardless
		idx := uint32(w.SwIfIndex)
		if id, owned := tbl.Owned(idx); !owned || id != ItfName(w.UserInstance) {
			continue
		}
		v := &vpnpb.WireguardInterface{
			Instance: w.UserInstance, PrivateKey: vpn.X25519RefFromPublic(w.PublicKey),
			Port: uint32(w.Port), SrcIp: srcIP(w.SrcIP),
		}
		out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: InterfaceMeta{SwIfIndex: idx}})
	}
	return sortKVs(out), nil
}

func srcIP(a ip_types.Address) string { return vpn.AddressString(a) }
