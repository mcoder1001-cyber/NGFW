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
	ikev2d "ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Test vector: the documented placeholder, never real material (00-CONTEXT "Never do these").
var (
	psk     = []byte("VRX_TEST_PSK_DF5_ikev2")
	secrets = vpn.NewMapResolver(psk)
	pskRef  = vpn.Ref(psk)
	owner   = "w4"
	ctx     = context.Background()
)

func newCfg(v *fakeVPP) ikev2d.Config {
	return ikev2d.Config{Client: v, Owner: owner, Secrets: secrets}
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
	want := []string{ikev2d.LocalKeyName, ikev2d.SleepIntervalName, ikev2d.LivenessName, ikev2d.ProfileName}
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
	n2.Auth.Psk = vpn.Ref(other)
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
	p.Auth.Psk = vpn.Ref([]byte("VRX_TEST_PSK_unknown"))
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
		"responder both":  {Name: "p", Responder: &vpnpb.Ikev2Responder{Address: "10.4.0.1", Hostname: "h"}},
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
	foreign := ikev2d.NewProfile(ikev2d.Config{Client: v, Owner: "w3", Secrets: secrets})
	if _, err := foreign.Create(ctx, fullProfile()); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Create(ctx, &vpnpb.Ikev2Profile{Name: "mine"}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, &vpnpb.Ikev2Profile{Name: "mine"})
	// "w4" must not match "w42-…"
	w42 := ikev2d.NewProfile(ikev2d.Config{Client: v, Owner: "w42"})
	if _, err := w42.Create(ctx, &vpnpb.Ikev2Profile{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, &vpnpb.Ikev2Profile{Name: "mine"})
}

// TestIDWithZeroOctet covers govpp cutting id.data at the first NUL: 10.4.0.1 arrives as 10.4.
func TestIDWithZeroOctet(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	p := &vpnpb.Ikev2Profile{Name: "zero", LocalId: &vpnpb.Ikev2Id{Type: "ip4", Value: "10.4.0.1"},
		RemoteId: &vpnpb.Ikev2Id{Type: "ip6", Value: "fd00::1"}}
	if _, err := d.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, d, p) // bridged by this process' cache
	// after a restart the tail is unknown: zero-filled, shows as drift, Update re-applies in place
	fresh := ikev2d.NewProfile(newCfg(v))
	kvs, _ := fresh.Retrieve(ctx)
	got := kvs[0].Value.(*vpnpb.Ikev2Profile)
	if got.GetLocalId().GetValue() != "10.4.0.0" || got.GetRemoteId().GetValue() != "fd00::" {
		t.Fatalf("zero-filled ids: %v", got)
	}
	if _, err := fresh.Update(ctx, got, p, kvs[0].Meta); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, fresh, p)
}

func TestProfileHostnameResponder(t *testing.T) {
	v := newFakeVPP()
	d := ikev2d.NewProfile(newCfg(v))
	p := &vpnpb.Ikev2Profile{Name: "h", Responder: &vpnpb.Ikev2Responder{Interface: "loop401", Hostname: "peer.vrx.test"}}
	if _, err := d.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	r := v.CallsNamed("ikev2_set_responder_hostname")[0].(*ikev2.Ikev2SetResponderHostname)
	if r.Hostname != "peer.vrx.test" || r.SwIfIndex != 1 {
		t.Fatalf("set_responder_hostname %+v", r)
	}
	mustRetrieve(t, d, p)
	// restart: the hostname is not dumped; the interface is
	kvs, _ := ikev2d.NewProfile(newCfg(v)).Retrieve(ctx)
	if got := kvs[0].Value.(*vpnpb.Ikev2Profile).GetResponder(); got.GetInterface() != "loop401" || got.GetHostname() != "" {
		t.Fatalf("responder after restart %v", got)
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
		{Key: "ipsec.itf/ipsec4001", Optional: true},
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
	if err := d.Delete(ctx, p, meta); err == nil {
		t.Fatal("deleting a missing profile must fail")
	}
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

	mustRetrieve(t, live) // nothing applied by this process yet
	if _, err := live.Create(ctx, &vpnpb.Ikev2Liveness{Period: 10, MaxRetries: 5}); err != nil {
		t.Fatal(err)
	}
	if v.liveness != [2]uint32{10, 5} {
		t.Fatalf("liveness %v", v.liveness)
	}
	mustRetrieve(t, live, &vpnpb.Ikev2Liveness{Period: 10, MaxRetries: 5})
	if _, err := live.Create(ctx, &vpnpb.Ikev2Liveness{Period: 10}); err == nil {
		t.Fatal("max_retries 0 must be refused")
	}
	_ = live.Delete(ctx, nil, nil)
	mustRetrieve(t, live)

	if _, err := lk.Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: "/run/vrx-test/w4/k.pem"}); err != nil {
		t.Fatal(err)
	}
	mustRetrieve(t, lk, &vpnpb.Ikev2LocalKey{KeyFile: "/run/vrx-test/w4/k.pem"})
	if _, err := lk.Create(ctx, &vpnpb.Ikev2LocalKey{}); err == nil {
		t.Fatal("empty path must be refused")
	}
	for _, d := range []scheduler.Descriptor{sleep, live, lk} {
		if d.KeyOf(nil) != scheduler.Join(d.Name(), "global") || d.Dependencies(nil) != nil {
			t.Fatalf("%s: singleton key/deps", d.Name())
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
	for _, enc := range []string{string(psk), fmt.Sprint(psk), fmt.Sprintf("%x", psk)} {
		if strings.Contains(buf.String(), enc) {
			t.Fatalf("PSK leaked into formatted output:\n%s", buf.String())
		}
	}
	if !strings.Contains(buf.String(), "sha256:") {
		t.Fatal("references should be visible (they are not secret)")
	}
}
