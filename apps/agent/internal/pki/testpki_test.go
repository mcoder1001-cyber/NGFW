package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"ngfw/agent/internal/descriptors/vpn"
)

// testPKI is throwaway PKI material generated per test run (nothing is committed; names carry the slot prefix).
type testPKI struct {
	caCert, caKey        []byte // CA certificate (CA:TRUE) and its key (PEM)
	leafCert, leafKey    []byte // gateway certificate signed by the CA, and its key
	otherKey             []byte // a key that matches nothing
	crl                  []byte // CRL signed by the CA
	foreignCRL           []byte // CRL signed by another CA
	notCA                []byte // a self-signed end-entity certificate (CA:FALSE)
	caLeaf, leafLeaf     *x509.Certificate
	caSigner, leafSigner crypto.Signer
}

func pemOf(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

func keyPEM(t *testing.T, k crypto.Signer) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pemOf(labelPKCS8, der)
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func cert(t *testing.T, tmpl, parent *x509.Certificate, pub crypto.PublicKey, signer crypto.Signer) (*x509.Certificate, []byte) {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c, pemOf("CERTIFICATE", der)
}

func crlOf(t *testing.T, ca *x509.Certificate, signer crypto.Signer) []byte {
	t.Helper()
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: time.Now().Add(-time.Hour), NextUpdate: time.Now().Add(24 * time.Hour),
		RevokedCertificateEntries: []x509.RevocationListEntry{{SerialNumber: big.NewInt(99), RevocationTime: time.Now().Add(-time.Minute)}},
	}, ca, signer)
	if err != nil {
		t.Fatal(err)
	}
	return pemOf("X509 CRL", der)
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	p := &testPKI{}
	now := time.Now()
	caKey := newKey(t)
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "w5-test-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	p.caLeaf, p.caCert = cert(t, caTmpl, caTmpl, caKey.Public(), caKey)
	p.caKey, p.caSigner = keyPEM(t, caKey), caKey

	leafKey := newKey(t)
	leafTmpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "gw.w5.test"},
		DNSNames: []string{"gw.w5.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	p.leafLeaf, p.leafCert = cert(t, leafTmpl, p.caLeaf, leafKey.Public(), caKey)
	p.leafKey, p.leafSigner = keyPEM(t, leafKey), leafKey

	p.otherKey = keyPEM(t, newKey(t))
	p.crl = crlOf(t, p.caLeaf, caKey)

	otherCAKey := newKey(t)
	otherTmpl := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "w5-other-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	oc, _ := cert(t, otherTmpl, otherTmpl, otherCAKey.Public(), otherCAKey)
	p.foreignCRL = crlOf(t, oc, otherCAKey)

	eeKey := newKey(t)
	eeTmpl := &x509.Certificate{SerialNumber: big.NewInt(4), Subject: pkix.Name{CommonName: "w5-not-a-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, IsCA: false}
	_, p.notCA = cert(t, eeTmpl, eeTmpl, eeKey.Public(), eeKey)
	return p
}

func rsaKeyPEM(t *testing.T) []byte {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pemOf(labelRSA, x509.MarshalPKCS1PrivateKey(k))
}

// source returns a MapSource holding the material under the refs the tests use.
func (p *testPKI) source() *MapSource {
	s := NewMapSource()
	s.Put("cert/w5-ca", p.caCert)
	s.Put("cert/w5-gw", p.leafCert)
	s.Put("key/w5-gw", p.leafKey)
	s.Put("cert/w5-ca.crl", p.crl)
	return s
}

// gwFiles is the file set of one cert-auth tunnel: gateway certificate + key, its CA and the CA's CRL.
func gwFiles() []File {
	return []File{
		{Kind: KindCert, Name: "w5-gw", Ref: "cert/w5-gw"},
		{Kind: KindKey, Name: "w5-gw", Ref: "key/w5-gw"},
		{Kind: KindCA, Name: "w5-ca", Ref: "cert/w5-ca"},
		{Kind: KindCRL, Name: "w5-ca", Ref: "cert/w5-ca.crl", Optional: true},
	}
}

func testKeyer(t *testing.T) *vpn.Keyer {
	t.Helper()
	k, err := vpn.NewKeyer([]byte("NGFW_TEST_PSK_FPKI_fingerprint-key-32b!"))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// newMat returns a materialiser over a temp swanctl dir and state dir.
func newMat(t *testing.T, root, state string, src Source) *Materialiser {
	t.Helper()
	m, err := New(Config{Root: root, Manifest: state + "/pki-files-w5.json", Keyer: testKeyer(t), Source: src})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
