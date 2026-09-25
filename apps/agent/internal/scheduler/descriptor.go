// Package scheduler defines the contract between the declarative reconciler (task P05)
// and the object descriptors written by the descriptor factories (DF-*). It contains the
// interfaces and the descriptor registry only; the reconciler itself lands in P05.
//
// The design follows docs/01-architecture.md AD-3 (a Kubernetes-style controller modelled
// on Ligato's KVScheduler):
//
//	desired state (protobuf) → per-object Descriptor → dependency graph → topological order
//	  → diff(desired, Retrieve()) → Plan → apply → verify by re-Retrieve → (rollback on error)
//
// # Keys
//
// Every object in desired and actual state is identified by a Key of the form
// "<descriptor name>/<object id>", built with Join. The segment before the first "/" is the
// descriptor name, so the reconciler can route any key back to its Descriptor. The id must be
// stable across restarts and human readable (an interface name, "vrf/prefix", …) — never a
// runtime handle such as sw_if_index; those belong in KV.Meta.
//
// # Transaction semantics (what P05 implements, what descriptors may rely on)
//
//  1. Validate: every desired KV has a registered Descriptor; every non-optional Dependency
//     key exists either in the desired set or in the retrieved actual state. A missing
//     mandatory dependency fails the transaction before anything is applied.
//  2. Order: objects are sorted topologically by their Dependencies (dependency before
//     dependent). Ties are broken by stage (StageVPP before StageDaemon, see Stages below), then
//     by descriptor registration order, then by key, so the order is deterministic. Optional
//     dependencies only influence ordering; they never block.
//  3. Diff: the actual state is the union of Descriptor.Retrieve() over all descriptors.
//     For each desired key: absent in actual → Create; present and proto.Equal(desired,
//     actual) → no-op; present and different → Update. Actual keys that are owned by this
//     agent (see Ownership) and absent from desired → Delete. Applying the same desired
//     state twice therefore yields an empty Plan (idempotency).
//     Check (TD-13): every Create and Update of a descriptor that implements Validator is
//     validated against the state after the transaction (see Validators below). A rejection
//     fails the transaction before its first operation: FAILED, nothing touched.
//  4. Apply: Creates and Updates run in topological order, Deletes in reverse topological
//     order (dependents first). Update may return ErrRecreate; the reconciler then Deletes
//     the old object and Creates the new one, and re-creates every dependent object as
//     well, because their handles (sw_if_index, …) may have been invalidated.
//  5. Verify: after apply the reconciler re-Retrieves and compares; a mismatch is an error.
//  6. Rollback: on any error the operations already applied in this transaction are
//     reverted in reverse order (Create → Delete, Delete → Create, Update → Update back with
//     the old value). A Create that failed after changing VPP (Meta with PartialCreate(err))
//     is journaled too and deleted by the rollback. The transaction result is
//     ROLLED_BACK with one Result per key. A failed rollback marks the agent DEGRADED (AD-4).
//
// The reconciler runs one transaction at a time; Create/Update/Delete of a descriptor are
// never called concurrently with each other. Retrieve may be called at any time (drift
// detection, state RPCs) and must be safe to call concurrently with itself. Every method
// honours ctx cancellation and deadlines.
//
// # Validators (tier 3) and stages (TD-13, D-125 ARCH-02)
//
// A descriptor whose objects are a daemon's configuration implements Validator (validator.go) and
// declares StageDaemon (Stager, stage.go). Validate renders the configuration into a private temp
// dir and runs the daemon's own checker on it (kea-dhcp4 -t, unbound-checkconf, nft -c,
// vtysh -C, …). The scheduler calls it for every Create and Update of the descriptor's objects in
// Plan (the DryRun RPC) and in every Apply, after planning and before the first operation — so a
// configuration the daemon would refuse never follows VPP writes of the same transaction. The
// contract a Validator must keep:
//
//   - read only: no side effect outside a private temp dir it removes again — no daemon file
//     written, no reload or restart, no VPP call, no ownership claim; a plan may run it any
//     number of times;
//   - never call back into the Scheduler (Plan holds its read lock, Apply its write lock): what it
//     needs of other objects is in the view;
//   - bounded: it honours ctx, which carries a deadline (Scheduler.ValidateTimeout, default
//     DefaultValidateTimeout = 30 s); the scheduler stops waiting at the deadline and a panic is
//     recovered — both are findings;
//   - safe for concurrent use with itself, Retrieve and (once abandoned at its deadline) the next
//     transaction's operations;
//   - its error names the offending leaf with InvalidAt(pointer, err) when the value carries a
//     pointer, and never carries a secret: every plaintext it resolved is masked by the validator
//     (rfkit.Redactor); the scheduler masks the value's secret references — D-051 references and
//     *_ref fields, the only secret leaves D-040 lets cross (RedactLeaves).
//
// A finding is an Issue with Rule RuleValidator; DryRun and a FAILED Apply report it as a
// ValidationIssue (rule "agent.validator") with the key in its message. Create still validates
// what it writes (defence in depth); it simply no longer finds a configuration the plan rejected.
//
// The stage breaks ties in the dependency sort (the first tie-breaker, greedy): of the operations
// ready at the same time, VPP-stage creates and updates go before daemon-stage ones, and deletes the
// other way round. A real dependency always wins, and the rollback is the exact reverse of what
// ran. The periodic drift check plans with PlanOptions.SkipValidators. See
// docs/agent/scheduler-validators.md.
//
// # Meta
//
// Meta is opaque per-object metadata a descriptor needs later (typically the VPP handle
// returned by Create, e.g. sw_if_index). The reconciler stores it, never inspects it, hands
// it back to Update and Delete, and replaces it with what Retrieve returns after a restart.
// Meta is not persisted: Retrieve must fill KV.Meta exactly as Create would.
//
// # Retrieve rules (restart safety depends on these)
//
//   - Dump everything of the object type from VPP or the daemon, then keep only objects
//     owned by this agent (see Ownership). Return owned objects that are not in the desired
//     state too — that is how drift and leftovers get deleted.
//   - Decode into the same proto type the desired state uses, canonicalised (IP addresses
//     through net/netip, repeated fields sorted, MACs lower-case) so that proto.Equal is a
//     correct diff. Read-only status (link state, counters) is never part of the Value.
//   - A descriptor without Retrieve is not done; Retrieve is what makes "rebuild the data
//     plane from the datastore after kill -9 vpp" possible.
//
// # Ownership (shared VPP)
//
// The agent is started with an owner id (VRX_OWNER, default "vrx"; tests use their slot's
// VRX_TEST_PREFIX). Descriptors tag every object they create with it (interface tags via
// sw_interface_tag_add_del, owner-prefixed names or an owner table for objects without tags)
// and filter Retrieve by it. Two agents with different owners on one VPP must never touch
// each other's objects. See internal/vpp OwnerTag and internal/descriptors/README.md.
package scheduler

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/proto"
)

