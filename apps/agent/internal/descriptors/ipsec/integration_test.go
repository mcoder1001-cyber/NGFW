package ipsec_test

// Host checks against the shared VPP (VRX_INTEGRATION=1, shared lab lock, slot prefix). Every
// object carries the slot: SPD/SA ids in VRX_VPP_TABLE_BASE..+499 (descriptors) and +500..+599
// (simulated charon SAs), loopback/ipip fixtures in the slot's instance range, addresses in
// 10.<slot>.0.0/16, UDP port 20000+100*slot (docs/agent/descriptors/ipsec.md). No peer exists:
// configuration is asserted, not traffic. VRX_DF5_PAUSE=<seconds> holds the objects before the
// final delete so `vppctl show` evidence can be captured from a shell.
//
// Flow: agent 1 (P05 reconciler + DF-1 alias + the ipsec descriptors, persisted record store)
// applies the desired state → Retrieve equals desired per object type → the same desired state
// plans nothing → restart simulation: agent 2 (fresh connection, fresh descriptors, same record
// store) plans nothing; an agent without the records adopts nothing; objects deleted behind the
// agent's back are re-created → charon orphan sweep (D-089) → apply of the empty desired state
// deletes everything of ours.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/binapi/ipsec_types"
	"ngfw/agent/internal/descriptors/dfkit"
	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/descriptors/vpn/vpntest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
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

func mustRetrieveNone(t *testing.T, d scheduler.Descriptor, why string) {
	t.Helper()
	kvs, err := d.Retrieve(vpntest.Context(t))
	if err != nil {
		t.Fatalf("%s Retrieve: %v", d.Name(), err)
	}
	if len(kvs) != 0 {
		t.Fatalf("%s (%s): Retrieve = %v", d.Name(), why, kvs)
	}
	t.Logf("%s: nothing retrieved (%s)", d.Name(), why)
}

// hostDescriptors builds one agent's ipsec descriptors (non-owner of the globals).
func hostDescriptors(cfg ipsecd.Config) []scheduler.Descriptor { return ipsecd.All(cfg) }

