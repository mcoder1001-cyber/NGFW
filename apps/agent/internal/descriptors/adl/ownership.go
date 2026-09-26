package adl

import (
	"errors"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/persist"
)

// Ownership declarations for the TD-11b persistence guard (dfkit/persist), added when F-rpf-adl-pbr was ported
// onto main (subsystems.Register refuses undeclared descriptors).

// CheckPersistent declares: ADL on an untagged interface is ours through the DF-2 claim store.
func (d *InterfaceDescriptor) CheckPersistent() error { return d.opts.CheckPersistent(InterfaceName) }

// CheckPersistent declares: an allow-list on an untagged interface is ours through the claim store, and the
// D-076 applied-once records live in the boot store; both must survive an agent restart.
func (d *AllowlistDescriptor) CheckPersistent() error {
	return errors.Join(persist.Require(AllowlistName+": allow-list claims", d.o.claims), dfkit.CheckBoot(AllowlistName, d.o.boot))
}
