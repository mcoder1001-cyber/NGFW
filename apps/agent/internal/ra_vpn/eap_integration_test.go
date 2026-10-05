package ravpn

import (
	"context"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/strongswan/govici/vici"
	"google.golang.org/protobuf/encoding/protojson"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
)

func privateMessage(values ...any) *vici.Message {
	m := vici.NewMessage()
	for i := 0; i < len(values); i += 2 {
		if err := m.Set(values[i].(string), values[i+1]); err != nil {
			panic(err)
		}
	}
	return m
}
func privateIP(t *testing.T, plan *NetworkPlan, args ...string) {
	t.Helper()
	argv := append([]string{"--net=" + filepath.Join(InstanceRoot, plan.Instance, "netns"), "--", "/usr/sbin/ip"}, args...)
	if exec.Command("/usr/bin/nsenter", argv...).Run() != nil {
		t.Fatal("owned namespace link setup refused")
	}
}
func createEAPNamespace(t *testing.T, plan *NetworkPlan) {
	t.Helper()
	if err := CreateNamespace(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveNamespace(plan.Instance, plan.NamespaceInode); err != nil {
			t.Error(err)
			return
		}
		if err := os.RemoveAll(filepath.Join(InstanceRoot, plan.Instance)); err != nil {
			t.Error(err)
		}
	})
}
func TestIntegrationPrivateEAPNegotiationAndObservedDisconnect(t *testing.T) {
	runPrivateEAP(t, false)
}

