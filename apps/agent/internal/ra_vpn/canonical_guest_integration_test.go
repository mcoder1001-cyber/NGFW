package ravpn

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/strongswan/govici/vici"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"io"
	"math/big"
	"ngfw/agent/binapi/interface_types"
	ipapi "ngfw/agent/binapi/ip"
	tapapi "ngfw/agent/binapi/tapv2"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const productionRADoc = `{"vrfs":{"outer":{"id":19000},"inner":{"id":19001}},"interfaces":{"loop2436":{"enabled":true,"vrf":"inner","ipv4":["10.19.0.53/32"]},"loop2437":{"enabled":true,"vrf":"inner","ipv4":["10.19.0.54/32"]}},"acl":{"lists":{"outer-in":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4"}]},"outer-out":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4"}]},"inner-in":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4","source":{"kind":"prefix","prefix":"10.19.200.0/24"},"destination":{"kind":"prefix","prefix":"10.19.0.53/32"}}]},"inner-out":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4","source":{"kind":"prefix","prefix":"10.19.0.53/32"},"destination":{"kind":"prefix","prefix":"10.19.200.0/24"}}]}}},"vpn":{"ipsec":{"proposals":{"ra-proposal":{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}}},"pki":{"certificates":{"server":{"certificateRef":"cert/server","privateKeyRef":"key/server"}}},"remoteAccess":{"road":{"enabled":true,"localAddr":"192.0.2.19","localId":"vpn.example.test","vrf":"inner","underlayVrf":"outer","auth":"eap-mschapv2","certificate":"server","proposal":"ra-proposal","transport":{"outer":{"vpp":"198.18.19.0/31","namespace":"198.18.19.1/31"},"inner":{"vpp":"198.18.19.2/31","namespace":"198.18.19.3/31"}},"pools":[{"name":"clients","prefix":"10.19.200.0/24","dns":["10.19.0.53"]}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}],"outerPolicy":{"ingress":["outer-in"],"egress":["outer-out"]},"accessPolicy":{"ingress":["inner-in"],"egress":["inner-out"]}}}}}`

