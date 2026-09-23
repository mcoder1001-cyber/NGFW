package scheduler

// The reconciler (task P05): the transaction engine behind descriptor.go's contract.
//
//	desired []KV ─┐
//	              ├─ validate (descriptor exists, in scope, no duplicate, mandatory deps satisfied)
//	Retrieve() ───┘   → plan (Delete in reverse topological order, then Create/Update in topological order)
//	                  → apply (journal every successful operation)
//	                  → verify (re-Retrieve the scope, proto.Equal against desired)
//	                  → on any error: undo the journal in reverse order → ROLLED_BACK (or DEGRADED)
//
// Scope implements D-041 (docs/contracts/proto.md §2): only descriptors in scope are planned; owned
// objects of in-scope descriptors that are absent from desired are deleted; objects of out-of-scope
// descriptors are never touched but still satisfy dependencies (and block deletes of what they
// depend on).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// KeyProvider is an optional Descriptor extension: additional keys an object satisfies as a
// dependency target (aliases). P05 core uses it so that a loopback "interface.loopback/loop701"
// also satisfies the cross-plugin reference "interface/loop701" and a VRF table satisfies
// "vrf/<id>" (the key conventions the DF-* prompts use). An alias never plans an operation; it
// only resolves Dependencies.
type KeyProvider interface {
	ProvidedKeys(obj proto.Message) []Key
}

// ResultCode classifies the outcome of one operation (mirrors vrx.v1.ObjectResultCode).
type ResultCode int

// Result codes.
const (
	CodeUnspecified ResultCode = iota
	CodeOK
	CodeFailed
	CodeSkipped
	CodeReverted
	CodeRevertFailed
	CodeDependencyMissing
	CodeInvalid
)

func (c ResultCode) String() string {
	switch c {
	case CodeOK:
		return "OK"
	case CodeFailed:
		return "FAILED"
	case CodeSkipped:
		return "SKIPPED"
	case CodeReverted:
		return "REVERTED"
	case CodeRevertFailed:
		return "REVERT_FAILED"
	case CodeDependencyMissing:
		return "DEPENDENCY_MISSING"
	case CodeInvalid:
		return "INVALID"
	default:
		return "UNSPECIFIED"
	}
}

// Outcome of a transaction (mirrors vrx.v1.ApplyStatus).
type Outcome int

// Outcomes.
const (
	OutcomeUnspecified Outcome = iota
	// OutcomeApplied: every operation succeeded and verification passed (or the plan was empty).
	OutcomeApplied
	// OutcomeFailed: validation or planning failed; nothing was touched.
	OutcomeFailed
	// OutcomeRolledBack: an operation or the verification failed; everything was reverted.
	OutcomeRolledBack
	// OutcomeDegraded: an operation failed and its rollback failed too.
	OutcomeDegraded
)

func (o Outcome) String() string {
	switch o {
	case OutcomeApplied:
		return "APPLIED"
	case OutcomeFailed:
		return "FAILED"
	case OutcomeRolledBack:
		return "ROLLED_BACK"
	case OutcomeDegraded:
		return "DEGRADED"
	default:
		return "UNSPECIFIED"
	}
}

// PlannedOp is one operation of a TxnPlan. For Create/Update, Value is the desired value; for
// Delete it is the actual value. Old is the actual object for Update/Delete.
type PlannedOp struct {
	Key   Key
	Op    string // OpCreate, OpUpdate, OpDelete
	Value proto.Message
	Old   *KV
}

// Issue is a validation finding that prevents the transaction (mirrors a ValidationIssue).
type Issue struct {
	Key     Key
	Code    ResultCode // CodeDependencyMissing or CodeInvalid
	Message string
}

func (i Issue) String() string { return fmt.Sprintf("%s: %s", i.Key, i.Message) }

// TxnPlan is the result of planning: the operations in execution order (deletes first in
// reverse topological order, then creates/updates in topological order), the number of desired
// objects already converged, and the validation issues (a plan with issues is never applied).
type TxnPlan struct {
	Ops       []PlannedOp
	Unchanged int
	Issues    []Issue
	// actual is the Retrieve() snapshot the plan was computed against (all descriptors).
	actual map[Key]KV
}

