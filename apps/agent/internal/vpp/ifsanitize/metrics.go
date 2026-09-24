package ifsanitize

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// Stats are the process-wide sanitize counters (exported by the agent's /metrics). Counters are
// keyed by phase ("create" / "delete") or by "<phase>/<state>".
type Stats struct {
	Runs        map[string]int64 // by phase
	Errors      map[string]int64 // by phase
	Inherited   map[string]int64 // by phase: runs that found bindings
	Cleared     map[string]int64 // by phase/state: bindings removed (their table existed)
	Freed       map[string]int64 // by phase/state: bindings to deleted tables removed through a placeholder
	Unclearable map[string]int64 // by phase/state
	// Quarantined is the number of sw_if_indexes this process holds in quarantine (gauge).
	Quarantined int64
	// QuarantineTotal counts quarantines.
	QuarantineTotal int64
}

func newStats() Stats {
	return Stats{Runs: map[string]int64{}, Errors: map[string]int64{}, Inherited: map[string]int64{},
		Cleared: map[string]int64{}, Freed: map[string]int64{}, Unclearable: map[string]int64{}}
}

var (
	mu    sync.Mutex
	stats = newStats()
)

// stateOf returns the state name an entry of Report.Cleared / Freed / Unclearable starts with.
func stateOf(entry string) string {
	s, _, _ := strings.Cut(entry, " ")
	return s
}

func record(r Report, err error) {
	mu.Lock()
	defer mu.Unlock()
	stats.Runs[r.Phase]++
	if err != nil {
		stats.Errors[r.Phase]++
	}
	if r.Inherited() {
		stats.Inherited[r.Phase]++
	}
	for _, e := range r.Cleared {
		stats.Cleared[r.Phase+"/"+stateOf(e)]++
	}
	for _, e := range r.Freed {
		stats.Freed[r.Phase+"/"+stateOf(e)]++
	}
	for _, e := range r.Unclearable {
		stats.Unclearable[r.Phase+"/"+stateOf(e)]++
	}
}

func recordQuarantine(delta int64) {
	mu.Lock()
	defer mu.Unlock()
	stats.Quarantined += delta
	if stats.Quarantined < 0 { // a holder made by an earlier process was released
		stats.Quarantined = 0
	}
	if delta > 0 {
		stats.QuarantineTotal += delta
	}
}

func cp(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Snapshot returns a copy of the counters.
func Snapshot() Stats {
	mu.Lock()
	defer mu.Unlock()
	return Stats{Runs: cp(stats.Runs), Errors: cp(stats.Errors), Inherited: cp(stats.Inherited), Cleared: cp(stats.Cleared),
		Freed: cp(stats.Freed), Unclearable: cp(stats.Unclearable), Quarantined: stats.Quarantined, QuarantineTotal: stats.QuarantineTotal}
}

// WriteMetrics renders the counters in the Prometheus text format (vrx_agent_iface_sanitize_*,
// vrx_agent_iface_quarantined).
func WriteMetrics(w io.Writer) {
	s := Snapshot()
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	byPhase := func(name, help string, m map[string]int64) {
		p("# HELP %s %s\n# TYPE %s counter\n", name, help, name)
		for _, k := range sortedKeys(m) {
			p("%s{phase=%q} %d\n", name, k, m[k])
		}
	}
	byState := func(name, help string, m map[string]int64) {
		p("# HELP %s %s\n# TYPE %s counter\n", name, help, name)
		for _, k := range sortedKeys(m) {
			phase, state, _ := strings.Cut(k, "/")
			p("%s{phase=%q,state=%q} %d\n", name, phase, state, m[k])
		}
	}
	byPhase("vrx_agent_iface_sanitize_total", "Interfaces sanitized (create: new sw_if_index; delete: before the interface is deleted), VPP V19/V21.", s.Runs)
	byPhase("vrx_agent_iface_sanitize_errors_total", "Sanitize runs that failed.", s.Errors)
	byPhase("vrx_agent_iface_sanitize_inherited_total", "Sanitize runs that found bindings.", s.Inherited)
	byState("vrx_agent_iface_sanitize_cleared_total", "Bindings removed, by state.", s.Cleared)
	byState("vrx_agent_iface_sanitize_freed_table_total", "Bindings to deleted classify tables removed through a resurrected placeholder, by state.", s.Freed)
	byState("vrx_agent_iface_sanitize_unclearable_total", "Bindings to deleted classify tables that could not be removed, by state.", s.Unclearable)
	p("# HELP vrx_agent_iface_quarantined sw_if_indexes held in quarantine by this process (admin-down, tag quarantine:<owner>).\n# TYPE vrx_agent_iface_quarantined gauge\nvrx_agent_iface_quarantined %d\n", s.Quarantined)
	p("# HELP vrx_agent_iface_quarantine_total Quarantines.\n# TYPE vrx_agent_iface_quarantine_total counter\nvrx_agent_iface_quarantine_total %d\n", s.QuarantineTotal)
}

func sortedKeys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
