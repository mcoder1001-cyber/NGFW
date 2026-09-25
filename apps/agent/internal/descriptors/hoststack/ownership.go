package hoststack

import "ngfw/agent/internal/descriptors/dfkit"

// Ownership declarations for the TD-11b persistence guard (dfkit/persist).

// RecordsNoOwnership declares that nothing is recorded: the session layer is a VPP global.
func (*SessionDescriptor) RecordsNoOwnership() {}

// CheckPersistent requires a persisted boot store: namespace indexes are recorded in the boot store (rules resolve through it).
func (d *NamespaceDescriptor) CheckPersistent() error {
	return dfkit.CheckBoot(NameNamespace, d.st.boot)
}

// CheckPersistent requires a persisted boot store: rules carry the owner-prefixed tag, their namespace indexes come from the boot store.
func (d *RuleDescriptor) CheckPersistent() error { return dfkit.CheckBoot(NameSessionRule, d.st.boot) }

// CheckPersistent requires a persisted boot store (D-076 applied-once records).
func (d *TCPSrcDescriptor) CheckPersistent() error { return dfkit.CheckBoot(NameTCPSrc, d.boot) }

// CheckPersistent requires a persisted boot store (D-076 applied-once records).
func (d *HTTPStaticDescriptor) CheckPersistent() error {
	return dfkit.CheckBoot(NameHTTPStatic, d.boot)
}