func productionRACredentials(t *testing.T) (Credentials, time.Time) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
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
	fixture := canonicalRADocument(t)
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
	peer, peerPlan, dnsOwnership := canonicalGuestPeer(t, desired.GetVpn().GetRemoteAccess()["road"], desired.GetVpn().GetIpsec().GetProposals()["ra-proposal"], credentials, password, transaction)
	initiate := func(success bool) {
		t.Helper()
		response, e := peer.Call(ctx, "initiate", privateMessage("child", "protected", "timeout", "20000"))
		if success && (e != nil || response == nil || response.Get("success") != "yes") {
			t.Fatal("canonical responder actual peer authentication failed")
		}
		if !success && (response == nil || response.Get("success") != "no") {
			t.Fatal("wrong password did not fail authentication")
		}
	}
	observe := func(want int) []*ngfwv1.RemoteAccessSession {
		t.Helper()
		for stop := time.Now().Add(8 * time.Second); time.Now().Before(stop); time.Sleep(50 * time.Millisecond) {
			rows, e := client.RemoteAccessSessions(ctx, &ngfwv1.RemoteAccessSessionsRequest{Owner: "ngfw", Profile: "road", Limit: 100})
			if e == nil && len(rows.GetSessions()) == want {
				return rows.GetSessions()
			}
		}
		t.Fatal("canonical observed session count did not converge")
		return nil
	}
	initiate(true)
	established := observe(1)
	if len(established[0].GetAddresses()) != 1 || !strings.HasPrefix(established[0].GetAddresses()[0], "10.19.200.") {
		t.Fatal("canonical pool VIP readback")
	}
	canonicalNegotiatedSelectors(t, peer, established[0].GetAddresses()[0])
	canonicalGuestDNS(t, peerPlan)
	verifyPrivateVPPPackets(t, peerPlan, established[0].GetAddresses()[0])
	apply(ctx, "-normal-preserve", desired, []string{"vrfs", "interfaces", "acl"}, nil)
	preserved := observe(1)
	if preserved[0].GetId() != established[0].GetId() {
		t.Fatal("ordinary Apply replaced active remote-access generation")
	}
	disconnected, e := client.RemoteAccessDisconnect(ctx, &ngfwv1.RemoteAccessDisconnectRequest{Owner: "ngfw", Profile: "road", Id: preserved[0].GetId()})
	if e != nil || !disconnected.GetDisconnected() {
		t.Fatal("canonical observed disconnect refused")
	}
	observe(0)
	peer = freshPrivateVICI(t, peerPlan)
	answer, e := peer.Call(ctx, "load-shared", privateMessage("id", "client-eap", "type", "EAP", "owners", []string{"client"}, "data", string(password)+"-wrong"))
	if e != nil || answer.Get("success") != "yes" {
		t.Fatal("negative peer credential load")
	}
	initiate(false)
	observe(0)
	answer, e = peer.Call(ctx, "load-shared", privateMessage("id", "client-eap", "type", "EAP", "owners", []string{"client"}, "data", string(password)))
	if e != nil || answer.Get("success") != "yes" {
		t.Fatal("peer credential restore")
	}
	initiate(true)
	observe(1)
	// This fixed restart is reachable only after the live independent guest marker
	// and PID1 systemd proof above. No Apply occurs after the agent restart.
	restartAt := time.Now()
	// #nosec G204 -- fixed command/unit inside the independently guarded disposable guest, no caller argv.
	if exec.CommandContext(ctx, "/usr/bin/systemctl", "restart", "ngfw-agent.service").Run() != nil {
		t.Fatal("canonical guest agent restart")
	}
	peer = freshPrivateVICI(t, peerPlan)
	initiate(true)
	observe(1)
	if elapsed := time.Since(restartAt); elapsed >= 30*time.Second {
		t.Fatal("canonical agent reconnect exceeded30s")
	}
	t.Log("canonical live EAP/VIP/selected FIB both ACL directions, ESP-only wire, wrong-password refusal, observed disconnect and agent reconnect under30s PASS")
	for _, scenario := range []string{"valid", "revoked", "foreign"} {
		tlsCredentials, certificate, key, _ := tlsCredentialsFixture(t, scenario)
		canonicalValidateTLSFixture(t, scenario, tlsCredentials, certificate, key)
		tlsDesired := proto.Clone(desired).(*ngfwv1.DesiredState)
		tlsProfile := tlsDesired.GetVpn().GetRemoteAccess()["road"]
		method, caName, caRef := "eap-tls", "clients", "cert/clients"
		tlsProfile.Auth = &method
		tlsProfile.ClientCa = &caName
		tlsProfile.Users = nil
		if tlsDesired.GetVpn().GetPki().Cas == nil {
			tlsDesired.GetVpn().GetPki().Cas = map[string]*ngfwv1.PkiCa{}
		}
		tlsDesired.GetVpn().GetPki().Cas[caName] = &ngfwv1.PkiCa{CertificateRef: &caRef}
		apply(ctx, "-tls-"+scenario, tlsDesired, []string{"vpn"}, &ngfwv1.SecretBundle{Values: map[string][]byte{"cert/server": tlsCredentials.Certificate, "key/server": tlsCredentials.PrivateKey, "cert/clients": tlsCredentials.ClientCA, "cert/clients.crl": tlsCredentials.ClientCRL}})
		observe(0)
		peer = freshPrivateVICI(t, peerPlan)
		_, _ = peer.Call(ctx, "terminate", privateMessage("ike", "ra-client", "timeout", "20000"))
		canonicalConfigureTLSClient(t, peer, peerPlan, tlsProfile, tlsCredentials, certificate, key)
		clear(key)
		clear(tlsCredentials.PrivateKey)
		if scenario == "valid" {
			initiate(true)
			tlsSessions := observe(1)
			canonicalNegotiatedSelectors(t, peer, tlsSessions[0].GetAddresses()[0])
			canonicalGuestDNS(t, peerPlan)
			removed, e := client.RemoteAccessDisconnect(ctx, &ngfwv1.RemoteAccessDisconnectRequest{Owner: "ngfw", Profile: "road", Id: tlsSessions[0].GetId()})
			if e != nil || !removed.GetDisconnected() {
				t.Fatal("actual TLS observed disconnect")
			}
			observe(0)
		} else {
			initiate(false)
			observe(0)
		}
		t.Log("canonical actual EAP-TLS case:", scenario)
	}

	dnsOwnership.Capture(t)
	physical := canonicalGuestTransportReadback(t)
	vpnRollback := proto.Clone(desired).(*ngfwv1.DesiredState)
	vpnRollback.Vpn = nil
	if baseline.GetDesiredState().GetVpn() != nil {
		vpnRollback.Vpn = proto.Clone(baseline.GetDesiredState().GetVpn()).(*ngfwv1.VpnConfig)
	}
	apply(ctx, "-vpn-rollback", vpnRollback, []string{"vpn"}, nil)
	canonicalGuestTransportRollback(t, physical, peerPlan)
	apply(ctx, "-rollback", baseline.GetDesiredState(), domains, nil)
	canonicalGuestUnitInactive(t)
	changed = false
	observed, err := client.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "ngfw", Subsystems: domains})
	if err != nil || observed.GetDesiredState() == nil || !proto.Equal(observed.GetDesiredState(), baseline.GetDesiredState()) {
		t.Fatal("canonical rollback full requested baseline readback")
	}
	t.Log("canonical original-unit agent Apply, fresh VICI readback, normal Apply preservation and baseline rollback PASS; actual packet and reconnect campaign executed")
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
	fixture := canonicalRADocument(t)
	if canonicalFixtureDisjoint(fixture.ProtoReflect(), fixture.ProtoReflect()) {
		t.Fatal("fixture accepted replacement of existing map objects")
	}
	if !canonicalFixtureDisjoint((&ngfwv1.DesiredState{}).ProtoReflect(), fixture.ProtoReflect()) {
		t.Fatal("empty fixture baseline refused")
	}
}

func canonicalRADocument(t *testing.T) *ngfwv1.DesiredState {
	t.Helper()
	state := new(ngfwv1.DesiredState)
	if protojson.Unmarshal([]byte(productionRADoc), state) != nil {
		t.Fatal("canonical profile fixture")
	}
	return state
}

