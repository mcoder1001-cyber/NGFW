// Package multiwan holds the link-health hysteresis for F-multiwan: given a WAN member's monitor configuration and the
// result of each health check, it decides when the member transitions down and back up. It is pure (no I/O, no clock
// beyond what the caller passes), so the failover behaviour is unit-testable without a data plane. The probing itself
// (ICMP/HTTP/DNS sourced from each member link) and the routing/NAT reaction are the agent's host-side wiring.
package multiwan

// MonitorConfig is the thresholds of one monitor (mirrors schema WanMonitor; only what the decision needs).
type MonitorConfig struct {
	// LossPct: a check is unhealthy when probe loss over the check reaches this percent (0..100).
	LossPct int
	// LatencyMs: a check is unhealthy above this average latency; 0 = no latency check.
	LatencyMs int
	// DownAfter / UpAfter: consecutive unhealthy / healthy checks before the member flips (hysteresis).
	DownAfter, UpAfter int
}

// CheckResult is the outcome of one health check (a small batch of probes).
type CheckResult struct {
	Sent, Received int
	// AvgLatencyMs over the received probes; 0 when none were received.
	AvgLatencyMs int
}

// Healthy reports whether one check passed the monitor's thresholds.
func (c MonitorConfig) Healthy(r CheckResult) bool {
	if r.Sent == 0 || r.Received == 0 {
		return false
	}
	loss := (r.Sent - r.Received) * 100 / r.Sent
	if loss >= c.LossPct {
		return false
	}
	if c.LatencyMs > 0 && r.AvgLatencyMs > c.LatencyMs {
		return false
	}
	return true
}

// State is one member's health as the hysteresis sees it. The zero value is a member that is up with no history.
type State struct {
	Up         bool
	okStreak   int
	failStreak int
	seen       bool
}

// NewState returns a member starting up (a member is assumed reachable until a check says otherwise).
func NewState() State { return State{Up: true} }

// Observe folds one check result in and reports whether the member's up/down state changed. A member goes down after
// DownAfter consecutive unhealthy checks and comes back up after UpAfter consecutive healthy ones; the opposite streak
// resets on each check.
func (s *State) Observe(cfg MonitorConfig, r CheckResult) (changed bool) {
	if !s.seen {
		s.seen = true
	}
	if cfg.Healthy(r) {
		s.okStreak++
		s.failStreak = 0
		if !s.Up && s.okStreak >= max1(cfg.UpAfter) {
			s.Up = true
			return true
		}
	} else {
		s.failStreak++
		s.okStreak = 0
		if s.Up && s.failStreak >= max1(cfg.DownAfter) {
			s.Up = false
			return true
		}
	}
	return false
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// Member is a WAN member with its failover attributes and current health (for the routing decision).
type Member struct {
	Interface string
	Weight    int
	Priority  int
	Up        bool
}

// FailoverActive returns the interface that should carry the default route in failover mode: the healthy member with
// the lowest priority (ties broken by interface name). "" when none is healthy.
func FailoverActive(members []Member) string {
	best := ""
	bestPri := 0
	for _, m := range members {
		if !m.Up {
			continue
		}
		if best == "" || m.Priority < bestPri || (m.Priority == bestPri && m.Interface < best) {
			best, bestPri = m.Interface, m.Priority
		}
	}
	return best
}

// BalancePaths returns the healthy members and their weights for weighted ECMP (balance mode), sorted by interface.
func BalancePaths(members []Member) []Member {
	var out []Member
	for _, m := range members {
		if m.Up && m.Weight > 0 {
			out = append(out, m)
		}
	}
	sortByInterface(out)
	return out
}

func sortByInterface(m []Member) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j].Interface < m[j-1].Interface; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}
