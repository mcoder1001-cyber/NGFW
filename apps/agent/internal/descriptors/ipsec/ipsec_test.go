package ipsec_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ipsec"
	"ngfw/agent/internal/descriptors/dfkit"
	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/descriptors/vpn/vpntest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Test vectors: the documented placeholder, never real material (00-CONTEXT "Never do these").
var (
	cryptoKey = []byte("VRX_TEST_PSK_DF5")     // 16 bytes → aes-gcm-128 / aes-cbc-128
	integKey  = []byte("VRX_TEST_PSK_DF5_int") // 20 bytes → sha1-96
	secrets   = vpn.NewMapResolver(keys, cryptoKey, integKey)
	owner     = "w4"
	ctx       = context.Background()
	boot      = vpntest.NewFakeBoot()
)

func newCfg(v *fakeVPP) ipsecd.Config {
	return ipsecd.Config{Keys: keys, Client: v, Owner: owner, Secrets: secrets, IDs: vpn.IDRange{Lo: 4000, Hi: 4999}, Boot: dfkit.NewMemoryBootStore()}
}

func mustEqual(t *testing.T, kvs []scheduler.KV, want ...proto.Message) {
	t.Helper()
	if len(kvs) != len(want) {
		t.Fatalf("Retrieve returned %d objects, want %d: %+v", len(kvs), len(want), kvs)
	}
	for i := range want {
		if !proto.Equal(kvs[i].Value, want[i]) {
			t.Fatalf("Retrieve[%d] = %v, want %v", i, kvs[i].Value, want[i])
		}
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	ipsecd.Register(reg, newFakeVPP(), owner, ipsecd.WithSecrets(secrets), ipsecd.WithIDRange(4000, 4999))
	want := []string{"ipsec.spd", "ipsec.spd-interface", "ipsec.spd-entry", "ipsec.sa", "ipsec.tunnel-protect", "ipsec.itf", "ipsec.backend", "ipsec.async-mode"}
	if got := reg.Names(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("registered %v, want %v", got, want)
	}
	for _, d := range reg.Descriptors() {
		if !scheduler.ValidName(d.Name()) {
			t.Fatalf("invalid name %q", d.Name())
		}
	}
	// D-071: a non-owner registers only requirements for the VPP-globals, the owner the setters
	for _, owner := range []bool{false, true} {
		reg := scheduler.NewRegistry()
		ipsecd.Register(reg, newFakeVPP(), "w4", ipsecd.WithGlobalsOwner(owner))
		for _, name := range []string{"ipsec.backend", "ipsec.async-mode"} {
			d, _ := reg.Get(name)
			if _, isReq := d.(*vpn.Require); isReq == owner {
				t.Fatalf("%s: globals owner=%v registered %T", name, owner, d)
			}
			if a, ok := d.(scheduler.AbsenceDeleter); !ok || a.DeleteOnAbsence() {
				t.Fatalf("%s: a global must never be deleted on absence", name)
			}
		}
	}
}

func TestSpd(t *testing.T) {
	v := newFakeVPP()
	v.spds[3001] = 1 // another worker's SPD
	d := ipsecd.NewSpd(newCfg(v))
	desired := &vpnpb.IpsecSpd{SpdId: 4001}
	if d.KeyOf(desired) != "ipsec.spd/4001" {
		t.Fatalf("key %s", d.KeyOf(desired))
	}
	if d.Dependencies(desired) != nil {
		t.Fatal("spd has no dependencies")
	}
	if _, err := d.Create(ctx, &vpnpb.IpsecSpd{SpdId: 3002}); err == nil {
		t.Fatal("id outside the owned range must be refused")
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if r := v.CallsNamed("ipsec_spd_add_del"); len(r) != 1 || !r[0].(*ipsec.IpsecSpdAddDel).IsAdd || r[0].(*ipsec.IpsecSpdAddDel).SpdID != 4001 {
		t.Fatalf("request %+v", r)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, desired)
	if kvs[0].Key != "ipsec.spd/4001" || kvs[0].Meta != meta {
		t.Fatalf("kv %+v", kvs[0])
	}
	if _, err := d.Update(ctx, desired, &vpnpb.IpsecSpd{SpdId: 4002}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	// a retried Create of our own SPD (reply lost) adopts it through our record, no second add
	if _, err := d.Create(ctx, desired); err != nil || len(v.CallsNamed("ipsec_spd_add_del")) != 1 {
		t.Fatalf("retry of our own SPD: %v", err)
	}

	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete: %+v", kvs)
	}
	if _, ok := v.spds[3001]; !ok {
		t.Fatal("foreign SPD touched")
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatalf("deleting a vanished SPD is done, not an error (D-074): %v", err)
	}

	// D-071: an SPD in our range that we did not create is never adopted or deleted
	v.spds[4003] = 99
	stranger := &vpnpb.IpsecSpd{SpdId: 4003}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("unrecorded SPD reported: %+v", kvs)
	}
	if _, err := d.Create(ctx, stranger); err == nil {
		t.Fatal("an existing SPD must not be adopted")
	}
	if err := d.Delete(ctx, stranger, SpdMetaOf(4003)); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("Delete of a stranger's SPD: %v", err)
	}
	if _, ok := v.spds[4003]; !ok {
		t.Fatal("stranger's SPD deleted")
	}

	// restart simulation: a fresh descriptor with the persisted store sees its SPD (agent
	// restart); after a VPP restart the record has expired and the id is not ours any more
	cfg := newCfg(v)
	if _, err := ipsecd.NewSpd(cfg).Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = ipsecd.NewSpd(cfg).Retrieve(ctx); len(kvs) != 1 {
		t.Fatalf("agent restart: %+v", kvs)
	}
	boot.RestartVPP()
	if kvs, _ = ipsecd.NewSpd(cfg).Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("VPP restart: records must expire: %+v", kvs)
	}
	v.SetConnected(false)
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
}

