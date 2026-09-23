package df2

import (
	"strconv"

	"ngfw/agent/internal/scheduler"
)

// Key prefixes of objects owned by other tasks that DF-2 descriptors depend on. They follow
// the DF-2 task prompt ("interface/<name>", "vrf/<id>") and DF-4 docs/agent/descriptors/acl.md ("acl.acl/<name>"); if P05 core / DF-4
// settle on other names, change them here only.
const (
	InterfaceKeyPrefix   = "interface"
	VRFKeyPrefix         = "vrf"
	ACLKeyPrefix         = "acl.acl"
	InterfaceIPKeyPrefix = "interface-ip"
)

// InterfaceKey is the key of a VPP interface by name.
func InterfaceKey(name string) scheduler.Key { return scheduler.Join(InterfaceKeyPrefix, name) }

// VRFKey is the key of an IP table (VRF) by id.
func VRFKey(id uint32) scheduler.Key {
	return scheduler.Join(VRFKeyPrefix, strconv.FormatUint(uint64(id), 10))
}

// ACLKey is the key of an acl-plugin ACL by name (DF-4).
func ACLKey(name string) scheduler.Key { return scheduler.Join(ACLKeyPrefix, name) }

// InterfaceIPKey is the key of an interface address (P05 core).
func InterfaceIPKey(iface, prefix string) scheduler.Key {
	return scheduler.Join(InterfaceIPKeyPrefix, iface, prefix)
}

// InterfaceDep is the mandatory dependency on an interface.
func InterfaceDep(name string) scheduler.Dependency {
	return scheduler.Dependency{Key: InterfaceKey(name)}
}

// VRFDeps is the mandatory dependency on a non-default table; table 0 always exists in VPP
// and is not modelled, so it yields nothing.
func VRFDeps(id uint32) []scheduler.Dependency {
	if id == 0 {
		return nil
	}
	return []scheduler.Dependency{{Key: VRFKey(id)}}
}
