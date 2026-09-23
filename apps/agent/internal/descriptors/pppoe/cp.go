package pppoe

import (
	"context"
	"fmt"

	pppoeapi "ngfw/agent/binapi/pppoe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
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

// cpProbe reads VPP's actual state of pppoe-input on the device-input arc (decides over the
// per-boot record: correct after interface re-creation and VPP restarts, review N1).
var cpProbe = df6.FeatureProbe("device-input", "pppoe-input", "", "")

func cpSpec(owner string, claims df6.ClaimStore) df6.SingletonSpec[*Cp] {
	send := func(ctx context.Context, c vpp.Client, cp *Cp, enable bool) error {
		boot, err := bootid.Current(ctx, c)
		if err != nil {
			return err
		}
		holder := df6.BootHolder(CpName, boot)
		ifs, err := df6.DumpInterfaces(ctx, c, owner)
		if err != nil {
			return err
		}
		idx, err := ifs.Index(cp.GetInterface())
		if err != nil {
			if !enable && df6.IsNoSuchInterface(err) {
				return nil // interface gone: nothing enabled on it any more
			}
			return err
		}
		// keyed by logical name AND sw_if_index (D-080, review N1): a recreated interface is new
		id := fmt.Sprintf("%s@%d", cp.GetInterface(), idx)
		on, err := cpProbe(ctx, c, idx, false)
		if err != nil {
			return err
		}
		if on == enable {
			if enable {
				return claims.Claim(id, holder)
			}
			return claims.Release(id, holder)
		}
		var isAdd uint8
		if enable {
			isAdd = 1
		}
		if _, err := pppoeapi.NewServiceClient(c).PppoeAddDelCp(ctx, &pppoeapi.PppoeAddDelCp{SwIfIndex: idx, IsAdd: isAdd}); err != nil {
			return fmt.Errorf("pppoe_add_del_cp: %w", err)
		}
		if enable {
			return claims.Claim(id, holder)
		}
		return claims.Release(id, holder)
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
		// moving the CP interface disables pppoe-input on the old one first (review N5)
		Change: func(ctx context.Context, c vpp.Client, old, cp *Cp) error {
			if old.GetInterface() != cp.GetInterface() {
				if err := send(ctx, c, old, false); err != nil {
					return err
				}
			}
			return send(ctx, c, cp, true)
		},
		Deps: func(cp *Cp) []scheduler.Dependency { return df6.InterfaceDeps(cp.GetInterface()) },
	}
}
