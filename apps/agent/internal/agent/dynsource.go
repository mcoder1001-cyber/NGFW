package agent

// Dynamic desired sources (S1, TD-8; subsystems.DynamicSource): readiness, the merge into a
// transaction, the config-only fallback, the source's own sync, its retry, and panic containment
// (TD-8 review R1–R3, R6). A source never costs the configuration its transaction:
//   - A source takes part in transactions only while it is in sync (its last sync ended APPLIED and no
//     transaction left it out since). Every source starts out of sync, so the first resync after an
//     agent restart neither creates nor deletes its objects (they are out of scope).
//   - A transaction that fails because of a dynamic object runs once more without the sources.
//   - A panic in Desired, in a sync or in Run is recovered.

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// Reasons of vrx_agent_dynamic_source_errors_total.
const (
	srcInvalid  = "invalid"  // Desired returned a key outside its descriptors, or a duplicate
	srcPanic    = "panic"    // Desired, a sync or Run panicked
	srcRejected = "rejected" // a transaction or the source's sync failed on its objects
	srcStopped  = "stopped"  // Run returned before the agent stopped
)

var srcReasons = []string{srcInvalid, srcPanic, srcRejected, srcStopped}

// dynSource is one dynamic desired source with its readiness state.
type dynSource struct {
	subsystems.DynamicSource
	own map[string]bool // Descriptors as a set

	// inSync: merged into every transaction (written under the txn lock, read by DryRun without it).
	inSync atomic.Bool
	// guarded by the txn lock:
	stopped bool          // Run panicked or returned early: out of sync for the life of the process
	retry   *time.Timer   // the agent's retry sync while out of sync
	delay   time.Duration // its backoff (Service.retryMin doubling to retryMax)
}

func newDynSources(srcs []subsystems.DynamicSource, m *metrics) []*dynSource {
	out := make([]*dynSource, 0, len(srcs))
	names := make([]string, 0, len(srcs))
	for _, src := range srcs {
		src.Descriptors = append([]string(nil), src.Descriptors...)
		ds := &dynSource{DynamicSource: src, own: map[string]bool{}}
		for _, d := range src.Descriptors {
			ds.own[d] = true
		}
		out = append(out, ds)
		names = append(names, src.Name)
	}
	m.setSources(names)
	return out
}

// checkSources refuses dynamic sources whose descriptors are not registered with the scheduler.
func checkSources(sched *scheduler.Scheduler, srcs []subsystems.DynamicSource) error {
	for _, src := range srcs {
		if src.Desired == nil {
			return fmt.Errorf("dynamic source %s: no Desired", src.Name)
		}
		for _, d := range src.Descriptors {
			if sched == nil {
				return fmt.Errorf("dynamic source %s: no scheduler", src.Name)
			}
			if _, ok := sched.Registry().Get(d); !ok {
				return fmt.Errorf("dynamic source %s: descriptor %s is not registered", src.Name, d)
			}
		}
	}
	return nil
}

// scopeNames lists the descriptors of domains (scopeOf's names).
func scopeNames(domains []string) []string {
	var names []string
	for _, d := range domains {
		names = append(names, domainDescriptors[d]...)
	}
	return names
}

func (s *Service) source(name string) *dynSource {
	for _, ds := range s.sources {
		if ds.Name == name {
			return ds
		}
	}
	return nil
}

// activeSources are the sources in sync: the ones a transaction merges.
func (s *Service) activeSources() []*dynSource {
	var out []*dynSource
	for _, ds := range s.sources {
		if ds.inSync.Load() {
			out = append(out, ds)
		}
	}
	return out
}

// ---- merge --------------------------------------------------------------------------------------

// leftOut is a source a transaction left out: why, and the dynamic key when one object caused it.
type leftOut struct {
	src    *dynSource
	key    scheduler.Key
	op     string
	reason string // srcInvalid, srcPanic, srcRejected; "" = left out together with the failing source
	cause  string
}

// srcMerge is what the dynamic sources add to one transaction.
type srcMerge struct {
	kvs    []scheduler.KV        // the configuration's objects, then the merged sources' objects
	names  []string              // the merged sources' descriptors (they join the scope)
	merged []*dynSource          // the sources whose objects are in kvs
	owner  map[string]*dynSource // descriptor → merged source
	left   []leftOut             // sources left out before the transaction (invalid output, panic)
}

