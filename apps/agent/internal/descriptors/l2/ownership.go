package l2

import "ngfw/agent/internal/descriptors/dfkit"

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-bridge-l2 was
// ported onto main (subsystems.Register refuses undeclared descriptors).

// RecordsNoOwnership declares: a bridge domain is ours through its VPP tag (BdTag), nothing is recorded.
func (*BridgeDomainDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares: a member is ours through its bridge domain (tagged), nothing is recorded.
func (*MemberDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares: a static FIB entry is ours through its bridge domain, nothing is recorded.
func (*FibEntryDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares: member flags are ours through the bridge (ours) the member sits in.
func (*FlagsDescriptor) RecordsNoOwnership() {}

// CheckPersistent declares: a tag rewrite on an untagged interface is ours through the owner's claim
// store (ClaimIfUntagged), which must survive an agent restart.
func (d *VlanTagRewriteDescriptor) CheckPersistent() error {
	return dfkit.CheckClaims(VlanTagRewriteName, d.owner)
}

// CheckPersistent declares: a cross-connect from an untagged interface is ours through the owner's
// claim store (ClaimIfUntagged), which must survive an agent restart.
func (d *XconnectDescriptor) CheckPersistent() error { return dfkit.CheckClaims(XconnectName, d.owner) }
