package vrrp

import "ngfw/agent/internal/descriptors/dfkit"

// Ownership declarations of the product agent's guard (TD-11b, dfkit/persist; F-vrrp-config-sync registers the
// family — subsystems.Register refuses to start with a descriptor that declares neither). Gap fix named by
// subsystems' TestRegisterGuardsEveryDescriptor and every test that registers the product wiring.

// CheckPersistent declares that a VR on an untagged interface is ours through the owner's DF-1 claim store (Create claims
// the interface with the VR's key), which must survive an agent restart.
func (d *VRDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameVR, d.Owner) }

// RecordsNoOwnership declares that peers are addressed through their VR (re-resolved, D-071) and record nothing.
func (*PeersDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that tracking is addressed through its VR and records nothing.
func (*TrackDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that the running state is addressed through its VR and records nothing (write-only start/stop
// is read back from the VR dump's runtime state, D-063).
func (*StateDescriptor) RecordsNoOwnership() {}
