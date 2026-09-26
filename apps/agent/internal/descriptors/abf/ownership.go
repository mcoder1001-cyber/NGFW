package abf

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-rpf-adl-pbr was ported
// onto main (subsystems.Register refuses undeclared descriptors).

// RecordsNoOwnership declares: an ABF policy is ours by its policy id (df2.IDRange) and its ACL, no store.
func (*PolicyDescriptor) RecordsNoOwnership() {}

// CheckPersistent declares: an ABF attachment on an untagged interface is ours through the DF-2 claim store.
func (d *AttachDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(AttachName) }
