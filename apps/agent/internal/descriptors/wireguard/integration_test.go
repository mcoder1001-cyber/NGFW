package wireguard_test

// Host checks against the shared VPP (VRX_INTEGRATION=1, shared lab lock, slot prefix), through
// P05's reconciler (vpntest.Agent) including the restart simulation. The interface is wg<table base + 1> tagged "<prefix>:wg…", its listen port is
// 20000+100*slot+10 (DF-5 port scheme, never 51820), addresses are in 10.<slot>.0.0/16, and every
// key is a documented test vector derived from the slot (VPP requires peer public keys to be unique
// VPP-wide, so slots must not share them). No peer exists: configuration is asserted, not
// handshakes. VRX_DF5_PAUSE=<seconds> holds the objects before cleanup so `vppctl show wireguard
// interface` / `show wireguard peer` evidence can be captured.

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/descriptors/vpn/vpntest"
	wgd "ngfw/agent/internal/descriptors/wireguard"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/vpptest"
)

func pauseForEvidence(t *testing.T) {
	t.Helper()
	if s := os.Getenv("VRX_DF5_PAUSE"); s != "" {
		n, _ := strconv.Atoi(s)
		t.Logf("VRX_DF5_PAUSE: holding objects for %ds", n)
		time.Sleep(time.Duration(n) * time.Second)
	}
}

func retrieveOne(t *testing.T, d scheduler.Descriptor, key scheduler.Key) (scheduler.KV, bool) {
	t.Helper()
	kvs, err := d.Retrieve(vpntest.Context(t))
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	for _, kv := range kvs {
		if kv.Key == key {
			return kv, true
		}
	}
	return scheduler.KV{}, false
}

func mustRetrieveEqual(t *testing.T, d scheduler.Descriptor, desired proto.Message) scheduler.KV {
	t.Helper()
	kv, ok := retrieveOne(t, d, d.KeyOf(desired))
	if !ok {
		t.Fatalf("%s: Retrieve does not show %s", d.Name(), d.KeyOf(desired))
	}
	if !proto.Equal(kv.Value, desired) {
		t.Fatalf("%s: Retrieve = %v\nwant %v", d.Name(), prototext.Format(kv.Value), prototext.Format(desired))
	}
	t.Logf("%s: Retrieve == desired: %s", d.Name(), prototext.Format(kv.Value))
	return kv
}

