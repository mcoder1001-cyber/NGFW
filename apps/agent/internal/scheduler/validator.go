package scheduler

// Tier-3 validation (TD-13, audit ARCH-02, D-125). A descriptor whose objects are a daemon's
// configuration (Kea, Unbound, nftables, FRR, strongSwan, …) checks that configuration with the
// daemon's own checker BEFORE the transaction writes anything. Until TD-13 the check ran inside the
// descriptor's Create (D-109 d), that is AFTER the VPP objects of the same transaction had changed,
// and DryRun ran no checker at all.
//
//	plan (validate desired, diff, order) → validate (every Create/Update of a Validator) → apply
//
// A rejection is an Issue of the plan: Plan (DryRun) reports it, and Apply answers FAILED with
// nothing touched — no VPP call, no daemon write.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Validator is an optional Descriptor extension: the tier-3 check of an object — typically a
// daemon's configuration rendered to a private temp dir and run through the daemon's own checker
// (`kea-dhcp4 -t`, `unbound-checkconf`, `nft -c`, `vtysh -C`, …).
//
// The scheduler calls Validate for every Create and Update operation of the descriptor's objects,
// in plan order, after planning and before the first operation of the transaction: in Plan (the
// DryRun RPC) and in every Apply, resync and revert. Deletes are not validated; neither is an
// object a recreate cascade re-creates with a value it already had.
//
// value is exactly what Create or Update would receive (the Normalizer's output) — a copy, so the
// validator cannot change what is applied. view is the state after the transaction.
//
// The contract (docs/agent/scheduler-validators.md):
//
//   - Read only. No side effect outside a private temp dir the validator removes before it returns:
//     no write to the daemon's files, no reload, no restart, no VPP call, no ownership claim. A
//     plan must be free to run a validator any number of times (DryRun, Apply, a retry without the
//     dynamic sources). Never call back into the Scheduler (Plan, Retrieve, Apply): Plan holds the
//     read lock and Apply the write lock, so the call deadlocks or stalls until the deadline. What
//     the validator needs of the other objects is in the view.
//   - Bounded by ctx. The scheduler gives every call a deadline (Scheduler.ValidateTimeout,
//     DefaultValidateTimeout) and stops waiting when it passes; a checker is started with
//     exec.CommandContext (or renderers.Command's Timeout) so it dies with ctx. A call that did not
//     return in time is a finding; a panic is a finding too (its stack goes to the agent log).
//   - Safe for concurrent use, like Retrieve: two DryRuns plan at the same time (Plan holds only
//     the read side of the scheduler lock), and a call abandoned at its deadline may still be
//     running when the next transaction's Create/Update/Delete run.
//   - A non-nil error rejects the object. Wrap it with InvalidAt to name the offending leaf by its
//     RFC 6901 pointer into the configuration document, when the value carries one.
//   - No secrets in the error. A validator that resolves secret references (rfkit.Secrets) masks
//     every plaintext it resolved (rfkit.Redactor.Error) before it returns: the scheduler cannot
//     know them. The scheduler additionally masks the secret references of value (RedactLeaves)
//     and bounds the text.
type Validator interface {
	Validate(ctx context.Context, key Key, value proto.Message, view ReadOnlyView) error
}

// ReadOnlyView is the state a Validator sees: every object that exists after the transaction —
// the desired objects of the transaction (normalised, as Create and Update receive them) and the
// actual objects it keeps (out-of-scope objects, observe-only objects). Objects the transaction
// deletes are absent. Values are copies; Meta is never exposed. A daemon whose configuration file
// is rendered from several objects builds the whole file from List(its descriptor name).
type ReadOnlyView interface {
	// Get returns a copy of the value k has after the transaction.
	Get(k Key) (proto.Message, bool)
	// List returns copies of every object of descriptor after the transaction, sorted by key,
	// with Meta nil.
	List(descriptor string) []KV
}

// DefaultValidateTimeout bounds one Validate call when Scheduler.ValidateTimeout is 0. It is a
// scheduler-local constant on purpose: the scheduler imports no ngfw package, and the bound is about
// daemon checkers, not VPP replies (the same 30 s as TD-9's VPP reply bound, but independent of it).
const DefaultValidateTimeout = 30 * time.Second

// RuleValidator is the Issue.Rule of a Validator's finding: the ValidationIssue rule that DryRun
// and a FAILED Apply report.
const RuleValidator = "agent.validator"

// maxValidatorMessage bounds the text of one finding (a checker may print a whole configuration).
const maxValidatorMessage = 2048

// ValidationError is a Validator error that names the offending leaf (InvalidAt).
type ValidationError struct {
	// Pointer is the RFC 6901 pointer of the offending value in the configuration document.
	Pointer string
	Err     error
}

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

