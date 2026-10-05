package ravpn

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	descacl "ngfw/agent/internal/descriptors/acl"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

func TestIntegrationPrivateVPPPolicyEAP(t *testing.T) {
	if os.Getenv("NGFW_RA_PRIVATE_VPP") != "1" {
		t.Skip("requires verified disposable VPP runner")
	}
	// The environment flag cannot authorize touching the shared VPP. Kernel
	// namespace and actual connected VPP PID must independently prove isolation.
	for _, kind := range []string{"net", "mnt"} {
		current, e1 := os.Stat("/proc/self/ns/" + kind)
		host, e2 := os.Stat("/proc/1/ns/" + kind)
		if e1 != nil || e2 != nil || current.Sys().(*syscall.Stat_t).Ino == host.Sys().(*syscall.Stat_t).Ino {
			t.Fatal("disposable VPP namespace boundary absent")
		}
	}
	runPrivateEAP(t, true)
}
func privateCLI(t *testing.T, args ...string) {
	t.Helper()
	argv := append([]string{"-s", "/run/vpp/cli.sock"}, args...)
	// #nosec G204 -- fixed CLI socket belongs to the verified disposable VPP namespace; arguments are compile-time fixture operations.
	if exec.Command("/usr/bin/vppctl", argv...).Run() != nil {
		t.Fatal("own disposable VPP command refused")
	}
}
func setupPrivateVPPTransport(t *testing.T, server, client *NetworkPlan) {
	t.Helper()
	ctx := context.Background()
	connection := vpp.Dial("/run/vpp/api.sock", vpp.ConnOptions{ReplyTimeout: 5 * time.Second})
	t.Cleanup(func() { connection.Close() })
	for deadline := time.Now().Add(5 * time.Second); !connection.Connected() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if !connection.Connected() {
		t.Fatal("own disposable VPP API unavailable")
	}
	boot, err := bootid.Current(ctx, connection)
	if err != nil || !boot.Complete() || boot.PID <= 0 {
		t.Fatal("own disposable VPP boot identity missing")
	}
	processNS, err := os.Stat("/proc/" + strconv.Itoa(boot.PID) + "/ns/net")
	currentNS, e := os.Stat("/proc/self/ns/net")
	if err != nil || e != nil || processNS.Sys().(*syscall.Stat_t).Ino != currentNS.Sys().(*syscall.Stat_t).Ino {
		t.Fatal("connected VPP is outside own disposable namespace")
	}
	claims, err := os.MkdirTemp("/run", "ngfw-ra-vpp-claims-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(claims); err != nil {
			t.Error("own VPP claims cleanup refused")
		}
	})
	privateCLI(t, "ip", "table", "add", "19000")
	privateCLI(t, "ip", "table", "add", "19001")
	var innerName string
	outerNames := map[string]string{}
	for p, plan := range []*NetworkPlan{server, client} {
		backend := tapv2.New(connection, plan.Owner)
		guard := &GuardedTAP{Tap: backend, Store: &LazyTAPReceipts{StateDir: claims}, Boot: func() bootid.Identity {
			identity, _ := bootid.Current(context.Background(), connection)
			return identity
		}, Plan: ReadAgentPlanByNamespace, AllowedID: func(id uint32) bool { return id >= 2432 && id <= 2435 }}
		outer, inner, err := TransitTAPs(plan, uint32(2432+p*2), uint32(2433+p*2))
		if err != nil {
			t.Fatal(err)
		}
		for n, endpoint := range []*tapv2.Tap{outer, inner} {
			meta, err := guard.Create(ctx, endpoint)
			if err != nil {
				t.Fatal("actual guarded TAP create/readback refused", err)
			}
			t.Cleanup(func() {
				if err := guard.Delete(context.Background(), endpoint, meta); err != nil {
					t.Error("actual owned TAP cleanup refused", err)
				}
			})
			table, err := iface.Dump(ctx, connection, plan.Owner)
			if err != nil {
				t.Fatal(err)
			}
			receipt := meta.(TAPReceipt)
			name := table.VPPName(receipt.Index)
			if name == "" {
				t.Fatal("actual TAP name missing")
			}
			vrf, prefix := "19000", plan.Outer.VPP
			if n == 1 {
				vrf, prefix = "19001", plan.Inner.VPP
			}
			privateCLI(t, "set", "interface", "ip", "table", name, vrf)
			privateCLI(t, "set", "interface", "ip", "address", name, prefix)
			privateCLI(t, "set", "interface", "state", name, "up")
			if n == 0 {
				outerNames[plan.Owner] = name
			}
			if p == 0 && n == 1 {
				innerName = name
			}
			lists := []descacl.ACL{{Name: "outer", Rules: []descacl.Rule{
				{Action: descacl.ActionPermit, Src: "192.0.2.0/24", Dst: "192.0.2.0/24", Proto: 17, SrcPortLast: 65535, DstPortFirst: 500, DstPortLast: 500},
				{Action: descacl.ActionPermit, Src: "192.0.2.0/24", Dst: "192.0.2.0/24", Proto: 17, SrcPortLast: 65535, DstPortFirst: 4500, DstPortLast: 4500},
				{Action: descacl.ActionPermit, Src: "192.0.2.0/24", Dst: "192.0.2.0/24", Proto: 50},
			}}}
			input, output := []string{"outer"}, []string{"outer"}
			if n == 1 {
				lists = []descacl.ACL{
					{Name: "inner-in", Rules: []descacl.Rule{{Action: descacl.ActionPermit, Src: "10.19.200.0/24", Dst: "10.19.0.53/32", Proto: 1, SrcPortLast: 255, DstPortLast: 255}}},
					{Name: "inner-out", Rules: []descacl.Rule{{Action: descacl.ActionPermit, Src: "10.19.0.53/32", Dst: "10.19.200.0/24", Proto: 1, SrcPortLast: 255, DstPortLast: 255}}},
				}
				input, output = []string{"inner-in"}, []string{"inner-out"}
			}
			// Per-profile list names may repeat across that profile's outer/inner
			// endpoints, so create only the distinct known list once per direction.
			aclDescriptor := descacl.NewACL(connection, plan.Owner)
			for _, list := range lists {
				desired := list.Proto()
				rows, err := aclDescriptor.Retrieve(ctx)
				if err != nil {
					t.Fatal(err)
				}
				exists := false
				for _, row := range rows {
					if row.Key == aclDescriptor.KeyOf(desired) {
						exists = true
						if !proto.Equal(row.Value, desired) {
							t.Fatal("foreign ACL content")
						}
					}
				}
				if !exists {
					aclMeta, err := aclDescriptor.Create(ctx, desired)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := aclDescriptor.Delete(context.Background(), desired, aclMeta); err != nil {
							t.Error(err)
						}
					})
				}
			}
			bindingDescriptor := descacl.NewInterfaceBinding(connection, plan.Owner)
			binding := descacl.InterfaceBinding{Interface: endpoint.Name, Input: input, Output: output}.Proto()
			bindingMeta, err := bindingDescriptor.Create(ctx, binding)
			if err != nil {
				t.Fatal("actual explicit ACL binding refused", err)
			}
			t.Cleanup(func() {
				if err := bindingDescriptor.Delete(context.Background(), binding, bindingMeta); err != nil {
					t.Error(err)
				}
			})
			rows, err := bindingDescriptor.Retrieve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			verified := false
			for _, row := range rows {
				if row.Key == bindingDescriptor.KeyOf(binding) && proto.Equal(row.Value, binding) {
					verified = true
				}
			}
			if !verified {
				t.Fatal("both ACL directions did not read back before daemon activation")
			}
		}
	}
	privateCLI(t, "ip", "route", "add", "192.0.2.19/32", "table", "19000", "via", "198.18.19.1", outerNames[server.Owner])
	privateCLI(t, "ip", "route", "add", "192.0.2.20/32", "table", "19000", "via", "198.18.19.7", outerNames[client.Owner])
	privateCLI(t, "ip", "route", "add", "10.19.200.0/24", "table", "19001", "via", "198.18.19.3", innerName)
	// A protected VPP local address supplies an actual ICMP responder in the
	// selected inner VRF; .54 is deliberately excluded by both explicit ACLs.
	privateCLI(t, "set", "interface", "ip", "address", innerName, "10.19.0.53/32")
	privateCLI(t, "set", "interface", "ip", "address", innerName, "10.19.0.54/32")
	t.Cleanup(func() {
		root := os.Getenv("NGFW_RA_VPP_RUNTIME")
		if root == "" {
			return
		}
		var file bytes.Buffer
		for _, command := range []string{"show interface", "show ip fib", "show ip neighbors", "show errors", "show acl-plugin interface"} {
			// #nosec G204 -- command is one of the fixed diagnostic allowlist entries; CLI is the verified disposable VPP socket and output is redacted.
			data, err := exec.Command("/usr/bin/vppctl", "-s", "/run/vpp/cli.sock", command).Output()
			if err == nil && len(data) <= 1<<20 {
				if _, err := file.WriteString(command + "\n"); err != nil {
					t.Error("own VPP diagnostic header write refused")
					return
				}
				if _, err := file.Write(data); err != nil {
					t.Error("own VPP diagnostic body write refused")
					return
				}
			}
		}
		if _, err := writePrivateFixtureEvidence(root, server.Instance, file.Bytes()); err != nil {
			t.Error("own VPP diagnostic publication refused")
		}
	})
}
func verifyPrivateVPPPackets(t *testing.T, client *NetworkPlan, vip string) {
	t.Helper()
	privateIP(t, client, "address", "add", vip+"/32", "dev", "lo")
	privateIP(t, client, "route", "replace", "10.19.0.0/16", "dev", "xfrm0")
	ns := filepath.Join(InstanceRoot, client.Instance, "netns")
	// This extra permit exists only in the disposable test client namespace.
	// The production responder retains its fixed failclosed firewall.
	// #nosec G204 -- fixed nft executable and literal owned disposable client policy; namespace and VIP come from the verified fixture.
	if exec.Command("/usr/bin/nsenter", "--net="+ns, "--", "/usr/sbin/nft", "add", "rule", "inet", "ngfw_ra", "output", "oifname", "xfrm0", "ip", "daddr", "10.19.0.0/16", "meta", "l4proto", "icmp", "accept").Run() != nil {
		t.Fatal("private client packet fixture policy refused")
	}
	// #nosec G204 -- fixed nft executable and literal owned disposable client policy; namespace and VIP come from the verified fixture.
	if exec.Command("/usr/bin/nsenter", "--net="+ns, "--", "/usr/sbin/nft", "insert", "rule", "inet", "ngfw_ra", "input", "iifname", "xfrm0", "ip", "daddr", vip, "meta", "l4proto", "icmp", "accept").Run() != nil {
		t.Fatal("private client encrypted reply fixture policy refused")
	}
	verifyWire := capturePrivateWire(t, client)
	ping := func(address string) error {
		// #nosec G204 -- fixed ping executable, verified fixture namespace/VIP, and literal private test destinations.
		return exec.Command("/usr/bin/nsenter", "--net="+ns, "--", "/usr/bin/ping", "-n", "-I", vip, "-c", "2", "-W", "2", address).Run()
	}
	if ping("10.19.0.53") != nil {
		t.Fatal("encrypted client packet/reply failed through selected VPP inner VRF and explicit ACL")
	}
	if ping("10.19.0.54") == nil {
		t.Fatal("explicit VPP ACL denial was bypassed")
	}
	verifyWire()
	t.Log("encrypted packet/reply through owned TAP + selected VPP VRF PASS; denied protected address remained unreachable")
}

var _ scheduler.Descriptor = (*GuardedTAP)(nil)
