// Package idempotency proves on the host VPP that applying the same DF-2 desired state twice
// yields an empty plan: every retrievable DF-2 object type is created once, then the plan is
// recomputed the way the reconciler does it (docs of internal/scheduler: absent → Create,
// !proto.Equal → Update, owned-but-undesired → Delete) and must be empty. Write-only types
// (adl, classify ip/l2 table bindings, output-acl: no dump in the VPP API) are excluded and
// logged. Run with `go test -p 1` so no other DF-2 package's w<N> objects exist meanwhile.
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/abf"
	"ngfw/agent/internal/descriptors/arp"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/df2/df2test"
	ip6nd "ngfw/agent/internal/descriptors/ip6_nd"
	ipneighbor "ngfw/agent/internal/descriptors/ip_neighbor"
	sessionredirect "ngfw/agent/internal/descriptors/ip_session_redirect"
	"ngfw/agent/internal/descriptors/urpf"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

type applied struct {
	d    scheduler.Descriptor
	obj  proto.Message
	meta any
}

// plan diffs desired against the union of Retrieve over descs, as the reconciler does.
func plan(ctx context.Context, descs []scheduler.Descriptor, desired []applied) (scheduler.Plan, error) {
	actual := map[scheduler.Key]scheduler.KV{}
	for _, d := range descs {
		kvs, err := d.Retrieve(ctx)
		if err != nil {
			return scheduler.Plan{}, fmt.Errorf("%s: %w", d.Name(), err)
		}
		for _, kv := range kvs {
			actual[kv.Key] = kv
		}
	}
	var p scheduler.Plan
	want := map[scheduler.Key]bool{}
	for _, a := range desired {
		k := a.d.KeyOf(a.obj)
		want[k] = true
		kv, ok := actual[k]
		switch {
		case !ok:
			p.Create = append(p.Create, scheduler.KV{Key: k, Value: a.obj})
		case !proto.Equal(kv.Value, a.obj):
			p.Update = append(p.Update, scheduler.KV{Key: k, Value: a.obj, Meta: kv.Meta})
		}
	}
	for k, kv := range actual {
		if !want[k] {
			p.Delete = append(p.Delete, kv)
		}
	}
	return p, nil
}