func runPrivateEAP(t *testing.T, privateVPP bool) { runPrivateEAPWithCertificate(t, privateVPP, "") }
func runPrivateEAPWithCertificate(t *testing.T, privateVPP bool, certificateCase string) {
	if os.Getenv("NGFW_INTEGRATION") != "1" || os.Getenv("NGFW_RA_ENGINE_ROOT") == "" {
		t.Skip("requires authenticated isolated engine artifact")
	}
	server := networkFixture()
	server.Owner = "w19-eap-server"
	server.Profile = strconv.Itoa(os.Getpid())
	server.Instance = InstanceID(server.Owner, server.Profile)
	server.Radius = nil
	client := networkFixture()
	client.Owner = "w19-eap-client"
	client.Profile = server.Profile
	client.Instance = InstanceID(client.Owner, client.Profile)
	client.LocalAddress = "192.0.2.20"
	client.Outer = Link{VPP: server.Outer.Namespace, Namespace: server.Outer.VPP}
	client.Inner = Link{VPP: "198.18.19.4/31", Namespace: "198.18.19.5/31"}
	client.Radius = nil
	if privateVPP {
		client.Outer = Link{VPP: "198.18.19.6/31", Namespace: "198.18.19.7/31"}
	}
	createEAPNamespace(t, server)
	createEAPNamespace(t, client)
	if privateVPP {
		setupPrivateVPPTransport(t, server, client)
	} else {
		// Both veth endpoints originate in the verified server namespace. The peer
		// moves directly into the held client namespace; no host interface is created.
		privateIP(t, server, "link", "add", "outer0", "type", "veth", "peer", "name", "outer0", "netns", filepath.Join(InstanceRoot, client.Instance, "netns"))
		for _, plan := range []*NetworkPlan{server, client} {
			privateIP(t, plan, "address", "add", plan.Outer.Namespace, "dev", "outer0")
			privateIP(t, plan, "link", "add", "inner0", "type", "dummy")
			privateIP(t, plan, "address", "add", plan.Inner.Namespace, "dev", "inner0")
		}
	}
	profile := new(ngfwv1.RemoteAccessProfile)
	proposal := new(ngfwv1.IpsecProposal)
	if protojson.Unmarshal([]byte(`{"localAddr":"192.0.2.19","localId":"vpn.example.test","auth":"eap-mschapv2","certificate":"server","pools":[{"name":"clients","prefix":"10.19.200.0/24","dns":["10.19.0.53"]}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}]}`), profile) != nil || protojson.Unmarshal([]byte(`{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}`), proposal) != nil {
		t.Fatal("profile fixture")
	}
	credentials, now := credentialsFixture(t)
	var clientCertificate, clientPrivateKey []byte
	if certificateCase != "" && certificateCase != "bad-password" {
		auth, caName := "eap-tls", "clients"
		profile.Auth = &auth
		profile.ClientCa = &caName
		profile.Users = nil
		credentials, clientCertificate, clientPrivateKey, now = tlsCredentialsFixture(t, certificateCase)
	} else {
		credentials.ClientCA = nil
		credentials.ClientCRL = nil
	}
	var serverFiles *strongswan.RAFiles
	for _, plan := range []*NetworkPlan{server, client} {
		dir := filepath.Join(InstanceRoot, plan.Instance)
		files, err := strongswan.BuildRAFiles(context.Background(), "road", profile, proposal, dir, strongswan.SecretResolverFunc(func(context.Context, string) ([]byte, error) { return []byte("NGFW_TEST_PASSWORD_RA19"), nil }))
		if err != nil {
			t.Fatal(err)
		}
		files.Daemon = []byte(strings.Replace(string(files.Daemon), "journal {", "filelog { fixture { path = "+filepath.Join(dir, "daemon/fixture.log")+"\n default = 1\n flush_line = yes\n } }\n journal {", 1))
		if err := WriteSnapshot(plan.Instance, PrivateSnapshot{Daemon: files.Daemon, Connection: files.Connection, Secrets: files.Secrets, Credentials: credentials, CertificateName: "server", Identity: "vpn.example.test", CertificateClients: certificateCase != "" && certificateCase != "bad-password", ClientCAName: profile.GetClientCa()}, now); err != nil {
			t.Fatal(err)
		}
		if plan == server {
			serverFiles = files
		}
	}
	serverVICI := startPrivateEngine(t, server)

	clientVICI := startPrivateEngine(t, client)
	t.Cleanup(func() {
		if privateVPP && t.Failed() {
			collectPrivatePacketDiagnostics(t, server)
			collectPrivatePacketDiagnostics(t, client)
		}
	})
	serverMaterial := strongswan.RAMaterial{Certificates: map[string][]byte{filepath.Join(InstanceRoot, server.Instance, "x509/server.pem"): credentials.Certificate}, PrivateKey: credentials.PrivateKey, CRL: credentials.ClientCRL}
	if certificateCase != "" && certificateCase != "bad-password" {
		serverMaterial.Certificates[filepath.Join(InstanceRoot, server.Instance, "x509ca/clients.pem")] = credentials.ClientCA
	}
	if _, err := strongswan.LoadRA(context.Background(), serverVICI, serverFiles, serverMaterial); err != nil {
		t.Fatal("private responder load refused", err)
	}
	call := func(name string, message *vici.Message) {
		t.Helper()
		response, err := clientVICI.Call(context.Background(), name, message)
		if err != nil || response == nil || response.Get("success") != "yes" {
			t.Fatal("private client configuration refused", name)
		}
	}
	_, remaining := pem.Decode(credentials.Certificate)
	ca, _ := pem.Decode(remaining)
	if ca == nil {
		t.Fatal("fixture CA absent")
	}
	call("load-cert", privateMessage("type", "X509", "flag", "CA", "data", string(ca.Bytes)))
	localAuth := privateMessage("auth", "eap-mschapv2", "id", "client", "eap_id", "client")
	if certificateCase != "" && certificateCase != "bad-password" {
		cert, _ := pem.Decode(clientCertificate)
		key, _ := pem.Decode(clientPrivateKey)
		if cert == nil || key == nil {
			t.Fatal("client certificate fixture")
		}
		call("load-cert", privateMessage("type", "X509", "flag", "NONE", "data", string(cert.Bytes)))
		call("load-key", privateMessage("type", "ANY", "data", string(key.Bytes)))
		localAuth = privateMessage("auth", "eap-tls", "id", "client", "eap_id", "client", "certs", []string{string(clientCertificate)})
	} else {
		password := "NGFW_TEST_PASSWORD_RA19"
		if certificateCase == "bad-password" {
			password = "NGFW_TEST_WRONG_PASSWORD_RA19"
		}
		call("load-shared", privateMessage("id", "client-eap", "type", "EAP", "owners", []string{"client"}, "data", password))
	}
	child := privateMessage("local_ts", []string{"dynamic"}, "remote_ts", []string{"10.19.0.0/16"}, "esp_proposals", []string{"aes256gcm16-ecp256"}, "if_id_in", "1", "if_id_out", "1", "set_mark_out", "1")
	conn := privateMessage("version", "2", "local_addrs", []string{client.LocalAddress}, "remote_addrs", []string{server.LocalAddress}, "vips", []string{"0.0.0.0"}, "proposals", []string{"aes256-sha256-prfsha256-ecp256"}, "local", localAuth, "remote", privateMessage("auth", "pubkey", "id", "vpn.example.test"), "children", privateMessage("protected", child))
	call("load-conn", privateMessage("ra-client", conn))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := clientVICI.Call(ctx, "initiate", privateMessage("child", "protected", "timeout", "20000"))
	if certificateCase == "revoked" || certificateCase == "foreign" || certificateCase == "bad-password" {
		if response == nil || response.Get("success") != "no" {
			t.Fatal("untrusted TLS client did not fail authentication")
		}
		raw, readErr := os.ReadFile(filepath.Join(InstanceRoot, server.Instance, "daemon/fixture.log"))
		proved := false
		markers := []string{"no trusted", "issuer certificate not found", "unable to get local issuer"}
		if certificateCase == "bad-password" {
			markers = []string{"EAP-MS-CHAPv2 verification failed", "EAP method EAP_MSCHAPV2 failed", "MSCHAPV2 verification failed"}
		}
		if certificateCase == "revoked" {
			markers = []string{"certificate was revoked", "certificate is revoked", "revoked"}
		}
		if readErr == nil && len(raw) < 1<<20 {
			for _, marker := range markers {
				if strings.Contains(string(raw), marker) {
					proved = true
					t.Log("bounded authentication rejection marker:", marker)
				}
			}
		}
		clear(raw)
		if !proved {
			t.Fatal("TLS rejection lacks bounded certificate trust evidence")
		}
		sessions, e := strongswan.ObserveRASessions(context.Background(), serverVICI, "road", "fixture-eap-generation", profile.GetPools())
		if e != nil || len(sessions) != 0 {
			t.Fatal("untrusted TLS client session remained", e)
		}
		t.Log("actual private authentication refused:", certificateCase)
		return
	}
	if err != nil || response == nil || response.Get("success") != "yes" {
		if response != nil {
			for _, diagnostic := range []string{"CHILD_SA config 'protected' not found", "establishing CHILD_SA 'protected' failed", "initiating CHILD_SA 'protected' failed", "initiation failed", "no config found", "timeout waiting for IKE_SA"} {
				if response.Get("errmsg") == diagnostic {
					t.Log("sanitized client failure:", diagnostic)
				}
			}
		}
		for _, plan := range []*NetworkPlan{server, client} {
			raw, readErr := os.ReadFile(filepath.Join(InstanceRoot, plan.Instance, "daemon/fixture.log"))
			if readErr == nil && len(raw) < 1<<20 {
				if evidence := os.Getenv("NGFW_RA_EVIDENCE_ROOT"); evidence != "" {
					os.WriteFile(filepath.Join(evidence, plan.Instance+"-eap.log"), raw, 0600)
				}
				for _, marker := range []string{"NO_PROPOSAL_CHOSEN", "AUTHENTICATION_FAILED", "no socket implementation", "Network is unreachable", "no acceptable proposal found", "no trusted RSA public key found", "no trusted ECDSA public key found", "certificate rejected", "EAP method not supported", "EAP_MSCHAPV2 failed", "no shared key found", "no EAP key found", "received EAP_FAILURE", "constraint check failed", "no private key found", "CHILD_SA", "IKE_SA"} {
					if strings.Contains(string(raw), marker) {
						t.Log("bounded private log marker:", plan.Owner, marker)
					}
				}
			}
		}
		t.Fatal("actual private EAP negotiation failed; diagnostics stay private")
	}
	sessions, err := strongswan.ObserveRASessions(context.Background(), serverVICI, "road", "fixture-eap-generation", profile.GetPools())
	if err != nil || len(sessions) != 1 {
		t.Fatal("negotiated EAP session readback failed", err)
	}
	if privateVPP {
		verifyPrivateVPPPackets(t, client, sessions[0].Addresses[0])
		// VICI connections are intentionally bounded to ten seconds per
		// operation. Packet capture runs longer; production also dials afresh.
		serverVICI = freshPrivateVICI(t, server)
	}
	if err := strongswan.DisconnectRASession(context.Background(), serverVICI, "road", "fixture-eap-generation", sessions[0].ID, profile.GetPools()); err != nil {
		t.Fatal("actual observed disconnect failed", err)
	}
	poolNames := make([]string, 0, len(profile.GetPools()))
	for _, pool := range profile.GetPools() {
		name, e := strongswan.ConnName(pool.GetName())
		if e != nil {
			t.Fatal(e)
		}
		poolNames = append(poolNames, name)
	}
	if err := strongswan.UnloadRA(context.Background(), serverVICI, "ra-road", poolNames); err != nil {
		t.Fatal("actual private connection/pool empty rollback readback", err)
	}
	t.Log("actual private", profile.GetAuth(), "VIP/session negotiation, observed disconnect, connection/pool empty rollback PASS")
}

func TestIntegrationPrivateEAPBadPasswordRefused(t *testing.T) {
	runPrivateEAPWithCertificate(t, false, "bad-password")
}