func byName(ds []scheduler.Descriptor, name string) scheduler.Descriptor {
	for _, d := range ds {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

type acker struct{ n int }

func (a *acker) AckRestart(context.Context) error { a.n++; return nil }

func TestIpsecOnHost(t *testing.T) {
	c := vpntest.Connect(t)
	ctx := vpntest.Context(t)
	owner := vpptest.Prefix(t)
	base := vpptest.TableBase(t)
	slot := vpptest.Slot(t)
	udpPort := uint32(20000 + 100*slot) //nolint:gosec // slots are 1–11
	store, err := dfkit.NewFileBootStore(filepath.Join(t.TempDir(), "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	ids := vpn.IDRange{Lo: base, Hi: base + 499}
	cfg := ipsecd.Config{Client: c, Owner: owner, Secrets: secrets, IDs: ids, Boot: store}

	// fixtures: a tagged loopback and an untagged one (the stand-in for a physical NIC, D-069) for
	// the SPD bindings, an ipip tunnel for tunnel-protect (DF-6 owns ipip; created via binapi)
	loop, _ := vpntest.Loopback(ctx, t, c, owner, 1)
	nic, _ := vpntest.UntaggedLoopback(ctx, t, c, 2)
	ipipName, _ := vpntest.Ipip(ctx, t, c, owner, base+1, vpntest.SlotAddr(t, 2, 1), vpntest.SlotAddr(t, 2, 2))

	ds := hostDescriptors(cfg)
	spd, spdIf, spdEntry := byName(ds, ipsecd.SpdName), byName(ds, ipsecd.SpdInterfaceName), byName(ds, ipsecd.SpdEntryName)
	sa, tp, itf := byName(ds, ipsecd.SaName), byName(ds, ipsecd.TunnelProtectName), byName(ds, ipsecd.ItfName)
	agent1 := vpntest.NewAgent(c, owner, ds...)

	spdV := &vpnpb.IpsecSpd{SpdId: base + 1}
	bindV := &vpnpb.IpsecSpdInterface{Interface: loop, SpdId: base + 1}
	bindNic := &vpnpb.IpsecSpdInterface{Interface: nic, SpdId: base + 1}
	entryV := &vpnpb.IpsecSpdEntry{
		SpdId: base + 1, Priority: 10, Direction: "outbound", Action: "bypass",
		LocalStart: vpntest.SlotAddr(t, 1, 0), LocalStop: vpntest.SlotAddr(t, 1, 255),
		RemoteStart: vpntest.SlotAddr(t, 3, 0), RemoteStop: vpntest.SlotAddr(t, 3, 255),
		LocalPortStop: 65535, RemotePortStop: 65535,
	}
	saT := &vpnpb.IpsecSa{SadId: base + 1, Spi: 1000 + base, Protocol: "esp", CryptoAlg: "aes-gcm-128",
		CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "none", Salt: 0x1234, Inbound: true}
	saU := &vpnpb.IpsecSa{SadId: base + 2, Spi: 1001 + base, Protocol: "esp", CryptoAlg: "aes-cbc-128",
		CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "sha1-96", IntegKey: vpn.Ref(integKey),
		UseEsn: true, UseAntiReplay: true, AntiReplayWindowSize: 128, UdpEncap: true, UdpSrcPort: udpPort, UdpDstPort: udpPort,
		Tunnel: &vpnpb.IpsecTunnel{Src: vpntest.SlotAddr(t, 0, 1), Dst: vpntest.SlotAddr(t, 0, 2), Dscp: 46, HopLimit: 64,
			EncapDecapFlags: []string{"encap-copy-df", "encap-copy-dscp"}}}
	saOut := &vpnpb.IpsecSa{SadId: base + 3, Spi: 1002 + base, Protocol: "esp", CryptoAlg: "aes-gcm-128", CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "none"}
	saIn := &vpnpb.IpsecSa{SadId: base + 4, Spi: 1003 + base, Protocol: "esp", CryptoAlg: "aes-gcm-128", CryptoKey: vpn.Ref(cryptoKey), IntegAlg: "none", Inbound: true}
	tpV := &vpnpb.IpsecTunnelProtect{Interface: ipipName, SaOut: base + 3, SaIn: []uint32{base + 4}}
	itfV := &vpnpb.IpsecItf{Instance: base + 1, Mode: "p2p"}
	desired := []proto.Message{spdV, bindV, bindNic, entryV, saT, saU, saOut, saIn, tpV, itfV}

	// safety net: whatever the test leaves, the empty desired state removes (records-based: ours only)
	t.Cleanup(func() {
		a := vpntest.NewAgent(c, owner, hostDescriptors(cfg)...)
		_ = a.S.Apply(context.Background(), nil, scheduler.All)
	})

	// ---- agent 1 applies; Retrieve equals desired for every object type ----
	agent1.Apply(ctx, t, desired)
	for _, v := range desired {
		for _, d := range ds {
			if d.Name() == vpntestDescriptorOf(v) {
				mustRetrieveEqual(t, d, v)
			}
		}
	}

	// ---- in-place update: swap the inbound SAs of the protection ----
	tpV2 := &vpnpb.IpsecTunnelProtect{Interface: ipipName, SaOut: base + 3, SaIn: []uint32{base + 1, base + 4}}
	desired2 := slices.Clone(desired)
	desired2[slices.Index(desired2, proto.Message(tpV))] = tpV2
	p := agent1.Plan(ctx, t, desired2)
	if s := p.Summary(); s.Updated != 1 || s.Created+s.Deleted != 0 {
		t.Fatalf("SA swap plan: %s", vpntest.PlanString(p))
	}
	t.Logf("SA swap plan: %s", vpntest.PlanString(p))
	agent1.Apply(ctx, t, desired2)
	mustRetrieveEqual(t, tp, tpV2)

	// ---- the same desired state again: empty plan (SA keys compared by reference) ----
	agent1.MustEmptyPlan(ctx, t, "agent 1, second apply", desired2)

	// ---- restart simulation: fresh connection + fresh descriptors, same persisted records ----
	c2 := vpntest.Connect(t)
	cfg2 := cfg
	cfg2.Client = c2
	agent2 := vpntest.NewAgent(c2, owner, hostDescriptors(cfg2)...)
	agent2.MustEmptyPlan(ctx, t, "agent restart (fresh agent, persisted records)", desired2)

	// an agent without our records never adopts: nothing of the id-only objects is its
	stranger := cfg2
	stranger.Boot = dfkit.NewMemoryBootStore()
	for _, d := range []scheduler.Descriptor{ipsecd.NewSpd(stranger), ipsecd.NewSa(stranger), ipsecd.NewSpdInterface(stranger), ipsecd.NewSpdEntry(stranger)} {
		mustRetrieveNone(t, d, "agent without our ownership records")
	}
	if _, err := ipsecd.NewSa(stranger).Create(ctx, saT); err == nil {
		t.Fatal("an existing SA must not be adopted")
	}

	// objects lost behind the agent's back (deleted via the binary API) → exactly their re-creation
	svc := ipsec.NewServiceClient(c2)
	if _, err := svc.IpsecSadEntryDel(ctx, &ipsec.IpsecSadEntryDel{ID: saU.GetSadId()}); err != nil {
		t.Fatal(err)
	}
	nicKV, _ := retrieveOne(t, spdIf, spdIf.KeyOf(bindNic))
	if _, err := svc.IpsecInterfaceAddDelSpd(ctx, &ipsec.IpsecInterfaceAddDelSpd{IsAdd: false,
		SwIfIndex: interfaceIndex(nicKV.Meta.(ipsecd.SpdInterfaceMeta).SwIfIndex), SpdID: base + 1}); err != nil {
		t.Fatal(err)
	}
	p = agent2.Plan(ctx, t, desired2)
	want := []string{"create " + string(sa.KeyOf(saU)), "create " + string(spdIf.KeyOf(bindNic))}
	var got []string
	for _, op := range p.Ops {
		got = append(got, op.Op+" "+string(op.Key))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("after loss: plan %v, want %v", got, want)
	}
	t.Logf("after loss (SA + NIC binding deleted via the API): plan %s", vpntest.PlanString(p))
	agent2.Apply(ctx, t, desired2)
	agent2.MustEmptyPlan(ctx, t, "after re-creation", desired2)

	// ---- non-owner globals (D-071): the backend requirement, async mode needs the owner ----
	backend, async := byName(ds, ipsecd.BackendName), byName(ds, ipsecd.AsyncModeName)
	if _, ok := backend.(*vpn.Require); !ok {
		t.Fatalf("test slots are never the globals owner: %T", backend)
	}
	actual, err := ipsecd.NewBackend(cfg2).Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual) == 0 {
		t.Logf("%s: ipsec_backend_dump returned no backend on VPP 26.06 — nothing to require", backend.Name())
	}
	for _, b := range actual {
		if _, err := backend.Create(ctx, b.Value); err != nil {
			t.Fatalf("requiring the active backend: %v", err)
		}
		t.Logf("%s: requirement satisfied without setting: %s", backend.Name(), prototext.Format(b.Value))
	}
	if _, err := async.Create(ctx, &vpnpb.IpsecAsyncMode{}); err == nil {
		t.Fatal("a non-owner must not set the async mode")
	} else {
		t.Logf("%s (non-owner): %v", async.Name(), err)
	}

	// ---- charon orphan sweep (D-089): two "charon" SAs, one live ----
	charon := vpn.IDRange{Lo: base + 500, Hi: base + 599}
	orphan, live := base+501, base+502
	for _, id := range []uint32{orphan, live} {
		if _, err := svc.IpsecSadEntryAddV2(ctx, &ipsec.IpsecSadEntryAddV2{Entry: ipsec_types.IpsecSadEntryV4{
			SadID: id, Spi: 2000 + id, Protocol: ipsec_types.IPSEC_API_PROTO_ESP,
			CryptoAlgorithm: ipsec_types.IPSEC_API_CRYPTO_ALG_NONE, IntegrityAlgorithm: ipsec_types.IPSEC_API_INTEG_ALG_NONE,
		}}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = svc.IpsecSadEntryDel(context.Background(), &ipsec.IpsecSadEntryDel{ID: id}) })
	}
	if _, err := svc.IpsecSpdAddDel(ctx, &ipsec.IpsecSpdAddDel{IsAdd: true, SpdID: base + 501}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = svc.IpsecSpdAddDel(context.Background(), &ipsec.IpsecSpdAddDel{IsAdd: false, SpdID: base + 501})
	})
	lo, _ := vpn.ParseAddress(vpntest.SlotAddr(t, 5, 0))
	hi, _ := vpn.ParseAddress(vpntest.SlotAddr(t, 5, 255))
	if _, err := svc.IpsecSpdEntryAddDelV2(ctx, &ipsec.IpsecSpdEntryAddDelV2{IsAdd: true, Entry: ipsec_types.IpsecSpdEntryV2{
		SpdID: base + 501, Priority: 5, SaID: orphan, Policy: ipsec_types.IPSEC_API_SPD_ACTION_PROTECT, Protocol: 255,
		LocalAddressStart: lo, LocalAddressStop: hi, RemoteAddressStart: lo, RemoteAddressStop: hi,
		LocalPortStop: 65535, RemotePortStop: 65535,
	}}); err != nil {
		t.Fatal(err)
	}
	// the descriptors never touch the charon SPD: it has no record of ours
	if _, err := ipsecd.NewSpdEntry(cfg2).Create(ctx, &vpnpb.IpsecSpdEntry{SpdId: base + 501, Priority: 6, Direction: "inbound",
		Action: "bypass", LocalStart: vpntest.SlotAddr(t, 5, 0), LocalStop: vpntest.SlotAddr(t, 5, 255),
		RemoteStart: vpntest.SlotAddr(t, 5, 0), RemoteStop: vpntest.SlotAddr(t, 5, 255)}); err == nil {
		t.Fatal("a policy in an SPD that is not ours must be refused")
	}
	ack := &acker{}
	res, err := ipsecd.SweepAndAck(ctx, cfg2, ipsecd.CharonSweep{IDs: charon, Live: func(spi uint32) bool { return spi == 2000+live }}, ack)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("charon sweep: deleted SAs %v, policies %d, in use %v; AckRestart calls %d", res.DeletedSAs, res.DeletedPolicies, res.InUse, ack.n)
	if !slices.Equal(res.DeletedSAs, []uint32{orphan}) || ack.n != 1 {
		t.Fatalf("sweep result %+v ack %d", res, ack.n)
	}
	left, err := saIDs(ctx, c2)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(left, orphan) || !slices.Contains(left, live) || !slices.Contains(left, saT.GetSadId()) {
		t.Fatalf("after sweep: SAs %v", left)
	}
	agent2.MustEmptyPlan(ctx, t, "after the charon sweep (our SAs untouched)", desired2)

	pauseForEvidence(t)

	// ---- the empty desired state deletes everything of ours, nothing else ----
	agent2.Apply(ctx, t, nil)
	for _, d := range []scheduler.Descriptor{spd, spdIf, spdEntry, sa, tp, itf} {
		mustRetrieveNone(t, d, "after applying the empty desired state")
	}
	left, _ = saIDs(ctx, c2)
	if !slices.Contains(left, live) {
		t.Fatal("the charon SA must survive our empty desired state")
	}
}

// saIDs lists every SA id in VPP.
func saIDs(ctx context.Context, c vpp.Client) ([]uint32, error) {
	stream, err := ipsec.NewServiceClient(c).IpsecSaV5Dump(ctx, &ipsec.IpsecSaV5Dump{SaID: ^uint32(0)})
	if err != nil {
		return nil, err
	}
	var out []uint32
	for {
		det, err := stream.Recv()
		if err != nil {
			return out, nil //nolint:nilerr // EOF ends the dump
		}
		clear(det.Entry.CryptoKey.Data)
		clear(det.Entry.IntegrityKey.Data)
		out = append(out, det.Entry.SadID)
	}
}

// vpntestDescriptorOf names the descriptor of a desired value.
func vpntestDescriptorOf(v proto.Message) string {
	switch v.(type) {
	case *vpnpb.IpsecSpd:
		return ipsecd.SpdName
	case *vpnpb.IpsecSpdInterface:
		return ipsecd.SpdInterfaceName
	case *vpnpb.IpsecSpdEntry:
		return ipsecd.SpdEntryName
	case *vpnpb.IpsecSa:
		return ipsecd.SaName
	case *vpnpb.IpsecTunnelProtect:
		return ipsecd.TunnelProtectName
	case *vpnpb.IpsecItf:
		return ipsecd.ItfName
	}
	return ""
}