// SpdMetaOf is the meta of SPD id.
func SpdMetaOf(id uint32) ipsecd.SpdMeta { return ipsecd.SpdMeta{SpdID: id} }

func TestSpdInterface(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	loop := v.addIface("loop400", "w4:w4-lan")
	other := v.addIface("loop300", "w3:loop300")
	wan := v.addIface("wan0", "") // untagged physical NIC
	if _, err := ipsecd.NewSpd(cfg).Create(ctx, &vpnpb.IpsecSpd{SpdId: 4001}); err != nil {
		t.Fatal(err)
	}
	v.spds[3001] = 12
	v.bindings[other] = 12
	d := ipsecd.NewSpdInterface(cfg)
	desired := &vpnpb.IpsecSpdInterface{Interface: "w4-lan", SpdId: 4001}
	if d.KeyOf(desired) != "ipsec.spd-interface/w4-lan" {
		t.Fatalf("key %s", d.KeyOf(desired))
	}
	deps := d.Dependencies(desired)
	if len(deps) != 2 || deps[0].Key != "ipsec.spd/4001" || deps[1].Key != "interface/w4-lan" || deps[0].Optional || deps[1].Optional {
		t.Fatalf("deps %+v", deps)
	}
	for name, bad := range map[string]error{"nope": vpn.ErrNoInterface, "loop400": vpn.ErrNoInterface, "loop300": vpn.ErrForeignInterface, "local0": vpn.ErrNoInterface} {
		if _, err := d.Create(ctx, &vpnpb.IpsecSpdInterface{Interface: name, SpdId: 4001}); !errors.Is(err, bad) {
			t.Fatalf("%s: %v, want %v (D-069 logical names)", name, err, bad)
		}
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	m := meta.(ipsecd.SpdInterfaceMeta)
	if m.SwIfIndex != loop || m.SpdID != 4001 || m.SpdIndex != v.spds[4001] {
		t.Fatalf("meta %+v", m)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, desired) // w3's binding filtered, pool index decoded to 4001 via the record
	if kvs[0].Meta != meta {
		t.Fatalf("meta %+v != %+v", kvs[0].Meta, meta)
	}
	if _, err := d.Update(ctx, desired, &vpnpb.IpsecSpdInterface{Interface: "w4-lan", SpdId: 4002}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}

	// an untagged (physical) interface: bound and reported through our record only
	onWan := &vpnpb.IpsecSpdInterface{Interface: "wan0", SpdId: 4001}
	metaWan, err := d.Create(ctx, onWan)
	if err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, desired, onWan)

	// agent restart with the persisted store: same objects, same spd ids (no Update planned)
	kvs, _ = ipsecd.NewSpdInterface(cfg).Retrieve(ctx)
	mustEqual(t, kvs, desired, onWan)

	// an agent without our records never adopts or deletes a binding (D-071)
	stranger := newCfg(v)
	if kvs, _ = ipsecd.NewSpdInterface(stranger).Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("unrecorded bindings reported: %+v", kvs)
	}
	if err := ipsecd.NewSpdInterface(stranger).Delete(ctx, onWan, metaWan); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("Delete without record: %v", err)
	}
	if _, err := ipsecd.NewSpdInterface(stranger).Create(ctx, onWan); err == nil {
		t.Fatal("a second SPD on an interface must fail in VPP")
	}

	if err := d.Delete(ctx, onWan, metaWan); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if _, bound := v.bindings[loop]; bound {
		t.Fatal("binding not removed")
	}
	if _, bound := v.bindings[wan]; bound {
		t.Fatal("wan binding not removed")
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatalf("second delete is a no-op (D-074): %v", err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete: %+v", kvs)
	}
	if _, bound := v.bindings[other]; !bound {
		t.Fatal("foreign binding touched")
	}

	// D-071/D-080: after a VPP restart the index in meta may belong to another interface
	if _, err := d.Create(ctx, desired); err != nil {
		t.Fatal(err)
	}
	boot.RestartVPP()
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("records must expire with the VPP instance: %+v", kvs)
	}
	stale := ipsecd.SpdInterfaceMeta{SwIfIndex: other, SpdID: 4001}
	if err := d.Delete(ctx, desired, stale); err != nil {
		t.Fatalf("stale index: %v", err)
	}
	if _, bound := v.bindings[loop]; !bound {
		t.Fatal("a delete with a stale index must not unbind anything")
	}
}

