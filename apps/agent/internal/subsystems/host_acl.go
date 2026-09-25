package subsystems

// F-host-acl-nftables: the host firewall family of the `acl` domain (Domains["acl"]): one singleton
// descriptor, host-acl.nftables/vrx, wrapping the nftables renderer (decision (a): no agent-core
// change). subsystems.go only carries the domain constant, the Domains entry and one registerHostACL call.
//
// Where the table goes (nftables.PathsFromEnv):
//
//	owner "vrx" + globals owner        table inet vrx      in the root network namespace (mode apply)
//	owner "vrx", VRX_GLOBALS_OWNER=0   table inet vrx      never loaded: `nft -c` only (mode check; tools/app)
//	other owners (test slots)          table inet vrx_<o>  never loaded: `nft -c` only (mode check) …
//	  … with VRX_HOST_ACL_NETNS=ns-…   loaded inside that namespace (mode netns; setns on a locked thread)
//	VRX_HOST_ACL_MODE=check            validate only, for any owner (a product stack on a shared host)

import (
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
)

// hostACLDescriptors is F-host-acl-nftables' part of Domains["acl"].
func hostACLDescriptors() []string { return []string{nftables.DescriptorName} }

// registerHostACL registers the host firewall descriptor, makes its runtime reachable for HostAclState
// and gives the builder the objects runtime's FQDN answers. Registered after the objects family: an FQDN
// answer change asks the agent for a resync (Env.Resync), which re-renders the table.
func (w *Wiring) registerHostACL(r scheduler.Registry) error {
	paths, err := nftables.PathsFromEnv(w.env.StateDir, w.env.Owner, w.env.GlobalsOwner)
	if err != nil {
		return err
	}
	ren := nftables.New(renderers.NewSystemRunner(nftables.Binaries()), paths)
	d := nftables.NewDescriptor(ren, nftables.NewStore(paths.StoreFile), w.env.Log.With("family", "host-acl"))
	r.Register(d)
	nftables.Register(w.env.StateDir, &nftables.Runtime{Owner: w.env.Owner, Descriptor: d})
	env := &desired.HostACLEnv{}
	if rt := w.ObjectModel(); rt != nil {
		env.FQDN, env.Applied = rt.FQDN, rt.Snapshot
		rt.Subscribe(func(objects.Change) { w.RequestResync() })
	}
	desired.SetHostACLEnv(env)
	w.env.Log.Info("host firewall wired", "table", "inet "+paths.Table, "mode", paths.Mode, "netns", paths.Netns, "rules_file", paths.RulesFile)
	return nil
}
