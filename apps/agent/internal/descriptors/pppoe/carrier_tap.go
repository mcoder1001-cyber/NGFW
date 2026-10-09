package pppoe

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"strings"
)

// CarrierTapName identifies PPP TAPs separately from remote-access TAPs.
const CarrierTapName = "pppoe.carrier.tap"

// RecordsNoOwnership declares that VPP carries the TAP owner tag. Namespace claims belong to
// the separately checked NamespaceDescriptor, never a process-local TAP store.
func (*CarrierTapDescriptor) RecordsNoOwnership() {}

// CarrierTapDescriptor has a distinct ownership scope from remote-access TAPs.
// It delegates generated VPP operations to the existing TAP implementation and
// requires that implementation's verified namespace admission for every write.
type CarrierTapDescriptor struct {
	Tap   *tapv2.TapDescriptor
	Admit func(context.Context, *tapv2.Tap) error
}

// Name returns the registered descriptor identity.
func (*CarrierTapDescriptor) Name() string { return CarrierTapName }

// KeyOf returns the stable reconciliation key.
func (*CarrierTapDescriptor) KeyOf(v proto.Message) scheduler.Key {
	p, ok := v.(*tapv2.Tap)
	if !ok {
		return ""
	}
	return scheduler.Join(CarrierTapName, p.Name)
}

// Dependencies declares prerequisite objects for safe reconciliation.
func (d *CarrierTapDescriptor) Dependencies(v proto.Message) []scheduler.Dependency {
	return d.Tap.Dependencies(v)
}

// Normalize canonicalizes the desired value before comparison.
func (d *CarrierTapDescriptor) Normalize(v proto.Message) proto.Message { return d.Tap.Normalize(v) }
func carrierTap(v proto.Message) (*tapv2.Tap, error) {
	p, ok := v.(*tapv2.Tap)
	if !ok || !strings.HasPrefix(p.HostNamespace, "ngp-") {
		return nil, errors.New("carrier TAP requires an owned PPP namespace")
	}
	return p, nil
}

// Create creates the validated owned object.
func (d *CarrierTapDescriptor) Create(ctx context.Context, v proto.Message) (any, error) {
	if _, err := carrierTap(v); err != nil {
		return nil, err
	}
	return d.Tap.Create(ctx, v)
}

// Update reconciles an existing owned object with the next value.
func (d *CarrierTapDescriptor) Update(ctx context.Context, old, next proto.Message, meta any) (any, error) {
	if _, err := carrierTap(next); err != nil {
		return nil, err
	}
	return d.Tap.Update(ctx, old, next, meta)
}

// Delete removes only the owned object.
func (d *CarrierTapDescriptor) Delete(ctx context.Context, v proto.Message, meta any) error {
	p, err := carrierTap(v)
	if err != nil {
		return err
	}
	if d.Admit == nil {
		return errors.New("carrier namespace admission unavailable")
	}
	if err = d.Admit(ctx, p); err != nil {
		return err
	}
	return d.Tap.Delete(ctx, v, meta)
}

// Retrieve reads actual owned state for reconciliation and recovery.
func (d *CarrierTapDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	rows, err := d.Tap.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	out := []scheduler.KV{}
	for _, row := range rows {
		tap, err := carrierTap(row.Value)
		if err != nil {
			continue
		}
		if d.Admit == nil {
			return nil, errors.New("carrier namespace admission unavailable")
		}
		if err = d.Admit(ctx, tap); err != nil {
			return nil, err
		}
		row.Key = scheduler.Join(CarrierTapName, tap.Name)
		out = append(out, row)
	}
	return out, nil
}

// ProvidedKeys advertises the actual VPP TAP creator identity that the central
// interface resolver reads back by device class. Reconciliation ownership remains
// in CarrierTapName; the remote-access descriptor never receives these objects.
func (*CarrierTapDescriptor) ProvidedKeys(v proto.Message) []scheduler.Key {
	p, err := carrierTap(v)
	if err != nil {
		return nil
	}
	return []scheduler.Key{scheduler.Join(tapv2.TapName, p.Name)}
}
