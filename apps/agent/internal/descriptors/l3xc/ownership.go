package l3xc

import "ngfw/agent/internal/descriptors/dfkit"

// CheckPersistent declares (TD-11b guard, dfkit/persist; added when F-bridge-l2 was ported onto main):
// an l3xc on an untagged interface is ours through the owner's claim store (ClaimIfUntagged, one holder
// per family), which must survive an agent restart.
func (d *Descriptor) CheckPersistent() error { return dfkit.CheckClaims(L3xcName, d.owner) }