// Empty reports whether nothing would change.
func (p *TxnPlan) Empty() bool { return len(p.Ops) == 0 }

// Summary counts the plan (planned operations, no failures).
func (p *TxnPlan) Summary() Summary {
	s := Summary{Unchanged: p.Unchanged}
	for _, op := range p.Ops {
		switch op.Op {
		case OpCreate:
			s.Created++
		case OpUpdate:
			s.Updated++
		case OpDelete:
			s.Deleted++
		}
	}
	return s
}

// Summary counts the operations of a transaction (mirrors vrx.v1.ApplySummary).
type Summary struct {
	Created, Updated, Deleted, Unchanged, Failed, Reverted int
}

// OpResult is the outcome of one executed (or skipped, or reverted) operation.
type OpResult struct {
	Key  Key
	Op   string // OpCreate, OpUpdate, OpDelete, OpRecreate
	Code ResultCode
	Err  error
}

// TxnResult is the outcome of Apply.
type TxnResult struct {
	Outcome  Outcome
	Plan     *TxnPlan
	Results  []OpResult
	Summary  Summary
	Err      error // first error (operation, verification or planning)
	Duration time.Duration
}

// Scope selects the descriptors a transaction manages. A nil Scope means every registered
// descriptor.
type Scope func(descriptor string) bool

// All is the Scope that manages every descriptor.
func All(string) bool { return true }

// Only returns a Scope managing exactly the named descriptors.
func Only(names ...string) Scope {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(d string) bool { return set[d] }
}

// Scheduler is the reconciler. One transaction runs at a time; Retrieve waits for a running
// transaction (it takes the read side of the same lock).
type Scheduler struct {
	reg *MapRegistry
	log *slog.Logger
	mu  sync.RWMutex
	// VerifyRetries re-Retrieves this many extra times (VerifyDelay apart) before a verification
	// mismatch counts as an error; 0 = verify once.
	VerifyRetries int
	VerifyDelay   time.Duration
}

// New returns a scheduler over reg. log may be nil.
func New(reg *MapRegistry, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Scheduler{reg: reg, log: log, VerifyRetries: 2, VerifyDelay: 50 * time.Millisecond}
}

// Registry returns the scheduler's registry.
func (s *Scheduler) Registry() *MapRegistry { return s.reg }

// Retrieve dumps the actual state of every descriptor in scope (nil = all), sorted by key.
func (s *Scheduler) Retrieve(ctx context.Context, scope Scope) ([]KV, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, err := s.retrieve(ctx, scope, true)
	if err != nil {
		return nil, err
	}
	return sortedKVs(m), nil
}

// retrieve dumps the descriptors in scope. With strict=false a failing out-of-scope descriptor
// is logged and skipped (it only contributes dependency targets).
func (s *Scheduler) retrieve(ctx context.Context, scope Scope, strict bool) (map[Key]KV, error) {
	if scope == nil {
		scope = All
	}
	out := make(map[Key]KV)
	for _, d := range s.reg.Descriptors() {
		in := scope(d.Name())
		if strict && !in {
			continue
		}
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			if !in {
				s.log.Warn("retrieve of out-of-scope descriptor failed; its objects cannot satisfy dependencies",
					"descriptor", d.Name(), "err", err)
				continue
			}
			return nil, fmt.Errorf("retrieve %s: %w", d.Name(), err)
		}
		for _, kv := range kvs {
			if kv.Key.Descriptor() != d.Name() {
				return nil, fmt.Errorf("retrieve %s: returned foreign key %q", d.Name(), kv.Key)
			}
			if _, dup := out[kv.Key]; dup {
				return nil, fmt.Errorf("retrieve %s: duplicate key %q", d.Name(), kv.Key)
			}
			out[kv.Key] = kv
		}
	}
	return out, nil
}

// Plan validates desired against the actual state and returns what Apply would do. It never
// mutates anything.
func (s *Scheduler) Plan(ctx context.Context, desired []KV, scope Scope) (*TxnPlan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.plan(ctx, desired, scope)
}

