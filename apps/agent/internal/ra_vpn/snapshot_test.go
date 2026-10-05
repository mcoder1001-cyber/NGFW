package ravpn

import (
	"context"
	"net"
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
	// A replacement with the same permissions is not the recorded generation.
	original := filepath.Join(dir, "private/server.pem")
	backup := filepath.Join(dir, "held-key.pem")
	if os.Rename(original, backup) != nil || os.WriteFile(original, []byte("replacement"), 0600) != nil {
		t.Fatal("replacement fixture")
	}
	if CleanupSnapshot(plan.Instance) == nil {
		t.Fatal("replacement credential inode accepted")
	}
	if os.Remove(original) != nil || os.Rename(backup, original) != nil {
		t.Fatal("replacement fixture restore")
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "daemon/vici.sock"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if os.Chmod(filepath.Join(dir, "daemon/vici.sock"), 0600) != nil {
		t.Fatal("socket mode")
	}
	if CleanupSnapshot(plan.Instance) == nil {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("live daemon generation removed")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CleanupSnapshot(plan.Instance); err != nil {
		t.Fatal("inactive exact generation cleanup", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "network.json")); err != nil {
		t.Fatal("cleanup removed namespace ownership", err)
	}
	if err := WriteSnapshot(plan.Instance, snapshot, now); err != nil {
		t.Fatal("replacement generation", err)
	}
	if err := CleanupSnapshot(plan.Instance); err != nil {
		t.Fatal(err)
	}
}
