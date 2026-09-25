package dhcp

import "ngfw/agent/internal/descriptors/dfkit"

// CheckPersistent is the product agent's guard (dfkit/persist, TD-11b fix round 1, review M1): a
// client on an untagged interface is ours through the owner's DF-1 claim store, which must survive
// an agent restart. Its Create claims before the add (dfkit.Target.ClaimFirst, TD-11b Q3).
func (d *ClientDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameClient, d.owner) }