func (s *Scheduler) plan(ctx context.Context, desired []KV, scope Scope) (*TxnPlan, error) {
	if scope == nil {
		scope = All
	}
	p := &TxnPlan{}
	want := make(map[Key]KV, len(desired))
	for _, kv := range desired {
		d, ok := s.reg.ForKey(kv.Key)
		switch {
		case !ok:
			p.Issues = append(p.Issues, Issue{Key: kv.Key, Code: CodeInvalid, Message: "no descriptor registered for " + kv.Key.Descriptor()})
			continue
		case !scope(d.Name()):
			p.Issues = append(p.Issues, Issue{Key: kv.Key, Code: CodeInvalid, Message: "descriptor " + d.Name() + " is not in the scope of this transaction"})
			continue
		case kv.Value == nil:
			p.Issues = append(p.Issues, Issue{Key: kv.Key, Code: CodeInvalid, Message: "nil value"})
			continue
		case d.KeyOf(kv.Value) != kv.Key:
			p.Issues = append(p.Issues, Issue{Key: kv.Key, Code: CodeInvalid, Message: fmt.Sprintf("key does not match descriptor KeyOf (%s)", d.KeyOf(kv.Value))})
			continue
		}
		if _, dup := want[kv.Key]; dup {
			p.Issues = append(p.Issues, Issue{Key: kv.Key, Code: CodeInvalid, Message: "duplicate key in desired state"})
			continue
		}
		want[kv.Key] = kv
	}

	actual, err := s.retrieve(ctx, scope, false)
	if err != nil {
		return nil, err
	}
	p.actual = actual

	// Objects that exist after the transaction: desired + actual objects that are not deleted.
	deletes := make(map[Key]KV)
	after := make(map[Key]KV, len(want)+len(actual))
	for k, kv := range actual {
		if _, keep := want[k]; keep {
			continue
		}
		if scope(k.Descriptor()) {
			deletes[k] = kv
			continue
		}
		after[k] = kv
	}
	for k, kv := range want {
		after[k] = kv
	}
	afterAlias := s.aliases(after)

	// Mandatory dependencies must exist after the transaction — for desired objects and for the
	// out-of-scope objects that stay.
	for _, k := range sortedKeys(after) {
		kv := after[k]
		d, _ := s.reg.ForKey(k)
		for _, dep := range d.Dependencies(kv.Value) {
			if dep.Optional || s.resolves(dep.Key, after, afterAlias) {
				continue
			}
			if _, isWanted := want[k]; isWanted {
				p.Issues = append(p.Issues, Issue{Key: k, Code: CodeDependencyMissing, Message: fmt.Sprintf("mandatory dependency %s is neither desired nor present", dep.Key)})
			} else if s.resolvesIn(dep.Key, deletes) {
				// an untouched object depends on something this transaction would delete
				p.Issues = append(p.Issues, Issue{Key: dep.Key, Code: CodeDependencyMissing, Message: fmt.Sprintf("cannot delete: %s (not managed by this transaction) depends on it", k)})
			}
		}
	}
	if len(p.Issues) > 0 {
		sortIssues(p.Issues)
		return p, nil
	}

	// Topological order over everything that is created, updated or deleted.
	nodes := make(map[Key]KV, len(want)+len(deletes))
	for k, kv := range want {
		nodes[k] = kv
	}
	for k, kv := range deletes {
		nodes[k] = kv
	}
	order, cyc := s.topo(nodes)
	if len(cyc) > 0 {
		for _, k := range cyc {
			p.Issues = append(p.Issues, Issue{Key: k, Code: CodeInvalid, Message: "dependency cycle"})
		}
		sortIssues(p.Issues)
		return p, nil
	}
	for i := len(order) - 1; i >= 0; i-- {
		k := order[i]
		if old, del := deletes[k]; del {
			o := old
			p.Ops = append(p.Ops, PlannedOp{Key: k, Op: OpDelete, Value: old.Value, Old: &o})
		}
	}
	for _, k := range order {
		kv, ok := want[k]
		if !ok {
			continue
		}
		cur, exists := actual[k]
		switch {
		case !exists:
			p.Ops = append(p.Ops, PlannedOp{Key: k, Op: OpCreate, Value: kv.Value})
		case proto.Equal(cur.Value, kv.Value):
			p.Unchanged++
		default:
			c := cur
			p.Ops = append(p.Ops, PlannedOp{Key: k, Op: OpUpdate, Value: kv.Value, Old: &c})
		}
	}
	return p, nil
}

