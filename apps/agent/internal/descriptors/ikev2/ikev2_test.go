package ikev2_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/ikev2_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/internal/descriptors/dfkit"
	ikev2d "ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/descriptors/vpn/vpntest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Test vector: the documented placeholder, never real material (00-CONTEXT "Never do these").
var (
	psk     = []byte("VRX_TEST_PSK_DF5_ikev2")
	secrets = vpn.NewMapResolver(keys, psk)
	pskRef  = keys.Ref(psk)
	owner   = "w4"
	ctx     = context.Background()
	boot    = vpntest.NewFakeBoot()
)

func newCfg(v *fakeVPP) ikev2d.Config {
	return ikev2d.Config{Keys: keys, Client: v, Owner: owner, Secrets: secrets, Boot: dfkit.NewMemoryBootStore()}
}

func fullProfile() *vpnpb.Ikev2Profile {
	return &vpnpb.Ikev2Profile{
		Name:      "site-a",
		Auth:      &vpnpb.Ikev2Auth{Method: "psk", Psk: pskRef},
		LocalId:   &vpnpb.Ikev2Id{Type: "fqdn", Value: "a.vrx.test"},
		RemoteId:  &vpnpb.Ikev2Id{Type: "ip4", Value: "10.4.5.2"},
		LocalTs:   &vpnpb.Ikev2Ts{EndPort: 65535, StartAddr: "10.4.6.0", EndAddr: "10.4.6.255"},
		RemoteTs:  &vpnpb.Ikev2Ts{Protocol: 17, StartPort: 1, EndPort: 2, StartAddr: "fd00::", EndAddr: "fd00::ffff"},
		Responder: &vpnpb.Ikev2Responder{Interface: "loop401", Address: "10.4.5.2"},
		Ike:       &vpnpb.Ikev2IkeTransforms{CryptoAlg: "aes-gcm-16", CryptoKeySize: 256, IntegAlg: "none", PrfAlg: "hmac-sha2-256", DhGroup: "ecp-256"},
		Esp:       &vpnpb.Ikev2EspTransforms{CryptoAlg: "aes-cbc", CryptoKeySize: 128, IntegAlg: "hmac-sha2-256-128"},
		Lifetime:  &vpnpb.Ikev2Lifetime{Seconds: 3600, Jitter: 5, Handover: 3, MaxData: 1 << 20},
		UdpEncap:  true, IpsecOverUdpPort: 20401, TunnelInterface: "ipsec4001", NattDisabled: true,
	}
}

func mustRetrieve(t *testing.T, d scheduler.Descriptor, want ...proto.Message) []scheduler.KV {
	t.Helper()
	kvs, err := d.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(kvs) != len(want) {
		t.Fatalf("Retrieve returned %d objects, want %d: %v", len(kvs), len(want), kvs)
	}
	for i := range want {
		if !proto.Equal(kvs[i].Value, want[i]) {
			t.Fatalf("Retrieve[%d] = %v\nwant %v", i, kvs[i].Value, want[i])
		}
	}
	return kvs
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	ikev2d.Register(reg, newFakeVPP(), owner, ikev2d.WithSecrets(secrets))
	// singletons first (an rsa-sig profile depends on the local key), the profile last
	want := []string{ikev2d.LocalKeyName, ikev2d.SleepIntervalName, ikev2d.LivenessName, ikev2d.ProfileName, ikev2d.ResponderHostnameName}
	if got := reg.Names(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("registered %v, want %v", got, want)
	}
	for _, n := range want {
		if !scheduler.ValidName(n) || !strings.HasPrefix(n, "ikev2.") {
			t.Fatalf("descriptor name %q", n)
		}
	}
}