// Key identifies one object in desired and actual state: "<descriptor name>/<object id>".
type Key string

// KeySeparator separates the descriptor name from the object id inside a Key.
const KeySeparator = "/"

// Join builds a Key from a descriptor name and the object id parts, e.g.
// Join("interface.loopback", "loop200") == "interface.loopback/loop200" and
// Join("ip.route", "vrf10", "10.0.0.0/24") == "ip.route/vrf10/10.0.0.0/24".
func Join(descriptor string, id ...string) Key {
	parts := make([]string, 0, 1+len(id))
	parts = append(parts, descriptor)
	parts = append(parts, id...)
	return Key(strings.Join(parts, KeySeparator))
}

// Descriptor returns the descriptor name segment of the key (everything before the first
// KeySeparator). It is the name the reconciler looks up in the Registry.
func (k Key) Descriptor() string {
	name, _, _ := strings.Cut(string(k), KeySeparator)
	return name
}

// ID returns the object id segment of the key (everything after the first KeySeparator).
func (k Key) ID() string {
	_, id, _ := strings.Cut(string(k), KeySeparator)
	return id
}

// String implements fmt.Stringer.
func (k Key) String() string { return string(k) }

// KV is one object: its Key, its desired or retrieved Value and its opaque Meta.
type KV struct {
	Key   Key
	Value proto.Message
	Meta  any
}

// Dependency names another object that must exist before this one is created (and that is
// deleted only after this one). Optional dependencies only order the plan when the key is
// present; a missing mandatory dependency fails the transaction.
type Dependency struct {
	Key      Key
	Optional bool
}

// ErrPartialCreate marks a Create error returned AFTER the Create changed VPP (see
// Descriptor.Create): the Create is journaled — with the Meta returned alongside, nil included — and
// the rollback Deletes the partial object. Wrap the error with PartialCreate; test with
// IsPartialCreate.
var ErrPartialCreate = errors.New("create failed after changing the data plane")

