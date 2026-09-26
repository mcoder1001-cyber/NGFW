package urpf

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-rpf-adl-pbr was ported
// onto main (subsystems.Register refuses undeclared descriptors).

// CheckPersistent declares: uRPF on an untagged interface is ours through the DF-2 claim store.
func (d *Descriptor) CheckPersistent() error { return d.opts.CheckPersistent(Name) }
