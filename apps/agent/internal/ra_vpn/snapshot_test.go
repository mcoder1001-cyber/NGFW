package ravpn

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIntegrationPrivateSnapshotExclusiveAndModes(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires exclusively owned namespace fixture")
	}
	plan := networkFixture()
	plan.Owner = "w19-snapshot"
	plan.Profile = strconv.Itoa(os.Getpid())
	plan.Instance = InstanceID(plan.Owner, plan.Profile)
	if err := CreateNamespace(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(InstanceRoot, plan.Instance)
	t.Cleanup(func() {
		if err := RemoveNamespace(plan.Instance, plan.NamespaceInode); err != nil {
			t.Error(err)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	credentials, now := credentialsFixture(t)
	snapshot := PrivateSnapshot{Daemon: []byte("private daemon config\n"), Connection: []byte("connections {}\n"), Secrets: []byte("private generated secrets\n"), Credentials: credentials, CertificateName: "server", ClientCAName: "clients", Identity: "vpn.example.test", CertificateClients: true}
	if err := WriteSnapshot(plan.Instance, snapshot, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"strongswan.conf", "swanctl.conf", "private/server.pem", "x509/server.pem", "x509ca/clients.pem", "x509crl/clients.pem"} {
		stat, err := os.Stat(filepath.Join(dir, name))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("snapshot permissions", name, err)
		}
	}
	snapshot.Secrets = []byte("changed private generated secrets\n")
	if WriteSnapshot(plan.Instance, snapshot, now) == nil {
		t.Fatal("active credential generation overwritten")
	}
	data, err := os.ReadFile(filepath.Join(dir, "swanctl.conf"))
	if err != nil || string(data) != "connections {}\nprivate generated secrets\n" {
		t.Fatal("refused overwrite altered existing generation")
	}
}
