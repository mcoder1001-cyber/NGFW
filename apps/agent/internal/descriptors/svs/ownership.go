package svs

// Ownership declarations for the product agent's guard (dfkit/persist, TD-11b fix round 1): every registered
// descriptor says how it records ownership.

import "ngfw/agent/internal/descriptors/dfkit"

// RecordsNoOwnership declares that an svs table is ours by its VPP name "<owner>:svs:<id>" (ip_table_add_del).
func (*TableDescriptor) RecordsNoOwnership() {}

// RecordsNoOwnership declares that an svs enablement is ours when its svs table is (found by key in svs_dump).
func (*InterfaceDescriptor) RecordsNoOwnership() {}

// CheckPersistent requires a persisted BootStore: the table an svs route selects is not in the dump, so it is
// kept as a D-076 applied-once record (Wiring.BootStore; the in-memory default is for unit tests).
func (d *RouteDescriptor) CheckPersistent() error { return dfkit.CheckBoot(RouteName, d.Boot) }