func canonicalGuestPeer(t *testing.T, profile *ngfwv1.RemoteAccessProfile, proposal *ngfwv1.IpsecProposal, credentials Credentials, password []byte, generation string) (strongswan.ViciConn, *NetworkPlan, *canonicalDNSOwnership) {
	t.Helper()
	if profile == nil || proposal == nil || os.Getenv("NGFW_RA_ENGINE_ROOT") == "" || os.Getenv("NGFW_RA_HELPER") == "" {
		t.Fatal("authenticated peer fixture artifacts absent")
	}
	plan := networkFixture()
	plan.Owner = "w19-canonical-peer"
	plan.Profile = generation
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	plan.LocalAddress = "192.0.2.20"
	plan.Outer = Link{VPP: "198.18.19.6/31", Namespace: "198.18.19.7/31"}
	plan.Inner = Link{VPP: "198.18.19.4/31", Namespace: "198.18.19.5/31"}
	plan.Radius = nil
	if e := CreateNamespace(context.Background(), plan); e != nil {
		t.Fatal("private canonical peer namespace creation")
	}
	allowCleanup := true
	t.Cleanup(func() {
		if !allowCleanup {
			t.Error("retaining owned private peer namespace after DNS ownership refusal")
			return
		}
		if e := RemoveNamespace(plan.Instance, plan.NamespaceInode); e != nil {
			t.Error("private peer exact namespace cleanup")
			return
		}
		if e := os.RemoveAll(filepath.Join(InstanceRoot, plan.Instance)); e != nil {
			t.Error("private peer owned directory cleanup")
		}
	})
	connection := vpp.Dial("/run/vpp/api.sock", vpp.ConnOptions{ReplyTimeout: 5 * time.Second})
	t.Cleanup(func() { connection.Close() })
	for stop := time.Now().Add(5 * time.Second); !connection.Connected() && time.Now().Before(stop); {
		time.Sleep(10 * time.Millisecond)
	}
	identity, e := bootid.Current(context.Background(), connection)
	if e != nil || !identity.Complete() {
		t.Fatal("canonical guest VPP identity")
	}
	current, e := os.Stat("/proc/self/ns/net")
	process, pe := os.Stat("/proc/" + strconv.Itoa(identity.PID) + "/ns/net")
	if e != nil || pe != nil || !os.SameFile(current, process) {
		t.Fatal("peer TAP VPP outside independent guest namespace")
	}
	claims := t.TempDir()
	backend := tapv2.New(connection, plan.Owner)
	guard := &GuardedTAP{Tap: backend, Store: &LazyTAPReceipts{StateDir: claims}, Boot: func() bootid.Identity { b, _ := bootid.Current(context.Background(), connection); return b }, Plan: ReadAgentPlanByNamespace, AllowedID: func(id uint32) bool { return id == 8190 || id == 8191 }}
	outer, inner, e := TransitTAPs(plan, 8190, 8191)
	if e != nil {
		t.Fatal("peer TAP specification")
	}
	for n, endpoint := range []*tapv2.Tap{outer, inner} {
		metadata, e := guard.Create(context.Background(), endpoint)
		if e != nil {
			t.Fatal("peer exact guarded TAP creation")
		}
		t.Cleanup(func() {
			if e := guard.Delete(context.Background(), endpoint, metadata); e != nil {
				t.Error("peer exact TAP cleanup")
			}
		})
		table, e := iface.Dump(context.Background(), connection, plan.Owner)
		if e != nil {
			t.Fatal("peer TAP readback")
		}
		name := table.VPPName(metadata.(TAPReceipt).Index)
		if name == "" {
			t.Fatal("peer TAP actual name")
		}
		vrf, prefix := "19000", plan.Outer.VPP
		if n == 1 {
			vrf, prefix = "19001", plan.Inner.VPP
		}
		privateCLI(t, "set", "interface", "ip", "table", name, vrf)
		privateCLI(t, "set", "interface", "ip", "address", name, prefix)
		privateCLI(t, "set", "interface", "state", name, "up")
		if n == 0 {
			privateCLI(t, "ip", "route", "add", "192.0.2.20/32", "table", "19000", "via", "198.18.19.7", name)
			t.Cleanup(func() {
				privateCLI(t, "ip", "route", "del", "192.0.2.20/32", "table", "19000", "via", "198.18.19.7", name)
			})
		}
	}
	root := filepath.Join(InstanceRoot, plan.Instance)
	files, e := strongswan.BuildRAFiles(context.Background(), "road", profile, proposal, root, strongswan.SecretResolverFunc(func(context.Context, string) ([]byte, error) { return append([]byte(nil), password...), nil }))
	if e != nil {
		t.Fatal("peer daemon configuration")
	}
	// The disposable test peer must exercise both supported EAP methods using
	// the same authenticated engine. The production responder plugin list is unchanged.
	files.Daemon = []byte(strings.Replace(string(files.Daemon), "eap-mschapv2 md4", "eap-mschapv2 md4 eap-tls resolve", 1))
	if !trustedFixturePath(filepath.Join(os.Getenv("NGFW_RA_ENGINE_ROOT"), "lib/ipsec/plugins/libstrongswan-resolve.so"), false) {
		t.Fatal("authenticated DNS peer plugin absent")
	}
	resolvePath := filepath.Join(root, "daemon/client-resolv.conf")
	files.Daemon = []byte(strings.Replace(string(files.Daemon), "vici {", "resolve { file = "+resolvePath+"\n }\n vici {", 1))
	if e := WriteSnapshot(plan.Instance, PrivateSnapshot{Daemon: files.Daemon, Connection: files.Connection, Secrets: files.Secrets, Credentials: credentials, CertificateName: "server", Identity: "vpn.example.test"}, time.Now()); e != nil {
		t.Fatal("peer private snapshot")
	}
	// #nosec G304 -- exclusive creation at a fixed basename within the newly verified root-private owned client snapshot directory.
	resolver, e := os.OpenFile(resolvePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		t.Fatal("private client resolver creation")
	}
	if resolver.Close() != nil {
		t.Fatal("private client resolver close")
	}
	dnsOwnership := &canonicalDNSOwnership{path: resolvePath}
	dnsOwnership.Capture(t)
	// Registered before starting the client: its exact process kill/reap runs
	// first. Unknown/replaced resolver files are preserved, never added to the
	// production CleanupSnapshot allowlist.
	t.Cleanup(func() {
		if !dnsOwnership.Cleanup() {
			allowCleanup = false
			t.Error("private peer resolver cleanup ownership refused")
		}
	})
	peer := startPrivateEngine(t, plan)
	call := func(name string, message *vici.Message) {
		t.Helper()
		r, e := peer.Call(context.Background(), name, message)
		if e != nil || r == nil || r.Get("success") != "yes" {
			t.Fatal("peer fixed VICI configuration")
		}
	}
	_, remaining := pem.Decode(credentials.Certificate)
	ca, _ := pem.Decode(remaining)
	if ca == nil {
		t.Fatal("peer trusted server CA")
	}
	call("load-cert", privateMessage("type", "X509", "flag", "CA", "data", string(ca.Bytes)))
	call("load-shared", privateMessage("id", "client-eap", "type", "EAP", "owners", []string{"client"}, "data", string(password)))
	child := privateMessage("local_ts", []string{"dynamic"}, "remote_ts", []string{"10.19.0.0/16"}, "esp_proposals", []string{"aes256gcm16-ecp256"}, "if_id_in", "1", "if_id_out", "1", "set_mark_out", "1")
	config := privateMessage("version", "2", "local_addrs", []string{plan.LocalAddress}, "remote_addrs", []string{profile.GetLocalAddr()}, "vips", []string{"0.0.0.0"}, "proposals", []string{"aes256-sha256-prfsha256-ecp256"}, "local", privateMessage("auth", "eap-mschapv2", "id", "client", "eap_id", "client"), "remote", privateMessage("auth", "pubkey", "id", "vpn.example.test"), "children", privateMessage("protected", child))
	call("load-conn", privateMessage("ra-client", config))
	return peer, plan, dnsOwnership
}

