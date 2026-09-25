package policer

// F-qos-flat gap tests (docs/status/tasks/F-qos-flat.md "descriptor gaps"): the TD-11b ownership declarations the
// product agent requires before it registers the family, claim-first attachments, and the States / ResetIndex
// helpers behind the QosPolicerState / QosPolicerReset RPCs.

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/policer"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// regRecorder records what Register registers.
type regRecorder struct{ ds []scheduler.Descriptor }

func (r *regRecorder) Register(d scheduler.Descriptor) { r.ds = append(r.ds, d) }

// persistedClaims is an iface.ClaimStore that says it survives an agent restart and can refuse to record.
type persistedClaims struct {
	mu   sync.Mutex
	m    map[[2]string]bool
	fail error
}

func (c *persistedClaims) Persistent() bool { return true }
func (c *persistedClaims) Claim(n, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.m[[2]string{n, h}] = true
	return nil
}
func (c *persistedClaims) Release(n, h string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, [2]string{n, h})
	return nil
}
func (c *persistedClaims) Claimed(n, h string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[[2]string{n, h}]
}

type persistedBoot struct{ dfkit.BootStore }

func (persistedBoot) Persistent() bool { return true }

// TestOwnershipDeclared (TD-11b): every descriptor policer.Register registers declares how it records ownership,
// and the checks accept only stores that survive an agent restart.
func TestOwnershipDeclared(t *testing.T) {
	const owner = "w0qd"
	r := &regRecorder{}
	Register(r, df7test.NewFake(), owner)
	if len(r.ds) != 4 {
		t.Fatalf("registered %d descriptors", len(r.ds))
	}
	for _, d := range r.ds {
		if err := persist.Declared(d); err != nil {
			t.Errorf("%s: %v", d.Name(), err)
		}
	}
	// in-memory defaults are refused …
	iface.SetClaimStore(owner, nil)
	df7.SetBootStore(owner, nil)
	for _, d := range r.ds {
		if _, checks := d.(persist.Checker); checks && persist.Check(d) == nil {
			t.Errorf("%s accepted an in-memory store", d.Name())
		}
	}
	// … persisted ones pass
	iface.SetClaimStore(owner, &persistedClaims{m: map[[2]string]bool{}})
	df7.SetBootStore(owner, persistedBoot{dfkit.NewMemoryBootStore()})
	t.Cleanup(func() { iface.SetClaimStore(owner, nil); df7.SetBootStore(owner, nil) })
	for _, d := range r.ds {
		if err := persist.Check(d); err != nil {
			t.Errorf("%s: %v", d.Name(), err)
		}
	}
}

// TestAttachmentClaimFirst (TD-11b): on an untagged interface the claim is recorded before policer_input; a claim
// that cannot be recorded fails the Create with nothing sent to VPP, and a refused apply releases the claim.
func TestAttachmentClaimFirst(t *testing.T) {
	const owner = df7test.Owner
	claims := &persistedClaims{m: map[[2]string]bool{}}
	iface.SetClaimStore(owner, claims)
	t.Cleanup(func() { iface.SetClaimStore(owner, nil) })
	f, _ := fakePolicers(t)
	ctx := t.Context()
	if _, err := NewPolicer(f, owner).Create(ctx, df7.Encode(gold)); err != nil {
		t.Fatal(err)
	}
	var order []string
	refuse := false
	f.On("policer_input", func(api.Message) ([]api.Message, error) {
		order = append(order, "apply")
		if refuse {
			return []api.Message{&policer.PolicerInputReply{Retval: int32(api.INVALID_VALUE)}}, nil
		}
		return []api.Message{&policer.PolicerInputReply{}}, nil
	})
	d := NewInterface(f, owner)
	eth := df7.Encode(Attachment{Interface: "eth0", Direction: DirInput, Policer: "gold"})

	claims.fail = errors.New("disk full")
	if _, err := d.Create(ctx, eth); err == nil || len(f.CallsNamed("policer_input")) != 0 {
		t.Fatalf("a refused claim must fail before VPP is touched: %v, %d applies", err, len(f.CallsNamed("policer_input")))
	}
	claims.fail = nil
	refuse = true
	if _, err := d.Create(ctx, eth); err == nil {
		t.Fatal("apply refused by VPP must fail")
	}
	if df7test.Claimed(ctx, f, owner, "eth0", "policer.interface/eth0/input") {
		t.Fatal("claim of a refused apply not released")
	}
	refuse = false
	if _, err := d.Create(ctx, eth); err != nil {
		t.Fatal(err)
	}
	if !df7test.Claimed(ctx, f, owner, "eth0", "policer.interface/eth0/input") {
		t.Fatal("untagged attachment not claimed")
	}
}

