package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"io"
	"math/big"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	ravpn "ngfw/agent/internal/ra_vpn"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

const productionRADoc = `{"vrfs":{"outer":{"id":19000},"inner":{"id":19001}},"interfaces":{"loop2436":{"enabled":true,"vrf":"inner","ipv4":["10.19.0.53/32"]},"loop2437":{"enabled":true,"vrf":"inner","ipv4":["10.19.0.54/32"]}},"acl":{"lists":{"outer-in":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4"}]},"outer-out":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4"}]},"inner-in":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4","source":{"kind":"prefix","prefix":"10.19.200.0/24"},"destination":{"kind":"prefix","prefix":"10.19.0.53/32"}}]},"inner-out":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4","source":{"kind":"prefix","prefix":"10.19.0.53/32"},"destination":{"kind":"prefix","prefix":"10.19.200.0/24"}}]}}},"vpn":{"ipsec":{"proposals":{"ra-proposal":{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}}},"pki":{"certificates":{"server":{"certificateRef":"cert/server","privateKeyRef":"key/server"}}},"remoteAccess":{"road":{"enabled":true,"localAddr":"192.0.2.19","localId":"vpn.example.test","vrf":"inner","underlayVrf":"outer","auth":"eap-mschapv2","certificate":"server","proposal":"ra-proposal","transport":{"outer":{"vpp":"198.18.19.0/31","namespace":"198.18.19.1/31"},"inner":{"vpp":"198.18.19.2/31","namespace":"198.18.19.3/31"}},"pools":[{"name":"clients","prefix":"10.19.200.0/24","dns":["10.19.0.53"]}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}],"outerPolicy":{"ingress":["outer-in"],"egress":["outer-out"]},"accessPolicy":{"ingress":["inner-in"],"egress":["inner-out"]}}}}}`

func productionRACredentials(t *testing.T) (ravpn.Credentials, time.Time) {
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
	return ravpn.Credentials{Certificate: append(encode("CERTIFICATE", leafDER), caPEM...), PrivateKey: encode("PRIVATE"+" KEY", keyDER), ClientCA: caPEM, ClientCRL: encode("X509 CRL", crlDER)}, now
}

