package subsystems

import (
	"context"
	"time"

	"ngfw/agent/internal/descriptors/tapv2"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/bootid"
)

func init() { Domains[VPN] = append(Domains[VPN], ravpn.NamespaceName, tapv2.TapName) }

// No daemon is activated by registering transport descriptors. Enabled profile
// projection must separately prove explicit outer/inner VRF and ACL handoff.
func (w *Wiring) registerRATransport(r scheduler.Registry) error {
	ids, err := w.IDRange()
	if err != nil {
		// A wiring fixture without numeric scope owns no TAP IDs. The product
		// agent validates its required scope at startup; this dormant descriptor
		// never relaxes missing scope into permission to allocate.
		ids = NoIDs()
	}
	namespace := ravpn.NewNamespaceDescriptor(w.env.Owner)
	if w.env.RA != nil {
		namespace.Inventory = w.env.RA.Inventory
	}
	if w.env.RA != nil && w.env.RA.Handoff != nil {
		namespace.Handoff = w.env.RA.Handoff
	} else {
		handoff, err := ravpn.NewFixedNamespaceHandoffForProvider(&ravpn.SystemdNamespaceTargets{
			ExpectedVPP: func(ctx context.Context) (bootid.Identity, error) {
				if w.env.Client == nil {
					return bootid.Identity{}, ravpn.ErrBoundary
				}
				current, err := bootid.Current(ctx, w.env.Client)
				if err != nil || !current.Complete() || current.PID <= 0 {
					return bootid.Identity{}, ravpn.ErrBoundary
				}
				return current, nil
			},
		})
		if err != nil {
			return err
		}
		namespace.Handoff = handoff
	}
	r.Register(namespace)
	r.Register(&ravpn.GuardedTAP{
		Tap:   tapv2.New(w.env.Client, w.env.Owner),
		Store: &ravpn.LazyTAPReceipts{StateDir: w.env.StateDir},
		Boot: func() bootid.Identity {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			identity, err := bootid.Current(ctx, w.env.Client)
			if err != nil {
				return bootid.Identity{}
			}
			return identity
		},
		Plan: func(instance string) (*ravpn.NetworkPlan, error) {
			plan, err := ravpn.ReadAgentPlanByNamespace(instance)
			if err != nil || plan.Owner != w.env.Owner {
				return nil, ravpn.ErrBoundary
			}
			return plan, nil
		},
		AllowedID: func(id uint32) bool { return ids == nil || (id >= ids.Lo && id <= ids.Hi) },
		Retired:   ravpn.RetiredVPPBoot,
		Absent: func(ctx context.Context, endpoint *tapv2.Tap, oldIndex uint32) error {
			return ravpn.VerifyTAPAbsent(ctx, w.env.Client, endpoint, oldIndex)
		},
	})
	return nil
}