// scope is the transaction's scope with the merged sources: domains' descriptors plus theirs.
func (mg *srcMerge) scope(domains []string) scheduler.Scope {
	return scheduler.Only(append(scopeNames(domains), mg.names...)...)
}

// mergeSources adds the objects of srcs for view (the stored document as it will be after the
// transaction; each source gets its own copy) to base. A source whose Desired panics, returns a key
// outside its descriptors or a duplicate is left out as a whole; dynamic objects have no JSON pointer.
func (s *Service) mergeSources(base []scheduler.KV, view *vrxv1.DesiredState, srcs []*dynSource) *srcMerge {
	if view == nil {
		view = &vrxv1.DesiredState{}
	}
	mg := &srcMerge{kvs: append([]scheduler.KV(nil), base...), owner: map[string]*dynSource{}}
	have := make(map[scheduler.Key]bool, len(base))
	for _, kv := range base {
		have[kv.Key] = true
	}
	for _, ds := range srcs {
		kvs, err := s.desiredOf(ds, view)
		if err != nil {
			mg.left = append(mg.left, leftOut{src: ds, reason: srcPanic, cause: err.Error()})
			continue
		}
		if bad := checkKVs(ds, kvs, have); bad != nil {
			mg.left = append(mg.left, *bad)
			continue
		}
		for _, kv := range kvs {
			have[kv.Key] = true
			mg.kvs = append(mg.kvs, scheduler.KV{Key: kv.Key, Value: kv.Value})
		}
		mg.names = append(mg.names, ds.Descriptors...)
		mg.merged = append(mg.merged, ds)
		for _, d := range ds.Descriptors {
			mg.owner[d] = ds
		}
	}
	return mg
}

// checkKVs finds the first key of ds's output outside its descriptors, or already planned.
func checkKVs(ds *dynSource, kvs []scheduler.KV, have map[scheduler.Key]bool) *leftOut {
	mine := make(map[scheduler.Key]bool, len(kvs))
	for _, kv := range kvs {
		switch {
		case !ds.own[kv.Key.Descriptor()]:
			return &leftOut{src: ds, key: kv.Key, reason: srcInvalid, cause: fmt.Sprintf("produced %s, outside its descriptors %v", kv.Key, ds.Descriptors)}
		case have[kv.Key] || mine[kv.Key]:
			return &leftOut{src: ds, key: kv.Key, reason: srcInvalid, cause: fmt.Sprintf("duplicate object %s", kv.Key)}
		}
		mine[kv.Key] = true
	}
	return nil
}

// desiredOf calls ds.Desired for a copy of view (never the stored state) and turns a panic into an
// error (review R3): a buggy source must not crash the agent, nor crash-loop it through the first
// resync.
func (s *Service) desiredOf(ds *dynSource, view *vrxv1.DesiredState) (kvs []scheduler.KV, err error) {
	g := goid()
	s.inDesired.Store(g, true) // the re-entrancy guard of sync (DryRun calls Desired without the lock)
	defer s.inDesired.Delete(g)
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("dynamic source: Desired panicked", "source", ds.Name, "panic", r, "stack", string(debug.Stack()))
			kvs, err = nil, fmt.Errorf("its Desired panicked: %v", r)
		}
	}()
	return ds.Desired(proto.Clone(view).(*vrxv1.DesiredState)), nil
}

// culprit finds the merged source whose object made res fail (review R2): a plan issue or the failed
// operation on one of its keys, or — when no key tells (a failed Retrieve or verification) — one of
// its descriptors named in the error. ok is false when the configuration failed on its own, and after
// APPLIED or DEGRADED (after a failed rollback nothing is retried).
func culprit(res *scheduler.TxnResult, mg *srcMerge) (leftOut, bool) {
	if res.Outcome != scheduler.OutcomeFailed && res.Outcome != scheduler.OutcomeRolledBack {
		return leftOut{}, false
	}
	if res.Plan != nil {
		for _, is := range res.Plan.Issues {
			if ds := mg.owner[is.Key.Descriptor()]; ds != nil {
				return leftOut{src: ds, key: is.Key, reason: srcRejected, cause: is.Message}, true
			}
		}
		if len(res.Plan.Issues) > 0 {
			return leftOut{}, false
		}
	}
	for _, r := range res.Results {
		if r.Code != scheduler.CodeFailed || r.Key == "" {
			continue
		}
		if ds := mg.owner[r.Key.Descriptor()]; ds != nil {
			return leftOut{src: ds, key: r.Key, op: r.Op, reason: srcRejected, cause: errText(r.Err)}, true
		}
		return leftOut{}, false // the first failed operation is the configuration's
	}
	if res.Err != nil {
		msg := res.Err.Error()
		for _, ds := range mg.merged {
			for _, d := range ds.Descriptors {
				if strings.Contains(msg, d+"/") || strings.Contains(msg, "retrieve "+d+":") {
					return leftOut{src: ds, reason: srcRejected, cause: msg}, true
				}
			}
		}
	}
	return leftOut{}, false
}

