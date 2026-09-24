package ifsanitize

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// Stats are the process-wide sanitize counters (exported by the agent's /metrics).
type Stats struct {
	Runs      int64            // Sanitize calls
	Errors    int64            // calls that failed (the creator removed the interface)
	Inherited int64            // calls that found inherited state
	Cleared   map[string]int64 // removed inherited bindings by state
	Stale     map[string]int64 // unclearable (dormant) bindings to deleted tables by state
}

var (
	mu    sync.Mutex
	stats = Stats{Cleared: map[string]int64{}, Stale: map[string]int64{}}
)

// stateOf returns the state name an entry of Report.Cleared / Unclearable starts with.
func stateOf(entry string) string {
	s, _, _ := strings.Cut(entry, " ")
	return s
}

func record(r Report, err error) {
	mu.Lock()
	defer mu.Unlock()
	stats.Runs++
	if err != nil {
		stats.Errors++
	}
	if r.Inherited() {
		stats.Inherited++
	}
	for _, e := range r.Cleared {
		stats.Cleared[stateOf(e)]++
	}
	for _, e := range r.Unclearable {
		stats.Stale[stateOf(e)]++
	}
}

// Snapshot returns a copy of the counters.
func Snapshot() Stats {
	mu.Lock()
	defer mu.Unlock()
	out := Stats{Runs: stats.Runs, Errors: stats.Errors, Inherited: stats.Inherited, Cleared: map[string]int64{}, Stale: map[string]int64{}}
	for k, v := range stats.Cleared {
		out.Cleared[k] = v
	}
	for k, v := range stats.Stale {
		out.Stale[k] = v
	}
	return out
}

// WriteMetrics renders the counters in the Prometheus text format (vrx_agent_iface_sanitize_*).
func WriteMetrics(w io.Writer) {
	s := Snapshot()
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	p("# HELP vrx_agent_iface_sanitize_total New interfaces checked for inherited per-interface state (VPP V19/V21).\n# TYPE vrx_agent_iface_sanitize_total counter\nvrx_agent_iface_sanitize_total %d\n", s.Runs)
	p("# HELP vrx_agent_iface_sanitize_errors_total Sanitize runs that failed (the new interface was removed).\n# TYPE vrx_agent_iface_sanitize_errors_total counter\nvrx_agent_iface_sanitize_errors_total %d\n", s.Errors)
	p("# HELP vrx_agent_iface_sanitize_inherited_total New interfaces that had inherited state.\n# TYPE vrx_agent_iface_sanitize_inherited_total counter\nvrx_agent_iface_sanitize_inherited_total %d\n", s.Inherited)
	p("# HELP vrx_agent_iface_sanitize_cleared_total Inherited bindings removed, by state.\n# TYPE vrx_agent_iface_sanitize_cleared_total counter\n")
	for _, k := range sortedKeys(s.Cleared) {
		p("vrx_agent_iface_sanitize_cleared_total{state=%q} %d\n", k, s.Cleared[k])
	}
	p("# HELP vrx_agent_iface_sanitize_unclearable_total Inherited bindings to deleted classify tables left dormant, by state.\n# TYPE vrx_agent_iface_sanitize_unclearable_total counter\n")
	for _, k := range sortedKeys(s.Stale) {
		p("vrx_agent_iface_sanitize_unclearable_total{state=%q} %d\n", k, s.Stale[k])
	}
}

func sortedKeys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
