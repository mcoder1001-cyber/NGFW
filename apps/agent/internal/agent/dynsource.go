package agent

// Dynamic desired sources (S1, TD-8; subsystems.DynamicSource): readiness, the merge into a
// transaction, per-key quarantine with the config-only fallback, the source's own sync, its retries
// and panic containment (TD-8 review R1–R3, R6; TD-8b: TD-8 verify V1, V4, TD-9 review L7).
//   - A source takes part in transactions only while it is in sync (its last sync ended APPLIED and no
//     transaction left it out since). Every source starts out of sync, so the first resync after an
//     agent restart neither creates nor deletes its objects (they are out of scope).
//   - A dynamic object whose change makes a transaction fail is quarantined on its own, and the
//     transaction runs again (V1). Still declarative: the object is desired as it was (a rejected
//     Create is dropped, a rejected Update or Delete keeps the old value), so the rest of its source
//     and the configuration go ahead. The agent retries the object with backoff.
//   - Only when that cannot settle the transaction (a whole-source cause, a key failing again, more
//     than maxKeyReruns keys) does the transaction run once more without the sources (R2).
//   - A panic in Desired, in a sync or in Run is recovered.

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// maxKeyReruns bounds the reruns of one transaction that each quarantine the dynamic keys that made
// the previous run fail (V1), and the keys one sync retries. Past it the sources are left out as a
// whole (R2): a transaction costs at most maxKeyReruns+2 runs.
const maxKeyReruns = 3

// sourceSyncTimeout is the agent's own deadline of one dynamic-source sync, its reruns included
// (TD-9 review L7/Q7: without it a sync of N operations holds the txn lock up to N × the VPP reply
// timeout). It matches TD-9's DefaultTxnTimeout; when TD-9 merges, the sync takes s.txnTimeout
// instead (a var for the tests).
var sourceSyncTimeout = 5 * time.Minute

// dynSource is one dynamic desired source with its readiness state.
type dynSource struct {
	subsystems.DynamicSource
	own map[string]bool // Descriptors as a set

	// inSync: merged into every transaction (written under the txn lock, read by DryRun without it).
	inSync atomic.Bool
	// guarded by the txn lock:
	stopped  bool          // Run panicked or returned early: out of sync for the life of the process
	retry    *time.Timer   // the agent's retry sync while out of sync
	delay    time.Duration // its backoff (Service.retryMin doubling to retryMax)
	keyRetry *time.Timer   // the agent's retry of the quarantined keys, at the earliest due
	keyGen   uint64        // keyRetry's generation: a superseded timer does nothing

	// quarantine holds the keys whose change VPP rejected (V1). Written under the txn lock, read by
	// DryRun without it: qmu is a leaf lock.
	qmu        sync.Mutex
	quarantine map[scheduler.Key]*quarantined
}