func TestProfileCreateRetrieveIdempotent(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	want := fullProfile()
	if d.KeyOf(want) != "ikev2.profile/site-a" {
		t.Fatalf("key %s", d.KeyOf(want))
	}
	meta, err := d.Create(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if meta.(ikev2d.ProfileMeta).VPPName != "w4-site-a" {
		t.Fatalf("meta %+v", meta)
	}
	for _, name := range []string{"ikev2_profile_add_del", "ikev2_profile_set_auth", "ikev2_set_responder", "ikev2_set_ike_transforms",
		"ikev2_set_esp_transforms", "ikev2_set_sa_lifetime", "ikev2_profile_set_udp_encap", "ikev2_profile_set_ipsec_udp_port",
		"ikev2_set_tunnel_interface", "ikev2_profile_disable_natt"} {
		if n := len(v.CallsNamed(name)); n != 1 {
			t.Fatalf("%s called %d times", name, n)
		}
	}
	if len(v.CallsNamed("ikev2_profile_set_id")) != 2 || len(v.CallsNamed("ikev2_profile_set_ts")) != 2 {
		t.Fatal("ids and selectors: one call each for local and remote")
	}
	auth := v.CallsNamed("ikev2_profile_set_auth")[0].(*ikev2.Ikev2ProfileSetAuth)
	if auth.AuthMethod != 2 || auth.IsHex || bytes.Contains(auth.Data, psk) {
		t.Fatalf("set_auth: method %d hex %v; request buffer must be zeroed after the call", auth.AuthMethod, auth.IsHex)
	}
	id := v.CallsNamed("ikev2_profile_set_id")[1].(*ikev2.Ikev2ProfileSetID)
	if id.IsLocal || id.IDType != 1 || !bytes.Equal(id.Data, []byte{10, 4, 5, 2}) {
		t.Fatalf("remote ip4 id encoded as %+v", id)
	}
	kvs := mustRetrieve(t, d, want)
	if kvs[0].Meta.(ikev2d.ProfileMeta).VPPName != "w4-site-a" {
		t.Fatalf("retrieved meta %+v", kvs[0].Meta)
	}
	// the same desired state again: equal → the scheduler plans nothing; also after a restart
	mustRetrieve(t, ikev2d.NewProfile(newCfg(v)), want)
	// Create of an existing profile fails (VPP: "policy already exists")
	if _, err := d.Create(ctx, want); err == nil {
		t.Fatal("duplicate Create must fail")
	}
}

func TestProfileMinimalAndDefaults(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	minimal := &vpnpb.Ikev2Profile{Name: "min"}
	if _, err := d.Create(ctx, minimal); err != nil {
		t.Fatal(err)
	}
	if len(v.Calls()) != 1 {
		t.Fatalf("a bare profile is one ikev2_profile_add_del, got %d calls", len(v.Calls()))
	}
	mustRetrieve(t, d, minimal) // VPP defaults (port NONE, tun_itf ~0, zero ts/transforms) decode to unset
}

func TestProfileUpdate(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	old := fullProfile()
	meta, err := d.Create(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	v.Reset()
	n := proto.Clone(old).(*vpnpb.Ikev2Profile)
	n.Esp.CryptoKeySize = 256
	n.IpsecOverUdpPort = 20402
	if _, err := d.Update(ctx, old, n, meta); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range v.Calls() {
		got = append(got, c.GetMessageName())
	}
	want := "ikev2_set_esp_transforms,ikev2_profile_set_ipsec_udp_port,ikev2_profile_set_ipsec_udp_port"
	if strings.Join(got, ",") != want {
		t.Fatalf("Update issued %v, want only the changed setters %s", got, want)
	}
	ports := v.CallsNamed("ikev2_profile_set_ipsec_udp_port")
	if ports[0].(*ikev2.Ikev2ProfileSetIpsecUDPPort).IsSet != 0 || ports[1].(*ikev2.Ikev2ProfileSetIpsecUDPPort).Port != 20402 {
		t.Fatal("port change must unset the old port, then set the new one")
	}
	mustRetrieve(t, d, n)

	// a new PSK re-issues set_auth in place
	other := []byte("VRX_TEST_PSK_DF5_ikev2_b")
	secrets.Add(other)
	n2 := proto.Clone(n).(*vpnpb.Ikev2Profile)
	n2.Auth.Psk = keys.Ref(other)
	v.Reset()
	if _, err := d.Update(ctx, n, n2, meta); err != nil {
		t.Fatal(err)
	}
	if len(v.Calls()) != 1 || v.Calls()[0].GetMessageName() != "ikev2_profile_set_auth" {
		t.Fatalf("PSK change: %d calls", len(v.Calls()))
	}
	mustRetrieve(t, d, n2)

	// what VPP cannot undo is a recreate
	for name, mut := range map[string]func(p *vpnpb.Ikev2Profile){
		"auth removed":      func(p *vpnpb.Ikev2Profile) { p.Auth = nil },
		"remote id removed": func(p *vpnpb.Ikev2Profile) { p.RemoteId = nil },
		"local ts removed":  func(p *vpnpb.Ikev2Profile) { p.LocalTs = nil },
		"responder removed": func(p *vpnpb.Ikev2Profile) { p.Responder = nil },
		"ike removed":       func(p *vpnpb.Ikev2Profile) { p.Ike = nil },
		"lifetime removed":  func(p *vpnpb.Ikev2Profile) { p.Lifetime = nil },
		"tunnel removed":    func(p *vpnpb.Ikev2Profile) { p.TunnelInterface = "" },
		"udp_encap off":     func(p *vpnpb.Ikev2Profile) { p.UdpEncap = false },
		"natt on again":     func(p *vpnpb.Ikev2Profile) { p.NattDisabled = false },
	} {
		n3 := proto.Clone(n2).(*vpnpb.Ikev2Profile)
		mut(n3)
		if _, err := d.Update(ctx, n2, n3, meta); !errors.Is(err, scheduler.ErrRecreate) {
			t.Fatalf("%s: got %v, want ErrRecreate", name, err)
		}
	}
	// removing the port is in place (ikev2_profile_set_ipsec_udp_port is_set=0)
	n4 := proto.Clone(n2).(*vpnpb.Ikev2Profile)
	n4.IpsecOverUdpPort = 0
	if _, err := d.Update(ctx, n2, n4, meta); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, n4)
}

func TestProfileCreateRollsBack(t *testing.T) {
	v := newFakeVPP()
	v.failAuth = true
	d := ikev2d.NewProfile(newCfg(v))
	if _, err := d.Create(ctx, fullProfile()); err == nil {
		t.Fatal("Create must fail when a setter fails")
	}
	if len(v.profiles) != 0 {
		t.Fatal("a failed Create must delete the half-built profile")
	}
	// unknown secret, unknown interface: same
	v.failAuth = false
	p := fullProfile()
	p.Auth.Psk = keys.Ref([]byte("VRX_TEST_PSK_unknown"))
	if _, err := d.Create(ctx, p); !errors.Is(err, vpn.ErrSecretNotFound) || len(v.profiles) != 0 {
		t.Fatalf("unknown psk: %v", err)
	}
	p = fullProfile()
	p.TunnelInterface = "loop999"
	if _, err := d.Create(ctx, p); err == nil || len(v.profiles) != 0 {
		t.Fatalf("unknown tunnel interface: %v", err)
	}
	v.SetConnected(false)
	if _, err := d.Create(ctx, fullProfile()); !errors.Is(err, vpp.ErrDisconnected) {
		t.Fatalf("disconnected: %v", err)
	}
}

func TestProfileValidation(t *testing.T) {
	d := ikev2d.NewProfile(newCfg(newFakeVPP()))
	for name, p := range map[string]*vpnpb.Ikev2Profile{
		"empty name":      {},
		"slash in name":   {Name: "a/b"},
		"too long":        {Name: strings.Repeat("x", 61)},
		"key-id":          {Name: "p", LocalId: &vpnpb.Ikev2Id{Type: "key-id", Value: "k"}},
		"ip4 id with ip6": {Name: "p", LocalId: &vpnpb.Ikev2Id{Type: "ip4", Value: "fd00::1"}},
		"bad id type":     {Name: "p", LocalId: &vpnpb.Ikev2Id{Type: "der", Value: "x"}},
		"bad method":      {Name: "p", Auth: &vpnpb.Ikev2Auth{Method: "eap"}},
		"psk w/o ref":     {Name: "p", Auth: &vpnpb.Ikev2Auth{Method: "psk"}},
		"rsa w/o cert":    {Name: "p", Auth: &vpnpb.Ikev2Auth{Method: "rsa-sig"}},
		"responder any":   {Name: "p", Responder: &vpnpb.Ikev2Responder{Address: "0.0.0.0"}},
		"responder none":  {Name: "p", Responder: &vpnpb.Ikev2Responder{Interface: "loop401"}},
		"ts mixed af":     {Name: "p", LocalTs: &vpnpb.Ikev2Ts{StartAddr: "10.4.0.0", EndAddr: "fd00::"}},
		"bad ike alg":     {Name: "p", Ike: &vpnpb.Ikev2IkeTransforms{CryptoAlg: "rot13", IntegAlg: "none", PrfAlg: "hmac-sha1", DhGroup: "none"}},
		"bad esp integ":   {Name: "p", Esp: &vpnpb.Ikev2EspTransforms{CryptoAlg: "aes-cbc", IntegAlg: "crc32"}},
		"port NONE":       {Name: "p", IpsecOverUdpPort: 0xffff},
	} {
		if _, err := d.Create(ctx, p); err == nil {
			t.Fatalf("%s: Create must fail", name)
		}
	}
}

func TestProfileOwnershipAndSecretsInDump(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	foreign := ikev2d.NewProfile(ikev2d.Config{Keys: keys, Client: v, Owner: "w3", Secrets: secrets})
	fp := fullProfile()
	if _, err := foreign.Create(ctx, fp); !errors.Is(err, vpn.ErrForeignInterface) {
		t.Fatalf("w3 must not use w4's interfaces (D-069): %v", err)
	}
	fp.Responder.Interface, fp.TunnelInterface = "loop301", ""
	if _, err := foreign.Create(ctx, fp); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, &vpnpb.Ikev2Profile{Name: "mine"}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, &vpnpb.Ikev2Profile{Name: "mine"})
	// "w4" must not match "w42-…"
	w42 := ikev2d.NewProfile(ikev2d.Config{Keys: keys, Client: v, Owner: "w42"})
	if _, err := w42.Create(ctx, &vpnpb.Ikev2Profile{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, &vpnpb.Ikev2Profile{Name: "mine"})
}

// TestIDWithZeroOctet covers govpp cutting id.data at the first NUL: an address whose bytes after
// the first zero are all zero round-trips exactly; one with a non-zero byte after a zero (10.4.0.1)
// could never be retrieved exactly and is refused (D-063: no cached desired state to paper over it).
func TestIDWithZeroOctet(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	for _, bad := range []*vpnpb.Ikev2Id{{Type: "ip4", Value: "10.4.0.1"}, {Type: "ip4", Value: "10.0.4.4"}, {Type: "ip6", Value: "fd00::1"}} {
		_, err := d.Create(ctx, &vpnpb.Ikev2Profile{Name: "zero", LocalId: bad})
		if err == nil || !strings.Contains(err.Error(), "first zero byte") {
			t.Fatalf("%s must be refused with the reason, got %v", bad.GetValue(), err)
		}
	}
	if len(v.profiles) != 0 {
		t.Fatal("refused before anything reached VPP")
	}
	ok := &vpnpb.Ikev2Profile{Name: "zero", LocalId: &vpnpb.Ikev2Id{Type: "ip4", Value: "10.4.0.0"},
		RemoteId: &vpnpb.Ikev2Id{Type: "ip6", Value: "fd00::"}}
	if _, err := d.Create(ctx, ok); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, ok) // zero-filled to data_len: exact
}