func canonicalConfigureTLSClient(t *testing.T, peer strongswan.ViciConn, plan *NetworkPlan, profile *ngfwv1.RemoteAccessProfile, credentials Credentials, certificate, key []byte) {
	t.Helper()
	call := func(name string, message *vici.Message) {
		t.Helper()
		r, e := peer.Call(context.Background(), name, message)
		if e != nil || r == nil || r.Get("success") != "yes" {
			t.Fatal("canonical TLS peer configuration")
		}
	}
	_, remaining := pem.Decode(credentials.Certificate)
	ca, _ := pem.Decode(remaining)
	leaf, _ := pem.Decode(certificate)
	private, _ := pem.Decode(key)
	if ca == nil || leaf == nil || private == nil {
		t.Fatal("canonical TLS fixture certificate material")
	}
	call("load-cert", privateMessage("type", "X509", "flag", "CA", "data", string(ca.Bytes)))
	call("load-cert", privateMessage("type", "X509", "flag", "NONE", "data", string(leaf.Bytes)))
	call("load-key", privateMessage("type", "ANY", "data", string(private.Bytes)))
	child := privateMessage("local_ts", []string{"dynamic"}, "remote_ts", []string{"10.19.0.0/16"}, "esp_proposals", []string{"aes256gcm16-ecp256"}, "if_id_in", "1", "if_id_out", "1", "set_mark_out", "1")
	config := privateMessage("version", "2", "local_addrs", []string{plan.LocalAddress}, "remote_addrs", []string{profile.GetLocalAddr()}, "vips", []string{"0.0.0.0"}, "proposals", []string{"aes256-sha256-prfsha256-ecp256"}, "local", privateMessage("auth", "eap-tls", "id", "client", "eap_id", "client", "certs", []string{string(certificate)}), "remote", privateMessage("auth", "pubkey", "id", "vpn.example.test"), "children", privateMessage("protected", child))
	call("load-conn", privateMessage("ra-client", config))
}

func canonicalValidateTLSFixture(t *testing.T, scenario string, credentials Credentials, certificate, key []byte) {
	t.Helper()
	caBlock, _ := pem.Decode(credentials.ClientCA)
	leafBlock, _ := pem.Decode(certificate)
	keyBlock, _ := pem.Decode(key)
	crlBlock, _ := pem.Decode(credentials.ClientCRL)
	if caBlock == nil || leafBlock == nil || keyBlock == nil || crlBlock == nil {
		t.Fatal("TLS adversarial fixture material absent")
	}
	ca, e := x509.ParseCertificate(caBlock.Bytes)
	if e != nil {
		t.Fatal("TLS fixture CA parse")
	}
	leaf, e := x509.ParseCertificate(leafBlock.Bytes)
	if e != nil {
		t.Fatal("TLS fixture leaf parse")
	}
	private, e := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if e != nil {
		t.Fatal("TLS fixture key parse")
	}
	signer, ok := private.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("TLS fixture key type")
	}
	a, e := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if e != nil {
		t.Fatal("TLS leaf public key")
	}
	b, e := x509.MarshalPKIXPublicKey(&signer.PublicKey)
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("TLS negative must retain matching private key")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, chainErr := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	if (scenario == "foreign") != (chainErr != nil) {
		t.Fatal("TLS foreign/trusted fixture classification")
	}
	crl, e := x509.ParseRevocationList(crlBlock.Bytes)
	if e != nil || crl.CheckSignatureFrom(ca) != nil {
		t.Fatal("TLS fixture CRL signature")
	}
	revoked := false
	for _, entry := range crl.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
			revoked = true
		}
	}
	if revoked != (scenario == "revoked") {
		t.Fatal("TLS fixture revocation classification")
	}
}

