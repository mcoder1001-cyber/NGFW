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
//     plan must be free to run a validator any number of times (DryRun, the drift check, a retry
//     without the dynamic sources).
//   - Bounded by ctx. The scheduler gives every call a deadline (Scheduler.ValidateTimeout,
//     DefaultValidateTimeout) and stops waiting when it passes; a checker is started with
//     exec.CommandContext (or renderers.Command's Timeout) so it dies with ctx. A call that did not
//     return in time is a finding; a panic is a finding too (its stack goes to the agent log).
//   - A non-nil error rejects the object. Wrap it with InvalidAt to name the offending leaf by its
//     RFC 6901 pointer into the configuration document, when the value carries one.
//   - No secrets in the error. A validator that resolves secret references (rfkit.Secrets) masks
//     what it resolved (rfkit.Redactor.Error) before it returns. The scheduler additionally masks
//     the secret leaves of value (secret-named fields and D-051 references) and bounds the text.
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

// DefaultValidateTimeout bounds one Validate call when Scheduler.ValidateTimeout is 0. It is the
// 30 s bound of one VPP reply (TD-9: vpp.DefaultReplyTimeout, not merged on this base — the rebase
// may reuse that constant).
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

// validate runs the Validators of p's Create and Update operations (plan order) against the state
// after the transaction (after) and records every rejection as an Issue; a plan with issues keeps
// no operation. It returns an error only when ctx ended: the transaction was cancelled, not the
// configuration found invalid.
func (s *Scheduler) validate(ctx context.Context, p *TxnPlan, after map[Key]KV) error {
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
				s.log.Error("validator panicked", "key", k, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
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

// secretRefRe is a D-051 secret-store reference ("psk/site-a"). The value is not the secret, but a
// checker that echoes a configuration line prints what stands next to it.
var secretRefRe = regexp.MustCompile(`^(psk|key|cert|password|token)/[A-Za-z0-9_.-]{1,64}$`)

// secretNameParts mark a field (or a map key, e.g. a structpb field) as a secret leaf: every string
// under it is masked.
var secretNameParts = []string{"secret", "password", "passwd", "passphrase", "psk", "preshared", "pre_shared", "privatekey", "private_key", "community"}

func secretName(name string) bool {
	n := strings.ToLower(name)
	for _, p := range secretNameParts {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// RedactLeaves returns text with every secret leaf of value masked as Redacted: the strings held by
// secret-named fields or map keys (secret, password, psk, private_key, community, …; everything
// below such a field) and every D-051 secret reference anywhere in value. A leaf is masked only
// where it stands as a whole token, so a short value never blanks part of an unrelated word.
func RedactLeaves(text string, value proto.Message) string {
	if value == nil {
		return text
	}
	set := map[string]bool{}
	collectSecrets(value.ProtoReflect(), false, set)
	if len(set) == 0 {
		return text
	}
	vals := make([]string, 0, len(set))
	for v := range set {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool { // longest first, so a secret containing another stays whole
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

// collectSecrets adds the secret strings of m to set; secret says m sits under a secret-named field.
func collectSecrets(m protoreflect.Message, secret bool, set map[string]bool) {
	if !m.IsValid() {
		return
	}
	add := func(s string, under bool) {
		if s != "" && (under || secretRefRe.MatchString(s)) {
			set[s] = true
		}
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		under := secret || secretName(string(fd.Name()))
		switch {
		case fd.IsMap():
			v.Map().Range(func(mk protoreflect.MapKey, mv protoreflect.Value) bool {
				entry := under
				if fd.MapKey().Kind() == protoreflect.StringKind && secretName(mk.String()) {
					entry = true
				}
				switch fd.MapValue().Kind() {
				case protoreflect.MessageKind, protoreflect.GroupKind:
					collectSecrets(mv.Message(), entry, set)
				case protoreflect.StringKind:
					add(mv.String(), entry)
				}
				return true
			})
		case fd.IsList():
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				switch fd.Kind() {
				case protoreflect.MessageKind, protoreflect.GroupKind:
					collectSecrets(l.Get(i).Message(), under, set)
				case protoreflect.StringKind:
					add(l.Get(i).String(), under)
				}
			}
		case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
			collectSecrets(v.Message(), under, set)
		case fd.Kind() == protoreflect.StringKind:
			add(v.String(), under)
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