func spdEntry(prio int32, dir, action string, sa uint32) *vpnpb.IpsecSpdEntry {
	return &vpnpb.IpsecSpdEntry{
		SpdId: 4001, Priority: prio, Direction: dir, Action: action, SaId: sa,
		LocalStart: "10.4.1.0", LocalStop: "10.4.1.255", RemoteStart: "10.4.2.0", RemoteStop: "10.4.2.255",
		LocalPortStop: 65535, RemotePortStop: 65535,
	}
}

func TestSpdEntry(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	if _, err := ipsecd.NewSpd(cfg).Create(ctx, &vpnpb.IpsecSpd{SpdId: 4001}); err != nil {
		t.Fatal(err)
	}
	if _, err := ipsecd.NewSa(cfg).Create(ctx, transportSA()); err != nil { // protect needs its SA
		t.Fatal(err)
	}
	v.spds[3001] = 2
	v.policies[3001] = []ipsecSpdEntryV2Alias{{SpdID: 3001, Priority: 1, Protocol: 255}}
	v.spds[4002] = 3 // in range, not ours
	v.policies[4002] = []ipsecSpdEntryV2Alias{{SpdID: 4002, Priority: 1, Protocol: 255}}
	d := ipsecd.NewSpdEntry(cfg)
	bypass := spdEntry(10, "outbound", "bypass", 0)
	protect := spdEntry(20, "inbound", "protect", 4001)
	protect.Protocol = 17
	if k := d.KeyOf(bypass); k != "ipsec.spd-entry/4001/outbound/10/bypass/0/0/10.4.1.0-10.4.1.255/0-65535/10.4.2.0-10.4.2.255/0-65535" {
		t.Fatalf("key %s", k)
	}
	if deps := d.Dependencies(bypass); len(deps) != 1 || deps[0].Key != "ipsec.spd/4001" {
		t.Fatalf("bypass deps %+v", deps)
	}
	if deps := d.Dependencies(protect); len(deps) != 2 || deps[1].Key != "ipsec.sa/4001" {
		t.Fatalf("protect deps %+v", deps)
	}
	for name, bad := range map[string]*vpnpb.IpsecSpdEntry{
		"direction": spdEntry(1, "sideways", "bypass", 0),
		"action":    spdEntry(1, "inbound", "allow", 0),
		"protect":   spdEntry(1, "inbound", "protect", 0),
		"address":   func() *vpnpb.IpsecSpdEntry { e := spdEntry(1, "inbound", "bypass", 0); e.LocalStart = "10.4"; return e }(),
		"port": func() *vpnpb.IpsecSpdEntry {
			e := spdEntry(1, "inbound", "bypass", 0)
			e.LocalPortStop = 70000
			return e
		}(),
		"protocol": func() *vpnpb.IpsecSpdEntry { e := spdEntry(1, "inbound", "bypass", 0); e.Protocol = 256; return e }(),
		"literal 255": func() *vpnpb.IpsecSpdEntry {
			e := spdEntry(1, "inbound", "bypass", 0)
			e.Protocol = 255 // "any" is 0 in desired state
			return e
		}(),
	} {
		if _, err := d.Create(ctx, bad); err == nil {
			t.Fatalf("%s: invalid entry accepted", name)
		}
	}
	for _, e := range []*vpnpb.IpsecSpdEntry{bypass, protect} {
		if _, err := d.Create(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	reqs := v.CallsNamed("ipsec_spd_entry_add_del_v2")
	if len(reqs) != 2 || !reqs[0].(*ipsec.IpsecSpdEntryAddDelV2).Entry.IsOutbound || reqs[1].(*ipsec.IpsecSpdEntryAddDelV2).Entry.Protocol != 17 {
		t.Fatalf("requests %+v", reqs)
	}
	// v2 stores protocol as sent: desired 0 ("any") must go out as 255 (IPSEC_POLICY_PROTOCOL_ANY),
	// or VPP installs a HOPOPT-only policy
	if p := reqs[0].(*ipsec.IpsecSpdEntryAddDelV2).Entry.Protocol; p != 255 {
		t.Fatalf("protocol any sent as %d, want 255", p)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, protect, bypass) // sorted by key: inbound < outbound; protocol any → 0
	again, _ := d.Retrieve(ctx)
	mustEqual(t, again, protect, bypass)
	if _, err := d.Update(ctx, bypass, protect, nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, bypass, nil); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, protect)
	if err := d.Delete(ctx, bypass, nil); err != nil {
		t.Fatalf("deleting a vanished policy is done (D-074): %v", err)
	}
	if n := len(v.CallsNamed("ipsec_spd_entry_add_del_v2")); n != 3 {
		t.Fatalf("the second delete must not reach VPP (%d calls)", n)
	}
	strangers := spdEntry(1, "inbound", "bypass", 0)
	strangers.SpdId = 4002
	if _, err := d.Create(ctx, spdEntry(2, "inbound", "bypass", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, func() *vpnpb.IpsecSpdEntry { e := spdEntry(3, "inbound", "bypass", 0); e.SpdId = 4002; return e }()); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("policy into an SPD that is not ours: %v", err)
	}
	if err := d.Delete(ctx, strangers, nil); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("policy in an SPD that is not ours: %v", err)
	}
	if len(v.policies[3001]) != 1 || len(v.policies[4002]) != 1 {
		t.Fatal("foreign policy touched")
	}
}