func TestCanonicalTLSFixtureAdversaries(t *testing.T) {
	for _, scenario := range []string{"valid", "revoked", "foreign"} {
		t.Run(scenario, func(t *testing.T) {
			credentials, certificate, key, _ := tlsCredentialsFixture(t, scenario)
			defer clear(key)
			defer clear(credentials.PrivateKey)
			canonicalValidateTLSFixture(t, scenario, credentials, certificate, key)
		})
	}
}

func canonicalGuestUnitInactive(t *testing.T) {
	t.Helper()
	// #nosec G204 -- fixed readonly manager query for the one canonical test-owned unit, after independent guest proof.
	raw, e := exec.Command("/usr/bin/systemctl", "show", "--property=MainPID,ControlPID,ActiveState", "ngfw-ra@"+InstanceID("ngfw", "road")+".service").Output()
	if e != nil || len(raw) > 1024 {
		t.Fatal("canonical rollback unit readback")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatal("canonical unit readback shape")
		}
		fields[parts[0]] = parts[1]
	}
	if fields["MainPID"] != "0" || fields["ControlPID"] != "0" || fields["ActiveState"] != "inactive" {
		t.Fatal("canonical rollback retained an active daemon")
	}
}

func canonicalNegotiatedSelectors(t *testing.T, peer strongswan.ViciConn, vip string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	observed := 0
	for event, e := range peer.CallStreaming(ctx, "list-sas", "list-sa", privateMessage("ike", "ra-client", "noblock", "yes")) {
		if e != nil || event == nil {
			t.Fatal("canonical peer negotiated selector readback")
		}
		sa, ok := event.Get("ra-client").(*vici.Message)
		if !ok || sa.Get("state") != "ESTABLISHED" {
			t.Fatal("canonical peer established IKE readback")
		}
		children, ok := sa.Get("child-sas").(*vici.Message)
		if !ok {
			t.Fatal("canonical peer CHILD readback")
		}
		for _, name := range children.Keys() {
			child, ok := children.Get(name).(*vici.Message)
			if !ok || child.Get("state") != "INSTALLED" {
				t.Fatal("canonical installed CHILD readback")
			}
			remote, ok := child.Get("remote-ts").([]string)
			if !ok || len(remote) != 1 || remote[0] != "10.19.0.0/16" {
				t.Fatal("canonical negotiated split tunnel broadened")
			}
			local, ok := child.Get("local-ts").([]string)
			if !ok || len(local) != 1 || (local[0] != vip && local[0] != vip+"/32") {
				t.Fatal("canonical negotiated VIP selector differs")
			}
			observed++
		}
	}
	if observed != 1 {
		t.Fatal("canonical negotiated CHILD count differs")
	}
}

func canonicalGuestDNS(t *testing.T, plan *NetworkPlan) {
	t.Helper()
	path := filepath.Join(InstanceRoot, plan.Instance, "daemon/client-resolv.conf")
	artifact, e := openNumericPublisherArtifact(path, 4096, false)
	if e != nil {
		t.Fatal("private client DNS file boundary")
	}
	raw, e := io.ReadAll(io.LimitReader(artifact.file, 4097))
	closeErr := artifact.file.Close()
	if e != nil || closeErr != nil || len(raw) > 4096 {
		t.Fatal("bounded actual client DNS readback")
	}
	nameservers := 0
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if fields[0] == "nameserver" {
			if len(fields) != 2 || fields[1] != "10.19.0.53" {
				t.Fatal("negotiated DNS differs from configured pool")
			}
			nameservers++
		}
	}
	if nameservers != 1 {
		t.Fatal("actual negotiated DNS was not installed in private client")
	}
}

type canonicalDNSOwnership struct {
	path   string
	parent os.FileInfo
	file   os.FileInfo
}

func (r *canonicalDNSOwnership) Capture(t *testing.T) {
	t.Helper()
	parent, e := os.Lstat(filepath.Dir(r.path))
	if e != nil || !canonicalPrivateDNSInfo(parent, true) || (r.parent != nil && !os.SameFile(parent, r.parent)) {
		t.Fatal("private resolver directory identity")
	}
	r.parent = parent
	file, e := os.Lstat(r.path)
	if os.IsNotExist(e) {
		r.file = nil
		return
	}
	if e != nil || !canonicalPrivateDNSInfo(file, false) {
		t.Fatal("private resolver file identity")
	}
	r.file = file
}

func canonicalPrivateDNSInfo(info os.FileInfo, directory bool) bool {
	if info == nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0022 == 0 && ((directory && info.IsDir()) || (!directory && info.Mode().IsRegular() && stat.Nlink == 1))
}

func (r *canonicalDNSOwnership) Cleanup() bool {
	parent, e := os.Lstat(filepath.Dir(r.path))
	if e != nil || !canonicalPrivateDNSInfo(parent, true) || !os.SameFile(parent, r.parent) {
		return false
	}
	current, e := os.Lstat(r.path)
	if os.IsNotExist(e) {
		return true
	}
	if e != nil || r.file == nil || !canonicalPrivateDNSInfo(current, false) || !os.SameFile(current, r.file) {
		return false
	}
	// #nosec G304 -- exact captured root-private resolver inode and parent, client already killed/reaped; nofollow refuses any link replacement.
	held, e := os.OpenFile(r.path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return false
	}
	actual, e := held.Stat()
	closeErr := held.Close()
	if e != nil || closeErr != nil || !os.SameFile(actual, r.file) {
		return false
	}
	return os.Remove(r.path) == nil
}

