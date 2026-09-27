package subsystems

// F-det44-map-dslite-cnat: the `nat` domain's CGNAT and transition families — DF-3's descriptors/{det44,mapnat,cnat}
// and the new descriptors/dslite — with the persisted NAT claim store and the D-071 globals flag. subsystems.go
// carries only the registration lines under this task's anchors. PNAT (descriptors/pnat) has no contract leaf yet
// (nat.pnat, NatConfig 27) and stays unwired.

import (
	"ngfw/agent/internal/descriptors/cnat"
	"ngfw/agent/internal/descriptors/det44"
	"ngfw/agent/internal/descriptors/dslite"
	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/scheduler"
)

// det44MapDsliteCnatDescriptors are this task's descriptors of the `nat` domain.
var det44MapDsliteCnatDescriptors = []string{
	det44.NameEnable,
	det44.NameTimeouts,
	det44.NameInterface,
	det44.NameMap,
	dslite.NameAftr,
	dslite.NameB4,
	dslite.NamePool,
	mapnat.NameParams,
	mapnat.NameDomain,
	mapnat.NameRule,
	mapnat.NameInterface,
	cnat.NameTranslation,
	cnat.NameSnatAddresses,
	cnat.NameSnatPolicy,
	cnat.NameSnatInterface,
	cnat.NameSnatExcludePfx,
	cnat.NameInterfaceFeature,
}

// registerDet44MapDsliteCnat registers the four families with the persisted claims of the "nat" family (claim keys
// are descriptor keys, so they never collide with the NAT44/NAT64 families) and the D-071 globals flag: a test slot
// only requires det44 enable/timeouts, the MAP parameters, the DS-Lite AFTR/B4 and the cnat default SNAT entry.
func (w *Wiring) registerDet44MapDsliteCnat(r scheduler.Registry) error {
	claims, err := w.KeyedClaims("nat")
	if err != nil {
		return err
	}
	opts := []natcommon.Option{natcommon.WithGlobalsOwner(w.env.GlobalsOwner), natcommon.WithClaims(claims)}
	det44.Register(r, w.env.Client, w.env.Owner, opts...)
	dslite.Register(r, w.env.Client, w.env.Owner, opts...)
	mapnat.Register(r, w.env.Client, w.env.Owner, opts...)
	cnat.Register(r, w.env.Client, w.env.Owner, opts...)
	return nil
}
