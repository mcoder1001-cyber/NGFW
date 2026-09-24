package agent

// Prometheus metrics in the text exposition format (version 0.0.4), written by hand: the agent
// needs a handful of gauges/counters and a duration histogram, which does not justify pulling
// prometheus/client_golang (and its dependency tree) into the privileged process.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/subsystems"
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
	drift        atomic.Int64    // TD-9: objects the last drift check found
	panics       [3]atomic.Int64 // TD-9: recovered panics by panicWhere

	mu       sync.Mutex
	byStatus map[string]int64
	errors   int64
	buckets  []int64
	sum      float64
	count    int64
	ops      map[string]int64 // created/updated/deleted/reverted totals
	// dynamic desired sources (TD-8): their error family is rendered only when there are sources
	srcNames []string
	srcErrs  map[string]int64 // "<source>\x00<reason>" → count

	// collectors returns the feature metrics collectors (TD-8: subsystems.Wiring.MetricsCollectors,
	// set before the endpoint serves); nil = none.
	collectors func() []subsystems.MetricsCollector
	collMu     sync.Mutex
	collErrs   map[string]int64 // failed scrapes by collector (guarded by collMu)
}

// collectorTimeout bounds one feature collector on one scrape (a var for the tests).
var collectorTimeout = 5 * time.Second

func newMetrics() *metrics {
	return &metrics{byStatus: map[string]int64{}, buckets: make([]int64, len(durationBuckets)), ops: map[string]int64{}}
}

// panicWhere are the places a panic is recovered (TD-9, review 1.1d/1.1e): a gRPC handler, a
// descriptor call (the scheduler), the agent's own transaction code.
var panicWhere = [3]string{"grpc", "descriptor", "transaction"}

// panicked counts a recovered panic.
func (m *metrics) panicked(where string) {
	for i, w := range panicWhere {
		if w == where {
			m.panics[i].Add(1)
		}
	}
}

func (m *metrics) setDrift(n int) { m.drift.Store(int64(n)) }

func (m *metrics) setVPP(v bool)      { m.vppConnected.Store(v) }
func (m *metrics) setDegraded(v bool) { m.degraded.Store(v) }
func (m *metrics) setPending(v bool)  { m.pending.Store(v) }
func (m *metrics) setObjects(n int)   { m.objects.Store(int64(n)) }
func (m *metrics) setWriteOnly(descs, objs int) {
	m.woDescs.Store(int64(descs))
	m.woObjects.Store(int64(objs))
}

// setSources names the dynamic desired sources (NewService).
func (m *metrics) setSources(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.srcNames = append([]string(nil), names...)
	sort.Strings(m.srcNames)
}

// sourceError counts a dynamic source failure (reason: srcReasons).
func (m *metrics) sourceError(source, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.srcErrs == nil {
		m.srcErrs = map[string]int64{}
	}
	m.srcErrs[source+"\x00"+reason]++
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

// write renders the exposition text (no deadline for the feature collectors beyond collectorTimeout).
func (m *metrics) write(w io.Writer) { m.writeCtx(context.Background(), w) }

// writeCtx renders the agent's families, then the feature collectors' (outside m.mu). The agent's
// families are rendered into memory first: a stalled scraper never holds m.mu, which observe takes
// under the transaction lock.
func (m *metrics) writeCtx(ctx context.Context, w io.Writer) {
	var agent bytes.Buffer
	m.writeAgent(&agent)
	_, _ = w.Write(agent.Bytes())
	m.writeCollectors(ctx, w)
}

// runCollector runs one collector; a panic is its error (TD-8 review R3), never an agent crash.
func runCollector(ctx context.Context, c subsystems.MetricsCollector, w io.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("metrics collector %s panicked: %v", c.Name, r)
		}
	}()
	return c.Collect(ctx, w)
}