func TestCanonicalDNSCleanupPreservesForeignReplacement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned private DNS fixture requires root")
	}
	for _, scenario := range []string{"owned", "replacement", "symlink", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "client-resolv.conf")
			if e := os.WriteFile(path, []byte("nameserver 10.19.0.53\n"), 0600); e != nil {
				t.Fatal(e)
			}
			receipt := &canonicalDNSOwnership{path: path}
			receipt.Capture(t)
			switch scenario {
			case "replacement":
				if e := os.Rename(path, path+".original"); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(path, []byte("foreign fixture marker\n"), 0600); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				if e := os.Rename(path, path+".original"); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(path+".original", path); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(path, path+".alias"); e != nil {
					t.Fatal(e)
				}
			}
			removed := receipt.Cleanup()
			if removed != (scenario == "owned") {
				t.Fatal("DNS cleanup ownership result differs")
			}
			_, e := os.Lstat(path)
			if scenario == "owned" {
				if !os.IsNotExist(e) {
					t.Fatal("owned DNS file remained")
				}
			} else if e != nil {
				t.Fatal("foreign DNS replacement was removed")
			}
		})
	}
}

// canonicalPhysicalTransport contains only bounded disposable-guest readback.
type canonicalPhysicalTransport struct {
	taps   map[uint32]tapapi.SwInterfaceTapV2Details
	routes map[uint32][]ipapi.IPRoute
}

func canonicalGuestTransportReadback(t *testing.T) canonicalPhysicalTransport {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection := vpp.Dial("/run/vpp/api.sock", vpp.ConnOptions{ReplyTimeout: 5 * time.Second})
	defer connection.Close()
	for stop := time.Now().Add(5 * time.Second); !connection.Connected() && time.Now().Before(stop); {
		time.Sleep(10 * time.Millisecond)
	}
	result := canonicalPhysicalTransport{taps: map[uint32]tapapi.SwInterfaceTapV2Details{}, routes: map[uint32][]ipapi.IPRoute{}}
	stream, err := tapapi.NewServiceClient(connection).SwInterfaceTapV2Dump(ctx, &tapapi.SwInterfaceTapV2Dump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		t.Fatal("canonical physical TAP dump")
	}
	for count := 0; ; count++ {
		row, e := stream.Recv()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil || row == nil || count > 8193 {
			t.Fatal("canonical physical TAP dump incomplete")
		}
		if _, duplicate := result.taps[row.SwIfIndex]; duplicate {
			t.Fatal("canonical physical TAP index duplicate")
		}
		result.taps[row.SwIfIndex] = *row
	}
	if err := stream.Close(); err != nil {
		t.Fatal("canonical physical TAP stream close")
	}
	for _, table := range []uint32{19000, 19001} {
		routes, e := ipapi.NewServiceClient(connection).IPRouteDump(ctx, &ipapi.IPRouteDump{Table: ipapi.IPTable{TableID: table}})
		if e != nil {
			t.Fatal("canonical physical FIB dump")
		}
		for count := 0; ; count++ {
			row, e := routes.Recv()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil || row == nil || count > 4096 {
				t.Fatal("canonical physical FIB dump incomplete")
			}
			result.routes[table] = append(result.routes[table], row.Route)
		}
		if err := routes.Close(); err != nil {
			t.Fatal("canonical physical FIB stream close")
		}
	}
	return result
}

func canonicalGuestTransportRollback(t *testing.T, before canonicalPhysicalTransport, peer *NetworkPlan) {
	t.Helper()
	after := canonicalGuestTransportReadback(t)
	serverNamespace := NamespacePath(InstanceID("ngfw", "road"))
	removed := map[uint32]bool{}
	peers := 0
	for index, row := range before.taps {
		if row.HostNamespace == serverNamespace {
			removed[index] = true
			if _, exists := after.taps[index]; exists {
				t.Fatal("canonical VPN rollback retained server TAP")
			}
		} else {
			observed, exists := after.taps[index]
			if !exists || observed != row {
				t.Fatal("canonical VPN rollback changed foreign TAP")
			}
			if row.HostNamespace == NamespacePath(peer.Instance) {
				peers++
			}
		}
	}
	if len(removed) != 2 || peers != 2 {
		t.Fatal("canonical rollback physical ownership proof absent")
	}
	for _, row := range after.taps {
		if row.HostNamespace == serverNamespace {
			t.Fatal("canonical rollback retained server endpoint")
		}
	}
	if _, err := os.Lstat(serverNamespace); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canonical rollback retained server namespace binding")
	}
	if _, err := ReadAgentPlan(peer.Instance); err != nil {
		t.Fatal("canonical VPN rollback removed foreign peer namespace")
	}
	foreignRoutes := 0
	for table, routes := range before.routes {
		for _, route := range routes {
			own, foreign := false, false
			for _, path := range route.Paths {
				if removed[path.SwIfIndex] {
					own = true
				}
				if row, exists := before.taps[path.SwIfIndex]; exists && row.HostNamespace == NamespacePath(peer.Instance) {
					foreign = true
				}
			}
			if foreign {
				foreignRoutes++
				found := false
				for _, observed := range after.routes[table] {
					if reflect.DeepEqual(route, observed) {
						found = true
					}
				}
				if !found {
					t.Fatal("canonical VPN rollback changed foreign peer route")
				}
			}
			if own {
				for _, observed := range after.routes[table] {
					for _, path := range observed.Paths {
						if removed[path.SwIfIndex] {
							t.Fatal("canonical VPN rollback retained server route")
						}
					}
				}
			}
		}
	}
	if foreignRoutes == 0 {
		t.Fatal("canonical rollback foreign route proof absent")
	}
}