func TestResponderHostname(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	d, h := ikev2d.NewProfile(cfg), ikev2d.NewResponderHostname(cfg)
	p := &vpnpb.Ikev2Profile{Name: "h"}
	if _, err := d.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	hn := &vpnpb.Ikev2ResponderHostname{Profile: "h", Interface: "loop401", Hostname: "peer.vrx.test"}
	if h.KeyOf(hn) != "ikev2.responder-hostname/h" {
		t.Fatalf("key %s", h.KeyOf(hn))
	}
	want := []scheduler.Dependency{{Key: "ikev2.profile/h"}, {Key: "interface/loop401", Optional: true}}
	if fmt.Sprint(h.Dependencies(hn)) != fmt.Sprint(want) {
		t.Fatalf("deps %v", h.Dependencies(hn))
	}
	for i := 0; i < 2; i++ { // re-applied on every resync: must not reach VPP twice (D-076)
		if _, err := h.Create(ctx, hn); err != nil {
			t.Fatal(err)
		}
	}
	calls := v.CallsNamed("ikev2_set_responder_hostname")
	if len(calls) != 1 {
		t.Fatalf("VPP's setter leaks and resets resolution on every call: %d calls, want 1", len(calls))
	}
	r := calls[0].(*ikev2.Ikev2SetResponderHostname)
	if r.Name != "w4-h" || r.Hostname != "peer.vrx.test" || r.SwIfIndex != 1 {
		t.Fatalf("set_responder_hostname %+v", r)
	}
	// a changed value is applied; after a VPP restart the record has expired → applied once more
	hn2 := &vpnpb.Ikev2ResponderHostname{Profile: "h", Interface: "loop401", Hostname: "peer2.vrx.test"}
	if _, err := h.Update(ctx, hn, hn2, nil); err != nil {
		t.Fatal(err)
	}
	boot.RestartVPP()
	if _, err := h.Create(ctx, hn2); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Create(ctx, hn2); err != nil {
		t.Fatal(err)
	}
	if n := len(v.CallsNamed("ikev2_set_responder_hostname")); n != 3 {
		t.Fatalf("%d calls, want 3 (first, changed value, after VPP restart)", n)
	}
	// the profile deleted and re-added (recreate): its hostname must be applied again
	if err := d.Delete(ctx, p, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Create(ctx, hn2); err != nil {
		t.Fatal(err)
	}
	if n := len(v.CallsNamed("ikev2_set_responder_hostname")); n != 4 {
		t.Fatalf("%d calls, want 4 (re-applied on the re-added profile)", n)
	}
	// D-063: write-only — the hostname is not dumped and never echoed
	if kvs, err := h.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) || kvs != nil {
		t.Fatalf("Retrieve: %v %v", kvs, err)
	}
	// the profile's value is unaffected (sw_if_index set, address unspecified → no responder)
	mustRetrieve(t, d, p)
	if err := h.Delete(ctx, hn, nil); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]*vpnpb.Ikev2ResponderHostname{
		"no profile":   {Hostname: "x"},
		"no hostname":  {Profile: "h"},
		"too long":     {Profile: "h", Hostname: strings.Repeat("h", 64)},
		"no interface": {Profile: "h", Hostname: "x", Interface: "loop999"},
		"foreign":      {Profile: "h", Hostname: "x", Interface: "loop301"},
		"vpp name":     {Profile: "h", Hostname: "x", Interface: "ipsec4001x"},
	} {
		if _, err := h.Create(ctx, bad); err == nil {
			t.Fatalf("%s: Create must fail", name)
		}
	}
}