// errText is err's text, "" for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// applySources runs the transaction kvs/scope (the configuration) with the sources in sync merged in
// (caller holds txn). When a source cannot be merged, it is left out; when the merged transaction
// fails because of a dynamic object, it runs once more without the sources (review R2). left lists
// every source left out.
func (s *Service) applySources(ctx context.Context, kvs []scheduler.KV, scope scheduler.Scope, domains []string, view *vrxv1.DesiredState, opts scheduler.ApplyOptions) (res *scheduler.TxnResult, left []leftOut) {
	active := s.activeSources()
	if len(active) == 0 {
		return s.sched.ApplyWith(ctx, kvs, scope, opts), nil
	}
	mg := s.mergeSources(kvs, view, active)
	left = mg.left
	if len(mg.merged) == 0 {
		return s.sched.ApplyWith(ctx, kvs, scope, opts), left
	}
	res = s.sched.ApplyWith(ctx, mg.kvs, mg.scope(domains), opts)
	lo, ok := culprit(res, mg)
	if !ok || ctx.Err() != nil { // a cancelled transaction is not the source's fault
		return res, left
	}
	left = append(left, lo)
	for _, ds := range mg.merged {
		if ds != lo.src {
			left = append(left, leftOut{src: ds})
		}
	}
	s.log.Warn("transaction failed because of a dynamic object: running it again without the dynamic sources",
		"source", lo.src.Name, "key", lo.key, "cause", lo.cause, "first_outcome", res.Outcome.String())
	return s.sched.ApplyWith(ctx, kvs, scope, opts), left
}

// planSources is DryRun's plan (no lock, no state change): what Apply would do, with the sources in
// sync merged for the stored document after the transaction. A source Apply would leave out at the
// plan stage (a panic, a key outside its descriptors, a plan issue on one of its keys) becomes a
// WARNING issue "agent.dynamic-source-skipped" in pj, and the plan is the configuration's alone.
func (s *Service) planSources(ctx context.Context, pj *projected, domains []string, update *vrxv1.DesiredState) (*scheduler.TxnPlan, error) {
	scope := scopeOf(domains)
	active := s.activeSources()
	if len(active) == 0 {
		return s.sched.Plan(ctx, pj.kvs, scope)
	}
	s.mu.Lock()
	stored := s.storedDoc
	s.mu.Unlock()
	mg := s.mergeSources(pj.kvs, mergeDomains(stored, update, domains), active)
	warn := func(lo leftOut) {
		pj.warnf("", "agent.dynamic-source-skipped", "dynamic source %s would be left out of this transaction (%s): %s", lo.src.Name, lo.reason, lo.cause)
	}
	for _, lo := range mg.left {
		warn(lo)
	}
	if len(mg.merged) == 0 {
		return s.sched.Plan(ctx, pj.kvs, scope)
	}
	plan, err := s.sched.Plan(ctx, mg.kvs, mg.scope(domains))
	res := &scheduler.TxnResult{Outcome: scheduler.OutcomeFailed, Plan: plan, Err: err}
	if lo, ok := culprit(res, mg); ok {
		warn(lo)
		return s.sched.Plan(ctx, pj.kvs, scope)
	}
	return plan, err
}

// leaveOutLocked records the sources a transaction left out (caller holds txn): each is out of sync
// until its next successful sync, which the agent retries with backoff. A source that caused it gets
// a SKIPPED result in resp (when one object did), an ERROR event, the error counter and a log line.
func (s *Service) leaveOutLocked(resp *vrxv1.ApplyResponse, txnID string, left []leftOut, log *slog.Logger) {
	for _, lo := range left {
		lo.src.inSync.Store(false)
		s.retrySourceLocked(lo.src)
		if lo.reason == "" {
			log.Warn("dynamic source left out of the transaction together with the failing one", "source", lo.src.Name)
			continue
		}
		msg := fmt.Sprintf("dynamic source %s left out of this transaction (%s): %s", lo.src.Name, lo.reason, lo.cause)
		if lo.key != "" && resp != nil {
			resp.Results = append(resp.Results, &vrxv1.ObjectResult{Key: string(lo.key), Op: opPB[lo.op], Code: vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_SKIPPED, Message: msg})
		}
		s.sourceError(lo.src, lo.reason, txnID, lo.key, msg)
	}
}