func TestWireguardOnHost(t *testing.T) {
	c := vpntest.Connect(t)
	ctx := vpntest.Context(t)
	owner := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	base := vpptest.TableBase(t)
	port := uint32(20000 + 100*slot + 10) //nolint:gosec // slots are 1–11

	res, itfRef := slotVectors(t, slot)
	cfg := wgd.Config{Client: c, Owner: owner, Secrets: res.resolver}
	reg := scheduler.NewRegistry()
	peer := wgd.Register(reg, c, owner, wgd.WithSecrets(res.resolver)) // a test slot is never the globals owner
	itf, _ := reg.Get(wgd.InterfaceName)
	async, _ := reg.Get(wgd.AsyncModeName)

	// ---- wireguard.peer events: subscribe first, so peers created below are registered too ----
	evCtx, evCancel := context.WithCancel(ctx)
	events, err := peer.Events(evCtx)
	if err != nil {
		t.Fatal(err)
	}

	itfV := &vpnpb.WireguardInterface{Instance: base + 1, PrivateKey: itfRef, Port: port, SrcIp: vpntest.SlotAddr(t, 8, 1)}
	p1 := &vpnpb.WireguardPeer{
		Interface: wgd.ItfName(base + 1), PublicKey: res.peerPub[0], Endpoint: vpntest.SlotAddr(t, 8, 2), Port: port + 1,
		AllowedIps:          []string{vpntest.SlotAddr(t, 9, 0) + "/24", vpntest.SlotAddr(t, 10, 0) + "/24"},
		PersistentKeepalive: 25, PresharedKey: res.pskRef,
	}
	// the allowed_ips canonical order is string order
	if p1.AllowedIps[0] > p1.AllowedIps[1] {
		p1.AllowedIps[0], p1.AllowedIps[1] = p1.AllowedIps[1], p1.AllowedIps[0]
	}
	p2 := &vpnpb.WireguardPeer{Interface: wgd.ItfName(base + 1), PublicKey: res.peerPub[1], AllowedIps: []string{vpntest.SlotAddr(t, 11, 0) + "/24"}}
	desired := []proto.Message{itfV, p1, p2}
	t.Cleanup(func() {
		_ = vpntest.NewAgent(c, owner, wgd.NewInterface(cfg), wgd.NewPeer(cfg)).S.Apply(context.Background(), nil, scheduler.All)
	})

	// ---- agent 1 (P05) applies; Retrieve equals desired ----
	agent1 := vpntest.NewAgent(c, owner, itf, peer)
	agent1.Apply(ctx, t, desired)
	mustRetrieveEqual(t, itf, itfV)
	mustRetrieveEqual(t, peer, p1)
	mustRetrieveEqual(t, peer, p2)
	agent1.MustEmptyPlan(ctx, t, "agent 1, second apply", desired)

	// ---- events: the subscription is live; the interface is admin-down and has no real peer,
	// so no status change is expected — drain briefly and close ----
	select {
	case ev, ok := <-events:
		if ok {
			t.Logf("peer event: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Log("wireguard peer events: subscribed (want_wireguard_peer_events ok), no status change without a real peer")
	}
	evCancel()
	for range events { //nolint:revive // drain until closed
	}

	// ---- restart simulation: fresh connection + fresh descriptors (nothing cached) ----
	c2 := vpntest.Connect(t)
	cfg2 := cfg
	cfg2.Client = c2
	agent2 := vpntest.NewAgent(c2, owner, wgd.NewInterface(cfg2), wgd.NewPeer(cfg2))
	agent2.MustEmptyPlan(ctx, t, "agent restart (fresh agent)", desired)

	// a peer lost behind the agent's back → exactly its re-creation
	kv, ok := retrieveOne(t, peer, peer.KeyOf(p2))
	if !ok {
		t.Fatal("p2 not retrieved")
	}
	if _, err := wireguard.NewServiceClient(c2).WireguardPeerRemove(ctx, &wireguard.WireguardPeerRemove{PeerIndex: kv.Meta.(wgd.PeerMeta).PeerIndex}); err != nil {
		t.Fatal(err)
	}
	p := agent2.Plan(ctx, t, desired)
	if len(p.Ops) != 1 || p.Ops[0].Op != scheduler.OpCreate || p.Ops[0].Key != peer.KeyOf(p2) {
		t.Fatalf("after loss: plan %s", vpntest.PlanString(p))
	}
	t.Logf("after loss (peer removed via the API): plan %s", vpntest.PlanString(p))
	agent2.Apply(ctx, t, desired)
	agent2.MustEmptyPlan(ctx, t, "after re-creation", desired)

	// ---- wireguard.async-mode: a VPP-global; a non-owner cannot set it ----
	if _, err := async.Create(ctx, &vpnpb.WireguardAsyncMode{}); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
		t.Fatalf("non-owner async mode: %v", err)
	} else {
		t.Logf("%s (non-owner): %v", async.Name(), err)
	}

	pauseForEvidence(t)

	// ---- the empty desired state deletes everything of ours ----
	agent2.Apply(ctx, t, nil)
	for _, d := range []scheduler.Descriptor{itf, peer} {
		kvs, err := d.Retrieve(ctx)
		if err != nil || len(kvs) != 0 {
			t.Fatalf("%s after the empty desired state: %v %v", d.Name(), kvs, err)
		}
		t.Logf("%s: nothing retrieved after the empty desired state", d.Name())
	}
}