func TestProfileRSASigAndDependencies(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	p := &vpnpb.Ikev2Profile{Name: "rsa", Auth: &vpnpb.Ikev2Auth{Method: "rsa-sig", CertFile: "/run/vrx-test/w4/c.pem"},
		Responder: &vpnpb.Ikev2Responder{Interface: "loop401", Address: "10.4.0.9"}, TunnelInterface: "ipsec4001"}
	if _, err := d.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, p) // cert path comes back with VPP's trailing NUL stripped
	deps := d.Dependencies(p)
	want := []scheduler.Dependency{
		{Key: "interface/loop401", Optional: true},
		{Key: "interface/ipsec4001", Optional: true}, // D-065 alias, provided by ipsec.itf
		{Key: ikev2d.LocalKeyKey, Optional: true},
	}
	if fmt.Sprint(deps) != fmt.Sprint(want) {
		t.Fatalf("deps %v, want %v", deps, want)
	}
	if deps := d.Dependencies(&vpnpb.Ikev2Profile{Name: "x"}); len(deps) != 0 {
		t.Fatalf("bare profile deps %v", deps)
	}
}

func TestProfileDelete(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	p := fullProfile()
	meta, err := d.Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, p, meta); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d)
	if err := d.Delete(ctx, p, meta); err != nil {
		t.Fatalf("deleting a vanished profile is done (D-074): %v", err)
	}
	if n := len(v.CallsNamed("ikev2_profile_add_del")); n != 2 {
		t.Fatalf("%d add/del calls, want 2 (the second delete must not reach VPP)", n)
	}
}

