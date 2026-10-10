package agent

// Host integration tests (NGFW_INTEGRATION=1, shared lab lock, slot prefix): the whole agent —
// gRPC server, reconciler, core descriptors, persistence — against the VPP on this host.
//
//	TestAgentOnHost          in-process agent: apply → Retrieve == desired → second owner cannot
//	                          touch it → stop, delete our objects via binapi, start → recreated
//	                          → restart without loss changes nothing → confirm timeout reverts
//	TestAgentProcessOnHost   the real ngfw-agent binary: kill -9 by PID, simulated loss, restart
//	                          → recreated within 30 s; kill -9 again → no VPP changes
//
// Only objects carrying the slot prefix are created (loop<N>xx, tables N000–N999 named
// "<prefix>…:", 10.<N>.0.0/16); everything is removed in t.Cleanup. VPP is never restarted.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
	"ngfw/agent/internal/vpp/vpptest"
)

func vppSocket() string {
	if p := os.Getenv("NGFW_VPP_API_SOCKET"); p != "" {
		return p
	}
	return "/run/vpp/api.sock"
}

// hostDoc is the slot's test document: a VRF, two loopbacks with addresses in it, two routes.
func hostDoc(t *testing.T, owner string) (*ngfwv1.DesiredState, *ngfwv1.DesiredState) {
	t.Helper()
	slot := vpptest.Slot(t)
	table := vpptest.TableBase(t) + 1
	l1 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 1))
	l2 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 2))
	vrf := owner + "-red"
	js := fmt.Sprintf(`{
	  "system": {"hostname": "ngfw-a"},
	  "vrfs": {"default": {"id": 0}, %[1]q: {"id": %[2]d}},
	  "interfaces": {
	    %[3]q: {"vrf": %[1]q, "ipv4": ["10.%[5]d.1.1/24"], "ipv6": ["2001:db8:%[5]d::1/64"]},
	    %[4]q: {"vrf": %[1]q, "ipv4": ["10.%[5]d.2.1/24"]}
	  },
	  "routing": {"static": [
	    {"prefix": "10.%[5]d.100.0/24", "vrf": %[1]q, "distance": 1, "nextHops": [{"address": "10.%[5]d.1.254", "weight": 1}]},
	    {"prefix": "10.%[5]d.101.0/24", "vrf": %[1]q, "nextHops": [{"address": "10.%[5]d.2.254", "interface": %[4]q, "weight": 1}]}
	  ]}
	}`, vrf, table, l1, l2, slot)
	// Retrieve (desired.Assemble, P08) always reports admin state and promiscuous mode; every implemented domain is
	// present. System identity reports its normalized defaults; DHCP and QoS report empty services.
	canon := fmt.Sprintf(`{
	  "system": {"hostname": "ngfw-a", "timezone": "UTC", "banner": {}, "dns": {"vrf": "default"}},
	  "vrfs": {%[1]q: {"id": %[2]d}},
	  "interfaces": {
	    %[3]q: {"enabled": false, "promiscuous": false, "vrf": %[1]q, "ipv4": ["10.%[5]d.1.1/24"], "ipv6": ["2001:db8:%[5]d::1/64"]},
	    %[4]q: {"enabled": false, "promiscuous": false, "vrf": %[1]q, "ipv4": ["10.%[5]d.2.1/24"]}
	  },
	  "routing": {"static": [
	    {"prefix": "10.%[5]d.100.0/24", "vrf": %[1]q, "distance": 1, "blackhole": false, "nextHops": [{"address": "10.%[5]d.1.254", "weight": 1}]},
	    {"prefix": "10.%[5]d.101.0/24", "vrf": %[1]q, "blackhole": false, "nextHops": [{"address": "10.%[5]d.2.254", "interface": %[4]q, "weight": 1}]}
	  ]},
	  "services": {"dhcp": {}, "qos": {}}
	}`, vrf, table, l1, l2, slot)
	return doc(t, js), doc(t, canon)
}

// dialAgent returns a gRPC client for the agent at sock.
func dialAgent(t *testing.T, sock string) ngfwv1.DataplaneClient {
	t.Helper()
	cc, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	return ngfwv1.NewDataplaneClient(cc)
}

