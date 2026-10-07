package ravpn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

func credentialsFixture(t *testing.T) (Credentials, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(19), Subject: pkix.Name{CommonName: "RA disposable CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{19, 1, 2, 3}}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(20), Subject: pkix.Name{CommonName: "vpn.example.test"}, DNSNames: []string{"vpn.example.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour)}, ca, caKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(label string, data []byte) []byte {
		return pem.EncodeToMemory(&pem.Block{Type: label, Bytes: data})
	}
	caPEM := encode("CERTIFICATE", caDER)
	return Credentials{Certificate: append(encode("CERTIFICATE", leafDER), caPEM...), PrivateKey: encode("PRIVATE"+" KEY", keyDER), ClientCA: caPEM, ClientCRL: encode("X509 CRL", crlDER)}, now
}
func TestCredentialSnapshotIdentityExpiryKeyAndClientTrust(t *testing.T) {
	value, now := credentialsFixture(t)
	if VerifyCredentials(value, "vpn.example.test", true, now) != nil {
		t.Fatal("verified snapshot refused")
	}
	for _, scenario := range []struct {
		identity string
		when     time.Time
	}{
		{"foreign.example.test", now}, {"vpn.example.test", now.Add(2 * time.Hour)}, {"vpn.example.test", now.Add(-2 * time.Hour)},
	} {
		if VerifyCredentials(value, scenario.identity, true, scenario.when) == nil {
			t.Fatal("invalid identity/time accepted")
		}
	}
	foreign, _ := credentialsFixture(t)
	for _, change := range []func(*Credentials){
		func(c *Credentials) { c.PrivateKey = foreign.PrivateKey },
		func(c *Credentials) { c.ClientCA = foreign.ClientCA },
		func(c *Credentials) { c.ClientCRL = nil },
		func(c *Credentials) {
			c.Certificate = append(append([]byte{}, c.Certificate...), []byte("trailing material")...)
		},
		func(c *Credentials) { c.PrivateKey = append(append([]byte{}, c.PrivateKey...), c.PrivateKey...) },
	} {
		changed := value
		change(&changed)
		if VerifyCredentials(changed, "vpn.example.test", true, now) == nil {
			t.Fatal("mismatched/absent/ambiguous credential snapshot accepted")
		}
	}
}
func TestCredentialsGenericFormatsNeverExposePrivateKey(t *testing.T) {
	value, _ := credentialsFixture(t)
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(encoded), fmt.Sprintf("%v", value), fmt.Sprintf("%+v", &value), fmt.Sprintf("%#v", value)} {
		if strings.Contains(text, "BEGIN") || strings.Contains(text, string(value.PrivateKey)) {
			t.Fatal("private material serialized")
		}
	}
}