// writeCollectors renders the feature collectors (TD-8) and their error counter; nothing without
// collectors. A collector that fails (error or deadline) contributes nothing but a count.
func (m *metrics) writeCollectors(ctx context.Context, w io.Writer) {
	if m.collectors == nil {
		return
	}
	cs := m.collectors()
	if len(cs) == 0 {
		return
	}
	for _, c := range cs {
		var buf bytes.Buffer
		cctx, cancel := context.WithTimeout(ctx, collectorTimeout)
		err := runCollector(cctx, c, &buf) // a collector that gives up on its deadline returns ctx.Err()
		cancel()
		if err != nil {
			m.collMu.Lock()
			if m.collErrs == nil {
				m.collErrs = map[string]int64{}
			}
			m.collErrs[c.Name]++
			m.collMu.Unlock()
			continue
		}
		if b := buf.Bytes(); len(b) > 0 && b[len(b)-1] != '\n' {
			buf.WriteByte('\n')
		}
		_, _ = w.Write(buf.Bytes())
	}
	_, _ = io.WriteString(w, "# HELP vrx_agent_metrics_collector_errors_total Scrapes on which a feature metrics collector failed (its families were not served).\n# TYPE vrx_agent_metrics_collector_errors_total counter\n")
	m.collMu.Lock()
	defer m.collMu.Unlock()
	for _, c := range cs {
		_, _ = fmt.Fprintf(w, "vrx_agent_metrics_collector_errors_total{collector=%q} %d\n", c.Name, m.collErrs[c.Name])
	}
}

// writeAgent renders the agent's own families.
func (m *metrics) writeAgent(w io.Writer) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	p("# HELP vrx_agent_vpp_connected 1 when the VPP binary API is connected.\n# TYPE vrx_agent_vpp_connected gauge\nvrx_agent_vpp_connected %d\n", b2f(m.vppConnected.Load()))
	p("# HELP vrx_agent_degraded 1 after a failed rollback until a transaction succeeds.\n# TYPE vrx_agent_degraded gauge\nvrx_agent_degraded %d\n", b2f(m.degraded.Load()))
	p("# HELP vrx_agent_confirm_pending 1 while a transaction awaits confirmation.\n# TYPE vrx_agent_confirm_pending gauge\nvrx_agent_confirm_pending %d\n", b2f(m.pending.Load()))
	p("# HELP vrx_agent_objects Desired objects of the last reconcile.\n# TYPE vrx_agent_objects gauge\nvrx_agent_objects %d\n", m.objects.Load())
	p("# HELP vrx_agent_confirm_reverts_total Confirm timeouts that reverted a transaction.\n# TYPE vrx_agent_confirm_reverts_total counter\nvrx_agent_confirm_reverts_total %d\n", m.reverts.Load())
	p("# HELP vrx_agent_retrieve_unsupported_descriptors Write-only descriptors (no VPP dump, D-063).\n# TYPE vrx_agent_retrieve_unsupported_descriptors gauge\nvrx_agent_retrieve_unsupported_descriptors %d\n", m.woDescs.Load())
	p("# HELP vrx_agent_retrieve_unsupported_objects Objects of write-only descriptors applied by this process (D-063).\n# TYPE vrx_agent_retrieve_unsupported_objects gauge\nvrx_agent_retrieve_unsupported_objects %d\n", m.woObjects.Load())
	p("# HELP vrx_agent_drift_objects Objects of the stored desired state that differ from VPP at the last drift check (a Plan, nothing applied).\n# TYPE vrx_agent_drift_objects gauge\nvrx_agent_drift_objects %d\n", m.drift.Load())
	p("# HELP vrx_agent_panics_total Panics recovered: in a gRPC handler, a descriptor call, or the agent's transaction code.\n# TYPE vrx_agent_panics_total counter\n")
	for i, w := range panicWhere {
		p("vrx_agent_panics_total{where=%q} %d\n", w, m.panics[i].Load())
	}
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
	if len(m.srcNames) > 0 {
		p("# HELP vrx_agent_dynamic_source_errors_total Dynamic desired source failures: left out of a transaction, or its own sync failed (reason invalid, panic, rejected, stopped).\n# TYPE vrx_agent_dynamic_source_errors_total counter\n")
		for _, n := range m.srcNames {
			for _, r := range srcReasons {
				p("vrx_agent_dynamic_source_errors_total{source=%q,reason=%q} %d\n", n, r, m.srcErrs[n+"\x00"+r])
			}
		}
	}
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
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.writeCtx(r.Context(), w)
	})
	return mux
}
