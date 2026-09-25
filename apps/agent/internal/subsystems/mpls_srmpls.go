package subsystems

// F-mpls-srmpls wiring (wave-A-hotspots A1): subsystems.go carries the descriptor names in
// Domains[Routing] and one registration call under its anchors; everything else is here.

import (
	"errors"
	"fmt"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/mpls"
	srmpls "ngfw/agent/internal/descriptors/sr_mpls"
	"ngfw/agent/internal/scheduler"
)

// Descriptor names of the MPLS (DF-7) and SR-MPLS (DF-6) families in Domains[Routing].
// sr-mpls.endpoint-color is registered but in no domain: SR-TE color steering is not configurable
// (out of scope), so it is never planned.
const (
	mplsTableName      = mpls.NameTable
	mplsInterfaceName  = mpls.NameInterface
	mplsRouteName      = mpls.NameRoute
	mplsIPBindName     = mpls.NameIPBind
	mplsTunnelName     = mpls.NameTunnel
	srMplsPolicyName   = srmpls.PolicyName
	srMplsSteeringName = srmpls.SteeringName
)

// registerMplsSrmpls registers the MPLS and SR-MPLS descriptors with the persisted stores of the
// Wiring (TD-11b): DF-7's table-0 label-route records go to the owner's BootStore (df7.SetBootStore,
// done by register), MPLS-interface claims to IfaceClaims (iface.SetClaimStore, done by register),
// and DF-6's keyed SR-MPLS claims to PairClaims("df6"). MPLS table ids come from the agent's id range
// (TD-8, df7.WithIDRange); MPLS table 0 follows the D-071 role: only the globals owner creates it,
// every other agent requires it (mpls.RegisterFor).
func registerMplsSrmpls(r scheduler.Registry, w *Wiring) error {
	c, owner := w.env.Client, w.env.Owner
	var opts []df7.Option
	ids, err := w.IDRange()
	switch {
	case errors.Is(err, ErrNoIDRange):
		// fail closed without refusing to start (the agent itself refuses a missing range, TD-8b):
		// the empty range owns no MPLS table id
		w.env.Log.Warn("MPLS: no VPP id range; this agent creates no MPLS table", "err", err)
	case err != nil:
		return fmt.Errorf("mpls: %w", err)
	}
	if r := ids.DF7(); r != nil {
		opts = append(opts, df7.WithIDRange(r.Lo, r.Hi))
	}
	mpls.RegisterFor(r, c, owner, w.env.GlobalsOwner, opts...)
	claims, err := w.PairClaims("df6")
	if err != nil {
		return fmt.Errorf("sr-mpls: %w", err)
	}
	srmpls.Register(r, c, owner, df6.WithClaims(claims), df6.WithGlobalsOwner(w.env.GlobalsOwner))
	return nil
}