// waitReady waits until the agent answers Health with VPP connected and a finished reconcile.
func waitReady(t *testing.T, c ngfwv1.DataplaneClient) *ngfwv1.HealthResponse {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last *ngfwv1.HealthResponse
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		h, err := c.Health(ctx, &ngfwv1.HealthRequest{})
		last, lastErr = h, err
		cancel()
		if err == nil && h.GetVppConnected() && h.GetLastReconcileAt() != nil && !h.GetReconcileInProgress() {
			return h
		}
		time.Sleep(100 * time.Millisecond)
	}
	stacks := make([]byte, 1<<20)
	n := runtime.Stack(stacks, true)
	for _, stack := range strings.Split(string(stacks[:n]), "\n\n") {
		if strings.Contains(stack, "(*Service).syncLocked") {
			t.Logf("in-progress dynamic reconciliation stack:\n%s", stack)
		}
	}
	t.Fatalf("agent not ready within 30 s: last health=%s error=%v", protojson.Format(last), lastErr)
	return nil
}

// waitConverged polls Retrieve until it equals want (≤ 30 s) and returns the time it took.
func waitConverged(t *testing.T, c ngfwv1.DataplaneClient, want *ngfwv1.DesiredState, since time.Time) time.Duration {
	t.Helper()
	var last *ngfwv1.DesiredState
	for time.Since(since) < 30*time.Second {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		got, err := c.Retrieve(ctx, &ngfwv1.RetrieveRequest{})
		cancel()
		if err == nil {
			last = got.GetDesiredState()
			if proto.Equal(last, want) {
				return time.Since(since)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("not converged within 30 s:\n got %s\nwant %s", protojson.Format(last), protojson.Format(want))
	return 0
}

// ownedOnHost lists, via binapi, this owner's loopbacks (name → sw_if_index) and tables.
func ownedOnHost(t *testing.T, c vpp.Client, owner string) (map[string]uint32, map[uint32]bool) {
	t.Helper()
	ctx := context.Background()
	ifs := map[string]uint32{}
	st, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if err != nil {
		t.Fatal(err)
	}
	for {
		d, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if id, ok := vpp.ParseOwnerTag(d.Tag, owner); ok {
			ifs[id] = uint32(d.SwIfIndex)
		}
	}
	tables := map[uint32]bool{}
	ts, err := ip.NewServiceClient(c).IPTableDump(ctx, &ip.IPTableDump{})
	if err != nil {
		t.Fatal(err)
	}
	for {
		d, err := ts.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := vpp.ParseOwnerTag(strings.TrimRight(d.Table.Name, "\x00"), owner); ok {
			tables[d.Table.TableID] = true
		}
	}
	return ifs, tables
}

// deleteOwned simulates loss (or cleans up): deletes this owner's loopbacks and tables via binapi.
func deleteOwned(t *testing.T, c vpp.Client, owner string) int {
	t.Helper()
	ctx := context.Background()
	ifs, tables := ownedOnHost(t, c, owner)
	n := 0
	for name, idx := range ifs {
		// D-095 c: the interface's bindings go first (VPP keeps them on the freed index, V19)
		if err := ifsanitize.BeforeDelete(ctx, c, uint32(idx), name); err != nil {
			t.Errorf("clear %s before delete: %v", name, err)
		}
		if _, err := interfaces.NewServiceClient(c).DeleteLoopback(ctx, &interfaces.DeleteLoopback{SwIfIndex: interface_types.InterfaceIndex(idx)}); err != nil {
			t.Errorf("delete %s: %v", name, err)
		}
		n++
	}
	for id := range tables {
		for _, v6 := range []bool{false, true} {
			// flush first: deleting a table that still holds API drop routes leaks them into the
			// next table that reuses the FIB index (seen on VPP 26.06, P05 review round)
			_, _ = ip.NewServiceClient(c).IPTableFlush(ctx, &ip.IPTableFlush{Table: ip.IPTable{TableID: id, IsIP6: v6}})
			if _, err := ip.NewServiceClient(c).IPTableAddDel(ctx, &ip.IPTableAddDel{IsAdd: false, Table: ip.IPTable{TableID: id, IsIP6: v6}}); err != nil {
				t.Errorf("delete table %d: %v", id, err)
			}
		}
		n++
	}
	return n
}

func hostConfig(t *testing.T, owner string) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		Socket: filepath.Join(dir, "agent.sock"), VPPAPISocket: vppSocket(), VPPStatsSocket: vpptest.StatsSocket(),
		StateDir: filepath.Join(dir, "state"), Owner: owner, MetricsAddr: "off",
	}
}

func TestAgentOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
	vpptest.LockLab(t)
	owner := vpptest.Prefix(t)
	raw := vpp.Dial(vppSocket(), vpp.ConnOptions{})
	t.Cleanup(raw.Close)
	wctx, wcancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer wcancel()
	if err := raw.WaitConnected(wctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteOwned(t, raw, owner); deleteOwned(t, raw, owner+"b") })

	cfg := hostConfig(t, owner)
	a, err := Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a != nil {
			a.Stop()
		}
	})
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	desired, canonical := hostDoc(t, owner)
	ctx := context.Background()

	// 1. apply → APPLIED, Retrieve == desired (canonical form).
	resp, err := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-it-1", DesiredState: desired})
	if err != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %v", err, resp)
	}
	t.Logf("apply: %s", protojson.Format(resp.GetSummary()))
	waitConverged(t, c, canonical, time.Now())
	resp, err = c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-it-2", DesiredState: desired})
	// 13 = system identity + VRF + 2 loopbacks + 2 interface/<name> aliases (P08, D-065)
	// + 2 table bindings + 3 addresses + 2 routes.
	if err != nil || len(resp.GetResults()) != 0 || resp.GetSummary().GetUnchanged() != 13 {
		t.Fatalf("idempotent apply: %v %v", err, resp)
	}
	ifsBefore, _ := ownedOnHost(t, raw, owner)

	// 2. a second agent with another owner on the same VPP sees nothing and deletes nothing.
	cfgB := hostConfig(t, owner+"b")
	b, err := Start(context.Background(), cfgB, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if b != nil {
			b.Stop()
		}
	})
	cb := dialAgent(t, cfgB.Socket)
	waitReady(t, cb)
	if got, err := cb.Retrieve(ctx, &ngfwv1.RetrieveRequest{}); err != nil || len(got.GetDesiredState().GetInterfaces())+len(got.GetDesiredState().GetVrfs())+len(got.GetDesiredState().GetRouting().GetStatic()) != 0 {
		t.Fatalf("owner %sb sees %v %v", owner, err, got)
	}
	rb, err := cb.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "b-empty", Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil || rb.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED || len(rb.GetResults()) != 0 {
		t.Fatalf("owner %sb authoritative-empty apply: %v %v", owner, err, rb)
	}
	if _, err := cb.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: "x", Owner: owner}); err == nil {
		t.Fatal("agent b accepted a request for owner a")
	}
	b.Stop()
	b = nil
	waitConverged(t, c, canonical, time.Now())

	// 3. simulated loss: stop the agent, delete our objects via binapi, start → recreated.
	a.Stop()
	a = nil
	if n := deleteOwned(t, raw, owner); n != 3 {
		t.Fatalf("deleted %d objects, want 2 loopbacks + 1 VRF", n)
	}
	start := time.Now()
	a, err = Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c = dialAgent(t, cfg.Socket)
	took := waitConverged(t, c, canonical, start)
	t.Logf("restart after loss: converged in %v", took)

	// 4. restart without loss: no VPP change (same sw_if_index values, nothing recreated).
	ifsMid, _ := ownedOnHost(t, raw, owner)
	a.Stop()
	a = nil
	a, err = Start(context.Background(), cfg, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c = dialAgent(t, cfg.Socket)
	waitReady(t, c)
	ifsAfter, _ := ownedOnHost(t, raw, owner)
	for name, idx := range ifsMid {
		if ifsAfter[name] != idx {
			t.Fatalf("%s changed sw_if_index %d → %d on a converged restart", name, idx, ifsAfter[name])
		}
	}
	_ = ifsBefore

	// 5. confirm timeout reverts on its own (rollback removes what the pending txn added).
	evs, err := c.StreamEvents(ctx, &ngfwv1.StreamEventsRequest{Kinds: []ngfwv1.EventKind{ngfwv1.EventKind_EVENT_KIND_CONFIRM_REVERTED, ngfwv1.EventKind_EVENT_KIND_RECONCILE_DONE}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	extra := proto.Clone(desired).(*ngfwv1.DesiredState)
	l3 := fmt.Sprintf("loop%d", vpptest.LoopbackInstance(t, 3))
	extra.Interfaces[l3] = &ngfwv1.Interface{Ipv4: []string{fmt.Sprintf("10.%d.3.1/24", vpptest.Slot(t))}}
	resp, err = c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-it-3", DesiredState: extra, ConfirmTimeoutSec: 2})
	if err != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED || resp.GetConfirmDeadline() == nil {
		t.Fatalf("pending apply: %v %v", err, resp)
	}
	if ifs, _ := ownedOnHost(t, raw, owner); ifs[l3] == 0 {
		t.Fatalf("%s not created", l3)
	}
	var seen []string
	for len(seen) < 3 {
		e, err := evs.Recv()
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, e.GetKind().String()+":"+e.GetTxnId())
	}
	if want := "EVENT_KIND_RECONCILE_DONE:" + owner + "-it-3,EVENT_KIND_CONFIRM_REVERTED:" + owner + "-it-3,EVENT_KIND_RECONCILE_DONE:"; strings.Join(seen, ",") != want {
		t.Fatalf("events %v", seen)
	}
	if ifs, _ := ownedOnHost(t, raw, owner); ifs[l3] != 0 {
		t.Fatalf("%s survived the confirm revert", l3)
	}
	waitConverged(t, c, canonical, time.Now())

	// 6. remove everything through the agent.
	resp, err = c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-it-4", Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED || resp.GetSummary().GetDeleted() != 10 {
		t.Fatalf("delete all: %v %v", err, resp)
	}
	a.Stop()
	a = nil
	if ifs, tables := ownedOnHost(t, raw, owner); len(ifs)+len(tables) != 0 {
		t.Fatalf("left over: %v %v", ifs, tables)
	}
}