// must fails t on err: must[T](t)(f()).
func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func TestApplyTwiceEmptyPlan(t *testing.T) {
	c := df2test.Connect(t)
	ctx := df2test.Ctx(t)
	owner := vpptest.Prefix(t)
	slot := df2test.Slot(t)
	base := vpptest.TableBase(t)
	ids := &df2.IDRange{Lo: base, Hi: base + 999}

	l1, i1 := df2test.Loopback(t, c, 50)
	l2, i2 := df2test.Loopback(t, c, 51)
	df2test.AddAddress(t, c, i1, fmt.Sprintf("10.%d.50.1/24", slot))
	df2test.AddAddress(t, c, i2, fmt.Sprintf("10.%d.51.1/24", slot))
	df2test.AddAddress(t, c, i1, fmt.Sprintf("2001:db8:%d:50::1/64", slot))
	df2test.VRF(t, c, base+50, false, "idem")
	df2test.ACL(t, c, "idem")
	store := must[*classify.FileStore](t)(classify.OpenFileStore(filepath.Join(t.TempDir(), "classify.json")))

	mask := make([]byte, 16)
	copy(mask[12:], []byte{255, 255, 255, 255})
	m1 := make([]byte, 16)
	copy(m1[12:], []byte{10, byte(slot), 50, 10}) //nolint:gosec // slot 1..12
	m2 := make([]byte, 16)
	copy(m2[12:], []byte{10, byte(slot), 50, 11}) //nolint:gosec // slot 1..12

	neigh := ipneighbor.NewNeighbor(c, owner)
	prange := arp.NewRange(c, ids)
	pif := arp.NewInterface(c, owner)
	rac := ip6nd.NewRaConfig(c, owner)
	rap := ip6nd.NewRaPrefix(c, owner)
	ur := urpf.New(c, owner)
	pol := abf.NewPolicy(c, owner, ids)
	att := abf.NewAttach(c, owner, ids)
	tbl := classify.NewTable(c, store)
	ses := classify.NewSession(c, store)
	iacl := classify.NewInputACL(c, owner, store)
	isr := sessionredirect.New(c, owner, store)
	descs := []scheduler.Descriptor{neigh, prange, pif, rac, rap, ur, pol, att, tbl, ses, iacl, isr}

	policy := must[*abf.Policy](t)(abf.NormalizePolicy(&abf.Policy{PolicyId: base + 50, Acl: "idem", Paths: []*df2.FibPath{{NextHop: fmt.Sprintf("10.%d.51.254", slot), Interface: l2}}}))
	redirect := must[*sessionredirect.Redirect](t)(sessionredirect.Normalize(&sessionredirect.Redirect{Table: owner + "-idem", Match: m2, OpaqueIndex: classify.NoIndex, Paths: []*df2.FibPath{{NextHop: fmt.Sprintf("10.%d.51.254", slot), Interface: l2}}}))
	// Desired state in dependency order (fixtures stand in for interface/vrf/acl keys).
	desired := []applied{
		{d: neigh, obj: &ipneighbor.Neighbor{Interface: l1, IpAddress: fmt.Sprintf("10.%d.50.10", slot), MacAddress: "02:00:00:03:50:10"}},
		{d: neigh, obj: &ipneighbor.Neighbor{Interface: l1, IpAddress: fmt.Sprintf("2001:db8:%d:50::10", slot), MacAddress: "02:00:00:03:50:11", NoFibEntry: true}},
		{d: prange, obj: &arp.ProxyRange{TableId: base + 50, Low: fmt.Sprintf("10.%d.50.100", slot), High: fmt.Sprintf("10.%d.50.120", slot)}},
		{d: pif, obj: &arp.ProxyInterface{Interface: l1}},
		{d: rac, obj: ip6nd.NormalizeRaConfig(&ip6nd.RaConfig{Interface: l1, Managed: true, RouterLifetime: 1200, MaxInterval: 400, MinInterval: 300})},
		{d: rap, obj: ip6nd.NormalizeRaPrefix(&ip6nd.RaPrefix{Interface: l1, Prefix: fmt.Sprintf("2001:db8:%d:50::/64", slot), ValidLifetime: 7200, PreferredLifetime: 3600})},
		{d: ur, obj: &urpf.Interface{Interface: l1, Mode: urpf.Interface_STRICT}},
		{d: pol, obj: policy},
		{d: att, obj: &abf.Attach{PolicyId: base + 50, Interface: l1, Priority: 5}},
		{d: tbl, obj: classify.NormalizeTable(&classify.Table{Name: owner + "-idem", MatchNVectors: 1, Mask: mask, MissNextIndex: classify.NoIndex, Nbuckets: 8})},
		{d: ses, obj: classify.NormalizeSession(&classify.Session{Table: owner + "-idem", Match: m1, HitNextIndex: classify.NoIndex, OpaqueIndex: classify.NoIndex})},
		{d: iacl, obj: &classify.InputAcl{Interface: l2, Ip4Table: owner + "-idem"}},
		{d: isr, obj: redirect},
	}

	first, err := plan(ctx, descs, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("apply #1 plan: create=%d update=%d delete=%d", len(first.Create), len(first.Update), len(first.Delete))
	if len(first.Create) != len(desired) || len(first.Update) != 0 || len(first.Delete) != 0 {
		t.Fatalf("first plan should create everything and nothing else (leftovers of %s?): %+v", owner, first)
	}
	for i := range desired {
		a := &desired[i]
		meta, err := a.d.Create(ctx, a.obj)
		if err != nil {
			t.Fatalf("create %s: %v", a.d.KeyOf(a.obj), err)
		}
		a.meta = meta
		t.Logf("  created %s", a.d.KeyOf(a.obj))
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), df2test.Timeout)
		defer cancel()
		for i := len(desired) - 1; i >= 0; i-- {
			if err := desired[i].d.Delete(ctx, desired[i].obj, desired[i].meta); err != nil {
				t.Errorf("cleanup %s: %v", desired[i].d.KeyOf(desired[i].obj), err)
			}
		}
	})

	second, err := plan(ctx, descs, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("apply #2 (same desired state) plan: create=%d update=%d delete=%d empty=%v", len(second.Create), len(second.Update), len(second.Delete), second.Empty())
	for _, kv := range append(append(second.Create, second.Update...), second.Delete...) {
		t.Errorf("  unexpected op on %s: %v", kv.Key, kv.Value)
	}
	for _, d := range []scheduler.Descriptor{classify.NewInterfaceIPTable(c, owner, store), classify.NewOutputACL(c, owner, store)} {
		if _, err := d.Retrieve(ctx); errors.Is(err, df2.ErrRetrieveUnsupported) {
			t.Logf("excluded (write-only, no VPP dump): %s", d.Name())
		}
	}
	df2test.Hold(t)
}
