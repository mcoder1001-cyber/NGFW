package pki

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

func TestCheckMaterial(t *testing.T) {
	p := newTestPKI(t)
	encrypted := pemOf(labelEncrypted, []byte{1, 2, 3})
	certWithKey := append(append([]byte{}, p.leafCert...), p.leafKey...)
	for _, tc := range []struct {
		name string
		kind Kind
		data []byte
		want string // "" = valid
	}{
		{"leaf certificate", KindCert, p.leafCert, ""},
		{"chain leaf+ca", KindCert, append(append([]byte{}, p.leafCert...), p.caCert...), ""},
		{"CA certificate", KindCA, p.caCert, ""},
		{"ECDSA PKCS#8 key", KindKey, p.leafKey, ""},
		{"RSA PKCS#1 key", KindKey, rsaKeyPEM(t), ""},
		{"CRL", KindCRL, p.crl, ""},
		{"private key inside a certificate secret", KindCert, certWithKey, "also holds a private key"},
		{"end-entity certificate as a CA", KindCA, p.notCA, "CA:TRUE is missing"},
		{"encrypted key", KindKey, encrypted, "encrypted private keys are not supported"},
		{"certificate as a key", KindKey, p.leafCert, "is not a private key"},
		{"two keys", KindKey, append(append([]byte{}, p.leafKey...), p.otherKey...), "exactly one"},
		{"not PEM", KindCert, []byte("hello"), "not PEM"},
		{"trailing garbage", KindCert, append(append([]byte{}, p.leafCert...), []byte("-----BEGIN x")...), "trailing data"},
		{"empty", KindCert, nil, "empty"},
		{"oversize key", KindKey, bytes.Repeat([]byte("A"), MaxKeySize+1), "larger than"},
		{"CRL as a certificate", KindCert, p.crl, "unexpected PEM block"},
		{"certificate as a CRL", KindCRL, p.caCert, "X509 CRL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := check(tc.kind, tc.data)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("check: %v", err)
				}
				if !bytes.HasPrefix(c.content, []byte("-----BEGIN ")) {
					t.Fatalf("content not normalised PEM")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, ErrMaterial) {
				t.Fatalf("err = %v, want ErrMaterial containing %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "BEGIN") || strings.Contains(err.Error(), "MII") {
				t.Fatalf("error quotes material: %v", err)
			}
		})
	}
}

// TestPlanApplyRetrieve: the files of one cert-auth tunnel are written with their modes, Retrieve reports exactly the
// planned value, private keys only as an HMAC (never a plain hash, never material), and an empty set removes them.
func TestPlanApplyRetrieve(t *testing.T) {
	p := newTestPKI(t)
	root, state := t.TempDir(), t.TempDir()
	m := newMat(t, root, state, p.source())
	ctx := context.Background()

	set, errs := m.Plan(ctx, gwFiles())
	if len(errs) > 0 {
		t.Fatalf("plan: %v", errs)
	}
	if len(set.GetFiles()) != 4 {
		t.Fatalf("plan: %d files, want 4: %v", len(set.GetFiles()), set)
	}
	if err := m.Apply(ctx, set); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := map[string]os.FileMode{
		"x509/w5-gw.pem": 0o644, "private/w5-gw.pem": 0o600, "x509ca/w5-ca.pem": 0o644, "x509crl/w5-ca.pem": 0o644,
	}
	for rel, mode := range want {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s: mode %v, want %v", rel, info.Mode().Perm(), mode)
		}
	}
	if info, _ := os.Stat(filepath.Join(root, "private")); info.Mode().Perm() != 0o700 {
		t.Errorf("private/: mode %v, want 0700", info.Mode().Perm())
	}
	got, err := m.Retrieve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, set) {
		t.Fatalf("Retrieve != Plan:\n got %v\nwant %v", got, set)
	}
	for _, f := range got.GetFiles() {
		switch f.GetKind() {
		case "key":
			if !strings.HasPrefix(f.GetFingerprint(), "hmac:") {
				t.Errorf("key fingerprint %q is not an HMAC", f.GetFingerprint())
			}
		default:
			if !strings.HasPrefix(f.GetFingerprint(), "sha256:") {
				t.Errorf("%s fingerprint %q", f.GetKind(), f.GetFingerprint())
			}
		}
	}
	// nothing that looks like key material in what Retrieve/State/the manifest expose
	man, _ := os.ReadFile(filepath.Join(state, "pki-files-w5.json"))
	dump := fmt.Sprintf("%v %v %s %v", got, mustState(t, m), man, m.src)
	for _, bad := range []string{"PRIVATE KEY", "MII", "MHc"} {
		if strings.Contains(dump, bad) {
			t.Fatalf("material visible in state/manifest (%q)", bad)
		}
	}
	// idempotent: applying the same set again changes nothing
	if err := m.Apply(ctx, set); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	// rollback / removal: an empty set removes every file this agent wrote
	if err := m.Apply(ctx, &ngfwv1.PkiFileStateSet{}); err != nil {
		t.Fatalf("apply empty: %v", err)
	}
	for rel := range want {
		if _, err := os.Stat(filepath.Join(root, rel)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists after the empty apply (err %v)", rel, err)
		}
	}
	if got, _ := m.Retrieve(ctx); len(got.GetFiles()) != 0 {
		t.Fatalf("Retrieve after removal: %v", got)
	}
}