// InvalidAt wraps a Validator error with the RFC 6901 pointer (into the configuration document, as
// the value carries it — e.g. a rendered rule's pointer) of the leaf it is about. The finding then
// points there instead of at the whole object. A pointer that does not start with "/" is ignored.
// nil stays nil.
func InvalidAt(pointer string, err error) error {
	if err == nil {
		return nil
	}
	return &ValidationError{Pointer: pointer, Err: err}
}

// PlanOptions tune a Plan (PlanWith).
type PlanOptions struct {
	// SkipValidators plans without running the Validators (TD-13 review M2). For the periodic drift
	// check (TD-9 CheckDrift): it asks whether the running state matches the stored desired state,
	// not whether a daemon would accept it, and a rejection would hide the drifted operations behind
	// one issue (a plan with issues keeps no operation). DryRun and Apply never skip them.
	SkipValidators bool
}

// PlanWith is Plan with options. It never mutates anything.
func (s *Scheduler) PlanWith(ctx context.Context, desired []KV, scope Scope, opts PlanOptions) (*TxnPlan, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.plan(ctx, desired, scope, ApplyOptions{skipValidators: opts.SkipValidators})
}

// validate runs the Validators of p's Create and Update operations (plan order) against the state
// after the transaction (after) and records every rejection as an Issue; a plan with issues keeps
// no operation. It returns an error only when ctx ended: the transaction was cancelled, not the
// configuration found invalid.
func (s *Scheduler) validate(ctx context.Context, p *TxnPlan, after map[Key]KV, opts ApplyOptions) error {
	if opts.skipValidators {
		return nil
	}
	var view *afterView
	for _, op := range p.Ops {
		if op.Op != OpCreate && op.Op != OpUpdate {
			continue
		}
		d, ok := s.reg.ForKey(op.Key)
		if !ok {
			continue
		}
		v, ok := d.(Validator)
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if view == nil {
			view = &afterView{objs: after}
		}
		err := s.runValidator(ctx, v, op.Key, op.Value, view)
		if err == nil {
			continue
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		is := validatorIssue(op.Key, op.Value, err)
		s.log.Warn("validator rejected the object; nothing is applied", "key", op.Key, "pointer", is.Pointer, "err", is.Message)
		p.Issues = append(p.Issues, is)
	}
	if len(p.Issues) > 0 {
		sortIssues(p.Issues)
		p.Ops, p.Unchanged = nil, 0 // like every plan with issues: nothing to apply
	}
	return nil
}

// validateTimeout is the bound of one Validate call (0 = none).
func (s *Scheduler) validateTimeout() time.Duration {
	switch {
	case s.ValidateTimeout == 0:
		return DefaultValidateTimeout
	case s.ValidateTimeout < 0:
		return 0
	}
	return s.ValidateTimeout
}

// runValidator calls v.Validate on its own goroutine with the call's deadline and waits for it or
// the deadline, whichever comes first: a validator that ignores ctx cannot hold the transaction.
// A panic is recovered there (the caller's recover cannot see another goroutine) and becomes a
// finding; its value and stack go to the log only.
func (s *Scheduler) runValidator(ctx context.Context, v Validator, k Key, value proto.Message, view ReadOnlyView) error {
	bound := s.validateTimeout()
	if bound > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, bound)
		defer cancel()
	}
	done := make(chan error, 1) // buffered: a validator that returns after the deadline never blocks
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// masked like a finding (review L2): a panic message may quote a configuration line
				s.log.Error("validator panicked", "key", k, "panic", boundText(RedactLeaves(fmt.Sprint(r), value), maxValidatorMessage), "stack", string(debug.Stack()))
				done <- errors.New("validator panicked (stack in the agent log)")
			}
		}()
		done <- v.Validate(ctx, k, proto.Clone(value), view)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("validator did not return within %s: %w", bound, ctx.Err())
	}
}

// validatorIssue turns a Validator error into the Issue of key: rule RuleValidator, the pointer the
// error names (InvalidAt), and the error text with the secret leaves of value masked and bounded.
func validatorIssue(k Key, value proto.Message, err error) Issue {
	is := Issue{Key: k, Code: CodeInvalid, Rule: RuleValidator}
	var ve *ValidationError
	if errors.As(err, &ve) && strings.HasPrefix(ve.Pointer, "/") {
		is.Pointer = ve.Pointer
	}
	is.Message = "validator: " + boundText(RedactLeaves(err.Error(), value), maxValidatorMessage)
	return is
}

// afterView implements ReadOnlyView over the plan's "after" set; objs is never written once built.
type afterView struct {
	objs map[Key]KV
	once sync.Once
	keys []Key
}

func (v *afterView) Get(k Key) (proto.Message, bool) {
	kv, ok := v.objs[k]
	if !ok || kv.Value == nil {
		return nil, false
	}
	return proto.Clone(kv.Value), true
}

func (v *afterView) List(descriptor string) []KV {
	v.once.Do(func() { v.keys = sortedKeys(v.objs) })
	var out []KV
	for _, k := range v.keys {
		if kv := v.objs[k]; k.Descriptor() == descriptor && kv.Value != nil {
			out = append(out, KV{Key: k, Value: proto.Clone(kv.Value)})
		}
	}
	return out
}

