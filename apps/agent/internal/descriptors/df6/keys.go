package df6

import (
	"strconv"

	"ngfw/agent/internal/scheduler"
)

// Key prefixes of objects owned by other tasks that DF-6 descriptors depend on. They follow
// the DF-6 task prompt ("interface/<name>", "vrf/<id>", "bridge-domain/<id>" from DF-1,
// "mpls-table/<id>" from DF-7); if P05 core settles on other names, change them here only.
const (
	InterfaceKeyPrefix    = "interface"
	VRFKeyPrefix          = "vrf"
	BridgeDomainKeyPrefix = "bridge-domain"
	MPLSTableKeyPrefix    = "mpls-table"
)

// InterfaceKey is the key of a VPP interface by name.
func InterfaceKey(name string) scheduler.Key { return scheduler.Join(InterfaceKeyPrefix, name) }

// VRFKey is the key of an IP table (VRF) by id.
func VRFKey(id uint32) scheduler.Key {
	return scheduler.Join(VRFKeyPrefix, strconv.FormatUint(uint64(id), 10))
}

// BridgeDomainKey is the key of an L2 bridge domain by id (DF-1).
func BridgeDomainKey(id uint32) scheduler.Key {
	return scheduler.Join(BridgeDomainKeyPrefix, strconv.FormatUint(uint64(id), 10))
}

// MPLSTableKey is the key of an MPLS table by id (DF-7).
func MPLSTableKey(id uint32) scheduler.Key {
	return scheduler.Join(MPLSTableKeyPrefix, strconv.FormatUint(uint64(id), 10))
}

// InterfaceDeps is the mandatory dependency on an interface; "" yields nothing.
func InterfaceDeps(name string) []scheduler.Dependency {
	if name == "" {
		return nil
	}
	return []scheduler.Dependency{{Key: InterfaceKey(name)}}
}

// VRFDeps is the mandatory dependency on a non-default table; table 0 always exists in VPP
// and is not modelled, so it yields nothing.
func VRFDeps(id uint32) []scheduler.Dependency {
	if id == 0 {
		return nil
	}
	return []scheduler.Dependency{{Key: VRFKey(id)}}
}

// U32 formats an id for a key.
func U32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }
