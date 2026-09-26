package agent

// F-lb host tests on the shared VPP — opt-in (every run leaves "removed" VIPs and the ASes' recursive /32s in table 0
// until a garbage collection or a VPP restart, V20; DF-7 precedent VRX_DF7_LB=1):
//
//	TestLbOnHost          VRX_INTEGRATION=1 VRX_LB_HOST=1 — in-process slot agent (not the globals owner): apply
//	                      services.lb → `show lb vips verbose` + LbState; flush; agent-restart simulation (re-applied
//	                      without duplicates, intf-nat applied once); simulated loss of a VIP while the agent is down
//	                      → re-created within 30 s; removal (delete messages; the VIPs listed as removed, V20).
//	TestLbGarbageCollectOnHost  + VRX_LB_GLOBALS=1 — VPP-global (D-082): flock -x /run/lock/vrx-globals.lock, manager
//	                      window only; one lb.GarbageCollect after the removal frees this slot's removed VIPs.
//
// Objects: VIPs 10.<slot>.250.0/24, application servers 10.<slot>.2.0/24, the NAT feature on the slot's loopback.
// Never show/trace commands; the evidence is `show lb vips verbose` / `show lb` (read-only) through cli_inband.

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"ngfw/agent/binapi/vlib"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df7"
	lbd "ngfw/agent/internal/descriptors/lb"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/vpptest"
)

func lbOptIn(t *testing.T) {
	t.Helper()
	vpptest.SkipUnlessIntegration(t)
	if os.Getenv("VRX_LB_HOST") != "1" {
		t.Skip("lb host test is opt-in (VRX_LB_HOST=1): every run leaves removed VIPs until the lb garbage collection (V20)")
	}
}

// lbShow runs a read-only lb show command through cli_inband and returns its output.
func lbShow(t *testing.T, c vpp.Client, cmd string) string {
	t.Helper()
	if !strings.HasPrefix(cmd, "show lb") {
		t.Fatalf("lbShow runs show lb commands only, not %q", cmd)
	}
	rep, err := vlib.NewServiceClient(c).CliInband(context.Background(), &vlib.CliInband{Cmd: cmd})
	if err != nil {
		t.Fatalf("%s: %v", cmd, err)
	}
	return rep.Reply
}

// lbHostDoc is the slot's lb document on loopback loop<N>31.
func lbHostDoc(t *testing.T) (*vrxv1.DesiredState, string, int) {
	t.Helper()
	n := vpptest.Slot(t)
	loop := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 31))
	js := fmt.Sprintf(`{
	  "interfaces": {%[1]q: {"ipv4": ["10.%[2]d.31.1/24"]}},
	  "services": {"lb": {
	    "vips": {
	      "web": {"prefix": "10.%[2]d.250.1/32", "protocol": "tcp", "port": 80, "encap": "gre4", "newFlowsTableLength": 1024,
	              "servers": [{"address": "10.%[2]d.2.10"}, {"address": "10.%[2]d.2.11", "flushOnDelete": true}]},
	      "dsr": {"prefix": "10.%[2]d.250.2/32", "protocol": "any", "encap": "l3dsr", "dscp": 10, "newFlowsTableLength": 256,
	              "servers": [{"address": "10.%[2]d.2.12"}]},
	      "dns": {"prefix": "10.%[2]d.250.3/32", "protocol": "udp", "port": 53, "encap": "nat4", "srvType": "clusterip",
	              "targetPort": 5353, "newFlowsTableLength": 1024, "servers": [{"address": "10.%[2]d.2.13"}]}
	    },
	    "natInterfaces": [{"interface": %[1]q, "family": "ip4"}]
	  }}
	}`, loop, n)
	return doc(t, js), loop, n
}

var vipLine = regexp.MustCompile(`(?m)^\s*\[\d+\]\s`)