// TestStatesAndResetIndex: the read-only walk behind QosPolicerState and the reset behind QosPolicerReset.
func TestStatesAndResetIndex(t *testing.T) {
	f, pool := fakePolicers(t)
	ctx := t.Context()
	d := NewPolicer(f, df7test.Owner)
	for _, p := range []Policer{gold, func() Policer { p := gold; p.Name = "shaper:up"; p.Type = Type1R2C; p.EIR, p.EB = 0, 0; return p }()} {
		if _, err := d.Create(ctx, df7.Encode(p)); err != nil {
			t.Fatal(err)
		}
	}
	pool[40] = &policer.PolicerDetails{Name: df7test.Other + ":gold", Cir: 1}
	pool[0].CurrentBucket, pool[0].CurrentLimit, pool[0].ExtendedLimit = 7, 9, 11
	st, err := States(ctx, f, df7test.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 2 || st[0].Spec.Name != "gold" || st[1].Spec.Name != "shaper:up" || st[0].Index != 0 || st[1].Index != 1 {
		t.Fatalf("states %+v", st)
	}
	if st[0].CurrentBucket != 7 || st[0].CurrentLimit != 9 || st[0].ExtendedLimit != 11 || st[0].Spec.CIR != gold.CIR {
		t.Fatalf("buckets %+v", st[0])
	}
	f.On("policer_reset", func(api.Message) ([]api.Message, error) {
		return []api.Message{&policer.PolicerResetReply{}}, nil
	})
	idx, err := ResetIndex(ctx, f, df7test.Owner, "shaper:up")
	if err != nil || idx != 1 || df7test.Last[*policer.PolicerReset](t, f, "policer_reset").PolicerIndex != 1 {
		t.Fatalf("reset %d %v", idx, err)
	}
	if _, err := ResetIndex(ctx, f, df7test.Owner, "nope"); !errors.Is(err, ErrNoPolicer) {
		t.Fatalf("unknown policer: %v", err)
	}
	// names are owner-scoped: the other agent's "gold" is its own, never ours
	if idx, err := ResetIndex(ctx, f, df7test.Other, "gold"); err != nil || idx != 40 {
		t.Fatalf("other owner: %d %v", idx, err)
	}
}

// TestAttachmentRepointsAfterPolicerLoss: VPP binds an attachment to the policer's pool index (policer_op.c) and
// policer_del leaves the binding dangling. When the policer is re-created behind the agent's back (new pool index),
// the resync's Create re-points the attachment — un-apply + apply, still exactly one feature instance — instead of
// skipping it on the applied-once record; a Delete once the policer is gone for good tolerates NO_SUCH_ENTRY.
func TestAttachmentRepointsAfterPolicerLoss(t *testing.T) {
	f, pool := fakePolicers(t)
	ctx := t.Context()
	pd := NewPolicer(f, df7test.Owner)
	if _, err := pd.Create(ctx, df7.Encode(gold)); err != nil {
		t.Fatal(err)
	}
	stack, bound := 0, uint32(0)
	var ops []string
	f.On("policer_input", func(m api.Message) ([]api.Message, error) {
		r := m.(*policer.PolicerInput)
		idx, ok := uint32(0), false
		for i, p := range pool {
			if p.Name == r.Name {
				idx, ok = i, true
			}
		}
		if !ok {
			return []api.Message{&policer.PolicerInputReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		if r.Apply {
			stack++
			bound = idx
			ops = append(ops, "apply")
		} else {
			stack--
			bound = ^uint32(0)
			ops = append(ops, "unapply")
		}
		return []api.Message{&policer.PolicerInputReply{}}, nil
	})
	d := NewInterface(f, df7test.Owner)
	in := df7.Encode(Attachment{Interface: "loop0", Direction: DirInput, Policer: "gold"})
	if _, err := d.Create(ctx, in); err != nil || stack != 1 || bound != 0 {
		t.Fatalf("first apply: %v stack %d bound %d", err, stack, bound)
	}
	if _, err := d.Create(ctx, in); err != nil || len(ops) != 1 { // resync, same policer: skipped (D-076)
		t.Fatalf("resync re-applied: %v %v", err, ops)
	}
	// the policer is deleted and re-created behind the agent's back: a new pool index
	delete(pool, 0)
	if _, err := pd.Create(ctx, df7.Encode(gold)); err != nil {
		t.Fatal(err)
	}
	newIdx, _, _ := LookupIndex(ctx, f, df7test.Owner, "gold")
	if newIdx == 0 {
		t.Fatal("the fake must give the re-created policer another index")
	}
	ops = nil
	if _, err := d.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ops, ",") != "unapply,apply" || stack != 1 || bound != newIdx {
		t.Fatalf("re-point: ops %v stack %d bound %d (want %d)", ops, stack, bound, newIdx)
	}
	if _, err := d.Create(ctx, in); err != nil || len(ops) != 2 {
		t.Fatalf("after the re-point a resync must skip again: %v %v", err, ops)
	}
	// gone for good: the un-apply cannot run (NO_SUCH_ENTRY) and Delete still succeeds and forgets the record
	delete(pool, newIdx)
	if err := d.Delete(ctx, in, nil); err != nil {
		t.Fatalf("delete with the policer gone: %v", err)
	}
	if ok, _, _ := d.appliedHere(ctx, string(KeyInterface("loop0", DirInput)), df7.IfaceValue(1, "loop0")); ok {
		t.Fatal("record kept after delete")
	}
}
