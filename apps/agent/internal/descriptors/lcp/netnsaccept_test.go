package lcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/lcp"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
)

// S-lcp-netns-224-accept: a pair outside the lcp default namespace gets an API-sourced
// (*,224.0.0.0/24) Accept on its phy at Create, re-added on a re-apply, removed at Delete.
func TestNetnsPairInstallsAndRemovesAccept(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	ctx := context.Background()
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if m.apiAccept[7] != 1 || m.apiAccept[^uint32(0)] != 1 || len(m.apiAccept) != 2 {
		t.Fatalf("after create: %v, want Accept on 7 + local Forward", m.apiAccept)
	}
	delete(m.apiAccept, 7) // simulated loss; re-apply (agent restart) restores it
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if m.apiAccept[7] != 1 {
		t.Fatal("re-apply did not restore the Accept")
	}
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if len(m.apiAccept) != 0 {
		t.Fatalf("after delete: %v", m.apiAccept)
	}
	if last := m.mlog[len(m.mlog)-1]; last != "mroute-del" {
		t.Fatalf("log %v", m.mlog)
	}
}

// Another owner's Accept left in the API source keeps the shared local Forward path.
func TestNetnsPairKeepsLocalWhileOtherAccept(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	ctx := context.Background()
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	m.apiAccept[8] = 1 // w6's pair
	if err := d.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	if m.apiAccept[7] != 0 || m.apiAccept[8] != 1 || m.apiAccept[^uint32(0)] != 1 {
		t.Fatalf("after delete: %v, want w6's Accept and the local path kept", m.apiAccept)
	}
}

// Root/default-namespace pairs are linux-cp's: no API mroute is ever added. (The fake's table shows
// linux-cp's lone local path, which the dump cannot tell from a leftover API one: Create sends the
// API local delete, a no-op for VPP — TD-lcp-leftover-local-path, leftover_test.go.)
func TestDefaultNetnsPairNoAPIAccept(t *testing.T) {
	for _, tc := range []struct{ def, ns string }{{"", ""}, {"ns-w5", "ns-w5"}, {"ns-w5", ""}} {
		f, m := newFake()
		m.ns = tc.def
		d := NewItfPair(f, "w5", WithHostAddrFlusher(noFlush{}))
		v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: tc.ns}.Proto()
		if _, err := d.Create(context.Background(), v); err != nil {
			t.Fatal(err)
		}
		vd := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: tc.def}.Proto()
		if err := d.Delete(context.Background(), vd, nil); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(m.mlog, "mroute-add") || len(m.apiAccept) != 0 {
			t.Fatalf("default %q pair %q: mroute calls %v, API paths %v", tc.def, tc.ns, m.mlog, m.apiAccept)
		}
	}
}

type noFlush struct{}

func (noFlush) FlushIPv4(int, string) (int, error) { return 0, nil }

// D-217 Q1: a netns pair is refused in a table holding a default-namespace pair (linux-cp's
// plugin-low Accept stays the one forwarded); in another table it is accepted.
func TestNetnsPairRefusedInMixedTable(t *testing.T) {
	f, m := newFake()
	m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP}
	m.lowAccept[8] = true
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	_, err := d.Create(context.Background(), v)
	if err == nil || !strings.Contains(err.Error(), "D-217") || !strings.Contains(err.Error(), "table 0") {
		t.Fatalf("create: %v, want D-217 refusal", err)
	}
	if _, ok := m.pairs[7]; ok || len(m.mlog) != 0 || !m.lowAccept[8] {
		t.Fatalf("refusal left state: pair %v mroutes %v", m.pairs[7], m.mlog)
	}
	if ok, _ := acceptIn(context.Background(), f, 0, 8); !ok {
		t.Fatal("plugin-low Accept of w6 no longer forwarded")
	}
	m.table[7] = 5 // loop501 in VRF 5: no default-ns pair there
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
}

// The reverse: a default-namespace pair is refused in a table holding a netns pair.
func TestDefaultPairRefusedInNetnsTable(t *testing.T) {
	f, m := newFake()
	m.pairs[8] = lcp.LcpItfPairDetails{PhySwIfIndex: 8, HostIfName: "w6-lcp0", HostIfType: lcp.LCP_API_ITF_HOST_TAP, Netns: "ns-w6"}
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap"}.Proto()
	if _, err := d.Create(context.Background(), v); err == nil || !strings.Contains(err.Error(), "D-217") {
		t.Fatalf("create: %v, want D-217 refusal", err)
	}
}

