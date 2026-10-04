package desired

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
	vpnpb "ngfw/agent/internal/descriptors/vpn/pb"
	"ngfw/agent/internal/scheduler"
)

func certificateFixture(t *testing.T) (*ngfwv1.DesiredState, IKEv2Env, map[string][]byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	makeCert := func(name string) []byte {
		cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	material := map[string][]byte{"key/local": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), "cert/local": makeCert("local.test"), "cert/peer": makeCert("remote.test")}
	keyer, err := vpn.NewKeyer(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	resolver := vpn.NewMapResolver(keyer)
	for _, m := range material {
		resolver.Add(m)
	}
	ds := nativeDoc(t)
	ds.Vpn.Ipsec.Tunnels["site"].Auth = &ngfwv1.IpsecAuth{Method: proto.String("cert"), Certificate: proto.String("local"), PeerCertificate: proto.String("peer")}
	ds.Vpn.Pki = &ngfwv1.PkiConfig{Certificates: map[string]*ngfwv1.PkiCertificate{"local": {CertificateRef: proto.String("cert/local"), PrivateKeyRef: proto.String("key/local")}, "peer": {CertificateRef: proto.String("cert/peer")}}}
	env := IKEv2Env{GlobalsOwner: true, NativeRoot: t.TempDir() + "/native", Resolve: resolver.Resolve, SecretRef: func(_ context.Context, ref string) (string, error) {
		m, ok := material[ref]
		if !ok {
			return "", fmt.Errorf("unavailable")
		}
		return keyer.Ref(m), nil
	}}
	t.Cleanup(func() {
		for _, m := range material {
			clear(m)
		}
	})
	return ds, env, material
}

func TestNativeCertificateProjectionAndPreflight(t *testing.T) {
	ds, env, _ := certificateFixture(t)
	s := &sink{}
	IKEv2(s, ds, inVPN, env)
	if len(s.errs) != 0 {
		t.Fatal(s.errs)
	}
	p := s.value(scheduler.Join(ikev2.ProfileName, "site")).(*vpnpb.Ikev2Profile)
	if p.Auth.Method != ikev2.AuthRSASig || s.value(ikev2.LocalKeyKey) == nil || !strings.Contains(p.Auth.CertFile, env.NativeRoot) {
		t.Fatal("native certificate objects missing")
	}
	for _, tc := range []struct {
		name  string
		alter func(*ngfwv1.DesiredState, *IKEv2Env)
	}{
		{"ca-only", func(d *ngfwv1.DesiredState, _ *IKEv2Env) {
			d.Vpn.Ipsec.Tunnels["site"].Auth.RemoteCa = proto.String("ca")
		}},
		{"missing-peer", func(d *ngfwv1.DesiredState, _ *IKEv2Env) { d.Vpn.Ipsec.Tunnels["site"].Auth.PeerCertificate = nil }},
		{"missing-local-key", func(d *ngfwv1.DesiredState, _ *IKEv2Env) { d.Vpn.Pki.Certificates["local"].PrivateKeyRef = nil }},
		{"wrong-remote-id", func(d *ngfwv1.DesiredState, _ *IKEv2Env) {
			d.Vpn.Ipsec.Tunnels["site"].RemoteId = proto.String("@wrong.test")
		}},
		{"wrong-local-id", func(d *ngfwv1.DesiredState, _ *IKEv2Env) {
			d.Vpn.Ipsec.Tunnels["site"].LocalId = proto.String("@wrong.test")
		}},
		{"non-globals-owner", func(_ *ngfwv1.DesiredState, e *IKEv2Env) { e.GlobalsOwner = false }},
		{"unavailable-material", func(_ *ngfwv1.DesiredState, e *IKEv2Env) {
			e.Resolve = func(context.Context, string) ([]byte, error) { return nil, fmt.Errorf("unavailable") }
		}},
		{"foreign-owner", func(_ *ngfwv1.DesiredState, e *IKEv2Env) {
			e.CertificateReady = func(context.Context) error { return fmt.Errorf("foreign profile") }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := proto.Clone(ds).(*ngfwv1.DesiredState)
			e := env
			tc.alter(d, &e)
			s := &sink{}
			IKEv2(s, d, inVPN, e)
			if len(s.errs) == 0 || len(s.kvs) != 0 {
				t.Fatal("invalid certificate configuration projected objects", s.errs)
			}
		})
	}
}
func TestNativeCertificateSharedIdentity(t *testing.T) {
	ds, env, _ := certificateFixture(t)
	second := proto.Clone(ds.Vpn.Ipsec.Tunnels["site"]).(*ngfwv1.IpsecTunnel)
	ds.Vpn.Ipsec.Tunnels["second"] = second
	if _, err := nativeCertificates(ds, env); err != nil {
		t.Fatal(err)
	}
	second.LocalId = proto.String("@other.test")
	if _, err := nativeCertificates(ds, env); err == nil {
		t.Fatal("conflicting local identity accepted")
	}
	second.Enabled = proto.Bool(false)
	if _, err := nativeCertificates(ds, env); err != nil {
		t.Fatal("disabled profile reserved identity", err)
	}
}

func TestNativeCertificateMaterialBoundsAndValidity(t *testing.T) {
	ds, _, material := certificateFixture(t)
	_ = ds
	key, err := nativeRSAKey(material["key/local"])
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*x509.Certificate)
	}{
		{"expired", func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-time.Minute) }},
		{"future", func(c *x509.Certificate) { c.NotBefore = time.Now().Add(time.Minute) }},
		{"CA", func(c *x509.Certificate) { c.IsCA = true; c.BasicConstraintsValid = true }},
		{"no-signature-usage", func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageKeyEncipherment }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"remote.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
			tc.change(c)
			der, e := x509.CreateCertificate(rand.Reader, c, c, &key.PublicKey, key)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = nativeLeaf(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); e == nil {
				t.Fatal("invalid leaf accepted")
			}
		})
	}
	for _, bad := range [][]byte{[]byte("invalid"), make([]byte, (64<<10)+1), append(append([]byte{}, material["cert/peer"]...), material["cert/local"]...)} {
		if _, e := nativeLeaf(bad); e == nil {
			t.Fatal("unbounded/non-leaf PEM accepted")
		}
	}
	for _, bad := range [][]byte{[]byte("invalid"), make([]byte, (16<<10)+1), material["cert/peer"]} {
		if _, e := nativeRSAKey(bad); e == nil {
			t.Fatal("invalid private key accepted")
		}
	}
}
