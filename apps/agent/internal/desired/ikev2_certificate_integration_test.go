package desired

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// This exercises the production sealed-cache/projection/scheduler connection,
// not peer negotiation. Global key writes are allowed only in disposable VPP.
func TestIKEv2NativeCertificateProduction(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" || os.Getenv("NGFW_DISPOSABLE_VPP") != "1" {
		t.Skip("requires disposable VPP")
	}
	binary := os.Getenv("NGFW_NATIVE_AGENT_BIN")
	if binary == "" {
		t.Fatal("production agent binary is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	leaf := func(name string) ([]byte, []byte) {
		t.Helper()
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	}
	localCert, localKey := leaf("local.test")
	peerCert, unusedPeerKey := leaf("remote.test")
	clear(unusedPeerKey)
	defer clear(localKey)
	ds := nativeDoc(t)
	ds.Vpn.Ipsec.Tunnels["site"].Auth = &ngfwv1.IpsecAuth{Method: proto.String("cert"), Certificate: proto.String("local"), PeerCertificate: proto.String("peer")}
	ds.Vpn.Pki = &ngfwv1.PkiConfig{Certificates: map[string]*ngfwv1.PkiCertificate{
		"local": {CertificateRef: proto.String("cert/local"), PrivateKeyRef: proto.String("key/local")},
		"peer":  {CertificateRef: proto.String("cert/peer")},
	}}
	values := map[string][]byte{"cert/local": localCert, "key/local": localKey, "cert/peer": peerCert}
	work := t.TempDir()
	socket := filepath.Join(work, "agent.sock")
	//nolint:gosec // Integration fixture creates only its private log file.
	logfile, err := os.Create(filepath.Join(work, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logfile.Close()
	t.Cleanup(func() {
		if t.Failed() {
			//nolint:gosec // Integration fixture reads only its own private log file.
			data, _ := os.ReadFile(filepath.Join(work, "agent.log"))
			t.Log(string(data))
		}
	})
	var process *exec.Cmd
	start := func() {
		t.Helper()
		//nolint:gosec // Explicit integration-only executable; no shell or user configuration.
		process = exec.CommandContext(ctx, binary)
		process.Env = append(os.Environ(), "NGFW_AGENT_SOCKET="+socket, "NGFW_OWNER=w8", "NGFW_GLOBALS_OWNER=1", "NGFW_AGENT_STATE_DIR="+filepath.Join(work, "state"), "NGFW_VPP_TABLE_BASE=8000", "NGFW_METRICS_ADDR=127.0.0.1:0", "NGFW_SOCKET_GROUP=root")
		process.Stdout, process.Stderr = logfile, logfile
		if err := process.Start(); err != nil {
			t.Fatal(err)
		}
	}
	stop := func() {
		t.Helper()
		if process != nil {
			_ = process.Process.Signal(syscall.SIGTERM)
			if err := process.Wait(); err != nil {
				t.Error(err)
			}
			process = nil
		}
	}
	start()
	defer stop()
	cc, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	client := ngfwv1.NewDataplaneClient(cc)
	ready := func() {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			if _, err := client.Health(ctx, &ngfwv1.HealthRequest{}); err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("production agent readiness timed out")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	ready()
	apply := func(txn string, state *ngfwv1.DesiredState, bundle map[string][]byte) {
		t.Helper()
		res, err := client.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: txn, DesiredState: state, Subsystems: []string{"tunnels", "vpn"}, SecretBundle: &ngfwv1.SecretBundle{Values: bundle}})
		if err != nil || res.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
			t.Fatalf("%s: err=%v result=%v", txn, err, res)
		}
	}
	show := func() string {
		t.Helper()
		//nolint:gosec // Fixed CLI arguments; sockets refer only to disposable VPP.
		out, err := exec.CommandContext(ctx, "vppctl", "-s", "/run/vpp/cli.sock", "show ikev2 profile").CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
		return string(out)
	}
	apply("cert-initial", ds, values)
	first := show()
	if !strings.Contains(first, "w8-site") || !strings.Contains(first, "rsa") {
		t.Fatal("native RSA profile missing", first)
	}
	got, err := client.Retrieve(ctx, &ngfwv1.RetrieveRequest{Owner: "w8", Subsystems: []string{"vpn"}})
	if err != nil || got.GetDesiredState().GetVpn().GetIpsec().GetTunnels()["site"].GetAuth().GetPeerCertificate() != "peer" {
		t.Fatal("configured peer reference did not survive live readback", err, got)
	}
	// Exact replay must converge with no secret material in returned desired state.
	apply("cert-replay", ds, values)
	rotatedCert, rotatedKey := leaf("local.test")
	defer clear(rotatedKey)
	rotated := map[string][]byte{"cert/local": rotatedCert, "key/local": rotatedKey, "cert/peer": peerCert}
	apply("cert-key-rotation", ds, rotated)
	second := show()
	if second == first {
		t.Fatal("local-key rotation did not replace peer snapshot generation")
	}
	apply("cert-revert-original", ds, values)
	if show() != first {
		t.Fatal("explicit revision revert did not restore original certificate profile")
	}
	stop()
	// Simulate loss of only this fixture's VPP profile and private snapshots.
	// Restart must recover from persisted desired state and sealed secrets,
	// without sending another Apply request.
	//nolint:gosec // Fixed CLI command targets only the disposable fixture's owned profile.
	if out, err := exec.CommandContext(ctx, "vppctl", "-s", "/run/vpp/cli.sock", "ikev2 profile del w8-site").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	files, err := filepath.Glob(filepath.Join(work, "state", "native-ikev2-w8", "*.pem"))
	if err != nil || len(files) == 0 {
		t.Fatal("native snapshot generations were not materialized", err)
	}
	for _, file := range files {
		//nolint:gosec // Only fixture-owned temporary snapshot files are removed.
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}
	start()
	ready()
	deadline := time.Now().Add(15 * time.Second)
	for show() != first {
		if time.Now().After(deadline) {
			t.Fatal("autonomous agent restart did not restore missing certificate profile")
		}
		time.Sleep(100 * time.Millisecond)
	}
	apply("cert-after-agent-restart", ds, values)
	if show() != first {
		t.Fatal("agent restart did not recover original certificate profile")
	}
	empty := proto.Clone(ds).(*ngfwv1.DesiredState)
	empty.Vpn.Ipsec.Tunnels = map[string]*ngfwv1.IpsecTunnel{}
	apply("cert-remove", empty, values)
	if strings.Contains(show(), "w8-site") {
		t.Fatal("removed certificate profile survived")
	}
	t.Log("production sealed certificate delivery, profile readback, replay, shared-key rotation, explicit revision revert, agent restart and profile removal passed; peer negotiation/packets not exercised")
}