func mustState(t *testing.T, m *Materialiser) []*ngfwv1.PkiFileStateFile {
	t.Helper()
	st, err := m.State()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestRemovesOnlyUnreferencedOwnFiles: a smaller set removes the files it no longer lists, and never a file this
// agent did not write.
func TestRemovesOnlyUnreferencedOwnFiles(t *testing.T) {
	p := newTestPKI(t)
	root, state := t.TempDir(), t.TempDir()
	m := newMat(t, root, state, p.source())
	ctx := context.Background()
	full, errs := m.Plan(ctx, gwFiles())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := m.Apply(ctx, full); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "x509ca", "someone-else.pem")
	if err := os.WriteFile(foreign, p.caCert, 0o644); err != nil {
		t.Fatal(err)
	}
	caOnly, errs := m.Plan(ctx, []File{{Kind: KindCA, Name: "w5-ca", Ref: "cert/w5-ca"}})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if err := m.Apply(ctx, caOnly); err != nil {
		t.Fatal(err)
	}
	for rel, exists := range map[string]bool{"x509ca/w5-ca.pem": true, "x509/w5-gw.pem": false, "private/w5-gw.pem": false,
		"x509crl/w5-ca.pem": false, "x509ca/someone-else.pem": true} {
		_, err := os.Stat(filepath.Join(root, rel))
		if (err == nil) != exists {
			t.Errorf("%s: exists=%v, want %v", rel, err == nil, exists)
		}
	}
}

func TestPlanFindings(t *testing.T) {
	p := newTestPKI(t)
	ctx := context.Background()
	src := p.source()
	src.Put("key/w5-other", p.otherKey)
	src.Put("cert/w5-foreign.crl", p.foreignCRL)
	src.Put("cert/w5-notca", p.notCA)
	m := newMat(t, t.TempDir(), t.TempDir(), src)
	for _, tc := range []struct {
		name  string
		files []File
		want  string
	}{
		{"key does not match", []File{{Kind: KindCert, Name: "w5-gw", Ref: "cert/w5-gw"}, {Kind: KindKey, Name: "w5-gw", Ref: "key/w5-other"}}, "does not match certificate w5-gw"},
		{"key without certificate", []File{{Kind: KindKey, Name: "w5-gw", Ref: "key/w5-gw"}}, "without its certificate"},
		{"CRL of another CA", []File{{Kind: KindCA, Name: "w5-ca", Ref: "cert/w5-ca"}, {Kind: KindCRL, Name: "w5-ca", Ref: "cert/w5-foreign.crl"}}, "not signed by CA w5-ca"},
		{"not a CA", []File{{Kind: KindCA, Name: "w5-notca", Ref: "cert/w5-notca"}}, "CA:TRUE"},
		{"missing secret", []File{{Kind: KindCA, Name: "w5-x", Ref: "cert/w5-x"}}, "secret not found"},
		{"bad reference is not echoed", []File{{Kind: KindCA, Name: "w5-x", Ref: "c2VjcmV0LXZhbHVl"}}, "<redacted>"},
		{"bad name", []File{{Kind: KindCA, Name: "../etc", Ref: "cert/w5-ca"}}, "not a vpn.pki object name"},
		{"duplicate", []File{{Kind: KindCA, Name: "w5-ca", Ref: "cert/w5-ca"}, {Kind: KindCA, Name: "w5-ca", Ref: "cert/w5-ca"}}, "listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, errs := m.Plan(ctx, tc.files)
			if set != nil || len(errs) == 0 {
				t.Fatalf("plan accepted: %v", set)
			}
			if !strings.Contains(errs[0].Error(), tc.want) {
				t.Fatalf("finding %q, want %q", errs[0], tc.want)
			}
			if strings.Contains(errs[0].Error(), "c2VjcmV0LXZhbHVl") {
				t.Fatalf("a malformed reference was echoed: %v", errs[0])
			}
		})
	}
	// an optional CRL without material is left out, not refused
	set, errs := m.Plan(ctx, []File{{Kind: KindCA, Name: "w5-ca", Ref: "cert/w5-ca"}, {Kind: KindCRL, Name: "w5-ca", Ref: "cert/w5-nocrl.crl", Optional: true}})
	if len(errs) > 0 || len(set.GetFiles()) != 1 {
		t.Fatalf("optional CRL: set %v errs %v", set, errs)
	}
}

func TestNoSource(t *testing.T) {
	m := newMat(t, t.TempDir(), t.TempDir(), nil)
	_, errs := m.Plan(context.Background(), gwFiles())
	if len(errs) != 3 { // the optional CRL is not a finding
		t.Fatalf("findings %v, want 3", errs)
	}
	for _, e := range errs {
		if !errors.Is(e, ErrNoSource) {
			t.Fatalf("finding %v is not ErrNoSource", e)
		}
	}
}

// TestApplyRefusesChangedMaterial: the material changed between Plan and Apply — nothing is written.
func TestApplyRefusesChangedMaterial(t *testing.T) {
	p := newTestPKI(t)
	src := p.source()
	root := t.TempDir()
	m := newMat(t, root, t.TempDir(), src)
	set, errs := m.Plan(context.Background(), gwFiles())
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	src.Put("cert/w5-ca", p.leafCert) // swapped for a certificate that is not even a CA
	err := m.Apply(context.Background(), set)
	if err == nil {
		t.Fatal("apply accepted changed material")
	}
	if _, serr := os.Stat(filepath.Join(root, "x509", "w5-gw.pem")); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("a file was written by the refused apply")
	}
}

func TestMapSourceIsOpaque(t *testing.T) {
	s := NewMapSource()
	s.Put("key/w5-gw", []byte("NGFW_TEST_PSK_FPKI_material"))
	for _, out := range []string{fmt.Sprint(s), fmt.Sprintf("%+v", s), fmt.Sprintf("%#v", s)} {
		if strings.Contains(out, "NGFW_TEST_PSK_FPKI_material") {
			t.Fatalf("material formatted: %s", out)
		}
	}
}
