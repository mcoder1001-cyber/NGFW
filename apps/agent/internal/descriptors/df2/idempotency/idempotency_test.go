// Package idempotency proves on the host VPP that applying the same DF-2 desired state twice
// yields an empty plan: every retrievable DF-2 object type is created once, then the plan is
// recomputed the way the reconciler does it (docs of internal/scheduler: absent → Create,
// !proto.Equal → Update, owned-but-undesired → Delete) and must be empty. Write-only types
// (adl, classify ip/l2 table bindings, output-acl: no dump in the VPP API) are excluded and
// logged. A second pass builds fresh descriptors on the reopened classify FileStore and claim
// store (an agent restart) and must plan nothing either. Run with `go test -p 1` so no other
// DF-2 package's w<N> objects exist meanwhile.
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/abf"
	"ngfw/agent/internal/descriptors/adl"
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
func descByName(ds []scheduler.Descriptor, name string) scheduler.Descriptor {
	for _, d := range ds {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

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
	l3, _ := df2test.UntaggedLoopback(t, c, 52) // stands in for a physical port (no owner tag)
	df2test.AddAddress(t, c, i1, fmt.Sprintf("10.%d.50.1/24", slot))
	df2test.AddAddress(t, c, i2, fmt.Sprintf("10.%d.51.1/24", slot))
	df2test.AddAddress(t, c, i1, fmt.Sprintf("2001:db8:%d:50::1/64", slot))
	df2test.VRF(t, c, base+50, false, "idem")
	df2test.ACL(t, c, "idem")
	dir := t.TempDir()
	storePath, claimsPath := filepath.Join(dir, "classify.json"), filepath.Join(dir, "claims.json")
	store := must[*classify.FileStore](t)(classify.OpenFileStore(storePath))
	claims := must[*df2.FileClaimStore](t)(df2.OpenFileClaimStore(claimsPath))

	mask := make([]byte, 16)
	copy(mask[12:], []byte{255, 255, 255, 255})
	m1 := make([]byte, 16)
	copy(m1[12:], []byte{10, byte(slot), 50, 10}) //nolint:gosec // slot 1..12
	m2 := make([]byte, 16)
	copy(m2[12:], []byte{10, byte(slot), 50, 11}) //nolint:gosec // slot 1..12

	type set struct {
		neigh  *ipneighbor.NeighborDescriptor
		prange *arp.RangeDescriptor
		pif    *arp.InterfaceDescriptor
		rac    *ip6nd.RaConfigDescriptor
		rap    *ip6nd.RaPrefixDescriptor
		ur     *urpf.Descriptor
		adli   *adl.InterfaceDescriptor
		pol    *abf.PolicyDescriptor
		att    *abf.AttachDescriptor
		tbl    *classify.TableDescriptor
		ses    *classify.SessionDescriptor
		iacl   *classify.InputACLDescriptor
		oacl   *classify.OutputACLDescriptor
		isr    *sessionredirect.Descriptor
		all    []scheduler.Descriptor
	}
	build := func(store classify.Store, claims df2.ClaimStore) set {
		o := df2.WithClaims(claims)
		x := set{
			neigh: ipneighbor.NewNeighbor(c, owner, o), prange: arp.NewRange(c, ids), pif: arp.NewInterface(c, owner, o),
			rac: ip6nd.NewRaConfig(c, owner, o), rap: ip6nd.NewRaPrefix(c, owner, o), ur: urpf.New(c, owner, o), adli: adl.NewInterface(c, owner, o),
			pol: abf.NewPolicy(c, owner, ids), att: abf.NewAttach(c, owner, ids, o), tbl: classify.NewTable(c, store), ses: classify.NewSession(c, store),
			iacl: classify.NewInputACL(c, owner, store, o), oacl: classify.NewOutputACL(c, owner, store, o), isr: sessionredirect.New(c, owner, store),
		}
		x.all = []scheduler.Descriptor{x.neigh, x.prange, x.pif, x.rac, x.rap, x.ur, x.adli, x.pol, x.att, x.tbl, x.ses, x.iacl, x.oacl, x.isr}
		return x
	}
	ds := build(store, claims)
	neigh, prange, pif, rac, rap, ur, pol, att, tbl, ses, iacl, isr := ds.neigh, ds.prange, ds.pif, ds.rac, ds.rap, ds.ur, ds.pol, ds.att, ds.tbl, ds.ses, ds.iacl, ds.isr
	descs := ds.all

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
		{d: ur, obj: &urpf.Interface{Interface: l3, Mode: urpf.Interface_LOOSE}}, // untagged: claimed
		{d: neigh, obj: &ipneighbor.Neighbor{Interface: l3, IpAddress: fmt.Sprintf("10.%d.52.10", slot), MacAddress: "02:00:00:03:52:10"}},
		{d: ds.adli, obj: &adl.Interface{Interface: l2}},
		{d: pol, obj: policy},
		{d: att, obj: &abf.Attach{PolicyId: base + 50, Interface: l1, Priority: 5}},
		{d: tbl, obj: classify.NormalizeTable(&classify.Table{Name: owner + "-idem", MatchNVectors: 1, Mask: mask, MissNextIndex: classify.NoIndex, Nbuckets: 8})},
		{d: ses, obj: classify.NormalizeSession(&classify.Session{Table: owner + "-idem", Match: m1, HitNextIndex: classify.NoIndex, OpaqueIndex: classify.NoIndex})},
		{d: iacl, obj: &classify.InputAcl{Interface: l2, Ip4Table: owner + "-idem"}},
		{d: ds.oacl, obj: &classify.OutputAcl{Interface: l3, Ip4Table: owner + "-idem"}}, // untagged: claimed
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
	// Agent restart: fresh descriptors on the reopened stores see the same state, same Meta.
	store2 := must[*classify.FileStore](t)(classify.OpenFileStore(storePath))
	claims2 := must[*df2.FileClaimStore](t)(df2.OpenFileClaimStore(claimsPath))
	fresh := build(store2, claims2)
	third, err := plan(ctx, fresh.all, desired)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("apply #3 (fresh descriptors, reopened classify store + claim store) plan: create=%d update=%d delete=%d empty=%v", len(third.Create), len(third.Update), len(third.Delete), third.Empty())
	for _, kv := range append(append(third.Create, third.Update...), third.Delete...) {
		t.Errorf("  unexpected op after restart on %s: %v", kv.Key, kv.Value)
	}
	for i := range desired {
		a := desired[i]
		kvs, err := descByName(fresh.all, a.d.Name()).Retrieve(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, kv := range kvs {
			if kv.Key == a.d.KeyOf(a.obj) && kv.Meta != a.meta {
				t.Errorf("  %s: Meta after restart %+v != Create's %+v", kv.Key, kv.Meta, a.meta)
			}
		}
	}
	for _, d := range []scheduler.Descriptor{classify.NewInterfaceIPTable(c, owner, store), classify.NewInterfaceL2Tables(c, owner, store), adl.NewAllowlist(c, owner)} {
		if _, err := d.Retrieve(ctx); errors.Is(err, df2.ErrRetrieveUnsupported) {
			t.Logf("excluded (write-only, no VPP dump): %s", d.Name())
		}
	}
	df2test.Hold(t)
}