func transportSA() *vpnpb.IpsecSa {
	return &vpnpb.IpsecSa{
		SadId: 4001, Spi: 1001, Protocol: "esp", CryptoAlg: "aes-gcm-128", CryptoKey: keys.Ref(cryptoKey),
		IntegAlg: "none", Salt: 0x1234, Inbound: true,
	}
}

func tunnelSA() *vpnpb.IpsecSa {
	return &vpnpb.IpsecSa{
		SadId: 4002, Spi: 1002, Protocol: "esp", CryptoAlg: "aes-cbc-128", CryptoKey: keys.Ref(cryptoKey),
		IntegAlg: "sha1-96", IntegKey: keys.Ref(integKey), UseEsn: true, UseAntiReplay: true, AntiReplayWindowSize: 128,
		UdpEncap: true, UdpSrcPort: 20400, UdpDstPort: 20400,
		Tunnel: &vpnpb.IpsecTunnel{Src: "10.4.0.1", Dst: "10.4.0.2", TableId: 4001, Dscp: 46, HopLimit: 64, EncapDecapFlags: []string{"encap-copy-df", "encap-copy-dscp"}},
	}
}

func TestSa(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewSa(newCfg(v))
	ts, tn := transportSA(), tunnelSA()
	if d.KeyOf(ts) != "ipsec.sa/4001" || d.Dependencies(ts) != nil {
		t.Fatalf("transport key/deps: %s %+v", d.KeyOf(ts), d.Dependencies(ts))
	}
	if deps := d.Dependencies(tn); len(deps) != 1 || deps[0].Key != "vrf/4001" || !deps[0].Optional {
		t.Fatalf("tunnel deps %+v", deps)
	}
	bad := map[string]func(*vpnpb.IpsecSa){
		"protocol":     func(s *vpnpb.IpsecSa) { s.Protocol = "gre" },
		"crypto alg":   func(s *vpnpb.IpsecSa) { s.CryptoAlg = "rot13" },
		"missing key":  func(s *vpnpb.IpsecSa) { s.CryptoKey = "" },
		"stray key":    func(s *vpnpb.IpsecSa) { s.IntegKey = keys.Ref(integKey) },
		"udp ports":    func(s *vpnpb.IpsecSa) { s.UdpEncap = true },
		"ports no udp": func(s *vpnpb.IpsecSa) { s.UdpDstPort = 20400 },
		"window":       func(s *vpnpb.IpsecSa) { s.UseAntiReplay = true; s.AntiReplayWindowSize = 100 },
		"window flag":  func(s *vpnpb.IpsecSa) { s.AntiReplayWindowSize = 64 },
		"unknown ref":  func(s *vpnpb.IpsecSa) { s.CryptoKey = "sha256:" + strings.Repeat("0", 64) },
		"tunnel af":    func(s *vpnpb.IpsecSa) { s.Tunnel = &vpnpb.IpsecTunnel{Src: "10.4.0.1", Dst: "2001:db8::1"} },
		"tunnel flag": func(s *vpnpb.IpsecSa) {
			s.Tunnel = &vpnpb.IpsecTunnel{Src: "10.4.0.1", Dst: "10.4.0.2", EncapDecapFlags: []string{"bogus"}}
		},
		"range": func(s *vpnpb.IpsecSa) { s.SadId = 1 },
	}
	for name, mutate := range bad {
		s := transportSA()
		mutate(s)
		_, err := d.Create(ctx, s)
		if err == nil {
			t.Fatalf("%s: invalid SA accepted", name)
		}
		if strings.Contains(err.Error(), string(cryptoKey)) || strings.Contains(err.Error(), string(integKey)) {
			t.Fatalf("%s: error leaks material: %v", name, err)
		}
	}
	if len(v.CallsNamed("ipsec_sad_entry_add_v2")) != 0 {
		t.Fatal("validation must happen before VPP is called")
	}
	metaTS, err := d.Create(ctx, ts)
	if err != nil {
		t.Fatal(err)
	}
	metaTN, err := d.Create(ctx, tn)
	if err != nil {
		t.Fatal(err)
	}
	reqs := v.CallsNamed("ipsec_sad_entry_add_v2")
	if len(reqs) != 2 {
		t.Fatalf("%d add requests", len(reqs))
	}
	got := reqs[1].(*ipsec.IpsecSadEntryAddV2).Entry
	if got.Flags != 1|2|4|16 || got.UDPDstPort != 20400 || got.Tunnel.TableID != 4001 || got.Tunnel.Dscp != 46 || got.Tunnel.HopLimit != 64 || got.Tunnel.EncapDecapFlags != 1|4 || got.AntiReplayWindowSize != 128 {
		t.Fatalf("tunnel SA request: flags=%v udp=%d tunnel=%+v window=%d", got.Flags, got.UDPDstPort, got.Tunnel, got.AntiReplayWindowSize)
	}
	// the request carried the material (VPP needs it) but the descriptor zeroed its buffer after the call
	if !bytes.Equal(got.CryptoKey.Data, make([]byte, len(cryptoKey))) || got.CryptoKey.Length != 16 {
		t.Fatalf("key buffer not zeroed after the call: len=%d", got.CryptoKey.Length)
	}
	if !bytes.Equal(v.sas[4001].CryptoKey.Data, cryptoKey) {
		t.Fatal("fake VPP did not receive the key")
	}

	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, ts, tn) // references rebuilt from the material VPP returned
	if kvs[0].Meta != metaTS || kvs[1].Meta != metaTN {
		t.Fatalf("meta %+v %+v", kvs[0].Meta, kvs[1].Meta)
	}
	again, _ := d.Retrieve(ctx) // idempotent: the same desired state diffs to nothing twice
	mustEqual(t, again, ts, tn)

	rekeyed := tunnelSA()
	rekeyed.CryptoKey = keys.Ref(integKey)
	if _, err := d.Update(ctx, tn, rekeyed, metaTN); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("key change: %v, want ErrRecreate", err)
	}
	if err := d.Delete(ctx, ts, metaTS); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, tn)
	if err := d.Delete(ctx, ts, metaTS); err != nil {
		t.Fatalf("deleting a vanished SA is done (D-074): %v", err)
	}
	// a foreign SA (outside the owned range) and an unrecorded one in range are never returned
	foreign := v.sas[4002]
	foreign.SadID = 3001
	v.sas[3001] = foreign
	stranger := v.sas[4002]
	stranger.SadID = 4005
	v.sas[4005] = stranger
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, tn)
	s5 := tunnelSA()
	s5.SadId = 4005
	if _, err := d.Create(ctx, s5); err == nil {
		t.Fatal("an existing SA must not be adopted")
	}
	if err := d.Delete(ctx, s5, nil); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("Delete of an unrecorded SA: %v", err)
	}
	// D-071/D-080: SA ids are reused after a VPP restart — our record expires, and an SA that
	// someone else created under our id (different SPI) is not ours even with the record
	boot.RestartVPP()
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("VPP restart: %+v", kvs)
	}
	if err := d.Delete(ctx, tn, metaTN); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("Delete after VPP restart: %v", err)
	}
	delete(v.sas, 4002)
	if _, err := d.Create(ctx, tn); err != nil {
		t.Fatal(err)
	}
	reused := v.sas[4002]
	reused.Spi = 9999
	v.sas[4002] = reused
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("SA with another SPI under our id reported: %+v", kvs)
	}
	if err := d.Delete(ctx, tn, metaTN); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("Delete of a replaced SA: %v", err)
	}
	if _, ok := v.sas[3001]; !ok {
		t.Fatal("foreign SA touched")
	}
}