type canonicalAPIHoldInput struct {
	Version        int    `json:"version"`
	Owner          string `json:"owner"`
	BootID         string `json:"bootId"`
	Generation     string `json:"generation"`
	CertificatePEM string `json:"certificatePem"`
	PrivateKeyPEM  string `json:"privateKeyPem"`
	Password       string `json:"password"`
}

func canonicalAPIPrivateRead(path string, limit int64) ([]byte, error) {
	// #nosec G304 -- callers supply only fixed guest fixture paths; held no-follow ownership and bounded reads are enforced below.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrBoundary
	}
	before, statErr := file.Stat()
	var raw []byte
	if statErr == nil {
		value, ok := before.Sys().(*syscall.Stat_t)
		if !ok || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || value.Uid != 0 || value.Nlink != 1 || before.Size() > limit {
			statErr = ErrBoundary
		} else {
			raw, err = io.ReadAll(io.LimitReader(file, limit+1))
		}
	}
	after, afterErr := file.Stat()
	closeErr := file.Close()
	current, currentErr := os.Lstat(path)
	if statErr != nil || err != nil || afterErr != nil || closeErr != nil || currentErr != nil || !os.SameFile(before, after) || !os.SameFile(after, current) || len(raw) > int(limit) || !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		clear(raw)
		return nil, ErrBoundary
	}
	return raw, nil
}

func canonicalAPIHoldValidate(input canonicalAPIHoldInput, boot string) error {
	generation, err := hex.DecodeString(input.Generation)
	if input.Version != 1 || input.Owner != "ngfw-ra-independent-guest" || input.BootID != boot || err != nil || len(generation) != 16 || input.Generation != strings.ToLower(input.Generation) || len(input.Password) < 16 || len(input.Password) > 256 || input.CertificatePEM == "" || input.PrivateKeyPEM == "" {
		return ErrBoundary
	}
	return nil
}

func TestCanonicalAPIHoldRejectsStalePrivateInput(t *testing.T) {
	input := canonicalAPIHoldInput{Version: 1, Owner: "ngfw-ra-independent-guest", BootID: "current", Generation: "00112233445566778899aabbccddeeff", CertificatePEM: "fixture", PrivateKeyPEM: "fixture", Password: strings.Repeat("x", 32)}
	if canonicalAPIHoldValidate(input, "current") != nil {
		t.Fatal("bound input refused")
	}
	for _, mutate := range []func(*canonicalAPIHoldInput){
		func(i *canonicalAPIHoldInput) { i.BootID = "old" },
		func(i *canonicalAPIHoldInput) { i.Generation = "../foreign" },
		func(i *canonicalAPIHoldInput) { i.Owner = "foreign" },
		func(i *canonicalAPIHoldInput) { i.Password = "short" },
		func(i *canonicalAPIHoldInput) { i.PrivateKeyPEM = "" },
	} {
		bad := input
		mutate(&bad)
		if canonicalAPIHoldValidate(bad, "current") == nil {
			t.Fatal("stale or foreign credential binding accepted")
		}
	}
}

func TestCanonicalAPIPrivateReadRefusesLinks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input")
	if os.WriteFile(path, []byte("private fixture"), 0600) != nil {
		t.Fatal("private input fixture")
	}
	link := filepath.Join(root, "alias")
	if os.Symlink(path, link) != nil {
		t.Fatal("symlink fixture")
	}
	if _, err := canonicalAPIPrivateRead(link, 1024); err == nil {
		t.Fatal("symlink input accepted")
	}
	if os.Remove(link) != nil || os.Link(path, link) != nil {
		t.Fatal("hard link fixture")
	}
	if _, err := canonicalAPIPrivateRead(path, 1024); err == nil {
		t.Fatal("input with a hard link accepted")
	}
}

