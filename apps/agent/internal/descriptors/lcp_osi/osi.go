// Package lcp_osi manages the irreversible VPP-wide IS-IS OSI punt. No runtime disable exists.
package lcp_osi

import (
	"context"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	lcpapi "ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Name identifies the irreversible OSI punt descriptor.
const Name = "lcp.osi-proto"

// ProtocolISIS is the ISO 10589 protocol discriminator 0x83.
const ProtocolISIS uint8 = 0x83

// Key identifies the single IS-IS OSI punt value.
var Key = scheduler.Join(Name, "isis")

// Spec carries the supported OSI protocol discriminator.
type Spec struct {
	Protocol uint8 `json:"protocol"`
}

// Value returns the desired IS-IS OSI punt singleton.
func Value() *structpb.Struct { return dfkit.Encode(Spec{Protocol: ProtocolISIS}) }

// Descriptor manages an explicitly authorized VPP-wide OSI punt.
type Descriptor struct {
	client  vpp.Client
	globals dfkit.Globals
	optIn   bool
}

// New constructs an OSI descriptor with its global ownership and opt-in policy.
func New(client vpp.Client, globals dfkit.Globals, optIn bool) *Descriptor {
	return &Descriptor{client: client, globals: globals, optIn: optIn}
}

// Name returns the scheduler descriptor name.
func (*Descriptor) Name() string { return Name }

// KeyOf returns the OSI singleton key.
func (*Descriptor) KeyOf(proto.Message) scheduler.Key { return Key }

// Dependencies returns no per-object dependencies for this global singleton.
func (*Descriptor) Dependencies(proto.Message) []scheduler.Dependency { return nil }
func (d *Descriptor) check(value proto.Message) error {
	if !d.globals.Owner() {
		return dfkit.ErrNotGlobalsOwner
	}
	if !d.optIn {
		return fmt.Errorf("%s: irreversible OSI enable requires explicit operator opt-in", Name)
	}
	var spec Spec
	if err := dfkit.Decode(value, &spec); err != nil {
		return err
	}
	if spec.Protocol != ProtocolISIS {
		return dfkit.Specf("only IS-IS OSI protocol is supported")
	}
	return nil
}
func (d *Descriptor) enabled(ctx context.Context) (bool, error) {
	rep, err := lcpapi.NewServiceClient(d.client).LcpOsiProtoGet(ctx, &lcpapi.LcpOsiProtoGet{})
	if err != nil {
		return false, err
	}
	if int(rep.Count) != len(rep.OsiProtos) {
		return false, fmt.Errorf("inconsistent OSI getter count")
	}
	return slices.Contains(rep.OsiProtos, ProtocolISIS), nil
}

// Create enables the opted-in OSI protocol after validating ownership.
func (d *Descriptor) Create(ctx context.Context, value proto.Message) (any, error) {
	if err := d.check(value); err != nil {
		return nil, err
	}
	enabled, err := d.enabled(ctx)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, nil
	}
	_, err = lcpapi.NewServiceClient(d.client).LcpOsiProtoEnable(ctx, &lcpapi.LcpOsiProtoEnable{OsiProto: ProtocolISIS})
	return nil, err
}

// Update validates and applies the singleton value.
func (d *Descriptor) Update(ctx context.Context, _, value proto.Message, _ any) (any, error) {
	return d.Create(ctx, value)
}

// Delete is a documented no-op: VPP has no disable API. It never claims revocation.
func (d *Descriptor) Delete(context.Context, proto.Message, any) error { return nil }

// Retrieve reads the enabled singleton without claiming revocation support.
func (d *Descriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	if !d.globals.Owner() {
		return nil, dfkit.ErrNotGlobalsOwner
	}
	if !d.optIn {
		return nil, nil
	}
	enabled, err := d.enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	return []scheduler.KV{{Key: Key, Value: Value()}}, nil
}

// RecordsNoOwnership indicates that this opted-in VPP-global singleton has no per-agent claims.
func (*Descriptor) RecordsNoOwnership() {}