func TestTunnelProtect(t *testing.T) {
	v := newFakeVPP()
	tun := v.addIface("ipip4001", "w4:ipip4001")
	foreign := v.addIface("ipip3001", "w3:ipip3001")
	v.addIface("ipip0", "") // untagged: somebody else's tunnel
	cfg := newCfg(v)
	d := ipsecd.NewTunnelProtect(cfg)
	sa := ipsecd.NewSa(cfg)
	for _, id := range []uint32{4001, 4002, 4003} {
		s := transportSA()
		s.SadId, s.Spi = id, 1000+id
		if _, err := sa.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	v.sas[3001] = v.sas[4001]
	v.tps[foreign] = ipsec.IpsecTunnelProtect{SwIfIndex: 0, SaOut: 3001, NSaIn: 1, SaIn: []uint32{3001}}
	v.tps[foreign] = func(tp ipsec.IpsecTunnelProtect) ipsec.IpsecTunnelProtect {
		tp.SwIfIndex = interfaceIndex(foreign)
		return tp
	}(v.tps[foreign])

	desired := &vpnpb.IpsecTunnelProtect{Interface: "ipip4001", SaOut: 4001, SaIn: []uint32{4002}}
	if d.KeyOf(desired) != "ipsec.tunnel-protect/ipip4001" {
		t.Fatalf("key %s", d.KeyOf(desired))
	}
	p2mp := &vpnpb.IpsecTunnelProtect{Interface: "ipip4001", Nh: "10.4.0.2", SaOut: 4001, SaIn: []uint32{4002}}
	if d.KeyOf(p2mp) != "ipsec.tunnel-protect/ipip4001/10.4.0.2" {
		t.Fatalf("p2mp key %s", d.KeyOf(p2mp))
	}
	deps := d.Dependencies(desired)
	if len(deps) != 3 || deps[0].Key != "interface/ipip4001" || deps[1].Key != "ipsec.sa/4001" || deps[2].Key != "ipsec.sa/4002" {
		t.Fatalf("deps %+v", deps)
	}
	// D-065: an ipsec interface is referenced by the alias interface/ipsec<N>, which ipsec.itf provides
	itf := ipsecd.NewItf(newCfg(v))
	onItf := &vpnpb.IpsecTunnelProtect{Interface: ipsecd.ItfInterfaceName(4001), SaOut: 4001, SaIn: []uint32{4002}}
	if deps, provided := d.Dependencies(onItf), itf.ProvidedKeys(&vpnpb.IpsecItf{Instance: 4001}); deps[0].Key != "interface/ipsec4001" || len(provided) != 1 || provided[0] != deps[0].Key {
		t.Fatalf("deps on ipsec itf %+v, provided %v", deps, provided)
	}
	if _, err := d.Create(ctx, &vpnpb.IpsecTunnelProtect{Interface: "ipip4001", SaOut: 4001}); err == nil {
		t.Fatal("no sa_in must be refused")
	}
	for name, want := range map[string]error{"ipip3001": vpn.ErrForeignInterface, "ipip0": vpn.ErrNotOurs, "nope": vpn.ErrNoInterface} {
		if _, err := d.Create(ctx, &vpnpb.IpsecTunnelProtect{Interface: name, SaOut: 4001, SaIn: []uint32{4002}}); !errors.Is(err, want) {
			t.Fatalf("%s: %v, want %v", name, err, want)
		}
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if meta.(ipsecd.TunnelProtectMeta).SwIfIndex != tun {
		t.Fatalf("meta %+v", meta)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, desired)
	if kvs[0].Meta != meta {
		t.Fatalf("meta %+v", kvs[0].Meta)
	}
	swapped := &vpnpb.IpsecTunnelProtect{Interface: "ipip4001", SaOut: 4003, SaIn: []uint32{4002, 4001}}
	if m, err := d.Update(ctx, desired, swapped, meta); err != nil || m != meta {
		t.Fatalf("in-place SA swap: %v %+v", err, m)
	}
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, swapped)
	if len(v.CallsNamed("ipsec_tunnel_protect_update")) != 2 {
		t.Fatal("update must re-issue ipsec_tunnel_protect_update")
	}
	if _, err := d.Update(ctx, swapped, p2mp, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("nh change: %v", err)
	}
	if err := d.Delete(ctx, swapped, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete %+v", kvs)
	}
	if err := d.Delete(ctx, swapped, meta); err != nil {
		t.Fatalf("second delete is a no-op (D-074): %v", err)
	}
	if n := len(v.CallsNamed("ipsec_tunnel_protect_del")); n != 1 {
		t.Fatalf("%d deletes reached VPP", n)
	}
	// an index that now belongs to another owner's tunnel (VPP restart) is refused
	stale := ipsecd.TunnelProtectMeta{SwIfIndex: foreign, Interface: "ipip4001"}
	if err := d.Delete(ctx, &vpnpb.IpsecTunnelProtect{Interface: "ipip4001", SaOut: 3001, SaIn: []uint32{3001}}, stale); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("stale index: %v", err)
	}
	if _, ok := v.tps[foreign]; !ok {
		t.Fatal("foreign protection touched")
	}
}

