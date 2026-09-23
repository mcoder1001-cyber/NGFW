package ikev2

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

// ResponderHostname points an owned profile's responder at a hostname
// (ikev2_set_responder_hostname). ikev2_profile_dump reports the responder's sw_if_index and
// address but not the hostname, so the descriptor is write-only (D-063): Retrieve returns
// vpn.ErrRetrieveUnsupported and the reconciler re-applies it on resync (idempotent). VPP cannot
// unset a responder, so Delete is a no-op; the hostname goes away with the profile. A profile uses
// either this or IKEv2Profile.responder (address), not both.
type ResponderHostname struct{ cfg Config }

// NewResponderHostname returns the descriptor.
func NewResponderHostname(cfg Config) *ResponderHostname { return &ResponderHostname{cfg: cfg} }

// Name implements scheduler.Descriptor.
func (*ResponderHostname) Name() string { return ResponderHostnameName }

// KeyOf implements scheduler.Descriptor: ikev2.responder-hostname/<profile>.
func (*ResponderHostname) KeyOf(obj proto.Message) scheduler.Key {
	o, _ := obj.(*vpnpb.Ikev2ResponderHostname)
	return scheduler.Join(ResponderHostnameName, o.GetProfile())
}

// Dependencies implements scheduler.Descriptor: the profile, and the interface (Optional).
func (*ResponderHostname) Dependencies(obj proto.Message) []scheduler.Dependency {
	o, _ := obj.(*vpnpb.Ikev2ResponderHostname)
	deps := []scheduler.Dependency{{Key: scheduler.Join(ProfileName, o.GetProfile())}}
	if i := o.GetInterface(); i != "" {
		deps = append(deps, scheduler.Dependency{Key: vpn.InterfaceKey(i), Optional: true})
	}
	return deps
}

// Create implements scheduler.Descriptor (idempotent: re-issuing the setter is harmless).
func (d *ResponderHostname) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.Ikev2ResponderHostname)
	if !ok {
		return nil, typeErr(ResponderHostnameName, obj)
	}
	if o.GetProfile() == "" || o.GetHostname() == "" || len(o.GetHostname()) > 63 || strings.ContainsRune(o.GetHostname(), 0) {
		return nil, errors.New("ikev2: responder-hostname needs a profile and a hostname of 1–63 bytes")
	}
	idx := interface_types.InterfaceIndex(noInterface)
	if o.GetInterface() != "" {
		tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client)
		if err != nil {
			return nil, err
		}
		if idx, err = tbl.Index(o.GetInterface()); err != nil {
			return nil, err
		}
	}
	name := vppName(d.cfg.Owner, o.GetProfile())
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2SetResponderHostname(ctx, &ikev2.Ikev2SetResponderHostname{
		Name: name, Hostname: o.GetHostname(), SwIfIndex: idx,
	}); err != nil {
		return nil, fmt.Errorf("ikev2_set_responder_hostname (%s): %w", name, err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *ResponderHostname) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: a no-op (see the type doc).
func (*ResponderHostname) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve implements scheduler.Descriptor: VPP does not dump the hostname (D-063, write-only).
func (*ResponderHostname) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", ResponderHostnameName, vpn.ErrRetrieveUnsupported)
}