// aliases maps every provided alias key of objs to the real key.
func (s *Scheduler) aliases(objs map[Key]KV) map[Key]Key {
	out := make(map[Key]Key)
	for k, kv := range objs {
		d, ok := s.reg.ForKey(k)
		if !ok {
			continue
		}
		if kp, ok := d.(KeyProvider); ok {
			for _, a := range kp.ProvidedKeys(kv.Value) {
				out[a] = k
			}
		}
	}
	return out
}

func (s *Scheduler) resolves(dep Key, objs map[Key]KV, alias map[Key]Key) bool {
	if _, ok := objs[dep]; ok {
		return true
	}
	_, ok := alias[dep]
	return ok
}

func (s *Scheduler) resolvesIn(dep Key, objs map[Key]KV) bool {
	return s.resolves(dep, objs, s.aliases(objs))
}

// topo sorts nodes so that every dependency (resolved through aliases, only among nodes)
// precedes its dependent. Ties: descriptor registration order, then key. Returns the keys left
// in a cycle, if any.
func (s *Scheduler) topo(nodes map[Key]KV) (order []Key, cycle []Key) {
	rank := make(map[string]int)
	for i, n := range s.reg.Names() {
		rank[n] = i
	}
	alias := s.aliases(nodes)
	deps := make(map[Key]map[Key]bool, len(nodes))
	users := make(map[Key][]Key)
	for k, kv := range nodes {
		deps[k] = map[Key]bool{}
		d, ok := s.reg.ForKey(k)
		if !ok {
			continue
		}
		for _, dep := range d.Dependencies(kv.Value) {
			target := dep.Key
			if _, ok := nodes[target]; !ok {
				a, ok := alias[target]
				if !ok {
					continue
				}
				target = a
			}
			if target == k || deps[k][target] {
				continue
			}
			deps[k][target] = true
			users[target] = append(users[target], k)
		}
	}
	less := func(a, b Key) bool {
		ra, rb := rank[a.Descriptor()], rank[b.Descriptor()]
		if ra != rb {
			return ra < rb
		}
		return a < b
	}
	var ready []Key
	for k := range nodes {
		if len(deps[k]) == 0 {
			ready = append(ready, k)
		}
	}
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
		k := ready[0]
		ready = ready[1:]
		order = append(order, k)
		for _, u := range users[k] {
			delete(deps[u], k)
			if len(deps[u]) == 0 {
				ready = append(ready, u)
			}
		}
		delete(deps, k)
	}
	for k, rem := range deps {
		if len(rem) > 0 {
			cycle = append(cycle, k)
		}
	}
	sort.Slice(cycle, func(i, j int) bool { return cycle[i] < cycle[j] })
	return order, cycle
}

// journalEntry is one successfully applied primitive operation, enough to undo it.
type journalEntry struct {
	key      Key
	op       string // OpCreate, OpUpdate, OpDelete
	oldValue proto.Message
	oldMeta  any
	newValue proto.Message
	newMeta  any
	result   int // index into TxnResult.Results of the operation this entry belongs to
}

