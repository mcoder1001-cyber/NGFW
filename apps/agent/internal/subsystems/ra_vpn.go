package subsystems

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
	"strconv"
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
	if w.env.RA != nil && w.env.RA.Handoff != nil {
		namespace.Handoff = w.env.RA.Handoff
	} else {
		handoff, err := ravpn.NewFixedNamespaceHandoff(func(ctx context.Context) ([]ravpn.MountTarget, error) {
			current, err := bootid.Current(ctx, w.env.Client)
			if err != nil || !current.Complete() || current.PID <= 0 {
				return nil, ravpn.ErrBoundary
			}
			vppTarget, err := raMountTarget(current)
			if err != nil {
				return nil, err
			}
			executable, err := os.Readlink("/proc/1/exe")
			if err != nil || (executable != "/usr/lib/systemd/systemd" && executable != "/lib/systemd/systemd") {
				return nil, ravpn.ErrBoundary
			}
			managerTarget, err := raMountTarget((bootid.Reader{}).ForPID(1))
			if err != nil {
				return nil, err
			}
			after, err := bootid.Current(ctx, w.env.Client)
			if err != nil || !after.Equal(current) {
				return nil, ravpn.ErrBoundary
			}
			return []ravpn.MountTarget{vppTarget, managerTarget}, nil
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

// raMountTarget reads a held kernel namespace handle, bound to a complete process
// identity before and after its inode is read. Profile data never selects a path.
func raMountTarget(identity bootid.Identity) (ravpn.MountTarget, error) {
	if identity.PID <= 0 || !identity.Complete() || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ravpn.MountTarget{}, ravpn.ErrBoundary
	}
	//nolint:gosec // G304: the positive PID comes from verified VPP or fixed PID1; only a kernel NSFS path is opened.
	file, err := os.Open("/proc/" + strconv.Itoa(identity.PID) + "/ns/mnt")
	if err != nil {
		return ravpn.MountTarget{}, ravpn.ErrBoundary
	}
	defer func() { _ = file.Close() }()
	var stat unix.Stat_t
	var filesystem unix.Statfs_t
	namespaceType, typeError := unix.IoctlRetInt(int(file.Fd()), unix.NS_GET_NSTYPE)
	if typeError != nil || namespaceType != unix.CLONE_NEWNS || unix.Fstat(int(file.Fd()), &stat) != nil || unix.Fstatfs(int(file.Fd()), &filesystem) != nil || filesystem.Type != unix.NSFS_MAGIC || stat.Ino == 0 || !(bootid.Reader{}).ForPID(identity.PID).Equal(identity) {
		return ravpn.MountTarget{}, ravpn.ErrBoundary
	}
	return ravpn.MountTarget{Boot: identity, MountInode: stat.Ino}, nil
}