func TestProfileInterfacesByLogicalName(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	// D-069: an untagged interface (physical NIC) by VPP's name is fine, another owner's is refused
	wan := &vpnpb.Ikev2Profile{Name: "wan", Responder: &vpnpb.Ikev2Responder{Interface: "wan0", Address: "10.4.0.9"}}
	if _, err := d.Create(ctx, wan); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, wan)
	foreign := &vpnpb.Ikev2Profile{Name: "f", Responder: &vpnpb.Ikev2Responder{Interface: "loop301", Address: "10.4.0.9"}}
	if _, err := d.Create(ctx, foreign); !errors.Is(err, vpn.ErrForeignInterface) {
		t.Fatalf("foreign interface: %v", err)
	}
	mustRetrieve(t, d, wan) // the failed Create rolled its profile back
}

func TestSingletons(t *testing.T) {
	v := newFakeVPP()
	cfg := newCfg(v)
	sleep, live, lk := ikev2d.NewSleepInterval(cfg), ikev2d.NewLiveness(cfg), ikev2d.NewLocalKey(cfg)

	mustRetrieve(t, sleep, &vpnpb.Ikev2SleepInterval{Seconds: 2})
	if _, err := sleep.Update(ctx, nil, &vpnpb.Ikev2SleepInterval{Seconds: 0.5}, nil); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, sleep, &vpnpb.Ikev2SleepInterval{Seconds: 0.5})
	if err := sleep.Delete(ctx, nil, nil); err != nil || v.sleep != 0.5 {
		t.Fatal("Delete leaves the plugin-wide value alone")
	}
	if _, err := sleep.Create(ctx, &vpnpb.Ikev2SleepInterval{}); err == nil {
		t.Fatal("0 s must be refused")
	}

	// D-063: liveness and the local key have no getter → write-only, never an echo
	for _, d := range []scheduler.Descriptor{live, lk} {
		if kvs, err := d.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) || kvs != nil {
			t.Fatalf("%s Retrieve: %v %v", d.Name(), kvs, err)
		}
	}
	if _, err := live.Create(ctx, &vpnpb.Ikev2Liveness{Period: 10, MaxRetries: 5}); err != nil {
		t.Fatal(err)
	}
	if v.liveness != [2]uint32{10, 5} {
		t.Fatalf("liveness %v", v.liveness)
	}
	if _, err := live.Create(ctx, &vpnpb.Ikev2Liveness{Period: 10}); err == nil {
		t.Fatal("max_retries 0 must be refused")
	}
	if _, err := lk.Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: "/run/vrx-test/w4/k.pem"}); err != nil {
		t.Fatal(err)
	}
	if v.localKey != "/run/vrx-test/w4/k.pem" {
		t.Fatalf("local key %q", v.localKey)
	}
	if _, err := lk.Create(ctx, &vpnpb.Ikev2LocalKey{}); err == nil {
		t.Fatal("empty path must be refused")
	}
	if _, err := live.Retrieve(ctx); !errors.Is(err, vpn.ErrRetrieveUnsupported) {
		t.Fatal("still write-only after Create")
	}
	if live.Delete(ctx, nil, nil) != nil || lk.Delete(ctx, nil, nil) != nil || v.liveness != [2]uint32{10, 5} {
		t.Fatal("Delete leaves VPP alone")
	}
	for _, d := range []scheduler.Descriptor{sleep, live, lk} {
		if d.KeyOf(nil) != scheduler.Join(d.Name(), "global") || d.Dependencies(nil) != nil {
			t.Fatalf("%s: singleton key/deps", d.Name())
		}
	}

	// D-071: a non-owner only requires; the getter-less globals can never be required
	req := map[string]scheduler.Descriptor{}
	for _, d := range ikev2d.All(newCfg(v)) {
		req[d.Name()] = d
	}
	before := len(v.Calls())
	if _, err := req[ikev2d.SleepIntervalName].Create(ctx, &vpnpb.Ikev2SleepInterval{Seconds: 0.5}); err != nil {
		t.Fatalf("require (satisfied): %v", err)
	}
	if _, err := req[ikev2d.SleepIntervalName].Create(ctx, &vpnpb.Ikev2SleepInterval{Seconds: 3}); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
		t.Fatalf("require (differs): %v", err)
	}
	for _, name := range []string{ikev2d.LivenessName, ikev2d.LocalKeyName} {
		if _, err := req[name].Create(ctx, &vpnpb.Ikev2Liveness{Period: 1, MaxRetries: 1}); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
			t.Fatalf("%s non-owner: %v", name, err)
		}
	}
	for _, c := range v.Calls()[before:] {
		if n := c.GetMessageName(); n != "ikev2_get_sleep_interval" {
			t.Fatalf("a non-owner sent %s", n)
		}
	}
}