func TestItf(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewItf(newCfg(v))
	desired := &vpnpb.IpsecItf{Instance: 4001, Mode: "p2mp"}
	if d.KeyOf(desired) != "ipsec.itf/ipsec4001" || d.Dependencies(desired) != nil {
		t.Fatalf("key %s", d.KeyOf(desired))
	}
	if _, err := d.Create(ctx, &vpnpb.IpsecItf{Instance: 1, Mode: "mesh"}); err == nil {
		t.Fatal("bad mode accepted")
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	idx := meta.(ipsecd.ItfMeta).SwIfIndex
	if v.ifaces[idx].Tag != "w4:ipsec4001" || v.ifaces[idx].InterfaceName != "ipsec4001" {
		t.Fatalf("interface %+v", v.ifaces[idx])
	}
	// a foreign ipsec interface
	fidx := v.addIface("ipsec3001", "w3:ipsec3001")
	v.itfs[fidx] = ipsec.IpsecItf{UserInstance: 3001, SwIfIndex: interfaceIndex(fidx)}
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, desired)
	if kvs[0].Meta != meta {
		t.Fatalf("meta %+v", kvs[0].Meta)
	}
	if _, err := d.Update(ctx, desired, &vpnpb.IpsecItf{Instance: 4001, Mode: "p2p"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete %+v", kvs)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatalf("second delete is a no-op (D-074): %v", err)
	}
	// the index of a deleted interface reused by another owner's interface is never deleted
	if err := d.Delete(ctx, desired, ipsecd.ItfMeta{SwIfIndex: fidx}); !errors.Is(err, vpn.ErrNotOurs) {
		t.Fatalf("stale index: %v", err)
	}
	if n := len(v.CallsNamed("ipsec_itf_delete")); n != 1 {
		t.Fatalf("%d deletes reached VPP", n)
	}
	if _, ok := v.itfs[fidx]; !ok {
		t.Fatal("foreign itf touched")
	}
}

func TestBackend(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewBackend(newCfg(v))
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, &vpnpb.IpsecBackend{Protocol: "ah", Name: "crypto engine backend"}, &vpnpb.IpsecBackend{Protocol: "esp", Name: "crypto engine backend"})
	if kvs[1].Key != "ipsec.backend/esp" {
		t.Fatalf("key %s", kvs[1].Key)
	}
	want := &vpnpb.IpsecBackend{Protocol: "esp", Name: "dpdk backend"}
	meta, err := d.Update(ctx, kvs[1].Value, want, kvs[1].Meta)
	if err != nil || meta.(ipsecd.BackendMeta).Index != 1 {
		t.Fatalf("select: %v %+v", err, meta)
	}
	if r := v.CallsNamed("ipsec_select_backend"); len(r) != 1 || r[0].(*ipsec.IpsecSelectBackend).Index != 1 {
		t.Fatalf("request %+v", r)
	}
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, &vpnpb.IpsecBackend{Protocol: "ah", Name: "crypto engine backend"}, want)
	if _, err := d.Create(ctx, &vpnpb.IpsecBackend{Protocol: "esp", Name: "qat"}); err == nil {
		t.Fatal("unknown backend accepted")
	}
	if err := d.Delete(ctx, want, meta); err != nil || len(v.CallsNamed("ipsec_select_backend")) != 1 {
		t.Fatal("Delete must be a no-op")
	}

	// non-owner (D-071): the requirement holds only when VPP already has the backend, never set
	var req scheduler.Descriptor
	for _, x := range ipsecd.All(newCfg(v)) {
		if x.Name() == ipsecd.BackendName {
			req = x
		}
	}
	if _, err := req.Create(ctx, want); err != nil {
		t.Fatalf("require (satisfied): %v", err)
	}
	if _, err := req.Create(ctx, &vpnpb.IpsecBackend{Protocol: "esp", Name: "crypto engine backend"}); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
		t.Fatalf("require (differs): %v", err)
	}
	if len(v.CallsNamed("ipsec_select_backend")) != 1 {
		t.Fatal("a non-owner selected a backend")
	}
}

