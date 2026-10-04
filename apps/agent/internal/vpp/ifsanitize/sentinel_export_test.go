package ifsanitize

// SetMaxSentinelPops sets MaxSentinelPops for a test and returns the function that restores it.
func SetMaxSentinelPops(n int) func() {
	old := MaxSentinelPops
	MaxSentinelPops = n
	return func() { MaxSentinelPops = old }
}