func TestSAState(t *testing.T) {
	v := newFakeVPP()
	secret := bytes.Repeat([]byte{0xAB}, 32)
	keys := ikev2_types.Ikev2Keys{SkD: append([]byte(nil), secret...), SkDLen: 32, SkEi: append([]byte(nil), secret...), SkEiLen: 32}
	v.sas = []ikev2_types.Ikev2SaV3{
		{SaIndex: 7, ProfileName: "w4-site-a", State: ikev2_types.AUTHENTICATED, Ispi: 1, Rspi: 2,
			Iaddr: ip_types.NewAddress([]byte{10, 4, 5, 1}), Raddr: ip_types.NewAddress([]byte{10, 4, 5, 2}), Keys: keys,
			IID:        ikev2_types.Ikev2ID{Type: 2, DataLen: 10, Data: "a.vrx.test"},
			Encryption: ikev2_types.Ikev2SaTransform{TransformType: 1, TransformID: 12, KeyLen: 32},
			Integrity:  ikev2_types.Ikev2SaTransform{TransformType: 3, TransformID: 12},
			Prf:        ikev2_types.Ikev2SaTransform{TransformType: 2, TransformID: 5},
			Dh:         ikev2_types.Ikev2SaTransform{TransformType: 4, TransformID: 14}, Uptime: 12},
		{SaIndex: 8, ProfileName: "w3-other", Keys: keys},
	}
	v.children[7] = []ikev2_types.Ikev2ChildSaV2{{SaIndex: 7, ChildSaIndex: 0, ISpi: 0x100, RSpi: 0x200, Keys: keys,
		Encryption: ikev2_types.Ikev2SaTransform{TransformType: 1, TransformID: 20, KeyLen: 32},
		Esn:        ikev2_types.Ikev2SaTransform{TransformType: 5, TransformID: 1}}}
	sas, err := ikev2d.SAs(ctx, v, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(sas) != 1 {
		t.Fatalf("owner filter: %d SAs", len(sas))
	}
	s := sas[0]
	if s.Profile != "site-a" || s.State != "AUTHENTICATED" || s.IAddr != "10.4.5.1" || s.IID.Value != "a.vrx.test" ||
		s.Encryption != "aes-cbc/256" || s.Integrity != "hmac-sha2-256-128" || s.PRF != "hmac-sha2-256" || s.DH != "modp-2048" {
		t.Fatalf("decoded %+v", s)
	}
	if len(s.Children) != 1 || s.Children[0].Encryption != "aes-gcm-16/256" || !s.Children[0].ESN || s.Children[0].ISPI != 0x100 {
		t.Fatalf("children %+v", s.Children)
	}
	if out := fmt.Sprintf("%+v", sas); strings.Contains(out, "171 171") || bytes.Contains([]byte(out), secret) {
		t.Fatal("derived keys must not be in the state")
	}
}

func TestActions(t *testing.T) {
	v := newFakeVPP()
	if err := ikev2d.InitiateSAInit(ctx, v, owner, "site-a"); err != nil {
		t.Fatal(err)
	}
	if r := v.CallsNamed("ikev2_initiate_sa_init")[0].(*ikev2.Ikev2InitiateSaInit); r.Name != "w4-site-a" {
		t.Fatalf("sa_init for %q", r.Name)
	}
	if err := ikev2d.DeleteIKESA(ctx, v, 5); err != nil {
		t.Fatal(err)
	}
	if err := ikev2d.DeleteChildSA(ctx, v, 6); err != nil {
		t.Fatal(err)
	}
	if err := ikev2d.RekeyChildSA(ctx, v, 7); err == nil {
		t.Fatal("a non-zero retval must surface")
	}
}

// TestNoMaterialInOutput formats every value, key, meta and error the profile descriptor produces
// with %v/%+v and through slog and asserts the PSK never appears.
func TestNoMaterialInOutput(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	p := fullProfile()
	meta, err := d.Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	kvs, _ := d.Retrieve(ctx)
	_, errDup := d.Create(ctx, p)
	v.failAuth = true
	p2 := proto.Clone(p).(*vpnpb.Ikev2Profile)
	p2.Name = "other"
	_, errAuth := d.Create(ctx, p2)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("profile", "desired", p, "key", d.KeyOf(p), "meta", meta, "retrieved", kvs, "err", errDup, "err2", errAuth)
	fmt.Fprintf(&buf, "%v %+v %s %v %v %+v %+v", p, kvs, d.KeyOf(p), meta, errDup, errAuth, d)
	for _, enc := range []string{string(psk), fmt.Sprintf("%d", psk), fmt.Sprintf("%x", psk)} {
		if strings.Contains(buf.String(), enc) {
			t.Fatalf("PSK leaked into formatted output:\n%s", buf.String())
		}
	}
	if !strings.Contains(buf.String(), "hmac:") || strings.Contains(buf.String(), "sha256:") {
		t.Fatal("references should be visible (they are not secret)")
	}
}
