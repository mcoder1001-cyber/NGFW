package ikev2_test

// Host checks against the shared VPP (VRX_INTEGRATION=1, shared lab lock, slot prefix), through
// P05's reconciler (vpntest.Agent) including the restart simulation. Profiles are named "<prefix>-…", fixtures are prefixed loopbacks, addresses
// are in 10.<slot>.0.0/16, the ipsec-over-udp port is 20000+100*slot+1 (DF-5 port scheme,
// docs/agent/descriptors/ikev2.md). The rsa-sig certificate and the local key are throwaway
// material generated at test time under /run/vrx-test/<prefix>/ and removed in Cleanup. The
// VPP-globals (sleep interval, liveness, local key) are only read or required as a non-owner;
// the owner's setters of the getter-less ones run only behind VRX_DF5_GLOBALS=1. No peer
// exists: configuration is asserted, not negotiation. VRX_DF5_PAUSE=<seconds> holds the objects
// before cleanup so `vppctl show ikev2 profile` evidence can be captured.

import (
	"context"
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

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/internal/descriptors/dfkit"
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

func byName(ds []scheduler.Descriptor, name string) scheduler.Descriptor {
	for _, d := range ds {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

func TestIkev2OnHost(t *testing.T) {
	c := vpntest.Connect(t)
	ctx := vpntest.Context(t)
	owner := vpptest.Prefix(t)
	slot := vpptest.Slot(t)
	udpPort := uint32(20000 + 100*slot + 1) //nolint:gosec // slots are 1–11
	store, err := dfkit.NewFileBootStore(filepath.Join(t.TempDir(), "records.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := ikev2d.Config{Client: c, Owner: owner, Secrets: secrets, Boot: store}
	dir := filepath.Join("/run/vrx-test", owner)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	certFile, _ := throwawayRSA(t, dir)

	respIf, _ := vpntest.Loopback(ctx, t, c, owner, 2)
	tunIf, _ := vpntest.Loopback(ctx, t, c, owner, 3)

	ds := ikev2d.All(cfg) // a test slot is never the globals owner (D-071)
	profile, hostname := byName(ds, ikev2d.ProfileName), byName(ds, ikev2d.ResponderHostnameName)
	sleep, liveness, localKey := byName(ds, ikev2d.SleepIntervalName), byName(ds, ikev2d.LivenessName), byName(ds, ikev2d.LocalKeyName)

	// ---- VPP-globals as a non-owner: read under the shared globals lock, never set ----
	vpntest.LockGlobals(t, false)
	kvs, err := ikev2d.NewSleepInterval(cfg).Retrieve(ctx)
	if err != nil || len(kvs) != 1 {
		t.Fatalf("sleep-interval Retrieve: %v %v", kvs, err)
	}
	t.Logf("%s: VPP reports %s (read under the shared globals lock)", sleep.Name(), prototext.Format(kvs[0].Value))
	if _, err := sleep.Create(ctx, kvs[0].Value); err != nil {
		t.Fatalf("requiring VPP's own sleep interval: %v", err)
	}
	t.Logf("%s (non-owner): requirement satisfied without setting", sleep.Name())
	for _, x := range []struct {
		d scheduler.Descriptor
		v proto.Message
	}{{liveness, &vpnpb.Ikev2Liveness{Period: 30, MaxRetries: 3}}, {localKey, &vpnpb.Ikev2LocalKey{KeyFile: filepath.Join(dir, "k.pem")}}} {
		if _, err := x.d.Create(ctx, x.v); !errors.Is(err, vpn.ErrNotGlobalsOwner) {
			t.Fatalf("%s as non-owner: %v", x.d.Name(), err)
		}
		t.Logf("%s (non-owner): refused: %v", x.d.Name(), err)
	}

	// ---- profiles (psk with every part, rsa-sig) + a responder hostname, applied through P05 ----
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
	rsaP := &vpnpb.Ikev2Profile{
		Name:    "df5-rsa",
		Auth:    &vpnpb.Ikev2Auth{Method: "rsa-sig", CertFile: certFile},
		LocalId: &vpnpb.Ikev2Id{Type: "ip4", Value: vpntest.SlotAddr(t, 5, 1)},
	}
	hn := &vpnpb.Ikev2ResponderHostname{Profile: rsaP.Name, Interface: respIf, Hostname: "peer." + owner + ".vrx.test"}
	desired := []proto.Message{psk, rsaP, hn}
	pd := []scheduler.Descriptor{profile, hostname}
	t.Cleanup(func() {
		_ = vpntest.NewAgent(c, owner, ikev2d.NewProfile(cfg), ikev2d.NewResponderHostname(cfg)).S.Apply(context.Background(), nil, scheduler.All)
	})
	agent1 := vpntest.NewAgent(c, owner, pd...)
	agent1.Apply(ctx, t, desired)
	mustRetrieveEqual(t, profile, psk)
	mustRetrieveEqual(t, profile, rsaP) // the hostname does not change the profile's value
	mustBeWriteOnly(t, hostname)

	// update in place: new remote id, new ESP transforms, new port → one Update
	psk2 := proto.Clone(psk).(*vpnpb.Ikev2Profile)
	psk2.RemoteId = &vpnpb.Ikev2Id{Type: "rfc822", Value: "peer@" + owner + ".vrx.test"}
	psk2.Esp = &vpnpb.Ikev2EspTransforms{CryptoAlg: "aes-cbc", CryptoKeySize: 128, IntegAlg: "sha1-96"}
	psk2.IpsecOverUdpPort = udpPort + 1
	desired2 := []proto.Message{psk2, rsaP, hn}
	p := agent1.Plan(ctx, t, desired2)
	if s := p.Summary(); s.Updated != 1 || s.Created+s.Deleted != 0 {
		t.Fatalf("update plan: %s", vpntest.PlanString(p))
	}
	t.Logf("update plan: %s", vpntest.PlanString(p))
	agent1.Apply(ctx, t, desired2)
	mustRetrieveEqual(t, profile, psk2)
	if _, err := profile.Update(ctx, psk2, &vpnpb.Ikev2Profile{Name: psk2.Name}, nil); err != scheduler.ErrRecreate {
		t.Fatalf("removing parts must be ErrRecreate, got %v", err)
	}

	// ---- the same desired state again → empty plan (PSK compared by reference) ----
	agent1.MustEmptyPlan(ctx, t, "agent 1, second apply", desired2)

	// ---- restart simulation: fresh connection + fresh descriptors, same persisted records ----
	c2 := vpntest.Connect(t)
	cfg2 := cfg
	cfg2.Client = c2
	agent2 := vpntest.NewAgent(c2, owner, ikev2d.NewProfile(cfg2), ikev2d.NewResponderHostname(cfg2))
	agent2.MustEmptyPlan(ctx, t, "agent restart (fresh agent, persisted records)", desired2)
	// the fresh agent re-applies the write-only hostname (D-063 resync); the applied-once record
	// (D-076) keeps VPP's leaking setter from being called again on the same VPP instance
	agent2.Apply(ctx, t, desired2)

	// a profile lost behind the agent's back → exactly its re-creation
	if _, err := ikev2.NewServiceClient(c2).Ikev2ProfileAddDel(ctx, &ikev2.Ikev2ProfileAddDel{Name: owner + "-" + psk2.Name, IsAdd: false}); err != nil {
		t.Fatal(err)
	}
	p = agent2.Plan(ctx, t, desired2)
	if len(p.Ops) != 1 || p.Ops[0].Op != scheduler.OpCreate || p.Ops[0].Key != profile.KeyOf(psk2) {
		t.Fatalf("after loss: plan %s", vpntest.PlanString(p))
	}
	t.Logf("after loss (profile deleted via the API): plan %s", vpntest.PlanString(p))
	agent2.Apply(ctx, t, desired2)
	agent2.MustEmptyPlan(ctx, t, "after re-creation", desired2)

	// ---- SA state helper: no peer, so no SA of ours ----
	sas, err := ikev2d.SAs(ctx, c2, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(sas) != 0 {
		t.Fatalf("unexpected IKE SAs for %s: %+v", owner, sas)
	}
	t.Logf("ikev2 SA state helper: %d SAs for owner %s (no peer)", len(sas), owner)

	pauseForEvidence(t)

	// ---- the empty desired state deletes our profiles ----
	agent2.Apply(ctx, t, nil)
	for _, v := range []*vpnpb.Ikev2Profile{psk2, rsaP} {
		if _, ok := retrieveOne(t, profile, profile.KeyOf(v)); ok {
			t.Fatalf("%s still retrieved after the empty desired state", profile.KeyOf(v))
		}
		t.Logf("%s: %s gone", profile.Name(), profile.KeyOf(v))
	}
}

// TestIkev2GlobalsOwnerOnHost exercises the globals owner's setters of the getter-less
// liveness and local key. Opt-in only (VRX_DF5_GLOBALS=1, exclusive globals lock): their previous
// values cannot be read back, so they cannot be restored (shared-host-rules §7).
func TestIkev2GlobalsOwnerOnHost(t *testing.T) {
	vpntest.SkipUnlessGlobals(t, "ikev2 liveness / local key")
	c := vpntest.Connect(t)
	ctx := vpntest.Context(t)
	owner := vpptest.Prefix(t)
	vpntest.LockGlobals(t, true)
	dir := filepath.Join("/run/vrx-test", owner)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, keyFile := throwawayRSA(t, dir)
	cfg := ikev2d.Config{Client: c, Owner: owner, Secrets: secrets, Boot: dfkit.NewMemoryBootStore(), GlobalsOwner: true}
	ds := ikev2d.All(cfg)
	live := &vpnpb.Ikev2Liveness{Period: 30, MaxRetries: 3} // VPP's built-in defaults
	if _, err := byName(ds, ikev2d.LivenessName).Create(ctx, live); err != nil {
		t.Fatal(err)
	}
	mustBeWriteOnly(t, byName(ds, ikev2d.LivenessName))
	if _, err := byName(ds, ikev2d.LocalKeyName).Create(ctx, &vpnpb.Ikev2LocalKey{KeyFile: keyFile}); err != nil {
		t.Fatal(err)
	}
	mustBeWriteOnly(t, byName(ds, ikev2d.LocalKeyName))
}

func mustBeWriteOnly(t *testing.T, d scheduler.Descriptor) {
	t.Helper()
	if kvs, err := d.Retrieve(vpntest.Context(t)); !errors.Is(err, vpn.ErrRetrieveUnsupported) || kvs != nil {
		t.Fatalf("%s: Retrieve must be unsupported (D-063), got %v %v", d.Name(), kvs, err)
	}
	t.Logf("%s: applied; write-only (Retrieve: %s)", d.Name(), vpn.ErrRetrieveUnsupported)
}