func TestLbOnHost(t *testing.T) {
	lbOptIn(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteOwned(t, raw, owner) })
	desired, loop, n := lbHostDoc(t)
	ctx := context.Background()

	cfg := hostConfig(t, owner)
	a, err := Start(ctx, cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			a.Stop()
		}
	})
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	before := lbShow(t, raw, "show lb vips")
	t.Logf("before: show lb vips:\n%s", before)

	// 1. commit → APPLIED; VPP lists the VIPs with their ASes and encapsulation
	resp, err := c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-lb-1", DesiredState: desired})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %v", err, resp)
	}
	t.Logf("apply: %s", protojson.Format(resp.GetSummary()))
	t.Logf("after commit: show lb vips verbose:\n%s", lbShow(t, raw, "show lb vips verbose"))
	st, err := c.LbState(ctx, &vrxv1.LbStateRequest{})
	if err != nil || len(st.GetVips()) != 3 {
		t.Fatalf("LbState: %v %v", err, st)
	}
	for _, v := range st.GetVips() {
		t.Logf("LbState: %s", protojson.Format(v))
		if !v.GetApplied() || v.GetVppEntries() < 1 || len(v.GetServers()) == 0 {
			t.Fatalf("VIP %s not in VPP: %v", v.GetName(), v)
		}
	}

	// 2. flush (an IPv4 VIP: the ip46 layout)
	fr, err := c.LbFlushVip(ctx, &vrxv1.LbFlushVipRequest{Name: "web"})
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	t.Logf("flush: %s", fr.GetVip())

	// 3. agent-restart simulation: same state dir, nothing lost → re-applied without duplicates within 30 s
	entries := len(vipLine.FindAllString(lbShow(t, raw, "show lb vips"), -1))
	feat := func() string {
		idx := ownedLoop(t, raw, owner, loop)
		rep, err := vlib.NewServiceClient(raw).CliInband(ctx, &vlib.CliInband{Cmd: fmt.Sprintf("show interface %d feat", idx)})
		if err != nil {
			t.Fatal(err)
		}
		return rep.Reply
	}
	nat4 := strings.Count(feat(), "lb-nat4-in2out")
	a.Stop()
	stopped = true
	t0 := time.Now()
	a, err = Start(ctx, cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopped = false
	c = dialAgent(t, cfg.Socket)
	waitReady(t, c)
	t.Logf("restart: ready and reconciled in %s", time.Since(t0).Round(time.Millisecond))
	if got := len(vipLine.FindAllString(lbShow(t, raw, "show lb vips"), -1)); got != entries {
		t.Fatalf("restart duplicated VIPs: %d entries, before %d", got, entries)
	}
	if got := strings.Count(feat(), "lb-nat4-in2out"); got != nat4 {
		t.Fatalf("restart re-enabled the NAT feature: %d, before %d (D-076)", got, nat4)
	}

	// 4. simulated loss while the agent is down: our web VIP deleted behind its back → re-created within 30 s
	a.Stop()
	stopped = true
	web := lbd.VIP{Prefix: fmt.Sprintf("10.%d.250.1/32", n), Protocol: lbd.ProtoTCP, Port: 80}
	vd := lbd.NewVIP(raw, owner)
	if err := vd.Delete(ctx, df7.Encode(lbd.VIPSpec{VIP: web, Encap: lbd.EncapGRE4}), nil); err != nil {
		t.Fatal(err)
	}
	t0 = time.Now()
	a, err = Start(ctx, cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopped = false
	c = dialAgent(t, cfg.Socket)
	waitReady(t, c)
	st, err = c.LbState(ctx, &vrxv1.LbStateRequest{Names: []string{"web"}})
	if err != nil || !st.GetVips()[0].GetApplied() || st.GetVips()[0].GetServers()[0].GetInUse() != true {
		t.Fatalf("lost VIP not re-created: %v %v", err, st)
	}
	t.Logf("loss: re-created in %s: %s", time.Since(t0).Round(time.Millisecond), protojson.Format(st.GetVips()[0]))

	// 5. removal: delete messages; VPP keeps the VIPs as removed until a garbage collection (V20)
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-lb-2", DesiredState: doc(t, fmt.Sprintf(`{"interfaces": {%q: {"ipv4": ["10.%d.31.1/24"]}}, "services": {}}`, loop, n))})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("remove: %v %v", err, resp)
	}
	for _, r := range resp.GetResults() {
		t.Logf("remove: %s %s %s", r.GetOp(), r.GetKey(), r.GetCode())
	}
	t.Logf("after removal: show lb vips verbose:\n%s", lbShow(t, raw, "show lb vips verbose"))
	if got := strings.Count(feat(), "lb-nat4-in2out"); got != 0 {
		t.Fatalf("NAT feature still enabled: %d", got)
	}
	t.Logf("show lb:\n%s", lbShow(t, raw, "show lb"))
	resp, err = c.Apply(ctx, &vrxv1.ApplyRequest{TxnId: owner + "-lb-3", Subsystems: []string{"interfaces", "services"}})
	if err != nil || resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("cleanup apply: %v %v", err, resp)
	}
}