// sourceError counts, publishes (ERROR, attributes source/reason/key) and logs a source failure.
func (s *Service) sourceError(ds *dynSource, reason, txnID string, key scheduler.Key, msg string) {
	s.metrics.sourceError(ds.Name, reason)
	attrs := map[string]string{"source": ds.Name, "reason": reason}
	if key != "" {
		attrs["key"] = string(key)
	}
	s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_ERROR, TxnId: txnID, Message: msg, Attributes: attrs})
	s.log.Warn(msg, "source", ds.Name, "reason", reason, "key", key, "txn_id", txnID)
}

// ---- sync ---------------------------------------------------------------------------------------

// sourceSync returns the SyncFunc the loop of the dynamic source name gets.
func (s *Service) sourceSync(name string) subsystems.SyncFunc {
	return func(ctx context.Context) error { return s.syncSource(ctx, name) }
}

// syncSource runs one transaction for the dynamic source name (the SyncFunc): its Desired for the
// stored document, scoped to its descriptors, under the transaction lock. It never changes the stored
// document or the confirm state, and its success does not clear DEGRADED (only a transaction over the
// configuration does). Called inside a transaction — from Desired or a descriptor call, on the
// goroutine that holds the lock — it is refused at once instead of deadlocking (review R6).
func (s *Service) syncSource(ctx context.Context, name string) (err error) {
	ds := s.source(name)
	if ds == nil {
		return fmt.Errorf("no dynamic source %q", name)
	}
	if s.insideTxn() {
		return status.Errorf(codes.FailedPrecondition, "sync of dynamic source %s called inside a transaction (from Desired or a descriptor call): only its Run may call sync", name)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	if ds.stopped {
		return status.Errorf(codes.FailedPrecondition, "dynamic source %s is stopped until the agent restarts (its Run ended)", name)
	}
	defer func() { // a panic in a descriptor of the source (Desired's own is recovered by desiredOf)
		if r := recover(); r != nil {
			s.log.Error("dynamic source: sync panicked", "source", name, "panic", r, "stack", string(debug.Stack()))
			ds.inSync.Store(false)
			s.metrics.sourceError(name, srcPanic)
			s.setDegraded(true, fmt.Sprintf("dynamic source %s: its sync panicked, the data plane may be partially changed: %v", name, r))
			err = fmt.Errorf("dynamic source %s: sync panicked: %v", name, r)
		}
	}()
	return s.syncLocked(ctx, ds)
}

// syncLocked is syncSource under the lock. A sync that changes nothing emits no event and no metric
// (review R7); otherwise RECONCILE_START/DONE (attribute "source", empty txn_id) are emitted once it
// finished. APPLIED puts the source in sync. A failure takes it out of sync and arms the agent's
// retry — except UNAVAILABLE for a source in sync (the reconnect resync includes it) and a done ctx.
func (s *Service) syncLocked(ctx context.Context, ds *dynSource) error {
	if !s.vpp.Connected() {
		if !ds.inSync.Load() {
			s.retrySourceLocked(ds)
		}
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	start := s.now()
	s.setReconciling(true)
	defer s.setReconciling(false)
	log := s.log.With("mode", "sync", "source", ds.Name)
	if s.beforeTxn != nil {
		s.beforeTxn()
	}
	resp := &vrxv1.ApplyResponse{}
	mg := s.mergeSources(nil, s.st.desired, []*dynSource{ds})
	reason := srcRejected
	if len(mg.left) > 0 {
		reason = mg.left[0].reason
		resp.Status = vrxv1.ApplyStatus_APPLY_STATUS_FAILED
		resp.Message = mg.left[0].cause
		resp.Summary = &vrxv1.ApplySummary{}
	} else {
		res := s.sched.ApplyWith(ctx, mg.kvs, scheduler.Only(mg.names...), scheduler.ApplyOptions{})
		fillResponse(resp, res, &projected{pointers: map[scheduler.Key]string{}})
	}
	applied := resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_DEGRADED {
		s.setDegraded(true, "dynamic source "+ds.Name+": "+resp.GetMessage())
	}
	d := s.now().Sub(start)
	if applied && len(resp.GetResults()) == 0 {
		log.Debug("sync: nothing to do", "unchanged", resp.GetSummary().GetUnchanged(), "duration", d)
	} else {
		s.metrics.observe(resp.GetStatus(), d, resp.GetSummary())
		s.mu.Lock()
		s.lastReconcileAt = s.now()
		s.mu.Unlock()
		attrs := map[string]string{"source": ds.Name}
		s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_RECONCILE_START, Message: "sync " + ds.Name, Attributes: attrs})
		s.bus.publish(&vrxv1.Event{Kind: vrxv1.EventKind_EVENT_KIND_RECONCILE_DONE, Summary: resp.GetSummary(), Message: resp.GetStatus().String(), Attributes: attrs})
		log.Info("reconcile done", "status", resp.GetStatus().String(), "summary", resp.GetSummary().String(), "duration", d, "err", resp.GetMessage())
	}
	if applied {
		ds.inSync.Store(!ds.stopped)
		s.stopSourceRetryLocked(ds)
		return nil
	}
	ds.inSync.Store(false)
	s.metrics.sourceError(ds.Name, reason)
	if ctx.Err() == nil {
		s.retrySourceLocked(ds)
	}
	return fmt.Errorf("dynamic source %s: %s: %s", ds.Name, resp.GetStatus(), resp.GetMessage())
}

