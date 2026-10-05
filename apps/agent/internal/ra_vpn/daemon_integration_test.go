package ravpn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/encoding/protojson"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	if !trustedFixturePath(os.Getenv("NGFW_RA_ENGINE_ROOT"), true) || !trustedFixturePath(os.Getenv("NGFW_RA_HELPER"), false) {
		t.Fatal("private fixture artifact paths are not root-owned protected absolute paths")
	}
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
			// #nosec G204 -- fixed nsenter/ip binaries, validated full instance and newly owned namespace; integration-only root fixture.
			if exec.Command("/usr/bin/nsenter", args...).Run() != nil {
				t.Fatal("private dummy endpoint setup refused")
			}
		}
	}
	// #nosec G204 -- fixed nsenter/ip binaries; namespace path derives solely from validated instance owned by this fixture.
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
	client := startPrivateEngine(t, plan)
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

var privateEngineGenerations sync.Map

type privateEngineGeneration struct {
	PID   int
	Start uint64
}

func freshPrivateVICI(t *testing.T, plan *NetworkPlan) strongswan.ViciConn {
	t.Helper()
	value, ok := privateEngineGenerations.Load(plan.Instance)
	if !ok {
		t.Fatal("private engine generation absent")
	}
	generation := value.(privateEngineGeneration)
	stat, err := os.Stat("/proc/" + strconv.Itoa(generation.PID) + "/ns/net")
	if err != nil || stat.Sys().(*syscall.Stat_t).Ino != plan.NamespaceInode || (bootid.Reader{}).StartTime(generation.PID) != generation.Start {
		t.Fatal("private engine generation changed")
	}
	client, err := strongswan.DialRAVICI(context.Background(), filepath.Join(InstanceRoot, plan.Instance, "daemon/vici.sock"), generation.PID)
	if err != nil {
		t.Fatal("private engine redial refused")
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return client
}
func startPrivateEngine(t *testing.T, plan *NetworkPlan) strongswan.ViciConn {
	t.Helper()
	dir := filepath.Join(InstanceRoot, plan.Instance)
	workspace := t.TempDir()
	// #nosec G304 -- workspace is a newly generated test-private directory; fixed basename and exclusive creation refuse replacement.
	log, err := os.OpenFile(filepath.Join(workspace, "private-startup.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := log.Close(); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(workspace, "private-startup.log"))
		if err == nil && len(data) < 1<<20 {
			for _, line := range strings.Split(string(data), "\n") {
				if line == "RA_FIXTURE_PHASE=8" || strings.HasPrefix(line, "private RA fixture sandbox refused phase=") || strings.HasPrefix(line, "remote-access: isolated daemon startup refused (") {
					t.Log(line)
				}
			}
			if evidence := os.Getenv("NGFW_RA_EVIDENCE_ROOT"); evidence != "" {
				if path, err := writePrivateFixtureEvidence(evidence, plan.Instance, data); err != nil {
					t.Error("private fixture evidence refused", err)
				} else {
					t.Log("private startup evidence", path)
				}
			}
		}
	})
	mountNamespace, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mountNamespace.Close(); err != nil {
			t.Error(err)
		}
	})
	launcher, err := filepath.Abs("../../../../test/topology/ra-vpn/private-daemon.py")
	if err != nil {
		t.Fatal(err)
	}
	if !trustedFixturePath(launcher, false) || !trustedFixturePath(os.Getenv("NGFW_RA_ENGINE_ROOT"), true) || !trustedFixturePath(os.Getenv("NGFW_RA_HELPER"), false) || plan.Validate() != nil {
		t.Fatal("private fixture launch paths refused")
	}
	// #nosec G204 G702 -- fixed nsenter/unshare/python binaries; protected root-owned launcher/artifact/helper paths and validated owned namespace. The launcher checks pinned engine receipt before exec.
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
			if err := syscall.Kill(childPID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Error("owned child SIGTERM refused", err)
			}
		}
		select {
		case <-wait:
			return
		case <-time.After(3 * time.Second):
		}
		if childPID > 1 && startTime != 0 && (bootid.Reader{}).StartTime(childPID) == startTime {
			if err := syscall.Kill(childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				t.Error("owned child SIGKILL refused", err)
			}
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error("owned launcher kill refused", err)
		}
		select {
		case <-wait:
		case <-time.After(3 * time.Second):
			t.Error("own private fixture parent did not stop")
		}
	}
	t.Cleanup(cleanup)
	socket := filepath.Join(dir, "daemon", "vici.sock")
	ready := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if childPID == 0 {
			children, _ := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/task/" + strconv.Itoa(cmd.Process.Pid) + "/children")
			for _, candidate := range strings.Fields(string(children)) {
				pid, err := strconv.Atoi(candidate)
				if err != nil || pid <= 1 {
					continue
				}
				// #nosec G703 -- kernel children list parsed as integer >1 and re-encoded decimal; no path segment can contain traversal.
				stat, err := os.Stat("/proc/" + strconv.Itoa(pid) + "/ns/net")
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
	privateEngineGenerations.Store(plan.Instance, privateEngineGeneration{PID: childPID, Start: startTime})
	t.Cleanup(func() { privateEngineGenerations.Delete(plan.Instance) })
	client, err := strongswan.DialRAVICI(context.Background(), socket, childPID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	version, err := client.Call(context.Background(), "version", nil)
	if err != nil || version.Get("version") != "6.1.0" {
		t.Fatal("authenticated private engine version mismatch")
	}
	return client
}

// trustedFixturePath constrains integration-only environment paths before launch.
func trustedFixturePath(path string, directory bool) bool {
	if len(path) == 0 || len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	var info unix.Stat_t
	if unix.Lstat(path, &info) != nil || info.Uid != 0 || info.Mode&0022 != 0 {
		return false
	}
	if directory {
		return info.Mode&unix.S_IFMT == unix.S_IFDIR
	}
	return info.Mode&unix.S_IFMT == unix.S_IFREG && info.Nlink == 1
}

func writePrivateFixtureEvidence(root, instance string, data []byte) (evidencePath string, failure error) {
	if !trustedFixturePath(root, true) || !ValidInstance(instance) || len(data) > 1<<20 {
		return "", ErrBoundary
	}
	directory, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", ErrBoundary
	}
	defer func() { failure = errors.Join(failure, unix.Close(directory)) }()
	var stat unix.Stat_t
	if unix.Fstat(directory, &stat) != nil || stat.Uid != 0 || stat.Mode&0077 != 0 {
		return "", ErrBoundary
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	generation := "launch-" + hex.EncodeToString(random[:])
	if unix.Mkdirat(directory, generation, 0700) != nil {
		return "", ErrBoundary
	}
	owned, err := unix.Openat(directory, generation, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", ErrBoundary
	}
	defer func() { failure = errors.Join(failure, unix.Close(owned)) }()
	descriptor, err := unix.Openat(owned, instance+"-startup.log", unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return "", ErrBoundary
	}
	file := os.NewFile(uintptr(descriptor), "private fixture evidence")
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	return filepath.Join(root, generation, instance+"-startup.log"), errors.Join(writeErr, closeErr)
}

func TestPrivateFixturePathsAndEvidenceRefuseUnsafeOwnership(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "helper")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if trustedFixturePath("relative", false) || trustedFixturePath(file+"/../helper", false) || trustedFixturePath(file, true) {
		t.Fatal("unbounded or wrong-kind fixture path accepted")
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(file, alias); err != nil {
		t.Fatal(err)
	}
	if trustedFixturePath(alias, false) {
		t.Fatal("symlink fixture artifact accepted")
	}
	if os.Geteuid() != 0 {
		if trustedFixturePath(file, false) {
			t.Fatal("nonroot fixture artifact accepted")
		}
		return
	}
	if !trustedFixturePath(file, false) {
		t.Fatal("protected owned artifact refused")
	}
	private := filepath.Join(root, "evidence")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	instance := InstanceID("fixture", "evidence")
	first, err := writePrivateFixtureEvidence(private, instance, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := writePrivateFixtureEvidence(private, instance, []byte("replacement"))
	if err != nil || first == second {
		t.Fatal("per-launch evidence generation not isolated", err)
	}
	data, err := os.ReadFile(first)
	if err != nil || string(data) != "first" {
		t.Fatal("evidence ownership refusal lost original", err)
	}
}