// Review M2: a failing Accept install rolls the unclaimed pair back.
func TestNetnsPairRollbackOnAcceptFailure(t *testing.T) {
	f, m := newFake()
	f.On("ip_mroute_add_del", func(api.Message) ([]api.Message, error) { return nil, errors.New("boom") })
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	if _, err := d.Create(context.Background(), v); err == nil {
		t.Fatal("want error")
	}
	if _, ok := m.pairs[7]; ok {
		t.Fatal("pair leaked after failed Accept install")
	}
}

// Review M3: Delete removes the Accept by the stored flag even after the default netns changed
// to the pair's namespace.
func TestNetnsPairDeleteAfterDefaultChange(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	meta, err := d.Create(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	m.ns = "ns-w5"
	if err := d.Delete(context.Background(), v, meta); err != nil {
		t.Fatal(err)
	}
	if len(m.apiAccept) != 0 {
		t.Fatalf("API Accept left: %v", m.apiAccept)
	}
}

// Without meta (after a restart) Delete removes the Accept VPP shows on the phy.
func TestNetnsPairDeleteWithoutMeta(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	m.ns = "ns-w5"
	if err := d.Delete(context.Background(), v, nil); err != nil {
		t.Fatal(err)
	}
	if len(m.apiAccept) != 0 {
		t.Fatalf("API Accept left: %v", m.apiAccept)
	}
}

// Review M4: Retrieve reports a netns pair without its Accept as drift.
func TestRetrieveReportsMissingAccept(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	kvs, err := d.Retrieve(context.Background())
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, v) || !kvs[0].Meta.(PairMeta).APIAccept {
		t.Fatalf("retrieve %v %v", kvs, err)
	}
	delete(m.apiAccept, 7)
	kvs, _ = d.Retrieve(context.Background())
	var got ItfPair
	_ = dfkit.Decode(kvs[0].Value, &got)
	if got.Drift != DriftAPIAcceptMissing || proto.Equal(kvs[0].Value, v) {
		t.Fatalf("no drift reported: %+v", got)
	}
}

// Ruling finding 1: the full drift-repair cycle the scheduler runs on ErrRecreate.
func TestDriftRepairCycle(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	v := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}.Proto()
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	delete(m.apiAccept, 7)
	kvs, err := d.Retrieve(context.Background())
	if err != nil || len(kvs) != 1 {
		t.Fatalf("retrieve %v %v", kvs, err)
	}
	cur := kvs[0]
	if _, err := d.Update(context.Background(), cur.Value, v, cur.Meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("update: %v, want ErrRecreate", err)
	}
	if err := d.Delete(context.Background(), cur.Value, cur.Meta); err != nil {
		t.Fatalf("delete drifted: %v", err)
	}
	if _, err := d.Create(context.Background(), v); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if len(m.apiAccept) == 0 {
		t.Fatal("Accept not restored")
	}
	kvs, _ = d.Retrieve(context.Background())
	var got ItfPair
	_ = dfkit.Decode(kvs[0].Value, &got)
	if got.Drift != "" {
		t.Fatalf("still drifted: %+v", got)
	}
}

// A drifted value can be deleted directly (no meta, as after a restart).
func TestDeleteDriftedPair(t *testing.T) {
	f, m := newFake()
	d := NewItfPair(f, "w5")
	base := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5"}
	if _, err := d.Create(context.Background(), base.Proto()); err != nil {
		t.Fatal(err)
	}
	drifted := base
	drifted.Drift = DriftAPIAcceptMissing
	if err := d.Delete(context.Background(), drifted.Proto(), nil); err != nil {
		t.Fatalf("delete drifted: %v", err)
	}
	if len(m.apiAccept) != 0 {
		t.Fatalf("API Accept left: %v", m.apiAccept)
	}
}

// Drift in desired config is still refused (desired/lcp.go calls ItfPair.Validate).
func TestDesiredDriftRefused(t *testing.T) {
	p := ItfPair{Interface: "loop501", HostIfName: "w5-lcp0", HostIfType: "tap", Netns: "ns-w5", Drift: DriftAPIAcceptMissing}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("validate: %v, want drift refusal", err)
	}
}
