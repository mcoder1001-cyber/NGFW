package desired

// F-host-acl-nftables: builder and assembler of the host firewall part of the `acl` domain. The whole
// host firewall is ONE scheduler object of the agent-local descriptor host-acl.nftables
// (internal/renderers/nftables):
//
//	acl.host, acl.hostAttachments, acl.hostSettings  →  host-acl.nftables/vrx   pointer /acl
//	                                                     value: nftables.HostTable{config, sets, chains}
//
// The value is built by nftables.Build from the document's host lists and the expanded address and
// service objects (FQDN objects through the running agent's resolver), so an object edit re-renders the
// table. Build's findings (unknown objects, expansion limit, the anti-lockout check, …) become DryRun
// issues at their pointers. The rest of `acl` (lists, macip, attachments, macipAttachments) belongs to
// F-acl (VPP acl plugin): until it lands, those leaves are reported as agent.unsupported-field.

import (
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/scheduler"
)

// HostACLEnv is what the host firewall builder needs from the running agent: the FQDN answers of its
// objects runtime and the applied objects document (for transactions that do not carry `objects`).
type HostACLEnv struct {
	FQDN    objects.FQDNLookup
	Applied func() *vrxv1.ObjectsConfig
}

// hostACLEnv is set by subsystems when the family is wired (one agent per process; the projection has
// no agent handle, and tests that run several agents in one process wire them one after another).
var hostACLEnv atomic.Pointer[HostACLEnv]

// SetHostACLEnv installs the running agent's environment (nil: no FQDN answers, no applied objects).
func SetHostACLEnv(e *HostACLEnv) { hostACLEnv.Store(e) }

// HostACL emits the host firewall object of acl. objs is the transaction's objects document; when
// objectsInTxn is false the running agent's applied objects document is used instead (if it has one).
func HostACL(s Sink, acl *vrxv1.AclConfig, objs *vrxv1.ObjectsConfig, objectsInTxn bool) {
	if acl == nil {
		return
	}
	for _, leaf := range []struct {
		name string
		n    int
	}{{"lists", len(acl.GetLists())}, {"macip", len(acl.GetMacip())}, {"attachments", len(acl.GetAttachments())}, {"macipAttachments", len(acl.GetMacipAttachments())}} {
		if leaf.n > 0 {
			s.Warnf(Ptr("acl", leaf.name), "agent.unsupported-field", "acl.%s is realised by the VPP acl plugin (F-acl), not by this agent build; it is not applied", leaf.name)
		}
	}
	in := nftables.Input{ACL: acl, Objects: objs}
	if env := hostACLEnv.Load(); env != nil {
		in.FQDN = env.FQDN
		if !objectsInTxn && env.Applied != nil {
			in.Objects = env.Applied()
		}
	}
	v, issues := nftables.Build(in)
	for _, is := range issues {
		if is.Warning {
			s.Warnf(is.Pointer, is.Rule, "%s", is.Message)
		} else {
			s.Errorf(is.Pointer, is.Rule, "%s", is.Message)
		}
	}
	if v != nil {
		s.Add(nftables.Key, v, Ptr("acl"))
	}
}

// AssembleHostACL adds the applied host firewall configuration (acl.host, acl.hostAttachments,
// acl.hostSettings of the retrieved host-acl.nftables value) to into, which may already hold other acl
// leaves; nil stays nil when there is no host firewall.
func AssembleHostACL(kvs []scheduler.KV, into *vrxv1.AclConfig) *vrxv1.AclConfig {
	for _, kv := range kvs {
		v, ok := kv.Value.(*nftables.HostTable)
		if !ok || kv.Key != nftables.Key || v.GetConfig() == nil {
			continue
		}
		c := proto.Clone(v.GetConfig()).(*vrxv1.AclConfig)
		if into == nil {
			into = &vrxv1.AclConfig{}
		}
		into.Host, into.HostAttachments, into.HostSettings = c.GetHost(), c.GetHostAttachments(), c.GetHostSettings()
	}
	return into
}