// TestIntegrationCanonicalGuestRAAPIHold is a read-only canonical-agent client
// and genuine disposable EAP peer. API candidate commits deliver every server
// credential; this driver never calls Apply or opens the agent's sealed store.
func TestIntegrationCanonicalGuestRAAPIHold(t *testing.T) {
	if os.Getenv("NGFW_RA_CANONICAL_API_HOLD") != "1" {
		t.Skip("requires actual API-committed independent guest")
	}
	marker, err := canonicalAPIPrivateRead("/run/ngfw-ra-guest-fixture", 1024)
	if err != nil {
		t.Fatal("API peer guest marker")
	}
	var identity struct {
		Owner  string `json:"owner"`
		BootID string `json:"bootId"`
	}
	boot, bootErr := os.ReadFile("/proc/sys/kernel/random/boot_id")
	manager, managerErr := os.Readlink("/proc/1/exe")
	if json.Unmarshal(marker, &identity) != nil || identity.Owner != "ngfw-ra-independent-guest" || bootErr != nil || identity.BootID != strings.TrimSpace(string(boot)) || managerErr != nil || (manager != "/usr/lib/systemd/systemd" && manager != "/lib/systemd/systemd") {
		t.Fatal("API peer real guest identity")
	}
	raw, err := canonicalAPIPrivateRead("/run/ngfw-ra-api-peer.json", 262144)
	if err != nil {
		t.Fatal("API peer private input")
	}
	var input canonicalAPIHoldInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&input)
	var extra any
	endErr := decoder.Decode(&extra)
	clear(raw)
	if decodeErr != nil || endErr != io.EOF || canonicalAPIHoldValidate(input, identity.BootID) != nil {
		t.Fatal("API peer input binding")
	}
	password := []byte(input.Password)
	credentials := Credentials{Certificate: []byte(input.CertificatePEM), PrivateKey: []byte(input.PrivateKeyPEM)}
	input.Password, input.CertificatePEM, input.PrivateKeyPEM = "", "", ""
	t.Cleanup(func() { clear(password); clear(credentials.Certificate); clear(credentials.PrivateKey) })
	if VerifyCredentials(credentials, "vpn.example.test", false, time.Now()) != nil {
		t.Fatal("API peer certificate key chain")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
	defer cancel()
	connection, err := grpc.NewClient("unix:///run/ngfw/agent.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal("API peer canonical socket")
	}
	t.Cleanup(func() {
		if connection.Close() != nil {
			t.Error("API peer canonical socket close")
		}
	})
	client := ngfwv1.NewDataplaneClient(connection)
	capability, err := client.RemoteAccessCapabilities(ctx, &ngfwv1.RemoteAccessCapabilitiesRequest{Owner: "ngfw"})
	if err != nil || !capability.GetOperational() {
		t.Fatal("API peer runtime not ready")
	}
	actual, err := client.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "ngfw", Subsystems: []string{"vpn"}})
	var expected ngfwv1.DesiredState
	if err != nil || protojson.Unmarshal([]byte(productionRADoc), &expected) != nil {
		t.Fatal("API peer desired readback")
	}
	profile := actual.GetDesiredState().GetVpn().GetRemoteAccess()["road"]
	proposal := actual.GetDesiredState().GetVpn().GetIpsec().GetProposals()["ra-proposal"]
	if !proto.Equal(profile, expected.GetVpn().GetRemoteAccess()["road"]) || !proto.Equal(proposal, expected.GetVpn().GetIpsec().GetProposals()["ra-proposal"]) {
		t.Fatal("API peer canonical profile differs")
	}
	sequence := uint64(0)
	var prior string
	var statusIdentity os.FileInfo
	t.Cleanup(func() {
		if !t.Failed() {
			canonicalAPIHoldStatus(t, &statusIdentity, input.Generation, sequence+1, "stopped", prior)
		}
	})
	peer, plan, _ := canonicalGuestPeer(t, profile, proposal, credentials, password, "api-"+input.Generation)
	for ctx.Err() == nil {
		rows, readErr := client.RemoteAccessSessions(ctx, &ngfwv1.RemoteAccessSessionsRequest{Owner: "ngfw", Profile: "road", Limit: 100})
		if ctx.Err() != nil {
			break
		}
		if readErr != nil || len(rows.GetSessions()) > 1 {
			t.Fatal("API peer observed session boundary")
		}
		if len(rows.GetSessions()) == 0 {
			sequence++
			canonicalAPIHoldStatus(t, &statusIdentity, input.Generation, sequence, "disconnected", prior)
			attempt, stop := context.WithTimeout(ctx, 15*time.Second)
			if prior != "" {
				terminated, terminateErr := peer.Call(attempt, "terminate", privateMessage("ike", "ra-client", "timeout", "2000"))
				if terminateErr != nil || terminated == nil {
					stop()
					t.Fatal("API peer stale owned SA removal")
				}
			}
			result, initiateErr := peer.Call(attempt, "initiate", privateMessage("child", "protected", "timeout", "12000"))
			stop()
			if initiateErr != nil || result == nil || result.Get("success") != "yes" {
				t.Fatal("API peer genuine bounded reconnect")
			}
		} else {
			row := rows.GetSessions()[0]
			if row.GetId() != prior {
				sequence++
				canonicalAPIHoldStatus(t, &statusIdentity, input.Generation, sequence, "connected", row.GetId())
				prior = row.GetId()
			}
			if len(row.GetAddresses()) != 1 {
				t.Fatal("API peer observed VIP")
			}
			canonicalNegotiatedSelectors(t, peer, row.GetAddresses()[0])
			canonicalGuestDNS(t, plan)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func canonicalAPIHoldStatus(t *testing.T, owned *os.FileInfo, generation string, sequence uint64, state, session string) {
	t.Helper()
	// A fixed root-owned guest status file never contains credential or identity text.
	data, err := json.Marshal(struct {
		Generation string `json:"generation"`
		Sequence   uint64 `json:"sequence"`
		State      string `json:"state"`
		SessionID  string `json:"sessionId"`
	}{generation, sequence, state, session})
	if err != nil {
		t.Fatal("API peer status encoding")
	}
	path := "/run/ngfw-ra-api-peer-status.json"
	if previous, statErr := os.Lstat(path); statErr == nil {
		value, ok := previous.Sys().(*syscall.Stat_t)
		if !ok || !previous.Mode().IsRegular() || previous.Mode().Perm() != 0600 || value.Uid != 0 || value.Nlink != 1 || *owned == nil || !os.SameFile(previous, *owned) {
			t.Fatal("API peer status foreign boundary")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("API peer status readback")
	}
	file, err := os.OpenFile(path+".new", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		t.Fatal("API peer exclusive status")
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || os.Rename(path+".new", path) != nil {
		t.Fatal("API peer atomic status")
	}
	*owned, err = os.Lstat(path)
	if err != nil {
		t.Fatal("API peer status identity capture")
	}
}
