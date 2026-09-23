package df6_test

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	greapi "ngfw/agent/binapi/gre"
	"ngfw/agent/binapi/interface_types"
	srapi "ngfw/agent/binapi/sr"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	"ngfw/agent/internal/descriptors/gre"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/descriptors/vxlan"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

type obj struct {
	d scheduler.Descriptor
	v proto.Message
}

func descriptors(c vpp.Client, owner string, tun *gre.Tunnel, sid *sr.LocalSid, pol *sr.Policy, byp *vxlan.Bypass) []obj {
	return []obj{
		{gre.NewTunnel(c, owner), tun},
		{sr.NewLocalSid(c, owner), sid},
		{sr.NewPolicy(c, owner), pol},
		{vxlan.NewBypass(c, owner), byp},
	}
}

// plan logs and returns the number of planned operations for the readable descriptors
// (write-only ones are re-applied by P05 on every resync, D-063/D-076).
func plan(t *testing.T, h *df6test.Host, set []obj, stage string) int {
	t.Helper()
	total := 0
	for _, o := range set {
		p, err := df6test.PlanFor(h.Ctx, o.d, o.v)
		if err != nil {
			t.Logf("%s: plan %s: %v (write-only: re-applied on resync)", stage, o.d.Name(), err)
			continue
		}
		t.Logf("%s: plan %s: create=%d update=%d delete=%d", stage, o.d.Name(), len(p.Create), len(p.Update), len(p.Delete))
		total += p.Len()
	}
	return total
}

func apply(t *testing.T, ctx context.Context, set []obj, stage string) {
	t.Helper()
	for _, o := range set {
		kv, err := o.d.Retrieve(ctx)
		present := false
		for _, x := range kv {
			if x.Key == o.d.KeyOf(o.v) {
				present = true
			}
		}
		if err == nil && present {
			continue
		}
		if _, err := o.d.Create(ctx, o.v); err != nil {
			t.Fatalf("%s: create %s: %v", stage, o.d.KeyOf(o.v), err)
		}
		t.Logf("%s: applied %s", stage, o.d.KeyOf(o.v))
	}
}

// TestAgentRestartOnHost (review M5): an agent creates prefixed objects; a restarted agent
// (fresh API connection, fresh descriptors, claim store reopened from its file) plans nothing;
// after the objects are deleted behind its back (simulated loss via binapi) it plans creates
// for exactly those, re-applies them, and plans nothing again.
func TestAgentRestartOnHost(t *testing.T) {
	h := df6test.Connect(t)
	store := filepath.Join(t.TempDir(), "claims.json")
	s1, err := df6.OpenFileClaimStore(store)
	if err != nil {
		t.Fatal(err)
	}
	iface.SetClaimStore(h.Owner, s1)
	t.Cleanup(func() { iface.SetClaimStore(h.Owner, nil) })

	loop, _ := h.Loopback(20, h.IP6(0x20, 1)+"/64")
	tun := &gre.Tunnel{Instance: uint32(h.Slot*100 + 20), Src: h.IP4(20, 1), Dst: h.IP4(20, 2)} //nolint:gosec // slot ≤ 12
	sid := &sr.LocalSid{Sid: h.IP6(0x5a, 1), Behavior: sr.Behavior_END}
	pol := &sr.Policy{Bsid: h.IP6(0xba, 1), Encap: true, EncapSrc: h.IP6(0x20, 1), SidLists: []*sr.SidList{{Sids: []string{h.IP6(0x5a, 1)}, Weight: 1}}}
	byp := &vxlan.Bypass{Interface: loop, Ipv4: true}

	a := descriptors(h.Client, h.Owner, tun, sid, pol, byp)
	for i := len(a) - 1; i >= 0; i-- {
		o := a[i]
		t.Cleanup(func() { _ = o.d.Delete(context.Background(), o.v, nil) })
	}
	apply(t, h.Ctx, a, "agent 1")
	if n := plan(t, h, a, "agent 1 re-apply"); n != 0 {
		t.Fatalf("agent 1 re-apply plans %d operations", n)
	}

	// agent restart: new connection, new descriptors, claim store reopened from disk
	c2 := h.Reconnect()
	s2, err := df6.OpenFileClaimStore(store)
	if err != nil {
		t.Fatal(err)
	}
	iface.SetClaimStore(h.Owner, s2)
	b := descriptors(c2, h.Owner, tun, sid, pol, byp)
	if n := plan(t, h, b, "agent 2 after restart"); n != 0 {
		t.Fatalf("restarted agent plans %d operations, want 0", n)
	}
	// resync re-apply of the write-only bypass is a no-op (claim for this VPP boot)
	if _, err := b[3].d.Create(h.Ctx, byp); err != nil {
		t.Fatal(err)
	}

	// simulated loss: delete our gre tunnel and local SID via binapi, behind the agent's back
	gt, _ := df6.AddressOf(tun.GetSrc())
	gd, _ := df6.AddressOf(tun.GetDst())
	if _, err := greapi.NewServiceClient(c2).GreTunnelAddDelV2(h.Ctx, &greapi.GreTunnelAddDelV2{IsAdd: false, Tunnel: greapi.GreTunnelV2{Instance: tun.GetInstance(), Src: gt, Dst: gd}}); err != nil {
		t.Fatalf("simulated loss (gre): %v", err)
	}
	sa, _ := df6.IP6Of(sid.GetSid())
	if _, err := srapi.NewServiceClient(c2).SrLocalsidAddDel(h.Ctx, &srapi.SrLocalsidAddDel{IsDel: true, Localsid: sa, SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)}); err != nil {
		t.Fatalf("simulated loss (localsid): %v", err)
	}
	if n := plan(t, h, b, "agent 2 after loss"); n != 2 {
		t.Fatalf("after losing 2 objects the plan has %d operations, want 2 creates", n)
	}
	apply(t, h.Ctx, b, "agent 2 reconcile")
	h.Hold()
	if n := plan(t, h, b, "agent 2 after reconcile"); n != 0 {
		t.Fatalf("after reconcile the plan has %d operations", n)
	}
}
