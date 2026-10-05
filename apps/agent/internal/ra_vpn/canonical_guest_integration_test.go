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
	"github.com/strongswan/govici/vici"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"io"
	"math/big"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"path/filepath"
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
	peer, peerPlan := canonicalGuestPeer(t, desired.GetVpn().GetRemoteAccess()["road"], desired.GetVpn().GetIpsec().GetProposals()["ra-proposal"], credentials, password, transaction)
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

	apply(ctx, "-rollback", baseline.GetDesiredState(), domains, nil)
	canonicalGuestUnitInactive(t)
	changed = false
	observed, err := client.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "ngfw", Subsystems: domains})
	if err != nil || len(observed.GetDesiredState().GetVpn().GetRemoteAccess()) != 0 {
		t.Fatal("canonical rollback remote-access readback")
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

func canonicalGuestPeer(t *testing.T, profile *ngfwv1.RemoteAccessProfile, proposal *ngfwv1.IpsecProposal, credentials Credentials, password []byte, generation string) (strongswan.ViciConn, *NetworkPlan) {
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
	createEAPNamespace(t, plan)
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
	files.Daemon = []byte(strings.Replace(string(files.Daemon), "eap-mschapv2 md4", "eap-mschapv2 md4 eap-tls", 1))
	if e := WriteSnapshot(plan.Instance, PrivateSnapshot{Daemon: files.Daemon, Connection: files.Connection, Secrets: files.Secrets, Credentials: credentials, CertificateName: "server", Identity: "vpn.example.test"}, time.Now()); e != nil {
		t.Fatal("peer private snapshot")
	}
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
	return peer, plan
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
