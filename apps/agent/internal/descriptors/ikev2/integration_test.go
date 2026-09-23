package ikev2_test

// One integration check per object type against the host VPP (VRX_INTEGRATION=1, shared lab
// lock, slot prefix). Profiles are named "<prefix>-…", fixtures are prefixed loopbacks, addresses
// are in 10.<slot>.0.0/16, the ipsec-over-udp port is 20000+100*slot+1 (DF-5 port scheme,
// docs/agent/descriptors/ikev2.md). The rsa-sig certificate and the local key are throwaway
// material generated at test time under /run/vrx-test/<prefix>/ and removed in Cleanup. No peer
// exists: configuration is asserted, not negotiation. VRX_DF5_PAUSE=<seconds> holds the objects
// before cleanup so `vppctl show ikev2 profile` evidence can be captured.

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	ikev2d "ngfw/agent/internal/descriptors/ikev2"
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

// throwawayRSA writes a fresh self-signed certificate and its private key (test-only material,
// never committed) into dir and returns both paths.
func throwawayRSA(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "vrx-df5-test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile = filepath.Join(dir, "ikev2-df5.crt"), filepath.Join(dir, "ikev2-df5.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(certFile); _ = os.Remove(keyFile) })
	return certFile, keyFile
}

func TestIkev2OnHost(t *testing.T) {
	c := vpntest.Connect(t)
	ctx := vpntest.Context(t)
	owner := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	udpPort := uint32(20000 + 100*slot + 1) //nolint:gosec // slots are 1–12
	cfg := ikev2d.Config{Client: c, Owner: owner, Secrets: secrets}
	dir := filepath.Join("/run/vrx-test", owner)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := throwawayRSA(t, dir)

	respIf, _ := vpntest.Loopback(ctx, t, c, owner, 2)
	tunIf, _ := vpntest.Loopback(ctx, t, c, owner, 3)

	profile, localKey := ikev2d.NewProfile(cfg), ikev2d.NewLocalKey(cfg)
	sleep, liveness := ikev2d.NewSleepInterval(cfg), ikev2d.NewLiveness(cfg)

	// ---- ikev2.sleep-interval: read-only on the shared host ----
	kvs, err := sleep.Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("sleep-interval Retrieve: %v %v", kvs, err)
	}
	t.Logf("%s: VPP reports %s (read-only check, not changed)", sleep.Name(), prototext.Format(kvs[0].Value))

	// ---- ikev2.liveness: re-applies VPP's built-in defaults (30 s / 3), no effective change ----
	live := &vpnpb.Ikev2Liveness{Period: 30, MaxRetries: 3}
	if _, err := liveness.Create(ctx, live); err != nil {
		t.Fatal(err)
	}
	mustBeWriteOnly(t, liveness)

	// ---- ikev2.local-key: throwaway key under /run/vrx-test/<prefix>/ ----
	lk := &vpnpb.Ikev2LocalKey{KeyFile: keyFile}
	if _, err := localKey.Create(ctx, lk); err != nil {
		t.Fatal(err)
	}
	mustBeWriteOnly(t, localKey)

	// ---- ikev2.profile: psk, every part set ----
	psk := &vpnpb.Ikev2Profile{
		Name:      "df5-psk",
		Auth:      &vpnpb.Ikev2Auth{Method: "psk", Psk: pskRef},
		LocalId:   &vpnpb.Ikev2Id{Type: "fqdn", Value: owner + "-local.vrx.test"},
		RemoteId:  &vpnpb.Ikev2Id{Type: "ip4", Value: vpntest.SlotAddr(t, 5, 2)},
		LocalTs:   &vpnpb.Ikev2Ts{Protocol: 0, StartPort: 0, EndPort: 65535, StartAddr: vpntest.SlotAddr(t, 6, 0), EndAddr: vpntest.SlotAddr(t, 6, 255)},
		RemoteTs:  &vpnpb.Ikev2Ts{Protocol: 17, StartPort: 1000, EndPort: 2000, StartAddr: vpntest.SlotAddr(t, 7, 0), EndAddr: vpntest.SlotAddr(t, 7, 255)},
		Responder: &vpnpb.Ikev2Responder{Interface: respIf, Address: vpntest.SlotAddr(t, 5, 2)},
		Ike:       &vpnpb.Ikev2IkeTransforms{CryptoAlg: "aes-cbc", CryptoKeySize: 256, IntegAlg: "hmac-sha2-256-128", PrfAlg: "hmac-sha2-256", DhGroup: "modp-2048"},
		Esp:       &vpnpb.Ikev2EspTransforms{CryptoAlg: "aes-gcm-16", CryptoKeySize: 256, IntegAlg: "none"},
		Lifetime:  &vpnpb.Ikev2Lifetime{Seconds: 3600, Jitter: 10, Handover: 5, MaxData: 1 << 30},
		UdpEncap:  true, IpsecOverUdpPort: udpPort, TunnelInterface: tunIf, NattDisabled: true,
	}
	meta, err := profile.Create(ctx, psk)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = profile.Delete(vpntest.Context(t), psk, meta) })
	t.Logf("profile meta: %+v", meta)
	mustRetrieveEqual(t, profile, psk)
	// a fresh descriptor (≈ agent restart) retrieves the same value: nothing is cached
	mustRetrieveEqual(t, ikev2d.NewProfile(cfg), psk)

	// update in place: new remote id, new ESP transforms, new port
	psk2 := proto.Clone(psk).(*vpnpb.Ikev2Profile)
	psk2.RemoteId = &vpnpb.Ikev2Id{Type: "rfc822", Value: "peer@" + owner + ".vrx.test"}
	psk2.Esp = &vpnpb.Ikev2EspTransforms{CryptoAlg: "aes-cbc", CryptoKeySize: 128, IntegAlg: "sha1-96"}
	psk2.IpsecOverUdpPort = udpPort + 1
	if meta, err = profile.Update(ctx, psk, psk2, meta); err != nil {
		t.Fatal(err)
	}
	mustRetrieveEqual(t, profile, psk2)
	if _, err := profile.Update(ctx, psk2, &vpnpb.Ikev2Profile{Name: psk2.Name}, meta); err != scheduler.ErrRecreate {
		t.Fatalf("removing parts must be ErrRecreate, got %v", err)
	}

	// ---- ikev2.profile: rsa-sig; ikev2.responder-hostname on it (write-only, D-063) ----
	rsaP := &vpnpb.Ikev2Profile{
		Name:    "df5-rsa",
		Auth:    &vpnpb.Ikev2Auth{Method: "rsa-sig", CertFile: certFile},
		LocalId: &vpnpb.Ikev2Id{Type: "ip4", Value: vpntest.SlotAddr(t, 5, 1)},
	}
	rsaMeta, err := profile.Create(ctx, rsaP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = profile.Delete(vpntest.Context(t), rsaP, rsaMeta) })
	mustRetrieveEqual(t, profile, rsaP)
	hostname := ikev2d.NewResponderHostname(cfg)
	hn := &vpnpb.Ikev2ResponderHostname{Profile: rsaP.Name, Interface: respIf, Hostname: "peer." + owner + ".vrx.test"}
	for i := 0; i < 2; i++ { // idempotent: the reconciler re-applies write-only objects on resync
		if _, err := hostname.Create(ctx, hn); err != nil {
			t.Fatal(err)
		}
	}
	mustBeWriteOnly(t, hostname)
	mustRetrieveEqual(t, profile, rsaP) // the hostname does not change the profile's value

	// ---- SA state helper: no peer, so no SA of ours ----
	sas, err := ikev2d.SAs(ctx, c, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(sas) != 0 {
		t.Fatalf("unexpected IKE SAs for %s: %+v", owner, sas)
	}
	t.Logf("ikev2 SA state helper: %d SAs for owner %s (no peer)", len(sas), owner)

	pauseForEvidence(t)

	for _, p := range []struct {
		v *vpnpb.Ikev2Profile
		m any
	}{{psk2, meta}, {rsaP, rsaMeta}} {
		if err := profile.Delete(ctx, p.v, p.m); err != nil {
			t.Fatal(err)
		}
		if _, ok := retrieveOne(t, profile, profile.KeyOf(p.v)); ok {
			t.Fatalf("%s still retrieved after Delete", profile.KeyOf(p.v))
		}
		t.Logf("%s: %s gone after Delete", profile.Name(), profile.KeyOf(p.v))
	}
	for _, d := range []scheduler.Descriptor{localKey, liveness, hostname} {
		if err := d.Delete(ctx, nil, nil); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: Delete is a no-op (write-only; plugin-wide or gone with its profile)", d.Name())
	}
}

func mustBeWriteOnly(t *testing.T, d scheduler.Descriptor) {
	t.Helper()
	if kvs, err := d.Retrieve(vpntest.Context(t)); !errors.Is(err, vpn.ErrRetrieveUnsupported) || kvs != nil {
		t.Fatalf("%s: Retrieve must be unsupported (D-063), got %v %v", d.Name(), kvs, err)
	}
	t.Logf("%s: applied; write-only (Retrieve: %s)", d.Name(), vpn.ErrRetrieveUnsupported)
}
