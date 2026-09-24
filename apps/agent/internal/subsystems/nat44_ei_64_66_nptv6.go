package subsystems

// F-nat44-ei-64-66-nptv6: the `nat` domain's NAT44-EI, NAT64 and NAT66 families (DF-3 descriptors/{nat44ei,nat64,nat66})
// and the NPTv6 family (descriptors/npt66), with the persisted NAT claim store and the D-071 globals flag.
// subsystems.go carries only the registration lines under this task's anchors.

import (
	"ngfw/agent/internal/descriptors/nat44ei"
	"ngfw/agent/internal/descriptors/nat64"
	"ngfw/agent/internal/descriptors/nat66"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/npt66"
	"ngfw/agent/internal/scheduler"
)

// nat44EI6466NptDescriptors are this task's descriptors of the `nat` domain. nat44-ei.ipfix is registered but in no
// domain: NAT IPFIX logging has no owner in this build (nat.ipfix stays an agent.unsupported-field warning).
var nat44EI6466NptDescriptors = []string{
	nat44ei.NameEnable,
	nat44ei.NameTimeouts,
	nat44ei.NameForwarding,
	nat44ei.NameInterfaceFeature,
	nat44ei.NameOutputFeature,
	nat44ei.NameAddressPool,
	nat44ei.NameInterfaceAddress,
	nat44ei.NameStaticMapping,
	nat44ei.NameIdentityMapping,
	nat64.NameEnable,
	nat64.NameTimeouts,
	nat64.NamePrefix,
	nat64.NamePool,
	nat64.NameInterface,
	nat64.NameStaticBIB,
	nat66.NameEnable,
	nat66.NameInterface,
	nat66.NameStaticMapping,
	npt66.NameBinding,
}

// registerNat44EI6466Nptv6 registers the four families with the persisted claims of the "nat" family (the same store
// as NAT44-ED: claim keys are descriptor keys, so they never collide) and the D-071 globals flag (a test slot only
// requires the nat44-ei enable/timeouts/forwarding and the nat64/nat66 enables).
func (w *Wiring) registerNat44EI6466Nptv6(r scheduler.Registry) error {
	claims, err := w.KeyedClaims("nat")
	if err != nil {
		return err
	}
	opts := []natcommon.Option{natcommon.WithGlobalsOwner(w.env.GlobalsOwner), natcommon.WithClaims(claims)}
	nat44ei.Register(r, w.env.Client, w.env.Owner, opts...)
	nat64.Register(r, w.env.Client, w.env.Owner, opts...)
	nat66.Register(r, w.env.Client, w.env.Owner, opts...)
	npt66.Register(r, w.env.Client, w.env.Owner, opts...)
	return nil
}
