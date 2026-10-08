package subsystems

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	pppoedesc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/tapv2"
	ren "ngfw/agent/internal/renderers/pppoe"
	"ngfw/agent/internal/scheduler"
)

func (w *Wiring) registerPppoeCarrier(reg scheduler.Registry, rt *PppoeRuntime) error {
	if !w.env.GlobalsOwner {
		return nil
	}
	host := &pppoeCarrierHost{runner: rt.runner}
	d := pppoedesc.NewCarrierNamespace(w.env.Owner, host)
	d.Admit = func(ctx context.Context, spec ren.CarrierSpec) error {
		return pppoedesc.AdmitCarrier(ctx, w.env.Client, w.env.Owner, spec)
	}
	lookup, ok := reg.(interface {
		ForKey(scheduler.Key) (scheduler.Descriptor, bool)
	})
	if !ok {
		return errors.New("PPPoE carrier requires a descriptor lookup registry")
	}
	descriptor, ok := lookup.ForKey(scheduler.Join(tapv2.TapName, "pppwan"))
	if !ok {
		return errors.New("PPPoE carrier TAP descriptor is unavailable")
	}
	tap, ok := descriptor.(interface {
		SetNamespaceAdmission(func(context.Context, *tapv2.Tap) error)
	})
	if !ok {
		return errors.New("PPPoE carrier TAP admission hook is unavailable")
	}
	tap.SetNamespaceAdmission(func(ctx context.Context, actual *tapv2.Tap) error {
		leases, err := host.Inventory(ctx, w.env.Owner)
		if err != nil {
			return err
		}
		for _, lease := range leases {
			spec := lease.Spec
			if lease.Token != actual.GetHostNamespace() {
				continue
			}
			rawID, transitID := spec.TapIDs()
			expected := &tapv2.Tap{Name: spec.Logical, Id: transitID, HostIfName: spec.TransitHost(), HostNamespace: spec.Token(), HostMtu: spec.MTU, HostIp4Prefix: spec.Host4, HostIp6Prefix: spec.Host6, RxRingSize: 256, TxRingSize: 256}
			if actual.Name == spec.RawLogical() {
				expected = &tapv2.Tap{Name: spec.RawLogical(), Id: rawID, HostIfName: spec.RawHost(), HostNamespace: spec.Token(), HostMtu: spec.MTU + 8, RxRingSize: 256, TxRingSize: 256}
			}
			if !proto.Equal(actual, expected) {
				return errors.New("PPPoE TAP differs from immutable namespace specification")
			}
			return nil
		}
		return errors.New("PPPoE TAP has no verified namespace lease")
	})
	reg.Register(d)
	return nil
}