// TestAgentProcessOnHost runs the real binary: kill -9 by PID, simulated loss, restart.
func TestAgentProcessOnHost(t *testing.T) {
	vpptest.SkipUnlessIntegration(t)
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

	bin := filepath.Join(t.TempDir(), "ngfw-agent")
	build := exec.Command("go", "build", "-o", bin, "ngfw/agent/cmd/ngfw-agent") //nolint:gosec // fixed argv, test only
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cfg := hostConfig(t, owner)
	logPath := filepath.Join(filepath.Dir(cfg.StateDir), "agent.log")
	start := func() *exec.Cmd {
		cmd := exec.Command(bin) //nolint:gosec // our own binary
		cmd.Env = append(os.Environ(),
			"NGFW_AGENT_SOCKET="+cfg.Socket, "NGFW_OWNER="+owner, "NGFW_AGENT_STATE_DIR="+cfg.StateDir,
			"NGFW_AGENT_VPP_API_SOCKET="+cfg.VPPAPISocket, "NGFW_METRICS_ADDR=off", "NGFW_SOCKET_GROUP=")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout, cmd.Stderr = f, f
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
				_ = cmd.Wait()
			}
		})
		return cmd
	}
	kill9 := func(cmd *exec.Cmd) {
		t.Helper()
		if err := cmd.Process.Kill(); err != nil { // SIGKILL to the PID we spawned, never by name
			t.Fatal(err)
		}
		_ = cmd.Wait()
		t.Logf("kill -9 %d", cmd.Process.Pid)
	}
	ctx := context.Background()
	desired, canonical := hostDoc(t, owner)

	p := start()
	c := dialAgent(t, cfg.Socket)
	waitReady(t, c)
	resp, err := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-proc-1", DesiredState: desired})
	if err != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply: %v %v", err, resp)
	}
	waitConverged(t, c, canonical, time.Now())

	kill9(p)
	if n := deleteOwned(t, raw, owner); n != 3 {
		t.Fatalf("deleted %d objects", n)
	}
	t0 := time.Now()
	p = start()
	c = dialAgent(t, cfg.Socket) // the stale socket of the killed process is replaced
	t.Logf("restart after kill -9 + loss: converged in %v", waitConverged(t, c, canonical, t0))

	ifsBefore, _ := ownedOnHost(t, raw, owner)
	kill9(p)
	p = start()
	c = dialAgent(t, cfg.Socket)
	waitReady(t, c)
	ifsAfter, _ := ownedOnHost(t, raw, owner)
	for name, idx := range ifsBefore {
		if ifsAfter[name] != idx {
			t.Fatalf("kill -9 + restart changed %s (%d → %d)", name, idx, ifsAfter[name])
		}
	}
	resp, err = c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: owner + "-proc-2", Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil || resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("cleanup apply: %v %v", err, resp)
	}
	_ = p.Process.Signal(syscall.SIGTERM)
	_ = p.Wait()
	if b, err := os.ReadFile(logPath); err == nil { //nolint:gosec // test temp dir
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "reconcile done") || strings.Contains(l, "resync finished") {
				t.Log(l)
			}
		}
	}
}
