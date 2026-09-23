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
	ipsecd "ngfw/agent/internal/descriptors/ipsec"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Test vectors: the documented placeholder, never real material (00-CONTEXT "Never do these").
var (
	cryptoKey = []byte("VRX_TEST_PSK_DF5")     // 16 bytes → aes-gcm-128 / aes-cbc-128
	integKey  = []byte("VRX_TEST_PSK_DF5_int") // 20 bytes → sha1-96
	secrets   = vpn.NewMapResolver(cryptoKey, integKey)
	owner     = "w4"
	ctx       = context.Background()
)

func newCfg(v *fakeVPP) ipsecd.Config {
	return ipsecd.Config{Client: v, Owner: owner, Secrets: secrets, IDs: vpn.IDRange{Lo: 4000, Hi: 4999}}
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
	if _, err := d.Create(ctx, desired); err == nil {
		t.Fatal("VPP retval must surface")
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
	v.SetConnected(false)
	if _, err := d.Retrieve(ctx); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
}

func TestSpdInterface(t *testing.T) {
	v := newFakeVPP()
	loop := v.addIface("loop400", "w4:loop400")
	other := v.addIface("loop300", "w3:loop300")
	v.spds[4001], v.spds[3001] = 11, 12
	v.bindings[other] = 12
	d := ipsecd.NewSpdInterface(newCfg(v))
	desired := &vpnpb.IpsecSpdInterface{Interface: "loop400", SpdId: 4001}
	if d.KeyOf(desired) != "ipsec.spd-interface/loop400" {
		t.Fatalf("key %s", d.KeyOf(desired))
	}
	deps := d.Dependencies(desired)
	if len(deps) != 2 || deps[0].Key != "ipsec.spd/4001" || deps[1].Key != "interface/loop400" || deps[0].Optional || deps[1].Optional {
		t.Fatalf("deps %+v", deps)
	}
	if _, err := d.Create(ctx, &vpnpb.IpsecSpdInterface{Interface: "nope", SpdId: 4001}); err == nil {
		t.Fatal("unknown interface must fail")
	}
	meta, err := d.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	m := meta.(ipsecd.SpdInterfaceMeta)
	if m.SwIfIndex != loop || m.SpdID != 4001 || m.SpdIndex != 11 {
		t.Fatalf("meta %+v", m)
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, kvs, desired) // w3's binding filtered, index 11 decoded to 4001 via the learned map
	if kvs[0].Meta != meta {
		t.Fatalf("meta %+v != %+v", kvs[0].Meta, meta)
	}
	if _, err := d.Update(ctx, desired, &vpnpb.IpsecSpdInterface{Interface: "loop400", SpdId: 4002}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}

	// A fresh descriptor (agent restart) has not learned the pool index: the binding is
	// retrieved with spd_id 0, Update recreates, Delete still works through any known SPD id.
	fresh := ipsecd.NewSpdInterface(newCfg(v))
	kvs, err = fresh.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || kvs[0].Value.(*vpnpb.IpsecSpdInterface).GetSpdId() != 0 {
		t.Fatalf("fresh Retrieve = %+v %v", kvs, err)
	}
	if _, err := fresh.Update(ctx, kvs[0].Value, desired, kvs[0].Meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("fresh Update: %v", err)
	}
	if err := fresh.Delete(ctx, kvs[0].Value, kvs[0].Meta); err != nil {
		t.Fatal(err)
	}
	if _, bound := v.bindings[loop]; bound {
		t.Fatal("binding not removed")
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after delete: %+v", kvs)
	}
	if _, bound := v.bindings[other]; !bound {
		t.Fatal("foreign binding touched")
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
	v.spds[4001], v.spds[3001] = 1, 2
	v.policies[3001] = []ipsec_typesSpdEntryV2{{SpdID: 3001, Priority: 1, Protocol: 255}}
	d := ipsecd.NewSpdEntry(newCfg(v))
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
	if err := d.Delete(ctx, bypass, nil); err == nil {
		t.Fatal("deleting twice must surface VPP's error")
	}
	if len(v.policies[3001]) != 1 {
		t.Fatal("foreign policy touched")
	}
}

type ipsec_typesSpdEntryV2 = ipsecSpdEntryV2Alias

func transportSA() *vpnpb.IpsecSa {
	return &vpnpb.IpsecSa{
		SadId: 4001, Spi: 1001, Protocol: "esp", CryptoAlg: "aes-gcm-128", CryptoKey: vpn.Ref(cryptoKey),
		IntegAlg: "none", Salt: 0x1234, Inbound: true,
	}
}

func tunnelSA() *vpnpb.IpsecSa {
	return &vpnpb.IpsecSa{
		SadId: 4002, Spi: 1002, Protocol: "esp", CryptoAlg: "aes-cbc-128", CryptoKey: vpn.Ref(cryptoKey),
		IntegAlg: "sha1-96", IntegKey: vpn.Ref(integKey), UseEsn: true, UseAntiReplay: true, AntiReplayWindowSize: 128,
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
		"stray key":    func(s *vpnpb.IpsecSa) { s.IntegKey = vpn.Ref(integKey) },
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
	rekeyed.CryptoKey = vpn.Ref(integKey)
	if _, err := d.Update(ctx, tn, rekeyed, metaTN); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("key change: %v, want ErrRecreate", err)
	}
	if err := d.Delete(ctx, ts, metaTS); err != nil {
		t.Fatal(err)
	}
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, tn)
	if err := d.Delete(ctx, ts, metaTS); err == nil {
		t.Fatal("second delete must surface VPP's error")
	}
	// a foreign SA (outside the owned range) is never returned
	foreign := v.sas[4002]
	foreign.SadID = 3001
	v.sas[3001] = foreign
	kvs, _ = d.Retrieve(ctx)
	mustEqual(t, kvs, tn)
}

func TestTunnelProtect(t *testing.T) {
	v := newFakeVPP()
	tun := v.addIface("ipip4001", "w4:ipip4001")
	foreign := v.addIface("ipip3001", "w3:ipip3001")
	d := ipsecd.NewTunnelProtect(newCfg(v))
	sa := ipsecd.NewSa(newCfg(v))
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
	if _, err := d.Create(ctx, &vpnpb.IpsecTunnelProtect{Interface: "ipip4001", SaOut: 4001}); err == nil {
		t.Fatal("no sa_in must be refused")
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
}

func TestAsyncMode(t *testing.T) {
	v := newFakeVPP()
	d := ipsecd.NewAsyncMode(newCfg(v))
	if kvs, err := d.Retrieve(ctx); err != nil || len(kvs) != 0 {
		t.Fatalf("unknown state must retrieve nothing: %+v %v", kvs, err)
	}
	on := &vpnpb.IpsecAsyncMode{Enabled: true}
	if d.KeyOf(on) != "ipsec.async-mode/global" {
		t.Fatalf("key %s", d.KeyOf(on))
	}
	if _, err := d.Create(ctx, on); err != nil {
		t.Fatal(err)
	}
	kvs, _ := d.Retrieve(ctx)
	mustEqual(t, kvs, on)
	if _, err := d.Update(ctx, on, &vpnpb.IpsecAsyncMode{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(v.async) != 2 || v.async[0] != true || v.async[1] != false {
		t.Fatalf("calls %v", v.async)
	}
	if err := d.Delete(ctx, on, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = d.Retrieve(ctx); len(kvs) != 0 || len(v.async) != 2 {
		t.Fatal("Delete must only forget the cached value")
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
		if bytes.Contains(buf.Bytes(), secret) {
			t.Fatalf("key material leaked into formatted output:\n%s", buf.String())
		}
	}
	if !strings.Contains(buf.String(), "sha256:") {
		t.Fatal("references should be visible (they are not secret)")
	}
}
