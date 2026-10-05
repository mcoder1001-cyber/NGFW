package ravpn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestIntegrationPrivateEAPTLSCertificateAcceptance(t *testing.T) {
	for _, scenario := range []string{"valid", "revoked", "foreign"} {
		t.Run(scenario, func(t *testing.T) { runPrivateEAPWithCertificate(t, false, scenario) })
	}
}
func tlsCredentialsFixture(t *testing.T, scenario string) (Credentials, []byte, []byte, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	key := func() *ecdsa.PrivateKey {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		return k
	}
	caKey := key()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(101), Subject: pkix.Name{CommonName: "RA private TLS CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{19, 2, 3, 4}}
	caDER, e := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if e != nil {
		t.Fatal(e)
	}
	ca, e := x509.ParseCertificate(caDER)
	if e != nil {
		t.Fatal(e)
	}
	encode := func(kind string, data []byte) []byte { return pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}) }
	issue := func(name string, serial int64, usage x509.ExtKeyUsage, issuer *x509.Certificate, issuerKey *ecdsa.PrivateKey) ([]byte, []byte) {
		k := key()
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		der, e := x509.CreateCertificate(rand.Reader, template, issuer, &k.PublicKey, issuerKey)
		if e != nil {
			t.Fatal(e)
		}
		private, e := x509.MarshalPKCS8PrivateKey(k)
		if e != nil {
			t.Fatal(e)
		}
		return encode("CERTIFICATE", der), encode("PRIVATE KEY", private)
	}
	serverCert, serverKey := issue("vpn.example.test", 102, x509.ExtKeyUsageServerAuth, ca, caKey)
	clientCA, clientCAKey := ca, caKey
	if scenario == "foreign" {
		clientCAKey = key()
		foreign := *caTemplate
		foreign.SerialNumber = big.NewInt(201)
		foreign.Subject.CommonName = "Foreign private CA"
		foreign.SubjectKeyId = []byte{99, 1, 2, 3}
		der, e := x509.CreateCertificate(rand.Reader, &foreign, &foreign, &clientCAKey.PublicKey, clientCAKey)
		if e != nil {
			t.Fatal(e)
		}
		clientCA, e = x509.ParseCertificate(der)
		if e != nil {
			t.Fatal(e)
		}
	}
	clientCert, clientKey := issue("client", 103, x509.ExtKeyUsageClientAuth, clientCA, clientCAKey)
	crl := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour)}
	if scenario == "revoked" {
		crl.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(103), RevocationTime: now.Add(-time.Minute)}}
	}
	crlDER, e := x509.CreateRevocationList(rand.Reader, crl, ca, caKey)
	if e != nil {
		t.Fatal(e)
	}
	caPEM := encode("CERTIFICATE", caDER)
	return Credentials{Certificate: append(serverCert, caPEM...), PrivateKey: serverKey, ClientCA: caPEM, ClientCRL: encode("X509 CRL", crlDER)}, clientCert, clientKey, now
}
