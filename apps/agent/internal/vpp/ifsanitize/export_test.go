package ifsanitize

// DisableResurrect turns resurrection off, so a binding to a freed table is unclearable without
// the run being capped (the quarantine-and-retry path of Acquire), and returns the function that
// turns it back on.
func DisableResurrect() func() {
	noResurrect = true
	return func() { noResurrect = false }
}
