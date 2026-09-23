package ipsec_test

// One integration check per object type against the host VPP (VRX_INTEGRATION=1, shared lab
// lock, slot prefix). Every object carries the slot: SPD/SA ids in VRX_VPP_TABLE_BASE..+999,
// loopback/ipip fixtures tagged "<prefix>:…", addresses in 10.<slot>.0.0/16, UDP port from the
// DF-5 scheme 20000+100*slot (docs/agent/descriptors/ipsec.md). No peer exists: configuration is
// asserted, not traffic. VRX_DF5_PAUSE=<seconds> holds the objects before cleanup so `vppctl show`
// evidence can be captured from a shell.

import (
	"os"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/descriptors/vpn/vpntest"
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

func mustBeGone(t *testing.T, d scheduler.Descriptor, key scheduler.Key) {
	t.Helper()
	if _, ok := retrieveOne(t, d, key); ok {
		t.Fatalf("%s: %s still retrieved after Delete", d.Name(), key)
	}
	t.Logf("%s: %s gone after Delete", d.Name(), key)
}

func TestIpsecOnHost(t *testing.T) {
	c := vpntest.Connect(t)
	ctx := vpntest.Context(t)
	owner := vpptest.Prefix(t)
	base := vpptest.TableBase(t)
	slot := vpptest.Slot(t)
	udpPort := uint32(20000 + 100*slot) //nolint:gosec // slots are 1–12
	cfg := ipsecd.Config{Client: c, Owner: owner, Secrets: secrets, IDs: vpn.IDRange{Lo: base, Hi: base + 999}}

	// fixtures: a loopback for the SPD binding, an ipip tunnel for tunnel-protect (DF-6 owns the
	// ipip descriptor; here it is created directly via binapi and deleted in Cleanup)
	loop, _ := vpntest.Loopback(ctx, t, c, owner, 1)
	ipipName, _ := vpntest.Ipip(ctx, t, c, owner, base+1, vpntest.SlotAddr(t, 2, 1), vpntest.SlotAddr(t, 2, 2))

	spd, spdIf, spdEntry := ipsecd.NewSpd(cfg), ipsecd.NewSpdInterface(cfg), ipsecd.NewSpdEntry(cfg)
	sa, tp, itf := ipsecd.NewSa(cfg), ipsecd.NewTunnelProtect(cfg), ipsecd.NewItf(cfg)
	backend, async := ipsecd.NewBackend(cfg), ipsecd.NewAsyncMode(cfg)

	// ---- ipsec.spd ----
	spdV := &vpnpb.IpsecSpd{SpdId: base + 1}
	spdMeta, err := spd.Create(ctx, spdV)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spd.Delete(vpntest.Context(t), spdV, spdMeta) })
	mustRetrieveEqual(t, spd, spdV)

	// ---- ipsec.spd-interface ----
	bindV := &vpnpb.IpsecSpdInterface{Interface: loop, SpdId: base + 1}
	bindMeta, err := spdIf.Create(ctx, bindV)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spdIf.Delete(vpntest.Context(t), bindV, bindMeta) })
	kv := mustRetrieveEqual(t, spdIf, bindV)
	t.Logf("spd-interface meta (pool index learned at bind): %+v", kv.Meta)

	// ---- ipsec.spd-entry ----
	entryV := &vpnpb.IpsecSpdEntry{
		SpdId: base + 1, Priority: 10, Direction: "outbound", Action: "bypass",
		LocalStart: vpntest.SlotAddr(t, 1, 0), LocalStop: vpntest.SlotAddr(t, 1, 255),
		RemoteStart: vpntest.SlotAddr(t, 3, 0), RemoteStop: vpntest.SlotAddr(t, 3, 255),
		LocalPortStop: 65535, RemotePortStop: 65535,
	}
	if _, err := spdEntry.Create(ctx, entryV); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spdEntry.Delete(vpntest.Context(t), entryV, nil) })
	mustRetrieveEqual(t, spdEntry, entryV)

	// ---- ipsec.sa: transport (AEAD) and tunnel (CBC+HMAC, UDP encap, anti-replay) ----
	saT := &vpnpb.IpsecSa{SadId: base + 1, Spi: 1000 + base, Protocol: "esp", CryptoAlg: "aes-gcm-128",
		CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "none", Salt: 0x1234, Inbound: true}
	saU := &vpnpb.IpsecSa{SadId: base + 2, Spi: 1001 + base, Protocol: "esp", CryptoAlg: "aes-cbc-128",
		CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "sha1-96", IntegKey: vpn.Ref(integKey),
		UseEsn: true, UseAntiReplay: true, AntiReplayWindowSize: 128, UdpEncap: true, UdpSrcPort: udpPort, UdpDstPort: udpPort,
		Tunnel: &vpnpb.IpsecTunnel{Src: vpntest.SlotAddr(t, 0, 1), Dst: vpntest.SlotAddr(t, 0, 2), Dscp: 46, HopLimit: 64,
			EncapDecapFlags: []string{"encap-copy-df", "encap-copy-dscp"}}}
	saOut := &vpnpb.IpsecSa{SadId: base + 3, Spi: 1002 + base, Protocol: "esp", CryptoAlg: "aes-gcm-128", CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "none"}
	saIn := &vpnpb.IpsecSa{SadId: base + 4, Spi: 1003 + base, Protocol: "esp", CryptoAlg: "aes-gcm-128", CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "none", Inbound: true}
	for _, s := range []*vpnpb.IpsecSa{saT, saU, saOut, saIn} {
		meta, err := sa.Create(ctx, s)
		if err != nil {
			t.Fatalf("sa %d: %v", s.GetSadId(), err)
		}
		t.Cleanup(func() { _ = sa.Delete(vpntest.Context(t), s, meta) })
		mustRetrieveEqual(t, sa, s)
	}
	// the second Retrieve is what "apply the same desired state twice" diffs against: still equal
	for _, s := range []*vpnpb.IpsecSa{saT, saU} {
		mustRetrieveEqual(t, sa, s)
	}

	// ---- ipsec.tunnel-protect on the ipip fixture, then swap the inbound SA in place ----
	tpV := &vpnpb.IpsecTunnelProtect{Interface: ipipName, SaOut: base + 3, SaIn: []uint32{base + 4}}
	tpMeta, err := tp.Create(ctx, tpV)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tp.Delete(vpntest.Context(t), tpV, tpMeta) })
	mustRetrieveEqual(t, tp, tpV)
	tpV2 := &vpnpb.IpsecTunnelProtect{Interface: ipipName, SaOut: base + 3, SaIn: []uint32{base + 1, base + 4}}
	if _, err := tp.Update(ctx, tpV, tpV2, tpMeta); err != nil {
		t.Fatal(err)
	}
	mustRetrieveEqual(t, tp, tpV2)

	// ---- ipsec.itf ----
	itfV := &vpnpb.IpsecItf{Instance: base + 1, Mode: "p2p"}
	itfMeta, err := itf.Create(ctx, itfV)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = itf.Delete(vpntest.Context(t), itfV, itfMeta) })
	mustRetrieveEqual(t, itf, itfV)

	// ---- ipsec.backend: read-first; re-select the currently active backend (no change) ----
	backends, err := backend.Retrieve(ctx)
	if err != nil {
		t.Fatalf("backend Retrieve: %v", err)
	}
	if len(backends) == 0 {
		// VPP 26.06 registers no ESP/AH backends any more (the dpdk ipsec backend is gone and
		// `show ipsec backends` no longer exists): ipsec_backend_dump is empty, the descriptor
		// reports nothing and ipsec_select_backend has nothing to select.
		t.Logf("%s: ipsec_backend_dump returned no backend on this VPP — nothing to select (read-only check passed)", backend.Name())
	}
	for _, b := range backends {
		t.Logf("backend active: %s = %s", b.Key, prototext.Format(b.Value))
	}
	if len(backends) > 0 {
		if _, err := backend.Update(ctx, backends[0].Value, backends[0].Value, backends[0].Meta); err != nil {
			t.Fatalf("re-selecting the active backend: %v", err)
		}
		after, _ := backend.Retrieve(ctx)
		for i := range backends {
			if !proto.Equal(after[i].Value, backends[i].Value) {
				t.Fatalf("backend changed: %v → %v", backends[i].Value, after[i].Value)
			}
		}
	}

	// ---- ipsec.async-mode: skipped on this host ----
	t.Logf("%s: skip — no worker threads on this host (docs/lab/host-vrx-a.md), ipsec_set_async_mode not exercised", async.Name())

	// ---- the same desired state again → empty plan (SA keys compared by reference) ----
	vpntest.MustEmptyPlan(t, []scheduler.Descriptor{spd, spdIf, spdEntry, sa, tp, itf, async},
		[]proto.Message{spdV, bindV, entryV, saT, saU, saOut, saIn, tpV2, itfV, &vpnpb.IpsecAsyncMode{}})

	pauseForEvidence(t)

	// ---- delete in reverse and verify Retrieve shows nothing of ours ----
	if err := itf.Delete(ctx, itfV, itfMeta); err != nil {
		t.Fatal(err)
	}
	mustBeGone(t, itf, itf.KeyOf(itfV))
	if err := tp.Delete(ctx, tpV2, tpMeta); err != nil {
		t.Fatal(err)
	}
	mustBeGone(t, tp, tp.KeyOf(tpV2))
	for _, s := range []*vpnpb.IpsecSa{saIn, saOut, saU, saT} {
		if err := sa.Delete(ctx, s, nil); err != nil {
			t.Fatal(err)
		}
		mustBeGone(t, sa, sa.KeyOf(s))
	}
	if err := spdEntry.Delete(ctx, entryV, nil); err != nil {
		t.Fatal(err)
	}
	mustBeGone(t, spdEntry, spdEntry.KeyOf(entryV))
	if err := spdIf.Delete(ctx, bindV, bindMeta); err != nil {
		t.Fatal(err)
	}
	mustBeGone(t, spdIf, spdIf.KeyOf(bindV))
	if err := spd.Delete(ctx, spdV, spdMeta); err != nil {
		t.Fatal(err)
	}
	mustBeGone(t, spd, spd.KeyOf(spdV))
}
