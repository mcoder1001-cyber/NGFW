package agent

// Prometheus metrics in the text exposition format (version 0.0.4), written by hand: the agent
// needs a handful of gauges/counters and a duration histogram, which does not justify pulling
// prometheus/client_golang (and its dependency tree) into the privileged process.

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/vpp/ifsanitize"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

type metrics struct {
	vppConnected atomic.Bool
	degraded     atomic.Bool
	pending      atomic.Bool
	objects      atomic.Int64
	reverts      atomic.Int64
	woDescs      atomic.Int64
	woObjects    atomic.Int64

	mu       sync.Mutex
	byStatus map[string]int64
	errors   int64
	buckets  []int64
	sum      float64
	count    int64
	ops      map[string]int64 // created/updated/deleted/reverted totals
}

func newMetrics() *metrics {
	return &metrics{byStatus: map[string]int64{}, buckets: make([]int64, len(durationBuckets)), ops: map[string]int64{}}
}

func (m *metrics) setVPP(v bool)      { m.vppConnected.Store(v) }
func (m *metrics) setDegraded(v bool) { m.degraded.Store(v) }
func (m *metrics) setPending(v bool)  { m.pending.Store(v) }
func (m *metrics) setObjects(n int)   { m.objects.Store(int64(n)) }
func (m *metrics) setWriteOnly(descs, objs int) {
	m.woDescs.Store(int64(descs))
	m.woObjects.Store(int64(objs))
}

func (m *metrics) observe(st vrxv1.ApplyStatus, d time.Duration, s *vrxv1.ApplySummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byStatus[st.String()]++
	if st != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		m.errors++
	}
	sec := d.Seconds()
	for i, b := range durationBuckets {
		if sec <= b {
			m.buckets[i]++
		}
	}
	m.sum += sec
	m.count++
	m.ops["created"] += int64(s.GetCreated())
	m.ops["updated"] += int64(s.GetUpdated())
	m.ops["deleted"] += int64(s.GetDeleted())
	m.ops["reverted"] += int64(s.GetReverted())
	m.ops["failed"] += int64(s.GetFailed())
}

func b2f(b bool) int {
	if b {
		return 1
	}
	return 0
}

// write renders the exposition text.
func (m *metrics) write(w io.Writer) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	p("# HELP vrx_agent_vpp_connected 1 when the VPP binary API is connected.\n# TYPE vrx_agent_vpp_connected gauge\nvrx_agent_vpp_connected %d\n", b2f(m.vppConnected.Load()))
	p("# HELP vrx_agent_degraded 1 after a failed rollback until a transaction succeeds.\n# TYPE vrx_agent_degraded gauge\nvrx_agent_degraded %d\n", b2f(m.degraded.Load()))
	p("# HELP vrx_agent_confirm_pending 1 while a transaction awaits confirmation.\n# TYPE vrx_agent_confirm_pending gauge\nvrx_agent_confirm_pending %d\n", b2f(m.pending.Load()))
	p("# HELP vrx_agent_objects Desired objects of the last reconcile.\n# TYPE vrx_agent_objects gauge\nvrx_agent_objects %d\n", m.objects.Load())
	p("# HELP vrx_agent_confirm_reverts_total Confirm timeouts that reverted a transaction.\n# TYPE vrx_agent_confirm_reverts_total counter\nvrx_agent_confirm_reverts_total %d\n", m.reverts.Load())
	p("# HELP vrx_agent_retrieve_unsupported_descriptors Write-only descriptors (no VPP dump, D-063).\n# TYPE vrx_agent_retrieve_unsupported_descriptors gauge\nvrx_agent_retrieve_unsupported_descriptors %d\n", m.woDescs.Load())
	p("# HELP vrx_agent_retrieve_unsupported_objects Objects of write-only descriptors applied by this process (D-063).\n# TYPE vrx_agent_retrieve_unsupported_objects gauge\nvrx_agent_retrieve_unsupported_objects %d\n", m.woObjects.Load())
	m.mu.Lock()
	defer m.mu.Unlock()
	p("# HELP vrx_agent_reconcile_total Reconcile transactions by outcome.\n# TYPE vrx_agent_reconcile_total counter\n")
	statuses := make([]string, 0, len(m.byStatus))
	for k := range m.byStatus {
		statuses = append(statuses, k)
	}
	sort.Strings(statuses)
	for _, k := range statuses {
		p("vrx_agent_reconcile_total{status=%q} %d\n", k, m.byStatus[k])
	}
	p("# HELP vrx_agent_reconcile_errors_total Reconcile transactions that did not end APPLIED.\n# TYPE vrx_agent_reconcile_errors_total counter\nvrx_agent_reconcile_errors_total %d\n", m.errors)
	p("# HELP vrx_agent_reconcile_operations_total Object operations by kind.\n# TYPE vrx_agent_reconcile_operations_total counter\n")
	for _, k := range []string{"created", "updated", "deleted", "reverted", "failed"} {
		p("vrx_agent_reconcile_operations_total{op=%q} %d\n", k, m.ops[k])
	}
	p("# HELP vrx_agent_reconcile_duration_seconds Duration of reconcile transactions.\n# TYPE vrx_agent_reconcile_duration_seconds histogram\n")
	for i, b := range durationBuckets {
		p("vrx_agent_reconcile_duration_seconds_bucket{le=%q} %d\n", fmtFloat(b), m.buckets[i])
	}
	p("vrx_agent_reconcile_duration_seconds_bucket{le=\"+Inf\"} %d\n", m.count)
	p("vrx_agent_reconcile_duration_seconds_sum %s\nvrx_agent_reconcile_duration_seconds_count %d\n", fmtFloat(m.sum), m.count)
	ifsanitize.WriteMetrics(w) // D-095: inherited per-interface state cleared on new interfaces
}

func fmtFloat(f float64) string {
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	return fmt.Sprintf("%g", f)
}

// handler serves /metrics.
func (m *metrics) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.write(w)
	})
	return mux
}
