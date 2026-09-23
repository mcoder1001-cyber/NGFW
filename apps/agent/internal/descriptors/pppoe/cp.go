package pppoe

import (
	"context"
	"fmt"

	pppoeapi "ngfw/agent/binapi/pppoe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// CpName is the descriptor name; the key is "pppoe.cp/global".
const CpName = "pppoe.cp"

// pppoe_add_del_cp sets VPP's single pem->cp_if_index and enables pppoe-input on the
// device-input arc of that interface (review M2): it is a VPP-global setting, managed only by
// the globals owner (D-071), write-only (no getter). Because the feature enable does not
// deduplicate, the enable is sent once per VPP boot (a claim "<interface>" held by
// "pppoe.cp@vpp-<boot>", D-076); Delete disables only what this agent enabled on the running
// VPP, after re-resolving the interface by logical name (foreign interfaces refused).

// CpDescriptor is the globals-owner setter.
type CpDescriptor = df6.SingletonDescriptor[*Cp]

// NewCp returns the globals-owner setter (tests / the globals owner only).
func NewCp(c vpp.Client, owner string, opts ...df6.Option) *CpDescriptor {
	return df6.NewSingletonDescriptor(cpSpec(owner, df6.BuildOptions(owner, opts).Claims), c)
}

func cpSpec(owner string, claims df6.ClaimStore) df6.SingletonSpec[*Cp] {
	send := func(ctx context.Context, c vpp.Client, cp *Cp, enable bool) error {
		boot, err := df6.BootID(ctx, c)
		if err != nil {
			return err
		}
		holder := df6.BootHolder(CpName, boot)
		if claims.Claimed(cp.GetInterface(), holder) == enable {
			return nil
		}
		ifs, err := df6.DumpInterfaces(ctx, c, owner)
		if err != nil {
			return err
		}
		idx, err := ifs.Index(cp.GetInterface())
		if err != nil {
			if !enable && df6.IsNoSuchInterface(err) {
				return claims.Release(cp.GetInterface(), holder)
			}
			return err
		}
		var isAdd uint8
		if enable {
			isAdd = 1
		}
		if _, err := pppoeapi.NewServiceClient(c).PppoeAddDelCp(ctx, &pppoeapi.PppoeAddDelCp{SwIfIndex: idx, IsAdd: isAdd}); err != nil {
			return fmt.Errorf("pppoe_add_del_cp: %w", err)
		}
		if enable {
			return claims.Claim(cp.GetInterface(), holder)
		}
		return claims.Release(cp.GetInterface(), holder)
	}
	return df6.SingletonSpec[*Cp]{
		Name:   CpName,
		Plugin: Plugin,
		Validate: func(cp *Cp) error {
			if cp.GetInterface() == "" {
				return fmt.Errorf("%w: interface is mandatory", df6.ErrBadValue)
			}
			return nil
		},
		Set:   func(ctx context.Context, c vpp.Client, cp *Cp) error { return send(ctx, c, cp, true) },
		Unset: func(ctx context.Context, c vpp.Client, cp *Cp) error { return send(ctx, c, cp, false) },
		Deps:  func(cp *Cp) []scheduler.Dependency { return df6.InterfaceDeps(cp.GetInterface()) },
	}
}
