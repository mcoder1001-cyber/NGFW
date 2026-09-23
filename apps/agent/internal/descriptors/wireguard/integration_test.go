package wireguard_test

// One integration check per object type against the host VPP (VRX_INTEGRATION=1, shared lab
// lock, slot prefix). The interface is wg<table base + 1> tagged "<prefix>:wg…", its listen port is
// 20000+100*slot+10 (DF-5 port scheme, never 51820), addresses are in 10.<slot>.0.0/16, and every
// key is a documented test vector derived from the slot (VPP requires peer public keys to be unique
// VPP-wide, so slots must not share them). No peer exists: configuration is asserted, not
// handshakes. VRX_DF5_PAUSE=<seconds> holds the objects before cleanup so `vppctl show wireguard
// interface` / `show wireguard peer` evidence can be captured.

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

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
	port := uint32(20000 + 100*slot + 10) //nolint:gosec // slots are 1–12

	res, itfRef := slotVectors(t, slot)
	cfg := wgd.Config{Client: c, Owner: owner, Secrets: res.resolver}
	itf, peer, async := wgd.NewInterface(cfg), wgd.NewPeer(cfg), wgd.NewAsyncMode(cfg)

	// ---- wireguard.peer events: subscribe first, so peers created below are registered too ----
	evCtx, evCancel := context.WithCancel(ctx)
	events, err := peer.Events(evCtx)
	if err != nil {
		t.Fatal(err)
	}

	// ---- wireguard.interface ----
	itfV := &vpnpb.WireguardInterface{Instance: base + 1, PrivateKey: itfRef, Port: port, SrcIp: vpntest.SlotAddr(t, 8, 1)}
	itfMeta, err := itf.Create(ctx, itfV)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = itf.Delete(vpntest.Context(t), itfV, itfMeta) })
	mustRetrieveEqual(t, itf, itfV)

	// ---- wireguard.peer: with preshared key + endpoint + keepalive, and a minimal one ----
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
	var metas []any
	for _, p := range []*vpnpb.WireguardPeer{p1, p2} {
		m, err := peer.Create(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		metas = append(metas, m)
		t.Cleanup(func() { _ = peer.Delete(vpntest.Context(t), p, m) })
		mustRetrieveEqual(t, peer, p)
	}
	// a fresh descriptor (≈ agent restart) retrieves the same values
	for _, p := range []*vpnpb.WireguardPeer{p1, p2} {
		mustRetrieveEqual(t, wgd.NewPeer(cfg), p)
	}
	mustRetrieveEqual(t, wgd.NewInterface(cfg), itfV)

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

	// ---- wireguard.async-mode: skipped on this host ----
	t.Logf("%s: skip — no worker threads on this host (docs/lab/host-vrx-a.md), wg_set_async_mode not exercised", async.Name())

	// ---- the same desired state again → empty plan (keys compared by reference) ----
	vpntest.MustEmptyPlan(t, []scheduler.Descriptor{itf, peer, async},
		[]proto.Message{itfV, p1, p2, &vpnpb.WireguardAsyncMode{}})

	pauseForEvidence(t)

	for i, p := range []*vpnpb.WireguardPeer{p1, p2} {
		if err := peer.Delete(ctx, p, metas[i]); err != nil {
			t.Fatal(err)
		}
		if _, ok := retrieveOne(t, peer, peer.KeyOf(p)); ok {
			t.Fatalf("%s still retrieved", peer.KeyOf(p))
		}
		t.Logf("%s: %s gone after Delete", peer.Name(), peer.KeyOf(p))
	}
	if err := itf.Delete(ctx, itfV, itfMeta); err != nil {
		t.Fatal(err)
	}
	if _, ok := retrieveOne(t, itf, itf.KeyOf(itfV)); ok {
		t.Fatalf("%s still retrieved", itf.KeyOf(itfV))
	}
	t.Logf("%s: %s gone after Delete", itf.Name(), itf.KeyOf(itfV))
}