// ---- redaction ---------------------------------------------------------------------------------

// Redacted replaces a secret in validator findings (the marker rfkit and the renderers use).
const Redacted = "<redacted>"

// secretRefRe is a D-051 secret-store reference ("psk/site-a"; the schema's secretRefOf). The value
// is not the secret, but a checker that echoes a configuration line prints what stands next to it.
var secretRefRe = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9_.-]{1,64}$`)

// structFullName is google.protobuf.Struct: its map keys are field names, not object names.
const structFullName = "google.protobuf.Struct"

// refField reports whether a field name marks a secret reference (the proto side of the schema's
// secretRefOf: password_ref, secret_ref, private_key_ref, …).
func refField(name string) bool { return strings.HasSuffix(name, "_ref") }

// RedactLeaves returns text with the secret references of value masked as Redacted:
//
//   - every string anywhere in value that has the D-051 reference form <kind>/<name>;
//   - the value of every string field whose name ends in "_ref" — a malformed reference too (in a
//     google.protobuf.Struct the map keys are the field names, so a "_ref" key counts as well).
//
// By D-040 nothing else in a value can carry a secret: schema leaves marked secret have no proto
// field, every other secret crosses only as a *_ref reference. Map keys (user object names) and
// other text (BGP communities, descriptions) stay readable. The plaintexts a validator resolved from
// the references are the validator's to mask (rfkit.Redactor). A value is masked only where it
// stands as a whole token, so a short one never blanks part of an unrelated word.
func RedactLeaves(text string, value proto.Message) string {
	if value == nil {
		return text
	}
	set := map[string]bool{}
	collectRefs(value.ProtoReflect(), false, set)
	if len(set) == 0 {
		return text
	}
	vals := make([]string, 0, len(set))
	for v := range set {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool { // longest first, so a reference containing another stays whole
		if len(vals[i]) != len(vals[j]) {
			return len(vals[i]) > len(vals[j])
		}
		return vals[i] < vals[j]
	})
	for _, v := range vals {
		text = replaceToken(text, v)
	}
	return text
}

// collectRefs adds the secret references of m to set. ref says m is the google.protobuf.Value (or
// ListValue) of a Struct field named *_ref: its strings are the reference.
func collectRefs(m protoreflect.Message, ref bool, set map[string]bool) {
	if !m.IsValid() {
		return
	}
	isStruct := m.Descriptor().FullName() == structFullName
	add := func(s string, isRef bool) {
		if s != "" && (isRef || secretRefRe.MatchString(s)) {
			set[s] = true
		}
	}
	// a Value/ListValue below a *_ref key passes the mark on; any other message starts afresh
	inherit := func(child protoreflect.Message) bool {
		n := child.Descriptor().FullName()
		return ref && (n == "google.protobuf.Value" || n == "google.protobuf.ListValue")
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		isRef := ref || refField(string(fd.Name()))
		switch {
		case fd.IsMap():
			v.Map().Range(func(mk protoreflect.MapKey, mv protoreflect.Value) bool {
				keyRef := isStruct && fd.MapKey().Kind() == protoreflect.StringKind && refField(mk.String())
				switch fd.MapValue().Kind() {
				case protoreflect.MessageKind, protoreflect.GroupKind:
					collectRefs(mv.Message(), keyRef, set)
				case protoreflect.StringKind:
					add(mv.String(), keyRef)
				}
				return true
			})
		case fd.IsList():
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				switch fd.Kind() {
				case protoreflect.MessageKind, protoreflect.GroupKind:
					collectRefs(l.Get(i).Message(), inherit(l.Get(i).Message()), set)
				case protoreflect.StringKind:
					add(l.Get(i).String(), isRef)
				}
			}
		case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
			collectRefs(v.Message(), inherit(v.Message()), set)
		case fd.Kind() == protoreflect.StringKind:
			add(v.String(), isRef)
		}
		return true
	})
}

func isTokenByte(c byte) bool {
	return c == '_' || c == '.' || c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// replaceToken replaces the whole-token occurrences of v in s by Redacted (rfkit.Redactor's rule:
// not preceded or followed by a letter, digit, '_', '.' or '-').
func replaceToken(s, v string) string {
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(s[i:], v)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		j += i
		end := j + len(v)
		left := j == 0 || !isTokenByte(s[j-1]) || !isTokenByte(v[0])
		right := end == len(s) || !isTokenByte(s[end]) || !isTokenByte(v[len(v)-1])
		b.WriteString(s[i:j])
		if left && right {
			b.WriteString(Redacted)
		} else {
			b.WriteString(v)
		}
		i = end
	}
}

// boundText cuts s to at most n bytes on a rune boundary and says so.
func boundText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf(" … (%d more bytes)", len(s)-cut)
}
