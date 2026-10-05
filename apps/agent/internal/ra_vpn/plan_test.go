package ravpn

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func networkFixture() *NetworkPlan {
	return &NetworkPlan{Format: 1, Instance: InstanceID("w19", "road"), LocalAddress: "192.0.2.19", Outer: Link{VPP: "198.18.19.0/31", Namespace: "198.18.19.1/31"}, Inner: Link{VPP: "198.18.19.2/31", Namespace: "198.18.19.3/31"}, Pools: []string{"10.19.200.0/24"}, Split: []string{"10.19.0.0/16"}, Radius: []RadiusEndpoint{{Address: "192.0.2.20", Port: 18120}}}
}
func TestIsolatedPlanRoutesMarkedOuterAndProtectedInner(t *testing.T) {
	plan := networkFixture()
	commands, err := plan.Commands(true, true)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"-4", "rule", "add", "priority", "100", "fwmark", "1", "lookup", "100"}, {"-4", "route", "replace", "table", "100", "default", "via", "198.18.19.0", "dev", "outer0"}, {"-4", "route", "replace", "10.19.0.0/16", "via", "198.18.19.2", "dev", "inner0"}, {"-4", "route", "replace", "10.19.200.0/24", "dev", "xfrm0"}, {"-4", "route", "replace", "192.0.2.20/32", "via", "198.18.19.0", "dev", "outer0", "src", "192.0.2.19"}}
	for _, required := range want {
		if !slices.ContainsFunc(commands, func(actual []string) bool { return slices.Equal(actual, required) }) {
			t.Fatalf("missing fixed route args %v", required)
		}
	}
}
func TestNamespaceFirewallPreventsDirectClientUnderlayAndEndpointBypass(t *testing.T) {
	plan := networkFixture()
	rules, err := plan.Firewall(false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rules)
	for _, required := range []string{"policy drop", "iifname \"xfrm0\" drop", "iifname \"xfrm0\" oifname \"inner0\" ip saddr 10.19.200.0/24 accept", "iifname \"inner0\" oifname \"xfrm0\" ip daddr 10.19.200.0/24 accept", "oifname \"outer0\" ip daddr 192.0.2.20 udp dport 18120 accept"} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing guard %s", required)
		}
	}
	if strings.Contains(text, "flush ruleset") || strings.Contains(text, "iifname \"xfrm0\" oifname \"outer0\"") {
		t.Fatal("namespace firewall exposes bypass")
	}
	sysctls, err := plan.Sysctls()
	if err != nil {
		t.Fatal(err)
	}
	for name := range sysctls {
		if !strings.HasPrefix(name, "net/") {
			t.Fatal("non-network tunable crosses boundary")
		}
	}
}
func TestNetworkPlanRefusesDangerousOrUnroutableInputs(t *testing.T) {
	mutations := []func(*NetworkPlan){func(p *NetworkPlan) { p.Instance = "../../foreign" }, func(p *NetworkPlan) { p.Outer.Namespace = p.Outer.VPP }, func(p *NetworkPlan) { p.Inner.Namespace = "198.18.19.1/31" }, func(p *NetworkPlan) { p.Pools = []string{"198.18.19.0/24"} }, func(p *NetworkPlan) { p.Pools = []string{"fd19::/64"} }, func(p *NetworkPlan) { p.Radius[0].Address = "radius.example.test" }, func(p *NetworkPlan) { p.Radius[0].Port = 65536 }, func(p *NetworkPlan) { p.LocalAddress = "127.0.0.1" }}
	for index, mutate := range mutations {
		plan := networkFixture()
		mutate(plan)
		if err := plan.Validate(); err == nil {
			t.Fatalf("mutation%d accepted", index)
		}
		if _, err := plan.Commands(true, true); err == nil {
			t.Fatal("invalid plan generated commands")
		}
	}
	encoded, err := json.Marshal(networkFixture())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") {
		t.Fatal("helper network plan has credential field")
	}
}

func TestPrivateNamespaceNFTParser(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("disposable network namespace parser check requires NGFW_INTEGRATION=1")
	}
	if os.Geteuid() != 0 {
		t.Fatal("selected namespace acceptance requires root")
	}
	modules, err := os.ReadFile("/proc/modules")
	if err != nil || !strings.Contains(string(modules), "nf_tables ") {
		t.Fatal("nf_tables must already be loaded; fixture never loads a host module")
	}
	rules, err := networkFixture().Firewall(false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "private.nft")
	if err := os.WriteFile(path, rules, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/unshare", "--net", "/usr/sbin/nft", "--check", "--file", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated nft parser failed: %v: %s", err, output)
	}
}