// ownedLoop returns the sw_if_index of this owner's loopback name.
func ownedLoop(t *testing.T, c vpp.Client, owner, name string) uint32 {
	t.Helper()
	ifs, _ := ownedOnHost(t, c, owner)
	idx, ok := ifs[name]
	if !ok {
		t.Fatalf("loopback %s of %s not found", name, owner)
	}
	return idx
}

// TestLbGarbageCollectOnHost runs VPP's lb garbage collection once (D-090) — VPP-global: opt-in VRX_LB_GLOBALS=1,
// exclusive globals lock, manager window only (D-082). It changes no lb_conf value (nothing to restore).
func TestLbGarbageCollectOnHost(t *testing.T) {
	lbOptIn(t)
	if os.Getenv("VRX_LB_GLOBALS") != "1" {
		t.Skip("the lb garbage collection is VPP-global: opt-in VRX_LB_GLOBALS=1, manager window only (D-082)")
	}
	vpptest.LockLab(t)
	lock, err := os.OpenFile("/run/lock/vrx-globals.lock", os.O_CREATE|os.O_RDWR, 0o644) //nolint:gosec // the shared globals lock of every slot (flock), readable by all
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) })
	owner := vpptest.Prefix(t)
	n := vpptest.Slot(t)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conf := lbShow(t, raw, "show lb")
	vd, ad := lbd.NewVIP(raw, owner), lbd.NewAS(raw, owner)
	vip := lbd.VIP{Prefix: fmt.Sprintf("10.%d.250.9/32", n), Protocol: lbd.ProtoTCP, Port: 8080}
	v, as := df7.Encode(lbd.VIPSpec{VIP: vip, Encap: lbd.EncapGRE4}), df7.Encode(lbd.AS{VIP: vip, Address: fmt.Sprintf("10.%d.2.19", n)})
	if _, err := vd.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if _, err := ad.Create(ctx, as); err != nil {
		t.Fatal(err)
	}
	if err := ad.Delete(ctx, as, nil); err != nil {
		t.Fatal(err)
	}
	if err := vd.Delete(ctx, v, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("removed, before GC: show lb vips verbose:\n%s", lbShow(t, raw, "show lb vips verbose"))
	t.Logf("waiting %s (LB_GARBAGE_RUN 60 s after the VIP's creation, LB_CONCURRENCY_TIMEOUT 10 s after the AS removal)", lbd.GCDelay)
	time.Sleep(lbd.GCDelay)
	if err := lbd.GarbageCollect(ctx, raw); err != nil {
		t.Fatal(err)
	}
	after := lbShow(t, raw, "show lb vips verbose")
	t.Logf("after GC: show lb vips verbose:\n%s", after)
	if strings.Contains(after, fmt.Sprintf("10.%d.250.9/32", n)) {
		t.Fatalf("the removed VIP survived the garbage collection")
	}
	if got := lbShow(t, raw, "show lb"); got[:strings.Index(got, "#vips")] != conf[:strings.Index(conf, "#vips")] {
		t.Fatalf("lb_conf changed:\n%s\nwas\n%s", got, conf)
	}
}
