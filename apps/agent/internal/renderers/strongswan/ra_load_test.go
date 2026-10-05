package strongswan

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"github.com/strongswan/govici/vici"
	"math/big"
	"strings"
	"testing"
	"time"
)

type raLoadFake struct {
	raFake
	calls              []string
	existing           bool
	malformed          bool
	fail               string
	loaded             bool
	eap                bool
	expectedCredential string
}

func (f *raLoadFake) Call(_ context.Context, name string, input *vici.Message) (*vici.Message, error) {
	f.calls = append(f.calls, name)
	if name == "get-conns" {
		if f.malformed {
			return msg("conns", "malformed"), nil
		}
		if f.existing {
			return msg("conns", []string{"foreign"}), nil
		}
		if f.loaded {
			return msg("conns", []string{"ra-road"}), nil
		}
		return msg("conns", []string{}), nil
	}
	if name == f.fail {
		return msg("success", "no", "errmsg", "NGFW_TEST_SECRET_ECHO"), errors.New("NGFW_TEST_SECRET_ECHO")
	}
	if name == "load-shared" {
		f.eap = str(input, "type") == "EAP" && str(input, "data") == f.expectedCredential
	}
	if name == "load-conn" {
		f.loaded = true
	}
	return msg("success", "yes"), nil
}
func loadFixture(t *testing.T) (*RAFiles, RAMaterial, string) {
	t.Helper()
	profile, proposal := raFixture(t)
	credential := raFixtureCredential(t)
	files, err := BuildRAFiles(context.Background(), "road", profile, proposal, "/run/ngfw/ra/fixture", testResolver(map[string]string{"password/client": credential}))
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(19), Subject: pkix.Name{CommonName: "vpn.example.test"}, DNSNames: []string{"vpn.example.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return files, RAMaterial{Certificates: map[string][]byte{"/run/ngfw/ra/fixture/x509/server.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, PrivateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})}, credential
}
func TestRALoadUsesEAPCredentialAndRequiresFreshDaemon(t *testing.T) {
	files, material, credential := loadFixture(t)
	fake := &raLoadFake{expectedCredential: credential}
	name, err := LoadRA(context.Background(), fake, files, material)
	if err != nil || name != "ra-road" || !fake.eap {
		t.Fatalf("valid EAP configuration refused: %v", err)
	}
	for _, f := range []*raLoadFake{{existing: true}, {malformed: true}} {
		if _, err := LoadRA(context.Background(), f, files, material); err == nil || len(f.calls) != 1 {
			t.Fatal("nonempty or malformed daemon accepted")
		}
	}
}
func TestRALoadNeverReturnsRemoteSecretEchoOrReadsUnmappedPath(t *testing.T) {
	files, material, _ := loadFixture(t)
	for _, command := range []string{"load-cert", "load-key", "load-shared", "load-pool", "load-conn"} {
		fake := &raLoadFake{fail: command}
		_, err := LoadRA(context.Background(), fake, files, material)
		if !errors.Is(err, ErrRALoad) || strings.Contains(err.Error(), "NGFW_TEST_SECRET_ECHO") {
			t.Fatal("remote error leaked or was ignored")
		}
	}
	delete(material.Certificates, "/run/ngfw/ra/fixture/x509/server.pem")
	material.Certificates["/foreign/private.pem"] = []byte("invalid")
	if _, err := LoadRA(context.Background(), &raLoadFake{}, files, material); err == nil {
		t.Fatal("unmapped profile credential accepted")
	}
}