// TestIntegrationCanonicalGuestRAActivation is a client-only driver for the
// independent disposable systemd guest. It never constructs a fake agent,
// supervisor, sealed store or handoff, and never starts a host service.
func TestIntegrationCanonicalGuestRAActivation(t *testing.T) {
	if os.Getenv("NGFW_RA_CANONICAL_GUEST") != "1" {
		t.Skip("requires independent disposable original-unit guest")
	}
	markerPath := "/run/ngfw-ra-guest-fixture"
	info, err := os.Lstat(markerPath)
	if err != nil {
		t.Fatal("independent guest marker absent")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || stat.Uid != 0 || stat.Nlink != 1 || info.Size() > 1024 {
		t.Fatal("independent guest marker boundary")
	}
	// #nosec G304 -- fixed guest-only marker path has just passed strict ownership/mode/link checks.
	held, err := os.OpenFile(markerPath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal("guest marker open")
	}
	current, statErr := held.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(held, 1025))
	closeErr := held.Close()
	if statErr != nil || !os.SameFile(info, current) || readErr != nil || closeErr != nil || len(raw) > 1024 {
		t.Fatal("guest marker held read")
	}
	var marker struct {
		Owner  string `json:"owner"`
		BootID string `json:"bootId"`
	}
	if json.Unmarshal(raw, &marker) != nil || marker.Owner != "ngfw-ra-independent-guest" {
		t.Fatal("foreign guest marker")
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || marker.BootID != strings.TrimSpace(string(boot)) {
		t.Fatal("stale guest generation")
	}
	init, err := os.Readlink("/proc/1/exe")
	if err != nil || (init != "/usr/lib/systemd/systemd" && init != "/lib/systemd/systemd") {
		t.Fatal("real guest manager absent")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	connection, err := grpc.NewClient("unix:///run/ngfw/agent.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal("canonical guest connection")
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error("guest client close")
		}
	})
	client := ngfwv1.NewDataplaneClient(connection)
	domains := []string{"vrfs", "interfaces", "acl", "vpn"}
	baseline, err := client.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "ngfw", Subsystems: domains})
	if err != nil || baseline.GetDesiredState() == nil {
		t.Fatal("canonical baseline readback")
	}
	if baseline.GetDesiredState().GetVpn() != nil && len(baseline.GetDesiredState().GetVpn().GetRemoteAccess()) != 0 {
		t.Fatal("guest already owns remote-access profiles")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("fixture random credential")
	}
	password := []byte(hex.EncodeToString(nonce[:]))
	defer clear(password)
	transaction := hex.EncodeToString(nonce[:8])
	desired := proto.Clone(baseline.GetDesiredState()).(*ngfwv1.DesiredState)
	fixture := doc(t, productionRADoc)
	if !canonicalFixtureDisjoint(desired.ProtoReflect(), fixture.ProtoReflect()) {
		t.Fatal("fixture would replace existing owned map object")
	}
	proto.Merge(desired, fixture)
	credentials, _ := productionRACredentials(t)
	defer clear(credentials.PrivateKey)
	apply := func(call context.Context, suffix string, state *ngfwv1.DesiredState, subsystems []string, secrets *ngfwv1.SecretBundle) {
		response, err := client.Apply(call, &ngfwv1.ApplyRequest{Owner: "ngfw", TxnId: "ra-guest-" + transaction + suffix, Subsystems: subsystems, DesiredState: state, SecretBundle: secrets})
		if err != nil || response.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Fatal("canonical guest Apply refused at " + suffix)
		}
	}
	changed := false
	t.Cleanup(func() {
		if !changed {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		response, err := client.Apply(cleanup, &ngfwv1.ApplyRequest{Owner: "ngfw", TxnId: "ra-guest-" + transaction + "-cleanup", Subsystems: domains, DesiredState: baseline.GetDesiredState()})
		if err != nil || response.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Error("canonical owned baseline rollback refused")
		}
	})
	changed = true
	apply(ctx, "-foundation", desired, []string{"vrfs", "interfaces", "acl"}, &ngfwv1.SecretBundle{Values: map[string][]byte{"cert/server": credentials.Certificate, "key/server": credentials.PrivateKey, "password/client": password}})
	capability, err := client.RemoteAccessCapabilities(ctx, &ngfwv1.RemoteAccessCapabilitiesRequest{Owner: "ngfw"})
	if err != nil || !capability.GetOperational() || capability.GetReason() != "" {
		t.Fatal("actual installed runtime readiness refused")
	}
	apply(ctx, "-enable", desired, []string{"vpn"}, nil)
	sessions, err := client.RemoteAccessSessions(ctx, &ngfwv1.RemoteAccessSessionsRequest{Owner: "ngfw", Profile: "road", Limit: 100})
	if err != nil || len(sessions.GetSessions()) != 0 {
		t.Fatal("actual fresh unit VICI session readback")
	}
	apply(ctx, "-normal-preserve", desired, []string{"vrfs", "interfaces", "acl"}, nil)
	apply(ctx, "-rollback", baseline.GetDesiredState(), domains, nil)
	changed = false
	observed, err := client.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "ngfw", Subsystems: domains})
	if err != nil || len(observed.GetDesiredState().GetVpn().GetRemoteAccess()) != 0 {
		t.Fatal("canonical rollback remote-access readback")
	}
	t.Log("canonical original-unit agent Apply, fresh VICI readback, normal Apply preservation and baseline rollback PASS; packet/restart campaign is separate")
}

func canonicalFixtureDisjoint(existing, addition protoreflect.Message) bool {
	safe := true
	addition.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if !existing.Has(field) {
			return true
		}
		if field.IsMap() {
			value.Map().Range(func(key protoreflect.MapKey, _ protoreflect.Value) bool {
				if existing.Get(field).Map().Has(key) {
					safe = false
				}
				return safe
			})
		} else if field.Kind() == protoreflect.MessageKind && !field.IsList() {
			safe = canonicalFixtureDisjoint(existing.Get(field).Message(), value.Message())
		}
		return safe
	})
	return safe
}

func TestCanonicalFixtureRefusesExistingOwnedMapObjects(t *testing.T) {
	fixture := doc(t, productionRADoc)
	if canonicalFixtureDisjoint(fixture.ProtoReflect(), fixture.ProtoReflect()) {
		t.Fatal("fixture accepted replacement of existing map objects")
	}
	if !canonicalFixtureDisjoint((&ngfwv1.DesiredState{}).ProtoReflect(), fixture.ProtoReflect()) {
		t.Fatal("empty fixture baseline refused")
	}
}