// quarantined is a dynamic key whose change a transaction could not apply (V1). Until a retry applies
// the source's value, every transaction desires hold for the key instead of what the source wants.
type quarantined struct {
	hold  proto.Message // desired meanwhile: the object as it was (nil: absent, a rejected Create)
	want  proto.Message // the source's value that failed (nil: the source wanted the object deleted)
	op    string        // the operation that failed
	cause string
	delay time.Duration // retry backoff: retryMin doubling to retryMax
	due   time.Time     // the next retry
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

// ---- quarantine (V1) ----------------------------------------------------------------------------

// held applies the quarantine to kvs, the source's Desired output: a quarantined key is desired as
// held (left out when its hold is nil), except the keys in release, which a sync retries.
func (ds *dynSource) held(kvs []scheduler.KV, release map[scheduler.Key]bool) []scheduler.KV {
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	if len(ds.quarantine) == 0 {
		return kvs
	}
	out := make([]scheduler.KV, 0, len(kvs)+len(ds.quarantine))
	for _, kv := range kvs {
		if ds.quarantine[kv.Key] == nil || release[kv.Key] {
			out = append(out, kv)
		}
	}
	for _, k := range sortedQuarantine(ds.quarantine) {
		if q := ds.quarantine[k]; q.hold != nil && !release[k] {
			out = append(out, scheduler.KV{Key: k, Value: q.hold})
		}
	}
	return out
}

func sortedQuarantine(m map[scheduler.Key]*quarantined) []scheduler.Key {
	keys := make([]scheduler.Key, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// releasable picks the quarantined keys a sync of ds retries, at most maxKeyReruns: first those whose
// value the source changed since it failed (new information), then — on the agent's key retry — those
// due, earliest first.
func (ds *dynSource) releasable(kvs []scheduler.KV, now time.Time, due bool) map[scheduler.Key]bool {
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	if len(ds.quarantine) == 0 {
		return nil
	}
	cur := make(map[scheduler.Key]proto.Message, len(kvs))
	for _, kv := range kvs {
		cur[kv.Key] = kv.Value
	}
	var changed, expired []scheduler.Key
	for _, k := range sortedQuarantine(ds.quarantine) {
		q := ds.quarantine[k]
		switch {
		case !sameValue(q.want, cur[k]):
			changed = append(changed, k)
		case due && !now.Before(q.due):
			expired = append(expired, k)
		}
	}
	sort.SliceStable(expired, func(i, j int) bool { return ds.quarantine[expired[i]].due.Before(ds.quarantine[expired[j]].due) })
	out := map[scheduler.Key]bool{}
	for _, k := range append(changed, expired...) {
		if len(out) == maxKeyReruns {
			break
		}
		out[k] = true
	}
	return out
}

// sameValue reports whether a and b are the same object value (nil: absent).
func sameValue(a, b proto.Message) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return proto.Equal(a, b)
}

// quarantineSummary returns the number of quarantined keys and a description of them (sync's error).
func (ds *dynSource) quarantineSummary() (int, string) {
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	var parts []string
	for _, k := range sortedQuarantine(ds.quarantine) {
		parts = append(parts, fmt.Sprintf("%s (%s: %s)", k, ds.quarantine[k].op, ds.quarantine[k].cause))
	}
	return len(parts), strings.Join(parts, "; ")
}

// quarantineLocked quarantines lo.key (caller holds txn): transactions desire lo.hold for it until a
// retry applies the source's value; its backoff doubles on every failure.
func (s *Service) quarantineLocked(lo leftOut) {
	ds := lo.src
	ds.qmu.Lock()
	if ds.quarantine == nil {
		ds.quarantine = map[scheduler.Key]*quarantined{}
	}
	delay := s.retryMin
	if q := ds.quarantine[lo.key]; q != nil && q.delay > 0 {
		if delay = 2 * q.delay; delay > s.retryMax {
			delay = s.retryMax
		}
	}
	ds.quarantine[lo.key] = &quarantined{hold: lo.hold, want: lo.want, op: lo.op, cause: lo.cause, delay: delay, due: s.now().Add(delay)}
	ds.qmu.Unlock()
	s.armKeyRetryLocked(ds)
}

// settleReleasedLocked ends the retry of the released keys that did not fail again (caller holds
// txn; the ones in again are quarantined anew by leaveOutLocked): applied, they leave the
// quarantine; otherwise (the sync failed as a whole) their backoff doubles, so a failed attempt never
// leaves them due.
func (s *Service) settleReleasedLocked(ds *dynSource, released map[scheduler.Key]bool, again []leftOut, applied bool) {
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	failed := map[scheduler.Key]bool{}
	for _, lo := range again {
		failed[lo.key] = true
	}
	for k := range released {
		q := ds.quarantine[k]
		switch {
		case q == nil || failed[k]:
		case applied:
			delete(ds.quarantine, k)
		default:
			if q.delay = 2 * q.delay; q.delay > s.retryMax || q.delay <= 0 {
				q.delay = s.retryMax
			}
			q.due = s.now().Add(q.delay)
		}
	}
}

// postponeDueLocked doubles the backoff of ds's due quarantined keys (caller holds txn): a key retry
// that could not run never leaves them due, so the timer cannot spin.
func (s *Service) postponeDueLocked(ds *dynSource) {
	ds.qmu.Lock()
	defer ds.qmu.Unlock()
	now := s.now()
	for _, q := range ds.quarantine {
		if now.Before(q.due) {
			continue
		}
		if q.delay = 2 * q.delay; q.delay > s.retryMax || q.delay <= 0 {
			q.delay = s.retryMax
		}
		q.due = now.Add(q.delay)
	}
}

// armKeyRetryLocked (re)arms the retry of ds's quarantined keys at the earliest due (caller holds
// txn). None after Close, once Run stopped, or with an empty quarantine.
func (s *Service) armKeyRetryLocked(ds *dynSource) {
	ds.keyGen++
	if ds.keyRetry != nil {
		ds.keyRetry.Stop()
		ds.keyRetry = nil
	}
	if s.closed || ds.stopped {
		return
	}
	ds.qmu.Lock()
	var next time.Time
	for _, q := range ds.quarantine {
		if next.IsZero() || q.due.Before(next) {
			next = q.due
		}
	}
	ds.qmu.Unlock()
	if next.IsZero() {
		return
	}
	d := next.Sub(s.now())
	if d < 0 {
		d = 0
	}
	gen := ds.keyGen
	ds.keyRetry = time.AfterFunc(d, func() { s.retryKeys(ds, gen) })
}

// retryKeys is the key retry timer: a sync that retries the due quarantined keys of a source in sync
// (out of sync, the source's own retry rejoins it first and re-arms this one).
func (s *Service) retryKeys(ds *dynSource, gen uint64) {
	if err := s.lock(context.Background()); err != nil {
		return
	}
	defer s.unlock()
	if gen != ds.keyGen {
		return // superseded by a later arm
	}
	ds.keyRetry = nil
	if s.closed || ds.stopped || !ds.inSync.Load() {
		return
	}
	if !s.vpp.Connected() { // the reconnect resync keeps them held: try again after their backoff
		s.postponeDueLocked(ds)
		s.armKeyRetryLocked(ds)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("dynamic source: key retry sync panicked", "source", ds.Name, "panic", r, "stack", string(debug.Stack()))
			ds.inSync.Store(false)
			s.metrics.sourceError(ds.Name, srcPanic)
			s.setDegraded(true, fmt.Sprintf("dynamic source %s: its sync panicked, the data plane may be partially changed: %v", ds.Name, r))
			s.retrySourceLocked(ds)
		}
	}()
	if err := s.syncLocked(context.Background(), ds, true); err != nil {
		s.log.Warn("dynamic source: key retry sync", "source", ds.Name, "err", err)
	}
}

// ---- merge --------------------------------------------------------------------------------------

// leftOut is a dynamic source or key a transaction left out: why, and the dynamic key when one
// object caused it. With quarantine, only the key is left out (V1): hold is what the transaction
// desired for it instead, and the source stays in sync.
type leftOut struct {
	src    *dynSource
	key    scheduler.Key
	op     string
	reason string // srcInvalid, srcPanic, srcRejected; "" = left out together with the failing source
	cause  string
	// per-key quarantine (V1)
	quarantine bool
	hold, want proto.Message
}

// srcMerge is what the dynamic sources add to one transaction.
type srcMerge struct {
	kvs    []scheduler.KV        // the configuration's objects, then the merged sources' objects
	names  []string              // the merged sources' descriptors (they join the scope)
	merged []*dynSource          // the sources whose objects are in kvs
	owner  map[string]*dynSource // descriptor → merged source
	left   []leftOut             // sources left out before the transaction (invalid output, panic)
	// want is each merged source's own value by key, before the quarantine (nil map: none).
	want map[scheduler.Key]proto.Message
	// released are the quarantined keys this transaction retries (a sync's).
	released map[scheduler.Key]bool
}

// scope is the transaction's scope with the merged sources: domains' descriptors plus theirs.
func (mg *srcMerge) scope(domains []string) scheduler.Scope {
	return scheduler.Only(append(scopeNames(domains), mg.names...)...)
}

// hold makes the transaction desire v for the dynamic key k instead (nil: leave k out).
func (mg *srcMerge) hold(k scheduler.Key, v proto.Message) {
	out := make([]scheduler.KV, 0, len(mg.kvs)+1)
	for _, kv := range mg.kvs {
		if kv.Key != k {
			out = append(out, kv)
		}
	}
	if v != nil {
		out = append(out, scheduler.KV{Key: k, Value: v})
	}
	mg.kvs = out
}

// releaseFunc picks the quarantined keys of a source that a transaction retries (nil: none).
type releaseFunc func(ds *dynSource, kvs []scheduler.KV) map[scheduler.Key]bool

// mergeSources adds the objects of srcs for view (the stored document as it will be after the
// transaction; each source gets its own copy) to base, with each source's quarantine applied except
// for the keys release picks. A source whose Desired panics, returns a key outside its descriptors
// or a duplicate is left out as a whole; dynamic objects have no JSON pointer.
func (s *Service) mergeSources(base []scheduler.KV, view *vrxv1.DesiredState, srcs []*dynSource, release releaseFunc) *srcMerge {
	if view == nil {
		view = &vrxv1.DesiredState{}
	}
	mg := &srcMerge{kvs: append([]scheduler.KV(nil), base...), owner: map[string]*dynSource{}, want: map[scheduler.Key]proto.Message{}}
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
		var rel map[scheduler.Key]bool
		if release != nil {
			rel = release(ds, kvs)
		}
		for _, kv := range kvs {
			mg.want[kv.Key] = kv.Value
		}
		for k := range rel {
			if mg.released == nil {
				mg.released = map[scheduler.Key]bool{}
			}
			mg.released[k] = true
		}
		for _, kv := range ds.held(kvs, rel) {
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

// culprit finds what of the merged sources made res fail (review R2, V1, V4), or ok false when the
// configuration failed on its own, and after APPLIED or DEGRADED (a failed rollback is never
// retried). The result is either per-key culprits (quarantine: each dynamic key and the value to hold
// for it) or one whole-source culprit (quarantine false):
//   - plan issues: only when every issue is on a key of a merged source; a missing dependency
//     quarantines its key (left out), any other issue (a source bug) blames the whole source;
//   - a failed operation: the first failed operation, when its key is a merged source's;
//   - a failed verification: only when every key it names belongs to one merged source (V4), each
//     key with the operation the plan had for it; a failed Retrieve of a source descriptor blames
//     the whole source.
func culprit(res *scheduler.TxnResult, mg *srcMerge) ([]leftOut, bool) {
	if res.Outcome != scheduler.OutcomeFailed && res.Outcome != scheduler.OutcomeRolledBack {
		return nil, false
	}
	if res.Plan != nil && len(res.Plan.Issues) > 0 {
		return issueCulprits(res.Plan.Issues, mg)
	}
	for _, r := range res.Results {
		if r.Code != scheduler.CodeFailed || r.Key == "" {
			continue
		}
		ds := mg.owner[r.Key.Descriptor()]
		if ds == nil {
			return nil, false // the first failed operation is the configuration's
		}
		lo := mg.keyCulprit(ds, r.Key, errText(r.Err), res.Plan)
		lo.op = r.Op
		return []leftOut{lo}, true
	}
	if res.Err != nil {
		return errCulprits(res, mg)
	}
	return nil, false
}

// issueCulprits: see culprit.
func issueCulprits(issues []scheduler.Issue, mg *srcMerge) ([]leftOut, bool) {
	var keys []leftOut
	seen := map[scheduler.Key]bool{}
	for _, is := range issues {
		if mg.owner[is.Key.Descriptor()] == nil {
			return nil, false // the configuration has an issue of its own
		}
	}
	for _, is := range issues {
		ds := mg.owner[is.Key.Descriptor()]
		if is.Code != scheduler.CodeDependencyMissing {
			return []leftOut{{src: ds, key: is.Key, reason: srcRejected, cause: is.Message}}, true
		}
		if !seen[is.Key] {
			seen[is.Key] = true
			keys = append(keys, leftOut{src: ds, key: is.Key, reason: srcRejected, cause: is.Message, quarantine: true, want: mg.want[is.Key]})
		}
	}
	return keys, true
}

// errCulprits: see culprit.
func errCulprits(res *scheduler.TxnResult, mg *srcMerge) ([]leftOut, bool) {
	msg := res.Err.Error()
	if keys := verifyKeys(msg); len(keys) > 0 {
		var ds *dynSource
		for _, k := range keys {
			o := mg.owner[k.Descriptor()]
			if o == nil || (ds != nil && o != ds) {
				return nil, false // a configuration key (or another source's) differs too (V4)
			}
			ds = o
		}
		los := make([]leftOut, 0, len(keys))
		for _, k := range keys {
			op := plannedOp(res.Plan, k)
			if op == nil {
				return []leftOut{{src: ds, reason: srcRejected, cause: msg}}, true // unchanged, yet it differs: not one key's change
			}
			lo := mg.keyCulprit(ds, k, msg, res.Plan)
			lo.op = op.Op
			los = append(los, lo)
		}
		return los, true
	}
	for _, ds := range mg.merged {
		for _, d := range ds.Descriptors {
			if strings.Contains(msg, "retrieve "+d+":") {
				return []leftOut{{src: ds, reason: srcRejected, cause: msg}}, true
			}
		}
	}
	return nil, false
}

// keyCulprit quarantines the dynamic key k of ds without its change: a Create's key is left out (hold
// nil), an Update or a Delete keeps the object as it was (the planned operation's Old). A key the plan
// did not change (a dependent re-created around its dependency's change) is left out too: it cannot
// follow the change, so it goes, and its retry brings it back.
func (mg *srcMerge) keyCulprit(ds *dynSource, k scheduler.Key, cause string, plan *scheduler.TxnPlan) leftOut {
	lo := leftOut{src: ds, key: k, reason: srcRejected, cause: cause, quarantine: true, want: mg.want[k]}
	if op := plannedOp(plan, k); op != nil && op.Op != scheduler.OpCreate && op.Old != nil {
		lo.hold = op.Old.Value
	}
	return lo
}

// plannedOp is the plan's operation on k, nil when it has none.
func plannedOp(plan *scheduler.TxnPlan, k scheduler.Key) *scheduler.PlannedOp {
	if plan == nil {
		return nil
	}
	for i := range plan.Ops {
		if plan.Ops[i].Key == k {
			return &plan.Ops[i]
		}
	}
	return nil
}

// verifyPrefix starts the list of differences in the scheduler's verification error.
const verifyPrefix = "actual state differs from desired: "

// verifyKeys are the keys a verification error names ("<key> missing; <key> differs: …; <key> still
// present"), none for any other error.
func verifyKeys(msg string) []scheduler.Key {
	_, list, ok := strings.Cut(msg, verifyPrefix)
	if !ok {
		return nil
	}
	var keys []scheduler.Key
	for _, p := range strings.Split(list, "; ") {
		if k, _, _ := strings.Cut(p, " "); strings.Contains(k, scheduler.KeySeparator) {
			keys = append(keys, scheduler.Key(k))
		}
	}
	return keys
}

// errText is err's text, "" for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// runQuarantining runs mg's transaction. When it fails because of dynamic keys, it quarantines them
// (mg desires their held value instead) and runs again, at most maxKeyReruns times. whole is the
// culprit when per-key quarantine cannot settle it — a whole-source culprit, a key already
// quarantined in this transaction failing again, or too many reruns — and the caller leaves the
// source out. held are the keys quarantined on the way.
func (s *Service) runQuarantining(ctx context.Context, mg *srcMerge, scope scheduler.Scope, opts scheduler.ApplyOptions, log *slog.Logger) (res *scheduler.TxnResult, held []leftOut, whole *leftOut) {
	again := map[scheduler.Key]bool{}
	for reruns := 0; ; reruns++ {
		res = s.sched.ApplyWith(ctx, mg.kvs, scope, opts)
		los, ok := culprit(res, mg)
		if !ok || ctx.Err() != nil { // a cancelled transaction is not the source's fault
			return res, held, nil
		}
		settle := los[0].quarantine && reruns < maxKeyReruns
		for _, lo := range los {
			settle = settle && !again[lo.key]
		}
		if !settle {
			// A key that failed again keeps the quarantine it had before this transaction: the value
			// this transaction held for it failed too, so it is not recorded.
			failing := map[scheduler.Key]bool{}
			for _, lo := range los {
				failing[lo.key] = true
			}
			kept := held[:0]
			for _, h := range held {
				if !failing[h.key] {
					kept = append(kept, h)
				}
			}
			lo := los[0]
			lo.quarantine, lo.hold, lo.want = false, nil, nil
			return res, kept, &lo
		}
		for _, lo := range los {
			mg.hold(lo.key, lo.hold)
			again[lo.key] = true
			held = append(held, lo)
			log.Warn("transaction failed because of a dynamic object: quarantining it and running the transaction again",
				"source", lo.src.Name, "key", lo.key, "op", lo.op, "cause", lo.cause, "first_outcome", res.Outcome.String())
		}
	}
}

// applySources runs the transaction kvs/scope (the configuration) with the sources in sync merged in
// (caller holds txn). When a source cannot be merged, it is left out. When the merged transaction
// fails because of dynamic objects, they are quarantined one by one and it runs again (V1); when that
// cannot settle it, it runs once more without the sources (review R2). left lists every source and
// every key left out.
func (s *Service) applySources(ctx context.Context, kvs []scheduler.KV, scope scheduler.Scope, domains []string, view *vrxv1.DesiredState, opts scheduler.ApplyOptions) (res *scheduler.TxnResult, left []leftOut) {
	active := s.activeSources()
	if len(active) == 0 {
		return s.sched.ApplyWith(ctx, kvs, scope, opts), nil
	}
	mg := s.mergeSources(kvs, view, active, nil)
	left = mg.left
	if len(mg.merged) == 0 {
		return s.sched.ApplyWith(ctx, kvs, scope, opts), left
	}
	res, held, whole := s.runQuarantining(ctx, mg, mg.scope(domains), opts, s.log)
	left = append(left, held...)
	if whole == nil {
		return res, left
	}
	left = append(left, *whole)
	for _, ds := range mg.merged {
		if ds != whole.src {
			left = append(left, leftOut{src: ds})
		}
	}
	s.log.Warn("transaction failed because of a dynamic object: running it again without the dynamic sources",
		"source", whole.src.Name, "key", whole.key, "cause", whole.cause, "first_outcome", res.Outcome.String())
	return s.sched.ApplyWith(ctx, kvs, scope, opts), left
}

// planSources is DryRun's plan (no lock, no state change): what Apply would do, with the sources in
// sync merged (their quarantine applied) for the stored document after the transaction. A dynamic key
// Apply would quarantine at the plan stage (a missing dependency) becomes a WARNING issue
// "agent.dynamic-object-quarantined" and is planned without its change; a source Apply would leave
// out (a panic, a key outside its descriptors, another plan issue on one of its keys) becomes a
// WARNING issue "agent.dynamic-source-skipped", and the plan is the configuration's alone.
func (s *Service) planSources(ctx context.Context, pj *projected, domains []string, update *vrxv1.DesiredState) (*scheduler.TxnPlan, error) {
	scope := scopeOf(domains)
	active := s.activeSources()
	if len(active) == 0 {
		return s.sched.Plan(ctx, pj.kvs, scope)
	}
	s.mu.Lock()
	stored := s.storedDoc
	s.mu.Unlock()
	mg := s.mergeSources(pj.kvs, mergeDomains(stored, update, domains), active, nil)
	warn := func(lo leftOut) {
		pj.warnf("", "agent.dynamic-source-skipped", "dynamic source %s would be left out of this transaction (%s): %s", lo.src.Name, lo.reason, lo.cause)
	}
	for _, lo := range mg.left {
		warn(lo)
	}
	if len(mg.merged) == 0 {
		return s.sched.Plan(ctx, pj.kvs, scope)
	}
	again := map[scheduler.Key]bool{}
	for reruns := 0; ; reruns++ {
		plan, err := s.sched.Plan(ctx, mg.kvs, mg.scope(domains))
		los, ok := culprit(&scheduler.TxnResult{Outcome: scheduler.OutcomeFailed, Plan: plan, Err: err}, mg)
		if !ok {
			return plan, err
		}
		settle := los[0].quarantine && reruns < maxKeyReruns
		for _, lo := range los {
			settle = settle && !again[lo.key]
		}
		if !settle {
			warn(los[0])
			return s.sched.Plan(ctx, pj.kvs, scope)
		}
		for _, lo := range los {
			pj.warnf("", "agent.dynamic-object-quarantined", "dynamic object %s of source %s would be quarantined (%s): %s", lo.key, lo.src.Name, lo.reason, lo.cause)
			mg.hold(lo.key, lo.hold)
			again[lo.key] = true
		}
	}
}

// leaveOutLocked records the sources and keys a transaction left out (caller holds txn). A key
// (V1) is quarantined, and its source stays in sync; a source is out of sync until its next
// successful sync. The agent retries both with backoff. Each culprit gets a SKIPPED result in resp
// (when one object caused it), an ERROR event, the error counter and a log line.
func (s *Service) leaveOutLocked(resp *vrxv1.ApplyResponse, txnID string, left []leftOut, log *slog.Logger) {
	for _, lo := range left {
		var msg string
		if lo.quarantine {
			s.quarantineLocked(lo)
			msg = fmt.Sprintf("dynamic source %s: object %s quarantined (%s; the agent retries it with backoff): %s", lo.src.Name, lo.key, lo.reason, lo.cause)
		} else {
			lo.src.inSync.Store(false)
			s.retrySourceLocked(lo.src)
			if lo.reason == "" {
				log.Warn("dynamic source left out of the transaction together with the failing one", "source", lo.src.Name)
				continue
			}
			msg = fmt.Sprintf("dynamic source %s left out of this transaction (%s): %s", lo.src.Name, lo.reason, lo.cause)
		}
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
	return s.syncLocked(ctx, ds, false)
}

// syncLocked is syncSource under the lock, bounded by sourceSyncTimeout (L7). A sync retries the
// quarantined keys whose value the source changed, and with retryDue (the agent's key retry) those
// due; a key that fails is quarantined and the sync runs again without its change (V1). A sync that
// changes nothing emits no event and no metric (review R7); otherwise RECONCILE_START/DONE (attribute
// "source", empty txn_id) are emitted once it finished. APPLIED puts the source in sync; it returns
// an error wrapping subsystems.ErrQuarantined while keys stay quarantined. A failure takes the source
// out of sync and arms the agent's retry — except UNAVAILABLE for a source in sync (the reconnect
// resync includes it) and a done ctx.
func (s *Service) syncLocked(parent context.Context, ds *dynSource, retryDue bool) error {
	if !s.vpp.Connected() {
		if !ds.inSync.Load() {
			s.retrySourceLocked(ds)
		}
		return status.Error(codes.Unavailable, "VPP binary API is not connected")
	}
	ctx, cancel := context.WithTimeout(parent, sourceSyncTimeout)
	defer cancel()
	start := s.now()
	s.setReconciling(true)
	defer s.setReconciling(false)
	log := s.log.With("mode", "sync", "source", ds.Name)
	if s.beforeTxn != nil {
		s.beforeTxn()
	}
	resp := &vrxv1.ApplyResponse{}
	mg := s.mergeSources(nil, s.st.desired, []*dynSource{ds}, s.releaseFor(retryDue))
	reason := srcRejected
	if len(mg.left) > 0 {
		reason = mg.left[0].reason
		resp.Status = vrxv1.ApplyStatus_APPLY_STATUS_FAILED
		resp.Message = mg.left[0].cause
		resp.Summary = &vrxv1.ApplySummary{}
	} else {
		res, held, _ := s.runQuarantining(ctx, mg, scheduler.Only(mg.names...), scheduler.ApplyOptions{}, log)
		fillResponse(resp, res, &projected{pointers: map[scheduler.Key]string{}})
		s.settleReleasedLocked(ds, mg.released, held, res.Outcome == scheduler.OutcomeApplied)
		s.leaveOutLocked(resp, "", held, log)
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
		s.armKeyRetryLocked(ds)
		if n, which := ds.quarantineSummary(); n > 0 {
			return fmt.Errorf("dynamic source %s: %w: %d object(s) held back and retried with backoff, the rest applied: %s", ds.Name, subsystems.ErrQuarantined, n, which)
		}
		return nil
	}
	ds.inSync.Store(false)
	s.metrics.sourceError(ds.Name, reason)
	if parent.Err() == nil { // the sync's own deadline is not a caller that went away: retry
		s.retrySourceLocked(ds)
	}
	return fmt.Errorf("dynamic source %s: %s: %s", ds.Name, resp.GetStatus(), resp.GetMessage())
}

// releaseFor is a sync's releaseFunc: see syncLocked.
func (s *Service) releaseFor(retryDue bool) releaseFunc {
	now := s.now()
	return func(ds *dynSource, kvs []scheduler.KV) map[scheduler.Key]bool {
		return ds.releasable(kvs, now, retryDue)
	}
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

// stopSourceRetryLocked stops both retries of ds (caller holds txn); syncLocked re-arms the key retry
// after a successful sync.
func (s *Service) stopSourceRetryLocked(ds *dynSource) {
	if ds.retry != nil {
		ds.retry.Stop()
		ds.retry = nil
	}
	ds.delay = 0
	ds.keyGen++
	if ds.keyRetry != nil {
		ds.keyRetry.Stop()
		ds.keyRetry = nil
	}
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
	if err := s.syncLocked(context.Background(), ds, false); err != nil {
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
