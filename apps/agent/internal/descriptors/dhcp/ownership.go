package dhcp

import "ngfw/agent/internal/descriptors/dfkit"

// CheckPersistent is the product agent's guard (dfkit/persist, TD-11b fix round 1, review M1): a
// client on an untagged interface is ours through the owner's DF-1 claim store, which must survive
// an agent restart. (Its Create still claims after the add — TD-11b questions Q3: switch to
// dfkit.Target.ClaimFirst.)
func (d *ClientDescriptor) CheckPersistent() error { return dfkit.CheckClaims(NameClient, d.owner) }
