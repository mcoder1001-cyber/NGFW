package ravpn

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/encoding/protojson"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
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

func TestIntegrationPrivateEngineLoadsProfileThroughVerifiedVICI(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" || os.Getenv("NGFW_RA_ENGINE_ROOT") == "" {
		t.Skip("requires authenticated private engine artifact fixture")
	}
	plan := networkFixture()
	plan.Owner = "w19-engine"
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
	for _, entry := range []struct{ name, prefix string }{{"outer0", plan.Outer.Namespace}, {"inner0", plan.Inner.Namespace}} {
		for _, arguments := range [][]string{{"link", "add", entry.name, "type", "dummy"}, {"address", "add", entry.prefix, "dev", entry.name}} {
			args := append([]string{"--net=" + filepath.Join(dir, "netns"), "--", "/usr/sbin/ip"}, arguments...)
			if exec.Command("/usr/bin/nsenter", args...).Run() != nil {
				t.Fatal("private dummy endpoint setup refused")
			}
		}
	}
	links, err := exec.Command("/usr/bin/nsenter", "--net="+filepath.Join(dir, "netns"), "--", "/usr/sbin/ip", "-j", "-d", "link", "show").Output()
	if err != nil || len(links) > 1<<20 {
		t.Fatal("private endpoint link readback failed")
	}
	var observed []struct {
		Name string `json:"ifname"`
		Info struct {
			Kind string `json:"info_kind"`
		} `json:"linkinfo"`
	}
	if json.Unmarshal(links, &observed) != nil {
		t.Fatal("private endpoint link readback decode failed")
	}
	for _, link := range observed {
		t.Logf("private fixture link name=%s kind=%s", link.Name, link.Info.Kind)
	}
	profile := new(ngfwv1.RemoteAccessProfile)
	if protojson.Unmarshal([]byte(`{"localAddr":"192.0.2.19","localId":"vpn.example.test","auth":"eap-mschapv2","certificate":"server","pools":[{"name":"clients","prefix":"10.19.200.0/24","dns":["10.19.0.53"]}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}]}`), profile) != nil {
		t.Fatal("profile fixture")
	}
	proposal := new(ngfwv1.IpsecProposal)
	if protojson.Unmarshal([]byte(`{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}`), proposal) != nil {
		t.Fatal("proposal fixture")
	}
	files, err := strongswan.BuildRAFiles(context.Background(), "road", profile, proposal, dir, strongswan.SecretResolverFunc(func(context.Context, string) ([]byte, error) { return []byte("NGFW_TEST_PASSWORD_RA19"), nil }))
	if err != nil {
		t.Fatal(err)
	}
	credentials, now := credentialsFixture(t)
	credentials.ClientCA = nil
	credentials.ClientCRL = nil
	if err = WriteSnapshot(plan.Instance, PrivateSnapshot{Daemon: files.Daemon, Connection: files.Connection, Secrets: files.Secrets, Credentials: credentials, CertificateName: "server", Identity: "vpn.example.test"}, now); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	log, err := os.OpenFile(filepath.Join(workspace, "private-startup.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(workspace, "private-startup.log"))
		if err == nil && len(data) < 1<<20 {
			for _, line := range strings.Split(string(data), "\n") {
				if line == "RA_FIXTURE_PHASE=8" || strings.HasPrefix(line, "private RA fixture sandbox refused phase=") || strings.HasPrefix(line, "remote-access: isolated daemon startup refused (") {
					t.Log(line)
				}
			}
			if evidence := os.Getenv("NGFW_RA_EVIDENCE_ROOT"); evidence != "" {
				if stat, err := os.Lstat(evidence); err == nil && stat.IsDir() && stat.Mode().Perm() == 0700 && stat.Sys().(*syscall.Stat_t).Uid == 0 {
					os.WriteFile(filepath.Join(evidence, plan.Instance+"-startup.log"), data, 0600)
				}
			}
		}
	})
	mountNamespace, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	defer mountNamespace.Close()
	launcher, err := filepath.Abs("../../../../test/topology/ra-vpn/private-daemon.py")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/nsenter", "--net="+filepath.Join(dir, "netns"), "--", "/usr/bin/unshare", "--mount", "--pid", "--fork", "--mount-proc", "/usr/bin/python3", launcher, plan.Instance, os.Getenv("NGFW_RA_ENGINE_ROOT"), os.Getenv("NGFW_RA_HELPER"), workspace)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	cmd.ExtraFiles = []*os.File{mountNamespace}
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		t.Fatal("private launcher start refused")
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	childPID := 0
	startTime := uint64(0)
	cleanup := func() {
		if childPID > 1 && startTime != 0 && (bootid.Reader{}).StartTime(childPID) == startTime {
			syscall.Kill(childPID, syscall.SIGTERM)
		}
		select {
		case <-wait:
			return
		case <-time.After(3 * time.Second):
		}
		if childPID > 1 && startTime != 0 && (bootid.Reader{}).StartTime(childPID) == startTime {
			syscall.Kill(childPID, syscall.SIGKILL)
		}
		cmd.Process.Kill()
		select {
		case <-wait:
		case <-time.After(3 * time.Second):
			t.Error("own private fixture parent did not stop")
		}
	}
	defer cleanup()
	socket := filepath.Join(dir, "daemon", "vici.sock")
	ready := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if childPID == 0 {
			children, _ := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/task/" + strconv.Itoa(cmd.Process.Pid) + "/children")
			for _, candidate := range strings.Fields(string(children)) {
				pid, _ := strconv.Atoi(candidate)
				stat, err := os.Stat("/proc/" + candidate + "/ns/net")
				if err == nil && pid > 1 && stat.Sys().(*syscall.Stat_t).Ino == plan.NamespaceInode {
					childPID = pid
					startTime = (bootid.Reader{}).StartTime(pid)
				}
			}
		}
		if childPID > 1 && startTime != 0 && strongswan.RestrictRAVICISocket(context.Background(), socket, childPID) == nil {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatal("private verified engine socket did not become ready; raw diagnostics remain private")
	}
	client, err := strongswan.DialRAVICI(context.Background(), socket, childPID)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	version, err := client.Call(context.Background(), "version", nil)
	if err != nil || version.Get("version") != "6.1.0" {
		t.Fatal("authenticated private engine version mismatch")
	}
	name, err := strongswan.LoadRA(context.Background(), client, files, strongswan.RAMaterial{Certificates: map[string][]byte{filepath.Join(dir, "x509/server.pem"): credentials.Certificate}, PrivateKey: credentials.PrivateKey})
	if err != nil || name != "ra-road" {
		t.Fatal("actual private engine profile load failed", err)
	}
	sessions, err := strongswan.ObserveRASessions(context.Background(), client, "road", "fixture-generation", profile.GetPools())
	if err != nil || len(sessions) != 0 {
		t.Fatal("actual empty-session readback failed", err)
	}
	t.Log("actual guarded private strongSwan6.1 profile load and VICI empty-session readback PASS; packet negotiation remains a separate acceptance")
}
