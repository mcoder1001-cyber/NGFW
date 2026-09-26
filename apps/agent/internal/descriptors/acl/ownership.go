package acl

import "fmt"

// Ownership declarations for the product agent's persistence guard (TD-11b, dfkit/persist: a
// descriptor declares exactly one of RecordsNoOwnership and CheckPersistent, or the agent refuses
// to start). Added by F-acl when it registered the family (gap edit; test:
// subsystems TestACLDescriptorsDeclareOwnership).

// RecordsNoOwnership declares that an ACL is ours by its VPP tag "<owner>:<name>".
func (*Descriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that a MACIP ACL is ours by its VPP tag "<owner>:<name>".
func (*MacipACLDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that an interface binding is ours by the tags of the ACLs it lists
// (other owners' entries are preserved, never recorded).
func (*InterfaceBindingDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that a MACIP binding is ours by the tag of the bound MACIP ACL.
func (*MacipBindingDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that the counters switch records no ownership: it is never deleted
// (V7), and the value it remembers per VPP boot identity only avoids re-sending an idempotent enable —
// losing it on an agent restart re-sends it once.
func (*StatsEnableDescriptor) RecordsNoOwnership() {}

// CheckPersistent requires the whitelist claim store to survive an agent restart: a whitelist on an
// untagged interface is ours only by that claim (WithEtypeClaims; the product passes
// subsystems.Wiring.KeyedClaims("acl")). Structural check (a store with Persistent() bool, the
// dfkit/persist protocol), so this package imports no store implementation.
func (d *EtypeWhitelistDescriptor) CheckPersistent() error {
	if p, ok := d.opts.claims.(interface{ Persistent() bool }); ok && p.Persistent() {
		return nil
	}
	return fmt.Errorf("%s: whitelist claim store %T does not survive an agent restart (pass WithEtypeClaims with a persisted store)", NameEtypeWhitelist, d.opts.claims)
}
