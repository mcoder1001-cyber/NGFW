package vpn

import (
	"fmt"
	"strconv"

	"ngfw/agent/internal/scheduler"
)

// Cross-plugin key conventions used by every DF-* prompt: an interface is "interface/<name>" and a
// FIB table is "vrf/<id>" (both provided by P05 core / DF-1). The DF-5 descriptors depend on them
// by these keys and never construct another plugin's descriptor-specific key.
const (
	InterfaceDescriptor = "interface"
	VRFDescriptor       = "vrf"
)

// InterfaceKey returns the dependency key of a VPP interface by name.
func InterfaceKey(name string) scheduler.Key { return scheduler.Join(InterfaceDescriptor, name) }

// VRFKey returns the dependency key of a FIB table.
func VRFKey(id uint32) scheduler.Key {
	return scheduler.Join(VRFDescriptor, strconv.FormatUint(uint64(id), 10))
}

// IDRange is the ownership rule for VPP objects that carry neither a tag nor a name: SPD ids and
// SA ids (docs/lab/shared-host-rules.md: a worker owns N000–N999). The zero value owns every id,
// which is the production setting (one agent per VPP). Create refuses ids outside the range so a
// test can never create an object it would not retrieve.
type IDRange struct{ Lo, Hi uint32 }

// Contains reports whether id is owned.
func (r IDRange) Contains(id uint32) bool {
	if r == (IDRange{}) {
		return true
	}
	return id >= r.Lo && id <= r.Hi
}

// Check returns an error naming kind when id is outside the owned range.
func (r IDRange) Check(kind string, id uint32) error {
	if !r.Contains(id) {
		return fmt.Errorf("vpn: %s id %d is outside the owned range %d–%d", kind, id, r.Lo, r.Hi)
	}
	return nil
}

// Uint returns id as a decimal key segment.
func Uint(id uint32) string { return strconv.FormatUint(uint64(id), 10) }
