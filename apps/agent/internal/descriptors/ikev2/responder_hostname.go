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
// vpn.ErrRetrieveUnsupported and the reconciler re-applies it on resync.
//
// D-076: VPP's setter is not idempotent — every call vec_dup()s the hostname without freeing the
// previous copy (a leak per resync) and resets responder.is_resolved (an initiator would resolve
// again). So Create keeps an applied-once record (vpn.Records, bound to the D-080 VPP boot
// identity) with the applied value "<hostname>|<sw_if_index>/<logical interface>" and skips the
// call while the record matches; after a VPP restart, or when the profile was deleted/re-added
// (Profile drops the record), the hostname is applied once more.
//
// VPP cannot unset a responder, so Delete issues nothing (it drops the record); the hostname goes
// away with the profile. A profile uses either this or Ikev2Profile.responder (address), not both.
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

// hostnameRecordKey is the applied-once record of the responder hostname of profile.
func hostnameRecordKey(profile string) string {
	return string(scheduler.Join(ResponderHostnameName, profile))
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

// Create implements scheduler.Descriptor (applied once per VPP instance and value, see the type
// doc).
func (d *ResponderHostname) Create(ctx context.Context, obj proto.Message) (any, error) {
	o, ok := obj.(*vpnpb.Ikev2ResponderHostname)
	if !ok {
		return nil, typeErr(ResponderHostnameName, obj)
	}
	if o.GetProfile() == "" || o.GetHostname() == "" || len(o.GetHostname()) > 63 || strings.ContainsAny(o.GetHostname(), "\x00|") {
		return nil, errors.New("ikev2: responder-hostname needs a profile and a hostname of 1–63 bytes")
	}
	idx := interface_types.InterfaceIndex(noInterface)
	if o.GetInterface() != "" {
		tbl, err := vpn.DumpInterfaces(ctx, d.cfg.Client, d.cfg.Owner)
		if err != nil {
			return nil, err
		}
		if idx, err = tbl.Resolve(o.GetInterface()); err != nil {
			return nil, fmt.Errorf("%s: %w", ResponderHostnameName, err)
		}
	}
	rec := d.cfg.records()
	bid, err := rec.Identity(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ResponderHostnameName, err)
	}
	key := hostnameRecordKey(o.GetProfile())
	value := fmt.Sprintf("%s|%d/%s", o.GetHostname(), uint32(idx), o.GetInterface())
	if v, ok := rec.Valid(bid, key); ok && v == value {
		return nil, nil // applied on this VPP instance already (D-076)
	}
	name := vppName(d.cfg.Owner, o.GetProfile())
	if _, err := ikev2.NewServiceClient(d.cfg.Client).Ikev2SetResponderHostname(ctx, &ikev2.Ikev2SetResponderHostname{
		Name: name, Hostname: o.GetHostname(), SwIfIndex: idx,
	}); err != nil {
		return nil, fmt.Errorf("ikev2_set_responder_hostname (%s): %w", name, err)
	}
	if err := rec.Put(bid, key, value); err != nil {
		return nil, fmt.Errorf("%s: record: %w", ResponderHostnameName, err)
	}
	return nil, nil
}

// Update implements scheduler.Descriptor.
func (d *ResponderHostname) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return d.Create(ctx, newObj)
}

// Delete implements scheduler.Descriptor: nothing in VPP (see the type doc); drops the record.
func (d *ResponderHostname) Delete(_ context.Context, obj proto.Message, _ any) error {
	o, ok := obj.(*vpnpb.Ikev2ResponderHostname)
	if !ok {
		return typeErr(ResponderHostnameName, obj)
	}
	return d.cfg.records().Drop(hostnameRecordKey(o.GetProfile()))
}

// Retrieve implements scheduler.Descriptor: VPP does not dump the hostname (D-063, write-only).
func (*ResponderHostname) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", ResponderHostnameName, vpn.ErrRetrieveUnsupported)
}