// Apply plans desired against the actual state and executes the plan as one transaction.
func (s *Scheduler) Apply(ctx context.Context, desired []KV, scope Scope) *TxnResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := time.Now()
	res := &TxnResult{}
	defer func() { res.Duration = time.Since(start) }()

	p, err := s.plan(ctx, desired, scope)
	if err != nil {
		res.Outcome, res.Err = OutcomeFailed, err
		return res
	}
	res.Plan = p
	if len(p.Issues) > 0 {
		res.Outcome = OutcomeFailed
		res.Err = fmt.Errorf("validation failed: %s", p.Issues[0])
		for _, is := range p.Issues {
			res.Results = append(res.Results, OpResult{Key: is.Key, Code: is.Code, Err: errors.New(is.Message)})
		}
		res.Summary = Summary{Unchanged: p.Unchanged, Failed: len(p.Issues)}
		return res
	}
	res.Summary.Unchanged = p.Unchanged
	if p.Empty() {
		res.Outcome = OutcomeApplied
		return res
	}

	x := &executor{s: s, res: res, live: make(map[Key]KV, len(p.actual)), desired: make(map[Key]KV, len(desired)), done: map[Key]bool{}}
	for k, kv := range p.actual {
		x.live[k] = kv
	}
	for _, kv := range desired {
		x.desired[kv.Key] = kv
	}
	var failed error
	for i, op := range p.Ops {
		if x.done[op.Key] {
			continue // already converged by a recreate of one of its dependencies
		}
		if err := ctx.Err(); err != nil {
			failed = err
		} else {
			failed = x.run(ctx, op)
		}
		if failed != nil {
			for _, rest := range p.Ops[i+1:] {
				if !x.done[rest.Key] {
					res.Results = append(res.Results, OpResult{Key: rest.Key, Op: rest.Op, Code: CodeSkipped})
				}
			}
			break
		}
	}
	if failed == nil {
		failed = s.verify(ctx, desired, scope)
		if failed != nil {
			res.Results = append(res.Results, OpResult{Key: "", Op: "verify", Code: CodeFailed, Err: failed})
		}
	}
	if failed == nil {
		res.Outcome = OutcomeApplied
		for _, r := range res.Results {
			switch r.Op {
			case OpCreate:
				res.Summary.Created++
			case OpUpdate, OpRecreate:
				res.Summary.Updated++
			case OpDelete:
				res.Summary.Deleted++
			}
		}
		return res
	}
	res.Err = failed
	res.Summary.Failed = 1
	// Rollback: undo the journal in reverse order. Rollback runs even if ctx was cancelled.
	rbCtx := context.WithoutCancel(ctx)
	degraded := false
	for i := len(x.journal) - 1; i >= 0; i-- {
		j := x.journal[i]
		err := x.undo(rbCtx, j)
		r := &res.Results[j.result]
		if err != nil {
			degraded = true
			r.Code = CodeRevertFailed
			r.Err = errors.Join(r.Err, fmt.Errorf("revert %s %s: %w", j.op, j.key, err))
			s.log.Error("rollback operation failed", "key", j.key, "op", j.op, "err", err)
			continue
		}
		if r.Code == CodeOK {
			r.Code = CodeReverted
			res.Summary.Reverted++
		}
	}
	if degraded {
		res.Outcome = OutcomeDegraded
	} else {
		res.Outcome = OutcomeRolledBack
	}
	return res
}

// verify re-Retrieves the scope and compares it with desired.
func (s *Scheduler) verify(ctx context.Context, desired []KV, scope Scope) error {
	var err error
	for attempt := 0; attempt <= s.VerifyRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.VerifyDelay):
			}
		}
		var actual map[Key]KV
		actual, err = s.retrieve(ctx, scope, true)
		if err != nil {
			return fmt.Errorf("verify: %w", err)
		}
		err = diffErr(desired, actual)
		if err == nil {
			return nil
		}
	}
	return err
}