func TestAsyncMode(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewAsyncMode(newCfg(v))
	// D-063: no getter → write-only, never an echo of what was applied
	if kvs, err := d.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) || len(kvs) != 0 {
		t.Fatalf("Retrieve must be unsupported: %+v %v", kvs, err)
	}
	on := &vpnpb.IpsecAsyncMode{Enabled: true}
	if d.KeyOf(on) != "ipsec.async-mode/global" {
		t.Fatalf("key %s", d.KeyOf(on))
	}
	if _, err := d.Create(ctx, on); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, on); err != nil {
		t.Fatal("Create must be idempotent (re-applied on every resync)")
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) {
		t.Fatal("still unsupported after Create")
	}
	if _, err := d.Update(ctx, on, &vpnpb.IpsecAsyncMode{}, nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(v.async) != "[true true false]" {
		t.Fatalf("calls %v", v.async)
	}
	if err := d.Delete(ctx, on, nil); err != nil || len(v.async) != 3 {
		t.Fatal("Delete leaves VPP alone")
	}
}

// TestNoMaterialInOutput formats every value, key, meta and error the SA descriptor produces
// with %v/%+v and through slog and asserts the key material never appears.
func TestNoMaterialInOutput(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewSa(newCfg(v))
	tn := tunnelSA()
	meta, err := d.Create(ctx, tn)
	if err != nil {
		t.Fatal(err)
	}
	kvs, _ := d.Retrieve(ctx)
	v.SetConnected(false)
	_, errDisc := d.Create(ctx, tn)
	v.SetConnected(true)
	_, errDup := d.Create(ctx, tn)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("sa", "desired", tn, "key", d.KeyOf(tn), "meta", meta, "retrieved", kvs, "err", errDisc, "err2", errDup)
	fmt.Fprintf(&buf, "%v %+v %s %v %v %+v", tn, kvs, d.KeyOf(tn), meta, errDisc, errDup)
	fmt.Fprintf(&buf, "%+v", d)
	for _, secret := range [][]byte{cryptoKey, integKey} {
		for _, enc := range []string{string(secret), fmt.Sprintf("%d", secret), fmt.Sprintf("%x", secret)} {
			if strings.Contains(buf.String(), enc) {
				t.Fatalf("key material leaked into formatted output:\n%s", buf.String())
			}
		}
	}
	if !strings.Contains(buf.String(), "hmac:") || strings.Contains(buf.String(), "sha256:") {
		t.Fatal("references should be visible (they are not secret)")
	}
}
