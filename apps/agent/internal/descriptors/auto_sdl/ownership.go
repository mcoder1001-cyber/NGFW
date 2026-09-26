package autosdl

import "ngfw/agent/internal/descriptors/dfkit"

// Ownership declaration for the TD-11b persistence guard (dfkit/persist), added when F-rpf-adl-pbr was ported onto main.

// CheckPersistent declares that the D-076 applied-once record lives in the boot store, which must survive a restart.
func (d *Descriptor) CheckPersistent() error { return dfkit.CheckBoot(Name, d.boot) }