func diffErr(desired []KV, actual map[Key]KV) error {
	var problems []string
	seen := make(map[Key]bool, len(desired))
	for _, kv := range desired {
		seen[kv.Key] = true
		a, ok := actual[kv.Key]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s missing", kv.Key))
		case !proto.Equal(a.Value, kv.Value):
			problems = append(problems, fmt.Sprintf("%s differs: want {%v} got {%v}", kv.Key, kv.Value, a.Value))
		}
	}
	for _, k := range sortedKeys(actual) {
		if !seen[k] {
			problems = append(problems, fmt.Sprintf("%s still present", k))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("verify: actual state differs from desired: %s", strings.Join(problems, "; "))
}

// executor carries the state of one Apply.
type executor struct {
	s       *Scheduler
	res     *TxnResult
	live    map[Key]KV // what exists right now (actual state updated with every operation)
	desired map[Key]KV
	journal []journalEntry
	done    map[Key]bool
}

func (x *executor) descriptor(k Key) Descriptor {
	d, _ := x.s.reg.ForKey(k)
	return d
}

func (x *executor) addResult(k Key, op string, code ResultCode, err error) int {
	x.res.Results = append(x.res.Results, OpResult{Key: k, Op: op, Code: code, Err: err})
	return len(x.res.Results) - 1
}

func (x *executor) run(ctx context.Context, op PlannedOp) error {
	switch op.Op {
	case OpCreate:
		idx := x.addResult(op.Key, OpCreate, CodeOK, nil)
		return x.around(ctx, op.Key, op.Value, func() error { return x.create(ctx, op.Key, op.Value, idx) })
	case OpDelete:
		idx := x.addResult(op.Key, OpDelete, CodeOK, nil)
		cur := x.live[op.Key]
		return x.around(ctx, op.Key, cur.Value, func() error { return x.del(ctx, op.Key, idx) })
	case OpUpdate:
		cur := x.live[op.Key]
		meta, err := x.descriptor(op.Key).Update(ctx, cur.Value, op.Value, cur.Meta)
		if errors.Is(err, ErrRecreate) {
			idx := x.addResult(op.Key, OpRecreate, CodeOK, nil)
			return x.around(ctx, op.Key, cur.Value, func() error {
				if err := x.del(ctx, op.Key, idx); err != nil {
					return err
				}
				return x.create(ctx, op.Key, op.Value, idx)
			})
		}
		if err != nil {
			x.addResult(op.Key, OpUpdate, CodeFailed, err)
			x.s.log.Warn("update failed", "key", op.Key, "err", err)
			return fmt.Errorf("update %s: %w", op.Key, err)
		}
		idx := x.addResult(op.Key, OpUpdate, CodeOK, nil)
		x.journal = append(x.journal, journalEntry{key: op.Key, op: OpUpdate, oldValue: cur.Value, oldMeta: cur.Meta, newValue: op.Value, newMeta: meta, result: idx})
		x.live[op.Key] = KV{Key: op.Key, Value: op.Value, Meta: meta}
		x.done[op.Key] = true
		x.s.log.Info("updated", "key", op.Key)
		return nil
	}
	return fmt.Errorf("unknown operation %q", op.Op)
}

func (x *executor) fail(k Key, idx int, op string, err error) error {
	x.res.Results[idx].Code = CodeFailed
	x.res.Results[idx].Err = err
	x.s.log.Warn(op+" failed", "key", k, "err", err)
	return fmt.Errorf("%s %s: %w", op, k, err)
}

// create creates k and journals it under result idx.
func (x *executor) create(ctx context.Context, k Key, v proto.Message, idx int) error {
	meta, err := x.descriptor(k).Create(ctx, v)
	if err != nil {
		return x.fail(k, idx, OpCreate, err)
	}
	x.journal = append(x.journal, journalEntry{key: k, op: OpCreate, newValue: v, newMeta: meta, result: idx})
	x.live[k] = KV{Key: k, Value: v, Meta: meta}
	x.done[k] = true
	x.s.log.Info("created", "key", k)
	return nil
}

// del deletes the live object k and journals it under result idx.
func (x *executor) del(ctx context.Context, k Key, idx int) error {
	cur := x.live[k]
	if err := x.descriptor(k).Delete(ctx, cur.Value, cur.Meta); err != nil {
		return x.fail(k, idx, OpDelete, err)
	}
	x.journal = append(x.journal, journalEntry{key: k, op: OpDelete, oldValue: cur.Value, oldMeta: cur.Meta, result: idx})
	delete(x.live, k)
	x.done[k] = true
	x.s.log.Info("deleted", "key", k)
	return nil
}

// around runs fn (a create, delete or recreate of key) with every live object that
// (transitively, through optional dependencies too) depends on key removed first and re-created
// afterwards with its desired value (its previous value when it is not desired). That is what
// keeps handles valid (a dependent created against an old sw_if_index) and satisfies VPP's
// ordering constraints (e.g. an interface cannot change its table while it has addresses).
// value is key's value used to compute the aliases it provides.
func (x *executor) around(ctx context.Context, key Key, value proto.Message, fn func() error) error {
	dependents := x.dependents(key, value)
	if len(dependents) == 0 {
		return fn()
	}
	x.s.log.Info("re-creating dependents", "key", key, "dependents", len(dependents))
	depIdx := make(map[Key]int, len(dependents))
	for _, k := range dependents {
		depIdx[k] = x.addResult(k, OpRecreate, CodeOK, nil)
	}
	for i := len(dependents) - 1; i >= 0; i-- {
		if err := x.del(ctx, dependents[i], depIdx[dependents[i]]); err != nil {
			return err
		}
	}
	if err := fn(); err != nil {
		return err
	}
	for _, k := range dependents {
		v := x.journalValue(k)
		if want, ok := x.desired[k]; ok {
			v = want.Value
		}
		if err := x.create(ctx, k, v, depIdx[k]); err != nil {
			return err
		}
	}
	return nil
}

// journalValue returns the last deleted value of k recorded in the journal.
func (x *executor) journalValue(k Key) proto.Message {
	for i := len(x.journal) - 1; i >= 0; i-- {
		if x.journal[i].key == k && x.journal[i].op == OpDelete {
			return x.journal[i].oldValue
		}
	}
	return nil
}

// dependents returns the live objects that transitively depend on key (whose value is value),
// in topological order (dependencies first). Objects the plan deletes anyway are gone already
// (deletes run first), so only objects that stay are returned.
func (x *executor) dependents(key Key, value proto.Message) []Key {
	users := make(map[Key]KV)
	provides := func(k Key, v proto.Message) map[Key]bool {
		out := map[Key]bool{k: true}
		if d := x.descriptor(k); d != nil && v != nil {
			if kp, ok := d.(KeyProvider); ok {
				for _, a := range kp.ProvidedKeys(v) {
					out[a] = true
				}
			}
		}
		return out
	}
	type item struct {
		k Key
		v proto.Message
	}
	frontier := []item{{key, value}}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		provided := provides(cur.k, cur.v)
		for _, k := range sortedKeys(x.live) {
			if k == key {
				continue
			}
			if _, seen := users[k]; seen {
				continue
			}
			kv := x.live[k]
			for _, dep := range x.descriptor(k).Dependencies(kv.Value) {
				if provided[dep.Key] {
					users[k] = kv
					frontier = append(frontier, item{k, kv.Value})
					break
				}
			}
		}
	}
	order, _ := x.s.topo(users)
	return order
}

// undo reverts one journal entry.
func (x *executor) undo(ctx context.Context, j journalEntry) error {
	d := x.descriptor(j.key)
	switch j.op {
	case OpCreate:
		return d.Delete(ctx, j.newValue, j.newMeta)
	case OpDelete:
		_, err := d.Create(ctx, j.oldValue)
		return err
	case OpUpdate:
		_, err := d.Update(ctx, j.newValue, j.oldValue, j.newMeta)
		if errors.Is(err, ErrRecreate) {
			if err := d.Delete(ctx, j.newValue, j.newMeta); err != nil {
				return err
			}
			_, err = d.Create(ctx, j.oldValue)
		}
		return err
	}
	return fmt.Errorf("unknown journal op %q", j.op)
}

func sortedKeys(m map[Key]KV) []Key {
	out := make([]Key, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedKVs(m map[Key]KV) []KV {
	out := make([]KV, 0, len(m))
	for _, k := range sortedKeys(m) {
		out = append(out, m[k])
	}
	return out
}

func sortIssues(is []Issue) {
	sort.SliceStable(is, func(i, j int) bool { return is[i].Key < is[j].Key })
}