// IsPartialCreate reports whether a Create error says VPP was changed (PartialCreate). It is the one
// predicate the reconciler (journal the Create) and the claim-first descriptors (keep the claim for
// the rollback's Delete) share, so they never disagree (TD-11b fix round 1, review M2). The marker
// alone decides: a nil Meta is journaled too, and a Delete that needs a Meta then fails loudly
// (DEGRADED) instead of leaving an unjournaled object behind silently.
func IsPartialCreate(err error) bool { return errors.Is(err, ErrPartialCreate) }

// PartialCreate marks err as a partial-Create failure (errors.Is(err, ErrPartialCreate)); the
// message is err's own. nil stays nil.
func PartialCreate(err error) error {
	if err == nil {
		return nil
	}
	return partialCreate{err}
}

type partialCreate struct{ err error }

func (p partialCreate) Error() string        { return p.err.Error() }
func (p partialCreate) Unwrap() error        { return p.err }
func (p partialCreate) Is(target error) bool { return target == ErrPartialCreate }

// ErrRecreate is returned by Descriptor.Update when the change cannot be applied in place
// (an immutable field changed). The reconciler then Deletes the old object and Creates the
// new one, re-creating dependents as well.
var ErrRecreate = errors.New("update requires recreate")

// Descriptor implements Create/Update/Delete/Retrieve/Dependencies for one object type. One
// descriptor per VPP object type or daemon config unit; see internal/descriptors/README.md
// for how to write one and internal/scheduler/example_descriptor_test.go for a worked
// example against the fake VPP client.
type Descriptor interface {
	// Name is the unique descriptor name, "<plugin>.<object>" in lower-case letters, digits,
	// "-" and ".", never "/" (it is the first segment of every Key), e.g. "interface.loopback".
	Name() string
	// KeyOf returns the Key of a desired object: Join(Name(), <stable id>). It must be a pure
	// function of obj.
	KeyOf(obj proto.Message) Key
	// Dependencies lists the keys obj depends on. Pure function of obj; may return nil.
	Dependencies(obj proto.Message) []Dependency
	// Create creates obj in VPP or the daemon and returns its Meta (e.g. sw_if_index).
	//
	// Create need not be atomic, but a failure must say what it left behind: a Create that
	// fails AFTER it changed VPP (the add succeeded, then an event subscription or a claim
	// record failed) returns the Meta of what it made (nil when it has none) together with
	// PartialCreate(err); the reconciler journals that partial object and the rollback calls
	// Delete(obj, meta), so Delete must accept it (keep any ownership claim until Delete releases
	// it; a Delete that cannot work without the Meta fails, and the agent is DEGRADED). Any other
	// error means nothing was changed, whatever Meta comes with it (many descriptors return a
	// zero Meta value with a failed add; deleting after "address in use" would fail or, worse,
	// remove an existing object). Ownership claims are recorded BEFORE the VPP call and released
	// when the call fails (TD-11b, review 3.3): a claim that cannot be recorded then fails the
	// Create with nothing written.
	Create(ctx context.Context, obj proto.Message) (meta any, err error)
	// Update changes an existing object from oldObj to newObj in place and returns the new
	// Meta, or returns ErrRecreate when the change is not possible in place.
	Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error)
	// Delete removes obj using the Meta returned by Create/Update/Retrieve.
	Delete(ctx context.Context, obj proto.Message, meta any) error
	// Retrieve dumps the ACTUAL state from VPP or the daemon: every object of this type owned
	// by this agent, decoded into the desired proto type and canonicalised, with Meta filled.
	Retrieve(ctx context.Context) ([]KV, error)
}

// Registry is where plugins register their descriptors; the reconciler routes keys through
// it. Register panics on an invalid or duplicate name (registration happens once at start-up
// and a duplicate is a programming error); use MapRegistry.Add for the error-returning form.
type Registry interface {
	Register(d Descriptor)
}

// Operation names used in Result.Op.
const (
	OpCreate   = "create"
	OpUpdate   = "update"
	OpDelete   = "delete"
	OpRecreate = "recreate"
	OpRevert   = "revert"
)

// Plan is the diff between desired and actual state: what a transaction will do, in the
// order it will do it (Create and Update in topological order, Delete in reverse). Update
// entries carry the desired Value and the Meta of the actual object.
type Plan struct {
	Create []KV
	Update []KV
	Delete []KV
}

// Empty reports whether the plan contains no operation (the idempotency check).
func (p Plan) Empty() bool { return p.Len() == 0 }

// Len returns the number of operations in the plan.
func (p Plan) Len() int { return len(p.Create) + len(p.Update) + len(p.Delete) }

// Result is the per-object outcome of a transaction: the operation attempted (one of the Op
// constants) and its error, nil on success.
type Result struct {
	Key Key
	Op  string
	Err error
}
