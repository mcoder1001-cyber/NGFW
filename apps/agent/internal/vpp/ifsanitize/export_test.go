package ifsanitize

// DisableResurrect turns resurrection off, so a binding a readback names (input ACL, policer,
// flow classify) to a freed table is unclearable without the run being capped (the
// quarantine-and-retry path of Acquire), and returns the function that turns it back on. The
// output ACL check still uses its probe table: when that placeholder lands on the freed index an
// output ACL slot names, the slot is cleared through it.
func DisableResurrect() func() {
	noResurrect = true
	return func() { noResurrect = false }
}