// retrySourceLocked arms the agent's retry sync of an out-of-sync source (caller holds txn): backoff
// retryMin doubling to retryMax, one timer per source, none after Close or once Run stopped.
func (s *Service) retrySourceLocked(ds *dynSource) {
	if s.closed || ds.stopped || ds.retry != nil {
		return
	}
	if ds.delay == 0 {
		ds.delay = s.retryMin
	} else if ds.delay *= 2; ds.delay > s.retryMax {
		ds.delay = s.retryMax
	}
	ds.retry = time.AfterFunc(ds.delay, func() { s.retrySource(ds) })
}

func (s *Service) stopSourceRetryLocked(ds *dynSource) {
	if ds.retry != nil {
		ds.retry.Stop()
		ds.retry = nil
	}
	ds.delay = 0
}

// retrySource is the retry timer: one sync unless the source is back in sync already.
func (s *Service) retrySource(ds *dynSource) {
	if err := s.lock(context.Background()); err != nil {
		return
	}
	defer s.unlock()
	ds.retry = nil
	if s.closed || ds.stopped || ds.inSync.Load() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("dynamic source: retry sync panicked", "source", ds.Name, "panic", r, "stack", string(debug.Stack()))
			s.metrics.sourceError(ds.Name, srcPanic)
			s.setDegraded(true, fmt.Sprintf("dynamic source %s: its sync panicked, the data plane may be partially changed: %v", ds.Name, r))
		}
	}()
	if err := s.syncLocked(context.Background(), ds); err != nil {
		s.log.Warn("dynamic source: retry sync failed", "source", ds.Name, "err", err, "next_in", ds.delay)
	}
}

// sourceStopped records that the Run of ds panicked or returned before the agent stopped: the source
// stays out of sync for the life of the process, and its objects stay as they are.
func (s *Service) sourceStopped(ds *dynSource, reason, cause string) {
	_ = s.lock(context.Background())
	defer s.unlock()
	ds.stopped = true
	ds.inSync.Store(false)
	s.stopSourceRetryLocked(ds)
	s.sourceError(ds, reason, "", "", fmt.Sprintf("dynamic source %s stopped until the agent restarts (%s): %s", ds.Name, reason, cause))
}

// ---- re-entrancy guard (review R6) --------------------------------------------------------------

// insideTxn reports whether the calling goroutine holds the transaction lock or is inside a
// source's Desired: a sync from there would wait for itself.
func (s *Service) insideTxn() bool {
	g := goid()
	if g == 0 {
		return false
	}
	_, in := s.inDesired.Load(g)
	return in || s.holder.Load() == g
}

// goid returns the calling goroutine's id from runtime.Stack's "goroutine N [" header (0 when it
// cannot be read). Only the S1 re-entrancy guard uses it: Go has no other way to ask "does this
// goroutine hold the lock?".
func goid() uint64 {
	var buf [64]byte
	b := bytes.TrimPrefix(buf[:runtime.Stack(buf[:], false)], []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i > 0 {
		if id, err := strconv.ParseUint(string(b[:i]), 10, 64); err == nil {
			return id
		}
	}
	return 0
}
