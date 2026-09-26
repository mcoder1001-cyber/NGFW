package mactime

import (
	"errors"

	"ngfw/agent/internal/descriptors/dfkit"
)

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-bridge-l2 was
// ported onto main (subsystems.Register refuses undeclared descriptors).

// RecordsNoOwnership declares: a mactime device range is ours through its owner-prefixed device name.
func (*DeviceDescriptor) RecordsNoOwnership() {}

// CheckPersistent declares: the enable keeps D-080 applied-once records in the boot store and, on an
// untagged interface, a claim in the owner's claim store; both must survive an agent restart.
func (d *EnableDescriptor) CheckPersistent() error {
	return errors.Join(dfkit.CheckBoot(EnableName, d.store), dfkit.CheckClaims(EnableName, d.owner))
}
