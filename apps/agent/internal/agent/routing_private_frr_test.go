package agent

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type routingFRRIdentity struct{ start, netns string }

func routingFRRProcess(pid string) (routingFRRIdentity, error) {
	data, err := os.ReadFile("/proc/" + pid + "/stat") //nolint:gosec // G304: caller validates a positive decimal PID from the kernel cgroup member list.
	if err != nil {
		return routingFRRIdentity{}, err
	}
	end := strings.LastIndex(string(data), ")")
	if end < 0 {
		return routingFRRIdentity{}, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) <= 19 {
		return routingFRRIdentity{}, fmt.Errorf("short process stat")
	}
	ns, err := os.Readlink("/proc/" + pid + "/ns/net")
	return routingFRRIdentity{fields[19], ns}, err
}

// A system FRR in the original host namespace cannot sweep the independently
// isolated kernel table. Prove every service member remains there; flags alone
// never authorize an active service in the test namespace.
func requireForeignSystemFRR(t *testing.T) {
	t.Helper()
	current, err := os.Readlink("/proc/self/ns/net")
	host, hostErr := os.Readlink("/proc/1/ns/net")
	mount, mountErr := os.Readlink("/proc/self/ns/mnt")
	hostMount, hostMountErr := os.Readlink("/proc/1/ns/mnt")
	if err != nil || hostErr != nil || mountErr != nil || hostMountErr != nil || current == host || mount == hostMount || os.Getenv("NGFW_DISPOSABLE_VPP") != "1" {
		t.Fatal("active system FRR requires independently private network/mount namespaces")
	}
	owned := ""
	info, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(info), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 4 && fields[4] == "/run/vpp" {
			if owned != "" || strings.Contains(fields[3], "\\") {
				t.Fatal("ambiguous private VPP mount")
			}
			owned = filepath.Join(fields[3], "api.sock")
		}
	}
	scratch := filepath.Join(repoRootP12(t), ".scratch")
	relative, err := filepath.Rel(scratch, owned)
	if err != nil || !strings.HasPrefix(relative, "isolated-vpp-") || strings.Count(relative, string(os.PathSeparator)) != 1 || filepath.Base(relative) != "api.sock" {
		t.Fatal("active system FRR requires owned disposable VPP mount")
	}
	a, err := os.Stat(owned) //nolint:gosec // G703: kernel mount source is restricted to the owned scratch isolated-vpp runtime and exact api.sock basename.
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat("/run/vpp/api.sock")
	if err != nil || !os.SameFile(a, b) {
		t.Fatal("private VPP socket mapping mismatch")
	}
	shared, err := os.Stat("/proc/1/root/run/vpp/api.sock")
	if err != nil || os.SameFile(a, shared) {
		t.Fatal("shared VPP socket refused")
	}
	e := &p12Env{t: t}
	group, err := e.cmd("systemctl", "show", "frr", "-p", "ControlGroup", "--value")
	if err != nil {
		t.Fatal(err)
	}
	group = strings.TrimSpace(group)
	if !strings.HasPrefix(group, "/system.slice/") || filepath.Clean(group) != group {
		t.Fatal("system FRR cgroup refused")
	}
	pid, err := e.cmd("systemctl", "show", "frr", "-p", "MainPID", "--value")
	if err != nil {
		t.Fatal(err)
	}
	pid = strings.TrimSpace(pid)
	number, err := strconv.Atoi(pid)
	if err != nil || number <= 1 {
		t.Fatal("system FRR main process missing")
	}
	members := filepath.Join("/sys/fs/cgroup", group, "cgroup.procs")
	capture := func() (map[string]routingFRRIdentity, error) {
		data, err := os.ReadFile(members) //nolint:gosec // G304: root-owned system-manager cgroup is canonical and restricted to /system.slice/.
		if err != nil {
			return nil, err
		}
		result := map[string]routingFRRIdentity{}
		for _, member := range strings.Fields(string(data)) {
			n, err := strconv.Atoi(member)
			if err != nil || n <= 1 {
				return nil, fmt.Errorf("invalid FRR member")
			}
			identity, err := routingFRRProcess(member)
			if err != nil {
				return nil, err
			}
			result[member] = identity
		}
		return result, nil
	}
	before, err := capture()
	if err != nil {
		t.Fatal(err)
	}
	after, err := capture()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateForeignFRR(before, after, pid, host, current); err != nil {
		t.Fatal(err)
	}
	t.Logf("system FRR remains foreign: cgroup=%s main=%s members=%d host=%s private=%s", group, pid, len(before), host, current)
}

func validateForeignFRR(before, after map[string]routingFRRIdentity, main, host, current string) error {
	if len(before) == 0 || !maps.Equal(before, after) || host == "" || host == current {
		return fmt.Errorf("FRR membership/identity not stable and foreign")
	}
	if _, ok := before[main]; !ok {
		return fmt.Errorf("FRR main missing from unit membership")
	}
	for _, identity := range before {
		if identity.start == "" || identity.netns != host || identity.netns == current {
			return fmt.Errorf("FRR process is not in original host namespace")
		}
	}
	return nil
}

func TestRoutingForeignFRRMembership(t *testing.T) {
	valid := map[string]routingFRRIdentity{"10": {"1", "host"}, "11": {"2", "host"}}
	if err := validateForeignFRR(valid, maps.Clone(valid), "10", "host", "private"); err != nil {
		t.Fatal(err)
	}
	for name, after := range map[string]map[string]routingFRRIdentity{"empty": {}, "removed": {"10": {"1", "host"}}, "added": {"10": {"1", "host"}, "11": {"2", "host"}, "12": {"3", "host"}}, "reused": {"10": {"9", "host"}, "11": {"2", "host"}}, "private": {"10": {"1", "host"}, "11": {"2", "private"}}} {
		t.Run(name, func(t *testing.T) {
			if validateForeignFRR(valid, after, "10", "host", "private") == nil {
				t.Fatal("unstable/nonforeign unit accepted")
			}
		})
	}
	for name, identities := range map[string]map[string]routingFRRIdentity{
		"same-private-namespace": {"10": {"1", "private"}},
		"foreign-namespace":      {"10": {"1", "another"}},
		"missing-starttime":      {"10": {"", "host"}},
	} {
		t.Run(name, func(t *testing.T) {
			if validateForeignFRR(identities, maps.Clone(identities), "10", "host", "private") == nil {
				t.Fatal("non-host identity accepted")
			}
		})
	}
	if validateForeignFRR(valid, valid, "12", "host", "private") == nil || validateForeignFRR(valid, valid, "10", "host", "host") == nil {
		t.Fatal("missing main or original namespace accepted")
	}
}
